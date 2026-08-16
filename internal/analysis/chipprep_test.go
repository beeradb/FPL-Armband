package analysis

import (
	"fmt"
	"testing"
)

// Can the transfer search see a chip coming?
//
// It could not, and that is the finding these pin. `XIValue` took a squad and
// nothing else, so a bench worth boosting was priced at zero however much the
// model knew about the boost, and a triple captain had no expression anywhere at
// all. `SuggestBenchWeight` computed the right number on the opening-squad path
// and the transfer path had nowhere to put it.
//
// The failure mode being guarded against is the one this codebase keeps hitting:
// a wired-but-inert option. A credit that reaches the objective but not the
// search, or the search but not the plan builder, returns a byte-identical null —
// indistinguishable in a sweep from a knob that does nothing.

// squadOf is a legal fifteen — 2/5/5/3 — as eleven equals in a 4-4-2 and a bench
// whose scores the caller varies.
//
// The quotas are not decoration. An earlier version of this fixture put six
// midfielders in the eleven, which is not a formation FPL allows, so bestXI
// promoted bench players to fill a legal side and every assertion below measured
// the fixture rather than the code.
func squadOf(benchScores [4]float64) []PlayerMetrics {
	const starter = 5.0
	starters := []string{"GKP", "DEF", "DEF", "DEF", "DEF", "MID", "MID", "MID", "MID", "FWD", "FWD"}
	sq := make([]PlayerMetrics, 0, 15)
	for i, pos := range starters {
		sq = append(sq, PlayerMetrics{
			ID: i + 1, Position: pos, Score: starter, Price: 5, StartShare: 0.9,
			// A club each: NewSquadState counts them, and fifteen players sharing
			// the empty string are fifteen players at one club, which the
			// three-per-club rule refuses every swap against.
			Team: fmt.Sprintf("C%d", i+1),
		})
	}
	// The bench: the reserve keeper and one spare of each outfield position, which
	// is what the four slots of a real fifteen are.
	for i, pos := range [4]string{"GKP", "DEF", "MID", "FWD"} {
		sq = append(sq, PlayerMetrics{
			ID: 100 + i, Position: pos, Score: benchScores[i], Price: 4, StartShare: 0.9,
			Team: fmt.Sprintf("C%d", 100+i),
		})
	}
	return sq
}

// TestTheBenchIsWorthNothingWithoutTheChip is the shipped behaviour, stated so the
// change below is visible as a change. An ordinary transfer decision credits a
// bench player at zero, because that is what FPL pays for a man who does not play.
func TestTheBenchIsWorthNothingWithoutTheChip(t *testing.T) {
	poor := XIValue(squadOf([4]float64{0.1, 0.1, 0.1, 0.1}))
	rich := XIValue(squadOf([4]float64{4, 4, 4, 4}))
	if poor != rich {
		t.Errorf("XIValue reads %.4f for a fodder bench and %.4f for a real one — "+
			"the ordinary transfer objective must not see the bench at all", poor, rich)
	}
}

// TestTheBenchCreditPricesTheBoostWeek — the credit is one week's payment spread
// over the horizon, so a bench four points better across four slots is worth
// exactly 16/H more per gameweek.
func TestTheBenchCreditPricesTheBoostWeek(t *testing.T) {
	const horizon = 5
	credit := ChipCredit{Bench: 1.0 / horizon}

	poor := xiValueForTransfer(squadOf([4]float64{0, 0, 0, 0}), credit)
	rich := xiValueForTransfer(squadOf([4]float64{4, 4, 4, 4}), credit)

	if want := 16.0 / horizon; !nearly(rich-poor, want) {
		t.Errorf("a bench worth 16 more scores %.4f more per gameweek, want %.4f — "+
			"the boost pays once and is amortised over the horizon", rich-poor, want)
	}
	// The eleven is untouched: this adds to the objective rather than reweighting it.
	if plain := XIValue(squadOf([4]float64{0, 0, 0, 0})); !nearly(poor, plain) {
		t.Errorf("a zero-scoring bench moved the objective from %.4f to %.4f", plain, poor)
	}
}

// TestTheBenchCreditPricesTheChipWeekNotTheHorizon is the correction that
// changes what the arm buys.
//
// `FixtureLoad` is matches per gameweek *averaged over the horizon*, so a club
// playing twice in the boost week reads 6/5 = 1.2 on a five-week horizon. The
// first version credited `Score` directly and therefore priced a doubling bench
// player at a fifth of his double — buying a *better* bench rather than a
// *doubling* one, which is not the mechanism the chip pays for.
func TestTheBenchCreditPricesTheChipWeekNotTheHorizon(t *testing.T) {
	const horizon = 5
	credit := ChipCredit{Bench: 1.0 / horizon}

	// Two identical benches, except one club plays twice in the chip's week. Both
	// carry the horizon average that a five-week view would show for a double.
	single := squadOf([4]float64{1, 1, 1, 1})
	double := squadOf([4]float64{1, 1, 1, 1})
	for i := 11; i < 15; i++ {
		single[i].FixtureLoad = 1
		double[i].FixtureLoad = 1.2
	}

	// Without week loads the two are within 20% — the horizon average — which is
	// the behaviour being corrected.
	flat := xiValueForTransfer(double, credit) - xiValueForTransfer(single, credit)

	// With them, the doubling bench is worth twice as much in the week it counts.
	withWeek := credit
	withWeek.WeekLoad = map[string]float64{}
	for i := 11; i < 15; i++ {
		withWeek.WeekLoad[double[i].Team] = 2
	}
	// The single bench's clubs play once.
	singleWeek := credit
	singleWeek.WeekLoad = map[string]float64{}
	for i := 11; i < 15; i++ {
		singleWeek.WeekLoad[single[i].Team] = 1
	}
	sharp := xiValueForTransfer(double, withWeek) - xiValueForTransfer(single, singleWeek)

	if sharp <= flat {
		t.Errorf("a doubling bench is worth %.4f more with the chip week priced and %.4f "+
			"more on the horizon average — the correction did nothing", sharp, flat)
	}
	// The bench sums to 4 points a gameweek; doubling it in one week of five is
	// worth exactly 4/5 more than playing it once.
	if want := 4.0 / horizon; !nearly(sharp, want) {
		t.Errorf("doubling bench credited %.4f, want %.4f", sharp, want)
	}
}

// TestTheCaptainCreditPaysTheArmbandAgain — the triple captain pays the armband
// one further time, and it must be the *shrunk* armband the transfer objective
// already uses rather than a fresh reading of who the captain is. A second
// implementation of the captaincy rule is this record's signature failure.
func TestTheCaptainCreditPaysTheArmbandAgain(t *testing.T) {
	const horizon = 5
	sq := squadOf([4]float64{1, 1, 1, 1})
	// One player the search would captain.
	sq[5].Score = 9

	plain := XIValue(sq)
	tripled := xiValueForTransfer(sq, ChipCredit{Captain: 1.0 / horizon})

	xi, _, _ := bestXI(sq)
	_, armband := xiValueShrunk(xi, captainShrink)
	if want := armband / horizon; !nearly(tripled-plain, want) {
		t.Errorf("the triple captain adds %.4f per gameweek, want %.4f (one more copy "+
			"of the shrunk armband, amortised)", tripled-plain, want)
	}
	if armband <= 0 {
		t.Fatal("no armband in the eleven — the fixture is wrong, not the code")
	}
}

// TestTheCreditsDoNotReachTheSquadBuilder is the confinement check, and it is the
// one that makes a replay measurement readable. Squad construction goes through
// the unexported xiValue and must not see any of this, or HOLD moves and the
// difference stops being attributable to the transfer decision.
func TestTheCreditsDoNotReachTheSquadBuilder(t *testing.T) {
	sq := squadOf([4]float64{1, 2, 3, 4})
	xi, _, _ := bestXI(sq)
	before := xiValue(xi)

	// Anything the transfer path can set, set at once.
	s := NewSquadState(sq)
	s.Chip = ChipCredit{Bench: 1, Captain: 1}
	_ = s.value(sq)

	if after := xiValue(xi); after != before {
		t.Errorf("the squad-building objective read %.6f and now reads %.6f — the chip "+
			"credit has leaked out of the transfer path", before, after)
	}
}

// TestTheChipCreditReachesTheRankedSearches is the wired-but-inert guard. The
// credit is useless unless the search that spends transfers can see it, and the
// bench is exactly where a silent no-op would hide: the ordinary objective is
// indifferent between these two swaps, so a search that ignored the credit would
// return the same ranking and nothing would fail.
func TestTheChipCreditReachesTheRankedSearches(t *testing.T) {
	sq := squadOf([4]float64{1, 1, 1, 1})
	s := NewSquadState(sq)
	s.Chip = ChipCredit{Bench: 0.2}

	// A better bench forward, affordable, and no improvement to the eleven.
	cand := []PlayerMetrics{{ID: 900, Position: "FWD", Score: 3, Price: 4, StartShare: 0.9, Team: "C900"}}

	got := RankSwaps(s, cand, 10)
	if len(got) == 0 {
		t.Fatal("the search found no bench upgrade with the boost credited — the credit " +
			"reaches the objective but not RankSwaps")
	}
	if want := 0.2 * (3 - 1); !nearly(got[0].Gain, want) {
		t.Errorf("bench upgrade gains %.4f, want %.4f", got[0].Gain, want)
	}

	// Without the credit the same swap is worth nothing and must not be offered.
	if plain := RankSwaps(NewSquadState(sq), cand, 10); len(plain) != 0 {
		t.Errorf("the ordinary search offered %d bench swaps worth nothing to the eleven",
			len(plain))
	}
}

// TestTheCaptainCreditReachesTheRankedSearches is the same liveness guard for the
// other channel, and it exists because of what the channel does *not* do.
//
// The credit adds one more copy of the armband, so it cancels exactly on every
// candidate move that leaves the best player alone — which is nearly all of them.
// A near-null on a replay grid is therefore the expected reading and is NOT
// evidence that the knob failed to arrive. This test is what separates the two:
// a move that installs a new captain must be worth strictly more with the credit
// than without it.
func TestTheCaptainCreditReachesTheRankedSearches(t *testing.T) {
	sq := squadOf([4]float64{1, 1, 1, 1})
	s := NewSquadState(sq)
	s.Chip = ChipCredit{Captain: 0.2}

	// A new best player in the eleven, so the armband moves and the credit bites.
	cand := []PlayerMetrics{{ID: 902, Position: "MID", Score: 9, Price: 5, StartShare: 0.9, Team: "C902"}}

	with := RankSwaps(s, cand, 10)
	without := RankSwaps(NewSquadState(sq), cand, 10)
	if len(with) == 0 || len(without) == 0 {
		t.Fatal("no upgrade found in one of the arms — the fixture is wrong, not the code")
	}
	if with[0].Gain <= without[0].Gain {
		t.Errorf("a new captain gains %.4f with the triple captain credited and %.4f "+
			"without — the credit reaches the objective but not RankSwaps",
			with[0].Gain, without[0].Gain)
	}

	// And it cancels where it should: a move that does not touch the armband must
	// be worth the same either way, which is why a replay null is unsurprising.
	xi, _, _ := bestXI(sq)
	_, armband := xiValueShrunk(xi, captainShrink)
	bench := []PlayerMetrics{{ID: 903, Position: "FWD", Score: 3, Price: 4, StartShare: 0.9, Team: "C903"}}
	if got := RankSwaps(s, bench, 10); len(got) != 0 {
		t.Errorf("the captain credit offered %d bench swaps, which cannot change an "+
			"armband of %.2f", len(got), armband)
	}
}

// TestACheaperArrivalCannotRaiseTheObjective is what RankSwaps's monotonicity
// prune rests on, checked directly rather than through the search.
//
// # This replaces a test that passed with the thing it guarded removed
//
// The first version asserted that the prune was *dropped* when the bench credit
// was on, and it was green with the prune fully restored — mutation testing found
// it. The fixture lowered the bench forward and offered a midfielder, so the only
// same-position slots were four starters at 5.0 and a bench midfielder at 1.0;
// the candidate scored 2, which is above the man it replaced, so the prune never
// fired on the slot that produced the gain. The case the test existed for was
// never constructed. That is the repo's own first-look failure — a regression
// test green because it measures nothing — guarding the one line whose comment
// calls it the silent no-op this package keeps paying for.
//
// The exemption itself was also wrong, which is why this asserts the property
// instead of the behaviour: the objective is monotone nondecreasing in every
// player's score for any credit, so a cheaper arrival can never raise it and the
// prune is exact.
func TestACheaperArrivalCannotRaiseTheObjective(t *testing.T) {
	base := squadOf([4]float64{3, 2.5, 2, 1.5})
	// A spread of scores across the eleven, so the armband and the bench ordering
	// both have something to move.
	for i := range base[:11] {
		base[i].Score = 4 + float64(i)*0.4
	}

	credits := []ChipCredit{
		{}, {Bench: 0.2}, {Bench: 1}, {Captain: 0.2}, {Captain: 1}, {Bench: 0.5, Captain: 0.5},
	}
	for _, credit := range credits {
		before := xiValueForTransfer(base, credit)
		for slot := range base {
			// Every same-position arrival strictly worse than the incumbent, at
			// each of several sizes — including one that lands between the bench
			// and the eleven, which is the rearrangement the exemption feared.
			for _, drop := range []float64{0.1, 0.5, 1, 2, 3.9} {
				trial := append([]PlayerMetrics(nil), base...)
				trial[slot].ID = 900 + slot
				trial[slot].Team = "CX"
				trial[slot].Score = base[slot].Score - drop
				if trial[slot].Score < 0 {
					continue
				}
				if after := xiValueForTransfer(trial, credit); after > before+1e-9 {
					t.Fatalf("credit %+v: dropping slot %d (%s) by %.1f RAISED the objective "+
						"%.6f -> %.6f — the prune in RankSwaps is not exact and the search "+
						"is skipping candidates that could win",
						credit, slot, base[slot].Position, drop, before, after)
				}
			}
		}
	}
}
