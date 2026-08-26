package backtest

// Expected goals conceded from a measured per-match source, in place of the
// reconstruction, for the seasons FPL never backfilled.
//
// # What this replaces and why it is worth replacing
//
// `xgcrepair.go` reconstructs xGC by sharing each club's OPPONENTS' expected
// goals out across its players' minutes. That is a transport step: it moves a
// quantity measured at one club onto another club's players, and it carries a
// 16-20% error against a directly measured opponent-xG. This reads the measured
// number instead and prorates it the same way.
//
// # ⚠️ The DEFAULT IS THE RECONSTRUCTION, deliberately, and that is not a
// hedge
//
// The external source is selected only when `Config.XGCExternalDir` names a
// directory (or `FPL_XGC_EXTERNAL_DIR` does). A clone with no such directory —
// which is every public clone — reads the reconstruction and behaves exactly as
// it did before this file existed. The shipped `config.json` does not set it.
//
// That split is a licensing constraint, not a taste: the per-match data this
// reads is a private research cache, and the repository is public. **Nothing in
// this file names its origin, its fetch route, or any figure derived from it.**
// It reads a CSV from a directory the operator names, and the operator supplies
// the directory.
//
// # ⚠️ Selection is EXPLICIT and missing data is an ERROR, never a fallback
//
// The tempting design is "use the external source when it is present". That is
// the failure this package already has on record twice: `configPath` accepting
// any non-empty `FPL_CONFIG` with no log line, and the xG repair applied inside
// `fetch` where the escape hatch became a no-op that looked like a null result.
// Presence-detection means a run cannot say which source it used, and two runs
// that disagree look like a data change.
//
// So: naming a directory that does not resolve to usable data for a season that
// needs it is a **hard error out of `Load`**. A run either has the external
// source for every season it repairs, or it does not start.
//
// # Ordering: before the xG repair, not after
//
// `applyXGCRepair` fills only a zero. Writing the measured values first
// therefore makes the reconstruction a no-op exactly where this source reached,
// and leaves it in charge everywhere it did not — which is the "maintain both"
// requirement expressed as ordering rather than as a branch. `xgcRepairResult`'s
// `Skipped` counter is what the overlap shows up in.
//
// # ⚠️ An estimator swap reads as a data change, at maximum scope
//
// Every figure measured on the reconstruction is incomparable with one measured
// on this source. That is not a caveat on this file; it is the price of using
// it, and `Season.XGCExternal` exists so a cell's provenance can say which arm
// produced it rather than leaving a reader to guess.

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// xgcExternalSeasons are the seasons this source can supply.
//
// ⚠️ **DECLARED, not inferred from which files happen to be present.** The
// directory also holds partial captures for 2018-19 and 2019-20, and those two
// seasons have no expected goals AT SOURCE — so a file with three of 380 rows in
// it is not a season this can repair, and treating "a file exists" as "the
// season is covered" would fill two seasons with almost nothing and report
// success. The list is the decision; the file check below is the verification.
//
// 2022-23 is here for GW1-15 only, which is not stated here but in `xgRepairs` —
// the window that keeps this out of the native rows from GW16 is the same window
// the reconstruction uses, and there must not be a second copy of it.
var xgcExternalSeasons = map[string]bool{
	"2020-21": true,
	"2021-22": true,
	"2022-23": true,
}

// externalClubShort maps the per-match source's club names onto FPL's
// three-letter short name.
//
// ⚠️ **Through the SHORT NAME, and any unmapped club is a hard error.** The two
// naming schemes agree on fewer than half the league — "Brighton" against
// "Brighton & Hove Albion", "Spurs" against "Tottenham Hotspur" — so a join on
// the full name silently drops the clubs that differ and computes a
// confident-looking answer on the rest. Short names are also stable across
// seasons, where FPL's display names are not.
//
// All 28 clubs appearing in the covered seasons are here, and
// `TestExternalXGCClubNamesAllResolve` fails if the source ever names a
// twenty-ninth.
var externalClubShort = map[string]string{
	"AFC Bournemouth":         "BOU",
	"Arsenal":                 "ARS",
	"Aston Villa":             "AVL",
	"Brentford":               "BRE",
	"Brighton & Hove Albion":  "BHA",
	"Burnley":                 "BUR",
	"Chelsea":                 "CHE",
	"Crystal Palace":          "CRY",
	"Everton":                 "EVE",
	"Fulham":                  "FUL",
	"Ipswich Town":            "IPS",
	"Leeds United":            "LEE",
	"Leicester City":          "LEI",
	"Liverpool":               "LIV",
	"Luton Town":              "LUT",
	"Manchester City":         "MCI",
	"Manchester United":       "MUN",
	"Newcastle United":        "NEW",
	"Norwich City":            "NOR",
	"Nottingham Forest":       "NFO",
	"Sheffield United":        "SHU",
	"Southampton":             "SOU",
	"Sunderland":              "SUN",
	"Tottenham Hotspur":       "TOT",
	"Watford":                 "WAT",
	"West Bromwich Albion":    "WBA",
	"West Ham United":         "WHU",
	"Wolverhampton Wanderers": "WOL",
}

// XGCExternalResult reports what the external source did to one season, and is
// `json:"-"` for the same reason `XGRepair` is: it describes this load, not the
// season.
type XGCExternalResult struct {
	// Dir is the directory the values came from. Empty means the external
	// source was not selected and the reconstruction is in charge.
	Dir string
	// Matches is how many per-match rows were read and joined.
	Matches int
	// Applied, Skipped and Empty mirror xgcRepairResult's counters so the two
	// arms are read the same way.
	Applied, Skipped, Empty int
	// SumXGC is the total written, which is what a provenance line can compare
	// between arms without re-reading every cell.
	SumXGC float64
}

var (
	xgcExternalMu  sync.RWMutex
	xgcExternalDir string
)

// SetXGCExternalDir selects the external source for every subsequent season
// load. The empty string selects the reconstruction, which is the default.
//
// Called once, from wherever config is resolved. It is not per-cell state: the
// source is a property of the archive a process is replaying, and a sweep whose
// arms disagreed about it would be comparing two data sets while reporting one.
// `seasonCache` is keyed on the resolved directory so a process that does change
// it cannot serve a season loaded under the other arm.
func SetXGCExternalDir(dir string) {
	xgcExternalMu.Lock()
	defer xgcExternalMu.Unlock()
	xgcExternalDir = dir
}

// externalXGCDir resolves the directory, environment first.
//
// The environment override exists so a diagnostic can run both arms without
// editing config, and it is fingerprinted in internal/snapshot alongside the
// three repair switches — an unfingerprinted switch that changes what a season
// holds is how two incomparable sweeps come to look like one.
func externalXGCDir() string {
	if d := strings.TrimSpace(os.Getenv("FPL_XGC_EXTERNAL_DIR")); d != "" {
		return d
	}
	xgcExternalMu.RLock()
	defer xgcExternalMu.RUnlock()
	return xgcExternalDir
}

// externalXGCPath is where one season's per-match file lives.
func externalXGCPath(dir, season string) string {
	return filepath.Join(dir, "xgc-"+season+".csv")
}

// applyExternalXGC writes the measured per-match xGC into one season, prorated
// across each player's minutes by exactly the code the reconstruction prorates
// with.
//
// Returns a nil error and a zero result when the external source is not
// selected, or when this season is not one it covers — those are decisions, not
// failures. It errors when the source IS selected and the data is unusable,
// which is the case that must never degrade quietly.
func (s *Season) applyExternalXGC() (XGCExternalResult, error) {
	var res XGCExternalResult
	dir := externalXGCDir()
	if dir == "" {
		return res, nil
	}
	if !xgcExternalSeasons[s.Name] {
		// Not an error: 2023-24 onward carry native xGC and need nothing, and
		// 2018-19/2019-20 cannot be supplied by this source at all.
		return res, nil
	}
	spec, ok := xgRepairs[s.Name]
	if !ok {
		return res, fmt.Errorf("external xGC: %s is declared covered but has no "+
			"repair window in xgRepairs — the two tables have diverged", s.Name)
	}

	clubXGC, matches, err := externalClubXGC(s, dir)
	if err != nil {
		return res, err
	}
	res.Dir, res.Matches = dir, matches

	rec, _ := proratedClubXGC(s, clubXGC)
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		for _, gw := range sortedGameweeks(rec[id]) {
			if gw < spec.FirstGW || gw > spec.LastGW {
				continue
			}
			g := p.GWs[gw]
			if g.XGC != 0 {
				res.Skipped++
				continue
			}
			if rec[id][gw] == 0 {
				res.Empty++
				continue
			}
			g.XGC = rec[id][gw]
			// Flagged as reconstructed even though it is measured, because every
			// consumer of that flag asks "is this row FPL's own published xGC?"
			// and the answer is still no. A second flag meaning "measured
			// elsewhere" would need every one of those consumers to decide about
			// it, and none of them has a reason to.
			g.XGCReconstructed = true
			p.GWs[gw] = g
			res.Applied++
			res.SumXGC += g.XGC
		}
	}
	return res, nil
}

// externalClubXGC reads one season's per-match file and returns each club's
// expected goals conceded by gameweek — the same shape, and the same meaning, as
// clubXGAPerGameweek's return.
//
// ⚠️ **The join is on the CLUB PAIR, not on the file's own round number.** A
// round is not an FPL gameweek: postponed matches are replayed into a different
// gameweek and that is exactly where double gameweeks come from, so joining on
// the round would file a rearranged fixture under the week it was originally
// scheduled for and quietly move a double. Each ordered (home, away) pair occurs
// once in a league season, so the pair identifies the fixture and FPL's own
// `Event` supplies the gameweek.
func externalClubXGC(s *Season, dir string) (map[[2]int]float64, int, error) {
	path := externalXGCPath(dir, s.Name)
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("external xGC for %s: %w — the external source "+
			"is selected, so this is a hard error rather than a fall back to the "+
			"reconstruction; unset it to use the reconstruction deliberately", s.Name, err)
	}
	defer f.Close()

	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, 0, fmt.Errorf("external xGC for %s: %s: %w", s.Name, path, err)
	}
	if len(recs) < 2 {
		return nil, 0, fmt.Errorf("external xGC for %s: %s has %d rows",
			s.Name, path, len(recs))
	}
	col := map[string]int{}
	for i, h := range recs[0] {
		col[strings.TrimSpace(h)] = i
	}
	for _, want := range []string{"home_team", "away_team", "home_xgc", "away_xgc"} {
		if _, ok := col[want]; !ok {
			return nil, 0, fmt.Errorf("external xGC for %s: %s has no %q column",
				s.Name, path, want)
		}
	}

	byShort := map[string]int{}
	for _, t := range s.Teams {
		byShort[t.ShortName] = t.ID
	}
	teamID := func(name string) (int, error) {
		short, ok := externalClubShort[strings.TrimSpace(name)]
		if !ok {
			return 0, fmt.Errorf("external xGC for %s: club %q is not in "+
				"externalClubShort — add it rather than letting the row drop, "+
				"because a dropped club is a silently smaller comparison", s.Name, name)
		}
		id, ok := byShort[short]
		if !ok {
			return 0, fmt.Errorf("external xGC for %s: club %q maps to short name "+
				"%q, which is not in this season's teams table", s.Name, name, short)
		}
		return id, nil
	}

	// The fixture index, keyed on the ordered club pair.
	event := map[[2]int]int{}
	for _, fx := range s.Fixtures {
		if fx.Event == nil {
			continue
		}
		event[[2]int{fx.TeamH, fx.TeamA}] = *fx.Event
	}

	out := make(map[[2]int]float64, 20*38)
	var matched int
	for _, rec := range recs[1:] {
		if len(rec) < len(recs[0]) {
			continue
		}
		h, err := teamID(rec[col["home_team"]])
		if err != nil {
			return nil, 0, err
		}
		a, err := teamID(rec[col["away_team"]])
		if err != nil {
			return nil, 0, err
		}
		gw, ok := event[[2]int{h, a}]
		if !ok {
			// A fixture the archive does not carry under this pair. Not silently
			// skipped: the two sides are meant to be the same 380 matches, and a
			// miss means the join is wrong rather than that a match is missing.
			return nil, 0, fmt.Errorf("external xGC for %s: no archived fixture "+
				"for club pair (%d, %d) — the per-match file and the archive "+
				"disagree about this season's fixtures", s.Name, h, a)
		}
		hx, err := strconv.ParseFloat(strings.TrimSpace(rec[col["home_xgc"]]), 64)
		if err != nil {
			return nil, 0, fmt.Errorf("external xGC for %s: home_xgc: %w", s.Name, err)
		}
		ax, err := strconv.ParseFloat(strings.TrimSpace(rec[col["away_xgc"]]), 64)
		if err != nil {
			return nil, 0, fmt.Errorf("external xGC for %s: away_xgc: %w", s.Name, err)
		}
		// Summed rather than assigned: a double gameweek puts two matches in one
		// (club, gameweek), and the reconstruction's own map accumulates for the
		// same reason.
		out[[2]int{h, gw}] += hx
		out[[2]int{a, gw}] += ax
		matched++
	}
	if matched == 0 {
		return nil, 0, fmt.Errorf("external xGC for %s: %s joined no matches",
			s.Name, path)
	}
	return out, matched, nil
}

// sortedGameweeksOfClubMap is unused by the overlay itself and exists so the
// club map can be walked reproducibly in tests and diagnostics. Floating-point
// addition is not associative and this feeds a score, which feeds a discrete
// fifteen — the same argument clubXGPerGameweek carries.
func sortedClubKeys(m map[[2]int]float64) [][2]int {
	out := make([][2]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}
