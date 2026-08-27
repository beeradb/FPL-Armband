package backtest

// Where is the transfer policy's estimate wrong, and by how much?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagTransferError -v -timeout 60m
//
// AGENTS.md's central claim about this policy is that a transfer decision is an
// **argmax**, so it does not live in the middle of the estimate distribution but
// in its tail: the player the search picks is disproportionately the one whose
// estimate is too high. That claim explains why better predictors have
// repeatedly made worse policies — recency on rates, the unified search, the
// magnitude ladders — and it has never been measured directly on the shipped
// model. Two figures for it survive only as prose in a comment.
//
// This measures it. Every move the policy makes carries the modelled score of
// both players; Judge carries what they actually returned over the window the
// decision was justified on. The difference is the estimation error, split by
// side:
//
//	buy error  = actual pts/gw of the player bought  - his modelled score
//	sell error = actual pts/gw of the player sold    - his modelled score
//
// Negative means the model rated him too highly. The argmax thesis predicts a
// specific, asymmetric signature: the buy side should be **more** negative than
// the sell side, because the search actively hunts the top of the distribution
// on the way in and merely accepts what it already owns on the way out.
//
// # What it licenses, and what it does not
//
// The gate is `gain x horizon >= charge`, and every constant in it has been
// swept to a spread with no structure. A measured bias with a known size is
// better evidence than the argmax of six noisy numbers — but only if it has the
// right *shape*.
//
// It does not. Once the funding legs are separated out (see fundingLeg below,
// which is where the first version of this went wrong) the buy-side error is
// flat in the modelled gain, not growing with it. A flat correction on the buy
// side is arithmetically the same as demanding that much more gain before
// moving, which is precisely what MinGain already does — so the measurement
// implies a value for an existing constant rather than a new mechanism.
// TestDiagTransferPolicy experiment C tests exactly that.
//
// The finding that does stand on its own is the sold-player split at the bottom
// of the output: the sell side is well calibrated for players who keep playing,
// and every bit of its error comes from the minority who stop.
//
// # Read the shipped config, not the figures this comment was written under
//
// Every number quoted above and below was produced at
// min_gain_for_free_transfer = 0.7. This test was added in the same commit that
// raised that constant, the raise was retracted three commits later, and nothing
// re-ran the diagnostic — so the file's headline "buy side over-rated by 0.53
// pts/gw" describes a setting that has not shipped since.
//
// Re-run at the shipped 0.4 the gate admits 327 moves rather than 222, and the
// asymmetry the argmax thesis predicts is absent: buy side −0.230 median and
// +0.079 mean, sell side −0.282 and −0.255, so buy-minus-sell is +0.051 rather
// than −0.474. The sold-player split survives (−0.100 for the 284 who appeared
// against −2.223 for the 43 who did not), which is why that is the half AGENTS.md
// still stands behind.
//
// The lesson is procedural: a diagnostic whose output is quoted as a constant's
// justification has to be re-run whenever that constant moves, and this one
// justified the very knob that moved.

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

func TestDiagTransferError(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()

	pairs := sweepPairNames()
	get := func(sn string) *Season { return loadSeason(t, cfg, sn) }

	type obs struct {
		gain              float64 // modelled gain per gameweek
		buyErr, sellErr   float64
		netErr            float64 // realised gain/gw minus modelled gain
		outPlayed         bool
		hit               bool
		realisedGain      float64
		inScore, outScore float64
		fundingLeg        bool
	}
	var all []obs

	// Named, so the provenance labels below can be derived from it rather than
	// spelling a season count nobody re-checks when the grid moves. Both labels
	// said "4 seasons x 3 start points" while this ran on six, and one of them is
	// banked into the model CSV as the row's grid — a wrong data state written into
	// the provenance field, which is the failure that field exists to prevent.
	errStarts := []int{1, 11, 21}
	for _, start := range errStarts {
		for _, pair := range pairs {
			prior, cur := get(pair[0]), get(pair[1])
			sc := seasonConfig(cfg, pair[1], start, false)
			res, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range Judge(cur, res.Moves, cfg.Weights.Horizon) {
				if v.Weeks <= 0 {
					continue
				}
				inActual := float64(v.InPoints) / float64(v.Weeks)
				outActual := float64(v.OutPoints) / float64(v.Weeks)
				all = append(all, obs{
					// decide() reports a pair's gain once, on its first leg,
					// and zeroes the rest. So a zero here is a funding leg
					// rather than a move the policy thought was worthless, and
					// bucketing on gain without splitting them apart puts every
					// downgrade in the bottom quartile and calls it a finding.
					fundingLeg:   v.Gain == 0,
					gain:         v.Gain,
					buyErr:       inActual - v.InScore,
					sellErr:      outActual - v.OutScore,
					netErr:       (inActual - outActual) - v.Gain,
					realisedGain: inActual - outActual,
					outPlayed:    v.OutPlayed,
					hit:          v.Hit,
					inScore:      v.InScore,
					outScore:     v.OutScore,
				})
			}
		}
	}
	if len(all) < 40 {
		t.Skipf("only %d moves to judge", len(all))
	}

	collect := func(rows []obs, f func(obs) float64) (med, mean float64) {
		var xs []float64
		for _, r := range rows {
			xs = append(xs, f(r))
		}
		return median(xs), meanOf(xs)
	}

	var priced, legs []obs
	for _, o := range all {
		if o.fundingLeg {
			legs = append(legs, o)
		} else {
			priced = append(priced, o)
		}
	}

	fmt.Printf("\n%d transfers judged, %d seasons x %d start points.\n",
		len(all), len(pairs), len(errStarts))
	fmt.Printf("%d carry their own modelled gain; %d are funding legs of a pair, whose\n",
		len(priced), len(legs))
	fmt.Printf("gain is reported once on the lead move and zeroed here.\n")
	fmt.Printf("Error is realised minus modelled, per gameweek. Negative = over-rated.\n\n")

	bm, bmean := collect(all, func(o obs) float64 { return o.buyErr })
	sm, smean := collect(all, func(o obs) float64 { return o.sellErr })
	nm, nmean := collect(all, func(o obs) float64 { return o.netErr })
	fmt.Printf("%-28s %9s %9s\n", "", "median", "mean")
	fmt.Printf("%-28s %+9.3f %+9.3f\n", "buy side (player in)", bm, bmean)
	fmt.Printf("%-28s %+9.3f %+9.3f\n", "sell side (player out)", sm, smean)
	fmt.Printf("%-28s %+9.3f %+9.3f\n", "net gain error", nm, nmean)

	// Emitted for the accuracy snapshot. The grid string records the move count
	// because this diagnostic's population *is* the gate setting: at
	// min_gain_for_free_transfer 0.7 it judged 222 moves and reported a buy-side
	// median of -0.760, and at the shipped 0.4 it judges 327 and reports -0.230.
	// Those are different populations, not a reproduction failure, and a figure
	// quoted without its move count invites exactly the mistake that put a
	// retracted -0.53 into three later sections of AGENTS.md as settled fact.
	grid := fmt.Sprintf("%d moves judged, %d seasons x %d start points, min_gain %.2f",
		len(all), len(pairs), len(errStarts), cfg.Review.MinGainForTransfer)
	sink.emitAll("transfer_error", grid, "buy side (player bought)", len(all),
		measure{"median", bm}, measure{"mean", bmean})
	sink.emitAll("transfer_error", grid, "sell side (player sold)", len(all),
		measure{"median", sm}, measure{"mean", smean})
	sink.emitAll("transfer_error", grid, "asymmetry (buy minus sell)", len(all),
		measure{"median", bm - sm}, measure{"mean", bmean - smean})

	// The asymmetry is the argmax signature. A model that is simply badly
	// calibrated moves both sides together; a search that hunts the tail moves
	// the buy side further.
	fmt.Printf("\nasymmetry (buy minus sell, median): %+.3f\n", bm-sm)

	// Does the error scale with the modelled gain? If the biggest-looking moves
	// are the most over-estimated, the gate wants to be steeper than linear —
	// the current gate is `gain x horizon >= charge`, which is linear.
	sort.Slice(priced, func(i, j int) bool { return priced[i].gain < priced[j].gain })
	fmt.Printf("\n%-16s %5s %9s %9s %9s %9s\n",
		"modelled gain", "n", "mean gain", "realised", "net err", "buy err")
	q := len(priced) / 4
	for b := 0; b < 4; b++ {
		lo, hi := b*q, (b+1)*q
		if b == 3 {
			hi = len(priced)
		}
		rows := priced[lo:hi]
		_, g := collect(rows, func(o obs) float64 { return o.gain })
		_, r := collect(rows, func(o obs) float64 { return o.realisedGain })
		_, n := collect(rows, func(o obs) float64 { return o.netErr })
		_, be := collect(rows, func(o obs) float64 { return o.buyErr })
		fmt.Printf("%-16s %5d %9.3f %9.3f %+9.3f %+9.3f\n",
			[]string{"lowest quarter", "second", "third", "highest quarter"}[b],
			len(rows), g, r, n, be)
	}

	// The sold player who never played again. AGENTS.md notes this is ~19% of
	// transfers and that Net() overstates them, because an autosub would have
	// covered a blanking starter for free.
	var played, blanked []obs
	for _, o := range all {
		if o.outPlayed {
			played = append(played, o)
		} else {
			blanked = append(blanked, o)
		}
	}
	pm, _ := collect(played, func(o obs) float64 { return o.sellErr })
	blm, _ := collect(blanked, func(o obs) float64 { return o.sellErr })
	fmt.Printf("\nsold player appeared:     n=%3d  sell error median %+.3f\n", len(played), pm)
	fmt.Printf("sold player never played: n=%3d  sell error median %+.3f  (%.0f%% of moves)\n",
		len(blanked), blm, 100*float64(len(blanked))/float64(len(all)))
	// The split that reframes the sell-side error as an availability problem
	// rather than a scoring one: for a player who keeps playing the model is close
	// to right, and nearly all the error sits in the minority who stop.
	sink.emitAll("transfer_error", grid, "sell error: sold player played again",
		len(played), measure{"median", pm})
	sink.emitAll("transfer_error", grid, "sell error: sold player never played again",
		len(blanked), measure{"median", blm})

	// Deliberately no "the gate should charge X" line here. Gain is a change in
	// XIValue and the realised figure is a raw two-player points difference, so
	// netErr carries a mechanical positive bias: a sold player who was not in
	// the eleven costs the XI nothing, and the modelled gain knows that while
	// the realised difference does not. Read buy and sell error, which compare
	// like with like, and read netErr only across buckets rather than as a
	// level.
	_ = math.Abs
}
