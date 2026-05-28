package nettest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

func TestMTUPass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 1400))
	}))
	defer srv.Close()

	res, err := nettest.MeasureMTU(context.Background(), srv.URL+"/preflight/mtu")
	require.NoError(t, err)
	assert.Equal(t, report.MTUStatusPass, res.Status)
}

func TestMTUFragmented(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 500))
	}))
	defer srv.Close()

	res, err := nettest.MeasureMTU(context.Background(), srv.URL+"/preflight/mtu")
	require.NoError(t, err)
	assert.Equal(t, report.MTUStatusFragmented, res.Status)
}

func TestMTUBlackHoleOnEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Empty body.
	}))
	defer srv.Close()

	res, _ := nettest.MeasureMTU(context.Background(), srv.URL+"/preflight/mtu")
	assert.Equal(t, report.MTUStatusBlackHole, res.Status)
}
