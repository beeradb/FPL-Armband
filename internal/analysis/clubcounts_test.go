package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"testing"
)

// tallyClubs counts a player list by club the long way.
//
// Deliberately a second implementation, local to this file: the point of the
// assertions below is that the map Optimize reports agrees with an independent
// count of the players it returned, and calling clubCountsOf here would make
// that a tautology. This is the one place in the package where a duplicated
// tally is the test rather than the defect — see clubCountsOf's own comment for
// why the shipped code must have exactly one.
func tallyClubs(players []PlayerMetrics) map[string]int {
	got := map[string]int{}
	for _, p := range players {
		got[p.Team]++
	}
	return got
}

// TestClubCountsDescribeTheReturnedSquad pins the reporting field to the squad
// actually returned.
//
// Squad.ClubCounts used to be the greedy fill's incremental `clubCount` map,
// whose only writer is the fill loop's add() — so the two stages that run after
// it, polish's steepest-ascent swaps and the DP seeds, replaced players without
// it noticing. On the committed capture that shipped a squad holding ARS×3,
// MUN×2, CHE×2 alongside a map reading ARS 0, MUN 0, CHE 1 and naming LEE×2,
// SUN, BOU and NEW, none of whom were in the squad at all.
//
// The bug class is "the reported map does not match the actual squad", so this
// asserts EQUALITY against an independent tally. The existing club assertions in
// squad_test.go and squadclubtrap_test.go check only `n <= MaxPerClub`, which
// the wrong map satisfied — it summed to fifteen and obeyed the cap, it just
// described a different fifteen — so neither of them ever saw this.
//
// Consumers: internal/agent/tools.go serves the map as tool JSON, which is
// replayed into every subsequent API call, so the agent's reasoning about club
// exposure was built on it; internal/viewmodel/build.go hands it to the web
// planner for the max-three rule the pitch enforces.
func TestClubCountsDescribeTheReturnedSquad(t *testing.T) {
	e := testEngine(t)
	skipDuringLiveGW1Gap(t, e)

	// runFillLoop (squadfillbounddiff_test.go) drives the real Optimize through
	// the shipped bound and captures the greedy seed on the way past, so the
	// check below can show this call really does exercise the post-seed stages
	// rather than passing because the seed happened to survive intact.
	seed, sq, err := runFillLoop(t, e, OptimizeRequest{MinMinutes: 500}, shippedBound)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	final := map[int]bool{}
	for _, p := range sq.Players {
		final[p.ID] = true
	}
	kept := 0
	for _, id := range seed.ids {
		if final[id] {
			kept++
		}
	}
	if kept == len(seed.ids) {
		// Not a failure of the fix — a failure of this test's power, and worth
		// failing loudly for. If polish and the DP seeds ever hand back the
		// greedy fifteen unchanged here, the stale map and a fresh tally agree
		// by accident and the assertion below cannot fail. That is not
		// hypothetical: it is exactly what squadclubtrap_test.go's synthetic
		// pool does (15 of 15 kept), which is why this guard is behavioural on
		// the capture and a source scan below, rather than behavioural twice.
		t.Errorf("polish and the DP seeds returned the greedy seed unchanged (%d of %d kept), "+
			"so this fixture no longer separates the seed's club counts from the final "+
			"squad's — the assertion below cannot fail and needs a new fixture", kept, len(seed.ids))
	}
	t.Logf("greedy seed kept %d of %d players through polish and the DP seeds", kept, len(seed.ids))

	want := tallyClubs(sq.Players)
	if !maps.Equal(sq.ClubCounts, want) {
		t.Errorf("ClubCounts = %v, want %v (tallied from the fifteen actually returned)",
			sq.ClubCounts, want)
	}

	// Spelled out separately because the swaps land on the bench more often
	// than the eleven: the map must cover the whole roster, not just the XI.
	if n := len(sq.StartingXI) + len(sq.Bench); n != SquadSize {
		t.Fatalf("XI + bench = %d, want %d", n, SquadSize)
	}
	roster := append(append([]PlayerMetrics(nil), sq.StartingXI...), sq.Bench...)
	if !maps.Equal(sq.ClubCounts, tallyClubs(roster)) {
		t.Errorf("ClubCounts = %v does not match the XI plus the bench %v",
			sq.ClubCounts, tallyClubs(roster))
	}
}

// TestOptimizeReportsClubCountsFromTheFinalSquad is the source-scan half.
//
// It exists because the behavioural test above draws its power from the
// committed capture producing a squad that drifts from its greedy seed. It does
// today — 6 of 15 survive — but that is a property of the fixture, not of the
// code, and a refreshed capture could quietly take the teeth out. This half
// cannot rot that way.
//
// A synthetic-pool version was written first and deliberately NOT shipped: on
// squadclubtrap_test.go's fixture the search returns the greedy fifteen intact,
// so the stale map and a correct tally agree and the test passes on the UNFIXED
// code. Green because the path could not carry the effect, which reads as
// coverage and is not.
//
// A scan is the right instrument anyway: the defect was never that the tally
// was wrong, it was that the wrong map was handed over. Same instrument as
// TestTheTransferPoolScalesItsMinutesFloor and
// TestTheHitCeilingIsReadByTheFundedPairBranch.
func TestOptimizeReportsClubCountsFromTheFinalSquad(t *testing.T) {
	// Parsed rather than string-matched. The doc comment on this very field
	// spells out `ClubCounts: clubCount` verbatim to explain the bug, so a
	// textual scan would match the prose describing the defect; and a
	// hand-rolled comment stripper mangles string literals containing "//".
	// The AST has neither problem.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "squad.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing squad.go: %v", err)
	}

	found := 0
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "Squad" {
			return true
		}
		for _, el := range lit.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "ClubCounts" {
				continue
			}
			found++
			call, ok := kv.Value.(*ast.CallExpr)
			if !ok {
				t.Errorf("%s: Squad.ClubCounts is set from %T, not a call to clubCountsOf. "+
					"Any club map the search maintained is stale by the time this struct is "+
					"built: the greedy fill's `clubCount` stops being written when the fill "+
					"loop ends, and polish and the DP seeds replace players afterwards.",
					fset.Position(kv.Pos()), kv.Value)
				continue
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok || fn.Name != "clubCountsOf" {
				t.Errorf("%s: Squad.ClubCounts is built by something other than clubCountsOf. "+
					"It must be tallied from the fifteen being returned.",
					fset.Position(kv.Pos()))
			}
		}
		return true
	})

	// Without this the scan passes vacuously the day someone renames the type
	// or builds the struct field-by-field.
	if found == 0 {
		t.Fatal("no `Squad{... ClubCounts: ...}` literal found in squad.go — this guard " +
			"is no longer looking at anything and needs updating to wherever the field is set")
	}
}
