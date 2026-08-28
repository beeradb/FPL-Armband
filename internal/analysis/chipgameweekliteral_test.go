package analysis

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// chipGameweekLiteral matches a chip field assigned a bare integer —
// `Wildcard: 3`, `BenchBoost: 12` — as opposed to one assigned an expression
// such as `up + 1`, `second.Start` or `wc`.
var chipGameweekLiteral = regexp.MustCompile(
	`\b(Wildcard|FreeHit|BenchBoost|TripleCaptain):\s*\d+\s*[,}]`)

// ⚠️ **A test that fetches the LIVE bootstrap may not write a gameweek number
// down.** It bit on 2026-08-28: `TestBenchBoostRaisesBenchWeight` asked for a
// bench boost at a hardcoded GW2, passed for as long as GW2 was ahead, and went
// red the hour that deadline passed — against code nobody had touched.
//
// The failure is worse than a false alarm, because the test was still asserting
// something, just not what it was named for. A chip planned for a gameweek
// already gone correctly changes nothing, so once GW2 was in the past the test
// demanded that a no-op raise the bench weight. It had inverted.
//
// What must stay constant is the RELATIONSHIP to now: `UpcomingGW()+1` is a
// gameweek in the near future in August and still one in March.
//
// ⚠️ **Scoped to files that call `chipEngine`**, the helper that goes to the
// network, because that is exactly the population with a calendar it does not
// control. Files building their own `fpl.Bootstrap` are deliberately untouched:
// they pin their own events, so a literal there is stable by construction and
// forbidding it would be cargo-culting the rule past its reason.
//
// This is the calendar's form of the rot AGENTS.md already records for tests
// pinned to a player or a score — "the underlying data changes weekly, so a test
// pinned to a specific player or score rots within days".
func TestNoLiveChipTestWritesAGameweekDown(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	var scanned int
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		// Only files that reach the live API. `chipEngine` is the one helper in
		// this package that builds a chip-carrying engine from the real
		// bootstrap; if another appears, add it here rather than widening the
		// scan to every file.
		if !strings.Contains(src, "chipEngine(") {
			continue
		}
		scanned++
		for i, line := range strings.Split(src, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, m := range chipGameweekLiteral.FindAllString(line, -1) {
				// A zero is "no chip planned", not a gameweek, and is the normal
				// way to spell an empty slot.
				if strings.Contains(m, " 0") {
					continue
				}
				t.Errorf("%s:%d writes a gameweek down against the live calendar: %q\n\n"+
					"Use `up := upcomingGW(e)` and express it relatively — `up+1`, "+
					"`up+2` — so the assertion means the same thing in every week "+
					"of the season. An absolute number is true for a week and then "+
					"quietly becomes a claim about the PAST, which is a different "+
					"question with a different right answer.\n\n"+
					"If this genuinely is a fixed gameweek — a window boundary, a "+
					"legality case that does not depend on today — say so in a "+
					"comment on the line and move the literal into a named "+
					"variable, or build a synthetic bootstrap instead of using "+
					"chipEngine.", name, i+1, m)
			}
		}
	}

	// ⚠️ A scan that matched nothing because it found no files to read passes
	// while guarding nothing. This is the same vacuity the record warns about
	// for source scans generally, and it is cheap to rule out.
	if scanned == 0 {
		t.Fatal("no test file in this package calls chipEngine, so this guard " +
			"scanned nothing. Either the helper was renamed — update the filter " +
			"above — or the live chip tests are gone and this guard should go " +
			"with them.")
	}
}
