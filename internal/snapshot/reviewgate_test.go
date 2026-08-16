package snapshot

import (
	"path/filepath"
	"testing"
)

// TestReviewCoversTheCurrentCode fails when reviewable code or the research record
// has moved since the newest review record.
//
// # What it enforces, and what it deliberately cannot
//
// It enforces that a review *covers this content*. It cannot and does not judge
// whether the review was any good — no test can. That is the same bargain
// TestSnapshotCoversTheCurrentCode strikes, and it is why the snapshot discipline
// stopped rotting after twelve commits had slipped through it.
//
// The reason to want it: a set of reviewer agents exists, configured per-machine
// rather than in this repository, and they were invoked ad hoc, so most changes
// went unreviewed. (This said "eight reviewer agents exist in `.claude/agents/`"
// until 2026-08-16, when they were moved out of the repository — a number and a
// path, both of which the move falsified.) The cost is on the record. A
// constant kept its "confirmed" label long enough to be cited as ground truth for a
// model that no longer existed. The research file accumulated ten competing figures
// for one quantity with none marked canonical, a false significance convention stated
// as canonical method, an arithmetic error inside the paragraph correcting that
// convention, and an annotation that had itself gone stale — which is the worst shape
// available, because a reader trusts an annotation more than the text it annotates.
//
// # Keyed on content, not on a commit
//
// ⚠️ Changed 2026-08-15. This gate used to read a commit SHA out of the record's
// directory name and diff `sha..HEAD`. A commit SHA is a history pointer and the
// question here is about content, so **every rebase broke the key while changing
// nothing this gate cares about** — thirteen re-key commits, one of them a rename
// with zero insertions and zero deletions, and one that deleted 915 lines of banked
// figures to keep a directory name valid. `WatchedDigest` explains the change in
// full. The gate now compares a digest, which rebase cannot disturb.
//
// The `merge-base --is-ancestor` check both guards carried is **gone rather than
// repaired**. It existed to catch a key that pointed at nothing, and a content
// digest cannot point at nothing. Its own comment recorded that it had to be a
// failure rather than a skip because "a skip here is indistinguishable from
// reviewed and clean" — that reasoning was right for the mechanism it guarded, and
// the mechanism is what changed.
//
// # Reviewers are the weaker lever, and this comment is the place to say so
//
// Judged on what has actually caught defects here, invariants beat reviewers
// decisively: a byte-identity check on the held metric caught seam violations,
// reproducing 29,747 of 29,747 bonus awards caught ten duplicate archive rows, a
// reproducibility test caught a map-iteration bug that had already corrupted a
// published figure, and the environment-switch completeness test rejected a build
// missing a registration. All free, all run every time.
//
// So when this test fails, the first question is not "which agent do I dispatch" but
// **what quantity must this change not move, and is that tested?** The review gate
// skill says the same thing in its first section. Falsification is roughly two orders
// of magnitude cheaper than confirmation on this harness.
//
// # What to do when it fails
//
// Invoke the `review-gate` skill, which holds the triage table — which reviewers a
// change owes, by what it touches — and the record format. Then commit the record
// under `reviews/`.
//
// # Deliberately permissive about its own preconditions
//
// It skips when git is unavailable or when no keyed review record exists yet. A
// guard that fails for reasons unrelated to what it guards gets disabled, and then
// it guards nothing. That lesson is already recorded against the snapshot guard and
// applies unchanged.
func TestReviewCoversTheCurrentCode(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	dir, key, ok := NewestKey(filepath.Join(root, "reviews"))
	if !ok {
		// No keyed record is the state this repository is in when the content key
		// first lands. Skipping rather than failing means adopting it does not
		// require re-reviewing everything that came before, which would be a large
		// enough bill to guarantee the gate got deleted instead.
		t.Skip("no keyed review record yet; run the review-gate skill to create one")
	}

	digest, perPath, err := WatchedDigest(root, "HEAD", ReviewWatchedPaths)
	if err != nil {
		// Fatal, not a skip: `repoRoot` has already established that git works, so
		// what is left is a watch-list entry matching nothing or a damaged
		// repository. A skip here would be indistinguishable from "reviewed and
		// clean" — the reason the deleted ancestor check was a failure rather than
		// a skip, and the one duty of that check which does not disappear with it.
		t.Fatalf("could not digest the watched paths: %v", err)
	}
	if digest == key.Digest {
		return
	}

	moved := describeMoved(DigestDiff(ReviewWatchedPaths, key.PerPath, perPath))
	t.Errorf("the newest review record is %s, and the watched content has changed "+
		"since it was recorded.\n\nTrees that moved:\n%s\n\n"+
		"Reviewer agents were invoked ad hoc before this guard, which is "+
		"how a retracted constant stayed cited as ground truth. Invoke the `review-gate` "+
		"skill: it holds the triage table for which reviewers a change owes, and the record "+
		"format. Commit the record under reviews/.\n\n"+
		"But read the first section of that skill before dispatching anybody. Invariants have "+
		"caught more real defects here than reviewers have, and they are free — so the first "+
		"question is what quantity this change must NOT move, and whether that is tested.\n\n"+
		"Test files are excluded, so adding a diagnostic does not trip this.\n\n"+
		"(Recorded digest %s, current %s. A rebase alone can no longer cause this: the key "+
		"is over content, so if this fires, content really moved.)",
		dir, moved, shortSHA(key.Digest), shortSHA(digest))
}
