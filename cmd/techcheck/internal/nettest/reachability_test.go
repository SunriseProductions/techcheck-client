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
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

func TestTCPReachable(t *testing.T) {
	ln := listenTCP(t)
	defer ln.Close()
	go acceptLoop(ln)

	host, port := hostPort(t, ln.Addr().String())
	res := nettest.MeasureTCP(context.Background(), host, port, 500*time.Millisecond)
	assert.Equal(t, report.ReachabilityReachable, res.Status)
}

func TestTCPBlocked(t *testing.T) {
	res := nettest.MeasureTCP(context.Background(), "127.0.0.1", 1, 100*time.Millisecond)
	assert.Equal(t, report.ReachabilityBlocked, res.Status)
}

func TestDNSTiming(t *testing.T) {
	ms, err := nettest.MeasureDNS(context.Background(), "localhost")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, ms, int64(0))
}

func TestWhoami(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"public_ip": "1.2.3.4",
			"pop_id":    "eu-west-1",
			"geo":       "London, GB",
		})
	}))
	defer srv.Close()

	r, err := nettest.Whoami(context.Background(), srv.URL+"/preflight/whoami")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", r.PublicIP)
	assert.Equal(t, "London, GB", r.Geo)
	assert.Equal(t, "eu-west-1", r.POPID)
}

func TestWhoamiRejectsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := nettest.Whoami(context.Background(), srv.URL+"/preflight/whoami")
	require.Error(t, err)
}
