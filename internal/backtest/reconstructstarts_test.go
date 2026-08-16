package backtest

import "testing"

// startsFixture builds a one-club, one-gameweek season from (id, minutes, starts).
func startsFixture(rows [][3]int, fixtures int) *Season {
	s := &Season{Name: "test", Players: map[int]*Player{}}
	for _, r := range rows {
		s.Players[r[0]] = &Player{ID: r[0], Team: 1, GWs: map[int]GW{
			1: {Minutes: r[1], Starts: r[2], Fixtures: fixtures},
		}}
	}
	return s
}

// TestReconstructStartsPicksElevenByMinutes pins the rule and its count.
//
// Eleven players start each fixture, so the eleven with the most minutes are the
// starters. Being exact in the COUNT is what makes this better than a "60+ minutes
// is a start" threshold, which measured three times worse against the seasons that
// do record starts — 6.86% of starter slots misclassified against 2.36%.
func TestReconstructStartsPicksElevenByMinutes(t *testing.T) {
	var rows [][3]int
	for i := 1; i <= 11; i++ {
		rows = append(rows, [3]int{i, 90, 0}) // eleven who played the full match
	}
	for i := 12; i <= 14; i++ {
		rows = append(rows, [3]int{i, 20, 0}) // three substitutes
	}
	rows = append(rows, [3]int{15, 0, 0}) // an unused substitute

	s := startsFixture(rows, 1)
	s.reconstructStarts()

	var starters, flagged int
	for id, p := range s.Players {
		g := p.GWs[1]
		if g.Starts > 0 {
			starters++
			if id > 11 {
				t.Errorf("player %d (a substitute) was credited with a start", id)
			}
		}
		if g.StartsReconstructed {
			flagged++
		}
	}
	if starters != 11 {
		t.Errorf("credited %d starters, want exactly 11", starters)
	}
	if flagged != 11 {
		t.Errorf("%d rows flagged as reconstructed, want 11 — a consumer must be "+
			"able to tell an inferred start from a recorded one", flagged)
	}
	if s.Players[15].GWs[1].Starts != 0 {
		t.Error("a player who recorded no minutes was credited with a start")
	}
}

// TestReconstructStartsLeavesRecordedDataAlone is the first boundary: the
// reconstruction repairs, it does not overwrite.
//
// It fires only where the archive recorded NOTHING for a whole club-gameweek.
// Anywhere with a single recorded start has real data, and promoting a genuine
// substitute inside it would be inventing rather than repairing — which matters
// because 2022-23 is broken through GW15 and correct from GW16, so both regimes
// exist inside one season.
func TestReconstructStartsLeavesRecordedDataAlone(t *testing.T) {
	rows := [][3]int{{1, 90, 1}, {2, 90, 0}, {3, 90, 0}}
	s := startsFixture(rows, 1)
	s.reconstructStarts()

	if got := s.Players[2].GWs[1].Starts; got != 0 {
		t.Errorf("player 2 got %d starts; a club-gameweek with recorded data must "+
			"not be touched", got)
	}
	for id, p := range s.Players {
		if p.GWs[1].StartsReconstructed {
			t.Errorf("player %d was flagged as reconstructed in a club-gameweek "+
				"that already had recorded starts", id)
		}
	}
}

// TestReconstructStartsSkipsDoubleGameweeks is the deliberate gap.
//
// A player can legitimately start both fixtures of a double, so Starts can be 2,
// and a gameweek total cannot say which. 2022-23 has 42 double team-gameweeks.
// They keep their recorded zero and stay unflagged, so a consumer can distinguish
// an unreconstructed row from a reconstructed one — an honest gap beats a
// confident guess, which is the lesson the doubles-counting bug taught on this
// same season.
func TestReconstructStartsSkipsDoubleGameweeks(t *testing.T) {
	var rows [][3]int
	for i := 1; i <= 14; i++ {
		rows = append(rows, [3]int{i, 180, 0})
	}
	s := startsFixture(rows, 2)
	s.reconstructStarts()

	for id, p := range s.Players {
		if g := p.GWs[1]; g.Starts != 0 || g.StartsReconstructed {
			t.Errorf("player %d in a double gameweek got starts=%d reconstructed=%v; "+
				"doubles are deliberately left alone", id, g.Starts, g.StartsReconstructed)
		}
	}
}

// TestReconstructStartsIsDeterministic guards the tie-break.
//
// Half-time substitutions leave a withdrawn starter and his replacement both on 45
// minutes, so ties are common — they are the residual error the rank rule cannot
// remove. Ranking must therefore break ties on element id rather than on map
// iteration order, which is the defect that made a clean-sheet diagnostic report a
// different figure on every run.
func TestReconstructStartsIsDeterministic(t *testing.T) {
	build := func() *Season {
		var rows [][3]int
		for i := 1; i <= 16; i++ {
			rows = append(rows, [3]int{i, 45, 0}) // every player tied
		}
		return startsFixture(rows, 1)
	}
	var first []int
	for run := 0; run < 8; run++ {
		s := build()
		s.reconstructStarts()
		var got []int
		for id := 1; id <= 16; id++ {
			if s.Players[id].GWs[1].Starts > 0 {
				got = append(got, id)
			}
		}
		if run == 0 {
			first = got
			continue
		}
		if len(got) != len(first) {
			t.Fatalf("run %d picked %d starters, run 0 picked %d", run, len(got), len(first))
		}
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("run %d picked %v, run 0 picked %v — the tie-break is not "+
					"deterministic", run, got, first)
			}
		}
	}
}

// TestStaleCacheWithoutStartsIsRefused pins the schema check.
//
// A version bump alone does not catch a stale file: bumping v4 to v5 for kickoff
// times once hit v5 files an experiment had left behind, so a fresh parser read a
// stale schema and reported no congestion anywhere — a null result that looked
// exactly like a real one. There are v2, v3 and v4 archives sitting beside the
// current ones on this machine, so leftovers demonstrably accumulate.
func TestStaleCacheWithoutStartsIsRefused(t *testing.T) {
	stale := startsFixture([][3]int{{1, 90, 0}, {2, 90, 0}}, 1)
	if stale.hasStarts() {
		t.Error("a season where nobody who played ever started was accepted as " +
			"current; eleven players start every match, so it cannot be")
	}

	repaired := startsFixture([][3]int{{1, 90, 0}, {2, 90, 0}}, 1)
	repaired.reconstructStarts()
	if !repaired.hasStarts() {
		t.Error("a reconstructed season was refused by its own schema check")
	}

	empty := &Season{Name: "test", Players: map[int]*Player{}}
	if !empty.hasStarts() {
		t.Error("a season with no football recorded was refused; there is nothing " +
			"to judge it on, so it must not be rejected")
	}
}
