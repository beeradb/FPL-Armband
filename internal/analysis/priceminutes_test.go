package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// priceEngine is a bootstrap with a known price spread inside one position, so
// percentiles are checkable by hand rather than by re-deriving the formula.
func priceEngine(w float64) *Engine {
	b := &fpl.Bootstrap{
		Season: "2026-27",
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
	}
	// Five midfielders spanning 4.0m to 12.0m, and five defenders all at 4.0m so
	// a fully-tied position can be checked too.
	for i, cost := range []int{40, 55, 70, 95, 120} {
		b.Elements = append(b.Elements, fpl.Element{
			ID: i + 1, Code: i + 1, ElementType: 3, Team: 1,
			WebName: "mid", NowCost: cost, Status: "a",
		})
	}
	for i := 0; i < 5; i++ {
		b.Elements = append(b.Elements, fpl.Element{
			ID: 100 + i, Code: 100 + i, ElementType: 2, Team: 1,
			WebName: "def", NowCost: 40, Status: "a",
		})
	}
	wt := DefaultWeights()
	wt.PriceMinutesPrior = w
	return NewEngine(b, nil, wt)
}

// ⚠️ **The lever must ship inert.** A default that changed anything would move
// every recorded figure at once and attribute it to whatever else was in the
// same run.
func TestThePriceTiltIsExactlyOneWhenOff(t *testing.T) {
	e := priceEngine(0)
	for _, id := range []int{1, 3, 5, 100} {
		el := e.Boot.ElementByID(id)
		if got := e.priceMinutesTilt(el); got != 1 {
			t.Errorf("player %d: tilt %v with the lever off; it must be exactly 1", id, got)
		}
	}
	if DefaultWeights().PriceMinutesPrior != 0 {
		t.Error("the shipped default must be 0, or this lever is live without a measurement")
	}
}

// ⚠️ **Centred on the position's median.** The tilt reorders; it must not
// inflate. If the average moved, the arm would measure a change to the league
// fallback rather than a ranking.
func TestThePriceTiltIsCentredSoItReordersWithoutInflating(t *testing.T) {
	const w = 0.5
	e := priceEngine(w)

	mid := e.Boot.ElementByID(3) // the median of five distinct prices
	if got := e.priceMinutesTilt(mid); math.Abs(got-1) > 1e-9 {
		t.Errorf("the median-priced player must be unchanged; got %v", got)
	}

	cheap := e.priceMinutesTilt(e.Boot.ElementByID(1))
	dear := e.priceMinutesTilt(e.Boot.ElementByID(5))
	if cheap >= 1 || dear <= 1 {
		t.Fatalf("the cheapest must tilt down and the dearest up; got %v and %v", cheap, dear)
	}
	// Symmetric about 1: what the top gains, the bottom loses.
	if d := (dear - 1) + (cheap - 1); math.Abs(d) > 1e-9 {
		t.Errorf("the tilt is not symmetric about 1: %v up against %v down", dear-1, 1-cheap)
	}
	// And bounded by the weight, so a knob of 0.5 cannot double anyone.
	if dear > 1+w+1e-9 || cheap < 1-w-1e-9 {
		t.Errorf("tilt escaped +/-%v: %v and %v", w, dear, cheap)
	}
}

// ⚠️ **Ties share a percentile.** FPL prices cluster hard at 4.0m, and splitting
// tied players by index would invent an ordering the price does not contain and
// hand it to an argmax.
func TestTiedPricesGetTheSameTilt(t *testing.T) {
	e := priceEngine(0.5)
	want := e.priceMinutesTilt(e.Boot.ElementByID(100))
	for _, id := range []int{101, 102, 103, 104} {
		if got := e.priceMinutesTilt(e.Boot.ElementByID(id)); got != want {
			t.Errorf("player %d is priced identically to 100 and got a different tilt: "+
				"%v against %v", id, got, want)
		}
	}
	// A position where every price is equal has no ordering to express, so every
	// member sits at the median and the tilt is 1.
	if math.Abs(want-1) > 1e-9 {
		t.Errorf("a fully tied position must tilt nobody; got %v", want)
	}
}

// ⚠️ **Percentile within POSITION, not across the league.** A league-wide
// percentile would read every goalkeeper as cheap and tilt a whole position
// down, which is a systematic change to one position rather than a ranking
// inside it.
func TestThePercentileIsWithinThePositionNotTheLeague(t *testing.T) {
	e := priceEngine(0.5)
	// The 4.0m defenders are the cheapest players in the whole bootstrap, but
	// within their own position they are all median — so they must not be tilted
	// down as they would be on a league-wide ranking.
	if got := e.priceMinutesTilt(e.Boot.ElementByID(100)); math.Abs(got-1) > 1e-9 {
		t.Errorf("a 4.0m defender in an all-4.0m position is median for his position "+
			"and must tilt by 1; got %v — a league-wide percentile would push him down", got)
	}
}
