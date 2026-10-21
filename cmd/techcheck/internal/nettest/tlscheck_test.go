package nettest_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

// testPKI is a CA with CN "Test Root" and a leaf it signed for 127.0.0.1
// and localhost.
type testPKI struct {
	root *x509.Certificate
	leaf tls.Certificate
}

func newTestPKI(t *testing.T) testPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	root, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, root, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	return testPKI{
		root: root,
		leaf: tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: leafKey},
	}
}

func (p testPKI) pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(p.root)
	return pool
}

// startTLSServer serves /preflight/ping over TLS with the leaf from pki.
func startTLSServer(t *testing.T, pki testPKI) (string, int) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/preflight/ping" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("pong"))
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{pki.leaf}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return hostPort(t, srv.Listener.Addr().String())
}

func TestTLSReachableWithExpectedRoot(t *testing.T) {
	pki := newTestPKI(t)
	host, port := startTLSServer(t, pki)

	res := nettest.MeasureTLS(context.Background(), nettest.TLSCheckInput{
		Host:          host,
		Port:          port,
		ServerName:    "localhost",
		ExpectedRoots: []string{"Test Root"},
		Roots:         pki.pool(),
		Timeout:       2 * time.Second,
	})
	assert.Equal(t, report.ReachabilityReachable, res.Status)
	assert.False(t, res.TLSIntercepted)
}

func TestTLSInterceptedWhenRootNotExpected(t *testing.T) {
	pki := newTestPKI(t)
	host, port := startTLSServer(t, pki)

	res := nettest.MeasureTLS(context.Background(), nettest.TLSCheckInput{
		Host:          host,
		Port:          port,
		ExpectedRoots: []string{"ISRG Root X1"},
		Roots:         pki.pool(),
		Timeout:       2 * time.Second,
	})
	assert.Equal(t, report.ReachabilityReachable, res.Status)
	assert.True(t, res.TLSIntercepted)
}

func TestTLSInterceptedWhenUntrusted(t *testing.T) {
	pki := newTestPKI(t)
	host, port := startTLSServer(t, pki)

	res := nettest.MeasureTLS(context.Background(), nettest.TLSCheckInput{
		Host:          host,
		Port:          port,
		ServerName:    "localhost",
		ExpectedRoots: []string{"Test Root"},
		Roots:         x509.NewCertPool(),
		Timeout:       2 * time.Second,
	})
	assert.Equal(t, report.ReachabilityReachable, res.Status)
	assert.True(t, res.TLSIntercepted)
}

func TestTLSInterceptedWhenHandshakeFails(t *testing.T) {
	// The port accepts TCP and hangs up, so the TLS handshake never completes.
	ln := listenTCP(t)
	defer ln.Close()
	go acceptLoop(ln)
	host, port := hostPort(t, ln.Addr().String())

	res := nettest.MeasureTLS(context.Background(), nettest.TLSCheckInput{
		Host:          host,
		Port:          port,
		ExpectedRoots: []string{"Test Root"},
		Timeout:       2 * time.Second,
	})
	assert.Equal(t, report.ReachabilityReachable, res.Status)
	assert.True(t, res.TLSIntercepted)
}

func TestTLSBlockedWhenNothingListens(t *testing.T) {
	ln := listenTCP(t)
	host, port := hostPort(t, ln.Addr().String())
	require.NoError(t, ln.Close())

	res := nettest.MeasureTLS(context.Background(), nettest.TLSCheckInput{
		Host:          host,
		Port:          port,
		ExpectedRoots: []string{"Test Root"},
		Timeout:       500 * time.Millisecond,
	})
	assert.Equal(t, report.ReachabilityBlocked, res.Status)
	assert.False(t, res.TLSIntercepted)
}
