package report_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

func TestWriteLocalToDir(t *testing.T) {
	dir := t.TempDir()
	r := report.New()
	r.User.FullName = "Jane Doe"
	r.User.Email = "jane@example.com"

	path, err := report.WriteLocalToDir(r, dir)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))
	assert.True(t, strings.HasPrefix(filepath.Base(path), "preflight-Jane-Doe-"))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed report.Report
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, r.RunID, parsed.RunID)
}

func TestWriteLocalSanitisesName(t *testing.T) {
	dir := t.TempDir()
	r := report.New()
	r.User.FullName = "Jane/Doe:Esq."

	path, err := report.WriteLocalToDir(r, dir)
	require.NoError(t, err)
	base := filepath.Base(path)
	for _, badChar := range []string{"/", ":"} {
		assert.NotContains(t, base, badChar, "filename must not contain %q", badChar)
	}
}

func TestWriteLocalHandlesEmptyName(t *testing.T) {
	dir := t.TempDir()
	r := report.New()
	r.User.FullName = ""

	path, err := report.WriteLocalToDir(r, dir)
	require.NoError(t, err)
	assert.Contains(t, filepath.Base(path), "anon")
}
