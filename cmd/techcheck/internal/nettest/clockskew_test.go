package nettest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
)

func TestClockSkewTinyDelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"iso":          time.Now().UTC().Format(time.RFC3339Nano),
			"monotonic_ns": time.Now().UnixNano(),
		})
	}))
	defer srv.Close()

	skew, err := nettest.MeasureClockSkew(context.Background(), srv.URL+"/preflight/echo-ts")
	require.NoError(t, err)
	// Local clock and the server are the same process → skew within a few seconds.
	if skew < 0 {
		skew = -skew
	}
	assert.LessOrEqual(t, skew, int64(5000))
}

func TestClockSkewRejectsMalformed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	_, err := nettest.MeasureClockSkew(context.Background(), srv.URL+"/preflight/echo-ts")
	require.Error(t, err)
}
