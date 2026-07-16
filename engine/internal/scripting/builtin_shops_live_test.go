package scripting

import (
	"context"
	"os"
	"testing"
)

// TestModrinthLive hits the real Modrinth API once to prove the embedded
// script's parameter construction (facet syntax, version filters) is accepted
// upstream — something the httptest fakes cannot show. Gated like the e2e
// suite; a CurseForge equivalent would need a key, so it is not included.
func TestModrinthLive(t *testing.T) {
	if os.Getenv("JHMC_INTEGRATION") != "1" {
		t.Skip("set JHMC_INTEGRATION=1 to run live-API tests")
	}
	host := NewHost(nil, nil, nil)
	ss := NewShopSet(host, nil, nil)
	reg := NewRegistry(host, nil)
	if _, err := LoadBuiltinSources(context.Background(), reg, ss, nil); err != nil {
		t.Fatal(err)
	}
	sh, _ := ss.Get("modrinth")
	ctx := context.Background()

	page, err := sh.Search(ctx, ShopQuery{Query: "sodium", Loader: "fabric", Kind: "mod", Sort: "downloads", Limit: 5})
	if err != nil {
		t.Fatalf("live search: %v", err)
	}
	if len(page.Projects) == 0 || page.Total == 0 {
		t.Fatalf("live search returned nothing: %+v", page)
	}
	id := page.Projects[0].ID

	detail, err := sh.Detail(ctx, id)
	if err != nil {
		t.Fatalf("live detail: %v", err)
	}
	if detail.Project.Title == "" || detail.Body == "" || detail.BodyFormat != "markdown" {
		t.Fatalf("live detail incomplete: title=%q bodyLen=%d", detail.Project.Title, len(detail.Body))
	}

	versions, err := sh.Versions(ctx, id, "", "fabric")
	if err != nil {
		t.Fatalf("live versions: %v", err)
	}
	if len(versions) == 0 || versions[0].Filename == "" {
		t.Fatalf("live versions incomplete: %+v", versions)
	}

	file, err := sh.ResolveFile(ctx, id, versions[0].ID, "", "fabric")
	if err != nil {
		t.Fatalf("live resolve_file: %v", err)
	}
	if file.URL == "" || file.SHA512 == "" {
		t.Fatalf("live file incomplete: %+v", file)
	}
	t.Logf("live ok: %s -> %s (%d bytes)", detail.Project.Title, file.Filename, file.Size)
}
