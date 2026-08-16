package main

import (
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

// The default has to stay chipless.
//
// Every season total this command has ever printed was measured without chips. If
// an unset switch started reading the config's plan, everyone with a plan saved
// would get a different number from the same command with nothing in the output
// saying so — which is the shape of this project's five recorded contamination
// events, where a change moved absolute totals silently.
func TestNoChipPlanIsTheDefault(t *testing.T) {
	t.Setenv("FPL_CHIP_PLAN", "")
	cfg := config.Config{Chips: analysis.ChipSchedule{First: analysis.ChipPlan{Wildcard: 6, BenchBoost: 8}}}
	got, _, err := chipPlanFromEnv(cfg, "2024-25")
	if err != nil {
		t.Fatal(err)
	}
	if got != (analysis.ChipPlan{}) {
		t.Errorf("an unset switch played %+v; the replay must stay chipless by default", got)
	}
}

func TestChipPlanReadsTheConfiguredPlanOnRequest(t *testing.T) {
	t.Setenv("FPL_CHIP_PLAN", "config")
	want := analysis.ChipPlan{Wildcard: 6, FreeHit: 16, BenchBoost: 8, TripleCaptain: 9}
	got, _, err := chipPlanFromEnv(config.Config{Chips: analysis.ChipSchedule{First: want}}, "2024-25")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestChipPlanParsesAnExplicitList(t *testing.T) {
	t.Setenv("FPL_CHIP_PLAN", "wildcard=6, bb=8 ,tc=9,free_hit=16")
	got, _, err := chipPlanFromEnv(config.Config{}, "2024-25")
	if err != nil {
		t.Fatal(err)
	}
	want := analysis.ChipPlan{Wildcard: 6, FreeHit: 16, BenchBoost: 8, TripleCaptain: 9}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// A mistyped chip must be an error, never a skip.
//
// This is the byte-identical-null failure in miniature: a plan that silently drops
// an unrecognised name returns a season indistinguishable from a chipless one, so
// the sweeper reads "the chip is worth nothing" when what happened is that it was
// never played. The standing rule is to check a setting is READ on the path being
// scored; refusing to parse is how this one enforces it.
func TestChipPlanRefusesWhatItCannotPlay(t *testing.T) {
	for _, spec := range []string{"bench_bost=8", "wildcard", "wildcard=0", "wildcard=39", "tc=x"} {
		t.Setenv("FPL_CHIP_PLAN", spec)
		if _, _, err := chipPlanFromEnv(config.Config{}, "2024-25"); err == nil {
			t.Errorf("%q was accepted; an unplayable plan must fail loudly", spec)
		}
	}
}

// A page headed "first 38 gameweeks" when 38 is the whole season reads as a
// truncation of something longer.
func TestReplayTitleSaysAllWhenItShowsEveryWeek(t *testing.T) {
	if got := replayTitle("2025-26", 38, 38); got != "Replay — 2025-26, all 38 gameweeks" {
		t.Errorf("a complete season is titled %q", got)
	}
	if got := replayTitle("2025-26", 10, 38); got != "Replay — 2025-26, first 10 gameweeks" {
		t.Errorf("a truncated replay is titled %q", got)
	}
	// A mid-season entry plays fewer than 38 and is still complete.
	if got := replayTitle("2025-26", 13, 13); got != "Replay — 2025-26, all 13 gameweeks" {
		t.Errorf("a late entry playing every week it had is titled %q", got)
	}
}

// The order is canonical — the order a season plays the chips out — and it
// changed here on 2026-08-13.
//
// This function carried its own four-name table, the fourth copy of the eight
// names, and it had already drifted: wildcard, bench boost, triple captain, free
// hit, against every other renderer's wildcard, free hit, bench boost, triple
// captain. So the replay page listed a plan in a different order from
// `armband chips`. It renders through `ChipSchedule.Entries` now, which is the
// single answer, and this expectation moves with it. Found by review — the drift
// the shared type exists to prevent, surviving inside the commit that added it.
func TestDescribeChipPlanNamesEveryChipItPlays(t *testing.T) {
	p := analysis.ChipPlan{Wildcard: 6, FreeHit: 16, BenchBoost: 8, TripleCaptain: 9}
	want := "wildcard GW6, free hit GW16, bench boost GW8, triple captain GW9"
	if got := describeChipPlan(p); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := describeChipPlan(analysis.ChipPlan{}); got != "none" {
		t.Errorf("an empty plan describes as %q", got)
	}
}
