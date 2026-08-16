package snapshot

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
)

// Cell is one replayed season from one entry gameweek under one setting.
//
// "Cell" is the harness's unit of measurement: four seasons times six entry
// gameweeks gives 24 cells per setting, and every comparison in this project is a
// paired difference between two settings *within* a cell — the same football, the
// same opening conditions, one setting changed.
type Cell struct {
	Sweep, RunID  string
	Variant       string
	VariantIndex  int
	IsBaseline    bool
	Season        string
	StartGW       int
	Weeks         int
	BankUpTo      int
	Infeasible    bool
	PolicyPoints  int
	HoldPoints    int
	Moves         int
	HasHoldRungs  bool
	HoldFixedCap  int
	HoldNoCaptain int
}

// Sweep is everything one sweep's cells say about themselves, with no inference.
//
// Completed and Killed are the point of this type. A cells file records what ran;
// only the provenance sidecar records what was *asked* to run, and the difference
// is the arm that died under load.
type Sweep struct {
	Label      string
	RunID      string
	Prov       Provenance // zero when no sidecar was found
	HasProv    bool
	Arms       []Arm
	Seasons    []string
	StartGWs   []int
	Banks      []int
	HoldRungs  bool // whether the captaincy-rung columns were populated
	Invariance []Invariance
}

// Arm is one setting under test.
type Arm struct {
	Label      string
	Index      int
	IsBaseline bool
	Cells      int  // cells that produced a usable result
	Infeasible int  // cells where the setting could not field a legal fifteen
	Declared   bool // named in the provenance as intended to run
	Ran        bool // emitted at least one cell
}

// Invariance is a quantity a change must leave alone, and whether it did.
//
// Falsification is enormously cheaper than confirmation on this harness — a
// violation shows up in a single cell, where *confirming* an effect on the
// transfer metric needs about 147 points a season — so an invariance check that
// passes is worth more than its cost suggests, and one that fails is decisive.
type Invariance struct {
	Metric   string
	Arms     int
	Cells    int
	Differs  int    // cells where a non-baseline arm differs from the baseline
	Example  string // the worst offending cell, when one exists
	Expected string // why it should hold, in one clause
}

// ReadCells parses a cells CSV.
//
// Tolerant of a file written by an older schema — a frozen fixture from before
// the captaincy rungs existed is still perfectly good for the season-and-path
// variance components, and refusing it would throw away a usable measurement over
// two absent columns. Column *positions* are not assumed: they are read from the
// header by name, so a schema that grows a column in the middle does not silently
// shift every field after it.
func ReadCells(path string) ([]Cell, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: unreadable header: %w", path, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	need := []string{"sweep", "run_id", "variant", "variant_index", "season",
		"start_gw", "weeks", "infeasible", "policy_points", "hold_points"}
	for _, n := range need {
		if _, ok := col[n]; !ok {
			return nil, fmt.Errorf("%s: no %q column; this is not a cells file", path, n)
		}
	}
	get := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }

	var out []Cell
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		c := Cell{
			Sweep: get(rec, "sweep"), RunID: get(rec, "run_id"),
			Variant: get(rec, "variant"), VariantIndex: atoi(get(rec, "variant_index")),
			IsBaseline: get(rec, "is_baseline") == "true",
			Season:     get(rec, "season"), StartGW: atoi(get(rec, "start_gw")),
			Weeks: atoi(get(rec, "weeks")), BankUpTo: atoi(get(rec, "bank_up_to")),
			Infeasible:   get(rec, "infeasible") == "true",
			PolicyPoints: atoi(get(rec, "policy_points")),
			HoldPoints:   atoi(get(rec, "hold_points")),
			Moves:        atoi(get(rec, "moves")),
		}
		if v := get(rec, "hold_nocap_points"); v != "" {
			c.HasHoldRungs = true
			c.HoldNoCaptain = atoi(v)
			c.HoldFixedCap = atoi(get(rec, "hold_fixedcap_points"))
		}
		out = append(out, c)
	}
	return out, nil
}

// GroupSweeps turns cells into per-sweep summaries and joins the provenance.
//
// Keyed on (sweep label, run_id) exactly as R keys a comparison. Two runs of one
// block must stay two samples: pooling them would shrink a standard error while
// adding no information, which is the manufactured-confidence failure the whole
// per-cell contract exists to prevent.
func GroupSweeps(cells []Cell, prov map[string]Provenance) []Sweep {
	type key struct{ sweep, run string }
	order := []key{}
	byKey := map[key][]Cell{}
	for _, c := range cells {
		k := key{c.Sweep, c.RunID}
		if _, seen := byKey[k]; !seen {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], c)
	}

	var out []Sweep
	for _, k := range order {
		group := byKey[k]
		s := Sweep{Label: k.sweep, RunID: k.run}
		if p, ok := prov[k.sweep+"\x00"+k.run]; ok {
			s.Prov, s.HasProv = p, true
		}

		// Arms in declaration order where provenance gives one, so a killed arm
		// keeps its place in the ladder rather than vanishing from the middle.
		type acc struct {
			idx        int
			base       bool
			ok, infeas int
			ran        bool
			declared   bool
		}
		arms := map[string]*acc{}
		var armOrder []string
		touch := func(label string) *acc {
			if a, ok := arms[label]; ok {
				return a
			}
			a := &acc{idx: -1}
			arms[label] = a
			armOrder = append(armOrder, label)
			return a
		}
		for i, label := range s.Prov.DeclaredArms {
			a := touch(label)
			a.declared, a.idx, a.base = true, i, i == 0
		}

		seasons, starts, banks := map[string]bool{}, map[int]bool{}, map[int]bool{}
		for _, c := range group {
			a := touch(c.Variant)
			a.ran = true
			if a.idx < 0 {
				a.idx = c.VariantIndex
			}
			a.base = a.base || c.IsBaseline
			if c.Infeasible {
				a.infeas++
			} else {
				a.ok++
				seasons[c.Season] = true
				starts[c.StartGW] = true
				banks[c.BankUpTo] = true
			}
			s.HoldRungs = s.HoldRungs || c.HasHoldRungs
		}
		for _, label := range armOrder {
			a := arms[label]
			s.Arms = append(s.Arms, Arm{
				Label: label, Index: a.idx, IsBaseline: a.base,
				Cells: a.ok, Infeasible: a.infeas,
				Declared: a.declared, Ran: a.ran,
			})
		}
		sort.SliceStable(s.Arms, func(i, j int) bool { return s.Arms[i].Index < s.Arms[j].Index })

		s.Seasons = sortedStrings(seasons)
		s.StartGWs = sortedInts(starts)
		s.Banks = sortedInts(banks)
		s.Invariance = invariance(group)
		out = append(out, s)
	}
	return out
}

// invariance checks the quantities a sweep's own structure says must not move.
//
// This is an equality count, not a statistical test — which is why it is here and
// not in R. "Byte-identical in all 24 cells" is a fact about integers.
//
// The check that earns its keep is HOLD under a transfer-only knob. HOLD buys the
// opening fifteen and never transfers, so a knob that only touches the weekly
// transfer decision cannot reach it; if it moves, the knob is leaking into
// scoring and the experiment is measuring two things at once. That is the exact
// bug that made the original fixture-horizon sweep uninterpretable, because
// Horizon was setting the fixture window *and* scaling the transfer threshold.
//
// It is reported rather than asserted, because "HOLD moved" is the expected
// answer for a scoring knob and the violation only for a transfer one. Naming
// which knob is which is a judgement the cells file does not carry.
func invariance(group []Cell) []Invariance {
	type cellKey struct {
		season string
		start  int
	}
	base := map[cellKey]Cell{}
	var others []Cell
	for _, c := range group {
		if c.Infeasible {
			continue
		}
		if c.VariantIndex == 0 {
			base[cellKey{c.Season, c.StartGW}] = c
			continue
		}
		others = append(others, c)
	}
	if len(base) == 0 || len(others) == 0 {
		return nil
	}
	check := func(metric, why string, of func(Cell) int) Invariance {
		iv := Invariance{Metric: metric, Expected: why}
		arms := map[string]bool{}
		worst := 0
		for _, c := range others {
			b, ok := base[cellKey{c.Season, c.StartGW}]
			if !ok {
				continue
			}
			arms[c.Variant] = true
			iv.Cells++
			d := of(c) - of(b)
			if d == 0 {
				continue
			}
			iv.Differs++
			if d < 0 {
				d = -d
			}
			if d > worst {
				worst = d
				iv.Example = fmt.Sprintf("%s entered at GW%d, %s: %d against %d",
					c.Season, c.StartGW, c.Variant, of(c), of(b))
			}
		}
		iv.Arms = len(arms)
		return iv
	}
	out := []Invariance{check("HOLD (hold the opening fifteen, never transfer)",
		"a knob that only reaches the weekly transfer decision cannot move it",
		func(c Cell) int { return c.HoldPoints })}
	if group[0].HasHoldRungs {
		out = append(out, check("nobody doubled (HOLD with no captain at all)",
			"a change to who wears the armband cannot move a metric that doubles nobody",
			func(c Cell) int { return c.HoldNoCaptain }))
	}
	return out
}

func sortedStrings(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedInts(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
