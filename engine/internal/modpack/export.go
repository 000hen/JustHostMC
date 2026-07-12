// Package modpack builds client-importable modpack packages from an installed
// modpack server: a CurseForge-format zip (manifest.json + overrides/) that
// launchers like the CurseForge app, Prism, and ATLauncher import directly,
// carrying the server's live configs.
package modpack

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/000hen/justhostmc/engine/internal/provider"
	"golang.org/x/sync/errgroup"
)

// DefaultFTBAPIBase is the public FTB modpack API (same upstream ftb.lua uses).
const DefaultFTBAPIBase = "https://api.feed-the-beast.com/v1/modpacks/public"

// Options configures one export.
type Options struct {
	ServerDir   string // installed server directory (read-only for export)
	DestZip     string // absolute output .zip path
	PackVersion string // opaque "packId/versionId" stored on the server record
	ServerName  string // used as the pack name in manifest.json
	FTBAPIBase  string // empty = DefaultFTBAPIBase; overridable in tests
}

// overrideDirs are the server dirs copied verbatim into overrides/ (when they
// exist). mods/ is handled separately so CF-covered jars aren't duplicated.
var overrideDirs = []string{
	"config", "defaultconfigs", "kubejs", "scripts", "resourcepacks", "shaderpacks",
}

// ftbManifest is the subset of the FTB version manifest export consumes.
type ftbManifest struct {
	Name    string `json:"name"`
	Targets []struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"targets"`
	Files []ftbFile `json:"files"`
}

type ftbFile struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Clientonly bool   `json:"clientonly"`
	Curseforge *struct {
		Project int64 `json:"project"`
		File    int64 `json:"file"`
	} `json:"curseforge"`
}

// dest is the file's zip-relative destination ("mods/x.jar").
func (f ftbFile) dest() string {
	p := strings.Trim(strings.TrimPrefix(strings.TrimSpace(f.Path), "./"), "/")
	if p == "" {
		return f.Name
	}
	return p + "/" + f.Name
}

// cfManifest is the CurseForge pack manifest.json shape.
type cfManifest struct {
	Minecraft struct {
		Version    string        `json:"version"`
		ModLoaders []cfModLoader `json:"modLoaders"`
	} `json:"minecraft"`
	ManifestType    string   `json:"manifestType"`
	ManifestVersion int      `json:"manifestVersion"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Author          string   `json:"author"`
	Files           []cfFile `json:"files"`
	Overrides       string   `json:"overrides"`
}

type cfModLoader struct {
	ID      string `json:"id"`
	Primary bool   `json:"primary"`
}

type cfFile struct {
	ProjectID int64 `json:"projectID"`
	FileID    int64 `json:"fileID"`
	Required  bool  `json:"required"`
}

func report(progress func(provider.Progress), p provider.Progress) {
	if progress != nil {
		progress(p)
	}
}

// Export writes a CurseForge-format client pack zip for the FTB pack install
// in o.ServerDir. Manifest-listed files with CurseForge ids (including
// client-only ones) become manifest entries; the server's live config dirs and
// hand-added mods go into overrides/; client-only direct-URL files are
// downloaded at export time.
func Export(ctx context.Context, client *http.Client, o Options, progress func(provider.Progress)) error {
	report(progress, provider.Progress{Step: "shop.export.preparing", Fraction: -1})

	pack, ver, ok := strings.Cut(o.PackVersion, "/")
	if !ok || pack == "" || ver == "" {
		return fmt.Errorf("invalid pack version %q", o.PackVersion)
	}
	base := o.FTBAPIBase
	if base == "" {
		base = DefaultFTBAPIBase
	}
	manifest, err := fetchManifest(ctx, client, fmt.Sprintf("%s/modpack/%s/%s", base, pack, ver))
	if err != nil {
		return err
	}

	out := cfManifest{
		ManifestType:    "minecraftModpack",
		ManifestVersion: 1,
		Name:            o.ServerName,
		Version:         manifest.Name,
		Overrides:       "overrides",
	}
	var loaderOK bool
	for _, t := range manifest.Targets {
		switch t.Type {
		case "game":
			out.Minecraft.Version = t.Version
		case "modloader":
			out.Minecraft.ModLoaders = []cfModLoader{
				{ID: strings.ToLower(t.Name) + "-" + t.Version, Primary: true},
			}
			loaderOK = true
		}
	}
	if out.Minecraft.Version == "" || !loaderOK {
		return fmt.Errorf("pack manifest has no game/modloader target")
	}

	// CF-hosted files (server-side AND client-only) become manifest entries;
	// their local copies must not be duplicated into overrides.
	covered := map[string]bool{}
	var clientDownloads []ftbFile
	for _, f := range manifest.Files {
		if f.Name == "" {
			continue
		}
		switch {
		case f.Curseforge != nil:
			out.Files = append(out.Files, cfFile{
				ProjectID: f.Curseforge.Project, FileID: f.Curseforge.File, Required: true,
			})
			covered[f.dest()] = true
		case f.Clientonly && f.URL != "":
			clientDownloads = append(clientDownloads, f)
		}
	}

	// Download client-only direct-URL files to a staging dir (they were never
	// installed server-side).
	staging, err := os.MkdirTemp("", "jhmc-export-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if len(clientDownloads) > 0 {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(6)
		done := 0
		total := len(clientDownloads)
		results := make(chan string, total)
		for _, f := range clientDownloads {
			g.Go(func() error {
				full := filepath.Join(staging, filepath.FromSlash(f.dest()))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					return err
				}
				if err := downloadTo(gctx, client, f.URL, full); err != nil {
					return fmt.Errorf("%s: %w", f.dest(), err)
				}
				results <- f.Name
				return nil
			})
		}
		go func() { _ = g.Wait(); close(results) }()
		for name := range results {
			done++
			report(progress, provider.Progress{Step: "shop.install.downloading",
				Fraction: float64(done) / float64(total), LogLine: name})
		}
		if err := g.Wait(); err != nil {
			return err
		}
	}

	report(progress, provider.Progress{Step: "shop.export.zipping", Fraction: -1})
	return writeZip(o, out, staging, covered)
}

func fetchManifest(ctx context.Context, client *http.Client, url string) (*ftbManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
	var m ftbManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode pack manifest: %v", err)
	}
	return &m, nil
}

func downloadTo(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// writeZip streams manifest.json, the staged client-only files, the server's
// override dirs, and uncovered mod jars into the destination zip.
func writeZip(o Options, manifest cfManifest, staging string, covered map[string]bool) error {
	zf, err := os.Create(o.DestZip)
	if err != nil {
		return err
	}
	defer zf.Close()
	zw := zip.NewWriter(zf)

	mj, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(mj.(io.Writer))
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return err
	}

	addFile := func(zipPath, fsPath string) error {
		w, err := zw.Create(zipPath)
		if err != nil {
			return err
		}
		f, err := os.Open(fsPath)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	}
	addTree := func(root, zipPrefix string, skip func(rel string) bool) error {
		return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			if skip != nil && skip(relSlash) {
				return nil
			}
			return addFile(path.Join(zipPrefix, relSlash), p)
		})
	}

	// Staged client-only downloads.
	if _, err := os.Stat(staging); err == nil {
		if err := addTree(staging, "overrides", nil); err != nil {
			return err
		}
	}
	// Live server config-ish dirs.
	for _, dir := range overrideDirs {
		root := filepath.Join(o.ServerDir, dir)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		if err := addTree(root, "overrides/"+dir, nil); err != nil {
			return err
		}
	}
	// Hand-added mods (anything the CF manifest doesn't already deliver).
	modsRoot := filepath.Join(o.ServerDir, "mods")
	if _, err := os.Stat(modsRoot); err == nil {
		if err := addTree(modsRoot, "overrides/mods", func(rel string) bool {
			return covered["mods/"+rel]
		}); err != nil {
			return err
		}
	}

	return zw.Close()
}
