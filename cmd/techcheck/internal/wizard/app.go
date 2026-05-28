package wizard

import (
	"context"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config/identity"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/config/popsfetch"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/nettest"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/report"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/sysinfo"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/updater"
	"github.com/sunriseproductions/techcheck-client/cmd/techcheck/internal/upload"
)

// App is the Wails-bound backend. Public methods on this type are callable
// from the frontend JS as `window.go.wizard.App.<Method>(...)`.
type App struct {
	ctx    context.Context
	cfg    *config.Config
	emitFn func(ctx context.Context, event string, optionalData ...interface{})

	mu         sync.Mutex
	current    *report.Report
	cancel     context.CancelFunc
	lastLocal  string
	lastUpload *upload.Response
}

// NewApp constructs the app with config loaded from the baked-in defaults
// plus an optional sidecar next to the binary. On startup it tries to fetch
// the live POP list from ingest; on any failure the baked-in list is used.
// Config errors are logged to stderr and the app falls back to defaults so
// the wizard never fails to launch.
func NewApp() *App {
	cfg, err := config.Load(sidecarPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "preflight: config load failed, using defaults: %v\n", err)
		cfg, _ = config.Load("")
	}
	return NewAppWithConfigAndLivePOPs(cfg, popsURL(cfg.IngestURL), 5*time.Second)
}

// NewAppWithConfigAndLivePOPs is the full-featured constructor used by tests
// and by NewApp. If popsURL is non-empty, it fetches the live list and, on
// success, replaces cfg.POPs. Any failure keeps cfg.POPs as the baked-in list.
func NewAppWithConfigAndLivePOPs(cfg *config.Config, popsURL string, timeout time.Duration) *App {
	emit := wruntime.EventsEmit
	if popsURL == "" {
		// No fetch — this is the test-seam path. Use a noop emit so tests don't
		// require a Wails runtime context.
		emit = noopEmit
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		pops, err := popsfetch.Fetch(ctx, popsURL, timeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "preflight: pops fetch failed, using baked-in list: %v\n", err)
		} else if len(pops) > 0 {
			cfg.POPs = pops
		}
	}
	return &App{cfg: cfg, emitFn: emit}
}

// POPs returns the effective POP list (live if the startup fetch succeeded,
// otherwise baked-in). Exposed for testing and for the frontend status screen.
func (a *App) POPs() []config.POP {
	return a.cfg.POPs
}

// CheckForUpdate queries the ingest server for the latest published
// version of this app. A 3-second timeout keeps the UI responsive; any
// error is swallowed and returns a zero Status (UI shows no banner).
// Bound to the Wails frontend.
func (a *App) CheckForUpdate() updater.Status {
	base := ingestBaseURL(a.cfg.IngestURL)
	if base == "" {
		return updater.Status{}
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	s, err := updater.Check(ctx, base, report.AppID, report.ToolVersion)
	if err != nil {
		return updater.Status{}
	}
	return s
}

// popsURL derives the POP registry URL from the ingest URL by stripping the
// path and appending /api/v1/config/pops. E.g.
// "https://telemetry.example/api/v1/reports" -> "https://telemetry.example/api/v1/config/pops".
func popsURL(ingestURL string) string {
	u, err := url.Parse(ingestURL)
	if err != nil {
		return ""
	}
	u.Path = "/api/v1/config/pops"
	return u.String()
}

// ingestBaseURL derives the scheme+host prefix from the ingest URL.
// "https://telemetry.example/api/v1/reports" -> "https://telemetry.example".
// Returns "" for an unparseable URL.
func ingestBaseURL(ingestURL string) string {
	if ingestURL == "" {
		return ""
	}
	u, err := url.Parse(ingestURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func noopEmit(ctx context.Context, event string, optionalData ...interface{}) {}

// Startup is called by Wails once the runtime context is available.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// ITContactEmail is shown on the fallback screen. Configured value.
func (a *App) ITContactEmail() string { return a.cfg.ITContactEmail }

// IngestURL is shown on the consent screen.
func (a *App) IngestURL() string { return a.cfg.IngestURL }

// SavedIdentity returns the name + email persisted from a previous run, or
// empty strings if none is saved. Used by the identify screen to pre-fill.
func (a *App) SavedIdentity() map[string]string {
	id := identity.Load()
	return map[string]string{"full_name": id.FullName, "email": id.Email}
}

// ValidateIdentity is called by the Identify screen before StartRun.
// Non-empty name + parsable email is the minimum bar.
func (a *App) ValidateIdentity(fullName, email string) (bool, error) {
	if strings.TrimSpace(fullName) == "" {
		return false, fmt.Errorf("name is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return false, fmt.Errorf("invalid email address")
	}
	// Persist for next run. Save failure is non-fatal — identity is a
	// convenience, not a requirement.
	_ = identity.Save(identity.Identity{FullName: strings.TrimSpace(fullName), Email: strings.TrimSpace(email)})
	return true, nil
}

// StartRun kicks off sysinfo → nettest → local write → upload. Returns
// immediately; the frontend listens for "progress" and "complete" events.
func (a *App) StartRun(fullName, email string) error {
	if _, err := a.ValidateIdentity(fullName, email); err != nil {
		return err
	}

	a.mu.Lock()
	runCtx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel
	r := report.New()
	r.User.FullName = strings.TrimSpace(fullName)
	r.User.Email = strings.TrimSpace(email)
	r.Consent.Accepted = true
	r.Consent.At = time.Now().UTC()
	a.current = r
	a.mu.Unlock()

	go a.run(runCtx, r)
	return nil
}

// Cancel aborts an in-progress run.
func (a *App) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// SendReport uploads the most recent report to the ingest server. Called
// from the result screen's Send button and, if that fails, the Retry button.
// The server dedupes on run_id so repeated calls are safe.
func (a *App) SendReport() error {
	a.mu.Lock()
	r := a.current
	a.mu.Unlock()
	if r == nil {
		return fmt.Errorf("no report to send")
	}
	resp, err := upload.Upload(a.ctx, upload.Options{
		URL:        a.cfg.IngestURL,
		Token:      a.cfg.UploadToken,
		Timeout:    time.Duration(a.cfg.UploadTimeoutSeconds) * time.Second,
		MaxRetries: a.cfg.UploadRetries,
	}, r)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.lastUpload = resp
	a.mu.Unlock()
	return nil
}

// LastUploadInfo returns id/received_at from the most recent successful upload,
// or zero values if nothing has been uploaded yet.
func (a *App) LastUploadInfo() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[string]string{"report_id": "", "received_at": ""}
	if a.lastUpload != nil {
		out["report_id"] = a.lastUpload.ID
		out["received_at"] = a.lastUpload.ReceivedAt
	}
	return out
}

// LocalReportPath returns the path written on the most recent run, or "".
func (a *App) LocalReportPath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastLocal
}

func (a *App) run(ctx context.Context, r *report.Report) {
	a.emit("progress", map[string]any{"phase": "sysinfo_start"})
	m, err := sysinfo.Collect(ctx, sysinfo.Options{OmitWifiSSID: a.cfg.OmitWifiSSID})
	if err != nil {
		r.RecordError("", "sysinfo", err.Error())
	}
	r.Machine = m
	a.emit("progress", map[string]any{"phase": "sysinfo_done"})

	// Forward per-test progress events from the runner.
	progCh := make(chan nettest.Progress, 128)
	progDone := make(chan struct{})
	go func() {
		for p := range progCh {
			a.emit("progress", map[string]any{
				"phase":   "test",
				"pop":     p.POPID,
				"test":    p.Test,
				"state":   p.Phase,
				"err":     p.Err,
				"summary": p.Summary,
			})
		}
		close(progDone)
	}()

	net, errs := nettest.RunAll(ctx, a.cfg, progCh)
	close(progCh)
	<-progDone

	r.Network = net
	r.Errors = append(r.Errors, errs...)
	r.RunCompletedAt = time.Now().UTC()

	// Local write — always, before upload so the user has the file even if
	// the network step fails.
	if path, err := report.WriteLocal(r); err == nil {
		a.mu.Lock()
		a.lastLocal = path
		a.mu.Unlock()
	}

	a.mu.Lock()
	localPath := a.lastLocal
	a.mu.Unlock()

	// Upload automatically — the consent screen covers this. The retry
	// button on the failure screen calls SendReport() if it fails here.
	uploadResp, uploadErr := upload.Upload(ctx, upload.Options{
		URL:        a.cfg.IngestURL,
		Token:      a.cfg.UploadToken,
		Timeout:    time.Duration(a.cfg.UploadTimeoutSeconds) * time.Second,
		MaxRetries: a.cfg.UploadRetries,
	}, r)
	completion := map[string]any{
		"local_path": localPath,
		"it_email":   a.cfg.ITContactEmail,
	}
	if uploadErr == nil {
		a.mu.Lock()
		a.lastUpload = uploadResp
		a.mu.Unlock()
		completion["uploaded"] = true
		completion["report_id"] = uploadResp.ID
		completion["received_at"] = uploadResp.ReceivedAt
	} else {
		completion["uploaded"] = false
		completion["error"] = uploadErr.Error()
	}
	a.emit("complete", completion)
}

func (a *App) emit(event string, data map[string]any) {
	if a.ctx == nil {
		return
	}
	a.emitFn(a.ctx, event, data)
}

// sidecarPath returns the absolute path to the optional preflight.config.json
// next to the binary.
func sidecarPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "preflight.config.json")
}
