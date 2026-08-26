package backtest

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestDocumentedCellsCommandsDisableTheTestCache requires `-count=1` on every
// documented command that writes a cells file.
//
// ⚠️ **A cells file is a SIDE EFFECT the Go test cache cannot see.** The cache
// keys on the test binary, the command-line flags and the environment and files
// the test consulted — never on what it wrote. So a second run of a diag test at
// the same commit with the same environment is served from the cache: the whole
// test body is skipped, the recorded stdout is replayed verbatim, `go test`
// prints `ok` with `(cached)` and **the cells file is never written**. The
// operator sees a passing run and a complete-looking log, and then reads either
// the file from a previous run or no file at all.
//
// That is this package's signature failure — silence reading as success — and it
// happened on 2026-08-26: an arm of the three-input xGC comparison was re-run to
// clear a contaminated file, exited 0, printed a full table, and left no cells
// behind at all. The contaminated file had already been deleted, so the only
// reason it was caught is that the next command could not open it.
//
// ⚠️ **Nothing inside the test can guard this**, which is why the guard is here
// and looks at documentation rather than at behaviour: on a cache hit no code in
// the test body runs, so there is no line that could check its own output. The
// command line is the only place the defence can live.
//
// The same trap is recorded twice more for a different symptom — two arms
// differing only in an environment variable read at package initialisation, in
// `enginerulesmovedcell_diag_test.go` and `internal/snapshot/staleness_test.go`.
// There the cache served one arm's output for the other and the arms printed
// identical numbers, which reads exactly like a confinement result.
func TestDocumentedCellsCommandsDisableTheTestCache(t *testing.T) {
	roots := []string{".", "../snapshot", "../analysis", "../agent", "../config"}
	goTest := regexp.MustCompile(`\bgo test\b`)
	var offenders []string
	for _, root := range roots {
		files, err := filepath.Glob(filepath.Join(root, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(b), "\n")
			// A command block is a run of comment lines. `writes` latches when
			// one of them sets FPL_CELLS and clears at the end of the block, so
			// a `go test` line is only judged against the block it belongs to.
			var writes bool
			for i, ln := range lines {
				trimmed := strings.TrimSpace(ln)
				if !strings.HasPrefix(trimmed, "//") {
					writes = false
					continue
				}
				if strings.Contains(ln, "FPL_CELLS=") {
					writes = true
				}
				if !writes || !goTest.MatchString(ln) {
					continue
				}
				// ⚠️ **The flags may be on a CONTINUATION line.** These commands
				// wrap with a trailing backslash, so `go test` and `-count=1`
				// routinely sit on different lines. Judging the `go test` line
				// alone reported a correctly-written command as an offender the
				// first time this guard met a wrapped one — which would have
				// pushed the fix toward unwrapping the command rather than toward
				// the flag. The whole continued command is one string.
				cmd := trimmed
				for j := i; strings.HasSuffix(cmd, "\\") && j+1 < len(lines); j++ {
					next := strings.TrimSpace(lines[j+1])
					if !strings.HasPrefix(next, "//") {
						break
					}
					cmd = strings.TrimSpace(strings.TrimSuffix(cmd, "\\")) + " " +
						strings.TrimSpace(strings.TrimPrefix(next, "//"))
				}
				if !strings.Contains(cmd, "-count=1") {
					offenders = append(offenders, filepath.Join(root, filepath.Base(f))+":"+strconv.Itoa(i+1)+" "+trimmed)
				}
				// One `go test` command ends the block, whatever follows.
				writes = false
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("documented commands write a cells file without -count=1, so a second run "+
			"at the same commit is served from the Go test cache and writes NOTHING while "+
			"printing ok:\n  %s\n\nAdd -count=1 to each. There is no in-test fix: on a cache "+
			"hit the test body does not run.", strings.Join(offenders, "\n  "))
	}
}
