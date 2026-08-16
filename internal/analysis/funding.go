package analysis

import (
	"os"
	"sort"
)

// Several downgrades funding one upgrade.
//
// The paired move in polish funds an upgrade from exactly one downgrade, and
// that is not always enough. Asked to reach a £6.0m forward from a £4.5m one,
// with the money spread across a goalkeeper and two defenders at £0.5m each, a
// single pair frees £0.5m of the £1.5m needed and the move is simply outside the
// search space. Every intermediate step is downhill — each downgrade on its own
// lowers the objective — so steepest ascent will not walk there either.
//
// This is the same gap RankPairs closes on the transfer side with MaxFundingSales,
// and it is why locking a player the squad already contained could beat the
// unconstrained search: pre-placing him changed which seed ranked first, and the
// restructure was reachable from that one.
//
// # The proxy filters, it does not decide
//
// Choosing funding sales by their own score deltas is exactly the mistake
// documented against RankPairs, where a greedy funding choice scored a mean of
// 2076 against 2158 for scoring the survivors properly. The objective is not
// separable — it counts the best eleven plus the captain, so a player's own
// score does not say what removing him costs.
//
// So the knapsack below runs on the additive proxy only to *propose* funding
// combinations, and every survivor is then scored with the real objective. The
// proxy narrows thousands of combinations to a handful; it never picks the
// winner.

// fundingSlack is how much more than the shortfall a combination may free.
// Some slack matters because the cheapest legal downgrade rarely lands exactly
// on the money needed, and a combination that frees a little extra can still be
// the best one. Too much and the knapsack spends its budget dimension on
// combinations that are obviously wasteful.
const fundingSlack = 25 // £2.5m

// fundingCombos returns candidate sets of downgrades that free at least
// shortfall, best-first on the additive proxy. Each result is a squad-index ->
// replacement mapping. At most limit are returned.
//
// It is a bounded knapsack over squad slots: each slot may be left alone or
// swapped for a cheaper player in the same position, and the DP maximises the
// summed score delta subject to freeing enough money.
func fundingCombos(current []PlayerMetrics, selected map[int]PlayerMetrics,
	clubCount map[string]int, downByPos map[string][]PlayerMetrics,
	skip map[int]bool, locked map[int]bool, shortfall int, limit int,
	slotWeight []float64, reserved int,
) []map[int]PlayerMetrics {

	if shortfall <= 0 {
		return nil
	}
	maxFreed := shortfall + fundingSlack

	// Per slot, the best replacement at each amount of money freed. Only the
	// strongest candidate per price point survives — two players freeing the
	// same money are interchangeable to the knapsack, and keeping both would
	// multiply the state space for nothing.
	type opt struct {
		in    PlayerMetrics
		freed int
		delta float64
	}
	slots := make([][]opt, len(current))
	for i, out := range current {
		if skip[out.ID] || locked[out.ID] {
			continue
		}
		bestAt := map[int]opt{}
		for _, in := range downByPos[out.Position] {
			if _, already := selected[in.ID]; already {
				continue
			}
			// The player being bought by the upgrade is spoken for. Without this
			// the knapsack can hand him out as a funding target too — he is
			// often both cheaper than some squad slot and better than it, which
			// makes him the most attractive downgrade on the board — and the
			// resulting squad contains him twice and is thrown out as illegal.
			// Every candidate for that upgrade then dies silently.
			if in.ID == reserved {
				continue
			}
			freed := priceUnits(out) - priceUnits(in)
			if freed <= 0 || freed > maxFreed {
				continue
			}
			// A downgrade that breaks the club limit on its own is never legal,
			// whatever else the combination does. Combinations that break it
			// only in aggregate are caught by the legality check on the result.
			if clubCountAfter(clubCount, out.Team, in.Team) > MaxPerClub {
				continue
			}
			// Weighted by whether this slot is in the eleven. An unweighted
			// delta is not a bad approximation of the objective, it is a
			// systematically wrong one: the objective pays the bench at
			// BenchWeight (0.02), so selling bench quality to fund a starter is
			// nearly free in reality and looks ruinous to a raw score sum.
			// Those are exactly the moves this phase exists to find, so ranking
			// them on the raw sum buries the right answer.
			d := (in.Score - out.Score) * slotWeight[i]
			if cur, ok := bestAt[freed]; !ok || d > cur.delta {
				bestAt[freed] = opt{in, freed, d}
			}
		}
		for _, o := range bestAt {
			slots[i] = append(slots[i], o)
		}
		sort.Slice(slots[i], func(a, b int) bool { return slots[i][a].freed < slots[i][b].freed })
	}

	// dp[i][f] is the best proxy delta using the first i slots and freeing f,
	// with a parent pointer into layer i-1. Freed amounts clamp at maxFreed, so
	// that level means "at least this much".
	//
	// The layers are kept rather than rolled into one array. A single rolling
	// array is enough to compute the best value but not to reconstruct the
	// combination behind it: the parent pointer refers to a state that the next
	// iteration overwrites, so walking it afterwards reads whatever happens to
	// be at that index later. That silently yields combinations the DP never
	// chose, which then fail the legality check and vanish — a bug that looks
	// exactly like "the move set did not help".
	type cell struct {
		delta float64
		used  bool
		in    PlayerMetrics
		took  bool
		prevF int
	}
	layers := make([][]cell, len(slots)+1)
	layers[0] = make([]cell, maxFreed+1)
	layers[0][0].used = true

	for i := range slots {
		prev := layers[i]
		next := make([]cell, maxFreed+1)
		copy(next, prev)
		// Carrying a state forward means "leave this slot alone", so the parent
		// is the same f with nothing taken.
		for f := range next {
			if prev[f].used {
				next[f] = cell{prev[f].delta, true, PlayerMetrics{}, false, f}
			}
		}
		for f := 0; f <= maxFreed; f++ {
			if !prev[f].used {
				continue
			}
			for _, o := range slots[i] {
				nf := f + o.freed
				if nf > maxFreed {
					nf = maxFreed
				}
				nd := prev[f].delta + o.delta
				if !next[nf].used || nd > next[nf].delta {
					next[nf] = cell{nd, true, o.in, true, f}
				}
			}
		}
		layers[i+1] = next
	}
	final := layers[len(slots)]

	// Every level that frees enough, best proxy first.
	type reach struct {
		f     int
		delta float64
	}
	var ends []reach
	for f := shortfall; f <= maxFreed; f++ {
		if final[f].used {
			ends = append(ends, reach{f, final[f].delta})
		}
	}
	sort.Slice(ends, func(a, b int) bool { return ends[a].delta > ends[b].delta })
	if len(ends) > limit {
		ends = ends[:limit]
	}

	var out []map[int]PlayerMetrics
	for _, e := range ends {
		combo := map[int]PlayerMetrics{}
		used := map[int]bool{}
		f := e.f
		dup := false
		for i := len(slots); i > 0; i-- {
			c := layers[i][f]
			if !c.used {
				break
			}
			if c.took {
				// Two slots in the same position can reach the same cheap
				// player, which would put him in the squad twice.
				if used[c.in.ID] {
					dup = true
					break
				}
				used[c.in.ID] = true
				combo[i-1] = c.in
			}
			f = c.prevF
		}
		if !dup && len(combo) > 0 {
			out = append(out, combo)
		}
	}
	return out
}

// fundedUpgrade searches for the best "downgrade several, upgrade one" move from
// the current squad and returns the resulting squad and its objective. ok is
// false when nothing beats bestScore.
//
// This is the exact-scoring half: fundingCombos proposes, this disposes.
func (e *Engine) fundedUpgrade(current []PlayerMetrics, selected map[int]PlayerMetrics,
	clubCount map[string]int, spend, budget int,
	frontierByPos map[string][]PlayerMetrics,
	benchWeight float64, boost bool, locked, mustStart map[int]bool, bestScore float64,
	changes changeBudget, spent int,
) (squad []PlayerMetrics, score float64, cost int, ok bool) {

	// What each slot is worth to the objective right now: a starter at full
	// weight, a bench player at BenchWeight. Recomputed once per call rather
	// than per candidate — the eleven shifts as moves are applied, but not
	// within a single scan.
	xi, _, _ := bestXIWith(current, mustStart)
	inXI := map[int]bool{}
	for _, p := range xi {
		inXI[p.ID] = true
	}
	slotWeight := make([]float64, len(current))
	for i, p := range current {
		slotWeight[i] = benchWeight
		if inXI[p.ID] {
			slotWeight[i] = 1
		}
	}

	best := bestScore
	for _, upOut := range current {
		if locked[upOut.ID] {
			continue
		}
		for _, upIn := range frontierByPos[upOut.Position] {
			if _, already := selected[upIn.ID]; already {
				continue
			}
			// The proxy filter: only chase upgrades that are upgrades. This
			// prunes the search, it does not choose between survivors.
			if upIn.Score <= upOut.Score {
				continue
			}
			if clubCountAfter(clubCount, upOut.Team, upIn.Team) > MaxPerClub {
				continue
			}
			shortfall := spend - priceUnits(upOut) + priceUnits(upIn) - budget
			if shortfall <= 0 {
				continue // affordable outright; the single-swap phase has it
			}

			skip := map[int]bool{upOut.ID: true}
			combos := fundingCombos(current, selected, clubCount, frontierByPos,
				skip, locked, shortfall, fundingCandidates, slotWeight, upIn.ID)

			for _, combo := range combos {
				trial := append([]PlayerMetrics(nil), current...)
				for idx, in := range combo {
					trial[idx] = in
				}
				for i := range trial {
					if trial[i].ID == upOut.ID {
						trial[i] = upIn
					}
				}
				if !squadIsLegal(trial, budget) || !holdsLocks(trial, locked) {
					continue
				}
				if !changes.unlimited() && changes.distance(trial) > changes.Max {
					continue
				}
				s := objectiveWith(trial, benchWeight, mustStart, boost)
				if s > best+1e-9 {
					total := 0
					for _, p := range trial {
						total += priceUnits(p)
					}
					best, squad, cost, ok = s, trial, total, true
				}
			}
		}
	}
	return squad, best, cost, ok
}

// fundingCandidates is how many funding combinations per upgrade get scored
// with the real objective. The knapsack returns them best-proxy-first, and the
// proxy is a poor guide to the top of the distribution, so this is deliberately
// more than one.
const fundingCandidates = 6

// positionFrontier is the candidate set for both halves of a funded move: for
// each position, the best scorer at every price point.
//
// Both halves originally reused the helpers the paired move uses, and both were
// the wrong shape for the same underlying reason — they select on score or on
// price alone, when what a restructure needs is the best player *at a given
// price*.
//
// cheapestByPosition returns the N absolutely cheapest in a position, and with a
// couple of hundred midfielders every one of the fourteen cheapest is under
// £5.0m; a £6.0m midfielder downgrading to a £5.5m one had nowhere to go.
// strongestByPosition returns the 30 highest scorers, all of them expensive; a
// £4.5m bench midfielder upgrading to the best £5.0m one had nowhere to go
// either. That second case is worth remembering, because it is counter-intuitive:
// upgrades on the *bench* matter, since the bench is credited at BenchWeight and
// a cheap bench slot is exactly where spare money ends up.
func positionFrontier(pool []PlayerMetrics, perPrice int) map[string][]PlayerMetrics {
	byPos := map[string][]PlayerMetrics{}
	for _, p := range pool {
		byPos[p.Position] = append(byPos[p.Position], p)
	}
	for pos := range byPos {
		byPos[pos] = frontier(byPos[pos], perPrice)
	}
	return byPos
}

// fundedUpgradeEnabled switches the multi-downgrade phase off. It exists so the
// search-quality harness and the replay can measure the same problem with and
// without it — a fix that cannot be measured against the thing it replaced is a
// guess. Set FPL_NO_FUNDED_UPGRADE=1 to disable it from the command line, which
// is how the backtest isolates this change from everything else that has moved.
// It is never changed in normal operation.
var fundedUpgradeEnabled = os.Getenv("FPL_NO_FUNDED_UPGRADE") == ""

// If this phase ever looks inert again, count four things before changing it:
// upgrades considered, how many were skipped for having no shortfall, how many
// combinations came back, and how many improved. A phase that never fires and
// one that fires and never helps look identical from the outside and need
// completely different fixes — this one presented as the former and was
// actually the latter twice over. Do not leave the counters in: package-level
// mutable state is a data race under the concurrent tool runner.
