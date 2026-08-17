package analysis

import (
	"fmt"
	"testing"

	"armband/internal/fpl"
)

// bandTieFixtures is a round robin whose goals-per-match are chosen to put a
// three-way tie exactly on a band boundary.
//
// Each club scores a constant `rate` in every one of its matches, so its
// goals-per-match *is* that rate and its goals conceded per match is the mean of
// its opponents' — which makes both orderings a function of one number per club
// and the construction easy to reason about.
//
// The rates are 0, 1, 1, 1, 2, 3, 4, 5 for clubs 1..8. The worst three attacks
// are therefore club 1 and **two of clubs 2, 3 and 4**, which are indistinguishable
// on the sort key. That is a boundary tie: it decides band membership rather than
// merely the order inside a band, and it is the only kind that can change what the
// engine scores. Ties wholly inside a band are invisible, which is why a test that
// relies on whatever ties an archive happens to contain has no guaranteed power.
func bandTieFixtures() []fpl.Fixture {
	rate := map[int]int{1: 0, 2: 1, 3: 1, 4: 1, 5: 2, 6: 3, 7: 4, 8: 5}
	var fx []fpl.Fixture
	id := 0
	for h := 1; h <= 8; h++ {
		for a := h + 1; a <= 8; a++ {
			id++
			hs, as := rate[h], rate[a]
			fx = append(fx, fpl.Fixture{
				ID: id, Finished: true,
				TeamH: h, TeamA: a,
				TeamHScore: &hs, TeamAScore: &as,
			})
		}
	}
	return fx
}

// TestBandAssignmentIsDeterministic pins `teamBands` shut, in the mould of
// TestSeedOrderIsDeterministic and for the same reason.
//
// # The defect it closes
//
// `teamBands` ranged a `map[int]*rec` into its candidate slice and ordered that
// slice with `sort.Slice`, which is not stable. Two clubs on the same
// goals-per-match — common early, being a small integer over the same small
// number of matches — were therefore separated by Go's randomised map iteration
// order, and *which* of them landed in the bottom or top three changed from one
// run to the next.
//
// This is the third instance of the class in this repository, after `Optimize`'s
// map-ordered bench fill and `newTeamFormIndex`. The first is pinned by
// TestSeedOrderIsDeterministic; this path had no equivalent, and the recorded
// cost of that was not hypothetical: two replays of one sweep at one commit with
// `band_strength 1` differed in 3 of 36 `hold_points` cells, 12 of 36
// `policy_points`, and 7 each on moves and hits. Decisions, not only scores.
//
// # Why it observes the cause rather than a consequence
//
// It asserts on the band assignment itself, not on a score or a squad that a band
// change may or may not move. TestSeedOrderIsDeterministic's own comment records
// why: the original optimiser reproducer watched the final fifteen, and it stopped
// reproducing mid-investigation when an unrelated pool change reset the landscape,
// leaving a census that returned a clean null from a harness never shown to detect
// anything. Watch the cause and no later change can silence it.
//
// # Why the fixtures are constructed rather than loaded
//
// The boundary tie is built in, so this test is a positive control for itself: the
// guard below fails if the construction ever stops containing the tie it is named
// for. An archive-driven version would pass whenever the season it happened to load
// had no boundary tie — 2024-25 at cutoff 7 is exactly such a season, with interior
// ties and no boundary one — and would then be asserting nothing while looking
// green. It also needs no network and no archive, so it never skips.
func TestBandAssignmentIsDeterministic(t *testing.T) {
	fx := bandTieFixtures()

	// The positive control. Three clubs tie on the sort key at the boundary
	// between the worst three attacks and the middle, so the assignment is
	// underdetermined by the key alone and only a total order can settle it.
	// If this ever stops holding, the test below is vacuous and says so here
	// rather than passing quietly.
	{
		e := &Engine{Fixtures: fx}
		b := e.teamBands()
		if !b.ready {
			t.Fatal("no bands were produced from the constructed fixtures, so this " +
				"test would assert determinism over an empty assignment")
		}
		tied := 0
		for _, id := range []int{2, 3, 4} {
			if b.attack[id] == bandWorst {
				tied++
			}
		}
		if tied != bandSize-1 {
			t.Fatalf("the constructed boundary tie is gone: %d of clubs 2, 3, 4 are in "+
				"the worst attack band, want %d. Without a tie ON a band boundary this "+
				"test cannot detect the defect it exists for.", tied, bandSize-1)
		}
	}

	// Fresh engines, because teamBands caches behind a sync.Once — a second call
	// on one engine returns the first call's answer and would pass whatever the
	// ordering did.
	const runs = 16
	seen := map[string]int{}
	for i := 0; i < runs; i++ {
		e := &Engine{Fixtures: fx}
		b := e.teamBands()
		s := ""
		for id := 1; id <= 8; id++ {
			s += fmt.Sprintf("%d:a%d/d%d;", id, b.attack[id], b.defence[id])
		}
		seen[s]++
	}

	t.Logf("%d identical teamBands calls: %d distinct band assignments", runs, len(seen))

	if len(seen) != 1 {
		t.Errorf("teamBands produced %d distinct band assignments from byte-identical "+
			"fixtures across %d calls, so the attack/defence bands are non-deterministic "+
			"again.\n\n"+
			"Look at how the candidate slice is built and ordered in bands.go. It must "+
			"be a TOTAL order: sort.SliceStable alone does not fix this, because "+
			"stability preserves the input order and the input order is the randomised "+
			"map one. Ties break on club id.\n\n"+
			"This matters beyond one squad. `FPL_WEIGHT=band=1` reaches a live "+
			"`armband review`, so a user can reach it; and every byte-identical "+
			"invariance in the research record is only evidence if what produced it "+
			"was deterministic.", len(seen), runs)
	}
}

// bandSplitFixtures is a round robin whose two ratings are DECOUPLED: the worst
// three attacks and the worst three defences are disjoint sets of clubs.
//
// ⚠️ **That decoupling is the whole point, and bandTieFixtures cannot substitute
// for it.** There every club scores a constant rate, so goals conceded is a
// strictly decreasing function of goals scored and the two orderings are exact
// mirrors — club 1 is simultaneously the worst attack and the worst defence.
// A test asking "does FixtureRunFor read the right SIDE" over those fixtures
// passes whichever side it reads, because both sides give the same answer. This
// was written that way first and confirmed vacuous by flipping the map and
// watching the test still pass.
//
// Here club i scores `i + 2j` against club j, so the two channels come apart:
// goals for work out to `5i + 72` and goals against to `36 + 13i`, both ascending
// in i. Clubs 1-3 are therefore the worst attacks and the BEST defences, and
// clubs 6-8 the best attacks and the WORST defences.
func bandSplitFixtures() []fpl.Fixture {
	var fx []fpl.Fixture
	id := 0
	for h := 1; h <= 8; h++ {
		for a := h + 1; a <= 8; a++ {
			id++
			hs, as := h+2*a, a+2*h
			fx = append(fx, fpl.Fixture{
				ID: id, Finished: true,
				TeamH: h, TeamA: a,
				TeamHScore: &hs, TeamAScore: &as,
			})
		}
	}
	return fx
}

// TestFixtureRunReadsTheSameBandSideTheEngineDoes pins the correspondence between
// the mediator's position-to-band-side map and the one the scoring path applies.
//
// # The shape this guards
//
// `FixtureRunFor` picks `b.attack` for keepers and defenders and `b.defence` for
// everyone else. `attackBandAdj` reads `b.defence` (an attacker wants a leaky
// defence) and `defenceBandAdj` reads `b.attack` (a defender wants a blunt
// attack). Those are two expressions of one mapping, in two files, with nothing
// forcing them to agree — the desynchronised-mirror shape this repository has
// already paid for twice on `baseXP90` and `fixtureSensitivePart`.
//
// A drift here is silent in the worst way: the mediator would report moves toward
// the better run while the engine priced the opposite, and the column would look
// entirely healthy.
//
// # What it does NOT claim
//
// It pins WHICH side each position reads, not that the mediator sees everything
// the engine does. It does not, and `FixtureRunFor`'s own comment says so:
// `fixtureAdjustedXP90` applies BOTH multipliers to EVERY position through
// `fixtureSensitiveAt`, so a defender's goals and assists are re-priced by the
// opponent's defence band and this counter cannot see that channel at all.
func TestFixtureRunReadsTheSameBandSideTheEngineDoes(t *testing.T) {
	e := &Engine{Fixtures: bandSplitFixtures()}
	e.Weights.Horizon = 8
	b := e.teamBands()
	if !b.ready {
		t.Fatal("no bands from the constructed fixtures")
	}

	// The positive control for the decoupling. Club 1 must be a WORST attack and a
	// BEST defence, and club 8 the reverse — otherwise the two band sides agree and
	// this test passes whichever one FixtureRunFor reads, which is exactly how it
	// was first written and exactly why it proved nothing.
	if b.attack[1] != bandWorst || b.defence[1] != bandBest ||
		b.attack[8] != bandBest || b.defence[8] != bandWorst {
		t.Fatalf("the two band ratings are not decoupled: club 1 is attack %d / "+
			"defence %d and club 8 is attack %d / defence %d, want worst/best and "+
			"best/worst. With them coupled, both sides give the same answer and this "+
			"test cannot detect a swapped map.",
			b.attack[1], b.defence[1], b.attack[8], b.defence[8])
	}
	for _, c := range []struct {
		name     string
		position int
		// adj is the engine's multiplier for a player of this position facing the
		// opponent; wantTarget says the mediator should count that opponent as a
		// target, which is the fixture the player wants.
		adj        func(opponent int) float64
		opponent   int
		wantTarget bool
	}{
		// Clubs 6-8 are the leakiest defences AND the best attacks; clubs 1-3 the
		// meanest defences AND the bluntest attacks. So the forward and the defender
		// want OPPOSITE opponents here, which is the whole reason these fixtures
		// decouple the two ratings and bandTieFixtures does not.
		{"forward facing the leakiest defence", 4,
			func(o int) float64 { return e.attackBandAdj(o, 1) }, 8, true},
		{"forward facing the meanest defence", 4,
			func(o int) float64 { return e.attackBandAdj(o, 1) }, 1, false},
		{"defender facing the bluntest attack", 2,
			func(o int) float64 { return e.defenceBandAdj(o, 1) }, 1, true},
		{"defender facing the sharpest attack", 2,
			func(o int) float64 { return e.defenceBandAdj(o, 1) }, 8, false},
	} {
		// The engine's verdict, as a direction. attackBandAdj scales returns UP for
		// a good fixture; defenceBandAdj scales goals conceded DOWN for one — so
		// "the engine likes this fixture" is `> 1` for an attacker and `< 1` for a
		// defender, which is exactly the asymmetry a naive mirror gets backwards.
		m := c.adj(c.opponent)
		engineLikes := m > 1
		if c.position == 1 || c.position == 2 {
			engineLikes = m < 1
		}
		if engineLikes != c.wantTarget {
			t.Errorf("%s: the engine's multiplier is %.4f, which reads as "+
				"favourable=%v, but the case expects %v", c.name, m, engineLikes,
				c.wantTarget)
		}

		// And the mediator's verdict on the same opponent, read THROUGH
		// FixtureRunFor rather than by re-deriving its map here.
		//
		// ⚠️ That distinction is the test. An earlier version looked the band up in
		// `b` with a copy of FixtureRunFor's position rule, and it passed with the
		// real rule inverted — a diagnostic carrying its own copy of the thing it
		// checks, which this project's standing rules forbid outright. Verified by
		// swapping the sides in bands.go and watching it stay green.
		//
		// The club is given exactly one upcoming fixture, against the opponent under
		// test, so Target/Avoid read out as a clean 1/0.
		const club = 5
		e.byTeamUpcoming = map[int][]FixtureBrief{
			club: {{Event: 10, OpponentID: c.opponent, Difficulty: 3}},
		}
		run := e.FixtureRunFor(club, 1, c.position)
		if run.Fixtures != 1 {
			t.Fatalf("%s: the run window held %d fixtures, want exactly 1 — the "+
				"assertion below reads Target and Avoid as a 1/0 flag", c.name, run.Fixtures)
		}
		if got := run.Target == 1; got != c.wantTarget {
			t.Errorf("%s: FixtureRunFor reports target=%v (Target %d, Avoid %d) where "+
				"the engine's multiplier %.4f says %v.\n\n"+
				"FixtureRunFor's position-to-band-side map has drifted from "+
				"attackBandAdj/defenceBandAdj. The mediator would then report moves "+
				"toward the better run while the engine priced the opposite, and the "+
				"column would look healthy throughout.",
				c.name, got, run.Target, run.Avoid, m, c.wantTarget)
		}
	}
}

// TestTheBandChannelIsNotLiveUnderMagnitudeDifficulty pins the guard that stops
// the mediator reporting a bypassed lever as a live one.
//
// `fixtureMultipliersFor` returns the magnitude multipliers and never calls
// attackBandAdj or defenceBandAdj when `magnitudeDifficulty` is set, so
// `BandStrength` reaches nothing at any value — while the bands themselves still
// compute from finished fixtures. A mediator gated on "are the ratings ready"
// would therefore populate all five columns with plausible counts off a lever
// that could not act, which is the inversion the block exists to prevent.
//
// The package var is restored with a defer rather than left set, because it is
// process-global and every other test in this package scores through the path it
// governs.
func TestTheBandChannelIsNotLiveUnderMagnitudeDifficulty(t *testing.T) {
	e := &Engine{Fixtures: bandTieFixtures()}
	e.Weights.Horizon = 8

	if !e.BandChannelLive() {
		t.Fatal("the band channel is not live on the constructed fixtures, so this " +
			"test cannot show the magnitude switch turning it off")
	}
	if run := e.FixtureRunFor(1, 8, 4); !run.Ready {
		t.Fatal("FixtureRunFor reports not-ready before the switch is set")
	}

	defer func(prev bool) { magnitudeDifficulty = prev }(magnitudeDifficulty)
	magnitudeDifficulty = true

	if e.BandChannelLive() {
		t.Error("BandChannelLive is true under magnitudeDifficulty, where " +
			"fixtureMultipliersFor returns before consulting the bands at all. The " +
			"fixture-run mediator gates on this, so every one of its columns would " +
			"populate off a lever that reaches nothing — a flat band arm would then " +
			"read as 'the policy had the opportunity and declined'.")
	}
	if run := e.FixtureRunFor(1, 8, 4); run.Ready || run.Fixtures != 0 {
		t.Errorf("FixtureRunFor returned ready=%v with %d fixtures under "+
			"magnitudeDifficulty; a run the engine could not price must report "+
			"not-ready", run.Ready, run.Fixtures)
	}
}

// TestBandTiesBreakTowardTheLowerClubID pins the tie-break itself, which is the
// one part of the ordering a reader cannot infer from the band sizes.
//
// Separate from the test above on purpose: that one asserts the invariant that
// matters (the same input gives the same answer), this one records WHICH total
// order was chosen. Any total order would satisfy determinism, so changing this
// one is a legitimate decision — but it re-bands clubs, so it must be a decision
// rather than a side effect of an edit, and this test is what makes it one.
func TestBandTiesBreakTowardTheLowerClubID(t *testing.T) {
	e := &Engine{Fixtures: bandTieFixtures()}
	b := e.teamBands()

	// Clubs 2, 3 and 4 all score exactly one goal a match. Two of the three join
	// club 1 in the worst attack band; the lower ids win.
	if b.attack[2] != bandWorst || b.attack[3] != bandWorst || b.attack[4] != bandMiddle {
		t.Errorf("clubs 2, 3, 4 tie on goals per match and landed attack bands "+
			"%d/%d/%d; want the two lowest ids (2 and 3) in the worst band and 4 in "+
			"the middle. The tie-break is club id, ascending.",
			b.attack[2], b.attack[3], b.attack[4])
	}
}
