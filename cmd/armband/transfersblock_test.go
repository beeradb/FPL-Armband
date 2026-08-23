package main

import (
	"fmt"
	"testing"

	"armband/internal/fpl"
)

// codesByType picks n permanent codes of the given element_type from the fixture's
// bootstrap, in bootstrap order, for tests that need real codes but do not need a legal,
// club-limited fifteen — diffSquadAgainstBase and buildTransfersBlock only ever diff and
// look up codes, they never validate a squad.
func codesByType(boot *fpl.Bootstrap, elementType, n int) []int {
	var out []int
	for _, el := range boot.Elements {
		if el.ElementType == elementType {
			out = append(out, el.Code)
			if len(out) == n {
				break
			}
		}
	}
	return out
}

// fifteenCodes assembles a 2/5/5/3 code list from the fixture, ignoring club limits (which
// diffSquadAgainstBase has no reason to know about).
func fifteenCodes(t *testing.T, boot *fpl.Bootstrap) []int {
	t.Helper()
	var out []int
	for _, n := range []struct {
		elType, want int
	}{{1, 2}, {2, 5}, {3, 5}, {4, 3}} {
		codes := codesByType(boot, n.elType, n.want)
		if len(codes) != n.want {
			t.Fatalf("fixture has only %d of element_type %d, want %d", len(codes), n.elType, n.want)
		}
		out = append(out, codes...)
	}
	return out
}

// TestDiffSquadAgainstBaseFindsAPositionMatchedSwap pins the arithmetic §1 of the design
// this implements describes: a transfer is Squad against Base, position-matched, out-for-in
// — computed from two code lists with no network call.
func TestDiffSquadAgainstBaseFindsAPositionMatchedSwap(t *testing.T) {
	s := fixtureServer(t)
	base := fifteenCodes(t, s.engine.Boot)

	// A second defender not already in base, to swap in for base's first DEF (base[2],
	// since GKP takes base[0:2]).
	var newDef int
	for _, el := range s.engine.Boot.Elements {
		if el.ElementType != 2 {
			continue
		}
		taken := false
		for _, c := range base {
			if c == el.Code {
				taken = true
				break
			}
		}
		if !taken {
			newDef = el.Code
			break
		}
	}
	if newDef == 0 {
		t.Fatal("fixture has no spare defender to swap in")
	}

	squad := append([]int(nil), base...)
	outCode := squad[2] // base[0:2] is GKP, base[2] is the first DEF
	squad[2] = newDef

	moves := diffSquadAgainstBase(s.engine, base, squad)
	if len(moves) != 1 {
		t.Fatalf("got %d moves, want exactly 1: %+v", len(moves), moves)
	}
	m := moves[0]
	if m.Pos != "DEF" {
		t.Errorf("move position = %q, want DEF", m.Pos)
	}
	if m.OutCode != outCode {
		t.Errorf("OutCode = %d, want %d", m.OutCode, outCode)
	}
	if m.InCode != newDef {
		t.Errorf("InCode = %d, want %d", m.InCode, newDef)
	}
	if m.OutName == "" || m.InName == "" {
		t.Errorf("a move must carry resolved names: %+v", m)
	}
}

// TestDiffSquadAgainstBaseIsEmptyForAnUnchangedSquad — no edits, no moves. This is the
// common case (imported and never touched) and it must not manufacture a swap out of
// nothing.
func TestDiffSquadAgainstBaseIsEmptyForAnUnchangedSquad(t *testing.T) {
	s := fixtureServer(t)
	base := fifteenCodes(t, s.engine.Boot)
	moves := diffSquadAgainstBase(s.engine, base, append([]int(nil), base...))
	if len(moves) != 0 {
		t.Errorf("an unchanged squad produced %d moves, want 0: %+v", len(moves), moves)
	}
}

// TestBuildTransfersBlockNoBaseline pins the free-hit / no-baseline path: Base empty means
// NoBaseline and FreeHitBase both true (see buildTransfersBlock's own comment for why the
// two are the same condition today), and no moves are computed against nothing.
func TestBuildTransfersBlockNoBaseline(t *testing.T) {
	s := fixtureServer(t)
	sess := session{Entry: 1234567, Squad: fifteenCodes(t, s.engine.Boot)}
	got := buildTransfersBlock(s.engine, sess, 1, nil)
	if !got.NoBaseline || !got.FreeHitBase {
		t.Errorf("got NoBaseline=%v FreeHitBase=%v, want both true with no Base", got.NoBaseline, got.FreeHitBase)
	}
	if len(got.Moves) != 0 {
		t.Errorf("got %d moves with no baseline to diff against, want 0", len(got.Moves))
	}
	if got.Hits != 0 || got.Cost != 0 {
		t.Errorf("got hits=%d cost=%d with no baseline, want both 0", got.Hits, got.Cost)
	}
}

// TestBuildTransfersBlockBaselineStale pins §1's staleness rule: once importWindow's
// current event has moved past BaseEvent, the count and cost are withheld rather than
// shown against a baseline this build no longer trusts — no Moves, no Hits, no Cost.
func TestBuildTransfersBlockBaselineStale(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 3, 4) // importEvent becomes 3
	codes := fifteenCodes(t, s.engine.Boot)
	sess := session{Entry: 1234567, Squad: codes, Base: codes, BaseEvent: 2} // fetched for GW2, now GW3

	got := buildTransfersBlock(s.engine, sess, 1, nil)
	if !got.BaselineStale {
		t.Error("BaselineStale is false with importEvent (3) past BaseEvent (2)")
	}
	if got.NoBaseline {
		t.Error("NoBaseline is true, but a Base was set — this is staleness, not absence")
	}
	if len(got.Moves) != 0 || got.Hits != 0 || got.Cost != 0 {
		t.Errorf("stale baseline still reported moves=%d hits=%d cost=%d, want all withheld",
			len(got.Moves), got.Hits, got.Cost)
	}
}

// TestBuildTransfersBlockFreshBaselineIsNotStale is the mirror: BaseEvent equal to (not
// less than) the current import event is current, not stale.
func TestBuildTransfersBlockFreshBaselineIsNotStale(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 3, 4)
	codes := fifteenCodes(t, s.engine.Boot)
	sess := session{Entry: 1234567, Squad: codes, Base: codes, BaseEvent: 3}

	got := buildTransfersBlock(s.engine, sess, 1, nil)
	if got.BaselineStale {
		t.Error("a baseline fetched for the CURRENT import event was reported stale")
	}
}

// TestBuildTransfersBlockCostArithmetic pins §2's four-line formula:
//
//	spent = len(diff(Base, Squad))
//	free  = fpl.FreeTransfers(history)   // -1 == unlimited
//	hits  = max(0, spent - free)         // and 0 when free == -1
//	cost  = hits * fpl.HitCost
//
// including the free==-1 (unlimited) case and the free_unknown case, both of which must
// zero the cost regardless of how many moves were made.
func TestBuildTransfersBlockCostArithmetic(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)
	base := fifteenCodes(t, s.engine.Boot)

	// Two spare defenders, so two moves can be made without reusing a code.
	var spare []int
	for _, el := range s.engine.Boot.Elements {
		if el.ElementType != 2 {
			continue
		}
		taken := false
		for _, c := range base {
			if c == el.Code {
				taken = true
				break
			}
		}
		if !taken {
			spare = append(spare, el.Code)
			if len(spare) == 2 {
				break
			}
		}
	}
	if len(spare) != 2 {
		t.Fatal("fixture has fewer than two spare defenders")
	}
	squad := append([]int(nil), base...)
	squad[2], squad[3] = spare[0], spare[1] // two DEF swaps: spent == 2

	for _, tc := range []struct {
		name        string
		free        int
		freeErr     error
		wantHits    int
		wantCost    int
		wantUnknown bool
	}{
		{"one free of two spent: one hit", 1, nil, 1, fpl.HitCost, false},
		{"two free of two spent: no hit", 2, nil, 0, 0, false},
		{"more free than spent: no hit, never negative", 5, nil, 0, 0, false},
		{"unlimited (-1): zero hits regardless of spend", fpl.UnlimitedTransfers, nil, 0, 0, false},
		{"unknown allowance: zero cost even though moves exist",
			fpl.UnlimitedTransfers, errTestHistory, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess := session{Entry: 1234567, Squad: squad, Base: base, BaseEvent: 1}
			got := buildTransfersBlock(s.engine, sess, tc.free, tc.freeErr)
			if len(got.Moves) != 2 {
				t.Fatalf("got %d moves, want 2 regardless of the allowance", len(got.Moves))
			}
			if got.Hits != tc.wantHits {
				t.Errorf("hits = %d, want %d", got.Hits, tc.wantHits)
			}
			if got.Cost != tc.wantCost {
				t.Errorf("cost = %d, want %d", got.Cost, tc.wantCost)
			}
			if got.FreeUnknown != tc.wantUnknown {
				t.Errorf("free_unknown = %v, want %v", got.FreeUnknown, tc.wantUnknown)
			}
		})
	}
}

var errTestHistory = fmt.Errorf("history unreachable")
