package snapshot

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// retracted is a figure the record has withdrawn, plus the phrase that marks a
// legitimate mention of it.
//
// Every entry is a number that was published, cited downstream, and later found not to
// reproduce. They keep reappearing because the record is long and a reader — human or
// agent — greps for a quantity and takes the first hit. This is the cheap half of the
// findings audit: it needs no judgement, so it should not cost an agent.
type retracted struct {
	figure  string   // the literal that must not appear bare
	what    string   // what it was, in plain terms
	context []string // at least one must be on the same line, or the number means something else
	unless  []string // ...but none of these, or it is a DIFFERENT live quantity
	now     string   // what to say instead
}

// The list is deliberately short and literal. It is not an attempt to police prose —
// it catches the specific case of a withdrawn number being quoted as current, which has
// happened repeatedly and is the failure the whole retraction discipline exists for.
//
// # Context is mandatory, and the first version of this test proved why
//
// Written as a bare substring search, it reported 28 sites and nearly all were
// coincidence: `0.53` is also Isak's score per gameweek in three separate passages, and
// it matched inside `+0.531`, a bias figure in the bonus table. A guard that fires on
// legitimate prose gets deleted, and then it guards nothing — the same reasoning already
// recorded against the snapshot guard's permissiveness.
//
// So a figure only counts when a word identifying *that quantity* is on the same line.
// That is the difference between a grep and a check.
var retractedFigures = []retracted{
	// ⚠️ The three bench-shape figures retracted on 2026-08-13 — 77, 51 and 79 —
	// are DELIBERATELY ABSENT, and the attempt is recorded because the next
	// person will have the same idea.
	//
	// An audit found three surviving copies of them after the retraction was
	// applied to internal/analysis/squad.go and nowhere else, two of them inside
	// an open PRIORITY item where a withdrawn number was ordering the work. That
	// is precisely this guard's job, so they were added.
	//
	// They fired on **seven** legitimate lines and every one was a coincidence:
	// "77 min/start" in docs/model.md, "| 0 (flat) | 8060 |" and "| flat (no
	// recency) | 28 | 51 |" in the harness note, "| flat (shipped) | 2115 |" in
	// the recency note, a season-clustered t of −5.51. These are bare two-digit
	// integers against context words — "flat", "bench", "held out" — that are
	// among the commonest words in this record, and the guard matches any one
	// context word on the line rather than all of them.
	//
	// The rule above decides it: a guard that fires on legitimate prose gets
	// deleted, and then it guards nothing. **A figure has to be distinctive
	// enough to key on before this mechanism can hold it**, which "0.53" and
	// "−0.709" are and "77" is not. Those three are held by the in-place
	// retraction markers instead, which an audit checks and this cannot.
	//
	// ⚠️ The transfer-gate bar demoted on 2026-08-15 — **0.89 / 89%, and the 94 and
	// 106 it is built from** — is DELIBERATELY ABSENT too, for a different reason
	// from the three above, and it is recorded so the next auditor does not re-derive
	// it. Those three are *not distinctive enough*; this one is the opposite problem.
	//
	// **0.89 is still quoted legitimately, in the sentence that does the retracting.**
	// What was demoted is 0.89's meaning as a BAR a constant must clear — a property
	// of the four-season comparison it was computed on. The Fieller rejections of 0.89
	// and 1.00 are facts about the recovered fraction and they STAND, so the figure
	// stays live in AGENTS.md and in internal/analysis/xpoints.go. An entry here would
	// fire on the correction itself, which is the guard eating its own fix.
	//
	// **94 and 106 collide with unrelated live quantities**, and no `context` word
	// separates them: "+106 HOLD" is the doubles-half-counted contamination event, and
	// the gate family's own context words — "gate", "transfer", "threshold" — are
	// among the commonest in the record. Compare the "322" entry, which works only
	// because "team news" and "oracle" name one quantity and nothing else.
	//
	// So all three are held by in-place retraction markers instead — in
	// stats/findings/2026-08-15-gatescaled.md, which carries the withdrawn
	// wording verbatim, in the demotion blocks in internal/backtest/gate.go and
	// internal/backtest/gatexpoints_diag_test.go, which is where the 94 is named as
	// the perfect arm's own threshold rather than a constant's, and by deletion in
	// AGENTS.md, which is verdict-only. An audit checks those; this cannot.
	{
		figure:  "0.53",
		what:    "the buy-side over-rating per gameweek",
		context: []string{"over-rat", "overrat", "buy-side", "buy side"},
		now:     "it does not reproduce at shipped config: −0.230 median, +0.079 mean",
	},
	// ⚠️ This entry is withdrawn for INADMISSIBILITY, not for failing to reproduce,
	// and it is the first of that kind on this list. 3.30 reproduces exactly — it is
	// the season×team t on the six-season stratum. What is wrong is quoting it at
	// all: fit.txt heads that stratum "POOLED STRATUM -- CONTEXT ONLY, NO VERDICT"
	// and disowns it in either direction, because three of its six seasons carry
	// reconstructed xGC so w1 is not one construct. So `now` must not say "does not
	// reproduce" — a reader who tries will succeed and conclude the guard is wrong.
	//
	// Added 2026-08-17, when the last in-scope copy was removed from AGENTS.md.
	//
	// TWO copies survive and neither is in scope, and only one of them is out by
	// design. reviews/2026-08-15-the-clean-sheet-regressor-refit is out by design — a
	// review record is a dated attestation about a named commit and must not be
	// rewritten. But stats/findings/2026-08-15-clean-sheet-2x2.md is NOT out by
	// design: the surface below globs stats/*.md NON-RECURSIVELY, so stats/findings/
	// is scanned by nothing at all — and AGENTS.md points a reader straight at that
	// file ("Sizes in ..."), two lines above the bullet this figure was cut from. It
	// carries both withdrawn readings, t 3.30 and "6 of 6 pooled". Marked in place
	// there instead. ⚠️ Widening the glob is the real fix and is NOT done here.
	//
	// Scoping: "3.30" appears nowhere else in scope, so there is no collision
	// surface. ⚠️ Do NOT add 1.5654 or 0.1712 — they are the pooled control the
	// hindsight run is required to reproduce, and the surface that would fire is
	// stats/defensive_fixture_pointintime_PREREGISTRATION.md, which carries both on
	// one line beside the word "pooled". NOT the .R scripts: stats/*.R is outside
	// this guard and inside TestNoLivePointerCitesTheRecordByPath, which is the one
	// difference between their surfaces.
	{
		figure:  "3.30",
		what:    "the defensive fixture ladder's t on the six-season POOLED stratum",
		context: []string{"pooled"},
		now:     "it reproduces but is inadmissible: that stratum carries no verdict in either direction. The native stratum reads t 4.14 against a t_crit(G−1 = 2) of 4.303 and does not resolve",
	},
	{
		figure:  "−0.709",
		what:    "the minutes-convexity exponent's effect against exponent 1.00",
		context: []string{"minutesweight", "convexity", "exponent"},
		now:     "re-swept with every sign flipped; 1.25 was the worst of five and no longer ships — the user moved the default to 1.0 (neutral) on 2026-08-25 rather than defend an un-locatable optimum",
	},
	{
		figure:  "−0.717",
		what:    "the minutes-convexity exponent's effect against exponent 1.00",
		context: []string{"minutesweight", "convexity", "exponent"},
		now:     "re-swept with every sign flipped; 1.25 was the worst of five and no longer ships — the user moved the default to 1.0 (neutral) on 2026-08-25 rather than defend an un-locatable optimum",
	},
	{
		figure: "322",
		what:   "the team-news oracle's held gain",
		// "oracle" and "team news" are the phrases that identify the quantity; 322
		// is otherwise a plausible points total.
		context: []string{"team news", "oracle", "availability"},
		// It IS sourced now — TestDiagAvailabilityOracle measures it — and the answer
		// is that it was a raw twelve-cell total read as a season figure. The `now`
		// field has to say what the record currently holds, or the guard teaches the
		// stale position it was written to retire.
		//
		// This field has now done exactly that once, which is why the warning above is
		// phrased as a rule. It named +183 as "the quantity that does resolve" — and
		// +183 was itself relabelled two commits later, so the guard spent that window
		// teaching a superseded position while passing. A `now:` field is prose inside
		// a test: nothing checks it, so it rots exactly like the record it polices.
		now: "a raw twelve-cell total read as a season figure. Measured on the standard " +
			"24-cell grid by TestDiagAvailabilityOracle, perfect team news is +14 a season " +
			"held and does not resolve. Nor does anything else in the family: the bounded " +
			"oracles are +73 for LINEUPS (CR2 t = 1.32) and +47 for MINUTES (t = 0.62). " +
			"Do not reach for +183 as the resolving one — that arm measures a " +
			"SEASON-AVERAGE window, correct arithmetic under a wrong label",
	},
	{
		figure:  "+16",
		what:    "the price-timing ceiling",
		context: []string{"price timing", "price"},
		// `+16` became AMBIGUOUS: it is also the availability oracle's measured value
		// (+16 points a season held), and the sentence comparing the two naturally
		// contains both "price timing" and "+16". Context alone cannot separate a
		// retracted figure from a live one that shares a literal, so an exclusion is
		// needed — the alternative is a guard that fires on the very correction it
		// was written to enforce, which is how a guard earns deletion.
		unless: []string{"team news", "held", "availability", "oracle"},
		// NOT +5.6. That figure came from the arm that bought low and SOLD LOW —
		// `decide` quoted the search the window maximum and the wallet credited the
		// minimum — so every price-timing number ever recorded, including the one
		// this field used to name, came from an incoherent policy. Corrected at
		// `3834334`, which roughly tripled both the bound and its standard error.
		now: "+15 a season on the corrected arm, CR2 t = 0.95 against a threshold near " +
			"50 — still \"too small for this harness to see\", but from an arm that is " +
			"now a coherent upper bound",
	},
	{
		figure: "+273",
		what:   "the availability reconstruction's held gain",
		// "jitter floor" is the phrase the original claim used, and it is how the
		// figure was justified.
		context: []string{"availability", "jitter floor", "reconstruct"},
		now: "about 8 points a season at 24 cells; +273 was three cells in the one " +
			"season Kane's move overlapped a GW1 deadline",
	},
	{
		figure:  "1.72",
		what:    "the premium-acquisition over-valuation",
		context: []string{"over-valuation", "over-estimate", "overvalu", "premium"},
		now:     "+1.242 with SE 1.019 at t = +1.22 — it was never a measurement",
	},
	{
		figure: "183",
		what:   "the minutes oracle's held gain, cited as the one oracle that resolves",
		// "oracle" and "team news" name the quantity. `minutes` alone would be far too
		// broad — half the record is about minutes — so it is deliberately absent.
		context: []string{"oracle", "team news", "information bound"},
		// The arm itself is a CORRECT measurement of a season-average window, and the
		// record legitimately tabulates it as one. What is withdrawn is the claim that
		// it bounds knowing a player's trajectory. So a line that says which window it
		// measures is the fixed version, not an offence.
		unless: []string{"season-average", "season average", "window"},
		now: "correct arithmetic under a wrong LABEL: it measures a season-average " +
			"oracle, not knowledge of the trajectory. The bounded arms are ≈73 for " +
			"lineups (CR2 t = 1.32) and ≈47 for minutes (t = 0.62), and neither resolves",
	},
	{
		figure:  "+5.6",
		what:    "the price-timing ceiling, from the arm that bought low and sold low",
		context: []string{"price", "timing", "oracle"},
		now: "+15 a season on the corrected arm, CR2 t = 0.95 — the old arm quoted the " +
			"search the window maximum while the wallet credited the minimum, so it was " +
			"never a coherent bound in either direction",
	},
	{
		figure:  "+10.8",
		what:    "the gain from anchoring the chips on the calendar",
		context: []string{"anchor", "chip", "calendar"},
		now: "refuted as a measurement — four estimators, the largest t is 1.39, and Holm " +
			"adjusts every p to 1.000. ⚠️ This does NOT establish that anchoring is worth " +
			"nothing: it now RESOLVES at +20.6 a season-path (4gw sight, CR2 t 3.63, " +
			"threshold 14.5) on clean banked cells at stats/cells/2026-08-25-f7d2be1b",
	},
	{
		figure: "+63",
		what: "the gain from anchoring the chips on the calendar, published 2026-08-25 " +
			"and wrong in two independent ways at once",
		// "anchor"/"chip"/"calendar" name the quantity; +63 is otherwise an
		// ordinary two-digit figure anywhere in the record.
		context: []string{"anchor", "chip", "calendar"},
		now: "+20.6 a season-path, CR2 t 3.63, threshold 14.5. The +63 was read on the " +
			"default per_gw scale — wrong for an event count, ~1.7x inflation — AND " +
			"measured on a tree carrying an uncommitted MinutesWeight change while its " +
			"sidecar stamped a commit shipping the old value. Read the sidecar's dirty " +
			"flag, and pass --scale=per_path",
	},
	{
		figure: "0.977",
		what:   "the Spearman between P(flip) and |t|, cited as evidence for rank-scoring",
		// "spearman" and "flip" name the quantity; the literal is otherwise a plausible
		// correlation or rate anywhere in the record.
		context: []string{"spearman", "flip", "rank"},
		now: "an algebraic identity recovered by simulation, not a measurement — under " +
			"data-independent weights P(flip) has a closed form in |t| alone that " +
			"reproduces every arm to 0.74 points. The real test computes the reweighting " +
			"rather than drawing it, where the correlation is −0.315 and the verdict holds",
	},
	{
		figure: "0.1112",
		what:   "the appearance fit's rms, cited as evidence the fit was poor",
		// "rms" alone would match the many legitimate rms figures elsewhere; the
		// retracted use is specifically about the appearance or sixty-minute fit.
		context: []string{"fit", "appear", "sixty", "60"},
		now: "it compared two curves against different floors — the binomial sampling floor " +
			"is 0.065 and 0.062, so most of the apparent misfit was noise in the target",
	},
}

// quotesFigure reports whether line mentions the figure as a complete number rather
// than as a prefix of a longer one — `+0.531` must not read as `0.53`.
func quotesFigure(line, figure string) bool {
	for i := 0; ; {
		j := strings.Index(line[i:], figure)
		if j < 0 {
			return false
		}
		end := i + j + len(figure)
		if end >= len(line) || line[end] < '0' || line[end] > '9' {
			return true
		}
		i = end
	}
}

func hasContext(low string, terms []string) bool {
	for _, c := range terms {
		if strings.Contains(low, c) {
			return true
		}
	}
	return false
}

// TestRetractedFiguresAreNotQuotedAsCurrent scans the research record for withdrawn
// numbers appearing without any nearby signal that they are withdrawn.
//
// # Why a test rather than an audit pass
//
// The findings audit is expensive — tens of minutes and several hundred thousand
// tokens — and most of its value is judgement: is this verdict word stronger than the
// evidence, has this mechanism argument gone stale, do two sections disagree. None of
// that is mechanisable.
//
// **This part is.** "A specific withdrawn number is being quoted as current" is exact,
// and it is the single commonest way this file has misled a reader: a body of evidence
// was measured with the transfer gate's minimum-gain threshold at 0.7, the value was
// retracted three commits later, nothing recorded the link, and a later audit cited the
// evidence as ground truth. Making the cheap half free is what keeps the expensive half
// affordable enough to run at all.
//
// # It checks the paragraph, not the line
//
// A retraction rarely sits on the same line as the number it withdraws — the number is
// usually in a table row and the retraction in the prose above it. So the window is the
// surrounding block, and the test is deliberately generous about what counts as a
// marker. A false pass is much cheaper here than a false failure: a guard that fires on
// legitimate prose gets deleted, and then it guards nothing. That is the same reasoning
// recorded against the snapshot staleness guard's permissiveness.
func TestRetractedFiguresAreNotQuotedAsCurrent(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	// `docs/` is in scope as well as AGENTS.md. The audit that motivated this found
	// five findings there, and `docs/model.md` is the file this project's own
	// instructions name as the thing to read before changing the scoring — so a
	// retracted figure is arguably worse there than in the research record.
	//
	// # What this guard no longer reaches, and why the line was deleted rather than
	// # repointed
	//
	// There used to be a second glob here, over the notes directory that held the
	// evidence behind each verdict. That directory is no longer in this repository,
	// and the evidence is not reachable from a checkout — so the line was removed.
	//
	// Read that as a real reduction in coverage, not as tidying. The comment it
	// replaces said the notes glob was "NOT optional", because `docs/*.md` does not
	// recurse and dropping it would have "quietly removed [most] of the record from
	// the one guard that reads it, while every test still passed" — the failure this
	// package exists to catch, arriving through the door it was built to watch. That
	// reasoning was correct and it still is. What changed is that the subject left,
	// not that the argument stopped applying: a glob kept pointing at a missing
	// directory would return zero matches and no error, which is the same silence in
	// a costume that looks like coverage.
	//
	// What remains in scope is the resident half — AGENTS.md's verdict lines, the
	// remaining docs, stats, and every Go source. That is most of this guard's
	// value, because a retracted figure does its damage where someone reads it without
	// choosing to, and the verdicts are the part that is read without choosing.
	//
	// If the evidence ever becomes reachable from a checkout again, restore the glob.
	// Do not point it outside the repository: a test whose verdict depends on state
	// that is not in the checkout can pass and fail at the same commit.
	// ⚠️ The work queue WAS in scope, and it was the harder case rather than an
	// afterthought. A superseded figure survives in a queue as a **task premise**
	// rather than as a claim, which changes what gets built next — worse than
	// misinforming a reader, because it sets the priority of everything downstream of
	// it. The worked case, from when the bodies were still resident: the availability
	// entry carried "595 held = 273 reconstructed + 322 for the judgement layer" as
	// live arithmetic long after all three numbers had been re-measured.
	//
	// The queue left this repository on 2026-08-15 and the append that named it is
	// gone with it. **Read that as a real reduction in coverage of the class this
	// guard was built for, and the second such reduction rather than the first** — the
	// evidence glob went the same way, one paragraph up, for the same reason.
	//
	// The append was REMOVED rather than left pointing at a deleted path, and that is
	// the whole of the care needed here: checkRetracted returns silently when a file
	// cannot be read, so a dangling entry would have passed forever while checking
	// nothing. That is the failure this package exists to catch, arriving through the
	// door it was built to watch — the same sentence the notes glob earned.
	//
	// Do not "restore" this by pointing at wherever the queue now lives. A test whose
	// verdict depends on state outside the checkout can pass and fail at the same
	// commit, which is the rule stated three paragraphs up and it has not changed.
	//
	// Go source is in scope, and it is the gap that let this class recur. Four sites
	// carried a retracted oracle figure as ground truth — a package doc, two
	// diagnostics, one of which PRINTS it, and the `now:` field of this very test —
	// and not one of them was scanned, because the guard read only prose. A comment
	// justifying shipped behaviour is a stronger claim than a line in the record, not
	// a weaker one: `swaps.go` explained why a correction exists using three figures
	// that had all been withdrawn.
	//
	// The same context rule does the work here. Bare literals are everywhere in Go —
	// 322 is a transfer count in `swaps.go` — so a figure still only counts when a
	// word naming that quantity shares its line.
	//
	// `README.md` and `.claude/` were added 2026-08-15, the guard's first WIDENING
	// after two narrowings.
	//
	// ⚠️ `.claude/` was REMOVED again 2026-08-20, the third such reduction rather
	// than the first two the paragraphs above describe. Its only tracked content —
	// `.claude/skills/merge-gate/SKILL.md` and `.claude/skills/review-gate/SKILL.md`
	// — was deleted when those two processes retired (see AGENTS.md), leaving the
	// tree with no tracked Markdown at all: `git ls-tree -r --name-only HEAD --
	// .claude` returns nothing. A surface with a floor of zero possible files is not
	// a guard, it is a permanent failure, so the entry goes with its content rather
	// than being kept red. Re-add it, keyed the same way, if `.claude/` ever tracks
	// Markdown again.
	//
	// Every surface is DISCOVERED, including the two single-file ones. A literal
	// {filepath.Join(root, "AGENTS.md")} has length 1 whether or not the file is
	// there, so the floor below could never fire on it — and the failure that hides
	// is not a quiet one. Asking git makes an absent file an empty surface and a
	// loud failure instead.
	surfaces := map[string][]string{
		"AGENTS.md":    trackedFiles(root, ".md", "AGENTS.md"),
		"README.md":    trackedFiles(root, ".md", "README.md"),
		"internal+cmd": goSources(root),
	}
	for _, tree := range []string{"docs", "stats"} {
		more, err := filepath.Glob(filepath.Join(root, tree, "*.md"))
		if err == nil {
			surfaces[tree+"/*.md"] = more
		}
	}
	// The floor is per-surface — see `requireEverySurface`. It replaces a composite
	// count of 20, added 2026-08-15 when this guard lost its SECOND scope entry: the
	// notes glob went when the evidence left, and the hand-appended queue went when
	// that left. A composite floor could not have caught either, because Go source
	// alone clears it three hundred times over, and `checkRetracted` is silent on an
	// unreadable path — so shrinkage here is invisible by construction.
	files := requireEverySurface(t, surfaces)
	for _, path := range files {
		checkRetracted(t, root, path)
	}
}

// goSources lists every Go file under the source trees, so a retracted figure cannot
// hide in a comment.
func goSources(root string) []string {
	return trackedFiles(root, ".go", "internal", "cmd")
}

// trackedFiles lists every tracked file under trees whose name ends in suffix, as an
// absolute path.
//
// One lister, two surfaces, for the reason `watched.go` gives for its shared helper:
// two copies of "enumerate the files under a tree" agree on the day they are written.
// Failures return nothing rather than being fatal — this is a guard, and one that
// fails for reasons unrelated to what it guards gets disabled. The per-surface floor
// in the callers is what turns "returned nothing" from a silent pass into a failure.
//
// `-z` for the same reason `watchedBlobs` uses it: git C-quotes non-ASCII paths
// depending on the reader's `core.quotePath`, so a newline-delimited listing is a
// guard whose scope varies with local configuration.
func trackedFiles(root, suffix string, trees ...string) []string {
	cmd := exec.Command("git", append([]string{"ls-files", "-z", "--"}, trees...)...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" || !strings.HasSuffix(rel, suffix) {
			continue
		}
		files = append(files, filepath.Join(root, rel))
	}
	return files
}

// requireEverySurface flattens surfaces and fails if any of them is empty.
//
// Per-surface rather than one composite count, and that is the whole point. Roughly
// 330 Go files dominate any composite floor, so losing the entire `.claude` surface —
// nine files — leaves a floor of 20 comfortably satisfied and reports PASS. This guard
// has already lost two scope entries (the evidence glob and the hand-appended queue),
// and both were noticed by a human audit rather than by the floor that existed to
// notice them.
func requireEverySurface(t *testing.T, surfaces map[string][]string) []string {
	t.Helper()
	var files []string
	for _, name := range slices.Sorted(maps.Keys(surfaces)) {
		got := surfaces[name]
		if len(got) == 0 {
			t.Fatalf("the %q surface matched no files, so this guard is no longer "+
				"reading it and would still report PASS. Either that tree moved or a "+
				"pattern stopped matching; a guard that quietly scans nothing is the "+
				"failure it exists to prevent.", name)
		}
		files = append(files, got...)
	}
	return files
}

func checkRetracted(t *testing.T, root, path string) {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if rel == "AGENTS.md" {
			t.Skipf("no research record to check: %v", err)
		}
		return
	}
	lines := strings.Split(string(body), "\n")

	// A block is the run of lines between blank lines. A retraction marker anywhere in
	// the same block covers every figure in it.
	blockOf := make([]int, len(lines))
	block := 0
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			block++
		}
		blockOf[i] = block
	}
	// Any of these anywhere in the block marks every figure in it as withdrawn. Broad on
	// purpose: a retraction is written in prose and this must not demand a fixed phrase.
	// `unsourced`, `totals-era` and `unresolved` are here because the transfer-policy
	// design document's §0 legitimately *tabulated* retracted figures in order to
	// classify them, using exactly those words. A guard that fired on the one document
	// doing this correctly would be deleted, and then it guards nothing.
	//
	// ⚠️ That document left the repository on 2026-08-14, so the justification is no
	// longer checkable from here. Keep the three markers anyway: the argument does not
	// expire with the file, and a marker list trimmed because nobody could find the
	// reason is how a guard loses the exemption that keeps it switched on.
	markers := []string{"retract", "retire", "superseded", "no longer", "was never real",
		"unsourced", "totals-era", "unresolved", "not established", "bound rather than",
		// Every phrasing of "it did not replicate" that this file actually uses. Missing
		// one makes the test fire on correctly-annotated prose, which is how a guard
		// earns deletion — "failed to reproduce" was absent on the first pass and it is
		// the wording of a section heading.
		"not reproduce", "failed to reproduce", "fails to reproduce", "did not replicate",
		"does not replicate", "not survive", "⚠️"}
	marked := map[int]bool{}
	for i, l := range lines {
		low := strings.ToLower(l)
		if hasContext(low, markers) {
			marked[blockOf[i]] = true
		}
	}

	for i, l := range lines {
		low := strings.ToLower(l)
		for _, r := range retractedFigures {
			// The figure alone is not enough — see the note on retractedFigures. It must
			// appear as a whole number AND alongside a word naming that quantity.
			if !quotesFigure(l, r.figure) || !hasContext(low, r.context) {
				continue
			}
			// The same literal naming a different, live quantity. See the note on
			// the `+16` entry for why this is necessary rather than a loophole.
			if hasContext(low, r.unless) {
				continue
			}
			// The same block, or either neighbour. A retraction is normally written
			// immediately before the claim (a warning heading) or immediately after it,
			// and demanding it share the paragraph would fire on correctly-annotated
			// prose. Anything further away is the case this test is for: a figure
			// hundreds of lines from its withdrawal, which is what a reader greps into.
			b := blockOf[i]
			if marked[b] || marked[b-1] || marked[b+1] {
				continue
			}
			t.Errorf("%s:%d quotes %s (%s) with nothing in the surrounding block "+
				"marking it as withdrawn.\n  line: %s\n  current position: %s\n\n"+
				"Either add the retraction note beside it, or remove the figure. A withdrawn "+
				"number quoted as current is how this file's evidence came to be cited for a "+
				"model that no longer existed.",
				rel, i+1, r.figure, r.what, strings.TrimSpace(l), r.now)
		}
	}
}

// TestSourceIsFormatted closes a gap in the stated build gate.
//
// The gate is documented as `go build ./... && go vet ./... && go test ./...`, and none
// of the three checks formatting — so five files once shipped unformatted, and it
// happened again this session. `gofmt -l` is in the recipe as a separate step, which
// means it depends on somebody remembering, which is exactly the class of discipline
// this codebase has watched rot four times over.
//
// Kept as a test rather than a lint config so it runs wherever the suite runs, with no
// extra tool and no CI to configure — this repository has no remote.
func TestSourceIsFormatted(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	out, err := output(root, "gofmt", "-l", "internal", "cmd")
	if err != nil {
		// gofmt absent is not a source defect, and failing here would make the suite
		// depend on the toolchain layout rather than on the code.
		t.Skipf("gofmt unavailable: %v", err)
	}
	var bad []string
	for _, f := range strings.Split(strings.TrimSpace(out), "\n") {
		if f = strings.TrimSpace(f); f != "" {
			bad = append(bad, f)
		}
	}
	if len(bad) > 0 {
		t.Errorf("%d file(s) are not gofmt-clean:\n  %s\n\nRun: gofmt -w %s\n\n"+
			"build, vet and test do not check formatting, which is why this is a test — "+
			"five files once shipped unformatted for exactly that reason.",
			len(bad), strings.Join(bad, "\n  "), strings.Join(bad, " "))
	}
}
