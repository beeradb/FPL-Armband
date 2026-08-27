package backtest

// What is knowing the real team news worth to the replay?
//
//	DIAG=1 EXP=ORACLETEAMNEWS FPL_CELLS=/tmp/teamnews/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagTeamNewsOracle$' -count=1 -v -timeout 8h
//	Rscript stats/sweep_inference.R /tmp/teamnews/cells.csv
//
// # The question, and why the existing answer does not answer it
//
// The replay reconstructs availability from an end-of-season snapshot. `statusAt`
// carries a *final* status of "u" or "i" back to the moment its news was posted,
// which is right for a departure and for a season-ending injury and is silent about
// everything else: it never emits "d", and a player out from September to November
// finishes the season fit and reads as fit throughout.
//
// `OracleAvailability` is narrower again. It fires on a season **total** of zero
// minutes, so it marks only players who never appear at all — and the record is
// explicit that its ≈14 points a season is a design average off a degenerate
// population, inert in 13 of 24 held cells, rather than an answer to "what is team
// news worth".
//
// This arm has the real thing: FPL's own flag at each deadline, recovered from
// crawled bootstrap payloads, covering all 228 gameweeks of the six seasons the grid
// plays.
//
// # Two arms because there are two questions
//
// **teamnews** replaces the reconstruction with the published flag. **teamnews +
// teamnews_chance** additionally hands `availabilityFactor` the published percentage
// chance of playing, which the replay has never once seen — nothing in
// `PointInTimeWith` or `PreSeasonWith` sets that field. The percentage overrides the
// flag inside that function, so the second arm is strictly the first plus
// granularity, and the *contrast between them* is what the granularity is worth on
// its own. Merged into one arm the figure would bound neither.
//
// # What to expect, stated in advance
//
// `MinExpectedMinutes` cliffs the opening-squad pool at 55 minutes a gameweek, and
// the record already notes that this removes 85-100% of flagged players from the
// squad the held metric scores. So the effect should land mostly on `POLICY`, whose
// median detection threshold is around 70 points a season, and this is a comparison
// that may well not resolve. Converting "unmeasurable" into "measured and too small"
// is the result on offer, and it is a real one.
//
// # It changes no default, and must not
//
// Every figure in AGENTS.md and the research record was measured against the
// reconstruction. Switching this on would inflate all of them at once and make the
// record incomparable with itself — the same rule that keeps FPL_ORACLE_AVAILABILITY off,
// with more force, because this oracle moves far more players.

import (
	"fmt"
	"os"
	"testing"
)

// teamNewsFilterFromEnv is the staleness cut the run is made under.
//
// A capture read two hours before a deadline is the team news; one read nine days
// before it is last week's, and it is wrong in a *direction* — a player flagged
// after the crawl reads as available — so stale gameweeks attenuate the arm toward
// the baseline. The headline runs with the events-behind cut only, which is the
// meaningful one across an international break, and FPL_TEAMNEWS_MAX_HOURS runs the
// robustness arm.
func teamNewsFilterFromEnv(t *testing.T) TeamNewsFilter {
	t.Helper()
	f := TeamNewsFilter{MaxEventsBehind: 0}
	if s := os.Getenv("FPL_TEAMNEWS_MAX_HOURS"); s != "" {
		var h float64
		if _, err := fmt.Sscanf(s, "%g", &h); err != nil || h <= 0 {
			t.Fatalf("FPL_TEAMNEWS_MAX_HOURS=%q is not a positive number of hours", s)
		}
		f.MaxHoursBefore = h
	}
	return f
}

func TestDiagTeamNewsOracle(t *testing.T) {
	requireDiag(t)
	news, err := LoadTeamNews(teamNewsFilterFromEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	// Printed before the sweep, because it decides how the sweep's number may be
	// worded, and because a coverage gap is the difference between "nothing to see"
	// and "could not run". It costs no replay.
	reportTeamNewsScope(t, news)

	fmt.Printf("\n=== the recovered-team-news oracle, full grid.\n")
	fmt.Printf("Baseline is the shipped model, which sees statusAt's end-of-season\n")
	fmt.Printf("reconstruction. Positive means the oracle gains, so each mean is an\n")
	fmt.Printf("upper bound on what knowing the real team news could be worth.\n")
	fmt.Printf("POLICY is where the effect is expected: MinExpectedMinutes cliffs most\n")
	fmt.Printf("flagged players out of the opening squad the HOLD metric scores.\n")
	fmt.Printf("The two oracled arms are nested, so read their contrast as the value\n")
	fmt.Printf("of the published percentage alone, over and above the flag.\n")
	runPolicySweep(t, []policyVariant{
		{label: "real (ships)", apply: func(sc *SimConfig) {}},
		oracleVariant(Oracles{Info: OracleTeamNews, News: news},
			"real flags at each deadline", nil),
		oracleVariant(Oracles{
			Info: OracleTeamNews | OracleTeamNewsChance, News: news,
		}, "flags plus published chance", nil),
	}, sweepStarts())
}

// reportTeamNewsScope sizes what the oracle can see against what the reconstruction
// could, from the archive and the export alone.
//
// # Why this is printed before the headline
//
// The mediator is the difference between two findings that look identical in a
// table. An arm whose flags never differ from the reconstruction changes nothing and
// reports a clean null — "could not run". An arm that visibly re-flags thousands of
// player-gameweeks and still reports a null is "nothing to see", which is a result.
// Only this table separates them, and it separates them before an hours-long sweep
// rather than after.
//
// The `established` column is the subset that can change a decision at all: a player
// with under half a season of prior minutes is not in the opening pool and mostly not
// in the transfer search's reach either, so re-flagging him is free.
func reportTeamNewsScope(t *testing.T, news *TeamNewsTable) {
	t.Helper()
	cfg := loadConfig(t)

	fmt.Printf("\n=== coverage of the recovered captures, under the filter in force\n\n")
	fmt.Printf("%-9s %9s %9s %11s %14s %13s\n",
		"season", "captured", "kept", "dropped", "median hours", "flagged p-gws")
	for _, s := range news.Summary() {
		fmt.Printf("%-9s %9d %9d %11d %14.1f %13d\n",
			s.Season, s.Captured, s.Kept, s.Dropped, s.MedianHours, s.Flagged)
	}
	fmt.Printf("\n'kept' is gameweeks of 38 whose capture survived the staleness cut.\n")
	fmt.Printf("A season at zero is a season where the intervention could not run,\n")
	fmt.Printf("and its cells would be byte-identical while still reporting a mean.\n")

	fmt.Printf("\n=== what the recovered flag says that the reconstruction does not\n\n")
	fmt.Printf("%-9s %12s %12s %12s %12s %12s\n",
		"season", "compared", "news worse", "news better", "doubtful", "established")

	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		priorByCode := prior.ByCode()

		var compared, worse, better, doubtful, established int
		for gw := 1; gw <= 38; gw++ {
			if !news.Covers(cur.Name, gw) {
				continue
			}
			// The cutoff the reconstruction is evaluated at for this deadline: the
			// same expression PointInTimeWith uses, so the two are compared at the
			// same moment rather than at two moments that look alike.
			cutoff := gameweekStart(cur, gw)
			for _, id := range sortedPlayerIDs(cur) {
				p := cur.Players[id]
				got, _, ok := news.FlagAt(cur.Name, gw, p.Code)
				if !ok {
					continue
				}
				want := statusAt(p, gw, cutoff, Oracles{})
				compared++
				if got == want {
					continue
				}
				// "Worse" and "better" are from the model's point of view: a flag it
				// did not have is a player it would now avoid, and the reverse is a
				// player the reconstruction wrongly wrote off.
				if want == "a" {
					worse++
				} else {
					better++
				}
				if got == "d" {
					doubtful++
				}
				// Half a season of prior minutes is the threshold priors.ThinSeason
				// already uses for a record worth believing, and the same one the
				// availability oracle's scope report takes.
				if q := priorByCode[p.Code]; q != nil && q.Minutes >= 1710 {
					established++
				}
			}
		}
		fmt.Printf("%-9s %12d %12d %12d %12d %12d\n",
			cur.Name, compared, worse, better, doubtful, established)
	}

	fmt.Printf("\n'compared' is player-gameweeks where both sources have an opinion.\n")
	fmt.Printf("'news worse' is the population the reconstruction is blind to: FPL was\n")
	fmt.Printf("flagging him and the end-of-season snapshot cannot say so. 'doubtful'\n")
	fmt.Printf("is the state statusAt refuses to reconstruct at all. 'established' is\n")
	fmt.Printf("the subset with half a season of prior minutes, which is roughly the\n")
	fmt.Printf("population a decision could turn on.\n")
	fmt.Printf("\nAn arm that moves these counts and not the points below is a real\n")
	fmt.Printf("null; an arm that moves neither could not run.\n")
}
