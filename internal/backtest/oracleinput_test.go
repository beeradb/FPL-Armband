package backtest

// Tier 1: what an information oracle is allowed to change about the model's view
// of the world, checked field by field.
//
//	go test ./internal/backtest -run TestInfoOracle -v
//
// # Why this tier exists at all
//
// Falsification here is roughly two orders of magnitude cheaper than
// confirmation. Establishing what an oracle is *worth* costs a 24-cell sweep and
// an effect large enough to clear this harness's own detection threshold;
// establishing that it perturbed something it had no business perturbing costs
// one field comparison and no replay whatsoever.
//
// Neither existing oracle had this guarantee. The price oracle had a cell-level
// invariance that was checked by reading a column by eye, and the availability
// oracle had nothing at all — its correctness rested on the fact that its
// implementation was three lines and somebody had read them.
//
// # It matters most for the oracle that does not exist yet
//
// Minutes multiply into every per-90 rate, so a minutes oracle that rewrote a
// rate as well as the minutes would quietly stop bounding "perfect rotation
// knowledge" and start bounding "knowing who will score" — a completely
// different and far larger quantity, reported under the first name. No amount of
// care at the call site catches that as reliably as a field diff, and the field
// diff costs seconds.
//
// # The mediator
//
// A declaration that a field *may* differ is only half the check. An oracle whose
// declared field never actually differs is wired and inert, which produces a
// clean null indistinguishable from a real one — this project's signature
// failure. So each oracle that declares a field must be observed moving it at
// least once across the grid, and one that declares none must leave the
// bootstrap byte-identical.

import (
	"context"
	"reflect"
	"testing"
	"time"

	"armband/internal/fpl"
)

// tierOneCacheDir is the archive the input diff reads.
//
// The repository cache rather than configPath's absolute path: this test is about
// the *data* an oracle perturbs, not about the shipped constants, so it needs no
// config at all and should run on any machine that has the archive.
const tierOneCacheDir = "../../.cache/fpl"

// loadForInputDiff loads a season, skipping rather than failing when the archive
// is out of reach — the same contract TestLoadArchive uses.
func loadForInputDiff(t *testing.T, name string) *Season {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	s, err := Load(ctx, tierOneCacheDir, name)
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	return s
}

// tierOneCases is every information oracle the input diff covers.
//
// Package-level and named so TestEveryInformationOracleHasAnInputDiff can count
// it against the implemented set. A new bit whose author forgets this list would
// otherwise ship with no Tier 1 guarantee and nothing to say so — and the Tier 1
// guarantee is the cheap one, so skipping it is the easy mistake.
var tierOneCases = []struct {
	name   string
	oracle InfoOracle

	// compose is the rest of the arm this case has to be run inside, for a bit
	// that is meaningless alone.
	//
	// OracleTeamNewsChance is the only such bit: availabilityFactor prefers the
	// percentage to the flag, so Validate refuses it without the real status
	// underneath. The *mediator* still checks only this case's own declared
	// fields, so composing does not let one bit pass on another's behaviour.
	compose InfoOracle

	// news is the recovered-team-news source, for the bits that need one. A
	// deliberately synthetic one: Tier 1 asks which fields an oracle may perturb,
	// which is a property of the seam and not of the data.
	news TeamNews
}{
	// Declares Status. It carries a final status of "u" back to the start of the
	// season for a player who never appeared, and must touch nothing else — not
	// his price, not his accumulated minutes, not a fixture result.
	{"availability", OracleAvailability, 0, nil},
	// Declares nothing. It perturbs the *transaction* seam — what the wallet pays
	// in decide — and not the information seam, so the bootstrap the optimiser is
	// quoted from must come back identical. That is the distinction the design
	// calls an honest complication, stated as a test rather than left as an
	// accident of where the hook went.
	{"transaction price", OracleTransactPrice, 0, nil},
	// Declares nothing, and that is the *strong* claim rather than a shrug. Minutes
	// on the bootstrap are the denominator of every counting rate — Bonus90,
	// Saves90, the cards, defensive contribution, and the rate blend's own evidence
	// weight — so an oracle that rewrote them there would divide every per-90 rate
	// by the same factor and quietly stop bounding "perfect rotation knowledge",
	// starting instead to bound "knowing who will score". This case asserts it does
	// not go near them; TestMinutesOraclePerturbsOnlyMinutes covers the seam it
	// does perturb.
	{"minutes", OracleMinutes, 0, nil},
	// Declares nothing, for the same reason and at the same seam. It carries the
	// coarser half of the same quantity — who is picked, priced at conditional
	// averages — and travels down MinutesPerMatch because nothing in the shipped
	// model reads StartShare at all, so a StartShare arm would be inert. That
	// makes the bootstrap claim identical to the minutes oracle's and just as
	// load-bearing.
	{"lineups", OracleLineups, 0, nil},
	// Declares Status, the same single field OracleTeamNews writes — and needing
	// no source, which is the whole difference between them. Team news replays
	// what FPL actually published at each deadline; this replays what turned out
	// to be true, so it is the upper bound the recovered feed is measured against.
	//
	// What this case pins is that it does NOT touch minutes. An oracle that said
	// "he plays" by editing his accumulated minutes would be a rate oracle wearing
	// an availability label, and the point of this bit is that it carries one
	// binary and no quantity.
	{"features", OracleFeatures, 0, nil},
	// Declares the whole aggregate block, which is the only case here where a
	// long list is the *point*. What matters is what the list leaves out — price,
	// availability, club, position — because an omniscient arm that moved one of
	// those would be a composition of oracles reported as a single one, and the
	// bitmask exists so a composition has to say so.
	{"omniscient", OracleOmniscient, 0, nil},
	// Declares Status, the same single field the reconstruction writes — because it
	// *replaces* the reconstruction rather than adding to it. What the declaration
	// excludes is the load-bearing half: a recovered payload carries prices,
	// element ids and registration state, and an oracle that let any of those
	// through would be a data backfill reported as a team-news bound.
	{"team news", OracleTeamNews, 0, tierOneNews},
	// Declares ChanceOfPlayingNextRound, which nothing in the replay has ever set —
	// so this is the one case whose baseline value is nil for every player in every
	// gameweek, and the mediator is the only thing standing between "the percentage
	// reached the model" and "the field is still nil and the arm is a null".
	{"team news chance", OracleTeamNewsChance, OracleTeamNews, tierOneNews},
}

// tierOneNews is a synthetic team-news source: it flags every seventh player code
// as doubtful with a published 25% chance, in every gameweek of every season.
//
// Synthetic on purpose. Tier 1 asks *which fields* an oracle may perturb, which is a
// property of the seam rather than of the payload — and a case wired to the real
// captures would fail on any machine whose checkout lacks them, turning the cheap
// guarantee into one that silently does not run. The real data is exercised by the
// sweep diagnostic, which is where a coverage gap is a finding rather than a skip.
//
// A pointer, because Oracles is compared with != in runPolicySweep and a
// non-comparable dynamic type would panic there rather than fail a check.
var tierOneNews = &fakeTeamNews{every: 7, status: "d", chance: 25}

type fakeTeamNews struct {
	every  int
	status string
	chance int
}

func (f *fakeTeamNews) Covers(string, int) bool { return true }

func (f *fakeTeamNews) FlagAt(_ string, _, code int) (string, *int, bool) {
	if code%f.every != 0 {
		return "a", nil, true
	}
	c := f.chance
	return f.status, &c, true
}

// TestInfoOraclePerturbsOnlyItsDeclaredFields runs PointInTime against
// PointInTimeWith over every season pair in the grid and every gameweek,
// including the pre-season view, and fails if anything outside the declared set
// differs.
func TestInfoOraclePerturbsOnlyItsDeclaredFields(t *testing.T) {
	if testing.Short() {
		t.Skip("reads the season archive")
	}
	for _, c := range tierOneCases {
		t.Run(c.name, func(t *testing.T) {
			// Every field any bit in the run may perturb is allowed, and only the
			// case's own are required to move. Otherwise a composed case would
			// either fail on its partner's legitimate change or let one bit pass
			// its mediator on the other's behaviour.
			allowed := map[string]bool{}
			for _, f := range c.oracle.bootstrapFields() {
				allowed[f] = true
			}
			for _, f := range c.compose.bootstrapFields() {
				allowed[f] = true
			}
			required := c.oracle.bootstrapFields()
			run := Oracles{Info: c.oracle | c.compose, News: c.news}
			moved := map[string]int{}

			for _, pair := range sweepPairNames() {
				prior := loadForInputDiff(t, pair[0])
				cur := loadForInputDiff(t, pair[1])
				// 0 is the pre-season view, which PointInTimeWith delegates to
				// PreSeasonWith — the seam's second half, and the one an opening
				// fifteen is chosen from. Leaving it out would leave the oracle
				// unchecked exactly where it does most of its work.
				for gw := 0; gw <= 38; gw++ {
					wantBoot, wantFx := PointInTime(cur, prior, gw)
					gotBoot, gotFx := PointInTimeWith(cur, prior, gw, run)
					diffBootstraps(t, wantBoot, gotBoot, wantFx, gotFx,
						allowed, moved, pair[1], gw)
				}
			}

			// The mediator. A declared field that never moves means the oracle is
			// wired and inert, and a sweep of it would report a clean null.
			//
			// The other direction — an oracle that declares nothing and changes
			// something — needs no assertion here: with an empty `allowed` set,
			// diffBootstraps fails on the first difference it finds, which is
			// exactly the claim the price oracle makes about this seam.
			for _, f := range required {
				if moved[f] == 0 {
					t.Errorf("%s declares it may change %s and never did across "+
						"the whole grid — an oracle that changes nothing measures "+
						"nothing, and reports it as a null result", c.name, f)
				}
			}
		})
	}
}

// diffBootstraps compares two reconstructions field by field.
//
// Reflection rather than a hand-written field list, deliberately: a hand-written
// list silently stops covering the field somebody adds to fpl.Element next, and
// this is a guard against exactly the kind of change nobody thinks to re-check.
func diffBootstraps(t *testing.T, want, got *fpl.Bootstrap, wantFx, gotFx []fpl.Fixture,
	allowed map[string]bool, moved map[string]int, season string, gw int) {
	t.Helper()

	// The fixture list carries results, and an information oracle that touched it
	// would be handing the model matches that had not been played — the one leak
	// this package guards hardest.
	if !reflect.DeepEqual(wantFx, gotFx) {
		t.Fatalf("%s GW%d: the fixture list moved; an information oracle may not "+
			"touch results", season, gw)
	}
	if !reflect.DeepEqual(want.Teams, got.Teams) {
		t.Fatalf("%s GW%d: team data moved", season, gw)
	}
	if !reflect.DeepEqual(want.Events, got.Events) {
		t.Fatalf("%s GW%d: the gameweek list moved", season, gw)
	}
	if len(want.Elements) != len(got.Elements) {
		t.Fatalf("%s GW%d: %d players against %d — an oracle may correct what is "+
			"known about a player, never whether he exists",
			season, gw, len(want.Elements), len(got.Elements))
	}

	for i := range want.Elements {
		a, b := reflect.ValueOf(want.Elements[i]), reflect.ValueOf(got.Elements[i])
		typ := a.Type()
		for f := 0; f < typ.NumField(); f++ {
			name := typ.Field(f).Name
			if a.Field(f).Interface() == b.Field(f).Interface() {
				continue
			}
			if !allowed[name] {
				t.Fatalf("%s GW%d: element %d field %s changed from %v to %v, "+
					"which this oracle did not declare. Either the declaration is "+
					"stale or the oracle is measuring something other than what "+
					"its name says", season, gw, want.Elements[i].ID, name,
					a.Field(f).Interface(), b.Field(f).Interface())
			}
			moved[name]++
		}
	}
}
