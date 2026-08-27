package snapshot

// The model half: is the scoring model right about football?
//
// This is a different question from the harness half and must not be blurred with
// it. A model can be well-calibrated while the harness cannot resolve any change
// to it, which is this project's actual situation — and reading one as though it
// answered the other is how "the harness could not see it" came to be written up
// as "there is no effect", repeatedly and in both directions.
//
// Everything here is measured against *outcomes* — what the players went on to
// score — rather than against another setting of the model. That is why these
// figures do not carry a standard error from the sweep machinery: their unit is a
// player-cutoff or a team-match, not a replayed season, so clustering four
// seasons would be answering a question nobody asked. It is also why they are the
// more trustworthy half: several of them rest on thousands of observations where
// the harness has twenty-four.

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Measurement is one number from one diagnostic.
type Measurement struct {
	Diagnostic string
	RunID      string
	Grid       string
	Group      string
	N          int
	Measure    string
	Value      float64
}

// Diagnostic is one model-accuracy table, ready to print.
type Diagnostic struct {
	Slug     string
	Title    string
	What     string // what it measures, one sentence
	Reading  string // which direction is better, and what the numbers mean
	Grid     string
	Groups   []string       // row labels, in emitted order
	Measures []string       // column labels, in first-seen order
	N        map[string]int // per group
	Values   map[string]map[string]float64
}

// registry is the prose. It is not data the CSV could carry: "ratio 0.899" means
// nothing to a reader who has to guess whether high is good, and a snapshot whose
// tables cannot be read without the source open is not a snapshot.
//
// A diagnostic absent from this map still appears, with its slug as the title and
// no reading note — so adding a diagnostic never silently drops its rows, which is
// the failure mode a lookup table invites.
var registry = map[string]struct{ title, what, reading string }{
	"calibration_drift": {
		title: "Does the model's confidence drift through a season?",
		what: "For each cutoff the model is built from data through that gameweek " +
			"and every player's predicted points per gameweek is compared with what " +
			"he actually scored over the next five. Restricted to players the model " +
			"would consider — 2.0+ predicted points on 45+ expected minutes — because " +
			"whether it correctly rates a reserve is not the question.",
		reading: "The ratio is actual divided by predicted, so **1.000 is perfect " +
			"calibration and below 1.000 means the model over-predicts**. Read the " +
			"predicted and actual columns separately rather than only the ratio: if " +
			"actual is flat while predicted rises, the model is not getting worse at " +
			"football, it is getting more confident while reality stays where it was.",
	},
	"transfer_error": {
		title: "Is the model more wrong about a player it buys than one it sells?",
		what: "Every transfer the replay made, judged against what the two players " +
			"went on to do over the window the decision was justified on. Error is " +
			"realised minus modelled, per gameweek.",
		reading: "**Negative means the player was over-rated.** The asymmetry row is " +
			"the one that matters: a model that is simply badly calibrated moves both " +
			"sides together, while a search that hunts the top of a noisy estimate " +
			"distribution moves the buy side further. Note the move count in the " +
			"population line — the transfer gate decides which moves exist to judge, " +
			"so two runs at different gate settings measure different populations and " +
			"are not a reproduction of each other.",
	},
	"defcon_bias": {
		title: "Is the defensive-contribution term already priced by something else?",
		what: "Players grouped into thirds by how many defensive actions they record " +
			"per 90 minutes, within position — because defensive contribution and " +
			"position are nearly the same variable, so pooling positions would read " +
			"the model's bias by position as a defcon effect. Reversing that " +
			"distinction reverses the answer.",
		reading: "Bias is actual minus predicted points per 90, so **negative means " +
			"over-prediction**. If the term were fully earned, bias would be flat " +
			"across the three groups. If it were entirely redundant, bias would fall " +
			"by exactly the term's growth and 'bias plus term' would be flat instead " +
			"— which is the redundancy signature to look for.",
	},
	"bps_rule_change": {
		title: "What does FPL's 2026/27 bonus rule change do to a player's bonus rate?",
		what: "FPL's new Bonus Points System applied to 2025-26's actual football: " +
			"BPS recomputed under the new clearances/blocks/interceptions divisor and " +
			"saves schedule, then **re-ranked within each match** and the 3/2/1 awards " +
			"recomputed. Re-ranking is the point — bonus goes to the top three BPS " +
			"scorers in a match, so what matters is whether the ordering moves, not " +
			"the level. The machinery is validated by reproducing all 29,747 recorded " +
			"awards from the recorded BPS exactly.",
		reading: "Shift is the percentage change in realised bonus per 90, so " +
			"**positive means the new rules pay a group more**. Unusually for this " +
			"snapshot, neither direction is better and there is no error being " +
			"measured: it is a rule, applied to a complete season, so the numbers are " +
			"an enumeration rather than an estimate. What is bad is **spread inside a " +
			"position**, because `Bonus90` is a historical rate measured under the old " +
			"rules and the optimiser consumes an ordering — a shift shared by every " +
			"player in a position cannot change which five defenders you buy, and one " +
			"that separates centre-backs from full-backs can. Two caveats travel " +
			"with it. The rate rows sweep the one input the archive lacks — what " +
			"share of a keeper's saves were saves from a big chance — and **rate 0.00 " +
			"is the assumption-free floor of that sweep**, using only the " +
			"exactly-computable CBI change. It is **not** a floor on the net rule " +
			"change, because a fifth change is missing from every column here and for " +
			"keepers it runs the other way. That fifth change is the removed " +
			"'tackled' penalty, and it **is** in the archive — a per-player-gameweek " +
			"column in 2016-17, 2017-18 and 2018-19, seasons inside the same BPS " +
			"regime — where it prices at about −15% for keepers, −5% to −11% for " +
			"defenders and +6% to +8% for midfielders and forwards. Those cannot be " +
			"added to the figures here: bonus is a rank over a fixed six-point pool " +
			"so the channels do not sum, and the two measurements are different " +
			"seasons under different schedules. So read every column below as **the " +
			"CBI-and-saves half of the change** rather than as a bound on it. Read " +
			"the within-position " +
			"terciles rather than the position totals: the position-wide defender " +
			"figure is about a third of the spread inside the position, and a bias " +
			"that moves players in opposite directions within a position is the kind " +
			"an argmax can see.",
	},
	"clean_sheet_calibration": {
		title: "Is the clean sheet priced correctly?",
		what: "One row per team-match, not per player, since eleven team-mates share " +
			"a clean sheet and counting them separately would multiply the same " +
			"observation by eleven.",
		reading: "Error is predicted minus actual, so **positive means the model " +
			"over-predicts**. The two rows separate the two candidate causes: if " +
			"expected and actual goals conceded agree, the bias is not in the " +
			"expected-goals figure but in the Poisson applied to it. " +
			"⚠️ **Read the size here as a property of THIS regressor, not of the " +
			"model.** These rows are cut on realised single-match xGC; the model " +
			"scores on `XGC90`, a blended and shrunk per-90 rate, and `exp()` is " +
			"convex, so the two disagree by construction. Refit against `XGC90` " +
			"itself, predicted/actual is 1.052 native and 1.004 pooled against the " +
			"~1.28 here. ⚠️ And the old reading — that a bias shared by every player " +
			"in a position is not an ordering error — **is contested**: the squad " +
			"quota fixes how many defenders you own, not how many are fielded, and " +
			"the optimiser is a knapsack against one budget rather than an ordering " +
			"consumer — **mechanism, unmeasured on points**. Correcting a measured " +
			"bias has still lost points five times.",
	},
	"sixty_minute_threshold": {
		title: "Are the terms FPL pays as a step scaled as a step?",
		what: "Appearance points and the clean sheet are paid at sixty minutes, not " +
			"prorated: a starter taken off at seventy banks both in full. The model " +
			"scales them by an estimated probability of reaching sixty minutes, and " +
			"this compares that estimate against how often players in each minutes " +
			"band actually reached it.",
		reading: "Error is what the model credits minus what happens, so **positive " +
			"means the model over-credits**. Unlike the clean-sheet bias this one is " +
			"*not* uniform across players, so it mis-ranks a part-timer against an " +
			"ever-present — an ordering error, which the optimiser does see. Read it " +
			"as a **shape**: the error crosses zero near fifty minutes, " +
			"under-crediting the fringe band and over-crediting rotation players, so " +
			"it is the slope between those two groups that is wrong rather than the " +
			"level of either. ⚠️ Figures for this section in snapshots before " +
			"`2026-08-10-27740ba` and every earlier snapshot measured the " +
			"minutes-reliability proxy `playsSixty` REPLACED, not the shipped curve — " +
			"the diagnostic kept its own copy and labelled it \"model now\". Their " +
			"error turns positive only in the top band, where the shipped curve's " +
			"crosses near fifty, so the two disagree about WHERE the model " +
			"over-credits and not merely by how much. Do not read a trend across that " +
			"boundary.",
	},
	"prediction_coverage": {
		title: "Did every season reach the prediction benchmark's sample?",
		what: "How many gameweeks and how many player-gameweeks each season " +
			"contributed to the benchmark's headline population — players who " +
			"played sixty or more minutes in one of the previous five gameweeks " +
			"their club played.",
		reading: "**Higher is better and even across seasons is what matters** — " +
			"the seasons should contribute roughly equally, and a season " +
			"contributing nothing means the population filter is reading an " +
			"archive column that season does not carry. That is not a hypothetical: " +
			"the per-gameweek `starts` field is empty for all of 2021-22 and for " +
			"2022-23 before GW16, and a filter reading it silently made a " +
			"four-season figure into a three-season one while every other table " +
			"stayed plausible. One gameweek missing from 2022-23 is expected — its " +
			"GW7 was postponed outright.",
	},
	"prediction_benchmark": {
		title: "How wrong is the model about one player in one gameweek?",
		what: "Out-of-sample error predicting a single gameweek from a model built " +
			"through the gameweek before, split by what the player ACTUALLY scored: " +
			"Zeros recorded no minutes, Blanks played for two points or fewer, " +
			"Tickers scored three or four, Haulers five or more. The categories are " +
			"OpenFPL's (arXiv 2508.09992) so the figures sit beside published ones. " +
			"Two naive baselines are shown for scale: the mean of the last five " +
			"gameweeks, which is OpenFPL's own baseline, and the flat season average, " +
			"which is what FPL's bootstrap publishes.",
		reading: "Mean absolute error and root-mean-square error are both in the " +
			"target's own units and **lower is better**. Bias is predicted minus " +
			"actual, so positive means over-prediction, and error sd is the spread " +
			"around that bias — root-mean-square error squared is exactly bias " +
			"squared plus error sd squared. **The categories condition on the " +
			"outcome, which rewards a noisier predictor in the extreme buckets**: a " +
			"predictor that fires more high numbers will look better on Haulers " +
			"while being worse calibrated at the top of its own distribution, so " +
			"read the Haulers column beside the calibration and ordering tables and " +
			"never on its own. This instrument ranks candidates and cannot price " +
			"them — the replay does that, and a better predictor can make a worse " +
			"policy.",
	},
	"prediction_calibration": {
		title: "Do the players the model rates at 5.0 score 5.0?",
		what: "The same one-gameweek-ahead predictions grouped by what was " +
			"PREDICTED rather than by what happened, so the table reads at the level " +
			"decisions are made at.",
		reading: "The ratio is actual divided by predicted, so **1.000 is perfect " +
			"and below 1.000 means the band is over-predicted**. The top band is " +
			"where a transfer search picks, so its ratio matters more than the " +
			"aggregate: a bias shared by every player is invisible to an argmax, and " +
			"this project has measured that correcting one costs points. Error sd is " +
			"the spread of the error around its own bias inside the band, and it is " +
			"the one figure here conditioned on what was PREDICTED rather than on what " +
			"happened — the benchmark table's error sd conditions on the outcome, so " +
			"it describes players who turned out to haul and not players the model " +
			"says will. **It is not a per-player interval and may not be quoted as " +
			"one**: the spread is pooled across the whole band, and the top band is " +
			"open-ended, so its figure mixes a prediction of 6 with a prediction of 15. " +
			"Each error sd is itself a point estimate with unreported sampling error — " +
			"near 2% by normal theory at these counts, and optimistic in the top bands, " +
			"where a small set of premium players recurs across weeks and the rows are " +
			"further from independent draws than their count suggests. Read the rise in " +
			"error sd and the fall in error sd divided by the prediction as **one fact " +
			"and not two**: variance over predicted is flat at 3.34 to 3.77 across every " +
			"band above 1.0, which is mean-variance scaling for count data doing the " +
			"work. The exception is the bottom band at 4.68, and that — not the " +
			"monotone rise — is the one figure here the scaling does not explain.",
	},
	"prediction_ordering": {
		title: "Does the model rank players correctly, and is its top over-rated?",
		what: "Spearman's rank correlation — the ordinary correlation computed on " +
			"ranks rather than values — between predicted and actual points within " +
			"each gameweek, averaged over gameweeks. Beside it, the signed error " +
			"over the twenty highest-predicted players in each gameweek, which is " +
			"roughly the set a transfer search chooses between.",
		reading: "For the rank correlation, +1 is a perfect ordering, 0 is no " +
			"ordering information and **higher is better**. This axis exists because " +
			"the optimiser consumes an ordering and never a level, which is why the " +
			"bonus term is kept despite being badly calibrated. For the tail figure, " +
			"**positive means the top of the predicted distribution is over-rated** " +
			"and closer to zero is better — it is the winner's curse as a measured " +
			"number rather than an inference.",
	},
	"prediction_candidates": {
		title: "Is a candidate change safe for an argmax, or a variance trade?",
		what: "Each arm of the benchmark paired against the shipped config on the " +
			"same observations. Two of the arms are controls rather than proposals: " +
			"switching minutes recency off must make the minutes error worse, and " +
			"switching the vice-captain fallback off must change nothing at all, " +
			"since it alters how a played-out gameweek is scored and not what the " +
			"model predicts.",
		reading: "These are differences, so **negative means better** for the error " +
			"columns. The distinction that matters is whether an improvement came " +
			"from shrinking the systematic part of the error (bias reduction, safe " +
			"for an argmax, because removing a systematic error cannot reorder " +
			"candidates by chance) or from shrinking the spread while the bias grew " +
			"(a bias-for-variance trade, dangerous — the recorded reason recency on " +
			"minutes gained points and recency on rates lost them). Read the tail " +
			"and ordering rows beside the error rows: a candidate that lowers " +
			"aggregate error while pushing the tail figure away from zero has the " +
			"better-predictor-worse-policy shape.",
	},
	"next_five_predictors": {
		title: "How much should a predictor weight recent gameweeks?",
		what: "Six ways of summarising a player's record so far, each predicting his " +
			"mean over the next five gameweeks. No model is built: every predictor is " +
			"arithmetic on the archive, which makes this a clean test of the recency " +
			"question rather than of the model that consumes the answer. A half-life " +
			"of h means a gameweek h gameweeks back counts half as much as the most " +
			"recent one, so smaller means sharper recency.",
		reading: "Mean absolute error in the target's own units, **lower is better**. " +
			"The shape to look for, not the level: **minutes reward sharp recency and " +
			"rates punish it.** Minutes are a statement about the present, so " +
			"weighting recent gameweeks removes a bias — a player who lost his place " +
			"six weeks ago is not a starter. Rates are a statement about quality, and " +
			"a short window chases finishing variance, so the same weighting trades " +
			"bias for variance. That is the evidence two shipped constants rest on, " +
			"and the diagnostic fails if either half stops holding.",
	},
	"team_rate_predictors": {
		title: "How much of this season should a team rating believe?",
		what: "For each club and each cutoff, predict the *remainder* of the season's " +
			"goals from the record so far, from last season, and from a blend of the " +
			"two at each prior strength. Scored out of sample, which is the one " +
			"predictor comparison this project can currently run.",
		reading: "Root-mean-square error predicting the rest of the season, so " +
			"**lower is better**. Compare within a row and never down a column: the " +
			"absolute errors rise at later cutoffs because a shorter remainder is a " +
			"noisier thing to predict, not because the predictor got worse. The prior " +
			"strength is in matches — a strength of k gives this season a weight of " +
			"n/(n+k) after n matches.",
	},
}

// ReadModel parses a model-accuracy CSV into printable diagnostics.
func ReadModel(path string) ([]Diagnostic, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: unreadable header: %w", path, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	for _, n := range []string{"diagnostic", "group", "measure", "value"} {
		if _, ok := col[n]; !ok {
			return nil, fmt.Errorf("%s: no %q column; this is not a model CSV", path, n)
		}
	}
	get := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}

	var rows []Measurement
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		n, _ := strconv.Atoi(get(rec, "n"))
		v, err := strconv.ParseFloat(get(rec, "value"), 64)
		if err != nil {
			continue
		}
		rows = append(rows, Measurement{
			Diagnostic: get(rec, "diagnostic"), RunID: get(rec, "run_id"),
			Grid: get(rec, "grid"), Group: get(rec, "group"), N: n,
			Measure: get(rec, "measure"), Value: v,
		})
	}
	return assemble(rows), nil
}

// ModelRunIDs returns every distinct run_id in a model CSV, in file order.
//
// # Why a caller needs this, and what it cost to find out
//
// ⚠️ **The model CSV APPENDS, its path is outside the repository, and the renderer
// keeps the newest run. Those three together shipped a wrong snapshot to `main`.**
//
// `stats/snapshots/2026-08-15-9e743cf` carries clean-sheet calibration measured on
// **2955 team-matches** where the commit it names produces **2870** — the 85 doubled
// rows that `3b6a698` ("Fix the clean-sheet diagnostic, and retract the finding it
// produced", 2026-08-14) removed. `3b6a698` is an *ancestor* of `9e743cf`, so the
// snapshot's own commit had the fix and its figures did not. Found 2026-08-15 by
// regenerating on a comment-only change and asking why seven figures moved.
//
// The mechanism is not a bug in any one place. `FPL_MODEL_CSV` accumulates rows run
// after run; the file lives in a scratch directory shared by every session and every
// branch on the machine; and `assemble` below resolves a collision by keeping the
// later row. So "the later one" means later in **wall-clock time**, which is only
// the same as "produced by this code" when one checkout owns the file. The reasoning
// in `assemble`'s comment is sound within a checkout and silently false across them.
//
// A commit column in the CSV would let this be a hard failure rather than a warning,
// and it is the right fix — the schema is `diagnostic,run_id,grid,group,n,measure,
// value` and records nothing about the code that produced a row. Until then, the
// honest signal available is that the file holds more than one run at all, which is
// what this reports. **It is a warning and not a gate**: a file with one run is not
// thereby trustworthy, it is merely not self-evidently mixed.
//
// The standing advice — "pass a path nobody else writes to" — is now load-bearing
// rather than tidy, and it is not sufficient either: a *fresh* path still collects
// several runs the moment two diagnostics are run separately into it.
func ModelRunIDs(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, err
	}
	col := -1
	for i, h := range head {
		if strings.TrimSpace(h) == "run_id" {
			col = i
		}
	}
	if col < 0 {
		return nil, fmt.Errorf("%s has no run_id column", path)
	}
	seen := map[string]bool{}
	var ids []string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if col < len(rec) && rec[col] != "" && !seen[rec[col]] {
			seen[rec[col]] = true
			ids = append(ids, rec[col])
		}
	}
	return ids, nil
}

// assemble pivots the long rows into one table per diagnostic.
//
// A later run of the same diagnostic overwrites an earlier one for the same
// (diagnostic, group, measure). That is right for this artefact and wrong for the
// cells file, and the difference is worth stating: a cells row is a *sample* and
// pooling two runs would manufacture confidence, whereas a model measurement is a
// deterministic function of the archive, so two runs of it are the same number
// unless the model changed — in which case the later one is the one that ships.
//
// ⚠️ **That last clause is only true within one checkout, and the file is shared
// across all of them.** See `ModelRunIDs` above for the snapshot this shipped to
// `main`. "The later one" is later in wall-clock time, not later in this history.
func assemble(rows []Measurement) []Diagnostic {
	byDiag := map[string]*Diagnostic{}
	var order []string
	for _, r := range rows {
		d, ok := byDiag[r.Diagnostic]
		if !ok {
			meta := registry[r.Diagnostic]
			title := meta.title
			if title == "" {
				// Unregistered: show it anyway. A lookup miss must not drop data.
				title = r.Diagnostic + " (no description registered)"
			}
			d = &Diagnostic{
				Slug: r.Diagnostic, Title: title, What: meta.what,
				Reading: meta.reading,
				N:       map[string]int{},
				Values:  map[string]map[string]float64{},
			}
			byDiag[r.Diagnostic] = d
			order = append(order, r.Diagnostic)
		}
		d.Grid = r.Grid
		if _, seen := d.Values[r.Group]; !seen {
			d.Groups = append(d.Groups, r.Group)
			d.Values[r.Group] = map[string]float64{}
		}
		if !contains(d.Measures, r.Measure) {
			d.Measures = append(d.Measures, r.Measure)
		}
		d.Values[r.Group][r.Measure] = r.Value
		if r.N > 0 {
			d.N[r.Group] = r.N
		}
	}
	out := make([]Diagnostic, 0, len(order))
	for _, k := range order {
		// Both groups and measures keep the order the diagnostic emitted them in,
		// which is meaningful in each case: groups are the ladder it swept, and
		// measures are the reading order it chose. Sorting the measures instead would
		// be stable but alphabetical, which renders "predicted" after "mean absolute
		// error" and separates the two columns the reader is meant to compare.
		out = append(out, *byDiag[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
