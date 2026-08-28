package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"armband/internal/analysis"
	"armband/internal/config"
)

// cmdXPoints ranks the whole player pool by expected points for ONE gameweek.
//
// # What the number is
//
// The model's projected FPL points for that gameweek alone, scored on FPL's own
// points rules, for every player in the game rather than for a squad. It is a
// projection made BEFORE the gameweek, not a record of what was scored in it.
//
// # Why it is not `armband squad` with a bigger table
//
// Every other ranking in this binary scores over the configured horizon, which
// is the right question for a fifteen you keep and the wrong one for a single
// week: the horizon averages across fixtures, so a double gameweek reads as two
// ordinary weeks and a blank is diluted rather than empty. `Engine.PoolAt` scores
// at horizon 1, which is also the only horizon where fixture load reaches Score
// at all. See that method for why it is not built out of `WeekViews`.
//
// # ⚠️ The roster overrides are applied, and skipping that is silent
//
// `applyRoster` installs the minutes corrections and collects the exclude list.
// Omitting it does not error — it produces a plausible-looking ranking with
// excluded players still ranked and corrected players still on their
// uncorrected minutes. That is exactly how the first hand-built version of this
// ranking went wrong: a backup keeper appeared rankable, and four players who
// only score correctly once their overrides apply were missing from the table
// entirely. The excludes are dropped from the output rather than scored to
// zero, because a zero row reads as a prediction and an absent one does not.
func cmdXPoints(cfg config.Config, engine *analysis.Engine, args []string) error {
	opt, err := parseXPointsFlags(engine, args)
	if err != nil {
		return err
	}

	// ⚠️ Load-bearing — see the doc comment. req collects the excludes.
	var req analysis.OptimizeRequest
	for _, note := range applyRoster(cfg, engine, &req) {
		fmt.Fprintf(os.Stderr, "%s\n", dim(note))
	}
	excluded := map[int]bool{}
	for _, id := range req.ExcludeIDs {
		excluded[id] = true
	}

	rows := make([]analysis.PlayerMetrics, 0, len(engine.Boot.Elements))
	for _, m := range engine.PoolAt(opt.week) {
		if !excluded[m.ID] && opt.wanted[m.Position] {
			rows = append(rows, m)
		}
	}
	if len(rows) == 0 {
		return fmt.Errorf("no player in gameweek %d matches those filters", opt.week)
	}
	// One table per position when several are asked for, so -n means "this many
	// forwards AND this many midfielders" rather than "this many players, mostly
	// midfielders". A single position is one table, which is the same rule.
	groups := groupByPosition(rows, opt.wanted, opt.rank, opt.n)

	switch opt.format {
	case "table":
		return printXPointsTable(opt.week, groups)
	case "csv":
		return writeXPointsCSV(opt.week, groups)
	case "json":
		return json.NewEncoder(os.Stdout).Encode(xpointsPayload(opt.week, groups))
	}
	return fmt.Errorf("-format must be table, csv or json, got %q", opt.format)
}

// xpointsOpts is the resolved command line: every flag turned into the thing it
// selects, so nothing downstream re-reads a string to decide what to do.
type xpointsOpts struct {
	week   int
	wanted map[string]bool
	rank   func(a, b analysis.PlayerMetrics) bool
	n      int
	format string
}

// parseXPointsFlags resolves and validates the flags, and refuses rather than
// falling back. A -sort or -pos this command does not understand is a typo in a
// scheduled job, and the scheduled job's whole value is that nobody is watching
// it — so the wrong answer must be an error rather than a default ranking that
// looks like the one that was asked for.
//
// -format is checked at the point of use rather than here, where it would be a
// second place the list of formats is spelled.
func parseXPointsFlags(engine *analysis.Engine, args []string) (xpointsOpts, error) {
	fs := flag.NewFlagSet("xpoints", flag.ContinueOnError)
	gw := fs.Int("gw", 0, "gameweek to score (default: the next one open)")
	pos := fs.String("pos", "", "positions to include, comma separated: GKP,DEF,MID,FWD (default: all)")
	sortBy := fs.String("sort", "xp", "sort by: xp, value, price, minutes, name")
	n := fs.Int("n", 0, "how many players to print, per position when -pos names "+
		"more than one (default: all)")
	format := fs.String("format", "table", "output format: table, csv, json")
	if err := fs.Parse(args); err != nil {
		return xpointsOpts{}, err
	}

	week := *gw
	if week == 0 {
		next := engine.Boot.NextEvent()
		if next == nil {
			return xpointsOpts{}, fmt.Errorf("no gameweek is open, so there is nothing " +
				"to project; name one with -gw")
		}
		week = next.ID
	}
	if week < 1 || week > 38 {
		return xpointsOpts{}, fmt.Errorf("-gw must be 1-38, got %d", week)
	}
	wanted, err := wantedPositions(*pos)
	if err != nil {
		return xpointsOpts{}, err
	}
	rank, err := xpointsRanker(*sortBy)
	if err != nil {
		return xpointsOpts{}, err
	}
	if *n < 0 {
		return xpointsOpts{}, fmt.Errorf("-n cannot be negative, got %d", *n)
	}
	return xpointsOpts{week: week, wanted: wanted, rank: rank, n: *n, format: *format}, nil
}

// positionOrder is the order tables are printed in, which is FPL's own and not
// alphabetical. A map has no order, so the slice is what carries it.
var positionOrder = []string{"GKP", "DEF", "MID", "FWD"}

// wantedPositions turns the -pos flag into a set. Empty means all four.
func wantedPositions(pos string) (map[string]bool, error) {
	all := map[string]bool{}
	for _, p := range positionOrder {
		all[p] = true
	}
	if strings.TrimSpace(pos) == "" {
		return all, nil
	}
	want := map[string]bool{}
	for _, p := range strings.Split(pos, ",") {
		p = strings.ToUpper(strings.TrimSpace(p))
		if !all[p] {
			return nil, fmt.Errorf("-pos %q is not a position; use GKP, DEF, MID or FWD", p)
		}
		want[p] = true
	}
	return want, nil
}

// xpointsRanker is the -sort flag as a comparison.
//
// Every rule is a strict total order with the player id as its last key, so two
// players who tie on the sorted quantity — which is common on price and on a
// rounded minutes figure — come out in the same order on every run. Without
// that, a weekly image would reshuffle its own ties between two runs of the
// same command and read as a change in the model.
func xpointsRanker(by string) (func(a, b analysis.PlayerMetrics) bool, error) {
	switch by {
	case "xp":
		return func(a, b analysis.PlayerMetrics) bool {
			if a.Score != b.Score {
				return a.Score > b.Score
			}
			return a.ID < b.ID
		}, nil
	case "value":
		// Points per million. Priced in tenths everywhere else in this binary;
		// Price is already in millions, and a zero price cannot reach here
		// because FPL prices every player.
		return func(a, b analysis.PlayerMetrics) bool {
			av, bv := a.Score/a.Price, b.Score/b.Price
			if av != bv {
				return av > bv
			}
			return a.ID < b.ID
		}, nil
	case "price":
		return func(a, b analysis.PlayerMetrics) bool {
			if a.Price != b.Price {
				return a.Price > b.Price
			}
			return a.ID < b.ID
		}, nil
	case "minutes":
		return func(a, b analysis.PlayerMetrics) bool {
			if a.ExpectedMinutes != b.ExpectedMinutes {
				return a.ExpectedMinutes > b.ExpectedMinutes
			}
			return a.ID < b.ID
		}, nil
	case "name":
		return func(a, b analysis.PlayerMetrics) bool {
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.ID < b.ID
		}, nil
	}
	return nil, fmt.Errorf("-sort must be xp, value, price, minutes or name, got %q", by)
}

// xpointsGroup is one position's table.
type xpointsGroup struct {
	Position string                   `json:"position"`
	Players  []analysis.PlayerMetrics `json:"players"`
}

// groupByPosition splits, sorts and truncates. n of 0 keeps everything.
func groupByPosition(rows []analysis.PlayerMetrics, wanted map[string]bool,
	rank func(a, b analysis.PlayerMetrics) bool, n int) []xpointsGroup {

	var out []xpointsGroup
	for _, p := range positionOrder {
		if !wanted[p] {
			continue
		}
		var in []analysis.PlayerMetrics
		for _, m := range rows {
			if m.Position == p {
				in = append(in, m)
			}
		}
		if len(in) == 0 {
			continue
		}
		sort.Slice(in, func(i, j int) bool { return rank(in[i], in[j]) })
		if n > 0 && len(in) > n {
			in = in[:n]
		}
		out = append(out, xpointsGroup{Position: p, Players: in})
	}
	return out
}

// xpointsPayload is the -format json document. The gameweek is carried beside
// the rows rather than left to the caller to remember, because the whole point
// of this command is a file that gets read later by something that was not
// there when it ran.
func xpointsPayload(week int, groups []xpointsGroup) map[string]any {
	return map[string]any{"gameweek": week, "groups": groups}
}

func printXPointsTable(week int, groups []xpointsGroup) error {
	fmt.Printf("\nEXPECTED POINTS, GAMEWEEK %d\n", week)
	fmt.Printf("Projected FPL points for this gameweek alone, scored on FPL's own points\n")
	fmt.Printf("rules. A projection made before the gameweek, not what was scored in it.\n")
	for _, g := range groups {
		fmt.Printf("\n  %s\n", g.Position)
		fmt.Printf("  %4s %-22s %-5s %7s %7s %7s\n", "", "player", "club", "price", "xp", "mins")
		for i, m := range g.Players {
			fmt.Printf("  %4d %-22s %-5s %7.1f %7.2f %7.0f\n",
				i+1, truncateName(m.Name, 22), m.Team, m.Price, m.Score, m.ExpectedMinutes)
		}
	}
	fmt.Println()
	return nil
}

// truncateName keeps the columns aligned. Names longer than the column are rare
// and an ellipsis is more honest than a table that shifts one row wider.
func truncateName(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func writeXPointsCSV(week int, groups []xpointsGroup) error {
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"gameweek", "position", "rank", "id", "player",
		"club", "price_m", "xp", "expected_minutes"}); err != nil {
		return err
	}
	for _, g := range groups {
		for i, m := range g.Players {
			if err := w.Write([]string{
				strconv.Itoa(week), g.Position, strconv.Itoa(i + 1),
				strconv.Itoa(m.ID), m.Name, m.Team,
				strconv.FormatFloat(m.Price, 'f', 1, 64),
				strconv.FormatFloat(m.Score, 'f', 3, 64),
				strconv.FormatFloat(m.ExpectedMinutes, 'f', 1, 64),
			}); err != nil {
				return err
			}
		}
	}
	w.Flush()
	return w.Error()
}
