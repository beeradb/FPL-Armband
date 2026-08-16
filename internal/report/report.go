package report

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Write saves a Markdown report and returns its path.
func Write(dir, title, body string, meta map[string]string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	now := time.Now()
	name := fmt.Sprintf("%s-%s.md", now.Format("2006-01-02"), slug(title))
	path := filepath.Join(dir, name)

	// Don't clobber an earlier report from the same day.
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%s-%d.md", now.Format("2006-01-02"), slug(title), i))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "*Generated %s*\n\n", now.Format("Mon 2 Jan 2006, 15:04 MST"))

	if len(meta) > 0 {
		keys := []string{"gameweek", "deadline", "model", "effort"}
		var wrote bool
		for _, k := range keys {
			if v, ok := meta[k]; ok && v != "" {
				fmt.Fprintf(&b, "- **%s**: %s\n", capitalize(k), v)
				wrote = true
			}
		}
		for k, v := range meta {
			if contains(keys, k) || v == "" {
				continue
			}
			fmt.Fprintf(&b, "- **%s**: %s\n", capitalize(k), v)
			wrote = true
		}
		if wrote {
			b.WriteString("\n")
		}
	}

	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
		s = strings.Trim(s, "-")
	}
	if s == "" {
		s = "report"
	}
	return s
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
