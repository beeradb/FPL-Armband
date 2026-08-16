package main

import (
	"strings"
	"testing"

	"armband/internal/analysis"
)

// The squad table sorts by position, and the bench's substitution order must survive
// that sort as data rather than as row order.
//
// This is the property that breaks silently. FPL uses a bench player when a starter
// records no minutes, **in bench order**, which is why this model's derived slot
// weights are P(one starter blanks), P(two) and P(three) rather than a flat weight —
// the first outfield slot is worth several times the third. Sort the bench by position
// and the queue vanishes from the page, leaving a table that looks like a preference
// ordering and is not one. Nothing errors; the reader simply believes the wrong thing.
//
// So the assertion is not "the rows are in position order" alone. It is that the rows
// are in position order **and** each bench row still says where it sits in the queue.
func TestTheSquadTableKeepsTheBenchQueueThroughThePositionSort(t *testing.T) {
	p := func(id int, name, pos string, score float64) analysis.PlayerMetrics {
		return analysis.PlayerMetrics{ID: id, Name: name, Position: pos, Team: "XXX",
			Price: 5.0, Score: score, ExpectedMinutes: 80, AvgDifficulty: 3}
	}
	// A bench whose FPL order deliberately disagrees with position order: the first
	// substitute is a midfielder, the second a defender. A position sort reverses
	// them, which is exactly the case that must not lose the numbering.
	sq := &analysis.Squad{
		Formation:  "3-4-3",
		StartingXI: []analysis.PlayerMetrics{p(1, "Keeper", "GKP", 4), p(2, "Back", "DEF", 5)},
		Bench: []analysis.PlayerMetrics{
			p(10, "FirstSub", "MID", 3),
			p(11, "SecondSub", "DEF", 2),
			p(12, "ThirdSub", "FWD", 1),
			p(13, "Reserve", "GKP", 0.5),
		},
	}

	var b strings.Builder
	briefSquadTable(&b, sq)
	out := b.String()

	for _, want := range []struct{ player, order string }{
		{"FirstSub", "sub 1"},
		{"SecondSub", "sub 2"},
		{"ThirdSub", "sub 3"},
		{"Reserve", "reserve GK"},
	} {
		var line string
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, want.player) {
				line = l
			}
		}
		if line == "" {
			t.Fatalf("%s is missing from the squad table entirely:\n%s", want.player, out)
		}
		if !strings.Contains(line, want.order) {
			t.Errorf("%s lost its place in the substitution queue — row reads:\n  %s\n"+
				"want it to carry %q. Sorting the bench by position without keeping the "+
				"queue presents an autosub priority as a position grouping.",
				want.player, line, want.order)
		}
	}

	// The reserve keeper is not in the outfield queue and must not be numbered into
	// it: he covers exactly one player, the starting keeper.
	for _, bad := range []string{"sub 4"} {
		if strings.Contains(out, bad) {
			t.Errorf("the reserve keeper was numbered into the outfield queue (%q); he "+
				"covers only the starting keeper", bad)
		}
	}

	// One table, not two.
	if strings.Contains(out, "### Starting XI") || strings.Contains(out, "### Bench") {
		t.Error("the squad is still split across two tables")
	}
	if n := strings.Count(out, "| Pos | Player |"); n != 1 {
		t.Errorf("expected one table header, found %d", n)
	}

	// Position order, across the whole table including the bench.
	rank := map[string]int{"GKP": 0, "DEF": 1, "MID": 2, "FWD": 3}
	seen, benched := -1, false
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "— bench —") {
			seen, benched = -1, true
			continue
		}
		for pos, r := range rank {
			if strings.HasPrefix(l, "| "+pos+" |") {
				if r < seen {
					half := "XI"
					if benched {
						half = "bench"
					}
					t.Errorf("%s row out of position order in the %s: %s", pos, half, l)
				}
				seen = r
			}
		}
	}
}
