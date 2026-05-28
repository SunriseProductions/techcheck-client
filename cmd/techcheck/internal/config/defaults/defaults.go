// Package defaults holds build-time-injected configuration values. Each
// variable below is populated by `go build -ldflags "-X …"` in the
// maintainer's build pipeline. Source builds (no ldflags) leave them empty;
// in that case the binary runs but uploads to the production ingest fail
// for lack of credentials, and the fallback POP list is empty.
package defaults

var (
	// UploadToken is the bearer token sent in `Authorization: Bearer` on
	// every report upload.
	UploadToken string

	// IngestURL is the report upload endpoint URL.
	IngestURL string

	// ITContactEmail is shown to users on the consent and result screens
	// for support questions.
	ITContactEmail string

	// FallbackPOPsJSON is a JSON-encoded `[]config.POP`. Used when the
	// dynamic POP fetch from the ingest service fails at startup.
	FallbackPOPsJSON string
)
