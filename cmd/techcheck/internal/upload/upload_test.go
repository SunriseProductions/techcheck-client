package upload_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/upload"
)

func TestUploadSuccess201(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer testtoken", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "abc", "received_at": "2026-04-15T10:00:00Z"})
	}))
	defer srv.Close()

	r := report.New()
	resp, err := upload.Upload(context.Background(), upload.Options{
		URL: srv.URL, Token: "testtoken", MaxRetries: 0,
	}, r)
	require.NoError(t, err)
	assert.Equal(t, "abc", resp.ID)
}

func TestUploadIdempotent200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "abc", "received_at": "2026-04-15T10:00:00Z"})
	}))
	defer srv.Close()

	r := report.New()
	resp, err := upload.Upload(context.Background(), upload.Options{
		URL: srv.URL, Token: "t", MaxRetries: 0,
	}, r)
	require.NoError(t, err)
	assert.Equal(t, "abc", resp.ID)
}

func TestUploadRetriesOn5xx(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&count, 1)
		if n < 3 {
			http.Error(w, "oops", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"id":"x","received_at":"now"}`)
	}))
	defer srv.Close()

	r := report.New()
	_, err := upload.Upload(context.Background(), upload.Options{
		URL: srv.URL, Token: "t", MaxRetries: 2,
	}, r)
	require.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&count))
}

func TestUploadDoesNotRetryOn4xx(t *testing.T) {
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		http.Error(w, "bad", http.StatusUnauthorized)
	}))
	defer srv.Close()

	r := report.New()
	_, err := upload.Upload(context.Background(), upload.Options{
		URL: srv.URL, Token: "t", MaxRetries: 3,
	}, r)
	require.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&count), "4xx must not retry")
}

func TestUploadFailsAfterRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()

	r := report.New()
	_, err := upload.Upload(context.Background(), upload.Options{
		URL: srv.URL, Token: "t", MaxRetries: 2,
	}, r)
	require.Error(t, err)
}
