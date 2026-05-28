// mockingest is a local stand-in for Preflight Ingest. Accepts POST
// /api/v1/reports with a Bearer token, stores reports in-memory keyed by
// run_id, and returns 201 for a fresh report or 200 for a duplicate —
// matching the idempotency contract in PRD §7.3.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

type response struct {
	ID         string `json:"id"`
	ReceivedAt string `json:"received_at"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:19090", "HTTP listen address")
	flag.Parse()

	var mu sync.Mutex
	byRunID := make(map[string]response)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/reports", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var parsed struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil || parsed.RunID == "" {
			http.Error(w, "missing run_id", http.StatusBadRequest)
			return
		}

		mu.Lock()
		defer mu.Unlock()

		if existing, ok := byRunID[parsed.RunID]; ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK) // idempotent replay
			_ = json.NewEncoder(w).Encode(existing)
			return
		}
		rec := response{ID: uuid.NewString(), ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		byRunID[parsed.RunID] = rec
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(rec)
	})

	srv := &http.Server{Addr: *addr, Handler: mux}
	go func() {
		log.Printf("mockingest: listen %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	_ = srv.Close()
}
