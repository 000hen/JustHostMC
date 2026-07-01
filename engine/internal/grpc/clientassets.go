package grpcsvc

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/dl"
)

var minecraftVersionManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
var minecraftClientDownloadMu sync.Mutex

type minecraftVersionManifest struct {
	Versions []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"versions"`
}

type minecraftVersionDetails struct {
	Downloads struct {
		Client struct {
			URL  string `json:"url"`
			SHA1 string `json:"sha1"`
		} `json:"client"`
	} `json:"downloads"`
}

// ensureMinecraftClient stores the version-matched client resources locally.
// Inventory rendering never hotlinks icon images: it reads models and textures
// from this archive, then lets server resource packs and mod jars override them.
func ensureMinecraftClient(ctx context.Context, paths appdata.Paths, version string) (string, error) {
	version = strings.TrimSpace(version)
	if version == "" || filepath.Base(version) != version || strings.ContainsAny(version, `/\\`) {
		return "", fmt.Errorf("invalid Minecraft version %q", version)
	}
	destination := filepath.Join(paths.ClientAssetsCache(), version, "client.jar")
	if info, err := os.Stat(destination); err == nil && info.Mode().IsRegular() {
		return destination, nil
	}
	minecraftClientDownloadMu.Lock()
	defer minecraftClientDownloadMu.Unlock()
	// A concurrent request may have populated the cache while this one waited.
	if info, err := os.Stat(destination); err == nil && info.Mode().IsRegular() {
		return destination, nil
	}

	client := &http.Client{Timeout: 3 * time.Minute}
	var manifest minecraftVersionManifest
	if err := fetchMinecraftJSON(ctx, client, minecraftVersionManifestURL, &manifest); err != nil {
		return "", err
	}
	detailsURL := ""
	for _, entry := range manifest.Versions {
		if entry.ID == version {
			detailsURL = entry.URL
			break
		}
	}
	if detailsURL == "" {
		return "", fmt.Errorf("Minecraft client assets for %s were not found", version)
	}
	var details minecraftVersionDetails
	if err := fetchMinecraftJSON(ctx, client, detailsURL, &details); err != nil {
		return "", err
	}
	download := details.Downloads.Client
	if download.URL == "" || download.SHA1 == "" {
		return "", fmt.Errorf("Minecraft %s has no client asset download", version)
	}

	temporary := destination + ".part"
	_ = os.Remove(temporary)
	sum, _, err := dl.Download(ctx, client, download.URL, temporary, sha1.New(), nil)
	if err != nil {
		_ = os.Remove(temporary)
		return "", fmt.Errorf("download Minecraft %s client assets: %w", version, err)
	}
	if !strings.EqualFold(sum, download.SHA1) {
		_ = os.Remove(temporary)
		return "", fmt.Errorf("Minecraft %s client asset checksum mismatch", version)
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		if info, statErr := os.Stat(destination); statErr == nil && info.Mode().IsRegular() {
			return destination, nil
		}
		return "", err
	}
	return destination, nil
}

func fetchMinecraftJSON(ctx context.Context, client *http.Client, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, response.Status)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(target)
}
