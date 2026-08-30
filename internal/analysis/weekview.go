package analysis

import (
	"fmt"
	"sort"
	"strings"
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
	// RebuildFailed is true when a wildcard or free hit WAS eligible to rebuild
	// this gameweek -- Chip named one of them and the gameweek was still open --
	// but the rebuild could not be completed: AssemblyBudget errored, or
	// Optimize did. Squad is then the squad PASSED IN, unchanged, same as when
	// Rebuilt is false for any other reason (a Bench Boost/Triple Captain week,
	// or a closed gameweek).
	//
	// It exists to keep those two zero-Rebuilt cases from reading the same way.
	// Before this field, a failed Optimize call and a genuine "the model
	// confirms your current fifteen" verdict were indistinguishable downstream:
	// both left Squad equal to the squad passed in, both left Rebuilt false, and
	// buildChipTeam (internal/viewmodel/build.go) diffs Squad against the
	// account's real fifteen to report Changes/Out -- so a FAILED rebuild
	// rendered as "0 changes, nothing transferred out", a confident answer with
	// no error and no caveat, on /api/wildcard, which is public, ungated, and
	// cached for 300s. RebuildFailed is the one bit that lets a reader (and
	// buildChipTeam) tell "the model looked and nothing changed" from "the model
	// never looked".
	RebuildFailed bool
	// Squad is the fifteen this week is scored on — the one passed in, or the
	// rebuilt one when Rebuilt is true. Still the one passed in when
	// RebuildFailed is true.
	Squad []PlayerMetrics
	// Caveat is prose for a reader. It carries two DIFFERENT warnings,
	// mutually exclusive because Rebuilt and RebuildFailed cannot both be true:
	// rebuildCaveat's thin-evidence note when Rebuilt, or a plain "this rebuild
	// did not run" statement when RebuildFailed. Empty otherwise -- see
	// rebuildCaveat and rebuildFailedCaveat.
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
		// ⚠️ GameweeksPlayed counts FINISHED events, so it is still 0 through the
		// multi-day span between a gameweek's first kickoff and FPL marking it
		// finished — bonus is not locked until well after the last whistle. Saying
		// "no gameweek has been played" there is simply false, and it understates
		// the evidence twice over: FPL's aggregates carry every completed match's
		// minutes the moment they finish, so the fifteen is already moving on this
		// season's football even while `played` reads zero. Observed live on
		// 2026-08-24, when all ten GW1 matches had been played, the wildcard fifteen
		// visibly changed on that data, and this sentence still claimed none of it
		// existed. Seventh instance of the family inLiveGameweekGap exists to head
		// off — see its own comment for the three classes.
		if e.inLiveGameweekGap() {
			return "A gameweek is under way but not final yet, so the minutes behind " +
				"this fifteen are a partial read of this season on top of last, and they " +
				"will move again once the last match is scored. A wildcard or free hit " +
				"re-picks all fifteen, which is the decision most exposed to a thin " +
				"sample; four or five gameweeks settle most roles."
		}
		return "No gameweek has been played, so this fifteen rests entirely on last " +
			"season plus any manual minutes corrections. A wildcard or free hit re-picks " +
			"all fifteen, which is the decision most exposed to that — the recorded " +
			"guidance is to wildcard late enough that roles have resolved, and four or " +
			"five gameweeks of real football settles most of them."
	}
	// Singularised rather than "gameweek(s)" — a parenthesised plural is the one
	// register this page otherwise avoids, and "1 gameweek(s) have" is also a
	// number/verb mismatch on top of it.
	noun, verb := "gameweeks", "have"
	if played == 1 {
		noun, verb = "gameweek", "has"
	}
	return fmt.Sprintf("Only %d %s %s been played, so the minutes behind this "+
		"fifteen are about %.0f%% this season and %.0f%% last. A wildcard or free hit "+
		"re-picks all fifteen on that mix, and it is the decision most exposed to a thin "+
		"sample; four or five gameweeks settle most roles.",
		played, noun, verb, share*100, (1-share)*100)
}

// rebuildFailedCaveat states plainly that a wildcard or free-hit rebuild did
// not run, so the reader does not mistake the squad shown for a
// recommendation.
//
// It deliberately says nothing about WHY -- not AssemblyBudget's error text,
// not Optimize's. This document is /api/wildcard: public, unauthenticated,
// and cached for every reader (see cmd/armband/chipteams.go's own comment for
// why that is safe only because the route takes no per-reader input). A
// diagnostic message is exactly the kind of detail a sibling change had to
// strip from this same payload for leaking the operator's own chip strategy
// (plan_wildcard_gw/plan_free_hit_gw) -- so this stays a fixed sentence, not
// a formatted error.
func rebuildFailedCaveat(chip string) string {
	// chip is "Wildcard" or "Free Hit" -- lowercased here so it reads as a
	// sentence rather than a proper-noun label, matching rebuildCaveat's own
	// register ("a wildcard or free hit re-picks all fifteen").
	return fmt.Sprintf(
		"The %s could not be rebuilt for this gameweek, so what is shown below "+
			"is your CURRENT squad, unchanged — not a recommendation, and not a "+
			"suggestion to hold. Try reloading in a moment.", strings.ToLower(chip))
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
func (e *Engine) WeekViews(squad []PlayerMetrics, n int, req OptimizeRequest) []WeekView {
	events := e.upcomingEvents(n)
	views := make([]WeekView, 0, len(events))

	for _, gw := range events {
		views = append(views, e.ChipWeekView(squad, gw, e.chipAt(gw), req))
	}
	return views
}

// ChipWeekView is one gameweek scored under a NAMED chip, whatever the
// configured plan says about that week.
//
// WeekViews asks the same question of the plan (chipAt) and is the schedule's
// reader. This is the hypothetical's: "what would a wildcard buy in gameweek
// 3", asked of a season whose plan points at gameweek 6. One implementation,
// two callers, because a second copy of the rebuild is how the page and the
// rail come to disagree about what a wildcard buys.
//
// req carries the caller's roster constraints -- LockIDs, StartIDs,
// ExcludeIDs. Budget, MinMinutes and MinExpectedMinutes are set here and any
// value the caller put in them is overwritten: the budget is AssemblyBudget's
// and the two floors are the rebuild's own, and a caller that could change
// them could publish a fifteen built to a different question than the rail's.
//
// gw is a gameweek number, NOT an index into upcomingEvents: that list
// filters on f.Finished, so a gameweek being played right now is still in it,
// and nobody can chip into a gameweek whose deadline has passed.
func (e *Engine) ChipWeekView(squad []PlayerMetrics, gw int, chip string,
	req OptimizeRequest) WeekView {

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
	weekSquad, rebuilt, rebuildFailed := squad, false, false
	// Nobody can chip into a gameweek whose deadline has passed. ChipWeekView
	// takes no clock, so it cannot check the deadline directly the way
	// nextOpenEvent does -- but a gameweek every one of whose fixtures has
	// already finished is unambiguously behind it, and that is a fact about
	// the fixture list this engine already carries. A caller that hands in a
	// closed gameweek gets the squad back unrebuilt rather than a fifteen for
	// a chip nobody could actually have played.
	if (chip == "Wildcard" || chip == "Free Hit") && !e.gameweekClosed(gw) {
		// Eligible to rebuild as of here -- set the failure bit now and clear
		// it only on confirmed success below. Every early exit from this
		// block (AssemblyBudget's error, Optimize's) then leaves
		// rebuildFailed true rather than silently false; see
		// WeekView.RebuildFailed's own comment for why the silent case was a
		// live bug.
		rebuildFailed = true
		if budget, _, err := e.AssemblyBudget(); err == nil {
			rebuildReq := req
			rebuildReq.Budget = budget
			rebuildReq.MinMinutes = PoolMinMinutes
			rebuildReq.MinExpectedMinutes = PoolMinExpectedMinutes
			// The two chips want fifteens built to different questions, so
			// they are built on different engines.
			//
			// A FREE HIT fields its fifteen for this one round and hands the
			// permanent squad back, so one gameweek is the whole horizon and
			// `wk` is the right engine. A player whose club does not play is
			// then not merely a poor pick — he cannot appear at all, bench
			// included — and `fixtureLoadFor` takes his score to zero, which
			// keeps him out of the eleven and does nothing about the four
			// bench slots, since a builder is indifferent between two players
			// worth nothing and takes the cheapest. So the guard is applied to
			// the POOL as well.
			//
			// A WILDCARD is the opposite: that fifteen is KEPT, so it must be
			// built over the horizon rather than for one round. It cannot use
			// `wk` at all. `wk` is horizon 1, where `FixtureLoadInScore()` is
			// true, so every blanking club's score is already zero before
			// `Optimize` ranks on it — and a wildcard planned for a heavy blank
			// (2023-24 GW29 blanked twelve clubs of twenty) would return a
			// permanent squad drawn entirely from the eight clubs that happened
			// to play that week. That is a free-hit squad presented as a
			// wildcard, and it is reachable only since `fixtureLoadFor` learned
			// to express a blank: before that, a blanking club read >= 1 and the
			// distortion could not arise. So the wildcard is built at the
			// caller's horizon, anchored on its own week, where the same blank
			// is correctly one week in five rather than the whole world.
			builder := wk
			if chip == "Wildcard" {
				builder = e.engineAtHorizon(gw, e.Weights.Horizon)
				// FPL allows one chip per gameweek, so a wildcard can never
				// *be* the bench boost week — it prepares for the one right
				// after it, which is the sequence the chip is actually used
				// in. Telling the rebuild so makes it optimise all fifteen
				// rather than the ordinary amortised bench weighting; a
				// boost further out is left to that amortised weighting.
				// Matches the replay's `playWildcard`, which sets the same
				// field from `cfg.plays(slotBenchBoost, gw+1)` — one
				// quantity, kept to one meaning across both paths.
				rebuildReq.BenchBoost = e.Chips.Plays(SlotBenchBoost, gw+1)
			} else {
				rebuildReq.ExcludeIDs = append(append([]int(nil), rebuildReq.ExcludeIDs...), wk.ElementsWithoutFixtures()...)
			}
			if built, err := builder.Optimize(rebuildReq); err == nil && built != nil {
				weekSquad, rebuilt, rebuildFailed = built.Players, true, false
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
		Opponents: opponents, Chip: chip, Rebuilt: rebuilt,
		RebuildFailed: rebuildFailed, Squad: scored,
	}
	switch {
	case rebuilt:
		v.Caveat = e.rebuildCaveat()
	case rebuildFailed:
		v.Caveat = rebuildFailedCaveat(chip)
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
	return v
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

// gameweekClosed reports whether every fixture in gw has already finished.
// An empty gw (no fixture at all, e.g. one that has fallen off the end of the
// fixture list) is NOT closed by this rule — there is nothing to say it is
// behind the deadline rather than simply outside the data this engine holds,
// and treating "unknown" as "closed" would silently swallow a caller's typo.
func (e *Engine) gameweekClosed(gw int) bool {
	found := false
	for _, f := range e.Fixtures {
		if f.Event == nil || *f.Event != gw {
			continue
		}
		found = true
		if !f.Finished {
			return false
		}
	}
	return found
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
func (e *Engine) engineAt(gw int) *Engine { return e.engineAtHorizon(gw, 1) }

// PoolAt scores every player in the pool against ONE gameweek, at horizon 1.
//
// AllMetrics answers "how good is this player over the configured horizon",
// which is the right question for a squad you keep and the wrong one for a
// single week: it averages over the horizon's fixtures, so a double reads as
// two ordinary weeks and a blank is diluted rather than empty. Only at horizon
// 1 does FixtureLoad reach Score at all, which is the difference between a
// ranking that knows who plays twice and one that does not.
//
// ⚠️ **It is deliberately NOT WeekViews with the whole pool handed in as the
// squad.** That route works, and it is how this project's first per-gameweek
// ranking was built, but it sends ~600 players through an XI picker, a
// formation search and a chip rebuild to arrive at a per-player number that
// none of the three affect. Worse, it answers a squad-shaped question, so a
// reader of that code cannot tell whether the eleven it picked was load-bearing.
// This is the same derived engine and the same per-player scoring with the
// detour removed.
func (e *Engine) PoolAt(gw int) []PlayerMetrics { return e.engineAt(gw).AllMetrics() }

// engineAtHorizon is engineAt with the horizon named.
//
// Horizon 1 is the week view itself. Anything longer is for a decision that
// outlives the week: a wildcard's fifteen is kept, so it is built over the
// horizon starting at its own gameweek rather than for that gameweek alone. The
// skip set does the anchoring in both cases — `TeamFixtures` extends past a
// skipped week, so N means N gameweeks from `gw` onward.
func (e *Engine) engineAtHorizon(gw, horizon int) *Engine {
	w := e.Weights
	w.Horizon = horizon
	wk := NewEngineFull(e.Boot, e.Fixtures, w, e.Cong, e.Role)
	wk.Priors = e.Priors
	wk.Recent = e.Recent
	// Carried, or the eleven is fielded under one selection policy and the
	// transfer decided under another — the split
	// TestDerivedEnginesCarryEverySource records for TeamForm, and inert in
	// exactly the same way until somebody switches the flag on.
	wk.Tiebreak = e.Tiebreak
	wk.MinutesOverride = e.MinutesOverride
	wk.MinutesOverrideUntil = e.MinutesOverrideUntil
	wk.MinutesOverrideConfirmed = e.MinutesOverrideConfirmed
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
