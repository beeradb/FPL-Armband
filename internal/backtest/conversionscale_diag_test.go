package backtest

// The fitted per-(season, position) conversion scale, printed so it can be banked
// beside a run that reads it.
//
//	DIAG=1 scripts/replay -run TestDiagConversionScales -v -timeout 20m
//
// # Why this needs to exist at all
//
// `ConversionScale` (the per-position multiplier turning archived xG and xA into
// the goals and assists FPL actually paid for) is not a config constant, so
// `writeSweepProvenance` does not record it — that sidecar banks `constants_digest`
// and the grid, both of which are inputs a reader can reconstruct from the tree.
// The scale is neither: it is FITTED, from the archive, in sample, and it moves
// with the DATA STATE. `FPL_NO_XG_REPAIR=1` changes which rows carry xG and
// therefore changes every number below.
//
// So a banked cells file that reads the instrument records the run but not the
// instrument, and AGENTS.md's rule is "name the data state or do not quote a
// recorded level". This is how the data state gets named. It is the one part of a
// gate-oracle run that is genuinely unrecoverable later: re-deriving it needs the
// archive in exactly the state it was in, and nothing else in the snapshot pins
// that.
//
// ⚠️ It prints rather than asserts, deliberately. The equality checks belong to
// `TestTheConversionScaleFollowsTheXGRepair` and `TestTheConversionScaleIsSeasonGlobal`,
// which own that question; a second set of expectations here would be one quantity
// with two implementations, which is this record's signature failure.

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestDiagConversionScales(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)

	fmt.Printf("\n=== the fitted conversion scale, per season and position\n")
	fmt.Printf("Goals is realised goals over archived xG; Assists the same for xA.\n")
	fmt.Printf("Fitted IN SAMPLE and season-global, so it carries a data state:\n")
	fmt.Printf("FPL_NO_XG_REPAIR=1 moves every row below. Bank this beside any run\n")
	fmt.Printf("whose figures read the instrument.\n\n")

	// The data state, printed INTO the artefact rather than left to the reader.
	//
	// ⚠️ The first version of this diagnostic told the reader that
	// `FPL_NO_XG_REPAIR=1` moves every row and then printed no indication of
	// whether it was set — so the banked file was indistinguishable from one
	// produced under the switch, in the artefact whose entire purpose is to name
	// the data state. Caught in review.
	//
	// Every FPL_-prefixed variable actually set is printed, rather than a list of
	// the ones that matter. A list here would be a second copy of
	// snapshot.fingerprint's enumeration and would go stale the day a switch is
	// added; "what is set" cannot.
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "FPL_") {
			env = append(env, kv)
		}
	}
	sort.Strings(env)
	if len(env) == 0 {
		fmt.Printf("data state: no FPL_* switch set — shipped defaults\n\n")
	} else {
		fmt.Printf("data state: %s\n\n", strings.Join(env, " "))
	}

	// element_type is FPL's own encoding and appears unglossed in the archive.
	names := map[int]string{1: "GKP", 2: "DEF", 3: "MID", 4: "FWD", 5: "MNG"}

	fmt.Printf("%-9s %-5s %9s %9s\n", "season", "pos", "goals", "assists")
	for _, pair := range pairs {
		s := pair.Cur
		scales := s.ConversionScales()
		types := make([]int, 0, len(scales))
		for pos := range scales {
			types = append(types, pos)
		}
		sort.Ints(types)
		for _, pos := range types {
			nm := names[pos]
			if nm == "" {
				nm = fmt.Sprintf("?%d", pos)
			}
			fmt.Printf("%-9s %-5s %9.4f %9.4f\n",
				s.Name, nm, scales[pos].Goals, scales[pos].Assists)
		}
	}
}
