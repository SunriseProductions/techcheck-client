package updater

import "runtime"

// serverPlatform maps Go's GOOS/GOARCH to the server's canonical
// (platform, arch) pair. The server uses darwin|windows|linux and
// arm64|x64|universal. Go's amd64 → server's x64.
func serverPlatform(goos, goarch string) (string, string) {
	arch := goarch
	if arch == "amd64" {
		arch = "x64"
	}
	return goos, arch
}

// CurrentPlatform returns the (platform, arch) for the running binary.
func CurrentPlatform() (string, string) {
	return serverPlatform(runtime.GOOS, runtime.GOARCH)
}
