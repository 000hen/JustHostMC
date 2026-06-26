package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/000hen/justhostmc/engine/internal/dl"
)

const defaultFabricMetaBase = "https://meta.fabricmc.net/v2"

type FabricOption func(*Fabric)

type Fabric struct {
	client   *http.Client
	metaBase string
}

func WithFabricHTTPClient(c *http.Client) FabricOption {
	return func(f *Fabric) {
		if c != nil {
			f.client = c
		}
	}
}

func WithFabricMetaBase(u string) FabricOption {
	return func(f *Fabric) {
		if u != "" {
			f.metaBase = u
		}
	}
}

func NewFabric(opts ...FabricOption) *Fabric {
	f := &Fabric{client: http.DefaultClient, metaBase: defaultFabricMetaBase}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

type fabricGameVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

type fabricVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

func (f *Fabric) Versions(ctx context.Context) ([]string, error) {
	var entries []fabricGameVersion
	if err := f.getJSON(ctx, f.metaBase+"/versions/game", &entries); err != nil {
		return nil, err
	}

	versions := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Stable && e.Version != "" {
			versions = append(versions, e.Version)
		}
	}
	return versions, nil
}

func (f *Fabric) Install(ctx context.Context, dir, version string, progress func(Progress)) (LaunchSpec, error) {
	loader, err := f.latestVersion(ctx, "loader")
	if err != nil {
		return LaunchSpec{}, fmt.Errorf("fabric loader: %w", err)
	}
	installer, err := f.latestVersion(ctx, "installer")
	if err != nil {
		return LaunchSpec{}, fmt.Errorf("fabric installer: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return LaunchSpec{}, err
	}

	jarPath := filepath.Join(dir, "server.jar")
	downloadURL := fmt.Sprintf("%s/versions/loader/%s/%s/%s/server/jar",
		f.metaBase,
		url.PathEscape(version),
		url.PathEscape(loader),
		url.PathEscape(installer),
	)

	report(progress, Progress{Step: "install.progress.downloading_server", Fraction: 0})
	_, _, err = dl.Download(ctx, f.client, downloadURL, jarPath, nil, func(done, total int64) {
		frac := -1.0
		if total > 0 {
			frac = float64(done) / float64(total)
		}
		report(progress, Progress{Step: "install.progress.downloading_server", Fraction: frac})
	})
	if err != nil {
		return LaunchSpec{}, fmt.Errorf("fabric download server: %w", err)
	}

	report(progress, Progress{Step: "install.progress.done", Fraction: 1})
	return LaunchSpec{JavaMajor: JavaMajorForMC(version), Args: []string{"-jar", "server.jar", "nogui"}}, nil
}

func (f *Fabric) latestVersion(ctx context.Context, kind string) (string, error) {
	var entries []fabricVersion
	if err := f.getJSON(ctx, f.metaBase+"/versions/"+kind, &entries); err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Stable && e.Version != "" {
			return e.Version, nil
		}
	}
	for _, e := range entries {
		if e.Version != "" {
			return e.Version, nil
		}
	}
	return "", ErrVersionNotFound
}

func (f *Fabric) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "JustHostMC (+https://github.com/000hen/justhostmc)")

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", endpoint, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
