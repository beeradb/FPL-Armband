package analysis

import "sort"

// Selection inside the model's own noise band.
//
// # The proposal, and why it needs a total order
//
// "Where two players sit within MinSeparableGain of each other, prefer the one
// with more X" is a pairwise rule, and a pairwise rule inside a band is not
// transitive: at a band of 0.41, A can beat B and B beat C while A and C are
// 0.8 apart and must be ordered by Score. An optimiser needs a total order, so
// the rule cannot be implemented as stated.
//
// What IS implementable, and is what this does: add to each player's Score a
// nudge in [0, band) drawn from the tiebreak signal. Two players whose Scores
// differ by less than the band can then be reordered by the signal; two whose
// Scores differ by MORE than the band cannot be, because the largest possible
// nudge is smaller than the gap. That is the proposal's intent, made transitive,
// and it is graded rather than a cliff — a 0.05 gap is overturned by a small
// signal edge, a 0.40 gap needs nearly the whole signal range.
//
// # Within position, not pooled
//
// The optimiser fills a positional quota, so within-position ordering is what it
// actually consumes. The record is explicit that the pooled reading of the
// ownership channel was a position artefact — commit 4f96dfb6, "the league
// fallback is FLAT inside a position and differs BETWEEN them" — and the
// within-position figures are the ones that resolve. Nudging across positions
// would also be meaningless in its own right: a keeper owned by 40% and a
// forward owned by 40% are not competing for the same slot.
//
// # Rank, not level
//
// The nudge uses the signal's rank within its position, scaled to [0, band).
// Levels would make the nudge depend on the shape of the ownership distribution
// — which is heavily skewed, and differently skewed at GW2 and GW30 — so the
// same policy would bite differently through a season for reasons that have
// nothing to do with the policy. A rank is scale-free and comparable across
// gameweeks and seasons, which is what a six-season replay needs.
type Tiebreak struct {
	// Signal is "" (off), "ownership", or "price". Off is the shipped default and
	// leaves every Score untouched.
	Signal string
	// Band is the width inside which the signal may reorder, and is
	// config.Review.MinSeparableGain — the model's own stated resolution. A zero
	// band disables the tiebreak however Signal is set, since a nudge of zero
	// reorders nothing.
	Band float64

	// HaulRate is the per-player ceiling the "haul" signal ranks on, keyed by
	// element id: the share of a prior season's gameweeks in which he returned
	// nine points or more.
	//
	// Supplied by the caller rather than read off PlayerMetrics, because
	// PlayerMetrics carries season AGGREGATES and not the distribution of returns
	// that produced them — a player averaging four points a week from steady
	// fours and one averaging four from a haul and three blanks are identical in
	// every field the engine holds. That distinction is the entire premise of
	// this signal, and internal/analysis must not learn about the archive to
	// discover it.
	//
	// ⚠️ A missing id ranks as zero, which is correct: no prior-season history
	// is not evidence of a ceiling.
	HaulRate map[int]float64
}

// TiebreakOff is the shipped policy: no reordering.
var TiebreakOff = Tiebreak{}

// active reports whether this policy can move anything.
func (t Tiebreak) active() bool {
	switch {
	case t.Band <= 0:
		return false
	case t.Signal == TiebreakOwnership, t.Signal == TiebreakPrice:
		return true
	case t.Signal == TiebreakHaul:
		// Without a table there is nothing to rank on, and ranking everyone at
		// zero would silently make this the baseline while reporting as an arm.
		return len(t.HaulRate) > 0
	}
	return false
}

const (
	TiebreakOwnership = "ownership"
	TiebreakPrice     = "price"
	TiebreakHaul      = "haul"
)

// applyTiebreak nudges Score in place and returns the nudge applied to each
// player, so the caller can take it back out of the squad it reports.
//
// ⚠️ The nudge MUST come back out before any expected-points figure is shown or
// compared. It is a selection device, not a forecast: leaving it in would inflate
// a fifteen's reported score by up to fifteen times the band, and would do it
// only in the arm that has the tiebreak switched on — which is exactly the shape
// that makes an A/B look like a win.
func applyTiebreak(all []PlayerMetrics, t Tiebreak) map[int]float64 {
	if !t.active() || len(all) == 0 {
		return nil
	}

	byPos := map[string][]int{}
	for i := range all {
		byPos[all[i].Position] = append(byPos[all[i].Position], i)
	}

	nudges := make(map[int]float64, len(all))
	for _, idx := range byPos {
		// Rank ascending on the signal, so the largest signal takes the largest
		// nudge. Ties in the signal take the same rank rather than an arbitrary
		// one, or the tiebreak would invent an ordering between two players it has
		// no opinion about — most of a positional pool is on identical ownership
		// near zero, and breaking those ties by pool order would be a hidden
		// dependence on element id.
		sorted := make([]int, len(idx))
		copy(sorted, idx)
		sig := func(i int) float64 {
			switch t.Signal {
			case TiebreakOwnership:
				return all[i].Ownership
			case TiebreakHaul:
				return t.HaulRate[all[i].ID]
			}
			return all[i].Price
		}
		sort.SliceStable(sorted, func(a, b int) bool { return sig(sorted[a]) < sig(sorted[b]) })

		n := len(sorted)
		if n < 2 {
			continue
		}
		rank := 0
		for k := 0; k < n; k++ {
			if k > 0 && sig(sorted[k]) > sig(sorted[k-1]) {
				rank = k
			}
			u := float64(rank) / float64(n) // in [0, 1)
			d := t.Band * u
			if d == 0 {
				continue
			}
			i := sorted[k]
			all[i].Score += d
			nudges[all[i].ID] = d
		}
	}
	return nudges
}

// removeTiebreak takes the nudge back out of a set of players, so what is
// reported and compared is the model's actual expected points.
func removeTiebreak(ps []PlayerMetrics, nudges map[int]float64) {
	if len(nudges) == 0 {
		return
	}
	for i := range ps {
		if d, ok := nudges[ps[i].ID]; ok {
			ps[i].Score -= d
		}
	}
}
