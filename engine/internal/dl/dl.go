// Package dl provides a small HTTP download helper with progress reporting and
// optional streaming checksum, shared by the providers and the JRE manager.
package dl

import (
	"context"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ProgressFunc receives cumulative bytes downloaded and the total size, where
// total is -1 when the server does not report a Content-Length.
type ProgressFunc func(downloaded, total int64)

// Download streams url into destPath (creating parent dirs). If h is non-nil it
// is fed every byte and the lowercase hex digest is returned for the caller to
// verify. onProgress may be nil.
func Download(ctx context.Context, client *http.Client, url, destPath string, h hash.Hash, onProgress ProgressFunc) (sum string, n int64, err error) {
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", 0, err
	}
	f, err := os.Create(destPath)
	if err != nil {
		return "", 0, err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	var dst io.Writer = f
	if h != nil {
		dst = io.MultiWriter(f, h)
	}
	pw := &progressWriter{w: dst, total: resp.ContentLength, onProgress: onProgress}
	n, err = io.Copy(pw, resp.Body)
	if err != nil {
		return "", n, err
	}
	if h != nil {
		sum = hex.EncodeToString(h.Sum(nil))
	}
	return sum, n, nil
}

type progressWriter struct {
	w          io.Writer
	total      int64
	done       int64
	onProgress ProgressFunc
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.onProgress != nil {
		p.onProgress(p.done, p.total)
	}
	return n, err
}
