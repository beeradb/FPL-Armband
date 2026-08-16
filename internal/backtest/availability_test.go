package backtest

// Can the model see a player about to stop playing?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagAvailability -v -timeout 60m
//
// TestDiagTransferError located the transfer policy's whole sell-side error in
// the 13% of sold players who never appeared again: −0.100 pts/gw for the ones
// who kept playing, −2.223 for the ones who stopped. That is not a scoring
// failure, it is an availability failure, and it is worth being exact about what
// it does and does not imply.
//
// (Those are the figures at the shipped min_gain of 0.4. An earlier version of
// this comment quoted +0.065 and −3.280 over 14% of moves, which was measured at
// the 0.7 that shipped briefly and was retracted — see the note in
// transfererror_test.go. The split survives the correction; only its magnitudes
// moved.)
//
// # The sign is the opposite of a loss, and the loss is elsewhere
//
// Those 30 moves *sold* a player the model still rated at ~3.3 pts/gw who then
// delivered nothing. The sale was therefore better than predicted: the policy
// got lucky. Nothing in that column is a cost.
//
// The cost is the same error on the other two populations, which no
// transfer-judging diagnostic can see because they generate no move at all: the
// player the model **keeps** because it still rates him, and the player it
// **buys** who then stops. Both are the identical modelling error — a footballer
// who has stopped playing still reading as a starter — and only its harmless
// manifestation happens to be measurable through transfers.
//
// So this measures the error itself rather than its shadow: across every player
// and every cutoff, what does the model expect, and what do they actually play?
//
// # The candidate signal
//
// MinutesHalfLife (4) already weights recent gameweeks, which is the documented
// fix for a dropped player reading as an ever-present. The question is whether
// anything is *left* after it — specifically whether a run of consecutive
// zero-minute gameweeks predicts continued absence beyond what an exponential
// average of the same gameweeks already carries.
//
// It might well not. An EWMA over a run of zeroes is already small. But the two
// are different shapes: an EWMA is symmetric in what produced the zeroes, while
// a *run* is the signature of a state — injured, dropped, gone — that persists
// until something changes it. If the run adds nothing, the model is doing as
// well as this data allows and the remaining gap is genuinely team news.
//
// Note the standard AGENTS.md sets for acting on this: recency on minutes works
// because it removes a **bias**, where recency on rates failed because it traded
// bias for variance. A blank-run signal is bias reduction of the same kind — a
// player who is not playing really is not playing — so it is the safe sort. That
// licenses measuring it, not shipping it.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

// availObs is one player at one cutoff: what the model expected of his minutes,
// and what he went on to play.
//
// ⚠️ It lives here rather than inside TestDiagAvailability's body because
// TestDiagAvailabilityByPosition re-runs the *same* calibration cut by position,
// and a second copy of this loop is exactly the failure AGENTS.md calls this
// project's signature one. The population filters — established players only,
// the trailing-run definition, the five-gameweek window — are the calibration.
// Two spellings of them would make the position split answer a question the
// pooled table never asked, and nothing would say so.
type availObs struct {
	expected float64 // modelled minutes per gameweek at the cutoff
	actual   float64 // realised minutes per gameweek over the window
	run      int     // consecutive zero-minute gameweeks ending at the cutoff
	played   int     // gameweeks with any minutes, before the cutoff
	flagged  bool    // FPL already says he is unavailable or doubtful
	season   string  // the current season, i.e. the one being predicted
	code     int     // permanent player code, the only stable player key
	cutoff   int     // the gameweek the prediction was made after
	elemType int     // 1 GKP, 2 DEF, 3 MID, 4 FWD
}

// availabilityCutoffs is every fourth gameweek: enough cutoffs to cover the
// season without paying for an engine build per week.
var availabilityCutoffs = []int{5, 9, 13, 17, 21, 25, 29, 33}

// availabilityWindow is how many gameweeks ahead the realised minutes are taken
// over.
const availabilityWindow = 5

func collectAvailabilityObs(t *testing.T, cfg config.Config) []availObs {
	t.Helper()
	pairs := sweepPairNames()
	cutoffs := availabilityCutoffs
	const window = availabilityWindow

	type obs = availObs
	var all []obs

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		idx := newPriorIndex(prior)

		for _, cut := range cutoffs {
			boot, fx := PointInTime(cur, prior, cut)
			e := analysis.NewEngineFull(boot, fx, cfg.Weights,
				analysis.Congestion{}, analysis.RoleRisk{})
			e.Priors = idx
			e.Recent = newRecentIndexWith(cur, cut, cfg.Weights.MinutesHalfLife,
				cfg.Weights.RateHalfLife)

			for i := range boot.Elements {
				el := &boot.Elements[i]
				p := cur.Players[el.ID]
				if p == nil {
					continue
				}
				// Only players who were established at the cutoff. Asking
				// whether a squad player stays a squad player is a different
				// question, and the money is in the starters.
				var mins, appearances float64
				for gw := 1; gw <= cut; gw++ {
					if g, ok := p.GWs[gw]; ok && g.Minutes > 0 {
						mins += float64(g.Minutes)
						appearances++
					}
				}
				if appearances < 5 || mins/appearances < 60 {
					continue
				}

				// Trailing run of blanks: gameweeks that exist in the record
				// with zero minutes, counting back from the cutoff.
				run := 0
				for gw := cut; gw >= 1; gw-- {
					g, ok := p.GWs[gw]
					if !ok {
						continue
					}
					if g.Minutes > 0 {
						break
					}
					run++
				}

				var future float64
				weeks := 0
				for gw := cut + 1; gw <= cut+window && gw <= 38; gw++ {
					if g, ok := p.GWs[gw]; ok {
						future += float64(g.Minutes)
					}
					weeks++
				}
				if weeks == 0 {
					continue
				}
				all = append(all, obs{
					// Whether FPL had already said so at the cutoff. In a
					// replay this is only what statusAt can reconstruct — a
					// final status of u or i, carried back from its news
					// timestamp — so it is a floor on what a live run sees.
					flagged:  el.Status != "" && el.Status != "a",
					expected: e.Metrics(el).ExpectedMinutes,
					actual:   future / float64(weeks),
					run:      run,
					played:   int(appearances),
					season:   pair[1],
					code:     el.Code,
					cutoff:   cut,
					elemType: el.ElementType,
				})
			}
		}
	}
	return all
}

// ⚠️ This test's output IS the table quoted in `analysis.blankRunFactor`'s doc
// comment, and since that term shipped, re-running it at shipped config no
// longer measures what the table says it does. `Metrics` assigns
// `ExpectedMinutes` from `blendFor`, which applies `blankRunFactor`, so the
// run-1/2/3 rows come back as the RESIDUAL AFTER the correction rather than the
// signal it was fitted to — with the same columns, the same headers and a
// plausibly small bias, which reads as "the correction is about right" rather
// than as a circular fit.
//
// So it refuses the same way `TestDiagAvailabilityByPosition` does. `analysis`
// reads FPL_NO_BLANK_RUN once at package init, so the switch has to be in the
// process environment and cannot be set from in here. Run it as:
//
//	FPL_NO_BLANK_RUN=1 DIAG=1 go test ./internal/backtest -run TestDiagAvailability -v
//
// The two tests now share one collector, which makes their outputs look like one
// measurement at two granularities. Guarding only the newer one would have made
// that reading wrong in exactly the direction nobody checks.
func TestDiagAvailability(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	if os.Getenv("FPL_NO_BLANK_RUN") == "" {
		t.Fatal("set FPL_NO_BLANK_RUN=1 as well: ExpectedMinutes carries " +
			"blankRunFactor, so at shipped config this measures the residual " +
			"after the term rather than the signal it was fitted to. See the " +
			"note above this test.")
	}
	cfg := loadConfig(t)
	pairs := sweepPairNames()
	const window = availabilityWindow

	type obs = availObs
	all := collectAvailabilityObs(t, cfg)
	if len(all) < 200 {
		t.Skipf("only %d observations", len(all))
	}

	fmt.Printf("\n%d player-cutoffs, %s, established players only\n", len(all), seasonsLabel(len(pairs)))
	fmt.Printf("(5+ appearances averaging 60+ minutes at the cutoff).\n")
	fmt.Printf("Predicting mean minutes per gameweek over the next %d.\n\n", window)

	summarise := func(label string, rows []obs) {
		if len(rows) == 0 {
			return
		}
		var exp, act, err, abs float64
		vanished := 0
		for _, r := range rows {
			exp += r.expected
			act += r.actual
			err += r.actual - r.expected
			d := r.actual - r.expected
			if d < 0 {
				d = -d
			}
			abs += d
			if r.actual == 0 {
				vanished++
			}
		}
		n := float64(len(rows))
		fmt.Printf("%-22s %5d %9.1f %9.1f %+9.1f %8.1f %8.0f%%\n",
			label, len(rows), exp/n, act/n, err/n, abs/n,
			100*float64(vanished)/n)
	}

	fmt.Printf("%-22s %5s %9s %9s %9s %8s %9s\n",
		"trailing blanks", "n", "expected", "actual", "bias", "MAE", "vanished")
	summarise("all", all)
	fmt.Printf("\n")

	byRun := map[int][]obs{}
	for _, r := range all {
		k := r.run
		if k > 4 {
			k = 4
		}
		byRun[k] = append(byRun[k], r)
	}
	for k := 0; k <= 4; k++ {
		label := fmt.Sprintf("%d", k)
		if k == 4 {
			label = "4 or more"
		}
		summarise(label, byRun[k])
	}

	fmt.Printf("\n'vanished' is the share who record no minutes at all over the window —\n")
	fmt.Printf("the population the transfer diagnostic could only see when it sold one.\n")
	fmt.Printf("If the model already prices a run of blanks, bias should not grow with it.\n")

	// The control that decides whether any of this is worth building.
	//
	// availabilityFactor already discounts a flagged player, and live FPL flags
	// most real injuries. If the bias lives in the flagged group, the model
	// already has the channel and the replay is simply missing flags it would
	// have in production. If it lives in the *unflagged* group, the blank run
	// is carrying information nothing else has.
	// The pool cliff Simulate builds its squads with.
	const poolFloor = 55.0

	// How much of this population the optimiser never sees anyway.
	//
	// MinExpectedMinutes (55) is a cliff, not a discount: a player below it is
	// not scored lower, he is dropped from the squad pool entirely. Run-1
	// players average 54.0 expected minutes, which is already the wrong side of
	// it, so a further discount may be pushing players below a line they had
	// mostly already crossed. That would explain a real bias worth no points.
	fmt.Printf("\n=== share already below MinExpectedMinutes (%.0f), the pool cliff\n\n",
		poolFloor)
	fmt.Printf("%-16s %6s %10s %10s\n", "trailing blanks", "n", "below", "share")
	for k := 0; k <= 4; k++ {
		var n, below int
		for _, r := range all {
			kk := r.run
			if kk > 4 {
				kk = 4
			}
			if kk != k {
				continue
			}
			n++
			if r.expected < poolFloor {
				below++
			}
		}
		if n == 0 {
			continue
		}
		label := fmt.Sprintf("%d", k)
		if k == 4 {
			label = "4 or more"
		}
		fmt.Printf("%-16s %6d %10d %9.0f%%\n", label, n, below, 100*float64(below)/float64(n))
	}

	fmt.Printf("\n=== split by whether FPL had already flagged him at the cutoff\n")
	fmt.Printf("(in a replay this is only what statusAt reconstructs, so it is a floor)\n\n")
	fmt.Printf("%-22s %5s %9s %9s %9s %8s %9s\n",
		"trailing blanks", "n", "expected", "actual", "bias", "MAE", "vanished")
	for _, want := range []bool{false, true} {
		head := "UNFLAGGED"
		if want {
			head = "FLAGGED"
		}
		fmt.Printf("%s\n", head)
		for k := 0; k <= 4; k++ {
			var rows []obs
			for _, r := range all {
				kk := r.run
				if kk > 4 {
					kk = 4
				}
				if kk == k && r.flagged == want {
					rows = append(rows, r)
				}
			}
			label := fmt.Sprintf("  %d", k)
			if k == 4 {
				label = "  4 or more"
			}
			summarise(label, rows)
		}
	}
}

// TestDiagAvailabilityImpact re-measures the statusAt reconstruction
// ("Give the replay the availability data the archive actually has") at the
// resolution this project now requires for a verdict.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagAvailabilityImpact -v -timeout 60m
//
// AGENTS.md recorded the reconstruction's value as +273 on the held metric,
// "comfortably above the jitter floor". That figure is **retracted**: the table
// was three cells in one season (hold@1/hold@11/hold@21 for 2023-24 alone), and
// that season is the one where Kane's move overlapped a GW1 deadline. Measured
// at 24 cells it is about **8 points a season**. This project's own standard,
// set once the transfer threshold sweep went from "noise" at twelve cells to
// t = +3.36 at twenty-four, is that a verdict reached at twelve cells or fewer
// is unverified. Three is far short of even that.
//
// statusAt checks FPL_NO_AVAILABILITY on every call rather than caching it at
// package init (unlike oraclePrices), so it can be toggled between cells in a
// single process and paired properly through reportPairedDifferences — the
// same seasons by start points every other paired result in AGENTS.md uses.
// (The recorded 8-points-a-season figure above was measured when that grid was
// 24 cells; the header this test prints counts whatever runs.)
func TestDiagAvailabilityImpact(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	defer os.Unsetenv("FPL_NO_AVAILABILITY")

	starts := sweepStarts()
	fmt.Printf("\n=== availability reconstruction (statusAt), full grid: %s.\n",
		gridLabel(len(sweepPairNames()), len(starts)))
	fmt.Printf("Both metrics reported: the opening squad changes (HOLD), and it\n")
	fmt.Printf("cascades into transfers (POLICY).\n")
	runPolicySweep(t, []policyVariant{
		{label: "reconstructed (ships)", apply: func(sc *SimConfig) {
			os.Unsetenv("FPL_NO_AVAILABILITY")
		}},
		{label: "blind (pre-fix)", apply: func(sc *SimConfig) {
			os.Setenv("FPL_NO_AVAILABILITY", "1")
		}},
	}, starts)
}
