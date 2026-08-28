package backtest

// Omniscience: the positive control for the oracle machinery itself.
//
// Every other oracle in the oracle-design document answers "is this capability worth
// building?". This one answers a question about the *apparatus*: does hindsight
// granted at the information seam actually reach a decision? Each of the others
// is a **negative** control at heart — it declares things it must not move, and
// passing means nothing moved. Omniscience is the opposite claim, and it is the
// only one whose failure is unambiguous. An arm told exactly what every player is
// about to do and scoring like the shipped model is not a small effect. It is a
// broken harness, and that is the whole finding.
//
// # It is a test fixture and never a report
//
// A hindsight bound looks exactly like a score, and this is the largest number
// the apparatus can produce. So it is refused at the *sweep* boundary —
// validateOracleArms rejects any arm carrying it — rather than at the means row
// the design proposes. Two reasons, both learned from the harness it is
// controlling:
//
//   - Refusing only the means row leaves the per-cell rows in the CSV, and
//     stats/sweep_inference.R would average them into an inference table with a
//     standard error and a p-value attached. The means file is not the only place
//     a number can be copied from; it is merely the most convenient one.
//   - Refusing only the *cells* instead is worse. writeSweepProvenance declares
//     the arms before the first cell precisely so a sweep killed under load reads
//     as a gap rather than as a complete run, and an arm deliberately omitted
//     from the cells file is indistinguishable from that.
//
// So the refusal is one rule at one boundary, and TestOmniscienceIsRefusedBySweeps
// pins it. The control runs Simulate directly instead, which is where a fixture
// belongs.
//
// # What "omniscient" means here, stated rather than assumed
//
// It rewrites every aggregate the bootstrap carries from the football that has
// **not been played yet** — the same accumulation PointInTimeWith performs over
// gameweeks 1..through, run over through+1..38 instead. Minutes, starts, goals,
// assists, clean sheets, goals conceded, saves, bonus, cards, defensive
// contribution and the expected-goals family, all of them realised.
//
// It deliberately does not touch price, position, club or availability. Those are
// either not hindsight at all (position, club) or another oracle's declared
// territory, and an omniscient arm that also moved them would be the composition
// of two things without saying so. The composition is available and is what the
// control actually runs: `OracleOmniscient | OracleMinutes | OracleAvailability`,
// which is the honest way to say "every information oracle at once" and exercises
// the bitmask while it is at it.
//
// **Two reasons it is not literally a points oracle**, both worth knowing before
// reading its number as a ceiling:
//
//   - The model still applies its own scoring function to perfect inputs. A term
//     the model prices wrongly is priced wrongly here too, on correct data.
//   - The rate blend still shrinks toward the prior season, and its evidence
//     weight is `el.Minutes / 90`. Late in a season there is little football left,
//     so a realised aggregate over the remaining weeks is a *smaller* sample than
//     the season-to-date one it replaced, and the blend trusts it less. The
//     control is therefore strongest at a GW1 entry, which is where it is run.
//
// Neither weakens it as a control. Both mean the observed effect is a floor on
// what perfect information can do to this harness, and a floor is all a positive
// control needs.

import (
	"armband/internal/analysis"
	"armband/internal/fpl"
)

// oraclePriorIndex is the prior season told the truth about the season to come.
//
// # It exists because the positive control found the leak it was built to find
//
// The bootstrap rewrite alone was not enough, and the reason is the third channel
// into "what the model believes about a player". `blendRates` has a dedicated
// pre-season branch: at `played == 0` it *overwrites the entire blend* from
// `Engine.Priors` whenever the prior has minutes and they differ from the
// element's. Under omniscience they always differ — the element carries the
// future — so the whole rewrite was discarded at exactly the entry point the
// control runs at, and the arm scored on last season's real record.
//
// The control caught it: the omniscient engine rated 9 of a season's twenty
// highest scorers in its own top twenty against the blind model's 9, which is the
// signature of hindsight computed and never delivered. That is the entire reason a
// positive control exists in a package whose every other guarantee is a negative
// one, and it is the second time the design's claim that `PointInTime` is the
// single information seam has failed against the shipped code — `Engine.Recent`
// was the first, recorded in minutesoracle.go.
//
// It wraps rather than replaces, so a player the honest prior knows and the
// archive has no future rows for keeps his honest record.
type oraclePriorIndex struct {
	base   analysis.PriorSeason
	future map[int]analysis.PriorPlayer
}

func (o oraclePriorIndex) Get(code int) (*analysis.PriorPlayer, bool) {
	if f, ok := o.future[code]; ok {
		p := f
		return &p, true
	}
	if o.base == nil {
		return nil, false
	}
	return o.base.Get(code)
}

// newOraclePriors builds the hindsight prior for a decision taken with data
// through gameweek `through`.
//
// The totals are the same football applyOmniscience writes onto the bootstrap, so
// the two channels cannot disagree about what the future holds. They are season
// *totals* rather than per-match rates because that is what PriorPlayer is: the
// pre-season blend divides minutes by GameweeksPerSeason itself, and handing it a
// per-match figure would understate every player by a factor of 38.
func newOraclePriors(s *Season, through int, base analysis.PriorSeason) analysis.PriorSeason {
	defcon := DefconScoredIn(s.Name)
	future := map[int]analysis.PriorPlayer{}
	for _, p := range s.Players {
		if p.Code == 0 {
			continue
		}
		var q analysis.PriorPlayer
		for gw := through + 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok {
				continue
			}
			q.Minutes += g.Minutes
			q.Starts += g.Starts
			q.Bonus += g.Bonus
			q.Saves += g.Saves
			q.Yellow += g.Yellow
			q.Red += g.Red
			if defcon {
				q.DefCon += g.DefCon
			}
			q.XG += g.XG
			q.XA += g.XA
			q.XGC += g.XGC
		}
		if q.Minutes == 0 {
			// No football left for him. Deliberately *not* recorded: the blend's
			// pre-season branch ignores a prior with no minutes, and its in-season
			// branch treats one as "no prior of his own" and shrinks him toward the
			// league rate — which would rate a player who is about to play nothing
			// as an ordinary member of his position. The bootstrap already says he
			// plays no minutes, and that is the statement that should govern.
			continue
		}
		future[p.Code] = q
	}
	return oraclePriorIndex{base: base, future: future}
}

// priors is the prior-season view a cell's engines read, with any hindsight this
// cell was granted already applied.
//
// One constructor rather than the two identical three-line blocks it replaced —
// Simulate's and the held-fifteen path's — for the reason SimConfig.recentIndex
// exists: an oracle wired into one of two copies would be wired into one of two
// copies silently, and this package's most-repeated bug is one quantity with
// several expressions.
func (c SimConfig) priors(cur, prior *Season) analysis.PriorSeason {
	var base analysis.PriorSeason
	if c.PriorMinutesHalfLife > 0 || c.PriorRateHalfLife > 0 {
		base = newPriorIndexRecent(prior, c.PriorMinutesHalfLife, c.PriorRateHalfLife)
	} else {
		base = newPriorIndexMulti(append([]*Season{prior}, c.OlderPriors...), c.PriorHalfLife)
	}
	if c.Oracles.Has(OracleOmniscient) {
		return newOraclePriors(cur, c.startGW()-1, base)
	}
	return base
}

// applyOmniscience rewrites the bootstrap's per-player aggregates from the
// football still to come.
//
// This is the `applyInfoOracles` seam the oracle-design document proposed and the
// harness commit declined to build, on the grounds that an empty seam function is
// the same silent no-op as an unimplemented constant. It arrives here with its
// first and only caller, which is the shape that rule was arguing for.
//
// `through` is the gameweek the model's view is built through, so the future is
// through+1 onward; pre-season that is the whole season. Both callers pass their
// own cutoff, so the pre-season and in-season halves of the seam cannot disagree
// about which football is hindsight.
func applyOmniscience(b *fpl.Bootstrap, cur *Season, through int) {
	defcon := DefconScoredIn(cur.Name)
	for i := range b.Elements {
		el := &b.Elements[i]
		p := cur.Players[el.ID]
		if p == nil {
			// A player on the bootstrap the archive has no gameweek rows for. He
			// keeps whatever the honest reconstruction gave him rather than being
			// zeroed: an oracle may correct what is known about a player, never
			// invent or erase one.
			continue
		}
		// Every stat field is cleared and refilled, so a field the honest pass set
		// and this one does not cannot survive as a stale season-to-date figure
		// hiding inside an arm labelled omniscient.
		el.Minutes, el.Starts, el.TotalPoints, el.Bonus = 0, 0, 0, 0
		el.GoalsScored, el.Assists = 0, 0
		el.CleanSheets, el.GoalsConceded, el.Saves = 0, 0, 0
		el.YellowCards, el.RedCards, el.DefensiveContribution = 0, 0, 0

		var xg, xa, xgc float64
		for gw := through + 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok {
				continue
			}
			el.Minutes += g.Minutes
			el.Starts += g.Starts
			el.TotalPoints += g.Points
			el.Bonus += g.Bonus
			el.GoalsScored += g.Goals
			el.Assists += g.Assists
			el.CleanSheets += g.CleanSheets
			el.GoalsConceded += g.GoalsConceded
			el.Saves += g.Saves
			el.YellowCards += g.Yellow
			el.RedCards += g.Red
			if defcon {
				el.DefensiveContribution += g.DefCon
			}
			xg += g.XG
			xa += g.XA
			xgc += g.XGC
		}
		el.ExpectedGoals, el.ExpectedAssists, el.ExpectedGoalsConceded =
			fpl.Num(xg), fpl.Num(xa), fpl.Num(xgc)
		el.ExpectedGoalsPer90, el.ExpectedAssistsPer90 = 0, 0
		el.ExpectedGCPer90, el.DefensiveContributionPer90 = 0, 0
		if el.Minutes > 0 {
			per90 := 90 / float64(el.Minutes)
			el.ExpectedGoalsPer90 = fpl.Num(xg * per90)
			el.ExpectedAssistsPer90 = fpl.Num(xa * per90)
			el.ExpectedGCPer90 = fpl.Num(xgc * per90)
			el.DefensiveContributionPer90 =
				fpl.Num(float64(el.DefensiveContribution) * per90)
		}
	}
}

// omniscientFields is what applyOmniscience may perturb, and it is the Tier 1
// declaration for OracleOmniscient.
//
// Written out rather than derived from the struct, because the useful half of the
// declaration is what is **absent**: NowCost, Status, Team, ElementType, Code and
// WebName. A price is not hindsight the model was denied — it is the price — and
// availability belongs to another bit. An omniscient arm that silently moved one
// of them would be a composition pretending to be a single oracle, which is the
// distinction the whole Oracles type exists to keep.
var omniscientFields = []string{
	"Minutes", "Starts", "TotalPoints", "Bonus",
	"GoalsScored", "Assists", "CleanSheets", "GoalsConceded", "Saves",
	"YellowCards", "RedCards", "DefensiveContribution",
	"ExpectedGoals", "ExpectedAssists", "ExpectedGoalsConceded",
	"ExpectedGoalsPer90", "ExpectedAssistsPer90", "ExpectedGCPer90",
	"DefensiveContributionPer90",
}
