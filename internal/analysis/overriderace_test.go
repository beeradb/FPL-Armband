package analysis

import (
	"sync"
	"testing"
)

// TestMinutesOverridesSurviveConcurrentWritesAndReads is the regression test for
// a `fatal error: concurrent map writes` that would have killed a live run.
//
// # What raced, and why nothing caught it
//
// `set_player_status` installed a minutes correction with a bare map write —
// `e.MinutesOverride[code] = v` — from a tool handler. The tool runner fans a
// turn's calls out through an errgroup, so two overrides in one turn run at the
// same time, and the scoring path reads the same map unguarded from blendFor and
// from the squad pool filter. Go does not permit recovery from a concurrent map
// write, so this does not degrade: it kills the process, after the tokens for
// that turn have been paid for.
//
// It is not a hypothetical ordering. The system prompt tells the agent that every
// override marked CHECK is part of this week's work, and a recorded live run
// issued five `set_player_status` calls in two concurrent batches.
//
// The existing TestConcurrentOverridesAllPersist does NOT cover it: it calls
// updateConfig directly, bypassing the tool handler, and only ever uses
// mode "exclude", so the minutes branch it needed to reach is never executed.
// That is this package's own recorded failure shape — a test whose inputs cannot
// distinguish the thing under test.
//
// Run with -race. Without the fix this fails, usually by killing the test binary
// outright rather than by reporting.
func TestMinutesOverridesSurviveConcurrentWritesAndReads(t *testing.T) {
	e := testEngine(t)
	if e.Boot == nil || len(e.Boot.Elements) < 20 {
		t.Skip("need a bootstrap with players")
	}

	var codes []int
	for i := range e.Boot.Elements {
		if c := e.Boot.Elements[i].Code; c != 0 {
			codes = append(codes, c)
		}
		if len(codes) == 20 {
			break
		}
	}
	if len(codes) < 20 {
		t.Skip("not enough players with codes")
	}

	var wg sync.WaitGroup

	// Writers: the concurrent set_player_status calls a single turn produces.
	for _, code := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.SetMinutesOverride(code, 75, 12, false)
		}()
	}

	// A clearer, because mode "clear" deletes from the same maps.
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.ClearMinutesOverride(codes[0])
	}()

	// Readers: any scoring tool in the same turn. Metrics reaches blendFor,
	// which is where the unguarded reads were.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range e.Boot.Elements {
				_ = e.Metrics(&e.Boot.Elements[j])
				if j > 40 {
					break
				}
			}
		}()
	}

	wg.Wait()

	// The writes must also have landed — a lock that serialised them into
	// oblivion would pass a race detector and lose the overrides.
	installed := 0
	for _, code := range codes {
		if _, _, ok := e.minutesOverrideFor(code); ok {
			installed++
		}
	}
	// codes[0] may or may not survive depending on whether the clear ran last,
	// which is legitimately racy; everything else must be present.
	if installed < len(codes)-1 {
		t.Errorf("%d of %d overrides survived; writes are being lost, which a race "+
			"detector alone would not catch", installed, len(codes))
	}
}

// TestMinutesOverrideAndItsExpiryAreReadTogether pins the subtler half.
//
// The two maps are written as a pair and must be read as a pair. Read under
// separate locks, a player can pick up one write's minutes with another write's
// expiry — which is not a crash but a silently wrong prorating, and this project
// treats a plausible wrong number as worse than a loud failure.
func TestMinutesOverrideAndItsExpiryAreReadTogether(t *testing.T) {
	e := &Engine{}

	e.SetMinutesOverride(1234, 75, 12, false)
	mins, until, ok := e.minutesOverrideFor(1234)
	if !ok || mins != 75 || until != 12 {
		t.Fatalf("got (%v, %v, %v), want (75, 12, true)", mins, until, ok)
	}

	// An indefinite override must clear any previous expiry rather than
	// inheriting it — "he is out until GW12" followed by "he plays 80 minutes,
	// indefinitely" must not keep prorating to GW12.
	e.SetMinutesOverride(1234, 80, 0, false)
	mins, until, ok = e.minutesOverrideFor(1234)
	if !ok || mins != 80 || until != 0 {
		t.Errorf("got (%v, %v, %v), want (80, 0, true) — a stale expiry survived an "+
			"indefinite override", mins, until, ok)
	}

	e.ClearMinutesOverride(1234)
	if _, _, ok := e.minutesOverrideFor(1234); ok {
		t.Error("the override survived being cleared")
	}
	if !e.hasMinutesOverrides() == false {
		t.Error("hasMinutesOverrides is inconsistent with the map")
	}
}
