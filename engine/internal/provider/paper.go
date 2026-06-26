package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/000hen/justhostmc/engine/internal/dl"
)

// defaultPaperAPI is PaperMC's "Fill" API (v3). The legacy v2 API
// (api.papermc.io/v2) is frozen and no longer serves current Minecraft versions.
const defaultPaperAPI = "https://fill.papermc.io/v3"

// userAgent identifies us to the PaperMC API, which asks callers to send one.
const userAgent = "JustHostMC (+https://github.com/000hen/justhostmc)"

// Paper downloads PaperMC server jars (the default plugin server; a Bukkit/Spigot
// drop-in). For a given MC version it resolves the latest build.
type Paper struct {
	client  *http.Client
	apiBase string
}

type PaperOption func(*Paper)

func WithPaperHTTPClient(c *http.Client) PaperOption { return func(p *Paper) { p.client = c } }
func WithPaperAPIBase(u string) PaperOption          { return func(p *Paper) { p.apiBase = u } }

func NewPaper(opts ...PaperOption) *Paper {
	p := &Paper{client: http.DefaultClient, apiBase: defaultPaperAPI}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Versions returns the supported Minecraft versions, newest first. The Fill API
// groups versions by series ({"26.2": [...], "1.21": [...]}); we flatten the
// groups (map order is unspecified) and sort newest-first.
func (p *Paper) Versions(ctx context.Context) ([]string, error) {
	var resp struct {
		Versions map[string][]string `json:"versions"`
	}
	if err := getJSON(ctx, p.client, p.apiBase+"/projects/paper", &resp); err != nil {
		return nil, err
	}
	var out []string
	for _, group := range resp.Versions {
		out = append(out, group...)
	}
	sortMCDesc(out)
	return out, nil
}

func (p *Paper) Install(ctx context.Context, dir, version string, progress func(Progress)) (LaunchSpec, error) {
	report(progress, Progress{Step: "install.progress.resolving_version", Fraction: -1})

	name, downloadURL, wantSHA, err := p.latestServerDownload(ctx, version)
	if err != nil {
		return LaunchSpec{}, err
	}

	jarPath := filepath.Join(dir, "server.jar")
	report(progress, Progress{Step: "install.progress.downloading_server", Fraction: 0, LogLine: name})
	gotSHA, _, err := dl.Download(ctx, p.client, downloadURL, jarPath, sha256.New(), func(done, total int64) {
		frac := -1.0
		if total > 0 {
			frac = float64(done) / float64(total)
		}
		report(progress, Progress{Fraction: frac})
	})
	if err != nil {
		return LaunchSpec{}, err
	}
	if wantSHA != "" && !strings.EqualFold(gotSHA, wantSHA) {
		return LaunchSpec{}, fmt.Errorf("paper %s: checksum mismatch (got %s, want %s)", version, gotSHA, wantSHA)
	}

	report(progress, Progress{Step: "install.progress.done", Fraction: 1})
	return LaunchSpec{JavaMajor: JavaMajorForMC(version), Args: []string{"-jar", "server.jar", "nogui"}}, nil
}

// latestServerDownload resolves the default server jar of a version's newest
// build: its name, absolute download URL, and expected sha256 (if published).
func (p *Paper) latestServerDownload(ctx context.Context, version string) (name, url, sha256hex string, err error) {
	var resp struct {
		Downloads map[string]struct {
			Name      string `json:"name"`
			URL       string `json:"url"`
			Checksums struct {
				SHA256 string `json:"sha256"`
			} `json:"checksums"`
		} `json:"downloads"`
	}
	endpoint := fmt.Sprintf("%s/projects/paper/versions/%s/builds/latest", p.apiBase, version)
	if err := getJSON(ctx, p.client, endpoint, &resp); err != nil {
		return "", "", "", err
	}
	d, ok := resp.Downloads["server:default"]
	if !ok || d.URL == "" {
		return "", "", "", fmt.Errorf("paper %q: %w", version, ErrVersionNotFound)
	}
	return d.Name, d.URL, d.Checksums.SHA256, nil
}

// getJSON is a small shared helper for provider GET-and-decode calls.
func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
