package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// Eight hand-maintained lists exist TWICE — as Go defaults and as literal copies
// in the shipped config.json — and which copy wins depends on the Go type of the
// field. That is the whole reason these tests exist.
//
// They are not deduplication. Overriding these from a config file is a feature
// and stays possible for anyone with their own file; what must not happen is the
// *shipped* pair drifting unnoticed, because the two copies carry different
// information. The Go copy carries the derivation — which nation, which fixture
// date, why the cutoff is semi-finalists — and the JSON copy carries bare names.
//
// # This has already shipped a live effect, which is why the scope is eight
//
// A first version of this guard covered only the four lists AGENTS.md's
// season-maintenance section names. That was too narrow: `long_haul_regions` is
// EMPTY in Go and `[30, 10]` in config.json with no backfill, so the file was
// authoritative and a 0.86 multiplier ran on every Brazilian and Argentine while
// a code comment said the term was inert. See DefaultCongestion's note. A guard
// that says "the season lists agree" while three duplicated lists disagree is
// documenting a false picture, so the divergent ones are listed here explicitly,
// with the reason each is allowed to differ.
//
// # The override contract, and the asymmetry it replaced
//
// `Load` does `cfg := Default()` and then unmarshals the file into it. **Since
// 2026-08-14 all eight lists checked here follow one rule: an absent key keeps the
// Go default, and a present key wins outright, `{}` and `null` included.** So a
// re-derivation done only in Go has no effect once the key is in the file — that
// is the hazard to hold in mind, and it is why the parity check below matters more
// than it looks.
//
// It used to be two rules, because encoding/json REPLACES a slice but MERGES into a
// non-nil map. The two campaign maps additionally had a `len(...) == 0` backfill,
// so they **could not be shortened by any route at all** — not by deleting a club,
// not by `{}`, not by `null`. `analysis.CampaignMap` now replaces, and that
// backfill and `TournamentAbsences`' are gone.
//
// ⚠️ **Two documented exceptions, and the reason is NOT "fixed arity".**
// `Review.Rules` and `MinutesWeightByPosition` still backfill, because for them an
// empty list is a *fallback trigger* rather than a statement — `Review.Rules` takes
// any number of rules, so "fixed arity" was the wrong word and is corrected here.
// `MinutesWeightByPosition` also still merges. `TestLoadLetsTheFileReplace...`
// asserts both as exceptions, so the claim cannot go wrong in either direction.
//
// Other list-valued fields outside this guard's scope — `criteria`, `rest_regions` —
// follow the same rule as the eight. The eight are specifically the lists that exist
// TWICE, which is what this file checks.
//
// # Why this compares the committed file rather than checking at runtime
//
// Same reason the R and median guards are source scans: the copies AGREE on the
// day they are written. Nothing at runtime can tell a correct pair from a pair
// that has not drifted yet.

// listCase is one duplicated list. `divergent` marks the ones that are allowed to
// differ, with the reason — a named exception is this guard's most useful output,
// not an embarrassment.
type listCase struct {
	jsonKey   string
	goFunc    string
	fold      bool   // compare case-insensitively
	divergent string // non-empty: expected to differ, and why
}

// shippedConfig parses the repository's config.json into a ZERO-value Config.
//
// It deliberately does NOT use Load, and that is still the right choice after the
// override contract changed. Load starts from Default(), so an ABSENT key comes
// back carrying the Go default and reads as agreement — which is exactly the drift
// this test hunts: a list dropped from the file entirely would look identical to
// one that matches. A zero value shows literally what the file says.
//
// (Before 2026-08-14 there was a second reason, now gone: Load merged into the
// campaign maps, so a map the file had emptied came back fully populated.)
func shippedConfig(t *testing.T) Config {
	t.Helper()
	path := filepath.Join("..", "..", "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		// Not a Skip. config.json is committed at the repository root, so its
		// absence is itself a defect, and this project's standing rule is that a
		// check which cannot run is indistinguishable in the output from a check
		// that passed.
		t.Fatalf("reading %s: %v", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return cfg
}

func TestTheShippedConfigsHandMaintainedListsMatchTheGoDefaults(t *testing.T) {
	cfg := shippedConfig(t)
	def := Default()

	// Slices. `fold` is set where the downstream lookup cannot see case:
	// NewCoachClubs goes through containsFold (rolerisk.go), and RestPlayers
	// through Boot.FindPlayers (client.go), which lowercases the query and then
	// matches exact/prefix/contains — so it is case-insensitive and considerably
	// fuzzier. TournamentAbsences is matched the same way.
	for _, c := range []struct {
		listCase
		inJSON, inGo []string
	}{
		{listCase{"weights.rest_players", "analysis.DefaultRestPlayers", true, ""},
			cfg.Weights.RestPlayers, def.Weights.RestPlayers},
		{listCase{"role_risk.new_coach_clubs", "analysis.DefaultNewCoachClubs", true, ""},
			cfg.RoleRisk.NewCoachClubs, def.RoleRisk.NewCoachClubs},
		{listCase{"role_risk.confirmed_starters", "RoleRisk default", true,
			"the Go default is empty and the shipped file names four players. " +
				"These exempt a player from NewSigningPenalty, which ships LIVE at " +
				"0.88, so the file is authoritative and the exemption only exists " +
				"for someone using the shipped config. Deliberate: the names are a " +
				"seasonal judgement, not a derivation."},
			cfg.RoleRisk.ConfirmedStarters, def.RoleRisk.ConfirmedStarters},
	} {
		t.Run(c.jsonKey, func(t *testing.T) {
			checkStrings(t, c.listCase, c.inJSON, c.inGo)
		})
	}

	// Int-code slices. No fold — these are numeric codes.
	for _, c := range []struct {
		listCase
		inJSON, inGo []int
	}{
		{listCase{"congestion.long_haul_regions", "DefaultCongestion", false,
			"the Go default is EMPTY and the shipped file sets [30, 10] " +
				"(Brazil, Argentina). There is no backfill for this field, so the " +
				"file is authoritative and a 0.86 multiplier was live on every run " +
				"while a comment claimed the term was inert. Kept populated " +
				"deliberately for a future measurement — see DefaultCongestion."},
			cfg.Congestion.LongHaulRegions, def.Congestion.LongHaulRegions},
		{listCase{"congestion.regular_international_regions", "DefaultCongestion", false,
			"the Go default is EMPTY and the shipped file sets five codes. Same " +
				"shape as long_haul_regions above."},
			cfg.Congestion.RegularIntlRegions, def.Congestion.RegularIntlRegions},
	} {
		t.Run(c.jsonKey, func(t *testing.T) {
			a := make([]string, 0, len(c.inJSON))
			for _, v := range c.inJSON {
				a = append(a, fmt.Sprint(v))
			}
			b := make([]string, 0, len(c.inGo))
			for _, v := range c.inGo {
				b = append(b, fmt.Sprint(v))
			}
			checkStrings(t, c.listCase, a, b)
		})
	}

	// tournament_absences is a struct slice, not a name list: each entry carries
	// the tournament, how many fixtures that group missed, and the players. It is
	// hand-maintained and dated (AFCON 2025), it is a SLICE so the file replaces
	// it, and it is live in scoring — so it belongs here even though it needs its
	// own comparison. Matched per group name, because a changed `Matches` on one
	// group is the interesting failure and a whole-slice DeepEqual would only say
	// "different".
	t.Run("weights.tournament_absences", func(t *testing.T) {
		byName := func(xs []analysis.TournamentAbsence) map[string]analysis.TournamentAbsence {
			m := map[string]analysis.TournamentAbsence{}
			for _, x := range xs {
				m[x.Name] = x
			}
			return m
		}
		inJSON, inGo := byName(cfg.Weights.TournamentAbsences), byName(def.Weights.TournamentAbsences)
		jsonNames := make([]string, 0, len(inJSON))
		for k := range inJSON {
			jsonNames = append(jsonNames, k)
		}
		goNames := make([]string, 0, len(inGo))
		for k := range inGo {
			goNames = append(goNames, k)
		}
		onlyJSON, onlyGo := setDiff(jsonNames, goNames, false)
		if len(onlyJSON) > 0 || len(onlyGo) > 0 {
			t.Errorf("weights.tournament_absences names different groups "+
				"(%d in the file, %d in Go).\n\n  only in config.json: %s\n"+
				"  only in Go:          %s\n\nA slice: the file replaces the Go copy.",
				len(inJSON), len(inGo), orNone(onlyJSON), orNone(onlyGo))
			return
		}
		for _, name := range goNames {
			j, g := inJSON[name], inGo[name]
			if j.Matches != g.Matches {
				t.Errorf("weights.tournament_absences[%q]: matches differ, "+
					"config.json %d against Go %d. That number is how many league "+
					"fixtures the group missed and it scales the correction.",
					name, j.Matches, g.Matches)
			}
			if only1, only2 := setDiff(j.Players, g.Players, true); len(only1) > 0 || len(only2) > 0 {
				t.Errorf("weights.tournament_absences[%q]: different players.\n"+
					"  only in config.json: %s\n  only in Go:          %s",
					name, orNone(only1), orNone(only2))
			}
		}
	})

	t.Run("congestion.european_campaigns", func(t *testing.T) {
		checkWindows(t, "congestion.european_campaigns", "analysis.DefaultEuropeanCampaigns",
			cfg.Congestion.European, def.Congestion.European)
	})
	t.Run("congestion.domestic_cup_campaigns", func(t *testing.T) {
		checkWindows(t, "congestion.domestic_cup_campaigns", "analysis.DefaultDomesticCups",
			cfg.Congestion.DomesticCups, def.Congestion.DomesticCups)
	})
}

// checkStrings compares two name lists as SETS. Order is not meaningful in any of
// them — every consumer is a membership test — and demanding an order would fail
// on a reordering that changes nothing, which is how a guard earns being deleted.
//
// ⚠️ A list marked `divergent` is asserted to STILL DIFFER. If someone reconciles
// it, this fails and asks for the exception to be removed, so the whitelist cannot
// quietly outlive its reason.
func checkStrings(t *testing.T, c listCase, inJSON, inGo []string) {
	t.Helper()
	onlyJSON, onlyGo := setDiff(inJSON, inGo, c.fold)
	same := len(onlyJSON) == 0 && len(onlyGo) == 0

	if c.divergent != "" {
		if same {
			t.Errorf("%s and %s() now AGREE, but this list is on the divergence "+
				"whitelist with the reason:\n\n  %s\n\n"+
				"If the divergence was deliberately closed, delete the whitelist "+
				"entry. An exception nobody removes outlives the reason it was "+
				"granted, which is what this branch of the check exists to stop.",
				c.jsonKey, c.goFunc, c.divergent)
		}
		return
	}
	if same {
		return
	}
	t.Errorf("%s and %s() have drifted apart (%d in the file, %d in Go).\n\n"+
		"  only in config.json: %s\n"+
		"  only in Go:          %s\n\n"+
		"This field is a SLICE, so the file REPLACES the Go copy: a re-derivation "+
		"done only in Go has no effect on any run that reads config.json. Note "+
		"that `[]` and `null` in the file both win too — deleting the key entirely "+
		"is the only way to fall back to the Go default.\n\n"+
		"These copies are maintained by hand every summer and carry different "+
		"information: the Go copy carries the derivation in its comment, the JSON "+
		"copy carries bare names. Edit BOTH, and read the Go comment first.\n"+
		"Overriding from your own config file is unaffected; this checks only the "+
		"pair committed here.",
		c.jsonKey, c.goFunc, len(inJSON), len(inGo),
		orNone(onlyJSON), orNone(onlyGo))
}

// checkWindows compares the two map copies on keys AND on the windows, because a
// stale DATE is the failure that matters most and a key-only check reads straight
// past it.
//
// ⚠️ Keys are compared EXACTLY, unlike the slices. The map lookups downstream are
// exact — `cg.European[team.ShortName]` — so a lowercase key in config.json is a
// real difference in that file's content, not formatting. Folding here also
// produced an actively misleading message: it matched "ars" to "ARS", then indexed
// the JSON map with the Go spelling and reported "config.json: (none)" for a club
// whose window the file does carry.
func checkWindows(t *testing.T, jsonKey, goFunc string,
	inJSON, inGo map[string][]analysis.CompetitionWindow) {
	t.Helper()

	jsonKeys := make([]string, 0, len(inJSON))
	for k := range inJSON {
		jsonKeys = append(jsonKeys, k)
	}
	goKeys := make([]string, 0, len(inGo))
	for k := range inGo {
		goKeys = append(goKeys, k)
	}
	onlyJSON, onlyGo := setDiff(jsonKeys, goKeys, false)
	if len(onlyJSON) > 0 || len(onlyGo) > 0 {
		t.Errorf("%s and %s() name different clubs (%d in the file, %d in Go).\n\n"+
			"  only in config.json: %s\n"+
			"  only in Go:          %s\n\n"+
			"⚠️ Since 2026-08-14 `analysis.CampaignMap` REPLACES on unmarshal, so the "+
			"file is authoritative when the key is present: a re-derivation done only "+
			"in Go has NO EFFECT, exactly as for the slice-typed lists. (It used to "+
			"merge, and could not be shortened by any route — the opposite hazard. The "+
			"conclusion is unchanged and the reason inverted.) A club that has gone out "+
			"of Europe must be removed from BOTH copies; `{}` now means \"nobody\".",
			jsonKey, goFunc, len(inJSON), len(inGo),
			orNone(onlyJSON), orNone(onlyGo))
		return
	}

	var stale []string
	for _, club := range goKeys {
		if !reflect.DeepEqual(inJSON[club], inGo[club]) {
			stale = append(stale, fmt.Sprintf("    %s:\n      config.json: %s\n      Go:          %s",
				club, summarise(inJSON[club]), summarise(inGo[club])))
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%s and %s() agree on which clubs but not on their windows:\n\n%s\n\n"+
			"A window carries start and end dates and sometimes the actual fixture "+
			"dates. A stale date in one copy is exactly the drift this checks for, "+
			"and it is invisible to a check on club names alone. Re-derive both.",
			jsonKey, goFunc, strings.Join(stale, "\n"))
	}
}

func summarise(ws []analysis.CompetitionWindow) string {
	if len(ws) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(ws))
	for _, w := range ws {
		s := fmt.Sprintf("%s %s..%s", w.Competition, w.Start, w.End)
		if len(w.MatchDates) > 0 {
			s += fmt.Sprintf(" (%d match dates)", len(w.MatchDates))
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// setDiff compares as sets. Entries are trimmed because both `containsFold` and
// `Boot.FindPlayers` trim, so a stray leading space is not a real difference.
func setDiff(a, b []string, fold bool) (onlyA, onlyB []string) {
	key := func(s string) string {
		s = strings.TrimSpace(s)
		if fold {
			s = strings.ToLower(s)
		}
		return s
	}
	inA := map[string]string{}
	for _, s := range a {
		inA[key(s)] = s
	}
	inB := map[string]string{}
	for _, s := range b {
		inB[key(s)] = s
	}
	for k, orig := range inA {
		if _, ok := inB[k]; !ok {
			onlyA = append(onlyA, orig)
		}
	}
	for k, orig := range inB {
		if _, ok := inA[k]; !ok {
			onlyB = append(onlyB, orig)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	return onlyA, onlyB
}

func orNone(xs []string) string {
	if len(xs) == 0 {
		return "(none)"
	}
	return strings.Join(xs, ", ")
}

// TestLoadLetsTheFileReplaceEveryHandMaintainedList pins the override contract
// the test above exists because of.
//
// ⚠️ It goes through `Load`, on a real file, and that is not incidental. A first
// version called `json.Unmarshal` directly, which pins a property of the standard
// library rather than of this repository — and a review probe proved it blind:
// inserting `cfg.Congestion.European = nil` into `Load` immediately before its
// unmarshal made deleting a club from config.json start working, the exact
// behaviour that test claimed to protect, and it still PASSED. A test that never
// exercises the function it names is testing the language.
//
// # What the contract now is, and what it used to be
//
// ONE rule for all eight duplicated lists: an absent key keeps the Go default,
// and a present key wins outright. So `{}` means "none", which is a statement a
// maintainer can now make.
//
// It used to be two rules. Slices replaced; maps MERGED, because encoding/json
// populates a non-nil map in place — and `Load` then backfilled an empty map back
// to the default, so the campaign lists could not be shortened by ANY route: not
// by deleting a club, not by `{}`, not by `null`. A club knocked out of Europe
// could not be removed. `analysis.CampaignMap.UnmarshalJSON` replaces, and the
// backfill is gone.
//
// This test asserts the DESIRED behaviour, unlike its predecessor which asserted
// the accidental one. The asymmetry was never chosen — it fell out of the field
// types — and it was pinned rather than fixed only because changing it alters what
// an existing config.json means. There is exactly one config.json, so that cost
// was payable.
func TestLoadLetsTheFileReplaceEveryHandMaintainedList(t *testing.T) {
	def := Default()
	goEuro := len(def.Congestion.European)
	goCups := len(def.Congestion.DomesticCups)
	goRest := len(def.Weights.RestPlayers)
	goCoach := len(def.RoleRisk.NewCoachClubs)
	if goEuro < 2 || goCups < 2 || goRest < 2 || goCoach < 2 {
		t.Fatalf("the Go defaults are too small to tell replacement from merging "+
			"(european %d, cups %d, rest %d, coach %d); each needs more than one entry",
			goEuro, goCups, goRest, goCoach)
	}

	load := func(t *testing.T, doc string) Config {
		t.Helper()
		p := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	// A file naming exactly ONE entry in each list. Under the old merge semantics
	// the maps would come back with the Go defaults still in them; that is the
	// regression this case catches.
	t.Run("a present key replaces", func(t *testing.T) {
		cfg := load(t, `{
		  "congestion":{
		    "european_campaigns":{"Nowhere FC":[{"competition":"UCL","start_date":"2026-01-01"}]},
		    "domestic_cup_campaigns":{"Nowhere FC":[{"competition":"League Cup","start_date":"2026-01-01"}]}
		  },
		  "weights":{"rest_players":["Nobody At All"]},
		  "role_risk":{"new_coach_clubs":["ZZZ"]}
		}`)
		for _, c := range []struct {
			name string
			got  int
		}{
			{"european_campaigns", len(cfg.Congestion.European)},
			{"domestic_cup_campaigns", len(cfg.Congestion.DomesticCups)},
			{"rest_players", len(cfg.Weights.RestPlayers)},
			{"new_coach_clubs", len(cfg.RoleRisk.NewCoachClubs)},
		} {
			if c.got != 1 {
				t.Errorf("%s: a file naming one entry should REPLACE the Go default, "+
					"leaving 1, got %d. If a map has gone back to merging, deleting a "+
					"club from config.json has silently stopped working again.",
					c.name, c.got)
			}
		}
		if _, ok := cfg.Congestion.European["ARS"]; ok {
			t.Error("european_campaigns still carries a Go-default club the file " +
				"never named, so it merged rather than replaced")
		}
	})

	// The distinction merging destroyed: "I did not say" against "I say: none".
	t.Run("an absent key keeps the Go default", func(t *testing.T) {
		cfg := load(t, `{"weights":{"horizon":5}}`)
		if got := len(cfg.Congestion.European); got != goEuro {
			t.Errorf("european_campaigns: an absent key must keep all %d Go defaults, got %d",
				goEuro, got)
		}
		if got := len(cfg.Weights.RestPlayers); got != goRest {
			t.Errorf("rest_players: an absent key must keep all %d Go defaults, got %d",
				goRest, got)
		}
	})

	// ⚠️ Every one of the eight, under BOTH empty forms.
	//
	// A first version checked `{}` and `null` for european_campaigns and `[]` for
	// rest_players, and left the other six unpinned — while `docs/configuration.md`
	// stated the contract for all eight. A doc claim that outruns its guard is how
	// this branch's own bug survived: the deleted `TournamentAbsences` backfill was
	// a `== nil`, so `null` resurrected the default while `[]` emptied it, and
	// nothing failed.
	//
	// `null` is the form to be careful about. The widespread belief is that it is a
	// no-op; it is not — encoding/json zeroes a map or slice on `null` — and that
	// belief is exactly what the deleted backfill encoded.
	t.Run("an empty list means none", func(t *testing.T) {
		for _, c := range []struct {
			field string
			docs  [2]string // the `{}`/`[]` form, then the null form
			got   func(Config) int
		}{
			{"congestion.european_campaigns",
				[2]string{`{"congestion":{"european_campaigns":{}}}`,
					`{"congestion":{"european_campaigns":null}}`},
				func(c Config) int { return len(c.Congestion.European) }},
			{"congestion.domestic_cup_campaigns",
				[2]string{`{"congestion":{"domestic_cup_campaigns":{}}}`,
					`{"congestion":{"domestic_cup_campaigns":null}}`},
				func(c Config) int { return len(c.Congestion.DomesticCups) }},
			{"weights.rest_players",
				[2]string{`{"weights":{"rest_players":[]}}`,
					`{"weights":{"rest_players":null}}`},
				func(c Config) int { return len(c.Weights.RestPlayers) }},
			{"weights.tournament_absences",
				[2]string{`{"weights":{"tournament_absences":[]}}`,
					`{"weights":{"tournament_absences":null}}`},
				func(c Config) int { return len(c.Weights.TournamentAbsences) }},
			{"role_risk.new_coach_clubs",
				[2]string{`{"role_risk":{"new_coach_clubs":[]}}`,
					`{"role_risk":{"new_coach_clubs":null}}`},
				func(c Config) int { return len(c.RoleRisk.NewCoachClubs) }},
			{"role_risk.confirmed_starters",
				[2]string{`{"role_risk":{"confirmed_starters":[]}}`,
					`{"role_risk":{"confirmed_starters":null}}`},
				func(c Config) int { return len(c.RoleRisk.ConfirmedStarters) }},
			{"congestion.long_haul_regions",
				[2]string{`{"congestion":{"long_haul_regions":[]}}`,
					`{"congestion":{"long_haul_regions":null}}`},
				func(c Config) int { return len(c.Congestion.LongHaulRegions) }},
			{"congestion.regular_international_regions",
				[2]string{`{"congestion":{"regular_international_regions":[]}}`,
					`{"congestion":{"regular_international_regions":null}}`},
				func(c Config) int { return len(c.Congestion.RegularIntlRegions) }},
		} {
			for _, doc := range c.docs {
				if got := c.got(load(t, doc)); got != 0 {
					t.Errorf("%s: with %s it must be EMPTY, got %d. An empty list is a "+
						"legitimate statement — nobody is in Europe this season — and "+
						"backfilling it conflates that with having said nothing. "+
						"docs/configuration.md states this contract for all eight fields.",
						c.field, doc, got)
				}
			}
		}
	})

	// The two documented exceptions, asserted as exceptions so the doc's "except
	// these two" cannot quietly become wrong in either direction.
	t.Run("the two documented exceptions still backfill", func(t *testing.T) {
		if got := len(load(t, `{"review_policy":{"rules":[]}}`).Review.Rules); got == 0 {
			t.Error("review_policy.rules no longer backfills on empty. That may be an " +
				"improvement, but docs/configuration.md names it as an exception, so " +
				"the page is now wrong.")
		}
		cfg := load(t, `{"weights":{"minutes_weight_by_position":{"GKP":1.0}}}`)
		if got := len(cfg.Weights.MinutesWeightByPosition); got < 2 {
			t.Errorf("minutes_weight_by_position no longer merges: naming one position "+
				"left %d entries. docs/configuration.md says the other three keep "+
				"their defaults.", got)
		}
	})
}

// TestRemovingAClubSurvivesASaveLoadCycle is the scenario the replace change
// exists for, end to end, and it is NOT the hand-editing one.
//
// `Engine.SetCompetitionWindows` is how the agent records that a club is out of a
// competition — `update_competition_status` with `remove: true` drops the club's
// last window and the result is persisted. Before `analysis.CampaignMap` replaced
// on unmarshal, that wrote a config with 8 clubs and the NEXT `Load` merged the
// 9 Go defaults back in, **silently resurrecting the club the agent had just
// removed**, with no error and nothing in the output to notice.
//
// That is far more reachable than a human editing config.json by hand, and it is
// the strongest justification for the change — a correction that undoes itself one
// run later is worse than one that never applied.
//
// The unit-level `{}`/`null` cases above do not cover it: this asserts the whole
// round trip, Save included, which is where a re-marshalled map could reintroduce
// the default.
func TestRemovingAClubSurvivesASaveLoadCycle(t *testing.T) {
	cfg := Default()
	before := len(cfg.Congestion.European)
	if before < 2 {
		t.Fatalf("need at least two clubs to remove one, have %d", before)
	}
	var victim string
	for club := range cfg.Congestion.European {
		if victim == "" || club < victim {
			victim = club // deterministic: a map range is not ordered
		}
	}

	// Copy-on-write, exactly as SetCompetitionWindows does, then drop the club.
	next := make(analysis.CampaignMap, before)
	for k, v := range cfg.Congestion.European {
		if k != victim {
			next[k] = v
		}
	}
	cfg.Congestion.European = next

	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, resurrected := back.Congestion.European[victim]; resurrected {
		t.Errorf("%q was removed, saved, and came back after Load. A config file "+
			"cannot express the removal, so an agent correction undoes itself on the "+
			"next run — silently. This is what analysis.CampaignMap's replacing "+
			"UnmarshalJSON exists to prevent.", victim)
	}
	if got := len(back.Congestion.European); got != before-1 {
		t.Errorf("after removing one club and a Save/Load cycle, expected %d clubs, "+
			"got %d", before-1, got)
	}

	// The complement: everything else must survive the cycle untouched, or the
	// test above would pass for the wrong reason — a Load that returned nothing.
	if got := len(back.Congestion.DomesticCups); got != len(cfg.Congestion.DomesticCups) {
		t.Errorf("the cup map did not survive the cycle: %d against %d",
			got, len(cfg.Congestion.DomesticCups))
	}
	if got := len(back.Weights.RestPlayers); got != len(cfg.Weights.RestPlayers) {
		t.Errorf("rest_players did not survive the cycle: %d against %d",
			got, len(cfg.Weights.RestPlayers))
	}
}
