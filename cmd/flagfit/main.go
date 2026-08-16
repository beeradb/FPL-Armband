// Command flagfit fits FPL's "chance of playing" flag against the model's own
// expected minutes, using Internet Archive captures of what FPL was publishing at
// each deadline.
//
// # Why this exists, and what its first two versions got wrong
//
// The first pass was in Python and measured realised minutes against a player's own
// rolling mean over recent *unflagged* gameweeks. It produced realised ≈ flag²,
// almost exactly, and the fit was an artefact of that denominator: a player is
// flagged *because* he has been out, so his most recent unflagged week is
// systematically older, and unflagged players with a six-week-stale baseline realise
// only 0.658 of it with no flag involved.
//
// The second pass — the first version of this program — replaced that baseline with
// `ExpectedMinutes` and claimed to measure "the denominator that actually gets
// multiplied". It did not. It built its engine with `analysis.NewEngineFull` and
// attached neither `Engine.Recent` nor `Engine.Priors`, which every engine inside
// `Simulate` attaches on the next two lines. With `Recent` nil the recency-weighted
// minutes index does not exist and `blankRunFactor` never fires; with `Priors` nil
// the prior-season blend returns early. So it read the right field carrying its
// *fallback* value — the flat season-to-date mean — and the mechanism its write-up
// named, the recency index carrying old absences, was not switched on at all. It
// also divided a two-match minutes total by a per-match expectation.
//
// Both are fixed here: the engine comes from `backtest.EngineAt`, which is the
// replay's own wiring, and minutes are per match. Wiring the engine moved the
// exponent from 1.59 to about 1.41 and the doubles fix moves it back up, so the two
// defects did not cancel and neither could be ignored.
//
// # Why it is a command rather than a diagnostic in internal/backtest
//
// `TestTheScoringPathCannotSeeRecoveredTeamNews` forbids `internal/analysis` and
// `internal/backtest` from importing the recovery packages, so that feeding real
// historical availability into the replay stays a deliberate reviewed act rather
// than an import. This does not weaken it: the scoring path still cannot see the
// captures, nothing here produces a bootstrap or an engine any replay consumes, and
// the output is a table. **Reads both sides, writes a table, feeds nothing** — that
// is the boundary a future composer should be held to as well.
//
// # Reading the output
//
// Every rung is normalised against players FPL is not warning about, because
// `ExpectedMinutes` does not predict a healthy player exactly and an unnormalised fit
// would measure the model's calibration rather than the flag's meaning. The reference
// is printed **split as well as pooled**, because its two halves disagree and pooling
// them is what an earlier version did silently.
//
// The exponent is printed with its sensitivities rather than alone: by reference
// choice, by season, and with the rung weights — the 25% rung carries most of the fit
// by construction and is the rung that replicates worst, so a single pooled number
// claims more than is known.
//
//	go run ./cmd/flagfit
//	go run ./cmd/flagfit -floor 0
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"armband/internal/backtest"
	"armband/internal/capture"
	"armband/internal/config"
)

// pairs are {prior, played}. Six played seasons, the same grid as
// `extendedPairNames`, so this and the replay cover the same football.
var pairs = [][2]string{
	{"2019-20", "2020-21"},
	{"2020-21", "2021-22"},
	{"2021-22", "2022-23"},
	{"2022-23", "2023-24"},
	{"2023-24", "2024-25"},
	{"2024-25", "2025-26"},
}

type obs struct {
	season   string
	flag     int // -1 for "no percentage published"
	status   string
	expected float64 // the model's ExpectedMinutes at the deadline
	actual   float64 // realised minutes, per match
	double   bool
}

type skips struct{ noCapture, noRow, belowFloor, tooStale int }

func main() {
	capRoot := flag.String("captures", "data/captures", "capture store root")
	cfgPath := flag.String("config", "config.json", "shipped config, for Weights")
	floor := flag.Float64("floor", 30, "minimum ExpectedMinutes to include")
	maxLead := flag.Float64("maxlead", 0, "drop captures taken more than this many hours "+
		"before the deadline (0 = keep all). A stale flag drifts toward the unflagged "+
		"population, and Wayback coverage improves across the archive, so this is "+
		"confounded with season.")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fatal("loading %s: %v\nWithout the shipped weights this measures a different model.", *cfgPath, err)
	}
	store, err := capture.Open(*capRoot)
	if err != nil {
		fatal("opening capture store at %s: %v", *capRoot, err)
	}

	var all []obs
	var sk skips
	ctx := context.Background()
	for _, p := range pairs {
		prior, err := backtest.Load(ctx, cfg.CacheDir, p[0])
		if err != nil {
			fatal("loading prior %s: %v", p[0], err)
		}
		cur, err := backtest.Load(ctx, cfg.CacheDir, p[1])
		if err != nil {
			fatal("loading %s: %v", p[1], err)
		}
		// MinutesHalfLife left zero: SimConfig falls back to Weights.MinutesHalfLife,
		// which is the shipped value.
		sc := backtest.SimConfig{Weights: cfg.Weights}
		all = append(all, collect(store, cur, prior, p[1], sc, *floor, *maxLead, &sk)...)
	}
	report(all, sk, *floor)
}

func collect(store *capture.Store, cur, prior *backtest.Season, season string,
	sc backtest.SimConfig, floor, maxLead float64, sk *skips) []obs {
	byCode := cur.ByCode()
	var out []obs
	for gw := 2; gw <= 38; gw++ {
		if store.Count(season, gw) == 0 {
			continue
		}
		// Read the capture once per gameweek. Calling store.Player per element
		// re-decompresses and re-parses the whole payload every time — about
		// 130,000 full parses over this grid, which is the difference between
		// running in seconds and in half an hour.
		av, err := store.At(season, gw)
		if err != nil {
			// Fatal rather than skipped: store.Count said a capture is here, so a
			// read failure is a real problem, and a silently short gameweek looks
			// exactly like a healthy one.
			fatal("reading capture %s GW%d: %v", season, gw, err)
		}
		if maxLead > 0 && av.HoursBefore > maxLead {
			// A capture taken days before the deadline records a flag that may have
			// been revised since, which attenuates the rung toward the unflagged
			// population. Coverage improves across the archive, so this is
			// confounded with season and has to be filterable.
			sk.tooStale++
			continue
		}
		// EngineAt is the replay's own wiring — recency index, prior blend, team
		// form. Do NOT replace it with PointInTime + NewEngineFull: that omission
		// is what invalidated this program's first result.
		e, boot := backtest.EngineAt(cur, prior, gw-1, sc)
		for i := range boot.Elements {
			el := &boot.Elements[i]
			m := e.Metrics(el)
			if m.ExpectedMinutes < floor {
				sk.belowFloor++
				continue
			}
			ps, ok := av.Players[el.Code]
			if !ok {
				sk.noCapture++
				continue
			}
			pl, ok := byCode[el.Code]
			if !ok {
				sk.noCapture++
				continue
			}
			g, ok := pl.GWs[gw]
			if !ok {
				// No row is a blank gameweek for his club rather than a broken
				// join — but counted, because a broken join would look the same.
				sk.noRow++
				continue
			}
			fx := g.Fixtures
			if fx < 1 {
				fx = 1
			}
			f := -1
			if ps.ChanceNext != nil {
				f = *ps.ChanceNext
			}
			// Per match on both sides. Every field on a gameweek row is a total
			// across that week's fixtures, so a double posts 180 minutes; the
			// wired denominator is per match, so the numerator must be too.
			out = append(out, obs{season: season, flag: f, status: ps.Status,
				expected: m.ExpectedMinutes, actual: float64(g.Minutes) / float64(fx),
				double: fx > 1})
		}
	}
	return out
}

var rungs = []int{0, 25, 50, 75, 100, -1}

func label(f int) string {
	if f < 0 {
		return "none"
	}
	return fmt.Sprintf("%d%%", f)
}

func report(all []obs, sk skips, floor float64) {
	if len(all) == 0 {
		fatal("no observations; is the capture store populated?")
	}
	by := group(all)
	refPooled := ratio(append(append([]obs{}, by[100]...), by[-1]...))
	refNone := ratio(by[-1])
	ref100 := ratio(by[100])

	fmt.Printf("flagfit — the flag against the model's own ExpectedMinutes\n")
	fmt.Printf("engine wired as the replay wires it; minutes per match\n\n")
	fmt.Printf("observations: %d over %d seasons (floor: ExpectedMinutes >= %.0f)\n",
		len(all), countSeasons(all), floor)
	fmt.Printf("skipped: %d below floor, %d no capture row, %d no gameweek row (blanks), "+
		"%d gameweeks dropped as stale captures\n",
		sk.belowFloor, sk.noCapture, sk.noRow, sk.tooStale)
	fmt.Printf("double gameweeks: %.2f%% of rows, divided out\n\n", 100*share(all))

	fmt.Printf("reference — the two halves DISAGREE, so the choice is printed not hidden:\n")
	fmt.Printf("  no flag        %.3f   (n=%d)\n", refNone, len(by[-1]))
	fmt.Printf("  explicit 100%%  %.3f   (n=%d)\n", ref100, len(by[100]))
	fmt.Printf("  pooled         %.3f\n\n", refPooled)

	fmt.Printf("%-6s %8s %10s %12s %12s %10s %10s\n",
		"flag", "n", "raw r/e", "norm(none)", "norm(pool)", "flag^2", "flag^1.5")
	for _, f := range rungs {
		v := by[f]
		if len(v) < 20 {
			continue
		}
		r := ratio(v)
		var sq, p15 string
		if f > 0 {
			x := float64(f) / 100
			sq, p15 = fmt.Sprintf("%10.3f", x*x), fmt.Sprintf("%10.3f", math.Pow(x, 1.5))
		} else {
			sq, p15 = "         -", "         -"
		}
		fmt.Printf("%-6s %8d %10.3f %12.3f %12.3f %s %s\n",
			label(f), len(v), r, r/refNone, r/refPooled, sq, p15)
	}

	fmt.Printf("\nexponent p in realised = flag^p, by reference choice:\n")
	for _, c := range []struct {
		name string
		ref  float64
	}{{"no flag", refNone}, {"pooled", refPooled}, {"explicit 100%", ref100}} {
		if p, ok := fit(by, c.ref); ok {
			fmt.Printf("  %-14s %.2f\n", c.name, p)
		}
	}

	fmt.Printf("\nrung weights in that fit — it is mostly ONE rung:\n")
	w := map[int]float64{}
	var tot float64
	for _, f := range []int{25, 50, 75} {
		if len(by[f]) >= 20 {
			x := math.Log(float64(f) / 100)
			w[f] = x * x
			tot += x * x
		}
	}
	for _, f := range []int{25, 50, 75} {
		if w[f] > 0 {
			fmt.Printf("  %-6s n=%-6d %.1f%% of the fit\n", label(f), len(by[f]), 100*w[f]/tot)
		}
	}

	fmt.Printf("\nper season — the pooled exponent averages over a trend:\n")
	fmt.Printf("%-9s %8s %8s %8s %8s\n", "season", "25%", "50%", "75%", "p")
	for _, s := range seasonList(all) {
		var sub []obs
		for _, o := range all {
			if o.season == s {
				sub = append(sub, o)
			}
		}
		sb := group(sub)
		rn := ratio(sb[-1])
		fmt.Printf("%-9s", s)
		for _, f := range []int{25, 50, 75} {
			if len(sb[f]) < 20 || math.IsNaN(rn) {
				fmt.Printf("%8s", "-")
				continue
			}
			fmt.Printf("%8.3f", ratio(sb[f])/rn)
		}
		if p, ok := fit(sb, rn); ok {
			fmt.Printf("%8.2f\n", p)
		} else {
			fmt.Printf("%8s\n", "-")
		}
	}

	fmt.Printf("\nthe no-flag rung split by status, since it pools two populations:\n")
	st := map[string][]obs{}
	for _, o := range by[-1] {
		st[o.status] = append(st[o.status], o)
	}
	keys := make([]string, 0, len(st))
	for k := range st {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if len(st[k]) < 20 {
			continue
		}
		fmt.Printf("  status %-2s n=%-6d r/e %.3f\n", k, len(st[k]), ratio(st[k]))
	}
}

func group(v []obs) map[int][]obs {
	by := map[int][]obs{}
	for _, o := range v {
		by[o.flag] = append(by[o.flag], o)
	}
	return by
}

func share(v []obs) float64 {
	var n int
	for _, o := range v {
		if o.double {
			n++
		}
	}
	return float64(n) / float64(len(v))
}

// ratio is the ratio of means — total realised over total expected — which is the
// estimand a multiplier wants.
func ratio(v []obs) float64 {
	var a, e float64
	for _, o := range v {
		a += o.actual
		e += o.expected
	}
	if e == 0 {
		return math.NaN()
	}
	return a / e
}

// fit solves for p in normalised = (flag/100)^p by least squares on the log scale,
// over the interior rungs only: 0% and 100% carry no information about an exponent
// because any p reproduces them.
func fit(by map[int][]obs, ref float64) (float64, bool) {
	if math.IsNaN(ref) || ref <= 0 {
		return 0, false
	}
	var sxy, sxx float64
	var n int
	for _, f := range []int{25, 50, 75} {
		v := by[f]
		if len(v) < 20 {
			continue
		}
		y := ratio(v) / ref
		if y <= 0 || math.IsNaN(y) {
			continue
		}
		x := math.Log(float64(f) / 100)
		sxy += x * math.Log(y)
		sxx += x * x
		n++
	}
	if n < 2 || sxx == 0 {
		return 0, false
	}
	return sxy / sxx, true
}

func countSeasons(all []obs) int { return len(seasonList(all)) }

func seasonList(all []obs) []string {
	seen := map[string]bool{}
	for _, o := range all {
		seen[o.season] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
