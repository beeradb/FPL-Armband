package backtest

import "sort"

// startersPerFixture is FPL's rule: eleven players start each match. It is the
// count the reconstruction below is exact in by construction, which is what makes
// a rank rule better than a minutes threshold.
const startersPerFixture = 11

// reconstructStarts infers the starting eleven from minutes where the archive
// recorded no starts at all.
//
// # The defect
//
// `merged_gw.csv` carries `starts` as zero — present and zero, not missing — for
// all of 2021-22 and for 2022-23 through GW15. Verified by counting the total per
// gameweek, where the correct value is 220 (eleven starters times twenty clubs):
// 2021-22 reads zero in all 38, 2022-23 reads zero in GW1-6 and GW8-15 and is
// correct from GW16, and the three later seasons are correct throughout.
//
// **The window is wider than that as of the archive extension, and the reason is
// worse than "present and zero".** `starts` is absent as a *column* from both
// `merged_gw.csv` and `players_raw.csv` before 2022-23 — checked against the headers —
// so 2019-20 and 2020-21 have no start data in any form either. Nothing here needed
// changing for them: the reconstruction fires on a club-gameweek with no recorded
// start, which is what an absent column produces, and it reconstructs about 7,000 to
// 8,200 starts per season in each. Measured counts: 8,206 for 2019-20, 7,394 for
// 2020-21, 7,005 for 2021-22, 2,993 for 2022-23's partial window, and 1 and 0 for the
// two seasons that record their own. The three boundaries below apply to those seasons
// exactly as they do to 2021-22, and boundary three — never as evidence about an
// individual rotation or returning player — is the one that matters most, since those
// seasons now carry more reconstructed rows than the two the boundaries were measured
// on.
//
// It matters because PointInTime accumulates Starts, so StartShare reads near
// zero for everyone in that window, and blankRate — which is
// 0.624 x (1 - StartShare) — then reads every player as blanking about 62% of the
// time. That corrupts the derived bench slot weights for a season and a half, and
// 2022-23 is the season several recorded verdicts lean on as their held-out check.
//
// # The rule, and why it is a rank rather than a threshold
//
// Eleven players start each fixture, so within a club-gameweek the eleven with the
// most minutes are the starters. That is exact in *count* by construction. A
// "60+ minutes counts as a start" threshold is not, and measures three times
// worse: validated against the seasons that do record starts, the rank rule
// misclassifies 2.36% of starter slots against the threshold's 6.86%, and ignoring
// the week's minutes in favour of season minutes is worse again at 15.24%. A
// tiebreak on season minutes is not worth the complexity — it moves the slot error
// only from 2.36% to 2.32%, so the residual is genuinely half-time substitutions
// being indistinguishable from their replacements rather than a tie-breaking
// artefact.
//
// # Three boundaries, all measured, all load-bearing
//
// **One: safe for start share and the bench weights.** The rule always picks
// exactly eleven, so a misclassified starter is offset by a misclassified
// substitute in the same club-gameweek and the seasonal aggregate barely moves.
// Measured against recorded truth, start share is within 0.005 in every
// mean-minutes band, and in the 0.70+ regime the blank-rate constant was fitted in
// it reads 0.847 against a true 0.852 — moving the blank rate from 0.093 to 0.095,
// an error of 0.003 against a constant of 0.624.
//
// **Two: never for fitting a start / substitute / unused multinomial.** That
// consumes per-gameweek classifications rather than a seasonal average, and the
// substitute class is exactly where the spuriously promoted players land. Fit it
// on 2023-24 onward, where starts are recorded, and use these seasons only to
// validate.
//
// **Three: never as evidence about an individual rotation or returning player.**
// The error is *systematically biased by role*, not symmetric per player — a
// nailed starter is the one being withdrawn at half time and a fringe player is
// the one coming on. Measured: in the 0.85+ start-share band 24.5% of players lose
// starts against 7.7% gaining them, a 3:1 asymmetry, for a net 62 starts lost by
// the nailed group and 85 gained by the fringe.
//
// The damaging case is the 45-minute one. A player eased back in at half time
// plays 45; a starter withdrawn at half time plays 45; they tie, and the returning
// player can be credited with a start he did not make. That inflates precisely the
// player whose start share is least certain, in the direction that makes him look
// MORE nailed than he is — which is the judgement a minutes override exists to
// make. The noise agrees: mean absolute error peaks at 0.024 in the 0.25-0.70
// bands, the rotation and returning population, against 0.011 for ever-presents.
//
// # What is deliberately left alone
//
// Doubles. A player can legitimately start both fixtures so Starts can be 2, and a
// gameweek total cannot say which — 2022-23 has 42 double team-gameweeks. They
// keep their recorded zero and StartsReconstructed stays false, so a consumer can
// tell an unreconstructed row from a reconstructed one. An honest gap beats a
// confident guess, which is the lesson the doubles-counting bug taught on this
// same season.
//
// Gameweeks that legitimately have no football. 2022-23 GW7 has no rows at all and
// GW8 is partial — 440 rows and 13,761 total minutes against a normal ~19,700.
// That is the September 2022 postponements after the Queen's death: GW7 was
// cancelled outright and several GW8 fixtures were pushed back for policing.
// Nothing here invents a match, and nothing should.
func (s *Season) reconstructStarts() {
	type slot struct {
		pid, minutes, fixtures int
	}
	byClubGW := map[[2]int][]slot{}
	for _, p := range s.Players {
		for gw, g := range p.GWs {
			key := [2]int{p.Team, gw}
			byClubGW[key] = append(byClubGW[key],
				slot{pid: p.ID, minutes: g.Minutes, fixtures: g.Fixtures})
		}
	}

	for key, slots := range byClubGW {
		gw := key[1]

		// Only where the archive recorded NOTHING for the whole club-gameweek. A
		// club-gameweek with any recorded start has real data, and promoting a
		// genuine substitute inside it would be inventing rather than repairing.
		// Doubles are skipped outright: see the note above.
		var recorded, doubles int
		for _, sl := range slots {
			recorded += s.Players[sl.pid].GWs[gw].Starts
			if sl.fixtures != 1 {
				doubles++
			}
		}
		if recorded > 0 || doubles > 0 {
			continue
		}

		// Rank by minutes, then by element id so the result is deterministic
		// rather than dependent on map iteration order — the defect that made a
		// clean-sheet diagnostic give a different answer on every run.
		sort.Slice(slots, func(i, j int) bool {
			if slots[i].minutes != slots[j].minutes {
				return slots[i].minutes > slots[j].minutes
			}
			return slots[i].pid < slots[j].pid
		})

		n := 0
		for _, sl := range slots {
			if n >= startersPerFixture {
				break
			}
			// A player who did not appear cannot have started. Slots are sorted
			// descending, so the first zero means every remaining player also
			// recorded nothing, and a club that fielded fewer than eleven is a
			// data problem rather than a football one — either way, do not
			// manufacture a starter.
			if sl.minutes <= 0 {
				break
			}
			g := s.Players[sl.pid].GWs[gw]
			g.Starts = 1
			g.StartsReconstructed = true
			s.Players[sl.pid].GWs[gw] = g
			n++
		}
	}
}
