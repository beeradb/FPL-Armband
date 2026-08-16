package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ChipSchedule is every chip a season grants: two sets of four from 2025-26.
//
// # Why a second set needs a type rather than a second field
//
// FPL's 2025-26 rule change expires the first set of chips after GW19 and issues
// a fresh set from GW20, so "bench boost GW14 **and** GW34" is a legal plan and
// `ChipPlan` — one gameweek per chip — cannot say it. Three layers had already
// grown their own answer to that: `backtest.SimConfig` carries `Chips` beside
// `Chips2`, `chipPlanFromEnv` parses a `2` suffix into whichever of the pair it
// picked, and the live agent carried no answer at all and silently planned one
// set. That is this record's signature failure — one quantity, several
// implementations — arriving for the fifth time, so the representation lives
// here and the other layers read it.
//
// # Slots are named because an index is not checkable
//
// The eight chips are addressed as `wc1`, `wc2`, `fh1`, `fh2`, `bb1`, `bb2`,
// `tc1`, `tc2`. A name that does not resolve is an **error** everywhere it is
// accepted, never a skip: a mistyped chip that quietly does nothing returns a
// season indistinguishable from one that planned no chip, which is the
// byte-identical null this project keeps being caught by. `chipPlanFromEnv` had
// already learned that lesson for the environment variable; this puts the same
// rule under every caller.
type ChipSchedule struct {
	// First expires at the GW19 deadline in a two-set season, and runs the whole
	// season in a one-set one. Second is empty except in a season that grants it.
	//
	// Which seasons those are is deliberately NOT decided here: it is a fact
	// about the season being played, and `backtest.ChipSetsFor` owns it, gated
	// per season for the same reason `BankLimitFor` is. A schedule is a
	// statement of intent, and validating it against the calendar is a separate
	// job from being able to express it.
	First  ChipPlan `json:"first"`
	Second ChipPlan `json:"second"`
}

// chipKind is one of the four chips, independent of which set holds it. The
// names are FPL's own, as they arrive in the bootstrap and as `planned` and
// `chipLabels` already spell them, so this adds no fifth vocabulary.
type chipKind string

const (
	kindWildcard      chipKind = "wildcard"
	kindFreeHit       chipKind = "freehit"
	kindBenchBoost    chipKind = "bboost"
	kindTripleCaptain chipKind = "3xc"
)

// chipKinds is the four chips in the order a season plays them out — the order
// used for every rendering and iteration here, so two callers never disagree
// about what "the first chip" means.
var chipKinds = []chipKind{kindWildcard, kindFreeHit, kindBenchBoost, kindTripleCaptain}

// slotAliases maps every spelling accepted for a chip kind onto that kind.
//
// The long forms are here because `FPL_CHIP_PLAN` already accepted them and
// breaking a documented environment variable to tidy a table is not worth it.
var slotAliases = map[string]chipKind{
	"wc": kindWildcard, "wildcard": kindWildcard,
	"fh": kindFreeHit, "freehit": kindFreeHit, "free_hit": kindFreeHit,
	"bb": kindBenchBoost, "bboost": kindBenchBoost, "bench_boost": kindBenchBoost,
	"tc": kindTripleCaptain, "3xc": kindTripleCaptain,
	"triple_captain": kindTripleCaptain,
}

// shortName is the canonical two-letter form each kind is printed as.
var shortName = map[chipKind]string{
	kindWildcard: "wc", kindFreeHit: "fh",
	kindBenchBoost: "bb", kindTripleCaptain: "tc",
}

// The slot names, as constants, because three of this type's methods cannot
// report a bad one.
//
// `Set`, `Get`, `SetAll` and `ParseChipSchedule` return an error on a name that
// does not resolve. `Plays`, `Next` and `Weeks` cannot — they return bool, int
// and []int — so they fall back to the nothing-planned answer, which is
// indistinguishable from a chip that is genuinely unplanned. Every caller of
// those three is inside this repository passing a literal, so the fix is to
// remove the literals: a typo in `SlotFreeHit` does not compile, where a typo in
// `"fh"` is the byte-identical null this type exists to abolish, arriving inside
// the code written to fix it. Found by review of that code.
//
// A bare name means the first set; both sets are consulted by the readers.
const (
	SlotWildcard      = "wc"
	SlotFreeHit       = "fh"
	SlotBenchBoost    = "bb"
	SlotTripleCaptain = "tc"
)

// ChipSlots lists the eight slot names in canonical order: the four chips of the
// first set, then the four of the second. Returned rather than exported as a
// variable so a caller cannot reorder the package's own ordering.
func ChipSlots() []string {
	out := make([]string, 0, 8)
	for _, set := range []int{1, 2} {
		for _, k := range chipKinds {
			out = append(out, fmt.Sprintf("%s%d", shortName[k], set))
		}
	}
	return out
}

// parseSlot splits a slot name into its chip and its set.
//
// A bare name means the FIRST set — `wc` is `wc1` — because that is what
// `FPL_CHIP_PLAN` has always meant and a plan written before the second set
// existed must keep meaning what it meant.
func parseSlot(name string) (chipKind, int, error) {
	k, set, _, err := parseSlotSet(name)
	return k, set, err
}

// parseSlotSet is parseSlot plus whether the name carried a set suffix at all.
//
// The writers need only the resolved set, since a bare name means set 1. The
// readers need the distinction: bare means "either set", which is not the same
// as "set 1" and is the whole reason `Weeks("fh")` finds two free hits.
func parseSlotSet(name string) (kind chipKind, set int, bare bool, err error) {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, "-", "_")
	if s == "" {
		return "", 0, false, fmt.Errorf("empty chip slot")
	}
	set = 1
	// "3xc" ends in a letter, so only a trailing 1 or 2 is a set marker and only
	// when something precedes it. Trimming any trailing digit would turn "3xc2"
	// into a lookup for "3xc" in set 2, which is right, and "3xc" into "3x" in
	// set c, which is why the digit set is closed rather than "any digit".
	if len(s) > 1 && (strings.HasSuffix(s, "1") || strings.HasSuffix(s, "2")) {
		if k, ok := slotAliases[s[:len(s)-1]]; ok {
			set, _ = strconv.Atoi(s[len(s)-1:])
			return k, set, false, nil
		}
	}
	k, ok := slotAliases[s]
	if !ok {
		return "", 0, false, fmt.Errorf("unknown chip slot %q (want one of %s)",
			name, strings.Join(ChipSlots(), ", "))
	}
	return k, set, true, nil
}

// plan returns the set a slot belongs to, so the accessors below do not each
// repeat the same two-way branch.
func (s *ChipSchedule) plan(set int) *ChipPlan {
	if set == 2 {
		return &s.Second
	}
	return &s.First
}

// field returns a pointer to the gameweek a slot names, for reading and writing.
func (s *ChipSchedule) field(k chipKind, set int) *int {
	p := s.plan(set)
	switch k {
	case kindWildcard:
		return &p.Wildcard
	case kindFreeHit:
		return &p.FreeHit
	case kindBenchBoost:
		return &p.BenchBoost
	case kindTripleCaptain:
		return &p.TripleCaptain
	}
	return nil
}

// Set places a chip, by slot name. A gameweek of 0 unplans it.
//
// Rejecting an out-of-range gameweek here rather than at validation time is
// deliberate: `ValidateChipPlan` reports problems for a human to read and the
// caller carries on, whereas a gameweek of 47 is not a plan anyone can act on
// and every layer that accepts one would have to re-check it.
func (s *ChipSchedule) Set(slot string, gw int) error {
	k, set, err := parseSlot(slot)
	if err != nil {
		return err
	}
	if gw < 0 || gw > 38 {
		return fmt.Errorf("chip slot %s: gameweek %d is not in 1..38 (0 unplans it)", slot, gw)
	}
	*s.field(k, set) = gw
	return nil
}

// Get returns the gameweek a slot is planned for, 0 meaning unplanned.
func (s ChipSchedule) Get(slot string) (int, error) {
	k, set, err := parseSlot(slot)
	if err != nil {
		return 0, err
	}
	return *s.field(k, set), nil
}

// SetAll applies a whole schedule at once, e.g. from a config block or a form.
//
// All-or-nothing: an unknown or out-of-range entry leaves the schedule
// untouched rather than half-applied, because a partly-applied plan is a plan
// nobody wrote and it would be played without complaint.
func (s *ChipSchedule) SetAll(m map[string]int) error {
	next := *s
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic error for a map with two bad entries
	for _, k := range keys {
		if err := next.Set(k, m[k]); err != nil {
			return err
		}
	}
	*s = next
	return nil
}

// All returns every planned slot as canonical-name to gameweek, skipping
// unplanned ones. The inverse of SetAll.
func (s ChipSchedule) All() map[string]int {
	out := map[string]int{}
	for _, slot := range ChipSlots() {
		if gw, err := s.Get(slot); err == nil && gw > 0 {
			out[slot] = gw
		}
	}
	return out
}

// Plays reports whether either set plays this chip in this gameweek.
//
// Zero never matches, since gameweeks start at 1 — an unplanned chip is
// therefore not played, with no separate check needed.
// A BARE name asks across both sets; a suffixed one asks about that set alone.
// `Set` and `Get` already honour the suffix, and a reader that parsed it and
// then ignored it would make `Plays("wc2", gw)` read as "the second wildcard"
// and mean "either wildcard" — a wrong answer that looks like the right one.
func (s ChipSchedule) Plays(slot string, gw int) bool {
	k, set, bare, err := parseSlotSet(slot)
	if err != nil || gw <= 0 {
		return false
	}
	sc := s
	for _, n := range setsToRead(set, bare) {
		if *sc.field(k, n) == gw {
			return true
		}
	}
	return false
}

// setsToRead is which sets a reader consults: both for a bare name, the named
// one for a suffixed name. One definition, so the three readers cannot drift.
func setsToRead(set int, bare bool) []int {
	if bare {
		return []int{1, 2}
	}
	return []int{set}
}

// Next is the earliest gameweek at or after `from` that plays this chip, or 0.
//
// The horizon logic needs "the next one", not "the one": with two sets there can
// be a wildcard behind the decision and another ahead of it, and asking for a
// single field returns whichever set happens to hold it. That is how a two-set
// plan silently reverts to single-set behaviour — the code still compiles, still
// runs, and prepares for the wrong chip.
func (s ChipSchedule) Next(slot string, from int) int {
	k, set, bare, err := parseSlotSet(slot)
	if err != nil {
		return 0
	}
	sc := s
	best := 0
	for _, n := range setsToRead(set, bare) {
		if w := *sc.field(k, n); w >= from && w > 0 && (best == 0 || w < best) {
			best = w
		}
	}
	return best
}

// Weeks lists every gameweek this chip is planned for, ascending. Used where a
// single "next" is not enough — excluding *all* free-hit weeks from scoring, for
// instance, rather than only the nearest one.
func (s ChipSchedule) Weeks(slot string) []int {
	k, set, bare, err := parseSlotSet(slot)
	if err != nil {
		return nil
	}
	sc := s
	var out []int
	for _, n := range setsToRead(set, bare) {
		if w := *sc.field(k, n); w > 0 {
			out = append(out, w)
		}
	}
	sort.Ints(out)
	return out
}

// From drops every chip planned before `gw`, leaving the ones a decision made in
// that gameweek still has ahead of it.
//
// A decision does not care about a wildcard it already played, and handing it one
// is how a March squad "prepares" for September's rebuild. The caller could
// instead rely on the engine's own idea of the upcoming gameweek, and the replay
// deliberately does not: it decides for a gameweek it names, and reading the
// bootstrap's next event would couple that to point-in-time reconstruction.
//
// Both sets survive the filter. Collapsing to a single next-chip-per-kind is what
// made a second free hit invisible to SetSkipGameweeks.
func (s ChipSchedule) From(gw int) ChipSchedule {
	out := s
	for _, set := range []int{1, 2} {
		for _, k := range chipKinds {
			if f := out.field(k, set); *f < gw {
				*f = 0
			}
		}
	}
	return out
}

// ChipEntry is one planned chip, ready to print.
type ChipEntry struct {
	Slot  string `json:"slot"`  // canonical name, e.g. "bb2"
	Label string `json:"label"` // human form, e.g. "Bench Boost (second set)"
	Set   int    `json:"set"`
	GW    int    `json:"gameweek"`
}

// Entries lists every planned chip in canonical order, labelled for display.
//
// Every command that shows a plan built its own name-to-gameweek map, and each
// would have needed the same second-set branch. One list, so they cannot drift
// into disagreeing about what a second-set chip is called.
func (s ChipSchedule) Entries() []ChipEntry {
	var out []ChipEntry
	sc := s
	for _, set := range []int{1, 2} {
		for _, k := range chipKinds {
			gw := *sc.field(k, set)
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
			out = append(out, ChipEntry{
				Slot:  fmt.Sprintf("%s%d", shortName[k], set),
				Label: label, Set: set, GW: gw,
			})
		}
	}
	return out
}

// WeekIn is the gameweek this chip is planned for inside a given window, or 0.
//
// Display code walks the windows FPL publishes and asks what is planned in each.
// Asking by set index instead would need every caller to know which set a window
// is, which is the bookkeeping this hides — and getting it wrong shows a
// second-set chip against the first set's row.
func (s ChipSchedule) WeekIn(name string, start, stop int) int {
	k, _, err := parseSlot(name)
	if err != nil {
		return 0
	}
	sc := s
	for _, set := range []int{1, 2} {
		if w := *sc.field(k, set); w >= start && w <= stop {
			return w
		}
	}
	return 0
}

// String renders a schedule as the same syntax ParseChipSchedule accepts, e.g.
// "wc1=4,bb1=14,wc2=28". Round-trips, which is what makes it safe to log.
func (s ChipSchedule) String() string {
	var parts []string
	for _, slot := range ChipSlots() {
		if gw, err := s.Get(slot); err == nil && gw > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", slot, gw))
		}
	}
	return strings.Join(parts, ",")
}

// ParseChipSchedule reads "wc1=4,bb2=34" into a schedule.
//
// Unrecognised names are an error rather than a skip, for the reason the type
// comment gives: a mistyped chip that silently does nothing is indistinguishable
// from a season that planned none.
func ParseChipSchedule(spec string) (ChipSchedule, error) {
	var s ChipSchedule
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, val, ok := strings.Cut(part, "=")
		if !ok {
			return ChipSchedule{}, fmt.Errorf("chip plan: %q is not name=gameweek", part)
		}
		// 1..38, so zero is refused here even though `Set` accepts it as "unplan
		// this". A spec is a statement of what to play, and `wc=0` in one is a
		// typo rather than an instruction to leave the wildcard alone — accepting
		// it returns a season with one fewer chip than the author asked for and
		// nothing saying so.
		gw, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil || gw < 1 || gw > 38 {
			return ChipSchedule{}, fmt.Errorf("chip plan: %q needs a gameweek in 1..38", part)
		}
		if err := s.Set(name, gw); err != nil {
			return ChipSchedule{}, fmt.Errorf("chip plan: %w", err)
		}
	}
	return s, nil
}

// UnmarshalJSON accepts the two-set form and the flat single-set form that
// config.json carried before the second set existed.
//
// This is the backfill the project's config rule demands: an existing
// `"chip_plan": {"wildcard_gameweek": 5, ...}` must keep loading, and it means
// the first set, because that is the only set the season it was written for
// granted. Without this every saved config becomes an error on upgrade.
func (s *ChipSchedule) UnmarshalJSON(b []byte) error {
	// A JSON null must leave the receiver alone rather than zero it. That is the
	// json.Unmarshaler convention, and the difference is invisible today only
	// because the zero schedule is also the default — it becomes a silent loss of
	// a non-zero default the day one ships.
	if string(bytes.TrimSpace(b)) == "null" {
		return nil
	}
	// The two forms are distinguished by key, not by shape: both are objects,
	// and a flat plan unmarshals into the two-set struct perfectly happily as
	// all-zero, which would silently discard a real plan.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(b, &probe); err != nil {
		return err
	}
	_, hasFirst := probe["first"]
	_, hasSecond := probe["second"]

	var next ChipSchedule
	if hasFirst || hasSecond {
		// Refused rather than merged. A mixed object is not a plan anybody wrote
		// on purpose, and taking the two-set branch silently discards the flat
		// siblings — a chip that vanishes between what the file says and what the
		// season plays.
		for _, legacy := range []string{
			"wildcard_gameweek", "free_hit_gameweek",
			"bench_boost_gameweek", "triple_captain_gameweek",
		} {
			if _, mixed := probe[legacy]; mixed {
				return fmt.Errorf("chip_plan mixes the flat form (%s) with the two-set "+
					"form (first/second); use one or the other", legacy)
			}
		}
		var two struct {
			First  ChipPlan `json:"first"`
			Second ChipPlan `json:"second"`
		}
		if err := json.Unmarshal(b, &two); err != nil {
			return err
		}
		next = ChipSchedule{First: two.First, Second: two.Second}
	} else {
		var flat ChipPlan
		if err := json.Unmarshal(b, &flat); err != nil {
			return err
		}
		next = ChipSchedule{First: flat}
	}

	// The same range invariant Set and ParseChipSchedule enforce. Without it a
	// hand-edited config was the one way into the type that skipped the check,
	// and an out-of-range week is not something a later caller can distinguish
	// from a deliberate one.
	for _, set := range []int{1, 2} {
		for _, k := range chipKinds {
			if gw := *next.field(k, set); gw < 0 || gw > 38 {
				return fmt.Errorf("chip_plan: %s%d is GW%d, which is not in 1..38 "+
					"(0 means unplanned)", shortName[k], set, gw)
			}
		}
	}
	*s = next
	return nil
}

// MarshalJSON writes the flat single-set form when there is no second set.
//
// Otherwise the first save after upgrading rewrites every existing config.json
// into the two-set form — an unexplained diff on a tracked file, and a one-way
// door: a consumer still typed `ChipPlan` reads the two-set object as all zeros
// **with no error**, losing the whole plan. That is the byte-identical null this
// type's own comment is about, so it would be a poor thing to introduce here.
//
// A schedule that genuinely uses the second set is written in the new form and
// an older binary cannot read it — which is correct, because an older binary
// cannot represent it either, and failing to express half the plan is the thing
// worth being loud about.
func (s ChipSchedule) MarshalJSON() ([]byte, error) {
	if s.Second == (ChipPlan{}) {
		return json.Marshal(s.First)
	}
	// A distinct type, or this recurses into itself. The tags come with it.
	type twoSet ChipSchedule
	return json.Marshal(twoSet(s))
}
