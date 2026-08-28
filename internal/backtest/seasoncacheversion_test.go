package backtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seasonCacheReaders are the files outside this package that open the parsed
// season cache by path. They cannot go through `Load`: they are Python, and Go
// exposes no interchange format they could read instead.
var seasonCacheReaders = []string{
	"stats/xpoints_common.py",
	"stats/xpoints_permove.py",
	"stats/xpoints_channel_audit.py",
	"scripts/flagcal.py",
}

// ⚠️ **Bumping the cache version without these four orphans every one of them,
// silently and permanently.**
//
// It happened: the v8 → v9 bump landed in `season.go` alone. Go stopped writing
// `backtest-v8-*`, so each script kept reading a snapshot frozen a week earlier —
// and on a fresh checkout, where no v8 file has ever existed, they fail forever
// with "run from the repo root", which is no longer a fix because nothing
// anywhere writes v8.
//
// That is the recorded stale-cache bug class arriving from the mirror direction:
// not an old writer colliding with a new reader, but a new version with no writer
// for the old readers.
//
// The scripts are checked for the CURRENT prefix rather than the old one, so this
// fails on the next bump too, not just on the one that prompted it.
func TestTheSeasonCacheVersionMatchesItsPythonReaders(t *testing.T) {
	root := repoRootFor(t)
	for _, name := range seasonCacheReaders {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Errorf("%s is listed as a reader of the season cache and could not be "+
				"read: %v. If it was deleted, delete it from seasonCacheReaders too — "+
				"a guard naming a file that does not exist stops guarding anything",
				name, err)
			continue
		}
		src := string(b)
		if !strings.Contains(src, seasonCachePrefix) {
			t.Errorf("%s does not mention %q. It reads the parsed season cache "+
				"directly, so a version bump in season.go must be made here too — "+
				"otherwise it reads a file Go no longer writes, which on a fresh "+
				"checkout does not exist at all",
				name, seasonCachePrefix)
		}
		// And no stale spelling left behind beside the new one, which would read
		// as updated while still opening the wrong path on one code branch.
		for _, old := range []string{"backtest-v7-", "backtest-v8-"} {
			if strings.Contains(src, old) {
				t.Errorf("%s still contains %q alongside the current %q",
					name, old, seasonCachePrefix)
			}
		}
	}
}

// repoRootFor walks up from the test's own directory to the module root, so the
// guard does not depend on where `go test` was invoked from.
func repoRootFor(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find the module root above the test's working directory")
	return ""
}
