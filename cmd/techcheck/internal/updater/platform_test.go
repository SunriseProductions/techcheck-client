package updater

import "testing"

func TestServerPlatform(t *testing.T) {
	cases := []struct {
		goos, goarch string
		wantPlat     string
		wantArch     string
	}{
		{"darwin", "arm64", "darwin", "arm64"},
		{"darwin", "amd64", "darwin", "x64"},
		{"windows", "amd64", "windows", "x64"},
		{"windows", "arm64", "windows", "arm64"},
		{"linux", "amd64", "linux", "x64"},
		{"linux", "arm64", "linux", "arm64"},
	}
	for _, c := range cases {
		p, a := serverPlatform(c.goos, c.goarch)
		if p != c.wantPlat || a != c.wantArch {
			t.Errorf("serverPlatform(%q,%q) = (%q,%q); want (%q,%q)",
				c.goos, c.goarch, p, a, c.wantPlat, c.wantArch)
		}
	}
}
