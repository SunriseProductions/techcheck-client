package wizard

import "testing"

func TestIngestBaseURL(t *testing.T) {
	cases := []struct {
		ingest, want string
	}{
		{"https://telemetry.example/api/v1/reports", "https://telemetry.example"},
		{"http://localhost:8000/api/v1/reports", "http://localhost:8000"},
		{"", ""},
		{":bad:", ""},
	}
	for _, c := range cases {
		got := ingestBaseURL(c.ingest)
		if got != c.want {
			t.Errorf("ingestBaseURL(%q) = %q; want %q", c.ingest, got, c.want)
		}
	}
}
