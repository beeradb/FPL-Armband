package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// Scheduling a run without a daemon.
//
// The useful moment to think about a gameweek is a few hours before its
// deadline, when team news is out. That is a bad fit for cron on its own: cron
// fires on a clock and deadlines move, so a fixed weekly entry drifts away from
// the thing it is supposed to precede.
//
// So the gate lives here instead. Run the command as often as you like — hourly
// is fine — and it does nothing until a deadline is close, then runs once.
//
// # Running once is the whole problem
//
// `review` bills real tokens. A scheduler that fires hourly inside a six-hour
// window would pay for six of them, and the failure is silent: each run looks
// correct on its own. State is kept on disk so a completed gameweek is not
// re-reviewed, and the record is written *before* the expensive work rather
// than after, because a crash mid-run must not license a retry loop that bills
// on every pass.
type dueState struct {
	// LastReviewed is the gameweek most recently attempted.
	LastReviewed int `json:"last_reviewed_gameweek"`
	// At is when, for the human reading the file.
	At string `json:"at"`
}

func dueStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".armband-schedule.json"
	}
	return filepath.Join(dir, "armband", "schedule.json")
}

func readDueState() dueState {
	var st dueState
	b, err := os.ReadFile(dueStatePath())
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st)
	return st
}

func writeDueState(gw int) error {
	st := dueState{LastReviewed: gw, At: time.Now().Format(time.RFC3339)}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	p := dueStatePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// dueVerdict is why a run should or should not happen now.
type dueVerdict struct {
	Run      bool
	Gameweek int
	Deadline time.Time
	Reason   string
}

// checkDue decides whether the upcoming deadline is close enough to act on and
// has not already been handled.
func checkDue(next *fpl.Event, leadHours float64, now time.Time) dueVerdict {
	if next == nil {
		return dueVerdict{Reason: "no upcoming gameweek — the season is over or not yet published"}
	}
	v := dueVerdict{Gameweek: next.ID, Deadline: next.DeadlineTime}

	until := next.DeadlineTime.Sub(now)
	switch {
	case until <= 0:
		v.Reason = fmt.Sprintf("GW%d's deadline has passed", next.ID)
		return v
	case until > time.Duration(leadHours*float64(time.Hour)):
		v.Reason = fmt.Sprintf("GW%d is %s away, outside the %.0fh window",
			next.ID, until.Round(time.Hour), leadHours)
		return v
	}
	if st := readDueState(); st.LastReviewed >= next.ID {
		v.Reason = fmt.Sprintf("GW%d was already reviewed (%s)", next.ID, st.At)
		return v
	}
	v.Run = true
	v.Reason = fmt.Sprintf("GW%d deadline in %s", next.ID, until.Round(time.Minute))
	return v
}

// cmdDue runs the review, but only when a deadline is close and that gameweek
// has not been handled. Safe to call on a schedule as often as you like.
func cmdDue(ctx context.Context, cfg config.Config, cfgPath string,
	client *fpl.Client, engine *analysis.Engine, writeReport bool) error {

	v := checkDue(engine.Boot.NextEvent(), cfg.Review.LeadHours, time.Now())
	if !v.Run {
		fmt.Printf("Not due: %s.\n", v.Reason)
		return nil
	}

	// Recorded before the run, not after. A run that crashes halfway has already
	// spent most of its tokens, so retrying it on the next tick would bill again
	// for the same gameweek — and an hourly schedule would keep doing that until
	// the deadline passed. Losing one report is the cheaper failure.
	if err := writeDueState(v.Gameweek); err != nil {
		return fmt.Errorf("recording the scheduled run: %w", err)
	}

	fmt.Printf("Due: %s. Running the review.\n", v.Reason)
	return cmdAgent(ctx, cfg, cfgPath, client, engine,
		advicePrompt(engine), fmt.Sprintf("FPL Review — GW%d", v.Gameweek),
		writeReport, false)
}

// cmdSchedule prints a crontab line for the current binary.
func cmdSchedule(cfg config.Config) error {
	exe, err := os.Executable()
	if err != nil {
		exe = "armband"
	}
	wd, _ := os.Getwd()
	fmt.Printf(`Run hourly and let the command decide when to act:

  0 * * * * cd %s && %s due >> /tmp/armband-due.log 2>&1

It exits immediately unless a deadline is within %.0f hours, and records the
gameweek so a second run inside the same window does nothing. Nothing is
submitted to FPL — it writes a report and stops.

State: %s

And the archival capture, which is separate and matters more over years:

  0 */6 * * * cd %s && %s capture >> /tmp/armband-capture.log 2>&1

Two public requests, about 128 KB gzipped, and it refuses to overwrite an
existing capture — so a six-hourly schedule costs almost nothing and guarantees
at least one capture inside the %.0f-hour window before every deadline. That
window is the point: a capture 200 hours out records a different information
state from one at six hours, and only the second says what was known when the
decision was taken.

Do NOT fold this into the "due" line. That command fires near deadlines by
design, so the resulting series would cluster on interesting weeks and be
missing exactly the quiet weeks that make a control group. A capture wants to
be boring.

It yields nothing this season, and that is not a reason to defer it — every week
not captured is point-in-time data that cannot be recovered afterwards, and four
recorded questions are blocked on precisely that. Commit the captures: this
repository has no remote, so an uncommitted one dies with its worktree.
`, wd, exe, cfg.Review.LeadHours, dueStatePath(), wd, exe, cfg.Review.LeadHours)
	return nil
}
