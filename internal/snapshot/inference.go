package snapshot

// Reading R's published inference. Nothing here computes any of it.
//
// stats/variance_components.R writes four tables into its --out directory. This
// file reads two of them and formats the numbers; every standard error, degree of
// freedom, F test and minimum detectable effect in the snapshot is R's arithmetic
// carried across unchanged. See stats/README.md for why the split exists, and
// TestInferenceLivesInOnePlace, which fails if Go grows a second copy.
//
// The temptation to "just compute the MDE here, it is two lines" is exactly the
// bug class behind DefaultBenchWeight against Weights.BenchWeight, where one
// quantity had two defaults and the measured one turned out not to be the one
// that ran.

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MDE is one estimator's answer to "what size of effect could this design see".
//
// Sig is the effect that would land exactly at p = Alpha — anything smaller
// cannot be called significant however cleanly it was measured. MDE is larger,
// and is the effect the design would actually *find* Power of the time. The gap
// between them is the difference between "would clear the bar if we saw it" and
// "would reliably see it".
type MDE struct {
	Metric string
	// Scope is what the row is about: "arm" for one alternative against the
	// baseline on one metric, "pooled" for the sweep's arms averaged.
	//
	// The distinction is the whole reason this field exists. A pooled row is
	// dominated by whichever arm disagrees most between seasons, so it can describe
	// no arm in the sweep: on the minutes-half-life grid one arm contributes 72% of
	// the pooled season variance and its own threshold is 232 points a season
	// against 80, 90 and 133 for the other three. Rows written before this column
	// existed carry no scope and are read as pooled, which is what they were.
	Scope string
	// Variant names the alternative an arm row compares against the baseline, and
	// is empty on a pooled row.
	Variant   string
	Estimator string
	SE        float64
	DF        int
	TCrit     float64
	SigGW     float64 // points per gameweek
	SigSeason float64 // the same, times a 38-gameweek season
	MDEGW     float64
	MDESeason float64
	// FSeason and PArm are this arm's own season F test: does the effect differ
	// between seasons at all? A small p licenses the season-clustered end of the
	// bracket; a large one does not license the other end, because the test has
	// only FPower of the power it would need to see the case that matters.
	FSeason float64
	PArm    float64
	HasPArm bool
	FPower  float64
	Primary bool    // the estimator the pooled season F test licenses, pooled rows only
	PSeason float64 // strictest arm's season F-test p-value
	// HasPSeason distinguishes "no p-value" from "p = 0".
	//
	// R writes NA when the F ratio is 0/0, which happens whenever every paired
	// difference is exactly zero — the state an invariance check *passing* produces.
	// Parsing that NA to a float gives 0.000, which reads as an overwhelmingly
	// significant season component for a metric that did not move at all. This
	// project has priced the blank-versus-zero distinction before, when an unset
	// penalty multiplied by zero instead of being a no-op.
	HasPSeason bool
	Power      float64
	Alpha      float64
	// Degenerate marks a metric whose differences are all exactly zero, so there is
	// nothing to resolve and every figure in its row is a formatting artefact.
	Degenerate bool
}

// Components is how one metric's noise splits, pooled over a sweep's arms.
//
// The split is the question that decides what can be done about the noise.
// Season heterogeneity means the effect genuinely differs from one season to the
// next, and only more seasons help — of which there are four and never will be
// more, since the archive carries expected goals from 2022-23 only. Path noise
// means the effect is the same and one discrete transfer flipped, which more
// entry gameweeks average away, and entry gameweeks are cheap.
type Components struct {
	Metric      string
	Arms        int
	Seasons     int // S
	StartGWs    int // G
	SDSeason    float64
	SDStart     float64
	SDResid     float64
	SDSeasonMap float64 // sd of the four per-season mean effects
	ShareSeason float64 // per cent of that spread that is season heterogeneity
	SharePath   float64
}

// Inference is everything read out of one --out directory.
//
// Source names the sweep it came from, and it is not cosmetic: the minimum
// detectable effect is a property of the *comparison*, since the variance
// components are estimated from that sweep's paired differences. A
// mechanism-certain change that lands almost identically in every cell resolves an
// order of magnitude more finely than a scoring constant whose effect varies by
// season, so a figure divorced from its sweep invites being read as a constant of
// the harness. It is not one.
type Inference struct {
	Source     string
	Dir        string
	MDE        []MDE
	Components []Components
	Missing    []string // tables that were expected and absent

	// Blocks is every sweep label R recorded in the tables it wrote, taken from
	// mde.csv's `block` column.
	//
	// It exists so the caller can check that these numbers came from the cells file
	// it is about to name them after. Those are the same fact from two sources —
	// the cells file says which sweep it holds, and R's output says which sweep it
	// was computed from — and this project's rule is that a duplicated quantity is
	// a pipeline test when it is checked and the bug when it is merely watched. See
	// MatchesCells.
	Blocks []string
}

// MatchesCells reports whether these tables were computed from a sweep the cells
// file actually contains.
//
// The failure it catches is real and has happened twice. `armband snapshot`
// defaults its cells path to a location a throwaway demo has written to, and
// reading whatever sits there stamps that file's grid, labels and provenance onto
// harness numbers computed from something else. The first occurrence went
// unnoticed for weeks because the output named no source; the second was caught
// only because a label changed visibly from VICECAP to T in a diff.
//
// R records the block as "<run id> / <LABEL>", so a label matches when it appears
// anywhere in a block string. An empty Blocks means the tables predate the column,
// which cannot be checked and must not be reported as a mismatch.
func (in Inference) MatchesCells(labels []string) bool {
	if len(in.Blocks) == 0 || len(labels) == 0 {
		return true
	}
	for _, b := range in.Blocks {
		for _, l := range labels {
			if l != "" && strings.Contains(b, l) {
				return true
			}
		}
	}
	return false
}

// ReadInference loads R's tables from an --out directory.
//
// A missing table is recorded rather than fatal, and the snapshot prints what is
// missing. Silence must not read as success: a harness section that quietly
// omitted its MDE would look much like a harness section that had nothing to say,
// and this whole artefact exists because that ambiguity has cost this project
// real money.
func ReadInference(dir string) (Inference, error) {
	in := Inference{Dir: dir}

	mdeRows, err := readNamedCSV(filepath.Join(dir, "mde.csv"))
	switch {
	case os.IsNotExist(err):
		in.Missing = append(in.Missing, "mde.csv (the minimum detectable effect)")
	case err != nil:
		return in, err
	default:
		for _, r := range mdeRows {
			row := MDE{
				Metric: r["metric"], Estimator: r["estimator"],
				// A row from before the column existed is a pooled row, which is what
				// every figure recorded in AGENTS.md was.
				Scope: scopeOr(r["scope"], "pooled"), Variant: r["variant"],
				SE: num(r["se"]), DF: int(num(r["df"]) + 0.5),
				TCrit: num(r["t_crit"]),
				SigGW: num(r["sig_gw"]), SigSeason: num(r["sig_season"]),
				MDEGW: num(r["mde_gw"]), MDESeason: num(r["mde_season"]),
				FSeason: num(r["f_season"]), PArm: num(r["p_season"]),
				FPower:  num(r["f_power"]),
				Primary: r["is_primary"] == "TRUE" || r["is_primary"] == "true",
				PSeason: num(r["p_season_pooled"]),
				Power:   num(r["power"]), Alpha: num(r["alpha"]),
			}
			row.HasPSeason = isNumber(r["p_season_pooled"])
			row.HasPArm = isNumber(r["p_season"])
			if row.Variant == "NA" {
				row.Variant = ""
			}
			// A standard error of exactly zero across the grid means every paired
			// difference was zero. That is an invariance holding exactly, not a
			// measurement of infinite precision, and it must not be printed as one.
			row.Degenerate = row.SE == 0
			in.MDE = append(in.MDE, row)
			if b := r["block"]; b != "" && !contains(in.Blocks, b) {
				in.Blocks = append(in.Blocks, b)
			}
		}
	}

	poolRows, err := readNamedCSV(filepath.Join(dir, "variance_pooled.csv"))
	switch {
	case os.IsNotExist(err):
		in.Missing = append(in.Missing, "variance_pooled.csv (the variance components)")
	case err != nil:
		return in, err
	default:
		for _, r := range poolRows {
			// v_*_used are floored at zero. A method-of-moments component can come
			// out negative, which is how a component says "smaller than the noise
			// around it" — informative, and not a quantity to take a square root
			// of. R reports both; the floored one is what sizes a design.
			vs, ve := num(r["v_season_used"]), num(r["v_resid"])
			g := num(r["G"])
			c := Components{
				Metric: r["metric"], Arms: int(num(r["arms"]) + 0.5),
				Seasons: int(num(r["S"]) + 0.5), StartGWs: int(g + 0.5),
				SDSeason: sqrt(vs), SDStart: sqrt(num(r["v_start_used"])),
				SDResid: sqrt(ve),
			}
			if tot := vs + ve/g; tot > 0 {
				c.SDSeasonMap = sqrt(tot)
				c.ShareSeason = 100 * vs / tot
				c.SharePath = 100 * (ve / g) / tot
			}
			in.Components = append(in.Components, c)
		}
	}
	return in, nil
}

// PrimaryMDE returns the pooled row for a metric that R marked as licensed.
//
// Pooled only, and deliberately: `is_primary` is the answer of a pre-test run on
// the whole sweep's arms, and R marks no primary on an arm row because the test
// that would choose between the two estimators has about 22% power against the
// case that would change the answer. For a per-comparison figure use Brackets.
func (in Inference) PrimaryMDE(metric string) (MDE, bool) {
	for _, m := range in.MDE {
		// Anything not explicitly an arm row is pooled, which is what a table written
		// before the column existed was.
		if m.Metric == metric && m.Scope != "arm" && m.Primary {
			return m, true
		}
	}
	return MDE{}, false
}

// Bracket is one comparison's resolution, reported as the range between the two
// defensible estimators rather than as a single number.
//
// Clustered treats the four seasons as the independent units, which is correct
// whenever the effect genuinely differs between them and costs a lot of precision
// — three degrees of freedom, so the multiple of the standard error needed to
// reach p = 0.05 is 3.18 rather than the familiar 2. StartFixed treats the six
// entry gameweeks as the fixed device they are — the same six replayed every
// season on purpose, so an offset between them cancels from a paired comparison —
// which is the same standard error on fifteen degrees of freedom, and is valid
// only where the season component really is zero.
//
// Both ends are reported because the test that would choose between them cannot:
// see FPower.
type Bracket struct {
	Metric  string
	Variant string
	// Clustered is the conservative end, StartFixed the optimistic one. Either may
	// be the larger number: the two estimators differ in df as well as in variance,
	// so a metric with no season component can read *higher* on the clustered end.
	Clustered  MDE
	StartFixed MDE
	FSeason    float64
	PArm       float64
	HasPArm    bool
	// FPower is the power of that F test against a season component just large
	// enough to double the clustered variance. It is R's arithmetic, read here, and
	// it is the reason no pre-test picks an end of the bracket.
	FPower float64
	// Degenerate marks a comparison whose paired differences were all exactly zero:
	// an invariance holding, where every figure in the row is a formatting artefact
	// rather than a resolution.
	Degenerate bool
}

// Brackets returns one bracket per arm of the sweep, for a metric, in R's order.
func (in Inference) Brackets(metric string) []Bracket {
	var out []Bracket
	at := map[string]int{}
	for _, m := range in.MDE {
		if m.Scope != "arm" || m.Metric != metric {
			continue
		}
		i, ok := at[m.Variant]
		if !ok {
			out = append(out, Bracket{
				Metric: metric, Variant: m.Variant, FSeason: m.FSeason,
				PArm: m.PArm, HasPArm: m.HasPArm, FPower: m.FPower,
			})
			i = len(out) - 1
			at[m.Variant] = i
		}
		// Matched on R's own estimator label rather than on df, so a design with a
		// different number of seasons or start points still lands in the right slot.
		switch {
		case strings.HasPrefix(m.Estimator, "season-clustered"):
			out[i].Clustered = m
		case strings.HasPrefix(m.Estimator, "start fixed"):
			out[i].StartFixed = m
		}
	}
	for i := range out {
		out[i].Degenerate = out[i].Clustered.SE == 0 && out[i].StartFixed.SE == 0
	}
	return out
}

// Metrics lists the metrics R reported on, in the order it reported them.
func (in Inference) Metrics() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range in.MDE {
		if !seen[m.Metric] {
			seen[m.Metric] = true
			out = append(out, m.Metric)
		}
	}
	return out
}

// scopeOr defaults a missing scope, so a table written before the column existed
// still reads correctly rather than falling into neither branch.
func scopeOr(s, def string) string {
	if s == "" || s == "NA" {
		return def
	}
	return s
}

// readNamedCSV reads a CSV into name-keyed maps, tolerating R's quoting.
func readNamedCSV(path string) ([]map[string]string, error) {
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
	var out []map[string]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		m := map[string]string{}
		for i, h := range head {
			if i < len(rec) {
				m[h] = rec[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// num parses a float, treating R's NA and an empty field as zero.
//
// Zero is wrong for a genuinely absent value and the callers above only use num
// on columns R always writes, so the distinction does not arise. It is worth
// naming because this project has been bitten by a blank read as a zero: an
// unmeasured layer and a layer measured at zero are different facts.
func num(s string) float64 {
	if !isNumber(s) {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// isNumber reports whether a field carried a value at all.
//
// R writes NA for a quantity it could not compute, and the two states must stay
// distinguishable: an F test that could not run and an F test that returned zero
// are different facts, and only one of them means the season component is real.
func isNumber(s string) bool {
	if s == "" || s == "NA" || s == "NaN" || s == "NULL" || s == "Inf" || s == "-Inf" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// sqrt guards the negative method-of-moments case, which is a legitimate value R
// reports and not an error: a variance component estimated below zero is a
// component saying it is indistinguishable from zero.
func sqrt(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return math.Sqrt(v)
}
