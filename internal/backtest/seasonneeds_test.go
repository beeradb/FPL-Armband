package backtest

import "fmt"

// What a season carries, declared once, so a diagnostic can say what it NEEDS
// instead of naming the seasons it happens to want.
//
// # The problem this solves
//
// Before it, four diagnostics carried literal season lists and there was no way
// to tell two very different reasons apart by looking:
//
//   - `TestDiagDefconBias` pins 2024-25/2025-26 because 2025-26 is the only
//     archived season in which defensive contribution was a scoring category.
//     That is correct — and correct *downward* only: no earlier season can
//     acquire it, while the category is live, so later ones will have it. See the
//     `defcon` field for why that asymmetry matters.
//   - `TestDiagCleanSheetPoisson` pins four seasons because it was written before
//     the Understat backfill existed. That is an accident.
//
// Both look identical in the source: a slice of season strings. So when the
// six-season grid arrived, there was no way to answer "should this extend?"
// except by reading each diagnostic and reasoning about it — and a reader who
// did not would silently measure a mixed population, half six-season and half
// four, with nothing in the output saying which.
//
// Declaring the requirement inverts that. A diagnostic states the property it
// depends on; the grid is a consequence. Adding a season becomes one edit here
// rather than eight, and a season that fails a requirement is excluded *by
// construction* rather than by somebody remembering.
type seasonCaps struct {
	// xG means expected goals, assists and goals conceded are usable — either
	// FPL's own or the embedded Understat repair. Without it the model runs
	// crippled and an intervention that scales xG returns byte-identical
	// output, which reads as "no effect" and is not.
	xG bool
	// nativeXG means the xG is FPL's OWN, not the backfill. The distinction
	// matters because a repaired season carries a *borrowed* provider offset:
	// the level error is benign (shared by every player in a season, and an
	// argmax consumes an ordering) but the per-player dispersion is not, and no
	// rescaling removes it — the record puts the p90 of the per-player ratio at
	// 1.54. A diagnostic fitting a per-player quantity should want native.
	nativeXG bool
	// defcon means defensive contribution was a scoring category that season.
	// One season has it today, which is why anything measuring it runs on a grid
	// of one.
	//
	// ⚠️ "and always will be" stood here and is wrong. Defensive contribution is a
	// LIVE scoring category, so 2026-27 will carry it. What is permanent is only
	// the other direction: no season before 2025-26 can ever acquire it, because
	// the category did not exist to be recorded.
	//
	// ⚠️ And the widening is NOT automatic — a first correction here said it was,
	// which would have sent the next maintainer down a path where nothing moves.
	// Three edits, in this order:
	//
	//  1. add the season to `seasonOrder` as well as to the map. `gridFor` walks
	//     `seasonOrder`, and the map is only ever indexed BY it, so a row added
	//     here alone changes no grid and reports nothing.
	//  2. `defcon_bias_test.go` takes `gridFor(needsDefcon)[0]` — the FIRST pair.
	//     A wider grid leaves it replaying {2024-25, 2025-26} and silently
	//     ignoring the new season, which is a widening that measures nothing new.
	//  3. `TestSeasonNeedsReproduceTheNamedGrids` pins `len(gridFor(needsDefcon))
	//     == 1` and the exact pair, so it fails loudly. That failure is the
	//     designed signal and not an obstacle: it is what stops (1) landing
	//     without (2).
	defcon bool
	// transfers means the season's transfer path and wallet are samples of the
	// same process as every other season's. False for 2019-20 alone: FPL
	// granted unlimited free transfers before the GW30+ deadline after the
	// COVID restart and froze prices for three months. Its SCORING is fine,
	// which is exactly the HOLD/POLICY split this package already draws.
	transfers bool
	// playable is false for a season that can only ever be a prior. 2018-19
	// publishes players, fixtures and gameweeks but no clubs, so the loader
	// accepts it prior-only and it can never be replayed.
	playable bool
}

// seasonCapabilities is the single source of truth. Every fact here is recorded
// in the research notes and verified against the archive rather than assumed —
// see "The archive: what it carries, what it gets wrong".
var seasonCapabilities = map[string]seasonCaps{
	// Prior-only: no teams.csv. Repaired xG.
	"2018-19": {xG: true, transfers: true, playable: false},
	// COVID restart. Repaired xG. The one season whose transfer path is not
	// comparable, and the reason scoringPairNames is HOLD-only.
	"2019-20": {xG: true, transfers: false, playable: true},
	"2020-21": {xG: true, transfers: true, playable: true},
	"2021-22": {xG: true, transfers: true, playable: true},
	// FPL's own xG begins here — though only from GW16, with GW1-15 supplied by
	// the same repair. Counted as native because every existing grid treats it
	// so, and because the season aggregate it is fitted against is FPL's.
	"2022-23": {xG: true, nativeXG: true, transfers: true, playable: true},
	"2023-24": {xG: true, nativeXG: true, transfers: true, playable: true},
	"2024-25": {xG: true, nativeXG: true, transfers: true, playable: true},
	// Defensive contribution's first season, and the only archived one so far.
	// The category is live, so 2026-27 gets `defcon: true` too — see the `defcon`
	// field for the three edits that takes, because adding a row here alone moves
	// no grid.
	"2025-26": {xG: true, nativeXG: true, defcon: true, transfers: true, playable: true},
}

// seasonOrder is the archive in chronological order. Pairs are always
// consecutive: a prior is the season immediately before the one played.
var seasonOrder = []string{
	"2018-19", "2019-20", "2020-21", "2021-22",
	"2022-23", "2023-24", "2024-25", "2025-26",
}

// seasonNeeds is what a diagnostic requires. The zero value means "any season
// that can be played", which is the loosest honest grid.
//
// The two xG fields are separate because a pair has two ROLES and they do not
// need the same thing. `sweepPairNames` needs xG in the season being *played*
// and is content with a prior that has none — 2021-22 supplies minutes and
// points perfectly well. A diagnostic that builds a model from the prior season
// and scores it on the next needs xG in *both*, and reading zeroes there is not
// a weaker measurement but a different one.
type seasonNeeds struct {
	// xG in the season being played.
	xG bool
	// nativeXG in the season being played — FPL's own, not the repair.
	nativeXG bool
	// priorXG in the season supplying priors.
	priorXG bool
	// priorNativeXG in the season supplying priors.
	priorNativeXG bool
	// defcon in the season being played.
	defcon bool
	// transfers: the played season's transfer path must be comparable. Set this
	// for anything judged on POLICY; leave it for anything judged on HOLD.
	transfers bool
}

// gridFor returns the consecutive {prior, played} pairs that satisfy a
// requirement, in chronological order.
func gridFor(n seasonNeeds) [][2]string {
	var out [][2]string
	for i := 1; i < len(seasonOrder); i++ {
		prior, played := seasonOrder[i-1], seasonOrder[i]
		p, ok1 := seasonCapabilities[prior]
		c, ok2 := seasonCapabilities[played]
		if !ok1 || !ok2 || !c.playable {
			continue
		}
		if n.xG && !c.xG {
			continue
		}
		if n.nativeXG && !c.nativeXG {
			continue
		}
		if n.defcon && !c.defcon {
			continue
		}
		if n.transfers && !c.transfers {
			continue
		}
		if n.priorXG && !p.xG {
			continue
		}
		if n.priorNativeXG && !p.nativeXG {
			continue
		}
		out = append(out, [2]string{prior, played})
	}
	return out
}

// String makes a requirement printable, so a diagnostic can say in its own
// output which seasons it ran on and why — the thing that was missing when four
// diagnostics silently measured a different population from the sweeps they were
// quoted beside.
func (n seasonNeeds) String() string {
	s := ""
	add := func(f bool, name string) {
		if f {
			if s != "" {
				s += ", "
			}
			s += name
		}
	}
	add(n.xG, "xG")
	add(n.nativeXG, "native xG")
	add(n.priorXG, "prior xG")
	add(n.priorNativeXG, "prior native xG")
	add(n.defcon, "defensive contribution")
	add(n.transfers, "a comparable transfer path")
	if s == "" {
		s = "no special requirement"
	}
	return fmt.Sprintf("%s (%d pairs)", s, len(gridFor(n)))
}

// The requirements the existing named grids express. Each reproduces its grid
// exactly — TestSeasonNeedsReproduceTheNamedGrids pins that — so this is a
// restatement of what was already true, not a redefinition.
var (
	// needsSweep is the shipped four: FPL's own xG in the played season, and a
	// transfer path comparable with the others.
	needsSweep = seasonNeeds{nativeXG: true, transfers: true}
	// needsExtended accepts the Understat repair, which is what takes the grid
	// from four pairs to six and the degrees of freedom from three to five.
	needsExtended = seasonNeeds{xG: true, transfers: true}
	// needsScoring drops the transfer requirement, which admits 2019-20 and
	// makes the grid seven pairs — and HOLD-only by construction.
	needsScoring = seasonNeeds{xG: true}
	// needsPriorXG is for a diagnostic that reads xG from the prior season too.
	needsPriorXG = seasonNeeds{nativeXG: true, priorNativeXG: true, transfers: true}
	// needsDefcon pins itself to the one season that scored it.
	needsDefcon = seasonNeeds{defcon: true}
)

// playedSeasons is the seasons a requirement admits as the season being
// *played*, for a diagnostic that iterates seasons rather than pairs.
func playedSeasons(n seasonNeeds) []string {
	var out []string
	for _, p := range gridFor(n) {
		out = append(out, p[1])
	}
	return out
}
