package analysis

import "sort"

// Rating opponents separately for attack and defence.
//
// FPL's FDR blends both into one number, so a side that scores freely and
// concedes freely reads as mid-table to everyone. Split into two ratings and
// banded 3/14/3 — worst three, an undifferentiated middle, best three — the
// underlying effects are large, and the defensive side is roughly twice the
// attacking one, which is exactly what a blended rating cannot express:
//
//	within-player, vs the middle 14 | target band  | avoid band
//	attackers vs opponent defence   | +25% / +13%  | -11% / -14%
//	defenders vs opponent attack    | +41% / +21%  | -23% / -27%
//
// The descriptive finding is real. Acting on it previously measured 2195 at
// full strength against 2208 for leaving FPL's blended FDR alone, and it is
// re-tested here against a corrected optimiser and a bench that is no longer
// priced at zero.
//
// # Detection carries no hindsight
//
// Bands come from goals scored and conceded in *finished* fixtures only, so at
// any gameweek the rating uses exactly what a manager could have known. Before
// bandMinMatches have been played there is no rating and no adjustment, which
// means the opening weeks run on FPL's FDR alone.
const bandMinMatches = 5

// bandSize is how many clubs sit in each tail.
const bandSize = 3

// teamBand is where a club sits on one of the two ratings.
type teamBand int

const (
	bandMiddle teamBand = iota
	bandWorst           // bottom three: the band to target
	bandBest            // top three: the band to avoid
)

// bands holds both ratings for every club.
type bands struct {
	attack  map[int]teamBand // how good this club is at scoring
	defence map[int]teamBand // how good this club is at not conceding
	ready   bool
}

// teamBands computes the 3/14/3 split from finished fixtures. Guarded by a
// sync.Once because the tool runner drives Metrics over the whole pool from
// several goroutines at once, and two of them racing to build the same map is a
// fatal "concurrent map writes" rather than a recoverable panic.
func (e *Engine) teamBands() bands {
	e.bandOnce.Do(func() {
		type rec struct{ for_, against, played float64 }
		by := map[int]*rec{}
		get := func(id int) *rec {
			if by[id] == nil {
				by[id] = &rec{}
			}
			return by[id]
		}
		for _, f := range e.Fixtures {
			if !f.Finished || f.TeamHScore == nil || f.TeamAScore == nil {
				continue
			}
			h, a := get(f.TeamH), get(f.TeamA)
			h.for_ += float64(*f.TeamHScore)
			h.against += float64(*f.TeamAScore)
			h.played++
			a.for_ += float64(*f.TeamAScore)
			a.against += float64(*f.TeamHScore)
			a.played++
		}

		type rated struct {
			id       int
			scored   float64
			conceded float64
		}
		// ⚠️ **Club id breaks every tie here, and that is the whole reason this
		// block is not three lines shorter.**
		//
		// This ranged the map straight into `rs` and ordered it with the
		// non-stable `sort.Slice`, so two clubs on the same goals-per-match — which
		// is common early, being a small integer over the same small number of
		// matches — were separated by Go's randomised map order. Only a tie *on a
		// band boundary* changes anything, but that is exactly the tie that decides
		// membership of the bottom or top three, and it changed from run to run.
		//
		// Measured before the fix, on two replays of one sweep at one commit with
		// `band_strength 1`: 3 of 36 `hold_points` cells differed, 12 of 36
		// `policy_points`, and 7 each on moves and hits — decisions, not only
		// scores. At cutoff 6 on 2024-25 the four band boundaries between them
		// admitted 144 distinct assignments from identical data. ⚠️ That count is
		// a product of `C(tie size, places inside the band)` over the four
		// boundaries and is NOT the product of the tie sizes, so do not try to
		// reproduce it from four numbers; the derivation is in
		// TestBandStrengthIsDeterministicAtTheShippedSetting's comment.
		//
		// ⚠️ **The map range was the ONLY source of run-to-run variation, and that
		// is sharper than "sort.Slice is unstable".** `sort.Slice` is deterministic
		// for a given input slice — pdqsort's pattern-breaking is seeded from the
		// slice length rather than randomised — so identical input always produced
		// identical output. What differed between runs was the input. Which is why
		// `sort.SliceStable` alone would NOT have fixed it: stability preserves the
		// input order, and the input order was the random one. The ordering has to
		// be a total order on a key that is itself stable, which is the club id.
		// This is the same class as `Optimize`'s map-ordered bench, pinned by
		// TestSeedOrderIsDeterministic, and `newTeamFormIndex`.
		ids := make([]int, 0, len(by))
		for id := range by {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		var rs []rated
		for _, id := range ids {
			r := by[id]
			if r.played < bandMinMatches {
				continue
			}
			rs = append(rs, rated{id, r.for_ / r.played, r.against / r.played})
		}
		if len(rs) < 2*bandSize+1 {
			e.bandCache = bands{}
			return
		}

		b := bands{attack: map[int]teamBand{}, defence: map[int]teamBand{}, ready: true}

		// Attack: fewest goals scored is the worst attack, and the band an
		// opposing defender wants to face.
		sort.Slice(rs, func(i, j int) bool {
			if rs[i].scored != rs[j].scored {
				return rs[i].scored < rs[j].scored
			}
			return rs[i].id < rs[j].id
		})
		for i := 0; i < bandSize; i++ {
			b.attack[rs[i].id] = bandWorst
			b.attack[rs[len(rs)-1-i].id] = bandBest
		}
		// Defence: most goals conceded is the worst defence, and the band an
		// opposing attacker wants to face.
		sort.Slice(rs, func(i, j int) bool {
			if rs[i].conceded != rs[j].conceded {
				return rs[i].conceded > rs[j].conceded
			}
			return rs[i].id < rs[j].id
		})
		for i := 0; i < bandSize; i++ {
			b.defence[rs[i].id] = bandWorst
			b.defence[rs[len(rs)-1-i].id] = bandBest
		}
		e.bandCache = b
	})
	return e.bandCache
}

// Band adjustments, applied on top of the FDR-derived multipliers.
//
// These are stated as effects on the quantity each one multiplies, not as
// effects on points, because that is the distinction that decided the
// new-manager calibration. attackBandAdj scales expected goals and assists;
// defenceBandAdj scales expected goals conceded.
//
//   - The three leakiest defences concede 31-70% more than the median club, and
//     attackers return about 23% above their own average against them.
//   - The bottom three attacks score materially less than the median, so a
//     defender facing one should be expected to concede less, which the clean
//     sheet term then converts into points on its own.
//
// The avoid side is deliberately smaller than the target side on attack, since
// the measured penalty for facing a top-three defence (-11% / -14%) is milder
// than the reward for facing a bottom-three one.
const (
	attackBandTarget  = 0.23
	attackBandAvoid   = 0.15
	defenceBandTarget = 0.25
	defenceBandAvoid  = 0.25
)

// attackBandAdj scales attacking returns by the opponent's defensive band.
func (e *Engine) attackBandAdj(opponentID int, strength float64) float64 {
	if strength <= 0 {
		return 1
	}
	b := e.teamBands()
	if !b.ready {
		return 1
	}
	switch b.defence[opponentID] {
	case bandWorst:
		return 1 + attackBandTarget*strength
	case bandBest:
		return 1 - attackBandAvoid*strength
	}
	return 1
}

// defenceBandAdj scales expected goals conceded by the opponent's attacking
// band. Facing a blunt attack means conceding less, so the target band returns
// a factor below one.
func (e *Engine) defenceBandAdj(opponentID int, strength float64) float64 {
	if strength <= 0 {
		return 1
	}
	b := e.teamBands()
	if !b.ready {
		return 1
	}
	switch b.attack[opponentID] {
	case bandWorst:
		return 1 - defenceBandTarget*strength
	case bandBest:
		return 1 + defenceBandAvoid*strength
	}
	return 1
}

// Reading a club's fixture RUN off the same bands the scoring path uses.
//
// # Why this exists, and what it must never become
//
// This is instrumentation. Nothing below is read by `Score`, by `Optimize` or by
// the transfer gate, and it must stay that way: "do not build a custom
// fixture-difficulty rating" is a closed line in this project's record, and a
// second rating that started life as a mediator would be exactly that, arriving
// through the one door nobody reviews as scoring.
//
// What it is for is making a *null readable*. A fixture-run arm that comes back
// flat has at least three explanations — the bands were never computed, the squad
// never faced a banded opponent, or the policy saw the distinction and declined to
// act on it — and they license opposite conclusions. Counting them apart is the
// same argument the banking funnel is built on.
//
// # It borrows the model's window rather than defining one
//
// The count runs over `TeamFixtures(team, horizon)`, which is the identical call
// `Metrics` makes at the identical horizon. So "the run" here is the run the
// engine actually scored, not a second opinion about how far ahead to look. That
// matters more than it sounds: "do not move the fixture window" is also closed,
// and a mediator quietly reading a different window would report on a lever nobody
// pulled.
//
// ⚠️ **The bands are computed from FINISHED fixtures only** (see teamBands), so
// this carries no scoreline a manager could not have seen. It does NOT clear the
// separate, known leak on the difficulty *rank*: `FixtureBrief.Difficulty` comes
// from the archive's end-stamped `team_h_difficulty`, which `playedFixtures`
// leaves alone. Nothing here reads Difficulty — only OpponentID and the bands —
// which is why this counter is clean where a difficulty-weighted one would not be.

// FixtureRun is what the 3/14/3 bands say about a club's next few fixtures:
// how many are against an opponent worth targeting, and how many against one
// worth avoiding.
type FixtureRun struct {
	// Ready is false before bandMinMatches have been played by enough clubs, when
	// there is no rating at all. A zero run with Ready false and a zero run with
	// Ready true are different facts — the first says the instrument could not
	// look, the second says it looked and found nothing.
	Ready bool
	// Fixtures is how many matches the window actually held.
	//
	// ⚠️ **Blanks and doubles do NOT change this**, which is the natural
	// misreading and the one that matters, since it is the reason a reader would
	// suspect a double gameweek of inflating an exposure count. The window is
	// `TeamFixtures(team, n)`, which takes the next n *fixtures* — so a double
	// changes which gameweeks those n span, not how many there are. It falls short
	// of the horizon only when the season runs out, or when a planned free hit has
	// removed gameweeks through SetSkipGameweeks.
	Fixtures int
	// Target and Avoid are the banded opponents in the window, counted from the
	// point of view of the position asked about.
	Target, Avoid int
}

// Net is the run as one signed number: how many more targets than avoids.
//
// Signed rather than a ratio because it is summed across moves downstream, and a
// ratio has no meaningful zero when the window is empty.
func (r FixtureRun) Net() int { return r.Target - r.Avoid }

// FixtureRunFor counts the banded opponents in a club's next `horizon` fixtures,
// from the point of view of a position.
//
// # Which band a position reads is not arbitrary
//
// A keeper or defender is paid mostly for the opponent NOT scoring, so the band
// that matters to him is the opponent's ATTACK: facing one of the three bluntest
// attacks is the fixture he wants. A midfielder or forward is paid for his own
// returns, so what matters is the opponent's DEFENCE.
//
// Which band each position reads is the same choice `attackBandAdj` and
// `defenceBandAdj` make, and it is why the model is already position-dependent
// without any per-position weight.
//
// ⚠️ **How much each is WORTH is not the same, so a count is never a proxy for
// the adjustment's size.** The model's coefficients are deliberately asymmetric
// on the attacking side — attackBandTarget 0.23 against attackBandAvoid 0.15,
// for the measured reason given above them — while `Net` weighs target and avoid
// equally. And the defensive band enters the clean sheet through `exp(-x)`, which
// is convex, so equal-weighted counts are not proportional there either.
//
// ⚠️ **Each position is scored on BOTH bands and this counts ONE, so the omitted
// channel is the larger one for a defender, not the smaller one for a
// midfielder.** `fixtureAdjustedXP90` passes both multipliers to
// `fixtureSensitiveAt` for *every* position, so a defender's goals and assists are
// re-priced by the opponent's **defence** band and nothing here sees it. Concretely:
// a defender moving to five opponents who are all bottom-three attacks and
// top-three defences counts `Target +5` and is entered in `band_run_moves`, while
// the engine also applied `1 - attackBandAvoid*s` to his attacking returns on all
// five — so on that channel the mediator's sign is the opposite of the model's.
//
// The midfielder case is the mirror and is milder: his clean sheet pays 1 against
// 5 or 6 for a goal, so dropping it costs precision rather than sign. The defender
// case is not obviously mild — the clean sheet is 26-45% of a defender's score, so
// the attacking remainder is a real share of him.
//
// This is documented rather than fixed because the block is instrumentation: it is
// a count of fixtures, never a proxy for what they were worth, and nothing is
// scored from any of it. A reader wanting the full picture must read the two
// channels off the engine, not off this column.
// TestFixtureRunReadsTheSameBandSideTheEngineDoes pins the side each position
// reads against `attackBandAdj`/`defenceBandAdj`, so the half that IS modelled
// cannot drift out of step silently.
func (e *Engine) FixtureRunFor(teamID, horizon, position int) FixtureRun {
	// BandChannelLive rather than teamBands().ready, so a run counted here is
	// always a run the engine could actually have priced. Under FPL_MAGNITUDE the
	// bands compute and reach nothing, and a non-ready FixtureRun is the honest
	// report of that. See BandChannelLive.
	if !e.BandChannelLive() {
		return FixtureRun{}
	}
	b := e.teamBands()
	// Keepers (1) and defenders (2) read the opponent's attack; midfielders and
	// forwards read the opponent's defence. See above. Bare element types because
	// that is this package's idiom — DefConThreshold spells it the same way.
	side := b.defence
	if position == 1 || position == 2 {
		side = b.attack
	}
	run := FixtureRun{Ready: true}
	for _, f := range e.TeamFixtures(teamID, horizon) {
		run.Fixtures++
		switch side[f.OpponentID] {
		case bandWorst:
			run.Target++
		case bandBest:
			run.Avoid++
		}
	}
	return run
}

// BandChannelLive reports whether the 3/14/3 bands can reach what the engine
// scores — which is a stronger question than whether the ratings exist.
//
// Two things have to hold, and only the first is about the bands:
//
//   - the ratings exist, which needs bandMinMatches played by enough clubs, so
//     it is false through the opening five or six gameweeks of every season;
//   - `fixtureMultipliersFor` actually consults them.
//
// ⚠️ **The second is why this is not simply `teamBands().ready`, and getting it
// wrong would have inverted the mediator this exists for.** Under
// `FPL_MAGNITUDE`, `fixtureMultipliersFor` returns `magnitudeAttack` and
// `magnitudeDefence` and **returns before `attackBandAdj` or `defenceBandAdj` is
// ever called** — so `BandStrength` reaches nothing at all, at any value. The
// bands themselves still compute perfectly well from finished fixtures, so a
// mediator asking only "are the ratings ready" would report a live-looking count
// off a lever that was bypassed one function above it.
//
// The failure that makes concrete: a tandem sweep runs with `FPL_MAGNITUDE` set,
// the band arm comes back flat, and the analyst reads "33 of 37 weeks ready, 31
// moves, 11 better / 10 worse" as *the policy had every opportunity and declined*
// — when the truth is the lever could not act. That is the exact inversion the
// mediator was built to prevent, delivered by the mediator.
//
// It is a *reading* and never a gate on scoring: nothing on the scoring path
// consults it, because attackBandAdj and defenceBandAdj already return 1 when the
// rating is absent, and the magnitude path never reaches them.
func (e *Engine) BandChannelLive() bool {
	if magnitudeDifficulty {
		return false
	}
	return e.teamBands().ready
}
