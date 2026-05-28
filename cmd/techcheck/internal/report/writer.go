package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// WriteLocal writes the report to the user's Desktop with a filename built
// from the user name and a UTC timestamp. Returns the absolute path.
// The local file is the evidence-trail fallback mentioned in PRD §5.2 — it
// exists even when upload succeeds, so the user can inspect exactly what was
// sent.
func WriteLocal(r *Report) (string, error) {
	dir, err := desktopDir()
	if err != nil {
		return "", err
	}
	return WriteLocalToDir(r, dir)
}

// WriteLocalToDir is WriteLocal with an explicit output directory; used by
// tests so we don't clobber the real Desktop.
func WriteLocalToDir(r *Report, dir string) (string, error) {
	filename := fmt.Sprintf("preflight-%s-%s.json",
		sanitise(r.User.FullName),
		time.Now().UTC().Format("20060102T150405Z"))
	full := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(full)
	if err != nil {
		return full, nil
	}
	return abs, nil
}

func desktopDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Desktop"), nil
}

var nameSanitiser = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

// sanitise turns a user-provided name into a filename-safe token. Empty or
// all-junk inputs collapse to "anon" so the writer never emits a filename
// starting with a dash or containing only the timestamp.
func sanitise(s string) string {
	if s == "" {
		return "anon"
	}
	cleaned := nameSanitiser.ReplaceAllString(s, "-")
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		return "anon"
	}
	return cleaned
}
