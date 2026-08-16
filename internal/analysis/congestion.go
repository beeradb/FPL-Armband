package analysis

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"armband/internal/fpl"
)

// CompetitionWindow is one club's participation in one competition over a date
// range. A club can hold several — Brighton's 2026/27 Conference League run is a
// qualifying play-off in August followed by a league phase from mid-October, with
// a two-month gap between them that a single window could not express.
//
// End is what makes this maintainable in-season. A club knocked out in November
// stops carrying the penalty from that date, without deleting the record of
// having been in the competition.
type CompetitionWindow struct {
	// Competition is UCL, UEL, UECL, or a cup name such as "League Cup".
	Competition string `json:"competition"`
	// Start is the first date the club is committed, YYYY-MM-DD.
	Start string `json:"start_date"`
	// End is the date the club's involvement ceased — elimination, or the final.
	// Empty means still active.
	End string `json:"end_date,omitempty"`
	// MatchDates optionally lists the actual fixture dates. When present, only
	// gameweeks adjacent to those dates carry the penalty, which is far more
	// accurate than assuming every week in the window has a midweek match: a
	// Champions League league phase is eight matches spread across twenty
	// gameweeks, not twenty.
	MatchDates []string `json:"match_dates,omitempty"`
	// Note records why the window is set as it is — a play-off tie, an
	// elimination, a source.
	Note string `json:"note,omitempty"`
}

// UnmarshalJSON accepts the legacy shorthand where a club mapped to a bare
// competition string, so an older config keeps working.
func (w *CompetitionWindow) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		w.Competition = s
		return nil
	}
	type alias CompetitionWindow
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*w = CompetitionWindow(a)
	return nil
}

// Active reports whether the window covers a given date.
func (w CompetitionWindow) Active(on time.Time) bool {
	if start, ok := parseDate(w.Start); ok && on.Before(start) {
		return false
	}
	if end, ok := parseDate(w.End); ok && on.After(end.AddDate(0, 0, 1)) {
		return false
	}
	return true
}

// CoversGameweek reports whether the window implies a midweek commitment around
// a gameweek with the given deadline. With MatchDates set, only gameweeks within
// a few days of a listed fixture count.
func (w CompetitionWindow) CoversGameweek(deadline time.Time) bool {
	if !w.Active(deadline) {
		return false
	}
	if len(w.MatchDates) == 0 {
		return true
	}
	for _, ds := range w.MatchDates {
		d, ok := parseDate(ds)
		if !ok {
			continue
		}
		// A midweek fixture affects the league match on either side of it.
		if diff := deadline.Sub(d).Hours() / 24; diff > -5 && diff < 5 {
			return true
		}
	}
	return false
}

// Congestion models the load a player carries beyond his club's Premier League
// fixtures: midweek European football, domestic cups, international duty, and
// the recovery cost of long-haul travel.
//
// International breaks and fixture turnarounds are derived from the FPL calendar
// and are reliable. Competition participation and nationality are not in the FPL
// API at all and must be configured — the agent refreshes them weekly via web
// search before scoring anyone.
// CampaignMap maps a club's short name to its competition windows, and it is a
// named type for exactly one reason: so that a config file REPLACES it rather
// than merging into it.
//
// # Why this exists
//
// `config.Load` does `cfg := Default()` and then unmarshals the file into it.
// Go's encoding/json REPLACES a slice wholesale but MERGES a map key by key, so
// before this type the four hand-maintained season lists split into two groups
// that behaved in opposite ways, invisibly, because they look alike in the file:
// editing `rest_players` replaced the Go list, while editing `european_campaigns`
// could only add or overwrite a club. **Deleting a club that had gone out of
// Europe did nothing** — and `Load` additionally backfilled an empty map straight
// back to the default, so `{}` and `null` did nothing either. The list could not
// be shortened by any route.
//
// That is a real maintenance need rather than a theoretical one: clubs are
// knocked out every season, and these four lists are re-derived every summer.
//
// Replacing makes all eight duplicated lists behave the same way — the file wins
// when the key is present, the Go default stands when it is absent. Note the
// distinction this restores: an ABSENT key means "I did not say", and `{}` means
// "I say: none". Merging conflated the two.
type CampaignMap map[string][]CompetitionWindow

// UnmarshalJSON replaces rather than merges. Assigning wholesale is the entire
// behaviour; without a method, encoding/json would populate the receiver's
// existing map in place and leave unnamed clubs behind.
func (m *CampaignMap) UnmarshalJSON(b []byte) error {
	var raw map[string][]CompetitionWindow
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*m = raw
	return nil
}

type Congestion struct {
	// European maps a club's short name to its European campaign windows.
	European CampaignMap `json:"european_campaigns"`

	// DomesticCups maps a club's short name to its cup campaign windows.
	DomesticCups CampaignMap `json:"domestic_cup_campaigns"`

	// LastVerified records when competition status was last checked, so a stale
	// configuration is visible rather than silently trusted.
	LastVerified string `json:"status_last_verified,omitempty"`

	// LongHaulRegions are FPL nationality codes whose players face
	// intercontinental travel for internationals. Run `armband nations`.
	LongHaulRegions []int `json:"long_haul_regions"`

	// RegularIntlRegions are codes whose players are near-certain call-ups but
	// travel short distances.
	RegularIntlRegions []int `json:"regular_international_regions"`

	// Penalties are minutes multipliers applied for an affected gameweek.
	UCLPenalty         float64 `json:"ucl_penalty"`
	UELPenalty         float64 `json:"uel_penalty"`
	UECLPenalty        float64 `json:"uecl_penalty"`
	DomesticCupPenalty float64 `json:"domestic_cup_penalty"`
	ShortRestPenalty   float64 `json:"short_rest_penalty"`
	VeryShortRest      float64 `json:"very_short_rest_penalty"`
	PostBreakPenalty   float64 `json:"post_break_penalty"`
	LongHaulPenalty    float64 `json:"long_haul_penalty"`
}

func DefaultCongestion() Congestion {
	return Congestion{
		European:           DefaultEuropeanCampaigns(),
		DomesticCups:       DefaultDomesticCups(),
		LongHaulRegions:    []int{},
		RegularIntlRegions: []int{},

		// Measured at 1.0, all three. The belief was that a Champions League
		// club rotates its league side materially and a Conference League club
		// barely does; the first half is simply not true of minutes.
		//
		// TestDiagEuropeanPenalty baselines each player on the season before
		// his club entered Europe and compares against players at clubs that
		// did not, over four seasons. Minutes come out at 0.98 / 0.97 / 1.05
		// against the control with every interval covering no effect, where
		// 0.93 predicts −0.07. Per-90 output looked like a real 11% loss for
		// UCL until it was compared within bands of prior output, at which
		// point it collapses to −0.007: European clubs hold the best players,
		// per-90 output reverts hardest at the top, and that was the whole of
		// the apparent effect.
		//
		// This is the third hand-set multiplier to measure as nothing, after
		// new_coach_penalty and the rest constants below. The pattern is worth
		// naming: a plausible mechanism, a plausible size, and no effect.
		UCLPenalty:  1.0,
		UELPenalty:  1.0,
		UECLPenalty: 1.0,
		// Not measured, and now 1.0 for that reason rather than despite it.
		//
		// Early cup rounds are largely reserve sides, which would protect league
		// starters rather than tire them, so the sign was never even clear. It
		// was previously left at 0.97 on the principle that a constant should
		// not move on its neighbours' evidence — which was right about evidence
		// and wrong about the default. An unmeasured multiplier that moves a
		// score is not neutral; 1.0 is the neutral value, and it is what the
		// three European penalties and both rest penalties already ship at after
		// measuring as nothing.
		//
		// The whole block is also on the wrong channel — see the rest note below
		// — so measuring this to justify keeping it here would be measuring the
		// wrong quantity.
		DomesticCupPenalty: 1.0,

		// Both measured at 1.0 on the channel they are applied to.
		//
		// Under four days' rest genuinely costs 4.3% of minutes — 0.957, tight
		// enough to exclude no effect — but these multiply Score, and on Score
		// the measured effect is *positive*: points rise 2.7% and points per 90
		// rise 7.2%, because who plays a midweek round is chosen and the chosen
		// are the trusted. Charging a penalty against that is wrong in sign.
		// The minutes finding is real and belongs on the minutes channel, the
		// way rest_minutes_factor does; until something puts it there, 1.0 is
		// the honest value here. Under *three* days' rest and the week after an
		// international break show nothing on either channel.
		ShortRestPenalty: 1.0,
		VeryShortRest:    1.0,
		PostBreakPenalty: 1.0,
		// Not measured, and the claim that it was inert was false in the
		// direction that matters.
		//
		// This comment used to say the term "is also inert as shipped, since
		// long_haul_regions is empty". The Go *default* is empty; config.json
		// sets `long_haul_regions: [30, 10]` and `config.Load` has no backfill
		// for that field, so the file is authoritative and the 0.86 multiplier
		// was live on every run. A comment claiming a term does nothing while it
		// does something is worse than the reverse, because a reader trusts it.
		//
		// Codes 30 and 10 are Brazil and Argentina. The field comment says
		// "players who face intercontinental travel" and the CLI help says "South
		// America, Africa, Asia" — yet Nigeria, Senegal, Morocco, Egypt, Ivory
		// Coast, Ghana, Ecuador, Japan and Uruguay are all absent, every one of
		// them correctly enumerated in DefaultTournamentAbsences. So what
		// actually shipped was an unmeasured 14% discount applied to Brazilians
		// and Argentines and to nobody else: an asymmetry *within* a position,
		// which by this project's own reasoning is the category an argmax can see,
		// unlike a discount applied to everyone equally.
		//
		// long_haul_regions stays populated so the list survives if the term is
		// ever measured properly — on the minutes channel, where it belongs.
		LongHaulPenalty: 1.0,
	}
}

// DefaultEuropeanCampaigns is the 2026/27 Premier League contingent in Europe.
//
// ⚠️ Season-specific. Re-derive every summer; the agent verifies and updates
// these weekly, setting End when a club is eliminated.
func DefaultEuropeanCampaigns() map[string][]CompetitionWindow {
	return map[string][]CompetitionWindow{
		// Champions League league phase begins 8-9 September 2026.
		"ARS": {{Competition: "UCL", Start: "2026-09-08"}},
		"MCI": {{Competition: "UCL", Start: "2026-09-08"}},
		"MUN": {{Competition: "UCL", Start: "2026-09-08"}},
		"AVL": {{Competition: "UCL", Start: "2026-09-08"}},
		"LIV": {{Competition: "UCL", Start: "2026-09-08"}},

		// Europa League league phase begins 16-17 September 2026.
		"BOU": {{Competition: "UEL", Start: "2026-09-16"}},
		"SUN": {{Competition: "UEL", Start: "2026-09-16"}},
		"CRY": {{Competition: "UEL", Start: "2026-09-16"}},

		// Brighton enter the Conference League at the play-off round, two legs
		// on 20 and 27 August — the midweeks either side of gameweeks 1 and 2.
		// The league phase only follows if they progress.
		"BHA": {
			{Competition: "UECL", Start: "2026-08-20", End: "2026-08-27",
				MatchDates: []string{"2026-08-20", "2026-08-27"},
				Note:       "play-off round, two legs; progression not yet known"},
			{Competition: "UECL", Start: "2026-10-15",
				Note: "league phase, conditional on winning the play-off"},
		},
	}
}

// DefaultDomesticCups is the 2026/27 League Cup entry schedule.
//
// Clubs in Europe enter at round three; everyone else enters at round two. That
// inverts the usual assumption for the opening weeks: in August the domestic cup
// load sits on the clubs *without* European football.
func DefaultDomesticCups() map[string][]CompetitionWindow {
	// Round dates: R2 26 Aug, R3 15-16 Sep, R4 28 Oct, SF 13 Jan and 3 Feb,
	// final 21 Mar. The quarter-final date is not yet published.
	inEurope := []string{"ARS", "MCI", "MUN", "AVL", "LIV", "BOU", "SUN", "CRY", "BHA"}
	rest := []string{"BRE", "CHE", "COV", "EVE", "FUL", "HUL", "IPS", "LEE", "NEW", "NFO", "TOT"}

	laterRounds := []string{"2026-10-28", "2027-01-13", "2027-02-03", "2027-03-21"}

	out := map[string][]CompetitionWindow{}
	for _, c := range inEurope {
		out[c] = []CompetitionWindow{{
			Competition: "League Cup", Start: "2026-09-15",
			MatchDates: append([]string{"2026-09-15"}, laterRounds...),
			Note:       "enters at round three as a European qualifier",
		}}
	}
	for _, c := range rest {
		out[c] = []CompetitionWindow{{
			Competition: "League Cup", Start: "2026-08-26",
			MatchDates: append([]string{"2026-08-26", "2026-09-15"}, laterRounds...),
			Note:       "enters at round two",
		}}
	}
	return out
}

// congestionState holds everything derived from the calendar, computed once.
type congestionState struct {
	postBreak   map[int]bool
	restDays    map[int]map[int]float64
	breakLength map[int]float64
	deadlines   map[int]time.Time
}

// parseDate reads a YYYY-MM-DD config date, returning ok=false when unset or
// malformed so callers can fall back to "no gating".
func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// IntlBreakThresholdDays is the deadline gap above which we treat the interval
// as an international break rather than an ordinary week.
const IntlBreakThresholdDays = 10

func (e *Engine) buildCongestionState() {
	st := &congestionState{
		postBreak:   map[int]bool{},
		restDays:    map[int]map[int]float64{},
		breakLength: map[int]float64{},
		deadlines:   map[int]time.Time{},
	}

	evs := append([]fpl.Event{}, e.Boot.Events...)
	sort.Slice(evs, func(i, j int) bool { return evs[i].ID < evs[j].ID })
	for i := range evs {
		st.deadlines[evs[i].ID] = evs[i].DeadlineTime
	}
	for i := 1; i < len(evs); i++ {
		gap := evs[i].DeadlineTime.Sub(evs[i-1].DeadlineTime).Hours() / 24
		if gap >= IntlBreakThresholdDays {
			st.postBreak[evs[i].ID] = true
			st.breakLength[evs[i].ID] = gap
		}
	}

	// Rest days between each club's consecutive league fixtures.
	type stamped struct {
		event int
		when  time.Time
	}
	byTeam := map[int][]stamped{}
	for _, f := range e.Fixtures {
		if f.KickoffTime == nil || f.Event == nil {
			continue
		}
		byTeam[f.TeamH] = append(byTeam[f.TeamH], stamped{*f.Event, *f.KickoffTime})
		byTeam[f.TeamA] = append(byTeam[f.TeamA], stamped{*f.Event, *f.KickoffTime})
	}
	for team, ss := range byTeam {
		sort.Slice(ss, func(i, j int) bool { return ss[i].when.Before(ss[j].when) })
		st.restDays[team] = map[int]float64{}
		for i := 1; i < len(ss); i++ {
			st.restDays[team][ss[i].event] = ss[i].when.Sub(ss[i-1].when).Hours() / 24
		}
	}

	e.congestion = st
}

func (c Congestion) penaltyFor(competition string) float64 {
	switch strings.ToUpper(strings.TrimSpace(competition)) {
	case "UCL":
		return c.UCLPenalty
	case "UEL":
		return c.UELPenalty
	case "UECL":
		return c.UECLPenalty
	default:
		return c.DomesticCupPenalty
	}
}

// CongestionFactor returns the expected-minutes multiplier for a player across
// the fixture horizon, with a human-readable explanation. 1.0 means no load.
// It is safe to call concurrently: the tool runner scores players from several
// goroutines at once while update_competition_status may be rewriting Cong.
func (e *Engine) CongestionFactor(el *fpl.Element) (float64, []string) {
	e.congMu.RLock()
	defer e.congMu.RUnlock()
	if e.congestion == nil {
		// Only reachable for an Engine built as a struct literal rather than
		// through a constructor; upgrade to a write lock to populate it.
		e.congMu.RUnlock()
		e.congMu.Lock()
		if e.congestion == nil {
			e.buildCongestionState()
		}
		e.congMu.Unlock()
		e.congMu.RLock()
	}
	cg := e.Cong
	st := e.congestion

	team := e.Boot.TeamByID(el.Team)
	if team == nil {
		return 1, nil
	}

	longHaul := regionIn(el.Region, cg.LongHaulRegions)
	regularIntl := longHaul || regionIn(el.Region, cg.RegularIntlRegions)

	fx := e.TeamFixtures(el.Team, e.Weights.Horizon)
	if len(fx) == 0 {
		return 1, nil
	}

	campaigns := append(
		append([]CompetitionWindow{}, cg.European[team.ShortName]...),
		cg.DomesticCups[team.ShortName]...)

	var total float64
	reasons := map[string]int{}

	for _, f := range fx {
		gwFactor := 1.0
		deadline, hasDeadline := st.deadlines[f.Event]

		if hasDeadline {
			for _, w := range campaigns {
				if !w.CoversGameweek(deadline) {
					continue
				}
				p := cg.penaltyFor(w.Competition)
				if p <= 0 || p >= 1 {
					continue
				}
				gwFactor *= p
				reasons[w.Competition+" midweek"]++
			}
		}

		// An unset penalty is zero, and zero here would multiply the player's
		// score to nothing rather than leaving it alone. penaltyFor already
		// guards the European terms this way; these did not, and the only
		// reason it never fired is that restDays needs fixture kickoff times
		// and the backtest archive did not carry them. It does now, and
		// Simulate builds every engine with an empty Congestion — so without
		// this a replayed December would zero the score of every player at a
		// club on a short turnaround.
		if rd, ok := st.restDays[el.Team][f.Event]; ok {
			switch {
			case rd < 3:
				gwFactor *= usable(cg.VeryShortRest)
				reasons["under 3 days' rest"]++
			case rd < 4:
				gwFactor *= usable(cg.ShortRestPenalty)
				reasons["under 4 days' rest"]++
			}
		}

		if st.postBreak[f.Event] && regularIntl {
			if longHaul {
				gwFactor *= usable(cg.LongHaulPenalty)
				reasons["long-haul international return"]++
			} else {
				gwFactor *= usable(cg.PostBreakPenalty)
				reasons["international break return"]++
			}
		}

		total += gwFactor
	}

	factor := total / float64(len(fx))

	var notes []string
	for r, n := range reasons {
		if n == len(fx) {
			notes = append(notes, r)
		} else {
			notes = append(notes, fmt.Sprintf("%s (%d of %d GWs)", r, n, len(fx)))
		}
	}
	sort.Strings(notes)
	return factor, notes
}

func regionIn(region *int, list []int) bool {
	if region == nil {
		return false
	}
	for _, r := range list {
		if r == *region {
			return true
		}
	}
	return false
}

// SetCompetitionWindows replaces one club's windows in either the European or
// the domestic-cup map and re-derives the congestion state, then returns a
// snapshot of the whole model for persisting to config.
//
// The agent corrects competition status mid-run, so this races against every
// tool that is scoring players at the same time. Callers must go through here
// rather than assigning to Engine.Cong directly.
func (e *Engine) SetCompetitionWindows(club string, european bool, windows []CompetitionWindow, verifiedOn string) Congestion {
	e.congMu.Lock()
	defer e.congMu.Unlock()

	// Copy-on-write: a caller may still be holding the old map from a read.
	src := e.Cong.DomesticCups
	if european {
		src = e.Cong.European
	}
	next := make(map[string][]CompetitionWindow, len(src)+1)
	for k, v := range src {
		next[k] = v
	}
	if len(windows) == 0 {
		delete(next, club)
	} else {
		next[club] = windows
	}

	if european {
		e.Cong.European = next
	} else {
		e.Cong.DomesticCups = next
	}
	e.Cong.LastVerified = verifiedOn
	e.buildCongestionState()
	return e.Cong
}

// MarkCompetitionsVerified stamps LastVerified without touching any club's
// windows — for the case where a human checked current results against the
// web and found nothing to correct. `armband verify-competitions` is the
// free, deterministic path to this; `update_competition_status` is the paid
// path that also corrects a window as a side effect of an LLM run.
func (e *Engine) MarkCompetitionsVerified(on string) Congestion {
	e.congMu.Lock()
	defer e.congMu.Unlock()
	e.Cong.LastVerified = on
	return e.Cong
}

// ActiveCampaigns lists a club's competition windows still live on a given date.
func (e *Engine) ActiveCampaigns(club string, on time.Time) []CompetitionWindow {
	e.congMu.RLock()
	defer e.congMu.RUnlock()
	var out []CompetitionWindow
	for _, w := range append(
		append([]CompetitionWindow{}, e.Cong.European[club]...),
		e.Cong.DomesticCups[club]...) {
		if w.Active(on) {
			out = append(out, w)
		}
	}
	return out
}

// CampaignGameweeks is one competition window, together with which gameweeks in a
// requested range it implies a midweek commitment for.
type CampaignGameweeks struct {
	Window CompetitionWindow
	// Gameweeks is the subset of the requested range that CoversGameweek matches.
	// It can be empty while the window is still returned: a campaign that runs
	// across the whole range but has no round inside it is a live commitment with
	// nothing due yet, and hiding it is how a stale window escapes review.
	Gameweeks []int
}

// CampaignsOverGameweeks lists a club's competition windows that are live at any
// point across the inclusive gameweek range from..to, and which of those gameweeks
// each one actually implies a midweek match for.
//
// This is the question a scoring horizon asks, and it is not the one ActiveCampaigns
// answers. ActiveCampaigns tests a single instant — "what is live today" — which is
// right for a status display and wrong for anything reported next to a score
// computed over several gameweeks: a Champions League matchday a fortnight out is
// invisible to it. A five-gameweek horizon opening in mid-August spans a Conference
// League play-off, the first Champions and Europa League matchdays and a League Cup
// round for all twenty clubs, and a display built on today's date calls every one of
// them "league only".
//
// The per-gameweek predicate is CoversGameweek, the same one CongestionFactor uses,
// so the two agree about *whether a given gameweek carries a midweek match* —
// MatchDates included, so a window listing specific fixture dates counts only for
// gameweeks within a few days of one rather than for every week it spans.
//
// **They can still disagree about which gameweeks are in scope, and the difference
// is worth knowing.** CongestionFactor derives its window from TeamFixtures — the
// club's next N *fixtures* — while this takes a range of *gameweeks*. For a club that
// blanks inside the range, TeamFixtures reaches a week further ahead than this does;
// for one that doubles, a week less far. So a cup round in the week either side of the
// boundary can be charged by scoring and not listed here, or the reverse. Reporting a
// gameweek range keeps this answerable against the range the caller announces, which
// is the tradeoff taken deliberately.
//
// A gameweek with no published event is skipped rather than guessed at, so a range
// past the end of the calendar simply yields less. Note that is a missing *event*: an
// event present with a zero deadline is passed through, where it reads as earlier
// than every window start and so matches nothing.
//
// Currently CLI-only, via `armband brief`. It reads engine state under the same read
// lock ActiveCampaigns uses and returns copies, so it is safe to call concurrently —
// but see the comment inside about GameweekDeadline, which is not.
func (e *Engine) CampaignsOverGameweeks(club string, from, to int) []CampaignGameweeks {
	// Deadlines are resolved before the lock is taken. GameweekDeadline lazily
	// *builds* e.congestion and takes no lock of its own — it is called elsewhere
	// with the write lock already held — so invoking it from under congMu.RLock
	// would be an unsynchronised write to engine state, not a re-entrant read.
	type dated struct {
		gw       int
		deadline time.Time
	}
	var weeks []dated
	for gw := from; gw <= to; gw++ {
		if d, ok := e.GameweekDeadline(gw); ok {
			weeks = append(weeks, dated{gw, d})
		}
	}

	e.congMu.RLock()
	defer e.congMu.RUnlock()
	var out []CampaignGameweeks
	for _, w := range append(
		append([]CompetitionWindow{}, e.Cong.European[club]...),
		e.Cong.DomesticCups[club]...) {
		live := false
		var gws []int
		for _, wk := range weeks {
			if !w.Active(wk.deadline) {
				continue
			}
			live = true
			// Active is CoversGameweek's own first condition, so this is a subset of
			// the weeks just counted as live.
			if w.CoversGameweek(wk.deadline) {
				gws = append(gws, wk.gw)
			}
		}
		if live {
			out = append(out, CampaignGameweeks{Window: w, Gameweeks: gws})
		}
	}
	return out
}

// PostBreakGameweeks reports which gameweeks follow an international break.
func (e *Engine) PostBreakGameweeks() map[int]float64 {
	if e.congestion == nil {
		e.buildCongestionState()
	}
	out := map[int]float64{}
	for gw, d := range e.congestion.breakLength {
		out[gw] = d
	}
	return out
}

// RestDays reports days between a club's consecutive league fixtures.
func (e *Engine) RestDays(teamID, gameweek int) (float64, bool) {
	if e.congestion == nil {
		e.buildCongestionState()
	}
	d, ok := e.congestion.restDays[teamID][gameweek]
	return d, ok
}

// GameweekDeadline exposes a gameweek's deadline for date comparisons.
func (e *Engine) GameweekDeadline(gw int) (time.Time, bool) {
	if e.congestion == nil {
		e.buildCongestionState()
	}
	d, ok := e.congestion.deadlines[gw]
	return d, ok
}

// StatusAge reports how long ago competition status was verified. ok is false
// when it has never been recorded.
func (e *Engine) StatusAge() (days int, ok bool) {
	t, parsed := parseDate(e.Cong.LastVerified)
	if !parsed {
		return 0, false
	}
	return int(time.Since(t).Hours() / 24), true
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), s) {
			return true
		}
	}
	return false
}

// usable turns an unset or nonsensical penalty into a no-op. Zero means "not
// configured", never "score this player at nothing", and above one is not a
// penalty at all.
func usable(p float64) float64 {
	if p <= 0 || p > 1 {
		return 1
	}
	return p
}
