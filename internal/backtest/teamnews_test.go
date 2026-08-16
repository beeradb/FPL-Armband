package backtest

// Regression tests for the recovered-team-news oracle.
//
// Every one of these covers a failure this project has already shipped in some
// other form: an oracle wired and inert, a join on the identifier that gets
// reassigned every summer, a repair that silently applies nothing, and a switch
// that becomes the default by accident.

import (
	"testing"

	"armband/internal/fpl"
)

// stubNews is a hand-written source, keyed the way the real one is.
//
// A pointer type, deliberately: Oracles is compared with != and a map-backed value
// would panic there. See TestTeamNewsSourceIsComparable.
type stubNews struct {
	covered map[int]bool
	flags   map[int]stubFlag
}

type stubFlag struct {
	status string
	chance *int
}

func (s *stubNews) Covers(_ string, gw int) bool { return s.covered[gw] }

func (s *stubNews) FlagAt(_ string, gw, code int) (string, *int, bool) {
	if !s.covered[gw] {
		return "", nil, false
	}
	if f, ok := s.flags[code]; ok {
		return f.status, f.chance, true
	}
	// A covered gameweek is authoritative: a player FPL was not flagging is a player
	// who is available, not a player nothing is known about. See the TeamNews doc.
	return "a", nil, true
}

func intp(v int) *int { return &v }

// TestTeamNewsIsOffUnlessAsked pins the default, in code rather than in prose.
//
// Every figure in AGENTS.md and the research record was measured against statusAt's
// end-of-season reconstruction. An oracle that reached the default view would inflate
// all of them at once and make the record incomparable with itself — which is the
// stated reason FPL_ORACLE_AVAILABILITY must never become the default, and it applies
// with more force here because this oracle moves far more players.
//
// Not DIAG-gated and it must not become so: a guard that runs only when somebody
// remembers to ask for it is not a guard.
func TestTeamNewsIsOffUnlessAsked(t *testing.T) {
	news := &stubNews{
		covered: map[int]bool{5: true},
		flags:   map[int]stubFlag{99: {status: "i", chance: intp(0)}},
	}
	el := fpl.Element{ID: 1, Code: 99, Status: "a"}

	// No bit set, source attached: a source with nothing asking for it is inert.
	applyTeamNews(&el, "2023-24", 5, Oracles{News: news})
	if el.Status != "a" || el.ChanceOfPlayingNextRound != nil {
		t.Errorf("a source with no oracle bit reached the model: status %q chance %v",
			el.Status, el.ChanceOfPlayingNextRound)
	}

	// The zero Oracles is what every ordinary caller passes, so this is literally
	// the default path rather than a reconstruction of it.
	applyTeamNews(&el, "2023-24", 5, Oracles{})
	if el.Status != "a" || el.ChanceOfPlayingNextRound != nil {
		t.Errorf("the zero Oracles changed availability: status %q chance %v",
			el.Status, el.ChanceOfPlayingNextRound)
	}
}

// TestTeamNewsReplacesTheReconstructionInBothDirections is the property that makes
// this a data change rather than a patch.
//
// statusAt carries a final status of "u" or "i" back to the moment its news was
// posted, which is right for a departure and wrong for an injury that resolved. On a
// covered gameweek the recovered payload is authoritative in **both** directions: it
// must be able to say "flagged" where the reconstruction said available, and
// "available" where the reconstruction said gone. Only overriding in the pessimistic
// direction would leave two availability models inside one arm, with the
// reconstruction quietly winning exactly the cases it gets wrong.
func TestTeamNewsReplacesTheReconstructionInBothDirections(t *testing.T) {
	news := &stubNews{
		covered: map[int]bool{5: true},
		flags: map[int]stubFlag{
			// FPL was flagging him at this deadline; the reconstruction cannot know,
			// because he finished the season fit.
			11: {status: "i", chance: intp(0)},
		},
	}
	o := Oracles{Info: OracleTeamNews, News: news}

	resolved := fpl.Element{Code: 11, Status: "a"}
	applyTeamNews(&resolved, "2023-24", 5, o)
	if resolved.Status != "i" {
		t.Errorf("an injury that resolved by May reads %q at the deadline it was "+
			"live, want %q — the oracle is not seeing the population it exists for",
			resolved.Status, "i")
	}

	// The reverse: statusAt carried a season-ending flag back over a stretch the
	// player was in fact available for.
	fit := fpl.Element{Code: 12, Status: "u"}
	applyTeamNews(&fit, "2023-24", 5, o)
	if fit.Status != "a" {
		t.Errorf("a player FPL was not flagging reads %q, want %q — an unflagged "+
			"player in a covered payload is available, and falling back to the "+
			"reconstruction here would let it win the cases it gets wrong", fit.Status, "a")
	}
}

// TestTeamNewsFallsBackWhereThereIsNoCapture is the other half of that contract.
//
// Coverage is genuinely patchy — the Internet Archive crawled FPL about two days in
// three in the earliest backfilled season — and a consumer that treated an absent
// gameweek as "nobody was injured" would produce clean-looking figures from no data
// at all. That is this data's recorded failure mode, twice over: a repair that
// silently applies nothing.
func TestTeamNewsFallsBackWhereThereIsNoCapture(t *testing.T) {
	news := &stubNews{
		covered: map[int]bool{5: true},
		flags:   map[int]stubFlag{11: {status: "i"}},
	}
	el := fpl.Element{Code: 11, Status: "u"}
	applyTeamNews(&el, "2023-24", 9, Oracles{Info: OracleTeamNews, News: news})
	if el.Status != "u" {
		t.Errorf("an uncovered gameweek reads %q, want the reconstruction's %q — "+
			"inventing availability for a gap is how a repair reports a clean null "+
			"from no data", el.Status, "u")
	}
}

// TestTeamNewsJoinsOnCodeNotElementID pins the trap this project has already paid
// for once, in the standing overrides.
//
// FPL reassigns element ids every summer, so a record keyed on one comes back next
// season attached to a different footballer. The two identifiers are constructed here
// to cross over, so a join on the wrong one does not merely fail — it silently flags
// the wrong player, which is what makes it worth a test rather than a comment.
func TestTeamNewsJoinsOnCodeNotElementID(t *testing.T) {
	news := &stubNews{
		covered: map[int]bool{5: true},
		flags:   map[int]stubFlag{200: {status: "i"}},
	}
	o := Oracles{Info: OracleTeamNews, News: news}

	// Injured this season: code 200, but this season his element id is 100.
	injured := fpl.Element{ID: 100, Code: 200, Status: "a"}
	// Fit: his element id happens to be 200, the other man's permanent code.
	fit := fpl.Element{ID: 200, Code: 300, Status: "a"}

	applyTeamNews(&injured, "2023-24", 5, o)
	applyTeamNews(&fit, "2023-24", 5, o)

	if injured.Status != "i" {
		t.Errorf("the flagged player reads %q: the join is not finding him by code",
			injured.Status)
	}
	if fit.Status != "a" {
		t.Errorf("a fit player whose element id equals another man's permanent code "+
			"reads %q — the join is on the identifier FPL reassigns every summer",
			fit.Status)
	}
}

// TestAPlayerWithNoPermanentKeyIsLeftAlone mirrors the rule the capture store keeps
// on the producing side.
//
// A player with no code cannot be joined to anything. Looking him up finds no flag,
// and because a covered gameweek is authoritative that would mark him **available** —
// a decision about a footballer's fitness taken from the absence of an identifier.
func TestAPlayerWithNoPermanentKeyIsLeftAlone(t *testing.T) {
	news := &stubNews{covered: map[int]bool{5: true}}
	el := fpl.Element{ID: 7, Code: 0, Status: "u"}
	applyTeamNews(&el, "2023-24", 5, Oracles{Info: OracleTeamNews, News: news})
	if el.Status != "u" {
		t.Errorf("a player with no permanent code reads %q, want the reconstruction's "+
			"%q — his availability was decided by his not having an identifier",
			el.Status, "u")
	}
}

// TestOnlyTheChanceArmSetsThePercentage keeps the decomposition honest.
//
// The two arms exist to be subtracted from each other. If the flag arm also carried
// the percentage, the contrast would be zero by construction and the sweep would
// report the granularity as worthless without ever having varied it.
func TestOnlyTheChanceArmSetsThePercentage(t *testing.T) {
	news := &stubNews{
		covered: map[int]bool{5: true},
		flags:   map[int]stubFlag{11: {status: "d", chance: intp(75)}},
	}

	flagOnly := fpl.Element{Code: 11, Status: "a"}
	applyTeamNews(&flagOnly, "2023-24", 5, Oracles{Info: OracleTeamNews, News: news})
	if flagOnly.Status != "d" {
		t.Errorf("flag arm status %q, want %q", flagOnly.Status, "d")
	}
	if flagOnly.ChanceOfPlayingNextRound != nil {
		t.Errorf("the flag-only arm set the percentage to %v — the two arms would "+
			"then differ by nothing and the contrast would be zero by construction",
			*flagOnly.ChanceOfPlayingNextRound)
	}

	both := fpl.Element{Code: 11, Status: "a"}
	applyTeamNews(&both, "2023-24", 5,
		Oracles{Info: OracleTeamNews | OracleTeamNewsChance, News: news})
	if both.ChanceOfPlayingNextRound == nil || *both.ChanceOfPlayingNextRound != 75 {
		t.Errorf("the percentage arm did not deliver the published figure: %v",
			both.ChanceOfPlayingNextRound)
	}
	// And it must be a real percentage rather than a status re-encoded, or the
	// second arm is the first one wearing a different label. availabilityFactor
	// prices "d" at 0.5 and this at 0.75.
	if got := float64(*both.ChanceOfPlayingNextRound) / 100; got == 0.5 {
		t.Error("the published chance came back at exactly the flag's own 0.5, so " +
			"this fixture cannot distinguish the two arms")
	}
}

// TestTeamNewsRefusesWhatItCannotMeasure covers the three Validate rules, each of
// which prevents a figure that would look exactly like a result.
func TestTeamNewsRefusesWhatItCannotMeasure(t *testing.T) {
	// A bit with no data behind it: the silent-null direction, and the one this
	// package keeps paying for. Refused before a sweep runs rather than after.
	if err := (Oracles{Info: OracleTeamNews}).Validate(); err == nil {
		t.Error("a team-news oracle with no source validates, so it would replay " +
			"the reconstruction for hours and report it as recovered team news")
	}
	news := &stubNews{covered: map[int]bool{}}
	if err := (Oracles{Info: OracleTeamNews, News: news}).Validate(); err != nil {
		t.Errorf("a sourced team-news oracle is refused: %v", err)
	}
	// The percentage without the flag underneath: availabilityFactor prefers the
	// percentage, so this would price flagged players on recovered data and everyone
	// else on the reconstruction.
	if err := (Oracles{Info: OracleTeamNewsChance, News: news}).Validate(); err == nil {
		t.Error("a chance-only arm validates, and it mixes two availability models " +
			"inside one figure")
	}
	if err := (Oracles{Info: OracleTeamNews | OracleTeamNewsChance, News: news}).
		Validate(); err != nil {
		t.Errorf("the composed decomposition arm is refused: %v", err)
	}
	// Two resolutions of one fact on one seam, exactly as minutes and lineups are.
	if err := (Oracles{Info: OracleTeamNews | OracleAvailability, News: news}).
		Validate(); err == nil {
		t.Error("teamnews composed with availability validates — both rewrite " +
			"Element.Status, so one silently wins and the stamp names a " +
			"decomposition that did not run")
	}
	// And a source attached with no bit is inert rather than an error: that is what
	// a sweep's baseline arm looks like when the harness hands every arm the same
	// config, and refusing it would make the baseline unbuildable.
	if err := (Oracles{News: news}).Validate(); err != nil {
		t.Errorf("an unused source is refused: %v", err)
	}
}

// TestTeamNewsSourceIsComparable pins the one way this design can fail at runtime
// rather than at a check.
//
// runPolicySweep compares two Oracles with != for every cell, to prove an arm's
// hindsight does not vary by cell. Interface comparison panics when the dynamic type
// is not comparable, so a map- or slice-backed source would take down a sweep hours
// in — and the fix looks like a harness bug rather than a source that broke a
// documented contract.
func TestTeamNewsSourceIsComparable(t *testing.T) {
	news := &stubNews{covered: map[int]bool{}}
	a := Oracles{Info: OracleTeamNews, News: news}
	b := Oracles{Info: OracleTeamNews, News: news}
	if a != b {
		t.Error("two Oracles sharing one source compare unequal, so every cell of " +
			"a sweep would fail the per-cell hindsight check")
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("comparing Oracles panicked (%v) — the source's dynamic type "+
				"is not comparable, which is a contract TeamNews states", r)
		}
	}()
	_ = a != Oracles{Info: OracleTeamNews, News: &stubNews{}}
}

// TestTeamNewsStampsItselfOnEveryArm keeps the provenance honest.
//
// The stamp is the join key between a cells file and a declaration, and an oracle
// figure is more dangerous than an ordinary one in that role: it is a hindsight upper
// bound that looks exactly like a score.
func TestTeamNewsStampsItselfOnEveryArm(t *testing.T) {
	news := &stubNews{covered: map[int]bool{}}
	if got := (Oracles{Info: OracleTeamNews, News: news}).Stamp(); got != "info:teamnews" {
		t.Errorf("flag arm stamps %q", got)
	}
	got := Oracles{Info: OracleTeamNews | OracleTeamNewsChance, News: news}.Stamp()
	if got != "info:teamnews+teamnews_chance" {
		t.Errorf("percentage arm stamps %q, and the two arms must be distinguishable "+
			"in a cells file by their stamp alone", got)
	}
}
