package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"armband/internal/analysis"
)

// Config holds everything the agent needs that isn't derived from the FPL API.
type Config struct {
	// EntryID is your FPL manager ID. Find it in the URL when you view your
	// points page: fantasy.premierleague.com/entry/<THIS NUMBER>/event/1.
	// Leave 0 before the season starts or if you only want squad suggestions.
	EntryID int `json:"entry_id"`

	// HypotheticalBudget is the money the squad builder plans with in millions
	// when EntryID is 0. Zero means FPL's £100.0m opening allowance.
	//
	// It applies only when there is no squad to price. With an EntryID the
	// budget is that squad's selling value plus its bank, and this is ignored —
	// overriding a real budget with a number from a file would be inventing
	// money. Its purpose is the mid-season projection: asking what £103m would
	// buy today, for a team that is not yours or does not exist.
	HypotheticalBudget float64 `json:"hypothetical_budget_m"`

	// Model is the Claude model to reason with.
	Model string `json:"model"`

	// Effort trades cost and latency against depth: low|medium|high|xhigh|max.
	Effort string `json:"effort"`

	// MaxIterations caps how many tool-calling rounds a single run may take.
	// This is a runaway ceiling, not a typical-case setting: the loop ends when
	// the model stops calling tools, so lowering it bounds the worst case
	// rather than reducing normal spend.
	MaxIterations int `json:"max_iterations"`

	// Weights tune the player scoring model.
	Weights analysis.Weights `json:"weights"`

	// Congestion models European, international and travel load. The FPL API
	// does not publish European qualification or nationality names, so the
	// club and region lists must be filled in by hand — run `armband nations`
	// to map nationality codes to countries.
	Congestion analysis.Congestion `json:"congestion"`

	// RoleRisk prices uncertainty about whether a player's statistical record
	// still applies — summer transfers and managerial changes. Neither is in
	// the FPL API's team data, so new_coach_clubs must be filled in by hand.
	RoleRisk analysis.RoleRisk `json:"role_risk"`

	// Chips records when you intend to play each chip. Zero means unplanned.
	// The plan feeds squad construction: a wildcard shortens the horizon the
	// current squad must serve, and a bench boost makes all fifteen players
	// count rather than eleven plus fodder.
	//
	// Both sets, since FPL grants a second from GW20 in 2025-26 onward. No
	// backfill is needed in Load: `ChipSchedule.UnmarshalJSON` accepts the flat
	// single-set object every existing config.json carries and reads it as the
	// first set, which is the only set the seasons those files were written for
	// granted.
	Chips analysis.ChipSchedule `json:"chip_plan"`

	// Review is the standing brief for the weekly decision: the thresholds and
	// rules the agent must work within when deciding whether to act.
	Review ReviewPolicy `json:"review_policy"`

	// OptionValue prices the four things this project holds and can only spend
	// once: a banked transfer, a wildcard, a bench boost and a free hit. Every
	// lever in it ships off. See OptionValuePolicy.
	OptionValue OptionValuePolicy `json:"option_value"`

	// Roster is the standing set of player locks and exclusions the analysis
	// layer has established — injuries, lost places, players the squad must be
	// built around. These bind every solver call and survive between runs.
	Roster Roster `json:"roster,omitempty"`

	// WildcardEnabled turns on /wildcard and /api/wildcard (what a wildcard
	// or free hit would buy right now) and the "If we chipped" link that
	// points at them. False by default, deliberately: an operator's own
	// config.json omitting the field, or a fresh one written before this
	// existed, gets the safe answer rather than silently publishing a page
	// nobody asked to ship yet. Flip it in config.json and reapply — no
	// rebuild — when ready; unlike ARMBAND_SIGNUPS_DSN this carries no
	// secret, so it belongs in the operator config that already carries
	// every other per-deployment toggle rather than in a second mechanism.
	WildcardEnabled bool `json:"wildcard_enabled"`

	// Criteria are your own rules, passed verbatim to the agent. This is the
	// main place to encode personal preferences, e.g.
	//   "Never own more than one Spurs player."
	//   "Prefer nailed starters over rotation risks, even at a points cost."
	Criteria []string `json:"criteria"`

	// ReportDir is where Markdown reports are written.
	ReportDir string `json:"report_dir"`

	// CacheDir stores FPL API responses between runs.
	CacheDir string `json:"cache_dir"`

	// CacheMinutes is how long cached API responses stay fresh.
	CacheMinutes int `json:"cache_minutes"`

	// PlayerCacheMinutes is CacheMinutes' own TTL, but only for ElementSummary —
	// the per-player match history the playerDetail route fetches on demand,
	// once per viewed player, rather than once per process. CacheMinutes is set
	// deep in `serve`'s deployment because Bootstrap and Fixtures are memoised
	// for the process and an on-call human wants that TTL near-infinite so an
	// FPL outage cannot CrashLoop a pod; ElementSummary carries no such
	// constraint; every call to it already re-checks the cache per request, so
	// a short TTL here just means a viewed player's stats catch up to FPL
	// between the (much slower) archive-wide refresh cycles, without touching
	// the frozen-worldview guarantee the other endpoints rely on.
	PlayerCacheMinutes int `json:"player_cache_minutes"`

	// SnapshotDir is an optional read-only base checked before CacheDir on every
	// read; CacheDir remains the only place anything is ever written. Empty (the
	// zero value, and Default()'s value) disables it entirely, which is every
	// existing config.json and every non-deployed use — the client behaves
	// exactly as it did before this field existed. This needs no backfill in
	// Load: Default() already leaves it "", and json.Unmarshal onto that default
	// leaves it "" for any config.json that predates the field, which is the
	// correct "disabled" state, not an omission that needs correcting.
	SnapshotDir string `json:"snapshot_dir"`

	// XGCExternalDir names a directory of measured per-match expected-goals-
	// conceded files, used in place of the reconstruction for the seasons FPL
	// never backfilled. Empty — the zero value, Default()'s value, and this
	// repository's shipped config.json — selects the reconstruction, which is
	// what every clone reads.
	//
	// ⚠️ **Setting it is a deliberate estimator swap, and the price is that every
	// figure measured on the reconstruction becomes incomparable with one
	// measured after.** It is an operator's choice about which archive a process
	// replays, not a tuning knob.
	//
	// ⚠️ **Naming a directory that does not resolve is a hard error out of
	// backtest.Load, not a fall back.** A source that silently degrades to the
	// reconstruction is indistinguishable from one that worked, which is how two
	// incomparable sweeps come to look like one. See internal/backtest/
	// xgcexternal.go for the whole argument.
	//
	// Like SnapshotDir this needs no backfill in Load: Default() leaves it "",
	// and unmarshalling a config.json that predates the field leaves it "",
	// which is the correct disabled state rather than an omission.
	XGCExternalDir string `json:"xgc_external_dir"`
}

func Default() Config {
	return Config{
		EntryID:       0,
		Model:         "claude-opus-5",
		Effort:        "high",
		MaxIterations: 25,
		Weights:       analysis.DefaultWeights(),
		Congestion:    analysis.DefaultCongestion(),
		RoleRisk:      analysis.DefaultRoleRisk(),
		Review:        DefaultReviewPolicy(),
		OptionValue:   DefaultOptionValuePolicy(),
		Criteria: []string{
			"Expected points only become real points if the player is on the pitch. Treat expected minutes as a first-class filter, not a tiebreaker: check expected_minutes_per_gw and rotation_risk before recommending anyone.",
			"Never recommend a starting XI player below roughly 60 expected minutes per gameweek unless you say explicitly why the rotation risk is worth it.",
			"Weight underlying numbers (xG, xA, xGC) over raw past points — form follows the underlying data, not the other way round.",
			"Value fixture runs over the next 4-6 gameweeks, not just the next match.",
			"Set-piece and penalty duty is a major tiebreaker: a penalty taker is worth roughly half a goal per five matches on its own.",
			"Be sceptical of players heavily overperforming their xG — say so explicitly when recommending or fading them.",
			"For new signings, last season's stats came from a different club. Say so, and be explicit that their role in the new side is unproven.",
			"Players returning late from a summer international tournament are routinely eased back in. Prefer rested alternatives for the opening weeks.",
		},
		ReportDir:          "reports",
		CacheDir:           filepath.Join(".cache", "fpl"),
		CacheMinutes:       60,
		PlayerCacheMinutes: 15,
	}
}

// Load reads config from path, creating it with defaults if absent.
func Load(path string) (Config, error) {
	cfg := Default()

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := Save(path, cfg); err != nil {
			return cfg, fmt.Errorf("writing default config: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Backfill anything the user left out.
	//
	// Only values that are meaningless at zero are backfilled. Term weights —
	// BonusWeight, FixtureWeight, SetPieceWeight — are deliberately absent from
	// this list, because zero is a setting rather than an omission: SetPieceWeight
	// ships at 0.0 after measurement showed it double-counted penalties, and a
	// guard here would silently overwrite anyone disabling one to re-run that.
	//
	// A key absent from the file keeps its default regardless, since Unmarshal
	// leaves fields it does not see alone and cfg starts from Default().
	d := Default()
	if cfg.Model == "" {
		cfg.Model = d.Model
	}
	if cfg.Effort == "" {
		cfg.Effort = d.Effort
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = d.MaxIterations
	}
	if cfg.Weights.Horizon <= 0 {
		cfg.Weights.Horizon = d.Weights.Horizon
	}
	if cfg.Weights.MinutesHalfLife <= 0 {
		cfg.Weights.MinutesHalfLife = d.Weights.MinutesHalfLife
	}
	// BonusPriorWeight's disabled state is -1, not 0: zero is a real setting
	// meaning "ignore a purely historical bonus rate entirely". An absent key
	// unmarshals to 0, which would silently switch the schedule on, so it is
	// probed for presence the way rest_minutes_factor is.
	if !hasKey(b, "weights", "bonus_prior_weight") {
		cfg.Weights.BonusPriorWeight = d.Weights.BonusPriorWeight
	}
	if cfg.Weights.BenchWeight <= 0 {
		cfg.Weights.BenchWeight = d.Weights.BenchWeight
	}
	if cfg.Weights.MinutesWeight <= 0 {
		cfg.Weights.MinutesWeight = d.Weights.MinutesWeight
	}
	// The post-tournament term used to be "rest_discount", a Score multiplier.
	// It now multiplies expected minutes instead, because that is the channel it
	// was measured in. The two are not the same number: the minutes exponent is
	// convex, so a minutes factor f lands at f^MinutesWeight on Score.
	//
	// Migrate rather than ignore. An unknown key unmarshals silently, so an old
	// file would lose the term altogether with nothing to show for it.
	//
	// Presence has to be probed separately. cfg starts from Default(), so a
	// missing rest_minutes_factor is indistinguishable from one written at the
	// default value — testing it against zero, as every other backfill here
	// does, would mean the migration never fires at all.
	var probe struct {
		Weights struct {
			Factor   *float64 `json:"rest_minutes_factor"`
			Discount *float64 `json:"rest_discount"`
		} `json:"weights"`
	}
	if err := json.Unmarshal(b, &probe); err == nil &&
		probe.Weights.Factor == nil && probe.Weights.Discount != nil &&
		*probe.Weights.Discount > 0 && *probe.Weights.Discount < 1 {
		exp := cfg.Weights.MinutesWeight
		if exp <= 0 {
			exp = d.Weights.MinutesWeight
		}
		// Preserve the old file's effective Score effect rather than jumping it
		// to the new calibrated default: the point is that behaviour does not
		// change silently under someone.
		cfg.Weights.RestMinutesFactor = math.Pow(*probe.Weights.Discount, 1/exp)
	}
	cfg.Weights.LegacyRestDiscount = 0
	if cfg.Weights.RestMinutesFactor <= 0 || cfg.Weights.RestMinutesFactor > 1 {
		cfg.Weights.RestMinutesFactor = d.Weights.RestMinutesFactor
	}
	// Backfill congestion penalties so a partially-written block still works.
	dc := d.Congestion
	for _, f := range []struct {
		p   *float64
		def float64
	}{
		{&cfg.Congestion.UCLPenalty, dc.UCLPenalty},
		{&cfg.Congestion.UELPenalty, dc.UELPenalty},
		{&cfg.Congestion.UECLPenalty, dc.UECLPenalty},
		{&cfg.Congestion.ShortRestPenalty, dc.ShortRestPenalty},
		{&cfg.Congestion.VeryShortRest, dc.VeryShortRest},
		{&cfg.Congestion.PostBreakPenalty, dc.PostBreakPenalty},
		{&cfg.Congestion.LongHaulPenalty, dc.LongHaulPenalty},
	} {
		if *f.p <= 0 || *f.p > 1 {
			*f.p = f.def
		}
	}
	// ⚠️ The two campaign maps are deliberately NOT backfilled on empty, and this
	// is the one place in this function where that is the right call.
	//
	// Every other backfill here reads "zero is meaningless for this field, so it
	// means the user left it out". For a list of clubs that is false: an empty
	// list is a legitimate statement — nobody is in Europe this season, or the
	// cup is over. Backfilling it conflated "I did not say" with "I say: none",
	// and combined with encoding/json merging into a non-nil map it meant the
	// list could not be SHORTENED by any route at all: not by deleting a club,
	// not by `{}`, not by `null`. A club knocked out of Europe could not be
	// removed.
	//
	// `analysis.CampaignMap` now replaces on unmarshal, so an absent key keeps
	// the Go default and a present key wins — the same rule the slice-typed
	// lists have always followed.
	// TestLoadLetsTheFileReplaceEveryHandMaintainedList pins all three cases.
	//
	// ⚠️ This is NOT a general licence to delete backfills. It applies to a
	// hand-maintained ENUMERATION whose membership can legitimately be zero. It
	// does not apply to `Review.Rules` or `MinutesWeightByPosition`, which are
	// fixed-arity structures where empty is meaningless rather than a statement,
	// and whose backfills below are correct.
	if cfg.Congestion.DomesticCupPenalty <= 0 || cfg.Congestion.DomesticCupPenalty > 1 {
		cfg.Congestion.DomesticCupPenalty = d.Congestion.DomesticCupPenalty
	}
	if len(cfg.Weights.MinutesWeightByPosition) == 0 {
		cfg.Weights.MinutesWeightByPosition = d.Weights.MinutesWeightByPosition
	}
	if cfg.Weights.BlendMinutesK <= 0 {
		cfg.Weights.BlendMinutesK = d.Weights.BlendMinutesK
	}
	// ⚠️ **This backfill is load-bearing in a way most are not.** JSON's zero
	// value is 0, and 0 here means "a player nobody has data on will not play" —
	// the defect this field exists to make measurable, not an off switch. Without
	// the backfill every config written before the field existed would silently
	// reinstate that bug on load. See Weights.UnknownPriorShare.
	if cfg.Weights.UnknownPriorShare <= 0 {
		cfg.Weights.UnknownPriorShare = d.Weights.UnknownPriorShare
	}
	if cfg.Weights.BlendRateK <= 0 {
		cfg.Weights.BlendRateK = d.Weights.BlendRateK
	}
	if cfg.Weights.LeagueShrinkK <= 0 {
		cfg.Weights.LeagueShrinkK = d.Weights.LeagueShrinkK
	}
	// ⚠️ No backfill for TournamentAbsences either, and for the same reason as the
	// campaign maps below: it is a hand-maintained ENUMERATION whose membership can
	// legitimately go to zero — a summer with no tournament — so an empty list is a
	// statement and only an absent key is an omission. `cfg` starts from Default(),
	// so omitting the key already keeps the six shipped groups.
	//
	// The `== nil` guard that used to be here made `"tournament_absences": null`
	// resurrect the default while `[]` emptied it, which is two answers to one
	// question and neither is documented.
	if cfg.RoleRisk.NewSigningPenalty <= 0 || cfg.RoleRisk.NewSigningPenalty > 1 {
		cfg.RoleRisk.NewSigningPenalty = d.RoleRisk.NewSigningPenalty
	}
	if cfg.RoleRisk.NewSigningGameweeks <= 0 {
		cfg.RoleRisk.NewSigningGameweeks = d.RoleRisk.NewSigningGameweeks
	}
	if cfg.RoleRisk.NewCoachPenalty <= 0 || cfg.RoleRisk.NewCoachPenalty > 1 {
		cfg.RoleRisk.NewCoachPenalty = d.RoleRisk.NewCoachPenalty
	}
	if cfg.RoleRisk.NewCoachGameweeks <= 0 {
		cfg.RoleRisk.NewCoachGameweeks = d.RoleRisk.NewCoachGameweeks
	}
	if cfg.Review.MinGainForTransfer <= 0 {
		cfg.Review.MinGainForTransfer = d.Review.MinGainForTransfer
	}
	if cfg.Review.MinGainForHit <= 0 {
		cfg.Review.MinGainForHit = d.Review.MinGainForHit
	}
	// Every numeric config field needs a backfill so existing config.json files stay
	// valid; this one arrived after they were written, so without it every deployed
	// config reads 0 and the band collapses to "nothing is equivalent to anything".
	if cfg.Review.MinSeparableGain <= 0 {
		cfg.Review.MinSeparableGain = d.Review.MinSeparableGain
	}
	if cfg.Review.FreeTransferValue <= 0 {
		cfg.Review.FreeTransferValue = d.Review.FreeTransferValue
	}
	// The early floor's off state is an explicit zero schedule, so an absent key
	// cannot be told from a deliberate off by value — probe presence the way
	// BonusPriorWeight does. An old file without the key gets the shipped
	// schedule, not the flat floor it never chose.
	if !hasKey(b, "review_policy", "early_floor") {
		cfg.Review.EarlyFloor = d.Review.EarlyFloor
	} else {
		// A written schedule is a choice, and it is still validated like the
		// flat constants are: a non-positive charge or gain bar would make the
		// gate accept anything through the schedule's window — the same failure
		// the flat value-guards above exist to refuse.
		if cfg.Review.EarlyFloor.FreeTransferValue <= 0 {
			cfg.Review.EarlyFloor.FreeTransferValue = d.Review.FreeTransferValue
		}
		if cfg.Review.EarlyFloor.MinGainForTransfer <= 0 {
			cfg.Review.EarlyFloor.MinGainForTransfer = d.Review.MinGainForTransfer
		}
		if cfg.Review.EarlyFloor.UntilGameweek < 0 {
			cfg.Review.EarlyFloor.UntilGameweek = 0
		}
	}
	if cfg.Review.BankUpTo <= 0 {
		cfg.Review.BankUpTo = d.Review.BankUpTo
	}
	// A ceiling of zero is not "no hits" — analysis.MoveLimit reads it as the
	// shipped default — so backfilling it changes nothing and records what is in
	// force. See ReviewPolicy.HitCeiling.
	if cfg.Review.HitCeiling <= 0 {
		cfg.Review.HitCeiling = d.Review.HitCeiling
	}
	// The option-value curve. Every one of these reads its zero as "the package
	// default", so a value-check backfill is safe and only records what is in
	// force.
	if cfg.OptionValue.Pricing.HalfLife <= 0 {
		cfg.OptionValue.Pricing.HalfLife = d.OptionValue.Pricing.HalfLife
	}
	if cfg.OptionValue.Pricing.CongestionSensitivity <= 0 {
		cfg.OptionValue.Pricing.CongestionSensitivity = d.OptionValue.Pricing.CongestionSensitivity
	}
	if cfg.OptionValue.Pricing.CongestionHorizon <= 0 {
		cfg.OptionValue.Pricing.CongestionHorizon = d.OptionValue.Pricing.CongestionHorizon
	}
	// ⚠️ The chip bars probe for KEY PRESENCE, not for the value. A bar of zero is
	// meaningful — "play it the first week it is worth anything at all" — so a
	// value-check migration would silently undo a deliberate 0, and it would never
	// fire anyway, since `cfg` starts from Default() and therefore already carries
	// the default. Same rule, same reason, as `bonus_prior_weight` above.
	for _, bar := range []struct {
		key  string
		into *float64
		from float64
	}{
		{"wildcard", &cfg.OptionValue.Wildcard.Bar, d.OptionValue.Wildcard.Bar},
		{"bench_boost", &cfg.OptionValue.BenchBoost.Bar, d.OptionValue.BenchBoost.Bar},
		{"free_hit", &cfg.OptionValue.FreeHit.Bar, d.OptionValue.FreeHit.Bar},
	} {
		if !hasKey(b, "option_value", bar.key, "bar_points") {
			*bar.into = bar.from
		}
	}
	if cfg.Review.LeadHours <= 0 {
		cfg.Review.LeadHours = d.Review.LeadHours
	}
	// BankTransfersLookahead defaulted ON on 2026-08-18, so the migration this
	// block's own predecessor comment predicted is now in force: a deliberate
	// `false` and an absent key are different facts. An old file without the
	// key keeps the behaviour it had (off); the shipped config.json writes the
	// key and gets the default. PrepareForChips still defaults off and still
	// needs nothing.
	if !hasKey(b, "review_policy", "bank_transfers_lookahead") {
		cfg.Review.BankTransfersLookahead = false
	}
	if len(cfg.Review.Rules) == 0 {
		cfg.Review.Rules = d.Review.Rules
	}
	if cfg.ReportDir == "" {
		cfg.ReportDir = d.ReportDir
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = d.CacheDir
	}
	if cfg.CacheMinutes <= 0 {
		cfg.CacheMinutes = d.CacheMinutes
	}
	if cfg.PlayerCacheMinutes <= 0 {
		cfg.PlayerCacheMinutes = d.PlayerCacheMinutes
	}
	backfillOverrideConfidence(b, &cfg)
	return cfg, nil
}

// legacyNailedOverrideFloor is the value analysis.nailedOverrideFloor used to
// carry, frozen here for exactly one purpose: reading a config.json written
// before RosterOverride.Confirmed existed. The runtime mechanism it came from
// is gone — inferring confidence from ExpectedMinutes' own magnitude was the
// reported bug (Tzolis, hedged at 82, read "nailed" because 82 cleared 80
// anyway) — so this constant must never again decide behaviour going
// forward. It exists solely to keep a pre-existing file's overrides reading
// exactly as they did the moment before this field shipped.
const legacyNailedOverrideFloor = 80.0

// backfillOverrideConfidence gives every minutes override that predates
// RosterOverride.Confirmed the same reading it had under the retired
// magnitude heuristic, so shipping the field does not silently flip anyone.
//
// This has to be a genuine one-time migration, not a permanent fallback —
// the whole point of Confirmed is that confidence is no longer inferred from
// a number. Two things make it actually one-time in practice:
//
//   - It only touches an override whose OWN JSON has no "confirmed" key at
//     all. Presence is probed on raw bytes, exactly like bonus_prior_weight
//     and rest_minutes_factor above, because cfg already starts from a zero
//     Confirmed and testing the parsed VALUE could never tell "omitted" from
//     "written as false" — which here are different facts.
//   - Confirmed carries no "omitempty" tag, so the moment ANY tool call
//     re-saves this config — which serialises the whole Roster, not just the
//     entry it touched — every override gets its own key written explicitly,
//     true or false. From that save onward hasKey sees it on every future
//     Load, and this function has nothing left to infer for that entry. A
//     newly written override that genuinely never addresses confidence keeps
//     reading false forever, which is the correct, permanent answer — only an
//     override old enough to predate the field at all gets the one historical
//     pass below.
//
// Verified against the deployed production config.json (the private ops
// repo's live account, not this repository's own dev config.json, which
// carries a different roster) by hand, override by override: Kinsky, van
// Ewijk and Mosquera read confidently in their own free text and cross the
// floor, so this keeps them "nailed" — the three the field exists to
// protect. Three others cross the floor despite each one's own reason
// explicitly hedging ("rather than a nailed 85") — this migration does NOT
// fix them, because doing so from the number alone would be the exact
// mechanism being retired. It reproduces today's reading for all six without
// exception; unhedging them requires an explicit "confirmed": false written
// to their entries by hand, same as any other override correction. See
// TestConfirmedBackfillMatchesTheLiveConfigOverrideByOverride for the shape
// this was checked against, reconstructed rather than pasted from the real
// file.
func backfillOverrideConfidence(raw []byte, cfg *Config) {
	var probe struct {
		Roster struct {
			Minutes []struct {
				Confirmed *bool `json:"confirmed"`
			} `json:"minutes"`
		} `json:"roster"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return
	}
	if len(probe.Roster.Minutes) != len(cfg.Roster.Minutes) {
		// Should not happen — same bytes, same shape — but an override this
		// cannot account for is left alone (Confirmed stays its Go zero,
		// false) rather than guessed at under a mismatched index.
		return
	}
	for i := range cfg.Roster.Minutes {
		if probe.Roster.Minutes[i].Confirmed != nil {
			continue // already explicit; not this function's to touch
		}
		o := &cfg.Roster.Minutes[i]
		if o.ExpectedMinutes != nil && *o.ExpectedMinutes >= legacyNailedOverrideFloor {
			o.Confirmed = true
		}
	}
}

func Save(path string, cfg Config) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// Written to a sibling and renamed over the target, rather than truncating
	// the live file. `os.WriteFile` opens O_TRUNC, so a crash, a SIGINT or a full
	// disk between the truncate and the write leaves a zero-length or half-written
	// config.json — taking the roster overrides and the review policy with it, and
	// those are the parts nothing else can reconstruct. Rename within a directory
	// is atomic, so a reader sees the old file or the new one and never a partial.
	//
	// The temporary file is a sibling deliberately: /tmp is frequently a different
	// filesystem, and rename across filesystems fails.
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
	// CreateTemp makes the file 0600; the config has always been 0644.
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// hasKey reports whether a nested key is present in the raw config JSON.
//
// Needed wherever a field's "unset" state is not its zero value. cfg starts
// from Default(), so testing the value cannot distinguish "absent" from
// "written at the default" — and for BonusPriorWeight the zero value is a real
// setting (ignore a purely historical bonus rate) while the disabled state is
// -1. Testing against zero would switch the schedule on for every existing
// config file. See the rest_minutes_factor migration for the same trap.
func hasKey(raw []byte, path ...string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	for i, k := range path {
		v, ok := m[k]
		if !ok {
			return false
		}
		if i == len(path)-1 {
			return true
		}
		m = nil
		if err := json.Unmarshal(v, &m); err != nil {
			return false
		}
	}
	return false
}
