package backtest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// The attack/defence bands WERE not run-to-run deterministic. Fixed, and pinned
// by `TestBandAssignmentIsDeterministic` in internal/analysis. Everything below
// is kept because it is the measurement of what the defect cost, and that governs
// how every pre-fix `BandStrength` figure may be read.
//
// # The defect
//
// `teamBands` built its candidate slice by ranging over a `map[int]*rec`, whose
// iteration order Go randomises per run, and then ordered it with `sort.Slice`,
// which is **not stable**. So whenever two clubs had the same goals-per-match —
// common early, because it is a small integer over the same small number of
// matches — *which* of them landed in the bottom or top three was decided by map
// order, and changed from one run to the next.
//
// This is the same defect the record already carries against `Optimize`, which
// "ranged over a map to order each DP seed's bench, and returned two different
// fifteens from identical inputs on about one landscape in seventy-two" and is
// pinned by `TestSeedOrderIsDeterministic`.
//
// **Only a tie ON A BAND BOUNDARY matters**, which is why the instability is not
// simply a function of how many ties there are. A tie wholly inside the bottom
// three reorders clubs that all get the same band, so it is invisible; a tie
// spanning the third and fourth places decides membership. 2024-25 at cutoff 7 has
// interior ties and no boundary tie, and is perfectly stable.
//
// The deterministic quantity is therefore the number of **reachable** assignments.
// Quote that, not a sample.
//
// ⚠️ **It is a product of `C(tie size, places inside the band)`, NOT a product of
// the tie sizes** — an earlier version of this comment listed the sizes alone and
// they multiply to 96, so the only arithmetic a reader could do returned the wrong
// answer. At cutoff 6 on 2024-25 the four boundaries are:
//
//	third-worst attack   3-way tie, 2 places inside   C(3,2) = 3
//	third-best attack    4-way tie, 1 place inside    C(4,1) = 4
//	third-worst defence  2-way tie, 1 place inside    C(2,1) = 2
//	third-best defence   4-way tie, 2 places inside   C(4,2) = 6
//
// giving 3 x 4 x 2 x 6 = **144 reachable assignments**. A run of 40 engines sees
// 30-34 of them and that count is itself a draw — do not write it down as a
// property. Cutoff 7 computes to exactly 1, which is why it was stable.
//
// The profile, as reachable counts: cutoff 6 is the worst on this season, cutoffs
// 5 and 6 are the only bad ones, and from GW7 on it is 1 or 2. `TestDiagBandDeterminism`
// prints the sampled counts, which track it.
//
// # What it cost the measurement, with its denominators
//
// Two replays of the identical sweep on identical data, `band_strength 1` arm,
// against `band_strength 0` identical in **36 of 36** cells on every column:
//
//	hold_points      3 of 36        <- the deciding column
//	squad_hash       1 of 36
//	hold_xpoints     6 of 36
//	policy_points   12 of 36
//	policy_xpoints  13 of 36
//
// ⚠️ **The union over all columns is 14 of 36, and it is carried by the POLICY
// pair.** Quoting 14 as though it were the held metric reads as 39% instability
// where the deciding column is 8%. On `hold` the three moves total 54 points, the
// largest single cell being 38 (2025-26 GW6, where the squad flipped), and the
// arm's mean moved **+0.339 -> +0.357 pts/gw — 0.7 points a season, about a tenth
// of that contrast's own CR2 standard error.**
//
// The jitter also concentrates *away* from the cells carrying the estimate: two of
// the three moved `hold` cells are the GW1 and GW6 entries, which re-pick through
// cutoffs 5-6, while the GW26 column that carries most of the point estimate sits
// at cutoff 25 and did not move.
//
// **So this cannot overturn a null.** The CR2 standard error is conditional on one
// map draw, and the omitted jitter can only widen the interval — on an arm that
// already fails to resolve. Had the arm resolved, the defect would have been
// disqualifying. One repeat is a single draw, not a variance estimate.
// See stats/snapshots/2026-08-16-band-strength/.
//
// # Why no DEFAULT configuration is affected
//
// ⚠️ **Not "nothing shipped", which is too strong.** `attackBandAdj` and
// `defenceBandAdj` return 1 before touching `teamBands()` when strength is <= 0,
// `DefaultWeights` leaves the field at its zero value and `config.json` ships 0 —
// so no default configuration reaches the bands, which is what the 36-of-36
// reproduction of the baseline arm demonstrates. But **two opt-in switches reach
// the live path, not only the replay**: `applyWeightOverrides` runs in
// `cmd/armband/main.go` before command dispatch, so `FPL_WEIGHT=band=1` makes a
// live `armband review` non-deterministic, and `FPL_BAND_STRENGTH` does the same
// for `armband backtest`.
//
// **The consequence is mostly for measurement, and it does not expire with the
// fix**: any `BandStrength` figure recorded BEFORE it — which is every historical
// one — is reproducible only to within this jitter, and such a sweep cannot be
// re-derived from its own cells. Figures taken after it can.
//
// The fix, taken: build the candidate slice in club-id order and break both sort
// ties on club id, giving a total order. `sort.SliceStable` alone would not have
// done it, since the input order was already random.
//
// # What the test below does and does NOT prove
//
// ⚠️ It does **not** prove the short circuit is there, and an earlier version of
// this comment claimed it did. At strength 0 the adjustment is `1 + target*0` and
// `1 - avoid*0`, both exactly 1 whatever band a club is in — so band membership
// cannot reach the multiplier whether the `strength <= 0` guard exists or not.
// Deleting both guards leaves this test passing. That is this file's own
// catalogued failure mode, so it is named rather than left for the next reader.
//
// What it does prove is worth having and is not vacuous: **the whole shipped
// scoring path is reproducible across engines**, compared on every player's
// `Score` rather than on a hand-built fixture's multipliers. It fails if any
// map-ordered quantity ever reaches the default configuration — this being the
// third instance of that class in this repository, after `Optimize` and
// `newTeamFormIndex`. `TestBandStrengthIsShippedOff` is what guards the premise
// that the constant is still 0.
func TestBandStrengthIsDeterministicAtTheShippedSetting(t *testing.T) {
	cfg := loadConfig(t)
	cur, err := Load(context.Background(), cfg.CacheDir, "2024-25")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	prior, err := Load(context.Background(), cfg.CacheDir, "2023-24")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	if cfg.Weights.BandStrength != 0 {
		t.Fatalf("band_strength ships at %v; this test is about the DEFAULT "+
			"configuration and stops rather than silently measuring another one",
			cfg.Weights.BandStrength)
	}

	// Cutoff 6, where the boundary ties are thickest on this season, so a
	// band-shaped nondeterminism reaching the default configuration would show
	// here before anywhere else.
	//
	// Compared on every player's Score, not on a hand-built FixtureBrief's
	// multipliers: those are attackMultiplier(3) and defenceMultiplier(3) times a
	// band factor that is identically 1 at strength 0, which makes them a pure
	// function of two constants — invariant to the engine, the archive and the
	// cutoff, and passing against an empty archive.
	seen := map[string]int{}
	scored := 0
	for i := 0; i < 20; i++ {
		e, _ := EngineAt(cur, prior, 6, sweepConfig(cfg, 7, false))
		var b strings.Builder
		n := 0
		for _, m := range e.AllMetrics() {
			fmt.Fprintf(&b, "%d:%.9f;", m.ID, m.Score)
			if m.Score != 0 {
				n++
			}
		}
		scored = n
		seen[b.String()]++
	}
	// Without this the test would pass on an empty pool, which is the asymmetry
	// TestBandStrengthArrivesOnTheScoredPath's own guard exists to refuse.
	if scored == 0 {
		t.Fatal("no player scored above zero at the cutoff, so this test would pass " +
			"on an empty pool rather than on a reproducible one")
	}
	if len(seen) != 1 {
		t.Fatalf("the DEFAULT configuration produced %d distinct score vectors over "+
			"%d scored players from byte-identical inputs. Something map-ordered has "+
			"reached the shipped scoring path — this is the class that has already "+
			"bitten in Optimize, newTeamFormIndex and teamBands.", len(seen), scored)
	}
}

// TestDiagBandDeterminism measures how unstable the bands are, for whoever picks
// up the fix. It is a measurement rather than an assertion, because asserting the
// current behaviour would pin the defect in place.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBandDeterminism -v
func TestDiagBandDeterminism(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	cur, err := Load(context.Background(), cfg.CacheDir, "2024-25")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	prior, err := Load(context.Background(), cfg.CacheDir, "2023-24")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}

	for _, cutoff := range []int{5, 6, 7, 8, 10, 12, 19, 25} {
		seen := map[string]int{}
		const trials = 40
		for i := 0; i < trials; i++ {
			sc := sweepConfig(cfg, cutoff+1, false)
			sc.Weights.BandStrength = 1
			e, _ := EngineAt(cur, prior, cutoff, sc)
			var s string
			for id := 1; id <= 20; id++ {
				atk, def := e.FixtureMultipliersFor(analysis.FixtureBrief{
					Event: cutoff + 1, OpponentID: id, Difficulty: 3,
				})
				s += fmt.Sprintf("%d:%.6f/%.6f;", id, atk, def)
			}
			seen[s]++
		}
		t.Logf("cutoff GW%-2d: %2d distinct band assignments from %d byte-identical engines",
			cutoff, len(seen), trials)
	}
}
