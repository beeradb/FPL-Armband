package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The research record used to live in two places on purpose: AGENTS.md carried a
// one-line verdict per finding, and a separate notes directory carried the evidence behind each
// one. The evidence half is no longer held in this repository. What remains
// resident is the verdict half, which is the part that stops an idea being
// rebuilt — and TestTheResidentIndexStaysSmall keeps it from regrowing.
//
// TestEveryNoteIsIndexed guarded the seam between the two, in both directions: a
// note nobody linked to was a finding that had silently left the record, and a
// link to a file that did not exist was the same failure wearing the opposite
// costume. With one side of the seam gone, only the second direction still has a
// subject, and that is what this test now is.
//
// # Why a dangling path is worth a test of its own
//
// It is not tidiness about broken links. A path into that former directory reads as a
// finding that was *moved* rather than one that is *unavailable from here*, so it
// sends the next reader looking for a file, finding nothing, and concluding the
// record was lost — when in fact the verdict they need is three lines above the
// pointer, in AGENTS.md, where it always was. The pointer actively misdirects.
//
// # One literal was not enough, and that is the finding
//
// The first version of this test matched the single repo-root spelling. It passed
// green over NINE live dangling pointers, on the day it was written, for two
// reasons it could not see. The reference docs cite the directory RELATIVELY, as a
// markdown link written from inside docs/, so the repo-root prefix never appears.
// And Go's idiomatic citation is a filepath.Join over separate quoted segments,
// which contains no slash at all — a sibling guard in this package still held a
// dead glob in exactly that form, and this test could not see it either.
//
// So the patterns are enumerated rather than inferred, and the five design
// documents are here too: they left in the same commit and nothing was watching
// them at all. If that list looks redundant, re-read the paragraph above — the
// redundant-looking spelling is the one that was actually in the tree.
//
// # Scope, and why it is not the whole tree
//
// Only the surfaces a reader treats as *live pointers* are checked: AGENTS.md, the
// remaining docs, the Go sources, and the R scripts. reviews/ and stats/snapshots/
// are deliberately excluded, and the reason is narrower than "they are old": each
// is a dated ATTESTATION about a named commit. A review record saying it checked a
// given file is a claim about what was true then, and rewriting the path inside it
// would make it attest to a location that did not exist. The pointer moves; the
// record of the pointer does not.
//
// ⚠️ The work queue used to be excluded here for a weaker reason, recorded as a
// known gap: a queue is read forward, so a stale pointer in one is a task PREMISE
// rather than a dated claim — which retracted_test.go argues, about this very file,
// is worse than misinforming a reader. Its open items should have been in scope and
// never were, because its completed entries are genuinely historical and nothing
// here can separate the two by parsing.
//
// The queue left this repository on 2026-08-15, so the gap is now moot HERE and
// permanent THERE: no guard in this checkout reaches it. That is a cost of the move
// and not a repair of the gap — the argument above is still correct, and there is
// simply nothing left in scope for it to apply to.
func TestNoLivePointerCitesTheRecordByPath(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	// The same surface as `TestRetractedFiguresAreNotQuotedAsCurrent`, plus this
	// guard's own `stats/*.R`. The two ask different questions of one population, and
	// scoping them separately is how the sibling came to read `.claude/` while this
	// one did not — `reviewgate_test.go` already records what fixing two guards
	// separately costs. `README.md` and `.claude/` were added 2026-08-15 and are green
	// on arrival; the case for them is the next dangling pointer, not a live one.
	// Discovered rather than asserted, for the reason the sibling's copy of this
	// block gives at length: a one-element slice literal has length 1 whether or not
	// the file exists, so the floor cannot fire on it.
	surfaces := map[string][]string{
		"AGENTS.md":    trackedFiles(root, ".md", "AGENTS.md"),
		"README.md":    trackedFiles(root, ".md", "README.md"),
		".claude/*.md": agentAndSkillDocs(root),
		"internal+cmd": goSources(root),
	}
	for _, pat := range []string{
		filepath.Join(root, "docs", "*.md"),
		filepath.Join(root, "stats", "*.md"),
		filepath.Join(root, "stats", "*.R"),
	} {
		more, err := filepath.Glob(pat)
		if err != nil {
			t.Fatalf("glob %s: %v", pat, err)
		}
		surfaces[filepath.Base(filepath.Dir(pat))+"/"+filepath.Base(pat)] = more
	}

	// A floor, so a reorganisation cannot silently reduce this to one file and still
	// report PASS. There is no legitimate absence to declare here — unlike the
	// snapshot series, this material is either cited or it is not — so a count is
	// cheaper than an environment switch and fails in the same direction. It is
	// per-surface rather than composite for the reason `requireEverySurface` gives.
	files := requireEverySurface(t, surfaces)

	// Every literal is split so this test does not match itself — goSources walks
	// _test.go files, so it scans its own source.
	stale := []string{
		"docs/" + "notes",      // repo-root form: AGENTS.md, Go comments, R scripts
		"](" + "notes/",        // relative form, from inside docs/
		`"docs", ` + `"notes"`, // Go filepath.Join form
		"oracle-" + "design.md",
		"transfer-policy-" + "design.md",
		"expected-points-" + "review.md",
		"rank-objective-" + "handoff.md",
		"claude-md-" + "reorganisation.md",
	}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		text := string(b)
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		for _, s := range stale {
			if !strings.Contains(text, s) {
				continue
			}
			t.Errorf("%s:%d cites %q, which is not in this repository.\n\n"+
				"That path reads as a finding that was moved rather than one that is "+
				"simply not available from here, so it sends a reader hunting for a "+
				"file instead of reading the verdict — which is still resident in "+
				"AGENTS.md, under \"What has been measured\".\n\n"+
				"Name it instead of pathing to it: \"the scoring-model note\", \"the "+
				"oracle-design document\". Do not replace it with a location outside "+
				"this repository.", rel, lineOf(text, s), s)
		}
	}
}

// lineOf returns the 1-indexed line where sub first appears, or 0. A guard that
// names a file but not a line is a grep in a 4,600-line file.
func lineOf(text, sub string) int {
	i := strings.Index(text, sub)
	if i < 0 {
		return 0
	}
	return 1 + strings.Count(text[:i], "\n")
}

// TestTheResidentIndexStaysSmall is the AGENTS.md size budget, and it lives in its
// own function for a reason that took a near-miss to notice.
//
// It used to be the tail of TestEveryNoteIsIndexed, which meant it sat behind that
// test's two early exits: t.Skip when the notes directory did not exist, and t.Skip when it
// is empty. Those skips are about the *notes*. The budget is about AGENTS.md and has
// nothing to do with where the evidence lives — so any change that removed the notes
// directory would have silently switched off the only guard against resident-file
// regrowth, while the suite stayed green.
//
// That is the same shape as the failure the retraction guard's own comment predicts:
// a guard that stops guarding through a door it was built to watch. Two unrelated
// assertions sharing a precondition is the mechanism, so the fix is to stop sharing
// it rather than to make the skips cleverer.
func TestTheResidentIndexStaysSmall(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	// The evidence used to be the bulk and has left the repository; the index is what
	// must stay small. That is not tidiness — AGENTS.md is loaded into every request,
	// so its size is a per-session tax on every task in this repository, whether or
	// not the task touches the model.
	fi, err := os.Stat(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("stat AGENTS.md: %v", err)
	}
	// 52 KB, cut from 160 on 2026-08-12. The file had reached 157 KB — roughly 40k
	// tokens before anyone had said anything — because the index stopped being an
	// index: 20 bullets of the scoring-model section alone ran to 29 KB, duplicating
	// a note that was already longer and better. A budget set just above the current
	// size does not bind, which is why the old one never fired; this one is set just
	// above the reorganised size so the next regrowth fails here rather than being
	// noticed a year later.
	//
	// It was 48 for about an hour, which is the part worth recording. The first pass
	// hit 48 by dropping qualifiers, and the findings audit caught six bullets that
	// had become *stronger* than their sources: a vice-captain t that is the retired
	// estimator's output reading as current, a "suggestive, not established" team-news
	// arm reading as established, a decoded BPS coefficient reading as though it
	// settled a contradiction it does not, a bench-slot tie measured under a
	// superseded blank rate, a surviving ~1% xGC overshoot, and a point estimate
	// reading as a bound. Restoring those cost 1.7 KB. A second review, after merging
	// main, found four more of the same shape in one bullet.
	//
	// So the lesson this constant encodes is that compression and overstatement are
	// the same operation past a certain point, and the qualifier is the first thing to
	// go — it costs bytes, carries no figure, and looks like padding to whoever is
	// cutting.
	//
	// # Why the number is not pinned to the current size
	//
	// It was, twice, and both times the next honest edit broke the build and needed a
	// budget commit of its own. That is not a guard, it is a ratchet, and it puts
	// pressure in exactly the wrong direction: when adding a rule costs a commit and
	// deleting a hedge costs nothing, hedges go. So the headroom here is deliberate.
	//
	// What this guard is actually for is CATEGORY regrowth — evidence moving back into
	// the resident half. The file went from 96 KB at the split to 161 KB because the
	// index started carrying its notes' evidence instead of their verdicts (the
	// scoring-model index alone reached 29 KB for 20 bullets), because failed
	// experiments accumulated with their full sweep tables, and because retraction
	// chains were written here rather than in the note.
	//
	// Do NOT use edit size as the test. That 65 KB arrived over about fifty commits, a
	// bullet at a time, at the same byte sizes as an honest edit — an earlier version
	// of this comment proposed "2-5 KB a paste versus tens of bytes" as the
	// discriminator, and a reader applying it would have waved through every commit
	// that produced the 65 KB. What separates the two is WHERE THE TEXT LANDS, not how
	// much arrives at once.
	//
	// Resident: conventions, the glossary, what the harness can resolve, the
	// contamination events, what each season can run, standing rules, shipped bugs,
	// closed lines, one verdict per note. Never resident, at any size: sweep tables,
	// derivations, worked examples, the history of a retraction.
	//
	// That last one is the entry people breach without noticing, because it looks
	// like diligence rather than growth. A re-measurement lands, and the honest
	// instinct is to write "X was Y, now withdrawn, because Z" — which is a
	// correction marked in place, the right thing to do where the evidence lives
	// and the wrong thing here. Breached on 2026-08-15 by a settlement that
	// annotated a bar as withdrawn instead of deleting it, and the file needed a
	// budget raise to fit the narration. AGENTS.md's Conventions section now states
	// the rule positively — a correction REPLACES the claim — so a writer meets it
	// before reaching this comment, which only fires once the bytes are already
	// spent. One caveat travels with it: deleting a figure still owes its referent,
	// or the cut leaves the withdrawn reading as the only inference available.
	//
	// Headroom is deliberate but it is not large — the last review-driven restoration
	// of hedges cost 1.7 KB, so this buys about one more. If a second one binds, raise
	// it again and say which claim needed its hedge back. Do not re-compress the
	// qualifiers; that is the failure this whole comment exists to record.
	//
	// **Raised to 58 KB on 2026-08-13, and this is that second binding.** The chip
	// work compressed four qualifiers to stay under 56 and a review caught every one:
	// "+20.8" lost its unit beside two neighbouring per-season figures; "a clean null"
	// lost "as a measurement", which is the distinction that section's finding *is*;
	// the triple-captain bullet lost "a decision null rather than a wiring null,
	// witnessed three ways", which is the clause that stops the next reader concluding
	// the channel was never wired; and two new results — the preparation 2x2 and the
	// wildcard-truncation bound — had no resident entry at all, so they would have been
	// rebuilt. All four are restored above rather than re-compressed, which is what
	// this comment says to do.
	// **Raised again to 60 KB on 2026-08-13, same session, and this is why.** Two
	// *standing rules* were added — the ones about crossing two levers in a 2x2 and
	// about a one-at-a-time null being a simple-effect null. Standing rules are named
	// in the resident list above; they are the class this file exists to carry, and
	// the second is a caveat on how every other null here should be read. Trimming
	// them to fit would be re-compressing a qualifier, which is what this comment
	// forbids.
	//
	// **Raised to 64 KB on merging the data-repair branch, and that branch is this
	// comment's own case study.** Working under the then-current 54 KB, it compressed a
	// qualifier to fit on FOUR separate occasions — the `MinutesWeight` non-inversion
	// argument, the fourth-cluster figures, the vice drift chain, and the `FIXW`
	// margins that a review had *just* restored after an earlier compression removed
	// them. Each was a defensible individual edit and the aggregate is exactly the
	// ratchet described above. The merge brings, from that side: the fourth cluster
	// (which retires "its price can never be measured here"), the `POLICY` grid
	// correction (10 of 11 arms, which retires the four-season exception for transfer
	// settings), the `MinutesWeight` data-state qualifier, the widened
	// grep-before-asserting rule, and the recorded-starts row in the season table. None
	// is a sweep table; all are verdicts or the qualifiers that stop them being
	// misread. The figures behind them are in stats/snapshots/2026-08-13-aa95f75/.
	//
	// Both halves of the merge were audited for duplication first — 128 bullets, zero
	// repeated openings — because two branches editing one index is the obvious way to
	// pay twice for one claim.
	//
	// **Raised to 68 KB on 2026-08-14, and the first remedy tried was the forbidden
	// one.** The budget was ALREADY breached at 65,749 B before that branch began, so
	// the schedule-screen work found this test red and treated it as its own mess to
	// clear. It moved four blocks out properly — the "no truth value" digression, the
	// composed-ladder arithmetic, the teamsheet trap and the `prior_half_life`
	// narrative, all four verified against their destinations and lossless — and then,
	// still 300 B over, re-compressed **the vice drift chain and the fourth-cluster
	// figures**: two of the four instances THIS COMMENT ALREADY NAMES from the
	// data-repair branch. The ratchet ran a second time, in a file that describes it.
	//
	// The findings audit caught it. What the vice compression cost, to no file at all:
	// **+0.4302** (today's four-season shipped value), the correction that **`c76c0d8`
	// alone is 129% of the net** drift, and "the two backfill terms are single-season
	// bookkeeping rather than causes". Worse, it pointed the reader at
	// scoring-model.md, whose ledger still said "about four fifths of the drift" —
	// the very phrasing the deleted clause existed to retract. A compression that
	// removes a correction and cites the uncorrected text is the failure mode at full
	// strength.
	//
	// So: restored above, budget raised, and the claims that needed their hedges back
	// are named in the paragraph you just read. **The lesson is not "compress more
	// carefully".** It is that "check the destination carries it" must be run as a
	// grep for the *figure*, not for the block — the branch did grep, matched a
	// neighbouring commit hash, and read that as confirmation.
	// # 72 KB from 2026-08-14, and the reason is a change in what this budget means
	//
	// The evidence half left the repository. That did NOT make this file smaller in
	// the way it looks like it should have: the notes were never resident, so the
	// move only deleted 34 link targets — about 675 bytes — and the review of that
	// same commit then spent all of it and more, restoring qualifiers to the new
	// prose. Which is the pattern this comment already documents twice: a review
	// costs kilobytes, and compression and overstatement are the same operation past
	// a certain point.
	//
	// What genuinely changed is that THERE IS NOWHERE TO MOVE EVIDENCE TO. The old
	// failure message said "move it into the notes and leave a verdict behind", and
	// that instruction is now unfollowable — the destination is not in this
	// repository and must not be named here. So this stopped being a budget with an
	// overflow valve and became a hard ceiling on the only surviving copy of the
	// record's resident half.
	//
	// A ceiling with no valve, held at the previous number, does exactly what the
	// section below warns about: it makes deleting a hedge the cheapest available
	// edit. Hence the raise, and hence the message now says raise-do-not-compress
	// rather than move-it-out.
	//
	// # 76 KB from 2026-08-15, and the claim that needed the room
	//
	// **"All eight congestion penalties ship at 1.00, so these four lists are
	// display-only" was false, and it was false about the one list that is live in
	// the two gameweeks in front of us.** `DefaultRestPlayers` reaches minutes
	// through `blendFor` → `restFactor` → `rest_minutes_factor` (`blend.go:165`),
	// which is a `Weights` field and not one of the eight `Congestion` penalties, so
	// a wrong name on the post-tournament teamsheet mis-scores a player at GW1 and
	// GW2. The same sentence stood in four places including the doc comment of the
	// test the other three cite.
	//
	// The correction cost ~1.5 KB because it is not a one-word fix. Three separate
	// things had to be written down or the next reader repeats the inference:
	// two unrelated mechanisms answer to "rest"; the obvious check refutes itself,
	// since `restFactor`'s other call site is labelled "Reporting only"; and the
	// count was right for the wrong reason, because `DefaultNewCoachClubs` is
	// display-only through a NINTH penalty outside the block.
	//
	// ⚠️ **This is the raise-do-not-compress instruction being followed for the
	// first time, and it is worth saying why the alternative was worse here.** The
	// cheapest edit that fits the old budget is to state the correction without the
	// three reasons — which leaves a reader who greps `restFactor` looking straight
	// at "Reporting only" and concluding the correction is wrong. The hedges ARE the
	// finding in this case.
	//
	// # 78 KB, later the same day, and the two claims that needed it
	//
	// Both are **standing rules that were not resident**, which is the case this
	// budget most has to accommodate: a rule only works if the next author meets
	// it, and the alternative to residency is a copy per place it might be needed.
	//
	//   - **"Review the plan, not just the output."** On 2026-08-15 two of five
	//     commissioned measurements had briefs whose inference was invalid and whose
	//     code was correct. Neither is visible in a diff, so nothing cheaper than a
	//     resident rule prevents the third.
	//   - **"A snapshot's figures are not guaranteed to have come from its own
	//     commit."** `stats/snapshots/2026-08-15-9e743cf` is on `main` carrying
	//     figures its own commit does not produce. The failure is silent and the
	//     artefact looks authoritative, which is the combination this file exists to
	//     stop.
	//
	// ⚠️ Both are *rules*, not findings, and that is why they are here rather than
	// left in the evidence store. The test's instruction is raise-do-not-compress;
	// this is the second consecutive raise, so the thing to watch is whether the
	// next one is also a rule. **Three raises for evidence would mean the boundary
	// has moved and the budget is not doing its job.**
	//
	// # 80 KB, and ⚠️ THIS ONE IS NOT A RULE — the watch above has fired
	//
	// The paragraph above asked whether the third consecutive raise would also be a
	// rule. It is not. It is a **supersession notice**: the xPoints instrument now
	// prices xG and xA through a per-position conversion scale
	// (`internal/analysis/xpoints.go`), and four banked figures move with it — the
	// gate arm's underlying level, the 0.645 recovered fraction,
	// AxisTransferGateResidual, and xppilot's SE figures.
	//
	// ⚠️ **The first version of this raise was for 1.9 KB and most of it did not
	// deserve residency.** A findings audit caught that two of its three bullets
	// restated what `xpoints.go`, `season.go` and `stats/xpoints_common.py` now carry
	// at length — a mirror, which is this record's other signature failure, paid for
	// out of the budget rather than caught by it. Those were cut. What is left is
	// ~890 bytes: the supersession list, and the qualifiers on it.
	//
	// **Compressing further would mean deleting qualifiers, which is what this
	// constant exists to prevent** — the in-sample scope (DEF/MID/FWD but not GKP),
	// the data state on cross-season levels, and "paired differences stay one metric
	// but are not numerically unchanged" are each the difference between a figure
	// being re-quoted and not. So: cut the mirror, keep the hedges, raise.
	//
	// ⚠️ **The boundary really has moved, and this is the evidence rather than a
	// defence.** Supersession notices for an evidence store the repository cannot
	// reach are a structural cost, not a one-off. Whoever raises this next should
	// decide whether they are a CLASS that belongs here — and if they are, this
	// budget keeps climbing and something else has to give.
	//
	// # 84 KB, and the question above is ANSWERED: a supersession notice is a
	// TEMPORARY resident, and this is the first one to prove it by PARTLY discharging
	//
	// ⚠️ **84 and not 88.** A first version of this raise went to 88 KB, leaving
	// 6.9 KB of slack where every prior step (72→76→78→80) left about 1 KB. Two
	// reviewers caught it independently, and the objection is not tidiness: a budget
	// with 6.9 KB of headroom does not bind, so the watch written at the bottom of
	// this comment could not fire until 6.9 KB of unremarked growth had already
	// landed. That is the `min_gain` pattern — a threshold set where it cannot act —
	// recorded in this repository twice. 84 leaves ~1.4 KB, in line with the 1.2 and
	// 1.9 of the last two raises.
	//
	// ⚠️ **It went to 82 in between, and 82 was wrong for an instructive reason.**
	// The reviewers sized the raise against the file as it then stood — before their
	// own strongest finding was applied. Acting on it grew the file again: the
	// verdict had to be downgraded, the 0.89 rejection promoted, and the contrast's
	// pre-registered null explained. **A budget sized against a pre-review draft is
	// sized against the wrong file**, which is worth one line here because the
	// obvious fix — size it last — is the one that keeps being skipped.
	//
	// The 80 KB raise was for the xPoints conversion scale's supersession list. That
	// list is now **discharged for the gate arms** — the re-run landed the same day
	// (`xpoints-scaled-gate-rerun`), and the entry carries replacement figures rather
	// than a promise of them. So the class is real but it is **not monotone**: a
	// supersession notice is rent paid until the re-run, and the discipline is that
	// **whoever lands the re-run collapses the notice into its result**, rather than
	// letting both halves sit here forever.
	//
	// ⚠️ **"Discharged" is NOT "discharged in full", and the difference was caught in
	// review rather than noticed here.** The same list supersedes xppilot's
	// `hold_xpoints`/`policy_xpoints` SE figures, which belong to a different sweep,
	// were not re-measured by this run and cannot be. They are still superseded and
	// still un-re-run. A raise argued on a discharge must name what did not
	// discharge, or the next reader inherits a clean bill the evidence does not
	// support.
	//
	// ⚠️ **This raise does NOT collapse it, deliberately, and that is the honest
	// reading of the cost.** Both halves are still here: the superseded figures and
	// their replacements. Marking a correction in place rather than overwriting it is
	// a standing rule of this record, and the trail it protects is real. What that
	// buys is a permanent doubling of every re-measured figure, which is the "something
	// else has to give" the paragraph above predicted — arriving one raise later than it
	// expected.
	//
	// The other ~1.3 KB is not supersession: two verdicts (the residual gate's
	// contrast, the 50% bar failing twice) and two standing rules (a confinement check
	// without a liveness check proves nothing; an oracle on the sign of the scored
	// quantity is a positive control on BOTH metrics). Rules and verdicts are what this
	// file is for, so those are not the growth to worry about.
	//
	// **The thing to watch is now specific**: if the NEXT raise is again a supersession
	// notice whose predecessor never discharged, the non-monotone reading above is
	// wrong and the notices are accumulating. Check whether the previous one collapsed
	// before believing this paragraph.
	// ⚠️ **Raised to 86 KB on 2026-08-15 for the same-club/talisman closed line.**
	// The room went to a refutation with **nothing measured**: the objective sums
	// deterministic Score predictions, so a covariance argument has nothing to enter,
	// and no grid width reaches that. It is the first entry in "Closed lines" closed
	// on *arithmetic* rather than on points, which is why it earns residency at all —
	// an unmeasurable claim cannot be retired by running more cells, so the argument
	// IS the artefact.
	//
	// ⚠️ **The entry roughly DOUBLED across two review rounds, and that is the case
	// for the raise rather than an argument against it.** Every repair was a
	// QUALIFIER, and two were outright retractions of things the first draft asserted:
	// `MaxPerClub` binds rather than making the knob inert (the draft had this
	// backwards AND offered it as support); the pool's covariance SIGN is not obvious
	// and the refutation does not need it; the sweep table peaking at 1.0 is
	// `BonusWeight` under the retired FLAT regime, not "some other knob" (the first
	// correction installed a fresh misreading while fixing one); and the
	// `LeagueShrinkK` precedent is a different population, on a retired estimator whose
	// two t's cannot be told apart, unresolved-and-negative rather than a measured
	// loss. Cutting any of those returns the entry to something a reader can dismiss,
	// which is the failure mode above arriving from the other side.
	//
	// ⚠️ **Headroom is now ~1.3 KB, not the ~2.4 KB this raise was sized for** — the
	// second review round spent it. The next entry of this kind needs its own raise,
	// and by then "three raises for evidence" (above) is worth revisiting as a class.
	//
	// # 90 KB — TWO independent raises merged, and the sum is the whole story
	//
	// The two blocks above were written the same day by two sessions that could not
	// see each other: 84 KB for the gate re-run's supersession replacement and two
	// standing rules, 86 KB for the same-club/talisman closed line. **Both are kept
	// as written** — they justify different content and neither subsumes the other,
	// and picking one would delete a rationale that is still load-bearing for text
	// still in the file.
	//
	// What could not be kept is either NUMBER. The merged file is **91,279 bytes**,
	// which neither 86 KB (88,064) nor 88 KB (90,112) admits, so the constant is
	// re-derived against the merged file rather than taken from the higher side:
	// **90 KB leaves 881 bytes**, in line with the 851 / 1,244 / 1,307 / 1,470 of
	// every prior step.
	//
	// ⚠️ **This is the merge case the record already warns about, arriving on a
	// constant instead of on a review key**: two edits, each correct alone, composing
	// into a tree where neither describes the result. A conflict marker made it
	// visible here. Nothing would have made it visible if the two sessions had raised
	// the budget by the same amount, and the file would then have been over its own
	// limit with a green test — which is the shape worth watching for next time.
	// # 92 KB — a MECHANISM fact, which is the class most worth the rent
	//
	// The 90 KB above was set at the merge and spent within the hour, by a plan
	// review that refuted a proposed measurement outright. What it bought is three
	// entries under "Closed lines": that the gate oracles are a **veto on one
	// candidate** rather than a selector and bypass the value bar entirely; that the
	// residual arm's negative is not the hit charge, from banked cells at no run
	// cost; and that its level is data-state-dependent with the "informative sign"
	// reading resting on an untested premise.
	//
	// ⚠️ **The first of those is the one that earns residency, and it is a different
	// class from a figure.** It says an entire family of statistics — anything
	// computed over a *candidate population* — does not describe this operator. A
	// figure misleads a reader; that misleads a design, and it had already produced
	// one invalid plan in this session before the review caught it. Cheaper to keep
	// resident than to re-derive from `simulate.go:2200` and `:1591` each time.
	//
	// ⚠️ **Quote a SIZE and the commit it was measured at, never a difference.** This
	// paragraph used to read "~220 bytes, down from ~730 on 2026-08-15: closing the
	// guard item took ~310", and 730 − 310 ≠ 220. Each end was right when written and
	// the sentence was wrong anyway: ~730 was the headroom at `891378b` (93,479
	// bytes), `6021401` then took the file to 93,676 without touching this comment,
	// and ~220/~310 are measured from *that* unrecorded intermediate. A difference is
	// false the moment either end moves, and nothing here notices; a size with a
	// commit beside it can be checked with `git show <commit>:AGENTS.md | wc -c`.
	//
	// # 97 KB — the scoring-chip timing correction
	//
	// The 92 KB above was at 93,985 bytes on `d0c7dc2`, 223 free. What needed the room
	// is the chip bullet under "Chips": that its `+0.000` is a **declared invariance**
	// — `mustNotMoveForAxis(AxisChipWeek)` returns the eight `cellMetricColumns` and
	// the axis plays no chip, so byte-identical output is what it is *required* to
	// produce — and therefore that its cell count is coverage of that check and not a
	// reading of timing. The old sentence read the invariance as a null result about
	// timing and put a cell count where a **data state** belongs, which is the failure
	// the record's own rule names. Beside it: the levels are unbanked sums over the two
	// scoring chips (`reportChipCells` prints per chip and no combined figure), are
	// functions of two asserted bars, and have their own data state — `d249d8a`'s
	// six-season tree — distinct from the one banked run, 24 cells at a dirty
	// pre-repair commit.
	//
	// ⚠️ **Half this entry is the AUDIT's, and that is the case for the raise.** A
	// first draft of the correction was itself wrong in five places, each in the
	// direction of overclaiming: "every collected column" (it is eight of them, and the
	// chip-reading columns are *required* to differ); "unsupported by anything in this
	// repository" (`d249d8a`'s message sources the 36 — unbanked, not unsourced);
	// "oracle minus the bar" (it is minus the first week *clearing* the bar); "pooled"
	// glossed as over chips when the source means over entry points; and a flat
	// "nothing has been re-measured" that erased the levels' own data state. Cutting
	// any of those restores a claim a reader can act on and shouldn't.
	//
	// ⚠️ **None of this gives the levels a standard error.** One WAS recorded — SE 1.25,
	// t 6.6 — and the entry now says why it is not quotable as significance: both
	// differences are >= 0 in every cell by construction, so the t is mechanical. That
	// is the opposite of an upgrade, and the next edit must not turn it into one.
	//
	// Two smaller repairs rode along: the "both Go guards record their near-misses"
	// sentence, wrong about which blind spot is populated and about what the shared-cell
	// guard scans; and the two adjacent bullets that both quote 13.3.
	// # 100 KB from 2026-08-15, and the claim that needed the room
	//
	// **The clean sheet's recorded ~30% over-prediction is a property of the
	// CALIBRATION'S regressor, not of the model** — `cs_calibration.R` fits realised
	// single-match xGC while `cleanSheetProb` scores `XGC90`, and on the latter
	// predicted/actual is 1.052 native and 1.004 pooled against a recorded 1.281.
	//
	// That alone would be a figure swap and would need no room. What needed the room
	// is everything that must travel WITH it, because each clause is the difference
	// between the finding and an overstatement of it — and a plan review and a
	// results review each caught a version of this file's author making exactly that
	// overstatement:
	//
	//   - it refutes a MAGNITUDE and establishes no calibration; the slope separates
	//     neither b = 1 nor b = 1.1731, MDE 0.424 against a candidate of 0.173;
	//   - the native ratio interval [0.90, 1.20] still admits a fifth of the bias;
	//   - the population flatters it, and it is now SIZED — the most-played
	//     defender or keeper, on the 85.8% of matches he finished; removing that
	//     selection takes the pooled ratio 1.0051 → 1.0305 and the omitted defcon
	//     coupling takes it to 1.0112, ~3.7% composed. ⚠️ This clause read "78%"
	//     and "UNSIZED", the complement of a 22% that was an audit estimate
	//     repeated without counting — and a grep for "22%" cannot find its
	//     complement, which is why it outlived four other corrections;
	//   - the realised-xGC fit is NOT withdrawn, being a fit of a different
	//     regressor, so both must be quoted with their regressor named;
	//   - the 2x2 ran and resolved nothing, 96% of its largest arm is one season, and
	//     that season's own slope points the other way;
	//   - the CANARY is the transferable lesson — a gross miscalibration costs only
	//     −21.6 against its own threshold of 28, so the family was ~4x below
	//     detection before the run started. **Size a candidate against a canary
	//     before spending 180 cells** is the sentence most likely to save a future
	//     sweep, and it is not derivable from any figure above it.
	//
	// The standing rule "a bias shared by every player in a position is not an
	// ordering error" also took a qualification in the same commit, because its
	// canonical example is the one that just lost its magnitude. Deleting any of the
	// above to fit would leave the record asserting a stronger result than was
	// measured, which is the precise failure this constant exists to prevent.
	//
	// # 106 KB — the merge, and why the raise is the merge's own and not either side's
	//
	// Two branches raised this constant concurrently and neither figure survives the
	// merge: the 97 KB above was measured against a file without the clean-sheet entry
	// and the 100 KB against one without the chip entry, so **both were correct and both
	// were obsolete before either landed.** The merged file is 105,655 bytes, over the
	// larger of the two, and no qualifier was cut to fit — which is the whole point of
	// the rule, and the first time it has had to survive a concurrent raise.
	//
	// ⚠️ **The predecessor sentence "Headroom is ~730 bytes. The two blocks above still
	// stand." is GONE, deliberately.** Its first half is the difference-quoting form the
	// paragraph above retracts by name, and it was already stale when written. Its second
	// half is preserved by this line: the blocks above it do still stand, all of them.
	//
	// ⚠️ **Both entries are kept in full rather than merged into one.** They name
	// different claims — a regressor swap and an invariance misread as a null — and the
	// rule's requirement is that the comment name *the claim that needed the room*, which
	// a summary of two claims does not do. Neither is a summary of the other.
	//
	// # 108 KB from 2026-08-17 — floating point is not portable across machines
	//
	// The claim that needed the room: **a banked absolute total is reproducible from a
	// commit AND a machine, and only the commit is recorded.** Go's `math.Exp` has
	// per-architecture assembly and on amd64 branches at run time on AVX+FMA, so two
	// amd64 CPUs can disagree from one binary; `math.Pow(0.5, 0.25)` is one ulp high on
	// arm64, which makes a recency-weighted 90 minutes read exactly 90 here and
	// 90.00000000000001 on CI. Two unit tests compared float64 exactly, so CI was red on
	// eight consecutive commits while green on every arm64 machine the work was done on.
	//
	// It cost its bytes twice over, because the first draft was wrong in three ways a
	// reviewer caught and the corrections are longer than the errors: it named priors and
	// team strength as exposed when `prior_half_life` 0 gives `BlendPriors` an integer
	// exponent that `Pow` takes exactly and both team-strength sites ship off; it omitted
	// the Poisson blocks and `defconCleanFactor`, which are exposed; and it asserted that
	// swapping the transcendental "moves every banked cell" while the next clause said the
	// points columns reproduce unless a decision flips. **The qualifiers ARE the entry
	// here** — an unhedged version would have recorded a mechanism argument as a
	// measurement, which is the failure this constant exists to prevent.
	//
	// The `teamBands` correction in the same commit is net-neutral and did not need room.
	//
	// # 110 KB from 2026-08-17 — the banking null became a checked zero
	//
	// The claim that needed the room: **the recorded "the policy never banks a transfer"
	// was an UNCHECKED zero, and is now a checked one that is not degenerate.** The one
	// banked `BankLookahead` arm in the corpus — `stats/snapshots/2026-08-13-reach/`, 4
	// seasons × 2 entry points — replays byte-exactly on all eight `policy_points` and
	// reads 236 consulted weeks, **169 weighed**, 0 banked. The 169 is the whole entry: it
	// separates "the rule weighed a real choice and preferred to act" from "nothing ever
	// cleared the gain floor", which license opposite conclusions and were previously one
	// number.
	//
	// The qualifiers are again most of the cost, and again they are the point. It is a
	// **confinement rather than a null** — the branch never ran, so byte-identical points
	// are a construction and a threshold would be a category error. `WeeklyXI = true` is
	// not shipped config. `bank_up_to` is pinned at 5 rather than the rule each season
	// played under, which runs *in favour* of banking and makes the zero conservative. And
	// the original `BANK` sweep's cells were never banked, so that arm stays unverifiable:
	// this is circumstantial evidence from a different sweep, not the same one.
	//
	// A prior draft of the same bullet asserted that the arm the null was measured on "no
	// longer exists", because a double free-transfer grant made it climb to the ceiling at
	// double speed. That was a **hypothesis contradicted by the only measurement of it** —
	// the arm banks 0 times, so the buggy branch never executed and nothing climbed. The
	// accrual fix ships on correctness alone. Recording the replacement cost bytes; the
	// erroneous version would have cost a retraction.
	//
	// **Raised to 112 KB on 2026-08-17 for the fixture-load anchor fix, which is a
	// contamination event and therefore the class this section exists to carry.**
	// `fixtureLoadFor` anchored on a club's next FIXTURE rather than the next
	// GAMEWEEK, so at horizon 1 the load was >= 1 by construction and a blank could
	// not be expressed at all — 170 missed over the six-season grid against zero
	// missed doubles. Two things had to be resident and neither compresses. The
	// first is that every banked `POLICY` total straddling the fix is contaminated
	// UNEVENLY: 12 of 12 cells move, by up to 77 points, in both directions, which
	// is the "invents shapes" pattern rather than added noise. The second is the
	// qualifier on the other half — `HOLD` is byte-identical by CONFINEMENT, a code
	// fact about `fixtureLoadWeeklyOnly` and `HoldCaptaincyWeekly`, not an empirical
	// null — and without it the same sentence reads as "the fix does nothing to
	// scoring", which is the opposite of true and would retire every banked `HOLD`
	// cell for no reason. The points measurement itself is deliberately NOT here:
	// it does not resolve on the estimator its own variance components call for.
	const budget = 112 * 1024
	// The figure is emitted by the thing that owns it. It was quotable only from a
	// failure before this line, which is how the paragraphs above came to reason from
	// differences between sizes nobody had recorded.
	t.Logf("AGENTS.md is %d bytes against a %d KB budget (%d free)",
		fi.Size(), budget/1024, budget-fi.Size())
	if fi.Size() > budget {
		t.Errorf("AGENTS.md is %d bytes, over the %d KB budget. It is loaded into "+
			"every request of every session in this repository, so growth here is "+
			"paid for by every task.\n\n"+
			"There is no longer an in-repo destination to move evidence to, and this "+
			"is the only place the record's resident half can grow. So the remedy is "+
			"NOT to compress: RAISE THE BUDGET, in this commit, and name in this "+
			"comment the claim that needed the room. Deleting a qualifier to fit is "+
			"the failure this constant exists to prevent — it has happened four "+
			"times, and each time the qualifier was the part carrying the "+
			"uncertainty.\n\n"+
			"Do not record where the evidence went.",
			fi.Size(), budget/1024)
	}
}
