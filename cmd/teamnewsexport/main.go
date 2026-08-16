// Command teamnewsexport turns the capture store into the two small tables the
// replay reads.
//
// # Why an export step rather than a direct dependency
//
// `internal/backfill`'s TestTheScoringPathCannotSeeRecoveredTeamNews forbids
// `internal/backtest` importing `internal/capture`, and it is right to: every
// figure in the research record was measured against `statusAt`'s end-of-season
// reconstruction, and improving that input silently would leave half the record
// incomparable with the other half.
//
// So this is the same shape as the expected-goals backfill, which is the closest
// precedent in the repository and the one the brief for this work named: an
// external producer writes a small, auditable data file, `internal/backtest`
// embeds it, and a table in the consumer says exactly which seasons it applies
// to. `stats/understat_xg_backfill.py` produces `internal/backtest/repairdata`;
// this produces `internal/backtest/teamnewsdata`.
//
// # What it writes, and why two files
//
// **coverage.csv** is one row per season-gameweek that a capture exists for,
// carrying how close to the deadline it was read and how many deadlines had
// already passed when it was crawled. It is the authority on what is known: a
// gameweek absent from it is a gap, and the replay falls back to the
// reconstruction there rather than inventing availability. Both staleness columns
// travel with the data because a stale flag attenuates toward the unflagged
// population, which is a confound rather than noise.
//
// **flags.csv** is one row per player FPL was saying anything about — a status
// other than "a", or a published chance of playing. Everyone else is available
// with no percentage, which is the normal state of a fit player and is implied by
// the gameweek being covered at all. That keeps the export at a few tens of
// thousands of rows instead of a few hundred thousand.
//
// Keyed on permanent player **code** throughout. Element ids are reassigned every
// summer, so a record keyed on one comes back next season attached to a different
// footballer.
//
// Usage:
//
//	go run ./cmd/teamnewsexport -captures data/captures -out internal/backtest/teamnewsdata
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"armband/internal/capture"
)

func main() {
	captures := flag.String("captures", "data/captures", "the captures root")
	out := flag.String("out", "internal/backtest/teamnewsdata", "where to write the tables")
	flag.Parse()

	if err := run(*captures, *out); err != nil {
		fmt.Fprintln(os.Stderr, "teamnewsexport:", err)
		os.Exit(1)
	}
}

func run(capturesRoot, outDir string) error {
	store, err := capture.Open(capturesRoot)
	if err != nil {
		return err
	}
	seasons := store.Seasons()
	if len(seasons) == 0 {
		// Silence must not read as success: an empty export would make every arm
		// built on it inert, and an inert oracle reports the same clean null as a
		// real one.
		return fmt.Errorf("no backfilled seasons under %s — an empty export would "+
			"produce an oracle that changes nothing and reports it as a null",
			capturesRoot)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	covFile, err := os.Create(filepath.Join(outDir, "coverage.csv"))
	if err != nil {
		return err
	}
	defer covFile.Close()
	flagFile, err := os.Create(filepath.Join(outDir, "flags.csv"))
	if err != nil {
		return err
	}
	defer flagFile.Close()

	cov := csv.NewWriter(covFile)
	flg := csv.NewWriter(flagFile)
	defer cov.Flush()
	defer flg.Flush()

	if err := cov.Write([]string{
		"season", "gw", "captured_at", "hours_before", "events_behind",
		"backfilled", "players", "flagged",
	}); err != nil {
		return err
	}
	if err := flg.Write([]string{"season", "gw", "code", "status", "chance"}); err != nil {
		return err
	}

	for _, season := range seasons {
		covered, flagged := 0, 0
		for gw := 1; gw <= 38; gw++ {
			a, err := store.At(season, gw)
			if err != nil {
				continue // a genuine gap; coverage.csv records absence by omission
			}
			covered++
			codes := make([]int, 0, len(a.Players))
			for code := range a.Players {
				codes = append(codes, code)
			}
			// Sorted, so the export is byte-identical between runs. A file that
			// changes on every regeneration cannot be reviewed as a diff, and this
			// project has already had a diagnostic whose figures moved run to run
			// because it ranged a map.
			sort.Ints(codes)

			n := 0
			for _, code := range codes {
				p := a.Players[code]
				if p.Status == "a" && p.ChanceNext == nil {
					continue
				}
				chance := ""
				if p.ChanceNext != nil {
					chance = strconv.Itoa(*p.ChanceNext)
				}
				if err := flg.Write([]string{
					season, strconv.Itoa(gw), strconv.Itoa(code), p.Status, chance,
				}); err != nil {
					return err
				}
				n++
			}
			flagged += n
			if err := cov.Write([]string{
				season, strconv.Itoa(gw), a.SnapshotAt.UTC().Format("2006-01-02T15:04Z"),
				strconv.FormatFloat(a.HoursBefore, 'f', 1, 64),
				strconv.Itoa(a.EventsBehind),
				strconv.FormatBool(a.Backfilled),
				strconv.Itoa(len(a.Players)), strconv.Itoa(n),
			}); err != nil {
				return err
			}
		}
		fmt.Printf("%-9s %2d/38 gameweeks covered, %6d flagged player-gameweeks\n",
			season, covered, flagged)
	}
	cov.Flush()
	flg.Flush()
	if err := cov.Error(); err != nil {
		return err
	}
	return flg.Error()
}
