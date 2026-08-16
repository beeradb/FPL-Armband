package snapshot

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Inputs is everything a snapshot is built from.
type Inputs struct {
	Date   time.Time
	Commit string
	Dirty  bool
	Branch string
	Sweeps []Sweep
	// One entry per cells file. Plural because the minimum detectable effect is a
	// property of the *comparison*, not of the harness — see renderHeadline — so a
	// snapshot that reported only one sweep's figure would invite exactly the
	// generalisation it must not.
	Inference []Inference
	Model     []Diagnostic
	ModelPath string
	CellsPath string
	Notes     []string // free-text caveats the operator wants stamped in
	Previous  *Values  // the last snapshot's figures, for the diff
	PrevName  string
}

// Values is a snapshot's figures in machine-readable form.
//
// Written beside the markdown so the *next* snapshot can diff against it. Without
// this the diff would have to parse the previous markdown, which is the coupling
// that makes report formats un-editable.
type Values struct {
	Keys  []string // in written order
	Value map[string]string
}

func newValues() *Values { return &Values{Value: map[string]string{}} }

func (v *Values) set(key, val string) {
	if _, seen := v.Value[key]; !seen {
		v.Keys = append(v.Keys, key)
	}
	v.Value[key] = val
}

func (v *Values) setf(key string, val float64, dp int) {
	v.set(key, strconv.FormatFloat(val, 'f', dp, 64))
}

// metricNames turns R's short metric slugs into something a reader can act on.
//
// Defined here rather than in R because it is presentation. The definitions
// themselves are load-bearing: this project's convention is that a scoring or
// squad constant is judged on HOLD and only a constant that is *about* transfers
// is judged on POLICY, and a reader who does not know which is which cannot tell
// whether a verdict was reached on the right instrument.
var metricNames = map[string]struct{ short, long string }{
	"policy": {"POLICY", "buy the opening fifteen, then make the weekly transfer " +
		"decision all season. The instrument for constants that are *about* " +
		"transfers, where the transfer path is the thing being measured."},
	"hold": {"HOLD", "buy the opening fifteen and never transfer, re-picking the " +
		"eleven and the captain every week with substitutes applied. The instrument " +
		"for scoring and squad constants, because it drops the noisiest component " +
		"without dropping anything FPL pays for."},
	"hold_fixedcap": {"armband pinned", "HOLD with the captain fixed at whoever the " +
		"model would have captained in the week the squad was bought. A candidate " +
		"quieter instrument, not a metric in its own right."},
	"hold_nocap": {"nobody doubled", "HOLD with no captain at all. Removes the " +
		"armband's variance contribution outright — and, being blind to any rule " +
		"about who gets doubled, is the reductio of choosing an instrument on " +
		"variance alone."},
}

func metricShort(m string) string {
	if n, ok := metricNames[m]; ok {
		return n.short
	}
	return m
}

// Render builds the snapshot markdown and its machine-readable companion.
func Render(in Inputs) (markdown string, values *Values) {
	var b strings.Builder
	v := newValues()
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	p("# Model and harness accuracy snapshot\n\n")
	p("%s", leadParagraph())

	renderHeadline(&b, in, v)
	renderStamp(&b, in, v)
	renderHarness(&b, in, v)
	renderModel(&b, in, v)
	renderDiff(&b, in, v)
	renderHowToRegenerate(&b, in)

	return b.String(), v
}

func leadParagraph() string {
	return "" +
		"Two halves, which must not be blurred together.\n\n" +
		"**Model accuracy** asks whether the scoring model is right about football. " +
		"It is measured against outcomes — what players went on to score — and rests " +
		"on thousands of observations.\n\n" +
		// ⚠️ Corrected 2026-08-15. This said "four seasons, which is all the
		// archive holds and all it ever will: expected goals begin in 2022-23",
		// and both halves were falsified by work that had already shipped. The
		// default grid is six pairs; and expected goals begin in 2022-23 only
		// NATIVELY — the Understat harvest backfills the earlier seasons, which is
		// what made the widening possible.
		//
		// It is the worst place in the tree for a stale claim, because it is
		// *generated into every snapshot*, so a reader meets it as this run's own
		// output rather than as documentation that might be old. Do not restate a
		// grid width here: say where it is decided and let the reader look.
		//
		// ⚠️ And the correction above left one in. "— six season pairs by default —"
		// survived the same edit that removed "four seasons", one clause after the
		// instruction not to write it, which is how a rule reads when it is added
		// beside the thing it forbids rather than applied to it.
		//
		// Deleted 2026-08-15, and deleted rather than derived for two reasons.
		// `leadParagraph` takes no arguments and runs before any sweep is read — a
		// model-only snapshot has no sweep at all — so there is no run to derive a
		// grid from at this point in the document. And the DEFAULT is not a
		// property of any input the renderer receives: it is `sweepPairNames` in
		// `internal/backtest`'s test files, which no shipped code can reach.
		//
		// ⚠️ Not "this package cannot see the grid": it can, later and for the run
		// in hand. `Sweep.Seasons` comes off the sweep's own provenance and the
		// sweep table below already prints "seasons replayed" and the cell count
		// from it. That is the derived label, it is in the right place, and this
		// sentence's job is to send the reader to it.
		"**Harness accuracy** asks whether the replay can see anything at all. Its " +
		"grid is whatever `FPL_SWEEP_SEASONS` selected for the sweep this snapshot " +
		"reads, and the per-arm thresholds below are the only authority on what it " +
		"could resolve.\n\n" +
		"A model can be well calibrated while the harness cannot resolve any change " +
		"to it. That is this project's actual situation, and reading one half as " +
		"though it answered the other is how \"the instrument could not see it\" came " +
		"to be recorded as \"there is no effect\" — repeatedly, and in both " +
		"directions.\n\n"
}

// renderHeadline leads with the minimum detectable effect.
//
// It leads because it is the number that makes every "unresolved" verdict readable
// as *below what this instrument can detect* rather than as *shown to have no
// effect*. Almost every constant this project argues over is worth 11 to 34 points
// a season, and the figures below are larger than that on both metrics — so
// "unresolved" is the expected outcome for a real effect of that size, not
// evidence against one.
func renderHeadline(b *strings.Builder, in Inputs, v *Values) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("## The headline: what this harness can detect at all\n\n")

	if !anyMDE(in.Inference) {
		p("**Not measured in this snapshot.** No `mde.csv` was found, so the harness " +
			"half is absent rather than empty. Nothing below should be read as a " +
			"statement about resolution.\n\n")
		for _, inf := range in.Inference {
			if len(inf.Missing) > 0 {
				p("- `%s`: missing %s\n", orDash(inf.Dir), joinOrDash(inf.Missing))
			}
		}
		p("\n")
		v.set("harness.mde.present", "false")
		return
	}
	v.set("harness.mde.present", "true")

	p("Two quantities, both in points over a 38-gameweek season, and both smaller " +
		"is better:\n\n")
	p("- **Significance threshold** — the effect that would land exactly at " +
		"p = 0.05. Anything smaller cannot be called significant however cleanly it " +
		"was measured.\n")
	p("- **Minimum detectable effect** — the effect this design would actually find " +
		"most of the time. Larger than the threshold; the gap between them is the " +
		"difference between \"would clear the bar if we saw it\" and \"would reliably " +
		"see it\".\n\n")

	// The per-comparison caveat is not a hedge; it is the single easiest way to
	// misread this table. The variance components come from the paired differences
	// of one specific sweep, so a mechanism-certain change that lands almost
	// identically in every cell resolves an order of magnitude more finely than a
	// scoring constant whose effect varies by season and entry gameweek. Quoting one
	// sweep's figure as "the harness's resolution" would be the same species of
	// error as quoting a buy-side over-rating measured at one gate setting as a
	// project constant.
	p("**The figure is per comparison, not per harness.** The variance behind it " +
		"comes from the paired differences of one specific setting against the " +
		"baseline, so a change whose effect is nearly identical in every cell resolves " +
		"far more finely than one whose effect varies between seasons. That is why " +
		"every arm gets its own row below, and why **no single row should be quoted as " +
		"\"the harness's resolution\"**. One was, for weeks: a sweep's arms were " +
		"averaged into a single figure per metric, and the average turned out to be " +
		"dominated by the one arm that disagreed most between seasons.\n\n")

	// ⚠️ Corrected 2026-08-15, with the sibling at "A season component estimated at
	// zero" below. This said "the four seasons … only three degrees of freedom — so
	// the multiple … is 3.18". Both halves are grid-dependent and the grid moved:
	// six seasons give df 5 and t_crit 2.571. Worse, the df belongs to the
	// *comparison* and is often lower still, so a fixed number here is wrong even
	// at a fixed grid.
	//
	// This is generated into every snapshot, so a reader meets it as this run's own
	// output. Eight banked snapshots carry the old sentence. The same defect was
	// corrected in `docs/replay.md` and, as a prescription, in
	// `docs/architecture.md:211` — this file was missed both times because nobody
	// greps generated prose. **Do not restate a df or a t_crit here**: the per-arm
	// table below is computed and is the only authority.
	p("Each row is a **range**, because two estimators are defensible and the test " +
		"that would choose between them cannot. *Clustered* treats the seasons " +
		"as the independent units, which is right whenever the effect genuinely " +
		"differs between them, and it costs degrees of freedom — one fewer than the " +
		"number of seasons, so the multiple of the standard error needed for " +
		"p = 0.05 is well above the familiar 2. The per-arm rows below carry the " +
		"figure actually used. *Start-fixed* treats the entry gameweeks as the fixed device they are — " +
		"the same ones are replayed in every season on purpose, so an offset between " +
		"them cancels from a paired comparison — which buys five times the degrees of " +
		"freedom and is valid only where the season component really is zero.\n\n")

	p("| source sweep | metric | comparison | clustered (3 df) | start-fixed (15 df) | " +
		"season F | p |\n")
	p("|---|---|---|---:|---:|---:|---:|\n")
	// The finest and coarsest comparison on the page, as order statistics of the
	// figures R published — not a re-estimate of anything. Taken over the arm rows,
	// which are the honest unit; a sweep whose tables predate the per-arm rows falls
	// back to its pooled row so an older directory still says something.
	//
	// Only the conservative end of each bracket feeds this summary. The start-fixed
	// end is valid where the season component is genuinely zero, and the test that
	// would establish that has 22% power — so a headline "this resolves 5 points a
	// season" built on it would be the same over-claim in a new place.
	worst, best := 0.0, 0.0
	consider := func(mde float64) {
		if mde <= 0 {
			return
		}
		if mde > worst {
			worst = mde
		}
		if best == 0 || mde < best {
			best = mde
		}
	}
	fPower := 0.0
	// The metric column is a short label, and a label whose meaning arrives forty
	// lines later is a label a reader guesses at. The full definitions are in the
	// components section; the first clause of each goes under the table.
	var seenMetrics []string
	seen := map[string]bool{}
	for _, inf := range in.Inference {
		for _, m := range inf.Metrics() {
			if !seen[m] {
				seen[m] = true
				seenMetrics = append(seenMetrics, m)
			}
			key := "harness." + slug(inf.Source) + "."
			brackets := inf.Brackets(m)
			for _, bk := range brackets {
				if bk.FPower > 0 {
					fPower = bk.FPower
				}
				// An exactly-invariant comparison has no resolution to report. Printing
				// "0 pts/season" would say it can detect an arbitrarily small effect,
				// when what happened is that it detected nothing at all because the
				// change cannot reach the metric. That is the reductio of picking an
				// instrument on its standard error: the quietest possible metric is one
				// that cannot move.
				if bk.Degenerate {
					p("| %s | **%s** | %s | — | — | — | **cannot move: every cell identical** |\n",
						inf.Source, metricShort(m), bk.Variant)
					v.set(key+"invariant."+m, "true")
					continue
				}
				pv := "—"
				if bk.HasPArm {
					pv = fmt.Sprintf("%.3f", bk.PArm)
				}
				p("| %s | **%s** | %s | %.0f–%.0f pts | %.0f–%.0f pts | %.2f | %s |\n",
					inf.Source, metricShort(m), bk.Variant,
					bk.Clustered.SigSeason, bk.Clustered.MDESeason,
					bk.StartFixed.SigSeason, bk.StartFixed.MDESeason,
					bk.FSeason, pv)
				consider(bk.Clustered.MDESeason)
				armKey := key + "mde_season." + m + "." + slug(bk.Variant)
				v.setf(armKey+".clustered", bk.Clustered.MDESeason, 0)
				v.setf(armKey+".startfixed", bk.StartFixed.MDESeason, 0)
			}
			// The pooled row, kept because every threshold recorded in AGENTS.md is one
			// and dropping it would orphan that record. Labelled, and no longer alone.
			row, ok := inf.PrimaryMDE(m)
			if !ok {
				continue
			}
			if row.Degenerate {
				// Printed rather than skipped even when no arm row said it: an invariance
				// holding exactly is a result about the change, and a metric absent from
				// the table reads as a metric nobody measured.
				if len(brackets) == 0 {
					p("| %s | **%s** | *pooled over the sweep's arms* | — | — | — | "+
						"**cannot move: every cell identical** |\n",
						inf.Source, metricShort(m))
				}
				v.set(key+"invariant."+m, "true")
				continue
			}
			p("| %s | %s | *pooled over the sweep's arms* | %s, %d df | — | — | %s |\n",
				inf.Source, metricShort(m), fmt.Sprintf("%.0f–%.0f pts",
					row.SigSeason, row.MDESeason), row.DF, pooledP(row))
			v.setf(key+"mde_season."+m, row.MDESeason, 0)
			v.setf(key+"sig_season."+m, row.SigSeason, 0)
			v.set(key+"estimator."+m, row.Estimator)
			v.set(key+"df."+m, strconv.Itoa(row.DF))
			if len(brackets) == 0 {
				consider(row.MDESeason)
			}
		}
	}
	p("\n")
	p("Each pair is *significance threshold–minimum detectable effect*: the first " +
		"is the effect that would land exactly at p = 0.05, the second the effect " +
		"the design would actually find most of the time. The metrics, in one clause " +
		"each — the full definitions are with the components below:\n\n")
	for _, m := range seenMetrics {
		if n, ok := metricNames[m]; ok {
			p("- **%s** — %s.\n", metricShort(m), firstClause(n.long))
		}
	}
	p("\n")

	p("**The season F test cannot be used to pick an end of the range.** It tests " +
		"whether the effect differs between seasons at all, and a small p licenses " +
		"the clustered figure. A large p does *not* license the other end")
	if fPower > 0 {
		// The power figure is computed for the grid actually run; the season count
		// is not, so it is not restated. Corrected 2026-08-15 with the two siblings.
		p(", because on this grid that test has only **%.0f%% power** against a "+
			"season component large enough to double the clustered variance — it fails "+
			"to reject four times in five when the thing it looks for is there",
			100*fPower)
	}
	p(". So both are reported, with the evidence beside them, and a reader who needs " +
		"one number should take the conservative end.\n\n")

	// The comparison against the constants' own size is computed rather than
	// asserted, because which side of it a snapshot lands on depends on the sweeps
	// in it — and a hardcoded "all of them sit below the threshold" would be a
	// claim the numbers above could contradict on the same page.
	const typicalConstant = 34.0 // the top of the 11-to-34 range these are worth
	p("For scale: nearly every constant argued over in this project is worth 11 to "+
		"34 points a season. Taking the conservative end of every range above, the "+
		"comparisons run from **%.0f to %.0f points**, ", best, worst)
	switch {
	case best > typicalConstant:
		p("so **every one of those constants sits below what any comparison here " +
			"could detect**. ")
	case worst <= typicalConstant:
		p("so a constant of that size is within reach of every comparison here — " +
			"which is unusual and worth checking, since it normally means the change " +
			"lands almost identically in every cell. ")
	default:
		p("so a constant of that size is within reach of the finest comparison here "+
			"and well below the coarsest. **Which comparison a verdict came from "+
			"therefore decides whether \"unresolved\" was ever avoidable** — a "+
			"mechanism-certain change resolves at %.0f points where a scoring constant "+
			"needs %.0f. ", best, worst)
	}
	p("A properly inferred, multiplicity-corrected re-judgement should be " +
		"*expected* to return \"unresolved\" for most scoring constants, and that is " +
		"compatible with the effects being real. The three legitimate responses are " +
		"to decide on mechanism (does the objective say what the game pays?), to " +
		"decide on shape (a plateau with a cliff, or monotonicity across several " +
		"settings, pools information a single comparison cannot), or to buy " +
		"resolution with more entry gameweeks where the components say that " +
		"helps.\n\n")
	v.setf("harness.mde_season.finest", best, 0)
	v.setf("harness.mde_season.coarsest", worst, 0)
}

// pooledP formats the pooled row's season F-test p-value, which R takes from the
// strictest arm rather than by averaging — averaging p-values tests nothing, and
// on the transfer metric it would license the narrow estimator on a metric whose
// season component is demonstrably real in one arm.
//
// A dash rather than 0.000 when the test could not run: an F ratio of 0/0 reads as
// the most significant value there is, for a metric that did not move.
func pooledP(row MDE) string {
	if !row.HasPSeason {
		return "—"
	}
	return fmt.Sprintf("%.3f (strictest arm)", row.PSeason)
}

func renderStamp(b *strings.Builder, in Inputs, v *Values) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("## Provenance\n\n")
	p("Every expensive failure in this project's history is a provenance failure " +
		"rather than an arithmetic one. A whole body of evidence was measured with the " +
		"transfer gate's minimum-gain threshold at 0.7, the value was retracted to " +
		"0.4 three commits later, nothing recorded the link, and a later audit cited " +
		"the evidence as ground truth. Separately, a six-arm sweep was killed under " +
		"load after three arms and the gap was invisible until somebody counted rows. " +
		"So:\n\n")

	dirty := ""
	if in.Dirty {
		dirty = " — **working tree was dirty**, so this commit alone does not identify " +
			"the code that ran"
	}
	p("| | |\n|---|---|\n")
	p("| snapshot taken | %s |\n", in.Date.Format("2006-01-02 15:04 MST"))
	p("| commit | `%s`%s |\n", shortSHA(in.Commit), dirty)
	if in.Branch != "" {
		p("| branch | `%s` |\n", in.Branch)
	}
	p("| cells file | `%s` |\n", orDash(in.CellsPath))
	p("| model file | `%s` |\n", orDash(in.ModelPath))
	for _, inf := range in.Inference {
		p("| inference for %s | `%s` |\n", inf.Source, orDash(inf.Dir))
	}
	p("\n")
	v.set("stamp.commit", shortSHA(in.Commit))
	v.set("stamp.dirty", strconv.FormatBool(in.Dirty))

	// ⚠️ Operator notes are rendered HERE, before the no-cells early return, and
	// that position is the whole point.
	//
	// They used to sit at the end of this function, after that return — so a
	// model-only snapshot silently DISCARDED every `-note`. The operator typed a
	// caveat, got no warning, and the artefact whose entire job is provenance
	// shipped without it. That is the shape the flag exists for: the staleness
	// guard's own recipe passes `-model` and no `-cells`, so the common case was
	// the broken one.
	//
	// Found 2026-08-14 while stamping the attribution for a seven-figure movement
	// that was a harness fix rather than a model change — exactly the caveat a
	// reader would otherwise have to be told separately, which means eventually
	// not be told. TestOperatorNotesSurviveASnapshotWithNoCells pins it.
	//
	// Before the return also means before the sweep sections, which is the better
	// reading order anyway: a caveat qualifying the whole snapshot belongs above
	// the tables it qualifies.
	renderNotes(b, in)

	if len(in.Sweeps) == 0 {
		p("**No sweep cells were supplied**, so there is no grid, no arm accounting " +
			"and no constants fingerprint in this snapshot. The harness half above, if " +
			"present, came from an inference directory whose provenance is therefore " +
			"unverified.\n\n")
		return
	}

	for _, s := range in.Sweeps {
		p("### Sweep `%s`\n\n", s.Label)
		if !s.HasProv {
			p("**No provenance sidecar for this sweep.** It predates the stamping, or " +
				"the sidecar was not carried along with the cells file. So the constants " +
				"in force are unknown, and — the part that matters — **there is no way " +
				"to tell whether every arm ran.** Read every figure derived from it as " +
				"unattributable.\n\n")
		}

		p("| | |\n|---|---|\n")
		if s.HasProv {
			d := ""
			if s.Prov.Dirty {
				d = ", tree dirty"
			}
			p("| ran at commit | `%s`%s |\n", shortSHA(s.Prov.Commit), d)
			p("| constants fingerprint | `%s` |\n", s.Prov.Digest)
			v.set("sweep."+s.Label+".digest", s.Prov.Digest)
			v.set("sweep."+s.Label+".commit", shortSHA(s.Prov.Commit))
		}
		p("| seasons replayed | %s |\n", joinOrDash(s.Seasons))
		p("| entry gameweeks | %s |\n", joinInts(s.StartGWs))
		p("| cells per arm | %d seasons x %d entry points = %d |\n",
			len(s.Seasons), len(s.StartGWs), len(s.Seasons)*len(s.StartGWs))
		p("| free-transfer bank | %s |\n", bankNote(s))
		p("| captaincy rungs emitted | %s |\n", yesNo(s.HoldRungs))
		p("\n")
		v.set("sweep."+s.Label+".seasons", strings.Join(s.Seasons, " "))
		v.set("sweep."+s.Label+".starts", joinInts(s.StartGWs))

		renderArms(b, s, v)
		renderInvariance(b, s, v)
	}

}

// renderNotes writes the operator's stamped caveats. It is called from
// renderStamp BEFORE the no-cells early return, so a model-only snapshot keeps
// them — see the comment at the call site for why that is not a detail.
func renderNotes(b *strings.Builder, in Inputs) {
	if len(in.Notes) == 0 {
		return
	}
	p := func(f string, a ...any) { fmt.Fprintf(b, f, a...) }
	p("### Operator notes\n\n")
	p("Stamped in by hand at snapshot time. These are the caveats a reader would " +
		"otherwise have to be told separately, which means eventually not be told.\n\n")
	for _, n := range in.Notes {
		p("- %s\n", n)
	}
	p("\n")
}

// renderArms is the killed-arm accounting. Silence must never read as success.
func renderArms(b *strings.Builder, s Sweep, v *Values) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	expected := len(s.Seasons) * len(s.StartGWs)
	var killed, partial []string
	// Whether a declaration exists at all. Without one, "not in the declaration" is
	// true of every arm and says nothing — it would put a warning on five healthy
	// rows and train the reader to skip the column. The absence is reported once,
	// below the table, where it belongs.
	declared := false
	for _, a := range s.Arms {
		declared = declared || a.Declared
	}

	p("**Arms**\n\n")
	p("| setting | role | cells run | cells infeasible | status |\n")
	p("|---|---|---:|---:|---|\n")
	for _, a := range s.Arms {
		role := "alternative"
		if a.IsBaseline {
			role = "**baseline**"
		}
		status := "complete"
		switch {
		case !a.Ran:
			status = "**KILLED — declared and emitted nothing**"
			killed = append(killed, a.Label)
		case a.Cells+a.Infeasible < expected && expected > 0:
			status = fmt.Sprintf("**PARTIAL — %d of %d cells**", a.Cells+a.Infeasible, expected)
			partial = append(partial, a.Label)
		case a.Infeasible > 0:
			status = fmt.Sprintf("complete, %d cells could not field a legal fifteen", a.Infeasible)
		case declared && !a.Declared:
			// Only meaningful when there *is* a declaration to be absent from. An
			// arm that ran without being declared means the two records disagree,
			// which is worth flagging on its own.
			status = "complete, but not in the declaration"
		case !declared:
			status = "ran; completeness unverifiable"
		}
		p("| %s | %s | %d | %d | %s |\n", a.Label, role, a.Cells, a.Infeasible, status)
	}
	p("\n")

	v.set("sweep."+s.Label+".arms", strconv.Itoa(len(s.Arms)))
	v.set("sweep."+s.Label+".arms_killed", strconv.Itoa(len(killed)))
	v.set("sweep."+s.Label+".arms_partial", strconv.Itoa(len(partial)))

	switch {
	case len(killed) > 0 || len(partial) > 0:
		p("**This sweep is incomplete and its figures are not a full grid.** ")
		if len(killed) > 0 {
			p("Killed outright: %s. ", strings.Join(killed, "; "))
		}
		if len(partial) > 0 {
			p("Partial: %s. ", strings.Join(partial, "; "))
		}
		p("A missing arm is not a neutral absence — every comparison is paired " +
			"*within* a cell, so an arm short of cells is compared on a different " +
			"population from the one it is being read against. Re-run before quoting " +
			"anything from it.\n\n")
	case !s.HasProv:
		p("Arm completeness **cannot be checked** without a provenance sidecar: the " +
			"table above lists the arms that emitted cells, which is not the same as " +
			"the arms that were asked for.\n\n")
	default:
		p("Every declared arm ran every cell.\n\n")
	}
}

func renderInvariance(b *strings.Builder, s Sweep, v *Values) {
	if len(s.Invariance) == 0 {
		return
	}
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("**Invariance checks**\n\n")
	p("A quantity the change under test must *not* move, and whether it moved. " +
		"These are worth far more than they cost: a violation shows up in a single " +
		"cell, where confirming an effect needs the whole grid and still usually " +
		"fails. Falsification is cheap here and confirmation is not.\n\n")
	p("Note both directions are informative. For a knob that only touches the " +
		"weekly transfer decision, an unmoved HOLD is the check passing. For a " +
		"scoring knob, HOLD *should* move, and these rows are then a description " +
		"rather than a failure — which of the two a sweep is testing is a judgement " +
		"the cells file does not carry.\n\n")
	p("| quantity | arms compared | cells | cells that differ | verdict |\n")
	p("|---|---:|---:|---:|---|\n")
	for _, iv := range s.Invariance {
		verdict := "**byte-identical in every cell**"
		if iv.Differs > 0 {
			verdict = fmt.Sprintf("moved in %d cells (worst: %s)", iv.Differs, iv.Example)
		}
		p("| %s | %d | %d | %d | %s |\n", iv.Metric, iv.Arms, iv.Cells, iv.Differs, verdict)
		v.set("sweep."+s.Label+".invariance_differs."+slug(iv.Metric),
			strconv.Itoa(iv.Differs))
	}
	p("\n")
}

func renderHarness(b *strings.Builder, in Inputs, v *Values) {
	if !anyComponents(in.Inference) {
		return
	}
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("## Harness accuracy: where the noise comes from\n\n")
	p("Most of the replay's \"noise\" is sensitivity rather than randomness: a " +
		"hair's-breadth score change flips one discrete transfer, and that transfer " +
		"changes the squad for every remaining week. Splitting that spread decides " +
		"what can be done about it.\n\n")
	p("**It is not ALL sensitivity, and this line used to claim it was.** `Optimize` " +
		"returns two different fifteens from byte-identical inputs on one engine — " +
		"measured at 48.643364 against 48.206244 in `XIScore`, about 17 points a " +
		"season — so part of the spread below is genuine non-determinism that nobody " +
		"has separated out. It also means a byte-identical invariance may have held by " +
		"luck. See `TestDiagOptimizerIsNotDeterministic`.\n\n")
	p("- **Season-to-season disagreement** means the effect genuinely differs " +
		"between seasons. Only more seasons help, and there are four.\n")
	p("- **Within-season path noise** means the effect is the same and the path " +
		"through it differed. More entry gameweeks average that away, and entry " +
		"gameweeks are cheap — linear runtime, no new football needed.\n\n")
	p("All figures are points per gameweek; multiply by 38 for a season. " +
		"`season F test p` is the test of whether the season component exists at " +
		"all, and a small value means it does.\n\n")

	p("| source sweep | metric | season-to-season | by entry gameweek | path noise | " +
		"spread of the season means | of which season | of which path | " +
		"season F test p |\n")
	p("|---|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, inf := range in.Inference {
		for _, c := range inf.Components {
			row, ok := inf.PrimaryMDE(c.Metric)
			if ok && row.Degenerate {
				p("| %s | **%s** | exact invariance: every paired difference is zero, "+
					"so there is no variance to split |\n",
					inf.Source, metricShort(c.Metric))
				continue
			}
			// "—" rather than 0.000 when the F test could not run. An F ratio of 0/0
			// reads as p = 0, which is the most significant value there is, for a
			// metric that did not move.
			pv := "—"
			if ok && row.HasPSeason {
				pv = fmt.Sprintf("%.3f", row.PSeason)
			}
			p("| %s | **%s** | %.3f | %.3f | %.3f | %.3f | %.0f%% | %.0f%% | %s |\n",
				inf.Source, metricShort(c.Metric), c.SDSeason, c.SDStart, c.SDResid,
				c.SDSeasonMap, c.ShareSeason, c.SharePath, pv)
			key := "harness." + slug(inf.Source) + "."
			v.setf(key+"sd_season."+c.Metric, c.SDSeason, 3)
			v.setf(key+"sd_resid."+c.Metric, c.SDResid, 3)
			v.setf(key+"share_season."+c.Metric, c.ShareSeason, 0)
		}
	}
	p("\n")
	p("A season component estimated at zero is not proof of zero — a handful of " +
		"seasons gives it a handful of degrees of freedom, and a component measured " +
		"at zero and one as large as the whole spread are not distinguishable at " +
		"that sample size. Read a zero as \"the best available reading\", not as a " +
		"fact.\n\n")

	p("### What each metric is\n\n")
	seen := map[string]bool{}
	for _, inf := range in.Inference {
		for _, m := range inf.Metrics() {
			if n, ok := metricNames[m]; ok && !seen[m] {
				seen[m] = true
				p("- **%s** — %s\n", n.short, n.long)
			}
		}
	}
	p("\n")

	for _, inf := range in.Inference {
		if len(inf.Missing) > 0 {
			p("**Absent for %s:** %s. Recorded rather than skipped, because a section "+
				"that quietly omitted its numbers would look much like a section that had "+
				"nothing to say.\n\n", inf.Source, strings.Join(inf.Missing, ", "))
		}
	}
}

func anyMDE(list []Inference) bool {
	for _, inf := range list {
		if len(inf.MDE) > 0 {
			return true
		}
	}
	return false
}

func anyComponents(list []Inference) bool {
	for _, inf := range list {
		if len(inf.Components) > 0 {
			return true
		}
	}
	return false
}

func renderModel(b *strings.Builder, in Inputs, v *Values) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("## Model accuracy: is the scoring model right about football?\n\n")
	if len(in.Model) == 0 {
		p("**Not measured in this snapshot.** No model-accuracy CSV was supplied, so " +
			"this half is absent rather than clean.\n\n")
		v.set("model.present", "false")
		return
	}
	v.set("model.present", "true")
	p("Measured against outcomes rather than against another setting of the model, " +
		"so these figures do not carry the harness's standard errors: their unit is a " +
		"player-cutoff or a team-match, not a replayed season, and several rest on " +
		"thousands of observations where the harness has twenty-four. That makes this " +
		"the more trustworthy half — and the half that cannot tell you whether acting " +
		"on a bias would gain points, which is a separate and much harder question " +
		"this project has answered \"no\" to five times.\n\n")

	for _, d := range in.Model {
		p("### %s\n\n", d.Title)
		if d.What != "" {
			p("%s\n\n", d.What)
		}
		p("*Population: %s.*\n\n", d.Grid)

		p("| |")
		if hasN(d) {
			p(" n |")
		}
		for _, m := range d.Measures {
			p(" %s |", m)
		}
		p("\n|---|")
		if hasN(d) {
			p("---:|")
		}
		for range d.Measures {
			p("---:|")
		}
		p("\n")
		for _, g := range d.Groups {
			p("| %s |", g)
			if hasN(d) {
				p(" %d |", d.N[g])
			}
			for _, m := range d.Measures {
				val, ok := d.Values[g][m]
				if !ok {
					p(" — |")
					continue
				}
				p(" %s |", trim3(val))
				v.setf("model."+d.Slug+"."+slug(g)+"."+slug(m), val, 4)
			}
			p("\n")
		}
		p("\n")
		if d.Reading != "" {
			p("**Reading it.** %s\n\n", d.Reading)
		}
	}

	p("### What is not measured here, and why\n\n")
	p("**No section here prices a change in points.** Every figure above is measured " +
		"against outcomes — what the players went on to do — and the unit is a " +
		"player-gameweek or a team-match rather than a replayed season. That is what " +
		"makes these the more trustworthy half: several rest on tens of thousands of " +
		"observations where the harness half has twenty-four cells. It is also what " +
		"they cannot do. A predictor that wins here is a candidate worth spending " +
		"replay time on, not a candidate already proved, and this project has a " +
		"measured case where about 2 per cent lower out-of-sample error cost roughly " +
		"49 points a season: a transfer policy is an argmax living in the tail of the " +
		"estimate distribution, so accuracy bought on the average player is paid for " +
		"with noise where the search looks.\n\n")
	p("The predictor comparison for minutes, points and expected goals **is now " +
		"runnable** and appears above as \"How much should a predictor weight recent " +
		"gameweeks?\". An earlier edition of this snapshot had to record its absence: " +
		"the figures were frozen prose in `internal/analysis` and `internal/recent` " +
		"with no code that could recompute them, which is precisely the orphaned " +
		"measurement this artefact exists to prevent. Read the relative columns rather " +
		"than the levels when comparing against those comments — the population behind " +
		"the original was never written down, and it predates the doubles-counting fix " +
		"that changed what a gameweek means in this archive.\n\n")
}

// renderDiff distinguishes a figure that moved because the model changed from one
// that moved because the harness did.
//
// Both halves are diffed against the same previous snapshot, and the constants
// fingerprint is diffed beside them — so "the model's calibration ratio moved and
// no constant changed" and "it moved and here is the constant that moved" are
// different readings rather than the same unexplained shift.
func renderDiff(b *strings.Builder, in Inputs, v *Values) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("## Change since the previous snapshot\n\n")
	if in.Previous == nil {
		p("**This is the first snapshot.** There is nothing to diff against, so every " +
			"figure above is a baseline rather than a movement. The next snapshot will " +
			"carry the comparison.\n\n")
		v.set("diff.baseline", "true")
		return
	}
	v.set("diff.baseline", "false")
	p("Compared against `%s`.\n\n", in.PrevName)

	type change struct {
		key           string
		before, after string
		delta         float64
		hasDelta      bool
	}
	// Keys describing the diff itself are excluded from it. `diff.baseline` is true
	// for a first snapshot and false for every one after, so comparing it would put
	// a spurious row at the head of every future diff — and a diff whose first entry
	// is always noise is one a reader learns to skip.
	comparable := func(k string) bool { return !strings.HasPrefix(k, "diff.") }

	var moved, added, gone []change
	for _, k := range v.Keys {
		if !comparable(k) {
			continue
		}
		now := v.Value[k]
		was, ok := in.Previous.Value[k]
		if !ok {
			added = append(added, change{key: k, after: now})
			continue
		}
		if was == now {
			continue
		}
		c := change{key: k, before: was, after: now}
		wf, e1 := strconv.ParseFloat(was, 64)
		nf, e2 := strconv.ParseFloat(now, 64)
		if e1 == nil && e2 == nil {
			c.delta, c.hasDelta = nf-wf, true
		}
		moved = append(moved, c)
	}
	for _, k := range in.Previous.Keys {
		if !comparable(k) {
			continue
		}
		if _, ok := v.Value[k]; !ok {
			gone = append(gone, change{key: k, before: in.Previous.Value[k]})
		}
	}

	if len(moved) == 0 && len(added) == 0 && len(gone) == 0 {
		p("**Nothing moved.** Every figure is identical to the previous snapshot, " +
			"which for a deterministic replay over a fixed archive is what an unchanged " +
			"model and an unchanged harness look like.\n\n")
		return
	}

	if len(moved) > 0 {
		p("### Figures that moved\n\n")
		p("| figure | previous | now | change |\n|---|---:|---:|---:|\n")
		sort.Slice(moved, func(i, j int) bool { return moved[i].key < moved[j].key })
		for _, c := range moved {
			d := "—"
			if c.hasDelta {
				d = fmt.Sprintf("%+g", roundTo(c.delta, 4))
			}
			p("| `%s` | %s | %s | %s |\n", c.key, c.before, c.after, d)
		}
		p("\n")
		p("**Attributing a movement.** Check the constants fingerprint rows first. A " +
			"figure that moved while the fingerprint held means the code changed and no " +
			"setting did — a scoring fix, a harness fix, or a bug. A figure that moved " +
			"*with* the fingerprint means a setting changed, and the constants diff " +
			"below names which.\n\n")
	}
	if len(added) > 0 {
		p("### Newly measured\n\n")
		for _, c := range added {
			p("- `%s` = %s\n", c.key, c.after)
		}
		p("\n")
	}
	if len(gone) > 0 {
		p("### No longer measured\n\n")
		p("Present in the previous snapshot and absent here. This is the case that " +
			"must not be mistaken for a clean result: a diagnostic that did not run is " +
			"not a diagnostic that found nothing.\n\n")
		for _, c := range gone {
			p("- `%s` was %s\n", c.key, c.before)
		}
		p("\n")
	}
}

func renderHowToRegenerate(b *strings.Builder, in Inputs) {
	p := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	p("## Regenerating this\n\n")
	p("See `stats/README.md` for the full recipe and the runtimes. In short: run a " +
		"sweep with `FPL_CELLS` set, which writes its own provenance; run the " +
		"calibration diagnostics with `FPL_MODEL_CSV` set; then `armband snapshot`, " +
		"which invokes the R inference and renders this file.\n\n")
	p("**Constants in force at the sweeps above** are recorded in the provenance " +
		"sidecar beside the cells file, not inlined here — there are over a hundred " +
		"of them and the fingerprint is what a reader needs. `armband snapshot " +
		"--constants` prints the full list for a fingerprint.\n")
}

// --- small helpers ---------------------------------------------------------

func hasN(d Diagnostic) bool {
	for _, g := range d.Groups {
		if d.N[g] > 0 {
			return true
		}
	}
	return false
}

func bankNote(s Sweep) string {
	if len(s.Banks) == 0 {
		return "unknown"
	}
	if len(s.Banks) == 1 && s.Banks[0] == 5 {
		return "5 for every cell — **historically wrong for 2022-23 and 2023-24**, " +
			"which ran a two-transfer bank. Deliberate: a setting compared across " +
			"cells governed by different transfer rules adds a nuisance factor that " +
			"interacts with the very knobs being swept. It means absolute totals from " +
			"this grid describe a hypothetical run under one rule set, and only the " +
			"paired differences carry across."
	}
	return joinInts(s.Banks) + " (varies by cell)"
}

func firstClause(s string) string {
	if i := strings.Index(s, "."); i > 0 {
		return s[:i]
	}
	return s
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "not supplied"
	}
	return s
}

func joinOrDash(xs []string) string {
	if len(xs) == 0 {
		return "unknown"
	}
	return strings.Join(xs, ", ")
}

func joinInts(xs []int) string {
	if len(xs) == 0 {
		return "unknown"
	}
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ", ")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// slug makes a stable key fragment out of a human label, so the values file is
// diffable across runs even when a label is rewritten for readability.
func slug(s string) string {
	var out []rune
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
			prevDash = false
		default:
			if !prevDash && len(out) > 0 {
				out = append(out, '_')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(string(out), "_")
}

func trim3(v float64) string {
	return strconv.FormatFloat(roundTo(v, 3), 'f', -1, 64)
}

func roundTo(v float64, dp int) float64 {
	pow := 1.0
	for i := 0; i < dp; i++ {
		pow *= 10
	}
	return float64(int64(v*pow+copysign(0.5, v))) / pow
}

func copysign(v, sign float64) float64 {
	if sign < 0 {
		return -v
	}
	return v
}
