package analysis

import (
	"fmt"
	"sort"
	"strings"

	"armband/internal/fpl"
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

// PlayableChips lists the chips the competition allows in gw, by key, in the order the
// bootstrap lists them.
//
// It reads the windows the FEED publishes rather than restating the rules. FPL gives every
// chip a start and a stop gameweek — on the 2026/27 payload the wildcard and the free hit
// open at gameweek 2 while the bench boost and the triple captain open at 1 — so "you cannot
// wildcard in gameweek 1" is already a fact in the data, and writing it out again here would
// be one rule with two statements that disagree the first time FPL moves a boundary.
//
// It answers only "does the competition allow it here", not "is it wise" and not "have you
// used it": a plan that has already spent a chip is the schedule's business, and whether a
// week is a good one for it is the model's.
//
// A gameweek in no window returns nothing, which is the honest answer — an empty rail beats
// offering a chip that would be refused.
func PlayableChips(boot *fpl.Bootstrap, gw int) []string {
	if boot == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range boot.Chips {
		if c.StartEvent == 0 && c.StopEvent == 0 {
			continue
		}
		// A stop of zero is an OPEN window, not a window that closed before the season
		// began. Reading it as a closed one drops the chip from every gameweek, and the
		// symptom is a chip row that is simply not there.
		stop := c.StopEvent
		if stop == 0 {
			stop = lastGameweek
		}
		if gw < c.StartEvent || gw > stop || seen[c.Name] {
			continue
		}
		// A chip this code cannot PLAY is not offered. Offering it would put a button on
		// the page that reaches chipsInto, matches no case, and does nothing — which is the
		// defect this whole surface was rewritten to remove. When FPL adds a chip, it
		// appears here by adding it to the schedule, not by leaking through.
		if chipOrder(c.Name) == unknownChip {
			continue
		}
		seen[c.Name] = true
		out = append(out, c.Name)
	}
	sort.SliceStable(out, func(i, j int) bool { return chipOrder(out[i]) < chipOrder(out[j]) })
	return out
}

// lastGameweek is the season's length, used to close an open-ended chip window.
const lastGameweek = 38

// unknownChip is what chipOrder returns for a key this code has no plan field for.
const unknownChip = 99

// chipOrder fixes the display order, which the bootstrap does not guarantee.
func chipOrder(key string) int {
	for i, k := range []string{"wildcard", "freehit", "bboost", "3xc"} {
		if k == key {
			return i
		}
	}
	return unknownChip
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
	return chipWindowsByKind(e.Boot)
}

func chipWindowsByKind(boot *fpl.Bootstrap) map[string][]ChipWindow {
	out := map[string][]ChipWindow{}
	if boot == nil {
		return out
	}
	for _, c := range boot.Chips {
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

// Place files a chip placement in the set whose window holds it, and reports whether it
// could be placed at all.
//
// FPL grants two sets of chips, the second from gameweek 20, and this schedule models both.
// A caller that knows only "the reader chose gameweek 25" cannot know which set that is
// without the feed's windows, and filing it in the wrong one is not a visible error: the
// plan is legal, the engine acts on it, and the squad is built around a chip the reader
// will not have. `containingSet` names that failure as distinct from an impossible week for
// exactly this reason.
//
// A gameweek in no window returns false and changes nothing, so a caller that has not
// validated its input cannot silently store a chip that can never be played.
func (s ChipSchedule) Place(boot *fpl.Bootstrap, key string, gw int) (ChipSchedule, bool) {
	set := containingSet(chipWindowsByKind(boot)[key], gw)
	if set == 0 {
		return s, false
	}
	plan := &s.First
	if set == 2 {
		plan = &s.Second
	}
	switch key {
	case "wildcard":
		plan.Wildcard = gw
	case "freehit":
		plan.FreeHit = gw
	case "bboost":
		plan.BenchBoost = gw
	case "3xc":
		plan.TripleCaptain = gw
	default:
		return s, false
	}
	return s, true
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

// ChipWindowStatus is the state of one chip window: its last gameweek, how
// many chips it grants, and how many of those are genuinely still unspent —
// counting ones already played, which a rail built from "current and
// upcoming" gameweeks cannot see on its own.
//
// It exists to fix a live client-side defect. `app.js:625` renders the
// remaining count as `(4 - GWS.filter(g => g.chip).length) + ' of 4 left'`,
// which is wrong three ways: `GWS` is the rail — current and upcoming only —
// so a chip already spent EARLIER in the window is never counted and the
// figure overstates what the reader has; the `4` is hard-coded, with no idea
// that a season has two windows; and it is the client deciding a rule about
// the competition, which `Gameweek.Playable`'s own doc comment forbids three
// lines away. This is what lets the client stop guessing.
type ChipWindowStatus struct {
	// EndsGW is the last gameweek of the window containing the gameweek
	// asked about — 19 for the first window in a two-set season, 38 for the
	// second (or for the whole season in a one-set one).
	EndsGW int
	// Size is how many chips this window grants. 4 today; not hard-coded,
	// because it is read off the same feed as everything else here.
	Size int
	// Remaining is how many of Size are not yet played as of the asked-
	// about gameweek: unplanned, or planned for a gameweek not yet reached.
	// A chip planned for a PAST gameweek relative to the one asked about
	// counts as spent even though the rail never showed it being spent —
	// that is the one-word fix for the defect above.
	Remaining int
}

// ChipWindowStatusAt reports the chip window containing gw, reading only the
// windows FPL's own feed publishes — the same source PlayableChips and
// ChipWindows read — so a season with one set of chips, or boundaries FPL
// moves, is answered correctly rather than by a rule restated here.
//
// The four chips in one set close together (NOTES.md: "eight chips a
// season, in two windows of four"), so the window's end is taken as the
// latest Stop among the windows that contain gw, found across every chip
// kind rather than assumed to be a single shared value — robust to a kind
// whose own window opens a gameweek or two later than its siblings, which
// the 2026/27 feed already does for the wildcard and the free hit.
//
// A gameweek outside every window returns the zero value: there is nothing
// left to report pressure about.
func (e *Engine) ChipWindowStatusAt(gw int) ChipWindowStatus {
	return ChipWindowStatusFor(e.Boot, e.Chips, gw)
}

// ChipWindowStatusFor is ChipWindowStatusAt without an Engine: a plain
// function of the bootstrap and a chip schedule, both of which
// internal/viewmodel already holds by the time it builds the rail. It exists
// so that package can call this directly — the same way it already calls
// PlayableChips — rather than reaching into an Engine, which would cross the
// line internal/viewmodel's own package comment draws: it arranges, it does
// not compute. ChipWindowStatusAt is this function's only caller inside this
// package, kept for callers that already have an Engine and nothing else.
func ChipWindowStatusFor(boot *fpl.Bootstrap, chips ChipSchedule, gw int) ChipWindowStatus {
	byKind := chipWindowsByKind(boot)

	end := 0
	for _, ws := range byKind {
		for _, w := range ws {
			if gw >= w.Start && gw <= w.Stop && w.Stop > end {
				end = w.Stop
			}
		}
	}
	if end == 0 {
		return ChipWindowStatus{}
	}

	var status ChipWindowStatus
	status.EndsGW = end
	for _, k := range chipKinds {
		for _, w := range byKind[string(k)] {
			if w.Stop != end {
				continue
			}
			status.Size++
			// WeekIn checks both sets against this window's own [Start,Stop]
			// range and returns whichever one is planned inside it, so this
			// does not need to know which set index this window happens to
			// be — the same reason WeekIn exists rather than a bare field
			// read.
			planned := chips.WeekIn(string(k), w.Start, w.Stop)
			if planned == 0 || planned >= gw {
				status.Remaining++
			}
		}
	}
	return status
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

// wantsWeek is the kind of gameweek each chip is anchored to. The wildcard is
// deliberately absent: no arm of `TestDiagAnchoredChips` plays one, and that
// file's own "no arm plays a wildcard, and that is the second repair" records
// why — a wildcard is the largest perturbation available to the path, and
// holding it fixed installs a worse confound than the one it removes. A chip
// this package cannot place is left to the manager rather than guessed at.
var wantsWeek = map[chipKind]string{
	kindFreeHit:       "blank",
	kindBenchBoost:    "double",
	kindTripleCaptain: "double",
}

// UnplannedChips reports chips the season has granted and the plan has not
// spent. An empty result means every chip the competition allows is accounted
// for.
//
// # Why this is a fourth function and not a branch of an existing one
//
// This file already keeps legality, wisdom and fixture facts apart because they
// fail differently. Completeness is a fourth kind and fails differently again: an
// unplanned chip is not illegal, so `ValidateChipPlan` must stay silent about it
// — its contract is "an empty result means it is legal", and an unspent chip is
// perfectly legal. It is not unwise either, so it is not a calendar note. It is
// simply not spent, and the only thing that can see it is a comparison between
// what the season GRANTS and what the plan FILLS.
//
// ⚠️ **Nothing detected this before.** `ValidateChipPlan` iterates the chips a
// plan names, so a set nobody planned produces zero iterations and zero
// messages. Measured on the shipped `config.json` on 2026-08-27: it fills the
// first set (wildcard GW6, bench boost GW8, triple captain GW9, free hit GW16)
// and leaves the second set entirely empty, in a season that grants two — and
// every existing surface reported the plan as fine.
//
// ⚠️ **It reports a COUNT and a name, never a points figure.** How much a chip
// is worth is a question for the replay, and the one recorded figure for
// spending both sets is a season-path number from a DIAG sweep that does not
// decompose cleanly into "what the second set alone is worth". Saying "you have
// four chips unspent" is settled by the plan and the calendar; saying what they
// would earn is not, and this function does not try.
//
// An unplayed chip is lost rather than carried, so "unplanned" and "forfeited"
// converge as a season runs on. That is why each message names its own set's
// deadline rather than leaving the reader to recall it.
//
// ⚠️ **That deadline is read from the published windows, never hardcoded.** The
// first version stated a fixed GW19, which is the first set's boundary only in a
// TWO-set season — `ChipSchedule.First`'s comment says the first set "runs the
// whole season in a one-set one" — so a one-set season was told its chips expired
// nineteen gameweeks early, and the trailing line quoted the FIRST set's date
// even when the SECOND was the one flagged. Both found by review.
func (e *Engine) UnplannedChips(s ChipSchedule) []string {
	windows := e.chipWindowsByKind()

	var out []string
	for _, set := range []int{1, 2} {
		var granted, unplanned []string
		expiry := 0
		for _, k := range chipKinds {
			// A set exists for this chip only when the competition published a
			// window for it. Read the boot rather than assuming two sets: how
			// many a season grants is a fact about the season, and this reads
			// FPL's own answer.
			if len(windows[string(k)]) < set {
				continue
			}
			label := chipLabels[string(k)]
			if label == "" {
				label = string(k)
			}
			granted = append(granted, label)
			if *s.field(k, set) > 0 {
				continue
			}
			unplanned = append(unplanned, label)
			// ⚠️ The deadline is READ, never assumed. An earlier version stated
			// a fixed GW19 here, which is the first set's boundary only in a
			// TWO-set season — `ChipSchedule.First`'s own comment says the first
			// set "runs the whole season in a one-set one", so a one-set season
			// was being told its chips expire nineteen gameweeks early. Found by
			// review, and `ValidateChipPlan` was already doing it correctly from
			// this same map, which made the constant a second statement of one
			// rule. Earliest wins when a set's chips somehow differ.
			if stop := windows[string(k)][set-1].Stop; stop > 0 && (expiry == 0 || stop < expiry) {
				expiry = stop
			}
		}
		if len(granted) == 0 || len(unplanned) == 0 {
			continue
		}
		which := "first set"
		if set == 2 {
			which = "second set"
		}
		// Named per set rather than once at the end: the trailing form quoted
		// the FIRST set's deadline even when the SECOND was the one flagged, so
		// a reader whose only gap was a second set got a date for chips they had
		// already planned.
		lost := ""
		if expiry > 0 {
			lost = fmt.Sprintf(" — unplayed, they are lost after GW%d", expiry)
		}
		if len(unplanned) == len(granted) {
			out = append(out, fmt.Sprintf(
				"the %s of chips is granted this season and NONE of its %d is planned (%s)%s",
				which, len(granted), strings.Join(unplanned, ", "), lost))
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s unplanned%s",
			which, strings.Join(unplanned, ", "), lost))
	}
	return out
}

// ChipCalendarNotes reports how a declared plan sits against the blanks and
// doubles the fixture list already shows for the squad actually held.
//
// This is the "is it wise" half that `PlayableChips` explicitly leaves alone —
// "It answers only 'does the competition allow it here', not 'is it wise' …
// whether a week is a good one for it is the model's" — and that
// `ValidateChipPlan` does not attempt. The two are kept apart because they fail
// differently: an illegal week is a mistake to correct, an ordinary week is a
// judgement to reconsider, and collapsing them would let a legality warning and
// a suggestion compete for the same line.
//
// ⚠️ **It reports FIXTURE FACTS and never a points figure.** "GW29 is a blank
// for four of your clubs" is settled by the fixture list. What anchoring a chip
// on the calendar is *worth* is not: the only thing that measures it is a
// DIAG-only sweep, `TestDiagAnchoredChips`, whose own closing instruction is to
// "read the lag columns, not the ceiling", and no figure from it ships. So these
// notes name weeks and players and stop there.
//
// ⚠️ **A count of blanks and doubles is not evenly comparable across seasons.**
// `ChipSetsFor`'s comment records that 11 of the 15 doubling club-gameweeks in
// the archived first halves are one COVID-rescheduled 2020-21 round. That is a
// caution about drawing rules from history, not about the current season's own
// fixture list, which is what this function reads.
//
// blanks and doubles are keyed by gameweek and hold the held-squad player names
// affected, as the caller computed them. An empty result means the plan has
// nothing to say against — either it fits, or no irregular week is known yet.
func ChipCalendarNotes(s ChipSchedule, blanks, doubles map[int][]string) []string {
	weeksOf := func(want string) []int {
		src := blanks
		if want == "double" {
			src = doubles
		}
		var out []int
		for gw, who := range src {
			if len(who) > 0 {
				out = append(out, gw)
			}
		}
		sort.Ints(out)
		return out
	}

	var notes []string
	for _, set := range []int{1, 2} {
		for _, k := range chipKinds {
			want, anchored := wantsWeek[k]
			if !anchored {
				continue
			}
			gw := *s.field(k, set)
			if gw <= 0 {
				continue
			}
			label := chipLabels[string(k)]
			if set == 2 {
				label += " (second set)"
			}

			candidates := weeksOf(want)
			if len(candidates) == 0 {
				continue
			}
			if hit := affected(blanks, doubles, want, gw); len(hit) > 0 {
				notes = append(notes, fmt.Sprintf(
					"%s is planned for GW%d, which is a %s for %s — the plan already sits on the calendar",
					label, gw, want, strings.Join(hit, ", ")))
				continue
			}
			notes = append(notes, fmt.Sprintf(
				"%s is planned for GW%d, an ordinary week for your squad. %s known so far: %s",
				label, gw, plural(want), joinWeeks(candidates, blanks, doubles, want)))
		}
	}
	return notes
}

// affected returns the held players a gameweek is the wanted kind for.
func affected(blanks, doubles map[int][]string, want string, gw int) []string {
	if want == "double" {
		return doubles[gw]
	}
	return blanks[gw]
}

func plural(want string) string {
	if want == "double" {
		return "Doubles"
	}
	return "Blanks"
}

// joinWeeks renders candidate weeks with how many of the held fifteen each hits,
// so a week touching four players is visibly worth more than one touching one.
func joinWeeks(gws []int, blanks, doubles map[int][]string, want string) string {
	parts := make([]string, 0, len(gws))
	for _, gw := range gws {
		parts = append(parts, fmt.Sprintf("GW%d (%d)", gw, len(affected(blanks, doubles, want, gw))))
	}
	return strings.Join(parts, ", ")
}
