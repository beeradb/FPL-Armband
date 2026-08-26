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

// TestPathFingerprintIsStableAndOpaque pins the digest the banked sidecars were
// scrubbed to, so a change to the scheme cannot silently orphan them.
func TestPathFingerprintIsStableAndOpaque(t *testing.T) {
	got := pathFingerprint("/a/b/c")
	if !strings.HasPrefix(got, "path:") {
		t.Fatalf("pathFingerprint(%q) = %q, want a path: prefix so a reader knows it is a digest", "/a/b/c", got)
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
