package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"armband/internal/capture"
	"armband/internal/config"
	"armband/internal/fpl"
)

// DefaultCaptureDir is where captures land, relative to the working directory.
//
// Inside the repository rather than under the user's cache directory, and
// deliberately not gitignored. The reasoning is durability: this repository has no
// remote, so a worktree can be deleted along with the session that made it, and an
// uncommitted capture is then gone permanently. Committing is the only durable
// storage available — which is also why the bodies are gzipped, at roughly 200 KB a
// week against a raw megabyte.
const DefaultCaptureDir = "data/captures"

// cmdCapture archives the live payload, dated and immutable.
//
// # Why this is a command and not a side effect of every run
//
// The temptation is to have `brief` or `review` archive whatever it fetched, which
// would need no scheduling at all. It is wrong for a reason the research record
// keeps arriving at from new directions: those commands run when somebody feels like
// running them, so the resulting series would be irregular, would cluster around
// interesting weeks, and would silently be missing exactly the quiet weeks that make
// a control group. A capture wants to be boring and on a timer.
//
// It is also why this does not read the process's already-fetched bootstrap. That
// data may have come from the cache, in which case it dates the capture to whenever
// the cache was written, and the entire value of a capture is the moment.
// captureFlags parses capture's own flags from the arguments after the subcommand.
//
// A dedicated FlagSet rather than more globals, because Go's `flag` stops at the
// first non-flag argument: with globals, `armband capture -list` silently ignores
// `-list` and takes a capture instead. That is the worst possible shape here — the
// user asked to *inspect* and got a write. Found by running it.
func captureFlags(args []string) (dir string, list bool, err error) {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	d := fs.String("dir", DefaultCaptureDir, "where captures are written")
	l := fs.Bool("list", false, "list existing captures and the gaps between them; capture nothing")
	if err := fs.Parse(args); err != nil {
		return "", false, err
	}
	return *d, *l, nil
}

func cmdCapture(ctx context.Context, cfg config.Config, client *fpl.Client, args []string) error {
	dir, list, err := captureFlags(args)
	if err != nil {
		return err
	}
	if dir == "" {
		dir = DefaultCaptureDir
	}

	if list {
		return listCaptures(dir)
	}

	at := time.Now()
	path, m, err := capture.Take(ctx, client, dir, at, buildRevision())
	if err != nil {
		return err
	}

	fmt.Printf("Captured %s\n", filepath.Base(path))
	var raw, stored int
	for _, f := range m.Files {
		if f.Note != "" {
			fmt.Printf("  %-24s FAILED: %s\n", f.Endpoint, f.Note)
			continue
		}
		raw += f.Bytes
		stored += f.Stored
		fmt.Printf("  %-24s %7.1f KB -> %6.1f KB  %s\n",
			f.Endpoint, float64(f.Bytes)/1024, float64(f.Stored)/1024, f.SHA256[:12])
	}
	if raw > 0 {
		fmt.Printf("  %-24s %7.1f KB -> %6.1f KB\n", "total",
			float64(raw)/1024, float64(stored)/1024)
	}

	// The deadline line is the point of capturing at all, so it is printed rather
	// than left in the manifest. A capture taken 200 hours out records a very
	// different information state from one taken six hours out, and only the second
	// answers "what did we know when the decision was taken".
	if m.HoursToDeadline != nil {
		h := *m.HoursToDeadline
		switch {
		case h < 0:
			fmt.Printf("  GW%d deadline passed %.1f h ago\n", m.Event, -h)
		case h <= cfg.Review.LeadHours:
			fmt.Printf("  GW%d deadline in %.1f h — inside the lead window, so this is a "+
				"decision-moment capture\n", m.Event, h)
		default:
			fmt.Printf("  GW%d deadline in %.1f h\n", m.Event, h)
		}
	}

	// Say what is owed, because this repository has no remote and the whole point is
	// that the file survives.
	fmt.Printf("\nCommit it — %s is not gitignored, and an uncommitted capture dies with "+
		"the worktree.\n", dir)
	return nil
}

// buildRevision is the commit this binary was built from, when the toolchain
// stamped one.
//
// Best-effort by design. It is recorded so a capture can be tied to the code that
// took it, the way an accuracy snapshot is — but a capture's value does not depend
// on it, and failing a capture because a build was not VCS-stamped would trade the
// thing that matters for the thing that is merely nice. `go run` frequently does not
// stamp, so an empty string is the normal case in development.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			if len(s.Value) > 12 {
				return s.Value[:12]
			}
			return s.Value
		}
	}
	return ""
}

func listCaptures(dir string) error {
	names, err := capture.List(dir)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no captures in %s yet — run `armband capture`", dir)
	}
	fmt.Printf("%d capture(s) in %s\n", len(names), dir)

	// Report the gaps rather than only the captures. A weekly series with a
	// three-week hole is not a weekly series, and the hole is invisible in a list of
	// what is present — the same reason the sweep harness flags an infeasible cell
	// instead of dropping the row.
	var prev time.Time
	for _, n := range names {
		at, err := time.Parse("2006-01-02T1504Z", n)
		if err != nil {
			continue
		}
		gap := ""
		if !prev.IsZero() {
			d := at.Sub(prev)
			gap = fmt.Sprintf("  +%.1f days", d.Hours()/24)
			if d > 10*24*time.Hour {
				gap += "  <- gap larger than a gameweek"
			}
		}
		fmt.Printf("  %s%s\n", n, gap)
		prev = at
	}
	if !prev.IsZero() {
		since := time.Since(prev).Hours() / 24
		fmt.Printf("\nNewest is %.1f days old.\n", since)
		if since > 10 {
			fmt.Fprintf(os.Stderr, "The series has stalled — every week not captured is "+
				"point-in-time data lost permanently.\n")
		}
	}
	return nil
}
