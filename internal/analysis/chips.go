package analysis

import (
	"fmt"
	"sort"
	"strings"
)

// ChipPlan records when you intend to play each chip. Zero means unplanned.
//
// Planning matters because chips expire. The first-half set must be used by
// gameweek 19 or it is lost, and only one chip may be played per gameweek — so
// four chips need four distinct gameweeks inside a fixed window. Deciding week
// by week reliably ends with two chips burned in the last fortnight.
//
// The plan also feeds squad construction, which is the part most managers miss:
//   - A wildcard at gameweek N makes the current squad disposable after N-1, so
//     it should be built for a short horizon, not a five-gameweek one.
//   - A bench boost needs fifteen playable footballers, not eleven plus fodder.
//     Planning one changes how the budget is spread across the whole squad.
type ChipPlan struct {
	Wildcard      int `json:"wildcard_gameweek"`
	FreeHit       int `json:"free_hit_gameweek"`
	BenchBoost    int `json:"bench_boost_gameweek"`
	TripleCaptain int `json:"triple_captain_gameweek"`
}

// ChipWindow is a chip's legal gameweek range, read from the FPL API.
type ChipWindow struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Start int    `json:"start_gameweek"`
	Stop  int    `json:"stop_gameweek"`
}

var chipLabels = map[string]string{
	"wildcard": "Wildcard",
	"freehit":  "Free Hit",
	"bboost":   "Bench Boost",
	"3xc":      "Triple Captain",
}

// firstGameweekChips are the only chips playable in gameweek 1: the bench boost and the
// triple captain.
//
// The wildcard and the free hit are not offered there. The wildcard's redundancy is the
// evident half — transfers are unlimited until the first deadline, so a chip granting
// unlimited transfers buys nothing — but that is an observation about why the rule is
// unsurprising, not a derivation of it. The rule is the game's.
//
// It lives here because it is a fact about the competition, alongside the chip windows and
// the two-set schedule. A page that decided this for itself would be a second statement of
// a rule the model also has to obey, and the two would disagree the first time either
// changed.
var firstGameweekChips = map[string]bool{"bboost": true, "3xc": true}

// PlayableChips lists the chips that may be played in gw, by key, in a stable order.
//
// It answers only "does the competition allow it here", not "is it wise" and not "have you
// used it" — a plan that has already spent a chip is the schedule's business, and whether a
// week is a good one for it is the model's.
func PlayableChips(gw int) []string {
	all := []string{"wildcard", "freehit", "bboost", "3xc"}
	if gw != 1 {
		return all
	}
	out := make([]string, 0, len(firstGameweekChips))
	for _, k := range all {
		if firstGameweekChips[k] {
			out = append(out, k)
		}
	}
	return out
}

// ChipLabel is the human name for a chip key, or the key itself if it is unknown — an
// unrecognised chip should be visible rather than silently dropped.
func ChipLabel(key string) string {
	if l, ok := chipLabels[key]; ok {
		return l
	}
	return key
}

// ChipWindows returns each chip's legal range. FPL splits chips into two sets;
// only the set covering the upcoming gameweek is returned.
func (e *Engine) ChipWindows() []ChipWindow {
	gw := 1
	if next := e.Boot.NextEvent(); next != nil {
		gw = next.ID
	}

	// FPL issues two sets of chips per season. Keep only the earliest window
	// per chip that has not yet expired — that is the set currently in play.
	earliest := map[string]ChipWindow{}
	for _, c := range e.Boot.Chips {
		if c.StartEvent == 0 && c.StopEvent == 0 {
			continue
		}
		if gw > c.StopEvent {
			continue // that half's chip has already expired
		}
		label := chipLabels[c.Name]
		if label == "" {
			label = c.Name
		}
		w := ChipWindow{Name: c.Name, Label: label, Start: c.StartEvent, Stop: c.StopEvent}
		if cur, ok := earliest[c.Name]; !ok || w.Stop < cur.Stop {
			earliest[c.Name] = w
		}
	}

	out := make([]ChipWindow, 0, len(earliest))
	for _, w := range earliest {
		out = append(out, w)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Stop != out[j].Stop {
			return out[i].Stop < out[j].Stop
		}
		return out[i].Start < out[j].Start
	})
	return out
}

// chipWindowsByKind groups every window the bootstrap lists, per chip, ascending
// by start gameweek — so index 0 is the first set's window and index 1 the
// second's.
//
// `ChipWindows` deliberately returns only the set currently in play, which is the
// right answer for "what can I use now" and the wrong one for validating a plan
// that reaches past the reset. Validating a second-set chip against the first
// set's window reported "not available in the current half of the season" for a
// plan that was perfectly legal, which is worse than no check: it trains the
// reader to ignore the output.
func (e *Engine) chipWindowsByKind() map[string][]ChipWindow {
	out := map[string][]ChipWindow{}
	for _, c := range e.Boot.Chips {
		if c.StartEvent == 0 && c.StopEvent == 0 {
			continue
		}
		label := chipLabels[c.Name]
		if label == "" {
			label = c.Name
		}
		out[c.Name] = append(out[c.Name], ChipWindow{
			Name: c.Name, Label: label, Start: c.StartEvent, Stop: c.StopEvent,
		})
	}
	for name := range out {
		w := out[name]
		sort.SliceStable(w, func(i, j int) bool { return w[i].Start < w[j].Start })
	}
	return out
}

// containingSet is the 1-based set whose window holds this gameweek, or 0.
//
// Used to tell "this chip is in an impossible week" apart from "this chip is in
// a legal week but filed under the wrong set", which are different problems with
// different fixes and read identically if both are reported as out-of-window.
func containingSet(ws []ChipWindow, gw int) int {
	for i, w := range ws {
		if gw >= w.Start && gw <= w.Stop {
			return i + 1
		}
	}
	return 0
}

// ValidateChipPlan checks a schedule against the real chip windows and FPL's
// rules, returning one message per problem. An empty result means it is legal.
//
// Each slot is checked against the window of *its own set*: `bb2` against the
// second bench-boost window, not against whichever one happens to be current.
func (e *Engine) ValidateChipPlan(s ChipSchedule) []string {
	windows := e.chipWindowsByKind()

	nextGW := 1
	if next := e.Boot.NextEvent(); next != nil {
		nextGW = next.ID
	}

	var problems []string
	byGW := map[int][]string{}

	for _, set := range []int{1, 2} {
		for _, k := range chipKinds {
			gw := *s.field(k, set)
			if gw <= 0 {
				continue
			}
			label := chipLabels[string(k)]
			if label == "" {
				label = string(k)
			}
			if set == 2 {
				label += " (second set)"
			}
			byGW[gw] = append(byGW[gw], label)

			ws := windows[string(k)]
			if len(ws) < set {
				problems = append(problems, fmt.Sprintf(
					"%s is planned for GW%d but this season grants %d set(s) of chips",
					label, gw, len(ws)))
				continue
			}
			// Checked against its own set's window FIRST, and against the other
			// sets only to tell two different problems apart.
			//
			// A flat legacy chip_plan carries no set information, so
			// `UnmarshalJSON` reads all of it as set 1 — which means a perfectly
			// legal "bench boost GW34" would be reported as outside its GW1-19
			// window, a FALSE error and a regression against the behaviour this
			// function had when it read only the current set. The chip is not in
			// the wrong week; it is filed under the wrong set, and saying so is
			// the actionable message. Found by review.
			w := ws[set-1]
			if gw < w.Start || gw > w.Stop {
				if other := containingSet(ws, gw); other > 0 {
					problems = append(problems, fmt.Sprintf(
						"%s planned for GW%d, which is in the set-%d window GW%d-%d "+
							"rather than the set-%d window GW%d-%d — file it as %s%d",
						label, gw, other, ws[other-1].Start, ws[other-1].Stop,
						set, w.Start, w.Stop, shortName[k], other))
				} else {
					problems = append(problems, fmt.Sprintf(
						"%s planned for GW%d but its window is GW%d-%d", label, gw, w.Start, w.Stop))
				}
			}
			if gw < nextGW {
				problems = append(problems, fmt.Sprintf(
					"%s planned for GW%d, which has already passed", label, gw))
			}
		}
	}

	for gw, chips := range byGW {
		if len(chips) > 1 {
			sort.Strings(chips)
			problems = append(problems, fmt.Sprintf(
				"only one chip may be played per gameweek, but GW%d has %s",
				gw, strings.Join(chips, " and ")))
		}
	}

	// Unplanned chips that will expire, reported only for the window the season
	// is CURRENTLY IN.
	//
	// A first draft suppressed only windows already behind us, which reads as the
	// same rule and is not: in a two-set season every second-set window stops at
	// GW38, so from GW1 it reported all four second-set chips as expiring — four
	// permanent lines on the shipped config, where the old code reported none, and
	// they are replayed to the agent in `get_chip_plan`'s issues array on every
	// call. Worse, the comment claimed to be preventing exactly that. Found by
	// review; the guard now matches what it says.
	//
	// Containment reproduces the old `ChipWindows` behaviour — the set in play —
	// and extends correctly to the second set once GW20 arrives.
	for _, set := range []int{1, 2} {
		for _, k := range chipKinds {
			ws := windows[string(k)]
			if len(ws) < set {
				continue
			}
			w := ws[set-1]
			if nextGW < w.Start || nextGW > w.Stop || *s.field(k, set) > 0 {
				continue
			}
			label := w.Label
			if set == 2 {
				label += " (second set)"
			}
			problems = append(problems, fmt.Sprintf(
				"%s is unplanned and expires after GW%d", label, w.Stop))
		}
	}

	sort.Strings(problems)
	return problems
}

// EffectiveHorizon is how many gameweeks the current squad actually needs to
// serve. A wildcard replaces the squad, so there is no value in optimising past
// it.
//
// # The free hit is not a barrier, and it used to be treated as one
//
// This read "a wildcard *or free hit* replaces the squad" and truncated at both.
// That is wrong about what the chip does: a free hit fields a *separate*
// temporary fifteen for one gameweek and hands the permanent squad straight back
// the week after. The squad's life is not ended by it — one week is removed from
// its earning stream, which is a different thing and is already modelled, by
// `ApplyFreeHitToScoring` dropping that gameweek from scoring and by
// `TeamFixtures` extending the window past skipped weeks so `n` still means n
// gameweeks that count.
//
// Truncating here as well both double-counted the free hit and got its shape
// wrong: a free hit at GW20 made a GW15 squad optimise over five gameweeks
// instead of the whole remaining season, on the reasoning that it would be
// thrown away — when in fact every one of those players is still owned at GW21.
//
// **It was found as a disagreement rather than by reading this function.**
// `backtest.SimConfig.chipCredit` excludes the free hit from its own barrier and
// says why, and `TestAWildcardEndsThePreparationWindow` pins that. Two
// implementations of "which chips end this squad's life" disagreed about the same
// chip, which is this project's signature failure; the one with the argument
// written down turned out to be the correct one.
//
// **It ships on mechanism, not on points — but it does move a recorded figure,
// and the first version of this comment said otherwise.** `AnticipateChips` is
// off by default, so nothing *shipped* changes. It is not inert, though:
// `TestDiagChipAnticipation` sets it **and plans a free hit**, so four decision
// weeks per cell that ran on a truncated horizon now run on the full one — more
// than half that arm's mechanism, since its wildcard only ever supplied three.
// Its recorded figures (the coherent arm's +2.5 a season, the −17 mismatch, and
// the 201→223 and 201→128 transfer counts in the chips note) **will not
// reproduce**. They are superseded by this change rather than merely stale.
// # Two sets, and why this asks for the NEXT wildcard rather than the wildcard
//
// With a second set there can be a wildcard behind the decision and another
// ahead of it. Reading a single field returns whichever set happens to hold it,
// so a squad built at GW22 would truncate at the GW6 wildcard it already played
// — that is, not at all — and a two-set plan reverts to single-set behaviour
// with nothing failing. `ChipSchedule.Next` is the shared answer to that, and
// `backtest.SimConfig.nextChip` was the first copy of it.
func (e *Engine) EffectiveHorizon(s ChipSchedule) (int, string) {
	nextGW := 1
	if next := e.Boot.NextEvent(); next != nil {
		nextGW = next.ID
	}
	horizon := e.Weights.Horizon

	best, reason := horizon, ""
	// Strictly after the upcoming gameweek: a wildcard played this week rebuilds
	// the squad now rather than ending the life of the one being built.
	if gw := s.Next(SlotWildcard, nextGW+1); gw > 0 {
		if span := gw - nextGW; span < best {
			best = span
			reason = fmt.Sprintf("%s planned for GW%d, so this squad only needs to last GW%d-%d",
				chipLabels["wildcard"], gw, nextGW, gw-1)
		}
	}
	if best < 1 {
		best = 1
	}
	return best, reason
}

// BenchBoostActive reports whether a bench boost falls inside the horizon the
// current squad must serve. When it does, every one of the fifteen players
// scores, so cheap non-playing fodder stops being free.
func (e *Engine) BenchBoostActive(s ChipSchedule) bool {
	return e.activeBenchBoost(s) > 0
}

// activeBenchBoost is BenchBoostActive's answer plus *which* boost it found, so
// SuggestBenchWeight can name the gameweek in its note rather than reading a
// field that may hold the other set's chip.
func (e *Engine) activeBenchBoost(s ChipSchedule) int {
	nextGW := 1
	if next := e.Boot.NextEvent(); next != nil {
		nextGW = next.ID
	}
	// The next boost at or after the upcoming gameweek. A boost already played
	// is not one the squad still has to be built for.
	boost := s.Next(SlotBenchBoost, nextGW)
	if boost == 0 {
		return 0
	}
	horizon, _ := e.EffectiveHorizon(s)
	// The window is counted in gameweeks that *score*, not calendar weeks, because
	// that is what the squad is actually scored over: `TeamFixtures` drops a
	// skipped week and extends past it, so `horizon` means n weeks that count.
	//
	// A free hit inside the window is the case, and it was unreachable until
	// `EffectiveHorizon` stopped truncating there. Next GW8, horizon 5, free hit
	// GW10, boost GW13: the scored window is {8,9,11,12,13} and contains the
	// boost, while a calendar test reads `13 < 13` and says no — so the squad
	// would be built with four non-players for a week it plays all fifteen.
	//
	// Both free hits count, not just the nearest: two of them inside one window
	// push the boost two weeks further out, and asking only about the next one
	// would under-extend exactly when the schedule is busiest.
	last := nextGW + horizon - 1
	for gw := nextGW; gw <= last && gw <= 38; gw++ {
		if s.Plays(SlotFreeHit, gw) {
			last++ // that week scores nothing, so the window reaches one further
		}
	}
	if boost >= nextGW && boost <= last {
		return boost
	}
	return 0
}

// SuggestBenchWeight returns the bench weight implied by the plan: a planned
// bench boost inside the horizon makes the bench nearly as valuable as the XI
// for the week it is played, amortised across the horizon.
// This is the *amortised* treatment, for a squad that has to serve the whole
// horizon and happens to contain a boost week. `OptimizeRequest.BenchBoost` is
// the other case — building the squad for the boost week itself, as in the
// standard wildcard-then-boost-then-transfer-out sequence — and there the bench
// counts in full rather than at a spread weight. Keep both: they answer
// different questions, and BenchBoost overrides this when set.
func (e *Engine) SuggestBenchWeight(s ChipSchedule) (float64, string) {
	base := e.Weights.BenchWeight
	boost := e.activeBenchBoost(s)
	if boost == 0 {
		return base, ""
	}
	horizon, _ := e.EffectiveHorizon(s)
	if horizon < 1 {
		horizon = 1
	}
	// For one gameweek in the horizon the bench scores in full.
	boosted := base + (1.0-base)/float64(horizon)
	if boosted > 1 {
		boosted = 1
	}
	return boosted, fmt.Sprintf(
		"bench boost planned for GW%d: bench weight raised %.2f -> %.2f so the squad is built with fifteen playable footballers",
		boost, base, boosted)
}

// ApplyChipPlan adjusts an optimise request to match the chip plan: shortening
// the horizon before a wildcard, and raising the bench weight for a bench boost.
// It returns the notes explaining what changed, for display.
func (e *Engine) ApplyChipPlan(req *OptimizeRequest) []string {
	var notes []string

	if horizon, why := e.EffectiveHorizon(e.Chips); why != "" && horizon != e.Weights.Horizon {
		e.Weights.Horizon = horizon
		e.buildFixtureIndex()
		notes = append(notes, why)
	}
	if bw, why := e.SuggestBenchWeight(e.Chips); why != "" {
		req.BenchWeight = bw
		notes = append(notes, why)
	}
	return notes
}

// ApplyFreeHitToScoring excludes a planned free-hit gameweek from scoring.
//
// The free hit is the one chip that makes a gameweek irrelevant to the squad
// being built: it fields a separate temporary fifteen and hands the permanent
// squad back the following week. Every other chip is played *with* the squad,
// so the squad still has to be good that week.
//
// This is the plan-derived default. The analysis layer can override it wholesale
// with SetSkipGameweeks when it knows something the plan does not.
// Both sets are excluded, not merely the next one. Skipping only the nearest
// free hit leaves a squad being optimised for a week it will not field — the
// exact error this function exists to prevent, reintroduced by a plan that is
// one chip longer.
func (e *Engine) ApplyFreeHitToScoring() {
	if weeks := e.Chips.Weeks(SlotFreeHit); len(weeks) > 0 {
		e.SetSkipGameweeks(weeks)
	}
}
