package snapshot

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestBankedProvenanceCarriesNoAbsolutePath refuses an absolute filesystem path
// in any committed provenance sidecar.
//
// ⚠️ **A sidecar is machine-written and committed to a PUBLIC repository.** An
// absolute path names the machine a figure was measured on, and where the path
// points at a data source the repository may not distribute, it names that too —
// so a banked cells file publishes both the host layout and the source, with
// nobody having decided to publish either.
//
// That happened. Three sidecars banked on 2026-08-25 carried an absolute path
// into `origin/main` and were only found the next day, by a scan run for a
// different reason. `envPathValued` in fingerprint.go is the fix at the source:
// a path-valued switch is recorded as `path:<digest>`, which keeps everything the
// fingerprint is for — two runs against different directories still differ, and
// `stats/sweep_inference.R` compares those values for inequality and never reads
// them — while losing only the ability to say WHICH directory, which is the part
// that must not be committed.
//
// This guard is the backstop for that fix. It is deliberately mechanical and
// brand-free: it matches the SHAPE of an absolute path rather than any particular
// directory, because a guard naming the thing it is protecting would publish it.
//
// ⚠️ **It cannot see prose.** Two READMEs named the source in hand-written text on
// the same day, from the same session, and no mechanical check would have caught
// them. That half is a review matter.
func TestBankedProvenanceCarriesNoAbsolutePath(t *testing.T) {
	// Absolute POSIX paths with at least two segments, and Windows drive paths.
	// One segment ("/tmp") is not enough to identify a machine and appears in
	// legitimate documented commands.
	abs := regexp.MustCompile(`(^|[\s,="'` + "`" + `])(/[A-Za-z0-9._-]+){2,}/?|[A-Za-z]:\\`)
	var offenders []string
	err := filepath.Walk("../../stats", func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".provenance.csv") {
			return err
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(b), "\n") {
			if m := abs.FindString(line); m != "" {
				offenders = append(offenders, p+":"+itoaLine(i+1)+" "+strings.TrimSpace(m))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("committed provenance sidecars carry absolute filesystem paths, which name the "+
			"machine a figure was measured on and can name a source this repository may not "+
			"distribute:\n  %s\n\nFingerprint the switch as a path digest instead — see "+
			"envPathValued in fingerprint.go — and scrub the banked files.",
			strings.Join(uniq(offenders), "\n  "))
	}
}

func itoaLine(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// TestPathFingerprintIsStableAndOpaque pins the properties a banked sidecar
// depends on: the value is marked as a digest, names nothing about this host, is
// deterministic, and still tells two different sources apart.
//
// The inputs are unreadable paths, which exercises the UNREADABLE branch. The
// leak assertion matters most there — an os error carries the path it failed on,
// so that branch is where a path would re-enter a public file.
func TestPathFingerprintIsStableAndOpaque(t *testing.T) {
	got := pathFingerprint("/a/b/c")
	if !strings.HasPrefix(got, "unreadable:") {
		t.Fatalf("pathFingerprint(%q) = %q, want an unreadable: prefix so a reader knows both that "+
			"it is a digest and that the source could not be read", "/a/b/c", got)
	}
	// ⚠️ Never the legacy tag. `path:` means "hash of the path string" and 12
	// banked sidecars carry it; reusing it for anything computed differently
	// would make an incomparability look like a difference in the data.
	if strings.HasPrefix(got, "path:") {
		t.Fatalf("pathFingerprint reused the legacy path: tag for a value computed another way: %q", got)
	}
	if strings.Contains(got, "/") {
		t.Fatalf("pathFingerprint leaked a separator: %q", got)
	}
	if same := pathFingerprint("/a/b/c"); same != got {
		t.Fatalf("pathFingerprint is not deterministic: %q then %q", got, same)
	}
	if other := pathFingerprint("/a/b/d"); other == got {
		t.Fatal("pathFingerprint collides on different paths, so two arms against different " +
			"sources would compare as one — the discrimination is the whole point of keeping it")
	}
}

// TestPathFingerprintMovesWhenContentsMoveAtAFixedPath is the bite test for the
// fourth comparability failure, and it is written to FAIL against the scheme it
// replaced.
//
// The path string is held FIXED and only the bytes under it change. A digest of
// the path string — what this function used to compute — returns the same value
// for both, so a sidecar recorded the two runs as identical: `commit` matched,
// `WatchedDigest` matched, and the data having moved was invisible to every
// check in the project. That is the failure mode, reproduced here as a test
// rather than asserted in a comment.
//
// ⚠️ Written first, before the scheme was changed, per this project's own rule:
// a green null proves nothing, only a bite does. Four guards in this codebase
// have read as evidence while unable to act, and each was found by injecting the
// thing it claimed to catch.
func TestPathFingerprintMovesWhenContentsMoveAtAFixedPath(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("a.json", "one")
	before := pathFingerprint(dir)

	write("a.json", "two")
	after := pathFingerprint(dir)

	if before == after {
		t.Fatalf("pathFingerprint(%s) unchanged at %q after the contents under it changed — "+
			"this is the fourth comparability failure exactly: two runs against one path "+
			"holding different data compare as one", "<tmp>", before)
	}
	if strings.HasPrefix(before, "unreadable:") || strings.HasPrefix(after, "unreadable:") {
		t.Fatalf("a readable directory digested as unreadable: %q then %q", before, after)
	}

	// Same bytes, different name: a rename must move the digest too, or a cache
	// reorganised in place reads as untouched.
	if err := os.Remove(filepath.Join(dir, "a.json")); err != nil {
		t.Fatal(err)
	}
	write("b.json", "two")
	if renamed := pathFingerprint(dir); renamed == after {
		t.Fatal("pathFingerprint ignores file names, so a reorganised source compares as unchanged")
	}

	// And the value still may not name the host.
	for _, v := range []string{before, after} {
		if strings.Contains(v, "/") || strings.Contains(v, dir) {
			t.Fatalf("pathFingerprint leaked a host path into a value bound for a public sidecar: %q", v)
		}
	}
}

// TestPathFingerprintSeparatesReadableFromUnreadable pins that the UNREADABLE
// branch is a distinct state rather than a quiet substitute. A run that could
// not see its data source must never compare equal to one that could.
func TestPathFingerprintSeparatesReadableFromUnreadable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	readable := pathFingerprint(dir)
	missing := pathFingerprint(filepath.Join(dir, "absent"))

	if readable == missing {
		t.Fatal("a readable source and a missing one produced one value")
	}
	if !strings.HasPrefix(missing, "unreadable:") {
		t.Fatalf("a missing source was not labelled unreadable: %q — silence here is the whole "+
			"bug class, because the comparison then passes on data one arm never read", missing)
	}
}

// TestBankedPathTagIsNotReusedByANewScheme refuses a value computed one way from
// wearing a tag that 12 banked sidecars already use for a value computed another
// way.
//
// The banked corpus carries `path:<hex>`, a hash of the path STRING. This package
// now digests CONTENTS and tags them `data:`. Both answer "which source", and a
// reader differencing new cells against banked ones must be able to tell "these
// differ" from "these cannot be compared". Reusing one tag destroys that
// distinction silently, which is how a comparability guard becomes a liar.
func TestBankedPathTagIsNotReusedByANewScheme(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{pathFingerprint(dir), pathFingerprint("/a/b/c")} {
		if strings.HasPrefix(v, "path:") {
			t.Fatalf("a newly-computed value wears the legacy path: tag: %q — "+
				"the 12 sidecars already banked mean that tag denotes a hash of the path "+
				"STRING, so this makes an incomparability read as a difference in the data", v)
		}
	}
}
