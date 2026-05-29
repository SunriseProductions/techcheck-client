package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
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
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("error = %v, want sha256 mismatch", err)
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
	if !strings.Contains(err.Error(), "size") {
		t.Errorf("error = %v, want size mismatch", err)
	}
}

func TestDownload_OverCap(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "artifact")
	err := Download(context.Background(), "http://127.0.0.1:0/never", "x", int64(600)<<20, dest, nil)
	if err == nil {
		t.Fatal("expected over-cap error, got nil")
	}
}

func TestDownload_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "artifact")
	err := Download(context.Background(), srv.URL, "x", 1, dest, nil)
	if err == nil {
		t.Fatal("expected non-2xx error, got nil")
	}
}

func TestDownload_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call
	dest := filepath.Join(t.TempDir(), "artifact")
	err := Download(ctx, srv.URL, "x", 1, dest, nil)
	if err == nil {
		t.Fatal("expected context-cancelled error, got nil")
	}
}
