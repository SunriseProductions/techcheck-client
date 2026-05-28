// mockprobe runs a local stand-in for a Sunrise nginx POP. It serves the
// documented /preflight/* HTTP endpoints and echoes UDP packets on the given
// port that start with the magic prefix. Use for end-to-end testing.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	httpAddr := flag.String("http", "127.0.0.1:18080", "HTTP listen address")
	udpAddr := flag.String("udp", "127.0.0.1:4172", "UDP echo listen address")
	magic := flag.String("magic", "PFLT", "UDP echo magic prefix")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/preflight/ping", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/preflight/download/10mb", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(make([]byte, 10*1024*1024))
	})
	mux.HandleFunc("/preflight/upload", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	})
	mux.HandleFunc("/preflight/mtu", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 1400))
	})
	mux.HandleFunc("/preflight/echo-ts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"iso":          time.Now().UTC().Format(time.RFC3339Nano),
			"monotonic_ns": time.Now().UnixNano(),
		})
	})
	mux.HandleFunc("/preflight/whoami", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"public_ip": "127.0.0.1",
			"pop_id":    "local",
			"geo":       "Localhost",
		})
	})

	httpSrv := &http.Server{Addr: *httpAddr, Handler: mux}
	go func() {
		log.Printf("mockprobe: http listen %s", *httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	pc, err := net.ListenPacket("udp", *udpAddr)
	if err != nil {
		log.Fatalf("udp listen %s: %v", *udpAddr, err)
	}
	log.Printf("mockprobe: udp listen %s", *udpAddr)
	go func() {
		buf := make([]byte, 1500)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n >= len(*magic) && string(buf[:len(*magic)]) == *magic {
				_, _ = pc.WriteTo(buf[:n], addr)
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	_ = httpSrv.Close()
	_ = pc.Close()
}
