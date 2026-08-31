package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The staleness guard asks a CONTENT question and used to key on a HISTORY pointer.
//
// `TestSnapshotCoversTheCurrentCode` — and, until it retired 2026-08-20,
// `TestReviewCoversTheCurrentCode` alongside it — wants to know one thing: has the
// measured content moved? Until 2026-08-15 both answered it by storing a commit SHA
// in a directory name and diffing `sha..HEAD`. A commit SHA identifies a position in
// history, and rebase **preserves content while rewriting history**, so every rebase
// broke both keys while changing nothing either guard cared about.
//
// The bill, measured on the history that produced this file: thirteen of the 807
// commits on it were pure re-key churn — eleven with zero insertions and zero
// deletions, two that only delete — and they concentrate late, four of the last
// twenty-five. `0ecef97` is a directory rename with zero insertions and zero
// deletions, the entire commit. Worse, `5a70915` DELETED `figures.csv`,
// `constants.csv` and `snapshot.md` — 915 lines of banked figures — because the
// naming convention could not survive a rebase, and `9191772` did the identical
// 915-line deletion to another directory, so it happened at least twice. A
// provenance guard that destroys provenance to keep its own filenames tidy has
// inverted its purpose.
//
// ⚠️ Count the denominator on the linear history, not on `--all`. `git rev-list
// --count --all` says 906, but 99 of those are on abandoned and backup refs —
// including `backup/pre-rebase-2026-08-14`, a ref created *for* these same
// rebases. Pairing that denominator with a numerator counted on `origin/main`
// understates the rate. Corrected 2026-08-15; the ratio is 1.6%, not 1.4%.
//
// A digest over the watched blobs is invariant under rebase, amend and
// cherry-pick, and moves exactly when the watched content moves. There is no
// pointer left to dangle, which is why the `merge-base --is-ancestor` check that
// both guards once carried is gone rather than repaired: it existed to detect a
// broken key, and the key can no longer break.
//
// # One helper, kept generic on purpose
//
// The two guards' own comments recorded that they were "one quantity, two
// implementations ... two guards, one rule, and only one of them repaired" — the
// ancestor check was fixed in `staleness_test.go` first and in the review gate's
// test only after a rebase caught it skipping and reporting PASS. `WatchedDigest`
// and the machinery around it (`Key`, `NewestKey`, `DigestDiff`) stay one shared
// implementation rather than being folded into the single remaining caller, so a
// second content guard — if one is ever added — inherits the fix instead of
// repeating the miss.
//
// # Why the watch list lives in non-test code
//
// It was a `var` declaration inside a `_test.go` file. It has to be reachable from
// `cmd/armband`, because `armband snapshot` computes the digest of what a snapshot
// record covers. Moving it here also means the watch list is shipped data rather
// than a test fixture, which is what it always was in substance.

// SnapshotWatchedPaths are the trees whose changes require a fresh accuracy
// snapshot: the scoring model, the replay harness, and the shipped constants. A
// change to docs or to the agent layer does not move a measured figure.
//
// ⚠️ `internal/config` was added 2026-08-14, and its absence was a real hole rather
// than an oversight to tidy. **`config.Load` is the second half of "the shipped
// constants"** — it decides the effective value of every field a config file omits,
// and `config.json` alone does not capture it. So deleting one backfill line moves
// every figure for any file omitting that key, while touching no watched path:
// `if cfg.Weights.BlendRateK <= 0 { cfg.Weights.BlendRateK = d.Weights.BlendRateK }`
// is a scoring change wearing a config-plumbing costume.
//
// Found when a commit that deleted two backfills tripped that guard only
// *incidentally*, through a type change in `internal/analysis` that happened to
// ride along. Had the same semantic change been made in `internal/config` alone —
// which is where it naturally belongs — it would have shipped with no snapshot.
// ⚠️ **`team.json` is watched too, and for the same reason `config.json` is.**
// The chip plan moved there on 2026-08-31 (see config.TeamConfig), and
// `Simulate` resolves it into the plan a replay plays — so a chip moved from
// GW6 to GW8 changes every replayed decision downstream of the horizon it
// truncates. Left unwatched, that is a scoring change with no watched path,
// which is precisely the hole the paragraph above describes for config.Load's
// backfills.
var SnapshotWatchedPaths = []string{
	"internal/analysis", "internal/backtest", "internal/config",
	"config.json", "team.json",
}

// IndexRev asks for the digest of the staged index rather than of a commit.
//
// Built for the review gate's `reviewkey` command, retired 2026-08-20 along with
// `TestReviewCoversTheCurrentCode` and `ReviewWatchedPaths` — see AGENTS.md's
// retired-process note. `armband snapshot` always digests a committed `HEAD`
// (measuring happens against a committed tree), so nothing in `cmd/armband` passes
// this today; `TestTheIndexDigestSeesStagedWork` keeps `WatchedDigest` honest about
// the staged-index case in case a future consumer needs it again.
const IndexRev = ""

// WatchedDigest returns a content digest over paths at rev, plus the same digest
// computed per top-level path.
//
// rev is a git revision, or IndexRev for the staged index. The per-path breakdown
// exists so a failing guard can name WHICH tree moved: the composite alone can only
// say that something did, and a guard that cannot say what it caught gets ignored.
//
// Test files are excluded, for the reason both guards once gave, back when there
// were two: adding a diagnostic or a regression test is not a change to what the
// program computes, and making tests owe a review would fire the gate on the very
// work that satisfies it.
func WatchedDigest(dir, rev string, paths []string) (digest string, perPath map[string]string, err error) {
	lines, err := watchedBlobs(dir, rev, paths)
	if err != nil {
		return "", nil, err
	}

	perPath = map[string]string{}
	for _, p := range paths {
		var owned []string
		for _, l := range lines {
			// A path is either the file itself or a prefix of it. The separator
			// check stops "stats" from claiming "statsomething".
			file := l[strings.IndexByte(l, '\t')+1:]
			if file == p || strings.HasPrefix(file, p+"/") {
				owned = append(owned, l)
			}
		}
		// A watch-list entry that matches nothing is a typo, a rename, or a
		// trailing slash — and all three read downstream as "this tree is clean".
		// `git ls-tree -- internal/analyis` exits 0 with no output, so the old gate
		// silently stopped covering a mistyped tree, and so would this one: the
		// composite simply has fewer lines and the guard keeps passing.
		//
		// It also catches being run from the wrong directory. With a relative
		// pathspec and a subdirectory cwd, every entry matches nothing and the
		// digest is sha256 of the empty string — a measurement-shaped answer that
		// is permanently wrong in whichever record it gets written to.
		if len(owned) == 0 {
			return "", nil, fmt.Errorf("watched path %q matches no files at %s: "+
				"a typo, a rename, a trailing slash, or the wrong working directory",
				p, revName(rev))
		}
		perPath[p] = digestOf(owned)
	}
	return digestOf(lines), perPath, nil
}

// RepoRoot resolves the repository root from dir. The watch lists are
// repo-relative, so every caller needs this and none should reimplement it.
func RepoRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git checkout at %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func revName(rev string) string {
	if rev == IndexRev {
		return "the staged index"
	}
	return rev
}

// watchedBlobs returns "<mode> <blob sha>\t<path>" for every non-test file under
// paths, sorted, so the digest depends on content and never on git's output order.
//
// # -z, and why it is not optional
//
// Both commands C-quote a path containing a non-ASCII byte *by default* — the same
// file lists as `"docs/jos\303\251.md"` or as `docs/josé.md` depending on the
// reader's `core.quotePath`. A digest that varies with a personal git setting is
// worse than no digest: two machines disagree on identical content, the gate fires
// permanently, and "a guard that fails for reasons unrelated to what it guards gets
// disabled". `-z` emits raw paths NUL-separated and removes the setting from the
// answer. No watched tree holds such a path today; `docs/` and `stats/` are watched
// and this project's data is full of accented names.
//
// # The mode is in the digest line
//
// `chmod -x stats/regenerate_mde.sh` leaves the blob id identical, so a digest over
// content alone passes it — while the old `git diff --name-only` gate caught it. Two
// files in a watched tree are executable today and both are scripts the documented
// workflow runs. The mode costs six characters and closes it.
func watchedBlobs(dir, rev string, paths []string) ([]string, error) {
	var args []string
	if rev == IndexRev {
		// `-s` prints "<mode> <object> <stage>\t<path>". Staged content, which is
		// what a record written alongside its own change must describe.
		args = append([]string{"ls-files", "-s", "-z", "--"}, paths...)
	} else {
		// `-r` recurses; output is "<mode> <type> <object>\t<path>".
		args = append([]string{"ls-tree", "-r", "-z", rev, "--"}, paths...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	var lines []string
	for _, raw := range strings.Split(string(out), "\x00") {
		tab := strings.IndexByte(raw, '\t')
		if tab < 0 {
			continue // the trailing empty record, or output we do not recognise
		}
		file := raw[tab+1:]
		if file == "" || strings.HasSuffix(file, "_test.go") {
			continue
		}
		fields := strings.Fields(raw[:tab])
		if len(fields) < 3 {
			continue
		}
		// Field 2 is the object id in both forms above: "mode type object" for
		// ls-tree, "mode object stage" for ls-files -s.
		object := fields[2]
		if rev == IndexRev {
			object = fields[1]
		}
		lines = append(lines, fields[0]+" "+object+"\t"+file)
	}
	sort.Strings(lines)
	return lines, nil
}

func digestOf(lines []string) string {
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// KeyFile is the machine-readable key an accuracy snapshot is identified by. It
// sits in the record's own directory.
//
// Also written into every `reviews/<date>-.../key.csv` from 2026-08-15 until the
// review gate retired 2026-08-20 — deliberately the same format for a review
// record and a snapshot, both asking the same staleness question about different
// watch lists, and this record's signature failure is one quantity with two
// implementations: the two guards had already diverged once, on the ancestor
// check. One file, one writer, one reader. The 171 existing `reviews/` directories
// keep their `key.csv` as history; nothing reads it now.
//
// It is deliberately NOT `figures.csv`. Putting the key there would make a
// snapshot's identity part of the figures it diffs against its predecessor, which
// is the circularity that already makes `stamp.commit` show up in the
// moved-figures table as a figure that moved. `figures.csv` keeps `stamp.commit`
// as provenance and no guard reads it.
const KeyFile = "key.csv"

// Keys inside KeyFile.
const (
	KeyDigest     = "watched_digest" // the composite over the whole watch list
	KeyRecordedAt = "recorded_at"    // RFC3339, and the ONLY ordering signal
	KeyCommit     = "commit"         // provenance; may become unreachable, never read by a guard
	keyPathPrefix = "path."          // one row per top-level watched path

	// KeyReconstructed is present, and non-empty, only on a key that was computed
	// after the fact rather than written by the run it describes.
	//
	// # Why a reconstructed key needs saying so
	//
	// `watched_digest` reconstructs EXACTLY: it is a pure function of the git tree
	// at the record's own commit, and `runSnapshot` digests that same commit, so
	// recomputing it later returns the identical string. Verified on all eight keys
	// that were written live — recomputing each from its own sha reproduced its
	// recorded digest, 8 of 8.
	//
	// `recorded_at` does NOT. It is wall-clock time at the moment the record was
	// written, and nothing in the tree remembers that. A reconstruction has to
	// substitute the record's own commit date, which for a record actually written
	// by a run is a lower bound: seven of the eight keys that predate the backfill
	// sit 0 to 388 seconds after the commit they name. The substitution is chosen
	// because it makes the ordering identical to the commit-date rule it replaced,
	// not because it is the time anything happened.
	//
	// ⚠️ **Seven of eight, not eight.** `2026-08-15-9e743cf` precedes its own commit
	// by four hours, which a run cannot do — it was seeded by hand when this
	// mechanism landed. It is therefore not evidence about live records, and citing
	// it as one would make this bound look wider than it is. See `FindPrevious`.
	//
	// So one field is exact and the other is a stand-in, and a reader cannot tell
	// which they have from the file's shape. This row says. It is deliberately a
	// sentence rather than a flag: a bare `true` would record that something was
	// substituted without recording what, and the substitution is the whole content
	// of the caveat.
	//
	// ⚠️ It is NOT read by any guard and must not become an ordering input. A record
	// is identified by its digest and ordered by its recorded time; adding a third
	// signal here would be the "two ways to identify a record" defect that this
	// file's opening comment records the guards already diverging on once.
	KeyReconstructed = "reconstructed"
)

// Key is a record's identity: what content it covers, and when it was recorded.
type Key struct {
	Digest     string
	RecordedAt time.Time
	Commit     string
	PerPath    map[string]string
	// Reconstructed is empty on a key written by the run it describes, and
	// otherwise says how it was rebuilt. See KeyReconstructed.
	Reconstructed string
}

// WriteKey writes a record's key into dir.
func WriteKey(dir string, k Key) error {
	v := newValues()
	v.set(KeyDigest, k.Digest)
	// RFC3339Nano, not RFC3339: second granularity means two records written in the
	// same second tie, and a tie falls to whatever os.ReadDir happens to yield —
	// which is the meaningless tie-break this key was introduced to remove, moved
	// from day granularity to second granularity rather than closed.
	v.set(KeyRecordedAt, k.RecordedAt.UTC().Format(time.RFC3339Nano))
	v.set(KeyCommit, k.Commit)
	// Written only when there is something to say. An empty row would put the word
	// "reconstructed" in every live key, which is the opposite of the distinction
	// the field exists to draw.
	if k.Reconstructed != "" {
		v.set(KeyReconstructed, k.Reconstructed)
	}
	for _, p := range sortedKeys(k.PerPath) {
		v.set(keyPathPrefix+p, k.PerPath[p])
	}
	return WriteValues(filepath.Join(dir, KeyFile), v)
}

// ReadKey reads a record's key from dir.
//
// A directory with no key file, or with an unparseable one, is reported as not a
// record at all. There is deliberately no fall back to the old commit-in-the-name
// scheme: two ways to identify a record is how the guards diverged in the first
// place, and only the newest record is ever read, so exactly one has to carry a key
// for the gate to work.
func ReadKey(dir string) (Key, error) {
	v, err := ReadValues(filepath.Join(dir, KeyFile))
	if err != nil {
		return Key{}, err
	}
	digest := v.Value[KeyDigest]
	if digest == "" {
		return Key{}, fmt.Errorf("%s carries no %s", filepath.Join(dir, KeyFile), KeyDigest)
	}
	at, err := time.Parse(time.RFC3339Nano, v.Value[KeyRecordedAt])
	if err != nil {
		return Key{}, fmt.Errorf("%s has an unreadable %s: %w",
			filepath.Join(dir, KeyFile), KeyRecordedAt, err)
	}
	k := Key{
		Digest:        digest,
		RecordedAt:    at,
		Commit:        v.Value[KeyCommit],
		Reconstructed: v.Value[KeyReconstructed],
		PerPath:       map[string]string{},
	}
	for _, name := range v.Keys {
		if strings.HasPrefix(name, keyPathPrefix) {
			k.PerPath[strings.TrimPrefix(name, keyPathPrefix)] = v.Value[name]
		}
	}
	return k, nil
}

// NewestKey returns the most recently recorded key under root, and the directory
// holding it. A directory named by exclude is skipped; pass "" to consider all.
//
// Ordered by the key's own `recorded_at`, which is the point. Both guards used to
// resolve the commit in each directory name to a commit date, so ordering needed
// git AND needed every recorded commit to still exist — the two properties a rebase
// removes. The failure that produced those comments (a lexical tie between two
// records sharing a date falling to the meaningless hex, which once reported a
// threshold as having moved 186 points when nothing had moved) is fixed here by
// recording the time rather than by inferring it.
//
// # It takes no exclude, and the parked note that asked for one was answered a
// # different way
//
// `FindPrevious` had to stop diffing a snapshot against itself: a re-run on the same
// day at the same commit reuses the directory name, and by the time the series is
// read that directory already holds a key. The note parked against that read "NewestKey
// takes no exclude", so an `exclude` parameter was the obvious repair — and it is the
// wrong one, because `FindPrevious` does not call this function at all. It cannot: it
// ranks only directories holding a `figures.csv`, and this ranks every directory. It
// shares the *ranking*, `newestKeyed`, and applies its own exclusion while selecting
// its own candidates.
//
// So an `exclude` here would have had no caller outside its own test, and it would have
// put the exclusion predicate in two places — the shape of the defect it was meant to
// avoid. The blocker dissolved rather than being fixed. Neither guard wants one: both
// ask "what is the newest record", with no record being written.
func NewestKey(root string) (dir string, k Key, ok bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", Key{}, false
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return newestKeyed(root, names)
}

// newestKeyed ranks an already-chosen set of directory names by recorded time.
//
// The one ordering implementation, and the reason it is split out: its two callers
// select candidates on different rules — every directory for `NewestKey`, only those
// holding figures for `FindPrevious` — while ranking them identically. Letting each
// rank for itself would be one quantity with two implementations, which this project's
// record names as its signature failure and which these two guards have already
// committed once, on the ancestor check.
//
// Names that carry no readable key are skipped; ok is false when none does.
func newestKeyed(root string, names []string) (dir string, k Key, ok bool) {
	for _, n := range names {
		cand, err := ReadKey(filepath.Join(root, n))
		if err != nil {
			continue // not a keyed record; historical directories are all of these
		}
		// A tie breaks on the larger directory name — a maximum, so it does not
		// depend on the order names arrive in and no caller has to pre-sort.
		// `After` is strict, so without this an exact tie silently keeps whichever
		// entry was read first.
		if !ok || cand.RecordedAt.After(k.RecordedAt) ||
			(cand.RecordedAt.Equal(k.RecordedAt) && n > dir) {
			dir, k, ok = n, cand, true
		}
	}
	return dir, k, ok
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DigestDiff names the top-level paths whose content differs between two per-path
// breakdowns, in the order the watch list declares them, then any path present in
// only one of the two.
//
// ⚠️ It iterates the UNION and not `paths`, and the difference is not cosmetic.
// `WatchedDigest` populates an entry for every path it is given, so iterating the
// current watch list can only ever see paths that still exist on it. **Removing an
// entry from a watch list therefore moves the composite while showing nothing
// moved** — the guard fires and prints an empty list, which is the failure the
// per-path breakdown was added to prevent. A record whose key predates a watch-list
// edit is exactly when a reader most needs to be told what changed.
func DigestDiff(paths []string, was, now map[string]string) []string {
	var moved []string
	seen := map[string]bool{}
	for _, p := range paths {
		seen[p] = true
		if was[p] != now[p] {
			moved = append(moved, p)
		}
	}
	// Anything the watch list no longer mentions, or did not mention then. Sorted,
	// because map order is not an order.
	var extra []string
	for _, m := range []map[string]string{was, now} {
		for p := range m {
			if !seen[p] {
				seen[p] = true
				extra = append(extra, p+" (no longer on the watch list)")
			}
		}
	}
	sort.Strings(extra)
	return append(moved, extra...)
}
