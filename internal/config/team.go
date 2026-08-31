package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"armband/internal/analysis"
)

// TeamConfig is one manager's own settings: the decisions he has taken about
// his own entry, as distinct from the facts about the world that everything
// else in config.json records.
//
// # Why this is a second FILE and not a second block
//
// `config.json` has two consumers — the owner's CLI and agent runs, and the
// public server — and they want different things from it. Most of the file is
// meant to be shared: the roster minutes and exclusions ARE the published
// team-news research, `entry_id` is the house team the spectator page exists to
// show, and the review thresholds are the decision engine `page.go` reads a
// watchlist out of. Serving all of that is the point.
//
// What is NOT meant to be shared is a manager's strategy. A chip plan is one
// entry's plan for one season; it was served to every visitor for as long as it
// existed in the shared file, and `EffectiveHorizon` truncated a stranger's
// optimiser to the gameweeks before a wildcard he had not planned and could not
// see. `forPlanner` now strips these in the server, and that is the containment
// — but a strip is a list somebody has to remember to extend. A separate file
// the server is never handed cannot be forgotten: the deployment mounts
// `config.json` and nothing else, so there is no code path from `team.json` to a
// reader at all.
//
// # Why these five, and not the six a first pass picks
//
// Three fields that read as personal are shipped features and deliberately stay
// in `config.json`: `entry_id` (the whole point of `/armband-team`, which shows
// what the site's own squad is doing, for anyone), `wildcard_enabled` (a
// per-deployment toggle, see its own comment), and `roster.minutes` /
// `roster.exclude` (team news, which describes the world). Moving any of them
// deletes a page or a feature.
//
// ⚠️ **`review_policy.rules` looks like it belongs here and does NOT move.** It
// is free text in the owner's own words, the same shape as `criteria`, and the
// first pass at this split moved it for exactly that reason. It is a PUBLISHED
// SURFACE: `page.go` copies it into `present.Policy`, the viewmodel carries it
// into the JSON state, and the site renders it under "The rules it is deciding
// under" — a section whose template is gated on the list being non-empty, so
// emptying it deletes the transfer thresholds shown beside it too. Only
// `scheduled_run_lead_hours` leaves `review_policy`; the other thirteen fields
// stay. Reading a field's own call sites beats reading how personal it sounds.
//
// # ⚠️ Two fields here came out of nested objects, and they are TOP-LEVEL now
//
// `scheduled_run_lead_hours` leaves `review_policy` and `lock` leaves `roster`,
// and neither is re-nested here. A second `review_policy` object in this file
// would mean one JSON key naming different things in two files, which is the
// kind of thing that gets half-read by whoever finds one of them first; the same
// argument applies to `roster`, which would otherwise carry three lists there
// and a fourth here. `scheduled_run_lead_hours` keeps its name, which is already
// unambiguous on its own. `lock` keeps its name too: nothing else in this file
// or in config.json is called that.
type TeamConfig struct {
	// Chips is when this manager intends to play each chip. Both sets, since FPL
	// grants a second from GW20 in 2025-26 onward; `ChipSchedule.UnmarshalJSON`
	// also accepts the flat single-set object older files carry.
	//
	// The confirmed leak, and the reason this file exists. See the type comment.
	Chips analysis.ChipSchedule `json:"chip_plan"`

	// HypotheticalBudget is the money the squad builder plans with in millions
	// when EntryID is 0, and it is ignored entirely once there is a real squad
	// to price. Zero means FPL's £100.0m opening allowance.
	//
	// A question one manager is asking — "what would £103m buy today" — with no
	// reader on the serve path at all.
	HypotheticalBudget float64 `json:"hypothetical_budget_m"`

	// Criteria are this manager's own rules, passed verbatim to the agent, e.g.
	//   "Never own more than one Spurs player."
	//   "Prefer nailed starters over rotation risks, even at a points cost."
	//
	// The main place to encode personal preferences, by its own former doc
	// comment — which is exactly why it belongs here rather than in a file the
	// server loads.
	Criteria []string `json:"criteria"`

	// Lock is `roster.lock`: players that must appear in every squad and must
	// not be sold. A lock is a DECISION, not a fact — it asserts a conclusion
	// the optimiser cannot decline — which is why `forPlanner` has always
	// stripped it, and it is the precedent the chip-plan fix followed.
	//
	// Top-level rather than nested under `roster`, so one `roster` key does not
	// span two files. See the type comment.
	Lock []RosterOverride `json:"lock"`

	// LeadHours is `review_policy.scheduled_run_lead_hours`: how long before a
	// deadline this manager's scheduled run fires. It describes one person's
	// cron, and means nothing at all to a reader of the site.
	LeadHours float64 `json:"scheduled_run_lead_hours"`
}

// DefaultTeamConfig is the shipped team file: no chips planned, no locks, no
// budget of its own, and the criteria and lead time that ship.
//
// Both come from the same function and constant Default() and
// DefaultReviewPolicy() read, not from a second copy: a run WITHOUT -team must
// behave exactly as a config.json that omitted these keys always did, and one
// quantity with two implementations is this project's signature failure.
func DefaultTeamConfig() TeamConfig {
	return TeamConfig{
		Criteria:  defaultCriteria(),
		LeadHours: defaultLeadHours,
	}
}

// Team extracts the team half of a merged Config. It is the inverse of
// ApplyTo, and it is what SaveTeam and SavePair write.
func (c Config) Team() TeamConfig {
	return TeamConfig{
		Chips:              c.Chips,
		HypotheticalBudget: c.HypotheticalBudget,
		Criteria:           c.Criteria,
		Lock:               c.Roster.Lock,
		LeadHours:          c.Review.LeadHours,
	}
}

// ApplyTo layers the team file over a loaded Config, returning the merged view
// the rest of the program reads.
//
// Every field is assigned unconditionally rather than merged. LoadTeam has
// already resolved absence against the defaults, so what arrives here is a
// complete statement, and a second merge rule here would be a second place for
// "unset" to mean something.
func (t TeamConfig) ApplyTo(c Config) Config {
	c.Chips = t.Chips
	c.HypotheticalBudget = t.HypotheticalBudget
	c.Criteria = t.Criteria
	c.Roster.Lock = t.Lock
	c.Review.LeadHours = t.LeadHours
	return c
}

// LoadTeam reads the team file named by -team.
//
// ⚠️ A missing file is an ERROR, not a default. Load creates config.json when it
// is absent because a first run needs somewhere to write; -team is different —
// it is an explicit statement that a team file exists, and answering a typo'd
// path with a silently empty chip plan is the byte-identical null this whole
// change is about. Omit the flag to run without one.
func LoadTeam(path string) (TeamConfig, error) {
	t := DefaultTeamConfig()

	b, err := os.ReadFile(path)
	if err != nil {
		return t, fmt.Errorf("reading the team file %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &t); err != nil {
		return t, fmt.Errorf("parsing %s: %w", path, err)
	}

	// The one backfill that used to live in Load, moved with its field. An
	// absent `scheduled_run_lead_hours` must keep the shipped six hours,
	// exactly as an absent key in config.json always did.
	//
	// Value-checked rather than probed for key presence, and that is correct
	// HERE for the same reason it was correct in Load: zero lead hours is not a
	// schedule, it is an omission. Contrast the campaign maps in Config, where
	// empty IS a statement and presence has to be probed.
	//
	// `criteria` needs no backfill of its own: an absent key leaves the value
	// DefaultTeamConfig already put there, which is the shipped list. An
	// explicitly empty `[]` therefore MEANS empty, which is right — a manager
	// who has deleted every criterion has said something.
	if t.LeadHours <= 0 {
		t.LeadHours = DefaultTeamConfig().LeadHours
	}
	return t, nil
}

// SaveTeam writes the team half of a config to path, through the same
// write-to-a-sibling-and-rename that Save uses. See Save's own comment: the
// chip plan and the locks are exactly the sort of thing nothing else can
// reconstruct after a truncated write.
func SaveTeam(path string, t TeamConfig) error {
	return writeJSONAtomically(path, t)
}

// SavePair writes an owner-side change to whichever of the two files it
// touched, and refuses one it has nowhere to put.
//
// This exists because the write path has to follow the read path. `-persist`
// and the agent's `set_player_status` tool both persist a LOCK, and a lock now
// lives in the team file — so a writer holding only a config path would either
// write `roster.lock` back into `config.json`, where the next Load hard-errors
// on it, or drop it silently, which is the button lying. Both are worse than
// refusing.
//
// The comparison is on the whole extracted TeamConfig rather than on named
// fields, so a field added to TeamConfig later is covered without anybody
// remembering this function exists.
//
// Nothing is written when the team half moved and there is nowhere to write it:
// the config file is left exactly as it was, so a refused change is a change
// that did not happen rather than half of one.
func SavePair(cfgPath, teamPath string, before, after Config) error {
	teamChanged := !reflect.DeepEqual(before.Team(), after.Team())
	if teamChanged && teamPath == "" {
		return fmt.Errorf("this change touches a team setting (%s), which lives in the "+
			"team file rather than in %s — re-run with -team <path>; nothing was written",
			strings.Join(teamKeysChangedBetween(before, after), ", "), cfgPath)
	}
	if err := Save(cfgPath, after); err != nil {
		return err
	}
	if teamChanged {
		return SaveTeam(teamPath, after.Team())
	}
	return nil
}

// teamKeysChangedBetween names the team keys a change actually moved, so the
// refusal above says which setting it is about rather than just that one moved.
func teamKeysChangedBetween(before, after Config) []string {
	b, a := before.Team(), after.Team()
	bv, av := reflect.ValueOf(b), reflect.ValueOf(a)
	typ := bv.Type()
	var names []string
	for i := 0; i < typ.NumField(); i++ {
		if reflect.DeepEqual(bv.Field(i).Interface(), av.Field(i).Interface()) {
			continue
		}
		names = append(names, jsonName(typ.Field(i)))
	}
	return names
}

func jsonName(f reflect.StructField) string {
	tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if tag == "" || tag == "-" {
		return f.Name
	}
	return tag
}

// movedTeamKey is one key that used to live in config.json and now lives in the
// team file: the JSON path it had, and the name it answers to now.
type movedTeamKey struct {
	path []string
	now  string
}

// movedTeamKeys is the migration's whole subject matter.
//
// ⚠️ This table is why the migration is LOUD. `internal/config` does not use
// DisallowUnknownFields, so a key moved out of Config and left in the file
// would be parsed, ignored, and the setting would simply stop applying — with
// nothing anywhere saying so. That is the byte-identical null this project keeps
// being caught by: a chip plan that reads as "no chips planned" is
// indistinguishable from a manager who planned none.
//
// So a config.json still carrying one of these is an ERROR, not a warning and
// not a silent merge. Adding a field to TeamConfig means adding a row here.
var movedTeamKeys = []movedTeamKey{
	{[]string{"chip_plan"}, "chip_plan"},
	{[]string{"hypothetical_budget_m"}, "hypothetical_budget_m"},
	{[]string{"criteria"}, "criteria"},
	{[]string{"roster", "lock"}, "lock"},
	{[]string{"review_policy", "scheduled_run_lead_hours"}, "scheduled_run_lead_hours"},
}

// ⚠️ `review_policy.rules` is deliberately NOT in that table. It stays in
// config.json — it is rendered on every page under "The rules it is deciding
// under" — and listing it here would refuse every config that carries it. See
// ReviewPolicy.Rules.

// checkNoTeamKeys refuses a config.json that still carries a team setting.
//
// It names the key AND what it is called in the team file, because the two
// differ for three of the six and a message that only said "move it" would send
// the reader looking for a `review_policy` block that is not there.
func checkNoTeamKeys(raw []byte, path string) error {
	for _, k := range movedTeamKeys {
		if !hasKey(raw, k.path...) {
			continue
		}
		was := strings.Join(k.path, ".")
		as := k.now
		if was == as {
			as = "the same key"
		} else {
			as = "\"" + as + "\""
		}
		return fmt.Errorf(
			"%s still carries %q, which moved to the team file: put it in a team.json "+
				"beside this file as %s and load it with -team, then delete it here. "+
				"Leaving it would parse and be ignored, and the setting would silently "+
				"stop applying",
			path, was, as)
	}
	return nil
}

// writeJSONAtomically is Save's write, shared with SaveTeam. See Save for the
// argument: os.WriteFile opens O_TRUNC, so a crash between the truncate and the
// write leaves a half-written file, and these two files hold the only copy of
// the roster overrides, the review policy and the chip plan.
func writeJSONAtomically(path string, v any) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	// A sibling deliberately: /tmp is frequently a different filesystem and
	// rename across filesystems fails.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once the rename succeeds
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file 0600; these files have always been 0644.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// FingerprintView is the JSON shape internal/snapshot digests into a
// constants_digest: the merged config with `chip_plan` put back.
//
// ⚠️ It exists because `Chips` is `json:"-"` now. `snapshot.FingerprintOf`
// marshals whatever it is handed and walks `modelSubtrees`, which names
// `chip_plan` — and a subtree it expects and cannot find is recorded as "ABSENT
// FROM CONFIG". So marshalling a bare Config would do two bad things at once:
// change every constants_digest over a byte-identical model, and stop
// fingerprinting a value the replay genuinely reads (`Simulate` resolves
// `cfg.Chips` into the plan it plays). Both are the "changed stamp over
// unchanged cells" failure the fingerprint's own comments name.
//
// Re-attaching the field reproduces the pre-split digest exactly for a config
// carrying the same plan, which is the property that keeps banked cells
// comparable across this change.
//
// The other four team settings are deliberately NOT here: none of them is in
// `modelSubtrees`, and `hypothetical_budget_m`, `criteria`, `lock` and
// `scheduled_run_lead_hours` were never fingerprinted before either.
func (c Config) FingerprintView() any {
	return struct {
		Config
		Chips analysis.ChipSchedule `json:"chip_plan"`
	}{Config: c, Chips: c.Chips}
}
