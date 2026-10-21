package nettest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
)

type TLSCheckInput struct {
	Host          string
	Port          int
	ServerName    string   // name to verify the certificate for; defaults to Host
	ExpectedRoots []string // root CommonNames the real certificate chains to
	Roots         *x509.CertPool
	Timeout       time.Duration // defaults to 5s
}

// MeasureTLS checks that the DCV TCP port answers HTTPS, and whether the
// certificate the client sees chains to one of the expected roots.
//
// A TLS-intercepting proxy installs its own root on the machine, so its
// certificates still pass the OS store. Checking the root's name catches it.
// The handshake skips Go's own verification and runs it in VerifyConnection
// instead, which records the outcome and lets the request go ahead either
// way: an intercepted port is still a reachable one.
//
// No TCP connection gives blocked. Any HTTP response, whatever its status,
// gives reachable. A TCP connection whose TLS handshake fails gives reachable
// with TLSIntercepted set, since something on the path broke the handshake.
func MeasureTLS(ctx context.Context, in TLSCheckInput) report.TLSReachability {
	if in.Timeout <= 0 {
		in.Timeout = 5 * time.Second
	}
	serverName := in.ServerName
	if serverName == "" {
		serverName = in.Host
	}

	var connected, verified, expectedRoot atomic.Bool
	dialer := &net.Dialer{Timeout: in.Timeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err == nil {
				connected.Store(true)
			}
			return conn, err
		},
		DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: true, // verified in VerifyConnection, without failing the handshake
			VerifyConnection: func(cs tls.ConnectionState) error {
				verified.Store(true)
				expectedRoot.Store(chainsToExpectedRoot(cs, serverName, in.Roots, in.ExpectedRoots))
				return nil
			},
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: in.Timeout}

	url := fmt.Sprintf("https://%s/preflight/ping", net.JoinHostPort(in.Host, strconv.Itoa(in.Port)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return report.TLSReachability{Status: report.ReachabilityBlocked}
	}
	resp, err := client.Do(req)
	if err != nil && !connected.Load() {
		return report.TLSReachability{Status: report.ReachabilityBlocked}
	}
	if err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
	}
	return report.TLSReachability{
		Status:         report.ReachabilityReachable,
		TLSIntercepted: !verified.Load() || !expectedRoot.Load(),
	}
}

// chainsToExpectedRoot verifies the peer chain against roots (nil means the
// system pool) and reports whether a verified chain ends at a root whose
// CommonName is in expected.
func chainsToExpectedRoot(cs tls.ConnectionState, serverName string, roots *x509.CertPool, expected []string) bool {
	if len(cs.PeerCertificates) == 0 {
		return false
	}
	intermediates := x509.NewCertPool()
	for _, c := range cs.PeerCertificates[1:] {
		intermediates.AddCert(c)
	}
	chains, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{
		DNSName:       serverName,
		Roots:         roots,
		Intermediates: intermediates,
	})
	if err != nil {
		return false
	}
	for _, chain := range chains {
		root := chain[len(chain)-1]
		if slices.Contains(expected, root.Subject.CommonName) {
			return true
		}
	}
	return false
}
