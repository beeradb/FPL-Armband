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

	// Default to the next gameweek that still has a fixture this command can
	// show. Doing this rather than "the current event" because a gameweek FPL
	// still calls current can be entirely played out.
	//
	// ⚠️ It scans on `showable`, the same predicate the rows are built with. An
	// earlier version scanned on a LOOSER one that ignored a missing kick-off
	// time, so a gameweek holding nothing but a rearranged TBC fixture could be
	// chosen and then render zero rows — the command going silent at exactly
	// the moment it had a later gameweek's worth of fixtures to print. Two
	// predicates for one question is how that happens; there is now one.
	if *week == 0 && *date == "" {
		best := 0
		for _, f := range e.Fixtures {
			if !showable(f) || f.Event == nil {
				continue
			}
			if best == 0 || *f.Event < best {
				best = *f.Event
			}
		}
		*week = best
	}

	type row struct {
		Gameweek int       `json:"gameweek"`
		Kickoff  time.Time `json:"kickoff"`
		Home     string    `json:"home"`
		Away     string    `json:"away"`
		HomeName string    `json:"home_name"`
		AwayName string    `json:"away_name"`
		// GoalsFor/GoalsAgainst are stated from the HOME side, so the away
		// side's for is the home side's against. Spelling both out rather than
		// making a reader invert it.
		HomeGoals float64 `json:"home_goals"`
		AwayGoals float64 `json:"away_goals"`
		// Played is the smaller of the two clubs' finished-match counts: how
		// much of this projection is the season rather than the prior.
		Played int `json:"played"`
	}

	name := map[int]string{}
	short := map[int]string{}
	for i := range e.Boot.Teams {
		t := &e.Boot.Teams[i]
		name[t.ID], short[t.ID] = t.Name, t.ShortName
	}

	var rows []row
	for _, f := range e.Fixtures {
		if !showable(f) || f.Event == nil {
			continue
		}
		if *week != 0 && *f.Event != *week {
			continue
		}
		if *date != "" && f.KickoffTime.In(uk).Format("2006-01-02") != *date {
			continue
		}
		h, a := e.FixtureGoals(f.TeamH, f.TeamA)
		played := e.TeamRatesFor(f.TeamH).Played
		if p := e.TeamRatesFor(f.TeamA).Played; p < played {
			played = p
		}
		rows = append(rows, row{
			Gameweek: *f.Event, Kickoff: f.KickoffTime.In(uk),
			Home: short[f.TeamH], Away: short[f.TeamA],
			HomeName: name[f.TeamH], AwayName: name[f.TeamA],
			HomeGoals: h, AwayGoals: a, Played: played,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].Kickoff.Equal(rows[j].Kickoff) {
			return rows[i].Kickoff.Before(rows[j].Kickoff)
		}
		return rows[i].Home < rows[j].Home
	})

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	if len(rows) == 0 {
		fmt.Println("\nNo unfinished fixtures match that selection.")
		return nil
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
	return nil
}

// showable reports whether a fixture can appear in a forecast at all: still to
// be played, and with a kick-off time to sort and group it by. A rearranged
// fixture with a TBC date has no time yet and is not showable — it is not
// missing from the season, only from this card, and it reappears the moment FPL
// sets a date.
//
// ⚠️ Both the row filter and the default-gameweek scan go through this. They
// are one question and must not drift apart; see cmdForecast's comment for what
// happened when they did.
func showable(f fpl.Fixture) bool {
	return !f.Finished && f.KickoffTime != nil
}
