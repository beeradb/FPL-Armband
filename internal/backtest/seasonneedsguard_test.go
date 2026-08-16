package backtest

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestSeasonNeedsReproduceTheNamedGrids is what licenses the refactor.
//
// A capability model that produced *different* grids from the four already in
// use would silently change the population of every diagnostic that adopted it,
// which is the exact failure the model exists to prevent. So the requirement for
// each named grid must reproduce that grid exactly, pair for pair and in order.
//
// Read a failure here as "the metadata is wrong", not "the grid is wrong" — the
// grids are the evidence, and the table is the thing being fitted to them.
func TestSeasonNeedsReproduceTheNamedGrids(t *testing.T) {
	// sweepPairNames honours FPL_SWEEP_SEASONS, so the shipped four have to be
	// asked for explicitly here rather than read from it.
	t.Setenv("FPL_SWEEP_SEASONS", "default")

	for _, c := range []struct {
		name  string
		got   [][2]string
		want  [][2]string
		needs seasonNeeds
	}{
		{"sweepPairNames", gridFor(needsSweep), sweepPairNames(), needsSweep},
		{"extendedPairNames", gridFor(needsExtended), extendedPairNames(), needsExtended},
		{"scoringPairNames", gridFor(needsScoring), scoringPairNames(), needsScoring},
		{"xgPairNames", gridFor(needsPriorXG), xgPairNames(), needsPriorXG},
	} {
		if len(c.got) != len(c.want) {
			t.Errorf("%s: %s produced %d pairs, the named grid has %d\n  derived: %v\n  named:   %v",
				c.name, c.needs, len(c.got), len(c.want), c.got, c.want)
			continue
		}
		for i := range c.want {
			if c.got[i] != c.want[i] {
				t.Errorf("%s: pair %d is %v derived and %v named", c.name, i, c.got[i], c.want[i])
			}
		}
	}

	// The defcon requirement has no named grid to check against — it replaces a
	// literal — so it is pinned directly. One season has defensive contribution
	// and that will not change until another is played.
	if got := gridFor(needsDefcon); len(got) != 1 || got[0] != [2]string{"2024-25", "2025-26"} {
		t.Errorf("needsDefcon = %v, want exactly {2024-25, 2025-26}: defensive contribution "+
			"was a scoring category in one season only", got)
	}
}

// TestSeasonNeedsExcludeWhatTheyShould pins the individual exclusions, because
// a grid can come out the right LENGTH for the wrong reason.
func TestSeasonNeedsExcludeWhatTheyShould(t *testing.T) {
	played := func(pairs [][2]string) map[string]bool {
		m := map[string]bool{}
		for _, p := range pairs {
			m[p[1]] = true
		}
		return m
	}

	// 2019-20 is playable and scoreable, and its transfer path is not comparable
	// with anything. It must appear where transfers are not required and vanish
	// where they are.
	if !played(gridFor(needsScoring))["2019-20"] {
		t.Error("2019-20 is missing from a HOLD-only grid; its scoring is fine and it is " +
			"one of only two seasons the backfill adds")
	}
	if played(gridFor(needsExtended))["2019-20"] {
		t.Error("2019-20 is being PLAYED in a grid that requires a comparable transfer " +
			"path. FPL granted unlimited free transfers and froze prices for three months " +
			"that season, so its wallet is not a sample of the same process")
	}

	// 2018-19 has no clubs, so it can be a prior and never a played season.
	for _, n := range []seasonNeeds{needsScoring, needsExtended, needsSweep} {
		if played(gridFor(n))["2018-19"] {
			t.Errorf("2018-19 is being played under %s; it has no teams.csv and the loader "+
				"accepts it prior-only", n)
		}
	}

	// The repair is exactly what separates the four-season grid from the six.
	if len(gridFor(needsSweep)) != 4 || len(gridFor(needsExtended)) != 6 {
		t.Errorf("native-xG grid is %d pairs and repaired-xG is %d; expected 4 and 6, which "+
			"is the whole degrees-of-freedom argument",
			len(gridFor(needsSweep)), len(gridFor(needsExtended)))
	}
}

// TestSnapshotDiagnosticsDeclareTheirSeasons is the structural guard, scoped
// deliberately to the eight diagnostics that feed the accuracy snapshot.
//
// # Why not the whole package
//
// Run against every file, this flags thirty — and most are RIGHT to name a
// season. bpsrules_test.go recomputes 2025-26's bonus under the 2026/27 rules
// because that is the only season carrying the CBI and tackle counts;
// prioronly_test.go exists to test that 2018-19 loads without clubs. Forcing
// those onto an allow-list would make the list the norm, and a guard whose
// exemptions outnumber its subjects guards nothing.
//
// The eight here are different because their figures are STAMPED INTO A
// COMMITTED RECORD and quoted beside each other. A mixed population there — half
// six-season, half four, with the constants fingerprint claiming one grid for
// all of it — is the failure this whole model exists to prevent, and it is not
// hypothetical: it is what would have happened had the model half been run under
// FPL_SWEEP_SEASONS=extended before this refactor.
//
// So the rule is narrow and the reason is legible. Widening it later is a
// decision; the wide version was tried and rejected here on purpose.
func TestSnapshotDiagnosticsDeclareTheirSeasons(t *testing.T) {
	// The files behind the eight diagnostics in the snapshot recipe.
	snapshotFiles := []string{
		"calibration_drift_test.go",
		"transfererror_test.go",
		"defcon_bias_test.go",
		"cleansheet_calibration_test.go",
		"sixty_calibration_test.go",
		"teamblend_calibration_test.go",
		"prediction_test.go",
		"nextfive_test.go",
	}

	season := regexp.MustCompile(`"20\d\d-\d\d"`)
	for _, f := range snapshotFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("cannot read %s: %v — if a snapshot diagnostic was renamed, this "+
				"list has to move with it or the guard silently covers nothing", f, err)
			continue
		}
		var hits []string
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if season.MatchString(line) {
				hits = append(hits, strings.TrimSpace(line))
			}
		}
		if len(hits) > 0 {
			t.Errorf("%s names a season directly:\n    %s\n\n"+
				"Its figures are stamped into the accuracy snapshot beside diagnostics "+
				"that derive their grid, so a literal here produces a mixed population "+
				"with nothing in the output saying so. Declare the requirement — "+
				"gridFor(needsDefcon) rather than a season string — and the grid follows.",
				f, strings.Join(hits, "\n    "))
		}
	}
}
