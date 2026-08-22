package viewmodel

import (
	"fmt"
	"math"
	"os"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// cbLabel names the defensive-action stat FPL counts for a position: CBIT
// (Clearances, Blocks, Interceptions, Tackles) for a defender, CBIRT (the same
// four plus Recoveries) for a midfielder or forward -- see
// analysis.RealisedMatch's own DefCon field comment, which draws the same
// split. Never called for a goalkeeper, who has no DefCon channel at all (see
// TeamPlayer.DefCon's own comment).
func cbLabel(pos int) string {
	if pos == 2 {
		return "CBIT"
	}
	return "CBIRT"
}

// scoreBreakdown is the hover explanation for one player's points figure -- see
// TeamPlayer.Breakdown's own comment for why this is computed here and not in
// team.js.
//
// It prices the match through analysis.DecomposeMatch, the SAME decomposer the
// realised-points research already uses (internal/analysis/realisedpoints.go),
// rather than restating FPL's scoring table a second time in this package --
// the same move analysis.DefConThreshold's own comment already makes for the
// one channel this file priced by hand before. Reusing it also reconciles for
// free: MatchPoints.Total() is compared against FPL's own total_points below,
// and a mismatch withholds the breakdown rather than showing one that
// disagrees with the number above it (see Breakdown's own comment) -- this
// package never invents a balancing line.
//
// pos is the element_type (1 GKP..4 FWD). The live season's rules always come
// from analysis.ScoringRulesFor(""), which is FPL's own documented meaning of
// the empty season string -- "the live game", never a replay -- so this never
// needs a season name of its own. DefConPaid is unconditionally true for the
// same reason: this function only ever prices the CURRENT gameweek, and
// defensive contribution has been live since 2025-26; a replay's season
// boundary (backtest.DefconScoredIn) belongs to a completely different caller.
//
// multiplier appends a final "Captain (×N)" line carrying the difference
// between points and the doubled (or tripled) figure the card actually shows
// -- a deliberate choice, not an oversight: the hover is attached to .tppts,
// and .tppts shows the multiplied figure (see team.js ptsHtml), so the
// breakdown has to reconcile against the SAME number it is explaining, not
// the raw total underneath it.
func scoreBreakdown(pos int, stats fpl.LiveStats, points, multiplier int) []ScoreLine {
	rules := analysis.ScoringRulesFor("")
	rm := analysis.RealisedMatch{
		Position:        pos,
		Minutes:         stats.Minutes,
		Goals:           stats.GoalsScored,
		Assists:         stats.Assists,
		CleanSheets:     stats.CleanSheets,
		GoalsConceded:   stats.GoalsConceded,
		Saves:           stats.Saves,
		Bonus:           stats.Bonus,
		Yellow:          stats.YellowCards,
		Red:             stats.RedCards,
		OwnGoals:        stats.OwnGoals,
		PenaltiesSaved:  stats.PenaltiesSaved,
		PenaltiesMissed: stats.PenaltiesMissed,
		DefCon:          stats.DefensiveContribution,
		DefConPaid:      true,
	}
	mp := analysis.DecomposeMatch(rm, rules)
	if got := int(math.Round(mp.Total())); got != points {
		fmt.Fprintf(os.Stderr,
			"viewmodel: breakdown for element_type %d disagrees with FPL's total_points "+
				"(computed %d, want %d) -- withholding the breakdown rather than showing "+
				"one that disagrees with the number above it\n", pos, got, points)
		return nil
	}

	var lines []ScoreLine
	add := func(label, detail string, pts float64) {
		if pts == 0 {
			return
		}
		lines = append(lines, ScoreLine{Label: label, Detail: detail, Points: int(math.Round(pts))})
	}

	// FPL's own scoring order, matching the spec table this was built from.
	playDetail := "1-59 mins"
	if stats.Minutes >= analysis.AppearanceMinutes {
		playDetail = "60+ mins"
	}
	add("Played", playDetail, mp.Appearance)

	goalDetail := ""
	if rm.Goals > 1 {
		goalDetail = fmt.Sprintf("%d × %d", rm.Goals, int(math.Round(rules.Goal[pos])))
	}
	add("Goals", goalDetail, mp.Goals)

	assistDetail := ""
	if rm.Assists > 1 {
		assistDetail = fmt.Sprintf("%d × %d", rm.Assists, int(math.Round(rules.Assist)))
	}
	add("Assists", assistDetail, mp.Assists)

	add("Clean sheet", "", mp.CleanSheet)

	if rm.Saves > 0 {
		add("Saves", fmt.Sprintf("%d saves", rm.Saves), mp.Saves)
	}

	// Penalty save and penalty miss are ONE channel in MatchPoints (Penalties):
	// DecomposeMatch prices a goalkeeper's save and a taker's miss through the
	// same field, because a single match usually has only one or the other for
	// a given player. Split by whichever raw count is actually non-zero, and
	// fall back to one combined line in the one case both fire, rather than
	// invent a per-event value DecomposeMatch does not expose separately.
	switch {
	case rm.PenaltiesSaved > 0 && rm.PenaltiesMissed == 0:
		detail := ""
		if rm.PenaltiesSaved > 1 {
			detail = fmt.Sprintf("%d", rm.PenaltiesSaved)
		}
		add("Penalty save", detail, mp.Penalties)
	case rm.PenaltiesMissed > 0 && rm.PenaltiesSaved == 0:
		detail := ""
		if rm.PenaltiesMissed > 1 {
			detail = fmt.Sprintf("%d", rm.PenaltiesMissed)
		}
		add("Penalty miss", detail, mp.Penalties)
	case rm.PenaltiesSaved > 0 && rm.PenaltiesMissed > 0:
		add("Penalties", fmt.Sprintf("%d saved, %d missed", rm.PenaltiesSaved, rm.PenaltiesMissed), mp.Penalties)
	}

	if rm.GoalsConceded > 0 {
		add("Goals conceded", fmt.Sprintf("%d conceded", rm.GoalsConceded), mp.Conceded)
	}

	// Yellow and red are ONE channel (Cards), for the same reason penalties are
	// two -- see above. FPL's live stats never carry both for one dismissal (a
	// second yellow arrives as a red with no yellow alongside it -- see
	// team.js badgeHtml's own comment), so the split below is exact in
	// practice; the combined fallback exists for the same reason it does above.
	switch {
	case rm.Yellow > 0 && rm.Red == 0:
		add("Yellow card", "", mp.Cards)
	case rm.Red > 0 && rm.Yellow == 0:
		add("Red card", "", mp.Cards)
	case rm.Yellow > 0 && rm.Red > 0:
		add("Cards", fmt.Sprintf("%d yellow, %d red", rm.Yellow, rm.Red), mp.Cards)
	}

	ogDetail := ""
	if rm.OwnGoals > 1 {
		ogDetail = fmt.Sprintf("%d", rm.OwnGoals)
	}
	add("Own goal", ogDetail, mp.OwnGoals)

	if mp.DefCon != 0 {
		add("Defensive contribution", fmt.Sprintf("%d %s", rm.DefCon, cbLabel(pos)), mp.DefCon)
	}

	add("Bonus", "", mp.Bonus)

	// The captain/triple-captain line: not a component FPL paid, but the
	// arithmetic that turns this player's own return into his contribution to
	// the team score, which is the number .tppts actually shows and therefore
	// the number this breakdown has to explain. See this function's own
	// doc comment for why that reconciliation target is deliberate.
	if multiplier > 1 {
		lines = append(lines, ScoreLine{
			Label:  fmt.Sprintf("Captain (×%d)", multiplier),
			Points: points * (multiplier - 1),
		})
	}
	return lines
}
