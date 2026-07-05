package scripting

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// rewriteTransport sends every request to the test server regardless of the
// host the script targeted, so the embedded shop scripts can be exercised
// against canned upstream responses without touching the network.
type rewriteTransport struct{ target *url.URL }

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newBuiltinShops loads the embedded shop scripts on a host whose HTTP
// traffic is redirected to handler. The curseforge shop gets key "test-key".
func newBuiltinShops(t *testing.T, handler http.Handler) *ShopSet {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: rewriteTransport{target: u}}
	ss := NewShopSet(NewHost(client, nil, nil), nil, func(id string) string {
		if id == "curseforge" {
			return "test-key"
		}
		return ""
	})
	if err := LoadBuiltinShops(ss); err != nil {
		t.Fatalf("LoadBuiltinShops: %v", err)
	}
	return ss
}

func TestBuiltinShopsLoad(t *testing.T) {
	ss := newBuiltinShops(t, http.NotFoundHandler())
	mr, ok := ss.Get("modrinth")
	if !ok || mr.Meta().NeedsKey || !mr.Ready() {
		t.Fatalf("modrinth: ok=%v meta=%+v", ok, mr.Meta())
	}
	cf, ok := ss.Get("curseforge")
	if !ok || !cf.Meta().NeedsKey || !cf.Ready() {
		t.Fatalf("curseforge: ok=%v meta=%+v", ok, cf.Meta())
	}
}

func TestModrinthSearchBuildsFacets(t *testing.T) {
	var gotPath, gotFacets, gotIndex, gotQuery string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotFacets = r.URL.Query().Get("facets")
		gotIndex = r.URL.Query().Get("index")
		gotQuery = r.URL.Query().Get("query")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hits": []map[string]any{{
				"project_id": "AABBCCDD", "slug": "sodium", "title": "Sodium",
				"description": "fast", "icon_url": "http://x/i.png", "author": "jelly",
				"downloads": 9000, "follows": 42, "categories": []string{"optimization"},
				"versions": []string{"26.1"}, "project_type": "mod",
			}},
			"offset": 0, "limit": 20, "total_hits": 1,
		})
	})
	ss := newBuiltinShops(t, handler)
	sh, _ := ss.Get("modrinth")

	page, err := sh.Search(context.Background(), ShopQuery{
		Query: "sodium", MCVersion: "26.1", Loader: "fabric", Kind: "mod", Sort: "downloads", Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v2/search" || gotQuery != "sodium" || gotIndex != "downloads" {
		t.Fatalf("request: path=%q query=%q index=%q", gotPath, gotQuery, gotIndex)
	}
	want := `[["project_type:mod"],["categories:fabric"],["versions:26.1"]]`
	if gotFacets != want {
		t.Fatalf("facets = %q, want %q", gotFacets, want)
	}
	if page.Total != 1 || len(page.Projects) != 1 || page.Projects[0].ID != "AABBCCDD" ||
		page.Projects[0].Downloads != 9000 {
		t.Fatalf("page: %+v", page)
	}
}

func TestModrinthPluginLoaderFansOut(t *testing.T) {
	var gotFacets string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFacets = r.URL.Query().Get("facets")
		_ = json.NewEncoder(w).Encode(map[string]any{"hits": []any{}, "total_hits": 0})
	})
	ss := newBuiltinShops(t, handler)
	sh, _ := ss.Get("modrinth")
	if _, err := sh.Search(context.Background(), ShopQuery{Loader: "paper", Kind: "plugin"}); err != nil {
		t.Fatal(err)
	}
	want := `[["project_type:mod"],["categories:paper","categories:spigot","categories:bukkit"]]`
	if gotFacets != want {
		t.Fatalf("facets = %q, want %q", gotFacets, want)
	}
}

func TestModrinthResolveFilePicksPrimary(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/project/sodium/version" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id": "v9", "project_id": "AABBCCDD", "name": "Sodium 1.0",
			"version_number": "1.0.0", "version_type": "release",
			"game_versions": []string{"26.1"}, "loaders": []string{"fabric"},
			"date_published": "2026-01-01T00:00:00Z", "downloads": 5,
			"files": []map[string]any{
				{"url": "http://cdn/x-sources.jar", "filename": "x-sources.jar", "primary": false, "size": 10},
				{"url": "http://cdn/x.jar", "filename": "x.jar", "primary": true, "size": 999,
					"hashes": map[string]string{"sha1": "aa", "sha512": "bb"}},
			},
			"dependencies": []any{},
		}})
	})
	ss := newBuiltinShops(t, handler)
	sh, _ := ss.Get("modrinth")

	f, err := sh.ResolveFile(context.Background(), "sodium", "", "26.1", "fabric")
	if err != nil {
		t.Fatal(err)
	}
	if f.Filename != "x.jar" || f.SHA512 != "bb" || f.Size != 999 {
		t.Fatalf("file: %+v", f)
	}
}

func TestCurseForgeSearchParams(t *testing.T) {
	var got url.Values
	var gotKey string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		gotKey = r.Header.Get("x-api-key")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"id": 310806, "slug": "jei", "name": "JEI", "summary": "items",
				"classId": 6, "downloadCount": 12345, "thumbsUpCount": 7,
				"logo":    map[string]any{"thumbnailUrl": "http://cf/t.png"},
				"authors": []map[string]any{{"name": "mezz"}},
				"categories": []map[string]any{{"name": "Utility"}},
			}},
			"pagination": map[string]any{"index": 0, "pageSize": 20, "totalCount": 55},
		})
	})
	ss := newBuiltinShops(t, handler)
	sh, _ := ss.Get("curseforge")

	page, err := sh.Search(context.Background(), ShopQuery{
		Query: "jei", MCVersion: "26.1", Loader: "neoforge", Kind: "mod", Sort: "downloads", Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "test-key" {
		t.Fatalf("x-api-key = %q", gotKey)
	}
	if got.Get("gameId") != "432" || got.Get("classId") != "6" || got.Get("sortField") != "6" ||
		got.Get("gameVersion") != "26.1" || got.Get("modLoaderType") != "6" ||
		got.Get("searchFilter") != "jei" {
		t.Fatalf("params: %v", got)
	}
	if page.Total != 55 || len(page.Projects) != 1 || page.Projects[0].ID != "310806" ||
		page.Projects[0].Author != "mezz" {
		t.Fatalf("page: %+v", page)
	}
}

func TestCurseForgeResolveFileDownloadURLFallback(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/mods/310806/files/500":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"id": 500, "fileName": "jei.jar", "fileLength": 777, "downloadUrl": nil,
				"hashes": []map[string]any{{"value": "cc", "algo": 1}},
			}})
		case "/v1/mods/310806/files/500/download-url":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "http://edge/jei.jar"})
		default:
			http.NotFound(w, r)
		}
	})
	ss := newBuiltinShops(t, handler)
	sh, _ := ss.Get("curseforge")

	f, err := sh.ResolveFile(context.Background(), "310806", "500", "26.1", "forge")
	if err != nil {
		t.Fatal(err)
	}
	if f.URL != "http://edge/jei.jar" || f.Filename != "jei.jar" || f.SHA1 != "cc" {
		t.Fatalf("file: %+v", f)
	}
}

func TestCurseForgeNotDistributable(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/mods/1/files/2":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"id": 2, "fileName": "x.jar", "fileLength": 1, "downloadUrl": "",
			}})
		default:
			http.NotFound(w, r)
		}
	})
	ss := newBuiltinShops(t, handler)
	sh, _ := ss.Get("curseforge")

	_, err := sh.ResolveFile(context.Background(), "1", "2", "", "")
	if !errors.Is(err, ErrShopNotDistributable) {
		t.Fatalf("want ErrShopNotDistributable, got %v", err)
	}
}

func TestCurseForgeVersionsMapsFiles(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/mods/310806/files":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
				"id": 500, "displayName": "JEI 26.1", "fileName": "jei-26.1.jar",
				"releaseType": 2, "fileDate": "2026-02-02T00:00:00Z",
				"downloadCount": 3, "fileLength": 777,
				"sortableGameVersions": []map[string]any{
					{"gameVersion": "26.1", "gameVersionName": "26.1"},
					{"gameVersion": "", "gameVersionName": "NeoForge"},
				},
				"dependencies": []map[string]any{{"modId": 999, "relationType": 3}},
			}}})
		case r.URL.Path == "/v1/mods" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"id": 999, "name": "Dep Mod"},
			}})
		default:
			http.NotFound(w, r)
		}
	})
	ss := newBuiltinShops(t, handler)
	sh, _ := ss.Get("curseforge")

	vs, err := sh.Versions(context.Background(), "310806", "26.1", "neoforge")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 {
		t.Fatalf("versions: %+v", vs)
	}
	v := vs[0]
	if v.Channel != "beta" || v.Filename != "jei-26.1.jar" || v.SizeBytes != 777 {
		t.Fatalf("version: %+v", v)
	}
	if len(v.GameVersions) != 1 || v.GameVersions[0] != "26.1" ||
		len(v.Loaders) != 1 || v.Loaders[0] != "neoforge" {
		t.Fatalf("compat: gv=%v loaders=%v", v.GameVersions, v.Loaders)
	}
	if len(v.Dependencies) != 1 || v.Dependencies[0].Title != "Dep Mod" || !v.Dependencies[0].Required {
		t.Fatalf("deps: %+v", v.Dependencies)
	}
}
