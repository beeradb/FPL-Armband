package backtest

// The positive control for the oracle machinery.
//
//	go test ./internal/backtest -run TestOmniscience -v
//
// Every other test of this apparatus is a **negative** control: an oracle
// declares what it must not move, the harness checks the declaration, and passing
// means nothing moved. A whole family of failures survives that. If hindsight
// were computed correctly and then never reached a decision — a seam that is not
// the live one, a value overwritten downstream, an arm whose config is rebuilt
// per cell from the environment — every negative control would still pass and
// every oracle would report a small, plausible, entirely fictional number.
//
// So one arm is given the answers. It is told exactly what every player is about
// to do, and it has to score far above the shipped model. If it does not, the
// finding is that the harness is broken, and it is a much more important finding
// than whatever the oracle under study was going to say.
//
// It is not DIAG-gated and it must not become so. A control nobody runs is not a
// control, and this is the one test in the package whose failure invalidates the
// others.
//
// **And for the same reason it does not read the shipped config.** Every DIAG
// diagnostic here resolves `configPath`, which is one author's home directory; on
// any other checkout that fails before a gameweek is read, which is why FPL_CONFIG
// exists. A guard that only runs on one laptop is not a guard either, and this one
// has no need of the constants in force: it asks whether information reaches a
// decision, and the answer is true or false at any setting of the model. It takes
// `config.Default()` and skips when the archive is out of reach.

import (
	"fmt"
	"sort"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// omniscientArm is every information oracle at once, which is what the word
// means.
//
// The bootstrap rewrite alone is not enough, and the reason is the finding this
// control produced. There are **three** channels into what the model believes
// about a player, not one:
//
//   - the bootstrap, which PointInTimeWith manufactures;
//   - `Engine.Recent`, which in-season `blendRates` uses to *replace* the
//     bootstrap's minutes outright — the seam OracleMinutes perturbs;
//   - `Engine.Priors`, which pre-season `blendRates` uses to overwrite the
//     entire blend — the seam newOraclePriors perturbs.
//
// An arm that told the truth on one of the three would be corrected back by the
// other two. Composing the bits is the honest way to say "no input is wrong", and
// it exercises the bitmask, Validate and the stamp on the way through.
//
// Availability is included though the rewrite subsumes it — a player with no
// future minutes scores near zero regardless — because leaving it out would make
// the arm "omniscient except about one thing", which is precisely the kind of
// quiet qualification the whole Oracles type exists to prevent.
var omniscientArm = Oracles{
	Info: OracleOmniscient | OracleMinutes | OracleAvailability,
}

// omniscientFloor is how many times the baseline the control must score.
//
// **Asserted, not measured**, and deliberately well below what is observed: the
// control is a smoke test for a wiring failure, not an estimate of what perfect
// information is worth. Setting it near the observed value would turn every
// ordinary change to the scoring model into a failure of the apparatus, which is
// the opposite of what a control is for.
//
// Observed at the four GW1 cells when this landed: 1.18 to 1.33 on POLICY and
// 1.29 to 1.64 on HOLD. 1.10 leaves a wide margin and still catches the failure it
// exists for, which lands at 1.00 — hindsight computed and never delivered.
//
// It is the *weaker* of the two assertions here and is second for that reason.
// The ratio is a season of football away from the thing being checked, so it
// answers the question with a lot of noise in the way; the rated-top-twenty count
// answers it directly and in integers.
const omniscientFloor = 1.10

// TestOmniscienceIsThePositiveControl replays each season from GW1 twice and
// checks that knowing the answers is worth a great deal.
//
// GW1 entry, and only GW1, for a stated reason rather than for speed. The rate
// blend weights current-season evidence by `el.Minutes / 90`, so a realised
// aggregate over the *remaining* weeks is a smaller sample the later the entry
// and the blend trusts it correspondingly less. Pre-season the whole season is
// ahead, the bootstrap carries it, and the model is at its most credulous — which
// is where a control belongs. A late entry would test the blend's shrinkage as
// much as the wiring.
func TestOmniscienceIsThePositiveControl(t *testing.T) {
	if testing.Short() {
		t.Skip("replays the season archive")
	}
	cfg := config.Default()

	type row struct {
		season              string
		basePolicy, oPolicy int
		baseHold, oHold     int
		// How many of the season's twenty highest scorers each engine *rates* in
		// its own top twenty. This is the transmission check.
		baseTopTwenty, oTopTwent int
		// How many of them each opening fifteen actually *bought*. Reported and
		// deliberately not asserted — see the table's footnote.
		baseSquadTop, oSquadTop int
	}
	var rows []row

	// Memoised locally rather than through loadSeason's process-global cache,
	// which is keyed on a config this test deliberately does not read.
	seasons := map[string]*Season{}
	load := func(name string) *Season {
		if s, ok := seasons[name]; ok {
			return s
		}
		s := loadForInputDiff(t, name)
		seasons[name] = s
		return s
	}

	for _, pair := range sweepPairNames() {
		prior, cur := load(pair[0]), load(pair[1])

		sc := sweepConfig(cfg, 1, false)
		sc.Oracles = Oracles{}
		base, err := Simulate(cur, prior, sc)
		if err != nil {
			t.Fatalf("%s baseline: %v", pair[1], err)
		}

		oc := sweepConfig(cfg, 1, false)
		oc.Oracles = omniscientArm
		if err := oc.Oracles.Validate(); err != nil {
			t.Fatalf("the control arm does not validate: %v", err)
		}
		got, err := Simulate(cur, prior, oc)
		if err != nil {
			t.Fatalf("%s omniscient: %v", pair[1], err)
		}

		top := topScorers(cur, 20)
		rows = append(rows, row{
			season:     pair[1],
			basePolicy: base.Points, oPolicy: got.Points,
			baseHold:      sumInts(HoldCaptaincyWeekly(cur, prior, sc, base.OpeningSquad).Full),
			oHold:         sumInts(HoldCaptaincyWeekly(cur, prior, oc, got.OpeningSquad).Full),
			baseTopTwenty: countIn(preSeasonTopByScore(t, cur, prior, sc, 20), top),
			oTopTwent:     countIn(preSeasonTopByScore(t, cur, prior, oc, 20), top),
			baseSquadTop:  countIn(base.OpeningSquad, top),
			oSquadTop:     countIn(got.OpeningSquad, top),
		})
	}

	fmt.Printf("\n=== POSITIVE CONTROL — an arm told the answers.\n")
	fmt.Printf("Not a bound on any capability and never reportable: this measures\n")
	fmt.Printf("whether hindsight granted at the information seam reaches a decision.\n")
	fmt.Printf("%-9s %9s %9s %6s | %9s %9s %6s | %6s %5s | %6s %5s\n", "season",
		"POLICY", "omni", "x", "HOLD", "omni", "x", "rated", "omni", "bought", "omni")
	var bp, op, bh, oh, bt, ot, bs, osq int
	for _, r := range rows {
		fmt.Printf("%-9s %9d %9d %6.2f | %9d %9d %6.2f | %6d %5d | %6d %5d\n", r.season,
			r.basePolicy, r.oPolicy, ratio(r.oPolicy, r.basePolicy),
			r.baseHold, r.oHold, ratio(r.oHold, r.baseHold),
			r.baseTopTwenty, r.oTopTwent, r.baseSquadTop, r.oSquadTop)
		bp += r.basePolicy
		op += r.oPolicy
		bh += r.baseHold
		oh += r.oHold
		bt += r.baseTopTwenty
		ot += r.oTopTwent
		bs += r.baseSquadTop
		osq += r.oSquadTop
	}
	fmt.Printf("%-9s %9d %9d %6.2f | %9d %9d %6.2f | %6d %5d | %6d %5d\n", "all",
		bp, op, ratio(op, bp), bh, oh, ratio(oh, bh), bt, ot, bs, osq)
	fmt.Printf("\n'rated' is how many of the season's twenty highest scorers each\n")
	fmt.Printf("pre-season engine puts in its OWN top twenty by Score. That is the\n")
	fmt.Printf("transmission check: an integer, counted without noise, and it asks\n")
	fmt.Printf("whether hindsight reaches the estimate rather than whether it\n")
	fmt.Printf("survives a season of budgets, gates and football.\n")
	fmt.Printf("'bought' is how many of them the opening fifteen actually holds, and\n")
	fmt.Printf("is reported rather than asserted: the optimiser maximises the eleven's\n")
	fmt.Printf("value under a budget, and the season's top scorers are its most\n")
	fmt.Printf("expensive players, so buying more of them is not something a correct\n")
	fmt.Printf("omniscient optimiser is obliged to do.\n")

	for _, r := range rows {
		if r.oTopTwent <= r.baseTopTwenty {
			t.Errorf("%s: the omniscient engine rates %d of the season's twenty "+
				"highest scorers in its own top twenty, against the blind model's %d. "+
				"Hindsight is not reaching the estimate at all — the seam is not the "+
				"live one, or the value is overwritten downstream, or the config is "+
				"rebuilt without it. Every other oracle in this package would still "+
				"report a plausible number in that state",
				r.season, r.oTopTwent, r.baseTopTwenty)
		}
		if got := ratio(r.oPolicy, r.basePolicy); got < omniscientFloor {
			t.Errorf("%s: an arm told what every player would do scored %d on POLICY "+
				"against the shipped model's %d, a factor of %.2f, below the floor "+
				"of %.2f", r.season, r.oPolicy, r.basePolicy, got, omniscientFloor)
		}
		if got := ratio(r.oHold, r.baseHold); got < omniscientFloor {
			t.Errorf("%s: the same arm scored %d on HOLD against %d, a factor of "+
				"%.2f, below the floor of %.2f. HOLD is checked as well as POLICY "+
				"because the two reach the model by different routes — the opening "+
				"optimise and the weekly re-pick — and an oracle can reach one and "+
				"not the other", r.season, r.oHold, r.baseHold, got, omniscientFloor)
		}
	}
}

// preSeasonTopByScore is the n players a pre-season engine rates highest.
//
// It reconstructs the engine Simulate builds for the opening squad rather than
// reaching into the result, because the quantity wanted is the model's *estimate*
// and the result only carries what survived a budget. Priors and the recency index
// are supplied the same way Simulate supplies them, or the comparison would be
// between two engines that differ in more than the oracle.
func preSeasonTopByScore(t *testing.T, cur, prior *Season, cfg SimConfig, n int) []int {
	t.Helper()
	boot, fx := PointInTimeWith(cur, prior, cfg.startGW()-1, cfg.Oracles)
	e := analysis.NewEngineFull(boot, fx, cfg.Weights,
		analysis.Congestion{}, analysis.RoleRisk{})
	e.Priors = cfg.priors(cur, prior)
	e.Recent = cfg.recentIndex(cur, cfg.startGW()-1)

	ms := e.AllMetrics()
	sort.Slice(ms, func(i, j int) bool {
		if ms[i].Score != ms[j].Score {
			return ms[i].Score > ms[j].Score
		}
		// Ties break on the lower element id. Deterministically, because
		// AllMetrics walks the bootstrap and a tie resolved by position would make
		// this test depend on a sort's stability rather than on the model.
		return ms[i].ID < ms[j].ID
	})
	if len(ms) > n {
		ms = ms[:n]
	}
	out := make([]int, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

// topScorers is the n highest-scoring players of a season, by realised points.
func topScorers(s *Season, n int) map[int]bool {
	ids := sortedPlayerIDs(s)
	// Insertion into a small ordered slice rather than a sort with a tiebreak,
	// because ids arrive sorted and ties must not depend on map iteration: this
	// package has already had one non-deterministic optimiser from exactly that.
	var best []int
	for _, id := range ids {
		p := s.Players[id]
		pts := 0
		for _, g := range p.GWs {
			pts += g.Points
		}
		at := len(best)
		for at > 0 && totalPoints(s, best[at-1]) < pts {
			at--
		}
		if at >= n {
			continue
		}
		best = append(best, 0)
		copy(best[at+1:], best[at:])
		best[at] = id
		if len(best) > n {
			best = best[:n]
		}
	}
	out := map[int]bool{}
	for _, id := range best {
		out[id] = true
	}
	return out
}

func totalPoints(s *Season, id int) int {
	p := s.Players[id]
	if p == nil {
		return 0
	}
	n := 0
	for _, g := range p.GWs {
		n += g.Points
	}
	return n
}

func countIn(ids []int, set map[int]bool) int {
	n := 0
	for _, id := range ids {
		if set[id] {
			n++
		}
	}
	return n
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// TestOmniscienceIsRefusedBySweeps pins the refusal at the boundary that matters.
//
// The design proposed refusing the *means row*. That is too narrow twice over: it
// leaves the per-cell rows for stats/sweep_inference.R to average into a table
// with a standard error attached, and refusing the cells instead would make the
// arm indistinguishable from one lost to a sweep killed under load — the exact
// gap writeSweepProvenance declares its arms up front to expose. So one rule at
// one boundary: a sweep will not run the arm at all.
func TestOmniscienceIsRefusedBySweeps(t *testing.T) {
	err := validateOracleArms([]policyVariant{
		{label: "real (ships)"},
		oracleVariant(omniscientArm, "told the answers", nil),
	})
	if err == nil {
		t.Fatal("a sweep accepted an omniscient arm, so its figure would reach the " +
			"cells file, the means file, the console table and R's inference table — " +
			"and a hindsight bound with no capability attached is the worst possible " +
			"number to leave somewhere it can be copied from")
	}
	// And an ordinary oracle arm is still accepted, or the guard above would pass
	// against a validateOracleArms that had started refusing everything.
	if err := validateOracleArms([]policyVariant{
		{label: "real (ships)"},
		oracleVariant(Oracles{Info: OracleMinutes}, "perfect minutes", nil),
	}); err != nil {
		t.Errorf("an ordinary oracle arm was refused: %v", err)
	}
	if (Oracles{Info: OracleMinutes}).Reportable() != true {
		t.Error("an ordinary oracle reports as unreportable")
	}
	if omniscientArm.Reportable() {
		t.Error("the omniscient arm reports as reportable")
	}
}

// TestOmniscienceRewritesTheFutureAndNotThePast pins what the rewrite means, on
// arithmetic rather than on the archive.
//
// The two halves matter separately. Reading the *future* is the oracle; leaving
// price, club, position and status alone is what stops it being a composition of
// oracles reported as one — and that half is easy to break by adding a field to
// the loop and forgetting the declaration, which is why Tier 1 covers it too.
func TestOmniscienceRewritesTheFutureAndNotThePast(t *testing.T) {
	cur := &Season{Name: "2025-26", Players: map[int]*Player{
		// Dreadful for two gameweeks and then transformed. A model built through
		// GW2 sees the first half; an omniscient one must see only the second.
		1: {ID: 1, Code: 101, Type: 4, Team: 1, GWs: map[int]GW{
			1: {Minutes: 90, Starts: 1, Points: 2, Fixtures: 1},
			2: {Minutes: 90, Starts: 1, Points: 2, Fixtures: 1},
			3: {Minutes: 90, Starts: 1, Points: 15, Goals: 3, Bonus: 3, XG: 1.5, Fixtures: 1},
			4: {Minutes: 45, Starts: 0, Points: 8, Goals: 1, Bonus: 1, XG: 0.5, Fixtures: 1},
		}},
	}}

	b := &fpl.Bootstrap{Elements: []fpl.Element{{
		ID: 1, Code: 101, ElementType: 4, Team: 1, WebName: "Future",
		NowCost: 75, Status: "a",
		// The season-to-date figures a model built through GW2 would carry.
		Minutes: 180, Starts: 2, TotalPoints: 4, Bonus: 0,
	}}}
	applyOmniscience(b, cur, 2)

	el := b.Elements[0]
	if el.Minutes != 135 || el.Starts != 1 {
		t.Errorf("minutes/starts %d/%d, want 135/1 — the rewrite must read GW3-38 "+
			"and not accumulate the past alongside it", el.Minutes, el.Starts)
	}
	if el.TotalPoints != 23 || el.GoalsScored != 4 || el.Bonus != 4 {
		t.Errorf("points/goals/bonus %d/%d/%d, want 23/4/4",
			el.TotalPoints, el.GoalsScored, el.Bonus)
	}
	if got := float64(el.ExpectedGoals); got < 1.99 || got > 2.01 {
		t.Errorf("expected goals %v, want 2.0", got)
	}
	// 2.0 xG over 135 minutes is 1.333 per 90. A per-90 left on a stale
	// denominator is the quiet half of this rewrite and would survive every check
	// above.
	if got := float64(el.ExpectedGoalsPer90); got < 1.32 || got > 1.35 {
		t.Errorf("xG/90 %v, want ~1.333 — the per-90 fields must be recomputed on "+
			"the rewritten minutes, not carried over", got)
	}
	// Not hindsight, and not this oracle's to touch.
	if el.NowCost != 75 || el.Status != "a" || el.Team != 1 || el.ElementType != 4 {
		t.Errorf("price/status/club/position moved to %d/%q/%d/%d — an omniscient "+
			"arm that moved one of those would be a composition of oracles "+
			"reported as a single one", el.NowCost, el.Status, el.Team, el.ElementType)
	}

	// Pre-season the whole season is the future, which is the view the control
	// runs at.
	c := &fpl.Bootstrap{Elements: []fpl.Element{{ID: 1, Code: 101, NowCost: 75}}}
	applyOmniscience(c, cur, 0)
	if c.Elements[0].Minutes != 315 || c.Elements[0].TotalPoints != 27 {
		t.Errorf("pre-season reads %d minutes and %d points, want 315 and 27",
			c.Elements[0].Minutes, c.Elements[0].TotalPoints)
	}

	// A player the archive has no rows for keeps what the honest pass gave him.
	// An oracle may correct what is known about a player, never erase one.
	d := &fpl.Bootstrap{Elements: []fpl.Element{{ID: 99, Code: 999, Minutes: 900, TotalPoints: 60}}}
	applyOmniscience(d, cur, 2)
	if d.Elements[0].Minutes != 900 || d.Elements[0].TotalPoints != 60 {
		t.Errorf("an unknown player was zeroed to %d minutes and %d points",
			d.Elements[0].Minutes, d.Elements[0].TotalPoints)
	}
}
