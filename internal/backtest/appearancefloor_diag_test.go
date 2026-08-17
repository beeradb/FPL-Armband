package backtest

// Two archive measurements about the appearance floor and about whether a nailed
// player stays nailed in a double gameweek.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagAppearanceFloor -v -timeout 30m
//	DIAG=1 go test ./internal/backtest -run TestDiagNailednessInDoubles -v -timeout 30m
//
// # What these are and are not
//
// They are **descriptive counts and rates over archive rows**. Nothing here
// replays a season, nothing scores a squad, and no figure below is a points
// verdict, a detection threshold or a case for moving a constant. The populations
// are large — hundreds of thousands of player-matches — so the season-clustering
// power problem that governs the replay does not apply to them, and neither does
// any of the machinery that goes with it.
//
// # Why they read matches rather than gameweeks
//
// `Player.GWs` is keyed by gameweek and a double's two matches are already summed
// into one entry by the time it exists. Three of FPL's channels are per-match step
// functions — the appearance floor, the concede block and the saves block — so a
// summed row cannot answer any question about a single match. Both diagnostics
// therefore walk `gameweekRows`, which yields one callback per surviving
// `merged_gw.csv` row with the gameweek renumbering and both row guards already
// applied. That walk is the loader's own, so the phantom-match and duplicate-row
// guards have one implementation rather than two — which matters more here than
// anywhere: a phantom match counted as real turns a single gameweek into a double,
// and telling those apart is exactly what the second diagnostic is for.
//
// # Where the calendar comes from
//
// A club-gameweek's fixture count comes from `fixtures.csv`, never from counting
// a player's rows. A player who appeared in one leg of a double and not the other
// contributes one row, and reading that as a single gameweek would drop precisely
// the rotated case the second diagnostic exists to find. The archive publishes no
// `fixtures.csv` for 2016-17 or 2017-18, so those two seasons carry no calendar
// and are excluded from everything about doubles and blanks. They stay in the
// first diagnostic, which needs no calendar.

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/stats"
)

// appearanceFloorSeasons is every season the archive publishes, oldest first.
//
// Deliberately not the replay's grid. These diagnostics do not replay anything,
// so the constraints that narrow the sweep grid — playability, a prior season, a
// comparable transfer rule — do not apply, and a wider archive is strictly more
// football. What each season can and cannot support is reported per table rather
// than assumed: `hasCalendar` gates the doubles work and `startsRecorded` gates
// anything reading the `starts` column.
var appearanceFloorSeasons = []string{
	"2016-17", "2017-18", "2018-19", "2019-20", "2020-21",
	"2021-22", "2022-23", "2023-24", "2024-25", "2025-26",
}

// mirrorArchive points `archiveURL` at a local cache of the real archive for the
// duration of one test, and restores it afterwards.
//
// # Why a mirror rather than more retries
//
// These two diagnostics read `gws/merged_gw.csv` for ten seasons, and unlike
// `Load` they cannot use the parsed-season cache — the whole point is to read the
// rows the parser accumulates away. That is forty megabytes of raw CSV per run
// from a host that returns 429 under load, and a diagnostic that fails on somebody
// else's traffic is a diagnostic nobody runs twice.
//
// It caches bytes, never parses, so what a season decodes to is unchanged. On a
// miss it fetches from the real archive with a small backoff; on a hit it serves
// from disk and the run is offline. The cache lives beside the parsed seasons, so
// one repository has one archive copy.
//
// ⚠️ It mutates a package-level var. That is what `archiveURL`'s own comment says
// the var is for, and both callers restore it through `t.Cleanup`; nothing in this
// package runs in parallel with them.
func mirrorArchive(t *testing.T, cfg config.Config) {
	t.Helper()
	dir := filepath.Join(cfg.CacheDir, "archive-mirror")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("no archive mirror (%v); reading the archive directly", err)
		return
	}
	upstream := archiveURL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/")
		local := filepath.Join(dir, filepath.FromSlash(rel))
		if b, err := os.ReadFile(local); err == nil {
			// A cached 404 is recorded as an empty file, because "this season
			// does not publish that file" is a fact worth caching: without it
			// every run re-asks the archive for 2016-17's teams.csv.
			if len(b) == 0 {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_, _ = w.Write(b)
			return
		}
		body, status := fetchWithBackoff(upstream + "/" + rel)
		if status == http.StatusNotFound {
			_ = os.MkdirAll(filepath.Dir(local), 0o755)
			_ = os.WriteFile(local, nil, 0o644)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if status != http.StatusOK {
			http.Error(w, "upstream "+strconv.Itoa(status), status)
			return
		}
		_ = os.MkdirAll(filepath.Dir(local), 0o755)
		_ = os.WriteFile(local, body, 0o644)
		_, _ = w.Write(body)
	}))
	archiveURL = srv.URL
	t.Cleanup(func() {
		archiveURL = upstream
		srv.Close()
	})
}

// fetchWithBackoff is the mirror's one upstream request, retried on the statuses
// that mean "busy" rather than "absent".
func fetchWithBackoff(url string) ([]byte, int) {
	delay := 2 * time.Second
	var status int
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, http.StatusInternalServerError
		}
		req.Header.Set("User-Agent", "armband/1.0 (+personal FPL analysis)")
		resp, err := (&http.Client{Timeout: 180 * time.Second}).Do(req)
		if err != nil {
			time.Sleep(delay)
			delay *= 2
			status = http.StatusServiceUnavailable
			continue
		}
		status = resp.StatusCode
		if status == http.StatusOK {
			b, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, http.StatusInternalServerError
			}
			return b, http.StatusOK
		}
		resp.Body.Close()
		if status == http.StatusNotFound {
			return nil, status
		}
		time.Sleep(delay)
		delay *= 2
	}
	return nil, status
}

// matchRow is one player's archive row for one match, priced.
type matchRow struct {
	Element int
	Club    int // from the fixture and was_home, not from players_raw
	Pos     int
	GW      int
	Fixture int
	Minutes int
	// Started is the archive's own `starts` column. -1 means the season records
	// none for this gameweek — an absent fact, which must never be read as a
	// player who did not start. `reconstructStarts` reconstructs it for the
	// loader, and its own boundary two forbids using a reconstruction for a
	// per-gameweek start classification, which is exactly what this file would be
	// doing. So the reconstruction is deliberately not consulted here.
	Started int
	Value   int // price in tenths, that gameweek
	Points  int // the archive's own total_points
	Comp    analysis.MatchPoints
}

// Appeared reports whether the player was on the pitch at all.
func (m matchRow) Appeared() bool { return m.Minutes > 0 }

// Sixty reports whether this match paid the second appearance point.
func (m matchRow) Sixty() bool { return m.Minutes >= analysis.AppearanceMinutes }

// seasonMatches is one archived season, per match, with its calendar.
type seasonMatches struct {
	Name string
	Rows []matchRow
	// Fixtures counts a club's matches in a gameweek: 0 blank, 1 single, 2+
	// double. Keyed [club][gw]. Empty when the season publishes no fixtures.csv.
	Fixtures map[int]map[int]int
	// StartsRecorded[gw] is true where the archive records at least one start in
	// that gameweek. Measured rather than asserted: `starts` is present-and-zero
	// for whole seasons, and a zero read as "nobody started" would make every
	// start rate below read zero for reasons that are not football.
	StartsRecorded map[int]bool
	Players        map[int]*Player
	Rules          analysis.ScoringRules
	DefConPaid     bool
	HasCalendar    bool
}

// clubGWFixtures is how many matches a club played in a gameweek.
func (s *seasonMatches) clubGWFixtures(club, gw int) int {
	if m, ok := s.Fixtures[club]; ok {
		return m[gw]
	}
	return 0
}

// loadMatches reads one season down to the match, priced under that season's
// rules.
func loadMatches(t *testing.T, cfg config.Config, name string) *seasonMatches {
	t.Helper()
	season := loadSeason(t, cfg, name)

	sm := &seasonMatches{
		Name:           name,
		Fixtures:       map[int]map[int]int{},
		StartsRecorded: map[int]bool{},
		Players:        season.Players,
		Rules:          analysis.ScoringRulesFor(name),
		DefConPaid:     DefconScoredIn(name),
	}
	// The calendar. `f.Event` is the gameweek; a fixture with none was never
	// scheduled and cannot be a club's match in any week.
	for _, f := range season.Fixtures {
		if f.Event == nil {
			continue
		}
		// Already renumbered and already bounded by `loadFixtures`, which applies
		// the same 2019-20 rule `gameweekRows` applies to the weekly rows — the
		// two views have to shift together or the calendar and the results
		// disagree for the restart's nine gameweeks. Re-checking the bound rather
		// than re-applying the shift, so this cannot become a second renumbering.
		gw := *f.Event
		if gw < 1 || gw > 38 {
			continue
		}
		for _, club := range []int{f.TeamH, f.TeamA} {
			if sm.Fixtures[club] == nil {
				sm.Fixtures[club] = map[int]int{}
			}
			sm.Fixtures[club][gw]++
		}
	}
	sm.HasCalendar = len(sm.Fixtures) > 0

	// Which club a row belongs to, from the fixture and the row's own was_home —
	// point-in-time and exact, where `players_raw.team` is an end-of-season
	// snapshot that puts a January transfer at the wrong club for half a season.
	side := make(map[int][2]int, len(season.Fixtures))
	for _, f := range season.Fixtures {
		side[f.ID] = [2]int{f.TeamH, f.TeamA}
	}

	_, err := season.gameweekRows(context.Background(),
		func(rec []string, col map[string]int, p *Player, gw int) {
			fid := ival(rec, col, "fixture")
			club := p.Team
			if s, ok := side[fid]; ok {
				if sval(rec, col, "was_home") == "True" || sval(rec, col, "was_home") == "true" {
					club = s[0]
				} else {
					club = s[1]
				}
			}
			starts := -1
			if _, ok := col["starts"]; ok {
				starts = ival(rec, col, "starts")
				if starts > 0 {
					sm.StartsRecorded[gw] = true
				}
			}
			r := matchRow{
				Element: p.ID,
				Club:    club,
				Pos:     p.Type,
				GW:      gw,
				Fixture: fid,
				Minutes: ival(rec, col, "minutes"),
				Started: starts,
				Value:   ival(rec, col, "value"),
				Points:  ival(rec, col, "total_points"),
			}
			// Element type 5 is 2024-25's assistant managers, which the scoring
			// table prices nothing for and `DecomposeMatch` refuses outright.
			// They record no minutes on any row and are not footballers.
			if !sm.Rules.Prices(p.Type) {
				return
			}
			r.Comp = analysis.DecomposeMatch(analysis.RealisedMatch{
				Position:        p.Type,
				Minutes:         r.Minutes,
				Goals:           ival(rec, col, "goals_scored"),
				Assists:         ival(rec, col, "assists"),
				CleanSheets:     ival(rec, col, "clean_sheets"),
				GoalsConceded:   ival(rec, col, "goals_conceded"),
				Saves:           ival(rec, col, "saves"),
				Bonus:           ival(rec, col, "bonus"),
				Yellow:          ival(rec, col, "yellow_cards"),
				Red:             ival(rec, col, "red_cards"),
				OwnGoals:        ival(rec, col, "own_goals"),
				PenaltiesSaved:  ival(rec, col, "penalties_saved"),
				PenaltiesMissed: ival(rec, col, "penalties_missed"),
				DefCon:          ival(rec, col, "defensive_contribution"),
				DefConPaid:      sm.DefConPaid,
			}, sm.Rules)
			sm.Rows = append(sm.Rows, r)
		})
	if err != nil {
		t.Skipf("%s: archive unreachable: %v", name, err)
	}
	// ⚠️ **Present-and-zero is not a recorded zero.** `starts` exists as a COLUMN
	// for the whole of 2021-22 and for 2022-23's first fifteen gameweeks while
	// carrying zero on every row — the defect `reconstructStarts` documents and
	// counts. Column presence therefore cannot gate a start rate: those rows would
	// enter as "did not start" and drag the rate down for reasons that are not
	// football, and they would do it unevenly, because a season's doubles are not
	// spread evenly across its gameweeks.
	//
	// So the gate is the per-gameweek fact, measured on this pass: a gameweek with
	// no recorded start anywhere in the league has none for anybody, and the field
	// goes back to the absent marker.
	for i := range sm.Rows {
		if !sm.StartsRecorded[sm.Rows[i].GW] {
			sm.Rows[i].Started = -1
		}
	}

	// Deterministic order. gameweekRows preserves the file's order, which is
	// stable, but nothing downstream should depend on that.
	sort.Slice(sm.Rows, func(i, j int) bool {
		a, b := sm.Rows[i], sm.Rows[j]
		if a.GW != b.GW {
			return a.GW < b.GW
		}
		if a.Element != b.Element {
			return a.Element < b.Element
		}
		return a.Fixture < b.Fixture
	})
	return sm
}

// posName is FPL's short name for an element type.
func posName(t int) string {
	switch t {
	case 1:
		return "GKP"
	case 2:
		return "DEF"
	case 3:
		return "MID"
	case 4:
		return "FWD"
	}
	return "?"
}

// priceBrackets are absolute FPL prices in tenths.
//
// Absolute rather than within-season percentiles, because the argument under test
// is about the game's own price ladder — "a premium", "bench fodder" — and those
// are absolute in a manager's head. The cost is that prices inflate across a
// decade of archive, so the top bracket is thinner in 2016-17 than in 2025-26;
// the top-N-by-points populations below are the season-relative counterpart and
// the two should be read together.
var priceBrackets = []struct {
	Lo, Hi int // tenths, [Lo, Hi)
	Label  string
}{
	{0, 50, "<5.0"},
	{50, 65, "5.0-6.4"},
	{65, 80, "6.5-7.9"},
	{80, 100, "8.0-9.9"},
	{100, 1 << 30, "10.0+"},
}

func priceBracket(v int) string {
	for _, b := range priceBrackets {
		if v >= b.Lo && v < b.Hi {
			return b.Label
		}
	}
	return "?"
}

// topByPoints returns the top n elements of a season by end-of-season total
// points.
//
// ⚠️ **End-stamped by construction.** It selects on the outcome, so it is the
// right population for "what did the season's best players' returns look like"
// and the wrong one for anything predictive. Where a prior-information version of
// the same population is wanted, the 10.0+ price bracket is it: a price is set
// before the deadline and is the market's view rather than the result.
func topByPoints(sm *seasonMatches, n int) map[int]bool {
	type pt struct{ id, points int }
	var all []pt
	for id, p := range sm.Players {
		if sm.Rules.Prices(p.Type) {
			all = append(all, pt{id, p.TotalPoints})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].points != all[j].points {
			return all[i].points > all[j].points
		}
		return all[i].id < all[j].id // deterministic ties
	})
	out := map[int]bool{}
	for i := 0; i < n && i < len(all); i++ {
		out[all[i].id] = true
	}
	return out
}

// quartiles returns Q1, the median and Q3, by the median-of-halves definition.
//
// `stats.Median` is the one implementation of the middle value in this repository
// and the halves go through it too, so nothing here re-spells the idiom the
// package guard exists to catch.
func quartiles(xs []float64) (q1, med, q3 float64) {
	if len(xs) == 0 {
		return 0, 0, 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	med = stats.Median(s)
	half := len(s) / 2
	lower := s[:half]
	upper := s[len(s)-half:]
	return stats.Median(lower), med, stats.Median(upper)
}

func rate(num, den int) float64 {
	if den == 0 {
		return math.NaN()
	}
	return float64(num) / float64(den)
}

// TestDiagAppearanceFloor measures how much of a player's realised return is the
// appearance floor, and decomposes the rest.
func TestDiagAppearanceFloor(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	mirrorArchive(t, cfg)

	var seasons []*seasonMatches
	for _, name := range appearanceFloorSeasons {
		seasons = append(seasons, loadMatches(t, cfg, name))
	}

	reportDecompositionReconciles(t, seasons)
	reportAppearanceShare(t, seasons)
	reportComponentsByPosition(t, seasons)
	reportExtraMatchSpread(t, seasons)
	reportSixtyRate(t, seasons)
}

// reportDecompositionReconciles checks the priced decomposition against the
// archive's own total_points, per season.
//
// This is the measurement that makes every table below readable. The four
// channels come from the season's own `ScoringRulesFor` table; the rest are
// package constants applied to every season, which is an exposure
// `ScoringRules`' docstring already declares. A season where one of those rules
// really did change shows up here as a non-zero residual on the rows it changed,
// rather than as a silent mis-attribution inside a component column.
func reportDecompositionReconciles(t *testing.T, seasons []*seasonMatches) {
	t.Log("== decomposition against the archive's own total_points, per season ==")
	t.Log("season   rows      exact   mean resid   |resid|>0 rows   worst")
	for _, sm := range seasons {
		var exact, off int
		var sum, worst float64
		for _, r := range sm.Rows {
			d := r.Comp.Total() - float64(r.Points)
			sum += d
			if d == 0 {
				exact++
			} else {
				off++
				if math.Abs(d) > math.Abs(worst) {
					worst = d
				}
			}
		}
		t.Logf("%-8s %6d  %9.5f  %+10.4f  %8d         %+6.0f",
			sm.Name, len(sm.Rows), rate(exact, len(sm.Rows)),
			sum/math.Max(1, float64(len(sm.Rows))), off, worst)
	}
}

// share is the appearance share of a population's returns, both ways round.
type share struct {
	Matches    int
	Appearance float64
	Total      float64
	// PerPlayer holds one share per player, for the distribution. A player's
	// share is his summed appearance points over his summed total points.
	PerPlayer []float64
}

func (s *share) add(r matchRow) {
	s.Matches++
	s.Appearance += r.Comp.Appearance
	s.Total += float64(r.Points)
}

// Pooled is the ratio of totals — the estimator that weights a player by how much
// he returned.
func (s *share) Pooled() float64 {
	if s.Total == 0 {
		return math.NaN()
	}
	return s.Appearance / s.Total
}

// minMatchesForAShare is how many matches a player must have played before his
// own appearance share is reported in a distribution.
//
// **Asserted.** A share is a ratio and its denominator is the player's realised
// points, so a two-match cameo returning 2 points reads as a share of 1.00 and a
// two-match cameo returning 0 reads as undefined. Ten matches is enough that the
// denominator is not one lucky goal; nothing was swept and no value was chosen
// against an outcome. The pooled estimator beside every distribution is
// unfiltered, so the filter's effect is visible rather than assumed.
const minMatchesForAShare = 10

// reportAppearanceShare is measurement one's headline table.
func reportAppearanceShare(t *testing.T, seasons []*seasonMatches) {
	// Buckets: by position, by price bracket, and by the two top-N populations.
	byPos := map[string]*share{}
	byPrice := map[string]*share{}
	byElite := map[string]*share{}
	perPlayer := map[string]map[[2]int]*share{} // bucket -> (season index, element)

	touch := func(m map[string]*share, k string) *share {
		if m[k] == nil {
			m[k] = &share{}
		}
		return m[k]
	}
	touchPlayer := func(bucket string, si, el int) *share {
		if perPlayer[bucket] == nil {
			perPlayer[bucket] = map[[2]int]*share{}
		}
		k := [2]int{si, el}
		if perPlayer[bucket][k] == nil {
			perPlayer[bucket][k] = &share{}
		}
		return perPlayer[bucket][k]
	}

	for si, sm := range seasons {
		top30 := topByPoints(sm, 30)
		top100 := topByPoints(sm, 100)
		for _, r := range sm.Rows {
			// Appeared only. A match a player did not play pays nothing and
			// deducts nothing, so including it measures selection rather than
			// the floor.
			if !r.Appeared() {
				continue
			}
			pos := posName(r.Pos)
			touch(byPos, pos).add(r)
			touchPlayer(pos, si, r.Element).add(r)

			pb := priceBracket(r.Value)
			touch(byPrice, pb).add(r)
			touchPlayer("price "+pb, si, r.Element).add(r)

			touch(byElite, "everyone").add(r)
			touchPlayer("everyone", si, r.Element).add(r)
			if top100[r.Element] {
				touch(byElite, "top 100 by points").add(r)
				touchPlayer("top 100 by points", si, r.Element).add(r)
			}
			if top30[r.Element] {
				touch(byElite, "top 30 by points").add(r)
				touchPlayer("top 30 by points", si, r.Element).add(r)
			}
		}
	}

	line := func(label string, s *share, bucket string) string {
		var shares []float64
		for _, p := range perPlayer[bucket] {
			if p.Matches >= minMatchesForAShare && p.Total > 0 {
				shares = append(shares, p.Appearance/p.Total)
			}
		}
		q1, med, q3 := quartiles(shares)
		return fmt.Sprintf("%-18s %8d %9.3f %9.3f    %6.3f %6.3f %6.3f  %6d",
			label, s.Matches, s.Total/float64(s.Matches), s.Pooled(), q1, med, q3, len(shares))
	}

	head := fmt.Sprintf("%-18s %8s %9s %9s    %6s %6s %6s  %6s",
		"bucket", "matches", "pts/match", "pooled", "Q1", "med", "Q3", "players")
	t.Log("== appearance points as a share of realised return, per match played ==")
	t.Log("`pooled` is the ratio of summed appearance to summed points; Q1/med/Q3 are the")
	t.Logf("distribution of the same ratio computed per player-season, over players with >= %d matches.",
		minMatchesForAShare)
	t.Log(head)
	for _, p := range []string{"GKP", "DEF", "MID", "FWD"} {
		if s := byPos[p]; s != nil {
			t.Log(line(p, s, p))
		}
	}
	t.Log("")
	for _, b := range priceBrackets {
		if s := byPrice[b.Label]; s != nil {
			t.Log(line("price "+b.Label, s, "price "+b.Label))
		}
	}
	t.Log("")
	for _, b := range []string{"everyone", "top 100 by points", "top 30 by points"} {
		if s := byElite[b]; s != nil {
			t.Log(line(b, s, b))
		}
	}
}

// reportComponentsByPosition decomposes an appeared match into every channel FPL
// pays, per position and for the elite populations.
func reportComponentsByPosition(t *testing.T, seasons []*seasonMatches) {
	type acc struct {
		n    int
		comp analysis.MatchPoints
		tot  []float64
	}
	buckets := map[string]*acc{}
	order := []string{}
	touch := func(k string) *acc {
		if buckets[k] == nil {
			buckets[k] = &acc{}
			order = append(order, k)
		}
		return buckets[k]
	}
	add := func(k string, r matchRow) {
		a := touch(k)
		a.n++
		a.comp.Appearance += r.Comp.Appearance
		a.comp.Goals += r.Comp.Goals
		a.comp.Assists += r.Comp.Assists
		a.comp.CleanSheet += r.Comp.CleanSheet
		a.comp.Conceded += r.Comp.Conceded
		a.comp.Saves += r.Comp.Saves
		a.comp.DefCon += r.Comp.DefCon
		a.comp.Bonus += r.Comp.Bonus
		a.comp.Cards += r.Comp.Cards
		a.comp.OwnGoals += r.Comp.OwnGoals
		a.comp.Penalties += r.Comp.Penalties
		a.tot = append(a.tot, float64(r.Points))
	}

	for _, sm := range seasons {
		top30 := topByPoints(sm, 30)
		for _, r := range sm.Rows {
			// Conditional on clearing the hour. That is the population the
			// argument is about — a player "nailed to play 70+" — and it is the
			// conditioning that makes the appearance channel exactly 2 with zero
			// variance, so the deterministic share below is a fact rather than an
			// estimate.
			if !r.Sixty() {
				continue
			}
			add(posName(r.Pos), r)
			if top30[r.Element] {
				add("top30 "+posName(r.Pos), r)
			}
		}
	}

	// Map iteration seeds `order` through first touch, and the row order of the
	// archive is not a thing a table should depend on. Sorted, so two runs print
	// the same table.
	sort.Strings(order)
	t.Log("== realised points per match, decomposed, conditional on 60+ minutes ==")
	t.Log("Every column is a mean per such match. `appear` is deterministic given selection;")
	t.Log("every other column is stochastic. `det` is appear/points.")
	t.Logf("%-11s %7s %6s %6s %6s %6s %6s %6s %6s %6s %6s %6s  %6s %6s %5s",
		"bucket", "n", "appear", "goals", "assist", "cs", "conc", "saves",
		"defcon", "bonus", "cards", "other", "points", "sd", "det")
	for _, k := range order {
		a := buckets[k]
		f := 1 / float64(a.n)
		mean := meanOf(a.tot)
		t.Logf("%-11s %7d %6.3f %6.3f %6.3f %6.3f %6.3f %6.3f %6.3f %6.3f %6.3f %6.3f  %6.3f %6.3f %5.3f",
			k, a.n,
			a.comp.Appearance*f, a.comp.Goals*f, a.comp.Assists*f, a.comp.CleanSheet*f,
			a.comp.Conceded*f, a.comp.Saves*f, a.comp.DefCon*f, a.comp.Bonus*f,
			a.comp.Cards*f, (a.comp.OwnGoals+a.comp.Penalties)*f,
			mean, sd(a.tot), a.comp.Appearance*f/mean)
	}
}

// reportExtraMatchSpread is what an extra match is worth and how uncertain it is.
//
// The mean is one half of the answer and the spread is the other, because the
// claim under test is about certainty rather than about size. A position whose
// extra match has a higher mean and a much wider spread is buying a different
// thing from one whose extra match is mostly the floor.
func reportExtraMatchSpread(t *testing.T, seasons []*seasonMatches) {
	type acc struct{ xs []float64 }
	buckets := map[string]*acc{}
	order := []string{}
	add := func(k string, v float64) {
		if buckets[k] == nil {
			buckets[k] = &acc{}
			order = append(order, k)
		}
		buckets[k].xs = append(buckets[k].xs, v)
	}
	for _, sm := range seasons {
		top30 := topByPoints(sm, 30)
		for _, r := range sm.Rows {
			if !r.Sixty() {
				continue
			}
			add(posName(r.Pos), float64(r.Points))
			if top30[r.Element] {
				add("top30 "+posName(r.Pos), float64(r.Points))
			}
			if r.Value >= 100 {
				add("10.0m+ "+posName(r.Pos), float64(r.Points))
			}
		}
	}
	sort.Strings(order)
	t.Log("== the distribution of one 60+ match's points ==")
	t.Logf("%-14s %8s %7s %7s %6s %6s %6s %7s %7s",
		"bucket", "n", "mean", "sd", "Q1", "med", "Q3", "P(<=2)", "P(>=8)")
	for _, k := range order {
		xs := buckets[k].xs
		q1, med, q3 := quartiles(xs)
		var floorOnly, hauls int
		for _, x := range xs {
			if x <= 2 {
				floorOnly++
			}
			if x >= 8 {
				hauls++
			}
		}
		t.Logf("%-14s %8d %7.3f %7.3f %6.1f %6.1f %6.1f %7.3f %7.3f",
			k, len(xs), meanOf(xs), sd(xs), q1, med, q3,
			rate(floorOnly, len(xs)), rate(hauls, len(xs)))
	}
}

// substantialAppearanceRate is the share of his club's matches a player must have
// appeared in before he counts as "gets fielded".
//
// **Asserted**, and chosen to be a low bar deliberately: the question it serves is
// what fraction of a fielded player's matches clear the hour, and setting the bar
// on 60+ minutes would put the answer in the definition. It is set on appearances,
// which is a different event.
const substantialAppearanceRate = 0.5

// reportSixtyRate answers what fraction of a fielded player's matches clear the
// hour, which is what decides whether his floor is 2 or 1.
func reportSixtyRate(t *testing.T, seasons []*seasonMatches) {
	type acc struct{ appeared, sixty int }
	buckets := map[string]*acc{}
	order := []string{}
	add := func(k string, r matchRow) {
		if buckets[k] == nil {
			buckets[k] = &acc{}
			order = append(order, k)
		}
		buckets[k].appeared++
		if r.Sixty() {
			buckets[k].sixty++
		}
	}
	for _, sm := range seasons {
		if !sm.HasCalendar {
			continue // needs the club's match count as a denominator
		}
		// Appearances per player, against his club's matches.
		appearances := map[int]int{}
		clubOf := map[int]int{}
		for _, r := range sm.Rows {
			if r.Appeared() {
				appearances[r.Element]++
			}
			clubOf[r.Element] = r.Club
		}
		clubMatches := map[int]int{}
		for club, byGW := range sm.Fixtures {
			for _, n := range byGW {
				clubMatches[club] += n
			}
		}
		fielded := map[int]bool{}
		for el, n := range appearances {
			if m := clubMatches[clubOf[el]]; m > 0 &&
				float64(n)/float64(m) >= substantialAppearanceRate {
				fielded[el] = true
			}
		}
		top30 := topByPoints(sm, 30)
		for _, r := range sm.Rows {
			if !r.Appeared() || !fielded[r.Element] {
				continue
			}
			add(posName(r.Pos), r)
			add("all positions", r)
			if top30[r.Element] {
				add("top30", r)
			}
		}
	}
	t.Logf("== of a fielded player's appearances, what share clears %d minutes ==",
		analysis.AppearanceMinutes)
	t.Logf("Fielded means he appeared in at least %.0f%% of his club's matches that season.",
		substantialAppearanceRate*100)
	t.Logf("%-14s %9s %9s %8s", "bucket", "appeared", "60+", "rate")
	sort.Strings(order)
	for _, k := range order {
		a := buckets[k]
		t.Logf("%-14s %9d %9d %8.4f", k, a.appeared, a.sixty, rate(a.sixty, a.appeared))
	}
}
