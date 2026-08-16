package capture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// captureRoot is the repository's captures directory, from this package's directory.
const captureRoot = "../../data/captures"

// TestEveryStoredCaptureIsPointInTimeHonest is the single most important test in the
// backfill, and it deliberately runs in the ordinary suite rather than behind DIAG.
//
// # What it proves
//
// A backfilled capture claims to be evidence about the moment a manager decided.
// That claim is worth nothing unless the bytes predate the deadline they are filed
// under — a crawl from an hour *after* a deadline carries the confirmed team sheet,
// and any figure derived from it would be excellent, plausible, and hindsight. The
// failure is silent by construction: nothing downstream can tell a leaked row from
// an honest one.
//
// So this walks every capture on disk and re-derives the honesty from the stored
// bytes, rather than trusting the manifest that was written beside them. The manifest
// is a claim; the payload's own `events[]` is the evidence, and `VerifyPreDeadline`
// interrogates the payload about its own timing without reference to any external
// clock. The two are then checked against each other, so a manifest that drifted from
// its bytes fails here rather than being believed.
//
// # Why it is not DIAG-gated
//
// Every network-dependent check in this project is gated and skips when the source is
// unreachable. This one reads only the filesystem, so gating it would mean the
// property most worth guarding is the one least often checked — and the whole point
// of storing a proof-carrying manifest is that the proof gets re-run for free.
func TestEveryStoredCaptureIsPointInTimeHonest(t *testing.T) {
	store, err := Open(captureRoot)
	if err != nil {
		t.Fatalf("opening %s: %v", captureRoot, err)
	}
	manifests := store.Manifests()
	if len(manifests) == 0 {
		t.Skip("no captures on disk yet")
	}

	checked, backfilled := 0, 0
	for dir, m := range manifests {
		checked++
		body, err := Read(dir, BootstrapEndpoint)
		if err != nil {
			t.Errorf("%s: reading the stored payload: %v", dir, err)
			continue
		}

		// The stored bytes must be the bytes the manifest describes. A capture is
		// evidence, and evidence whose hash does not match its record is not
		// evidence — it is a file that happens to sit in the right directory.
		for _, f := range m.Files {
			if f.Endpoint != BootstrapEndpoint || f.SHA256 == "" {
				continue
			}
			sum := sha256.Sum256(body)
			if got := hex.EncodeToString(sum[:]); got != f.SHA256 {
				t.Errorf("%s: stored payload hashes to %s, manifest says %s", dir, got[:12], f.SHA256[:12])
			}
			if f.Bytes != 0 && f.Bytes != len(body) {
				t.Errorf("%s: stored payload is %d bytes, manifest says %d", dir, len(body), f.Bytes)
			}
		}

		if m.Backfill == nil {
			// A live capture may legitimately be taken after a deadline — the
			// series runs on a timer, not on the fixture list. What it may not do
			// is describe a deadline its own payload disagrees with.
			checkManifestMatchesPayload(t, dir, m, body)
			continue
		}
		backfilled++

		if m.Backfill.Season == "" || m.Backfill.Source == "" || m.Backfill.WaybackTimestamp == "" {
			t.Errorf("%s: backfilled capture with incomplete provenance %+v", dir, m.Backfill)
		}

		// The proof, from the payload alone.
		q, err := VerifyPreDeadline(body, m.Event, m.CapturedAt)
		if err != nil {
			t.Errorf("%s: LEAK — %v", dir, err)
			continue
		}

		// And the manifest must agree with it. These are two routes to one
		// quantity and this project's signature failure is exactly that shape:
		// one quantity, two implementations, and the measured one is not the one
		// that runs.
		if m.EventDeadline == nil {
			t.Errorf("%s: backfilled capture records no deadline", dir)
			continue
		}
		if !m.EventDeadline.Equal(q.Deadline) {
			t.Errorf("%s: manifest deadline %s but the payload says %s",
				dir, m.EventDeadline.Format(time.RFC3339), q.Deadline.Format(time.RFC3339))
		}
		if m.HoursToDeadline == nil {
			t.Errorf("%s: backfilled capture records no distance to its deadline", dir)
			continue
		}
		if *m.HoursToDeadline <= 0 {
			t.Errorf("%s: LEAK — manifest records %.3f h to the deadline; a backfilled "+
				"capture must be strictly before it", dir, *m.HoursToDeadline)
		}
		if diff := *m.HoursToDeadline - q.HoursBefore; diff > 1e-6 || diff < -1e-6 {
			t.Errorf("%s: manifest says %.6f h before the deadline, the payload implies %.6f",
				dir, *m.HoursToDeadline, q.HoursBefore)
		}
	}
	t.Logf("checked %d capture(s), of which %d backfilled", checked, backfilled)
}

// checkManifestMatchesPayload holds a live capture to the one property that is not
// about hindsight: it must not describe a gameweek its own payload contradicts.
func checkManifestMatchesPayload(t *testing.T, dir string, m *Manifest, body []byte) {
	t.Helper()
	if m.Event == 0 || m.EventDeadline == nil {
		return
	}
	events, err := ParseEvents(body)
	if err != nil {
		t.Errorf("%s: %v", dir, err)
		return
	}
	for _, e := range events {
		if e.ID != m.Event {
			continue
		}
		if !e.DeadlineTime.UTC().Equal(m.EventDeadline.UTC()) {
			t.Errorf("%s: manifest puts the GW%d deadline at %s, the payload at %s",
				dir, m.Event, m.EventDeadline.Format(time.RFC3339),
				e.DeadlineTime.Format(time.RFC3339))
		}
		return
	}
	t.Errorf("%s: manifest claims GW%d but the payload's events[] has no such gameweek", dir, m.Event)
}

// TestNoBackfilledSeasonSilentlyClaimsFullCoverage guards the other direction: not
// that a stored row is dishonest, but that a season with holes reads as one.
//
// Coverage genuinely is patchy — the Archive crawled FPL about two days in three in
// some seasons — and the danger is not the holes, it is a consumer that cannot see
// them. A gap that is visible is a gap somebody can reason about; a gap that has been
// filled with the nearest available crawl is a measurement of hindsight. So the
// store's own coverage view must report an absent gameweek as absent, and `At` must
// refuse it rather than return an empty availability that reads as "nobody injured".
func TestNoBackfilledSeasonSilentlyClaimsFullCoverage(t *testing.T) {
	store, err := Open(captureRoot)
	if err != nil {
		t.Fatalf("opening %s: %v", captureRoot, err)
	}
	seasons := store.Seasons()
	if len(seasons) == 0 {
		t.Skip("no backfilled seasons on disk yet")
	}
	for _, season := range seasons {
		covered := 0
		for gw := 1; gw <= 38; gw++ {
			if store.Count(season, gw) > 0 {
				covered++
				continue
			}
			// The contract for a gap: asking for it is an error, never an empty
			// answer that a caller could mistake for a clean gameweek.
			if _, err := store.At(season, gw); err == nil {
				t.Errorf("%s GW%d has no capture but At() returned data", season, gw)
			}
		}
		t.Logf("%s: %d of 38 gameweeks covered", season, covered)
	}
}

// TestTheStoreIndexesBothLayouts pins that one reader serves the live series and the
// backfilled seasons, which is the reason the backfill writes into the capture layout
// at all rather than inventing its own.
func TestTheStoreIndexesBothLayouts(t *testing.T) {
	root := t.TempDir()

	live := filepath.Join(root, DirName(time.Date(2026, 8, 10, 5, 28, 0, 0, time.UTC)))
	writeCapture(t, live, boot(t, 3, "2026-08-21T17:30:00Z", 3), &Manifest{
		CapturedAt: time.Date(2026, 8, 10, 5, 28, 0, 0, time.UTC), Event: 3,
	})

	at := time.Date(2020, 10, 2, 9, 0, 0, 0, time.UTC)
	deadline := time.Date(2020, 10, 3, 10, 0, 0, 0, time.UTC)
	hours := deadline.Sub(at).Hours()
	back := filepath.Join(SeasonDir(root, "2020-21"), BackfillDirName(4, at))
	writeCapture(t, back, boot(t, 4, "2020-10-03T10:00:00Z", 4), &Manifest{
		CapturedAt: at, Event: 4, EventDeadline: &deadline, HoursToDeadline: &hours,
		Backfill: &Backfill{Season: "2020-21", Source: "u", WaybackTimestamp: "20201002090000"},
	})

	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.At("", 3); err != nil {
		t.Errorf("the live capture was not indexed: %v", err)
	}
	a, err := store.At("2020-21", 4)
	if err != nil {
		t.Fatalf("the backfilled capture was not indexed: %v", err)
	}
	if !a.Backfilled {
		t.Error("a backfilled capture read back as a live one, so a consumer cannot tell " +
			"our own fetch from a third party's crawl")
	}
	if got := store.Seasons(); len(got) != 1 || got[0] != "2020-21" {
		t.Errorf("Seasons() = %v, want [2020-21]", got)
	}
}

// writeCapture lays out a capture directory by hand, so the reader is tested against
// the format rather than only against its own writer.
func writeCapture(t *testing.T, dir string, body []byte, m *Manifest) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writeGz(filepath.Join(dir, FileName(BootstrapEndpoint)), body); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	m.Files = []File{{Endpoint: BootstrapEndpoint, Name: FileName(BootstrapEndpoint),
		Bytes: len(body), SHA256: hex.EncodeToString(sum[:])}}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
