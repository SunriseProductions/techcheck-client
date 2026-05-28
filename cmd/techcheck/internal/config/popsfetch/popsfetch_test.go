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

	pops, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", time.Second)
	require.NoError(t, err)
	assert.Equal(t, []config.POP{{
		ID: "eu-west-2", RegionLabel: "London", Hostname: "euw2.beacon.example", UDPEcho: true,
	}}, pops)
}

func TestFetchUnknownSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"schema":"pops.v99","pops":[]}`))
	}))
	defer srv.Close()

	pops, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", time.Second)
	assert.Nil(t, pops)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognised schema")
}

func TestFetchNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusInternalServerError)
	}))
	defer srv.Close()

	pops, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", time.Second)
	assert.Nil(t, pops)
	require.Error(t, err)
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	pops, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", 50*time.Millisecond)
	assert.Nil(t, pops)
	require.Error(t, err)
}

func TestFetchBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	pops, err := popsfetch.Fetch(context.Background(), srv.URL+"/api/v1/config/pops", time.Second)
	assert.Nil(t, pops)
	require.Error(t, err)
}
