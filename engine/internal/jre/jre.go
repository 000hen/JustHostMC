// Package jre resolves and caches per-major OpenJDK runtimes on demand from the
// Adoptium (Eclipse Temurin) API, so users never need Java pre-installed. A given
// Java major version is downloaded once and shared across all servers.
package jre

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/000hen/justhostmc/engine/internal/dl"
	"github.com/000hen/justhostmc/engine/internal/provider"
)

const defaultAPIBase = "https://api.adoptium.net"

// ErrJREUnavailable means Adoptium has no matching JRE for the request.
var ErrJREUnavailable = errors.New("no JRE available for the requested Java version")

// Manager downloads and caches JREs under cacheDir/<major>/.
type Manager struct {
	client   *http.Client
	cacheDir string
	apiBase  string
}

type Option func(*Manager)

func WithHTTPClient(c *http.Client) Option { return func(m *Manager) { m.client = c } }
func WithAPIBase(u string) Option         { return func(m *Manager) { m.apiBase = u } }

// NewManager builds a JRE manager caching under cacheDir.
func NewManager(cacheDir string, opts ...Option) *Manager {
	m := &Manager{client: http.DefaultClient, cacheDir: cacheDir, apiBase: defaultAPIBase}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *Manager) majorDir(major int) string {
	return filepath.Join(m.cacheDir, strconv.Itoa(major))
}

// EnsureJRE returns a path to java.exe for the given Java feature version,
// downloading and extracting it from Adoptium if it is not already cached.
func (m *Manager) EnsureJRE(ctx context.Context, major int, progress func(provider.Progress)) (string, error) {
	dir := m.majorDir(major)
	if java, ok := findJava(dir); ok {
		return java, nil // cache hit; shared across servers
	}

	link, checksum, err := m.resolveAsset(ctx, major, "jre")
	if err != nil {
		return "", err
	}

	report(progress, provider.Progress{Step: "install.progress.downloading_jre", Fraction: 0})
	tmp, err := os.CreateTemp("", "jhmc-jre-*.zip")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	sum, _, err := dl.Download(ctx, m.client, link, tmpPath, sha256.New(), func(done, total int64) {
		frac := -1.0
		if total > 0 {
			frac = float64(done) / float64(total)
		}
		report(progress, provider.Progress{Fraction: frac})
	})
	if err != nil {
		return "", err
	}
	if checksum != "" && !strings.EqualFold(sum, checksum) {
		return "", fmt.Errorf("jre %d: %w (got %s want %s)", major, provider.ErrChecksumMismatch, sum, checksum)
	}

	report(progress, provider.Progress{Step: "install.progress.extracting_jre", Fraction: -1})
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := extractZip(tmpPath, dir); err != nil {
		// Leave no half-extracted cache behind to mask the failure next time.
		_ = os.RemoveAll(dir)
		return "", err
	}

	java, ok := findJava(dir)
	if !ok {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("jre %d: java.exe not found after extraction: %w", major, ErrJREUnavailable)
	}
	return java, nil
}

// EnsureJDK returns a path to java.exe for the given Java feature version from a JDK package,
// which includes the Java compiler (javac) required by build tools like Spigot BuildTools.
func (m *Manager) EnsureJDK(ctx context.Context, major int, progress func(provider.Progress)) (string, error) {
	dir := filepath.Join(m.cacheDir, "jdk", strconv.Itoa(major))
	if java, ok := findJava(dir); ok {
		return java, nil // cache hit; shared across servers
	}

	link, checksum, err := m.resolveAsset(ctx, major, "jdk")
	if err != nil {
		return "", err
	}

	report(progress, provider.Progress{Step: "install.progress.downloading_jre", Fraction: 0})
	tmp, err := os.CreateTemp("", "jhmc-jdk-*.zip")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	sum, _, err := dl.Download(ctx, m.client, link, tmpPath, sha256.New(), func(done, total int64) {
		frac := -1.0
		if total > 0 {
			frac = float64(done) / float64(total)
		}
		report(progress, provider.Progress{Fraction: frac})
	})
	if err != nil {
		return "", err
	}
	if checksum != "" && !strings.EqualFold(sum, checksum) {
		return "", fmt.Errorf("jdk %d: %w (got %s want %s)", major, provider.ErrChecksumMismatch, sum, checksum)
	}

	report(progress, provider.Progress{Step: "install.progress.extracting_jre", Fraction: -1})
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := extractZip(tmpPath, dir); err != nil {
		// Leave no half-extracted cache behind to mask the failure next time.
		_ = os.RemoveAll(dir)
		return "", err
	}

	java, ok := findJava(dir)
	if !ok {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("jdk %d: java.exe not found after extraction: %w", major, ErrJREUnavailable)
	}
	return java, nil
}

type adoptiumAsset struct {
	Binary struct {
		Package struct {
			Link     string `json:"link"`
			Checksum string `json:"checksum"`
			Name     string `json:"name"`
		} `json:"package"`
	} `json:"binary"`
}

// adoptiumArchitectures lists the Adoptium `architecture` query values to try, in
// preference order, for the running engine's architecture. Java server processes
// run on the same machine as the engine, so the JRE must match its architecture.
// On Windows/arm64 a native aarch64 build isn't published for every Java major, so
// we fall back to x64 — Windows on ARM runs it under emulation.
func adoptiumArchitectures() []string {
	switch runtime.GOARCH {
	case "arm64":
		return []string{"aarch64", "x64"}
	case "386":
		return []string{"x86"}
	default: // amd64
		return []string{"x64"}
	}
}

func (m *Manager) resolveAsset(ctx context.Context, major int, imageType string) (link, checksum string, err error) {
	for _, arch := range adoptiumArchitectures() {
		link, checksum, err = m.resolveAssetForArch(ctx, major, imageType, arch)
		if err == nil {
			return link, checksum, nil
		}
		// Only fall through to the next architecture when this one simply has no
		// build; surface transport/decode failures immediately.
		if !errors.Is(err, ErrJREUnavailable) {
			return "", "", err
		}
	}
	return "", "", err
}

func (m *Manager) resolveAssetForArch(ctx context.Context, major int, imageType, arch string) (link, checksum string, err error) {
	url := fmt.Sprintf("%s/v3/assets/latest/%d/hotspot?architecture=%s&image_type=%s&os=windows&vendor=eclipse",
		m.apiBase, major, arch, imageType)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("adoptium assets (java %d, %s): unexpected status %s", major, arch, resp.Status)
	}
	var assets []adoptiumAsset
	if err := json.NewDecoder(resp.Body).Decode(&assets); err != nil {
		return "", "", err
	}
	if len(assets) == 0 || assets[0].Binary.Package.Link == "" {
		return "", "", fmt.Errorf("java %d (%s): %w", major, arch, ErrJREUnavailable)
	}
	return assets[0].Binary.Package.Link, assets[0].Binary.Package.Checksum, nil
}

// findJava locates a .../bin/java.exe under root (the Adoptium archive nests it
// under a top-level directory).
func findJava(root string) (string, bool) {
	var found string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "java.exe") &&
			strings.EqualFold(filepath.Base(filepath.Dir(path)), "bin") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found, found != ""
}

func extractZip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	cleanDest := filepath.Clean(dest)
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		// Guard against zip-slip path traversal.
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeZipEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipEntry(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func report(progress func(provider.Progress), p provider.Progress) {
	if progress != nil {
		progress(p)
	}
}
