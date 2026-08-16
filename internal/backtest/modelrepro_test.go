package backtest

// Are the model-accuracy diagnostics reproducible?
//
//	DIAG=1 go test ./internal/backtest -run TestModelDiagnosticsAreReproducible -v
//
// The accuracy snapshot diffs each run against the last, and the entire value of
// that diff is that a movement means something changed. A diagnostic that returns
// different numbers from identical inputs puts a spurious movement in every
// snapshot, and a reader who learns the comparison always shows changes stops
// reading it — so a non-reproducible diagnostic does not merely add noise, it
// disables the feature.
//
// This is not hypothetical. The clean-sheet diagnostic keeps one representative row
// per team-match and iterated Season.Players, which is a map: Go randomises map
// iteration, so which team-mate represented each match varied per run, and identical
// runs disagreed on the pooled clean-sheet rate by 0.7%. Accumulation is safe —
// addition commutes, give or take the last few bits of a float — but the moment a
// diagnostic *chooses* one of several equivalent rows, map order becomes the choice.
//
// Written as a real replay rather than a source scan because the failure is
// arithmetic and a scan would have to guess which map iterations feed a dedup.
//
// # ⚠️ Nothing this computes is a statement about the model
//
// `pred` here is `exp(-g.XGC)` on REALISED single-match expected goals conceded.
// The engine does not score that quantity: `baseXP90` evaluates the clean sheet
// on `m.XGC90`, a per-90 rate blended toward a prior season, shrunk, and read
// point-in-time. `exp` is convex, so an aggregate over one is not an aggregate
// over the other, and a figure taken from here and quoted as a calibration would
// be the wrong-regressor defect this project has already paid for once — it
// reached four documents and a reviewer's brief before anybody asked which
// quantity it was fitted against.
//
// **The regressor is incidental to what is being pinned.** What this test
// compares is the reduction against ITSELF across repeats. Any per-row quantity
// that varies between team-mates would serve. `pred` is never printed on success,
// never written to a dump, and no figure in this repository is sourced from it —
// checked when this note was added.
// `TestNonlinearTransformsScoreTheModelsOwnRegressor` carries the same statement
// as an exemption, so the scan for that class does not have to be widened to
// excuse this file quietly.
//
// # ⚠️ What it reaches is NARROWER than the paragraphs above imply
//
// **Only the OUTER order is live.** The dedup key is `[2]int{p.Team, gw}`, and
// `gw` is the inner map's own key — so within one player every key is distinct
// and the inner `range p.GWs` cannot change WHICH rows are selected, only the
// order the floats are added in. The bit-exact half therefore cannot fire on
// repeats of one ordering; what it pins is that the outer loop is `sortedPlayerIDs`
// rather than a range over `s.Players`, which is the historical bug, in this
// file's copy of the loop.
//
// **And it cannot see the diagnostic at all**, because this is a copy of the
// reduction rather than a call into it. If `TestDiagCleanSheetPoisson` regressed
// to ranging `s.Players`, this test would stay green — the failure the project's
// own rule against a diagnostic carrying its own copy of the thing it checks
// predicts, arriving here. The stated reason for the copy (the diagnostic prints
// a table and writes a CSV) is an argument for extracting a shared reduction, not
// for duplicating one. **Extracting it, and asserting across a deliberately
// PERMUTED player order rather than repeats of one, is owed work and is not done
// here.**
//
// # Why there is no `Fixtures != 1` guard, and it is measured rather than assumed
//
// Its sibling drops doubled rows, because a doubled row carries xGC summed over
// both matches while `CleanSheets > 0` still reads "at least one" — the
// intersection against the union. **That artefact is about comparing pred against
// act, which this test never does**, so it cannot reach the property here.
//
// The question that could reach it is the opposite one: would dropping doubles
// leave nothing for the player order to choose between, so that the pin passed
// vacuously? Counted over exactly the population the reduction keeps — every
// position but forwards, 90 minutes, xGC above zero — team-gameweeks whose
// eligible rows disagree on the CLEAN SHEET, which is the bit-exact channel:
//
//	          disagreeing   of which single-fixture
//	2023-24        10                  6
//	2024-25        18                 14
//	2025-26        30                 26
//
// So the guard would leave 6, 14 and 26 cells the order can still move. It is not
// needed and it is not free, so the wider population is kept.
//
// ⚠️ **Do not count xGC disagreement instead.** It is 3-9x larger (67, 76, 100 on
// single fixtures) and it feeds only the 1e-4 tolerance, not the bit-exact
// assertion the 0.7% history is about. ⚠️ **And the doubles column is
// construction-forced**: `loadGameweeks` accumulates, so a doubled cell disagrees
// whenever any eligible player missed one of the two matches. Monotonicity the
// construction forces is not evidence.
//
// ⚠️ **The cause is TWO things, and the second is a defect rather than football.**
// Team-mates do carry different per-player xGC. But `Player.Team` comes from
// `players_raw.csv`, which is the END-OF-SEASON snapshot, so a mid-season
// transferee's earlier gameweeks are keyed to the club he finished at. Measured:
// of the single-fixture cells that disagree on the clean sheet, the rows also
// disagree on `GoalsConceded` in **6 of 6, 14 of 14 and 26 of 26** — they were not
// in the same match. So every route to the bit-exact assertion on a single fixture
// is that keying, not a football difference. **The sibling inherits the same
// keying and has no guard for it**; sizing that is a separate item and is not done
// here.
//
// ⚠️ **All of the counts above were taken once, off a loaded season, by nothing
// that is in this tree.** If the population moves they do not, and nothing here
// will notice.

import (
	"context"
	"math"
	"os"
	"testing"
)

func TestModelDiagnosticsAreReproducible(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	ctx := context.Background()

	// One season is enough: the bug is per-season and a single one keeps this cheap
	// enough to run beside the others rather than being skipped for time.
	s, err := Load(ctx, cfg.CacheDir, "2024-25")
	if err != nil {
		t.Fatal(err)
	}

	// The clean-sheet team-match reduction, in the shape the diagnostic uses it in
	// — with two differences, not one: the missing `Fixtures != 1` guard, argued
	// in the header, and the gameweek loop, which the sibling sorts for its CSV
	// dump and this one does not need to. ⚠️ Being a copy at all is the header's
	// standing complaint against itself.
	reduce := func() (n, pred, act float64) {
		seen := map[[2]int]bool{}
		for _, id := range sortedPlayerIDs(s) {
			p := s.Players[id]
			if p.Type == 4 {
				continue
			}
			for gw, g := range p.GWs {
				if g.Minutes < 90 || g.XGC <= 0 {
					continue
				}
				key := [2]int{p.Team, gw}
				if seen[key] {
					continue
				}
				seen[key] = true
				n++
				pred += math.Exp(-g.XGC)
				if g.CleanSheets > 0 {
					act++
				}
			}
		}
		return
	}

	n0, pred0, act0 := reduce()
	if n0 == 0 {
		t.Skip("no team-matches in the archive for this season")
	}

	// Two different standards, deliberately, because two different things are going
	// on and only one of them is a bug.
	//
	// **The selected sample must be bit-exact.** Which team-matches are in it, and
	// how many kept a clean sheet, are integer counts that the dedup decides — so
	// they are precisely what map iteration was corrupting, and any variation is the
	// bug.
	//
	// **The float sums need only agree to the precision the snapshot records**, which
	// is four decimal places. Adding 750 floats in a different order changes the last
	// bits, and chasing bit-identical sums would mean sorting every inner map in
	// every diagnostic to buy a difference of 1e-13. The invariant that matters is
	// that the *artefact* is stable.
	const dp = 1e-4
	for i := 0; i < 10; i++ {
		n, pred, act := reduce()
		if n != n0 || act != act0 {
			t.Fatalf("repeat %d selected a different sample:\n"+
				"  team-matches  %.0f against %.0f\n"+
				"  clean sheets  %.0f against %.0f\n\n"+
				"A diagnostic that deduplicates must iterate in a fixed order — see "+
				"sortedPlayerIDs. Otherwise every accuracy snapshot reports a movement "+
				"that is really map iteration, and the diff stops meaning anything.",
				i+1, n, n0, act, act0)
		}
		if math.Abs(pred/n-pred0/n0) > dp {
			t.Fatalf("repeat %d changed the predicted clean-sheet rate: %.6f against "+
				"%.6f. That is more than float addition order can explain, so the "+
				"selected rows differ even though the counts match.",
				i+1, pred/n, pred0/n0)
		}
	}
}
