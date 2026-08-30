package backtest

import (
	"sort"
	"testing"
)

// The share conversion has to survive contact with each season's archive, not
// just satisfy its own arithmetic.
//
// The identity `entries = sum(selected)/15` is exact only if the archive carries
// every player's row. Where it does not, the sum is short, the implied entry
// count is low and every percentage is inflated by the same factor — and nothing
// about that failure looks wrong from inside the function. So this checks the
// two quantities that have known real-world magnitudes: how many entries the
// season implies, and what the most-owned player in it comes out at.
func TestOwnershipSharesAreAPlausiblePercentage(t *testing.T) {
	cfg := loadConfig(t)

	for _, pair := range sweepPairNames() {
		cur := loadSeason(t, cfg, pair[1])
		reg := registeredBy(cur, 10)
		own := ownershipAt(cur, 10, reg)

		if len(own.pct) == 0 {
			t.Errorf("%s: no ownership at all at GW10. Either the archive has no "+
				"`selected` column for this season — in which case say so here — or "+
				"the seam has come unwired and every replayed player is 0%% owned, "+
				"which is the state this file was written to end.", pair[1])
			continue
		}

		// FPL has run between roughly 3.5m and 11m entries across the archived
		// span. An implied count outside an order of magnitude either side means
		// the denominator is wrong, not that the game changed size.
		if own.entries < 1_000_000 || own.entries > 20_000_000 {
			t.Errorf("%s: implied %d entries at GW10, which is not a number of "+
				"people who have ever played FPL. sum(selected)/15 is only an "+
				"identity while the archive carries every player's row.",
				pair[1], own.entries)
		}

		var shares []float64
		var sum float64
		for _, v := range own.pct {
			shares = append(shares, v)
			sum += v
		}
		sort.Float64s(shares)
		top := shares[len(shares)-1]

		// Every share adds to 1500%: fifteen players per entry, each counted once.
		// This is the identity restated on the output side, and it catches a
		// normalisation applied to the wrong denominator.
		if sum < 1400 || sum > 1600 {
			t.Errorf("%s: shares sum to %.0f%%, not ~1500%%. Fifteen picks per "+
				"entry means the percentages must total fifteen hundred.", pair[1], sum)
		}

		// The most-owned player in a season is a genuine template pick and lands
		// somewhere between a third and near-universal. Under 10% would mean the
		// denominator is far too large; over 100% is arithmetically impossible.
		if top < 10 || top > 100 {
			t.Errorf("%s: most-owned player at %.1f%%, which is not what the top of "+
				"an FPL ownership table looks like", pair[1], top)
		}

		t.Logf("%s: %d entries implied, top ownership %.1f%%, %d players priced",
			pair[1], own.entries, top, len(own.pct))
	}
}

// Ownership is a level read at one gameweek, not a total accumulated across
// them. The parser records why: a double gameweek repeats the current owner
// count on each row, so summing reports twice as many managers as exist.
func TestOwnershipIsALevelNotASum(t *testing.T) {
	cfg := loadConfig(t)
	cur := loadSeason(t, cfg, sweepPairNames()[0][1])
	reg := registeredBy(cur, 10)

	at5 := ownershipAt(cur, 5, reg)
	at10 := ownershipAt(cur, 10, reg)

	// If shares accumulated, the later cutoff would carry roughly twice the
	// count. Normalisation hides that in the percentages, so compare the implied
	// entry counts, which is where an accumulation would show.
	if at10.entries > 2*at5.entries {
		t.Errorf("implied entries went %d -> %d between GW5 and GW10. Ownership is "+
			"being accumulated across gameweeks rather than read at one.",
			at5.entries, at10.entries)
	}
}
