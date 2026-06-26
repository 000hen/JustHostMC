package provider

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/000hen/justhostmc/engine/internal/dl"
)

const (
	defaultManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	// Pre-1.17 versions omit javaVersion in their metadata; they run on Java 8.
	defaultVanillaJavaMajor = 8
)

// Vanilla downloads official Mojang server jars.
type Vanilla struct {
	client      *http.Client
	manifestURL string
}

// VanillaOption customizes a Vanilla provider (used by tests to inject URLs).
type VanillaOption func(*Vanilla)

func WithHTTPClient(c *http.Client) VanillaOption { return func(v *Vanilla) { v.client = c } }
func WithManifestURL(u string) VanillaOption      { return func(v *Vanilla) { v.manifestURL = u } }

// NewVanilla builds a Vanilla provider with Mojang's production manifest URL.
func NewVanilla(opts ...VanillaOption) *Vanilla {
	v := &Vanilla{client: http.DefaultClient, manifestURL: defaultManifestURL}
	for _, o := range opts {
		o(v)
	}
	return v
}

type versionManifest struct {
	Latest   map[string]string `json:"latest"`
	Versions []manifestEntry   `json:"versions"`
}

type manifestEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

type versionDetail struct {
	Downloads struct {
		Server struct {
			URL  string `json:"url"`
			Size int64  `json:"size"`
			SHA1 string `json:"sha1"`
		} `json:"server"`
	} `json:"downloads"`
	JavaVersion struct {
		MajorVersion int `json:"majorVersion"`
	} `json:"javaVersion"`
}

func (v *Vanilla) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Versions returns all installable Minecraft version IDs (newest first, as
// Mojang orders them).
func (v *Vanilla) Versions(ctx context.Context) ([]string, error) {
	var m versionManifest
	if err := v.getJSON(ctx, v.manifestURL, &m); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(m.Versions))
	for _, e := range m.Versions {
		ids = append(ids, e.ID)
	}
	return ids, nil
}

// Install downloads server.jar for the given version into dir and returns its
// launch spec (including the Java major version Mojang declares for it).
func (v *Vanilla) Install(ctx context.Context, dir, version string, progress func(Progress)) (LaunchSpec, error) {
	report(progress, Progress{Step: "install.progress.resolving_version", Fraction: -1})

	var m versionManifest
	if err := v.getJSON(ctx, v.manifestURL, &m); err != nil {
		return LaunchSpec{}, err
	}
	entry, ok := findVersion(m.Versions, version)
	if !ok {
		return LaunchSpec{}, fmt.Errorf("vanilla %q: %w", version, ErrVersionNotFound)
	}

	var detail versionDetail
	if err := v.getJSON(ctx, entry.URL, &detail); err != nil {
		return LaunchSpec{}, err
	}

	jarPath := filepath.Join(dir, "server.jar")
	report(progress, Progress{Step: "install.progress.downloading_server", Fraction: 0, LogLine: "server.jar"})
	sum, _, err := dl.Download(ctx, v.client, detail.Downloads.Server.URL, jarPath, sha1.New(),
		func(done, total int64) {
			frac := -1.0
			if total > 0 {
				frac = float64(done) / float64(total)
			}
			report(progress, Progress{Fraction: frac})
		})
	if err != nil {
		return LaunchSpec{}, err
	}
	if want := detail.Downloads.Server.SHA1; want != "" && sum != want {
		return LaunchSpec{}, fmt.Errorf("vanilla %q server.jar: %w (got %s want %s)", version, ErrChecksumMismatch, sum, want)
	}

	major := detail.JavaVersion.MajorVersion
	if major == 0 {
		major = defaultVanillaJavaMajor
	}
	report(progress, Progress{Step: "install.progress.done", Fraction: 1})

	return LaunchSpec{JavaMajor: major, Args: []string{"-jar", "server.jar", "nogui"}}, nil
}

func findVersion(entries []manifestEntry, id string) (manifestEntry, bool) {
	for _, e := range entries {
		if e.ID == id {
			return e, true
		}
	}
	return manifestEntry{}, false
}
