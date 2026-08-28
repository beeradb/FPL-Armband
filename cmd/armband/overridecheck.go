package main

import (
	"context"
	"flag"
	"fmt"
	"sort"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// cmdOverrideCheck asks whether the model still needs the minutes overrides a
// human wrote for it.
//
// # Why this is the loop closing rather than a new question
//
// The shipped config carries hand-written minutes overrides, and they exist
// because the model said a player would play nothing. Several say so outright —
// "scores 0.00 only because he has no Premier League minutes", "the market is
// right and the model cannot see why". They are a person supplying, by hand and
// from public sources, information the model had no channel for.
//
// That was traced to a real defect: before the season starts, a player with no
// prior returned from the blend at EXACTLY zero expected minutes, never reaching
// the league fallback the in-season path uses. Across six seasons that was 122 to
// 284 players a season.
//
// So the fix has an obvious test, and it is this one: **for each override, what
// does the model say on its own now?** If the natural estimate has moved close to
// what the human set, the override is doing little and can be retired. If it has
// not, the fix does not capture what the person saw, and that is worth knowing
// before anyone trusts it.
//
// ⚠️ **This does not decide whether an override is RIGHT.** A human watching
// pre-season friendlies knows things no model does, and `natural` agreeing with
// `override` is not proof either is correct — only that they agree. What it can
// show is where the override is now redundant.
//
// ⚠️ **`natural` is the model with THIS player's own override suppressed**, via
// NaturalMetrics, not the model with all overrides off. Every other override
// still applies, which is the right comparison: the question is whether THIS one
// is still carrying weight.
func cmdOverrideCheck(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine, args []string) error {

	fs := flag.NewFlagSet("overrides", flag.ContinueOnError)
	within := fs.Float64("within", 10, "minutes tolerance below which an override is "+
		"reported as redundant")
	if err := fs.Parse(args); err != nil {
		return err
	}

	type row struct {
		name             string
		override, actual float64
		gap              float64
		note             string
	}
	var rows []row

	for _, o := range cfg.Roster.Minutes {
		if o.ExpectedMinutes == nil {
			continue // a lock or exclusion, not a minutes correction
		}
		// ⚠️ Resolved by permanent CODE, never by element id or name. Ids are
		// reassigned every summer and a name match is a guess; the override
		// carries the code for exactly this reason.
		var el *fpl.Element
		for i := range e.Boot.Elements {
			if e.Boot.Elements[i].Code == o.Code {
				el = &e.Boot.Elements[i]
				break
			}
		}
		if el == nil {
			rows = append(rows, row{name: o.Name, override: *o.ExpectedMinutes,
				note: "code not in the live bootstrap — he may have left the league"})
			continue
		}
		nat := e.NaturalMetrics(el).ExpectedMinutes
		rows = append(rows, row{
			name:     el.WebName,
			override: *o.ExpectedMinutes,
			actual:   nat,
			gap:      nat - *o.ExpectedMinutes,
		})
	}
	if len(rows) == 0 {
		fmt.Println("\nNo minutes overrides are set, so there is nothing to check.")
		return nil
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })

	fmt.Printf("\nDO THE MINUTES OVERRIDES STILL EARN THEIR KEEP?\n")
	fmt.Printf("`natural` is what the model says with THAT player's override\n")
	fmt.Printf("suppressed and every other one left in place.\n")
	fmt.Printf("⚠️ Agreement does not mean either is right — only that the override\n")
	fmt.Printf("is no longer changing the answer.\n\n")
	fmt.Printf("  %-18s %9s %9s %8s  %s\n", "player", "override", "natural", "gap", "")

	redundant := 0
	for _, r := range rows {
		if r.note != "" {
			fmt.Printf("  %-18s %9.0f %9s %8s  %s\n", r.name, r.override, "-", "-", r.note)
			continue
		}
		verdict := ""
		if r.gap < *within && r.gap > -*within {
			verdict = "redundant — the model now agrees"
			redundant++
		} else if r.gap < 0 {
			verdict = "still lifting him"
		} else {
			verdict = "still holding him down"
		}
		fmt.Printf("  %-18s %9.0f %9.1f %+8.1f  %s\n",
			r.name, r.override, r.actual, r.gap, verdict)
	}
	fmt.Printf("\n  %d of %d are within %.0f minutes of what the model says by itself.\n",
		redundant, len(rows), *within)
	fmt.Printf("⚠️ A redundant override is not automatically safe to delete: it may be\n")
	fmt.Printf("holding the estimate where it is. Re-check after removing one.\n")
	return nil
}
