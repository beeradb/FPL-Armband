package backtest

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestDiagXGCTransport scores the expected-goals-conceded chain on the input
// distribution it actually runs on, which is not the one it was validated on.
//
//	python3 stats/understat_xg_backfill.py --transport     # once, from cache, no network
//	DIAG=1 go test ./internal/backtest -run TestDiagXGCTransport -v
//
// The harvest is only needed to *produce* the input. The four files it writes are banked
// under `stats/cells/xgc-transport/`, so a re-run is a copy — `stats/out` is gitignored
// scratch and a fresh checkout has none:
//
//	mkdir -p stats/out && cp stats/cells/xgc-transport/transport-*.csv stats/out/
//
// For the substitution-tercile column's inference, which runs in R off a CSV this test
// writes:
//
//	DIAG=1 FPL_XGC_TERCILE_CSV=/tmp/xgcterc \
//	  go test ./internal/backtest -run TestDiagXGCTransport -v -count=1
//	Rscript stats/xgc_tercile_transport.R /tmp/xgcterc
//
// # The gap this closes
//
// `TestDiagXGCReconstruction` runs the chain on 2022-23 to 2025-26 and reports pooled
// 0.9994, ever-presents 1.0088 and partial minutes 0.9853. Those seasons' per-player xG
// is **FPL's own**. The chain only ever *runs* on 2018-19 to 2022-23 GW1-15, whose xG is
// **Understat's, divided by a borrowed provider offset**. So every figure licensing the
// repair was measured off the population the repair serves, which `xgcrepair.go`'s
// transport block records as untested rather than untestable.
//
// Both arms here call `reconstructedXGC` — the shipped function. A diagnostic carrying
// its own copy of the thing it checks has shipped twice in this package and been wrong
// both times, and it would be worst here, where the entire claim is that one chain
// behaves the same on two inputs.
//
//   - **Arm A, FPL-fed.** The season as loaded. This reproduces the existing validation
//     and is the control: if it does not match the recorded figures, the comparison is
//     measuring a code change rather than an input change.
//   - **Arm B, Understat-fed.** The identical season with every player-gameweek's xG
//     replaced by the Understat figure, rescaled by the **leave-one-out** borrowed
//     offset. Leave-one-out because a repaired season is never one of the seasons its
//     offset is averaged from; including it would fit the level on the season being
//     scored.
//
// A player the crosswalk does not reach gets **zero**, not his FPL figure, because that
// is what happens on the seasons the repair runs on. Measured by the harvest, the
// crosswalk carries 0.998 to 1.000 of FPL's own xG mass, so coverage is not a live
// mechanism here — but the arm is built the faithful way regardless, since an arm that
// silently substitutes the control's input is the byte-identical null this record calls
// its signature failure.
//
// # What is pre-registered, because the temptation afterwards is to explain the outcome
//
// This exists to sign one open question: the 18-cell `FPL_NO_XGC_REPAIR` sweep reads
// −34 a season on `HOLD` and does not resolve, leaving "a better-specified objective
// makes a worse policy" against "the reconstruction's own bias picks the wrong
// defenders". The second needs the chain to be *worse* on Understat-fed input.
//
// **The level is not the discriminating statistic.** `borrowed_ratio` already argues
// that a level error shared by every player in a season is invisible to an argmax, and
// for this chain it is shared by every *club*, so it is a level twice over. Expect the
// ratio to move by roughly what the borrowed offset gets wrong — 7.7% on 2022-23, 0.2%
// on 2023-24, 4.3% on 2024-25, 2.1% on 2025-26 — and read nothing into it.
//
// **The discriminating statistic is the ordering**: Spearman correlation, across keepers
// and defenders with 900+ minutes, between reconstructed and actual season xGC90. That
// is what decides which defenders get bought.
//
//   - If arm B's Spearman is materially below arm A's, the reconstruction genuinely
//     picks worse defenders on the input it runs on, and the −34 gains its second
//     explanation.
//   - If the two are indistinguishable, the transport assumption holds, the second
//     hypothesis loses its premise, and the −34 is either the objective hypothesis or a
//     draw.
//
// Per-player xG dispersion between the two providers — the p90 ratio of 1.54 the
// backfill script records — is expected to be **absent** here, because
// `clubXGPerGameweek` sums the whole club before anything else happens, so
// redistribution inside a club cancels exactly. Club-match-level disagreement is the
// channel that survives, and it is what arm B measures.
func TestDiagXGCTransport(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	t.Log("season   arm         n      ratio    corr    MAE%   ever-n  ever ratio  " +
		"ever MAE%   spearman  n")
	for _, name := range xgcSeasonsWithRealData() {
		s := loadSeason(t, cfg, name)
		ust, err := readTransportXG(name)
		if err != nil {
			// Loud rather than skipped. An absent input file and a chain that
			// transports perfectly must never produce the same output, which is
			// this package's signature failure arriving one layer out.
			t.Fatalf("%s: %v\n\tproduce it with:\n"+
				"\tpython3 stats/understat_xg_backfill.py --transport", name, err)
		}
		recA, _ := reconstructedXGC(s, 1.0)
		recB, _ := reconstructedXGC(withTransportXG(s, ust), 1.0)
		var scored [2]xgcArmResult
		for i, arm := range []struct {
			label string
			rec   map[int]map[int]float64
		}{{"FPL-fed", recA}, {"UST-fed", recB}} {
			r := scoreXGCArm(s, arm.rec)
			scored[i] = r
			t.Logf("%-8s %-8s %6d   %7.4f  %6.3f  %5.1f   %6d   %8.4f   %7.1f   %7.4f %4d",
				name, arm.label, r.n, r.ratio, r.corr, 100*r.mae,
				r.everN, r.everRatio, 100*r.everMAE, r.spearman, r.spearN)
		}

		// The POSITIVE CONTROL. Arm A is the existing validation re-run, so it must
		// land on the recorded figures — ever-presents 1.0088 at 3.9% MAE, pooled
		// 0.9994. If it does not, this is measuring a code change and the
		// input-change reading is wrong. An apparatus with only negative controls
		// cannot detect a delivery failure, and that lesson is already on the record
		// against the oracle harness.
		if a := scored[0]; a.everRatio < 0.99 || a.everRatio > 1.03 || a.everMAE > 0.07 {
			t.Errorf("%s: the FPL-fed arm reads ever-presents %.4f at %.1f%% MAE, "+
				"outside the recorded validation (1.0088 at 3.9%%). The control has "+
				"moved, so the arm-B difference cannot be attributed to the input",
				name, a.everRatio, 100*a.everMAE)
		}

		// The LIVENESS check. If the transport input failed to arrive entirely, arm B
		// would collapse onto arm A and the run would report a PERFECT transport, which
		// is the byte-identical null this package calls its signature failure.
		//
		// ⚠️ Kept, but it is NOT the guard that matters, and an earlier version of this
		// comment said it was. Total non-arrival is already unreachable: readTransportXG
		// fatals on a missing file and again on zero rows. What is reachable is PARTIAL
		// arrival, and it points the other way — withTransportXG gives an unmapped
		// player zero, so a thin crosswalk inflates arm B's error and manufactures
		// exactly the finding this test exists to report. A guard that can only fire
		// against the null result is no guard at all for a positive one.
		if scored[0].everMAE == scored[1].everMAE {
			t.Fatalf("%s: both arms scored identically (%.4f), so the Understat xG "+
				"did not reach the chain. A transport test that measures nothing "+
				"reads exactly like one that found no transport error", name,
				scored[0].everMAE)
		}

		// The COVERAGE check, which is the one that matters. Compare the transported xG
		// mass against FPL's own over the identical scored rows. The expected ratio is
		// this season's own in-season provider ratio over the leave-one-out borrow —
		// measured at 0.929 to 1.045 across the four seasons, which is why the band is
		// wide — while a crosswalk reaching 60% of the league lands near 0.6 and a
		// column read wrong lands near 0. The Python prints this figure and nothing
		// asserted it; the whole finding rests on the difference between "the providers
		// disagree" and "half the players are missing".
		fplMass, ustMass := transportMass(s, ust)
		cover := ustMass / fplMass
		t.Logf("%-8s coverage: transported %.1f against FPL %.1f = %.4f",
			name, ustMass, fplMass, cover)
		if cover < 0.85 || cover > 1.15 {
			t.Fatalf("%s: transported xG mass is %.4f of FPL's over the scored rows, "+
				"outside [0.85, 1.15]. The crosswalk is thin or misread, and an "+
				"unmapped player scores zero — which RAISES arm B's error and "+
				"manufactures the transport failure this test reports", name, cover)
		}

		// The PRORATION-EXPOSURE TERCILE column. `xgcrepair.go` records that the
		// proration's two errors "largely cancel at player-season level" — 0.983-1.014
		// across substitution terciles, within ±1.7% — and records in the same breath
		// that the figure is FPL-fed and the transport run measured dispersion and
		// ordering rather than this. So the claim covers a population its evidence does
		// not, and these three rows per arm are what close that.
		//
		// The outcome is written up at the end of `xgcrepair.go`'s xgcScale block: the
		// FPL-fed band reproduces (0.9857-1.0100 on the three full seasons, inside
		// 0.983-1.014), and the paired transport contrast **does not resolve** — a tie
		// that leans, sharpest form t 1.66. Not "the cancellation transports".
		//
		// The buckets are cut on exposure, which is a minutes property both arms share,
		// so this is paired at player level.
		//
		// The FPL-fed rows are the control, and they are NOT asserted, unlike the
		// ever-present control above. That one re-runs a statistic this package
		// produced and can therefore be pinned; the recorded 0.983-1.014 has no
		// producing test here and its exposure cut had to be defined in this file, so a
		// gate on it would be pinning a reproduction to a figure whose estimator is not
		// on the record. What it does reproduce is the summary — "within ±1.7%" — and
		// `xgcrepair.go` says so in the words a reader would otherwise assume.
		liveN, liveTot := xgcTercileLiveness(t, name, scored[0], scored[1])
		for _, arm := range []struct {
			label string
			r     xgcArmResult
		}{{"FPL-fed", scored[0]}, {"UST-fed", scored[1]}} {
			for b, lab := range []string{"low", "mid", "high"} {
				c := arm.r.terc[b]
				t.Logf("%-8s %-8s tercile %-4s n %3d  exposure %.3f-%.3f  "+
					"ratio-of-totals %.4f  mean-of-ratios %.4f",
					name, arm.label, lab, c.n, c.loExp, c.hiExp, c.ratio, c.meanRatio)
			}
		}
		t.Logf("%-8s tercile liveness: %d of %d player-seasons move between arms = %.4f",
			name, liveN, liveTot, float64(liveN)/float64(liveTot))

		// The LIVENESS check for this column specifically, and unlike the one above it
		// is reachable. The two arms share minutes, the truth column, the population and
		// the cut; only the reconstruction differs. If the per-player reconstructed rate
		// did not move, the tercile table would be one quantity printed twice and the
		// "cancellation survives" reading would be manufactured rather than measured —
		// the byte-identical null wearing the clothes of a confirmation.
		//
		// ⚠️ **This is necessary and NOT sufficient, and it is the weaker of the two
		// liveness checks the finding needs.** A pure per-season SCALAR rescale of arm A
		// would pass this at 100% while forcing every recentred tercile contrast to zero
		// by construction, because the recentring divides by the whole-population ratio
		// and a scalar cancels. The guard with power against that is the within-season
		// dispersion of `rec90_ust/rec90_fpl`, which is zero under a rescale; it lives in
		// `stats/xgc_tercile_transport.R` because it is an inference-side quantity, and
		// it reads CV 4.3-4.8%. Do not quote the fraction below as if it covered that.
		if frac := float64(liveN) / float64(liveTot); frac < 0.90 {
			t.Fatalf("%s: only %.1f%% of the %d scored player-seasons have a different "+
				"reconstructed XGC90 between the arms. The tercile column is re-emitting "+
				"one quantity under two labels, so its agreement measures nothing",
				name, 100*frac, liveTot)
		}

		writeXGCTercileRows(t, name, scored[0], scored[1])
	}
}

// xgcTercileLiveness counts the player-seasons whose reconstructed rate actually differs
// between the arms, over the rows both arms scored.
//
// Relative rather than absolute, because XGC90 runs around 1.3 and an absolute epsilon
// would be a different sensitivity for a keeper behind a good defence than a bad one.
// The rows are element-ordered in both arms and the population is a minutes filter, so a
// length disagreement is a bug rather than data and says so.
// Both guards are provably unreachable today — `reconstructedXGC`'s key set depends only
// on minutes and the fixture list, `withTransportXG` shares both, and each arm appends in
// `sortedSeasonPlayerIDs` order under one filter with no map iteration on the path. They
// are `t.Fatalf` rather than `panic` anyway: a panic in a test aborts the whole binary so
// no sibling test in this package runs, and it loses the season name every other guard
// here carries.
func xgcTercileLiveness(t *testing.T, season string, a, b xgcArmResult) (moved, total int) {
	t.Helper()
	if len(a.rows) != len(b.rows) {
		t.Fatalf("%s: the arms scored %d and %d players, which is impossible while "+
			"withTransportXG copies minutes verbatim", season, len(a.rows), len(b.rows))
	}
	for i := range a.rows {
		if a.rows[i].element != b.rows[i].element {
			t.Fatalf("%s: the arms' rows are not aligned by element at index %d (%d vs %d)",
				season, i, a.rows[i].element, b.rows[i].element)
		}
		if math.Abs(a.rows[i].rec90-b.rows[i].rec90) > 1e-9*math.Abs(a.rows[i].rec90) {
			moved++
		}
		total++
	}
	return moved, total
}

// writeXGCTercileRows dumps the per-player rows both arms scored, when
// FPL_XGC_TERCILE_CSV names a directory.
//
// Go computes the buckets and R computes the standard errors, which is this project's
// standing division: Go prints no standard error, no t and no verdict word. The tercile
// assignment travels in the CSV rather than being re-derived in R, so the cut has one
// implementation.
//
// **The bucket RATIOS travel too, in a sidecar, and R asserts against them.** R has to
// re-form the estimator to bootstrap it, so a second implementation of the ratio is
// unavoidable; what is avoidable is the two disagreeing silently, which an earlier
// version of this comment claimed was impossible while the only check was a human
// eyeballing two console files. `xgc-tercile-<season>-buckets.csv` carries Go's `ratio`
// and `meanRatio` per bucket and `stats/xgc_tercile_transport.R` stops on a mismatch.
//
// Values are written at 9 decimals rather than 6. At XGC90 ~ 1.3, 6 dp quantises at
// ~8e-7 relative, three orders above the 1e-9 liveness threshold R applies — so the file
// itself was setting the sensitivity of a guard that is supposed to be about the data.
func writeXGCTercileRows(t *testing.T, season string, fpl, ust xgcArmResult) {
	t.Helper()
	dir := os.Getenv("FPL_XGC_TERCILE_CSV")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	// writeCSV closes and flushes with the errors CHECKED. csv.Writer buffers, so
	// `Write` only ever returns a previous flush's error; the error that matters
	// surfaces at Flush/Error and at Close. Deferring both discarded meant a full disk
	// produced a truncated file, a "wrote ..." log line and a passing test — and a
	// truncation landing on a record boundary reads downstream as a complete, smaller
	// season.
	writeCSV := func(name string, recs [][]string) string {
		path := filepath.Join(dir, name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("creating %s: %v", path, err)
		}
		w := csv.NewWriter(f)
		for _, rec := range recs {
			if err := w.Write(rec); err != nil {
				f.Close()
				t.Fatalf("writing %s: %v", path, err)
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			f.Close()
			t.Fatalf("flushing %s: %v", path, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("closing %s: %v", path, err)
		}
		return path
	}
	num := func(v float64, prec int) string {
		return strconv.FormatFloat(v, 'f', prec, 64)
	}

	rows := [][]string{{"season", "element", "club", "minutes", "exposure", "tercile",
		"act90", "rec90_fpl", "rec90_ust"}}
	for i := range fpl.rows {
		r := fpl.rows[i]
		rows = append(rows, []string{
			season,
			strconv.Itoa(r.element),
			strconv.Itoa(r.club),
			num(r.minutes, 1),
			num(r.exposure, 9),
			strconv.Itoa(r.tercile),
			num(r.act90, 9),
			num(r.rec90, 9),
			num(ust.rows[i].rec90, 9),
		})
	}
	path := writeCSV(fmt.Sprintf("xgc-tercile-%s.csv", season), rows)

	buckets := [][]string{{"season", "arm", "tercile", "n", "ratio", "mean_ratio"}}
	for _, arm := range []struct {
		label string
		r     xgcArmResult
	}{{"FPL", fpl}, {"UST", ust}} {
		for b, c := range arm.r.terc {
			buckets = append(buckets, []string{
				season, arm.label, strconv.Itoa(b), strconv.Itoa(c.n),
				num(c.ratio, 9), num(c.meanRatio, 9),
			})
		}
	}
	writeCSV(fmt.Sprintf("xgc-tercile-%s-buckets.csv", season), buckets)
	t.Logf("%-8s wrote %s and its buckets sidecar", season, path)
}

// TestTheProrationExposureCutIsNotTheEverPresentCut pins the distinction the tercile
// column rests on, because collapsing the two is a defect this file has already shipped
// once and it is invisible in a diff.
//
// The exposure cut asks "did the proration invent anything", which is false whenever
// `minutes == 90*n` for an n-fixture gameweek. The ever-present population asks "the full
// 90 in a SINGLE-fixture gameweek". They differ exactly on fully-played multi-fixture
// gameweeks, and the first version of the cut used the ever-present predicate for both —
// which booked a 90+90 double as fully exposed, mislabelled 18-71% of the scored
// population by season, moved 22% of player-seasons between terciles, and manufactured an
// apparent 2022-23 anomaly that vanished on the correct predicate.
//
// ⚠️ Those two percentages need the contaminated cut to re-derive and only the corrected
// exposure is banked, so **this test's own count is the checkable version of that story**
// and the percentages are the correction's account of itself.
//
// **This is a liveness check, not a confinement check.** If it ever counts zero it means
// the archive has no fully-played double gameweeks in these seasons, and then the two
// predicates coincide and the tercile figures in `xgcrepair.go` were measured on a
// distinction that does not exist. Either way the number must be reported rather than
// assumed, which is why the count is logged and not merely asserted.
//
// Skips rather than fails when the archive is unreachable, per this package's convention.
func TestTheProrationExposureCutIsNotTheEverPresentCut(t *testing.T) {
	cfg := loadConfig(t)
	total := 0
	for _, name := range xgcSeasonsWithRealData() {
		s := loadSeason(t, cfg, name)
		matches := clubMatches(s)
		differ := 0
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			for gw, g := range p.GWs {
				n := matches[[2]int{p.Team, gw}]
				if n <= 0 || g.Minutes <= 0 {
					continue
				}
				everPresent := g.Minutes == 90 && n == 1
				proratedExactly := g.Minutes == 90*n
				if everPresent != proratedExactly {
					differ++
				}
			}
		}
		t.Logf("%s: %d player-gameweeks where the two predicates disagree", name, differ)
		total += differ
	}
	if total == 0 {
		t.Fatalf("the proration-exposure predicate and the ever-present predicate agree " +
			"on every row in every season. Either a fully-played multi-fixture gameweek " +
			"has stopped existing in this archive, or the two have been collapsed back " +
			"into one — and the tercile column in xgcrepair.go then reports a cut on a " +
			"distinction that is not there")
	}
}

// readTransportXG reads the Understat xG written by `--transport`, keyed the same way
// the season is.
//
// Read from stats/out rather than embedded because it is a diagnostic input for a
// season that needs **no repair**: a file under repairdata/ would be loaded as a repair
// by everything in this package, which is a far worse failure than a relative path.
func readTransportXG(season string) (map[int]map[int]float64, error) {
	path := filepath.Join("..", "..", "stats", "out",
		fmt.Sprintf("transport-%s-xg.csv", season))
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: reading header: %w", path, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, want := range []string{"element", "GW", "expected_goals"} {
		if _, ok := col[want]; !ok {
			return nil, fmt.Errorf("%s: no %q column", path, want)
		}
	}
	out := map[int]map[int]float64{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		el, err1 := strconv.Atoi(strings.TrimSpace(rec[col["element"]]))
		gw, err2 := strconv.Atoi(strings.TrimSpace(rec[col["GW"]]))
		xg, err3 := strconv.ParseFloat(strings.TrimSpace(rec[col["expected_goals"]]), 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return nil, fmt.Errorf("%s: unparseable row %v", path, rec)
		}
		if out[el] == nil {
			out[el] = map[int]float64{}
		}
		out[el][gw] += xg
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no rows", path)
	}
	return out, nil
}

// withTransportXG copies a season with every player-gameweek's xG replaced by the
// Understat figure, and **zero where the crosswalk does not reach**.
//
// Copied rather than mutated because the caller scores the FPL-fed arm from the same
// season, and a process-global season handed to two arms is how a paired comparison
// silently measures one arm twice.
func withTransportXG(s *Season, ust map[int]map[int]float64) *Season {
	out := *s
	out.Players = make(map[int]*Player, len(s.Players))
	for id, p := range s.Players {
		q := *p
		q.GWs = make(map[int]GW, len(p.GWs))
		for gw, g := range p.GWs {
			g.XG = ust[id][gw]
			q.GWs[gw] = g
		}
		out.Players[id] = &q
	}
	return &out
}

// transportMass totals FPL's own xG and the transported xG over the rows the arms are
// actually scored on, so the two are comparable.
//
// Same skip as scoreXGCArm — a row with no real xGC figure is not scored, so counting
// its xG here would measure coverage on a population the finding never touches. That
// matters for 2022-23, where only GW16-38 carries the real column.
func transportMass(s *Season, ust map[int]map[int]float64) (fpl, transported float64) {
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		for gw, g := range p.GWs {
			if g.XGC <= 0 || g.XGCReconstructed || g.Minutes <= 0 {
				continue
			}
			fpl += g.XG
			transported += ust[id][gw]
		}
	}
	return fpl, transported
}

type xgcArmResult struct {
	n                int
	ratio, corr, mae float64
	everN            int
	everRatio        float64
	everMAE          float64
	spearman         float64
	spearN           int
	// The substitution-tercile column. See xgcTercile.
	terc [3]xgcTercile
	// One row per scored player-season in the 900+ minute keeper-and-defender
	// population, ordered by element id. Kept so the caller can pair the two arms
	// row by row — the liveness fraction is a cross-arm quantity and cannot be
	// computed inside one arm — and so the inference can run in R off a CSV rather
	// than in Go. This package's standing division is Go for the engine, R for the
	// inference, and Go prints no standard error.
	rows []xgcPlayerRow
}

// xgcPlayerRow is one keeper or defender's season at the level `xgcrepair.go`'s
// cancellation claim is made: a season rate, and how much of that rate went through
// the step the claim says is approximate.
//
// `exposure` is the share of the player's SCORED minutes that did **not** arrive on a
// row the proration got exactly right — `minutes == 90*n` for a gameweek with n fixtures,
// the one shape where the reconstruction invents nothing because the player faced his
// club's whole exposure. See `scoreXGCArm` for why that is deliberately NOT the
// ever-present predicate: sharing one predicate made this substantially a double-gameweek
// cut, and correcting it moved 22% of player-seasons between terciles.
//
// ⚠️ **This is still PRORATION exposure rather than SUBSTITUTION exposure**, and the
// claim it re-scores is worded in substitutions. The two now agree on double gameweeks
// and can still differ on a blank one. It is the right cut for the mechanism — the
// proration's two errors are what the cancellation claim is about — and the wrong name
// for "was he substituted", so it is not called that anywhere.
//
// The cut is therefore a **minutes** property. Both arms see identical minutes —
// `withTransportXG` replaces xG and nothing else — so the terciles are the same players
// in both arms and the contrast is paired at player level rather than at bucket level.
type xgcPlayerRow struct {
	element  int
	club     int
	minutes  float64
	exposure float64
	act90    float64
	rec90    float64
	tercile  int // 0 lowest exposure, 1 middle, 2 highest
}

// xgcTercile is one substitution-exposure bucket's answer to "reconstructed over actual".
//
// **Two estimators, both reported, because this record has twice mistaken an estimator
// swap for a data change.** `ratio` is the ratio of totals over per-player XGC90 rates
// (Σ rec90 / Σ act90); `meanRatio` is the mean of per-player ratios. They are different
// quantities and the second is biased upward when the denominator is small and variable.
// The recorded 0.983-1.014 does not say which it was, so neither does this — it reports
// both and names them.
//
// Note that `ratio` weights every player equally in *rate* space, not by minutes: the
// claim is about a season XGC90, and a minutes-weighted version would be a third
// quantity again.
type xgcTercile struct {
	n            int
	loExp, hiExp float64
	ratio        float64
	meanRatio    float64
}

// scoreXGCArm compares one reconstruction against the season's real xGC column.
//
// Scored only on rows carrying a real figure: `XGCReconstructed` rows are the repair's
// own output and comparing against them would be scoring the chain on itself, which is
// the same class of error as a diagnostic carrying its own copy of what it checks.
func scoreXGCArm(s *Season, rec map[int]map[int]float64) xgcArmResult {
	var out xgcArmResult
	var sumA, sumR, sumAbs, sumAA, sumRR, sumAR float64
	var everA, everR, everAbs float64
	matches := clubMatches(s)
	// Season-level rate per player, for the ordering statistic and the terciles.
	// `exact` is the minutes on rows where the proration invents nothing, which is
	// what the substitution-exposure cut is one minus the share of.
	type rate struct {
		act, rec, mins, exact float64
		club                  int
	}
	rates := map[int]*rate{}
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		for _, gw := range sortedGameweeks(rec[id]) {
			g := p.GWs[gw]
			if g.XGC <= 0 || g.XGCReconstructed {
				continue
			}
			act, r := g.XGC, rec[id][gw]
			out.n++
			sumA += act
			sumR += r
			sumAbs += math.Abs(act - r)
			sumAA += act * act
			sumRR += r * r
			sumAR += act * r
			// TWO predicates, because they are two different questions and an earlier
			// version of this code used one for both — which made the tercile cut
			// substantially a double-gameweek cut. Keep them apart.
			//
			//   everPresent — "the full 90 in a SINGLE-fixture gameweek". The
			//     pre-existing validation population, pinned to the recorded 1.0088 by
			//     the positive control above. Not to be touched.
			//   proratedExactly — "the proration invented nothing", which is
			//     `minutes == 90*n` for n fixtures. A player who goes 90+90 in a double
			//     has share 180/180 = 1 and receives his club's whole two-match xGA
			//     exactly, so he belongs at exposure 0; the ever-present predicate books
			//     him as fully exposed instead. Measured, that mislabels a fully-played
			//     multi-fixture gameweek for 18-71% of the population depending on the
			//     season, and re-cutting on the correct predicate moves 22% of
			//     player-seasons into a different tercile.
			//
			// The exposure cut is a statement about the PRORATION, so it takes the
			// second. This is not the "one quantity, two implementations" failure — it
			// is two quantities that were wrongly sharing one.
			n := matches[[2]int{p.Team, gw}]
			everPresent := g.Minutes == 90 && n == 1
			proratedExactly := n > 0 && g.Minutes == 90*n
			if everPresent {
				out.everN++
				everA += act
				everR += r
				everAbs += math.Abs(act - r)
			}
			if p.Type == 1 || p.Type == 2 {
				if rates[id] == nil {
					rates[id] = &rate{club: p.Team}
				}
				rates[id].act += act
				rates[id].rec += r
				rates[id].mins += float64(g.Minutes)
				if proratedExactly {
					rates[id].exact += float64(g.Minutes)
				}
			}
		}
	}
	if sumA > 0 {
		out.ratio = sumR / sumA
		out.mae = sumAbs / sumA
	}
	// xgcrepair_test.go's corr, not a second one. It was inlined here first, which is
	// this record's signature failure arriving in its cheapest form: the two forms are
	// algebraically identical today, and that is exactly how the four recorded instances
	// began.
	// Guarded on n because corr divides by it: an empty population would return NaN
	// rather than the zero the inlined form used to produce.
	if out.n > 0 {
		out.corr = corr(sumAR, sumA, sumR, sumAA, sumRR, out.n)
	}
	if everA > 0 {
		out.everRatio = everR / everA
		out.everMAE = everAbs / everA
	}

	// The ordering statistic, and the population is the one that gets bought: keepers
	// and defenders with a real body of football. A rate, not a total — two players
	// with the same season xGC and different minutes are not equally attractive.
	var xa, xb []float64
	for _, id := range sortedSeasonPlayerIDs(s) {
		r := rates[id]
		if r == nil || r.mins < 900 {
			continue
		}
		xa = append(xa, r.act/(r.mins/90))
		xb = append(xb, r.rec/(r.mins/90))
		// The same population, one filter, so the ordering statistic and the
		// cancellation statistic can never drift onto different players.
		out.rows = append(out.rows, xgcPlayerRow{
			element:  id,
			club:     r.club,
			minutes:  r.mins,
			exposure: 1 - r.exact/r.mins,
			act90:    r.act / (r.mins / 90),
			rec90:    r.rec / (r.mins / 90),
			tercile:  -1,
		})
	}
	out.spearN = len(xa)
	assignXGCTerciles(&out)
	// stats_test.go's spearman, not a second one. "One quantity, two
	// implementations" is this record's signature failure and it has four recorded
	// instances; a rank correlation is exactly the kind of small helper that
	// acquires a duplicate.
	// The ok is kept rather than discarded: a degenerate population reports 0.0000,
	// which is indistinguishable from "the orderings are unrelated" — the exact
	// distinction spearman's own doc comment exists to preserve.
	var ok bool
	out.spearman, ok = spearman(xa, xb)
	if !ok {
		out.spearman = math.NaN()
	}
	return out
}

// assignXGCTerciles cuts the 900+ minute population on proration exposure — see
// xgcPlayerRow, and note it is NOT substitution exposure — and fills the three buckets,
// in place.
//
// The cut follows `bpsrules_test.go`'s convention exactly — ascending on the key, ties
// broken by element id, k = n/3 with the remainder in the middle bucket — because a
// second convention for "tercile" in one package is a difference nobody would think to
// look for. It is not the same function only because that one is typed to `bpsTotals`.
//
// Ties are handled and, as it turns out, do not currently bind. A player whose every
// appearance prorated exactly has exposure 0; counted in the banked rows there are
// **25 / 20 / 23 / 23** of them across the four seasons against low-bucket boundaries at
// 40 / 51 / 47 / 50, so the boundary falls inside the zero run in none of them. ⚠️ An
// earlier version of this line read 3 / 6 / 17 / 14, which was the **contaminated**
// ever-present cut — that predicate booked a fully-played double as non-exact and so
// produced far fewer exposure-zero players. The conclusion is unchanged, but the counts
// were sourced to the banked rows and did not match them.
//
// The element-id tiebreak is kept regardless — `sort.Slice` is unstable, and
// `(exposure, element)` is a total order because element is unique per row, which is what
// makes the cut reproducible.
func assignXGCTerciles(out *xgcArmResult) {
	if len(out.rows) < 3 {
		return
	}
	idx := make([]int, len(out.rows))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		ra, rb := out.rows[idx[a]], out.rows[idx[b]]
		if ra.exposure != rb.exposure {
			return ra.exposure < rb.exposure
		}
		return ra.element < rb.element
	})
	n := len(idx)
	k := n / 3
	bounds := [3][2]int{{0, k}, {k, n - k}, {n - k, n}}
	for b, cut := range bounds {
		var sumA, sumR, sumRatio float64
		t := xgcTercile{loExp: math.Inf(1), hiExp: math.Inf(-1)}
		for _, i := range idx[cut[0]:cut[1]] {
			r := &out.rows[i]
			r.tercile = b
			t.n++
			t.loExp = math.Min(t.loExp, r.exposure)
			t.hiExp = math.Max(t.hiExp, r.exposure)
			sumA += r.act90
			sumR += r.rec90
			// Not guarded, deliberately. `act90` is provably positive here — `act`
			// accumulates only rows with `XGC > 0` and `mins` is at least 900 — and a
			// guard that skipped a player from `sumRatio` while still counting him in
			// `n` would silently pull `meanRatio` toward zero. Fail loudly instead: a
			// NaN in the printed table is visible and a quietly deflated mean is not.
			sumRatio += r.rec90 / r.act90
		}
		if sumA > 0 {
			t.ratio = sumR / sumA
		}
		if t.n > 0 {
			t.meanRatio = sumRatio / float64(t.n)
		}
		out.terc[b] = t
	}
}
