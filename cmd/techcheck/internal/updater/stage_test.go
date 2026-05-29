package updater

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func makeZip(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "art.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractZipExe_HappyPath(t *testing.T) {
	zipPath := makeZip(t, map[string][]byte{
		"SunriseTechCheck.exe": []byte("MZ-fake-exe-bytes"),
		"Read Me First.txt":    []byte("ignore me"),
	})
	dest := filepath.Join(t.TempDir(), ".SunriseTechCheck.exe.new")
	if err := extractZipExe(zipPath, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "MZ-fake-exe-bytes" {
		t.Errorf("content = %q", got)
	}
}

func TestExtractZipExe_Missing(t *testing.T) {
	zipPath := makeZip(t, map[string][]byte{"other.txt": []byte("x")})
	dest := filepath.Join(t.TempDir(), "out.exe")
	if err := extractZipExe(zipPath, dest); err == nil {
		t.Fatal("expected error when exe missing from zip")
	}
}

func TestExtractDMGApp_BadPathErrors(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("DMG extraction only runs on darwin")
	}
	dest := filepath.Join(t.TempDir(), ".SunriseTechCheck.app.new")
	if err := extractDMGApp("/no/such/file.dmg", dest); err == nil {
		t.Fatal("expected error mounting a nonexistent dmg")
	}
}
