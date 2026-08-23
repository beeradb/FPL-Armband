package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// TestWriteTheWildcardFixtures regenerates the chip_teams block on
// internal/webui/testdata/state/gameweek-one.json, and writes the sibling
// chip-unavailable.json, both from real GET /api/wildcard responses against
// the committed capture -- the same "realistic because it came from a real
// run" discipline TestWriteTheStateFixture documents for the rest of this
// file.
//
// Two clock pins over the same fixture server give the two states the
// /wildcard layout suite needs: the committed capture's own bootstrap opens
// the wildcard and free hit at gameweek 2 (analysis.PlayableChips), so at the
// fixture's ordinary pinned clock (before GW1's own deadline) both chips
// answer their real "not open yet" sentence, and pinned a week later --
// before GW2's deadline, after GW1's -- both answer a real rebuilt fifteen.
//
//	go test ./cmd/armband/ -run TestWriteTheWildcardFixtures -update
func TestWriteTheWildcardFixtures(t *testing.T) {
	if !*updateGoldens {
		t.Skip("generator; run with -update to rewrite the wildcard fixtures")
	}

	base := readBaseStateBytesForWildcardFixture(t)

	// chip-unavailable: the fixture's own ordinary clock, before GW1's deadline.
	// GW1 is state 2 for both chips in the committed capture.
	unavailable := fixtureServer(t)
	ct := decodeChipTeams(t, getChipTeams(t, unavailable, nil))
	writeWildcardFixture(t, base, ct, "chip-unavailable")

	// gameweek-one's own chip_teams: the same clock pinned to gameweek 2, where
	// the capture's bootstrap opens both chips, so this is a genuine rebuild
	// from the real optimiser over the real capture.
	available := fixtureServer(t)
	available.clock = func() time.Time { return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC) }
	ct = decodeChipTeams(t, getChipTeams(t, available, nil))

	// ⚠️ Changes/Out/KeptIDs are synthetic, patched in here rather than genuine
	// -- fixtureServer's cfg.EntryID is 0 (see its own comment) and s.client is
	// nil, so houseRealPicks has no "today's squad" to compare the rebuild
	// against and the real handler answers these three fields at their honest
	// zero value. Exercising that comparison for real would need a *fpl.Client
	// backed by a seeded disk cache carrying a Picks response, which is more
	// machinery than a layout fixture earns; everything else on ct (formation,
	// XI, bench, scores, prices) is the real optimiser's own output and
	// untouched. This is here ONLY so the OUT list and the "9/15 change"
	// strip -- the page's own headline -- have something to screenshot; the
	// numbers are not a claim about GW2.
	patchSyntheticChanges(available.engine.Boot, ct.Wildcard, 6, 9)
	patchSyntheticChanges(available.engine.Boot, ct.FreeHit, 3, 12)

	writeWildcardFixture(t, base, ct, "gameweek-one")
}

// patchSyntheticChanges sets Changes/Out/KeptIDs on a generated ChipTeam for
// fixture purposes only -- see TestWriteTheWildcardFixtures' own comment.
// keepN of the rebuild's own XI+bench are marked kept; outN real web names,
// not present in the rebuild, are borrowed from the bootstrap for the OUT
// list.
func patchSyntheticChanges(boot *fpl.Bootstrap, team *viewmodel.ChipTeam, keepN, outN int) {
	if team == nil {
		return
	}
	var ids []int
	for _, p := range team.XI {
		ids = append(ids, p.ID)
	}
	for _, p := range team.Bench {
		ids = append(ids, p.ID)
	}
	inRebuild := make(map[int]bool, len(ids))
	for _, id := range ids {
		inRebuild[id] = true
	}
	var names []string
	for i := range boot.Elements {
		el := &boot.Elements[i]
		if inRebuild[el.ID] {
			continue
		}
		names = append(names, el.WebName)
		if len(names) >= outN {
			break
		}
	}
	if keepN > len(ids) {
		keepN = len(ids)
	}
	team.Changes = len(ids) - keepN
	team.Out = names
	team.KeptIDs = append([]int(nil), ids[:keepN]...)
}

// readBaseStateBytesForWildcardFixture reads the currently-committed
// gameweek-one.json as raw bytes, deliberately NOT decoded into
// viewmodel.State and re-marshaled: a decode/re-encode round trip reformats
// every float in the file (encoding/json's shortest-round-trip float
// formatter does not always reproduce the exact text a value was first
// written with, e.g. exponential vs plain notation for a number near 1e-6),
// which would touch hundreds of unrelated lines for a field this change never
// reads. writeWildcardFixture instead splices the new field into these bytes
// textually, so every byte this change did not intend to touch does not move.
func readBaseStateBytesForWildcardFixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "internal", "webui", "testdata", "state", "gameweek-one.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the base state fixture: %v", err)
	}
	// Decoded once, and discarded, purely to fail loudly if the committed file
	// no longer parses -- the splice below does not otherwise notice.
	var probe viewmodel.State
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("the base state fixture no longer decodes: %v", err)
	}
	return raw
}

func decodeChipTeams(t *testing.T, w *httptest.ResponseRecorder) *viewmodel.ChipTeams {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("GET /api/wildcard answered %d, want 200: %s", w.Code, w.Body.String())
	}
	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("GET /api/wildcard did not decode: %v\n%s", err, w.Body.Bytes())
	}
	if st.ChipTeams == nil {
		t.Fatal("GET /api/wildcard answered with no chip_teams")
	}
	return st.ChipTeams
}

// writeWildcardFixture splices "chip_teams" onto the base document's TOP
// LEVEL as one more field, textually -- see readBaseStateBytesForWildcardFixture
// for why not by decode/re-encode. base must end in the two-space-indented
// "}\n}\n" json.MarshalIndent(_, "", "  ") always produces for this contract;
// the splice inserts before the outer brace and re-indents the new value's
// own lines to the same top-level convention (a MarshalIndent prefix of two
// spaces, matching every other top-level field's nested content).
func writeWildcardFixture(t *testing.T, base []byte, ct *viewmodel.ChipTeams, name string) {
	t.Helper()
	// Strip exactly the ROOT closing brace and the newline before it, keeping
	// everything else -- including the closing brace of whatever the last
	// existing top-level field is ("results", today) -- so the splice only
	// ever ADDS bytes, never re-shapes any that were already there.
	trimmed := bytes.TrimRight(base, "\n")
	if !bytes.HasSuffix(trimmed, []byte("}")) {
		t.Fatalf("%s does not end in \"}\" -- refusing to splice blind", name)
	}
	trimmed = bytes.TrimRight(trimmed[:len(trimmed)-1], "\n")
	if !bytes.HasSuffix(trimmed, []byte("}")) {
		t.Fatalf("%s's second-to-last top-level field does not close with \"}\" -- "+
			"refusing to splice blind", name)
	}
	ctJSON, err := json.MarshalIndent(ct, "  ", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	out.Write(bytes.TrimRight(trimmed, "\n"))
	out.WriteString(",\n  \"chip_teams\": ")
	out.Write(ctJSON)
	out.WriteString("\n}\n")

	// Fails loudly here, not silently in the browser, if the splice produced
	// something the contract cannot read back.
	var check viewmodel.State
	if err := json.Unmarshal(out.Bytes(), &check); err != nil {
		t.Fatalf("%s: spliced document no longer decodes: %v", name, err)
	}
	if check.ChipTeams == nil {
		t.Fatalf("%s: spliced document decoded with no chip_teams", name)
	}

	path := filepath.Join("..", "..", "internal", "webui", "testdata", "state", name+".json")
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d bytes)", path, out.Len())
}
