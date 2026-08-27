package backtest

import (
	"context"
	"fmt"
	"sort"
	"testing"
)

// TestDiagTeamNewsTransferValue prices knowing who plays **as a reason to make a
// transfer**, which is what the knowledge is actually for.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagTeamNewsTransferValue -v
//
// # The question, and why the earlier measurements were not it
//
// A manager finds out an hour before the deadline that his midfielder is not in
// the squad. The value of knowing is that he **sells him and buys someone who is
// playing**. It is not that he reorders his bench.
//
// Two measurements in this package answered the bench question by construction
// and should not be read as answering this one:
//
//   - `TestDiagLineupEventValue` holds the opening fifteen fixed and oracles only
//     the weekly re-pick, so a transfer is impossible in it.
//   - `TestDiagAutosubValue` prices optimal bench reordering at **+0.1005 points
//     per blank**, which at 1.831 blanks a gameweek is about +7.0 over 38. It
//     bounds a *different* quantity from this arm's eleven-only rung — it
//     maximises realised points over a fixed eleven with no armband, where this
//     supplies only whether a player features and then re-picks the eleven and the
//     captain on `Score` — so the two are not comparable and were once wrongly
//     reported as agreeing.
//
// This arm lets the policy act. `OracleFeatures` rewrites `Element.Status` from
// whether the player actually featured in the gameweek about to be played, and
// `FeaturesFrom` is set to the entry gameweek so the **opening fifteen is bought
// on honest information in both arms**. That is a checkable invariance — the
// squads must come back byte-identical.
//
// ⚠️ That does **not** make every difference a transfer. The oracle rewrites
// `Status`, which the weekly eleven reads too, so a `POLICY` difference is the
// transfer channel *plus* the bench channel. Both metrics are therefore run in
// one process over the same cells, and **`POLICY` minus `HOLD` is the transfer
// channel** — a paired diff-in-diff rather than a subtraction of two numbers
// measured separately, which is what the statistical review asked for when the
// same question came up for the squad channel.
//
// # What this is a bound on, and what it is not
//
// ⚠️ **It is NOT an upper bound**, though it was written as one, and the reason
// is the gate rather than the oracle. `Status` reaches `Score` through
// `availabilityFactor`, which returns exactly 0, and the transfer gate then
// charges `gain x DecisionHorizon` — so **a one-week absence is priced as a
// five-week write-off**. The arm over-reacts to its own information by
// construction, cannot buy anyone who blanks in the transfer week however much it
// wants him for the next five, and never learns that an absence persists. A
// system with the same knowledge and a correct representation of *duration* would
// beat it. Read the figure as a dirty measurement in both directions.
//
// Real sources — lineup-prediction sites, late rumours, press conferences — are
// partial, and FPL's own published flags are the *late* end of that range rather
// than the good end. The recovered-capture arm (`OracleTeamNews`) prices FPL's
// actual published flag at **POLICY +25 a season against a threshold of 37**, so
// roughly half of what is measured here is already available free to a live user
// and the incremental value of a *better* source is nearer +30 — neither figure
// resolving. The baseline here is the deliberately blinded replay, not the live
// system.
//
// ⚠️ **The two arms are not gated the same way, so their difference is not clean.**
// `applyTeamNews` has no `FeaturesFrom` equivalent and therefore moves the opening
// fifteen, while this arm is pinned from `start+1`. Subtracting them mixes in a
// squad-selection channel present in only one of them.
//
// ⚠️ It is a **one-gameweek** fact. A transfer costs a scarce resource and is
// judged over the decision horizon, so acting on a single week's absence may well
// be the wrong move even when the information is perfect — the policy is free to
// decline, which is exactly why this is an information oracle rather than a
// decision oracle. *Hypothesis, untested:* most of the recoverable value is in
// absences that persist, and a one-week oracle will therefore understate what a
// real multi-week injury report is worth. `OracleMinutes` at a longer window is
// the arm that would test it.
func TestDiagTeamNewsTransferValue(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	type cell struct {
		season               string
		start                int
		basePts, withPts     int
		baseMoves, withMoves int
		baseHits, withHits   int
		baseHold, withHold   int
		squadsIdentical      bool
		weeks                int
		// The same contrast with hits forbidden, so the information's value and
		// the willingness to pay for it are separated rather than summed.
		freeBase, freeWith         int
		freeBaseMoves, freeWithMov int
	}
	var cells []cell

	for _, pair := range sweepPairNames() {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatalf("loading %s: %v", pair[0], err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatalf("loading %s: %v", pair[1], err)
		}
		for _, start := range sweepStarts() {
			sc := sweepConfig(cfg, start, true)
			// sweepConfig seeds Oracles from the environment, and every treatment
			// arm below assigns cfg.Oracles wholesale — so a stray oracle switch
			// left exported from an earlier run would reach the BASELINE and not
			// the treatment, and the printed difference would silently be
			// "team news minus perfect price timing". validateOracleArms refuses
			// exactly this for sweeps; a standalone diagnostic has no equivalent.
			if sc.Oracles != (Oracles{}) {
				t.Fatalf("an oracle switch is set in the environment (%s), which "+
					"would reach the baseline arm only and contaminate every "+
					"difference printed here", sc.Oracles.Stamp())
			}
			base, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatalf("simulating %s@%d: %v", pair[1], start, err)
			}

			// Free transfers only. The realistic behaviour is holding a free
			// transfer in reserve until team news lands rather than paying four
			// points for the privilege of reacting, so this arm is the one that
			// answers "what is the information worth", and the hit-taking arm
			// below answers the separate question "does paying for it pay".
			fc := sc
			fc.MaxHits = 0
			freeBase, err := Simulate(cur, prior, fc)
			if err != nil {
				t.Fatalf("simulating free-only baseline %s@%d: %v", pair[1], start, err)
			}
			fo := fc
			fo.Oracles = Oracles{Info: OracleFeatures, FeaturesFrom: start + 1}
			freeWith, err := Simulate(cur, prior, fo)
			if err != nil {
				t.Fatalf("simulating free-only oracled %s@%d: %v", pair[1], start, err)
			}

			oc := sc
			// start+1, not start. The opening fifteen is built from
			// PointInTimeWith(.., start-1, ..), which evaluates statusAt at the
			// gameweek about to be played — `start` itself. Gating from `start`
			// therefore fires on the build, and the invariance below caught it:
			// the fifteen differed in 35 of 36 cells. There is also nothing to
			// measure at the entry gameweek, because there is no transfer to make
			// yet; the first decision the knowledge can inform is start+1.
			oc.Oracles = Oracles{Info: OracleFeatures, FeaturesFrom: start + 1}
			if err := oc.Oracles.Validate(); err != nil {
				t.Fatalf("%s@%d: %v", pair[1], start, err)
			}
			with, err := Simulate(cur, prior, oc)
			if err != nil {
				t.Fatalf("simulating oracled %s@%d: %v", pair[1], start, err)
			}

			cells = append(cells, cell{
				season: pair[1], start: start,
				basePts: base.Points, withPts: with.Points,
				baseMoves: base.Transfers, withMoves: with.Transfers,
				baseHits: base.Hits, withHits: with.Hits,
				baseHold:        sumInts(HoldWeekly(cur, prior, sc, base.OpeningSquad)),
				withHold:        sumInts(HoldWeekly(cur, prior, oc, base.OpeningSquad)),
				squadsIdentical: sameSquad(base.OpeningSquad, with.OpeningSquad),
				weeks:           39 - start,
				freeBase:        freeBase.Points, freeWith: freeWith.Points,
				freeBaseMoves: freeBase.Transfers, freeWithMov: freeWith.Transfers,
			})
		}
	}

	// The declared invariance first. If the opening squads differ, the comparison
	// is not the transfer channel — it is the transfer channel plus a different
	// fifteen, and no amount of averaging separates them afterwards.
	var differing []string
	for _, c := range cells {
		if !c.squadsIdentical {
			differing = append(differing, fmt.Sprintf("%s@%d", c.season, c.start))
		}
	}
	if len(differing) > 0 {
		t.Fatalf("the opening fifteen differs in %d of %d cells (%v), so the gate "+
			"is not holding and this measures squad selection as well as transfers",
			len(differing), len(cells), differing)
	}

	fmt.Printf("\nPerfect deadline team news, priced as a reason to TRANSFER\n")
	fmt.Printf("The opening fifteen is bought on honest information in both arms and\n")
	fmt.Printf("comes back byte-identical in all %d cells, so every difference below\n", len(cells))
	fmt.Printf("is a transfer the knowledge caused, or a consequence of one.\n\n")

	fmt.Printf("%-10s %6s %10s %10s %10s %9s %9s\n",
		"season", "start", "base", "with news", "diff/gw", "moves", "hits")
	var totDiff, totWeeks float64
	var totBaseMoves, totWithMoves, totBaseHits, totWithHits int
	bySeason := map[string][]float64{}
	// Every headline number needs its own clustering ingredient. Three of the four
	// had none, so only the POLICY arm could ever have been given a verdict.
	holdBySeason := map[string][]float64{}
	freeBySeason := map[string][]float64{}
	actBySeason := map[string][]float64{}
	for _, c := range cells {
		d := float64(c.withPts-c.basePts) / float64(c.weeks)
		fmt.Printf("%-10s %6d %10d %10d %+10.4f %4d→%-4d %4d→%-4d\n",
			c.season, c.start, c.basePts, c.withPts, d,
			c.baseMoves, c.withMoves, c.baseHits, c.withHits)
		totDiff += float64(c.withPts - c.basePts)
		totWeeks += float64(c.weeks)
		totBaseMoves += c.baseMoves
		totWithMoves += c.withMoves
		totBaseHits += c.baseHits
		totWithHits += c.withHits
		bySeason[c.season] = append(bySeason[c.season], d)
		h := float64(c.withHold-c.baseHold) / float64(c.weeks)
		holdBySeason[c.season] = append(holdBySeason[c.season], h)
		freeBySeason[c.season] = append(freeBySeason[c.season],
			float64(c.freeWith-c.freeBase)/float64(c.weeks))
		actBySeason[c.season] = append(actBySeason[c.season], d-h)
	}

	// Cell-equal-weighted, which is the estimator every other figure in this
	// record uses. A gameweek-weighted mean is a different number and the record
	// already carries a retraction about swapping the two silently.
	var mean float64
	for _, c := range cells {
		mean += float64(c.withPts-c.basePts) / float64(c.weeks)
	}
	mean /= float64(len(cells))

	fmt.Printf("\nper gameweek, cell-equal-weighted   %+.4f  = %+.1f a season\n",
		mean, mean*38)
	fmt.Printf("transfers                           %d → %d\n", totBaseMoves, totWithMoves)
	fmt.Printf("hits                                %d → %d\n", totBaseHits, totWithHits)

	// The mediator. An arm that makes the same moves is inert, whatever its
	// points column says, and this package has read an inert arm as a null before.
	var moved int
	for _, c := range cells {
		if c.baseMoves != c.withMoves || c.basePts != c.withPts {
			moved++
		}
	}
	fmt.Printf("cells where anything changed        %d of %d\n", moved, len(cells))
	if moved == 0 {
		t.Fatal("no cell changed, so the arm could not run — that is not a null")
	}

	// Per season, as an INGREDIENT and not as inference. Go prints no standard
	// error, no t and no verdict word in this repository — inference lives in
	// stats/sweep_inference.R and nowhere else, so that one place decides how
	// clustering, small-sample corrections and multiplicity are handled rather
	// than each diagnostic deciding for itself. TestInferenceLivesInOnePlace
	// caught an earlier version of this block computing its own clustered SE.
	fmt.Printf("\nper season, the axis inference clusters on — all four arms, because a\n")
	fmt.Printf("number with no per-season ingredient can never be given a verdict\n\n")
	names := make([]string, 0, len(bySeason))
	for k := range bySeason {
		names = append(names, k)
	}
	sort.Strings(names)
	fmt.Printf("%-10s %12s %12s %12s %12s\n",
		"season", "POLICY", "HOLD", "acting", "free only")
	for _, n := range names {
		fmt.Printf("%-10s %+12.1f %+12.1f %+12.1f %+12.1f\n", n,
			meanOf(bySeason[n])*38, meanOf(holdBySeason[n])*38,
			meanOf(actBySeason[n])*38, meanOf(freeBySeason[n])*38)
	}
	fmt.Printf("\nRun this through the sweep harness for a verdict. %d season means\n", len(names))
	fmt.Printf("with this much disagreement is exactly the case where a naive SE and a\n")
	fmt.Printf("clustered one part company, so do not eyeball it.\n")

	// The diff-in-diff, which is the whole point of running both metrics in one
	// process. This oracle reaches TWO decisions: which eleven to field, and which
	// transfer to make. HOLD carries the first alone, because it never transfers,
	// and the same fifteen is held in both arms. So POLICY minus HOLD is what the
	// knowledge bought by being ACTED on — the question this test exists for —
	// and it is a paired contrast within a cell rather than a difference of two
	// separately measured numbers on different grids.
	var holdMean, polMean float64
	for _, c := range cells {
		holdMean += float64(c.withHold-c.baseHold) / float64(c.weeks)
		polMean += float64(c.withPts-c.basePts) / float64(c.weeks)
	}
	holdMean /= float64(len(cells))
	polMean /= float64(len(cells))

	// Free transfers only, against hits allowed. The gap is what a willingness to
	// pay four points adds ON TOP of the information, which is a different
	// question from what the information is worth and was silently summed into the
	// headline until it was split out.
	var freeMean float64
	var freeB, freeW int
	for _, c := range cells {
		freeMean += float64(c.freeWith-c.freeBase) / float64(c.weeks)
		freeB += c.freeBaseMoves
		freeW += c.freeWithMov
	}
	freeMean /= float64(len(cells))

	fmt.Printf("\nWhat the information is worth WITHOUT paying for it\n")
	fmt.Printf("free transfers only   %+.4f pts/gw = %+6.1f a season  (%d → %d moves)\n",
		freeMean, freeMean*38, freeB, freeW)
	fmt.Printf("hits allowed          %+.4f pts/gw = %+6.1f a season  (%d → %d moves)\n",
		mean, mean*38, totBaseMoves, totWithMoves)
	fmt.Printf("what the hits added   %+.4f pts/gw = %+6.1f a season\n",
		mean-freeMean, (mean-freeMean)*38)
	fmt.Printf("\nThe free-only arm is the honest headline: a manager holds his transfer\n")
	fmt.Printf("for team news rather than paying four points to react to it. If the hit\n")
	fmt.Printf("row is large and positive, taking hits on confirmed absences is worth\n")
	fmt.Printf("its own investigation; if it is negative, the headline was inflated by\n")
	fmt.Printf("a behaviour nobody would choose.\n")

	fmt.Printf("\nWhich decision the knowledge paid in\n")
	fmt.Printf("HOLD  — the eleven only, no transfers   %+.4f pts/gw = %+6.1f a season\n",
		holdMean, holdMean*38)
	fmt.Printf("POLICY — the eleven AND the transfers   %+.4f pts/gw = %+6.1f a season\n",
		polMean, polMean*38)
	fmt.Printf("difference, what ACTING adds           %+.4f pts/gw = %+6.1f a season\n",
		polMean-holdMean, (polMean-holdMean)*38)
	fmt.Printf("\nThe difference is what the ability to ACT adds to the value of the\n")
	fmt.Printf("news. It is NOT a channel decomposition: HOLD scores a squad frozen in\n")
	fmt.Printf("week one while POLICY scores one that has since made ~25 transfers, so\n")
	fmt.Printf("the residual carries the difference between two elevens as well.\n")
}

func sameSquad(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]int(nil), a...)
	y := append([]int(nil), b...)
	sort.Ints(x)
	sort.Ints(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
