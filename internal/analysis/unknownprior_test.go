package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// ⚠️ **The shipped default must be 1, and an absent field must become 1.**
//
// `UnknownPriorShare` is the one weight whose zero value is not "off" — it means
// "tell the optimiser that a player nobody has data on will not play", which is
// the defect the field exists to make measurable. JSON's zero value is 0, so a
// config written before this field existed loads as 0 unless something
// intervenes.
//
// Two things must therefore hold, and only one of them is checked here: the
// DEFAULT is 1 (this test), and `config.Load` BACKFILLS an absent field to the
// default (checked in internal/config, where Load lives). Splitting them is
// deliberate — this package cannot see Load, and a test that reached across
// would be asserting someone else's behaviour from the wrong side.
func TestTheUnknownPriorShareShipsAtOne(t *testing.T) {
	if got := DefaultWeights().UnknownPriorShare; got != 1 {
		t.Errorf("UnknownPriorShare ships at %v; it must be 1. A shipped 0 tells the "+
			"optimiser that every player it has no history for — 122 to 284 a season — "+
			"will play no football at all", got)
	}
}

// The arm the sweep needs: 0 reproduces the pre-fix behaviour exactly, so the
// fix is an attributable difference rather than a cross-commit comparison.
// stubPriors knows nobody, which is exactly the state an unknown player is in:
// present in the bootstrap, absent from last season.
type stubPriors struct{}

func (stubPriors) Get(int) (*PriorPlayer, bool) { return nil, false }

func TestAShareOfZeroReproducesTheOldZeroMinutes(t *testing.T) {
	// A no-prior player in a bootstrap where the rest of the league has minutes,
	// so leagueRates is populated and the fallback is real.
	b := &fpl.Bootstrap{
		Season: "2026-27",
		ElementTypes: []fpl.ElementType{
			{ID: 3, SingularNameShort: "MID"},
		},
		Events: []fpl.Event{{ID: 1, Name: "Gameweek", IsNext: true}},
		Teams:  []fpl.Team{{ID: 1, Name: "Club", ShortName: "CLB", Strength: 3}},
	}
	for i := 0; i < 8; i++ {
		mins := 2000
		if i == 0 {
			mins = 0 // the unknown
		}
		b.Elements = append(b.Elements, fpl.Element{
			ID: i + 1, Code: i + 1, ElementType: 3, Team: 1,
			WebName: "p", NowCost: 50 + 10*i, Status: "a",
			Minutes: mins, Starts: mins / 90,
		})
	}

	full := DefaultWeights()
	full.UnknownPriorShare = 1
	off := DefaultWeights()
	off.UnknownPriorShare = 0

	// ⚠️ Priors must be non-nil or blendRatesCode returns before the pre-season
	// branch is reached at all. That early return is itself a live gap — a
	// no-history player gets zero minutes whenever priors failed to load — but it
	// is a separate path from the one under test here and is noted rather than
	// conflated with it.
	eFull := NewEngine(b, nil, full)
	eFull.Priors = stubPriors{}
	eOff := NewEngine(b, nil, off)
	eOff.Priors = stubPriors{}
	unknown := eFull.Boot.ElementByID(1)

	got := eFull.Metrics(unknown).ExpectedMinutes
	if got <= 0 {
		t.Fatalf("at a share of 1 the unknown player should receive the position's "+
			"league average, not zero; got %v", got)
	}
	if zero := eOff.Metrics(eOff.Boot.ElementByID(1)).ExpectedMinutes; zero != 0 {
		t.Errorf("at a share of 0 the unknown player must read exactly zero, which is "+
			"the behaviour the sweep's baseline arm reproduces; got %v", zero)
	}
}
