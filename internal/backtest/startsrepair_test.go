package backtest

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"
)

// The harvest must never credit more starters than were on the pitch.
//
// ⚠️ **This test's first version asserted equality with 11 x 20 x 38 = 8,360 and
// called that proof the harvest could not have "dropped rows, double counted a double
// gameweek, or mistaken a substitute for a starter". Statistics review refuted it
// using this harvest's own data**: 2021-22 did two of those three and landed on 8,360
// exactly, because four club-gameweeks were wrong by +1, +1, −1, −1. Its GW11, GW12
// and GW23 totals were 219, 219 and 244, none of them a multiple of 11.
//
// A season total is a **net-leakage** check, not an assignment check — an over-credit
// and an under-credit cancel in it exactly. So this asserts the direction that
// actually matters and that cannot cancel: **no gameweek may exceed 220 starters
// (11 x 20) and no season may exceed 8,360.** Over-crediting is the harvest asserting
// a start nobody made, which propagates into the model as a fact. Under-crediting is a
// gap, and a gap is filled downstream by `reconstructStarts`, which is why the
// harvest is deliberately allowed to come in short — 2021-22 does, at 8,357, because
// the per-match guard drops two ambiguous cells rather than guessing at them.
//
// 2022-23 is included here where the old equality test had to exclude it: a bound
// holds on a partial window, where an identity cannot. Its GW1-15 is genuinely short
// of 15 x 220 because GW7 was cancelled after the Queen's death and GW8 played in
// part.
func TestTheHarvestNeverOverCreditsStarters(t *testing.T) {
	const perGameweek = 11 * 20
	const perSeason = perGameweek * 38
	for _, season := range []string{"2018-19", "2019-20", "2020-21", "2021-22", "2022-23"} {
		f, err := repairData.Open("repairdata/" + season + "-starts.csv")
		if err != nil {
			t.Skipf("%s: no harvest committed yet: %v", season, err)
		}
		r := csv.NewReader(f)
		head, err := r.Read()
		if err != nil {
			f.Close()
			t.Fatalf("%s: header: %v", season, err)
		}
		col, gwCol := -1, -1
		for i, h := range head {
			switch strings.TrimSpace(h) {
			case "starts":
				col = i
			case "GW":
				gwCol = i
			}
		}
		if col < 0 || gwCol < 0 {
			f.Close()
			t.Fatalf("%s: no starts or GW column", season)
		}
		total := 0
		byGW := map[int]int{}
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				t.Fatalf("%s: %v", season, err)
			}
			n, err := strconv.Atoi(strings.TrimSpace(rec[col]))
			if err != nil {
				f.Close()
				t.Fatalf("%s: unparseable starts %q", season, rec[col])
			}
			gw, err := strconv.Atoi(strings.TrimSpace(rec[gwCol]))
			if err != nil {
				f.Close()
				t.Fatalf("%s: unparseable GW %q", season, rec[gwCol])
			}
			total += n
			byGW[gw] += n
		}
		f.Close()
		if total > perSeason {
			t.Errorf("%s: harvested %d starts, more than the %d a season contains "+
				"(11 starters x 20 clubs x 38 gameweeks). The harvest is crediting "+
				"starts nobody made — most likely a foreign fixture admitted by the "+
				"per-gameweek filter", season, total, perSeason)
		}
		// Per gameweek the bound is 11 x that gameweek's club-fixtures, and a double
		// gameweek has more than twenty of them — 2018-19 GW32 legitimately carries
		// 330 starters, which is 11 x 30. The fixture count is not in this file, so
		// the sharper per-club-gameweek assertion lives in the harvest script, where
		// merged_gw.csv is already loaded. What is checkable here is that no gameweek
		// exceeds a *double* round, which still catches a whole club's worth of
		// leakage.
		for gw, n := range byGW {
			if n > 2*perGameweek {
				t.Errorf("%s GW%d: %d starters, more than the %d a full double "+
					"round could contain", season, gw, n, 2*perGameweek)
			}
		}
	}
}

// reconstructStarts must never overwrite a start the ARCHIVE recorded.
//
// ⚠️ This test's first version claimed to pin the repair's ordering, and both the
// claim and the ordering it named were wrong. The repair does not run before
// `reconstructStarts`; it runs after it and after the cache write, in `repaired`,
// because a repair applied before the write is baked into the cache and its escape
// hatch then reads a repaired cache. `TestTheStartsSwitchWorksOnACacheHit` is what
// pins that, and this test never touched the loader at all — it hand-applies starts
// and calls the reconstruction directly.
//
// What it does pin is still worth having and is a genuine precondition for the
// repair being safe to run late: the reconstruction fires only on a club-gameweek
// with no recorded start, so a season whose archive records its own starts is left
// alone. Kept for that, with an honest description of its scope.

func TestStartsRepairPrefersRecordedOverReconstructed(t *testing.T) {
	// A club-gameweek with twelve players who all played, so the rank rule has a
	// genuine choice to get wrong: eleven started, and the twelfth is a substitute
	// who out-played one of them. That is the 45-minute tie the rank rule cannot
	// break, in its purest form.
	s := &Season{Name: "test", Players: map[int]*Player{}}
	for el := 1; el <= 12; el++ {
		mins := 90
		if el == 11 {
			mins = 45 // a starter withdrawn at half time
		}
		if el == 12 {
			mins = 45 // a substitute brought on at half time
		}
		s.Players[el] = &Player{
			ID: el, Team: 1,
			GWs: map[int]GW{1: {Minutes: mins, Fixtures: 1}},
		}
	}

	// What the repair would carry: element 11 started, element 12 did not.
	apply := func(el, starts int) {
		p := s.Players[el]
		g := p.GWs[1]
		g.Starts = starts
		g.StartsReconstructed = false
		p.GWs[1] = g
	}
	for el := 1; el <= 11; el++ {
		apply(el, 1)
	}
	apply(12, 0)

	// Now run the reconstruction on hand-applied recorded starts. ⚠️ This is NOT the
	// order Load uses — see this test's header — and the sentence here said "exactly
	// as Load does after the repair" until 2026-08-15, repeating the very error the
	// header corrects. What this checks is the precondition that makes the late
	// repair safe: the reconstruction never overwrites a recorded start.
	s.reconstructStarts()

	if got := s.Players[11].GWs[1].Starts; got != 1 {
		t.Errorf("the withdrawn starter lost his recorded start: Starts = %d, want 1. "+
			"reconstructStarts must not overwrite a repaired row", got)
	}
	if got := s.Players[12].GWs[1].Starts; got != 0 {
		t.Errorf("the substitute was credited with a start he did not make: "+
			"Starts = %d, want 0", got)
	}
	for el := 1; el <= 12; el++ {
		if s.Players[el].GWs[1].StartsReconstructed {
			t.Errorf("element %d is flagged reconstructed, but its start was "+
				"recorded — a consumer checking the flag would wrongly apply the "+
				"reconstruction's boundaries to it", el)
		}
	}
}

func TestStartsRepairIsSkippedWhenSwitchedOff(t *testing.T) {
	t.Setenv("FPL_NO_STARTS_REPAIR", "1")
	s := &Season{Name: "2021-22", Players: map[int]*Player{
		1: {ID: 1, Team: 1, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1}}},
	}}
	res, err := applyStartsRepair(s)
	if err != nil {
		t.Fatalf("applyStartsRepair: %v", err)
	}
	if res.Rows != 0 || res.Applied != 0 {
		t.Errorf("FPL_NO_STARTS_REPAIR did not disable the repair: %+v", res)
	}
	if s.Players[1].GWs[1].Starts != 0 {
		t.Error("a start was written with the repair switched off")
	}
}

// A season with no harvest file must load unchanged rather than erroring. The harvest
// is a separate, network-bound step and every season that has not been harvested still
// has to replay — the same contract the expected-goals repair keeps.
func TestStartsRepairToleratesAMissingHarvest(t *testing.T) {
	s := &Season{Name: "no-such-season", Players: map[int]*Player{
		1: {ID: 1, Team: 1, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1}}},
	}}
	res, err := applyStartsRepair(s)
	if err != nil {
		t.Fatalf("a missing harvest must not be an error: %v", err)
	}
	if res.Rows != 0 {
		t.Errorf("rows read from a season with no harvest: %+v", res)
	}
}

// The harvest must be structurally incapable of overwriting a recorded start.
//
// This is the confinement check, and it is the same shape as the one the
// expected-goals-conceded repair carries: the strongest thing you can say about a
// repair is not that its numbers look right but that it cannot reach anywhere it has
// no business reaching. `applyStartsRepair` also counts a `Conflict` when it meets a
// non-zero recorded start, but that is a runtime backstop on live data — this is the
// static version, and it fails at build time on a bad harvest rather than after a
// season has been replayed.
//
// Two windows, both from the archive's own defect. `starts` is absent as a column
// before 2022-23, so those seasons are repairable end to end. 2022-23 records its own
// starts from GW16, so anything past GW15 there would be the harvest arguing with the
// archive about a value the archive actually has.
func TestTheStartsHarvestCannotReachARecordedSeason(t *testing.T) {
	windows := map[string][2]int{
		"2018-19": {1, 38},
		"2019-20": {1, 38},
		"2020-21": {1, 38},
		"2021-22": {1, 38},
		"2022-23": {1, 15},
	}
	// Seasons that record their own starts throughout. A harvest file for any of
	// these is a defect however good its contents look.
	for _, season := range []string{"2023-24", "2024-25", "2025-26"} {
		if f, err := repairData.Open("repairdata/" + season + "-starts.csv"); err == nil {
			f.Close()
			t.Errorf("%s records its own starts, so a harvest for it can only "+
				"disagree with the archive: remove repairdata/%s-starts.csv",
				season, season)
		}
	}
	for season, w := range windows {
		f, err := repairData.Open("repairdata/" + season + "-starts.csv")
		if err != nil {
			continue // not harvested yet; TestHarvestedStarts... skips likewise
		}
		r := csv.NewReader(f)
		head, err := r.Read()
		if err != nil {
			f.Close()
			t.Fatalf("%s: header: %v", season, err)
		}
		gwCol := -1
		for i, h := range head {
			if strings.TrimSpace(h) == "GW" {
				gwCol = i
			}
		}
		if gwCol < 0 {
			f.Close()
			t.Fatalf("%s: no GW column", season)
		}
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				t.Fatalf("%s: %v", season, err)
			}
			gw, err := strconv.Atoi(strings.TrimSpace(rec[gwCol]))
			if err != nil {
				f.Close()
				t.Fatalf("%s: unparseable GW %q", season, rec[gwCol])
			}
			if gw < w[0] || gw > w[1] {
				f.Close()
				t.Fatalf("%s: harvest carries GW%d, outside the repairable window "+
					"GW%d-%d. Past that boundary the archive records its own "+
					"starts and this would overwrite them", season, gw, w[0], w[1])
			}
		}
		f.Close()
	}
}

// The starts switch has to work on a cache hit, and the first version of this did not.
//
// `Load` caches the fully-parsed season and `reconstructStarts` is part of that parse,
// so the cached bytes legitimately carry rank-inferred starts. What must NOT be in them
// is the harvest. The repair was briefly applied inside `fetch`, before the write, and
// the consequence was exactly what `repaired`'s comment predicts: every machine holding
// a cache written before the harvest replayed on the rank rule with nothing erroring,
// and FPL_NO_STARTS_REPAIR could not change the outcome in either direction — a
// two-arm sweep of it would have returned a clean, tight null on the thing it was
// built to measure.
//
// Measured at the time on this machine: the cached 2021-22 season held 7,005 starts, of
// which 7,005 were flagged reconstructed and none harvested.
func TestTheStartsSwitchWorksOnACacheHit(t *testing.T) {
	// A cached season as the parser leaves it: starts present, inferred, flagged.
	// Element 1 GW1 is a real row in 2021-22-starts.csv, so the harvest can reach it.
	orig := &Season{
		Name: "2021-22",
		Players: map[int]*Player{
			1: {ID: 1, Code: 1001, Team: 1, GWs: map[int]GW{
				1: {Minutes: 90, Fixtures: 1, Starts: 1, StartsReconstructed: true},
			}},
		},
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var cached Season
	if err := json.Unmarshal(b, &cached); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !cached.Players[1].GWs[1].StartsReconstructed {
		t.Fatal("the cached season lost the reconstructed flag, so this test cannot " +
			"tell a harvested start from an inferred one")
	}

	load := func(off bool) *Season {
		var s Season
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if off {
			t.Setenv("FPL_NO_STARTS_REPAIR", "1")
		} else {
			t.Setenv("FPL_NO_STARTS_REPAIR", "")
		}
		out, err := repaired(&s)
		if err != nil {
			t.Fatalf("repaired: %v", err)
		}
		return out
	}

	on, off := load(false), load(true)

	if off.StartsRepair.Applied != 0 || !off.Players[1].GWs[1].StartsReconstructed {
		t.Errorf("with the switch off the harvest applied %d rows and cleared the "+
			"reconstructed flag (%v) from a cache hit — the escape hatch is reading a "+
			"repaired cache", off.StartsRepair.Applied,
			!off.Players[1].GWs[1].StartsReconstructed)
	}
	// The arms must be capable of differing, or the assertion above passes on a corpse.
	if on.StartsRepair.Applied == 0 {
		t.Fatal("with the switch on the harvest applied nothing, so the off arm " +
			"proves nothing. Either the repair no longer runs in `repaired`, or it " +
			"refuses a reconstructed row instead of replacing it")
	}
	if on.StartsRepair.Replaced == 0 {
		t.Error("the harvest applied rows but replaced no inferred one, so it is not " +
			"reaching the reconstruction it exists to supersede")
	}
	if g := on.Players[1].GWs[1]; g.StartsReconstructed {
		t.Error("a harvested row is still flagged reconstructed: a consumer checking " +
			"the flag would wrongly apply the rank rule's boundaries to recorded data")
	}
	if on.StartsRepair.Conflict != 0 {
		t.Errorf("the harvest hit %d rows the archive had recorded itself, which "+
			"means its gameweek windows have drifted", on.StartsRepair.Conflict)
	}
}
