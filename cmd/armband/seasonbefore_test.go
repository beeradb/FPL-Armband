package main

import "testing"

// TestSeasonBeforePreservesTheCallersFormat calls the real `seasonBefore`, which is
// the whole point of it living here.
//
// # It replaces a test that carried its own copy and therefore could not fail
//
// `internal/analysis/priorblend_test.go` used to pin this arithmetic with a local
// re-implementation, under the comment "kept here because the arithmetic is the part
// worth pinning, not the location". That was the standing rule "a diagnostic must
// never carry its own copy of the thing it is checking", and it cost exactly what
// the rule predicts: when `seasonBefore` was consolidated onto
// `backtest.PriorSeasonName`, the real function started emitting the archive's
// two-digit form for a four-digit input and **the test went on passing**, because it
// was still exercising the deleted code.
//
// A test in `cmd/armband` cannot do that: `seasonBefore` is package-private to
// `main`, so the only way to pin it is to call it.
//
// # Why the format matters at all
//
// `priorSeasonName` emits "2025-2026" and the walk feeds its own output back in, so
// a function that answered "2024-25" would produce a list whose first element is in
// one format and the rest in another. The archive uses the string as a cache key and
// a URL path segment, so the wrong one is a 404 — and `LoadBlended` skips a season
// it cannot fetch, which degrades the blend to the single season it exists to
// improve on, silently.
func TestSeasonBeforePreservesTheCallersFormat(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		// Four-digit, which is what priorSeasonName emits and what the walk
		// therefore feeds back into this function.
		{"2025-2026", "2024-2025"},
		{"2001-2002", "2000-2001"},
		// The archive's own two-digit form, passed through unchanged.
		{"2025-26", "2024-25"},
		// The century rollover, where a naive "end - 1" gives "-1".
		{"2000-01", "1999-00"},
		// Unparseable is the empty string, which is how the caller's loop ends.
		{"not a season", ""},
	} {
		if got := seasonBefore(c.in); got != c.want {
			t.Errorf("seasonBefore(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSeasonBeforeWalkStaysInOneFormat is the property the unit cases above imply
// and the regression actually broke: feeding the output back in must not change
// format part-way, because that is what a caller does.
func TestSeasonBeforeWalkStaysInOneFormat(t *testing.T) {
	name := "2025-2026"
	for i := 0; i < 4; i++ {
		name = seasonBefore(name)
		if name == "" {
			t.Fatalf("the walk ended after %d steps; it should reach at least 4", i)
		}
		if len(name) != len("2024-2025") {
			t.Fatalf("step %d gave %q, which is not the four-digit form the walk "+
				"started in. A mixed-format list makes every season after the first "+
				"a 404, and the blend degrades with no error", i+1, name)
		}
	}
}
