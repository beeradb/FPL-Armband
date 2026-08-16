package present

import (
	"fmt"
	"strings"

	"armband/internal/analysis"
)

// The player row and the card behind it.
//
// # Why this file exists
//
// The squad page grew three views, and all three show players. A watchlist row, a
// research-target row and a roster row are the same object with different columns —
// so they are built by ONE function here rather than by three constructors that
// drift. This package's standing rule is that a figure is read off the engine's
// structs and never re-derived; the corollary is that there is one place doing the
// reading.
//
// Nothing here computes a football number. `corrections` selects which of the
// engine's already-computed multipliers are worth printing, and `fdrCells` copies
// fixtures across. The one piece of arithmetic is `ScorePct`, which is a bar width
// in percent — presentation, not football, and the same thing summaryBar.Height
// already does for the replay chart.

// Override is one hand-set correction, bound to the player it acts on.
//
// It is the only thing on the page that says the eleven below is not the model's
// unaided answer, which is why it is carried on the card rather than listed once at
// the top: a reader looking at Kinsky in the starting eleven has to be able to learn,
// without leaving the row, that a human put him there.
//
// Everything here is passed in by the caller. This package does not read config and
// does not decide what has lapsed — `config.Roster` already answers both, and a second
// implementation of "is this override still live" is exactly the failure this project
// has paid for most often.
type Override struct {
	// Kind is one of lock, lockXI, exclude, minutes, club. It drives nothing but
	// the sort order; the badge reads Label.
	Kind string
	// Label is the badge text and it always carries the VALUE, never just the kind.
	// "MIN 88" and "MIN 15" are opposite interventions — one is holding a backup
	// keeper in the eleven, the other is writing an injured defender down — and a
	// badge reading "MIN" tells the reader nothing about which one he is looking at.
	Label string
	// Reason is the full stored prose. It runs to a hundred-odd words in practice,
	// which is why it is never rendered in a table cell.
	Reason string
	// SetOn, Until and Checked are pre-formatted by the caller. An expiry is a guess
	// made when the injury happened and is wrong in both directions, so the dates are
	// shown rather than resolved into a verdict.
	SetOn, Until, Checked string
	// NeedsCheck marks an override due a look at this week's news. Rendered on two
	// channels — a warn border and a literal "!" — because it is the only state here
	// with an action attached.
	NeedsCheck bool
	// Lapsed marks one that has run past its gameweek. Kept on the page rather than
	// dropped: deleting it would hide that a player's treatment CHANGED, which is the
	// class of silent failure LastChecked exists to catch.
	Lapsed bool
	// Inherited marks a club-level correction riding on a player's row. It is drawn
	// dashed rather than solid so the reader can see the override is not about him.
	Inherited bool
	// Player, Team, Pos, Price and Score describe the subject, for the card lists in
	// views 2 and 3 where the override is shown away from any table row.
	Player, Team, Pos string
	Price, Score      float64
	// InSquad marks an override acting on a player who is actually in the fifteen.
	//
	// It drives the sort, and it is the distinction that matters most in the list: an
	// override holding a backup keeper in the starting eleven is doing something
	// right now, and one on a player the optimiser declined anyway is a note. Sorting
	// by kind put the first fifth of nine.
	InSquad bool
	// AffectsInSquad names the members of the fifteen a club override touches. Without
	// it a reader cannot tell whether a club correction is doing anything to THIS
	// squad, which is the only question he is asking of it.
	//
	// Empty is a real answer — "it reaches nobody you own" — and the template says so
	// rather than rendering nothing, because a silent absence is indistinguishable
	// from the page having forgotten to ask.
	AffectsInSquad []string
	// CheckAge is the age in days that the badge reports. Carried separately from the
	// formatted Checked string so the flag itself can show it: nine identical CHECK
	// badges cannot prioritise anything, which is what a flag is for, and the age is
	// the variable that actually differs between them.
	//
	// ⚠️ It is NOT "days since verified" when nobody ever verified. config.checkAge
	// falls back to SetOn, so an override set twelve days ago and never checked
	// returns 12 — indistinguishable from one checked twelve days ago, which is the
	// one distinction the badge exists to carry. NeverChecked carries it separately.
	CheckAge int
	// NeverChecked marks an override with no LastChecked at all, whose age is
	// therefore measured from when it was set rather than from any verification.
	NeverChecked bool
}

// Flag is the CHECK badge's text, with the age in it.
//
// An override nobody has ever re-read is the one most likely to be wrong, so it says
// so rather than reporting the age of the decision as though it were the age of a
// check. This rendered "CHECK 12d" beside a meta row reading "never (12d)" — two
// renderings of one fact, disagreeing on the same line.
func (o Override) Flag() string {
	if o.NeverChecked {
		return fmt.Sprintf("CHECK — never verified, set %dd ago", o.CheckAge)
	}
	return fmt.Sprintf("CHECK %dd", o.CheckAge)
}

// BadgeClass is the modifier list for the .ovb badge. Kept here rather than in the
// template so the four states are decided in one place and can be tested.
func (o Override) BadgeClass() string {
	var c []string
	if o.Inherited {
		c = append(c, "club")
	}
	if o.NeedsCheck {
		c = append(c, "check")
	}
	if o.Lapsed {
		c = append(c, "lapsed")
	}
	return strings.Join(c, " ")
}

// CardClass is the same for the .ovr card in views 2 and 3. An override binding a
// player who is actually in the fifteen is marked as well as sorted first — a reader
// scanning a column of nine cards should not have to cross-reference the eleven to
// find the one that is holding a player in it.
func (o Override) CardClass() string {
	var c []string
	switch {
	case o.Lapsed:
		c = append(c, "lapsed")
	case o.NeedsCheck:
		c = append(c, "check")
	}
	if o.InSquad && !o.Lapsed {
		c = append(c, "insquad")
	}
	return strings.Join(c, " ")
}

// fdrCell is one upcoming fixture, as drawn in the strip.
//
// One cell per FIXTURE, not per gameweek. A double gameweek is two cells, which is
// what makes the strip comparable across players — pack it by gameweek instead and a
// club playing twice in GW7 lines up against a club playing once, silently.
type fdrCell struct {
	Diff     int
	Opponent string
	Home     bool
	Event    int
}

// Where renders "BOU (H)" for the why-card's fixture list.
func (f fdrCell) Where() string {
	if f.Home {
		return "H"
	}
	return "A"
}

// correction is one scoring multiplier that is NOT neutral, with the engine's own
// explanation of it attached.
//
// # Why there is no waterfall here, and must never be one
//
// Score is not the product of these. metrics.go splits the per-90 estimate into rate
// and threshold parts, scales them by two DIFFERENT appearance probabilities, adds a
// per-gameweek defcon term, clamps a negative rate to zero, and only then multiplies
// by congestion, role and availability. A "base × a × b × c = score" line on this page
// would be a second implementation of that arithmetic living in a template, and it
// would be wrong for the first player who hits the clamp.
//
// So this list carries no equals sign and no total. It says which terms departed from
// 1.0 and why, which is the honest half of the question and the half a reader is
// actually asking.
type correction struct {
	Factor float64
	Label  string
	Note   string
}

// whyCard is everything behind a player's name.
//
// Deliberately not the whole of PlayerMetrics. The underlying per-90 figures — xG, xA,
// defcon, saves, cards, finishing — describe the PLAYER; they do not answer "why is
// this number what it is", which is what the card is for. They are one command away in
// the briefing document.
type whyCard struct {
	Score, Value               float64
	Base, SetPiece, FixtureAdj float64
	// SetPieceNote is the full prose note. It lives here rather than on the row
	// because it is long enough to truncate in a table cell and the card has room.
	SetPieceNote         string
	ExpMins, Reliability float64
	Band                 string
	Matches              int
	Absence              string
	// PriorPct is the share of this player's rates taken from the current season,
	// shown only when it is not the whole of them. Pre-season it is 100 for everyone
	// and saying so on every card is noise.
	PriorPct    int
	Corrections []correction
	Status      string
	Chance      string
	News        string
	AvgFDR      float64
	Fixtures    []fdrCell
	Override    *Override
}

// NoCorrections reports the common case, which the card states rather than leaves
// blank. An empty region under a heading reads as a rendering gap; "this is the raw
// model number" is information, and it is true of most players most weeks.
func (w whyCard) NoCorrections() bool { return len(w.Corrections) == 0 }

// NoCorrectionsLine is what to say in that case, and it depends on whether a human
// has been at the inputs.
//
// The first version said "this is the raw model number" unconditionally, and on
// Kinsky's card that sat six lines under "MIN 88 — hand-set override" and flatly
// contradicted it. It is not the raw model number: someone wrote 88 into his minutes,
// which is the only reason he is in the eleven. A multiplier override and an INPUT
// override are different things and only the first shows up in the corrections list —
// so the absence of corrections has to be reported differently when an input was set.
func (w whyCard) NoCorrectionsLine() string {
	if w.Override != nil && !w.Override.Inherited {
		return "None — but the inputs above are hand-set, not measured."
	}
	return "None — this is the raw model number."
}

// corrections picks out the multipliers that moved, in the order they are applied.
//
// The comparisons are deliberately asymmetric and each one is a claim. Congestion and
// FixtureLoad are reported in BOTH directions, because a double gameweek above 1.0 is
// as decision-changing as a blank below it. RoleFactor, RestMinutesFactor and
// availability only ever discount, so they are tested against 1 downward. The two club
// factors are zero when the feature is off, and zero is "not applied" rather than
// "multiplied by nothing" — testing them with `< 1` alone would report every player in
// the league as carrying a club correction of ×0.00.
func corrections(p analysis.PlayerMetrics, loadInScore bool) []correction {
	var out []correction
	add := func(f float64, label string, notes ...string) {
		var kept []string
		for _, n := range notes {
			if n != "" {
				kept = append(kept, n)
			}
		}
		out = append(out, correction{Factor: f, Label: label, Note: strings.Join(kept, "; ")})
	}

	if p.Congestion != 0 && p.Congestion != 1 {
		add(p.Congestion, "congestion", p.CongestionNotes...)
	}
	if p.RoleFactor != 0 && p.RoleFactor < 1 {
		add(p.RoleFactor, "role certainty", p.RoleNotes...)
	}
	if p.RestMinutesFactor != 0 && p.RestMinutesFactor < 1 {
		add(p.RestMinutesFactor, "post-tournament rest", p.RestRisk)
	}
	if p.AvailabilityFactor != 0 && p.AvailabilityFactor < 1 {
		add(p.AvailabilityFactor, "availability", availabilityNote(p))
	} else if p.AvailabilityFactor == 0 {
		// Zero is the whole explanation of the score above it, so it is the one
		// value that must never be filtered out as "empty".
		add(0, "availability", availabilityNote(p))
	}
	if p.FixtureLoad != 0 && p.FixtureLoad != 1 {
		note := "a blank inside the horizon"
		if p.FixtureLoad > 1 {
			note = "a double inside the horizon"
		}
		// Whether this one is IN the score above depends on the horizon, and the
		// engine is the only thing that knows: Score multiplies by FixtureLoad only
		// when FixtureLoadInScore() is true, which at the shipped weekly-only setting
		// means horizon 1. This page runs at the scoring horizon, which is 5.
		//
		// Listed beside congestion and role — which always apply — it told a reader
		// that a double gameweek was priced into the headline number when it was not.
		// internal/agent already exports FixtureLoadInScore() for exactly this
		// question, and carries a note beside the figure for the same reason; the card
		// was the third consumer and the only one not asking.
		if !loadInScore {
			note += " — NOT in the score above, which is a per-gameweek average"
		}
		add(p.FixtureLoad, "fixtures per gameweek", note)
	}
	if p.TeamXGCFactor != 0 && p.TeamXGCFactor != 1 {
		add(p.TeamXGCFactor, "club expected goals conceded", "hand-set correction")
	}
	if p.TeamFormFactor != 0 && p.TeamFormFactor != 1 {
		add(p.TeamFormFactor, "club form", "")
	}
	return out
}

func availabilityNote(p analysis.PlayerMetrics) string {
	s := p.Status
	if p.ChancePlay != nil {
		s = fmt.Sprintf("%s, %d%% chance of playing", s, *p.ChancePlay)
	}
	return s
}

// fdrCells copies the fixture list across, capped so one club's double-heavy run
// cannot stretch the column. The overflow count is reported rather than dropped.
const maxFDRCells = 6

func fdrCells(fs []analysis.FixtureBrief) ([]fdrCell, int) {
	var out []fdrCell
	for _, f := range fs {
		out = append(out, fdrCell{Diff: f.Difficulty, Opponent: f.Opponent,
			Home: f.Home, Event: f.Event})
	}
	if len(out) > maxFDRCells {
		return out[:maxFDRCells], len(out) - maxFDRCells
	}
	return out, 0
}

// bandClass maps a rotation band to its badge modifier, and reports whether it should
// be drawn at all.
//
// `nailed` returns false. RotationRisk is a BAND LABEL and is never empty — the
// package already learned this once, in isRisky, after the terminal pitch flagged all
// eleven starters. The HTML page then made the same mistake in a different way: it
// printed the label whenever it was non-empty, in the bad colour, so `nailed` — good
// news — was rendered in the same red as an injury on eleven of fifteen rows.
//
// So the good band is silent, and one footnote under the table says so. Silence has to
// be legible, or its absence reads as missing data.
func bandClass(band string) (string, bool) {
	switch band {
	case "likely starter":
		return "b2", true
	case "rotation risk":
		return "b3", true
	case "squad player":
		return "b4", true
	case "fringe":
		return "b5", true
	}
	return "", false
}

// WatchRow is one candidate on the watchlist: a player not in the fifteen, and the
// gap between him and the starter he would have to displace.
//
// Delta is against the WEAKEST STARTER IN HIS POSITION, not against a rank. "The
// eighth best midfielder" answers nothing; "+0.46 pts/gw on the midfielder you are
// currently starting, and £2.0m dearer" is the whole decision. The caller computes it
// because the caller is the only thing that knows both squads.
type WatchRow struct {
	Player analysis.PlayerMetrics
	Delta  float64
	// ClearsGate reports whether Delta reaches the free-transfer threshold the
	// policy actually applies.
	//
	// It exists because colouring the gap against ZERO was the page recommending in
	// colour what it refused in prose: all twelve positive candidates in the GW1 build
	// were drawn in the good colour, ten of them short of the threshold the same page
	// prints two tabs away. A positive gap is not a case for a transfer; clearing the
	// gate is, and only the gate knows which.
	//
	// ⚠️ The gate is `min_gain_for_transfer`, which ships at 0.4 — NOT
	// `free_transfer_value`, which is 2.0 and is a different constant answering a
	// different question. An earlier draft of this comment cited 2.00 and named the
	// +0.81 candidate as a false positive it had caught; at the shipped 0.4 that
	// candidate clears the gate and is correctly green.
	ClearsGate bool
}

// DeltaClass is the row's colour, and it has three states rather than two.
//
// The middle one is the point: a candidate better than your starter but short of the
// gate gets no colour at all, because the sign already carries "he is better" and
// green would add "so buy him", which the model denies.
func (r WatchRow) DeltaClass() string {
	switch {
	case r.ClearsGate:
		return "up"
	case r.Delta < 0:
		return "down"
	}
	return ""
}

// WatchGroup is one position's candidates, with the starter they are measured against
// named so the delta column has a referent.
// The benchmark's PRICE is carried as well as his score, because the delta column
// answers half the question and the reader was left doing the other half by hand: a
// candidate at £6.0m against a starter whose price is three rows up in a different
// table is a subtraction this column exists to spare him.
type WatchGroup struct {
	Position       string
	BenchmarkName  string
	BenchmarkScore float64
	BenchmarkPrice float64
	Rows           []WatchRow
}

// watchRowView and watchGroupView are the same thing after the players have been
// turned into cards. Two shapes rather than one because the exported half is what a
// caller can build — it has no business knowing about htmlCard — and the unexported
// half is what the template reads.
type watchRowView struct {
	Card  htmlCard
	Delta float64
	// DeltaClass is decided by WatchRow, not recomputed here — the "does this clear
	// the gate" question has one answer and one implementation.
	DeltaClass string
}

type watchGroupView struct {
	Position       string
	BenchmarkName  string
	BenchmarkScore float64
	BenchmarkPrice float64
	Rows           []watchRowView
}

// ResearchGroup is one category of "where the model says it is blind", carried
// verbatim from analysis.ResearchCategory.
//
// This is the best account of how the model is seeing things that exists anywhere in
// the codebase, and it is already computed for free — the deterministic layer decides
// WHO to research and states WHY it cannot see them. It reaches the page unedited;
// this package does not get to summarise the engine's own explanation of itself.
type ResearchGroup struct {
	Name, Why, Ask string
	Targets        []analysis.PlayerMetrics
}

// researchGroupView is one category after its targets have been turned into cards.
//
// The targets used to render as raw PlayerMetrics fields, which made them the one row
// type on the page not built by newCard — so they carried no override badge, no
// rotation band and no availability flag. That is not cosmetic here: ResearchTargets
// ranges the whole unfiltered pool, and its first category selects flagged defenders
// and keepers, which is precisely where an EXCLUDED player lands. An excluded defender
// appeared as an ordinary research target, unmarked, on the same view whose other half
// exists to say why he is not in the squad.
type researchGroupView struct {
	Name, Why, Ask string
	Targets        []htmlCard
}

// Policy is the gate the transfer decision is made under, so a reader can see that
// "no move" is a decision with thresholds behind it rather than an empty section.
type Policy struct {
	MinGainTransfer float64
	MinGainHit      float64
	BankUpTo        int
	MaxHitsPerWeek  int
	Rules           []string
}

// Reasoning is view 2: why this eleven and not another one.
//
// Everything in it is already computed by the engine or read from config, and none of
// it currently reaches any page — it is printed once into a Markdown briefing meant
// for a reasoning layer and then lost. The three things a human wants from that
// document are what a human OVERRODE, where the model admits it is blind, and what it
// structurally cannot see; the rest of the briefing is instructions to an agent.
type Reasoning struct {
	Overrides []Override
	Lapsed    []Override
	Research  []ResearchGroup
	// Blind is the fixed list of structural blind spots, passed in as prose.
	Blind []string
	// Breaks names the gameweeks following an international break, when there are any.
	Breaks string
	Policy Policy
}

// Due is how many live overrides are due a re-check, and Oldest names the one that
// has gone longest.
//
// Stated once at the top of the section because a flag that is on for every card
// cannot prioritise anything, which is what a flag is for. The state is real and is
// not suppressed — hiding a condition because it is universal is how a page starts
// lying by omission — but the reader is told the shape of it before he starts reading
// nine identical borders.
func (r *Reasoning) Due() int {
	n := 0
	for _, o := range r.Overrides {
		if o.NeedsCheck {
			n++
		}
	}
	return n
}

func (r *Reasoning) Oldest() string {
	best := -1
	name := ""
	for _, o := range r.Overrides {
		if o.NeedsCheck && o.CheckAge > best {
			best, name = o.CheckAge, o.Player
		}
	}
	if name == "" {
		return ""
	}
	return fmt.Sprintf("%s, %d days", name, best)
}

// Any reports whether view 2 has anything to show. A tab that opens on nothing is
// worse than no tab.
func (r *Reasoning) Any() bool {
	if r == nil {
		return false
	}
	return len(r.Overrides) > 0 || len(r.Lapsed) > 0 || len(r.Research) > 0 ||
		len(r.Blind) > 0 || len(r.Policy.Rules) > 0 || r.Policy.MinGainTransfer > 0
}

// Watchlist is view 3.
type Watchlist struct {
	Groups []WatchGroup
	// Excluded are the players a standing override keeps out of every squad. They are
	// NOT in the ranked groups: they are not candidates, they are decisions, and the
	// reason is printed in full rather than hidden behind a hover — "why is this
	// obviously good player not here" is the most important thing on this view.
	Excluded []Override
	// Count is how many ranked candidates there are, for the tab badge.
	Count int
	// Gate is the free-transfer threshold, and Clearing is how many candidates reach
	// it. Both are on the page so it can say the one thing this list is for — whether
	// any of it is actionable — instead of leaving the reader to compare a column of
	// deltas against a number printed on another tab.
	Gate     float64
	Clearing int
}

func (w *Watchlist) Any() bool {
	if w == nil {
		return false
	}
	return len(w.Groups) > 0 || len(w.Excluded) > 0
}
