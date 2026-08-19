package webui

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheBuildContextStillCarriesTheAssets guards a coupling that is invisible from either
// side.
//
// The image build does `COPY . .` and then `go build`, so the embedded assets reach the
// binary only if they reach the build context. .dockerignore excludes `*.html` and `*.md`
// — patterns written to keep documentation out of the context, at a time when no HTML was
// part of the program. Two of the four things this package embeds are .html files.
//
// They survive today because Docker's `*` does not cross a `/`, so `*.html` matches only
// at the context root. That is a subtlety of the matcher rather than a decision anyone
// recorded, and the failure if it changes is a red CI build on every branch with a message
// about a missing embed — a long way from the line that caused it.
//
// The image build cannot be run here (no container runtime), so this asserts the property
// the build depends on instead of the build itself: nothing in .dockerignore matches an
// embedded asset by Docker's own rules.
func TestTheBuildContextStillCarriesTheAssets(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".dockerignore"))
	if err != nil {
		t.Skipf("no .dockerignore to check: %v", err)
	}

	var patterns []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, strings.TrimPrefix(line, "!"))
	}
	if len(patterns) == 0 {
		t.Fatal("parsed no patterns out of .dockerignore")
	}

	// Every path this package embeds, as it appears in the build context.
	var embedded []string
	err = filepath.WalkDir("assets", func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		embedded = append(embedded, path.Join("internal/webui", filepath.ToSlash(p)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(embedded) < 8 {
		t.Fatalf("found %d embedded files; the walk is not reaching them", len(embedded))
	}

	for _, asset := range embedded {
		for _, pattern := range patterns {
			// Docker matches a pattern against the whole path, and `*` does not cross a
			// separator — so a pattern with fewer segments than the path cannot match it
			// unless it ends in `**`. Comparing segment by segment is that rule.
			if dockerIgnores(pattern, asset) {
				t.Errorf(".dockerignore pattern %q excludes %s from the build context.\n"+
					"The image build does COPY . . and then go build, so //go:embed would "+
					"fail to compile and CI would go red on every branch.", pattern, asset)
			}
		}
	}
}

// dockerIgnores reports whether a .dockerignore pattern matches a context-relative path,
// using Docker's segment-wise rules: `*` matches within one segment, `**` matches any
// number of segments.
func dockerIgnores(pattern, name string) bool {
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	np := strings.Split(name, "/")
	return matchSegments(pp, np)
}

func matchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		// A directory pattern excludes everything beneath it.
		return true
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(name); i++ {
			if matchSegments(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	ok, err := path.Match(pattern[0], name[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(pattern[1:], name[1:])
}
