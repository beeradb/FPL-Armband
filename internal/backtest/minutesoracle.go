package backtest

// The minutes oracle: OracleMinutes tells the model exactly how much football
// each player is about to play, over the window the decision is judged on.
//
// It is the generalisation of which OracleAvailability is the degenerate case.
// Availability sees only the players who record no minutes *all season* — a
// summer departure, a season-ending pre-season injury — and is blind to the far
// larger population who play until October and then stop. This sees everything:
// rotation, cameos, a lost place, and the injuries that *resolve*, which the
// record correctly says no end-of-season snapshot can show and which the
// per-gameweek rows carry all along.
//
// It therefore bounds the whole rotation-risk family at once — MinutesHalfLife,
// BlendMinutesK, blankRunFactor, the squad pool's minimum-expected-minutes cliff
// — the same way the transfer-gate oracle bounds the gate-constant family.
//
// # The first version answered a season-average question, and that was the defect
//
// The original summed each player's minutes and his club's fixtures across the
// **entire remainder of the season** and divided, producing one scalar per player
// that never changed from one gameweek to the next. It could not express "starts
// GW20, subbed GW21, missed GW22-27, back GW28": a player who breaks his leg in
// October and returns in February arrived as a mildly-reduced ever-present, every
// week, including the weeks he was in hospital.
//
// That is the right quantity for a *squad* decision, which cares about season
// averages, and structurally blunt for a *weekly transfer* decision, which needs
// the trajectory — which is why its two metrics disagreed, and why the recorded
// HOLD/POLICY split was partly an artifact of the instrument rather than a fact
// about the model. See the scoring-model note.
//
// Two things follow, and both are implemented here.
//
// **The window is bounded.** At a cutoff of `through` the oracle reports the next
// `SimConfig.oracleWindow()` gameweeks and nothing beyond them, and the index is
// rebuilt at every gameweek (`simulate.go` calls `cfg.recentIndex(cur, gw-1)` for
// each of the three engines), so the value moves as the season does. The window
// is the shipped fixture/decision horizon, which is the span `Score` is an
// expectation over and the span the transfer gate multiplies a gain by. It is an
// **asserted** choice, not a measured one: it makes the oracle answer the
// question the decision asks rather than a season-average question, and no sweep
// selected it.
//
// **The denominator is the club's fixture list, not the player's rows.** The
// archive omits a player's gameweek row entirely when he is not in the squad —
// about 3,000 of 30,000 club-gameweeks a season, and disproportionately the
// injured and departed, who are exactly this oracle's population. Counting only
// the gameweeks he has a row for made a player who vanishes for six weeks read as
// an ever-present over those six weeks, which is the same blindness in a second
// place. A club fixture with no row for him is an absence and is counted as one.
//
// # The seam, which is the recency index and not the bootstrap
//
// The oracle-design document says a minutes oracle "rewrites the minutes and starts on
// the bootstrap that expectedMinutes divides". Both halves of that turn out not
// to hold against the shipped code, and each on its own is disqualifying:
//
//   - **In-season the bootstrap's minutes are not read at all.** `blendRates`
//     computes minsPerMatch from the element, and then, whenever `Engine.Recent`
//     is present and the player has any matches, *replaces* it outright with
//     `RecentPlayer.MinutesPerMatch` (internal/analysis/blend.go, the `e.Recent !=
//     nil && played > 0` branch). The replay supplies Recent from the archive in
//     every gameweek it plays, so a bootstrap-only minutes oracle would be inert
//     in exactly the regime it exists to measure.
//   - **Minutes on the bootstrap are the denominator of every counting rate.**
//     `Bonus90`, `Saves90`, `Yellow90`, `Red90` and defensive contribution are all
//     `count / el.Minutes`, and the rate blend's evidence weight is
//     `el.Minutes / 90`. Rewriting minutes there cannot leave the per-90 rates
//     alone — it would divide all of them by the same factor — so the one thing
//     the design most insists on ("never touch a per-90 rate, or it becomes a
//     points oracle") is unachievable at that seam.
//
// So the oracle perturbs the **recency index** instead. That is still entirely
// inside internal/backtest — `newRecentIndexWith` manufactures it from the
// archive, exactly as `PointInTimeWith` manufactures the bootstrap — so the hard
// boundary the design actually cares about holds: nothing in internal/analysis
// learns that oracles exist, and `Engine.expectedMinutes` is untouched.
//
// And the confinement is *better* at this seam, not worse. `analysis.RecentPlayer`
// carries `MinutesPerMatch`, `StartShare`, `Matches` and `BlankRun` as fields in
// their own right, separate from `XG90`, `XA90`, `XGC90`, `DefCon90`, `Bonus90`,
// `Saves90` and `Minutes90`. Rewriting the first four provably cannot move the
// last seven, where at the bootstrap it provably must.
//
// The three-channel finding still applies and is checked rather than assumed:
// the bootstrap comes back byte-identical (`tierOneCases`), and `Engine.Priors`
// overwrites the whole blend pre-season, which is why the pre-season blindness
// below is a property of the harness rather than of this file.
//
// # What it is blind to, stated rather than discovered
//
// **Pre-season.** `newRecentIndexWith` returns nil before a gameweek is played and
// `blendRates` ignores Recent at `played == 0` regardless, so an opening fifteen
// bought at GW1 gets no minutes hindsight at all. Five of the grid's six entry
// points build their squad in-season and do get it; the GW1 cells measure the
// oracle's effect on transfers alone. That is a real understatement of the bound
// and it is why the per-start columns are worth reading here.
//
// **The blend still shrinks it toward the prior.** With BlendMinutesK at 5, a GW6
// entry weights the corrected current-season figure at 5/(5+5). That is not a
// defect: an *information* oracle corrects an input and leaves the policy free to
// decide on it, which is the design's own definition and the whole line between an
// information oracle and a decision oracle. The model is given perfect data and
// still does not fully believe it, exactly as it does not fully believe real data.
//
// **One index serves three engines.** The transfer engine runs at the configured
// horizon and the weekly-eleven engine at horizon 1, and both read the same
// `Engine.Recent`. So the eleven is picked on a window-averaged truth rather than
// on this Saturday's, which understates the oracle for the one decision that is
// re-made weekly at no cost. Narrowing the window per consumer is the seam trick
// the fixture-load finding used; it is not done here, and it is the obvious next
// instrument if this bound ever needs tightening.

import "armband/internal/analysis"

// defaultOracleWindow is the lookahead an oracle over the future uses when the
// config carries no horizon — which happens only in unit tests that construct a
// bare SimConfig.
//
// **Asserted, not measured.** It matches the shipped `fixture_horizon` of 5, and
// the point of matching it is that the oracle should answer the question the
// decision asks: `Score` is an expectation over the horizon and the transfer gate
// tests `gain x horizon`. No sweep chose 5 here and none could — a window is a
// property of the instrument, not of the model, so widening it would make the
// oracle bound a different capability rather than measure the same one better.
const defaultOracleWindow = 5

// decisionHorizon is how many gameweeks a transfer decision is judged over.
//
// Factored out of `decide` rather than restated here. `Horizon` used to do two
// jobs — the fixture-average window and the transfer threshold — and
// `SimConfig.DecisionHorizon` exists precisely to separate them, so an oracle
// that read `Weights.Horizon` directly would silently re-merge the two the moment
// anybody swept the threshold with an oracled arm in the grid.
func (c SimConfig) decisionHorizon() int {
	if c.DecisionHorizon > 0 {
		return c.DecisionHorizon
	}
	return c.Weights.Horizon
}

// anticipate tells one engine that a chip is planned, so the squad it is scoring
// only has to serve until the chip replaces it.
//
// It calls `ApplyChipPlan` rather than restating what a planned chip implies,
// even though the transfer path has nowhere to put the bench weight that comes
// back: re-deriving the shortened horizon here would be a second expression of
// one quantity, and the live agent's copy is the one that would drift.
//
// No-op unless AnticipateChips is set, so the baseline arm is byte-identical and
// every figure recorded before this switch existed stays comparable.
// gw is the gameweek being decided, and it is a parameter rather than something
// this can do without: `analysis.ChipPlan` holds ONE week per chip, so with two
// sets there is no single plan to hand over. What the decision needs is the next
// chip ahead of it — a March decision handed September's wildcard sees a rebuild
// behind it and prepares as though none were coming, and a second free hit is
// invisible to `SetSkipGameweeks`, which takes one week.
//
// That failure is silent in both directions: the arm runs, the season looks
// ordinary, and a second-set-only plan makes AnticipateChips a total no-op while
// still stamping the variant. It is the byte-identical null this record has been
// caught by four times, and it was caught here by review rather than by a test.
func (c SimConfig) anticipate(e *analysis.Engine, gw int) {
	if !c.AnticipateChips {
		return
	}
	// Both sets, filtered to what is still ahead. This was four nextChip calls
	// collapsed into a single ChipPlan, which is what the comment above describes
	// as a silent failure in both directions — `analysis.ChipSchedule` is the
	// type that removes the need for the collapse, so a second free hit now
	// reaches SetSkipGameweeks instead of vanishing.
	e.Chips = c.schedule().From(gw)
	var req analysis.OptimizeRequest
	e.ApplyChipPlan(&req)
	e.ApplyFreeHitToScoring()
}

// oracleWindow is how many gameweeks ahead an oracle over the future tells the
// truth about.
//
// `OracleWindow` overrides it, and exists for one measurement rather than as a
// setting: the first minutes oracle averaged the whole remainder of the season
// *and* divided by the player's own rows instead of his club's fixtures, and
// those two defects shipped in one commit. An arm at window 38 with the corrected
// denominator is what separates them.
func (c SimConfig) oracleWindow() int {
	if c.OracleWindow >= 1 {
		return c.OracleWindow
	}
	if h := c.decisionHorizon(); h >= 1 {
		return h
	}
	return defaultOracleWindow
}

// oracleFutureIndex is a recency index with the minutes half told the truth about
// the window ahead.
//
// It wraps rather than replaces, so every rate field comes from the honest index
// underneath and cannot be perturbed even by accident. `base` may be nil, which
// is what happens pre-season and for a player who has not appeared: the wrapper
// then supplies minutes for a player the honest index has no entry for at all,
// which is the January-signing and returning-injury case.
type oracleFutureIndex struct {
	base   analysis.RecentForm
	future map[int]analysis.RecentPlayer
}

func (o oracleFutureIndex) Get(code int) (analysis.RecentPlayer, bool) {
	known := analysis.RecentPlayer{}
	if o.base != nil {
		if r, ok := o.base.Get(code); ok {
			known = r
		}
	}
	f, ok := o.future[code]
	if !ok {
		return known, o.base != nil && known.Matches > 0
	}
	// Only the four minutes fields are taken from hindsight. Everything else is
	// whatever the honest index computed, which for a player it had no entry for
	// is zero — and zero rates mean blendRates leaves his output on the bootstrap,
	// which is the correct un-oracled behaviour for a quantity this oracle has no
	// business correcting.
	known.MinutesPerMatch = f.MinutesPerMatch
	known.StartShare = f.StartShare
	known.Matches = f.Matches
	known.BlankRun = f.BlankRun
	return known, true
}

// fixtureCalendar is how many matches each club plays in each gameweek.
//
// The denominator for everything below, and the reason it is the club's calendar
// rather than the player's rows: a player with no row for a gameweek his club
// played did not play, and counting only his own rows reads him as an
// ever-present over exactly the weeks he was absent.
//
// **One denominator, not two.** A row that exists carries its own `Fixtures`
// count, and preferring it where present would be marginally more accurate for a
// player who moved clubs mid-season — `Player.Team` is his end-of-season club, so
// the calendar is his *new* club's for weeks before the move. It is not done,
// because two expressions of one quantity is this package's signature bug.
//
// The cost is small and is **not** fully described by the obvious check. Where a
// row exists the two disagree on about 22 of 110,268 archive rows across the four
// seasons, 0.02%. But that check can only see gameweeks a player has a row for,
// and the residual risk is the opposite case: for a mover, a window entirely
// before his transfer books an absence in a week his *new* club played and his old
// one blanked, and drops the minutes he did play in the reverse case. Confined to
// PL-to-PL movers in pre-transfer windows where the two clubs' fixture counts
// differ, so smaller again — but it is not the 22 rows that bound it.
func fixtureCalendar(s *Season) map[int]map[int]int {
	cal := map[int]map[int]int{}
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		for _, team := range [2]int{f.TeamH, f.TeamA} {
			if cal[team] == nil {
				cal[team] = map[int]int{}
			}
			cal[team][*f.Event]++
		}
	}
	return cal
}

// futureSelection is what a player is about to do over one window, counted at
// club-fixture resolution.
//
// `starts` and `subs` are the *selection* fact — which fixtures he is picked for
// and in what role — and `minutes` is the quantity given that selection. The two
// oracles are exactly this split: OracleLineups reads starts and subs and prices
// them at conditional averages, OracleMinutes reads the realised minutes.
type futureSelection struct {
	minutes  float64
	starts   float64
	subs     float64
	fixtures float64
}

// selectionOver walks the window and counts what happened, treating a club
// fixture with no row for the player as an absence.
//
// # The substitute count in a double gameweek, and why the rule is an inequality
//
// The archive records `starts` and `minutes` per *gameweek*, not per match, so in
// a double there is no direct way to say which fixtures he appeared in. The first
// version credited a substitute appearance whenever any non-started fixture
// existed and he recorded minutes at all — which is wrong for the **commonest**
// double-gameweek shape: a player who starts one leg and is left out of the other
// records `Starts: 1, Minutes: 90`, and was credited with a start *and* a
// substitute appearance, pricing him at about 50 minutes a match where the truth
// is 45. Counted over the four archive seasons that shape occurs **418** times
// against **319** for the shape the comment did document, so the undocumented
// direction was the larger one — and it lands entirely on the lineups arm, which
// is to say on the residual the decomposition exists to produce.
//
// `minutes > 90 x starts` fixes it exactly in that direction, because minutes
// beyond ninety per started fixture can only have come from a fixture he did not
// start. What remains is the smaller, opposite error: a player withdrawn early
// from his start and then used off the bench in the other leg can total under
// ninety, and reads as no substitute appearance. That one is genuinely
// unrecoverable from a per-gameweek row, and it is the conservative direction —
// the lineups arm under-credits rather than inventing football.
func selectionOver(p *Player, cal map[int]int, from, to int) futureSelection {
	var sel futureSelection
	for gw := from; gw <= to && gw <= 38; gw++ {
		fx := float64(cal[gw])
		if fx < 1 {
			continue // his club has no fixture: not an absence, just no football
		}
		sel.fixtures += fx
		g, ok := p.GWs[gw]
		if !ok {
			continue // no row for a gameweek his club played: he did not feature
		}
		mins := float64(g.Minutes)
		sel.minutes += mins
		starts := float64(g.Starts)
		if starts > fx {
			starts = fx
		}
		sel.starts += starts
		if starts < fx && mins > 90*starts {
			sel.subs++
		}
	}
	return sel
}

// reconstructedInWindows is how many of the club-gameweeks an oracle at this
// window would classify rest on a `Starts` value inferred from minutes rather
// than recorded.
//
// It exists because the claim it supports is about *exposure* and the obvious
// proxy is not. Counting reconstructed rows across a whole season answers a
// different question — 2022-23's reconstruction stops at GW15, so a season-wide
// share is diluted by twenty-three gameweeks the lineups arm never classified
// from inferred data — and a season constant cannot vary with the window at all,
// which is the tell that it is measuring something else.
//
// See reconstructStarts, whose own recorded boundary is "never as evidence about
// an individual rotation or returning player". That is precisely what a lineups
// oracle consumes, so the exposure is reported rather than corrected.
func reconstructedInWindows(s *Season, window int) (classified, reconstructed int) {
	cal := fixtureCalendar(s)
	for _, p := range s.Players {
		if p.Code == 0 {
			continue
		}
		for through := 0; through < 38; through++ {
			for gw := through + 1; gw <= through+window && gw <= 38; gw++ {
				if cal[p.Team][gw] < 1 {
					continue
				}
				g, ok := p.GWs[gw]
				if !ok {
					continue
				}
				classified++
				if g.StartsReconstructed {
					reconstructed++
				}
			}
		}
	}
	return classified, reconstructed
}

// newOracleMinutes builds the hindsight minutes view for a decision taken with
// data through gameweek `through`.
//
// "Perfect minutes" is what he goes on to play **per match his club plays**, over
// the next `window` gameweeks. Per match rather than per gameweek for the reason
// the honest index divides by the fixture count: a double gameweek correctly
// records 180 minutes, and reporting that as a per-gameweek figure would predict
// 180 for the single gameweeks that follow. `MinutesPerMatch` has to stay a
// statement about how much of a *match* he plays, because that is what
// MinutesRating, the sixty-minute threshold and the blend all assume.
//
// BlankRun is zero throughout: it exists to catch the onset of an absence that
// the exponential average under-reacts to, and an oracle that knows the future
// minutes has nothing left for it to correct. Leaving the honest BlankRun in
// place would apply a 0.75 penalty on top of a figure that is already right.
func newOracleMinutes(s *Season, through, window int, base analysis.RecentForm) analysis.RecentForm {
	cal := fixtureCalendar(s)
	future := map[int]analysis.RecentPlayer{}
	for _, p := range s.Players {
		if p.Code == 0 {
			continue
		}
		sel := selectionOver(p, cal[p.Team], through+1, through+window)
		if sel.fixtures == 0 {
			// No football at all in the window — the season has run out, or his
			// club blanks every week of it. There is nothing for an oracle to be
			// right about, so it says nothing and the honest index stands.
			continue
		}
		// Matches carries a different quantity under the oracle than in the honest
		// index — club fixtures in the forward window, against gameweeks the player
		// has a row for up to the cutoff — and that is safe only because its sole
		// consumer is the boolean `r.Matches > 0` in blendRates. Anything that ever
		// reads it as a sample size gets a different number under hindsight, and
		// TestMinutesOraclePerturbsOnlyMinutes cannot catch that, because Matches is
		// a field the oracle is *allowed* to move.
		future[p.Code] = analysis.RecentPlayer{
			MinutesPerMatch: sel.minutes / sel.fixtures,
			StartShare:      sel.starts / sel.fixtures,
			Matches:         int(sel.fixtures),
		}
	}
	return oracleFutureIndex{base: base, future: future}
}

// recentIndex is the recency-weighted view of the season a cell's engines read,
// with any hindsight this cell was granted already applied.
//
// One constructor rather than the four near-identical `newRecentIndexWith` calls
// it replaced. Those calls were the shape this package's most-repeated bug takes:
// four copies of one expression, and an oracle wired into three of them would be
// wired into three of them silently. `TestEveryScoringEngineGetsRecency` counts
// them for exactly that reason.
func (c SimConfig) recentIndex(s *Season, through int) analysis.RecentForm {
	base := newRecentIndexWith(s, through, c.minutesHalfLife(), c.Weights.RateHalfLife)
	// A restriction only one branch honours is the shape of this package's
	// most-repeated bug: an arm labelled "minutes: flagged" that ran the
	// unrestricted minutes oracle would reproduce the unrestricted arm exactly,
	// and the record would read that as the same structural inertness the flagged
	// LINEUPS arm genuinely has. `FeatureScope` gets this refusal from `Validate`;
	// `lineupCovers` cannot, because it is unexported and off `Oracles`, so it has
	// to be refused here. Pinned by TestALineupRestrictionNeedsTheLineupsOracle.
	if c.lineupCovers != nil && !c.Oracles.Has(OracleLineups) {
		panic("backtest: lineupCovers is set without OracleLineups, so the " +
			"restriction would be silently dropped and the arm would report the " +
			"unrestricted oracle under a restricted label")
	}
	// Validate refuses both bits at once, so the order here is not a precedence
	// rule — there is no arm in which both are set.
	switch {
	case c.Oracles.Has(OracleMinutes):
		return newOracleMinutes(s, through, c.oracleWindow(), base)
	case c.Oracles.Has(OracleLineups):
		return newOracleLineupsCovering(s, through, c.oracleWindow(), base, c.lineupCovers)
	}
	return base
}
