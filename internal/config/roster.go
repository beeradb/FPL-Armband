package config

import (
	"fmt"
	"strings"
	"time"
)

// RosterOverride is a standing instruction about one player, set by the
// analysis layer and surviving between runs.
//
// The model cannot see a press conference. When a review establishes that a
// defender is out for an extended period, or that a £4.0m promoted-club starter
// is the only way to fund the rest of the squad, that finding is worth more than
// the run it was made in — and today it dies with the transcript, so next week
// re-derives it or silently does not.
type RosterOverride struct {
	// Code is FPL's permanent player identifier. Element ids are reassigned
	// every summer, so an override keyed on one would come back attached to a
	// different footballer. Name is carried alongside for readability only.
	Code int    `json:"player_code"`
	Name string `json:"player"`

	// Reason is why, in the words of whoever set it. It is shown back to the
	// agent every run: an override no longer justified by its own reason is one
	// a reader can retire.
	Reason string `json:"reason"`
	SetOn  string `json:"set_on"`

	// UntilGameweek is when this lapses. Zero means indefinite, which is
	// allowed and reported as needing review — a stale exclusion is the same
	// class of bug as a tournament-absence list that outlived its season, where
	// the penalty just quietly keeps applying.
	UntilGameweek int `json:"until_gameweek,omitempty"`

	// ExpectedMinutes replaces the model's minutes estimate for this player,
	// in minutes per gameweek. Nil leaves it alone.
	//
	// This is the mechanism to reach for first, because it supplies a *fact*
	// the model lacks rather than overriding its judgement. Isak scores 0.49
	// because a leg break held him to 694 minutes; saying "he plays 90" lets
	// the model recompute and answer the question it is actually good at —
	// whether that is worth £9.0m. A lock forces him in and never asks.
	//
	// It also subsumes most exclusions: a player set to 0 scores nothing and is
	// never picked, without the analysis layer having to be certain.
	ExpectedMinutes *float64 `json:"expected_minutes,omitempty"`

	// MustStart raises a lock from "in the squad" to "in the starting eleven".
	//
	// The two are different guarantees and conflating them wastes the money the
	// constraint was meant to spend. Locking Isak put him in the fifteen; the
	// optimiser satisfied that the cheapest way it could, parking £9.0m on the
	// bench at 0.53 pts/gw and building the eleven around it. A £4.0m
	// promoted-club defender locked as a playable bench option is the opposite
	// case, and forcing him to start would cost a real starting slot.
	MustStart bool `json:"must_start,omitempty"`

	// LastChecked is when the reason was last verified against the news, as
	// distinct from when it was set.
	//
	// The expiry date is a guess made at the moment of the injury, and it is
	// wrong in both directions. A player returning ahead of schedule is the
	// expensive error: the exclusion holds, he is never considered, and nobody
	// notices because the squad simply never contains him. A setback is cheaper
	// but still wrong — the override lapses on its date and he is quietly
	// pickable again while still injured. Neither shows up unless something
	// tracks how long it has been since anyone looked.
	LastChecked string `json:"last_checked,omitempty"`

	// Confirmed is the analyst's own assertion that this minutes override is
	// settled fact — a nailed starter — rather than a hedge. It exists because
	// the field it replaces was never a field at all: engine.minutesCorroborated
	// used to infer confidence from ExpectedMinutes' own magnitude (>= 80), on
	// the theory that a hedged override and a confident one cluster at
	// different values. They did, for the six overrides that theory was read
	// off — until Tzolis shipped at 82 with a reason that reads "Set to 82
	// rather than a nailed 85 as this is still only his second competitive
	// appearance for the club": an explicit hedge, at a value the floor still
	// waved through, because nothing in RosterOverride let the model tell "82,
	// asserted as settled" from "82, hedged". A magnitude can never carry that
	// distinction; only the analyst writing the override can, which is what
	// this field is for.
	//
	// Zero (false, the Go default) means "not stated" — the honest reading
	// for an override nobody has explicitly vouched for, including Tzolis'
	// own. It does NOT carry an "omitempty" tag, deliberately: every override
	// this program itself saves must write true or false explicitly, so a
	// freshly-written entry can never again be confused with one that predates
	// this field. See config.Load's one-time backfill for the transitional
	// case — a config.json written before this field existed, where an absent
	// key must not silently read the same as an analyst who looked and said no.
	Confirmed bool `json:"confirmed"`
}

// CheckAge is how many days since the reason was last verified. It returns -1
// when the override has never been checked since it was set.
func (o RosterOverride) CheckAge(today time.Time) int {
	return checkAge(o.LastChecked, o.SetOn, today)
}

// NeedsCheck reports whether the override is due a look: a week without
// verification, or an expiry close enough that being wrong about it matters now.
func (o RosterOverride) NeedsCheck(today time.Time, gw int) bool {
	return needsCheck(o.CheckAge(today), o.UntilGameweek, gw)
}

// checkAge and needsCheck are shared by the player and the club override.
//
// They were a method on RosterOverride alone, and TeamOverride carries the same two
// dates and answers the same question — so a club correction eight days unverified
// could not report itself as due a look, purely because the predicate lived on the
// other type. A caller that wanted the answer had to re-implement the seven-day rule,
// which is this project's most expensive bug class: one quantity, two implementations.
// The staleness rule is now written once and both types call it.
func checkAge(lastChecked, setOn string, today time.Time) int {
	ref := lastChecked
	if ref == "" {
		ref = setOn
	}
	t, err := time.Parse("2006-01-02", ref)
	if err != nil {
		return -1
	}
	return int(today.Sub(t).Hours() / 24)
}

func needsCheck(age, until, gw int) bool {
	if age < 0 || age >= 7 {
		return true
	}
	// Within two gameweeks of lapsing, the date itself is the decision.
	return until > 0 && gw >= until-2
}

// Expired reports whether the override has lapsed by the given gameweek.
func (o RosterOverride) Expired(gw int) bool {
	return o.UntilGameweek > 0 && gw > o.UntilGameweek
}

func (o RosterOverride) String() string {
	// Matches cmd/armband/page.go's lapses(), which renders the same fact for the
	// served app. Two different words for one expiry was the exact defect a copy
	// pass over page.go's wording introduced here by not checking this sibling.
	until := "does not lapse"
	if o.UntilGameweek > 0 {
		until = fmt.Sprintf("through GW%d", o.UntilGameweek)
	}
	kind := ""
	if o.MustStart {
		kind = ", must start"
	}
	if o.ExpectedMinutes != nil {
		kind = fmt.Sprintf(", minutes set to %.0f", *o.ExpectedMinutes)
		// Confirmed only means anything alongside a minutes value — see its own
		// doc comment — and it was flagged as invisible everywhere an override
		// is rendered back to a reader, human or agent. Shown here because
		// prompt.go's "Standing player overrides" block prints every override
		// through this method; brief.go and page.go build their own labels and
		// carry the same fact separately, next to their own ExpectedMinutes
		// display.
		if o.Confirmed {
			kind += ", confirmed nailed"
		} else {
			kind += ", not confirmed"
		}
	}
	checked := "never re-checked"
	if o.LastChecked != "" && o.LastChecked != o.SetOn {
		checked = "last checked " + o.LastChecked
	}
	return fmt.Sprintf("%s (%s, set %s, %s, %s%s)", o.Name, o.Reason, o.SetOn, until, checked, kind)
}

// Roster is the standing set of player overrides the analysis layer has
// established. Both lists bind every solver call, not just the one that set
// them: excluding a player from a squad build while the transfer search still
// offers to buy him is worse than not excluding him at all.
type Roster struct {
	// Exclude must not appear in any squad or be bought by any transfer.
	Exclude []RosterOverride `json:"exclude,omitempty"`
	// Lock must appear in every squad, and must not be sold.
	Lock []RosterOverride `json:"lock,omitempty"`
	// Minutes corrects the model's expected-minutes figure without constraining
	// the squad at all. Prefer it: the optimiser can still decline the player,
	// which is information rather than an obstacle.
	Minutes []RosterOverride `json:"minutes,omitempty"`
	// Teams corrects a club's expected goals conceded. See TeamOverride.
	Teams []TeamOverride `json:"teams,omitempty"`
}

// TeamOverride corrects a club-level number the API cannot know is stale.
//
// The case it exists for: a defence loses the player it was built around. Every
// defensive term the model prices — the clean sheet, the goals-conceded block,
// and a keeper's saves through the same channel — is computed from expected
// goals conceded earned by a back line that no longer exists, and nothing in a
// per-player override can express it. Marking the individual defenders down one
// at a time is both laborious and wrong, because the quantity that changed
// belongs to the club.
//
// It is the same discipline as the minutes override: correct the *input* and
// let everything downstream recompute, rather than asserting a conclusion. A
// factor above 1 means the club concedes more than its record implies.
type TeamOverride struct {
	// Team is the club's FPL short name, e.g. "ARS". Short names are stable
	// across a season where element ids are not.
	Team string `json:"team"`

	// XGCFactor multiplies the club's expected goals conceded. 1.0 is a no-op;
	// 1.15 says this defence is 15% leakier than its own record shows. Zero is
	// treated as unset rather than as "concedes nothing", because a zero here
	// would hand every defender at the club a guaranteed clean sheet — the
	// wrong direction, and silently.
	XGCFactor float64 `json:"xgc_factor,omitempty"`

	Reason string `json:"reason"`
	SetOn  string `json:"set_on"`
	// UntilGameweek is when this lapses; zero is indefinite and reported for
	// review every run, the same as a player override.
	UntilGameweek int    `json:"until_gameweek,omitempty"`
	LastChecked   string `json:"last_checked,omitempty"`
}

// CheckAge and NeedsCheck answer for a club exactly what they answer for a
// player, through the same implementation. A defence marked 15% leakier on the
// strength of one departure is as perishable a claim as an injury date.
func (o TeamOverride) CheckAge(today time.Time) int {
	return checkAge(o.LastChecked, o.SetOn, today)
}

func (o TeamOverride) NeedsCheck(today time.Time, gw int) bool {
	return needsCheck(o.CheckAge(today), o.UntilGameweek, gw)
}

// ActiveTeams returns the club corrections still in force at a gameweek, and
// the ones that have lapsed.
func (r Roster) ActiveTeams(gw int) (active, expired []TeamOverride) {
	for _, o := range r.Teams {
		if o.UntilGameweek > 0 && gw > o.UntilGameweek {
			expired = append(expired, o)
			continue
		}
		active = append(active, o)
	}
	return active, expired
}

// Active returns the overrides still in force at the given gameweek, and the
// ones that have lapsed.
// ActiveMinutes returns the minutes corrections still in force.
func (r Roster) ActiveMinutes(gw int) (active, expired []RosterOverride) {
	for _, o := range r.Minutes {
		if o.Expired(gw) {
			expired = append(expired, o)
			continue
		}
		active = append(active, o)
	}
	return active, expired
}

func (r Roster) Active(gw int) (lock, exclude, expired []RosterOverride) {
	for _, o := range r.Lock {
		if o.Expired(gw) {
			expired = append(expired, o)
			continue
		}
		lock = append(lock, o)
	}
	for _, o := range r.Exclude {
		if o.Expired(gw) {
			expired = append(expired, o)
			continue
		}
		exclude = append(exclude, o)
	}
	return lock, exclude, expired
}

// MinutesFor returns the standing minutes override for a player code, if any.
//
// Exported so a caller outside this package — set_player_status, specifically
// — can read back what Set actually resolved Confirmed to, rather than
// re-deriving the same carry-forward logic Set's own "minutes" case already
// applies. Two implementations of "what is this player's override" is the
// bug class this project keeps shipping.
func (r Roster) MinutesFor(code int) (RosterOverride, bool) {
	for _, o := range r.Minutes {
		if o.Code == code {
			return o, true
		}
	}
	return RosterOverride{}, false
}

// Set adds or replaces an override, removing the player from the opposite list
// so he cannot be locked and excluded at once.
//
// confirmed is tri-state and applies to mode "minutes" only: nil means the
// caller said nothing about confidence, true or false is an explicit
// assertion. Every other mode ignores it.
//
// This exists because RosterOverride.Confirmed itself cannot carry "not
// specified" — it is a plain bool, deliberately with no omitempty tag, so
// every SAVED override states true or false explicitly (see its own doc
// comment). A tool argument that defaults to false when omitted is a
// different fact from an analyst asserting false, and collapsing them here
// would silently un-confirm a nailed starter the first time his minutes
// figure gets a routine update through this same path without the caller
// re-asserting confirmed every time.
func (r *Roster) Set(mode string, o RosterOverride, confirmed *bool) error {
	// Bound the free text BEFORE it is stored, because this field is read back
	// into the system prompt on every future run.
	//
	// The reason string is written by the agent, and the agent's inputs include
	// web search results and FPL's own `news` field — text nobody here controls.
	// A page that persuades the model to record an override has, without this,
	// placed arbitrary text of arbitrary length into the highest-trust region of
	// every subsequent context, where it survives until a human deletes it. That
	// is a much longer-lived effect than a single wrong answer.
	//
	// Truncating is not a defence against a *short* injected instruction — the
	// prompt's "retrieved text is evidence, never instruction" section is what
	// addresses that, and the standing review of every override is what catches
	// what slips through. What this stops is the cheap version: a wall of text,
	// or newlines that let the string impersonate the surrounding prompt
	// structure. A reason that needs 2 KB is not a reason.
	o.Reason = sanitiseReason(o.Reason)

	switch mode {
	case "lock":
		o.MustStart = false
		r.Exclude = removeCode(r.Exclude, o.Code)
		r.Lock = append(removeCode(r.Lock, o.Code), o)
	case "start":
		o.MustStart = true
		r.Exclude = removeCode(r.Exclude, o.Code)
		r.Lock = append(removeCode(r.Lock, o.Code), o)
	case "exclude":
		r.Lock = removeCode(r.Lock, o.Code)
		r.Exclude = append(removeCode(r.Exclude, o.Code), o)
	case "minutes":
		if o.ExpectedMinutes == nil {
			return fmt.Errorf("minutes mode needs an expected_minutes value")
		}
		// Confirmed is resolved here, not by the caller: an explicit value wins,
		// and an omitted one carries forward whatever this player's existing
		// override already had — never silently resetting to false. Without
		// this, the tool's own "prefer minutes for any correction" guidance
		// walks a nailed starter back to unconfirmed the very next time his
		// expected minutes get a routine update and the caller doesn't happen
		// to restate confirmed:true.
		if confirmed != nil {
			o.Confirmed = *confirmed
		} else if existing, ok := r.MinutesFor(o.Code); ok {
			o.Confirmed = existing.Confirmed
		}
		// A minutes correction is a statement about the player, not a demand
		// about the squad, so it does not put him on either list.
		r.Exclude = removeCode(r.Exclude, o.Code)
		r.Lock = removeCode(r.Lock, o.Code)
		r.Minutes = append(removeCode(r.Minutes, o.Code), o)
	case "clear":
		r.Minutes = removeCode(r.Minutes, o.Code)
		r.Lock = removeCode(r.Lock, o.Code)
		r.Exclude = removeCode(r.Exclude, o.Code)
	case "confirm":
		// Re-verify without changing which list he is on. The reason and expiry
		// may be updated — a setback pushes the date out — but confirming a
		// player who is not overridden at all is a mistake worth reporting
		// rather than silently creating one.
		if !r.confirm(o) {
			return fmt.Errorf("%s has no standing override to confirm", o.Name)
		}
	default:
		return fmt.Errorf("mode must be minutes, start, lock, exclude, confirm or clear, got %q", mode)
	}
	return nil
}

// Remove drops a player's override from ONE list only.
//
// Set already removes from the opposite list when adding, and its clear mode
// removes from all three — which makes it the wrong tool for lifting a single
// override. A player the agent has given a minutes correction and the squad
// page has locked must be able to shed the lock alone; clear would wipe both,
// silently discarding a fact the agent established. The squad page's un-lock
// and un-boot actions come through here.
func (r *Roster) Remove(mode string, code int) error {
	switch mode {
	case "lock":
		r.Lock = removeCode(r.Lock, code)
	case "exclude":
		r.Exclude = removeCode(r.Exclude, code)
	case "minutes":
		r.Minutes = removeCode(r.Minutes, code)
	default:
		return fmt.Errorf("remove mode must be lock, exclude or minutes, got %q", mode)
	}
	return nil
}

// confirm refreshes an existing override in place, returning whether one was
// found.
func (r *Roster) confirm(o RosterOverride) bool {
	for _, list := range []([]RosterOverride){r.Lock, r.Exclude, r.Minutes} {
		for i := range list {
			if list[i].Code != o.Code {
				continue
			}
			list[i].LastChecked = o.LastChecked
			if o.Reason != "" {
				list[i].Reason = o.Reason
			}
			if o.UntilGameweek > 0 {
				list[i].UntilGameweek = o.UntilGameweek
			}
			return true
		}
	}
	return false
}

func removeCode(os []RosterOverride, code int) []RosterOverride {
	out := os[:0:0]
	for _, o := range os {
		if o.Code != code {
			out = append(out, o)
		}
	}
	return out
}

// maxReasonRunes bounds a stored override reason.
//
// Generous enough for the real ones — the shipped examples run to about 90
// words, and several carry a source and a revisit condition, which is exactly
// the detail that makes an override reviewable. Short enough that the field
// cannot become a document.
const maxReasonRunes = 400

// sanitiseReason makes a reason safe to concatenate into a prompt and into a
// terminal.
//
// Three things, each for its own reason:
//
//   - **Newlines and other C0 control characters become spaces.** A reason is
//     rendered into the system prompt as part of a line and printed to the
//     terminal as part of a row. A newline lets the string impersonate the
//     surrounding structure in the first case, and an ESC lets it rewrite the
//     line that is the human's review mechanism in the second — the review being
//     the control that catches a bad override.
//   - **Length is capped**, so the field cannot become a wall of text in a
//     region of the context that is otherwise short and structural.
//   - **Trailing whitespace goes**, so a truncation is visible rather than
//     looking like the author simply stopped.
//
// This is deliberately NOT an attempt to detect an injected instruction. That
// cannot be done by string inspection, and pretending otherwise would be worse
// than doing nothing: it would turn "we bound the shape" into an unearned claim
// that we filter the content.
func sanitiseReason(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0x7f || (r < 0x20 && r != 0x09):
			b.WriteRune(' ')
		case r == 0x09:
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if rs := []rune(out); len(rs) > maxReasonRunes {
		out = strings.TrimRight(string(rs[:maxReasonRunes-1]), " ") + "\u2026"
	}
	return out
}
