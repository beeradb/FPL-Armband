package config

import "armband/internal/analysis"

// OptionValuePolicy is the four levers that price a held option, plus the one
// curve all four share.
//
// # Why they are one block and not four settings in four places
//
// A banked transfer, a wildcard, a bench boost and a free hit are the same thing
// wearing different names: an option whose value decays as the window it can be
// exercised over shrinks. Every one of them is priced by a constant today.
// Splitting the four switches across `review_policy`, `chip_plan` and `weights`
// would let three of them be turned on and the fourth forgotten, which is how a
// tandem arm becomes a simple-effect arm without anybody deciding it should be.
//
// ⚠️ **Every switch here ships OFF and every default reproduces the shipped
// constant.** That is not caution about the mechanism — it is what makes the levers
// measurable at all. The replay compares arms, so off-by-default costs nothing in
// power, while defaulting one on would make every banked figure in
// `stats/snapshots/` non-comparable with every figure taken after it.
// `TestTheOptionValueLeversAreOffByDefault` fails if any default changes behaviour.
type OptionValuePolicy struct {
	// Pricing is the shared decay and congestion curve. See
	// analysis.OptionPricing; zeroes mean the package defaults.
	Pricing analysis.OptionPricing `json:"pricing"`

	// TaperFreeTransferValue makes `free_transfer_value` a function of the
	// season's remaining life and the squad's forward fixture congestion, rather
	// than the flat 2.0 charged in every gameweek.
	//
	// # The recorded prohibition, and why it does not bind here
	//
	// This record says `free_transfer_value` "must not taper as the season ends".
	// ⚠️ **That sentence is a CONSEQUENCE OF A CLASSIFICATION, not a measurement.**
	// If the constant is a confidence threshold filtering moves too small to be
	// told from noise, tapering is meaningless — noise does not shrink in May. If
	// it is an opportunity cost, tapering is *required*, because the future the
	// transfer is being saved for does shrink and is empty at GW38. Nothing here
	// measures which it is, and `free_transfer_value` has never been varied in any
	// banked sweep, so the level is untested at any value let alone any shape.
	//
	// What the taper adds is two meanings the shipped rule has no term for: the
	// value of a **reserved exit** from a fixture-driven move, and **insurance
	// against forced demand** when the fixtures pile up. See
	// analysis.TransferHoldFactor for which term carries which.
	TaperFreeTransferValue bool `json:"taper_free_transfer_value"`

	// Wildcard fires the chip on a repair cost priced in points.
	//
	// # This is a closed line, and the closure's stated reason does not transport
	//
	// "Do not build a state trigger for the wildcard" is recorded, and its reason
	// is that the tested trigger — the literal reading of *"cannot fix it with free
	// transfers"* — **measures transfer scarcity rather than squad quality, so it
	// fires at GW2 when the model has least data**. A repair cost in points looked
	// like a way past that, being a MAGNITUDE rather than a move count.
	//
	// ⚠️ **IT IS NOT, AND THE FALSIFIER FIRED.** Measured 2026-08-17 by
	// `TestDiagWildcardTrigger`, four seasons at entry GW1 and GW16: at this bar
	// the rule fires in the cell's **second week in 8 of 8 cells**, weighing
	// exactly one week in seven of them. Raising the reservation to 20 delays it by
	// one to three weeks on the GW16 cells and not at all on the GW1 cells.
	//
	// The magnitude is real and it is **model churn rather than squad decay**: one
	// gameweek after the fifteen is bought at the model's own optimum, that optimum
	// has moved by five to nine players, so the repair cost reads 20 to 36 points
	// in exactly the week the model knows least. **The closure stands unaltered**,
	// and this lever ships off and should stay off.
	//
	// It is kept, rather than deleted, because a refuted lever with a diagnostic
	// beside it is what stops the argument being rebuilt — and because the rule the
	// mechanism actually wants is a different quantity: a repair cost measured
	// against a squad the model still endorses, rather than against a fresh argmax
	// over the whole pool. That is unbuilt.
	//
	// ⚠️ **The replay cannot value a wildcard** — it replaces all fifteen, so the
	// within-season spread swamps it, and the record already says a wildcard replay
	// must not be read as a valuation. So this lever's deliverable is a **decision
	// count**: does it fire, and in which gameweeks. Do not quote a points figure
	// from it.
	Wildcard ChipTrigger `json:"wildcard"`

	// BenchBoost plays the chip on the biggest double available, against a bar
	// that falls as the chip's own life runs out.
	//
	// The shipped alternative is `chipBarBenchBoost` = 16, a fixed number on an
	// expiring chip, and this record already calls it asserted rather than measured
	// and notes that a bar set too low flatters timing while one set too high
	// flatters the threshold rule. An unplayed chip scores nothing at all, so a bar
	// that does not fall toward the expiry refuses weeks that were the best
	// remaining offer.
	BenchBoost ChipTrigger `json:"bench_boost"`

	// FreeHit plays the chip on the biggest blank available, on the same decaying
	// bar.
	//
	// The free hit is the cleanest of the four options because it has **no unwind
	// cost by construction**: it fields a temporary fifteen for one gameweek and
	// hands the permanent squad straight back. So its whole value is the week it is
	// spent on, and its whole cost is the better week it might have been spent on
	// instead — which is exactly what a decaying bar prices.
	FreeHit ChipTrigger `json:"free_hit"`
}

// ChipTrigger is one chip's state rule: whether it fires at all, and the bar it
// must clear when its window is unconstrained.
//
// The bar is a **base**: what the option is worth to hold with a long window
// ahead. What this week's decision is measured against is that base scaled by
// analysis.ChipBarAt, which falls to exactly zero in the last week the chip could
// be played — use it or lose it.
type ChipTrigger struct {
	// Enabled turns the rule on. Off means the chip is played only where a
	// `chip_plan` names it, which is the shipped behaviour.
	Enabled bool `json:"enabled"`

	// Bar is the base bar in points. Zero is meaningful — it means "play it the
	// first week it is worth anything at all" — so Load probes for the KEY rather
	// than for the value when backfilling it. A value-check migration would never
	// fire, because `cfg` starts from Default().
	Bar float64 `json:"bar_points"`
}

// DefaultBenchBoostBar and DefaultFreeHitBar are the base bars the chip triggers
// ship with.
//
// **Asserted, and inherited rather than derived.** They are the diagnostic's own
// `chipBarBenchBoost` = 16 and a free-hit figure set beside it, and this record
// already records both as asserted rather than measured. Carrying them across
// unchanged is deliberate: the change under test is the SHAPE — a bar that decays
// against one that does not — and moving the level at the same time would make the
// two channels inseparable in exactly the way a 2x2 exists to prevent.
//
// ⚠️ There is no recorded free-hit bar at all. 16 is used for both because the two
// chips pay in the same units (points in one week, against the squad that would
// otherwise be fielded), not because anybody measured the free hit.
const (
	DefaultBenchBoostBar = 16.0
	DefaultFreeHitBar    = 16.0
)

// DefaultWildcardReservation is the base reservation price a repair cost must
// beat before the wildcard fires.
//
// **Asserted.** 12 points is three hits, which is the point at which the FPL
// community's own rule of thumb — "if it takes more than two hits to fix, wildcard
// it" — starts to bite. ⚠️ That threshold was **unexpressible on this code until
// the hit ceiling became configurable**: `analysis.MoveLimit` clamped the allowance
// to one hit unconditionally, so "more than two hits to repair" had no input
// quantity on the transfer path. The repair cost here is computed from a full
// rebuild rather than from the transfer search, so it does not depend on that
// ceiling — but a rule phrased in hits and a search that cannot express two of them
// is the mismatch that made the constant look arbitrary.
const DefaultWildcardReservation = 12.0

// DefaultOptionValuePolicy is every lever off, with the bars at their asserted
// defaults so that turning a lever on does not also require choosing a level.
func DefaultOptionValuePolicy() OptionValuePolicy {
	return OptionValuePolicy{
		Pricing: analysis.OptionPricing{
			HalfLife:              analysis.DefaultOptionHalfLife,
			CongestionSensitivity: analysis.DefaultCongestionSensitivity,
			CongestionHorizon:     analysis.DefaultCongestionHorizon,
		},
		Wildcard:   ChipTrigger{Bar: DefaultWildcardReservation},
		BenchBoost: ChipTrigger{Bar: DefaultBenchBoostBar},
		FreeHit:    ChipTrigger{Bar: DefaultFreeHitBar},
	}
}

// Any reports whether any lever in the block is on.
//
// Used where a caller wants to skip the whole apparatus rather than ask four
// questions — and, more importantly, where a mediator needs to distinguish "the
// block was off" from "the block ran and nothing fired". Those license opposite
// conclusions and a zero count pools them.
func (p OptionValuePolicy) Any() bool {
	return p.TaperFreeTransferValue || p.Wildcard.Enabled ||
		p.BenchBoost.Enabled || p.FreeHit.Enabled
}
