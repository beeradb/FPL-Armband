package capture

import (
	"encoding/json"
	"fmt"

	"armband/internal/fpl"
)

// FixturesEndpoint is the second endpoint a LIVE capture carries. Backfilled captures have
// only the bootstrap — see BootstrapEndpoint — so a caller that needs fixtures must use a
// capture from the live series.
const FixturesEndpoint = "/fixtures/"

// LiveCapture names the committed capture that deterministic tests are built on, under
// data/captures. It is here rather than in a test file because two packages need the same
// answer, and two spellings of "which fixture" is two fixtures.
//
// It is pinned to GW1 of 2026-27 deliberately rather than incidentally. At GW1 the engine
// has no prior-season blend and no recency index to fetch — GameweeksPlayed() is zero, so
// the live binary skips both — which means an engine built from this capture alone is what
// production actually produces at this point in the season, not an approximation of it.
// Pin a mid-season capture instead and the nil Priors and Recent would silently return
// fallback values: populated, plausible, and different numbers.
//
// It is also the state the design was drawn against, GW1 opening Friday 21 August at 17:30,
// which is what lets a screenshot be compared with the handoff at all.
const LiveCapture = "2026-08-19T1348Z"

// Replay decodes a stored capture back into the two payloads the engine is built from.
//
// # Why this exists
//
// A capture is the bytes FPL served at a moment, kept so point-in-time questions stay
// answerable later. Until now the only reader over it was Store, which unmarshals into a
// player-status index and throws the rest away — so the archive held a complete bootstrap
// that nothing could turn back into one.
//
// That is what a deterministic test needs. The live path fetches from the API and gets a
// different answer every week; a test built on it either skips when the network is down or
// asserts something that rots. Reading a committed capture gives byte-identical inputs on
// every run, on any machine, with no network at all.
//
// # Season is deliberately left empty
//
// Empty means the live game, which analysis.ScoringRulesFor turns into today's scoring
// rules. That is correct for a capture from the current season and would be wrong for a
// fabricated historical bootstrap — but this reads real captured bytes, and the season it
// was captured from is the season whose rules were in force when FPL served them. The
// replay's own fabricated bootstraps set Season explicitly and are built elsewhere.
//
// # What this does not do
//
// It does not verify the manifest's hashes. Read already fails loudly on a truncated or
// corrupt gzip stream, and the manifest is checked where it is written. A caller wanting
// the stronger guarantee should compare against the manifest itself.
func Replay(dir string) (*fpl.Bootstrap, []fpl.Fixture, error) {
	raw, err := Read(dir, BootstrapEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the captured bootstrap from %s: %w", dir, err)
	}
	var boot fpl.Bootstrap
	if err := json.Unmarshal(raw, &boot); err != nil {
		return nil, nil, fmt.Errorf("decoding the captured bootstrap from %s: %w", dir, err)
	}
	if len(boot.Elements) == 0 {
		return nil, nil, fmt.Errorf("the capture at %s decoded to a bootstrap with no "+
			"players, which is not a bootstrap", dir)
	}

	raw, err = Read(dir, FixturesEndpoint)
	if err != nil {
		// A backfilled capture carries no fixtures. That is a real and expected
		// shape, so it is named rather than returned as a bare file-not-found.
		return nil, nil, fmt.Errorf("reading the captured fixtures from %s: %w "+
			"(backfilled captures carry only the bootstrap — use one from the live "+
			"series if you need fixtures)", dir, err)
	}
	var fixtures []fpl.Fixture
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		return nil, nil, fmt.Errorf("decoding the captured fixtures from %s: %w", dir, err)
	}
	return &boot, fixtures, nil
}
