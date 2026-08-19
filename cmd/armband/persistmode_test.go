package main

import (
	"net/http"
	"path/filepath"
	"testing"

	"armband/internal/config"
)

// TestUnderPersistABlockReachesTheFileAndTheSquad.
//
// `-persist` is documented as the durable mode: corrections go to config.json and bind every
// future run. What it actually did was change which store the document CLAIMED to be using.
// `effectiveCfgFrom` returned the raw config, throwing the session away, and `saveSession`
// never opened the file — the only config.Save on the serve path belongs to a form route
// nothing in the client posts to.
//
// So: the badge lit, the document reported `store: "config"`, the player stayed in the
// squad, and the file was untouched. The branch's own defect — a control that changes the
// badge and not the model — in the mode with the strongest claim to be durable.
//
// The test asserts BOTH halves, because either alone was true at some point in this
// branch's life: the correction must bind the build it was made on, and it must be in the
// file afterwards.
func TestUnderPersistABlockReachesTheFileAndTheSquad(t *testing.T) {
	s := fixtureServer(t)
	s.persist = true
	s.cfgPath = filepath.Join(t.TempDir(), "config.json")

	before := getWith(t, s, routeState, nil)
	if len(before.Squad.Players) != 15 {
		t.Fatalf("the opening squad has %d players", len(before.Squad.Players))
	}
	victim := before.Squad.Players[0].Code
	victimName := before.Squad.Players[0].Name

	w, after := put(t, s, session{Version: sessionVersion, Exclude: []int{victim}}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("the block answered %d: %s", w.Code, w.Body.String())
	}

	// One: it binds the build.
	for _, p := range after.Squad.Players {
		if p.Code == victim {
			t.Errorf("%s is still in the fifteen after being blocked under -persist. The "+
				"session was discarded and the correction bound nothing.", victimName)
		}
	}

	// Two: it is in the file, which is the whole point of the flag.
	saved, err := config.Load(s.cfgPath)
	if err != nil {
		t.Fatalf("-persist wrote no config at all: %v", err)
	}
	var found bool
	for _, o := range saved.Roster.Exclude {
		if o.Code == victim {
			found = true
			if o.Name == "" {
				t.Error("the saved override has no name, so the reader cannot tell who it " +
					"is when they open the file")
			}
			if o.SetOn == "" || o.LastChecked == "" {
				t.Errorf("the saved override carries no dates: %+v", o)
			}
		}
	}
	if !found {
		t.Errorf("config.json carries no exclusion for %s (code %d) after a block under "+
			"-persist. The document said the store was the config; the config disagrees. "+
			"Saved exclusions: %+v", victimName, victim, saved.Roster.Exclude)
	}
}

// TestUnderPersistTheCorrectionLivesInExactlyOneStore.
//
// Writing the file is only half of it. If the correction also stays in the cookie, two
// stores hold it — and that is how a dismissal comes back: the reader clears it from the
// file, the session still carries it, and the next build applies it again from the copy
// nobody was looking at.
func TestUnderPersistTheCorrectionLivesInExactlyOneStore(t *testing.T) {
	s := fixtureServer(t)
	s.persist = true
	s.cfgPath = filepath.Join(t.TempDir(), "config.json")

	before := getWith(t, s, routeState, nil)
	victim := before.Squad.Players[0].Code

	w, after := put(t, s, session{Version: sessionVersion, Exclude: []int{victim}}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("the block answered %d", w.Code)
	}
	if hasID(after.Session.Blocked, victim) {
		t.Errorf("the session still carries the block after it was written to the file. "+
			"Two stores hold one correction, and clearing the file will not clear it: %v",
			after.Session.Blocked)
	}
	if after.Session.Store != "config" {
		t.Errorf("the document reports the store as %q under -persist", after.Session.Store)
	}
}

// TestUnderPersistADismissalRemovesTheOverrideFromTheFile.
//
// A dismissal in session mode SUPPRESSES a config override, because a browser must not edit
// the standing record. Under -persist that reasoning inverts: editing the standing record is
// the entire purpose of the flag, so a dismissal there has to be a deletion. Without this,
// -persist would be a mode where a reader can add a correction and never remove one.
func TestUnderPersistADismissalRemovesTheOverrideFromTheFile(t *testing.T) {
	s := fixtureServer(t)
	s.persist = true
	s.cfgPath = filepath.Join(t.TempDir(), "config.json")

	before := getWith(t, s, routeState, nil)
	victim := before.Squad.Players[0].Code

	if w, _ := put(t, s, session{Version: sessionVersion, Exclude: []int{victim}}, nil); w.Code != http.StatusOK {
		t.Fatalf("the block answered %d", w.Code)
	}
	if w, _ := put(t, s, session{Version: sessionVersion, Dismissed: []int{victim}}, nil); w.Code != http.StatusOK {
		t.Fatalf("the dismissal answered %d", w.Code)
	}

	saved, err := config.Load(s.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range saved.Roster.Exclude {
		if o.Code == victim {
			t.Errorf("the exclusion is still in config.json after being cleared. Under "+
				"-persist a reader could add a correction and never remove one: %+v",
				saved.Roster.Exclude)
		}
	}
}
