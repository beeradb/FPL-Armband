package backtest

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

// Does a TRAJECTORY -- a player rising or falling across the two seasons behind
// him -- separate inside the noise band, the same way TestDiagHaulChannel asked
// whether haul propensity does?
//
// Same instrument as that test: same-position pairs drawn from the top 30 by
// Score per position, paired only when both sit inside MinSeparableGain of each
// other, outcome is realised points the gameweek after the cutoff. What differs
// is the predictor. Haul channel asked about a player's SHAPE of returns within
// the season the model already sees; this asks about his DIRECTION across the
// two seasons the model does not look at, because nothing on the scoring path
// reads more than one season of history at a time.
//
// # Why a trend needs three seasons to say anything
//
// A single season's rate is a level, not a trajectory -- it cannot distinguish a
// player who has always played this well from one who arrived there this year.
// Telling those apart takes the rate from the season before as well, which is
// N-2 relative to the season being predicted. `sweepPairNames` was built for a
// two-season instrument (prior blend, current replay) and hands back exactly
// one prior per pair; this diagnostic loads a second one itself.
//
// RESULT (four seasons 2022-23..2025-26, 83,994 pairs, df 3, t_crit 3.182).
// Nothing separates inside the band except the engine itself: Score +0.184
// (n=80,999, SE 0.018, thr 0.057) RESOLVES and survives Holm (p=0.0020, Holm
// 0.0260) across a THIRTEEN-comparison family -- the five main effects below
// plus four price bands x two stratified predictors (minutes trend, pts/90
// trend). None of the other four main effects separates: minutes trend -0.038
// (thr 0.069), minutes LEVEL -0.059 (thr 0.192, the ungated control), pts/90
// trend -0.030 (thr 0.230), xGI/90 trend -0.046 (thr 0.128). None of the eight
// price-band comparisons clears its own uncorrected threshold either.
//
// ⚠️ Do NOT repeat the squash commit that merged this file (#156, "Measure
// whether trajectory separates decisions inside the band") saying "the gate
// earned it". The 900-minute rate gate is ILLUSTRATED by one case -- Palmer
// 2023-24, rate_gate_ok=0, ungated Man City xGI/90 FELL 0.46 to 0.20 while only
// his minutes rose, the season before a 244-point campaign -- and that case
// shows what the gate is FOR. It cannot show what the gate DOES to the
// estimate, because this test never banks the comparison that would show it.
// rate_gate_ok is 0 on 55,669 of the 83,994 pairs (verified against the CSV
// this test writes), and the writer emits "NA" into both d_pts90_trend and
// d_xgi90_trend on every one of those 55,669 rows, no exceptions. So there is
// no ungated rate-trend column anywhere in this output to compare the gated
// -0.030 / -0.046 against -- "does the gate move the estimate, or only shrink
// the sample it's computed on" cannot be answered from what this test
// currently writes. Answering it needs the ungated pair emitted alongside the
// gated one, which this test does not do.
func TestDiagTrajectoryChannel(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	band := cfg.Review.MinSeparableGain
	if band <= 0 {
		t.Fatal("MinSeparableGain is zero, so there is no band to pair inside")
	}

	const (
		firstCut     = 5  // cutoffs g = 5..37
		lastCut      = 37 // predict through+1, so 38 is the last outcome
		topN         = 30 // per position: the pool the optimiser actually chooses among
		rateGateMins = 900
	)

	// played is the set of seasons this harness is willing to call a completed
	// PLAY-THROUGH -- every season that appears as the CURRENT half of some pair
	// in sweepPairNames. A season that only ever shows up as somebody else's
	// PRIOR (2018-19, 2019-20 in the extended grid below) loads fine and its
	// per-gameweek rows are real, backfilled Understat numbers -- but the
	// harness itself has never scored a replay against it, and a trajectory
	// diagnostic that is trying to say something about the same population the
	// policy sweeps measure should not lean on a season those sweeps never
	// touch as an outcome. Deriving this from sweepPairNames rather than
	// hardcoding it means the set moves automatically if the grid ever does.
	played := map[string]bool{}
	for _, pair := range sweepPairNames() {
		played[pair[1]] = true
	}

	rows := [][]string{{"season", "gw", "pos", "code_hi", "code_lo", "name_hi",
		"d_score", "d_pts90_trend", "d_xgi90_trend", "d_mins_trend", "d_mins_level",
		"price_hi_tenths", "d_points", "rate_gate_ok"}}

	var totalPairs, totalGated int
	seasonCounts := map[string]int{}

	for _, pair := range sweepPairNames() {
		priorName, curName := pair[0], pair[1]
		n2Name := prevSeasonName(priorName)
		if !played[n2Name] {
			t.Logf("skip %s: N-2 (%s) is not a played season in this grid (prior=%s)",
				curName, n2Name, priorName)
			continue
		}
		t.Logf("keep %s: N-1=%s N-2=%s", curName, priorName, n2Name)

		cur := loadSeason(t, cfg, curName)
		prior := loadSeason(t, cfg, priorName)
		twoBack := loadSeason(t, cfg, n2Name)
		sc := sweepConfig(cfg, 1, false)

		// Season-level signals do not depend on the cutoff g -- both N-1 and
		// N-2 are seasons already complete by the time `cur` starts -- so
		// these are built once per (season, N-2) pair rather than once per g.
		ratesN1 := computeSeasonRates(prior.ByCode())
		ratesN2 := computeSeasonRates(twoBack.ByCode())

		idToCode := map[int]int{}
		byID := map[int]*Player{}
		for _, p := range cur.Players {
			idToCode[p.ID] = p.Code
			byID[p.ID] = p
		}

		before := len(rows) - 1
		for through := firstCut; through <= lastCut; through++ {
			// The engine is built exactly once per (season, g): everything
			// inside the position loop below reuses this same e.
			e, _ := EngineAt(cur, prior, through, sc)
			if e == nil {
				continue
			}

			byPos := map[string][]analysis.PlayerMetrics{}
			for _, m := range e.AllMetrics() {
				byPos[m.Position] = append(byPos[m.Position], m)
			}

			for pos, ms := range byPos {
				sort.SliceStable(ms, func(i, j int) bool { return ms[i].Score > ms[j].Score })
				if len(ms) > topN {
					ms = ms[:topN]
				}
				for i := 0; i < len(ms); i++ {
					for j := i + 1; j < len(ms); j++ {
						a, b := ms[i], ms[j]
						if a.Score-b.Score > band {
							// Sorted descending, so every later j is further away.
							break
						}
						ca, cb := idToCode[a.ID], idToCode[b.ID]
						if ca <= 0 || cb <= 0 {
							// No permanent Code to join on -- see ByCode's own
							// comment on why this join can never fall back to ID.
							continue
						}

						// Orient every column on d_score, hi = higher Score, so
						// the whole row reads as one consistent hi-minus-lo
						// comparison. Ties (d_score == 0) orient on Code, lower
						// wins, purely for a deterministic row -- Code carries
						// no ranking meaning here.
						hi, lo := a, b
						hiCode, loCode := ca, cb
						dScore := a.Score - b.Score
						switch {
						case dScore < 0:
							hi, lo = b, a
							hiCode, loCode = cb, ca
							dScore = -dScore
						case dScore == 0:
							if ca > cb {
								hi, lo = b, a
								hiCode, loCode = cb, ca
							}
						}

						// # Why the rate trends are gated and the minutes trend is not
						//
						// A per-90 rate is only a fair year-over-year comparison
						// between two SAMPLES of comparable size. A player who
						// played 200 minutes last season and 1,800 this one can
						// swing from a fluky 0.9 pts90 to a settled 0.5 without
						// having gotten any worse -- the early number was never
						// a rate on a season, it was a rate on three substitute
						// appearances. The canonical case this project has
						// already been burned by: a naive rate trend marks a
						// player like Cole Palmer as DECLINING the season before
						// his 244-point season, because his per-90 numbers at
						// Man City fell relative to a token handful of minutes
						// while only his MINUTES rose once he moved and started
						// playing every week. The signal that season actually
						// carried was a role change, not a rate change, and a
						// gate-free trend reads it backwards.
						//
						// Minutes share carries no such artifact -- a sum of
						// actual minutes divided by the season's maximum is
						// exactly as meaningful at 200 minutes as at 3,000, so
						// d_mins_trend and d_mins_level stay ungated and serve
						// as the control the gated columns are checked against.
						gateOK := ratesN1.minutes[hiCode] >= rateGateMins &&
							ratesN1.minutes[loCode] >= rateGateMins &&
							ratesN2.minutes[hiCode] >= rateGateMins &&
							ratesN2.minutes[loCode] >= rateGateMins

						dPts90Trend := "NA"
						dXgi90Trend := "NA"
						if gateOK {
							hiTrend := ratesN1.pts90[hiCode] - ratesN2.pts90[hiCode]
							loTrend := ratesN1.pts90[loCode] - ratesN2.pts90[loCode]
							dPts90Trend = f(hiTrend - loTrend)

							hiXGITrend := ratesN1.xgi90[hiCode] - ratesN2.xgi90[hiCode]
							loXGITrend := ratesN1.xgi90[loCode] - ratesN2.xgi90[loCode]
							dXgi90Trend = f(hiXGITrend - loXGITrend)
							totalGated++
						}

						// Minutes trend and level are ungated: they are the
						// control the rate trends are checked against, and
						// gating them the same way would make the control
						// vanish on exactly the pairs it exists to explain --
						// see the doc comment on rate_gate_ok below.
						hiMinsTrend := ratesN1.minsShare[hiCode] - ratesN2.minsShare[hiCode]
						loMinsTrend := ratesN1.minsShare[loCode] - ratesN2.minsShare[loCode]
						dMinsTrend := hiMinsTrend - loMinsTrend
						dMinsLevel := ratesN1.minsShare[hiCode] - ratesN1.minsShare[loCode]

						gateCol := "0"
						if gateOK {
							gateCol = "1"
						}

						rows = append(rows, []string{
							curName, strconv.Itoa(through + 1), pos,
							strconv.Itoa(hiCode), strconv.Itoa(loCode), hi.Name,
							f(dScore),
							dPts90Trend,
							dXgi90Trend,
							f(dMinsTrend),
							f(dMinsLevel),
							strconv.Itoa(int(math.Round(hi.Price * 10))),
							f(float64(realised(byID, hi.ID, through+1) -
								realised(byID, lo.ID, through+1))),
							gateCol,
						})
					}
				}
			}
		}

		n := len(rows) - 1 - before
		seasonCounts[curName] = n
		totalPairs += n
		t.Logf("%s: %d pairs", curName, n)
	}

	t.Logf("total: %d pairs across %d seasons, %d passed the rate gate",
		totalPairs, len(seasonCounts), totalGated)
	for season, n := range seasonCounts {
		if totalPairs > 0 && float64(n)/float64(totalPairs) > 0.40 {
			t.Logf("WARNING: %s carries %d/%d pairs (%.1f%%), over the 40%% void-check limit",
				season, n, totalPairs, 100*float64(n)/float64(totalPairs))
		}
	}
	if totalPairs < 2000 {
		t.Logf("WARNING: only %d pairs total, below the ~2000 void-check floor", totalPairs)
	}

	// Banked outside the repo for the same reason TestDiagHaulChannel's output
	// is: this is a measurement, not testdata, and a hardcoded path (no env
	// override) keeps it that way -- see that test's comment on
	// TestEnvSwitchListIsComplete for why a second, environment-driven way to
	// choose the output location would itself be an undeclared switch.
	dir := "/work/drop/trajectory-2026-08-30"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/pairs.csv"
	fh, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	w := csv.NewWriter(fh)
	if err := w.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	t.Logf("wrote %s (%d pairs)", path, len(rows)-1)
}

// seasonRates is the three per-90 trailing signals this diagnostic reads from one
// completed season, plus the raw minutes total the rate gate below needs, all
// indexed by Player.Code.
type seasonRates struct {
	pts90     map[int]float64
	xgi90     map[int]float64
	minsShare map[int]float64
	minutes   map[int]int
}

// computeSeasonRates derives pts90, xgi90, minsShare and total minutes from one
// completed season, keyed by Code rather than ID.
//
// # Why Code, never ID
//
// season.go:559 is explicit that Code is "FPL's permanent code, which is the
// only identifier" that survives a season boundary -- element ids are reassigned
// every summer, so the same footballer can hold id 214 one year and id 87 the
// next, while a DIFFERENT footballer now holds 214. A two-hop join through ID --
// look up this year's id, use it to index last year's players -- does not fail
// loudly when that happens. It returns a row, for the wrong player, and every
// number downstream of it is a comparison between two careers that were never
// the same career. `Season.ByCode` exists precisely so nothing has to take that
// risk, and every read in this file goes through it.
//
// # Why an absent Code reads as a zero-minute season, not a missing one
//
// A player with no entry in `byCode` did not play in the top flight that
// season -- relegated the year before, or not yet promoted to it, or a target's
// permanent code not yet issued. That is the same fact "0 if minutes==0" already
// covers for a player who WAS in the league and simply never took the pitch; not
// having a season at all is that fact one step further back, not a different
// kind of missingness. Go's map zero value already returns 0.0 / 0 for an absent
// key, so the caller needs no separate presence check to get the right answer --
// only the rate gate needs to know minutes specifically, which is why minutes is
// carried alongside the rates rather than folded into them.
func computeSeasonRates(byCode map[int]*Player) seasonRates {
	r := seasonRates{
		pts90:     map[int]float64{},
		xgi90:     map[int]float64{},
		minsShare: map[int]float64{},
		minutes:   map[int]int{},
	}
	for code, p := range byCode {
		var mins, pts int
		var xg, xa float64
		for _, g := range p.GWs {
			mins += g.Minutes
			pts += g.Points
			xg += g.XG
			xa += g.XA
		}
		r.minutes[code] = mins
		r.minsShare[code] = float64(mins) / (38 * 90)
		if mins > 0 {
			r.pts90[code] = float64(pts) * 90 / float64(mins)
			r.xgi90[code] = (xg + xa) * 90 / float64(mins)
		}
		// mins == 0 leaves pts90/xgi90 at the map's zero value, which is the
		// same 0 the "0 if minutes==0" definition asks for.
	}
	return r
}

// prevSeasonName returns the season immediately before name: "2022-23" -> "2021-22".
//
// String arithmetic rather than a lookup table, because the table this
// diagnostic actually needs to consult is `played` above, not a hardcoded list
// of valid season names -- the archive's own boundary (2016-17) is never
// reached from any of the four seasons this grid keeps, so there is nothing here
// that needs to know where the archive starts.
func prevSeasonName(name string) string {
	var y1, y2 int
	if _, err := fmt.Sscanf(name, "%4d-%2d", &y1, &y2); err != nil {
		panic("prevSeasonName: " + name + " is not a YYYY-YY season name: " + err.Error())
	}
	return fmt.Sprintf("%04d-%02d", y1-1, y2-1)
}
