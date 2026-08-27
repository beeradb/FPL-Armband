package backtest

// Does anchoring the chips on the fixture calendar beat placing them by week
// number?
//
//	DIAG=1 EXP=ANCHORED FPL_CELLS=/tmp/anchored/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagAnchoredChips$' -count=1 -v -timeout 4h
//
// This is how the chips are actually played: free hit the blank, build toward the
// double, boost it, wildcard into or out of it. Every chip week this harness has
// ever measured is a fixed number, a fixed offset from entry, or a hindsight
// argmax over per-week gains — so the strategy that dominates real play has never
// been in the search space at all. `TestDiagFixtureCalendar` established that
// there is something to anchor to: 42 double club-gameweeks in 2022-23 falling to
// 10 in each of the last two, and irregular weeks that sit almost entirely in the
// second half.
//
// # The first version of this measurement was broken, and this is the repair
//
// It reported +10.8 points a season and the figure is retracted. Three defects
// produced it, none about anchoring, and the repairs are the reason this file now
// looks the way it does:
//
//   - **The control could not match the chip set at a late entry.** It placed the
//     triple captain at `start+13`, which at a GW26 entry is GW39, and dropped it.
//     Three cells of twenty-four therefore compared four chips against three, in
//     the direction of the reported effect. Offsets now place *backwards* from
//     GW38 when they overrun, and a chip no arm can place is dropped from every
//     arm — see `matchedChips`.
//   - **The lag arms played a different number of chips from each other.** The
//     lookahead in `firstUnbeaten` skipped a taken week as a *candidate* while
//     still counting it as competition, so a week spent on the bench boost vetoed
//     every earlier week for the triple captain and the function returned nothing.
//     `laggedPlan(38)` came back with three chips against `anchoredPlan`'s four in
//     five cells, which is a mechanical explanation for full sight underperforming
//     a four-gameweek lag. There is now one placement rule at a parameterised
//     sight and full sight *is* the anchored arm, so the two cannot diverge.
//   - **The wildcard was the whole early-phase result.** The control wildcarded at
//     `start+3`, which from a GW1 entry is GW4 — the one week this record
//     establishes as reliably positive — while the anchored arm wildcarded at
//     GW28-33, where the same sweep reads −0.8 to −19.8. That is a re-run of the
//     GW4 wildcard finding wearing a calendar label.
//
// A fourth defect was in the reporting rather than the code: three chips are an
// **event count**, so the per-gameweek convention inflates them. See
// the harness-and-inference note; read the per-season-path column first.
//
// # No arm plays a wildcard, and that is the second repair
//
// The obvious fix for the third defect above is to hold the wildcard at the same
// week in every arm, so it becomes a nuisance factor shared by the pair. **That
// was tried and it installs a worse confound than the one it removes.** The
// anchored arm's wildcard week is `bigDouble − 1` and its bench boost week is
// `bigDouble`, so at full sight the boost lands the gameweek after the rebuild
// **by construction, in every cell** — counted over the archive, 30 of 30 against
// 3 to 5 of 30 for the lag arms and 5 of 30 for the control.
//
// That is not a shared nuisance factor. This record already establishes that a
// bench boost played on a wildcard-rebuilt squad is worth materially more — the
// optimiser buys 59% more bench quality for £2.5m when it is told the chip is
// coming — and that measuring a boost on a squad built for something else
// measures a *floor*. So the one arm whose boost lands on a prepared squad would
// have been the arm the measurement is arguing for, in the direction of the
// reported effect.
//
// **So the wildcard is played by nobody.** Every arm boosts a squad built for
// something else, every arm measures the same floor, and the floor cancels. What
// is lost is the "build toward the double" half of the strategy, which this
// harness cannot measure anyway: the wildcard is the largest perturbation
// available to a chaotic path, its measured value at a given week spans −267 to
// +91 within one season, and the record's standing verdict is that wildcard
// timing is unmeasurable here. Confounding the three chips that *are* measurable
// with the one that is not was the mistake.
//
// # How much of this is hindsight
//
// The first version called the full-sight arm an oracle and labelled it
// HINDSIGHT, on the reasoning that the archive's fixture list is final from GW1
// where FPL's resolves as cup rounds finish. **That overstates it, and the
// correction comes from someone who has played the game rather than from the
// archive.**
//
// Blanks and doubles are speculated about for weeks. The fixture list is public,
// so which clubs have postponed matches — and therefore the *pool* of candidate
// doublers — is knowable well in advance; what resolves late is exactly which of
// that pool doubles, and in which week. There are surprises, and the pool is not
// one of them.
//
// So the honest split is:
//
//   - **which gameweek** carries a double: largely knowable early, and this is all
//     the arms below use, because the planner chooses *weeks* and nothing else;
//   - **which clubs** double: a known pool resolving to a known set late, which is
//     what a "build toward the double" strategy needs and what nothing here wires;
//   - **the exact magnitude**: genuinely uncertain, and the lag sweep below prices
//     the cost of guessing the wrong week because of it.
//
// The residual is a judgement-layer question — "there will be a double around GW34,
// probably these clubs, certain in a fortnight" is a web-search task the agent
// already does for team news — and this harness has no judgement layer, which is
// the same reason it cannot measure perfect team news. Read the full-sight arm as a
// **target**, not as a fiction.
//
// # The control is matched on chip *set*, not just on count
//
// Placement is the only thing being tested, so every arm must play the same chips.
// A season's biggest blank can fall before a cell's entry gameweek — 2022-23's is
// GW8, unreachable from a GW11 entry — and dropping that chip from one arm while
// another still plays it would measure a chip against no chip. `matchedChips`
// intersects what every arm in the grid can place and masks all of them to it, so
// the matched-set guarantee holds in both directions rather than only one.

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

// sightLags are the reveal lags the sweep runs, and the single definition the
// arms and the matched set both read.
//
// A second list would let the sweep run an arm the matched set never considered,
// which is the "one quantity, two expressions" failure this package keeps paying
// for — and here it would silently reintroduce the unequal-chip-count defect this
// file exists to fix.
var sightLags = []int{2, 4, 6}

// fullSight is a lag long enough to see the rest of any season from any entry.
const fullSight = 38

// controlOffsets place the three tested chips at fixed gameweek counts from
// entry — the placement rule every earlier chip measurement here used.
//
// The wildcard has no offset because no arm plays one; see the file header.
// Spread so they cannot collide, and see `controlWeeks` for what happens when one
// overruns the season.
var controlOffsets = struct{ benchBoost, freeHit, tripleCaptain int }{6, 10, 13}

// minAnchorClubs lives in chipsplan.go — one definition for the measurement
// planner and the functional one, so the two cannot drift into two bars.
//
// **Asserted, not measured**, and the lag columns are sensitive to it. Four is
// the floor of the range the arms are choosing among rather than a tuned value:
// the full-sight anchors across the five archived seasons involve 4 to 12
// clubs, and a two-club double reaches at most two of a fifteen — which no
// manager holds a chip for.

// anchors are the calendar features a chip plan can be pinned to.
type anchors struct {
	bigDouble, secondDouble int // gameweeks, 0 when none exists
	bigBlank                int
}

// findAnchors reads the season's calendar for the weeks a chip plan cares about.
//
// "Biggest" is the number of clubs involved rather than the number of fixtures,
// because what a bench boost or a free hit is worth scales with how much of a
// fifteen is affected, and a squad is drawn from clubs.
//
// Only weeks strictly after `start` are eligible: a chip cannot be played in a
// gameweek the cell has already entered past, and one played *in* the entry week
// would be decided with no information the opening squad did not also have.
//
// It is no longer the placement rule — `sightedWeeks` is, at a parameterised
// sight — and it survives as the *independent* statement of what the calendar's
// maxima are, which `TestAnchoredPlanSitsOnTheCalendarMaxima` checks the placement
// against. Two expressions of one quantity is normally this package's most
// expensive bug; here it is the point, because one of them is a test.
func findAnchors(cur *Season, start int) anchors {
	played, count, teams := teamGameweeks(cur.Fixtures)
	type wk struct{ gw, doubling, blanking int }
	var weeks []wk
	for gw := range played {
		if gw <= start {
			continue
		}
		w := wk{gw: gw}
		for team := range teams {
			switch n := count[gw][team]; {
			case n == 0:
				w.blanking++
			case n >= 2:
				w.doubling++
			}
		}
		weeks = append(weeks, w)
	}
	// Sorted by gameweek first so ties resolve to the earlier week
	// deterministically — cells run in one process and a map walk would make the
	// plan depend on iteration order, which has already made Optimize
	// non-deterministic once in this package.
	sort.Slice(weeks, func(i, j int) bool { return weeks[i].gw < weeks[j].gw })

	// Maxima by club count, keeping the earliest week on a tie.
	best, bestN := 0, 0
	second, secondN := 0, 0
	blank, blankN := 0, 0
	for _, w := range weeks {
		if w.doubling > bestN {
			second, secondN = best, bestN
			best, bestN = w.gw, w.doubling
		} else if w.doubling > secondN {
			second, secondN = w.gw, w.doubling
		}
		if w.blanking > blankN {
			blank, blankN = w.gw, w.blanking
		}
	}
	return anchors{bigDouble: best, secondDouble: second, bigBlank: blank}
}

// sightedWeeks is the placement rule, at a manager who can see `lag` gameweeks
// ahead and no further.
//
// # One rule at two sights, not two rules
//
// The constraint a short lag imposes is **not** that you cannot find a double. It
// is that you cannot tell a double from *the biggest double of the season*. At two
// weeks' sight you learn about GW29 at GW27 and commit, never knowing GW34 carries
// a bigger one; at full sight you wait for the best. So the rule is: play the first
// eligible week that nothing better is visible ahead of.
//
// ⚠️ **That premise is an assumption and it was never measured, and the manager who
// plays this game reports it is false** (2026-08-13): in 3 years of 3 the season's
// biggest double was known well in advance, because the cup and European calendar
// makes it predictable long before the fixture is formally moved.
//
// The arms are still worth running and no measured figure changes — but read the
// ladder the other way round. `fullSight` is not the arm that cheats, it is the arm
// a real manager plays, so its −5.4 is the estimate of what calendar anchoring is
// worth. The lag arms bound **pessimism**: they price a manager with less
// information than anyone actually has.
//
// At `fullSight` that reduces to "the biggest, earliest on a tie", which is the
// anchored arm — and `anchoredPlan` is literally this function at that lag rather
// than a second implementation of the same idea. The previous file had two, and
// they disagreed in five cells of twenty-four while an assertion comparing one
// chip at one entry point passed.
//
// No wildcard is placed, by any arm, at any sight. See the file header: holding
// it at a common week made the full-sight arm the only one boosting a
// wildcard-rebuilt squad, in every cell.
func sightedWeeks(cur *Season, start, lag int) analysis.ChipPlan {
	played, count, teams := teamGameweeks(cur.Fixtures)
	clubs := func(gw int, want func(int) bool) int {
		n := 0
		for team := range teams {
			if want(count[gw][team]) {
				n++
			}
		}
		return n
	}
	isDouble := func(n int) bool { return n >= 2 }
	isBlank := func(n int) bool { return n == 0 }

	// firstUnbeaten walks forward and returns the first gameweek whose club count
	// is not bettered by anything *still available* inside the visible window.
	//
	// `taken` is excluded from the lookahead as well as from the candidates, and
	// that is the repair: a week already spent on another chip is not competition
	// for this one, and counting it as competition let the bench boost's week veto
	// every earlier week for the triple captain, which then went unplayed.
	//
	// Strictly-better ("> here") resolves ties to the earliest week, which is what
	// makes full sight reproduce the calendar's maxima.
	//
	// `minAnchorClubs` is what stops a lag arm spending a chip on a two-club week
	// in September; without it the sweep measures the missing bar rather than the
	// lag. Full sight is unaffected — its answer is the season maximum, always
	// well above the bar — so the bar is a property of the lag columns alone.
	firstUnbeaten := func(want func(int) bool, taken map[int]bool, lag int) int {
		for gw := start + 1; gw <= 38; gw++ {
			if !played[gw] || taken[gw] {
				continue
			}
			here := clubs(gw, want)
			if here < minAnchorClubs {
				continue
			}
			better := false
			for ahead := gw + 1; ahead <= gw+lag && ahead <= 38; ahead++ {
				if played[ahead] && !taken[ahead] && clubs(ahead, want) > here {
					better = true
					break
				}
			}
			if !better {
				return gw
			}
		}
		return 0
	}

	var p analysis.ChipPlan
	taken := map[int]bool{}
	place := func(gw int, set *int) {
		if gw <= start || taken[gw] {
			return
		}
		taken[gw] = true
		*set = gw
	}
	// Bench boost on the biggest double, because it pays all fifteen and a
	// doubling club plays twice. Free hit on the biggest blank, which is the week
	// the permanent squad is worth least. Triple captain on the best double that
	// is left, which at full sight is the second-biggest.
	place(firstUnbeaten(isDouble, taken, lag), &p.BenchBoost)
	place(firstUnbeaten(isBlank, taken, lag), &p.FreeHit)
	place(firstUnbeaten(isDouble, taken, lag), &p.TripleCaptain)
	return p
}

// controlWeeks plays the three tested chips at fixed offsets from entry, which
// is the placement rule every earlier chip measurement here used. No wildcard —
// see the file header.
//
// # Overrunning the season places backwards, and never drops
//
// `start+13` is GW39 at a GW26 entry. Dropping the chip there is what made three
// cells compare four chips against three, so an offset past GW38 walks back from
// the end of the season instead. That keeps the control arbitrary — which is its
// job — without letting it become a different *set* of chips from the arm it is
// being compared with.
//
// Returning zero is still possible if the season is too short to hold everything,
// and `matchedChips` then drops that chip from every arm rather than from this one.
func controlWeeks(cur *Season, start int) analysis.ChipPlan {
	var p analysis.ChipPlan
	taken := map[int]bool{}
	place := func(off int, set *int) {
		gw := start + off
		for gw <= 38 && taken[gw] {
			gw++
		}
		if gw > 38 {
			gw = 38
			for gw > start && taken[gw] {
				gw--
			}
		}
		if gw > 38 || gw <= start {
			return
		}
		taken[gw] = true
		*set = gw
	}
	place(controlOffsets.benchBoost, &p.BenchBoost)
	place(controlOffsets.freeHit, &p.FreeHit)
	place(controlOffsets.tripleCaptain, &p.TripleCaptain)
	return p
}

// chipSlots is a plan viewed as four independent pointers, in a fixed order, so
// the set operations below do not have to name each chip four times.
func chipSlots(p *analysis.ChipPlan) [4]*int {
	return [4]*int{&p.Wildcard, &p.BenchBoost, &p.FreeHit, &p.TripleCaptain}
}

// chipNames labels chipSlots, in the same order.
var chipNames = [4]string{"wildcard", "bench boost", "free hit", "triple captain"}

// chipCount is how many chips a plan actually plays.
func chipCount(p analysis.ChipPlan) int {
	n := 0
	for _, s := range chipSlots(&p) {
		if *s > 0 {
			n++
		}
	}
	return n
}

// matchedChips is the set of chips *every* arm in this sweep can place for this
// cell — the intersection, computed once and applied to all of them.
//
// The previous version derived the control's set from the anchored plan, which
// made the guarantee one-directional: it stopped the anchored arm dropping a chip
// the control played, and not the reverse. Both failures then happened. Reading
// the intersection over the whole grid is the only version that cannot be
// one-directional, and it costs nothing because every arm is a pure function of
// the season and the entry gameweek.
func matchedChips(cur *Season, start int) [4]bool {
	var in [4]bool
	for i := range in {
		in[i] = true
	}
	intersect := func(p analysis.ChipPlan) {
		s := chipSlots(&p)
		for i := range in {
			if *s[i] == 0 {
				in[i] = false
			}
		}
	}
	intersect(controlWeeks(cur, start))
	intersect(sightedWeeks(cur, start, fullSight))
	for _, lag := range sightLags {
		intersect(sightedWeeks(cur, start, lag))
	}
	return in
}

// mask drops any chip the matched set excludes.
func mask(p analysis.ChipPlan, in [4]bool) analysis.ChipPlan {
	s := chipSlots(&p)
	for i := range in {
		if !in[i] {
			*s[i] = 0
		}
	}
	return p
}

// anchoredPlan pins the chips to the calendar, the way they are actually played.
// It is `sightedWeeks` at full sight and nothing else, so "the lagged rule at full
// sight equals the anchored rule" is true by construction rather than by
// assertion — which is the point, because asserting it is what failed before.
func anchoredPlan(cur *Season, start int) analysis.ChipPlan {
	return mask(sightedWeeks(cur, start, fullSight), matchedChips(cur, start))
}

// laggedPlan is anchoring under a reveal lag. See sightedWeeks.
func laggedPlan(lag int) func(cur *Season, start int) analysis.ChipPlan {
	return func(cur *Season, start int) analysis.ChipPlan {
		return mask(sightedWeeks(cur, start, lag), matchedChips(cur, start))
	}
}

// controlPlan plays the same chips at fixed offsets from entry.
func controlPlan(cur *Season, start int) analysis.ChipPlan {
	return mask(controlWeeks(cur, start), matchedChips(cur, start))
}

// chipArm is one arm of the sweep: a planner and the label it is reported under.
type chipArm struct {
	name  string // short, for assertion messages
	label string // the sweep's own label
	plan  func(cur *Season, start int) analysis.ChipPlan
}

// armPlanners is every planner the sweep runs, in the order the arms appear, so
// the assertions below cannot check a set of arms the sweep does not use — and
// so `matchedChips` and the sweep cannot disagree about which arms exist.
func armPlanners() []chipArm {
	out := []chipArm{
		{"control", "chips at fixed offsets from entry", controlPlan},
		{"anchored", "chips anchored on the calendar (full sight)", anchoredPlan},
	}
	for _, lag := range sightLags {
		out = append(out, chipArm{
			name:  fmt.Sprintf("sight %d", lag),
			label: fmt.Sprintf("anchored, %d gameweeks of sight", lag),
			plan:  laggedPlan(lag),
		})
	}
	return out
}

// loadPairsOrSkip is loadPairs for a test that must not fail on a machine with
// no archive. The DIAG diagnostics may fatal — they are run deliberately — but
// the invariant checks below run in the ordinary `go test ./...` and this
// project's standing rule is that such a test skips when its data is
// unreachable.
func loadPairsOrSkip(t *testing.T, cfg config.Config) []seasonPair {
	t.Helper()
	for _, p := range sweepPairNames() {
		for _, name := range p {
			if _, err := Load(context.Background(), cfg.CacheDir, name); err != nil {
				t.Skipf("archive unavailable (%s): %v", name, err)
			}
		}
	}
	return loadPairs(t, cfg)
}

// TestAnchoredArmsPlayTheSameChips is the matched-set property the whole design
// rests on, asserted where it can be checked in a second rather than inferred
// from a points table after eight minutes of replay.
//
// It is deliberately *every arm at every entry point*, not one chip at one entry:
// the previous version compared `full.BenchBoost` at `start = 1` and passed
// against a rule that dropped the triple captain in five cells and moved it in
// four more.
func TestAnchoredArmsPlayTheSameChips(t *testing.T) {
	cfg := loadConfig(t)
	for _, pair := range loadPairsOrSkip(t, cfg) {
		for _, start := range sweepStarts() {
			arms := armPlanners()
			want := arms[0].plan(pair.Cur, start)
			wantSlots := chipSlots(&want)
			for _, arm := range arms[1:] {
				got := arm.plan(pair.Cur, start)
				gotSlots := chipSlots(&got)
				for i := range chipNames {
					if (*wantSlots[i] > 0) != (*gotSlots[i] > 0) {
						t.Errorf("%s@%d: %s plays the %s and %s does not — the arms are "+
							"comparing different chip sets, which is a chip against no "+
							"chip rather than placement against placement",
							pair.Name, start, arms[0].label, chipNames[i], arm.label)
					}
				}
				if a, b := chipCount(want), chipCount(got); a != b {
					t.Errorf("%s@%d: %s plays %d chips and %s plays %d",
						pair.Name, start, arms[0].label, a, arm.label, b)
				}
			}
		}
	}
}

// TestLaggedPlanAtFullSightIsTheAnchoredPlan is a tripwire, and it is honest
// about being one: `anchoredPlan` and `laggedPlan(fullSight)` are the same
// function at the same argument, so this cannot fail without a refactor
// separating them.
//
// It is kept because that refactor is exactly what happened last time — two
// implementations of "anchor on the calendar" drifted apart while a one-chip,
// one-entry assertion reported them equal. All four chips at all six entry
// points, so the next divergence is caught at the point it is introduced.
func TestLaggedPlanAtFullSightIsTheAnchoredPlan(t *testing.T) {
	cfg := loadConfig(t)
	full := laggedPlan(fullSight)
	for _, pair := range loadPairsOrSkip(t, cfg) {
		for _, start := range sweepStarts() {
			a, b := anchoredPlan(pair.Cur, start), full(pair.Cur, start)
			if a != b {
				t.Errorf("%s@%d: anchored %v, lagged at full sight %v", pair.Name, start, a, b)
			}
		}
	}
}

// TestAnchoredPlanSitsOnTheCalendarMaxima is the check the tripwire above cannot
// be: it compares the placement rule against `findAnchors`, which computes the
// calendar's maxima by a different route.
//
// The bench boost must sit on the biggest double of the remaining season. The
// free hit must sit on the biggest blank *unless* an earlier chip has already
// claimed that week — which is the bench boost, since it is placed first, and a
// week can carry both the season's biggest double and its biggest blank once a
// cup round and a rearranged fixture land together.
//
// **The exemption list must name every chip placed before the one being checked**,
// and the first version of this named only the wildcard. It never fired on this
// archive, so the test passed while being one entry short of the code it watches.
// `findAnchors` ignores `minAnchorClubs` deliberately: the bar is a property of
// the lag arms and full sight must land on the true maximum regardless of it.
func TestAnchoredPlanSitsOnTheCalendarMaxima(t *testing.T) {
	cfg := loadConfig(t)
	for _, pair := range loadPairsOrSkip(t, cfg) {
		for _, start := range sweepStarts() {
			a := findAnchors(pair.Cur, start)
			p := sightedWeeks(pair.Cur, start, fullSight)
			if p.Wildcard != 0 {
				t.Errorf("%s@%d: a wildcard was placed at GW%d. No arm plays one — "+
					"holding it common made full sight the only arm boosting a "+
					"wildcard-rebuilt squad, in every cell", pair.Name, start, p.Wildcard)
			}
			if a.bigDouble > 0 && p.BenchBoost != a.bigDouble {
				t.Errorf("%s@%d: the biggest double is GW%d and the bench boost is on GW%d",
					pair.Name, start, a.bigDouble, p.BenchBoost)
			}
			if a.bigBlank > 0 && a.bigBlank != p.BenchBoost && p.FreeHit != a.bigBlank {
				t.Errorf("%s@%d: the biggest blank is GW%d and the free hit is on GW%d",
					pair.Name, start, a.bigBlank, p.FreeHit)
			}
		}
	}
}

func TestDiagAnchoredChips(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== does anchoring the chips on the calendar beat placing them by week?\n")
	fmt.Printf("Every arm plays the SAME chips: bench boost, free hit, triple captain.\n")
	fmt.Printf("NO ARM PLAYS A WILDCARD. Holding it at a common week made the\n")
	fmt.Printf("full-sight arm the only one boosting a wildcard-rebuilt squad — 30 of\n")
	fmt.Printf("30 cells against 3-5 for the lag arms — which is a bigger confound\n")
	fmt.Printf("than the GW4 one it was removing. Every arm now boosts a squad built\n")
	fmt.Printf("for something else, so that floor cancels.\n")
	fmt.Printf("What varies is placement: the control uses fixed offsets from entry,\n")
	fmt.Printf("which is what every earlier chip measurement here used, and the\n")
	fmt.Printf("anchored arms read the fixture calendar.\n")
	fmt.Printf("**The full-sight arm is a target, not a policy**: FPL announces\n")
	fmt.Printf("doubles as cup rounds resolve. The lag arms are what a manager runs,\n")
	fmt.Printf("and they will not spend a chip on a week involving fewer than %d clubs.\n",
		minAnchorClubs)
	fmt.Printf("**Read points per season-path, not per gameweek.** Three chips are an\n")
	fmt.Printf("event count, and dividing a one-off by weeks played inflates the late\n")
	fmt.Printf("entries — see the harness-and-inference note.\n\n")

	// Print the plans once, so the arms are auditable rather than asserted.
	cfg := loadConfig(t)
	for _, pair := range loadPairs(t, cfg) {
		for _, start := range starts {
			a, c := anchoredPlan(pair.Cur, start), controlPlan(pair.Cur, start)
			fmt.Printf("  %-9s @%-3d anchored WC%2d BB%2d FH%2d TC%2d (%d chips) | control WC%2d BB%2d FH%2d TC%2d (%d)\n",
				pair.Name, start,
				a.Wildcard, a.BenchBoost, a.FreeHit, a.TripleCaptain, chipCount(a),
				c.Wildcard, c.BenchBoost, c.FreeHit, c.TripleCaptain, chipCount(c))
		}
	}

	var arms []policyVariant
	for _, arm := range armPlanners() {
		plan := arm.plan
		arms = append(arms, policyVariant{
			label: arm.label,
			apply: func(sc *SimConfig) { sc.ChipPlanner = plan },
		})
	}

	runPolicySweep(t, arms, starts)

	fmt.Printf("\nRead the lag columns, not the ceiling. Full sight is hindsight; two\n")
	fmt.Printf("to six gameweeks is what FPL actually gives you. If the value survives\n")
	fmt.Printf("at four it is strategy; if it only exists at full sight it is not.\n")
}
