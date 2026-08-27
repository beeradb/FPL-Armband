// Package snapshot builds the model-and-harness accuracy snapshot: a dated,
// stamped record of what the scoring model gets right about football and what
// size of effect the replay harness can see at all.
//
// # Why this exists at all
//
// This project's expensive failures are provenance failures rather than
// arithmetic ones. A whole section of AGENTS.md was measured with the transfer
// gate's minimum-gain threshold at 0.7, the threshold was retracted to 0.4 three
// commits later, and nothing recorded the link — so a later audit cited the
// section as ground truth for a model that no longer existed. Separately, a
// six-arm sweep was killed under load after three arms and the gap was invisible
// until somebody counted rows.
//
// Both are fixed by the same thing: every measurement travels with a stamp
// saying what produced it, and silence is never allowed to read as success.
//
// # What is deliberately NOT here
//
// No standard errors, no t statistics, no p-values, no verdict words. Those are
// computed in stats/*.R and nowhere else — see stats/README.md, and
// TestInferenceLivesInOnePlace, which fails if Go grows a second copy. This
// package *reads* R's numbers and formats them beside the model's. Two
// implementations of one quantity is the bug class behind DefaultBenchWeight
// against Weights.BenchWeight, where the measured value turned out not to be the
// one that ran.
package snapshot

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Constant is one shipped setting in force at the moment a sweep ran.
//
// Path is the dotted config path as it appears in config.json ("weights.
// minutes_half_life"), so a reader can find and change the thing named. Value is
// its canonical text form.
type Constant struct {
	Path  string
	Value string
}

// Fingerprint is the full set of constants in force, plus a short digest of it.
//
// The digest is what a snapshot header carries — it answers "was this measured
// under the same model as that?" in one glance. The full list is what a diff
// between two snapshots reads, so a figure that moved can be attributed to the
// constant that moved rather than left unexplained.
type Fingerprint struct {
	Digest    string     // first 12 hex chars of the sha256 of the canonical list
	Constants []Constant // sorted by Path
	Env       []Constant // FPL_* switches actually set, sorted by name
}

// envSwitches are the environment variables that change what the model or the
// replay does. Every one of them silently reshapes a measurement, so a sweep run
// with one set is not comparable with a sweep run without it.
//
// Generated rather than hand-listed: the list is derived by grepping the tree for
// FPL_ identifiers, and TestEnvSwitchListIsComplete fails when the source grows
// one this slice does not have. Hand-maintained lists in this project rot — the
// four season lists that go stale every summer are the standing example — so the
// only safe kind is one a test counts.
//
// FPL_SESSION is excluded deliberately: it is a credential, and a snapshot is a
// committed artefact.
var envSwitches = []string{
	// The four constants of the two appearance curves, as
	// "sixty_slope,sixty_midpoint,cond_intercept,cond_slope". It reaches four
	// scoring consumers at once — the appearance points, the clean sheet's
	// sixty-minute scaling, the derived bench slot weights and the defensive
	// contribution's exposure — so a run with it set is scoring a different model,
	// not the same model more precisely.
	"FPL_APPEARANCE_FIT",
	"FPL_ATK_FIXTURE_SCALE",
	// The club-form blend's weight: it multiplies every player's expected goals
	// and assists at a club by (recent / season-to-date)^w, so a run with it set
	// is scoring a different league, not the same league more precisely.
	"FPL_TEAM_FORM",
	// Restores the club-form blend's *unadjusted* form signal, which is form plus
	// the fixture list rather than form at neutral difficulty. The two differ by
	// exactly the fixture component, and a club's fixture ease is anti-persistent
	// at −0.519, so the two arms are materially different measurements and not one
	// measurement at two precisions.
	"FPL_TEAM_FORM_RAW",
	"FPL_BAND_STRENGTH",
	"FPL_BENCH_SLOTS",
	"FPL_BENCH_WEIGHT",
	"FPL_BLANK_RUN_MAX",
	"FPL_BLANK_RUN_PENALTY",
	"FPL_BUDGET_WEIGHT",
	"FPL_BUY_DISCOUNT",
	"FPL_CAPTAIN_SHRINK",
	// A chip plan changes what the season scored, so two runs disagreeing on it
	// are not comparable. It belongs here rather than in the skip list with the
	// page-output switches for exactly that reason: it reaches the model.
	// ⚠️ Read only by `cmd/armband backtest`, not by the sweep harness, which
	// sets SimConfig.Chips directly. Fingerprinted because the *command* scores a
	// different season with it — but a sweep run with it set gets a changed stamp
	// over byte-identical cells, which is the byte-identical null inverted and
	// exactly as misleading.
	"FPL_CHIP_PLAN",
	// Not a model setting but a pointer to *all* of them: it selects which
	// config.json the diagnostics load, so two runs disagreeing on it disagree
	// on every weight at once. That is the strongest possible reason to
	// fingerprint it rather than skip it as a mere path.
	"FPL_CONFIG",
	"FPL_CS_SCALE",
	"FPL_CS_XGC_FACTOR",
	"FPL_DEF_FIXTURE_SCALE",
	"FPL_FIXED_BENCH_SLOTS",
	"FPL_FIXTURE_LOAD",
	"FPL_FLAT_BENCH",
	"FPL_MAGNITUDE",
	"FPL_MAGNITUDE_ALPHA",
	"FPL_MINUTES_WEIGHT",
	"FPL_MULTI_SURCHARGE",
	"FPL_NO_AVAILABILITY",
	"FPL_NO_BLANK_RUN",
	"FPL_NO_FIXTURE_LOAD",
	"FPL_NO_FUNDED_UPGRADE",
	"FPL_NO_LOAD_TRANSFERS",
	"FPL_NO_SAVES_FIXTURE",
	// Unpins the engine's scoring RULES rather than a constant, so every archived
	// season is re-scored under whatever FPL pays today — a goalkeeper's goal at
	// 10 where the season paid 6. It is the arm the pin's confinement run is
	// measured against, and it moves `BaseXP90` on 552 player-cutoffs of the
	// six-season grid, so a cell banked with it set is not a cell banked without.
	// It reaches the xPoints instrument too, since both read `ScoringRulesFor`.
	"FPL_NO_SEASON_SCORING_RULES",
	"FPL_NO_SHORT_PLAY",
	"FPL_NO_THRESHOLD_SPLIT",
	"FPL_NO_UNIFIED_APPEARANCE",
	// Restores the bare map index this project removed: an element_type the rules
	// have no entry for prices its goals channel at the map's zero and scores on
	// through, rather than being refused. Stronger than a constant — it changes
	// *what gets scored at all* — and on the archive it puts FPL's 2024-25
	// assistant managers back at BaseXP90 2.0, which is 40 rows that a run with it
	// set carries and a run without it does not.
	"FPL_NO_UNPRICED_POSITION_GUARD",
	"FPL_NO_VICE_CAPTAIN",
	// Restores the old autosub behaviour, which substituted in bench order
	// regardless of formation legality — an outfielder for a blanking keeper, or a
	// fourth defender behind a back three. Worth 7 to 14 points a season, so a run
	// with it set is measuring a materially different scoring rule.
	"FPL_NO_LEGAL_AUTOSUBS",
	// Whether a wildcard played the week BEFORE a bench boost optimises the
	// fifteen FOR the chip or builds an ordinary squad. It separates the cost of
	// the wildcard from the cost of building for the boost, which the first
	// sequence measurement conflated — so two runs at different settings are
	// measuring different questions, not the same one more precisely.
	"FPL_WC_IGNORES_BOOST",
	// Leaves a backfilled season's expected-goals *totals* at zero while still
	// repairing its weekly rows. The narrower of the two backfill switches: it
	// isolates what a prior season read through an unwritten aggregate cost, which
	// is a different question from what the backfill is worth.
	"FPL_NO_XG_AGGREGATE",
	// Restores the unrepaired 2022-23 archive. It changes the *data* rather than a
	// constant, which is if anything a stronger reason to fingerprint it: two
	// sweeps at identical settings are not comparable if one replayed a season
	// with fifteen gameweeks of xG missing.
	"FPL_NO_XG_REPAIR",
	// Turns off the expected-goals-CONCEDED reconstruction while leaving the
	// attacking backfill in place. The narrowest of the three, and the one whose
	// two arms differ most: `baseXP90` gates both the clean sheet and the
	// goals-conceded deduction on `XGC90 > 0`, so a run with this set scores every
	// defender and keeper in four seasons with neither term — 26-45% of their
	// points — and picks a different squad as a result.
	"FPL_NO_XGC_REPAIR",
	// Selects a measured per-match xGC source in place of the reconstruction for
	// the seasons FPL never backfilled. It changes what a season HOLDS, so two
	// sweeps that disagree about it are measuring different archives — the same
	// reason the three switches around it are fingerprinted, arriving from the
	// other direction: this one adds data rather than removing it.
	"FPL_XGC_EXTERNAL_DIR",
	// Overwrites EVERY priced xGC row from a named source, including rows FPL
	// published. Diagnostic only, and the most invasive switch in this list: an
	// arm that sets it is not replaying the archive at all.
	"FPL_XGC_FORCE",
	// Which horizon the template diagnostic reads the optimum on. It is a
	// diagnostic-only lens, but it changes WHICH SQUAD is called optimal — a
	// horizon-1 optimum chases the imminent fixture and churns far wider than a
	// season-long one — so two runs that disagree about it are not measuring the
	// same concentration. Fingerprinted for the same reason the repair switches
	// are: a reading whose lens is not recorded cannot be compared with another.
	"FPL_TEMPLATE_HORIZON",
	// Restores the rank reconstruction of the starting eleven in place of the
	// recorded starts harvested from Understat.
	//
	// ⚠️ An earlier version of this comment claimed a run with this set "flatters
	// exactly the rotation and returning players a squad decision turns on". That is
	// **wrong at shipped config, and wrong twice over**: `reliabilityFrom` computes
	// `w*minutesShare + (1-w)*startShare` with `reliabilityMinutesShare = 1.0`, so
	// start share is multiplied by zero; and `appearanceOdds` reads `StartShare` only
	// in the `!unifiedAppearance` branch, which does not ship, so `blankRate` and the
	// bench-slot weights derived from it do not read it either. **At shipped flags
	// no scoring path reads `Starts`**, and the two arms are expected to be
	// byte-identical.
	//
	// It is fingerprinted anyway, and the expectation is the reason rather than an
	// exception to it. The switch does change what the oracles, the diagnostics and
	// the agent-facing field see, and it becomes a scoring switch the moment
	// `FPL_RELIABILITY_SPLIT` or `FPL_NO_UNIFIED_APPEARANCE` is set. A run whose
	// cells differ while this is set has found something worth knowing, and it can
	// only be noticed if the digest records which arm ran.
	"FPL_NO_STARTS_REPAIR",
	// Which config.json a diagnostic measures against. Not a model constant but
	// the *source* of every model constant, so it belongs here rather than in the
	// skip list beside the output paths: two sweeps run against different config
	// files are no more comparable than two run at different scoring weights, and
	// nothing else in a snapshot would say which file was read.
	"FPL_CONFIG",
	"FPL_ORACLE_AVAILABILITY",
	"FPL_ORACLE_PRICES",
	"FPL_POS_MINUTES_SCALE",
	"FPL_PRIOR",
	"FPL_RELIABILITY_SPLIT",
	// The replay's entry-gameweek grid. Not a model setting, but it changes the
	// *population* a sweep measures, so two runs at different grids are no more
	// comparable than two runs at different scoring constants.
	"FPL_SWEEP_STARTS",
	// The replay's SEASON grid. "extended" swaps the shipped four pairs for the
	// six the Understat backfill makes playable, which takes the season-clustered
	// degrees of freedom from three to five. That is the single biggest change
	// available to what this harness can resolve, so a figure measured under it
	// is not comparable with one measured on the four — and unlike a constant,
	// nothing about the printed output looks different.
	"FPL_SWEEP_SEASONS",
	// How stale a recovered team-news capture may be, in hours before the deadline
	// it is evidence about. Like FPL_NO_XG_REPAIR it changes the *data* rather than
	// a constant, and it changes it silently: two runs of the team-news oracle at
	// different cuts replay different amounts of recovered availability — 228 of
	// 228 gameweeks unset against 199 at 24 hours — and nothing in the printed
	// output distinguishes them. An unset value is the whole export, which is the
	// setting every headline figure in that section was measured at.
	"FPL_TEAMNEWS_MAX_HOURS",
	// Restores the unregistered players to the replay's pool, priced at the
	// closing price. Like FPL_NO_XG_REPAIR it changes the *population* rather
	// than a constant — it puts 18-26% more players in the GW1 pool, some of them
	// below FPL's £4.0m minimum — so a sweep run with it set is not comparable
	// with one run without it.
	"FPL_UNREGISTERED_POOL",
	"FPL_UNIFIED_TRANSFERS",
	"FPL_WEIGHT",
}

// modelSubtrees are the config fields whose values change what a player scores or
// what a transfer decision does. Everything else in Config — the entry id, the
// report directory, the cache window, the model name — cannot move a replayed
// point, so including it would make the digest change for reasons that are not
// about the model.
var modelSubtrees = []string{"weights", "congestion", "role_risk", "review_policy", "chip_plan"}

// Fingerprint flattens a config into the constants in force and digests them.
//
// It goes through JSON rather than reflection for two reasons. The paths come out
// as the json tags, which are the names a human edits in config.json rather than
// Go field names nobody greps for. And Go's encoder emits map keys in sorted
// order, so the walk is deterministic without this code having to sort anything
// but its own output — a hand-rolled reflection walk over the two
// map[string]... fields in Congestion would have had to, and would have been the
// place a future field quietly became order-dependent.
//
// cfg is taken as any so this package does not import config, which imports
// analysis: the snapshot renderer is a leaf and should stay one.
// envPathValued names the fingerprinted switches whose value is a FILESYSTEM
// PATH rather than a setting, so the sidecar records a digest of it instead of
// the path itself.
//
// ⚠️ **A provenance sidecar is committed to a PUBLIC repository**, and a path
// names the machine it was measured on. One of these switches also names a data
// source that may not be published at all, so writing its directory into a
// banked cells file publishes both the host layout and the source. That happened:
// three sidecars banked on 2026-08-25 carry an absolute path and reached
// `origin/main` before this existed.
//
// A digest keeps everything the fingerprint is FOR. Two runs against different
// directories still differ, which is the whole job — the guard in
// `stats/sweep_inference.R` compares values for inequality and never reads them.
// What is lost is the ability to tell WHICH directory from the sidecar alone,
// and that is the thing that must not be in there.
var envPathValued = map[string]bool{
	"FPL_XGC_EXTERNAL_DIR": true,
	"FPL_CONFIG":           true,
}

// pathFingerprint renders a path-valued switch as a short digest of WHAT IS AT
// the path, tagged so a reader knows a digest is what they are looking at rather
// than a corrupted value.
//
// ⚠️ **It digests CONTENTS, not the path string, and the difference is the whole
// point.** An earlier version hashed the string. Two runs pointing at one
// directory whose contents changed between them then produced byte-identical
// sidecars, so `commit` said the code matched, `WatchedDigest` said the watched
// files matched, and nothing at all said the DATA had moved. That is a guard
// reading as evidence while unable to act, which is this project's most expensive
// recurring failure — see TestPathFingerprintMovesWhenContentsMoveAtAFixedPath,
// which is written to fail against the string-hashing version.
//
// Cost is why this was not always so, and it was measured rather than assumed:
// an exact digest of the 784M/3205-file xGC cache takes 2.9s warm. A diagnostic
// that runs minutes of replay can afford that, and the exactness buys out both
// approximations — an mtime inventory reports false differences after a
// re-download of identical bytes, and a name-and-size one is blind to an in-place
// edit that keeps the size.
//
// Errors are values here rather than failures. A path that cannot be read is
// itself a comparability fact — "this run could not see its data source" — and
// recording it as such keeps two runs distinguishable, where returning an error
// would push callers into dropping the field and comparing as if it matched.
func pathFingerprint(v string) string {
	sum, err := contentDigest(v)
	if err != nil {
		// ⚠️ The error is DISCARDED rather than reported, and that is deliberate:
		// an os error carries the path it failed on, and this value is written
		// into a sidecar committed to a public repository. Reporting it would
		// reintroduce the leak this function exists to close.
		//
		// The digest falls back to the path STRING here, and this is the one place
		// that is right: an unreadable source is a distinct, labelled state rather
		// than a quiet substitute for a readable one. Two runs that could not read
		// the same path are genuinely in the same state and compare equal; a run
		// that could read it never collides with one that could not, because the
		// label differs. What is preserved is the old scheme's discrimination
		// between two different unreadable paths.
		sum := sha256.Sum256([]byte(v))
		return fmt.Sprintf("path:UNREADABLE:%x", sum[:6])
	}
	return fmt.Sprintf("path:%x", sum[:6])
}

// contentDigest digests a file, or a directory tree, byte for byte.
//
// Paths are walked in lexical order (fs.WalkDir guarantees it) and each file's
// repo-relative name is mixed in alongside its bytes, so a rename moves the
// digest and two different trees cannot collide by holding the same bytes under
// different names. Directory entries contribute their names and no bytes, so an
// emptied directory is still visible.
func contentDigest(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if !info.IsDir() {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		fmt.Fprintf(h, "file %s\n", filepath.Base(path))
		if _, err := io.Copy(h, f); err != nil {
			return nil, err
		}
		return h.Sum(nil), nil
	}
	err = fs.WalkDir(os.DirFS(path), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			fmt.Fprintf(h, "dir %s\n", name)
			return nil
		}
		fmt.Fprintf(h, "file %s\n", name)
		f, err := os.Open(filepath.Join(path, name))
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(h, f)
		return err
	})
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// EnvState is the fingerprinted environment as one run saw it.
//
// Set holds the switches that were set. Digest covers the same list, each written
// with its NAME, which is what makes it separate an arm that set a switch from an
// arm that did not — the signature of the fourth recorded comparability failure,
// where FPL_XGC_EXTERNAL_DIR was set for one run and unset for the other, nothing
// was dirty, no commit differed, both runs were individually correct, and a
// published verdict flipped sides. TestEnvDigestSeparatesSetFromUnset holds it.
type EnvState struct {
	Set    []Constant // fingerprinted switches actually set, sorted, paths digested
	Digest string     // over every switch, set or not
}

// CurrentEnv reads the fingerprinted switches out of the process environment.
//
// It is the single implementation: FingerprintOf builds a sidecar from it and the
// diagnostics print a run stamp from it, so a table and the cells banked beside it
// cannot disagree about what was set. Two implementations of one quantity is the
// DefaultBenchWeight-against-Weights.BenchWeight bug class, where the measured
// value turned out not to be the one that ran.
func CurrentEnv() EnvState {
	var set []Constant
	h := sha256.New()
	for _, k := range envSwitches {
		v, ok := os.LookupEnv(k)
		if !ok {
			// ⚠️ No "(unset)" line is written, and an earlier draft of this
			// function wrote one on the theory that "absence is a value". It is,
			// in stats/sweep_inference.R, which compares sidecars FIELD BY FIELD
			// and would otherwise skip a field one side does not carry. It is not
			// here: the loop writes each switch's NAME alongside its value, so a
			// digest over the set switches alone already separates an arm that set
			// a switch from one that did not. The extra line changed no digest.
			//
			// Recorded rather than deleted because the draft shipped with a test
			// asserting the line was load-bearing, and that test passed with the
			// line removed — it never distinguished the two. It was caught by
			// injecting the removal, which is the only thing that ever catches
			// this: a green null proves nothing, only a bite does.
			continue
		}
		// An empty-string value is still "set", and several switches are
		// tested for presence rather than value, so record it as set.
		if v == "" {
			v = "(set, empty)"
		} else if envPathValued[k] {
			v = pathFingerprint(v)
		}
		set = append(set, Constant{Path: k, Value: v})
		fmt.Fprintf(h, "env %s=%s\n", k, v)
	}
	return EnvState{Set: set, Digest: fmt.Sprintf("%x", h.Sum(nil))[:12]}
}

func FingerprintOf(cfg any) (Fingerprint, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return Fingerprint{}, fmt.Errorf("fingerprint: marshal config: %w", err)
	}
	var tree map[string]any
	if err := json.Unmarshal(b, &tree); err != nil {
		return Fingerprint{}, fmt.Errorf("fingerprint: reparse config: %w", err)
	}

	var out []Constant
	for _, sub := range modelSubtrees {
		v, ok := tree[sub]
		if !ok {
			// Absent rather than empty: a subtree this build expects and does not
			// find is a rename, and silently fingerprinting the remainder would
			// produce a digest that looks fine and covers less than it claims.
			out = append(out, Constant{Path: sub, Value: "ABSENT FROM CONFIG"})
			continue
		}
		out = append(out, flatten(sub, v)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	env := CurrentEnv().Set

	// The digest covers the env switches too. A sweep run with
	// FPL_NO_VICE_CAPTAIN=1 measures a different game from one run without it,
	// and a digest that ignored that would call the two comparable.
	h := sha256.New()
	for _, c := range out {
		fmt.Fprintf(h, "%s=%s\n", c.Path, c.Value)
	}
	for _, c := range env {
		fmt.Fprintf(h, "env %s=%s\n", c.Path, c.Value)
	}
	return Fingerprint{
		Digest:    fmt.Sprintf("%x", h.Sum(nil))[:12],
		Constants: out,
		Env:       env,
	}, nil
}

// flatten turns a decoded JSON value into path=value leaves.
//
// Lists of strings are summarised as a count plus a digest rather than inlined.
// The hand-maintained season lists — European campaigns, new-manager clubs,
// post-tournament rest — run to dozens of names each, and a snapshot header is
// not the place to read them. The digest still changes when the list does, which
// is the whole requirement: a summer maintenance pass that edits the rest list
// must show up as a changed fingerprint rather than as an unexplained shift in
// every number below it.
func flatten(path string, v any) []Constant {
	switch t := v.(type) {
	case nil:
		return []Constant{{Path: path, Value: "null"}}
	case bool:
		return []Constant{{Path: path, Value: strconv.FormatBool(t)}}
	case float64:
		// 'g' with -1 precision round-trips exactly, so a fingerprint never
		// collapses two genuinely different constants into one string.
		return []Constant{{Path: path, Value: strconv.FormatFloat(t, 'g', -1, 64)}}
	case string:
		return []Constant{{Path: path, Value: t}}
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []Constant
		for _, k := range keys {
			out = append(out, flatten(path+"."+k, t[k])...)
		}
		if len(out) == 0 {
			return []Constant{{Path: path, Value: "{} (empty)"}}
		}
		return out
	case []any:
		if len(t) == 0 {
			return []Constant{{Path: path, Value: "[] (empty)"}}
		}
		// Scalars: summarise. Structures: recurse, so a competition window's
		// dates are individually visible in a diff.
		scalar := true
		for _, e := range t {
			switch e.(type) {
			case map[string]any, []any:
				scalar = false
			}
		}
		if !scalar {
			var out []Constant
			for i, e := range t {
				out = append(out, flatten(fmt.Sprintf("%s[%d]", path, i), e)...)
			}
			return out
		}
		h := sha256.New()
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, fmt.Sprint(e))
		}
		// Sorted before digesting: a reordered list of the same clubs is the same
		// model, and a digest that changed for a reorder would report a
		// difference the replay cannot see.
		sort.Strings(parts)
		fmt.Fprint(h, strings.Join(parts, "\x00"))
		return []Constant{{
			Path:  path,
			Value: fmt.Sprintf("%d items, sha %x", len(t), h.Sum(nil)[:4]),
		}}
	default:
		return []Constant{{Path: path, Value: fmt.Sprint(t)}}
	}
}
