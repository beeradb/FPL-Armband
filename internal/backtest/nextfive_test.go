package backtest

// Re-deriving the predictor comparison that set two constants and was then
// frozen into prose.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagNextFivePredictors -v -timeout 30m
//
// # What this is for
//
// Two of the model's constants rest on one out-of-sample table: `MinutesHalfLife`
// is 4 because sharpening recency on *minutes* predicted better, and
// `RateHalfLife` is 0 — flat — because doing the same to *rates* predicted worse.
// The table was recorded in AGENTS.md and in comments in `internal/analysis` and
// `internal/recent`, and nothing in the repository could recompute it: the test
// beside those comments asserts the constants rather than re-deriving the errors.
// So the figures had become **orphaned** — quotable, and not reproducible — which
// is the specific failure the accuracy snapshot exists to prevent, and the reason
// the snapshot's first edition had to say the predictor comparison was missing.
//
// This recomputes it. It is deliberately separate from the one-gameweek-ahead
// benchmark in prediction_test.go, because it answers a different question and
// costs almost nothing: no engine is built at all. Every predictor here is
// arithmetic on the archive, which is what makes it a clean test of the *recency
// question* rather than of the model that consumes the answer.
//
// # What is compared
//
// At each cutoff, four ways of summarising a player's record so far, each used to
// predict his mean over the **next five gameweeks**:
//
//	season to date        every gameweek so far weighted equally. This is what
//	                      FPL's own bootstrap publishes, so it is the predictor
//	                      the model falls back to when per-gameweek history is
//	                      unavailable.
//	last 3 gameweeks      a short flat window.
//	ewma half-life h      an exponentially weighted moving average: a gameweek h
//	                      gameweeks back counts half as much as the most recent
//	                      one. Smaller h means sharper recency.
//
// Three targets, each per gameweek his club played: minutes, FPL points, and
// expected goals plus expected assists.
//
// # The claim being re-derived, which is a shape and not a number
//
// **Minutes reward sharp recency and rates punish it.** Minutes are a statement
// about the present — a player who lost his place six weeks ago is not a starter
// — so weighting recent gameweeks removes a *bias*. Rates are a statement about
// quality, and a short window chases finishing variance, so weighting recent
// gameweeks trades bias for variance. That distinction is the project's rule for
// whether a recency change is safe for an argmax, and this table is where it came
// from.
//
// The recorded figures were 20.83 minutes mean absolute error for the season
// average against 18.98 at a half-life of 2, with points 4% worse and expected
// goals plus assists 4% worse at the same setting. **Do not expect those exact
// numbers back.** They were measured on three seasons rather than four, before
// the doubles-counting fix that changed what a gameweek means in this archive,
// and their population was never written down. What has to reproduce is the
// ordering: the minutes column minimised at a short half-life and the two rate
// columns minimised at a long one or flat.
//
// The run fails if that ordering does not reproduce, because the ordering is the
// evidence for two shipped constants and an instrument that cannot see it is not
// worth having.

import (
	"fmt"
	"math"
	"testing"
)

// nextFiveWindow is how many gameweeks ahead the prediction covers. Five, to
// match the recorded table and because it is the model's shipped horizon.
const nextFiveWindow = 5

// nextFiveCutoffs is where the model stands when it predicts. From GW6 so every
// predictor has at least five gameweeks behind it, and stopping early enough that
// a full five-gameweek window remains ahead.
func nextFiveCutoffs() []int {
	var out []int
	for gw := predictFirstGW - 1; gw <= 38-nextFiveWindow; gw++ {
		out = append(out, gw)
	}
	return out
}

// recencyPredictor is one way of summarising a player's record so far.
type recencyPredictor struct {
	label string
	// halfLife in gameweeks; 0 means flat over the window.
	halfLife float64
	// window caps how many club gameweeks are looked at; 0 means all of them.
	window int
}

func recencyPredictors() []recencyPredictor {
	return []recencyPredictor{
		{label: "season to date (flat)"},
		{label: "last 3 gameweeks (flat)", window: 3},
		{label: "ewma, half-life 2", halfLife: 2},
		{label: "ewma, half-life 4", halfLife: 4},
		{label: "ewma, half-life 8", halfLife: 8},
		{label: "ewma, half-life 20", halfLife: 20},
	}
}

// weightedRecord applies one predictor to a player's history up to and including
// `cut`, per gameweek his club played.
//
// Gameweeks the club did not play are skipped and gameweeks it played in which he
// has no row count as zero, which is the same convention the one-gameweek-ahead
// benchmark uses — one definition of a gameweek across both instruments, so the
// two are readable side by side.
func weightedRecord(p *Player, clubGWs map[int]int, cut int, pr recencyPredictor,
	f func(GW) float64) float64 {
	var num, den float64
	seen := 0
	for w := cut; w >= 1; w-- {
		if clubGWs[w] == 0 {
			continue
		}
		if pr.window > 0 && seen >= pr.window {
			break
		}
		seen++
		wt := 1.0
		if pr.halfLife > 0 {
			wt = math.Pow(0.5, float64(cut-w)/pr.halfLife)
		}
		num += wt * f(p.GWs[w])
		den += wt
	}
	if den == 0 {
		return 0
	}
	return num / den
}

// nextFiveMean is the realised mean over the next five gameweeks his club played,
// and how many of them there were.
func nextFiveMean(p *Player, clubGWs map[int]int, cut int, f func(GW) float64) (float64, int) {
	var sum float64
	n := 0
	for w := cut + 1; w <= 38 && n < nextFiveWindow; w++ {
		if clubGWs[w] == 0 {
			continue
		}
		sum += f(p.GWs[w])
		n++
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}

func TestDiagNextFivePredictors(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()

	pairs := loadPairs(t, cfg)
	preds := recencyPredictors()
	tgts := []struct {
		name, unit string
		f          func(GW) float64
	}{
		{"minutes", "minutes per gameweek", gwMinutes},
		{"points", "FPL points per gameweek", gwPoints},
		{"expected goals + assists", "xG+xA per gameweek", gwXGI},
	}

	// Keyed target | predictor.
	acc := map[string]*errAcc{}
	get := func(k string) *errAcc {
		a := acc[k]
		if a == nil {
			a = &errAcc{}
			acc[k] = a
		}
		return a
	}
	perSeason := map[string]int{}

	for _, pair := range pairs {
		cur := pair.Cur
		clubs := clubGameweeks(cur)
		for _, cut := range nextFiveCutoffs() {
			for _, id := range sortedPlayerIDs(cur) {
				p := cur.Players[id]
				clubGWs := clubs[p.Team]
				// The same model-independent squad-relevance filter the
				// one-gameweek benchmark uses, for the same reason: an error
				// figure over every registered footballer is dominated by
				// reserves whose every predictor says zero and is right.
				if !playedRecently(p, clubGWs, cut+1) {
					continue
				}
				perSeason[cur.Name]++
				for _, tg := range tgts {
					act, n := nextFiveMean(p, clubGWs, cut, tg.f)
					if n == 0 {
						continue
					}
					for _, pr := range preds {
						get(predKey(tg.name, pr.label)).
							add(weightedRecord(p, clubGWs, cut, pr, tg.f), act)
					}
				}
			}
		}
	}

	grid := fmt.Sprintf("%d seasons, cutoffs GW%d-%d, next %d gameweeks",
		len(pairs), predictFirstGW-1, 38-nextFiveWindow, nextFiveWindow)

	fmt.Printf("\n## How much should a predictor weight recent gameweeks?\n\n")
	fmt.Printf("Predicting each player's mean over the NEXT %d gameweeks his club plays,\n", nextFiveWindow)
	fmt.Printf("from his record up to and including the cutoff. No model is built: every\n")
	fmt.Printf("predictor here is arithmetic on the archive, which is what makes this a clean\n")
	fmt.Printf("test of the recency question rather than of the model that consumes it.\n\n")
	fmt.Printf("A half-life of h means a gameweek h gameweeks back counts half as much as\n")
	fmt.Printf("the most recent one, so SMALLER means sharper recency. Mean absolute error\n")
	fmt.Printf("and root-mean-square error are both in the target's own units and LOWER IS\n")
	fmt.Printf("BETTER for both.\n\n")
	fmt.Printf("Population: %s, at the cutoff.\n", popRelevant)
	fmt.Printf("Grid: %s.\n\n", grid)
	fmt.Printf("player-cutoffs contributing, by season:\n")
	for _, pair := range pairs {
		fmt.Printf("  %-10s %8d\n", pair.Name, perSeason[pair.Name])
		if perSeason[pair.Name] == 0 {
			t.Errorf("season %s contributed nothing; the population filter is "+
				"reading something that season does not carry", pair.Name)
		}
	}
	fmt.Printf("\n")

	best := map[string]string{}
	for _, tg := range tgts {
		fmt.Printf("### %s — %s\n\n", tg.name, tg.unit)
		fmt.Printf("%-28s %9s %10s %10s %12s\n", "predictor", "n", "MAE", "RMSE", "vs flat")
		flat := acc[predKey(tg.name, "season to date (flat)")]
		bestMAE, bestLabel := math.Inf(1), ""
		for _, pr := range preds {
			a := acc[predKey(tg.name, pr.label)]
			if a == nil || a.n == 0 {
				continue
			}
			rel := ""
			if flat != nil && flat.mae() > 0 {
				rel = fmt.Sprintf("%+.1f%%", 100*(a.mae()-flat.mae())/flat.mae())
			}
			fmt.Printf("%-28s %9d %10.4f %10.4f %12s\n",
				pr.label, a.n, a.mae(), a.rmse(), rel)
			sink.emitAll("next_five_predictors", grid, tg.name+" — "+pr.label, a.n,
				measure{"mean absolute error", a.mae()},
				measure{"root-mean-square error", a.rmse()},
				measure{"bias (predicted minus actual)", a.bias()})
			if a.mae() < bestMAE {
				bestMAE, bestLabel = a.mae(), pr.label
			}
		}
		best[tg.name] = bestLabel
		fmt.Printf("\nbest: %s\n\n", bestLabel)
	}

	fmt.Printf("### Does the recorded shape reproduce?\n\n")
	fmt.Printf("The claim two shipped constants rest on is that MINUTES reward sharp recency\n")
	fmt.Printf("and RATES punish it. Minutes are a statement about the present, so weighting\n")
	fmt.Printf("recent gameweeks removes a bias — a player who lost his place six weeks ago\n")
	fmt.Printf("is not a starter. Rates are a statement about quality, and a short window\n")
	fmt.Printf("chases finishing variance, so weighting recent gameweeks trades bias for\n")
	fmt.Printf("variance instead. Only the first is safe for an argmax, which is why\n")
	fmt.Printf("minutes_half_life is 4 and the rate half-life ships flat.\n\n")
	fmt.Printf("%-28s %-30s\n", "target", "lowest mean absolute error at")
	for _, tg := range tgts {
		fmt.Printf("%-28s %-30s\n", tg.name, best[tg.name])
	}
	fmt.Printf("\n")

	// The shape, checked rather than admired. A half-life is "sharp" here if it
	// is at most 4 gameweeks or a 3-gameweek flat window.
	sharp := map[string]bool{
		"last 3 gameweeks (flat)": true,
		"ewma, half-life 2":       true,
		"ewma, half-life 4":       true,
	}
	if !sharp[best["minutes"]] {
		t.Errorf("minutes are predicted best by %q, which is not sharp recency.\n"+
			"That contradicts the out-of-sample result MinutesHalfLife=4 rests on, so "+
			"either this instrument is wrong or a shipped constant has lost its "+
			"evidence. Both are worth stopping for.", best["minutes"])
	}
	for _, rate := range []string{"points", "expected goals + assists"} {
		if sharp[best[rate]] {
			t.Errorf("%s is predicted best by %q, which IS sharp recency.\n"+
				"That contradicts the recorded finding that rates punish recency, which "+
				"is why RateHalfLife ships flat. Either this instrument is wrong or that "+
				"constant has lost its evidence.", rate, best[rate])
		}
	}
	fmt.Printf("Both halves of the recorded shape are checked above and the run fails if\n")
	fmt.Printf("either stops holding, because the shape is the evidence for two shipped\n")
	fmt.Printf("constants and an instrument that cannot see it is not worth keeping.\n\n")
	fmt.Printf("What this does NOT establish is a value. It compares predictors and the\n")
	fmt.Printf("replay decides points: a better predictor can make a worse policy, and\n")
	fmt.Printf("recency on rates is the recorded case where the ~2%% better predictor cost\n")
	fmt.Printf("about 49 points a season.\n\n")
}
