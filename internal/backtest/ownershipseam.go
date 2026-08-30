package backtest

// Ownership at a replayed deadline, turned from the archive's raw owner COUNT
// into the percentage the live bootstrap publishes.
//
// # Why a conversion is needed at all, rather than passing the count through
//
// `GW.Selected` is a count of managers; `fpl.Element.SelectedByPercent` is a
// percentage, and `analysis.PlayerMetrics.Ownership` is read as one — the
// research surface compares it against `researchOwnershipFloor = 3.0` and
// `researchDisagreementOwnership = 10.0`. Assigning a count into that field
// would put every player in the game above both bars, permanently, in a way
// nothing would flag: the numbers are all plausible and all wrong.
//
// The count's own doc comment says it is "only meaningful as a RANKING within
// one gameweek" without a denominator. For a tiebreak between two players in the
// same gameweek that would have been enough, since ranking is order-preserving.
// It is not enough for the field it has to live in, so the denominator is
// computed rather than the constraint documented and left to rot.
//
// # The denominator, and why it is exact rather than estimated
//
// Every entry in the game owns exactly fifteen players, so summing `selected`
// over all players in a gameweek counts every entry exactly fifteen times:
//
//	entries = sum(selected) / 15
//
// That is an identity, not an approximation, and it needs no external figure for
// how many people played FPL in 2019-20 — which the archive does not carry and
// which would be a second source to keep true.
//
// ⚠️ It IS sensitive to archive completeness. A season whose player rows are
// partial under-counts the sum, under-counts entries, and inflates every
// percentage by the same factor. `TestOwnershipSharesAreAPlausiblePercentage`
// bounds that: it checks the implied entry count and the resulting top-of-league
// ownership against what the game actually looks like, per season, rather than
// asserting the identity and walking away.
//
// # Level, not accumulation
//
// Ownership is read at a single gameweek, the way price is. The parser's own
// comment records why: the archive repeats the current owner count on each row
// of a double gameweek, so summing would report twice as many managers as exist.
// Where a player has no row at `through` — he blanked, or was not in the squad
// data that week — the most recent earlier gameweek is used, because his owners
// did not vanish, the row did.
type ownershipShares struct {
	pct     map[int]float64
	entries int
}

// ownershipAt builds the share table for the deadline after gameweek `through`.
func ownershipAt(s *Season, through int, reg registration) ownershipShares {
	counts := map[int]int{}
	total := 0
	for _, p := range s.Players {
		if !reg.has(p.ID) {
			continue
		}
		// Most recent gameweek at or before `through` that records a count.
		for gw := through; gw >= 1; gw-- {
			g, ok := p.GWs[gw]
			if !ok || g.Selected <= 0 {
				continue
			}
			counts[p.ID] = g.Selected
			total += g.Selected
			break
		}
	}
	out := ownershipShares{pct: make(map[int]float64, len(counts))}
	if total <= 0 {
		// A season whose archive carries no `selected` column at all. Leaving the
		// map empty means every player reads 0% — the same thing the replay did
		// before this file existed, and honest, rather than a division by zero or
		// a fabricated uniform share.
		return out
	}
	out.entries = total / squadSize
	for id, n := range counts {
		out.pct[id] = 100 * float64(n) * squadSize / float64(total)
	}
	return out
}

// squadSize is the fifteen every FPL entry owns, which is what makes the
// denominator above an identity.
const squadSize = 15
