package snapshot

// The machine-readable companion to the markdown.
//
// A snapshot is written as a pair: the prose a human reads, and a flat key/value
// file the *next* snapshot diffs against. The alternative — parsing the previous
// markdown — would couple the diff to the report's layout, so improving a heading
// would break the comparison. That is the same coupling this project avoided by
// giving the diagnostics a CSV instead of scraping their printed tables.

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WriteValues writes a snapshot's figures beside its markdown.
func WriteValues(path string, v *Values) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"figure", "value"}); err != nil {
		return err
	}
	for _, k := range v.Keys {
		if err := w.Write([]string{k, v.Value[k]}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// ReadValues reads a previous snapshot's figures.
func ReadValues(path string) (*Values, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil {
		if err == io.EOF {
			return newValues(), nil
		}
		return nil, err
	}
	v := newValues()
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) >= 2 {
			v.set(rec[0], rec[1])
		}
	}
	return v, nil
}

// FindPrevious returns the most recent snapshot directory under root, excluding
// one being written now.
//
// # Ordering is by the record's own recorded time, and both obvious alternatives are wrong
//
// Directories are named date-then-short-commit, so lexical order LOOKS
// chronological and is wrong the moment two snapshots share a day: the tie then
// falls to the hex commit, which carries no time information at all. That is not
// hypothetical — with snapshots at 2eaca08, c676f42 and fac46eb on one date, the
// lexical rule picked fac46eb because "f" sorts last, so a diff meant to isolate a
// single change silently spanned two of them and reported a detection threshold as
// having moved 186 points when nothing had moved at all. Modification time is not
// used either: a checkout, a copy or a rebase rewrites mtimes.
//
// So each candidate is ranked on the `recorded_at` in its own `key.csv`, through the
// same `newestKeyed` the two staleness guards rank records with.
//
// # Why this stopped being keyed on the commit
//
// Until 2026-08-15 each candidate's short commit was resolved to a commit date with
// `git show`. That needed git AND needed every recorded commit to still exist, and a
// rebase preserves content while rewriting history — so it broke on exactly the
// operation that changes nothing this function cares about. Worse, it broke
// asymmetrically: a candidate that failed to resolve was skipped while others
// resolved, and a rebase orphans the NEWEST shas, which is precisely the baseline
// that matters. The failure was silent to a machine, because only whether a baseline
// existed was ever banked, never which one.
//
// The recorded time also carries strictly more information than the inferred one: a
// commit date says when the code was committed, not when the measurement was
// written.
//
// # The switch was made baseline-neutral, not assumed to be
//
// Two things had to be true first, and both were checked rather than argued.
//
// Coverage. Only 8 of the 59 candidates carried a key, so switching would have made
// this ignore the other 51 and silently diff against a different baseline — a moved
// figure smuggled inside a plumbing change. The other 51 were backfilled, which was
// possible ONLY because every one of the 59 recorded commits still resolved in this
// checkout at the time. ⚠️ **That window closes.** Each rebase orphans more of them,
// and a candidate whose commit has gone cannot be given a key after the fact at all.
// If this ever has to be done again for a directory added later, do it while its
// commit is still reachable.
//
// `13cded0` is the proof that the deadline is real rather than prospective: it is
// already NOT an ancestor of `origin/main` and resolved only from this checkout's
// object store, so its key could be written here and in no fresh clone — and under
// the old rule a fresh clone was silently skipping it in the ranking. That skip is
// what the committed key removes.
//
// Each backfilled key takes `recorded_at` from its own commit's date, which is what
// makes the two rules agree: the ordering signal becomes a recorded copy of the
// signal it replaces. A backfilled key says so in a `reconstructed` row — see
// `KeyReconstructed` for why a reconstruction must not be indistinguishable from a
// contemporaneous record.
//
// Neutrality. Every one of the 59 snapshots was asked, under both rules, which
// baseline it would be diffed against, and all 59 answers were identical; so was the
// answer for a hypothetical new snapshot, and so was the full 59-deep ordering
// obtained by peeling the newest off the series repeatedly — with one exception,
// which did not reach any baseline. `2026-08-15-9e743cf` moves from 8th-newest to
// 11th, because its key (the only one seeded by hand, when the key mechanism landed)
// carries a `recorded_at` four hours BEFORE the commit it names. A live record cannot
// do that: the snapshot runs at a checkout of that commit, so it is written at or
// after it. Left as it stands rather than corrected, because it is somebody's
// attestation and no figure depends on it.
//
// # Three outcomes, deliberately distinguished
//
// Conflating any two rebuilds that defect one level up:
//
//   - **No candidate at all** — the first snapshot in a series. An empty name and no
//     error, and the caller reports the snapshot as the baseline.
//   - **Exactly one candidate** — returned, keyed or not. There is no ordering to get
//     wrong when there is nothing to order against, so refusing here would be
//     ceremony rather than safety: it is the predecessor by construction. The
//     186-point failure needed three same-day candidates. This also keeps a snapshot
//     root working outside a git checkout entirely.
//   - **Several candidates, none of which carries a key** — a baseline exists, this
//     cannot say which, and no guess repairs that. An error. There is deliberately no
//     fall back to the lexical rule: it is the exact rule the first paragraph exists
//     to refuse, so a fallback would re-arm the 186-point failure in the one
//     situation where nothing else could catch it.
//
// # What is still NOT fixed here: a PARTIAL absence of keys
//
// A candidate with no key is skipped while others have one, so an unkeyed snapshot
// loses to an older keyed one. That is the same shape as the dangling-commit skip it
// replaces, one property better: a key cannot be orphaned by a rebase, so nothing
// that happens to history can put a candidate into that state.
//
// A run still can. `runSnapshot` writes `figures.csv` and then `key.csv`, with three
// error returns between them — `WriteConstants`, `RepoRoot`, and `WatchedDigest`,
// which is deliberately fatal when a watched path matches nothing. So a snapshot that
// dies in that window leaves a keyless candidate. The write ORDER is what makes that
// fail safe rather than silent: the half-written directory can never be *ranked* as
// anyone's baseline, and `TestEverySnapshotCandidateCarriesAKey` fails on it as soon
// as it is committed. Writing the key first would invert both properties.
func FindPrevious(root, exclude string) (dir string, values *Values, err error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == exclude {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), ValuesFile)); err != nil {
			continue
		}
		names = append(names, e.Name())
	}
	// The first snapshot in a series. Not an error: there is genuinely nothing to
	// diff against, and the caller says so in its output.
	if len(names) == 0 {
		return "", nil, nil
	}
	// Deliberately NOT sorted. A `sort.Strings` here was load-bearing under the
	// deleted commit-date rule, whose strict comparison kept the FIRST name seen and
	// so inherited the caller's order. `newestKeyed` breaks a tie on the LARGER name
	// instead — a maximum, which is invariant to the order names arrive in — so
	// sorting could no longer change any answer, while its comment claimed it fixed
	// which name won a tie. An inert line with a live-sounding justification is worse
	// than no line.
	//
	// One candidate is the predecessor by construction: nothing is being ordered, so
	// there is no ordering to get wrong and no reason to read a key at all.
	pick := names[0]
	if len(names) > 1 {
		pick, _, _ = newestKeyed(root, names)
	}
	if pick == "" {
		return "", nil, fmt.Errorf(
			"%d snapshot directories under %s hold figures, and none of them "+
				"carries a %s, so the baseline to diff against cannot be "+
				"identified.\n\n"+
				"Guessing from the directory names is not a recovery: they sort by "+
				"date and then by a hex commit that carries no time, and that rule "+
				"once reported a detection threshold as having moved 186 points "+
				"when nothing had moved at all.\n\n"+
				"There is no \"this one is the first\" answer available: those %d "+
				"already hold figures, so this snapshot follows one of them — say "+
				"which, with -previous. The flag takes a path rather than a bare "+
				"name, so it reads `-previous %s/<dir>`, and -no-r reuses the "+
				"inference this run has already written.",
			len(names), root, KeyFile, len(names), root)
	}
	v, err := ReadValues(filepath.Join(root, pick, ValuesFile))
	if err != nil {
		return "", nil, err
	}
	return pick, v, nil
}

// File names inside a snapshot directory.
const (
	MarkdownFile  = "snapshot.md"
	ValuesFile    = "figures.csv"
	ConstantsFile = "constants.csv"
)

// WriteConstants writes the full constants list for every sweep in the snapshot.
//
// Not inlined in the markdown: there are well over a hundred, and what a reader
// needs in prose is the fingerprint. What a reader needs when a figure has moved
// unexpectedly is this file, diffed against the previous snapshot's copy — which is
// the whole reason the fingerprint alone is not enough.
func WriteConstants(path string, sweeps []Sweep) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"sweep", "digest", "kind", "name", "value"}); err != nil {
		return err
	}
	for _, s := range sweeps {
		if !s.HasProv {
			continue
		}
		for _, c := range s.Prov.Constants {
			if err := w.Write([]string{s.Label, s.Prov.Digest, "config", c.Path, c.Value}); err != nil {
				return err
			}
		}
		// Env switches are written even when none is set, as an explicit row, so
		// "no switches were in force" is a recorded fact rather than an absence.
		if len(s.Prov.Env) == 0 {
			if err := w.Write([]string{s.Label, s.Prov.Digest, "env",
				"(none set)", "the shipped defaults ran"}); err != nil {
				return err
			}
		}
		for _, c := range s.Prov.Env {
			if err := w.Write([]string{s.Label, s.Prov.Digest, "env", c.Path, c.Value}); err != nil {
				return err
			}
		}
	}
	w.Flush()
	return w.Error()
}

// DirName is the snapshot directory name: sortable, and carrying its commit.
func DirName(date string, commit string) string {
	return fmt.Sprintf("%s-%s", date, strings.TrimSpace(shortSHA(commit))[:min(7, len(shortSHA(commit)))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
