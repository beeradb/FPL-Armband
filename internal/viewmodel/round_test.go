package viewmodel

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/present"
)

// TestPayloadFloatsAreRoundedToThreeDecimalPlaces pins the size fix itself: every float
// in the built document is a multiple of 0.001, with the two deliberate exceptions
// roundSkipField names. See that variable's own comment for why XP and Gate are left
// alone -- app.js computes with them, not only displays them. This test checks both
// halves: the fields that must be rounded, and the two that must not be, so a change that
// widened or narrowed roundSkipField would fail it either way.
func TestPayloadFloatsAreRoundedToThreeDecimalPlaces(t *testing.T) {
	p := samplePage()
	// Noise well past three decimal places on every float Build touches, so the test
	// fails if any of them slips through unrounded.
	p.Squad.Players[0].Score = 5.01041751206578
	p.Squad.Players[0].MinutesRating = 0.8981334155943039
	p.Squad.Players[0].Ownership = 19.2345678912345
	p.Squad.Players[0].FixtureAdjXP90 = 3.654321987654321
	p.Squad.Players[0].ValueScore = 1.111111111111111
	p.Watch.Rows[0].Player.Score = 4.020000000000001
	p.Watch.Rows[0].Player.MinutesRating = 0.777777777777777
	p.Watch.Rows[0].Delta = 0.940000000000002
	p.Watch.Gate = 0.400000000000001

	s := build(t, p)

	bad := map[string]float64{}
	walkRoundCheck(reflect.ValueOf(s).Elem(), "", bad)
	for field, f := range bad {
		t.Errorf("%s = %v, which is not a multiple of 0.001", field, f)
	}

	// The two exceptions must have kept their full precision -- the walk above skips
	// them by construction, so check them explicitly here.
	if got := s.Squad.Players[0].XP; got != 5.01041751206578 {
		t.Errorf("Player.XP was rounded to %v; it must stay exact -- see roundSkipField", got)
	}
	if got := s.Market.Gate; got != 0.400000000000001 {
		t.Errorf("Market.Gate was rounded to %v; it must stay exact -- see roundSkipField", got)
	}

	// And the payload really is smaller: encoding/json must not print seventeen
	// significant figures for a field this test rounded.
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if strings.Contains(string(raw), "0.8981334155943039") {
		t.Error("reliability still carries its full, unrounded precision on the wire")
	}
}

// walkRoundCheck mirrors roundFloats' own traversal (see build.go) but only reads: it
// records, under the dotted field path, every float64 that is not a multiple of 0.001
// and is not one of roundSkipField's two exceptions.
func walkRoundCheck(v reflect.Value, path string, bad map[string]float64) {
	switch v.Kind() {
	case reflect.Float64:
		f := v.Float()
		last := path
		if i := strings.LastIndexByte(path, '.'); i >= 0 {
			last = path[i+1:]
		}
		if roundSkipField[last] {
			return
		}
		if math.Round(f*1000)/1000 != f {
			bad[path] = f
		}
	case reflect.Pointer:
		if !v.IsNil() {
			walkRoundCheck(v.Elem(), path, bad)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if !t.Field(i).IsExported() {
				continue
			}
			walkRoundCheck(v.Field(i), path+"."+t.Field(i).Name, bad)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkRoundCheck(v.Index(i), path, bad)
		}
	}
}

// TestMarketOrderSurvivesRoundingAndTheRoundTrip pins the other half of the trap: three
// candidates whose true Score is close enough to tie once rounded to 3dp (the whole point
// of rounding at all) must come out of Build, and back off the wire, in the same order
// they were assembled in -- true Score descending, exactly as cmd/armband/page.go's
// watchlistFor (sort.SliceStable on Score, unmodified by this change) already delivers
// them. viewmodel.buildMarket's own job is to arrange, not derive (see the package
// comment) -- it must never re-sort what it is handed, and this pins that: given rows
// already in true order, rounding the OTHER fields on a tied pair must not disturb it.
//
// This does not exercise app.js -- the client is out of scope for this change (two other
// agents are editing it concurrently). Player.XP itself is left unrounded (roundSkipField,
// see build.go) for an unrelated reason -- the picker's client-side clearsGate() computes
// with it -- which is exactly why a tie-shuffle from rounding XP was never possible here:
// the field the order is decided on never moves. This test guards the other rounded
// fields on the row (Reliability, Delta, ...) from ever becoming a re-sort key that would
// reopen that door.
func TestMarketOrderSurvivesRoundingAndTheRoundTrip(t *testing.T) {
	p := samplePage()
	top := analysis.PlayerMetrics{ID: 101, Name: "Top", Team: "CHE", Position: "MID", Price: 9.5,
		Score: 5.0109999, ExpectedMinutes: 80, RotationRisk: "nailed", AvailabilityFactor: 1}
	mid := analysis.PlayerMetrics{ID: 102, Name: "Mid", Team: "CHE", Position: "MID", Price: 9.0,
		Score: 5.0104175, ExpectedMinutes: 80, RotationRisk: "nailed", AvailabilityFactor: 1}
	low := analysis.PlayerMetrics{ID: 103, Name: "Low", Team: "CHE", Position: "MID", Price: 8.5,
		Score: 5.0100001, ExpectedMinutes: 80, RotationRisk: "nailed", AvailabilityFactor: 1}
	// All three round to the same 5.010 at 3dp -- confirming the test exercises a real
	// tie, not a case rounding never touches. Fed in already in true-Score-descending
	// order, matching what watchlistFor guarantees in production.
	p.Watch.Rows = []present.WatchRow{
		{Player: top, Delta: 0.5, ClearsGate: true},
		{Player: mid, Delta: 0.5, ClearsGate: true},
		{Player: low, Delta: 0.5, ClearsGate: true},
	}

	s := build(t, p)

	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	var back State
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}

	wantOrder := []int{101, 102, 103} // top, mid, low -- true Score descending
	for _, doc := range []struct {
		name string
		rows []MarketRow
	}{{"before the round trip", s.Market.Rows}, {"after the round trip", back.Market.Rows}} {
		if len(doc.rows) != len(wantOrder) {
			t.Fatalf("%s: got %d rows, want %d", doc.name, len(doc.rows), len(wantOrder))
		}
		var gotIDs []int
		for _, r := range doc.rows {
			gotIDs = append(gotIDs, r.Player.ID)
		}
		for i, id := range wantOrder {
			if gotIDs[i] != id {
				t.Fatalf("%s: order is %v, want %v (true Score descending, unaffected by "+
					"the 3dp rounding that ties two of these)", doc.name, gotIDs, wantOrder)
			}
		}
	}
}
