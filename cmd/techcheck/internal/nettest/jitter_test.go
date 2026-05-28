package nettest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
)

func TestJitterUnderLoad(t *testing.T) {
	ln := listenTCP(t)
	defer ln.Close()
	go acceptLoop(ln)

	host, port := hostPort(t, ln.Addr().String())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(make([]byte, 1<<20)) // 1 MiB
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, dropped, err := nettest.MeasureJitter(ctx, nettest.JitterInput{
		Host:         host,
		TCPPort:      port,
		DownloadURL:  srv.URL + "/preflight/download/10mb",
		Duration:     1 * time.Second,
		PingInterval: 100 * time.Millisecond,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, res.VarianceMS, 0.0)
	assert.GreaterOrEqual(t, dropped, 0)
}
