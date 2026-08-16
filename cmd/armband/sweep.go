package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"armband/internal/config"
)

// applyWeightOverrides lets a sweep reach the scoring weights without editing
// config, as FPL_WEIGHT="bonus=0.5,fixture=0.8,minutes_half_life=8".
//
// It is applied to the loaded config before anything is built from it, so the
// engine, the replay's opening optimise and its per-gameweek engines all see
// the same value. Reaching only one of them is a real hazard here — a patch
// once wired recency into two of the three engines Simulate builds and the
// whole measured gain came from the one it missed.
//
// Diagnostic only. Every key defaults to whatever config says.
func applyWeightOverrides(cfg *config.Config) {
	raw := strings.TrimSpace(os.Getenv("FPL_WEIGHT"))
	if raw == "" {
		return
	}
	var applied []string
	for _, part := range strings.Split(raw, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		v, err := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64)
		if err != nil {
			continue
		}
		switch key {
		case "bonus":
			cfg.Weights.BonusWeight = v
		case "defcon_clean":
			cfg.Weights.DefConCleanCoupling = v
		case "bonus_prior":
			cfg.Weights.BonusPriorWeight = v
		case "fixture":
			cfg.Weights.FixtureWeight = v
		case "rate_half_life":
			cfg.Weights.RateHalfLife = v
		case "minutes_half_life":
			cfg.Weights.MinutesHalfLife = v
		case "prior_half_life":
			cfg.Weights.PriorHalfLife = v
			// This one key does not reach a replay, and it looks exactly like the
			// keys that do. Nothing in internal/analysis reads
			// Weights.PriorHalfLife — grep it: there is a declaration, two
			// comments and no consumer. Its only reader is recent.LoadPriors on
			// the live path, which needs element-summary's history_past. So
			// `armband backtest` applies this override and then builds an engine
			// with no multi-season prior index attached, and the run is
			// byte-identical to not having set it.
			//
			// The replay proper reads a DIFFERENT field, SimConfig.PriorHalfLife,
			// which is honoured only when SimConfig.OlderPriors is populated —
			// and nothing outside cmd/priorblend and one test populates it.
			//
			// Two same-named knobs, one of them unread by the engine, is this
			// project's signature failure. Say so rather than returning a clean
			// null. TestPriorHalfLifeSaysItCannotReachTheReplay pins the warning.
			fmt.Fprintf(os.Stderr, "FPL_WEIGHT: prior_half_life reaches the LIVE "+
				"blend only (recent.LoadPriors). It is inert on `armband "+
				"backtest`, and the replay reads SimConfig.PriorHalfLife with "+
				"SimConfig.OlderPriors, which this cannot set.\n")
		case "band":
			cfg.Weights.BandStrength = v
		case "horizon":
			cfg.Weights.Horizon = int(v)
		case "free_transfer_value":
			cfg.Review.FreeTransferValue = v
		case "min_gain":
			cfg.Review.MinGainForTransfer = v
		case "bench":
			cfg.Weights.BenchWeight = v
		default:
			continue
		}
		applied = append(applied, key+"="+kv[1])
	}
	if len(applied) > 0 {
		fmt.Fprintf(os.Stderr, "FPL_WEIGHT: %s\n", strings.Join(applied, " "))
	}
}
