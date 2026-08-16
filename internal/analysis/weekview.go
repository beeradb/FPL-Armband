package analysis

import (
	"fmt"
	"sort"
)

// WeekView is one gameweek of the horizon: the eleven that would actually be
// fielded, who each player faces, and who wears the armband.
//
// # Why this is here and not in the presentation layer
//
// A squad's score is an average over the horizon, so "the optimal squad for the
// next five gameweeks" is one fifteen and *five different elevens* — a player
// with four easy fixtures and one hard one is started in some weeks and not
// others, and a club that blanks fields nobody at all. Showing one eleven under
// a five-gameweek heading is a real mis-statement, not a display shortcut.
//
// Working that out means re-scoring the fifteen against a single gameweek, which
// is scoring, so it belongs in this package. The renderer consumes WeekView and
// computes nothing — the standing rule being that a second implementation of a
// quantity is how two of them come to disagree.
type WeekView struct {
	// Event is the gameweek number.
	Event int
	// XI, Bench and Formation are what would be fielded THAT week, picked on
	// that week's fixture rather than on the horizon average.
	XI        []PlayerMetrics
	Bench     []PlayerMetrics
	Formation string
	// Captain and ViceCaptain are picked by the same rule the replay scores:
	// the two highest-scoring players in the eleven, so the armband and the
	// objective never disagree about who wears it.
	Captain     PlayerMetrics
	ViceCaptain PlayerMetrics
	// Opponents is keyed by player id. A player whose club does not play that
	// week is absent from the map, which is what a blank looks like.
	Opponents map[int][]FixtureBrief
	// XIScore is the plain sum of the eleven for that week; Expected is what
	// FPL would actually pay, which depends on the chip.
	XIScore  float64
	Expected float64

	// Chip is the chip planned for this gameweek — "Wildcard", "Free Hit",
	// "Bench Boost", "Triple Captain" — or empty.
	//
	// It is not decoration. Each one changes a different thing, so a week view
	// that ignored it would be wrong in a different way for each:
	//
	//   - Bench Boost pays the bench too, so Expected includes all fifteen.
	//   - Triple Captain pays the armband three times rather than twice.
	//   - Wildcard and Free Hit change WHICH FIFTEEN plays, so the squad below
	//     is re-optimised for that week rather than being the one you own.
	Chip string
	// Rebuilt is true when Squad is a fresh fifteen for a wildcard or free hit
	// rather than the squad passed in. A free hit's fifteen is handed back the
	// following week; a wildcard's is kept, and that difference is the whole
	// reason the two chips are planned differently.
	Rebuilt bool
	// Squad is the fifteen this week is scored on — the one passed in, or the
	// rebuilt one when Rebuilt is true.
	Squad []PlayerMetrics
	// Caveat warns that a rebuilt squad rests on thin evidence. Empty when there
	// is nothing to say — see rebuildCaveat, which measures it rather than
	// assuming a gameweek number.
	Caveat string
}

// rebuildCaveat warns when a wildcard or free hit is being planned on evidence
// that is mostly last season's.
//
// A rebuild re-picks all fifteen, so it is the decision most exposed to a thin
// sample — and early in a season the model is largely reporting last year. The
// blend is `n/(n+k)`: with `BlendMinutesK` at 5, two gameweeks played give this
// season **29%** of the weight on minutes, and minutes are the dominant term.
//
// It is computed from the blend rather than hardcoded to "GW1 or 2", for two
// reasons. The threshold moves if the constant does, and a number a reader can
// check ("this season is 29% of the estimate") is worth more than an assertion
// that it is early. Returns empty once the current season carries most of the
// weight, so it stops nagging on its own.
func (e *Engine) rebuildCaveat() string {
	played := e.GameweeksPlayed()
	k := e.Weights.BlendMinutesK
	if k <= 0 {
		k = 5
	}
	share := float64(played) / (float64(played) + k)
	if share >= 0.5 {
		return ""
	}
	if played == 0 {
		return "No gameweek has been played, so this fifteen rests entirely on last " +
			"season plus any manual minutes corrections. A wildcard or free hit re-picks " +
			"all fifteen, which is the decision most exposed to that — the recorded " +
			"guidance is to wildcard late enough that roles have resolved, and four or " +
			"five gameweeks of real football settles most of them."
	}
	return fmt.Sprintf("Only %d gameweek(s) have been played, so the minutes behind this "+
		"fifteen are about %.0f%% this season and %.0f%% last. A wildcard or free hit "+
		"re-picks all fifteen on that mix, and it is the decision most exposed to a thin "+
		"sample; four or five gameweeks settle most roles.",
		played, share*100, (1-share)*100)
}

// Blanks lists the squad members whose club has no fixture that week. They are
// the reason a week's eleven can differ from every other week's, so they are
// reported rather than left to be inferred from an absence in Opponents.
func (w WeekView) Blanks(squad []PlayerMetrics) []PlayerMetrics {
	var out []PlayerMetrics
	for _, p := range squad {
		if len(w.Opponents[p.ID]) == 0 {
			out = append(out, p)
		}
	}
	return out
}

// WeekViews re-scores a squad against each of the next n gameweeks separately
// and returns what would be fielded in each.
//
// # How a single gameweek is isolated
//
// `TeamFixtures` returns a club's next fixtures in order, minus any gameweek in
// the skip set. So an engine that skips every gameweek before G, at horizon 1,
// scores exactly G's fixture. That machinery already exists for the free hit;
// this is a second, read-only use of it.
//
// Each week gets its own engine and the caller's is never mutated. That is not
// tidiness: the tool runner drives searches from several goroutines at once, and
// setting a skip set on a shared engine for one call and restoring it afterwards
// is the exact shape of the config-write race that lost three findings in a
// single live run.
//
// # The blank trap, which the obvious implementation walks into
//
// Skipping the gameweeks before G does not guarantee the first remaining fixture
// IS G's. If the club blanks in G, `TeamFixtures` happily returns its NEXT
// fixture instead, and the player would be scored — and fielded — on a match
// played in a different week. So the fixture is checked against G and dropped if
// it does not match. A blanking club then has no entry in Opponents, its players
// score zero for that week, and `BestXI` fields whoever is left.
func (e *Engine) WeekViews(squad []PlayerMetrics, n int) []WeekView {
	events := e.upcomingEvents(n)
	views := make([]WeekView, 0, len(events))

	for _, gw := range events {
		wk := e.engineAt(gw)

		// A wildcard or free hit fields a DIFFERENT fifteen, so the week has to
		// be scored on the squad that chip would buy rather than on the one you
		// own. Anything else answers the wrong question — and answers it
		// confidently, which is worse.
		//
		// Budget: a wildcard spends the squad's selling value plus the bank, the
		// same allowance AssemblyBudget resolves for any in-season rebuild.
		// Failing to price it is reported by leaving the squad alone rather than
		// by inventing £100m, since a fifteen built on money that does not exist
		// is a recommendation that dies at the deadline.
		weekSquad, chip, rebuilt := squad, e.chipAt(gw), false
		if chip == "Wildcard" || chip == "Free Hit" {
			if budget, _, err := e.AssemblyBudget(); err == nil {
				if built, err := wk.Optimize(OptimizeRequest{
					Budget:             budget,
					MinMinutes:         600,
					MinExpectedMinutes: 55,
				}); err == nil && built != nil {
					weekSquad, rebuilt = built.Players, true
				}
			}
		}

		scored := make([]PlayerMetrics, 0, len(weekSquad))
		opponents := map[int][]FixtureBrief{}
		for _, p := range weekSquad {
			el := e.Boot.ElementByID(p.ID)
			if el == nil {
				scored = append(scored, p)
				continue
			}
			m := wk.Metrics(el)

			// Everything a club plays IN this gameweek, which is two fixtures in
			// a double and none in a blank.
			var fixtures []FixtureBrief
			for _, f := range wk.TeamFixtures(el.Team, 2) {
				if f.Event == gw {
					fixtures = append(fixtures, f)
				}
			}
			if len(fixtures) == 0 {
				// He cannot score, so he must not be picked. Zeroing the score
				// is what makes BestXI field the eleven a manager actually
				// would, rather than one containing a player with no match.
				m.Score = 0
			}
			opponents[m.ID] = fixtures
			scored = append(scored, m)
		}

		xi, bench, formation := BestXI(scored)
		v := WeekView{
			Event: gw, XI: xi, Bench: bench, Formation: formation,
			Opponents: opponents, Chip: chip, Rebuilt: rebuilt, Squad: scored,
		}
		if rebuilt {
			v.Caveat = e.rebuildCaveat()
		}
		ranked := append([]PlayerMetrics(nil), xi...)
		sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
		if len(ranked) > 0 {
			v.Captain = ranked[0]
		}
		if len(ranked) > 1 {
			v.ViceCaptain = ranked[1]
		}
		for _, p := range xi {
			v.XIScore += p.Score
		}

		// What FPL would actually pay, which is the whole point of naming the
		// chip. The armband is counted once MORE than it already is in XIScore,
		// so a triple captain adds him twice again rather than three times.
		v.Expected = v.XIScore + v.Captain.Score
		switch chip {
		case "Bench Boost":
			for _, p := range bench {
				v.Expected += p.Score
			}
		case "Triple Captain":
			v.Expected += v.Captain.Score
		}
		views = append(views, v)
	}
	return views
}

// upcomingEvents is the next n gameweek numbers that any club actually plays,
// read off the fixture list rather than counted forward from the next event —
// a cancelled or rearranged gameweek would otherwise shift every week after it.
func (e *Engine) upcomingEvents(n int) []int {
	seen := map[int]bool{}
	var events []int
	for _, f := range e.Fixtures {
		if f.Finished || f.Event == nil || *f.Event <= 0 {
			continue
		}
		if !seen[*f.Event] {
			seen[*f.Event] = true
			events = append(events, *f.Event)
		}
	}
	sort.Ints(events)
	if n < len(events) {
		events = events[:n]
	}
	return events
}

// engineAt builds a horizon-1 engine anchored on one gameweek. It is a fresh
// engine every time, never the caller's — see WeekViews.
//
// The copy block carries **every** field the constructor does not set, for the
// reason set out on WeekEngine: this is the second of two derived-engine
// builders, `TeamForm` was added to neither, and a source wired into one view of
// the league and not the other is this package's most expensive recorded bug
// class. TestDerivedEnginesCarryEverySource enumerates the fields from the
// struct so the two cannot drift apart or fall behind it.
func (e *Engine) engineAt(gw int) *Engine {
	w := e.Weights
	w.Horizon = 1
	wk := NewEngineFull(e.Boot, e.Fixtures, w, e.Cong, e.Role)
	wk.Priors = e.Priors
	wk.Recent = e.Recent
	wk.MinutesOverride = e.MinutesOverride
	wk.MinutesOverrideUntil = e.MinutesOverrideUntil
	wk.TeamXGCFactor = e.TeamXGCFactor
	wk.TeamForm = e.TeamForm
	wk.SellPrices = e.SellPrices
	wk.Chips = e.Chips
	wk.Entry = e.Entry
	wk.SquadValue = e.SquadValue
	wk.HypotheticalBudget = e.HypotheticalBudget
	wk.Bank = e.Bank
	wk.Budget = e.Budget

	var skip []int
	for _, g := range e.upcomingEvents(38) {
		if g < gw {
			skip = append(skip, g)
		}
	}
	wk.SetSkipGameweeks(skip)
	return wk
}

// chipAt names the chip planned for a gameweek, or "".
//
// It reads the plan in config, which is a *hypothesis* rather than a commitment —
// the weekly review re-derives chip timing from what is known now and says when
// it differs. So this answers "what does the plan currently say about this
// week", which is the right question for a projection and the wrong one for a
// recommendation.
//
// Zero never matches, because an unplanned chip is 0 and there is no gameweek 0.
// A switch on gw would happily return "Wildcard" for every week if the plan were
// empty, which is why the zero case is excluded explicitly rather than relied on.
func (e *Engine) chipAt(gw int) string {
	if gw <= 0 {
		return ""
	}
	// Checked across both sets, so a second-set chip is named rather than
	// silently rendering as an ordinary week.
	for _, c := range []struct {
		slot, label string
	}{
		{SlotWildcard, "Wildcard"}, {SlotFreeHit, "Free Hit"},
		{SlotBenchBoost, "Bench Boost"}, {SlotTripleCaptain, "Triple Captain"},
	} {
		if e.Chips.Plays(c.slot, gw) {
			return c.label
		}
	}
	return ""
}
