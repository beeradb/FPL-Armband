package webui

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestStripJSOnlyRemovesComments runs the same structural check strip_test.go uses for
// CSS and HTML against every .js file this project ships: the only difference between raw
// and stripJS(raw) is deleted // and /* */ spans (with a /*! license block or a
// sourceMappingURL/sourceURL directive left in place, showing up as no divergence).
func TestStripJSOnlyRemovesComments(t *testing.T) {
	sub, err := fs.Sub(assets, "assets/static")
	if err != nil {
		t.Fatal(err)
	}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !bytes.HasSuffix([]byte(p), []byte(".js")) {
			return err
		}
		raw, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		t.Run(p, func(t *testing.T) {
			assertOnlyCommentSpansRemoved(t, raw, stripJS(raw), []delimPair{{"/*", "*/"}, {"//", ""}})
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestStripJSPassesNodeCheck asks a real JS engine whether every stripped static asset is
// still syntactically valid -- the empirical check requirement 2 of the strip-comments
// work asked for. It skips, rather than fails, when node is not on PATH: a missing engine
// is an absent tool, the same call browsertest.Find makes for a missing browser, not
// evidence the repository is broken. TestStripJSOnlyRemovesComments above and the
// synthetic cases below do not depend on node being present.
func TestStripJSPassesNodeCheck(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found on PATH; skipping the empirical syntax check")
	}
	sub, err := fs.Sub(assets, "assets/static")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	walkErr := fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !bytes.HasSuffix([]byte(p), []byte(".js")) {
			return err
		}
		raw, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		t.Run(p, func(t *testing.T) {
			out := filepath.Join(dir, filepath.Base(p))
			if err := os.WriteFile(out, stripJS(raw), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(node, "--check", out)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("node --check %s (stripped): %v\n%s", p, err, output)
			}
		})
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}

// TestStripJSPreservesStringsRegexAndTemplates is requirement 2's own list, plus the
// codebase's one real case: analytics.js has a plain string carrying a comment delimiter
// ('https://www.googletagmanager.com/...'), and app.js has three regex literals whose own
// '/' a division-blind scanner cannot tell from a comment or a divide.
func TestStripJSPreservesStringsRegexAndTemplates(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "string containing // is not a line comment",
			src:  `loader.src = 'https://www.googletagmanager.com/gtag/js?id=' + id;`,
			want: `loader.src = 'https://www.googletagmanager.com/gtag/js?id=' + id;`,
		},
		{
			name: "double-quoted string containing /* is not a block comment",
			src:  `const s = "look: /* not a comment */ still a string";`,
			want: `const s = "look: /* not a comment */ still a string";`,
		},
		{
			name: "line comment after code is removed, code kept",
			src:  "const a=1; // trailing note\nconst b=2;",
			want: "const a=1; \nconst b=2;",
		},
		{
			name: "line comment containing a URL is removed entirely",
			src:  "// see https://example.com/path\nconst a=1;",
			want: "\nconst a=1;",
		},
		{
			name: "regex literal survives, division still divides",
			src:  `const m=/^replace-(\d+)$/.exec(hash); const q=a/b/c;`,
			want: `const m=/^replace-(\d+)$/.exec(hash); const q=a/b/c;`,
		},
		{
			name: "regex following an operator, not after an identifier",
			src:  `if(x)return/^a$/.test(y);`,
			want: `if(x)return/^a$/.test(y);`,
		},
		{
			name: "regex directly after a control paren's close, not a division",
			src:  `if(x)/^a$/.test(y);`,
			want: `if(x)/^a$/.test(y);`,
		},
		{
			name: "division after a call's close, not mistaken for a regex",
			src:  `const q=foo(x)/2;`,
			want: `const q=foo(x)/2;`,
		},
		{
			name: "template literal with a substitution and a URL-shaped string inside it",
			src:  "const u=`${base}/api?next=${encodeURIComponent('http://x/y')}`;",
			want: "const u=`${base}/api?next=${encodeURIComponent('http://x/y')}`;",
		},
		{
			name: "comment inside a template substitution is removed",
			src:  "const u=`x${/* which one */ a}y`;",
			want: "const u=`x${ a}y`;",
		},
		{
			name: "nested template literal inside a substitution",
			src:  "const u=`outer${`inner${a}`}end`;",
			want: "const u=`outer${`inner${a}`}end`;",
		},
		{
			name: "use strict directive is a string, not touched",
			src:  "'use strict';\nconst a=1;",
			want: "'use strict';\nconst a=1;",
		},
		{
			name: "license block is preserved",
			src:  "/*! license text */\nconst a=1;",
			want: "/*! license text */\nconst a=1;",
		},
		{
			name: "sourceMappingURL directive is preserved",
			src:  "const a=1;\n//# sourceMappingURL=app.js.map",
			want: "const a=1;\n//# sourceMappingURL=app.js.map",
		},
		{
			name: "ordinary block comment between statements is removed",
			src:  "const a=1;/* explain b */const b=2;",
			want: "const a=1;const b=2;",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(stripJS([]byte(c.src)))
			if got != c.want {
				t.Errorf("stripJS(%q)\n  got:  %q\n  want: %q", c.src, got, c.want)
			}
		})
	}
}
