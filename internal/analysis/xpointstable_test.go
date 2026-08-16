package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestTheXPointsScriptsShareTheScoringTable.
//
// # Why a test rather than a comment
//
// stats/xpoints_common.py prices goals and clean sheets off realised underlying, so
// it needs FPL's per-position points — and it carries its own copy of them, because
// it is Python and the originals are here. That is a second implementation of a
// quantity that already has one, which is this record's signature failure: four in
// the engine, three more in stats/*.R found by the guard written to stop the next
// one, and two of those had already diverged.
//
// This one HAD diverged. GOAL[1] read 6 against goalPoints[1] of 10 — FPL pays a
// keeper ten for a goal and publishes it in game_config, so 6 was the script's own
// invention rather than an older rule it had been left behind by.
//
// ⚠️ **Retracted 2026-08-16 — that last clause is exactly backwards.** 6 is a
// value FPL really paid: a keeper's goal was worth 6 in 2020-21, decoded from the
// archive's only goalkeeper goal. Whether *this script's* 6 descended from that
// rule or was typed is unknown and unrecoverable, so the retraction is of
// "invention" and nothing stronger. See the amendment block below and
// `scoringrules.go`. The rest of this paragraph's argument stands.
//
// # The reason it is worth pinning is the reason nobody caught it
//
// It moved no published figure. There are ZERO keeper goals in all 33,878
// appearances across the three native-xG seasons, so in every row of data this
// project holds, `(goals - xg) * GOAL[1]` multiplied a goals term of zero and the
// two implementations agreed exactly.
//
// A divergence the available data cannot exercise is not a small bug. It is the one
// that survives every check made by running things, waits for the first keeper to
// score, and then moves a figure that has been quoted for months. The comparison has
// to be structural — against the constant, not against the output — which is what
// this does.
//
// goalPoints and cleanSheetPoints are themselves asserted against FPL's own
// published game_config by TestScoringConstantsMatchFPL, so pinning the script to
// them pins it to FPL transitively. One direction of truth, rather than two files
// agreeing with each other about a third thing neither has read.
//
// # ⚠️ Amended 2026-08-16: this pins TODAY'S table, and the Go instrument no
// longer prices every season through it
//
// "6 was the script's own invention" is wrong. **6 is what FPL paid a keeper in
// 2020-21**, decoded from the archive's only goalkeeper goal — see
// `scoringrules.go`. What is retracted is "invention"; where the script's own 6
// came from is unknown. `XPointsResidual` now takes an `ScoringRules` and prices
// each season under its own table, while this script still carries one table for
// every season it is pointed at.
//
// This guard stays as it is, and deliberately: what it protects is the script
// against a drifting copy of the CURRENT constants, which is still the failure it
// was written for. The season awareness the script lacks is **declared and
// sized** in `stats/xpoints_common.py` — one row at the default seasons, worth
// 0.08 residual points, and not zero, because `GOAL[1]` multiplies xG as well as
// goals. Do not read this test as evidence that the two sides agree per season.
// They do not.
//
// Parsed out of the source rather than executed, because `go test` must pass on a
// machine with no Python installed — the same reason stats/cells_reader_selftest.R
// is asserted by existence.
func TestTheXPointsScriptsShareTheScoringTable(t *testing.T) {
	// From internal/analysis. Not via git: this must hold in an export too, and a
	// guard that skips when it cannot find its subject is a guard nobody notices
	// losing.
	path := filepath.Join("..", "..", "stats", "xpoints_common.py")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("stats/xpoints_common.py is unreadable (%v).\n\n"+
			"It is the shared scorer behind every xpoints_*.py figure. If it has "+
			"been renamed, this guard must follow it — a guard that silently stops "+
			"covering its subject is worse than none.", err)
	}

	for _, c := range []struct {
		name  string
		model map[int]float64
	}{
		{"GOAL", goalPoints},
		{"CS", cleanSheetPoints},
	} {
		got, err := parsePyIntFloatMap(string(src), c.name)
		if err != nil {
			t.Errorf("%s in stats/xpoints_common.py: %v\n\n"+
				"This guard reads the literal, so it must stay a literal keyed by "+
				"element_type.", c.name, err)
			continue
		}
		if len(got) != len(c.model) {
			t.Errorf("%s has %d positions in the script and %d in the engine",
				c.name, len(got), len(c.model))
		}
		for pos, want := range c.model {
			have, ok := got[pos]
			if !ok {
				t.Errorf("%s is missing element_type %d, which the engine prices "+
					"at %g", c.name, pos, want)
				continue
			}
			if have != want {
				t.Errorf("%s[%d] is %g in stats/xpoints_common.py and %g in the "+
					"engine.\n\n"+
					"These are one quantity. The engine's copy is asserted against "+
					"FPL's published game_config by TestScoringConstantsMatchFPL, "+
					"so the engine is right by construction and the script is what "+
					"moves. Fix the script; do not relax this test.",
					c.name, pos, have, want)
			}
		}
	}
}

// The scalars beside the maps. These were left unpinned by the first version of
// this guard, which is the same omission it was written to close — the assist
// points sat as a bare `* 3` one line below `GOAL`, in the file whose docstring
// claims one implementation, and the 2026/27 rule discussion makes it a constant
// that can actually move.
func TestTheXPointsScriptsShareTheScalarConstants(t *testing.T) {
	path := filepath.Join("..", "..", "stats", "xpoints_common.py")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("stats/xpoints_common.py is unreadable: %v", err)
	}

	for _, c := range []struct {
		name  string
		model float64
		what  string
	}{
		{"ASSIST", assistPoints, "assist points"},
		// FPL pays the clean sheet only from sixty minutes, which is the same
		// threshold appearancePoints is the long_play value of.
		{"CS_MINUTES", 60, "the clean-sheet minutes threshold"},
	} {
		got, err := parsePyScalar(string(src), c.name)
		if err != nil {
			t.Errorf("%s in stats/xpoints_common.py: %v", c.name, err)
			continue
		}
		if got != c.model {
			t.Errorf("%s (%s) is %g in stats/xpoints_common.py and %g in the "+
				"engine.\n\nOne quantity. Fix the script.",
				c.name, c.what, got, c.model)
		}
	}
}

// parsePyScalar pulls a `NAME = 3.0` literal out of Python source. Strict for the
// same reason as parsePyIntFloatMap: an unreadable constant is an error, never a
// silently skipped comparison.
func parsePyScalar(src, name string) (float64, error) {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) +
		`\s*=\s*([0-9]+(?:\.[0-9]+)?)\s*(?:#.*)?$`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return 0, fmt.Errorf("no `%s = <number>` literal found", name)
	}
	return strconv.ParseFloat(m[1], 64)
}

// parsePyIntFloatMap pulls a `NAME = {1: 10, 2: 6, ...}` literal out of Python
// source. Deliberately strict: anything it cannot read is an error rather than an
// empty map, because an empty map would make every comparison above vacuous and the
// test would pass by measuring nothing.
func parsePyIntFloatMap(src, name string) (map[int]float64, error) {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\s*=\s*\{([^}]*)\}`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return nil, fmt.Errorf("no `%s = {...}` literal found", name)
	}
	out := map[int]float64{}
	for _, pair := range strings.Split(m[1], ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, ":")
		if !ok {
			return nil, fmt.Errorf("entry %q is not `key: value`", pair)
		}
		ki, err := strconv.Atoi(strings.TrimSpace(k))
		if err != nil {
			return nil, fmt.Errorf("key %q is not an int: %v", k, err)
		}
		vf, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fmt.Errorf("value %q is not a number: %v", v, err)
		}
		out[ki] = vf
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("`%s` parsed to an empty map", name)
	}
	return out, nil
}
