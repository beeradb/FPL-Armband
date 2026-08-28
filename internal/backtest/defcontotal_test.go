package backtest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"testing"
)

// TestDefConAggregateMatchesTheArchivesOwnTotal is the validation the derivation
// never sees.
//
// # Why an independent target matters here
//
// `Player.DefCon` is summed from `merged_gw.csv`'s weekly rows. `players_raw.csv`
// publishes its own `defensive_contribution` season total for the same players,
// computed by somebody else from the same football. Checking one against the other is
// the same shape as 2022-23's complete xG aggregate validating `rebuildXGAggregates`:
// the derivation cannot have been fitted to a file it does not read.
//
// # The gate is "identically zero", not "close"
//
// This project's standing rule is that a reconstruction is judged on an exactly zero
// residual, never on a good fit — mis-coding one sibling feature returns plausible
// coefficients at an 88%-exact match. Measured when the derivation was written: 840
// of 841 elements agree outright, and the single exception is the archive's known
// duplicate `(element, fixture)` rows, nine of which belong to element 100 and are
// worth exactly the 11-point gap. `loadGameweeks` drops those rows, so the residual
// through the loader is zero everywhere.
//
// So this asserts equality on every element, and one mismatch fails it. If a future
// season's two files genuinely disagree, that is a finding about the archive and
// belongs in the record rather than in a tolerance here.
func TestDefConAggregateMatchesTheArchivesOwnTotal(t *testing.T) {
	// 2025-26 is the only season carrying the column at all — see AGENTS.md's
	// season table. Every earlier season sums to zero on both sides, which would
	// make this test pass while asserting nothing.
	const season = "2025-26"

	want, err := rawDefConTotals(season)
	if errors.Is(err, errNoSuchFile) || want == nil {
		t.Skipf("%s/players_raw.csv unavailable or carries no "+
			"defensive_contribution column", season)
	}
	if err != nil {
		t.Skipf("archive unreachable: %v", err)
	}

	cfg := loadConfig(t)
	s := loadSeason(t, cfg, season)

	var checked int
	for id, w := range want {
		p := s.Players[id]
		if p == nil {
			continue
		}
		checked++
		if p.DefCon != w {
			t.Errorf("element %d: summed %d from the weekly rows, but "+
				"players_raw.csv publishes %d. The two files are one source counted "+
				"twice, so a difference is either a duplicate row the guard missed "+
				"or a defect in the derivation — it is not a tolerance",
				id, p.DefCon, w)
		}
	}
	if checked == 0 {
		t.Fatal("no element matched between the two files, so the comparison did " +
			"not run — which reads exactly like a pass")
	}
	t.Logf("defensive contribution reconciled on %d elements of %s", checked, season)
}

// rawDefConTotals reads the season-total column straight from the archive, returning
// a nil map when the column is absent — which is every season before 2025-26.
func rawDefConTotals(season string) (map[int]int, error) {
	r, c, col, err := rows(context.Background(), season, "players_raw.csv")
	if err != nil {
		return nil, err
	}
	defer c.Close()
	dc, okDC := col["defensive_contribution"]
	id, okID := col["id"]
	if !okDC || !okID {
		return nil, nil
	}
	out := map[int]int{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if dc >= len(rec) || id >= len(rec) {
			continue
		}
		i, err1 := strconv.Atoi(rec[id])
		v, err2 := strconv.Atoi(rec[dc])
		if err1 != nil || err2 != nil {
			continue
		}
		out[i] = v
	}
	return out, nil
}

// TestDefConIsNotCarriedByTheCache pins the ordering the field's own comment argues
// for, and that this package has got wrong twice on other fields.
//
// `Load` writes the cache *before* `repaired()` runs, so a derived field carrying a
// JSON tag would be marshalled as zero and read back as zero for ever after, with
// `parsedByThisVersion` unable to see anything wrong. That is the kickoff-times bug
// and then the starts-harvest bug, both recorded in `repaired()`'s own comment.
//
// The assertion is about a round trip on purpose: derive, marshal, unmarshal, and
// require the value to be **gone** — proving the cache does not carry it — then
// re-derive and require it back. A test that only checked the end state would pass
// for a field that was cached, which is the failure mode.
func TestDefConIsNotCarriedByTheCache(t *testing.T) {
	s := &Season{Name: "2025-26", Players: map[int]*Player{
		7: {ID: 7, Code: 70, Minutes: 180, GWs: map[int]GW{
			1: {Minutes: 90, DefCon: 12},
			2: {Minutes: 90, DefCon: 5},
		}},
	}}
	s.sumDefCon()
	if got := s.Players[7].DefCon; got != 17 {
		t.Fatalf("derivation gave %d, want 17", got)
	}

	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back Season
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if got := back.Players[7].DefCon; got != 0 {
		t.Errorf("the cached bytes carry DefCon %d. It must be `json:\"-\"`: Load "+
			"caches before repaired() runs, so a tagged derived field is written as "+
			"zero and read back as zero on every later cache hit, with no schema "+
			"check able to see it", got)
	}
	back.sumDefCon()
	if got := back.Players[7].DefCon; got != 17 {
		t.Errorf("after a cache round trip the derivation gave %d, want 17 — the "+
			"weekly rows it derives from survive the trip, so it must", got)
	}
}

// TestEveryArchivePriorProjectionCarriesTheSameFields is the guard for the defect
// A3 records: four projections of one season into one struct, all four omitting
// `DefCon` while the two live paths carried it.
//
// It compares the three archive-side constructions against `PriorFrom` on a fixture
// with every field distinct and non-zero, so a dropped field is a mismatch rather
// than a coincidence of zeros. The failure it stops is somebody adding a statistic
// to one projection and not the others — which is how this drifted, since each copy
// was correct on the day it was written.
func TestEveryArchivePriorProjectionCarriesTheSameFields(t *testing.T) {
	q := &Player{
		ID: 7, Code: 70, Minutes: 1800, Starts: 20,
		XG: 3.5, XA: 2.25, XGC: 11.75, DefCon: 137,
		Bonus: 9, Saves: 4, Yellow: 3, Red: 1,
		GWs: map[int]GW{1: {Minutes: 90, DefCon: 137}},
	}
	want := PriorFrom(q)
	if want.DefCon == 0 {
		t.Fatal("PriorFrom drops DefCon, which is the defect this guards")
	}

	// Priors.Get, the single-season index the replay uses at shipped config.
	s := &Season{Name: "2025-26", Players: map[int]*Player{7: q}}
	got, ok := Priors{S: s}.Get(70)
	if !ok {
		t.Fatal("Priors.Get found no player")
	}
	if *got != want {
		t.Errorf("Priors.Get projects %+v, PriorFrom projects %+v — one field list "+
			"has drifted from the other", *got, want)
	}

	// newPriorIndexRecent, the actively-swept recency arm. Both half-lives are 0
	// here, which is the flat path, so every field must match exactly.
	//
	// ⚠️ An earlier version of this comment said DefCon was "a field no half-life
	// touches", which was true of the code and wrong as a design statement: the
	// rate half-life re-bases every other rate onto the rewritten minutes, and a
	// DefCon left flat is then divided by a smaller denominator downstream. It is
	// re-based with the others now; TestRecencyArmRebasesDefConWithTheOtherRates
	// pins that.
	idx := newPriorIndexRecent(s, 0, 0, 0)
	rec, ok := idx.Get(70)
	if !ok {
		t.Fatal("newPriorIndexRecent found no player")
	}
	if rec.DefCon != want.DefCon {
		t.Errorf("the recency arm projects DefCon %d against %d. It is the arm most "+
			"likely to be swept next, so a divergence here would be measured and "+
			"attributed to the half-life", rec.DefCon, want.DefCon)
	}
}

// TestRecencyArmRebasesDefConWithTheOtherRates pins the property whose absence was
// invisible: every rate the rate half-life touches must be re-expressed against the
// same minutes base, or the odd one out is inflated by the ratio between them.
//
// The fixture gives a player a strong first half and a faded second, so a rate
// half-life short enough to favour the recent weeks pulls every rate DOWN. If DefCon
// were left at the flat season total it would instead read HIGHER than the flat
// arm, because the denominator downstream shrinks while the numerator does not.
func TestRecencyArmRebasesDefConWithTheOtherRates(t *testing.T) {
	q := &Player{ID: 1, Code: 10, Minutes: 1800, GWs: map[int]GW{}}
	for gw := 1; gw <= 20; gw++ {
		dc, xg := 10, 0.5
		if gw > 10 {
			dc, xg = 1, 0.05 // faded
		}
		q.GWs[gw] = GW{Minutes: 90, DefCon: dc, XG: xg}
		q.DefCon += dc
		q.XG += xg
	}
	s := &Season{Name: "2025-26", Players: map[int]*Player{1: q}}

	flat, ok := newPriorIndexRecent(s, 0, 0, 0).Get(10)
	if !ok {
		t.Fatal("flat arm found no player")
	}
	recent, ok := newPriorIndexRecent(s, 0, 3, 0).Get(10)
	if !ok {
		t.Fatal("recency arm found no player")
	}
	if !(recent.XG < flat.XG) {
		t.Fatalf("the fixture is not exercising the rate half-life: xG %v against "+
			"flat %v", recent.XG, flat.XG)
	}
	if recent.DefCon >= flat.DefCon {
		t.Errorf("DefCon %d against the flat arm's %d. Every other rate fell, so a "+
			"DefCon that did not is still the season total against a rewritten "+
			"minutes base — inflated by fullMinutes/recencyMinutes once blendRates "+
			"divides it", recent.DefCon, flat.DefCon)
	}
}

// TestDefConDoesNotLeakIntoAPlayedSeason asserts behaviourally what the field's
// comment argues, on the same terms as its xG sibling.
//
// Summing a whole season into an aggregate is the shape of a point-in-time leak, and
// this package refuses that elsewhere on exactly those grounds. What makes it safe
// is that the aggregate is only ever read for a PRIOR season, while the season being
// played is read through `PointInTime`, which accumulates weeks 1..through and never
// touches `Player.DefCon`.
//
// That is a claim about behaviour, so it is asserted behaviourally rather than by
// trusting the comment — the argument `TestRepairedAggregateDoesNotLeakIntoAPlayedSeason`
// makes for xG, which this had no counterpart to. A view at GW5 must see the first
// five gameweeks and strictly less than the season, even though the aggregate now
// holds all of it.
func TestDefConDoesNotLeakIntoAPlayedSeason(t *testing.T) {
	cur := &Season{Name: "2025-26", Players: map[int]*Player{
		1: {ID: 1, Code: 10, Team: 1, Type: 2, Minutes: 1800, GWs: map[int]GW{}},
	}}
	for gw := 1; gw <= 20; gw++ {
		cur.Players[1].GWs[gw] = GW{Minutes: 90, Fixtures: 1, DefCon: 5, Value: 50}
	}
	cur.sumDefCon()
	if cur.Players[1].DefCon != 100 {
		t.Fatalf("fixture: season total %d, want 100", cur.Players[1].DefCon)
	}

	boot, _ := PointInTime(cur, &Season{Name: "2024-25", Players: map[int]*Player{}}, 5)
	var seen int
	for _, el := range boot.Elements {
		if el.Code == 10 {
			seen = el.DefensiveContribution
		}
	}
	if seen != 25 {
		t.Errorf("a view through GW5 reports %d defensive contributions, want 25 "+
			"(five gameweeks at five). The season total is 100 — if this reads 100 "+
			"the whole-season aggregate has reached the season being PLAYED, which "+
			"is hindsight", seen)
	}
}
