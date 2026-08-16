package backfill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scoringPackages are the two trees whose output is the research record.
//
// `internal/analysis` computes every football number and `internal/backtest` replays
// seasons through it. Between them they produce every figure in `AGENTS.md` and
// in the research record.
var scoringPackages = []string{"../analysis", "../backtest"}

// recoveredTeamNews are the packages that exist only to recover historical
// availability, plus the store they write into.
var recoveredTeamNews = []string{
	"armband/internal/backfill",
	"armband/internal/wayback",
	"armband/internal/capture",
}

// TestTheScoringPathCannotSeeRecoveredTeamNews is the invariant behind "this task
// delivers the data and proves it honest; using it is the next job".
//
// # Why this is a test and not a promise in a comment
//
// The backfill recovers something the replay would obviously like to have: what FPL
// was actually saying about a player's fitness at each deadline, including the
// injuries that resolved, which `backtest.statusAt` cannot reconstruct at all. Wiring
// it in is a one-line import and an entirely reasonable-looking change.
//
// It would also silently invalidate every measured figure in the research record.
// Those were all produced with the reconstruction as it stands, and improving the
// input underneath them does not make them better — it makes them **incomparable with
// each other**, half computed under one availability model and half under another,
// with nothing in the output saying which. That is this project's signature failure
// arriving through the front door: one quantity, two implementations, and the measured
// one is not the one that runs.
//
// So the change is not forbidden — it is a real improvement and should happen. What is
// forbidden is it happening *by accident*, without the measurement pass that reprices
// the record. Deleting a line from `recoveredTeamNews` is that decision, made
// deliberately, in a diff somebody reviews.
//
// Source-scanning rather than a runtime check, for the reason the neighbouring guards
// give: the failure is a new import in a new file, and it agrees with everything else
// on the day it is written.
func TestTheScoringPathCannotSeeRecoveredTeamNews(t *testing.T) {
	var offenders []string
	for _, pkg := range scoringPackages {
		files, err := filepath.Glob(filepath.Join(pkg, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 0 {
			t.Fatalf("%s has no Go files; this guard is scanning the wrong place and would "+
				"pass forever", pkg)
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range recoveredTeamNews {
				if strings.Contains(string(b), `"`+imp+`"`) {
					offenders = append(offenders,
						filepath.Join(filepath.Base(pkg), filepath.Base(f))+" imports "+imp)
				}
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("the scoring path has reached the recovered team news:\n  %s\n\n"+
			"Every figure in AGENTS.md and the research record was measured with "+
			"backtest.statusAt as it stands. Feeding it real historical availability "+
			"is a genuine improvement "+
			"and it reprices the whole record — so it needs its own measurement pass, not "+
			"an import. If that pass has been done, remove the package from "+
			"recoveredTeamNews in this file and say so in the review record.",
			strings.Join(offenders, "\n  "))
	}
}

// TestTheBackfillDoesNotComputeFootballNumbers pins the other half of the boundary.
//
// `internal/analysis` is pure computation and is the only place a football number is
// produced. A recovery tool that started deriving, say, an availability multiplier
// from the flags it recovers would be a second scoring implementation in a package
// nobody thinks of as scoring — and it would be the one the replay was not using.
func TestTheBackfillDoesNotComputeFootballNumbers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	self := "isolation_test.go"
	var offenders []string
	for _, f := range files {
		if filepath.Base(f) == self {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), `"armband/internal/analysis"`) {
			offenders = append(offenders, filepath.Base(f))
		}
	}
	if len(offenders) > 0 {
		t.Errorf("%s imports internal/analysis. This package recovers what FPL published "+
			"and must not interpret it; the moment it derives a number, that number is a "+
			"second implementation of something the engine already owns.",
			strings.Join(offenders, ", "))
	}
}
