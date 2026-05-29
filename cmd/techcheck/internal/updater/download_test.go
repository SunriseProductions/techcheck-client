package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func serveBytes(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestDownload_HappyPath(t *testing.T) {
	payload := []byte("hello update payload")
	srv := serveBytes(t, payload)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "artifact")
	var lastDone int64
	err := Download(context.Background(), srv.URL, sha256Hex(payload), int64(len(payload)), dest, func(done, total int64) {
		lastDone = done
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(payload) {
		t.Errorf("file content = %q", got)
	}
	if lastDone != int64(len(payload)) {
		t.Errorf("final progress done = %d, want %d", lastDone, int64(len(payload)))
	}
}

func TestDownload_SHAMismatch(t *testing.T) {
	payload := []byte("hello update payload")
	srv := serveBytes(t, payload)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "artifact")
	err := Download(context.Background(), srv.URL, "not-the-real-hash", int64(len(payload)), dest, nil)
	if err == nil {
		t.Fatal("expected sha256 mismatch error, got nil")
	}
}

func TestDownload_SizeMismatch(t *testing.T) {
	payload := []byte("hello update payload")
	srv := serveBytes(t, payload)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "artifact")
	err := Download(context.Background(), srv.URL, sha256Hex(payload), int64(len(payload))+100, dest, nil)
	if err == nil {
		t.Fatal("expected size mismatch error, got nil")
	}
}
