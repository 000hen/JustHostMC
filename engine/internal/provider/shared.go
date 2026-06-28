package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// userAgent identifies JustHostMC to upstream metadata APIs. Shared by the
// remaining Go providers (Forge/NeoForge) pending their migration to Lua.
const userAgent = "JustHostMC (+https://github.com/000hen/justhostmc)"

// getJSON GETs url and decodes the JSON body into out.
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
