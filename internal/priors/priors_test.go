package priors

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// cacheDir is the project cache. A finished season is immutable, so reusing it
// across runs is safe and avoids re-downloading ten megabytes per test.
const cacheDir = "../../.cache/fpl"

const priorSeason = "2025-2026"

func load(t *testing.T) *Season {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	s, err := Load(ctx, cacheDir, priorSeason)
	if err != nil {
		t.Skipf("prior season unavailable: %v", err)
	}
	return s
}

func TestPriorSeasonLoads(t *testing.T) {
	s := load(t)
	if len(s.Players) < 400 {
		t.Errorf("only %d players in %s; a Premier League season has 500+", len(s.Players), s.Name)
	}

	var withMinutes, full int
	for _, p := range s.Players {
		if p.Minutes > 0 {
			withMinutes++
		}
		if p.Gameweeks >= 38 {
			full++
		}
	}
	if withMinutes < 300 {
		t.Errorf("only %d players have minutes", withMinutes)
	}
	if full < 300 {
		t.Errorf("only %d players have a full 38 gameweek snapshots; the season may be truncated", full)
	}
	t.Logf("%s: %d players, %d with minutes, %d with 38 snapshots", s.Name, len(s.Players), withMinutes, full)
}

// TestPriorMatchesTheLiveAPI is the validation that only works before GW1.
//
// FPL carries last season's totals until the first gameweek completes, so until
// then the prior and the live API describe the same thing and must agree. Once
// the season starts FPL overwrites its aggregates and the comparison becomes
// meaningless — which is the entire reason this package exists.
//
// It also proves the join. Matching is by player_code, FPL's permanent
// identifier, never by name; if that were wrong the disagreement count would be
// enormous rather than a handful.
func TestPriorMatchesTheLiveAPI(t *testing.T) {
	b, err := os.ReadFile(cacheDir + "/bootstrap-static.json")
	if err != nil {
		t.Skipf("no cached bootstrap to compare against: %v", err)
	}
	var boot struct {
		Events []struct {
			Finished bool `json:"finished"`
		} `json:"events"`
		Elements []struct {
			Code    int    `json:"code"`
			WebName string `json:"web_name"`
			Minutes int    `json:"minutes"`
			Goals   int    `json:"goals_scored"`
			Starts  int    `json:"starts"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(b, &boot); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	for _, e := range boot.Events {
		if e.Finished {
			t.Skip("season has started; FPL has overwritten last season's totals so the two " +
				"sources no longer describe the same thing")
		}
	}

	s := load(t)
	var matched, disagreed, unmatchedWithMinutes int
	for _, e := range boot.Elements {
		p, ok := s.Get(e.Code)
		if !ok {
			if e.Minutes > 0 {
				unmatchedWithMinutes++
				t.Errorf("%s has %d minutes in the live API but no prior; the code join is "+
					"incomplete", e.WebName, e.Minutes)
			}
			continue
		}
		matched++
		if p.Minutes != e.Minutes || p.Goals != e.Goals || p.Starts != e.Starts {
			disagreed++
		}
	}

	if matched < 300 {
		t.Fatalf("only %d players matched by code; the join is broken", matched)
	}
	// A few disagreements are expected and are the prior doing its job: FPL
	// zeroes a player's record when he returns to the league under a fresh
	// entry, while the prior still holds the season he actually played.
	if rate := float64(disagreed) / float64(matched); rate > 0.05 {
		t.Errorf("%d of %d matched players disagree with the live API (%.1f%%); above a few "+
			"percent this is a join or parsing fault, not FPL housekeeping",
			disagreed, matched, rate*100)
	}
	t.Logf("matched %d by code, %d disagreements (%.1f%%), %d unmatched with minutes",
		matched, disagreed, float64(disagreed)/float64(matched)*100, unmatchedWithMinutes)
}

// TestPriorTotalsAreCumulative guards the assumption the loader rests on: each
// gameweek row is a running season total, so the last row is the season, not
// that week's return.
func TestPriorTotalsAreCumulative(t *testing.T) {
	s := load(t)
	var checked int
	for _, p := range s.Players {
		if p.Gameweeks < 30 || p.Minutes == 0 {
			continue
		}
		checked++
		// A season total across 30+ appearances cannot look like one match.
		if p.Minutes < 200 && p.Starts > 5 {
			t.Errorf("%s: %d minutes across %d starts and %d snapshots — the loader is keeping "+
				"a single gameweek rather than the cumulative total",
				p.WebName, p.Minutes, p.Starts, p.Gameweeks)
		}
		if p.Starts > p.Gameweeks {
			t.Errorf("%s has %d starts from %d snapshots", p.WebName, p.Starts, p.Gameweeks)
		}
	}
	if checked == 0 {
		t.Skip("no near-ever-present players")
	}
}

// TestPriorIsCachedIndefinitely — a completed season does not change, so once
// written the cache is authoritative and the network is never touched again.
// That is what makes a single-maintainer upstream an acceptable dependency.
func TestPriorIsCachedIndefinitely(t *testing.T) {
	load(t) // ensure it is populated
	path := cacheDir + "/priors-" + priorSeason + ".json"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("cache not written: %v", err)
	}
	old := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Skipf("cannot age the cache file: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Load(ctx, cacheDir, priorSeason); err != nil {
		t.Errorf("a year-old cache was not reused: %v", err)
	}
}
