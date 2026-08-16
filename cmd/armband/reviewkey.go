package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"armband/internal/snapshot"
)

// runReviewKey writes the key.csv that identifies what a review record covers.
//
// # Why this exists, and why it reads the INDEX
//
// `TestReviewCoversTheCurrentCode` used to key a record by the commit in its
// directory name and diff `sha..HEAD`. Two costs followed. A rebase orphaned the
// key and cost a re-key commit — thirteen of them across this history. And because
// the gate diffed *committed* history, the record had to name a commit that did not
// exist when it was written, which forced the "record the changes first, then commit
// the review record alone" two-step onto every review.
//
// Digesting the staged index removes the second cost as well as the first.
// `reviews/` is not itself a watched path, so once the change is staged the watched
// content is final: this command digests it, writes the key, and the record can be
// staged and committed **in the same commit as the change it reviews**.
//
// So the workflow is one commit:
//
//	git add -A
//	go run ./cmd/armband reviewkey -out reviews/2026-08-15-my-change
//	# write reviews/2026-08-15-my-change/review.md
//	git add reviews/2026-08-15-my-change && git commit
//
// # The directory name is free text now
//
// It was `<date>-<short sha>` because the guard parsed the sha back out of it. The
// guard reads key.csv instead, so the name can say what the review was *about* —
// which is what a human reading `reviews/` wants — and nothing has to be renamed
// when history moves under it.
func runReviewKey(args []string) error {
	fs := flag.NewFlagSet("reviewkey", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `armband reviewkey — write a review record's key

Digests the STAGED index over the review watch list and writes key.csv into -out.
Stage your change before running it, or the key will describe the wrong content.

  -out string     Review record directory (created if absent). Name it for what the
                  review is about; the staleness guard does not read the name.
  -rev string     Digest this revision instead of the staged index. For recording a
                  review of a tree that is already committed.

`)
		fs.PrintDefaults()
	}
	out := fs.String("out", "", "review record directory")
	rev := fs.String("rev", snapshot.IndexRev, "digest this revision instead of the index")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*out) == "" {
		fs.Usage()
		return fmt.Errorf("reviewkey needs -out")
	}

	// From the repository root, not from the cwd. The watch list is repo-relative,
	// so run from a subdirectory every pathspec matches nothing — which
	// WatchedDigest now rejects rather than digesting the empty string and dressing
	// it up as a measurement, but resolving the root means it simply works.
	root, err := snapshot.RepoRoot(".")
	if err != nil {
		return err
	}
	digest, perPath, err := snapshot.WatchedDigest(root, *rev, snapshot.ReviewWatchedPaths)
	if err != nil {
		return fmt.Errorf("digesting the watched paths: %w", err)
	}
	sha, dirty := snapshot.GitState(root)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := snapshot.WriteKey(*out, snapshot.Key{
		Digest: digest, RecordedAt: time.Now(), Commit: sha, PerPath: perPath,
	}); err != nil {
		return err
	}

	source := "the staged index"
	if *rev != snapshot.IndexRev {
		source = *rev
	}
	fmt.Printf("wrote %s\n", filepath.Join(*out, snapshot.KeyFile))
	fmt.Printf("      digest %s over %d watched paths, from %s\n",
		digest[:12], len(perPath), source)
	// A dirty tree is only worth mentioning when the key came from the index, since
	// that is the case where unstaged work is silently outside what the key covers.
	if dirty && *rev == snapshot.IndexRev {
		fmt.Printf("note: the tree has unstaged changes. The key covers what is STAGED; " +
			"anything left unstaged will trip the gate on the next run.\n")
	}
	fmt.Printf("now write %s and commit both with your change.\n",
		filepath.Join(*out, "review.md"))
	return nil
}
