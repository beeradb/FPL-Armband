package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"armband/internal/backtest"
	"armband/internal/config"
)

// TestTheOptionValueBlockReachesTheReplay is the arrival guard for the whole
// `option_value` config block.
//
// # The failure it refuses
//
// A `config.Config` field with no line in the `SimConfig` mapping is a setting
// that never arrives. The user turns a lever on, runs `armband backtest`, and gets
// a **byte-identical result from a knob the replay never saw** — which reads as
// "the lever does nothing" and is the standing trap this project checks mediators
// for. It is not hypothetical here: the whole block shipped with no consumer at
// all, found in review, and `BankLookahead` and `PrepareForChips` already carry a
// paragraph at their call site about the same thing.
//
// # Two halves, and the second is the one that rots
//
// The behavioural half runs the mapping and checks every field landed. The
// SOURCE half is the one that catches the next field: it scans
// `applyOptionValue`'s body and requires every `OptionValuePolicy` field name to
// appear in it, so a tenth field added to the config type and forgotten here
// fails rather than going quiet. Same technique, and the same reason, as
// `TestTheHitCeilingIsReadByTheFundedPairBranch`.
func TestTheOptionValueBlockReachesTheReplay(t *testing.T) {
	// Every lever on, every constant distinguishable, so a value landing in a
	// neighbour's field is visible. Same argument as `sampleRow`'s.
	p := config.OptionValuePolicy{
		Pricing:                config.Default().OptionValue.Pricing,
		TaperFreeTransferValue: true,
		Wildcard:               config.ChipTrigger{Enabled: true, Bar: 11},
		BenchBoost:             config.ChipTrigger{Enabled: true, Bar: 13},
		FreeHit:                config.ChipTrigger{Enabled: true, Bar: 17},
	}
	p.Pricing.HalfLife = 6
	p.Pricing.CongestionSensitivity = 0.7
	p.Pricing.CongestionHorizon = 4

	var sc backtest.SimConfig
	applyOptionValue(&sc, p)

	if sc.OptionPricing != p.Pricing {
		t.Errorf("the pricing curve did not arrive: %+v, want %+v",
			sc.OptionPricing, p.Pricing)
	}
	if !sc.TaperFreeTransferValue {
		t.Errorf("the free-transfer taper did not arrive")
	}
	for _, c := range []struct {
		name string
		on   bool
		bar  float64
		want float64
	}{
		{"wildcard", sc.WildcardTrigger, sc.WildcardReservation, 11},
		{"bench boost", sc.BenchBoostTrigger, sc.BenchBoostBar, 13},
		{"free hit", sc.FreeHitTrigger, sc.FreeHitBar, 17},
	} {
		if !c.on {
			t.Errorf("the %s trigger did not arrive", c.name)
		}
		if c.bar != c.want {
			t.Errorf("the %s bar is %v, want %v — a bar landing in a "+
				"neighbour's field is silent on any check that only asks whether "+
				"the switch arrived", c.name, c.bar, c.want)
		}
	}

	// And the shipped defaults turn nothing on, so the mapping itself cannot be
	// the thing that enables a lever. Field by field rather than by struct
	// equality, because SimConfig contains analysis.Weights and is not comparable
	// — and listing them is not a second hand-written list, because the SOURCE
	// half below is what catches a field this forgets.
	var got backtest.SimConfig
	applyOptionValue(&got, config.Default().OptionValue)
	if got.TaperFreeTransferValue || got.WildcardTrigger ||
		got.BenchBoostTrigger || got.FreeHitTrigger {
		t.Errorf("mapping the shipped defaults turned a lever on: %+v", got)
	}
	if got.OptionPricing != config.Default().OptionValue.Pricing {
		t.Errorf("the shipped curve did not arrive: %+v", got.OptionPricing)
	}

	// The source half.
	src, err := os.ReadFile("optionvalue.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "optionvalue.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "applyOptionValue" || fd.Body == nil {
			continue
		}
		body = string(src[fset.Position(fd.Body.Pos()).Offset:fset.Position(fd.Body.End()).Offset])
	}
	if body == "" {
		t.Fatal("no applyOptionValue in cmd/armband — the guard is following a " +
			"seam that has been renamed or removed, so update it deliberately " +
			"rather than letting it pass vacuously")
	}
	// The field names are read off the type rather than listed, so a field added
	// to config.OptionValuePolicy is covered without anybody remembering to add
	// it here. That is the whole point: a hand-written list is a second place to
	// forget.
	for _, name := range optionPolicyFields(t) {
		if !strings.Contains(body, "p."+name) {
			t.Errorf("applyOptionValue never reads OptionValuePolicy.%s.\n\n"+
				"A config field with no line in this mapping is a setting that "+
				"never arrives: the user turns it on, replays, and gets a "+
				"byte-identical result from a knob the replay never saw. That is "+
				"exactly how the whole block shipped with no consumer.", name)
		}
	}
}

// optionPolicyFields lists config.OptionValuePolicy's exported field names, read
// from the source rather than from reflection.
//
// Reflection would work and would be worse here: the guard above is about SOURCE
// text, so reading the type the same way keeps both halves looking at one thing.
// It also means a field renamed in the config type is reported as missing from the
// mapping, which is the correct reading.
func optionPolicyFields(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("../../internal/config/optionvalue.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "optionvalue.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "OptionValuePolicy" {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, f := range st.Fields.List {
			for _, id := range f.Names {
				if id.IsExported() {
					out = append(out, id.Name)
				}
			}
		}
		return false
	})
	if len(out) == 0 {
		t.Fatal("no exported fields found on config.OptionValuePolicy; the guard " +
			"is reading the wrong type and would pass vacuously")
	}
	return out
}

// TestAnUnhonourableLeverIsReportedNotHidden pins the other half of the arrival
// rule: the three chip state rules have no live consumer, and a live command must
// SAY so rather than run the shipped behaviour in silence.
//
// A limitation stated is a different failure from a limitation hidden. The rule
// this project keeps paying for is that a setting which quietly means nothing is
// indistinguishable from one that means what it says.
func TestAnUnhonourableLeverIsReportedNotHidden(t *testing.T) {
	if n := noteUnhonouredLevers(config.Default().OptionValue); n != "" {
		t.Errorf("the shipped config produced a warning: %q", n)
	}
	// The taper alone is honoured live, so it must NOT warn.
	taper := config.Default().OptionValue
	taper.TaperFreeTransferValue = true
	if n := noteUnhonouredLevers(taper); n != "" {
		t.Errorf("the taper warned, and it reaches every live consumer: %q", n)
	}
	for _, c := range []struct {
		name string
		set  func(*config.OptionValuePolicy)
	}{
		{"wildcard", func(p *config.OptionValuePolicy) { p.Wildcard.Enabled = true }},
		{"bench_boost", func(p *config.OptionValuePolicy) { p.BenchBoost.Enabled = true }},
		{"free_hit", func(p *config.OptionValuePolicy) { p.FreeHit.Enabled = true }},
	} {
		p := config.Default().OptionValue
		c.set(&p)
		n := noteUnhonouredLevers(p)
		if n == "" {
			t.Errorf("the %s trigger is on and no live command says it cannot be "+
				"honoured — that is the silent null this block exists to price",
				c.name)
		}
		if !strings.Contains(n, c.name) {
			t.Errorf("the warning for %s does not name it: %q", c.name, n)
		}
	}
}
