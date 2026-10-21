package popsfetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config/popsfetch"
)

func TestFetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/config/pops", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema":"pops.v1","pops":[
			{"id":"eu-west-2","region_label":"London","hostname":"euw2.beacon.example","udp_echo":true}
		]}`))
	}))
	defer srv.Close()

	res, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", time.Second)
	require.NoError(t, err)
	assert.Equal(t, []config.POP{{
		ID: "eu-west-2", RegionLabel: "London", Hostname: "euw2.beacon.example", UDPEcho: true,
	}}, res.POPs)
}

func TestFetchUnknownSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema":"pops.v99","pops":[]}`))
	}))
	defer srv.Close()

	res, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", time.Second)
	assert.Nil(t, res.POPs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognised schema")
}

func TestFetchNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusInternalServerError)
	}))
	defer srv.Close()

	res, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", time.Second)
	assert.Nil(t, res.POPs)
	require.Error(t, err)
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	res, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", 50*time.Millisecond)
	assert.Nil(t, res.POPs)
	require.Error(t, err)
}

func TestFetchBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	res, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", time.Second)
	assert.Nil(t, res.POPs)
	require.Error(t, err)
}

func TestFetchReturnsRunSettings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema":"pops.v1","pops":[],"run":{
			"full_test_max_latency_ms":80.0,"full_test_max_pops":2,
			"tls_expected_roots":["Root A"],"dcv_tcp_port":9443,"dcv_udp_port":9444,
			"udp_stream":{"packet_bytes":1200,"ladder_mbps":[5.0,15.5],"step_seconds":2.0,
				"loss_limit_pct":0.5,"teradici_mbps":4.0,"teradici_seconds":6.0}}}`))
	}))
	defer srv.Close()

	res, err := popsfetch.Fetch(context.Background(), srv.URL, time.Second)
	require.NoError(t, err)
	require.NotNil(t, res.Run)
	assert.Equal(t, config.RunSettings{
		FullTestMaxLatencyMS: 80, FullTestMaxPOPs: 2,
		TLSExpectedRoots: []string{"Root A"}, DCVTCPPort: 9443, DCVUDPPort: 9444,
		UDPStream: config.UDPStreamSettings{
			PacketBytes: 1200, LadderMbps: []float64{5, 15.5}, StepSeconds: 2,
			LossLimitPct: 0.5, TeradiciMbps: 4, TeradiciSeconds: 6,
		},
	}, *res.Run)
}

func TestFetchWithoutRunKeyReturnsNilRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema":"pops.v1","pops":[{"id":"a"}]}`))
	}))
	defer srv.Close()

	res, err := popsfetch.Fetch(context.Background(), srv.URL, time.Second)
	require.NoError(t, err)
	assert.Nil(t, res.Run)
	assert.Len(t, res.POPs, 1)
}
