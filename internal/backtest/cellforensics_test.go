package backtest

// What actually happened in the cells that carry a concentrated result?
//
//	DIAG=1 go test ./internal/backtest -run '^TestDiagCellForensics$' -v -timeout 60m
//
// # Why this exists
//
// `stats/concentration_screen.R` flags 21 of 36 banked arms as means resting on
// a handful of cells. A flag is not a verdict: a setting whose mechanism only
// fires sometimes — a wildcard landing better, a chip week, a squad flip that
// avoids one bad buy — *ought* to be lumpy, and averaging it over 36 cells is
// the wrong thing to do to it. What separates a real intermittent effect from an
// argmax over cells is **a mechanism that predicts where it fires**, and no
// statistic supplies one. A human looking at the actual squad can.
//
// So this prints, for each flagged cell, the two things a reader needs to judge
// it with football knowledge rather than arithmetic:
//
//   - **the opening fifteen, and exactly which players differ.** On HOLD this is
//     the entire intervention, so a difference that looks like a sensible
//     upgrade is a different story from one that looks like a coin flip
//   - **every gameweek's points for both arms, with the captain each picked**,
//     so the weeks carrying the cell can be read off and checked against what
//     actually happened in that gameweek
//
// # ⚠️ These arms make no transfers, and that is not a limitation of this probe
//
// Every figure the concentration screen flags is on **HOLD**: buy the opening
// fifteen at the entry deadline and never transfer, re-picking the eleven and
// the captain each week. So "what move did the team make" has exactly two
// answers here — **which fifteen it bought**, and **who it captained each week**.
// There is no transfer to inspect, and a reader expecting one will otherwise
// conclude the probe is broken.
//
// That is also why HOLD is the right metric for a scoring constant: it excludes
// the transfer path's own 303-point noise floor. The cost is that a setting
// whose real mechanism is a better *transfer* cannot show it here at all.
//
// # The cells
//
// Chosen from the screen's output, not by looking at the outcome first. Each
// names why it is here. The pairing is deliberate: `TEAMFORM`'s carriers sit in
// ONE season at adjacent entry points, which replay overlapping football, while
// `BlendRateK`'s sit in three different seasons — so the two have different
// prior odds of being real, and reading them side by side is the point.

import (
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
)

// forensicCell carries BOTH sides, and that is not redundant.
//
// ⚠️ A sweep's baseline is its own `variants[0]`, which is often **not** the
// plain `sweepConfig`. `TEAMFORM`'s two arms both set `WeeklyXI = true` and
// differ only in the club-form weight, so reconstructing the baseline as a bare
// sweepConfig would compare the club-form weight AND the imminent-fixture eleven
// at once — measuring their sum and neither, which is the failure this record
// names by that phrase. The baseline func is copied from the sweep's own arm
// list, not inferred.
type forensicCell struct {
	label  string
	season string
	start  int
	base   func(*SimConfig)
	arm    func(*SimConfig)
	why    string
	// want is the paired difference this cell reads ON THIS CODE, in points per
	// gameweek, and the probe FAILS if it does not reproduce it.
	//
	// ⚠️ This is the check that caught the bug this file shipped once: a
	// process-global left set by an earlier cell put both BlendRateK cells on a
	// configuration neither arm swept, and the output was entirely plausible. A
	// forensic probe that cannot reproduce the number it is explaining is
	// explaining a different number, and nothing else here would say so.
	want    float64
	hasWant bool
	// banked is what the same cell reads in the committed CSV, when that differs.
	// Zero when it agrees.
	//
	// ⚠️ It differs for TEAMFORM and the reason is the DATA STATE, not a leak.
	// Its bank is `2026-08-12-4d61058`, before the archive repairs; the
	// BlendRateK cells were banked on the current tree and reproduce to three
	// decimals, which is the control that separates the two explanations. Recorded
	// here rather than silently reconciled, because this record's rule is to name
	// the data state or not quote the level.
	banked float64
}

func TestDiagCellForensics(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	byName := map[string]seasonPair{}
	for _, p := range pairs {
		byName[p.Name] = p
	}

	// TEAMFORM's weight is a package variable, not a SimConfig field, because it
	// is read deep inside scoring by an engine the sweep does not construct.
	// Restore whatever was live rather than assuming it was off — copied from
	// TestDiagTeamFormSweep, which is also where both arm funcs come from.
	//
	// ⚠️ **A `defer` is NOT enough, and the first version of this file shipped
	// that bug and it changed every number below it.** The defer restores once,
	// at the end of the test, so the TEAMFORM cells left the weight at 0.50 and
	// **both `BlendRateK` cells then ran with club form switched on** — a
	// configuration neither arm ever swept, silently, with plausible output. That
	// is this file's own warning about process-global state, one level up from
	// where it was written. Every cell now resets to the shipped value first, so
	// an arm that does not mention a global gets the shipped one.
	shippedForm := analysis.TeamFormWeight()
	shippedBuy := analysis.BuyDiscount()
	a, b, c, dd := analysis.AppearanceFit()
	defer func() {
		analysis.SetTeamFormWeight(shippedForm)
		analysis.SetBuyDiscount(shippedBuy)
		analysis.SetAppearanceFit(a, b, c, dd)
	}()
	// ⚠️ **This covers the three globals that HAVE getters, and `internal/analysis`
	// exports eleven setters.** `SetBenchSlots`, `SetDerivedBenchSlots`,
	// `SetCaptainShrink`, `SetUnifiedAppearance` and the three `SetFixtureLoad*`
	// have no reader, so their shipped value cannot be captured and restored here.
	//
	// **Adding a cell whose arm touches one of those — `benchshape` is a flagged
	// arm and therefore an obvious candidate — reproduces the leak this function
	// exists to stop.** The fix then is a getter beside the setter, not another
	// hard-coded line. Stated because the first version of this comment claimed
	// "every cell now resets to the shipped value", which was true of exactly one
	// global and false as a general promise, and a general promise is what a
	// reader would rely on.
	resetGlobals := func() {
		analysis.SetTeamFormWeight(shippedForm)
		analysis.SetBuyDiscount(shippedBuy)
		analysis.SetAppearanceFit(a, b, c, dd)
	}

	teamForm := func(w float64) func(*SimConfig) {
		return func(sc *SimConfig) {
			sc.WeeklyXI = true
			analysis.SetTeamFormWeight(w)
		}
	}
	blend := func(k float64) func(*SimConfig) {
		return func(sc *SimConfig) { sc.Weights.BlendRateK = k }
	}

	cells := []forensicCell{
		{
			label: "TEAMFORM club form 0.50 vs 0 (both WeeklyXI)", season: "2023-24", start: 26,
			base: teamForm(0), arm: teamForm(0.50), hasWant: true, want: 7.154, banked: 10.231,
			why: "largest single carrier in the bank: +10.23 pts/gw. Its arm reads +23.6 a season and reverses when this cell and 2023-24@21 are dropped",
		},
		{
			label: "TEAMFORM club form 0.50 vs 0 (both WeeklyXI)", season: "2023-24", start: 21,
			base: teamForm(0), arm: teamForm(0.50), hasWant: true, want: 7.111, banked: 7.333,
			why: "the second carrier, +7.33 pts/gw — SAME season as the first and five gameweeks earlier, so it replays overlapping football and is probably the same event counted twice",
		},
		{
			label: "BlendRateK=16 vs 12", season: "2021-22", start: 26,
			base: blend(12), arm: blend(16), hasWant: true, want: 9.692,
			why: "carrier of the 16-vs-12 contrast, which survives every leave-one-season-out subset while three cells carry 99% of it",
		},
		{
			label: "BlendRateK=16 vs 12", season: "2022-23", start: 16,
			base: blend(12), arm: blend(16), hasWant: true, want: 7.826,
			why: "second carrier, a DIFFERENT season — which is why leave-one-season-out cannot see the concentration",
		},
	}

	for _, fc := range cells {
		pair, ok := byName[fc.season]
		if !ok {
			t.Fatalf("%s is not in this grid: %v", fc.season, sweepPairNames())
		}
		forensicOne(t, cfg, pair, fc, resetGlobals)
	}
}

func forensicOne(t *testing.T, cfg config.Config, pair seasonPair, fc forensicCell,
	resetGlobals func()) {
	t.Helper()
	fmt.Printf("\n%s\n", divider)
	fmt.Printf("%s — %s entering GW%d\n", fc.label, fc.season, fc.start)
	fmt.Printf("why this cell: %s\n\n", fc.why)

	// ⚠️ Each side is built and run to completion before the other is touched.
	// TEAMFORM's setting is process-global, so building both configs first and
	// simulating afterwards would run BOTH cells at whichever weight was set
	// last — a silent null that looks like a clean measurement.
	resetGlobals()
	base := sweepConfig(cfg, fc.start, false)
	fc.base(&base)
	bres, err := Simulate(pair.Cur, pair.Prior, base)
	if err != nil {
		t.Fatalf("baseline %s@%d: %v", fc.season, fc.start, err)
	}
	bh := HoldCaptaincyWeekly(pair.Cur, pair.Prior, base, bres.OpeningSquad)

	resetGlobals()
	arm := sweepConfig(cfg, fc.start, false)
	fc.arm(&arm)
	ares, err := Simulate(pair.Cur, pair.Prior, arm)
	if err != nil {
		t.Fatalf("arm %s@%d: %v", fc.season, fc.start, err)
	}
	ah := HoldCaptaincyWeekly(pair.Cur, pair.Prior, arm, ares.OpeningSquad)

	// The opening fifteen, which on HOLD is the entire intervention.
	inBase := map[int]bool{}
	for _, id := range bres.OpeningSquad {
		inBase[id] = true
	}
	inArm := map[int]bool{}
	for _, id := range ares.OpeningSquad {
		inArm[id] = true
	}
	var only [][2]string
	for _, id := range bres.OpeningSquad {
		if !inArm[id] {
			only = append(only, [2]string{"baseline only", describe(pair.Cur, id)})
		}
	}
	for _, id := range ares.OpeningSquad {
		if !inBase[id] {
			only = append(only, [2]string{"arm only", describe(pair.Cur, id)})
		}
	}
	if len(only) == 0 {
		fmt.Printf("OPENING FIFTEEN: identical. On HOLD that makes every weekly\n")
		fmt.Printf("difference below impossible — if any appears, something is wrong.\n\n")
	} else {
		fmt.Printf("OPENING FIFTEEN — %d of 15 differ:\n", len(only)/2)
		for _, o := range only {
			fmt.Printf("   %-14s %s\n", o[0], o[1])
		}
		fmt.Println()
	}

	// Real calendar dates beside the gameweek number, from the archive's own
	// kickoff times. "GW30" is not orientable — "GW30, 30 Mar 2024" is, and the
	// whole purpose of this probe is to let somebody check a week against what
	// they remember actually happening in it.
	when := gameweekDates(pair.Cur)

	fmt.Printf("%-4s %-15s %7s %7s %8s   %-22s %-22s\n",
		"gw", "weekend", "base", "arm", "delta", "baseline captain", "arm captain")
	type wk struct {
		gw, delta int
	}
	var weeks []wk
	total := 0
	for i := range bh.Full {
		d := ah.Full[i] - bh.Full[i]
		total += d
		mark := ""
		if d > 15 || d < -15 {
			mark = "  <<<"
		}
		fmt.Printf("%-4d %-15s %7d %7d %+8d   %-22s %-22s%s\n",
			bh.GW[i], when[bh.GW[i]], bh.Full[i], ah.Full[i], d,
			capName(pair.Cur, bh.Captain[i], bh.Vice[i], bh.GW[i]),
			capName(pair.Cur, ah.Captain[i], ah.Vice[i], ah.GW[i]), mark)
		weeks = append(weeks, wk{bh.GW[i], d})
	}
	n := len(bh.Full)
	got := float64(total) / float64(n)
	fmt.Printf("\ntotal %+d points over %d gameweeks = %+.3f pts/gw\n", total, n, got)

	// Reproduce the banked cell, or say so loudly. Tolerance is 0.01 because the
	// `want` figures are quoted to two decimals from the CSV, not because the
	// replay is approximate — it is exact, and a real mismatch means this probe
	// ran a different configuration from the sweep.
	// ⚠️ Guarded on `hasWant`, not on `want != 0`. A cell added without a want
	// would otherwise take Go's zero value and skip the check silently — the
	// "gate that cannot bind" pattern — and 0.000 is itself a legitimate paired
	// difference, so the sentinel and a real value were the same thing.
	if !fc.hasWant {
		t.Fatalf("%s %s@%d declares no `want`. Every forensic cell must state the "+
			"paired difference it reproduces, or the probe cannot know it is "+
			"explaining the right number.", fc.label, fc.season, fc.start)
	}
	if abs(got-fc.want) > 0.01 {
		t.Errorf("%s %s@%d: this probe reads %+.3f pts/gw and expects %+.3f on this "+
			"code. It is explaining a DIFFERENT number — check for a process-global "+
			"left set by an earlier cell, which is the bug this check exists for.",
			fc.label, fc.season, fc.start, got, fc.want)
	}
	if fc.banked != 0 {
		fmt.Printf("⚠️ the committed CSV has %+.3f for this cell, not %+.3f. That is a\n",
			fc.banked, got)
		fmt.Printf("   DATA-STATE difference, not a leak: this bank predates the archive\n")
		fmt.Printf("   repairs, and the BlendRateK cells banked on the current tree\n")
		fmt.Printf("   reproduce to three decimals. Quote the level with its state.\n")
	}

	sort.Slice(weeks, func(i, j int) bool {
		return abs(float64(weeks[i].delta)) > abs(float64(weeks[j].delta))
	})
	top := 0
	for i := 0; i < 3 && i < len(weeks); i++ {
		top += weeks[i].delta
	}
	fmt.Printf("the three biggest gameweeks are %s GW%d (%s, %+d), GW%d (%s, %+d), "+
		"GW%d (%s, %+d)",
		fc.season,
		weeks[0].gw, when[weeks[0].gw], weeks[0].delta,
		weeks[1].gw, when[weeks[1].gw], weeks[1].delta,
		weeks[2].gw, when[weeks[2].gw], weeks[2].delta)
	fmt.Printf(" = %+d of %+d\n", top, total)
	if total != 0 {
		fmt.Printf("so %.0f%% of this cell is three gameweeks.\n",
			100*float64(top)/float64(total))
	}
	fmt.Printf("\nRead the captain columns beside the delta: on HOLD the only two\n")
	fmt.Printf("channels are which fifteen was bought and who wore the armband.\n")
}

const divider = "=============================================================================="

// gameweekDates renders each gameweek's real calendar weekend from the archive's
// own kickoff times, so a week can be checked against what a reader remembers.
//
// The EARLIEST kickoff in the gameweek, because that is the weekend a human
// names it by — a Monday-night straggler or a rearranged Thursday fixture should
// not relabel the round. Blank where the archive carries no kickoff, which is
// real for some older seasons rather than a bug.
func gameweekDates(s *Season) map[int]string {
	first := map[int]time.Time{}
	for _, f := range s.Fixtures {
		if f.Event == nil || f.KickoffTime == nil {
			continue
		}
		gw := *f.Event
		if t, ok := first[gw]; !ok || f.KickoffTime.Before(t) {
			first[gw] = *f.KickoffTime
		}
	}
	out := map[int]string{}
	for gw, t := range first {
		out[gw] = t.Format("2 Jan 2006")
	}
	return out
}

// describe renders a player as a reader would recognise him.
func describe(s *Season, id int) string {
	p, ok := s.Players[id]
	if !ok {
		return fmt.Sprintf("id %d (not found)", id)
	}
	return fmt.Sprintf("%-18s %-3s %3d pts, %4d min over the season",
		p.WebName, posShort(p.Type), p.TotalPoints, p.Minutes)
}

// capName names the armband, and says when the vice took over — which is a real
// scoring channel rather than a detail, and is invisible in a points column.
func capName(s *Season, captain, vice, gw int) string {
	p, ok := s.Players[captain]
	if !ok {
		return "-"
	}
	if p.GWs[gw].Minutes == 0 {
		return fmt.Sprintf("%s blank->%s", p.WebName, webName(s, vice))
	}
	return fmt.Sprintf("%s (%d)", p.WebName, p.GWs[gw].Points)
}

func webName(s *Season, id int) string {
	if p, ok := s.Players[id]; ok {
		return p.WebName
	}
	return "-"
}

func posShort(elementType int) string {
	switch elementType {
	case 1:
		return "GKP"
	case 2:
		return "DEF"
	case 3:
		return "MID"
	case 4:
		return "FWD"
	}
	return "?"
}
