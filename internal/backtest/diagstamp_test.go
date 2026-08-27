package backtest

// Every diagnostic table says what produced it.
//
// # The failure this closes
//
// A sidecar records what produced one run of a SWEEP. Repeatedly, two arms were
// differenced that did not share a code state, and none was caught by a number
// looking wrong — the numbers were plausible every time. Three are checkable from
// this repository alone: the pre-#82 +20.6/t 3.63 superseded at AGENTS.md's
// transfer-gate section, the free-hit reading that describes a codebase older
// than 5b970338, and the one below. ⚠️ A running TALLY of these exists, but only
// in the research record, so it is deliberately not asserted here — a count this
// file cannot substantiate is exactly the unverifiable claim the file is about.
//
// The last is the sharpest: `FPL_XGC_EXTERNAL_DIR` was set for one run and unset for the other,
// nothing was dirty, no commit differed, both runs were individually correct, and
// a published verdict flipped sides. The information existed; the comparison did
// not.
//
// ⚠️ **That run never passed through the guard that would have caught it, and
// could not have.** `writeSweepProvenance` returns immediately when `FPL_CELLS`
// is unset or the sink is nil, so a diagnostic invoked without it banks no
// sidecar at all and prints its table to stdout. The guarded path is
// sweep -> cells -> stats/sweep_inference.R, and stdout is not on it. Yet stdout
// is where the quoted evidence comes from: that table went into a PR body, into
// AGENTS.md and into the research vault having passed no comparability check of
// any kind.
//
// So the stamp travels with the table rather than beside the cells. A figure
// pasted anywhere carries the state that produced it, and two pasted tables can
// be compared by eye at the moment of reading rather than reconstructed two days
// later.
//
// # Why the gate is a function and not a convention
//
// This was 151 copies of the same four lines across 134 files. A convention
// saying "also print the stamp" would rot exactly as this project's other
// hand-maintained lists rot — the four season lists that go stale every summer,
// an override list that outlived its situation. So the gate every diagnostic
// already had to pass through is the thing that prints, nothing is added to
// remember, and TestEveryDiagGateGoesThroughRequireDiag fails if a 152nd copy
// appears.
//
// # What the stamp covers, and what it does not
//
// Commit, dirty flag and the fingerprinted environment. Those are the three that
// produced all four recorded failures, so the scope is the evidence rather than a
// guess.
//
// ⚠️ It does NOT carry the constants digest, because that needs the config and
// `loadConfig` fails the test when the config is unreadable — a diagnostic that
// needs none would start failing for a stamp.
//
// ⚠️ **That is a resolution gap, not a soundness gap, and an earlier draft of
// this comment overstated it.** It read "WHICH config file was read is covered;
// what that file CONTAINED is not". Both halves of that are wrong. When
// `FPL_CONFIG` is set, it is path-valued, so `snapshot.CurrentEnv` digests the
// file's BYTES into the env line and the contents are covered directly. When it
// is unset, the shipped `config.json` is read — and that file is git-tracked and
// listed in `snapshot.SnapshotWatchedPaths`, so a clean tree at a stamped commit
// has exactly those constants and `commit` plus `dirty` cover it. The same holds
// for `internal/config`, which supplies the defaults for fields a config omits.
//
// What is genuinely missing is RESOLUTION: two stamps at different commits say
// that something moved, not whether the constants were part of it. A reader has
// to assume the worst and diff the commits. That is worth fixing one day and is
// not a hole through which an incomparable pair passes.

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"armband/internal/snapshot"
)

// requireDiag skips a diagnostic unless DIAG is set, and stamps it when it runs.
//
// The two are one call deliberately. A diagnostic cannot run without passing
// here, so it cannot print an unattributable table.
func requireDiag(t *testing.T) {
	t.Helper()
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	fmt.Print(runStamp(t.Name()))
}

// stampOnce memoises the git state, which shells out. A run of several
// diagnostics would otherwise pay for it once per test for an answer that cannot
// change mid-process.
var stampOnce struct {
	sync.Once
	sha   string
	dirty bool
}

// runStamp renders the state that decides whether this table may be differenced
// against another one.
//
// Printed per TEST rather than once per process. A reader pastes one table, not a
// session transcript, and a stamp at the top of a run of forty diagnostics is not
// attached to the fortieth in any way that survives a copy.
func runStamp(name string) string {
	stampOnce.Do(func() {
		stampOnce.sha, stampOnce.dirty = snapshot.GitState(".")
	})

	env := snapshot.CurrentEnv()
	s := "\n"
	s += "--- run stamp: " + name + "\n"
	// ⚠️ dirty first and spelled out. It means the record cannot be verified,
	// not that it is wrong — a dirty run was once re-run and reproduced to the
	// digit, which is not a defence, because that was only knowable after
	// re-running.
	if stampOnce.dirty {
		s += "  commit:  " + stampOnce.sha + "  ⚠️ DIRTY TREE — these figures are not reproducible from any commit\n"
	} else {
		s += "  commit:  " + stampOnce.sha + "\n"
	}
	// The digest covers the switches that are NOT set as well, so two stamps with
	// equal digests ran under the same environment even where both lines below
	// read "none".
	s += "  env:     digest " + env.Digest
	if len(env.Set) == 0 {
		s += "  (no fingerprinted switch set)\n"
	} else {
		s += fmt.Sprintf("  (%d set)\n", len(env.Set))
		for _, c := range env.Set {
			// ⚠️ Path-valued switches arrive here already digested by
			// snapshot.CurrentEnv. One of them names an unlicensed data source and
			// this text gets pasted into public records, so the value must never be
			// un-digested on its way to a terminal.
			s += "             " + c.Path + "=" + c.Value + "\n"
		}
	}
	s += "  compare: two tables may be differenced only if commit and env digest both match,\n"
	s += "           or the difference between them IS the declared variable.\n"
	return s
}

// TestEveryDiagGateGoesThroughRequireDiag refuses a diagnostic that gates itself
// on DIAG without passing through requireDiag.
//
// This existed as 151 identical copies across 134 files before requireDiag, and a
// 152nd would print no stamp while looking exactly like the other 151 in review.
// That is the shape of every silent failure in this project: something that reads
// as covered and is not. The test is the coverage.
//
// It scans source text rather than behaviour because there is no behaviour to
// observe — a diagnostic that skips prints nothing either way, so the only place
// the omission is visible is the source.
func TestEveryDiagGateGoesThroughRequireDiag(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || e.Name() == "diagstamp_test.go" {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, `os.Getenv("DIAG")`) {
				offenders = append(offenders, fmt.Sprintf("%s:%d", e.Name(), i+1))
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a diagnostic reads DIAG directly instead of calling requireDiag(t): %s\n"+
			"Its table would print with no run stamp, so a figure copied out of it into a PR "+
			"body, AGENTS.md or the research record would carry nothing saying which commit and "+
			"which environment produced it. That is the fourth comparability failure exactly.",
			strings.Join(offenders, ", "))
	}
}
