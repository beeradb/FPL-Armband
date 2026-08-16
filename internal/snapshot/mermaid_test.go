package snapshot

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestMermaidBlocksAreWellFormed structurally checks every ```mermaid block in the
// documentation.
//
// # Why a test and not a careful author
//
// A mermaid block that fails to parse renders as an error box, and it does so only in
// the viewer — nothing in `go test`, `gofmt` or a code review notices. Two of the files
// carrying these diagrams are the ones AGENTS.md names as required reading before
// changing code or scoring, so a broken diagram there is worse than no diagram: it is a
// visible defect in the document a newcomer is told to trust first.
//
// This machine has no node and therefore no mermaid renderer, so the diagrams could not
// be validated by drawing them. This is not a parser either. It catches the four
// breakages that are actually plausible when a diagram is written by hand:
//
//   - unbalanced subgraph/end, which silently swallows the rest of the chart;
//   - an odd number of quotes on a line, which is how a label with an embedded quote
//     fails;
//   - <b>/<i>/<strong> tags, which render as literal angle brackets wherever the host
//     disables HTML labels — mermaid handles <br/> in both modes, but nothing else;
//   - HTML-escaped entities (&lt;br/&gt;), which belong in an HTML <pre> and render
//     literally in a markdown block. That one is not hypothetical: it was written into
//     docs/README.md while preparing this very change, by copying a block out of an
//     HTML preview page.
//
// Node ids are checked against mermaid's keywords for the same reason `end` as an id is
// a classic: the chart parses and draws the wrong thing.
func TestMermaidBlocksAreWellFormed(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	var files []string
	files = append(files, filepath.Join(root, "AGENTS.md"))
	// The notes glob that used to sit here was deleted with its directory, for the
	// reason written out at length in retracted_test.go: a glob left pointing at a
	// missing directory returns zero matches AND no error, which is silence wearing
	// a costume that looks like coverage.
	//
	// It survived the first pass of that clean-up, which is the interesting part.
	// TestNoLivePointerCitesTheRecordByPath was written in the same commit to catch
	// exactly this and could not see it, because a Go filepath.Join citation spells
	// the path as separate quoted segments and contains no slash. The guard now
	// matches that form too — this line is why.
	for _, pat := range []string{
		filepath.Join(root, "docs", "*.md"),
		filepath.Join(root, "stats", "*.md"),
	} {
		if more, err := filepath.Glob(pat); err == nil {
			files = append(files, more...)
		}
	}

	block := regexp.MustCompile("(?s)```mermaid\n(.*?)```")
	// A node id is the identifier immediately before a shape opener.
	nodeID := regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*[\[({]`)
	reserved := map[string]bool{
		"end": true, "graph": true, "subgraph": true, "class": true,
		"classDef": true, "click": true, "style": true, "linkStyle": true,
		"flowchart": true, "o": true, "x": true,
	}

	total := 0
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, path)
		for _, m := range block.FindAllStringSubmatch(string(body), -1) {
			total++
			src := m[1]
			lines := strings.Split(src, "\n")

			var kind string
			for _, l := range lines {
				if s := strings.TrimSpace(l); s != "" {
					kind = s
					break
				}
			}

			if strings.HasPrefix(kind, "flowchart") || strings.HasPrefix(kind, "graph") {
				var opens, closes int
				for _, l := range lines {
					s := strings.TrimSpace(l)
					if strings.HasPrefix(s, "subgraph") {
						opens++
					}
					if s == "end" {
						closes++
					}
				}
				if opens != closes {
					t.Errorf("%s: %d subgraph against %d end. An unbalanced subgraph "+
						"swallows the rest of the chart rather than erroring on the line "+
						"that is wrong.", rel, opens, closes)
				}
			}

			for i, l := range lines {
				n := i + 1
				if strings.Count(l, `"`)%2 != 0 {
					t.Errorf("%s:%d odd number of quotes: %s", rel, n, strings.TrimSpace(l))
				}
				for _, tag := range []string{"<b>", "</b>", "<i>", "</i>", "<em>", "<strong>"} {
					if strings.Contains(l, tag) {
						t.Errorf("%s:%d contains %s, which renders as literal angle "+
							"brackets wherever the host disables HTML labels. Only <br/> "+
							"is safe in both modes: %s", rel, n, tag, strings.TrimSpace(l))
					}
				}
				for _, ent := range []string{"&lt;", "&gt;", "&amp;"} {
					if strings.Contains(l, ent) {
						t.Errorf("%s:%d contains the escaped entity %s. Escaping belongs "+
							"in an HTML <pre>, not in a markdown mermaid block, where it "+
							"renders literally: %s", rel, n, ent, strings.TrimSpace(l))
					}
				}
				s := strings.TrimSpace(l)
				if strings.HasPrefix(s, "classDef") || strings.HasPrefix(s, "class ") ||
					strings.HasPrefix(s, "subgraph") || strings.HasPrefix(s, "%%") ||
					strings.HasPrefix(s, "click") || strings.HasPrefix(s, "style") {
					continue
				}
				if id := nodeID.FindStringSubmatch(l); id != nil && reserved[id[1]] {
					t.Errorf("%s:%d uses the mermaid keyword %q as a node id — the chart "+
						"parses and draws the wrong thing: %s", rel, n, id[1], s)
				}
			}
		}
	}
	if total == 0 {
		t.Skip("no mermaid blocks to check")
	}
	t.Logf("checked %d mermaid block(s)", total)
}
