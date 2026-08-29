package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// forecastRow is one fixture's projection, and the shape of the -json output.
type forecastRow struct {
	Gameweek int       `json:"gameweek"`
	Kickoff  time.Time `json:"kickoff"`
	Home     string    `json:"home"`
	Away     string    `json:"away"`
	HomeName string    `json:"home_name"`
	AwayName string    `json:"away_name"`
	// Stated from the HOME side, so the away side's "for" is the home side's
	// "against". Spelled out both ways rather than making a reader invert one.
	HomeGoals float64 `json:"home_goals"`
	AwayGoals float64 `json:"away_goals"`
	// Played is the SMALLER of the two clubs' finished-match counts: how much of
	// this projection rests on the season rather than on the pre-season prior.
	// The smaller, because that is the weaker of the two legs.
	Played int `json:"played"`
}

// cmdForecast prints projected goals for and against, per fixture.
//
// `armband fixtures` answers "who has the easy run", on FPL's integer 1-5
// difficulty. This answers a different question — how many goals each side is
// projected to score in one specific match — and it is the magnitude that
// integer cannot express: the span across clubs is a factor of about 2.4.
//
// ⚠️ **Projected goals, not xG.** See Engine.FixtureGoals: nothing here reads
// FPL's expected_goals. Naming the output "xG" would name a different quantity.
func cmdForecast(e *analysis.Engine, args []string) error {
	fs := flag.NewFlagSet("forecast", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	date := fs.String("date", "", "only fixtures kicking off on this UK date (YYYY-MM-DD)")
	week := fs.Int("gameweek", 0, "only this gameweek (default: the next one with fixtures left)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	uk, err := time.LoadLocation("Europe/London")
	if err != nil {
		return fmt.Errorf("loading Europe/London: %w", err)
	}

	if *week == 0 && *date == "" {
		*week = nextForecastableGameweek(e)
	}
	rows := forecastRows(e, uk, *week, *date)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	printForecast(rows)
	return nil
}

// nextForecastableGameweek is the earliest gameweek this command can actually
// render something for.
//
// ⚠️ It scans on `showable`, the same predicate the rows are built with. An
// earlier version scanned on a LOOSER one that ignored a missing kick-off time,
// so a gameweek holding nothing but a rearranged TBC fixture could be chosen
// and then render zero rows — the command going silent at exactly the moment it
// had a later gameweek's worth of fixtures to print. Two predicates for one
// question is how that happens; there is now one.
//
// Deliberately not "the current event": a gameweek FPL still calls current can
// be entirely played out.
func nextForecastableGameweek(e *analysis.Engine) int {
	best := 0
	for _, f := range e.Fixtures {
		if !showable(f) || f.Event == nil {
			continue
		}
		if best == 0 || *f.Event < best {
			best = *f.Event
		}
	}
	return best
}

// forecastRows projects every fixture matching the selection, in kick-off order.
// A zero week and an empty date each mean "do not filter on this".
func forecastRows(e *analysis.Engine, uk *time.Location,
	week int, date string) []forecastRow {

	name := map[int]string{}
	short := map[int]string{}
	for i := range e.Boot.Teams {
		t := &e.Boot.Teams[i]
		name[t.ID], short[t.ID] = t.Name, t.ShortName
	}

	var rows []forecastRow
	for _, f := range e.Fixtures {
		if !showable(f) || f.Event == nil {
			continue
		}
		if week != 0 && *f.Event != week {
			continue
		}
		if date != "" && f.KickoffTime.In(uk).Format("2006-01-02") != date {
			continue
		}
		h, a := e.FixtureGoals(f.TeamH, f.TeamA)
		rows = append(rows, forecastRow{
			Gameweek: *f.Event, Kickoff: f.KickoffTime.In(uk),
			Home: short[f.TeamH], Away: short[f.TeamA],
			HomeName: name[f.TeamH], AwayName: name[f.TeamA],
			HomeGoals: h, AwayGoals: a,
			Played: playedBetween(e, f.TeamH, f.TeamA),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].Kickoff.Equal(rows[j].Kickoff) {
			return rows[i].Kickoff.Before(rows[j].Kickoff)
		}
		return rows[i].Home < rows[j].Home
	})
	return rows
}

// playedBetween is the weaker of the two clubs' evidence: a projection is only
// as grounded as its less-played side.
func playedBetween(e *analysis.Engine, home, away int) int {
	h, a := e.TeamRatesFor(home).Played, e.TeamRatesFor(away).Played
	if a < h {
		return a
	}
	return h
}

func printForecast(rows []forecastRow) {
	if len(rows) == 0 {
		fmt.Println("\nNo unfinished fixtures match that selection.")
		return
	}
	fmt.Printf("\nProjected goals per fixture (not xG — see `armband forecast -h`)\n\n")
	day := ""
	for _, r := range rows {
		if d := r.Kickoff.Format("Mon 2 Jan"); d != day {
			day = d
			fmt.Printf("  %s\n", day)
		}
		fmt.Printf("    %s  %-4s %.2f - %.2f %-4s   %s\n",
			r.Kickoff.Format("15:04"), r.Home, r.HomeGoals, r.AwayGoals, r.Away,
			dim(fmt.Sprintf("(%d played)", r.Played)))
	}
}

// showable reports whether a fixture can appear in a forecast at all: still to
// be played, and with a kick-off time to sort and group it by. A rearranged
// fixture with a TBC date has no time yet and is not showable — it is not
// missing from the season, only from this card, and it reappears the moment FPL
// sets a date.
//
// ⚠️ Both the row filter and the default-gameweek scan go through this. They are
// one question and must not drift apart; see nextForecastableGameweek for what
// happened when they did.
func showable(f fpl.Fixture) bool {
	return !f.Finished && f.KickoffTime != nil
}
