package fpl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func liveBootstrap(t *testing.T) *Bootstrap {
	t.Helper()
	b, err := New(t.TempDir(), 24*time.Hour, 24*time.Hour).Bootstrap(context.Background())
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	return b
}

// TestBootstrapFetchedAtTracksTheDiskCacheAndTTL pins the two facts the News
// tab's FPL-status freshness line is built from: the disk cache file's own
// mtime is the fetch time, and CacheTTL reports the same window New was
// constructed with, so a caller can compute "next read at" without a second
// copy of the TTL.
func TestBootstrapFetchedAtTracksTheDiskCacheAndTTL(t *testing.T) {
	dir := t.TempDir()
	ttl := 45 * time.Minute
	c := New(dir, ttl, ttl)

	if got := c.BootstrapFetchedAt(); !got.IsZero() {
		t.Errorf("BootstrapFetchedAt = %v before anything was cached, want the zero time", got)
	}

	before := time.Now()
	if _, err := c.Bootstrap(context.Background()); err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	after := time.Now()

	fetched := c.BootstrapFetchedAt()
	if fetched.IsZero() {
		t.Fatal("BootstrapFetchedAt is still zero after a successful fetch")
	}
	if fetched.Before(before.Add(-time.Second)) || fetched.After(after.Add(time.Second)) {
		t.Errorf("BootstrapFetchedAt = %v, want between %v and %v", fetched, before, after)
	}
	if c.CacheTTL() != ttl {
		t.Errorf("CacheTTL = %v, want the %v passed to New", c.CacheTTL(), ttl)
	}
}

// TestElementByCodeFindsThePermanentIdentifier pins the lookup the News
// surface's before/after line depends on: config keys every standing
// correction by permanent code, never by the season-scoped element id, so
// resolving one to an *Element has to go through the code.
func TestElementByCodeFindsThePermanentIdentifier(t *testing.T) {
	boot := &Bootstrap{Elements: []Element{
		{ID: 1, Code: 111, WebName: "Kinsky"},
		{ID: 2, Code: 222, WebName: "Kadıoğlu"},
	}}
	if got := boot.ElementByCode(222); got == nil || got.WebName != "Kadıoğlu" {
		t.Errorf("ElementByCode(222) = %+v, want Kadıoğlu", got)
	}
	if got := boot.ElementByCode(999); got != nil {
		t.Errorf("ElementByCode(999) = %+v, want nil for a code nobody has", got)
	}
	if got := boot.ElementByCode(0); got != nil {
		t.Errorf("ElementByCode(0) = %+v, want nil — 0 is never a real player code", got)
	}
}

// FreeTransfers is pure, so unlike most tests in this repo it needs no network.
// Fixtures are built from JSON because EntryHistory's fields are anonymous
// structs — which also exercises the wire shape the API actually returns.
func TestFreeTransfersReconstruction(t *testing.T) {
	cases := []struct {
		name string
		json string
		want int
		why  string
	}{
		{
			name: "before the first deadline",
			json: `{"current":[],"past":[],"chips":[]}`,
			want: UnlimitedTransfers,
			why:  "the initial squad can still be changed freely; reporting 1 invites hoarding a constraint that does not exist",
		},
		{
			name: "one gameweek played, nothing spent",
			json: `{"current":[{"event":1,"event_transfers":0}],"chips":[]}`,
			want: 2,
			why:  "the unspent transfer banks and the next gameweek grants another",
		},
		{
			name: "spending every week leaves one",
			json: `{"current":[{"event":1,"event_transfers":1},{"event":2,"event_transfers":1},
			         {"event":3,"event_transfers":1}],"chips":[]}`,
			want: 1,
		},
		{
			name: "banking is capped",
			json: `{"current":[{"event":1,"event_transfers":0},{"event":2,"event_transfers":0},
			         {"event":3,"event_transfers":0},{"event":4,"event_transfers":0},
			         {"event":5,"event_transfers":0},{"event":6,"event_transfers":0},
			         {"event":7,"event_transfers":0},{"event":8,"event_transfers":0}],"chips":[]}`,
			want: MaxBankedTransfers,
			why:  "FPL stops banking at the cap however long you sit on them",
		},
		{
			name: "a wildcard week does not consume the allowance",
			json: `{"current":[{"event":1,"event_transfers":0},{"event":2,"event_transfers":12}],
			        "chips":[{"name":"wildcard","event":2}]}`,
			want: 3,
			why:  "twelve transfers under a wildcard are free and must not zero the balance",
		},
		{
			name: "a hit cannot push the balance negative",
			json: `{"current":[{"event":1,"event_transfers":4}],"chips":[]}`,
			want: 1,
			why:  "taking a -8 leaves you on the next gameweek's single transfer, not a deficit",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h EntryHistory
			if err := json.Unmarshal([]byte(tc.json), &h); err != nil {
				t.Fatalf("bad fixture: %v", err)
			}
			if got := FreeTransfers(&h); got != tc.want {
				t.Errorf("FreeTransfers = %d, want %d\n%s", got, tc.want, tc.why)
			}
		})
	}
}

// TestEarliestResultEvent pins the bound the gameweek rail's past-tab tabs use: the
// earliest gameweek the manager's OWN history has a result for, never a guess at the
// season's own first gameweek — a manager who joined FPL after gameweek one has no
// history before the gameweek they created their team.
func TestEarliestResultEvent(t *testing.T) {
	cases := []struct {
		name string
		json string
		want int
	}{
		{
			name: "no history at all",
			json: `{"current":[],"past":[],"chips":[]}`,
			want: 0,
		},
		{
			name: "history starts at the season's first gameweek",
			json: `{"current":[{"event":1},{"event":2},{"event":3}],"chips":[]}`,
			want: 1,
		},
		{
			name: "history starts mid-season -- this manager joined after GW1-4",
			json: `{"current":[{"event":5},{"event":6}],"chips":[]}`,
			want: 5,
		},
		{
			name: "out of order in the JSON does not change the minimum",
			json: `{"current":[{"event":7},{"event":5},{"event":6}],"chips":[]}`,
			want: 5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h EntryHistory
			if err := json.Unmarshal([]byte(tc.json), &h); err != nil {
				t.Fatalf("bad fixture: %v", err)
			}
			if got := EarliestResultEvent(&h); got != tc.want {
				t.Errorf("EarliestResultEvent = %d, want %d", got, tc.want)
			}
		})
	}
	if got := EarliestResultEvent(nil); got != 0 {
		t.Errorf("EarliestResultEvent(nil) = %d, want 0 — a failed read must not be treated as GW1", got)
	}
}

// A player's own display name must resolve to that player. The agent looks
// players up by name, so a mis-ranked match means it reasons about the wrong
// footballer — which happened live: "Rodri" returned Rodrigo Bentancur, because
// a WebName prefix hit and a first-name prefix hit scored the same and the
// points tiebreak preferred Bentancur.
func TestFindPlayersPrefersDisplayName(t *testing.T) {
	boot := liveBootstrap(t)

	// Invariant: searching any unique WebName returns that player first.
	seen := map[string]int{}
	for i := range boot.Elements {
		seen[strings.ToLower(boot.Elements[i].WebName)]++
	}
	var checked int
	for i := range boot.Elements {
		el := &boot.Elements[i]
		if seen[strings.ToLower(el.WebName)] != 1 {
			continue // genuinely ambiguous; nothing to assert
		}
		got := boot.FindPlayers(el.WebName)
		if len(got) == 0 {
			t.Errorf("%q (id %d) matches nothing", el.WebName, el.ID)
			continue
		}
		if got[0].ID != el.ID {
			t.Errorf("%q returned %q (id %d) first, want id %d",
				el.WebName, got[0].WebName, got[0].ID, el.ID)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no unique web names to check — the invariant was never exercised")
	}
	t.Logf("checked %d uniquely-named players", checked)
}

// A prefix of a player's display name must beat a prefix of someone else's
// first name. Stated as a property so it does not rot when the squad changes.
func TestFindPlayersDisplayNamePrefixBeatsForename(t *testing.T) {
	boot := liveBootstrap(t)

	for i := range boot.Elements {
		el := &boot.Elements[i]
		web := strings.ToLower(el.WebName)
		if len(web) < 5 {
			continue
		}
		q := web[:len(web)-1] // a strict prefix of this player's display name

		// Only meaningful when someone else matches the same query on forename.
		var rival *Element
		for j := range boot.Elements {
			o := &boot.Elements[j]
			if o.ID == el.ID || strings.HasPrefix(strings.ToLower(o.WebName), q) {
				continue
			}
			if strings.HasPrefix(strings.ToLower(o.FirstName+" "+o.SecondName), q) {
				rival = o
				break
			}
		}
		if rival == nil {
			continue
		}
		got := boot.FindPlayers(q)
		if len(got) > 0 && got[0].ID == rival.ID {
			t.Errorf("query %q returned %q (forename match) ahead of %q (display-name match)",
				q, rival.WebName, el.WebName)
		}
	}
}
