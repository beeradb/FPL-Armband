package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The watch list lives in watched.go, beside the digest that reads it. It used to
// sit here as an unexported test var; `cmd/armband` needs it now, because writing
// a snapshot means writing the key describing what it covers.

// TestSnapshotCoversTheCurrentCode fails when the scoring model, the replay harness
// or the shipped constants have moved since the newest accuracy snapshot.
//
// # Why this exists, and why it is a test rather than a convention
//
// The snapshot was specified as a standing requirement: any change to the model or
// the harness produces one, and it is part of the change rather than a follow-up.
// The design tried to guarantee that by making cell emission a *side effect of
// running a sweep* — which is the right instinct, and it does not cover the case
// that actually happened. Most changes are not sweeps. Twelve commits landed after
// the first snapshot with no second one, including three that altered the replay's
// data, the scoring constants and squad selection.
//
// So the side-effect half was never enough on its own, and "remember to run it"
// is precisely the kind of hand-maintained discipline this codebase has watched rot
// four separate times — the four season lists, an override list that outlived its
// situation, a cache version that a stale file defeated, a comment claiming a live
// term was inert. The enforcement has to be mechanical.
//
// # It is cheap, which removes the last excuse
//
// The expensive half is the harness accuracy, which needs per-cell data from a sweep
// and carries its own staleness stamp. The model half — calibration drift, the
// clean-sheet rate, the transfer error, the prediction benchmark — runs in about
// twelve seconds and produces roughly 500 rows. There is no cost argument for
// skipping it.
//
// # What to do when this fails
//
//	FPL_MODEL_CSV=/tmp/model.csv DIAG=1 go test ./internal/backtest -count=1 \
//	  -run 'TestDiagCalibrationDrift|TestDiagCleanSheetPoisson|TestDiagPredictionBenchmark|TestDiagNextFivePredictors|TestDiagSixty|TestDiagDefconBias|TestDiagTeamBlend|TestDiagTransferError' \
//	  -timeout 120m
//	go run ./cmd/armband snapshot -model /tmp/model.csv
//	git add stats/snapshots/<new> && git commit ...
//
// ⚠️ **`-count=1` is load-bearing and was missing from this recipe until 2026-08-15.**
// Without it the SECOND invocation on an unchanged tree returns `ok (cached)`, runs
// none of the diagnostics, writes nothing to `FPL_MODEL_CSV` — and **exits 0**. The
// renderer then finds no model rows and banks a snapshot carrying
// `model.present,false`: 555 figures silently absent, from a command that reported
// success. It happened here, on the second regeneration of one snapshot, and the
// first regeneration had been correct — so the failure appears only once the obvious
// "just run it again" has been done. A caller who deletes the CSV first, as the
// paragraph below recommends, converts the stale-file trap into this one.
//
// The check that catches it is one line, and it is worth writing beside any
// invocation: **fail if the model CSV is absent or empty after the test run.**
//
// ⚠️ **Pass `-model` explicitly, and pass the same path twice.** The diagnostics
// write to `FPL_MODEL_CSV`; the renderer reads a `-model` FLAG defaulting to
// /tmp/model.csv and does not look at that variable. Writing the diagnostics
// somewhere else and then running the renderer bare does not fail — it silently
// builds the snapshot from whatever is already at /tmp/model.csv, which on a shared
// machine may be another run's file or a stale one from before the change being
// measured. That happened: a snapshot regenerated this way reproduced the previous
// one's figures exactly, which is the most convincing possible wrong answer, since
// "nothing moved" is what a reader expects most of the time.
//
// The safe habit is a path nobody else writes to, given to both halves.
//
// ⚠️ **`TestDiagTransferError` was missing from that list and is now in it.** Following
// the recipe as written produced a snapshot with the eight `model.transfer_error.*` rows
// **absent** — the buy/sell asymmetry and the sold-player-played-again split, which is
// the evidence behind "the sell side is calibrated; its error is entirely availability".
// Nothing failed. The renderer emits what it is given, so a short model CSV produces a
// short snapshot, and the guard this comment belongs to only asks that a snapshot
// *exists*. A silently smaller snapshot is the same shape as the stale-`/tmp` trap above
// and is worse in one way: the missing rows read as "not measured at this commit" rather
// than as "the operator ran the wrong command".
//
// # Permissive about the environment, strict about the record
//
// It used to skip in three cases: git unavailable, no snapshots at all, and the
// recorded commit not being an ancestor of HEAD. The first is environmental and is
// still a skip. **The other two are not, and skipping them was the guard turning
// itself off without saying so** — which is the failure this whole package exists to
// prevent, arriving inside the guard.
//
// ⚠️ The third case no longer exists, as of 2026-08-15. It described history being
// rewritten underneath a snapshot, which was possible only because the snapshot's
// key was a commit. The key is now a digest over the watched content — see
// `WatchedDigest` — so a rebase cannot orphan it, and there is nothing left for that
// check to detect. This is the fix for the churn it caused: thirteen re-key commits,
// and `5a70915` deleting `figures.csv`, `constants.csv` and `snapshot.md` — 915
// lines of banked figures — to satisfy a directory name. **A provenance guard that
// destroys provenance to keep its filenames tidy has inverted its purpose**, and
// that is the argument for the change rather than the commit count.
//
// "No snapshots at all" is the same shape as before: in a repository that has the
// discipline, zero snapshots means somebody deleted them, not that none were ever
// due. It still fails.
//
// # The one legitimate absence, and why it is opt-in
//
// If the series is moved out of git — published by CI as an artefact rather than
// committed — then a fresh clone genuinely has none until CI fetches them. That is a
// real workflow and FPL_SNAPSHOTS_EXTERNAL=1 declares it. It is opt-in rather than
// inferred, because "absent" and "absent for a good reason" are indistinguishable
// from in here, and guessing in favour of silence is how the skip got here in the
// first place.
func TestSnapshotCoversTheCurrentCode(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	external := os.Getenv("FPL_SNAPSHOTS_EXTERNAL") != ""

	dir, key, ok := NewestKey(filepath.Join(root, "stats", "snapshots"))
	if !ok {
		if external {
			t.Skip("FPL_SNAPSHOTS_EXTERNAL is set and no keyed snapshot is present; " +
				"the series is expected to come from CI rather than from git")
		}
		t.Fatal("no keyed accuracy snapshot is present in stats/snapshots.\n\n" +
			"In a repository that carries this discipline that means they were " +
			"deleted, not that none was ever due — and with none present this guard " +
			"cannot tell whether the model has changed since anything was measured.\n\n" +
			"Generate one (see the note on this test), or set " +
			"FPL_SNAPSHOTS_EXTERNAL=1 if the series is deliberately published by CI " +
			"instead of committed.\n\n" +
			"(A snapshot directory with no " + KeyFile + " does not count. Every " +
			"snapshot written before 2026-08-15 was in that state by construction " +
			"until they were backfilled; one snapshot carrying a key is what this " +
			"guard needs.)")
	}

	digest, perPath, err := WatchedDigest(root, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		// NOT a skip. `repoRoot` has already established that git works and this is
		// a checkout, so a failure here is a watch-list entry matching nothing, or
		// git telling us the repository is damaged. The predecessor made the
		// equivalent case fatal with the same reasoning, and downgrading it while
		// the comment above still promises "permissive about the environment,
		// strict about the record" would be a one-directional weakening.
		t.Fatalf("could not digest the watched paths: %v", err)
	}
	if digest == key.Digest {
		return
	}

	moved := describeMoved(DigestDiff(SnapshotWatchedPaths, key.PerPath, perPath))
	t.Errorf("the newest accuracy snapshot is %s, and the watched content has moved "+
		"since it was recorded.\n\nTrees that moved:\n%s\n\n"+
		"A snapshot is part of a scoring or harness change, not a follow-up: without one "+
		"there is no record of which constants a figure was measured under, and this "+
		"project has already had a sweep's numbers outlive the constant they were "+
		"measured at and then be cited as ground truth. Regenerate it — the model half "+
		"takes about twelve seconds. See the note on this test for the two commands.\n\n"+
		"Test files are excluded, so adding a diagnostic does not trip this. But the "+
		"exclusion is by filename, not by whether the change is executable: a "+
		"comment-only edit to a watched non-test file moves the digest and fires this "+
		"gate. So a snapshot in this series records WHEN it was taken, not that the "+
		"model behaved differently at that commit — do not read the series as a "+
		"history of behavioural change.\n\n"+
		"(Recorded digest %s, current %s. A rebase alone can no longer cause this.)",
		dir, moved, shortSHA(key.Digest), shortSHA(digest))
}

// describeMoved renders DigestDiff's answer, and says so plainly when it is empty.
//
// An empty list beside a fired gate is not impossible — it means the composite moved
// for a reason the per-path breakdown cannot attribute, which is worth naming rather
// than printing as a blank bullet under "Trees that moved". A reader who sees an
// empty list assumes the guard is broken, and on this evidence they would be nearly
// right.
func describeMoved(moved []string) string {
	if len(moved) == 0 {
		return "  (none — the composite moved but no single watched tree did. " +
			"That means the watch list itself changed, or a file moved between two " +
			"watched trees. Compare the path.* rows in the record's key.csv.)"
	}
	return "  " + strings.Join(moved, "\n  ")
}

// newestSnapshot is gone. It ordered candidates by resolving the commit in each
// directory name to a commit date, which needed git and needed every recorded
// commit to still exist — the two properties a rebase removes. `NewestKey` orders
// by a recorded timestamp instead. The failure its comment described (a lexical tie
// between two records sharing a date falling to the meaningless hex, which once
// reported six files as stale that a newer snapshot already covered) is fixed by
// recording the time rather than by inferring it.

// repoRoot delegates rather than reimplementing: `RepoRoot` is the shipped one,
// and this package's recorded failure mode is one quantity with two
// implementations.
func repoRoot() (string, error) { return RepoRoot(".") }

func output(dir string, args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	b, err := cmd.Output()
	return string(b), err
}

// `run` is gone with the two ancestor checks that were its only callers.
