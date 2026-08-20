package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestEveryRouteConstantIsNamedInTheRouteTable is the guard for routeFor's
// own reason to exist: every route*/prefix* constant declared in
// webroutes.go must resolve, through routeFor, to a label distinct from
// every other one. routeFor is the ONE table ServeHTTP and the metrics it
// records both read — see its doc comment — and this test is what stops a
// ninth route being added to webroutes.go's const block without a matching
// case ever landing in routeFor, which would otherwise mislabel that
// route's requests under "notfound" or another route's name silently.
//
// The idiom matches TestEnvSwitchListIsComplete in
// internal/snapshot/fingerprint_test.go: parse the source, count what it
// declares, and check it against what the code that is SUPPOSED to know
// about all of them actually resolves — rather than trusting a comment to
// stay in sync with the const block by hand.
func TestEveryRouteConstantIsNamedInTheRouteTable(t *testing.T) {
	src, err := os.ReadFile("webroutes.go")
	if err != nil {
		t.Fatalf("reading webroutes.go: %v", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "webroutes.go", src, 0)
	if err != nil {
		t.Fatalf("parsing webroutes.go: %v", err)
	}

	nameRe := regexp.MustCompile(`^(route|prefix)[A-Z]`)
	values := map[string]string{} // constant name -> its string value
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !nameRe.MatchString(name.Name) || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquoting %s's value %s: %v", name.Name, lit.Value, err)
				}
				values[name.Name] = val
			}
		}
	}
	if len(values) == 0 {
		t.Fatal("found no route*/prefix* constants in webroutes.go — did the const block move or rename?")
	}

	s := &squadServer{}
	labels := map[string]string{} // label -> the first constant that resolved to it
	for name, path := range values {
		_, label := s.routeFor(path)
		if label == "" || label == "notfound" {
			t.Errorf("routeFor(%q) (from %s) resolved to %q — every declared route/prefix "+
				"constant must have its own case in routeFor", path, name, label)
			continue
		}
		if prior, seen := labels[label]; seen {
			t.Errorf("%s and %s both resolve through routeFor to label %q — "+
				"the metrics they record would be indistinguishable", prior, name, label)
			continue
		}
		labels[label] = name
	}
}

// TestEveryRenderLockGoesThroughLockRender pins that squadServer.mu is taken
// through lockRender and nowhere else. lockRender both serialises renders
// AND records armband_render_mutex_wait_seconds; a `s.mu.Lock()` added
// anywhere else would take the lock correctly and silently miss the metric,
// which is exactly the kind of drift a source scan catches and a runtime
// test would not (the lock would still work).
//
// lockRender's own body is the one place `s.mu.Lock()` is allowed to
// appear literally, so it is excluded by name rather than by counting: the
// alternative — asserting an exact total count across the package — breaks
// every time an unrelated call site is added or removed.
func TestEveryRenderLockGoesThroughLockRender(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading cmd/armband: %v", err)
	}

	fset := token.NewFileSet()
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			// lockRender's own body is where s.mu.Lock() is meant to live.
			if fd.Name.Name == "lockRender" {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Lock" {
					return true
				}
				// The receiver must read "s.mu" (or "<x>.mu") specifically —
				// this is squadServer.mu, not some other mutex this package
				// might one day hold.
				inner, ok := sel.X.(*ast.SelectorExpr)
				if !ok || inner.Sel.Name != "mu" {
					return true
				}
				pos := fset.Position(call.Pos())
				t.Errorf("%s:%d: s.mu.Lock() called directly in %s — every render lock "+
					"must go through lockRender so the wait is recorded",
					name, pos.Line, fd.Name.Name)
				return true
			})
		}
	}
}
