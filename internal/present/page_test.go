package present

import (
	"fmt"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// Regression tests for the three-view squad page.
//
// The first two pin bugs this package SHIPPED, both of the same shape: a template
// printing a field whenever it was non-empty, where the field's neutral value is not
// empty. That shape is invisible in review — the markup reads correctly — and visible
// only in a render, which is why the tests below assert on rendered output rather than
// on the helper functions underneath.

// briefedPage renders the sample squad through the full three-view path.
func briefedPage(t *testing.T, mut func(*Page)) string {
	t.Helper()
	sq := sampleSquad()
	// Every sample player — bench included — is nailed and has fixtures, which is
	// the state both shipped bugs mis-rendered. The bench matters: it is four more
	// rows of the same table, and the first version of this helper seeded only the
	// eleven and duly reported four false blanks.
	seed := func(ps []analysis.PlayerMetrics) {
		for i := range ps {
			ps[i].RotationRisk = "nailed"
			ps[i].Fixtures = []analysis.FixtureBrief{
				{Event: 1, Opponent: "BOU", Home: true, Difficulty: 2},
				{Event: 2, Opponent: "MCI", Home: false, Difficulty: 5},
			}
			ps[i].Congestion = 1
			ps[i].RoleFactor = 1
			ps[i].FixtureLoad = 1
			ps[i].AvailabilityFactor = 1
		}
	}
	seed(sq.StartingXI)
	seed(sq.Bench)
	sq.Players = append(append([]analysis.PlayerMetrics{}, sq.StartingXI...), sq.Bench...)

	p := Page{Title: "T", Squad: sq, Horizon: 5}
	if mut != nil {
		mut(&p)
	}
	var b strings.Builder
	if err := Render(&b, p); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return b.String()
}

// TestTheRosterNeverLabelsANailedStarter pins the second occurrence of this
// package's oldest bug.
//
// analysis.rotationLabel returns a band for EVERY player alive, so "nailed" is a value
// and not an absence. The terminal pitch flagged all eleven starters in red until
// isRisky was written; the HTML page then did it again in a different renderer —
// `.risk` in var(--bad), printed whenever RotationRisk was non-empty, so eleven of
// fifteen rows said "nailed" in the same colour as an injury. Good news in the warning
// colour trains a reader to ignore the warning.
// The assertion is on the BADGE, not on the word. The why-card states the band in
// full — that is the card's job, it is explaining the number — and a test that
// forbade the string outright would forbid the explanation along with the noise.
func TestTheRosterNeverLabelsANailedStarter(t *testing.T) {
	out := briefedPage(t, nil)
	for _, badge := range []string{`class="band b1">nailed`, `class="band ">nailed`} {
		if strings.Contains(out, badge) {
			t.Errorf("a nailed starter carries %s; the good band must be silent, or "+
				"the badge fires on nearly every row and means nothing", badge)
		}
	}
	if n := strings.Count(out, `class="band`); n != 0 {
		t.Errorf("a squad of fifteen nailed players rendered %d rotation badges", n)
	}
	if strings.Contains(out, `class="risk"`) {
		t.Error("the .risk span is back — it printed the rotation band unconditionally " +
			"in the bad colour, which is what made every nailed starter look flagged")
	}
}

// TestTheLockAndBootFormsRenderOnlyWhenServed pins the boundary between the two
// forms of the page. The static export and the replay must carry no write
// forms at all — a lock button on a file:// page posts to nothing — and the
// served page's forms must carry the PERMANENT player code, never the
// season-scoped element id, which an override keyed on would come back next
// August attached to a different footballer.
func TestTheLockAndBootFormsRenderOnlyWhenServed(t *testing.T) {
	// The static page: no forms anywhere. The CSS class is shared by every
	// page, so the assertion is on the rendered elements, not the stylesheet.
	out := briefedPage(t, nil)
	if strings.Contains(out, "action=\"/action\"") {
		t.Error("the static page renders write forms; its buttons would post to nothing")
	}
	if strings.Contains(out, `td class="c-act"`) {
		t.Error("the static page renders the action column; it is served-page furniture")
	}

	// The served page: an action column, one boot form per row, the lock form
	// flipped to unlock where a lock override binds the player.
	sq := sampleSquad()
	locked := sq.StartingXI[0]
	out = briefedPage(t, func(p *Page) {
		p.Token = "tok"
		p.Codes = map[int]int{sq.StartingXI[0].ID: 987654, sq.Bench[0].ID: 111}
		p.Overrides = map[int]Override{locked.ID: {Kind: "lock", Label: "LOCK"}}
	})
	if n := strings.Count(out, `name="a" value="boot"`); n != 15 {
		t.Errorf("the served page renders %d boot forms for a fifteen-man squad, want 15", n)
	}
	if n := strings.Count(out, `name="a" value="lock"`); n != 14 {
		t.Errorf("the served page renders %d lock forms beside one locked player, want 14", n)
	}
	if !strings.Contains(out, `name="a" value="unlock"`) {
		t.Error("the locked player's form still offers lock; his lock icon must flip to unlock")
	}
	// The form posts the permanent code, not the element id.
	if !strings.Contains(out, `name="c" value="987654"`) {
		t.Error("the lock form does not carry the permanent player code")
	}
	if strings.Contains(out, fmt.Sprintf(`name="c" value="%d"`, locked.ID)) {
		t.Error("the lock form carries the element id, which FPL reassigns every summer")
	}
	// The CSRF token rides in every form.
	if n := strings.Count(out, `name="t" value="tok"`); n != 30 {
		t.Errorf("the token appears in %d forms, want one per form (30)", n)
	}
}

// TestARiskyBandIsStillLabelled is the other half: silencing the default must not
// silence the four bands that are real warnings.
func TestARiskyBandIsStillLabelled(t *testing.T) {
	for _, band := range []string{"likely starter", "rotation risk", "squad player", "fringe"} {
		out := briefedPage(t, func(p *Page) {
			p.Squad.StartingXI[0].RotationRisk = band
		})
		if !strings.Contains(out, band) {
			t.Errorf("%q is a real warning and did not reach the page", band)
		}
	}
	if _, show := bandClass("nailed"); show {
		t.Error("nailed is good news and must not be badged")
	}
}

// TestTheRosterDoesNotClaimEveryoneBlanks pins the loudest bug in the previous page.
//
// htmlPlayer never set Opponent, so the squad table's `{{if .Opponent}}` always fell
// to its else branch and printed the word "blank" in var(--bad) on all fifteen rows.
// The page told the reader that every player he owned had no fixture. Only the week
// tabs ever filled that field.
//
// The column is now a fixture strip built from PlayerMetrics.Fixtures, which the
// engine produces per player — so the test is that a player WITH fixtures does not
// render as having none.
func TestTheRosterDoesNotClaimEveryoneBlanks(t *testing.T) {
	out := briefedPage(t, nil)
	if n := strings.Count(out, "no fixture"); n > 0 {
		t.Errorf("%d players are shown as having no fixture, but every sample player "+
			"has two — this is the shape of the bug where the column was never filled", n)
	}
	// The fixtures that do exist must actually be drawn.
	if !strings.Contains(out, `class="fdr"`) {
		t.Error("no fixture strip was rendered at all")
	}
}

// TestABlankIsStillShown is the mirror. Silencing the false positive must not silence
// a real blank, which is the one case in the column that changes a decision.
func TestABlankIsStillShown(t *testing.T) {
	out := briefedPage(t, func(p *Page) {
		p.Squad.StartingXI[0].Fixtures = nil
	})
	if !strings.Contains(out, "no fixture") {
		t.Error("a player with no fixtures in the horizon is not marked as blanking")
	}
}

// TestTheOverrideBadgeCarriesItsValue pins the distinction the badge exists for.
//
// "MIN 88" holds a backup keeper in the starting eleven; "MIN 15" writes an injured
// defender down. They are opposite interventions, and a badge reading "MIN" cannot
// tell a reader which one is acting on the row he is looking at.
func TestTheOverrideBadgeCarriesItsValue(t *testing.T) {
	out := briefedPage(t, func(p *Page) {
		p.Overrides = map[int]Override{
			1: {Kind: "minutes", Label: "MIN 88", Reason: "named first choice",
				SetOn: "2026-08-07", Until: "after GW6"},
		}
		p.Reasoning = &Reasoning{Overrides: []Override{{
			Kind: "minutes", Label: "MIN 88", Player: "Raya", Reason: "named first choice",
		}}}
	})
	if !strings.Contains(out, "MIN 88") {
		t.Error("the override badge lost its value")
	}
	if !strings.Contains(out, "named first choice") {
		t.Error("the override's reason never reaches the page")
	}
}

// TestAnOverrideReasonIsAlsoPrintedUnhidden is the accessibility rule the design
// rests on, as an assertion.
//
// The why-card is a hover object. Hover does not exist on touch and a `title=` is not
// keyboard-reachable, so the standing rule is that the card is never the only home for
// prose: every reason it shows is also printed, unhidden, in the "why" view. This test
// fails if someone later drops the second copy as duplication — it is not duplication,
// it is the non-hover path.
func TestAnOverrideReasonIsAlsoPrintedUnhidden(t *testing.T) {
	const reason = "Saliba has no return date and Arsenal are shopping for a centre-back"
	out := briefedPage(t, func(p *Page) {
		p.Overrides = map[int]Override{1: {Kind: "exclude", Label: "EXCL", Reason: reason}}
		p.Reasoning = &Reasoning{Overrides: []Override{
			{Kind: "exclude", Label: "EXCL", Player: "Saliba", Reason: reason},
		}}
	})
	// Once inside the hover card, once inside the standing-overrides view.
	if n := strings.Count(out, reason); n < 2 {
		t.Errorf("the reason appears %d time(s); it must appear in the why-card AND "+
			"unhidden in the reasoning view, because hover is not reachable on touch", n)
	}
}

// TestTheWhyCardStatesWhenNothingWasCorrected.
//
// "No corrections" is the common case and it is information, not an absence. An empty
// region under a heading reads as a rendering gap — the reader cannot tell whether the
// model applied nothing or whether the page failed to say what it applied.
func TestTheWhyCardStatesWhenNothingWasCorrected(t *testing.T) {
	out := briefedPage(t, nil)
	if !strings.Contains(out, "this is the raw model number") {
		t.Error("a player with every multiplier at 1.0 shows an empty corrections " +
			"block; that state has to be stated")
	}
}

// TestAZeroedAvailabilityIsReportedNotDropped.
//
// availabilityFactor returns 0 for a ruled-out player, and 0 is the whole explanation
// of his score. Any filter of the form "skip this term when it is empty" drops exactly
// the case that matters and keeps the uninformative 1.0s — which is why
// PlayerMetrics.AvailabilityFactor is deliberately not `omitempty` and why corrections
// tests it explicitly rather than by truthiness.
func TestAZeroedAvailabilityIsReportedNotDropped(t *testing.T) {
	p := analysis.PlayerMetrics{
		Congestion: 1, RoleFactor: 1, FixtureLoad: 1,
		AvailabilityFactor: 0, Status: "injured",
	}
	got := corrections(p, false)
	if len(got) != 1 || got[0].Label != "availability" {
		t.Fatalf("a ruled-out player reported %d corrections (%+v); the zero must "+
			"survive, it is the reason his score is zero", len(got), got)
	}
	if got[0].Factor != 0 {
		t.Errorf("availability factor reported as %v, want 0", got[0].Factor)
	}
}

// TestANeutralMultiplierIsNotListed is the other side: the list is "departures from
// 1.0", so a page that lists every term at 1.00 is back to being a dump.
func TestANeutralMultiplierIsNotListed(t *testing.T) {
	p := analysis.PlayerMetrics{
		Congestion: 1, RoleFactor: 1, FixtureLoad: 1, AvailabilityFactor: 1,
		// Both club factors zero, which is "feature off" rather than "multiplied by
		// nothing". Reading a zero here as a correction would report every player in
		// the league as carrying a club correction of x0.00.
		TeamXGCFactor: 0, TeamFormFactor: 0,
	}
	if got := corrections(p, false); len(got) != 0 {
		t.Errorf("a player with nothing corrected reported %+v", got)
	}
}

// TestTheThreeViewPageIsStillSelfContained.
//
// The whole premise of writing this file to disk is that it opens offline from a
// file:// URL. Three views, a hover card and a sticky tab bar are all the sort of thing
// that invites a script tag or a web font.
func TestTheThreeViewPageIsStillSelfContained(t *testing.T) {
	out := briefedPage(t, func(p *Page) {
		// A URL in prose is the realistic case, and must NOT trip the guard: the live
		// config's override reasons cite news sites by name, and FPL's own news field
		// can carry one.
		p.Reasoning = &Reasoning{
			Blind:     []string{"press conferences — see https://example.com/news"},
			Overrides: []Override{{Label: "EXCL", Player: "X", Reason: "per https://example.com/x"}},
		}
		p.Watch = &Watchlist{
			Benchmarks: []WatchBenchmark{{Position: "MID", Name: "Saka", Score: 6.9}},
			Rows:       []WatchRow{{Player: sampleSquad().StartingXI[0], Delta: -2.8}},
		}
	})
	// Asserted on emitted MARKUP, not on the presence of a URL anywhere in the byte
	// stream.
	//
	// The page now renders FPL's injury news and human-written override reasons, and
	// both routinely contain a URL as prose — the live config cites ESPN and WhoScored
	// by name. A substring scan for "https://" fails on those and calls it "the page
	// reaches outside itself", which is false: the URL is escaped text inside a span.
	// That matters because the obvious response to a red test whose premise is wrong
	// is to weaken the assertion, and this is the only guard between the page and an
	// external fetch.
	for _, bad := range []string{`src=`, `href="http`, `url(`, "@import", "<script", "<iframe"} {
		if strings.Contains(out, bad) {
			t.Errorf("the page reaches outside itself (%q); it must render offline "+
				"from a file:// URL", bad)
		}
	}
	for _, want := range []string{`id="view-team"`, `id="view-why"`, `id="view-watch"`} {
		if !strings.Contains(out, want) {
			t.Errorf("%s is missing", want)
		}
	}
}

// TestTheWatchlistColoursAgainstTheGateAndNotAgainstZero pins the worst bug of the
// first build of this page.
//
// Every positive delta was drawn in the good colour, so twelve candidates were shown
// as upgrades on a page that states, two tabs away, that a free transfer needs 2.00
// pts/gw — and the best of the twelve was +0.81. The page recommended in colour what
// it refused in prose.
//
// "Better than what you have" and "worth a transfer" are different claims and only the
// gate can make the second. The middle state — positive but short of the gate — must
// therefore carry no colour at all, because the sign already says "better" and green
// would add "so buy him".
func TestTheWatchlistColoursAgainstTheGateAndNotAgainstZero(t *testing.T) {
	cases := []struct {
		name       string
		delta      float64
		clears     bool
		wantClass  string
		wantColour string
	}{
		{"clears the gate", 2.4, true, "up", "good"},
		{"better but short of the gate", 0.81, false, "", "none"},
		{"worse than your starter", -0.4, false, "down", "muted"},
	}
	for _, c := range cases {
		r := WatchRow{Delta: c.delta, ClearsGate: c.clears}
		if got := r.DeltaClass(); got != c.wantClass {
			t.Errorf("%s (Δ%+.2f): class %q, want %q — %s", c.name, c.delta,
				got, c.wantClass, c.wantColour)
		}
	}
}

// TestTheEnhancementScriptRendersOnlyWhenServed. The progressive-enhancement
// script rides on the served page alone: the static export must stay
// script-free — TestTheThreeViewPageIsStillSelfContained guards the offline
// promise — and the served page's rows must carry the permanent-code keys the
// script reconciles on.
func TestTheEnhancementScriptRendersOnlyWhenServed(t *testing.T) {
	static := briefedPage(t, func(p *Page) {
		p.Watch = &Watchlist{Rows: []WatchRow{{Player: analysis.PlayerMetrics{
			ID: 7, Name: "X", Team: "AAA", Position: "MID", Score: 5, Price: 6}}}}
	})
	if strings.Contains(static, "<script") {
		t.Error("the static export carries the enhancement script; it must stay script-free")
	}
	if strings.Contains(static, "data-pid") {
		t.Error("the static export carries row keys it has no script to use")
	}

	served := briefedPage(t, func(p *Page) {
		p.Token = "tok"
		p.Codes = map[int]int{p.Squad.StartingXI[0].ID: 987654, p.Squad.Bench[0].ID: 111}
	})
	for _, want := range []string{"fetch(\"/action\"", "data-pid=\"987654\"",
		`"slide-in"`, `"slide-out"`} {
		if !strings.Contains(served, want) {
			t.Errorf("the served page lacks %q", want)
		}
	}
}

// TestTheSessionModePageDisablesConfigSourcedControls. On the session-mode
// page, an override that lives in config.json renders its control disabled —
// the page cannot clear what it does not own — while a session-sourced one
// stays live.
func TestTheSessionModePageDisablesConfigSourcedControls(t *testing.T) {
	sq := sampleSquad()
	out := briefedPage(t, func(p *Page) {
		p.Token = "tok"
		p.SessionMode = true
		p.Codes = map[int]int{sq.StartingXI[0].ID: 987654}
		p.Overrides = map[int]Override{
			sq.StartingXI[0].ID: {Kind: "lock", Label: "LOCK", Code: 987654},
		}
	})
	if !strings.Contains(out, `class="actbtn on" type="button" disabled`) {
		t.Error("a config-sourced lock renders a live unlock button on the " +
			"session-mode page; it cannot clear what it does not own")
	}
	if !strings.Contains(out, "Restart serve with -persist") {
		t.Error("the disabled lock does not explain why")
	}

	out = briefedPage(t, func(p *Page) {
		p.Token = "tok"
		p.SessionMode = true
		p.Codes = map[int]int{sq.StartingXI[0].ID: 987654}
		p.Overrides = map[int]Override{
			sq.StartingXI[0].ID: {Kind: "lock", Label: "LOCK", Code: 987654, Session: true},
		}
	})
	if strings.Contains(out, `type="button" disabled`) {
		t.Error("a session-sourced lock renders disabled; the page owns it and " +
			"must be able to clear it")
	}
	if !strings.Contains(out, `name="a" value="unlock"`) {
		t.Error("a session-sourced lock does not offer unlock")
	}

	// The excluded card: config-sourced exclusions carry a note, session ones
	// the bring-back button.
	out = briefedPage(t, func(p *Page) {
		p.Token = "tok"
		p.SessionMode = true
		p.Watch = &Watchlist{
			Excluded: []Override{{Kind: "exclude", Label: "EXCL", Player: "X",
				Code: 987654, Reason: "r"}},
			Rows: []WatchRow{{Player: analysis.PlayerMetrics{
				ID: 7, Name: "Y", Team: "AAA", Position: "MID", Score: 5, Price: 6}}},
		}
	})
	if !strings.Contains(out, "restart serve with -persist") {
		t.Error("a config-sourced exclusion carries no explanation on the " +
			"session-mode page")
	}
	out = briefedPage(t, func(p *Page) {
		p.Token = "tok"
		p.SessionMode = true
		p.Watch = &Watchlist{
			Excluded: []Override{{Kind: "exclude", Label: "EXCL", Player: "X",
				Code: 987654, Reason: "r", Session: true}},
			Rows: []WatchRow{{Player: analysis.PlayerMetrics{
				ID: 7, Name: "Y", Team: "AAA", Position: "MID", Score: 5, Price: 6}}},
		}
	})
	if !strings.Contains(out, `name="a" value="unboot"`) {
		t.Error("a session-sourced exclusion does not offer the bring-back button")
	}
}

// TestTheWatchlistControlsRenderOnlyWhenServed. The served watchlist carries a
// filter form, sort links and a pager over 50-row pages; the static export
// shows the same rows sorted the same default way, with no controls — a sort
// link on a file:// page points at a state that page cannot render.
func TestTheWatchlistControlsRenderOnlyWhenServed(t *testing.T) {
	static := briefedPage(t, func(p *Page) {
		p.Watch = &Watchlist{Rows: []WatchRow{{Player: analysis.PlayerMetrics{
			ID: 7, Name: "X", Team: "AAA", Position: "MID", Score: 5, Price: 6}}}}
	})
	// The CSS classes ship with every page; the assertion is on the rendered
	// elements, not the stylesheet.
	if strings.Contains(static, `<form class="wfilter"`) ||
		strings.Contains(static, `<a class="sort`) ||
		strings.Contains(static, `<nav class="pager"`) {
		t.Error("the static export renders watchlist controls that cannot work offline")
	}

	rows := make([]WatchRow, 60)
	for i := range rows {
		rows[i] = WatchRow{Player: analysis.PlayerMetrics{
			ID: i + 1, Name: fmt.Sprintf("P%02d", i), Team: "AAA", Position: "MID",
			Score: 5, Price: float64(60 - i)}, Delta: 1}
	}
	served := briefedPage(t, func(p *Page) {
		p.Token = "tok"
		p.Watch = &Watchlist{Rows: rows}
		p.WatchQuery = DefaultWatchQuery()
		p.Teams = []string{"AAA", "BBB"}
	})
	for _, want := range []string{
		`<form class="wfilter" method="get" action="/">`,
		`name="pos"`, `name="team"`, `name="q"`,
		// The active column's link flips the direction.
		`href="?dir=asc&sort=price&v=watch"`,
		`<a class="sort on" href="?dir=asc&sort=price&v=watch">&pound; ↓</a>`,
		"1–50 of 60", "page 1 of 2", "next ›",
	} {
		if !strings.Contains(served, want) {
			t.Errorf("the served watchlist lacks %q", want)
		}
	}
	// The position column: one row per candidate, position visible.
	if !strings.Contains(served, `<td class="pos">MID</td>`) {
		t.Error("the watchlist has no position column")
	}
}

// TestTheWatchlistSaysWhenNothingClearsTheGate. The count is the most useful sentence
// the view can carry, and it is the one a reader would otherwise have to derive by
// comparing a column against a number on another tab.
func TestTheWatchlistSaysWhenNothingClearsTheGate(t *testing.T) {
	out := briefedPage(t, func(p *Page) {
		p.Watch = &Watchlist{
			Gate: 2.0, Count: 2, Clearing: 0,
			Benchmarks: []WatchBenchmark{{Position: "MID", Name: "Saka",
				Score: 6.9, Price: 10.1}},
			Rows: []WatchRow{{Player: analysis.PlayerMetrics{Name: "X"}, Delta: 0.81}},
		}
	})
	if !strings.Contains(out, "Nothing on this list does") {
		t.Error("the watchlist does not say that no candidate clears the gate; " +
			"without it a column of positive deltas reads as a list of upgrades")
	}
	if !strings.Contains(out, "£10.1m") {
		t.Error("the benchmark's price is missing from the group header, so the " +
			"reader must do the subtraction the delta column exists to spare him")
	}
}

// TestAHandSetPlayerIsNotCalledARawModelNumber.
//
// The card said "None — this is the raw model number" whenever no MULTIPLIER had
// moved, which on Kinsky sat six lines under "MIN 88 — hand-set override" and
// contradicted it. An input override and a multiplier override are different things
// and only the second shows in the corrections list, so the empty-list message has to
// know which it is looking at.
func TestAHandSetPlayerIsNotCalledARawModelNumber(t *testing.T) {
	hand := whyCard{Override: &Override{Kind: "minutes", Label: "MIN 88"}}
	if got := hand.NoCorrectionsLine(); strings.Contains(got, "raw model number") {
		t.Errorf("a player whose minutes were hand-set is described as %q", got)
	}
	plain := whyCard{}
	if got := plain.NoCorrectionsLine(); !strings.Contains(got, "raw model number") {
		t.Errorf("an untouched player is described as %q", got)
	}
	// An inherited club correction is itself a correction and appears in the list, so
	// it must not take the hand-set wording.
	club := whyCard{Override: &Override{Kind: "club", Inherited: true}}
	if got := club.NoCorrectionsLine(); !strings.Contains(got, "raw model number") {
		t.Errorf("an inherited club override took the hand-set input wording: %q", got)
	}
}

// TestAClubOverrideThatTouchesNobodySaysSo. Empty is a real answer, and rendering
// nothing makes "it reaches nobody you own" indistinguishable from "the page forgot".
func TestAClubOverrideThatTouchesNobodySaysSo(t *testing.T) {
	out := briefedPage(t, func(p *Page) {
		p.Reasoning = &Reasoning{Overrides: []Override{{
			Kind: "club", Label: "ARS ×1.15", Player: "Arsenal — club correction",
			Reason: "lost the player the back line was built around", Inherited: true,
		}}}
	})
	if !strings.Contains(out, "it moves the watchlist only") {
		t.Error("a club override touching nobody in the squad renders no line at all")
	}
}

// TestADoubleIsNotClaimedToBeInTheScoreWhenItIsNot.
//
// FixtureLoad is the one reported multiplier whose presence in Score depends on the
// horizon: metrics.go applies it only when FixtureLoadInScore(), which at the shipped
// weekly-only setting means horizon 1, and this page runs at the scoring horizon.
//
// Listed beside congestion and role — which always apply — a "×2.00 fixtures per
// gameweek" line told the reader a double gameweek was priced into the headline
// number when it was not. internal/agent exports FixtureLoadInScore() precisely so a
// consumer cannot assume the condition; the card was the third consumer and the only
// one that did not ask.
func TestADoubleIsNotClaimedToBeInTheScoreWhenItIsNot(t *testing.T) {
	doubled := analysis.PlayerMetrics{
		Congestion: 1, RoleFactor: 1, AvailabilityFactor: 1, FixtureLoad: 2,
	}
	out := corrections(doubled, false)
	if len(out) != 1 {
		t.Fatalf("a doubling club reported %d corrections, want 1: %+v", len(out), out)
	}
	if !strings.Contains(out[0].Note, "NOT in the score") {
		t.Errorf("at a horizon where the engine does not apply FixtureLoad, the card "+
			"says %q — a reader takes that as the double being priced in", out[0].Note)
	}
	// And the mirror: at horizon 1 it IS applied, and saying otherwise would be the
	// same error in the opposite direction.
	in := corrections(doubled, true)
	if len(in) != 1 {
		t.Fatalf("want 1 correction, got %d", len(in))
	}
	if strings.Contains(in[0].Note, "NOT in the score") {
		t.Error("at horizon 1 the engine does apply FixtureLoad; the card must not " +
			"disclaim it")
	}
}

// TestResearchTargetsCarryTheirOverrideBadge.
//
// ResearchTargets ranges the whole unfiltered pool, and its first category selects
// flagged defenders and keepers — which is exactly where an EXCLUDED player lands.
// The research rows were the one row type not built by newCard, so an excluded player
// appeared there unmarked, on the same view whose other half exists to say why he is
// not in the squad.
func TestResearchTargetsCarryTheirOverrideBadge(t *testing.T) {
	out := briefedPage(t, func(p *Page) {
		p.Overrides = map[int]Override{
			99: {Kind: "exclude", Label: "EXCL", Reason: "out until at least GW12"},
		}
		p.Reasoning = &Reasoning{Research: []ResearchGroup{{
			Name: "Defences whose numbers no longer describe the side",
			Why:  "the model cannot see a back line that has changed",
			Ask:  "who is actually playing there now",
			Targets: []analysis.PlayerMetrics{
				{ID: 99, Name: "Saliba", Team: "ARS", Position: "DEF", Price: 6.0},
			},
		}}}
	})
	if !strings.Contains(out, "Saliba") {
		t.Fatal("the research target never reached the page")
	}
	if !strings.Contains(out, `class="ovb`) {
		t.Error("an excluded player appears as an ordinary research target with no " +
			"override badge — the one view that exists to explain his absence")
	}
}

// TestEveryCheckFlagCarriesItsAge.
//
// A flag that is on for every card cannot prioritise anything, which is what a flag is
// for — nine identical CHECK badges are wallpaper. The age is the variable that
// differs, so it goes in the badge.
//
// The test exists because the badge is rendered in two places, the reasoning view and
// the excluded block, and the second one kept a hard-coded "CHECK" through an edit
// that updated the first. Two copies of one label is the same failure this codebase
// keeps paying for, one layer down.
func TestEveryCheckFlagCarriesItsAge(t *testing.T) {
	stale := Override{
		Kind: "exclude", Label: "EXCL", Player: "Saliba", Reason: "out indefinitely",
		NeedsCheck: true, CheckAge: 10,
	}
	out := briefedPage(t, func(p *Page) {
		p.Reasoning = &Reasoning{Overrides: []Override{stale}}
		p.Watch = &Watchlist{Excluded: []Override{stale}}
	})
	if strings.Contains(out, `class="flag">CHECK<`) {
		t.Error("a CHECK flag rendered without its age; the age is the only thing " +
			"that distinguishes one overdue override from another")
	}
	if n := strings.Count(out, "CHECK 10d"); n != 2 {
		t.Errorf("the aged flag appears %d time(s), want 2 — the reasoning view and "+
			"the excluded block must render the same badge", n)
	}
	// "Never verified" is carried by its own field, not by a sentinel age.
	//
	// config.checkAge falls back to SetOn when LastChecked is empty, so an override
	// set twelve days ago and never re-read returns 12 — the same number as one
	// verified twelve days ago, which is exactly the distinction the badge exists to
	// draw. It reported "CHECK 12d" beside a meta row reading "never (12d)": two
	// renderings of one fact, disagreeing on the same line.
	never := Override{NeedsCheck: true, CheckAge: 12, NeverChecked: true}.Flag()
	if !strings.Contains(never, "never") {
		t.Errorf("an override nobody has ever verified reports %q, which is "+
			"indistinguishable from one verified 12 days ago", never)
	}
	if !strings.Contains(never, "12") {
		t.Errorf("%q drops the age; how long the unverified decision has stood is "+
			"the thing that ranks it against the others", never)
	}
}

// TestASingleViewPageHasNoTabBar.
//
// The replay renders one view. A tab bar with one tab is furniture pretending to be a
// control, and it would suggest there are other views to look for.
func TestASingleViewPageHasNoTabBar(t *testing.T) {
	out := briefedPage(t, nil)
	if strings.Contains(out, `class="viewtabs"`) {
		t.Error("a page with only the squad view rendered a tab bar")
	}
	if !strings.Contains(out, `id="view-team"`) {
		t.Error("the squad view is missing entirely")
	}
}

// TestEveryNameIsEscapedInEveryView. The security check, extended: the reasoning and
// watchlist views render FPL-supplied names through paths the original test never
// reached, and an override reason is free text a human typed.
func TestEveryNameIsEscapedInEveryView(t *testing.T) {
	const evil = `<script>alert(1)</script>`
	out := briefedPage(t, func(p *Page) {
		p.Squad.StartingXI[0].Name = evil
		p.Overrides = map[int]Override{1: {Kind: "exclude", Label: "EXCL", Reason: evil}}
		p.Reasoning = &Reasoning{
			Overrides: []Override{{Label: "EXCL", Player: evil, Reason: evil}},
			Research: []ResearchGroup{{Name: evil, Why: evil, Ask: evil,
				Targets: []analysis.PlayerMetrics{{Name: evil, Team: evil}}}},
		}
		p.Watch = &Watchlist{
			Excluded: []Override{{Label: "EXCL", Player: evil, Reason: evil}},
			Rows:     []WatchRow{{Player: analysis.PlayerMetrics{Name: evil, Team: evil}}},
		}
	})
	if strings.Contains(out, evil) {
		t.Error("a name or reason was written into the page unescaped — html/template " +
			"has been swapped for something that does not escape")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("it does not appear escaped either; is it being dropped?")
	}
}
