package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"armband/internal/backfill"
	"armband/internal/backtest"
	"armband/internal/capture"
	"armband/internal/config"
	"armband/internal/wayback"
)

// BackfillSeasons are the seasons the Internet Archive can actually support.
//
// **Asserted from a coverage check of the CDX index, not measured against anything.**
// From 2020-21 the Archive holds crawls of FPL's `/api/bootstrap-static/` on between
// two days in three and nearly every day. Before that FPL served `/drf/` paths, which
// have of the order of ten captures a year — far too sparse to place against 38
// deadlines, so those seasons are not offered rather than offered and mostly empty.
var BackfillSeasons = []string{"2020-21", "2021-22", "2022-23", "2023-24", "2024-25", "2025-26"}

// cmdBackfill recovers historical team news from the Internet Archive.
//
// # Why a separate command and not part of `capture`
//
// `capture` records the present and must stay boring: it is meant to run on a timer
// and take one payload. This reaches out to a third party's index, makes a few
// hundred requests, and is run once per season and then never again. Sharing a
// command would mean a scheduled capture could accidentally start a twenty-minute
// crawl of somebody else's infrastructure.
func cmdBackfill(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	dir := fs.String("dir", DefaultCaptureDir, "the captures root to write into")
	perGW := fs.Int("per-gameweek", 1, "how many crawls to keep per deadline; 1 is the "+
		"decision-relevant cadence, higher is for the availability-trajectory question")
	coverageOnly := fs.Bool("coverage", false, "report what is on disk and fetch nothing")
	refresh := fs.Bool("refresh-index", false, "re-query the Archive's index instead of "+
		"reading the cached one")
	interval := fs.Duration("min-interval", 1500*time.Millisecond, "minimum gap between "+
		"requests to the Internet Archive")
	show := fs.Int("show", 0, "print the availability recovered for this gameweek and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	seasons := fs.Args()
	if len(seasons) == 0 {
		return fmt.Errorf("backfill needs a season, e.g. `armband backfill 2020-21`, or "+
			"`all` for %s", strings.Join(BackfillSeasons, ", "))
	}
	if len(seasons) == 1 && seasons[0] == "all" {
		seasons = BackfillSeasons
	}

	if *show > 0 {
		if len(seasons) != 1 {
			return fmt.Errorf("-show takes one season")
		}
		return showAvailability(*dir, seasons[0], *show)
	}

	client := wayback.New(cfg.CacheDir)
	client.MinInterval = *interval

	for _, season := range seasons {
		if *coverageOnly {
			if err := reportCoverage(*dir, season); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", season, err)
			}
			continue
		}
		if err := backfillSeason(ctx, cfg, client, *dir, season, *perGW, *refresh); err != nil {
			// One season failing does not abort the others: they are independent
			// bodies of evidence, and a season the Archive cannot support is a
			// fact to record rather than a reason to abandon the run.
			fmt.Fprintf(os.Stderr, "\n%s FAILED: %v\n", season, err)
		}
	}
	return nil
}

func backfillSeason(ctx context.Context, cfg config.Config, client *wayback.Client,
	dir, season string, perGW int, refresh bool) error {

	fmt.Printf("\n=== %s ===\n", season)

	// The season archive first, because its kickoff times are what the recovered
	// calendar gets checked against — and because picking the crawl to read the
	// calendar out of needs to know when the season's football actually happened.
	fmt.Fprintf(os.Stderr, "loading the season archive for %s...\n", season)
	s, err := backtest.Load(ctx, cfg.CacheDir, season)
	if err != nil {
		return fmt.Errorf("loading the season archive: %w", err)
	}
	kickoffs := firstKickoffs(s)
	if len(kickoffs) == 0 {
		return fmt.Errorf("the season archive for %s carries no kickoff times, so the "+
			"recovered calendar could not be checked against anything", season)
	}

	res, err := backfill.Run(ctx, client, backfill.Options{
		Root: dir, Season: season, PerGameweek: perGW,
		Kickoffs: kickoffs, Version: buildRevision(), RefreshIndex: refresh,
		Log: func(msg string) { fmt.Fprintf(os.Stderr, "  %s\n", msg) },
	})
	if err != nil {
		return err
	}
	printResult(res)
	return nil
}

// firstKickoffs is the earliest kickoff the season archive records per gameweek.
//
// The archive has no deadline field — deadlines live only in `bootstrap-static`,
// which is the thing being recovered — so kickoffs are the independent witness
// available. FPL sets a deadline ninety minutes before its gameweek's first fixture,
// which is what makes the comparison meaningful rather than merely adjacent.
func firstKickoffs(s *backtest.Season) map[int]time.Time {
	out := map[int]time.Time{}
	for _, f := range s.Fixtures {
		if f.Event == nil || f.KickoffTime == nil {
			continue
		}
		gw := *f.Event
		k := f.KickoffTime.UTC()
		if cur, ok := out[gw]; !ok || k.Before(cur) {
			out[gw] = k
		}
	}
	return out
}

func printResult(r *backfill.Result) {
	if r.Deadlines != nil {
		c := r.Deadlines.Check
		fmt.Printf("calendar: %d gameweeks, cross-checked against %d archive kickoffs, "+
			"median deadline-to-first-kickoff %.2f h, %d inverted\n",
			len(r.Deadlines.By), c.Compared, c.MedianGapHours, c.Inverted)
	}
	if r.Stored > 0 {
		fmt.Printf("stored %d capture(s) this run, %.1f MB\n", r.Stored, float64(r.Bytes)/(1<<20))
	}
	printCoverageTable(r.Rows)
	fmt.Printf("coverage: %d of 38 gameweeks (%.0f%%), of which %d are from a crawl inside "+
		"the gameweek's own run-up\n", r.Covered, r.CoveragePct(), r.Fresh)
	printFreshness(r.Rows)
	printGaps(r.Rows)
}

// printFreshness says how much of the coverage is decision-relevant.
//
// Coverage and usefulness are different quantities and the first flatters the second.
// A capture nine days before a deadline is honest — it cannot contain team news the
// manager did not have — and it is *last week's* team news, which is not what anyone
// wants to calibrate an injury flag against. Reporting the headline percentage alone
// would be the same defect this project fixed in the sweep harness: a number that is
// true and reads as better than it is.
func printFreshness(rows []backfill.Row) {
	var day, threeDays, stale int
	for _, r := range rows {
		if !r.Found {
			continue
		}
		switch {
		case r.HoursBefore <= 24:
			day++
		case r.HoursBefore <= 72:
			threeDays++
		default:
			stale++
		}
	}
	fmt.Printf("freshness: %d within a day of the deadline, %d within three days, %d older\n",
		day, threeDays, stale)
	if stale > 0 {
		fmt.Printf("  The %d older one(s) are honest — nothing after a deadline is ever "+
			"stored — but they carry the team news of a previous week, so treat them as "+
			"weak evidence rather than as coverage.\n", stale)
	}
}

// printCoverageTable prints one line per gameweek, present or not.
//
// Every gameweek gets a line whether or not it was found, because a table listing
// only what exists makes a two-thirds-covered season look complete. That is the same
// discipline the sweep harness applies to an infeasible cell, and it is the reason
// this data can be trusted at all: a gap that is reported is a gap somebody can
// reason about, and a gap that is silently filled is a measurement of nothing.
func printCoverageTable(rows []backfill.Row) {
	fmt.Printf("\n  GW  deadline           crawl              hours before  behind\n")
	for _, r := range rows {
		dl := "        —"
		if !r.Deadline.IsZero() {
			dl = r.Deadline.Format("2006-01-02 15:04")
		}
		if !r.Found {
			fmt.Printf("  %2d  %s  %-17s  %12s  %6s\n", r.Event, dl, "GAP", "—", "—")
			continue
		}
		behind := "—"
		if r.EventsBehind >= 0 {
			behind = fmt.Sprintf("%d", r.EventsBehind)
		}
		flag := ""
		if r.EventsBehind > 0 {
			flag = "  <- predates an earlier deadline"
		}
		// Two decimals, not one: a genuine capture 51 seconds before a deadline
		// reads as "0.0" at one decimal, which is the one value this store refuses
		// to hold. The tightest honest row must not display as a leak.
		fmt.Printf("  %2d  %s  %s  %12.2f  %6s%s\n", r.Event, dl,
			r.At.Format("2006-01-02 15:04"), r.HoursBefore, behind, flag)
	}
	fmt.Println()
}

func printGaps(rows []backfill.Row) {
	var gaps []string
	for _, r := range rows {
		if !r.Found {
			gaps = append(gaps, fmt.Sprintf("GW%d", r.Event))
		}
	}
	if len(gaps) == 0 {
		return
	}
	fmt.Printf("gaps (%d): %s\n", len(gaps), strings.Join(gaps, " "))
	fmt.Printf("  These gameweeks have no pre-deadline crawl and must be reported as " +
		"missing. Nothing fills them: the nearest crawl after a deadline carries team " +
		"news the manager did not have.\n")
}

func reportCoverage(dir, season string) error {
	store, err := capture.Open(dir)
	if err != nil {
		return err
	}
	// The same table builder the fetch path uses, so the two reports cannot come to
	// differ about what is on disk.
	dl, _ := backfill.LoadDeadlines(dir, season)
	out := backfill.Rows(store, season, dl)
	covered := 0
	for _, r := range out {
		if r.Found {
			covered++
		}
	}
	fmt.Printf("\n=== %s ===\n", season)
	printCoverageTable(out)
	fmt.Printf("coverage: %d of 38 gameweeks (%.0f%%)\n", covered, 100*float64(covered)/38)
	printFreshness(out)
	printGaps(out)
	return nil
}

// showAvailability exercises the read API: what FPL was saying about every player at
// one gameweek's deadline.
//
// It prints the distribution and then the flagged players, which is the shape the
// data is actually for — the interesting population is the few dozen with news, not
// the four hundred who were fine.
func showAvailability(dir, season string, gw int) error {
	store, err := capture.Open(dir)
	if err != nil {
		return err
	}
	a, err := store.At(season, gw)
	if err != nil {
		return err
	}
	fmt.Printf("%s GW%d — deadline %s, crawl %s (%.1f h before), backfilled=%v\n\n",
		season, gw, a.Deadline.Format("2006-01-02 15:04Z"),
		a.SnapshotAt.Format("2006-01-02 15:04Z"), a.HoursBefore, a.Backfilled)

	byStatus := map[string]int{}
	byChance := map[string]int{}
	var flagged []capture.PlayerStatus
	for _, p := range a.Players {
		byStatus[p.Status]++
		if p.ChanceNext == nil {
			byChance["none published"]++
		} else {
			byChance[fmt.Sprintf("%d%%", *p.ChanceNext)]++
		}
		if p.Status != "a" {
			flagged = append(flagged, p)
		}
	}
	fmt.Printf("%d players, keyed by permanent code\n", len(a.Players))
	fmt.Printf("status:  %s\n", counts(byStatus))
	fmt.Printf("chance of playing next round:  %s\n\n", counts(byChance))

	sort.Slice(flagged, func(i, j int) bool { return flagged[i].WebName < flagged[j].WebName })
	fmt.Printf("%d players carrying a flag:\n", len(flagged))
	for _, p := range flagged {
		chance := "—"
		if p.ChanceNext != nil {
			chance = fmt.Sprintf("%d%%", *p.ChanceNext)
		}
		news := p.News
		if len(news) > 58 {
			news = news[:55] + "..."
		}
		fmt.Printf("  code %-7d %-18s %s  %-5s  %s\n", p.Code, p.WebName, p.Status, chance, news)
	}
	return nil
}

func counts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, "  ")
}
