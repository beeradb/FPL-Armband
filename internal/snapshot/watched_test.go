package snapshot

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The property that motivates the whole file: a rebase must not change the digest.
//
// Both staleness guards used to key on a commit SHA, and a rebase preserves content
// while rewriting history — so the key broke on every rebase while nothing either
// guard cares about had moved. Thirteen commits of pure re-keying followed, one of
// them deleting 915 lines of banked figures to keep a directory name valid.
//
// This test replays that exact situation: a branch is rebased onto a main that never
// touched the watched paths, every commit SHA on it changes, and the digest must not.
func TestTheDigestSurvivesARebase(t *testing.T) {
	repo := newGitRepo(t)
	repo.seedWatched()
	repo.write("README.md", "not watched\n")
	repo.commitAll("base")
	repo.git("branch", "feature")

	// main moves, touching nothing watched.
	repo.write("README.md", "still not watched, but changed\n")
	repo.commitAll("main moves")

	repo.git("checkout", "feature")
	repo.write("internal/analysis/score.go", "package analysis\n\nconst K = 8\n")
	repo.commitAll("the work")

	before, _, err := WatchedDigest(repo.dir, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		t.Fatalf("digest before rebase: %v", err)
	}
	shaBefore := repo.rev("HEAD")

	repo.git("rebase", "master")

	after, _, err := WatchedDigest(repo.dir, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		t.Fatalf("digest after rebase: %v", err)
	}
	if shaBefore == repo.rev("HEAD") {
		t.Fatal("the rebase did not rewrite history, so this test proves nothing")
	}
	if before != after {
		t.Errorf("the rebase changed the digest.\n before %s\n after  %s\n\n"+
			"This is the defect the content key exists to remove: the watched content "+
			"is identical across the rebase and only the commit moved.", before, after)
	}
}

// A change to watched content must move the digest, or the guard guards nothing.
// The mirror of the test above, and the reason that one is not satisfied by
// returning a constant.
func TestTheDigestMovesWhenWatchedContentMoves(t *testing.T) {
	repo := newGitRepo(t)
	repo.seedWatched()
	repo.commitAll("base")

	before, _, err := WatchedDigest(repo.dir, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}

	repo.write("internal/analysis/score.go", "package analysis\n\nconst K = 24\n")
	repo.commitAll("move a constant")

	after, perPath, err := WatchedDigest(repo.dir, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("changing a watched file did not move the digest")
	}

	// And the per-path breakdown must name the tree that moved, because a guard that
	// can only say "something changed" gets ignored.
	_, wasPerPath, err := WatchedDigest(repo.dir, "HEAD~1", SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	moved := DigestDiff(SnapshotWatchedPaths, wasPerPath, perPath)
	if len(moved) != 1 || moved[0] != "internal/analysis" {
		t.Errorf("DigestDiff = %v, want exactly [internal/analysis]", moved)
	}
}

// Test files are excluded, so adding a regression test does not fire the gate on the
// very work that satisfies it. Both guards promised this before the change and both
// must still keep it.
func TestTheDigestIgnoresTestFilesAndUnwatchedPaths(t *testing.T) {
	repo := newGitRepo(t)
	repo.seedWatched()
	repo.commitAll("base")

	before, _, err := WatchedDigest(repo.dir, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}

	repo.write("internal/analysis/score_test.go", "package analysis\n// a new diagnostic\n")
	repo.write("docs/model.md", "not on the SNAPSHOT watch list\n")
	repo.write("cmd/armband/main.go", "package main\n")
	repo.commitAll("a test, a doc and a command")

	after, _, err := WatchedDigest(repo.dir, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Error("a _test.go file, an unwatched doc or an unwatched command moved the digest")
	}

	// docs IS on the review watch list, so the same tree must move that digest. This
	// is the check that would catch the two lists being accidentally unified.
	reviewDigest, _, err := WatchedDigest(repo.dir, "HEAD", ReviewWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	reviewBefore, _, err := WatchedDigest(repo.dir, "HEAD~1", ReviewWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if reviewDigest == reviewBefore {
		t.Error("docs/model.md changed and the review digest did not move")
	}
}

// The index digest is what makes a review record a one-commit operation: it must see
// staged content that HEAD does not.
func TestTheIndexDigestSeesStagedWork(t *testing.T) {
	repo := newGitRepo(t)
	repo.seedWatched()
	repo.commitAll("base")

	head, _, err := WatchedDigest(repo.dir, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	index, _, err := WatchedDigest(repo.dir, IndexRev, SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if head != index {
		t.Fatalf("with nothing staged the index and HEAD must agree:\n %s\n %s", head, index)
	}

	repo.write("internal/analysis/score.go", "package analysis\n\nconst K = 8\n")
	repo.git("add", "-A")

	staged, _, err := WatchedDigest(repo.dir, IndexRev, SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if staged == head {
		t.Error("the index digest did not see staged work, so a record written from it " +
			"would describe the wrong content")
	}
}

// A watched path must not claim a sibling whose name merely starts with it.
//
// The per-path split walks one flat listing and assigns each file to a watched path
// by prefix, so without the separator check "internal/analysis" would swallow
// "internal/analysis2" — and every change to the sibling would be reported against
// the wrong tree, which is worse than not reporting it, since the guard's message is
// what a reader acts on.
func TestAWatchedPathDoesNotClaimItsPrefixSibling(t *testing.T) {
	repo := newGitRepo(t)
	repo.write("internal/analysis/score.go", "package analysis\n")
	repo.write("internal/analysis2/other.go", "package analysis2\n")
	repo.commitAll("base")

	paths := []string{"internal/analysis", "internal/analysis2"}
	_, before, err := WatchedDigest(repo.dir, "HEAD", paths)
	if err != nil {
		t.Fatal(err)
	}

	repo.write("internal/analysis2/other.go", "package analysis2\n\nconst X = 1\n")
	repo.commitAll("move the sibling only")

	_, after, err := WatchedDigest(repo.dir, "HEAD", paths)
	if err != nil {
		t.Fatal(err)
	}
	moved := DigestDiff(paths, before, after)
	if len(moved) != 1 || moved[0] != "internal/analysis2" {
		t.Errorf("DigestDiff = %v, want exactly [internal/analysis2] — "+
			"the prefix sibling was reported against the wrong tree", moved)
	}
	if before["internal/analysis"] != after["internal/analysis"] {
		t.Error("internal/analysis moved when only internal/analysis2 changed")
	}
}

// A key must survive the round trip, and NewestKey must order by the recorded time
// rather than by the directory name.
//
// The ordering matters for the reason the deleted `newestSnapshot` recorded: a
// directory name starting with an ISO date LOOKS chronological, so two records
// sharing a day tie-break on whatever follows — which once made a diff span two
// changes while reporting one. Names are free text now, so name order carries even
// less information than it did.
func TestTheNewestKeyIsTheMostRecentlyRecordedOne(t *testing.T) {
	root := t.TempDir()
	older := Key{
		Digest:     "aaaa",
		RecordedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
		Commit:     "1111111",
		PerPath:    map[string]string{"stats": "s1", "docs": "d1"},
	}
	middling := Key{
		Digest:     "cccc",
		RecordedAt: time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC),
		Commit:     "3333333",
		PerPath:    map[string]string{"stats": "s3", "docs": "d1"},
	}
	newer := Key{
		Digest:     "bbbb",
		RecordedAt: time.Date(2026, 8, 15, 17, 0, 0, 0, time.UTC),
		Commit:     "2222222",
		PerPath:    map[string]string{"stats": "s2", "docs": "d1"},
	}
	// ⚠️ THREE records, with the newest in the lexical MIDDLE. Two cannot do this
	// job, and the first version of this test used two and was vacuous.
	//
	// `os.ReadDir` returns entries sorted, so with two records the wanted answer is
	// either the first read or the last, and the test then passes for some wrong
	// implementation by coincidence. The original put the newest at "alpha" and so
	// passed for first-wins, for lexical-min, AND for no ordering at all — deleting
	// the `RecordedAt.After` comparison entirely left the whole package green. It
	// discriminated against exactly one wrong implementation, lexical-max. Caught in
	// review, 2026-08-15.
	//
	// With the newest in the middle, every ordering rule that is not "read
	// recorded_at" picks a different directory:
	//
	//	first-wins / lexical-min / no ordering → alpha   (oldest)
	//	lexical-max / readdir-last             → zebra   (middling)
	//	recorded_at                            → middle  (newest)  ✓
	//
	// All three share a date, which is the tie the old scheme resolved on
	// meaningless hex.
	for name, k := range map[string]Key{
		"2026-08-15-alpha":  older,
		"2026-08-15-middle": newer,
		"2026-08-15-zebra":  middling,
	} {
		mustMkdir(t, filepath.Join(root, name))
		if err := WriteKey(filepath.Join(root, name), k); err != nil {
			t.Fatal(err)
		}
	}
	// A historical directory with no key at all must be ignored rather than crash.
	mustMkdir(t, filepath.Join(root, "2026-08-10-unkeyed"))

	dir, got, ok := NewestKey(root)
	if !ok {
		t.Fatal("NewestKey found nothing")
	}
	if dir != "2026-08-15-middle" {
		t.Errorf("picked %s, want 2026-08-15-middle (the latest recorded_at; "+
			"alpha is first and oldest, zebra is last and middling, so no name-order "+
			"rule reaches the right answer)", dir)
	}
	if got.Digest != newer.Digest || got.Commit != newer.Commit {
		t.Errorf("round trip lost data: %+v", got)
	}
	if !got.RecordedAt.Equal(newer.RecordedAt) {
		t.Errorf("recorded_at round trip: got %v want %v", got.RecordedAt, newer.RecordedAt)
	}
	if got.PerPath["stats"] != "s2" || got.PerPath["docs"] != "d1" {
		t.Errorf("per-path round trip: %+v", got.PerPath)
	}
	if moved := DigestDiff([]string{"stats", "docs"}, older.PerPath, got.PerPath); len(moved) != 1 || moved[0] != "stats" {
		t.Errorf("DigestDiff = %v, want [stats]", moved)
	}
}

// A directory with no key is not a record. There is deliberately no fall back to the
// old commit-in-the-name scheme, because two ways to identify a record is how the two
// guards diverged in the first place.
func TestAnUnkeyedDirectoryIsNotARecord(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "2026-08-14-eb1b546"))
	if _, _, ok := NewestKey(root); ok {
		t.Error("a directory with no key.csv was accepted as a record")
	}
}

// A reconstructed key must not read as a contemporaneous one.
//
// The distinction is load-bearing because the two halves of a key reconstruct
// differently: `watched_digest` is a pure function of the tree at the record's
// commit and comes back exact, while `recorded_at` is wall-clock time that nothing
// remembers and has to be stood in for. A reader cannot tell from the file's shape
// which they hold, so the file has to say.
//
// The absence half is asserted too. A marker written unconditionally would appear on
// every live key and stop distinguishing anything, which is the same defect as no
// marker at all wearing the opposite costume.
func TestAReconstructedKeySaysSo(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "live")
	rebuilt := filepath.Join(root, "rebuilt")
	mustMkdir(t, live)
	mustMkdir(t, rebuilt)

	when := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	if err := WriteKey(live, Key{Digest: "d", RecordedAt: when}); err != nil {
		t.Fatal(err)
	}
	const why = "backfilled; recorded_at is the commit's date, not the write time"
	if err := WriteKey(rebuilt, Key{Digest: "d", RecordedAt: when, Reconstructed: why}); err != nil {
		t.Fatal(err)
	}

	got, err := ReadKey(rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reconstructed != why {
		t.Errorf("the reconstruction note round-tripped as %q, want %q", got.Reconstructed, why)
	}
	got, err = ReadKey(live)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reconstructed != "" {
		t.Errorf("a live key carries %q; the marker must be absent when there is "+
			"nothing to disclose, or it stops distinguishing anything", got.Reconstructed)
	}
	b, err := os.ReadFile(filepath.Join(live, KeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), KeyReconstructed) {
		t.Errorf("a live key file mentions %q:\n%s", KeyReconstructed, b)
	}
}

// TestEverySnapshotCandidateCarriesAKey.
//
// `FindPrevious` ranks the series on each candidate's own key, and a candidate with
// no key cannot be placed in it — so it is skipped, and a snapshot gets diffed
// against an older baseline than the one immediately before it. That is invisible
// downstream: only WHETHER a baseline existed is ever banked, never which one.
//
// This is the invariant that let the content key be switched on at all. It held only
// because the 51 pre-key candidates were backfilled from their own commits, which was
// possible only while every one of those commits still resolved — and it will not
// stay possible. So the guard is against the state returning, not against the
// original gap.
//
// It is a candidate-level check rather than a directory-level one, deliberately:
// `stats/snapshots` also holds banked-cell directories with no `figures.csv`, which
// are not snapshots, are never ranked, and owe no key.
func TestEverySnapshotCandidateCarriesAKey(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	series := filepath.Join(root, "stats", "snapshots")
	entries, err := os.ReadDir(series)
	if err != nil {
		t.Skipf("no snapshot series to check: %v", err)
	}
	candidates, unkeyed := 0, []string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(series, e.Name(), ValuesFile)); err != nil {
			continue // banked cells, not a snapshot
		}
		candidates++
		if _, err := ReadKey(filepath.Join(series, e.Name())); err != nil {
			unkeyed = append(unkeyed, e.Name())
		}
	}
	if candidates == 0 {
		t.Skip("no snapshot candidates present")
	}
	if len(unkeyed) > 0 {
		t.Errorf("%d of %d snapshot candidates carry no readable %s:\n  %s\n\n"+
			"FindPrevious cannot place them in the series, so they are skipped and "+
			"some snapshot is diffed against an older baseline than its real "+
			"predecessor — which nothing downstream records.\n\n"+
			"Write one with the commit that directory names, while that commit is "+
			"still reachable. A rebase orphans it and then it cannot be written at all.",
			len(unkeyed), candidates, KeyFile, strings.Join(unkeyed, "\n  "))
		return // or the line below contradicts the failure it sits under
	}
	t.Logf("%d snapshot candidates, all keyed", candidates)
}

// TestAKeyDescribesTheCommitItNames.
//
// Presence is not correctness. The sibling test above checks that every candidate
// carries a parseable key; this one checks that the key says something TRUE about the
// commit its own `commit` row names — that `watched_digest` and every `path.*` row
// recompute from that tree.
//
// # What it catches that nothing else could
//
// A backfill is a bulk write of derived data into banked directories, and the way it
// goes wrong is quiet. Digest `HEAD` fifty-one times instead of each directory's own
// commit and every file is well-formed, every other test in this package passes, and
// the series silently reorders — a wrong answer shaped exactly like the right one,
// which is the failure class this whole file exists to refuse. There is no other
// check on the substance of a key.
//
// # It is scoped to what a checkout can still answer, and it decays
//
// A key is verified only when its commit resolves HERE. Anything unreachable is
// counted and skipped rather than failed, because a rebase orphans commits and a
// shallow or fresh clone never had them — and a guard that fails for reasons
// unrelated to what it guards gets deleted. `stats/snapshots/2026-08-10-13cded0` is
// already in that state relative to `origin/main` and survives only in this
// checkout's object store, so its key can be verified today and never again from a
// clone. ⚠️ The verifiable fraction only falls. That is an argument for checking now,
// not for checking loosely.
//
// The recomputation uses the paths the KEY itself records rather than the current
// `SnapshotWatchedPaths`. The watch list is edited over time — `internal/config` was
// added on 2026-08-14 — and a key is a statement about the list in force when it was
// written. Reading today's list would make every historical key fail the next time
// the list changes, which is a false alarm rather than a finding.
func TestAKeyDescribesTheCommitItNames(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	series := filepath.Join(root, "stats", "snapshots")
	entries, err := os.ReadDir(series)
	if err != nil {
		t.Skipf("no snapshot series to check: %v", err)
	}
	checked, unreachable := 0, 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		key, err := ReadKey(filepath.Join(series, e.Name()))
		if err != nil || key.Commit == "" {
			continue // not a keyed record, or a key that names no commit
		}
		var paths []string
		for p := range key.PerPath {
			paths = append(paths, p)
		}
		if len(paths) == 0 {
			continue
		}
		sort.Strings(paths)
		digest, perPath, err := WatchedDigest(root, key.Commit, paths)
		if err != nil {
			unreachable++
			continue // orphaned by a rebase, or absent from a shallow clone
		}
		checked++
		if digest != key.Digest {
			t.Errorf("%s: %s records %s but commit %s produces %s.\n\n"+
				"The key describes a different tree from the one it names. A key "+
				"digested from the wrong revision is well-formed, passes every "+
				"other check, and silently reorders the series.",
				e.Name(), KeyFile, shortSHA(key.Digest), shortSHA(key.Commit),
				shortSHA(digest))
			continue
		}
		if moved := DigestDiff(paths, key.PerPath, perPath); len(moved) > 0 {
			t.Errorf("%s: the composite digest matches but these per-path rows do "+
				"not recompute from %s: %s", e.Name(), shortSHA(key.Commit),
				strings.Join(moved, ", "))
		}
	}
	if checked == 0 {
		t.Skip("no key names a commit reachable from this checkout")
	}
	if t.Failed() {
		return // "verified" would contradict the failures printed above it
	}
	t.Logf("%d keys verified against their own commit, %d unreachable and skipped",
		checked, unreachable)
}

// --- fixture ---------------------------------------------------------------

type gitRepo struct {
	t   *testing.T
	dir string
}

func newGitRepo(t *testing.T) *gitRepo {
	t.Helper()
	r := &gitRepo{t: t, dir: t.TempDir()}
	r.git("init")
	r.git("config", "user.email", "test@example.com")
	r.git("config", "user.name", "Test")
	// Rebase needs an identity and a deterministic default branch name.
	r.git("checkout", "-b", "master")
	return r
}

// seedWatched gives every entry on BOTH watch lists at least one file.
//
// `WatchedDigest` errors on an entry that matches nothing, because in the real
// repository that means a typo, a rename or the wrong working directory — all of
// which otherwise read downstream as "this tree is clean". A fixture that seeds one
// path out of four is that same condition, so it has to seed all of them.
//
// Driven off the lists themselves rather than a hand-written copy: adding a watch
// entry would otherwise break every fixture here with an error about the fixture
// instead of about the change, and someone would fix it by loosening the check.
func (r *gitRepo) seedWatched() {
	r.t.Helper()
	seen := map[string]bool{}
	for _, list := range [][]string{ReviewWatchedPaths, SnapshotWatchedPaths} {
		for _, p := range list {
			if seen[p] {
				continue
			}
			seen[p] = true
			// A watch entry is either a file or a directory. The two in the lists
			// today that are files carry an extension; everything else is a tree.
			if filepath.Ext(p) != "" {
				r.write(p, "seed\n")
				continue
			}
			r.write(filepath.Join(p, "seed.txt"), "seed\n")
		}
	}
}

func (r *gitRepo) git(args ...string) string {
	r.t.Helper()
	return r.gitEnv(nil, args...)
}

// gitEnv is the one place a git command is built, so that hardening it later — a
// `-c` override, say — reaches every caller rather than the subset that happened not
// to need a custom environment.
//
// env is appended AFTER os.Environ() because os/exec takes the last value for a
// duplicated key, so an entry here overrides a developer's ambient setting rather
// than being overridden by it.
func (r *gitRepo) gitEnv(env []string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (r *gitRepo) write(rel, content string) {
	r.t.Helper()
	path := filepath.Join(r.dir, rel)
	mustMkdir(r.t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *gitRepo) commitAll(msg string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-m", msg)
}

// commitAt makes a commit with a fixed committer date and returns its full sha.
//
// The date has to be forced rather than taken from the clock, because the ordering
// under test is `git show --format=%ct` — two commits made in the same test would
// otherwise share a second and tie, which is the one case the ordering rule cannot
// demonstrate. It is the COMMITTER date that %ct reads, so setting only the author
// date would produce a test that passed for the wrong reason.
func (r *gitRepo) commitAt(name, date string) string {
	r.t.Helper()
	r.write(name+".txt", name+"\n")
	r.git("add", "-A")
	r.gitEnv([]string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date},
		"commit", "-m", name)
	return strings.TrimSpace(r.rev("HEAD"))
}

func (r *gitRepo) rev(ref string) string {
	r.t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = r.dir
	out, err := cmd.Output()
	if err != nil {
		r.t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return string(out)
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
