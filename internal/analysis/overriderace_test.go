package analysis

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
		if _, _, _, ok := e.minutesOverrideFor(code); ok {
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
	mins, until, confirmed, ok := e.minutesOverrideFor(1234)
	if !ok || mins != 75 || until != 12 || confirmed {
		t.Fatalf("got (%v, %v, %v, %v), want (75, 12, false, true)", mins, until, confirmed, ok)
	}

	// An indefinite override must clear any previous expiry rather than
	// inheriting it — "he is out until GW12" followed by "he plays 80 minutes,
	// indefinitely" must not keep prorating to GW12.
	e.SetMinutesOverride(1234, 80, 0, false)
	mins, until, confirmed, ok = e.minutesOverrideFor(1234)
	if !ok || mins != 80 || until != 0 || confirmed {
		t.Errorf("got (%v, %v, %v, %v), want (80, 0, false, true) — a stale expiry survived an "+
			"indefinite override", mins, until, confirmed, ok)
	}

	e.ClearMinutesOverride(1234)
	if _, _, _, ok := e.minutesOverrideFor(1234); ok {
		t.Error("the override survived being cleared")
	}
	if !e.hasMinutesOverrides() == false {
		t.Error("hasMinutesOverrides is inconsistent with the map")
	}
}

// TestMinutesOverrideConfirmedIsReadWithItsValue pins the third leg of the same
// pair. `MinutesOverrideConfirmed` shipped as a THIRD map guarded by the same
// `overrideMu`, but for one revision it had its own two-line accessor
// (`minutesOverrideConfirmed`) that took and released `overrideMu.RLock()`
// independently of `minutesOverrideFor` — exactly the torn-read shape
// TestMinutesOverrideAndItsExpiryAreReadTogether already exists to catch for
// `until`, just with a third map instead of a second. A caller doing
// `minutesOverrideFor(code)` then separately `minutesOverrideConfirmed(code)`
// could observe one write's minutes value alongside a DIFFERENT write's
// confirmed flag — silently reading a hedge as settled fact, or a settled fact
// as a hedge, which is precisely the distinction Confirmed exists to carry.
//
// The two states below are constructed so minutes and confirmed are only ever
// consistent with each other (75 <-> true, 80 <-> false), so any combination
// straddling the two writes is detectable without needing -race to catch it:
// the accessor under test can produce a torn pair silently, with no data race
// at all, if minutes/until/confirmed are read under separate lock
// acquisitions rather than one.
func TestMinutesOverrideConfirmedIsReadWithItsValue(t *testing.T) {
	e := &Engine{}
	e.SetMinutesOverride(1234, 75, 0, true)

	done := make(chan struct{})
	var wg sync.WaitGroup

	// Writer: the concurrent set_player_status calls a live turn produces,
	// flipping between two internally-consistent states.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for i := 0; i < 5000; i++ {
			e.SetMinutesOverride(1234, 75, 0, true)
			e.SetMinutesOverride(1234, 80, 0, false)
		}
	}()

	// Readers: the exact accessor Engine.minutesCorroborated calls.
	var tornReads int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if mins, _, confirmed, ok := e.minutesOverrideFor(1234); ok {
					if (mins == 75) != confirmed {
						atomic.AddInt32(&tornReads, 1)
					}
				}
			}
		}()
	}
	wg.Wait()

	if tornReads > 0 {
		t.Errorf("%d torn reads: minutes and confirmed came from two different writes — "+
			"they must be read under one lock acquisition, not two", tornReads)
	}
}

// TestEngineAtHorizonReadsOverridesTogether pins the same "read together"
// requirement one level up: not the accessor SetMinutesOverride's own package
// uses internally, but the two call sites that copy all three override maps
// onto a DERIVED engine — WeekEngine (plan.go) and engineAtHorizon
// (weekview.go), the ones `set_player_status`'s writes actually have to cross
// to reach a transfer plan or a week view.
//
// Before the fix, each site did three unguarded field reads —
// `wk.MinutesOverride = e.MinutesOverride`, then `Until`, then `Confirmed` —
// with no lock and no ordering guarantee against SetMinutesOverride's own
// three-map swap under overrideMu. A derived engine could carry one write's
// minutes value paired with a DIFFERENT write's expiry or confirmed flag: the
// exact hazard minutesOverrideFor's doc comment describes, one level further
// out. This test forces that interleaving deterministically, the same way
// TestMinutesOverrideConfirmedIsReadWithItsValue does for the in-package
// accessor: the writer alternates between two states that are only ever
// internally consistent with EACH OTHER (75/12/true, or 80/30/false), so any
// combination straddling the two writes is detectable without needing -race
// to catch it.
func TestEngineAtHorizonReadsOverridesTogether(t *testing.T) {
	e := testEngine(t)
	if e.Boot == nil || len(e.Boot.Elements) == 0 {
		t.Skip("need a bootstrap with players")
	}
	var code int
	for i := range e.Boot.Elements {
		if c := e.Boot.Elements[i].Code; c != 0 {
			code = c
			break
		}
	}
	if code == 0 {
		t.Skip("no player with a code")
	}

	e.SetMinutesOverride(code, 75, 12, true)

	// A fixed iteration count races to finish before the readers — each of
	// which pays for a full NewEngineFull build — ever get scheduled, so
	// nothing overlaps and nothing is caught. Run the writer on a time budget
	// instead, so it is still hammering for the whole window the readers are
	// sampling in, independent of how many iterations that turns out to be.
	stopWriter := make(chan struct{})
	var writerWG sync.WaitGroup
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		for {
			select {
			case <-stopWriter:
				return
			default:
			}
			// Writer: the concurrent set_player_status calls a live turn
			// produces, flipping between two internally-consistent states.
			e.SetMinutesOverride(code, 75, 12, true)
			e.SetMinutesOverride(code, 80, 30, false)
		}
	}()

	// Readers: exactly the derived-engine construction WeekEngine and
	// engineAtHorizon perform, run through the unexported horizon helper both
	// eventually call.
	stopReaders := make(chan struct{})
	var readerWG sync.WaitGroup
	var tornReads, reads int32
	for i := 0; i < 8; i++ {
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
				}
				wk := e.engineAtHorizon(1, 1)
				atomic.AddInt32(&reads, 1)
				mins, ok := wk.MinutesOverride[code]
				if !ok {
					continue
				}
				until := wk.MinutesOverrideUntil[code]
				confirmed := wk.MinutesOverrideConfirmed[code]
				stateA := mins == 75 && until == 12 && confirmed
				stateB := mins == 80 && until == 30 && !confirmed
				if !stateA && !stateB {
					atomic.AddInt32(&tornReads, 1)
				}
			}
		}()
	}

	time.Sleep(500 * time.Millisecond)
	close(stopReaders)
	readerWG.Wait()
	close(stopWriter)
	writerWG.Wait()

	if reads == 0 {
		t.Fatal("no reads happened during the race window — test is not exercising anything")
	}
	if tornReads > 0 {
		t.Errorf("%d of %d reads out of a derived engine were torn: minutes, expiry and "+
			"confirmed came from different SetMinutesOverride writes — WeekEngine/"+
			"engineAtHorizon must copy all three maps under one overrideMu acquisition, "+
			"not three separate field reads", tornReads, reads)
	}
}

// TestWeekEngineCopiesOverridesUnderTheRaceDetector is the -race regression
// for the same bug: run with `go test -race`, it fails on the unfixed
// three-field-read copy in WeekEngine, because a plain field read racing
// SetMinutesOverride's overrideMu-guarded write is exactly what the detector
// is built to catch, independent of whether this run happened to observe a
// torn VALUE.
//
// WeekEngine caches its result on e.weekEngine behind sync.Once, so only the
// FIRST call per Engine instance ever touches the copy — later calls return
// the cached engine without going near e.MinutesOverride again. To keep the
// race window open, each trial gets its own fresh Engine (built from the same
// captured boot/fixtures/weights so this stays fast), with a writer hammering
// SetMinutesOverride throughout the single WeekEngine build that trial makes.
func TestWeekEngineCopiesOverridesUnderTheRaceDetector(t *testing.T) {
	base := testEngine(t)
	if base.Boot == nil || len(base.Boot.Elements) == 0 {
		t.Skip("need a bootstrap with players")
	}
	var code int
	for i := range base.Boot.Elements {
		if c := base.Boot.Elements[i].Code; c != 0 {
			code = c
			break
		}
	}
	if code == 0 {
		t.Skip("no player with a code")
	}

	// Each trial gets its own fresh Engine, because WeekEngine caches its
	// result on e.weekEngine behind sync.Once: only the FIRST call per
	// instance ever touches the override fields, so a shared engine across
	// trials would let only trial 0 exercise anything. The writer is kept
	// running for the ENTIRE WeekEngine() call, not a fixed iteration count —
	// racing a writer that finishes before the reader is even scheduled
	// proves nothing, which is exactly what a fixed count did here first.
	const trials = 15
	for trial := 0; trial < trials; trial++ {
		e := NewEngineFull(base.Boot, base.Fixtures, base.Weights, base.Cong, base.Role)
		e.Priors = base.Priors
		e.Recent = base.Recent

		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func(e *Engine) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				e.SetMinutesOverride(code, 75, 12, true)
				e.SetMinutesOverride(code, 80, 30, false)
			}
		}(e)

		_ = e.WeekEngine()
		close(stop)
		wg.Wait()
	}
}
