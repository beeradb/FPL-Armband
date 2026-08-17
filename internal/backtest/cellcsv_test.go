package backtest

// The CSV contract between the Go replay and the R inference.
//
//	FPL_CELLS=/tmp/cells.csv DIAG=1 EXP=A go test ./internal/backtest \
//	    -run TestDiagTransferPolicy -v -timeout 90m
//	Rscript stats/sweep_inference.R /tmp/cells.csv
//
// # Why a file rather than more Go
//
// runPolicySweep used to run the grid *and* hand-roll the statistics that decide
// what the grid means, and the statistics half was the weak part in three
// specific ways: the season-clustered SE averaged four seasons and took their
// spread with no small-sample correction and no principled df, the variance
// decomposition knowingly computed marginal SEs by root-sum-square across layers
// that share every cell, and nothing anywhere controlled for multiplicity while
// roughly twenty constants times four to six alternatives were being judged at
// |t| >= 2.
//
// None of those are hard problems in a language with a mixed-model library, and
// all three are the kind of thing that is quietly wrong for a year in
// hand-rolled Go. So the split is: **Go owns the engine, R owns the inference,
// this CSV is the contract.** The engine stays in Go because the hot path is
// discrete combinatorial search — the per-formation DP, the knapsack, the paired
// local search — which is branchy scalar work, and a sweep arm is already ~15
// minutes wall.
//
// # What the contract guarantees
//
// One row per (sweep, variant, season, start point), which is the unit every
// paired comparison is built from. Specifically:
//
//   - **weeks is populated**, so the per-gameweek normalisation is reproducible
//     downstream rather than only pre-baked. Both the raw total and the
//     per-gameweek figure are emitted; R re-derives one from the other as a
//     check, because a silently wrong denominator is exactly how pooling start
//     points once weighted the earliest regime twice as heavily.
//   - **an infeasible cell still emits a row**, flagged. A variant that cannot
//     field a legal fifteen is a result about the variant, and dropping the cell
//     silently would leave R unable to see the hole — it would read as a
//     comparison on fewer cells rather than as a partial failure.
//   - **is_baseline marks exactly one arm per sweep.** reportPairedDifferences
//     pairs everything against variants[0], so R cannot pick the reference on its
//     own without guessing.
//   - **bank_up_to travels with the row.** sweepBankLimit pins every cell at 5
//     regardless of season, which is historically wrong for 2022-23 and 2023-24;
//     carrying it means a downstream analysis cannot forget that.
//   - **the arm says what it did, not what it was called.** setting,
//     min_expected_minutes and squad_hash are all read off the config the cell
//     actually ran under, so a sweep's own file can verify its arm and its
//     mediator instead of a reader verifying them by reading Go. See
//     cellRow.HasSetting for what each closes; the schedule screen's label
//     parsing is the consumer that motivated the first.
//   - **rows append and identify their run.** Several sweeps run in one session
//     and losing the earlier ones is the failure mode, so the file is opened for
//     append and every row carries a sweep label plus a per-process run_id. Two
//     runs of the same EXP block therefore stay distinguishable instead of
//     merging into one over-confident sample.
//
// Cells are flushed as they are computed rather than at the end, because full
// sweeps on this machine get killed under load and a partial CSV is worth having.

import (
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cellRow is one replayed cell: one variant, one season pair, one entry point.
//
// Identity fields are filled before Simulate runs so that a failure still has
// something to report; the metric fields are filled afterwards, or left zero
// with Infeasible set.
type cellRow struct {
	Sweep        string
	RunID        string
	Variant      string
	VariantIndex int
	IsBaseline   bool
	Season       string // the season being played
	PriorSeason  string // the season its priors come from
	StartGW      int
	Weeks        int // len(res.Weeks): gameweeks actually played
	BankUpTo     int
	Infeasible   bool
	PolicyPoints int
	HoldPoints   int
	Moves        int
	Hits         int

	// The variance decomposition's intermediate layers, when a sweep measures
	// them. HoldPoints is the autosub layer and PolicyPoints the transfer layer,
	// so only three more are needed for the full ladder:
	//
	//	Frozen         XI picked once, never re-picked, no captain, no autosub
	//	FrozenCaptain  the same XI with the armband restored
	//	Weekly         XI and captain re-picked weekly, still no autosub
	//	HoldPoints     + autosubs                     (= HOLD)
	//	PolicyPoints   + transfers                    (= POLICY)
	//
	// They exist so R can compute each mechanism's *marginal* contribution as a
	// per-cell quantity. The Go version computed it as the root-sum-square of two
	// adjacent cumulative SEs, which assumes independence between layers that
	// differ by one mechanism on the same cells and the same weeks — the test
	// documented that as invalid and reported it anyway. Adjacent layers
	// differenced per cell need no such assumption.
	HasLayers     bool
	Frozen        int
	FrozenCaptain int
	Weekly        int

	// The captaincy rungs: HOLD re-scored with the armband pinned to the day-one
	// pick, and with nobody doubled at all. See HoldCaptaincy for what each means
	// and why neither may replace HOLD.
	//
	// These are emitted by every ordinary sweep rather than by one diagnostic,
	// which is deliberate and free: runPolicySweep already computes HOLD, the
	// expensive part is the per-gameweek engine rebuild the rungs share, and the
	// question they answer — "does this metric keep the signal while losing the
	// noise?" — can only be asked of a sweep whose effect is already known. So
	// every sweep carries both instruments and any of them can be re-read later.
	//
	// Separate from HasLayers on purpose: the variance decomposition measures the
	// frozen ladder and not these, an ordinary sweep measures these and not the
	// ladder, and a blank column must keep meaning "not measured".
	HasCaptainRungs  bool
	HoldFixedCaptain int
	HoldNoCaptain    int

	// The transfer-banking mediator, embedded rather than flattened for the same
	// reason chipReadings is: it is the engine's own struct, so the column block
	// and the quantity it reports cannot drift into two definitions.
	//
	// # What it makes readable
	//
	// A banking arm's whole claim is that declining a move this week buys a
	// bigger package next week. Every reading of that claim so far has been a
	// points difference, and a points difference of zero has several explanations
	// that license opposite conclusions. The block is a **funnel**, and each step
	// removes one of them:
	//
	//	decision_weeks    weeks that reached the transfer decision at all
	//	consulted_weeks   of those, weeks the banking rule was asked in
	//	weighed_weeks     of those, weeks it had a real choice to weigh
	//	banked_weeks      of those, weeks it said wait
	//
	// Each is a subset of the one above, which is an invariant a reader can check
	// on any row — TestTheBankingFunnelNests pins it — and it is what makes a
	// zero attributable rather than ambiguous. `consulted < decision` says a
	// guard appeared in front of the rule; `weighed < consulted` says the rule
	// refused without comparing anything, because the allowance was at its
	// ceiling, the season was ending, or nothing cleared the gain floor in either
	// week; `banked < weighed` is the rule genuinely preferring to act now.
	//
	// **The counts are counts, not rates.** Cells run 38 to 13 gameweeks, so
	// banked_weeks pooled across entry points weights the earliest regime nearly
	// three times as heavily. decision_weeks is the denominator, and it is here
	// precisely because it is NOT recoverable as `weeks - 1` on any arm that
	// plays a wildcard or a free hit — those weeks make no transfer decision and
	// the file records no column for them.
	//
	// free_at_decision is the fifth column and the only mean: the allowance a
	// decision week ran with. ⚠️ In an arm that actually banks it is a
	// **post-treatment** quantity — banking raises the allowance it then
	// measures — so it is a covariate only where banked_weeks is 0, which is the
	// case it exists to diagnose. And it bounds the ceiling guard in one
	// direction only: the accrual guarantees at least 1 every week, so by Markov
	// on `free - 1` the share of weeks at the ceiling is at most
	// `(mean - 1)/(BankUpTo - 1)` — a low mean exonerates that guard outright, a
	// high one convicts nothing.
	//
	// # The off / never-consulted distinction, and how it is spelled
	//
	// shouldBank is only reachable when SimConfig.BankLookahead is on, so on an
	// arm that leaves it off the honest reading is "the question was never asked"
	// rather than "the answer was never yes". The file's standing rule spells
	// that: **blank is a gap and zero is a measurement**. banked_weeks and
	// weighed_weeks are blank when the rule was never consulted; consulted_weeks,
	// decision_weeks and free_at_decision are written on every arm that decided
	// anything, banking or not. So a reader sees
	//
	//	blank banked, blank consulted   the block was not recorded for this row —
	//	                                an infeasible cell, or a sweep that does
	//	                                not populate it
	//	blank banked, consulted 0       the rule was never consulted; today that
	//	                                means BankLookahead was off
	//	0 banked, consulted n           the rule ran n times and never fired
	//	m banked, consulted n           the rule ran n times and fired m
	//
	// which is the whole point of the block, and is why the five columns are one
	// unit and must be read together.
	HasBanking bool
	BankingMediator

	// The fixture-run mediator, the second funnel in this region. Named rather
	// than embedded because FixtureRunMediator.Moves would collide with
	// cellRow.Moves, which is the season's transfer count — two different
	// quantities one letter apart, and embedding would have made the shadowing
	// silent.
	//
	// It shares HasBanking as its gate on purpose: both funnels are counted on
	// weeks that reached `decide`, so they are recorded together or not at all,
	// and a reader can put band_ready_weeks over decision_weeks without checking
	// two flags. ⚠️ The nesting is `decision_weeks >= band_ready_weeks` and
	// `band_moves >= band_run_moves` — NOT one chain of four, because the last
	// three count moves and the first counts weeks. See fixtureRunCols.
	FixtureRuns FixtureRunMediator

	// The four option-value levers' funnels, and the chip-preparation credit's.
	//
	// They share HasBanking's gate for the reason the fixture-run block does: all
	// three funnels are counted on weeks that reached `decide`, so they are
	// recorded together or not at all and a reader can put any of them over
	// `decision_weeks` without checking a second flag.
	//
	// ⚠️ **Each lever's block goes blank on its OWN count, not on a shared one.**
	// `ftv_priced_weeks` is blank when the taper was never consulted; a chip
	// trigger's firing columns are blank when it never fired. That is what keeps a
	// null on one lever readable without reference to the others, which is the
	// whole reason the four switches are independent.
	TransferHold   TransferHoldMediator
	WildcardTrig   ChipTriggerMediator
	BenchBoostTrig ChipTriggerMediator
	FreeHitTrig    ChipTriggerMediator
	ChipPrep       ChipPrepMediator

	// The per-cell fixture dose. See doseCols for the two windows, the two traps,
	// and why nothing regresses on them here.
	//
	// Gated separately from HasBanking, because a dose is a property of the
	// SEASON and the ENTRY GAMEWEEK rather than of anything the decision loop did
	// — a cell that played no gameweek at all still has one — so it follows the
	// arm rule rather than the metric rule and survives asInfeasible.
	HasDose     bool
	ActDoubles  int
	ActBlanks   int
	LateDoubles int
	LateBlanks  int

	// What each chip actually returned in the week it was played, and which week
	// that was. Zero week means the arm did not play that chip in this cell.
	//
	// # These are event points and must never be divided by weeks
	//
	// Every other metric here is a season total that R normalises per gameweek,
	// and this pair is the exception: a chip pays once, so `gain` is a single
	// gameweek's points and the per-gw columns are deliberately absent rather
	// than blank. Dividing a one-off by weeks played is the inflation this
	// record already paid for once, on the anchored-chips arms.
	//
	// They exist because the mechanism was the sharpest reading in the chip
	// preparation work and was thrown away: the diagnostic accumulated an
	// arm-level integer, so "9.4 against 16.7" had no standard error and lived in
	// no file. Emitted per cell they are a paired column like any other, at zero
	// replay cost, which is this record's own rule — make the comparison sharper
	// rather than run more cells.
	//
	// **The denominator is the cells that played the chip, not the grid.** A cell
	// where the placement rule found no week is a cell the intervention could not
	// run in, and pooling it as a zero is the error the triple-captain null made
	// when it was quoted over 36 cells rather than 23.
	HasChipWeeks  bool
	BenchBoostGW  int
	BenchBoostPts int
	TripleCapGW   int
	TripleCapPts  int

	// The chip-week oracle's three readings of each scoring chip, per cell: the
	// best week in hindsight, the median week, and the first week clearing a
	// fixed bar. Filled from chipReadingsOf, which is where the bars and the
	// threshold rule are documented.
	//
	// # Why these are columns and not a printed table
	//
	// They were a printed table and nothing else. The table does print a line per
	// cell, so this is not "an aggregate was recorded and the detail was thrown
	// away" — it is that nothing downstream reads stdout, and the CSV the R layer
	// does read had no columns to put them in. So the quotable form was the
	// summary: six means over the grid, no dispersion, no standard error.
	// Emitted per cell they are keyed by the row they sit on, so
	// `(run_id, sweep, season, start_gw)` — read one `variant` at a time, since
	// the un-oracled arm of the same sweep banks blanks — pairs them without a
	// second key having to be invented and kept in step.
	//
	// # These are event points and must never be divided by weeks
	//
	// Same rule as the chip block above and for the same reason: a chip pays
	// once, so every `_pts` column here is a single gameweek's points and the
	// per-gw columns are deliberately absent rather than blank.
	//
	// **The median is a float.** The series is integer gains, so an even number
	// of weeks puts the median on a half-integer, and an int column would
	// truncate every such cell toward zero by up to half a point. That is a
	// systematic per-cell shift of a fixed sign rather than noise, so it does
	// not average away, and it lands on the *median* — which raises
	// `oracle − median` for a positive median rather than lowering it.
	// ⚠️ The sign here read the other way until review; the act was right and
	// its stated reason was backwards, which is the correction `as_flag` in
	// stats/cells_common.R already carries. A recorded "up to half a point,
	// which is the whole size of the quantity that row reports" went with it:
	// no run has resolved either chip difference, so the comparison was an
	// assertion.
	//
	// **`oracle` here is the argmax over gameweeks, not the `oracle` stamp at
	// the end of the schema.** The stamp says what hindsight the *cell* ran
	// under; these say what that hindsight found. A row carrying them is one
	// that ran under `decision:chipweek` **by construction** — chipReadingsOf
	// returns a block only when the oracle placed one, and `under` stamps from
	// the same SimConfig — but nothing asserts the implication, so it is what a
	// reader should filter on rather than a guarantee the schema enforces.
	// ⚠️ The argmax takes the FIRST week on a tie (see bestChipWeek), and the
	// gains are small integers, so a distribution built from `*_oracle_gw` is
	// biased early by construction and the block records no tie count.
	//
	// **The bars travel with the row**, on the rule the arm block states: a
	// sweep's own file must be able to verify what produced its columns rather
	// than a reader verifying it by reading Go. They are build constants and not
	// a swept setting, so this is one copy of one number per cell — the same shape
	// as min_expected_minutes, and for the same reason. ⚠️ This said "36 copies"
	// and the grid width has nothing to do with it: the claim is about redundancy
	// per ROW, which is one copy however many rows a run writes, and a comment is
	// exempt from the label scan by design — so this is a place a grid width can
	// only be written correctly, never derived. Two things need them.
	// `*_threshold_pts` is a *mixture*: `firstClearing` returns the first gain
	// at or above the bar, else the last week's, so `threshold >= bar` is
	// exactly "a week cleared" and `threshold < bar` is exactly the forced
	// end-of-season spend — a partition that is unrecoverable without the bar
	// and that says which rule the cell actually exercised. And the cells file
	// is opened for append and stacked across runs, so a bar edited between two
	// runs would pool two different quantities in one column with nothing
	// marking it. ⚠️ A commit in the provenance sidecar does not close either:
	// it names a build, and this record already carries a snapshot whose
	// figures its own named commit does not produce.
	//
	// The readings are a struct of their own, embedded, rather than another run
	// of flat fields. That is what lets the diagnostic carry a cell's readings
	// around and print them WITHOUT building a cellRow: a cellRow built outside
	// the sweep is one that claims no hindsight whatever its config ran under,
	// which TestEveryCellRowIsStamped refuses for good reason.
	HasChipOracle bool
	chipReadings

	// The accumulated-xPoints instrument: the same two season totals as
	// PolicyPoints and HoldPoints, scored from the *identical* eleven, autosubs,
	// armband outcome and chip, with the four conversion channels
	// analysis.XPointsResidual replaces taken off each player-gameweek.
	//
	// # Why they are a second pair of columns and not a second metric convention
	//
	// The pilot this exists for tunes on the quieter reading and validates once on
	// the loud one, so both must come out of the SAME cell — a paired difference
	// between two arms scored on two metrics is only interpretable if the arms
	// replayed identical football. Emitted per cell at zero extra replay cost, on
	// the same reasoning as the captaincy rungs: the question can only be asked of
	// a sweep whose effect is already known, so recording it on demand means it is
	// missing when wanted.
	//
	// **Float, not int.** An expectation is not a score; rounding it here would
	// quantise away most of what the instrument claims to be — see xPointsOver.
	//
	// Gated by HasXPoints for the reason every other block here is gated: the
	// variance decomposition builds its own cellRow and does not compute these, and
	// a 0.0 in a season-total column is a plausible number rather than a visible
	// gap. "Not measured" and "measured as nothing" are different facts.
	HasXPoints    bool
	PolicyXPoints float64
	HoldXPoints   float64

	// What the arm actually did, as opposed to what its label says it did.
	//
	// # Why these are read off the applied config rather than declared
	//
	// bank_up_to established the pattern — "so a variant that changes the bank
	// says so in the CSV" — and these three extend it to the two quantities a
	// sweep could not previously verify about itself.
	//
	//	Setting        the swept value, from a getter run on the applied SimConfig
	//	MinExpMinutes  the resolved opening-squad pool floor
	//	SquadHash      the opening fifteen, as a set
	//
	// Setting is what stops the schedule screen inferring a sweep's design from
	// its label text. stats/schedule_screen.R parsed the first number out of each
	// arm label to decide whether a family was an ordered ladder, which is a
	// coincidence rather than a test: it refused BONUS because two labels parse to
	// 0.5, it dropped TEAMFORM's baseline and MINHL's "flat" arm for carrying no
	// number at all, and it would have silently *accepted* a two-dimensional
	// family whose first coordinates happened to differ and reported a slope with
	// no referent. A getter cannot do any of those, because it runs on the config
	// the cell is about to be simulated under — the same trick as the oracle
	// stamp, and for the same reason. HasSetting is false for an arm that varies
	// no single scalar, which is a fact about the family the screen must be told
	// rather than left to infer.
	//
	// MinExpMinutes closes a gap named twice by the floor work: with no applied
	// column, a floor sweep's cells file could not verify its own arm and the
	// check was a code reading.
	//
	// SquadHash is the one that makes squad-selection arms self-verifying.
	// Squad-identical implies HOLD-identical, but the converse is false — a
	// swapped fifteenth man who is never fielded and never autosubbed leaves HOLD
	// untouched — so every "the mediator did not move" reading off a points column
	// is a proxy that a static probe otherwise has to close by hand, once per arm.
	// The hash is over the *sorted* ids, so it is set identity: the optimiser's
	// output order is not a fact about the squad.
	HasSetting    bool
	Setting       float64
	MinExpMinutes float64
	SquadHash     string

	// The hindsight this cell was granted, as the canonical stamp and its coarse
	// kind. **Last in the schema and never blank** — "-" and "none" for an
	// ordinary row, because blank means "not measured" everywhere else in this
	// file and every row does know its oracle state.
	//
	// They are written from the SimConfig the cell actually ran under, not from
	// the variant's label, so the stamp cannot disagree with what ran. That
	// distinction is the whole reason the oracle moved onto the config: with an
	// environment variable, provenance is a second mechanism that has to be kept
	// in sync by hand, and this package's signature bug is two expressions of one
	// quantity drifting apart.
	Oracle     string
	OracleKind string
}

// chipReadings is one cell's chip-week oracle block: per chip, the hindsight
// week and its gain, the median week's gain, and the threshold rule's.
//
// A named struct so the diagnostic that prints these can hold a cell's readings
// without holding a cellRow — see the field comment on cellRow.HasChipOracle.
type chipReadings struct {
	BenchBoostOracleGW     int
	BenchBoostOraclePts    int
	BenchBoostMedianPts    float64
	BenchBoostThresholdPts int
	BenchBoostBarPts       int
	TripleCapOracleGW      int
	TripleCapOraclePts     int
	TripleCapMedianPts     float64
	TripleCapThresholdPts  int
	TripleCapBarPts        int
}

// under stamps a row with the hindsight its cell actually ran under.
//
// Every diagnostic that builds a cellRow calls it, **from the SimConfig it is
// about to hand Simulate** rather than from a label or a remembered flag. That is
// the whole point of the oracle being a config field: the stamp and the
// simulation read one value, so they cannot disagree.
// TestEveryCellRowIsStamped counts the construction sites.
func (r cellRow) under(o Oracles) cellRow {
	r.Oracle, r.OracleKind = o.Stamp(), o.Kind()
	return r
}

// stamp is the oracle columns as they are written, defaulting an unset row to
// the inert pair rather than to blanks.
func (r cellRow) stamp() (string, string) {
	o, k := r.Oracle, r.OracleKind
	if o == "" {
		o = Oracles{}.Stamp()
	}
	if k == "" {
		k = Oracles{}.Kind()
	}
	return o, k
}

// asInfeasible marks a cell that could not be simulated.
//
// The metrics are zeroed rather than left at whatever the caller had, because a
// zero that is also flagged cannot be mistaken for a real score, whereas a
// half-filled row can. Weeks goes to zero too: Simulate failed, so no gameweeks
// were played and any per-gameweek figure would be a division by a number nobody
// measured.
// The oracle columns survive: they describe the arm rather than the
// measurement, and an infeasible cell of an oracled arm is still an oracled cell.
// The arm columns follow the oracle rule rather than the metric rule: Setting
// and MinExpMinutes describe the variant, and an infeasible cell of a k=24 arm
// is still a k=24 cell. SquadHash is the exception and is cleared, because it is
// a measurement — the whole reason this cell is flagged is that no fifteen was
// built, and a hash of an empty squad is a value where a gap is meant.
func (r cellRow) asInfeasible() cellRow {
	r.Infeasible = true
	r.Weeks, r.PolicyPoints, r.HoldPoints, r.Moves, r.Hits = 0, 0, 0, 0, 0
	r.HasLayers, r.Frozen, r.FrozenCaptain, r.Weekly = false, 0, 0, 0
	r.HasCaptainRungs, r.HoldFixedCaptain, r.HoldNoCaptain = false, 0, 0
	// The banking block follows the metric rule rather than the arm rule: it is
	// a count of what the decision loop did, and an infeasible cell never ran
	// one. Leaving it would report decision weeks in a cell that played none.
	r.HasBanking, r.BankingMediator = false, BankingMediator{}
	// The fixture-run funnel goes with it, for the identical reason: it counts
	// what the decision loop did, and an infeasible cell never ran one. So do the
	// four option-value funnels and the preparation credit's.
	r.FixtureRuns = FixtureRunMediator{}
	r.TransferHold = TransferHoldMediator{}
	r.WildcardTrig, r.BenchBoostTrig, r.FreeHitTrig = ChipTriggerMediator{},
		ChipTriggerMediator{}, ChipTriggerMediator{}
	r.ChipPrep = ChipPrepMediator{}
	// ⚠️ The DOSE block survives, and that is the arm rule rather than an
	// oversight: it is a function of the season and the entry gameweek, which an
	// infeasible cell still has, exactly as it still has `season` and `start_gw`.
	// Clearing it would report a cell with no doubles in a season that had them.
	r.HasChipWeeks = false
	r.BenchBoostGW, r.BenchBoostPts, r.TripleCapGW, r.TripleCapPts = 0, 0, 0, 0
	r.HasChipOracle, r.chipReadings = false, chipReadings{}
	r.HasXPoints, r.PolicyXPoints, r.HoldXPoints = false, 0, 0
	r.SquadHash = ""
	return r
}

// squadHash renders an opening fifteen as a short stable digest.
//
// Sorted first, so the digest is set identity rather than a fact about the
// optimiser's output order — TestSeedOrderIsDeterministic exists because that
// order was once not stable at all, and a hash that moved with it would report a
// squad change on every such landscape.
//
// FNV-1a and twelve hex digits: this is an equality check between cells of one
// sweep, not a cryptographic commitment, and a short column stays readable in a
// CSV a human scans. Empty for an empty squad, so "no fifteen" cannot be
// mistaken for a fifteen that happened to hash to zero.
func squadHash(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	sorted := append([]int(nil), ids...)
	sort.Ints(sorted)
	h := fnv.New64a()
	for _, id := range sorted {
		fmt.Fprintf(h, "%d,", id)
	}
	return fmt.Sprintf("%012x", h.Sum64()&0xffffffffffff)
}

var cellHeader = []string{
	"sweep", "run_id", "variant", "variant_index", "is_baseline",
	"season", "prior_season", "start_gw", "weeks", "bank_up_to", "infeasible",
	"policy_points", "hold_points", "moves", "hits",
	"policy_per_gw", "hold_per_gw",
	"frozen_points", "frozen_per_gw",
	"frozen_captain_points", "frozen_captain_per_gw",
	"weekly_points", "weekly_per_gw",
	"hold_fixedcap_points", "hold_fixedcap_per_gw",
	"hold_nocap_points", "hold_nocap_per_gw",
	"decision_weeks", "consulted_weeks", "weighed_weeks", "banked_weeks",
	"free_at_decision",
	"band_ready_weeks", "band_moves", "band_run_moves", "band_worse_moves",
	"band_exposure",
	"ftv_weeks", "ftv_priced_weeks", "ftv_gate_calls", "ftv_flips",
	"ftv_mean_charge", "ftv_mean_load",
	"wc_trig_offered", "wc_trig_weeks", "wc_trig_weighed", "wc_trig_gw",
	"wc_trig_value", "wc_trig_bar",
	"bb_trig_offered", "bb_trig_weeks", "bb_trig_weighed", "bb_trig_gw",
	"bb_trig_value", "bb_trig_bar",
	"fh_trig_offered", "fh_trig_weeks", "fh_trig_weighed", "fh_trig_gw",
	"fh_trig_value", "fh_trig_bar",
	"prep_weeks", "prep_credit_weeks", "prep_bench_sum", "prep_captain_sum",
	"dose_act_doubles", "dose_act_blanks",
	"dose_late_doubles", "dose_late_blanks",
	"bench_boost_gw", "bench_boost_pts", "triple_captain_gw", "triple_captain_pts",
	"bench_boost_oracle_gw", "bench_boost_oracle_pts",
	"bench_boost_median_pts", "bench_boost_threshold_pts", "bench_boost_bar_pts",
	"triple_captain_oracle_gw", "triple_captain_oracle_pts",
	"triple_captain_median_pts", "triple_captain_threshold_pts",
	"triple_captain_bar_pts",
	"hold_xpoints", "hold_xpoints_per_gw",
	"policy_xpoints", "policy_xpoints_per_gw",
	"setting", "min_expected_minutes", "squad_hash",
	"oracle", "oracle_kind",
}

// captainRungCols is how many columns the captaincy rungs occupy, and oracleCols
// how many the oracle stamp occupies.
//
// Named so the stale-header regression test can strip exactly these blocks to
// synthesise an earlier schema, rather than carrying magic numbers that silently
// stop matching the next time a column is added. The oracle block is **last**, so
// stripping oracleCols alone yields precisely this build's predecessor and
// stripping both yields the one before that.
const (
	captainRungCols = 4
	// bankingCols is the transfer-banking mediator: the four-step funnel from
	// decision weeks down to weeks the rule said wait, plus the mean allowance a
	// decision week ran with. See cellRow.HasBanking for what each step removes.
	//
	// Five rather than two. The pair `banked_weeks` and `free_at_decision` is the
	// readable minimum, and three separate reviews found the same two holes in
	// it: a count with no denominator cannot be pooled across cells of 38 and 13
	// gameweeks, and a blank cannot say whether the rule was switched off or
	// merely never reached. Each missing column costs a **full sweep** to recover
	// and one integer to record, which is the whole argument.
	//
	// ⚠️ **Only the last is a rate.** The four counts must not be divided by
	// `weeks` — their denominator is `decision_weeks`, which is in the block —
	// and the mean must not be divided by anything at all.
	//
	// It sits between the captaincy rungs and the chip block. That is the only
	// free slot in the schema: the chip-oracle block must stay immediately after
	// the chip block, xPoints immediately after that, the arm block after that
	// and the oracle pair last, and every one of those four contracts has a
	// position test indexing from the end.
	bankingCols = 5
	// fixtureRunCols is the fixture-run mediator, the second funnel in the
	// decision-mediator region and the counterpart of the banking one beside it:
	//
	//	band_ready_weeks  of decision_weeks, weeks the 3/14/3 bands existed
	//	band_moves        transfers made in those weeks, with both players resolvable
	//	band_run_moves    of those, moves toward the better run
	//	band_worse_moves  of those, moves that traded the run away
	//	band_exposure     the signed size of the whole, in banded fixtures
	//
	// ⚠️ **band_run_moves and band_worse_moves do not sum to band_moves**, and
	// that is the point of carrying both: the remainder is moves the bands had
	// nothing to say about, which "runs converge" predicts is the modal case. With
	// only the favourable count, `band_moves - band_run_moves` would pool ties
	// with reversals and `band_run_moves` would have no null to be read against.
	//
	// A separate counted block rather than five more banking columns, because the
	// two funnels have different denominators — the banking one counts weeks all
	// the way down, and the last four of these count MOVES. Merging them into one
	// `bankingCols` would put a moves count under a header block whose own
	// documentation says every column in it is a week, which is precisely the kind
	// of mislabelling this file's rules exist to stop.
	//
	// It sits immediately after the banking block and before the chip block, so
	// both of those keep their named neighbours — see
	// TestTheFixtureRunBlockSitsBetweenBankingAndTheChips.
	//
	// ⚠️ **`band_exposure` is the one signed column in the file**, and the only
	// one where a negative number is a normal reading rather than a fault: across
	// 63 replayed transfers the incoming player's fixtures got *harder* 63% of the
	// time, so a negative sum is the recorded prior and not a sign error.
	fixtureRunCols = 5
	// optionCols is the four option-value levers' funnels, the third and last
	// block in the decision-mediator region:
	//
	//	ftv_weeks / ftv_priced_weeks   weeks the free-transfer taper ran, and
	//	                               weeks it actually moved the charge
	//	ftv_gate_calls / ftv_flips     gate answers in those weeks, and how many
	//	                               the untapered charge would have reversed
	//	ftv_mean_charge / ftv_mean_load the applied charge and the squad's forward
	//	                               fixture density, averaged over ftv_weeks
	//	<chip>_trig_offered            weeks the chip rule was OFFERED, before its
	//	                               own eligibility guards
	//	<chip>_trig_weeks / _weighed   of those, weeks it was consulted and weeks it
	//	                               had a reading to weigh: wc, bb, fh
	//	<chip>_trig_gw / _value / _bar the week it fired in, the reading that
	//	                               cleared, and the bar it cleared
	//	prep_weeks / prep_credit_weeks the chip-preparation credit's funnel, which
	//	                               had NO mediator at all before this
	//	prep_bench_sum / _captain_sum  the credit's level, in the per-gameweek
	//	                               units the gate consumed
	//
	// ⚠️ **Four levers, four independent funnels, and no block-level flag.** Each
	// lever is switchable on its own — the likely end state is chip placement on
	// and banking off — so a null on one has to be readable without reference to
	// the others. A single `option_value_on` column would make that impossible and
	// is deliberately absent.
	//
	// ⚠️ **`ftv_flips` counts GATE ANSWERS, not weeks and not transfers.** A week
	// offering a funded pair and a solo swap asks the gate three times, so it can
	// exceed `ftv_weeks`; `ftv_gate_calls` is its denominator and is in the block
	// for exactly that reason. And a flip is not a changed transfer — `decide`
	// returns on a refusal, so a flip on a later candidate may change nothing.
	//
	// ⚠️ **`_trig_offered` is what tells a lever that was OFF from one that ran and
	// was blocked all season.** `eligible` refuses the whole season when a plan
	// already places that chip, so a 2x2 crossing calendar placement with a trigger
	// reads an all-zero funnel in the planned corner — a lever that ran and
	// correctly declined, wearing the clothes of one that was never wired. Read
	// `offered = 0` as off, `offered > 0` with `weeks = 0` as blocked.
	//
	// ⚠️ **`wc_trig_gw` is the wildcard lever's whole deliverable.** The replay
	// cannot value a wildcard — it replaces all fifteen and the within-season
	// spread swamps it — so the question is a decision count, and the recorded
	// closure of the wildcard-trigger line rests on the tested trigger firing at
	// GW2. Reading this column tells you whether a repair-cost trigger does the
	// same. Do not quote a points figure from that arm.
	optionCols = 28
	// doseCols is the per-cell fixture dose, and it is NOT a mediator: it is a
	// function of the season and the entry gameweek alone, identical on every arm
	// of a cell, and it exists so a doubles or blanks arm can be read as a
	// dose-response rather than as a pooled mean over 36 differences.
	//
	//	dose_act_doubles / dose_act_blanks   club-gameweeks in the ACTIONABLE
	//	                                     window [start+1, 38]
	//	dose_late_doubles / dose_late_blanks club-gameweeks beyond the opening
	//	                                     squad's own horizon, [start+H, 38]
	//
	// # Why the actionable window and not the played one
	//
	// A cell entering at GW n plays [n, 38] but can ACT only on [n+1, 38]: the
	// opening fifteen is chosen at the entry deadline, so a double in the entry
	// week is football the cell scores and no transfer can be banked into.
	// Counting it as dose credits the mechanism with a week it could not act on.
	//
	// # And why a second, sharper pair
	//
	// The opening squad is built on a horizon of H gameweeks, so every double
	// inside [start+1, start+H-1] was already visible to and priced by the squad
	// build. What is left for the TRANSFER POLICY to add is what falls beyond it,
	// which is the `dose_late_*` pair. That is the quantity nobody had defined.
	//
	// ⚠️ **Two traps, and either one manufactures a slope.**
	//
	//   - **92% of doubles fall after GW19**, so a late-entry cell has more dose
	//     per gameweek AND fewer gameweeks. Dose and denominator move together, so
	//     a per-gameweek outcome regressed on dose picks up the entry point rather
	//     than the doubles. Put entry gameweek in the model or stratify on it.
	//   - **The 36 cells collapse to about 14 distinct doses**, with effective
	//     seasons around 4.4, because cells within a season are nested and share
	//     nearly all their doubles. The dose axis is far thinner than 36 rows look,
	//     and a standard error computed as though the rows were independent is
	//     wrong by roughly the ratio.
	//
	// ⚠️ **These columns are emitted and NOTHING here regresses on them.** No
	// slope has been fitted, none is quoted, and a dose-response is a separate act
	// requiring its own pre-registration against both traps above.
	//
	// ⚠️ The alternative reading of "captured by the opening squad" — clubs the
	// opening fifteen actually owns — is deliberately not the one taken. It varies
	// by arm, which would make this a mediator rather than a dose, and a covariate
	// that moves with the treatment is not a covariate.
	//
	// ⚠️ **A THIRD trap: the dose is hindsight.** It reads the whole fixture list,
	// so it knows every double from GW1 where a real manager learns of one as cup
	// rounds resolve. As a covariate that is fine — identical across a cell's arms,
	// so it cannot flatter one — and it is fatal to reading a fitted slope as "what
	// targeting doubles is worth". See dose.go.
	doseCols = 4
	// chipWeekCols is the chip block: two gameweeks and two one-off point
	// totals. Four rather than eight because there are no per-gw columns here —
	// see cellRow.HasChipWeeks for why a chip must not be normalised by weeks.
	chipWeekCols = 4
	// chipOracleCols is the chip-week oracle's block: per chip, the hindsight
	// week and its gain, the median week's gain, the threshold rule's, and the
	// bar that rule was run against. Ten rather than the six readings the table
	// printed. The chosen gameweek is the block's mediator — an argmax that
	// picked one week everywhere would be a fixed-week policy wearing an
	// oracle's label, which reportChipCells checks and could not check off a
	// banked file without it — and the bar is what makes the threshold column
	// interpretable at all; see cellRow.HasChipOracle for both. No per-gw
	// columns, for the reason given there.
	//
	// It sits immediately after the chip block and before the xPoints block, so
	// the two positional contracts below both still hold: the oracle pair stays
	// last and the arm block stays immediately before it.
	chipOracleCols = 10
	// xPointsCols is the accumulated-xPoints instrument: HOLD and POLICY as
	// totals and per gameweek. Four rather than two because both carry the
	// per-gameweek figure the whole `weeks` contract exists for — these are
	// season totals, not one-off event points like the chip block above.
	//
	// Inserted *before* the arm block and therefore before the oracle pair, which
	// is what keeps both stripping contracts true —
	// TestTheArmBlockIsBeforeTheOracleBlockAndCounted and
	// TestOracleColumnsAreLastAndCounted both index from the end.
	xPointsCols = 4
	// armCols is the self-verification block: the swept setting, the applied
	// pool floor and the opening-fifteen hash. Added *after* the chip block and
	// *before* the oracle pair, which is the only position that keeps the oracle
	// stripping contract true — see TestOracleColumnsAreLastAndCounted.
	armCols    = 3
	oracleCols = 2
)

// The means file carries the stamp too, and that matters *more* than the cells
// file carrying it: the means rows are the pre-computed numbers, and an
// already-averaged number is the one that gets pasted into a document. The
// failure being designed against is a whole section measured under one setting,
// retracted, and then cited as ground truth by a later audit that had no way to
// see which arm the figure came from.
var meansHeader = []string{
	"sweep", "run_id", "metric", "variant", "variant_index",
	"baseline_variant", "mean_per_gw", "n_cells", "oracle",
}

// perGW renders points/weeks, or the empty string when no gameweeks were played.
//
// Empty rather than zero: an infeasible cell has no per-gameweek figure, and
// writing 0 would put a plausible number in a column R averages. 'g' with -1
// precision is the shortest representation that round-trips the float64 exactly,
// so R reads back the same value Go computed and the reproduction check compares
// arithmetic rather than formatting.
func perGW(points, weeks int) string {
	if weeks <= 0 {
		return ""
	}
	return strconv.FormatFloat(float64(points)/float64(weeks), 'g', -1, 64)
}

// floatOrBlank and perGWf are perGW's pair for a metric whose total is already a
// float — the accumulated-xPoints columns.
//
// Same formatting rule and for the same reason: 'g' with -1 precision is the
// shortest representation that round-trips a float64 exactly, so R's own
// `per_gw == points / weeks` check compares arithmetic rather than formatting.
// Writing the total at anything less would make the contract check fail on
// rounding and be "fixed" by loosening the tolerance, which is how a real
// denominator bug gets through.
func floatOrBlank(x float64) string {
	return strconv.FormatFloat(x, 'g', -1, 64)
}

func perGWf(x float64, weeks int) string {
	if weeks <= 0 {
		return ""
	}
	return strconv.FormatFloat(x/float64(weeks), 'g', -1, 64)
}

// cellSink appends cell rows and per-comparison means to CSV files.
//
// A nil *cellSink is usable and does nothing, so every call site can be
// unconditional and the env var is checked in exactly one place.
type cellSink struct {
	mu    sync.Mutex
	cells *csvFile
	means *csvFile
	runID string
}

// The run id and the sweep ordinal are **process-global**, not per-sink.
//
// Each runPolicySweep call opens its own sink, so a counter living on the sink
// restarts at 1 for every block — and with EXP unset every block in a test shares
// one label, so two of them would both be written as "TestDiagRejudge#1". R keys
// a comparison on (run_id, sweep), so that collision would silently pool two
// unrelated experiments into one over-confident sample, which is precisely what
// the label exists to prevent. A per-sink run id has the same flaw one level up:
// two sinks opened in the same second would agree on it too.
var (
	cellRunOnce sync.Once
	cellRunID   string
	sweepMu     sync.Mutex
	sweepSeq    int
)

func runIDForProcess() string {
	cellRunOnce.Do(func() {
		cellRunID = fmt.Sprintf("%d-%d", time.Now().Unix(), os.Getpid())
	})
	return cellRunID
}

func nextSweepOrdinal() int {
	sweepMu.Lock()
	defer sweepMu.Unlock()
	sweepSeq++
	return sweepSeq
}

type csvFile struct {
	f *os.File
	w *csv.Writer
}

// newCSVFile opens path for append, writing the header only if the file is new.
//
// It refuses a file whose existing header is not this build's, which is the
// "a cache version is not a schema check" lesson applied here: appending 23-column
// rows under a 17-column header from an older build produces a file that is
// ragged rather than obviously broken, and the most likely reader — a
// half-remembered `Rscript` on last week's path — would either error somewhere
// unhelpful or silently misalign the columns. Refusing at open time costs one
// stat and names the problem.
func newCSVFile(path string, header []string) (*csvFile, error) {
	// Append, never truncate. Several sweeps run in one session and an earlier
	// block's cells are not recoverable once overwritten.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	c := &csvFile{f: f, w: csv.NewWriter(f)}
	if st.Size() == 0 {
		if err := c.w.Write(header); err != nil {
			f.Close()
			return nil, err
		}
		c.w.Flush()
		return c, nil
	}
	if err := checkHeader(path, header); err != nil {
		f.Close()
		return nil, err
	}
	return c, nil
}

// checkHeader compares an existing file's first record against the header this
// build writes.
func checkHeader(path string, want []string) error {
	r, err := os.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()
	got, err := csv.NewReader(r).Read()
	if err != nil {
		return fmt.Errorf("%s exists but its header is unreadable: %w", path, err)
	}
	if len(got) != len(want) {
		return fmt.Errorf("%s has a %d-column header and this build writes %d; "+
			"it was written by a different schema — use a fresh path",
			path, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Errorf("%s column %d is %q and this build writes %q; "+
				"it was written by a different schema — use a fresh path",
				path, i+1, got[i], want[i])
		}
	}
	return nil
}

func (c *csvFile) write(rec []string) {
	if c == nil {
		return
	}
	// Flush every row: a sweep that is killed under load should keep the cells
	// it had already paid for.
	_ = c.w.Write(rec)
	c.w.Flush()
}

func (c *csvFile) close() {
	if c == nil {
		return
	}
	c.w.Flush()
	c.f.Close()
}

// openCellSink returns a sink writing to FPL_CELLS, or nil when it is unset.
//
// The means file sits beside the cells file with a .means.csv suffix. It carries
// Go's own paired means, which R recomputes from the cells and asserts against —
// the one quantity deliberately computed twice, because a checked duplicate is a
// pipeline test and an unchecked one is the bug class this project has shipped
// twice.
func openCellSink(path string) (*cellSink, error) {
	if path == "" {
		return nil, nil
	}
	cells, err := newCSVFile(path, cellHeader)
	if err != nil {
		return nil, err
	}
	means, err := newCSVFile(strings.TrimSuffix(path, ".csv")+".means.csv", meansHeader)
	if err != nil {
		cells.close()
		return nil, err
	}
	return &cellSink{
		cells: cells,
		means: means,
		// Seconds and pid, fixed for the process: enough to separate two runs of
		// the same EXP block appended to one file, which would otherwise pool as
		// a single over-confident sample.
		runID: runIDForProcess(),
	}, nil
}

// sweepLabel names a sweep uniquely within this process.
//
// The ordinal matters: with EXP unset a whole test runs every block, and without
// it every sweep in the session would share one label and R would pair variants
// across unrelated experiments.
func (s *cellSink) sweepLabel(base string) string {
	if s == nil {
		return ""
	}
	if base == "" {
		base = "sweep"
	}
	return fmt.Sprintf("%s#%d", base, nextSweepOrdinal())
}

func (s *cellSink) run() string {
	if s == nil {
		return ""
	}
	return s.runID
}

func (s *cellSink) cell(r cellRow) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := []string{
		r.Sweep, r.RunID, r.Variant,
		strconv.Itoa(r.VariantIndex), strconv.FormatBool(r.IsBaseline),
		r.Season, r.PriorSeason,
		strconv.Itoa(r.StartGW), strconv.Itoa(r.Weeks), strconv.Itoa(r.BankUpTo),
		strconv.FormatBool(r.Infeasible),
		strconv.Itoa(r.PolicyPoints), strconv.Itoa(r.HoldPoints),
		strconv.Itoa(r.Moves), strconv.Itoa(r.Hits),
		perGW(r.PolicyPoints, r.Weeks), perGW(r.HoldPoints, r.Weeks),
	}
	// Empty, not zero, when a sweep did not measure the layers. A zero here
	// would read downstream as "the frozen eleven scored nothing", which is a
	// number rather than a gap.
	if r.HasLayers {
		rec = append(rec,
			strconv.Itoa(r.Frozen), perGW(r.Frozen, r.Weeks),
			strconv.Itoa(r.FrozenCaptain), perGW(r.FrozenCaptain, r.Weeks),
			strconv.Itoa(r.Weekly), perGW(r.Weekly, r.Weeks))
	} else {
		rec = append(rec, "", "", "", "", "", "")
	}
	// Same rule for the captaincy rungs: blank when a sweep did not score them,
	// because "nobody was doubled and the total was zero" is a number and "this
	// sweep did not measure the rung" is a gap.
	if r.HasCaptainRungs {
		rec = append(rec,
			strconv.Itoa(r.HoldFixedCaptain), perGW(r.HoldFixedCaptain, r.Weeks),
			strconv.Itoa(r.HoldNoCaptain), perGW(r.HoldNoCaptain, r.Weeks))
	} else {
		rec = append(rec, "", "", "", "")
	}
	// The transfer-banking mediator.
	//
	// # Pre-registered liveness rule
	//
	// **Any banking arm whose banked_weeks is 0 everywhere is a comparison that
	// never ran, and its deliverable is the mediator count, not a null.**
	//
	// ⚠️ That sentence is the pre-registration as handed over, and one word of it
	// is looser than this file's own vocabulary. "A comparison that never ran" is
	// the term of art for a setting that never reached its consumer — the
	// BandStrength case — and a non-blank 0 here **refutes** exactly that. The
	// precise claim is stronger and is a code fact: the banked branch of `decide`
	// is a pure early return, so an arm that never banks takes every decision the
	// greedy arm takes, and its points columns are byte-identical **by
	// construction**. That is a confinement rather than a null, and the count is
	// what carries information.
	//
	// ⚠️ **A dose is not an effect.** An arm firing in fewer than four of the six
	// seasons is *unmeasurable* rather than null — the season-clustered t is
	// capped by construction, as this record already records for the minutes
	// floor — so it gets no p, no interval and no threshold, and the per-season
	// fire counts are the report. Read banked_weeks as a rate over
	// decision_weeks, never as a pooled count.
	//
	// The four counts are blank or written under two different gates, because
	// they answer different questions: the two the rule owns go blank when it was
	// never consulted, and the two the decision loop owns are written whenever it
	// ran. See cellRow.HasBanking for the readings that produces.
	if r.HasBanking && r.DecisionWeeks > 0 {
		rec = append(rec,
			strconv.Itoa(r.DecisionWeeks), strconv.Itoa(r.ConsultedWeeks))
	} else {
		rec = append(rec, "", "")
	}
	if r.HasBanking && r.ConsultedWeeks > 0 {
		rec = append(rec,
			strconv.Itoa(r.WeighedWeeks), strconv.Itoa(r.BankedWeeks))
	} else {
		rec = append(rec, "", "")
	}
	if r.HasBanking && r.DecisionWeeks > 0 {
		rec = append(rec, floatOrBlank(r.MeanFreeAtDecision()))
	} else {
		rec = append(rec, "")
	}
	// The fixture-run funnel, under the same blank-is-a-gap rule and with its own
	// second step. band_ready_weeks is written whenever the decision loop ran, so
	// a zero there is a measurement: it says the bands never existed in this cell,
	// which is the true reading of an early-entry cell and was invisible before.
	//
	// The three move columns go blank when band_ready_weeks is 0, because with no
	// bands there is no exposure to have moved and a 0 would assert that transfers
	// were measured against a rating that did not exist. That is the same
	// distinction weighed_weeks and banked_weeks draw against consulted_weeks.
	if r.HasBanking && r.DecisionWeeks > 0 {
		rec = append(rec, strconv.Itoa(r.FixtureRuns.ReadyWeeks))
	} else {
		rec = append(rec, "")
	}
	if r.HasBanking && r.FixtureRuns.ReadyWeeks > 0 {
		rec = append(rec,
			strconv.Itoa(r.FixtureRuns.Moves),
			strconv.Itoa(r.FixtureRuns.RunMoves),
			strconv.Itoa(r.FixtureRuns.WorseMoves),
			strconv.Itoa(r.FixtureRuns.Exposure))
	} else {
		rec = append(rec, "", "", "", "")
	}
	// The free-transfer taper's funnel. `ftv_weeks` is written whenever the
	// decision loop ran, so a zero there is a measurement — it says the taper was
	// switched off — and the other five go blank behind it, because with no
	// consulted week there is no charge to have moved and a 0 would assert that
	// one was measured.
	if r.HasBanking && r.DecisionWeeks > 0 {
		rec = append(rec, strconv.Itoa(r.TransferHold.ConsultedWeeks))
	} else {
		rec = append(rec, "")
	}
	if r.HasBanking && r.TransferHold.ConsultedWeeks > 0 {
		rec = append(rec,
			strconv.Itoa(r.TransferHold.PricedWeeks),
			strconv.Itoa(r.TransferHold.GateCalls),
			strconv.Itoa(r.TransferHold.Flips),
			floatOrBlank(r.TransferHold.MeanCharge()),
			floatOrBlank(r.TransferHold.MeanLoad()))
	} else {
		rec = append(rec, "", "", "", "", "")
	}
	// The three chip state rules, each on its own counts. `_trig_weeks` is written
	// whenever the decision loop ran — a zero says the rule was off or never
	// eligible — and the firing triple goes blank until it fires, because a
	// gameweek of 0 is already "did not fire" and a bar of 0.0 alongside it would
	// read as a bar that was measured.
	for _, m := range []ChipTriggerMediator{r.WildcardTrig, r.BenchBoostTrig, r.FreeHitTrig} {
		if r.HasBanking && r.DecisionWeeks > 0 {
			rec = append(rec, strconv.Itoa(m.OfferedWeeks),
				strconv.Itoa(m.ConsultedWeeks), strconv.Itoa(m.WeighedWeeks))
		} else {
			rec = append(rec, "", "", "")
		}
		if r.HasBanking && m.FiredGW > 0 {
			rec = append(rec, strconv.Itoa(m.FiredGW),
				floatOrBlank(m.FiredValue), floatOrBlank(m.FiredBar))
		} else {
			rec = append(rec, "", "", "")
		}
	}
	// The chip-preparation credit, which had no mediator at all until now — so
	// every recorded preparation figure is a points column with no funnel behind
	// it, and "the credit never fired" and "it fired and bought nothing" were one
	// number.
	if r.HasBanking && r.DecisionWeeks > 0 {
		rec = append(rec, strconv.Itoa(r.ChipPrep.ConsultedWeeks))
	} else {
		rec = append(rec, "")
	}
	if r.HasBanking && r.ChipPrep.ConsultedWeeks > 0 {
		rec = append(rec, strconv.Itoa(r.ChipPrep.CreditWeeks),
			floatOrBlank(r.ChipPrep.BenchSum), floatOrBlank(r.ChipPrep.CaptainSum))
	} else {
		rec = append(rec, "", "", "")
	}
	// The dose block. Blank when the sweep did not compute it; a zero is a real
	// reading — a cell whose actionable window carries no double is exactly the
	// cell a doubles arm cannot run in, which is what this column exists to say.
	if r.HasDose {
		rec = append(rec,
			strconv.Itoa(r.ActDoubles), strconv.Itoa(r.ActBlanks),
			strconv.Itoa(r.LateDoubles), strconv.Itoa(r.LateBlanks))
	} else {
		rec = append(rec, "", "", "", "")
	}
	// Same rule again for the chips, and for the same reason: a blank says the
	// sweep did not record them, where a zero would say the chip returned
	// nothing in a week it was never played.
	if r.HasChipWeeks {
		rec = append(rec,
			strconv.Itoa(r.BenchBoostGW), strconv.Itoa(r.BenchBoostPts),
			strconv.Itoa(r.TripleCapGW), strconv.Itoa(r.TripleCapPts))
	} else {
		rec = append(rec, "", "", "", "")
	}
	// The chip-week oracle's readings, blank under the same rule again. A zero is
	// especially believable here: a bench boost worth nothing in its best week is
	// a number a reader would accept, and the block is left unmeasured on every
	// arm but the oracled one — including the baseline arm of the chip-week
	// oracle's own sweep, which is a fact about the ARM rather than the sweep.
	if r.HasChipOracle {
		rec = append(rec,
			strconv.Itoa(r.BenchBoostOracleGW), strconv.Itoa(r.BenchBoostOraclePts),
			floatOrBlank(r.BenchBoostMedianPts), strconv.Itoa(r.BenchBoostThresholdPts),
			strconv.Itoa(r.BenchBoostBarPts),
			strconv.Itoa(r.TripleCapOracleGW), strconv.Itoa(r.TripleCapOraclePts),
			floatOrBlank(r.TripleCapMedianPts), strconv.Itoa(r.TripleCapThresholdPts),
			strconv.Itoa(r.TripleCapBarPts))
	} else {
		rec = append(rec, "", "", "", "", "", "", "", "", "", "")
	}
	// The accumulated-xPoints instrument, blank under the same rule as everything
	// above it — a 0.0 season total on a metric whose whole claim is that it is a
	// *smoother* reading of the same season is the most believable wrong number in
	// this schema, and it is the variance decomposition's rows that would carry it.
	if r.HasXPoints {
		rec = append(rec,
			floatOrBlank(r.HoldXPoints), perGWf(r.HoldXPoints, r.Weeks),
			floatOrBlank(r.PolicyXPoints), perGWf(r.PolicyXPoints, r.Weeks))
	} else {
		rec = append(rec, "", "", "", "")
	}
	// The arm block. Blank setting means the arm varies no single scalar, which
	// is the fact the schedule screen needs in order to refuse a family rather
	// than infer one from labels — so it must be a gap and not a 0, which is a
	// real setting for several knobs here. The floor is always written: it is
	// resolved from the config that ran and every cell has one.
	if r.HasSetting {
		rec = append(rec, strconv.FormatFloat(r.Setting, 'g', -1, 64))
	} else {
		rec = append(rec, "")
	}
	rec = append(rec,
		strconv.FormatFloat(r.MinExpMinutes, 'g', -1, 64),
		r.SquadHash)
	// Never blank, and always last. See cellRow.Oracle.
	oracle, kind := r.stamp()
	rec = append(rec, oracle, kind)
	s.cells.write(rec)
}

// mean records one comparison's mean paired difference, for R to check itself
// against.
// The oracle stamp is the *variant's*, not the baseline's: the baseline arm of an
// oracled sweep is required to be un-oracled, so the stamp on a mean row says
// what hindsight produced the difference.
func (s *cellSink) mean(sweep, metric, variant, baseline string, vi int, m float64, n int, oracle string) {
	if s == nil {
		return
	}
	if oracle == "" {
		oracle = Oracles{}.Stamp()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.means.write([]string{
		sweep, s.runID, metric, variant, strconv.Itoa(vi), baseline,
		strconv.FormatFloat(m, 'g', -1, 64), strconv.Itoa(n), oracle,
	})
}

func (s *cellSink) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cells.close()
	s.means.close()
}
