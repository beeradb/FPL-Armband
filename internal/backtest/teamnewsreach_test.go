package backtest

// How many of the re-flagged players could have changed a decision at all?
//
//	DIAG=1 go test ./internal/backtest -run '^TestDiagTeamNewsReach$' -v -timeout 60m
//
// # Why the headline needs this
//
// The team-news sweep reports what recovered availability is worth on two metrics.
// If it reports a null, there are two very different reasons it might, and the
// sweep's own output cannot tell them apart:
//
//   - the information reaches players nobody would have bought anyway, so the arm
//     is inert where it matters even though it re-flags thousands of
//     player-gameweeks;
//   - the information reaches buyable players and does not pay.
//
// `MinExpectedMinutes` is the reason to suspect the first. It cliffs the opening
// squad pool at 55 expected minutes a gameweek, and the record already notes that
// this removes 85-100% of flagged players from the population the held metric
// scores. So this counts the re-flagged players **on the model's own expected
// minutes**, which is the quantity the cliff is applied to.
//
// # It builds the engine through EngineAt, and that is not a detail
//
// A measurement was withdrawn for using `analysis.NewEngineFull` directly here.
// That constructor leaves `Recent` and `Priors` nil, so `ExpectedMinutes` falls
// back to the flat season-to-date mean and `blankRunFactor` — the discount for a
// player one to three gameweeks into an absence — never fires at all. The number
// still looks like expected minutes. `EngineAt` wires the engine the way `Simulate`
// wires it, which is the whole reason it is exported.

import (
	"fmt"
	"os"
	"testing"
)

func TestDiagTeamNewsReach(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	news, err := LoadTeamNews(teamNewsFilterFromEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(t)
	// The sweep's own entry points, read from the one place the grid is declared,
	// so this table and the sweep describe the same moments. Not every gameweek:
	// each row builds a fully wired engine, which is the expensive part of a replay
	// week, and the question is about the shape of the population rather than about
	// a season total.
	weeks := sweepStarts()

	fmt.Printf("\n=== can the recovered flag reach a decision?\n\n")
	fmt.Printf("Re-flagged means the recovered status differs from statusAt's\n")
	fmt.Printf("reconstruction at that deadline. 'in pool' is the subset the opening\n")
	fmt.Printf("squad could actually have bought: MinExpectedMinutes cliffs the pool\n")
	fmt.Printf("at 55 expected minutes a gameweek, measured on the fully wired engine.\n")
	fmt.Printf("The transfer search has no such floor, so the pool column bounds what\n")
	fmt.Printf("HOLD can see and understates what POLICY can.\n\n")

	fmt.Printf("%-9s %5s %12s %12s %10s %12s\n",
		"season", "gw", "re-flagged", "in pool", "of pool", "pool size")

	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])

		for _, gw := range weeks {
			if !news.Covers(cur.Name, gw) {
				continue
			}
			sc := sweepConfig(cfg, gw, false)
			// through = gw-1: everything up to the previous gameweek is known and
			// this deadline's is not, which is the convention EngineAt takes and the
			// one Simulate passes.
			e, boot := EngineAt(cur, prior, gw-1, sc)
			metrics := e.AllMetrics()

			// The cliff, resolved exactly as Simulate resolves it — zero means the
			// historical 55 and negative means no floor, so reading the raw field
			// would price an unset value as "no floor at all".
			floor := sc.MinExpectedMinutes
			switch {
			case floor == 0:
				floor = 55
			case floor < 0:
				floor = 0
			}

			var reflagged, inPool, pool int
			for _, m := range metrics {
				if m.ExpectedMinutes >= floor {
					pool++
				}
				el := boot.ElementByID(m.ID)
				if el == nil {
					continue
				}
				got, _, ok := news.FlagAt(cur.Name, gw, el.Code)
				if !ok || got == el.Status {
					continue
				}
				reflagged++
				if m.ExpectedMinutes >= floor {
					inPool++
				}
			}
			share := 0.0
			if reflagged > 0 {
				share = float64(inPool) / float64(reflagged)
			}
			fmt.Printf("%-9s %5d %12d %12d %9.1f%% %12d\n",
				cur.Name, gw, reflagged, inPool, 100*share, pool)
		}
	}

	fmt.Printf("\nA low 'of pool' share says the information mostly reaches players the\n")
	fmt.Printf("opening squad could not have bought, so a null on HOLD is the cliff\n")
	fmt.Printf("rather than the news. A high one says the news reaches buyable\n")
	fmt.Printf("players, and a null then means the news does not pay.\n")
}
