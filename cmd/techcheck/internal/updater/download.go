package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// hardCap bounds any single artifact download regardless of the advertised
// size — a defence against a malicious or misconfigured server.
const hardCap = 500 << 20 // 500 MiB

// Download streams the artifact at url into dest, computing its SHA256 as it
// goes and failing if the hash, byte count, or size cap is violated. progress
// (if non-nil) is called after each chunk with (bytesWritten, wantSize).
//
// On any error dest may contain a partial file; callers download into a temp
// dir and discard it on failure.
func Download(ctx context.Context, url, wantSHA string, wantSize int64, dest string, progress func(done, total int64)) error {
	if wantSize > hardCap {
		return fmt.Errorf("updater: advertised size %d exceeds cap %d", wantSize, hardCap)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("updater: download status %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	limit := int64(hardCap)
	if wantSize > 0 {
		limit = wantSize + 4096 // small slack for benign discrepancies
	}

	h := sha256.New()
	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > limit {
				return fmt.Errorf("updater: response exceeds expected size")
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			h.Write(buf[:n])
			if progress != nil {
				progress(written, wantSize)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}

	if wantSize > 0 && written != wantSize {
		return fmt.Errorf("updater: size mismatch: got %d want %d", written, wantSize)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, wantSHA) {
		return fmt.Errorf("updater: sha256 mismatch: got %s want %s", got, wantSHA)
	}
	return nil
}
