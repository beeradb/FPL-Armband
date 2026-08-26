// Package backtest replays a past season to test what the optimiser would have
// picked, and how it would have done.
//
// Every calibration in this project so far has been term by term: does the
// defensive-contribution term predict defensive contributions, does the blend
// predict rest-of-season output. None of it asks the question that matters —
// does the assembled squad score points.
//
// # What this can and cannot test
//
// It tests the optimiser's instincts: given the numbers available before a ball
// was kicked, does it choose players who go on to return?
//
// It cannot test the judgement layer. Every override in a live review comes from
// team news, press conferences and predicted line-ups, and those are worthless
// in hindsight — the searches would return today's articles about a season that
// finished two years ago. So a backtest measures the floor the model provides,
// not the system's actual output.
//
// # Overfitting
//
// Three seasons is a small sample and it is easy to tune constants until the
// replay looks good. Treat this as a check on instincts, not a target to
// optimise against, and keep at least one season unexamined while adjusting
// anything.
//
// # Source
//
// github.com/vaastav/Fantasy-Premier-League, which archives the FPL API by
// gameweek back to 2016-17. Its weekly updates stopped after 2024-25, which
// makes it a poor live source and an excellent historical one. Rows are
// per-gameweek rather than cumulative, so actual weekly returns are read
// directly.
package backtest

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// archiveURL is a var rather than a const for exactly one reason: so a test can point
// it at a stub server and check that a 404 loads as prior-only while a 500 stays a hard
// error. That distinction is the whole safety property of prior-only loading and it is
// unreachable against the real archive, which serves neither on demand.
var archiveURL = "https://raw.githubusercontent.com/vaastav/Fantasy-Premier-League/master/data"

// errNoSuchFile marks a 404 from the archive, which is the one failure that means
// "this season does not publish that file" rather than "something went wrong".
//
// The distinction is the whole of what makes prior-only loading safe. A timeout, a 500,
// a DNS failure or a truncated body must stay a hard error — treating any of them as
// absence is how a transient network fault turns a playable season into a silently
// prior-only one, which would remove it from a grid while every number still printed.
// Only a status the server chose is allowed to mean the file is not there.
var errNoSuchFile = errors.New("the archive does not publish this file")

// GW is one player's actual return in one gameweek.
//
// The detail beyond points and minutes exists so a replay can reconstruct what
// the model knew at any point in the season: accumulate these through gameweek
// N and you have the aggregates FPL would have been showing that week.
type GW struct {
	Points  int `json:"points"`
	Minutes int `json:"minutes"`
	// Fixtures is how many matches his club played this gameweek: 1 normally,
	// 2 in a double, and the row does not exist at all in a blank.
	//
	// Every other field here is a *total across those fixtures*, so anything
	// wanting a per-match figure has to divide by this. That distinction is the
	// whole reason the field exists: minutes of 180 means two full matches, not
	// an impossible one.
	Fixtures int `json:"fixtures"`
	// Value is his price that week, in tenths. Prices move all season and the
	// model has no concept of it, so a replay can measure what that costs.
	Value int `json:"value"`

	Starts int `json:"starts"`
	// StartsReconstructed marks a Starts value this parser inferred from minutes
	// because the archive recorded none. Read the three boundaries on
	// reconstructStarts before using such a row for anything.
	StartsReconstructed bool `json:"starts_reconstructed,omitempty"`

	Goals         int     `json:"goals"`
	Assists       int     `json:"assists"`
	CleanSheets   int     `json:"clean_sheets"`
	GoalsConceded int     `json:"goals_conceded"`
	Saves         int     `json:"saves"`
	Bonus         int     `json:"bonus"`
	DefCon        int     `json:"defcon"`
	Yellow        int     `json:"yellow"`
	Red           int     `json:"red"`
	XG            float64 `json:"xg"`
	XA            float64 `json:"xa"`
	XGC           float64 `json:"xgc"`

	// XGCReconstructed marks an XGC this package derived from the opposition's
	// expected goals rather than read from the archive, on the same terms as
	// StartsReconstructed. Read the guard-rails on applyXGCRepair before using
	// such a row for anything: it carries no information about *when* in a match
	// the danger came, so it must never be quoted as evidence about the
	// substitution channel — and measured against the real column it is 4-8% high
	// for a substituted starter and far low for a late substitute, because a
	// player's true exposure is not linear in his minutes.
	//
	// Not serialised, and that is not an oversight. The repair runs after the
	// cache is written — see `repaired` — so what is cached is the archive as it
	// stands and a serialised flag here would always be false, which is a worse
	// lie than an absent field.
	XGCReconstructed bool `json:"-"`

	// Deliberately not parsed: the archive's `xP` column, FPL's own published
	// expected points. It looks like a free external baseline and is not one —
	// the archive scrapes it *after* each gameweek has ended and its own data
	// dictionary warns it may therefore carry post-match information, so it
	// cannot be quoted as the pre-match figure a manager saw. Adding it would
	// also cost a cache-version bump plus a field check in
	// parsedByThisVersion, since a version bump alone does not catch a stale
	// file an experiment left behind. Real cost for a contaminated reference.
	// prediction_test.go uses the clean baseline instead: the mean of the last
	// five gameweeks, which is also OpenFPL's, so the figures stay comparable
	// with published ones.
}

// Player is a player's season: who he was, what he finished with, and what he
// returned week by week.
type Player struct {
	ID      int    `json:"id"` // element id, unique to this season only
	Code    int    `json:"code"`
	Type    int    `json:"element_type"`
	Team    int    `json:"team"`
	WebName string `json:"web_name"`

	Minutes       int     `json:"minutes"`
	Starts        int     `json:"starts"`
	TotalPoints   int     `json:"total_points"`
	Bonus         int     `json:"bonus"`
	Saves         int     `json:"saves"`
	CleanSheets   int     `json:"clean_sheets"`
	GoalsConceded int     `json:"goals_conceded"`
	Goals         int     `json:"goals_scored"`
	Assists       int     `json:"assists"`
	Yellow        int     `json:"yellow_cards"`
	Red           int     `json:"red_cards"`
	XG            float64 `json:"expected_goals"`
	XA            float64 `json:"expected_assists"`
	XGC           float64 `json:"expected_goals_conceded"`
	NowCost       int     `json:"now_cost"`

	// DefCon is the season's defensive contribution, summed from the weekly rows
	// because the archive publishes no season total for it — `players_raw.csv`
	// carries the column only from 2025-26 and `loadPlayers` does not read it.
	//
	// `json:"-"` and derived in `repaired()`, for the reason that function's own
	// comment gives at length: `Load` caches the parsed season *before* repairs,
	// so a derived field with a JSON tag would be written as zero and read back as
	// zero on every later cache hit, with `parsedByThisVersion` unable to see the
	// difference. That is the cache-version bug this file catalogues twice —
	// kickoff times, then the starts harvest — and it costs nothing to avoid.
	//
	// **Zero means "the archive recorded none", which before 2025-26 means the
	// column did not exist.** Consumers that price it must gate on
	// `DefconScoredIn`; consumers that blend it must say so with `NoDefCon`.
	DefCon int `json:"-"`

	// Conversion is this player's POSITION's expected-to-realised conversion scale
	// for this season, which the xPoints instrument prices xG and xA through.
	//
	// It is resolved onto the player rather than passed down because `playerWeek`
	// — the week-scoring path — has no `*Season` in scope, and threading a
	// season-global through `weekScore`, `weekScoreWithChip` and `playerWeek` would
	// put a league-wide quantity into four signatures that are otherwise about a
	// squad. A player belongs to exactly one season and exactly one position, so
	// the resolved scale is a property of him. `xPointsOf` is still the one mapping.
	//
	// `json:"-"` and set in `repaired()`, for the reason that function gives at
	// length and which bites HARDER here than for DefCon: the scale is fitted on
	// xG, so computing it before the cache write would fit it on the UNREPAIRED
	// archive and `FPL_NO_XG_REPAIR=1` would then compare two arms whose scales
	// were identical. The hatch would be a partial no-op that reads as a null.
	// `TestTheConversionScaleFollowsTheXGRepair` fails if it moves.
	//
	// **Zero is not a valid scale and is never a season's answer** —
	// `calibrateConversion` writes every position, falling back to neutral 1.0
	// through `analysis.CalibrationRatio`'s thin-sample floor. A zero here means a
	// `Player` built outside `Load`, and `analysis.XPointsResidual` panics on it
	// rather than silently pricing the underlying at nothing.
	Conversion analysis.ConversionScale `json:"-"`

	// Rules is the FPL points table THIS SEASON was played under, for the four
	// channels the xPoints instrument replaces.
	//
	// It sits beside Conversion, is resolved in the same place and for the same
	// reason: `playerWeek` — the week-scoring path — has no `*Season` in scope,
	// and a player belongs to exactly one season. `xPointsOf` is still the one
	// mapping, and it now carries both.
	//
	// Why it is not simply `analysis.goalPoints` and its siblings, read at the
	// point of use: those are asserted against FPL's *current* published
	// game_config by `TestScoringConstantsMatchFPL`, so the day FPL moves one the
	// whole archive would be re-priced under a rule nobody played under. That is
	// what `BankLimitFor` and `DefconScoredIn` exist to prevent for the transfer
	// bank and for defensive contribution; `analysis.ScoringRulesFor` is the same
	// device for the instrument.
	//
	// `json:"-"` and set in `repaired()`, exactly like Conversion and DefCon: a
	// derived field with a JSON tag is written into the cache as its zero value
	// and read back as zero on every later hit, with `parsedByThisVersion`
	// unable to see the difference. Unlike Conversion it does not depend on the
	// xG repair, so its placement in `repaired()` is about the cache and nothing
	// else.
	//
	// **The zero value is not a season's answer**, and `analysis.XPointsResidual`
	// panics on it rather than pricing every goal at nothing.
	Rules analysis.ScoringRules `json:"-"`

	// Status and NewsAdded are the archive's only availability signal, and they
	// are an *end-of-season snapshot* — the status on the day the file was
	// captured, plus when that particular news was posted. There is no
	// per-gameweek history.
	//
	// That makes them usable one way only. A player whose final status is "u"
	// with an August timestamp really was unavailable from August; a player
	// injured in September and fit by November shows a clean record and is
	// invisible. Good enough to stop the replay buying someone who never plays
	// a minute, useless for the transient cases.
	Status    string `json:"status"`
	NewsAdded string `json:"news_added"`

	GWs map[int]GW `json:"gws"`
}

// Season is one completed season, ready to replay.
type Season struct {
	Name     string          `json:"season"`
	Players  map[int]*Player `json:"players"` // by element id
	Teams    []fpl.Team      `json:"teams"`
	Fixtures []fpl.Fixture   `json:"fixtures"`

	// XGCExternal reports what the measured per-match xGC source did to this
	// season, and is `json:"-"` for the reason XGRepair is: it describes this
	// load rather than the season, and baking it into the cache would make the
	// two arms indistinguishable on a cache hit. Zero-valued when the source is
	// not selected, which is the default and every public clone.
	XGCExternal XGCExternalResult `json:"-"`

	// Absent names the archive files this season does not publish, and is what
	// makes it PriorOnly.
	//
	// The archive thins out going backwards: 2018-19 has `players_raw.csv`,
	// `fixtures.csv` and `gws/merged_gw.csv` and **no `teams.csv`** — checked by
	// status code, not assumed. `PreSeason` reads only season aggregates from the
	// prior and takes `Teams` from the *current* season, so those two files are
	// needed to **play** a season and not to **be** one, and refusing the load
	// outright was the whole blocker on extending the prior axis.
	//
	// It is recorded rather than inferred so the marker names its cause: "this
	// season is prior-only" is much less useful at a call site than "the archive
	// does not publish teams.csv for it". Serialised, so a cache hit says the same
	// thing, and cross-checked against what was actually parsed on every load —
	// see absentIsConsistent. A partial load that reads as a whole one is this
	// package's signature failure, and it must not be reachable by leaving a field
	// unset.
	Absent []string `json:"absent,omitempty"`

	// StrengthAbsent declares that this season's teams.csv carries NO club
	// strength ratings, and that this is a property of the source rather than a
	// damaged cache.
	//
	// ⚠️ DECLARED, never inferred, and that is the whole point of the field.
	// `hasTeamStrength` treats an all-zero teams table as a stale cache and makes
	// `Load` refetch — which is correct, because that shape is what a file written
	// by an older parser looks like. Without a declaration there is no way to tell
	// "the source has no ratings" from "the parser did not read them", and the two
	// need opposite handling. A bare zero must keep failing.
	//
	// # Why a season would have none
	//
	// FPL publishes `strength_*` in `bootstrap-static` for the LIVE season only,
	// and the archive's teams.csv begins at 2019-20. For 2016-17, 2017-18 and
	// 2018-19 the ratings were never archived: the upstream file 404s, and the
	// only surviving copies are mid-season Wayback captures of FPL's own API,
	// which have already absorbed results. Using those would put hindsight into a
	// pre-season prior — the leakage class this package catalogues — so a season
	// wired from them is NOT what this field is for.
	//
	// # What the engine then does, which is why this is safe
	//
	// Nothing special, and nothing new. `analysis.priorFromStrength` already
	// degrades: it uses the granular 1000-1400 ratings only above 100, falls back
	// to the coarse 1-5 rating, and where a club has neither it leaves both priors
	// at `leagueAverageGoals`. That path exists because the granular numbers are
	// unpopulated live in August, so it is exercised every pre-season already.
	//
	// So a declared-absent season starts with every club at league average and
	// learns actual strength from actual results through the shipped blend —
	// point-in-time, with no hindsight anywhere. ⚠️ **That is a different
	// estimand, not a free extra season**: fixture difficulty is flat until the
	// blend moves, so no figure from such a season is comparable with one measured
	// where FPL's own ratings were available. Label it; do not pool it silently.
	StrengthAbsent bool `json:"strength_absent,omitempty"`

	// XGRepair reports what the 2022-23 expected-goals repair did on THIS load.
	//
	// Not serialised: it is a statement about this load rather than data about the
	// season, and the repair is applied after the cache is written so that the
	// escape hatch keeps working on a cache hit. See `repaired`.
	XGRepair xgRepairResult `json:"-"`

	// StartsRepair reports what the recorded-starts harvest did on THIS load, and
	// it exists to be *read* rather than merely counted. Its `Conflict` field is
	// the only thing that can say the harvest windows have drifted into gameweeks
	// the archive records itself, and a counter nothing stores is a counter that
	// cannot report — which is how the first version of this shipped. Not
	// serialised, for the same reason as XGRepair.
	StartsRepair StartsRepairResult `json:"-"`

	// RowGuards reports the merged_gw.csv rows loadGameweeks refused, and unlike
	// XGRepair it IS serialised — because the refusal happens before the cache is
	// written, so a cached season is a season the guards have already been applied
	// to and the count is data about the file rather than about this load.
	//
	// It is a **pointer** so that absent and zero are distinguishable, which is the
	// whole reason it is on the struct at all: nearly every season legitimately
	// drops nothing, so a plain int of 0 cannot say whether the guards ran. Absent
	// means "written by a parser that had no guards", and Load treats that as a
	// cache miss. See rowGuardReport.
	RowGuards *rowGuardReport `json:"row_guards,omitempty"`
}

// rowGuardReport counts the merged_gw.csv rows the parser refused, by reason.
//
// # Both defects are the same defect wearing two hats
//
// The archive files a handful of rows that make a player's club appear to have
// played a match it did not. `loadGameweeks` accumulates `Fixtures`, and
// `simulate.go` divides by it to get `MinutesPerMatch` and `StartShare` — so a
// phantom match does not add anything visible, it silently **halves a rate**. A
// player with a real 90-minute gameweek contributes 45 minutes per match to the
// recency index, at `MinutesHalfLife` 4, and it bleeds through the following four
// or five gameweeks. Points and minutes are untouched, which is what makes it the
// expensive kind of failure by this file's own catalogue: every number stays
// plausible.
//
// # Misfiled, found in 2019-20 and nowhere else
//
// The postponed Manchester City v Arsenal fixture (id 275) appears **twice** in
// `gws/merged_gw.csv`: once at GW29 with zero minutes and zero points, dated the
// day it was to have been played, and once at the renumbered GW30 where it
// actually was. `fixtures.csv` carries it once, at the real event. So the archive
// contradicts itself and the fixture list is the side to believe — it is the file
// the rest of the replay reads its calendar from, and the offending rows are
// empty.
//
// Measured across every season the archive publishes a fixtures.csv for:
// **exactly 59 rows in 2019-20 and 0 in 2018-19, 2020-21, 2021-22, 2022-23,
// 2023-24, 2024-25 and 2025-26.** That zero elsewhere is what makes this a guard
// rather than a special case — nothing here names a season or a fixture id.
//
// # Duplicated, found in 2025-26 and nowhere else
//
// Ten rows are repeated **byte for byte**, for elements 100 (Kroupi) and 391
// (Gannon-Doak). They are the entire weekly-versus-season aggregate drift in that
// season: +163 minutes, +27 points, +4 goals, +4 bonus and +1.6 xG. Kroupi reads
// `Fixtures` 2 in nine ordinary single gameweeks, which is the same halved-rate
// channel as above.
//
// A player cannot appear twice in one fixture, so `(element, fixture)` is a key
// rather than a heuristic, and the count is again 0 in every other season.
//
// # Why the counts are kept rather than the rows silently dropped
//
// A guard that quietly repairs its input is indistinguishable from a guard that
// never fired, and this package has shipped that failure more than once. The
// counts are asserted by TestDiagArchiveRowGuards, so a future archive
// re-publication that fixes these rows upstream — or introduces new ones — turns
// into a test failure rather than a silent change of data.
type rowGuardReport struct {
	// Guards is how many guards the parser that wrote this file had.
	//
	// The pointer on Season.RowGuards distinguishes "parsed with guards" from
	// "parsed without", and that is a **one-generation** gate: it cannot tell a
	// file written with two guards from one written with three. Without this
	// field the next guard added below would read every existing cache as
	// current, drop nothing, and report its own count as a plausible zero —
	// which is the v5-collision failure the Load header describes, in the one
	// shape that header's advice does not cover, because the field would exist
	// and be zero rather than be missing.
	//
	// Bump it when you add a guard. Load treats a lower number as a stale cache.
	Guards int `json:"guards"`
	// Misfiled is rows whose gameweek label disagrees with the event
	// fixtures.csv files that fixture under.
	Misfiled int `json:"misfiled"`
	// Duplicate is rows repeating an (element, fixture) pair already read.
	Duplicate int `json:"duplicate"`
}

// rowGuardCount is how many guards loadGameweeks applies. See rowGuardReport.Guards.
const rowGuardCount = 2

// PriorOnly reports whether this season can be used as a prior but not played.
//
// A prior-only season is a complete, honest record of what players did; what it lacks
// is the club and fixture data a *replay* needs. That distinction is the point of the
// marker: the prior axis is the scarce one — four seasons give the season-clustered
// standard error three degrees of freedom and no number of entry points moves that —
// and a season the archive publishes players for should be allowed to extend it.
func (s *Season) PriorOnly() bool { return len(s.Absent) > 0 }

// PlayableAsCurrent reports why this season cannot be the season being *played*.
//
// It exists as a checked error rather than a paragraph for the reason
// TransferPathComparable does: this package's record is mostly of facts that were
// true, written down, and then violated. A season added to a grid on a Tuesday will
// not have a comment read to it, and a prior-only season played would produce a squad
// with no clubs and no fixtures — a replay that is entirely plausible and entirely
// meaningless, which is the shape of failure this file catalogues.
func (s *Season) PlayableAsCurrent() error {
	if !s.PriorOnly() {
		return nil
	}
	return fmt.Errorf("%s is prior-only: the archive does not publish %s for it, and a "+
		"replay needs club strength and a fixture list. It can be the PRIOR in a pair "+
		"(PreSeason reads only season aggregates from the prior) but not the season "+
		"played. See Season.Absent",
		s.Name, strings.Join(s.Absent, " or "))
}

// mustBePlayable is the backstop for the two functions that have no error channel.
//
// PreSeason and PointInTime are the choke point every playing path passes through —
// Simulate, Hold, HoldWeekly and HoldCaptaincyWeekly all reach one of them — so
// checking here is what makes the guard impossible to forget, where checking only in
// Simulate would leave the three Hold entry points open.
//
// It panics, which is deliberate and has a precedent in parseSweepStarts: playing a
// prior-only season is a programming error rather than a runtime condition, you have to
// have written it into a grid to get here, and silence must not be allowed to read as
// success. Callers with an error channel check PlayableAsCurrent first and report
// cleanly, so this only fires on the paths that could not.
func mustBePlayable(cur *Season) {
	if err := cur.PlayableAsCurrent(); err != nil {
		panic(err.Error())
	}
}

// absentIsConsistent reports whether the Absent list agrees with what was parsed.
//
// This is the check that stops the marker being a field somebody forgot to set. Both
// directions matter and only one of them is obvious: a season claiming teams.csv is
// absent must have no teams, **and** a season with no teams must say so. The second is
// the schema check — a cache written by a parser that could not read teams.csv, or one
// truncated by an interrupted write, would otherwise be read as a legitimate prior-only
// season and silently excluded from every replay it appeared in.
//
// An unrecognised filename fails too, since it means the file was written by a parser
// this one does not understand — the same reasoning as the sidecar window check.
func (s *Season) absentIsConsistent() bool {
	const teams, fixtures = "teams.csv", "fixtures.csv"
	var saysTeams, saysFixtures bool
	for _, f := range s.Absent {
		switch f {
		case teams:
			saysTeams = true
		case fixtures:
			saysFixtures = true
		default:
			return false
		}
	}
	if saysTeams != (len(s.Teams) == 0) {
		return false
	}
	return saysFixtures == (len(s.Fixtures) == 0)
}

// HasXG, HasXGC and HasDefCon report whether this season, **as loaded**, carries a
// statistic at all.
//
// # Measured from the data, never from the season name
//
// A name-keyed predicate is the obvious implementation and it is wrong here, because
// the repairs move the boundary: after `applyXGRepair` the seasons that carry no
// native expected goals do carry them, and after `applyXGCRepair` the same for
// expected goals conceded. A name written before the repair would flag a season that
// now has the figures, discarding real data.
//
// Reading the loaded aggregates instead makes these follow every switch
// automatically, including ones not yet invented — `FPL_NO_XG_REPAIR`,
// `FPL_NO_XGC_REPAIR` and `FPL_NO_XG_AGGREGATE` all change what a season holds and
// therefore change these answers, with nothing here needing to know they exist.
//
// # Season level, not player level
//
// The question is "did this feed measure the statistic", which is a fact about the
// season. Asked per player it would flag every goalkeeper as having no expected
// goals and every forward as having no expected goals conceded, and the blend would
// then drop exactly the players it is most needed for.
//
// One non-zero total anywhere is enough: a season that measured the statistic has
// somebody with a non-zero figure, and a season that did not has nobody.
func (s *Season) HasXG() bool  { return s.anyNonZero(func(p *Player) bool { return p.XG != 0 }) }
func (s *Season) HasXGC() bool { return s.anyNonZero(func(p *Player) bool { return p.XGC != 0 }) }
func (s *Season) HasDefCon() bool {
	return s.anyNonZero(func(p *Player) bool { return p.DefCon != 0 })
}

func (s *Season) anyNonZero(f func(*Player) bool) bool {
	if s == nil {
		return false
	}
	for _, p := range s.Players {
		if f(p) {
			return true
		}
	}
	return false
}

// ByCode indexes players by FPL's permanent code, which is the only identifier
// that survives a season boundary — element ids are reassigned every summer.
func (s *Season) ByCode() map[int]*Player {
	m := make(map[int]*Player, len(s.Players))
	for _, p := range s.Players {
		if p.Code > 0 {
			m[p.Code] = p
		}
	}
	return m
}

// Load returns a completed season, from cache when present. A finished season
// does not change, so the cache never expires.
//
// The version in the filename invalidates it when the *parser* changes rather
// than the data — a cache written by an older parser is a perfectly valid file
// with no way to know it is missing a field.
//
// The version alone is not enough, and relying on it cost an afternoon. Bumping
// v4 to v5 for kickoff times hit v5 files left behind by an earlier experiment,
// so a fresh parser read a stale schema and reported no congestion anywhere —
// the null result looked exactly like a real one. So the cache is *checked*
// rather than trusted: a season with fixtures but no kickoff times cannot have
// come from this parser, whatever the filename says. Add a check here for any
// field a measurement depends on, since the failure is always silent.
//
// A season the archive publishes players for but not clubs or fixtures loads as
// **prior-only** rather than failing — see Season.Absent and PlayableAsCurrent. The
// cache-hit conditions include absentIsConsistent for that reason: a partial load has to
// be distinguishable from a whole one on a cached file too, or the marker is a field
// somebody forgot to write.
//
// # A nil RowGuards is a stale cache, and the obvious alternative check does not work
//
// The row guards drop rows before the cache is written, so a file written without them
// carries the phantom matches — and the only thing distinguishing it is that its report
// is absent. Hence the pointer, and hence checking it here.
//
// The tempting check is the invariant itself: no player may record more `Fixtures` in a
// gameweek than his club actually played. **Measured across all eight seasons, that
// check false-positives** — 1, 4, 4, 7 and 15 player-gameweeks in 2024-25, 2023-24,
// 2022-23, 2020-21 and 2021-22, and it is not a defect. `Player.Team` is the club a
// player finished the season at, so a January move files his August rows against a club
// that was blank that week: Chambers, Trossard, Barkley, Alcaraz. The two seasons that
// carried real phantoms now read exactly zero, which is what makes the invariant a good
// *audit* and a bad *gate*.
func Load(ctx context.Context, cacheDir, season string) (*Season, error) {
	path := filepath.Join(cacheDir, "backtest-v8-"+season+".json")
	if b, err := os.ReadFile(path); err == nil {
		var s Season
		if err := json.Unmarshal(b, &s); err == nil && len(s.Players) > 0 &&
			s.parsedByThisVersion() && s.hasTeamStrength() &&
			s.strengthDeclarationIsConsistent() && s.hasAvailability() &&
			s.hasStarts() && s.hasRestartGameweeks() && s.absentIsConsistent() &&
			s.RowGuards != nil && s.RowGuards.Guards >= rowGuardCount {
			return repaired(&s)
		}
	}
	s, err := fetch(ctx, season)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err == nil {
		if b, err := json.Marshal(s); err == nil {
			_ = os.WriteFile(path, b, 0o644)
		}
	}
	return repaired(s)
}

// repaired applies the xG repair on the way out of Load, so what is CACHED is the
// archive as it stands and what is RETURNED is the repaired season.
//
// # The ordering here is the whole design and getting it wrong is silent
//
// The obvious place for the repair is inside `fetch`, beside the other loaders. That
// is wrong, and wrong in the way this file catalogues over and over: `Load` caches
// the fully-parsed `Season`, so a repair applied before the write would be baked into
// the cache — and then `FPL_NO_XG_REPAIR=1` would read a repaired cache and report the
// unrepaired arm as identical to the repaired one. The escape hatch would be a no-op
// that looked like a null result, which is precisely the shape of the cache-version
// bug that "cost an afternoon" above, and of the appearance-estimator hatch that
// reached one consumer of two.
//
// Applying it after both paths costs a few thousand map writes per season load and
// makes the two arms genuinely comparable. `TestXGRepairSwitchWorksOnACacheHit` fails
// if this moves back inside `fetch`.
//
// It also means the cache needs no version bump: the cached bytes are unchanged, and
// `XGRepair` is `json:"-"` because it is a report about this load rather than data
// about the season.
func repaired(s *Season) (*Season, error) {
	// The measured per-match xGC source, BEFORE the repair rather than after.
	// applyXGCRepair fills only a zero, so writing measured values first makes
	// the reconstruction a no-op exactly where this reached and leaves it in
	// charge everywhere it did not. Reversing the two would have the
	// reconstruction win every row and this write nothing, silently. See
	// xgcexternal.go.
	ext, err := s.applyExternalXGC()
	if err != nil {
		return nil, err
	}
	s.XGCExternal = ext
	rep, err := s.applyXGRepair()
	if err != nil {
		return nil, err
	}
	s.XGRepair = rep
	// The starts harvest, for the same reason and on the same path. It was briefly
	// applied inside `fetch` instead, which is exactly what the comment above
	// forbids, and the consequence was the one predicted there: every machine with
	// a v8 cache written before the harvest replayed on rank-reconstructed starts
	// with nothing erroring, and FPL_NO_STARTS_REPAIR could not decide the outcome
	// in either direction. TestTheStartsSwitchWorksOnACacheHit fails if it moves
	// back.
	sr, err := applyStartsRepair(s)
	if err != nil {
		return nil, err
	}
	s.StartsRepair = sr
	// The defensive-contribution season total, on the same path and for the same
	// reason. It is a derivation rather than a repair — nothing is reconstructed,
	// the weekly rows are simply added up — but it is on this side of the cache
	// write because a derived field written into the cache as zero is
	// indistinguishable from a season that recorded none. See Player.DefCon.
	s.sumDefCon()
	// The xPoints instrument's two per-season inputs — the per-position conversion
	// scale and this season's own FPL points table. On this side of the cache
	// write for the reason above and, for the scale, one stronger.
	//
	// The scale is fitted ON xG, so computing it before the
	// repair would fit it on the unrepaired archive — and then FPL_NO_XG_REPAIR=1
	// would compare a repaired arm against an unrepaired one through the SAME
	// scale, understating the switch on the instrument. It must also come after
	// applyXGRepair specifically, not merely after the cache write.
	// ⚠️ TestTheConversionScaleFollowsTheXGRepair checks that the stored scale is the
	// one this season's POST-repair rows imply, which catches a fit over the wrong
	// row population. It does NOT currently catch the ordering itself: the xG repair
	// fills from a harvest keyed on real player codes, so on a synthetic fixture it
	// moves no minutes-bearing row and the arm contrast skips. The test says so out
	// loud. Pinning the ordering wants a real archived season through Load.
	s.resolveInstrumentInputs()
	return s, nil
}

// resolveInstrumentInputs resolves onto every player the two per-season
// quantities the xPoints instrument prices a row through: the fitted conversion
// scale, and the FPL points table this season was actually played under.
//
// **They are one step because they must never be done separately.** Both are
// `json:"-"` fields set on this side of the cache write, both are read by
// `xPointsOf` on every player-gameweek, and a fixture that resolved one and not
// the other would either price the underlying at nothing or refuse the row
// outright. A hand-built season that stands in for `repaired()` calls this, not
// its halves. `calibrateConversion` stays separately callable only for
// `conversionscale_test.go`, which is about the fit itself.
func (s *Season) resolveInstrumentInputs() {
	s.calibrateConversion()
	s.resolveScoringRules()
}

// resolveScoringRules puts the season's own points table onto every player.
//
// One resolution per season rather than one per row: `ScoringRulesFor` builds
// maps, and `xPointsOf` runs on every player-gameweek of every cell.
//
// It is separate from `calibrateConversion` because the two answer different
// questions — one is fitted from this season's rows, the other is read off the
// calendar — and unlike the scale it does not depend on the xG repair, so its
// placement in `repaired()` is about the cache and nothing else.
func (s *Season) resolveScoringRules() {
	r := analysis.ScoringRulesFor(s.Name)
	for _, p := range s.Players {
		p.Rules = r
	}
}

// calibrateConversion fits, per position, how this season's expected goals and
// assists converted into the goals and assists FPL paid for, and resolves the
// answer onto every player of that position.
//
// # The population is league-wide, and that is the load-bearing property
//
// It sums EVERY player-gameweek in the season with minutes on it, not a squad's
// rows and not a cell's window. That is what keeps the instrument arm-invariant: a
// scale identical in every arm of a paired comparison leaves hold_xpoints and
// policy_xpoints differences as comparisons of one metric, whereas a scale fitted
// on the squad would move with the arm and quietly turn a paired difference into
// two metrics. TestTheConversionScaleIsSeasonGlobal pins it.
//
// # It differs from the engine's population on purpose
//
// analysis.calibrateExpectedStats sums the bootstrap's season-to-date aggregates
// over players registered by the cutoff; this sums an archived season's weekly rows
// whole. Only the ratio itself — the [0.5, 3.0] clamp and the minCalibrationSample
// floor — is shared, through analysis.CalibrationRatio. The engine wants a
// point-in-time answer because it is predicting; the instrument wants the season's
// own answer because it is measuring what that season's chances were worth, and
// because a season-to-date scale would be neutral for the opening gameweeks and
// therefore differ between two cells that entered at different deadlines — which is
// the arm-invariance property above, failing on the start-point axis instead.
//
// ⚠️ The fit is IN SAMPLE. See XPointsResidual's comment for what that costs: the
// position-mean attacking residual afterwards is exactly zero, so the instrument
// reports within-position deviation only and cross-season LEVELS are recentred.
//
// # The floor binds for keepers only, and that is correct
//
// minCalibrationSample is 20.0 expected events. Defenders, midfielders and forwards
// clear it by orders of magnitude on a full season, so the guard is inert for them.
// Keepers do not clear it — keeper xG is nowhere near 20 in any season — so GKP
// falls back to neutral 1.0. The floor binds on the EXPECTED total rather than the
// realised one, so this holds whatever a keeper scores.
//
// ⚠️ **And it is NOT because "a keeper's attacking residual is zero anyway", which an
// earlier version of this comment claimed.** Keeper assists/xA across the archive runs
// 1.5 to 7.8 — roughly half a keeper's assists are long punts xA cannot model — so a
// keeper keeps a positive unremoved residual, and the identity below is a claim about
// DEF, MID and FWD only. ⚠️ Nor is "zero keeper goals in the archive" the right
// support: xpointstable_test.go records that for the THREE NATIVE-XG SEASONS
// specifically, CalibrationRatio's own comment quotes a live-bootstrap keeper goal
// count that is not zero, and this record's standing rule is that "the archive does
// not have X" is season-scoped and has failed seven times.
func (s *Season) calibrateConversion() {
	scales := s.conversionFit(countExposedReturns)
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		sc, ok := scales[p.Type]
		if !ok {
			// A position with no players cannot reach here, but a player whose
			// element_type the archive did not file would: neutral rather than
			// zero, because zero prices his underlying at nothing.
			sc = analysis.ConversionScale{Goals: 1, Assists: 1}
		}
		p.Conversion = sc
	}
}

// exposedReturns is what the fit does with a row that realised a goal or an assist
// in a gameweek whose channel this season DOES record, but which carries no
// underlying of its own — a won penalty, a deflection, a rebound off the
// player's own shot.
//
// It is a parameter rather than two functions because the alternative arm is a
// MEASUREMENT of the shipped one, and a diagnostic that carried its own copy of
// this fit would be checking a second implementation. See
// conversionfit_diag_test.go, which is the only caller that passes anything but
// the shipped value.
type exposedReturns int

const (
	// countExposedReturns is what ships: the realised return enters the numerator
	// with nothing behind it in the denominator, so the fitted ratio prices it
	// across every row that DOES carry underlying. That is the scale's stated
	// job — FPL pays assists an expected-assists model does not count.
	//
	// ⚠️ It is NOT most of why a forward's assists/xA sits near 2.1, which an
	// earlier version of this comment said. Measured by dropping them
	// (conversionfit_diag_test.go): the forward ratio's excess over 1 falls by
	// **13.5% to 26.6%** by season — so the exposed rows are a FLOOR on the
	// definitional offset and not its size.
	//
	// ⚠️ They are not a measurement of it, and reading them as one is the error
	// this comment made on its second attempt. `XA == 0` is a display threshold:
	// the archive publishes expected_assists to two decimal places, the 0.01-0.05
	// band carries five to six times as many assists in the FPL-fed seasons, and
	// those rows are the same phenomenon on the other side of a rounding
	// boundary. What causes the remainder is UNTESTED — under-prediction on
	// genuinely-measured chances and near-zero-but-rounded returns are not
	// separated here, and nothing in the tree separates them.
	countExposedReturns exposedReturns = iota

	// dropExposedReturns removes the row from the channel it is exposed on,
	// leaving the other channel alone. It exists to size how far the shipped
	// ratio moves because of those rows; it is not a shipped setting, and
	// underlyingCoverage's comment says why gating them is the wrong repair.
	dropExposedReturns
)

// conversionFit is the fit itself, per position, keyed by element_type.
//
// Split out from calibrateConversion so that the two answers — shipped, and with
// the blank-underlying rows dropped — come from ONE implementation. The
// accumulation order is the fitted quantity's, unchanged: sortedSeasonPlayerIDs
// then gameweek 1..38, goals channel before assists, because float addition is
// not associative and a reordering here would move the shipped scale in its last
// bits without anything failing.
func (s *Season) conversionFit(blank exposedReturns) map[int]analysis.ConversionScale {
	recordsXG, recordsXA := s.underlyingCoverage()

	type totals struct{ goals, xG, assists, xA float64 }
	sums := map[int]*totals{}
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		t := sums[p.Type]
		if t == nil {
			t = &totals{}
			sums[p.Type] = t
		}
		for gw := 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok || g.Minutes <= 0 {
				continue
			}
			// ⚠️ A gameweek the season does not RECORD underlying for contributes
			// its realised events to the numerator and nothing to the denominator,
			// which inflates the ratio by however much of the season is missing.
			// See underlyingCoverage.
			if recordsXG[gw] && !(blank == dropExposedReturns && g.XG == 0 && g.Goals > 0) {
				t.goals += float64(g.Goals)
				t.xG += g.XG
			}
			if recordsXA[gw] && !(blank == dropExposedReturns && g.XA == 0 && g.Assists > 0) {
				t.assists += float64(g.Assists)
				t.xA += g.XA
			}
		}
	}

	scales := make(map[int]analysis.ConversionScale, len(sums))
	for pos, t := range sums {
		scales[pos] = analysis.ConversionScale{
			Goals:   analysis.CalibrationRatio(t.goals, t.xG),
			Assists: analysis.CalibrationRatio(t.assists, t.xA),
		}
	}
	return scales
}

// underlyingCoverage reports, per gameweek, whether this season records expected
// goals and expected assists at all — which is a CAPABILITY question and not a
// per-row one.
//
// # Why the fit needs this, and what it cost to find out
//
// A ratio is realised over expected. A row the archive records no xG for still
// carries the player's realised goals, so counting it puts events in the numerator
// with nothing behind them in the denominator and the ratio inflates by however
// much of the season is missing.
//
// At shipped config this is exactly harmless — measured over all eight cached
// seasons and all four positions, **zero goals sit on a zero-xG row**. Under
// `FPL_NO_XG_REPAIR=1` it is not, because 2022-23 records native xG only from
// GW16 and the fifteen blind gameweeks keep their goals:
//
//	2022-23, repair off    fitted    coverage-matched    goals on zero-xG rows
//	  DEF                  1.0002        0.6974                  33
//	  MID                  1.5394        0.9703                 217
//	  FWD                  1.4189        0.9211                 120
//
// Nothing errors, no scale is zero, the panic guard never fires and the in-sample
// identity still holds season-wide. The instrument simply stops measuring
// conversion and starts redistributing fifteen gameweeks of unrecorded chances
// onto twenty-three gameweeks of recorded ones — and the split lands on the ENTRY
// GAMEWEEK axis, so a window opening at GW11 and one at GW21 are on different
// instruments. That is the reproduction switch `AGENTS.md` requires for every
// figure recorded before 2026-08-10.
//
// # ⚠️ It must NOT be a per-row `XG > 0` test
//
// A realised assist on a row with zero xA is usually REAL rather than missing — a
// won penalty, a rebound off the player's own shot, a deflected pass, none of
// which an expected-assists model counts. Measured: 8 to 27 assists a season sit on
// zero-xA rows at shipped config, 5-13% of a position's total. Gating those out
// would move the shipped forward assists scale by 5-14% (2024-25: 2.077 → 1.932)
// and would leave exactly the events the scale exists to price as unremoved
// positive residual.
//
// So the question is per (gameweek, channel): did this season record the column at
// all that week. That is the same shape as `DefconScoredIn` and is what the queue's
// missing-data item already prescribes — a capability gate, never a zero test.
//
// The two channels are resolved separately because they can differ: the xG repair
// and the Understat harvest fill xG and xA on their own windows.
func (s *Season) underlyingCoverage() (xg, xa map[int]bool) {
	xg, xa = map[int]bool{}, map[int]bool{}
	for _, p := range s.Players {
		for gw, g := range p.GWs {
			if g.XG > 0 {
				xg[gw] = true
			}
			if g.XA > 0 {
				xa[gw] = true
			}
		}
	}
	return xg, xa
}

// ConversionScales reports the fitted per-position scale, for diagnostics and for
// the tests that check it. Keyed by element_type.
//
// It reads back off the players rather than being stored separately, because a
// second copy of a quantity is this record's signature failure and the players are
// where the instrument actually reads it.
func (s *Season) ConversionScales() map[int]analysis.ConversionScale {
	out := map[int]analysis.ConversionScale{}
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		out[p.Type] = p.Conversion
	}
	return out
}

// sumDefCon fills each player's season defensive contribution from his weekly rows.
//
// # Why the archive needs this at all
//
// `defensive_contribution` reaches this package only through `loadGameweeks`
// (`g.DefCon`); `loadPlayers` never reads a season total, and before 2025-26 the
// column does not exist in either file. So `Player.DefCon` was structurally absent,
// and every prior built from an archived season carried nothing for it — which is
// not a missing term but a wrong one, because `blendRates` mixes toward whatever the
// prior says and `baseXP90` prices the result.
//
// # It is validated against a source the derivation never sees
//
// `players_raw.csv` publishes its own `defensive_contribution` total for 2025-26.
// Summed against it element by element the two agree on **840 of 841**, and the
// single exception is the archive's known duplicate `(element, fixture)` rows —
// nine of the ten belong to element 100 and are worth exactly the 11-point gap.
// After the row guard the residual is **identically zero on 841 of 841**, which is
// the gate this project demands of a reconstruction ("residual identically zero,
// never a good fit"). `TestDefConAggregateMatchesTheArchivesOwnTotal` pins it.
//
// # Integer, so the ordering argument does NOT apply
//
// It goes through `weeklyTotal` so the 1..38 walk has one implementation, but not
// for that function's stated reason: integer addition is associative, so a map-order
// sum would give the same answer. The float64 round trip is exact — a season total
// is a few hundred at most, nowhere near 2^53.
func (s *Season) sumDefCon() {
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		p.DefCon = int(weeklyTotal(p, func(g GW) float64 { return float64(g.DefCon) }))
	}
}

// parsedByThisVersion reports whether a cached season carries the fields the
// current parser produces. It is a schema check, not a data check: every
// completed season has kickoff times for every fixture, so their absence means
// the file was written before the parser read them.
func (s *Season) parsedByThisVersion() bool {
	if len(s.Fixtures) == 0 {
		return true // nothing to judge it on; the players are what matter
	}
	var kickoffs bool
	for _, f := range s.Fixtures {
		if f.KickoffTime != nil {
			kickoffs = true
			break
		}
	}
	if !kickoffs {
		return false
	}
	// Gameweek rows must carry a fixture count. Written before that field
	// existed, they hold one fixture's data per double gameweek instead of the
	// sum, and every replayed double is scored at half strength — silently,
	// because the numbers are all plausible. A version bump alone would not
	// catch it: a v7 file left by an experiment would be read as current.
	for _, p := range s.Players {
		for _, g := range p.GWs {
			if g.Minutes > 0 || g.Points != 0 {
				return g.Fixtures > 0
			}
		}
	}
	return true
}

// hasStarts reports whether the cached season carries a usable starts field —
// either recorded by the archive or reconstructed by this parser.
//
// This is the schema check for the starts reconstruction, and it is needed for the
// reason the kickoff-time check documents: a version bump alone does not catch a
// stale file. There are v2, v3 and v4 archive files sitting beside the current ones
// on this machine, so leftovers demonstrably accumulate, and a v8 file written by
// an experiment before the reconstruction landed would otherwise be read as
// current — reporting every player in 2021-22 as never starting, which is exactly
// the silent null the reconstruction exists to remove.
//
// The test is that SOME gameweek with minutes has a start. That holds for every
// real season: eleven players start every match. A file where nobody who played
// ever started cannot have come from this parser.
func (s *Season) hasStarts() bool {
	var sawMinutes bool
	for _, p := range s.Players {
		for _, g := range p.GWs {
			if g.Starts > 0 {
				return true
			}
			if g.Minutes > 0 {
				sawMinutes = true
			}
		}
	}
	// No football recorded at all: nothing to judge it on, so do not reject.
	return !sawMinutes
}

// hasRestartGameweeks is the schema check for the 2019-20 renumber.
//
// Before renumberGW existed, loadGameweeks dropped that season's restart rounds — they
// are labelled 39-47 and the bounds check took 1..38 — so a cache written then holds 29
// gameweeks and reads as a season that stopped in March. Every figure computed from it
// would be plausible and a quarter short, which is the doubles-counting failure again.
//
// A version bump alone would not catch it, for the reason documented on Load: there are
// v2, v3 and v4 archives sitting beside the current ones on this machine, so leftovers
// demonstrably accumulate. A bump would also invalidate the four seasons this change
// does not touch, and re-fetching them to fix a season that has never been cached is
// churn rather than caution — so the check is targeted at the season that can be wrong.
//
// The test is that a renumbered season records football above gameweek 29. That holds
// for 2019-20 by construction once the renumber runs: the restart is nine rounds and
// they land on 30-38.
func (s *Season) hasRestartGameweeks() bool {
	if !xgRepairs[s.Name].Renumber {
		return true
	}
	for _, p := range s.Players {
		for gw, g := range p.GWs {
			if gw > 29 && (g.Minutes > 0 || g.Fixtures > 0) {
				return true
			}
		}
	}
	return false
}

// hasAvailability reports whether the cached season carries player status. Every
// season has some, so an absence means the file predates the parser reading it.
func (s *Season) hasAvailability() bool {
	for _, p := range s.Players {
		if p.Status != "" {
			return true
		}
	}
	return len(s.Players) == 0
}

// hasTeamStrength reports whether the cached season carries FPL's pre-season
// club strength ratings. Same schema check as the kickoff times: every season
// whose teams.csv HAS them must show them, so their absence means the file
// predates the parser reading them.
//
// ⚠️ The check is deliberately asymmetric, and each branch answers a different
// question:
//
//   - `StrengthAbsent` — the source has no ratings, DECLARED. Accept.
//   - a non-empty table with any non-zero rating — parsed fine. Accept.
//   - a non-empty table, all zero, NOT declared — indistinguishable from a file
//     an older parser wrote, so treat it as a stale cache and refetch. **This
//     must keep failing**; it is the case the guard exists for.
//   - an empty table — the season is prior-only and says so in `Absent`.
//
// Reading the declaration first is what lets a legitimately strengthless season
// exist without weakening the guard for everything else. Inferring it from the
// zeros instead would delete the guard, because the two look identical.
func (s *Season) hasTeamStrength() bool {
	// Declared absence beats inspection: see Season.StrengthAbsent for why the
	// engine is safe without ratings, and for why a bare zero is not enough.
	if s.StrengthAbsent {
		return true
	}
	return s.carriesAnyStrength() || len(s.Teams) == 0
}

// carriesAnyStrength reports whether ANY club has a rating. Literally that, and
// nothing else — an empty table returns false here, because "there are no clubs"
// and "the clubs have no ratings" are different facts and the callers need them
// apart. hasTeamStrength adds the empty case back; the consistency check below
// must not.
func (s *Season) carriesAnyStrength() bool {
	for _, t := range s.Teams {
		if t.StrengthAttackHome > 0 || t.StrengthDefenceHome > 0 {
			return true
		}
	}
	return false
}

// strengthDeclarationIsConsistent reports whether `StrengthAbsent` describes the
// teams table it sits beside.
//
// ⚠️ A season may not wear the label falsely, and the reason is downstream of
// this package. The declaration is the ONLY thing telling a later reader that
// the season's fixture difficulty started flat and was learned from results
// rather than given by FPL — which makes its figures a different estimand.
// A season that claims absence while carrying real ratings gets pooled with
// ordinary seasons and nothing objects, which is the mislabelling this whole
// field exists to prevent.
//
// The check is one-directional on purpose: carrying ratings WITHOUT declaring
// anything is the ordinary case and must stay legal. Only the claim is checked,
// never the silence.
//
// ⚠️ It asks a BROADER question than `hasTeamStrength`, and the difference is the
// whole reason this is a separate function rather than a `!hasTeamStrength()`.
// `hasTeamStrength` inspects the granular attack/defence pair only — which is
// correct for it, because a file with those unset is what an older parser wrote.
// But `analysis.priorFromStrength` also reads the COARSE rating, `t.Strength`,
// falling back to `t.StrengthOverallHome`, and `fpl.Team.Strength`'s own comment
// says the coarse value is **"pre-season … the *only* one"**.
//
// So a teams table carrying coarse ratings and nothing else still produces
// club-DIFFERENTIATED priors through `coarseConceded`/`coarseScored`. Checking
// only the granular pair here would accept `StrengthAbsent` on such a season,
// and the label would then assert flat difficulty for a season that has none of
// the sort — which is exactly the mislabelling the field exists to prevent, with
// the guard nodding it through.
func (s *Season) strengthDeclarationIsConsistent() bool {
	if !s.StrengthAbsent {
		return true
	}
	return !s.carriesAnyUsableStrength()
}

// carriesAnyUsableStrength reports whether ANY field `priorFromStrength` can act
// on is populated — granular or coarse. Broader than `carriesAnyStrength` by the
// coarse pair, deliberately; see `strengthDeclarationIsConsistent`.
func (s *Season) carriesAnyUsableStrength() bool {
	for _, t := range s.Teams {
		if t.StrengthAttackHome > 0 || t.StrengthAttackAway > 0 ||
			t.StrengthDefenceHome > 0 || t.StrengthDefenceAway > 0 ||
			t.StrengthOverallHome > 0 || t.StrengthOverallAway > 0 ||
			t.Strength > 0 {
			return true
		}
	}
	return false
}

func fetch(ctx context.Context, season string) (*Season, error) {
	s := &Season{Name: season, Players: map[int]*Player{}}
	if err := s.loadPlayers(ctx); err != nil {
		return nil, err
	}
	// Before the gameweeks, and that ordering is load-bearing rather than tidy.
	// loadGameweeks checks every row's gameweek label against the event
	// fixtures.csv files the same fixture under, which it cannot do if the fixture
	// list is not there yet. See rowGuardReport.
	if err := s.loadFixtures(ctx); err != nil {
		return nil, err
	}
	if err := s.loadGameweeks(ctx); err != nil {
		return nil, err
	}
	if err := s.loadTeams(ctx); err != nil {
		return nil, err
	}
	// Canonical order, so absentIsConsistent and a cached file do not depend on which
	// loader happens to run first.
	sort.Strings(s.Absent)
	// Last, because it needs every player's minutes for the whole season and it
	// reads the fixture count that loadGameweeks accumulates.
	//
	// The starts REPAIR is deliberately not here. It runs in `repaired`, on the way
	// out of Load, for the reason that function documents at length: a repair
	// applied before the cache write is baked into the cache, and its escape hatch
	// then reads a repaired cache and reports both arms as identical. This
	// reconstruction is a parser step and belongs in the cached bytes; the repair
	// is not and does not.
	s.reconstructStarts()
	return s, nil
}

func rows(ctx context.Context, season, file string) (*csv.Reader, io.Closer, map[string]int, error) {
	url := fmt.Sprintf("%s/%s/%s", archiveURL, season, file)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("User-Agent", "armband/1.0 (+personal FPL analysis)")
	resp, err := (&http.Client{Timeout: 180 * time.Second}).Do(req)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, nil, nil, fmt.Errorf("GET %s: %w", url, errNoSuchFile)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	r := csv.NewReader(resp.Body)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		resp.Body.Close()
		return nil, nil, nil, fmt.Errorf("%s: %w", file, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	return r, resp.Body, col, nil
}

func (s *Season) loadPlayers(ctx context.Context) error {
	r, c, col, err := rows(ctx, s.Name, "players_raw.csv")
	if err != nil {
		return err
	}
	defer c.Close()
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("players_raw.csv: %w", err)
		}
		id := ival(rec, col, "id")
		if id == 0 {
			continue
		}
		s.Players[id] = &Player{
			ID: id, Code: ival(rec, col, "code"), Type: ival(rec, col, "element_type"),
			Team: ival(rec, col, "team"), WebName: sval(rec, col, "web_name"),
			Minutes: ival(rec, col, "minutes"), Starts: ival(rec, col, "starts"),
			TotalPoints: ival(rec, col, "total_points"), Bonus: ival(rec, col, "bonus"),
			Saves: ival(rec, col, "saves"), CleanSheets: ival(rec, col, "clean_sheets"),
			GoalsConceded: ival(rec, col, "goals_conceded"), Goals: ival(rec, col, "goals_scored"),
			Assists: ival(rec, col, "assists"),
			Yellow:  ival(rec, col, "yellow_cards"), Red: ival(rec, col, "red_cards"),
			XG: fval(rec, col, "expected_goals"), XA: fval(rec, col, "expected_assists"),
			XGC: fval(rec, col, "expected_goals_conceded"), NowCost: ival(rec, col, "now_cost"),
			Status: sval(rec, col, "status"), NewsAdded: sval(rec, col, "news_added"),
			GWs: map[int]GW{},
		}
	}
	if len(s.Players) == 0 {
		return fmt.Errorf("%s: no players", s.Name)
	}
	return nil
}

func (s *Season) loadGameweeks(ctx context.Context) error {
	guards, err := s.gameweekRows(ctx, func(rec []string, col map[string]int, p *Player, gw int) {
		// Accumulate, never assign.
		//
		// FPL publishes one row per *fixture*, so a double gameweek gives a
		// player two rows with the same gameweek number. Assigning meant the
		// second silently overwrote the first and the replay scored half of
		// every double — confirmed by the largest minutes figure recorded in a
		// double being 90, where two full matches is 180. Doubles run 10 to 42
		// team-gameweeks a season, so this was not a rounding error.
		g := p.GWs[gw]
		g.Fixtures++
		g.Points += ival(rec, col, "total_points")
		g.Minutes += ival(rec, col, "minutes")
		g.Starts += ival(rec, col, "starts")
		g.Goals += ival(rec, col, "goals_scored")
		g.Assists += ival(rec, col, "assists")
		g.CleanSheets += ival(rec, col, "clean_sheets")
		g.GoalsConceded += ival(rec, col, "goals_conceded")
		g.Saves += ival(rec, col, "saves")
		g.Bonus += ival(rec, col, "bonus")
		g.Yellow += ival(rec, col, "yellow_cards")
		g.Red += ival(rec, col, "red_cards")
		g.DefCon += ival(rec, col, "defensive_contribution")
		g.XG += fval(rec, col, "expected_goals")
		g.XA += fval(rec, col, "expected_assists")
		g.XGC += fval(rec, col, "expected_goals_conceded")
		// Price is a level, not a count: the second fixture's value is the
		// current one and summing them would double a player's price.
		g.Value = ival(rec, col, "value")
		p.GWs[gw] = g
	})
	if err != nil {
		return err
	}
	s.RowGuards = guards
	return nil
}

// gameweekRows walks `gws/merged_gw.csv` and calls fn once per surviving row,
// with the row's renumbered gameweek and the player it belongs to.
//
// # Why this is separate from loadGameweeks
//
// A row of `merged_gw.csv` is **one match**, and `GW` is a whole gameweek — so a
// double's two matches are added together the moment loadGameweeks' callback
// returns and the per-match split is gone. Anything measuring a single match
// (does this appearance clear the hour, what did this one match pay) has to read
// the rows themselves.
//
// The alternative is a second walk that re-derives the gameweek renumbering and
// the two row guards, which is this project's signature failure — one quantity
// with two implementations — on the guards it can least afford: a phantom match
// counted as real turns a single gameweek into a double, which is precisely the
// distinction a doubles measurement exists to make. So the walk is written once
// and the accumulation is the caller's.
//
// The report is RETURNED rather than assigned to s, because a second caller
// walking an already-loaded season must not overwrite what the load recorded.
func (s *Season) gameweekRows(ctx context.Context,
	fn func(rec []string, col map[string]int, p *Player, gw int)) (*rowGuardReport, error) {
	r, c, col, err := rows(ctx, s.Name, "gws/merged_gw.csv")
	if err != nil {
		return nil, err
	}
	defer c.Close()
	// Older seasons label the gameweek "round"; newer ones add "GW".
	key := "GW"
	if _, ok := col[key]; !ok {
		key = "round"
	}
	// The fixture list, indexed by id, is what the two row guards check against.
	// Empty for a season the archive publishes no fixtures.csv for, and both
	// guards degrade to nothing rather than to a refusal — a season with no
	// calendar has nothing to contradict. See rowGuardReport.
	event := make(map[int]int, len(s.Fixtures))
	for _, f := range s.Fixtures {
		if f.Event != nil {
			event[f.ID] = *f.Event
		}
	}
	seen := make(map[[2]int]bool, 30000)
	guards := &rowGuardReport{Guards: rowGuardCount}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("merged_gw.csv: %w", err)
		}
		p := s.Players[ival(rec, col, "element")]
		if p == nil {
			continue
		}
		// 2019-20's gameweeks are labelled 1-29 then 39-47, because COVID stopped
		// the season and FPL numbered the restarted rounds afresh rather than
		// reusing 30-38. Without the renumber the bounds check below drops all nine
		// restart rounds — a quarter of the season, sitting in the archive, reading
		// as a season that simply stopped in March. See renumberGW.
		gw := renumberGW(s.Name, ival(rec, col, key))
		if gw < 1 || gw > 38 {
			continue
		}
		// The two row guards, in the order that makes their counts disjoint: a
		// misfiled row is refused before it can claim its (element, fixture) key,
		// so it cannot then make the row that IS correctly filed look like a
		// duplicate of it. Reversing these two blocks reports 2019-20's 59 rows
		// under whichever of the pair the reader happens to reach first.
		if fid := ival(rec, col, "fixture"); fid > 0 {
			// Guard A needs the calendar and is inert without it. Guard B does
			// not — "a player cannot appear twice in one fixture" is
			// self-contradictory with no calendar at all — so the two are gated
			// separately. An earlier version put both behind `len(event) > 0`,
			// which switched duplicate detection off for 2016-17 and 2017-18,
			// the seasons that publish no fixtures.csv, for a reason that
			// applies only to the other guard.
			if ev, ok := event[fid]; ok && ev != gw {
				guards.Misfiled++
				continue
			}
			if seen[[2]int{p.ID, fid}] {
				guards.Duplicate++
				continue
			}
			seen[[2]int{p.ID, fid}] = true
		}
		fn(rec, col, p, gw)
	}
	return guards, nil
}

func (s *Season) loadTeams(ctx context.Context) error {
	r, c, col, err := rows(ctx, s.Name, "teams.csv")
	if errors.Is(err, errNoSuchFile) {
		// 2018-19 and earlier publish no teams.csv. That makes the season unplayable
		// and leaves it perfectly good as a prior, so it is recorded rather than
		// refused. See Season.Absent.
		s.Absent = append(s.Absent, "teams.csv")
		return nil
	}
	if err != nil {
		return err
	}
	defer c.Close()
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("teams.csv: %w", err)
		}
		if id := ival(rec, col, "id"); id > 0 {
			// Strength is FPL's own pre-season assessment of each club, and the
			// archive's teams.csv is the pre-season snapshot — played and points
			// are zero — so it is a point-in-time-safe prior rather than
			// hindsight. It is the only club-specific thing known about a
			// promoted side before it kicks a ball, which is exactly the case a
			// team-strength blend has no prior for.
			s.Teams = append(s.Teams, fpl.Team{
				ID: id, Name: sval(rec, col, "name"), ShortName: sval(rec, col, "short_name"),
				Strength:            ival(rec, col, "strength"),
				StrengthOverallHome: ival(rec, col, "strength_overall_home"),
				StrengthOverallAway: ival(rec, col, "strength_overall_away"),
				StrengthAttackHome:  ival(rec, col, "strength_attack_home"),
				StrengthAttackAway:  ival(rec, col, "strength_attack_away"),
				StrengthDefenceHome: ival(rec, col, "strength_defence_home"),
				StrengthDefenceAway: ival(rec, col, "strength_defence_away"),
			})
		}
	}
	return nil
}

func (s *Season) loadFixtures(ctx context.Context) error {
	r, c, col, err := rows(ctx, s.Name, "fixtures.csv")
	if errors.Is(err, errNoSuchFile) {
		// Tolerated on the same terms as teams.csv, and for the same reason: a season
		// with no fixture list cannot be played and can still be a prior. 2018-19
		// does publish this one — verified by status code — so nothing exercises this
		// branch today; it is here because the two files are one requirement and
		// handling only the half that currently fails is how the next season back
		// becomes a surprise.
		s.Absent = append(s.Absent, "fixtures.csv")
		return nil
	}
	if err != nil {
		return err
	}
	defer c.Close()
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("fixtures.csv: %w", err)
		}
		// Renumbered with the same rule loadGameweeks uses, and it has to be: a
		// fixture list on 39-47 beside gameweek rows on 30-38 would make
		// TeamFixtures find nothing for the restart, so every player at every club
		// would read as blanking for the last nine gameweeks while still scoring
		// points. Two views of one quantity, and only one of them shifted.
		//
		// The upper bound is new alongside the renumber, and it is inert on every
		// season the record is measured on — checked against the payloads rather
		// than reasoned about: 2019-20 is the only season whose fixtures.csv carries
		// any event above 38, and it carries exactly 39-47. So this cannot quietly
		// drop a fixture the previous parser kept.
		ev := renumberGW(s.Name, ival(rec, col, "event"))
		if ev < 1 || ev > 38 {
			continue
		}
		e := ev
		// Scores are carried so a replay can rate teams from results actually
		// played. PointInTime is responsible for hiding the ones that had not
		// happened yet — they are hindsight everywhere else.
		f := fpl.Fixture{
			ID: ival(rec, col, "id"), Event: &e,
			TeamH: ival(rec, col, "team_h"), TeamA: ival(rec, col, "team_a"),
			TeamHDifficulty: ival(rec, col, "team_h_difficulty"),
			TeamADifficulty: ival(rec, col, "team_a_difficulty"),
		}
		if sval(rec, col, "team_h_score") != "" && sval(rec, col, "team_a_score") != "" {
			h, a := ival(rec, col, "team_h_score"), ival(rec, col, "team_a_score")
			f.TeamHScore, f.TeamAScore = &h, &a
		}
		// Kickoff time, which is what makes fixture congestion measurable:
		// rest days are the gap between a club's consecutive kickoffs, and an
		// international break is a gap in the calendar. Without it the archive
		// can say who played and not how crowded the week was.
		if v := sval(rec, col, "kickoff_time"); v != "" {
			if ko, err := time.Parse(time.RFC3339, v); err == nil {
				f.KickoffTime = &ko
			}
		}
		s.Fixtures = append(s.Fixtures, f)
	}
	return nil
}

func sval(rec []string, col map[string]int, name string) string {
	if i, ok := col[name]; ok && i < len(rec) {
		return rec[i]
	}
	return ""
}

func fval(rec []string, col map[string]int, name string) float64 {
	f, _ := strconv.ParseFloat(sval(rec, col, name), 64)
	return f
}

func ival(rec []string, col map[string]int, name string) int { return int(fval(rec, col, name)) }
