package backtest

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// repairData holds the per-gameweek figures the archive is missing.
//
// Embedded rather than read from a path, so the repair travels with the binary and
// cannot resolve differently depending on which directory a test was invoked from.
// `go:embed` reaches only inside its own package tree, which is why the file lives
// here rather than beside the other data in the repository root.
//
//go:embed repairdata/*.csv repairdata/*.json
var repairData embed.FS

// xgRepair describes one season's expected-goals backfill.
//
// # Two different defects, and the difference is the whole caveat
//
// 2022-23 has a **hole in a series that exists**: `gws/merged_gw.csv` carries
// `expected_goals` as zero for gameweeks 1-15 with entirely normal minutes, and the
// series begins at GW16. FPL backfilled the season totals when it introduced the
// statistic in December 2022 and never backfilled the weekly history. So the provider
// offset is fitted *within that season* on GW16-38, and `players_raw.csv`'s complete
// aggregate — which the repair never sees — is an independent validation target.
//
// 2019-20, 2020-21 and 2021-22 have **no series at all**: `expected_goals`,
// `expected_assists` and `expected_goals_conceded` are absent as *columns* from both
// files, checked against the headers rather than assumed. So there is no window to fit
// the offset on and no aggregate to validate against, and the offset has to be borrowed
// from the seasons that have both. That is a weaker claim and `OffsetSource` says so on
// every load, because a caveat that lives only in a script's stdout is a caveat nobody
// downstream will see.
//
// # Deliberately a table and not a mechanism
//
// A repair that applies to "whatever season has a file" is a repair nobody can reason
// about later. This project already gates the transfer bank and defensive contribution
// by season for the same reason: handing a season a rule it was not played under is how
// a replay measures something that never happened. Each entry below had its own header
// checked, and a data file naming a season absent from this table is an error rather
// than an invitation.
type xgRepair struct {
	FirstGW, LastGW int
	// Renumber marks 2019-20, whose gameweeks are numbered 1-29 then 39-47.
	// See renumberGW.
	Renumber bool
	// NoAggregate marks a season whose players_raw.csv carries no
	// `expected_goals` column at all, so the **season total** is missing as well
	// as the weekly series and has to be rebuilt. See rebuildXGAggregates, which
	// is where the whole of the reasoning lives.
	//
	// Checked against the headers rather than assumed, and the two groups are
	// cleanly separated: 2019-20 through 2021-22 have no column and therefore a
	// season aggregate of exactly 0.0, while 2022-23 has the column and a
	// **complete** aggregate of 1097.3 against weekly rows summing to 731.5. So
	// 2022-23 must NOT be rebuilt — its total is already right, and adding the
	// repair to it would inflate the season by half again.
	NoAggregate bool
}

var xgRepairs = map[string]xgRepair{
	// 2018-19 is repaired for a different purpose from the rest, and it is worth knowing
	// which. The archive publishes no teams.csv for it, so it loads **prior-only** and
	// can never be played — see Season.Absent. It is backfilled solely so it can be the
	// *prior* for 2019-20, which takes the playable season count from six to seven and
	// the season-clustered degrees of freedom from five to six. A prior is read through
	// the season total, which is what rebuildXGAggregates produces from these rows.
	"2018-19": {FirstGW: 1, LastGW: 38, NoAggregate: true},
	"2019-20": {FirstGW: 1, LastGW: 38, Renumber: true, NoAggregate: true},
	"2020-21": {FirstGW: 1, LastGW: 38, NoAggregate: true},
	"2021-22": {FirstGW: 1, LastGW: 38, NoAggregate: true},
	"2022-23": {FirstGW: 1, LastGW: 15},
}

// renumberGW maps 2019-20's gameweek labels onto 1..38.
//
// COVID stopped the season after GW29 and FPL numbered the restarted rounds **39-47**
// rather than reusing 30-38. Checked against `fixtures.csv`: 38 distinct events and 380
// fixtures, with events 30-38 entirely absent — so the shift is exactly minus nine and
// it cannot collide with a real gameweek.
//
// It matters because `loadGameweeks` drops anything outside 1..38, so without this the
// replay would silently lose the **nine gameweeks after the restart** — a quarter of the
// season, present in the archive, reading as a season that simply stopped in March. That
// is the doubles-counting failure again: plausible numbers, a quarter of the football
// missing.
//
// The same arithmetic exists in `stats/understat_xg_backfill.py`, because the repair
// rows have to land in the same weeks the loader puts the football in.
// TestNineteenTwentyIsRenumberedToThirtyEight pins both halves.
func renumberGW(season string, gw int) int {
	if xgRepairs[season].Renumber && gw >= 39 {
		return gw - 9
	}
	return gw
}

// noXGRepair restores the unrepaired archive, so the two arms can be compared.
//
// Read on every call rather than cached at process start, so a single test binary can
// toggle it between cells and pair the results properly — the same reason `statusAt`
// checks its own switch per call. Caching it would make a paired comparison silently
// measure one arm twice.
func noXGRepair() bool { return os.Getenv("FPL_NO_XG_REPAIR") != "" }

// noXGAggregate restores the state before the season-aggregate half of the repair
// existed: weekly rows filled, season totals left at zero.
//
// It is a *narrower* hatch than FPL_NO_XG_REPAIR on purpose. That one turns the whole
// backfill off, which answers "what is expected goals worth in these seasons"; this one
// isolates the defect rebuildXGAggregates fixes, which answers "what did reading a prior
// season through an unwritten aggregate cost". Those are different questions, and the
// second one has a shipped-grid cell in it — the 2021-22 → 2022-23 pair — so a figure
// measured before this landed needs a way to be re-measured against it rather than an
// argument about whether it moved.
//
// Read on every call for the same reason noXGRepair is: a single test binary has to be
// able to toggle it between cells and pair the results, and caching it at process start
// would make a paired comparison silently measure one arm twice.
func noXGAggregate() bool { return os.Getenv("FPL_NO_XG_AGGREGATE") != "" }

// xgRepairMeta is what the harvest recorded about the repair it wrote.
//
// It exists so the **offset the backfilled xG is on** travels with the data instead of
// living in a script's stdout. A backfilled season's single largest assumption is that
// number, and a snapshot naming the season has to be able to name it too.
//
// It also duplicates the window and the renumber flag, which `xgRepairs` has its own
// copy of. That is deliberate and it is *checked*: `applyXGRepair` refuses a repair
// whose sidecar disagrees with the table. A duplicate verified on every load is a
// pipeline test; an unverified one is the bug this package keeps rediscovering.
type xgRepairMeta struct {
	Season       string  `json:"season"`
	FirstGW      int     `json:"first_gw"`
	LastGW       int     `json:"last_gw"`
	Renumbered   bool    `json:"renumbered"`
	Crosswalk    string  `json:"crosswalk"`
	OffsetSource string  `json:"offset_source"` // "in-season" or "borrowed"
	OffsetXG     float64 `json:"offset_xg"`
	OffsetXA     float64 `json:"offset_xa"`
	Rows         int     `json:"rows"`
	SumXG        float64 `json:"sum_xg"`
	SumXA        float64 `json:"sum_xa"`
	// JoinGoalsRatio is understat goals over FPL goals on the joined cells, excluding
	// any cell whose club benefited from an opponent's own goal. Both sources count an
	// exact integer, so this is the check that validates the crosswalk and the
	// date-to-gameweek mapping rather than the offset — it reads 1.0000 on every
	// repaired season, and anything else means the join is wrong.
	//
	// The own-goal exclusion is not a loosening. Understat credits an own goal to the
	// attacker who forced it and FPL credits it to the defender, so including those
	// cells measured a definitional difference between the sources: it is what made
	// 2018-19 read 1.0019 on two cells whose minutes match to the minute.
	JoinGoalsRatio   float64 `json:"join_goals_ratio"`
	JoinMinutesRatio float64 `json:"join_minutes_ratio"`
	// JoinGoalCells is how many cells the ratio was read on, and JoinGoalMismatch how
	// many of them the two sources disagree on.
	//
	// **The mismatch count is the sharper of the two checks, and the ratio alone is not
	// sufficient.** 2019-20's ratio is exactly 1.0000 with two disagreeing cells:
	// Understat gives Kevin De Bruyne a Manchester City goal in GW10 that FPL gives to
	// David Silva, so one cell is over and one is short and they cancel. A mis-mapped
	// *date* has that same signature — one week over, one week short — which is
	// precisely what this anchor exists to catch, so a ratio of 1.0000 was never proof
	// the join was right.
	//
	// Zero for a file harvested before these were recorded, which is why the guard
	// treats absence as "not measured" rather than as "no mismatches".
	JoinGoalCells    int `json:"join_goal_cells"`
	JoinGoalMismatch int `json:"join_goal_mismatch"`
}

// xgRepairResult reports whether the repair actually changed anything, for the
// provenance stamp. A repair that silently did nothing must not read as one that ran.
type xgRepairResult struct {
	Season  string  // the season repaired, "" when none was
	Rows    int     // rows in the repair file
	Applied int     // rows that actually filled a hole
	Skipped int     // rows whose target already had a value — never overwritten
	Unknown int     // rows naming an element this season does not have
	SumXG   float64 // xG added, for the aggregate check
	SumXA   float64

	// AggFilled and AggKept are the season-aggregate half of the repair: players
	// whose season total was rebuilt from their weekly rows, and players whose
	// total already carried a value and was therefore left alone.
	//
	// Reported rather than assumed, because the weekly half and the aggregate half
	// can each silently do nothing and they fail in different places. AggKept must
	// be zero on every NoAggregate season — a nonzero count means the table says a
	// season has no aggregate and the archive disagrees.
	AggFilled int
	AggKept   int
	AggXG     float64
	AggXA     float64

	// XGC is the expected-goals-conceded reconstruction, which is a separate
	// repair with its own escape hatch and its own calibration. It is reported
	// beside the others rather than folded into them because it is derived from
	// them: the chain consumes the xG this same call just wrote, so a load where
	// the xG half did nothing and the xGC half claims thousands of rows is a
	// contradiction a reader should be able to see.
	XGC xgcRepairResult

	Meta xgRepairMeta
}

func (r xgRepairResult) String() string {
	if r.Rows == 0 {
		return "xG repair: no data"
	}
	agg := "aggregate already complete"
	if xgRepairs[r.Season].NoAggregate {
		agg = fmt.Sprintf("aggregate rebuilt for %d players (+%.1f xG +%.1f xA), "+
			"%d already had one", r.AggFilled, r.AggXG, r.AggXA, r.AggKept)
	}
	return fmt.Sprintf("xG repair %s GW%d-%d: %d of %d rows applied, %d already had "+
		"values, %d unknown elements, +%.1f xG +%.1f xA; %s; offset %s xG/%.4f xA/%.4f, "+
		"join goals ratio %.4f; xGC reconstructed for %d player-gameweeks (+%.1f), "+
		"%d already had one, %d aggregates rebuilt (+%.1f)",
		r.Season, r.Meta.FirstGW, r.Meta.LastGW,
		r.Applied, r.Rows, r.Skipped, r.Unknown, r.SumXG, r.SumXA, agg,
		r.Meta.OffsetSource, r.Meta.OffsetXG, r.Meta.OffsetXA, r.Meta.JoinGoalsRatio,
		r.XGC.Applied, r.XGC.SumXGC, r.XGC.Skipped, r.XGC.AggFilled, r.XGC.AggXGC)
}

// repairedSeasons names every season carrying a backfill, for a test or a report that
// wants to enumerate them without reaching into the map.
func repairedSeasons() []string {
	out := make([]string, 0, len(xgRepairs))
	for s := range xgRepairs {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// applyXGRepair fills a season's missing per-gameweek expected goals.
//
// # Why the archive could not repair itself
//
// The tempting repair for 2022-23 uses only archive data — the season aggregate is
// complete, so the missing total is aggregate minus the known weeks, distributable by
// minutes. **That is a point-in-time leak.** The aggregate includes matches after the
// cutoff, so a model built through GW5 would read a quantity derived from the whole
// season. A repair that is exactly right in aggregate and leaky per week is worse than
// the hole, because the leak is invisible and inflates every figure at once. Understat
// is the honest source because it is per *match*, and a match has a date. Note the trap
// does not even arise for the three older seasons: there is no aggregate to leak from.
//
// # Two properties that make this safe
//
// It only ever fills a zero. A row whose target already carries a value is counted and
// skipped, never overwritten — so the repair is idempotent, and if FPL ever backfills
// the weekly history it degrades to a no-op rather than fighting the real data.
//
// It is partial and says so. Only xG and xA come from Understat; **`XGC` is
// reconstructed rather than harvested**, by pairing a club's repaired xG through the
// fixture list to become its opponents' xGA. See applyXGCRepair, which carries the
// method and its one calibrated constant.
//
// ⚠️ **This paragraph used to say XGC was deliberately left alone, and the reason it
// gave answered the wrong question.** It argued that Understat publishes team xGA
// rather than per-player, and that a prorated club rate would delete the substitution
// channel the per-player figure carries — worth +0.140/+0.067/+0.007 pts/gw across
// substitution terciles. Both halves are true and the conclusion did not follow: the
// choice was never "per-player figure against club rate", it was "reconstruction
// against **nothing**". `baseXP90` gates the clean sheet and the goals-conceded
// deduction on `XGC90 > 0`, so leaving it alone did not preserve the substitution
// channel — it switched off 26-45% of every defender's and keeper's score in 18 of 36
// cells. The reconstruction is flagged per row for exactly the objection that
// paragraph raised.
func (s *Season) applyXGRepair() (xgRepairResult, error) {
	var res xgRepairResult
	spec, ok := xgRepairs[s.Name]
	if !ok || noXGRepair() {
		return res, nil
	}
	res.Season = s.Name
	meta, err := readXGRepairMeta(s.Name, spec)
	if err != nil {
		return res, err
	}
	res.Meta = meta

	f, err := repairData.Open("repairdata/" + s.Name + "-xg.csv")
	if err != nil {
		// A missing repair file is not an error: the harvest is a separate,
		// network-bound step and the replay must still run without it. It is
		// reported instead, because an absent repair and an applied one must never
		// look the same downstream.
		return res, nil
	}
	defer f.Close()

	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		return res, fmt.Errorf("xG repair %s: reading header: %w", s.Name, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(h)] = i
	}
	for _, want := range []string{"element", "GW", "expected_goals", "expected_assists"} {
		if _, ok := col[want]; !ok {
			return res, fmt.Errorf("xG repair %s: no %q column", s.Name, want)
		}
	}

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, fmt.Errorf("xG repair %s: %w", s.Name, err)
		}
		res.Rows++
		el, err1 := strconv.Atoi(strings.TrimSpace(rec[col["element"]]))
		gw, err2 := strconv.Atoi(strings.TrimSpace(rec[col["GW"]]))
		if err1 != nil || err2 != nil {
			return res, fmt.Errorf("xG repair %s: row %d has a non-numeric element "+
				"or GW", s.Name, res.Rows)
		}
		if gw < spec.FirstGW || gw > spec.LastGW {
			return res, fmt.Errorf("xG repair %s: row %d is GW%d, outside the "+
				"repaired window GW%d-%d — the file and this table disagree, which "+
				"means one of them is wrong rather than that the row should be "+
				"skipped", s.Name, res.Rows, gw, spec.FirstGW, spec.LastGW)
		}
		p := s.Players[el]
		if p == nil {
			res.Unknown++
			continue
		}
		g := p.GWs[gw]
		// Only fill a hole. See the note above on idempotence.
		if g.XG != 0 || g.XA != 0 {
			res.Skipped++
			continue
		}
		xg, _ := strconv.ParseFloat(strings.TrimSpace(rec[col["expected_goals"]]), 64)
		xa, _ := strconv.ParseFloat(strings.TrimSpace(rec[col["expected_assists"]]), 64)
		g.XG += xg
		g.XA += xa
		p.GWs[gw] = g
		res.Applied++
		res.SumXG += xg
		res.SumXA += xa
	}
	if spec.NoAggregate && !noXGAggregate() {
		res.AggFilled, res.AggKept, res.AggXG, res.AggXA = s.rebuildXGAggregates()
	}
	// Last, and it has to be: the reconstruction reads the repaired xG this
	// function has just written. See applyXGCRepair.
	if !noXGCRepair() {
		res.XGC = s.applyXGCRepair()
	}
	return res, nil
}

// rebuildXGAggregates fills a season's expected-goals TOTALS from its weekly rows.
//
// # The defect, which was live and silent
//
// The weekly half of this repair writes `g.XG` and `g.XA` on the per-gameweek rows and
// nothing else. `PointInTime` accumulates those rows, so a **played** season saw the
// repair. `PreSeason` — and the prior index behind `newPriorIndex`, which is the same
// quantity reached from the transfer path — reads the season **aggregate** `p.XG`, and
// the seasons with no `expected_goals` column carry that as exactly zero. So every
// season whose prior is one of those built its opening fifteen, and blended every later
// gameweek, with **no expected goals at all**:
//
//	season played   its prior   prior xG seen
//	2020-21         2019-20     zero
//	2021-22         2020-21     zero
//	2022-23         2021-22     zero  — pre-existing, and never connected to 2022-23
//
// The last row is the one that matters beyond the new seasons: 2022-23 is in the shipped
// four-season grid, so a figure measured on that cell moves when this is fixed. Note the
// symmetric case does not arise — 2022-23's own aggregate is complete, so 2023-24's prior
// was always fine.
//
// # Why summing a whole season here is NOT the leak it looks like
//
// This has to be stated rather than assumed, because summing a season total is normally
// *precisely* the point-in-time leak this package guards against, and the header of
// `applyXGRepair` refuses an aggregate-minus-known-weeks repair on exactly those grounds.
// Three things make this one different:
//
//   - **A prior season is entirely in the past.** The aggregate consumed here is last
//     season's, relative to the season being played, so there is no future in it to
//     leak. FPL genuinely publishes last season's totals pre-season — that is what
//     `PreSeason` models, and it is the evidence a real manager had in August.
//   - **Nothing reads the aggregate of the season being played.** `PointInTime`
//     accumulates `p.GWs[1..through]` and never touches `p.XG`, so a mid-season view is
//     unaffected by this and stays truncated at its cutoff.
//     `TestRepairedAggregateDoesNotLeakIntoAPlayedSeason` asserts that behaviourally
//     rather than by reading the code, which is the half of the pin that matters.
//   - **It is the same arithmetic FPL itself did**, not a redistribution. The season
//     total is the sum of the weeks; no quantity is invented and none is moved between
//     weeks, which is where the rejected repair went wrong.
//
// # Two properties, matching the weekly half
//
// It only ever fills a hole: a player whose aggregate already carries a value is counted
// in `kept` and left alone, so this is idempotent and degrades to a no-op if FPL ever
// publishes the totals. `XGC` has its own rebuild, in rebuildXGCAggregates, which runs
// after the reconstruction has written the weekly rows it sums — this one deliberately
// does not touch it, because at the point it runs those rows are still empty.
//
// # The weekly rows are summed in gameweek order, and that is not cosmetic
//
// Floating-point addition is **not associative**, so summing a player's gameweeks in map
// order gives a value that differs in the last bits from one run to the next. That is
// enough to matter here: the aggregate feeds a prior, the prior feeds a score, and the
// optimiser returns a *discrete* fifteen — so a difference of 1e-16 can flip a slot and
// change a replayed season. Gameweek order also matches how `PointInTime` accumulates the
// same quantity, which is the ordering a reader would assume.
//
// This is the map-iteration defect the clean-sheet diagnostic already hit, one layer
// down. The recorded lesson was "accumulation over a map is safe and selection is not" —
// true of the *value* and false of *reproducibility*, and that distinction is what this
// loop turns on.
func (s *Season) rebuildXGAggregates() (filled, kept int, sumXG, sumXA float64) {
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		xg, xa := weeklyXGTotals(p)
		if xg == 0 && xa == 0 {
			continue
		}
		if p.XG != 0 || p.XA != 0 {
			kept++
			continue
		}
		p.XG, p.XA = xg, xa
		filled++
		sumXG += xg
		sumXA += xa
	}
	return filled, kept, sumXG, sumXA
}

// weeklyTotal sums one per-gameweek field over a player's season, in gameweek order
// so the result is reproducible.
//
// # Why the walk is a function and not four lines at each site
//
// Floating-point addition is not associative, so two loops summing the same map in
// two orders disagree in the last bits — which is how this arrived, as a test
// failure with both numbers printing identically. The aggregate feeds a prior, the
// prior feeds a score, and the optimiser returns a *discrete* fifteen, so a
// difference of 1e-16 can flip a slot and change a replayed season. One
// implementation, used by every rebuild and by every test that checks one.
//
// **That argument was written for xG and did not reach xGC.** The xGC rebuild
// inlined its own copy of this walk and so did the test checking it — which is the
// standing rule "a diagnostic must never carry its own copy of the thing it is
// checking", on the statistic whose repair moves 18 of 36 cells. Taking the field
// as a function is what lets both statistics share the walk; the accessor is
// evaluated per gameweek and the addition order is identical to the loop it
// replaces, so no aggregate moves.
//
// The bound is 1..38 because `loadGameweeks` drops anything outside it, after
// `renumberGW` has mapped 2019-20's restart rounds down from 39-47.
func weeklyTotal(p *Player, of func(GW) float64) float64 {
	var total float64
	for gw := 1; gw <= 38; gw++ {
		g, ok := p.GWs[gw]
		if !ok {
			continue
		}
		total += of(g)
	}
	return total
}

// weeklyXGTotals sums a player's per-gameweek expected goals and assists.
//
// Two calls rather than one two-accumulator loop: each statistic is summed in
// gameweek order either way, and the additions within a statistic are in the same
// sequence, so the totals are bit-identical to the single-pass version this
// replaced.
func weeklyXGTotals(p *Player) (xg, xa float64) {
	return weeklyTotal(p, func(g GW) float64 { return g.XG }),
		weeklyTotal(p, func(g GW) float64 { return g.XA })
}

// sortedSeasonPlayerIDs is the season's element ids in ascending order, so anything
// iterating players produces a reproducible result.
func sortedSeasonPlayerIDs(s *Season) []int {
	out := make([]int, 0, len(s.Players))
	for id := range s.Players {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// readXGRepairMeta loads the harvest's own record of what it did, and refuses it if it
// disagrees with this package's table.
//
// The disagreement it is looking for is the one that fails silently: a repair harvested
// for a different window, or with the renumber on when the loader has it off, puts rows
// in the wrong gameweeks. Both sides would still parse and every number downstream would
// still be plausible, which is this package's signature failure and why the check is a
// hard error rather than a warning.
func readXGRepairMeta(season string, spec xgRepair) (xgRepairMeta, error) {
	var m xgRepairMeta
	b, err := repairData.ReadFile("repairdata/" + season + "-xg.meta.json")
	if err != nil {
		// Absent alongside an absent CSV is the ordinary un-harvested state. Absent
		// alongside a *present* CSV is not: it means data shipped without the offset
		// it was scaled by, and the offset is the whole caveat.
		if _, csvErr := repairData.Open("repairdata/" + season + "-xg.csv"); csvErr == nil {
			return m, fmt.Errorf("xG repair %s: the data is present and its "+
				"%s-xg.meta.json is not. That file records the provider offset the "+
				"backfilled xG is on, which is the single assumption a reader has to "+
				"be told about; shipping the data without it is not allowed",
				season, season)
		}
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("xG repair %s: reading meta: %w", season, err)
	}
	switch {
	case m.Season != season:
		return m, fmt.Errorf("xG repair %s: meta names season %q", season, m.Season)
	case m.FirstGW != spec.FirstGW || m.LastGW != spec.LastGW:
		return m, fmt.Errorf("xG repair %s: harvested for GW%d-%d, table says "+
			"GW%d-%d — the rows would land in the wrong weeks and every figure "+
			"downstream would still look plausible",
			season, m.FirstGW, m.LastGW, spec.FirstGW, spec.LastGW)
	case m.Renumbered != spec.Renumber:
		return m, fmt.Errorf("xG repair %s: harvested with renumber=%v, table says "+
			"%v — 2019-20's gameweeks are labelled 1-29 then 39-47, so the two "+
			"sides disagreeing puts nine gameweeks of repair rows nine weeks out",
			season, m.Renumbered, spec.Renumber)
	case m.OffsetXG <= 0 || m.OffsetXA <= 0:
		return m, fmt.Errorf("xG repair %s: meta records a non-positive provider "+
			"offset (%v, %v)", season, m.OffsetXG, m.OffsetXA)
	}
	return m, nil
}
