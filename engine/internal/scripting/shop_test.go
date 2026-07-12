package scripting

import (
	"context"
	"errors"
	"testing"
)

// testShopSrc is a minimal but complete shop script used across the tests.
// It echoes its ctx back so tests can assert what the adapter passed in.
const testShopSrc = `
meta = {
  id = "testshop",
  name = "Test Shop",
  version = "1.0",
  permissions = { { kind = "network", reason = "talk to the fake api" } },
}

function home(ctx)
  return { sections = { { title_key = "shop.home.popular", projects = {
    { project_id = "p1", title = "Alpha", downloads = 42 },
  } } } }
end

function search(ctx)
  return {
    projects = {
      { project_id = ctx.query, slug = "s", title = "T", summary = "sum",
        icon_url = "http://i/x.png", author = "au", downloads = 7, follows = 3,
        categories = { "tech" }, project_type = "mod" },
    },
    total = 123,
  }
end

function detail(ctx)
  return {
    project = { project_id = ctx.project_id, title = "Detail" },
    body = "# hi",
    body_format = "markdown",
    gallery = { { url = "http://g/1.png", title = "shot" } },
    links = { website = "http://w", source = "http://s" },
    game_versions = { "26.1" },
    loaders = { "fabric" },
    license = "MIT",
    updated = "2026-01-01T00:00:00Z",
  }
end

function versions(ctx)
  return { versions = { {
    id = "v1", name = "One", version_number = "1.0.0", channel = "release",
    game_versions = { ctx.mc_version }, loaders = { ctx.loader },
    date = "2026-01-01T00:00:00Z", downloads = 5,
    filename = "one.jar", size = 100,
    dependencies = { { project_id = "dep1", title = "Dep", required = true } },
  } } }
end

function resolve_file(ctx)
  if ctx.project_id == "gone" then error("project not found") end
  if ctx.project_id == "nodist" then error("file not distributable") end
  return { url = "http://f/one.jar", filename = "one.jar", size = 100, sha1 = "aa" }
end
`

const categoryShopSrc = `
meta = { id = "category-shop", name = "Category Shop" }
function categories(ctx)
  return { categories = {
    { id = "12", name = "Technology", slug = "technology",
      localization_key = "shop.category.curseforge.technology" },
  } }
end
function search(ctx)
  return { projects = {{ project_id = "p", title = "P",
    distribution = "website_only" }}, total = 1 }
end
function home(ctx) return { sections = {} } end
function detail(ctx) return { project = { project_id = ctx.project_id } } end
function versions(ctx) return { versions = {} } end
function resolve_file(ctx)
  return { url = "https://example.test/x.jar", filename = "x.jar" }
end
`

func newTestShopSet(t *testing.T, keyFn func(string) string) *ShopSet {
	t.Helper()
	return NewShopSet(NewHost(nil, nil, nil), nil, keyFn)
}

func TestShopContract(t *testing.T) {
	ss := newTestShopSet(t, nil)
	if _, err := ss.AddSource(context.Background(), `meta = { id = "x" }`, true); err == nil {
		t.Fatal("script without shop functions must be rejected")
	}
	s, err := ss.AddSource(context.Background(), testShopSrc, true)
	if err != nil {
		t.Fatal(err)
	}
	if s.Meta().ID != "testshop" || s.meta.NeedsKey {
		t.Fatalf("meta: %+v", s.Meta())
	}
	if !s.Ready() {
		t.Fatal("keyless shop must be ready")
	}
}

func TestShopSearchMapsResult(t *testing.T) {
	ss := newTestShopSet(t, nil)
	s, err := ss.AddSource(context.Background(), testShopSrc, true)
	if err != nil {
		t.Fatal(err)
	}
	page, err := s.Search(context.Background(), ShopQuery{Query: "sodium", Offset: 20, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 123 || page.Offset != 20 || len(page.Projects) != 1 {
		t.Fatalf("page: %+v", page)
	}
	p := page.Projects[0]
	if p.ID != "sodium" || p.Downloads != 7 || p.Follows != 3 ||
		len(p.Categories) != 1 || p.Categories[0] != "tech" || p.ProjectType != "mod" {
		t.Fatalf("project: %+v", p)
	}
}

func TestShopCategoriesMapsOptionalFunction(t *testing.T) {
	ss := newTestShopSet(t, nil)
	shop, err := ss.AddSource(context.Background(), categoryShopSrc, true)
	if err != nil {
		t.Fatal(err)
	}
	categories, err := shop.Categories(context.Background(), "mod")
	if err != nil {
		t.Fatal(err)
	}
	want := ShopCategory{
		ID:              "12",
		Name:            "Technology",
		Slug:            "technology",
		LocalizationKey: "shop.category.curseforge.technology",
	}
	if len(categories) != 1 || categories[0] != want {
		t.Fatalf("categories = %#v, want %#v", categories, want)
	}
}

func TestShopCategoriesMissingFunctionIsEmpty(t *testing.T) {
	ss := newTestShopSet(t, nil)
	shop, err := ss.AddSource(context.Background(), testShopSrc, true)
	if err != nil {
		t.Fatal(err)
	}
	categories, err := shop.Categories(context.Background(), "mod")
	if err != nil || len(categories) != 0 {
		t.Fatalf("categories = %#v, err = %v", categories, err)
	}
}

func TestShopProjectMapsDistribution(t *testing.T) {
	ss := newTestShopSet(t, nil)
	shop, err := ss.AddSource(context.Background(), categoryShopSrc, true)
	if err != nil {
		t.Fatal(err)
	}
	page, err := shop.Search(context.Background(), ShopQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if got := page.Projects[0].Distribution; got != ShopDistributionWebsiteOnly {
		t.Fatalf("distribution = %q", got)
	}
}

func TestShopHomeDetailVersions(t *testing.T) {
	ss := newTestShopSet(t, nil)
	s, _ := ss.AddSource(context.Background(), testShopSrc, true)
	ctx := context.Background()

	secs, err := s.Home(ctx, ShopQuery{})
	if err != nil || len(secs) != 1 || secs[0].TitleKey != "shop.home.popular" || len(secs[0].Projects) != 1 {
		t.Fatalf("home: %+v err=%v", secs, err)
	}

	d, err := s.Detail(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Project.ID != "p1" || d.BodyFormat != "markdown" || len(d.Gallery) != 1 ||
		d.Links.Website != "http://w" || d.License != "MIT" {
		t.Fatalf("detail: %+v", d)
	}

	vs, err := s.Versions(ctx, "p1", "26.1", "fabric")
	if err != nil || len(vs) != 1 {
		t.Fatalf("versions: %+v err=%v", vs, err)
	}
	v := vs[0]
	if v.Channel != "release" || v.GameVersions[0] != "26.1" || v.Loaders[0] != "fabric" ||
		len(v.Dependencies) != 1 || !v.Dependencies[0].Required {
		t.Fatalf("version: %+v", v)
	}
}

func TestShopResolveFileAndErrorBridging(t *testing.T) {
	ss := newTestShopSet(t, nil)
	s, _ := ss.AddSource(context.Background(), testShopSrc, true)
	ctx := context.Background()

	f, err := s.ResolveFile(ctx, "p1", "v1", "26.1", "fabric")
	if err != nil || f.URL == "" || f.Filename != "one.jar" || f.SHA1 != "aa" {
		t.Fatalf("file: %+v err=%v", f, err)
	}

	if _, err := s.ResolveFile(ctx, "gone", "", "", ""); !errors.Is(err, ErrShopNotFound) {
		t.Fatalf("want ErrShopNotFound, got %v", err)
	}
	if _, err := s.ResolveFile(ctx, "nodist", "", "", ""); !errors.Is(err, ErrShopNotDistributable) {
		t.Fatalf("want ErrShopNotDistributable, got %v", err)
	}
}

func TestShopNeedsKeyGate(t *testing.T) {
	keyed := testShopSrc + "\nmeta.needs_key = true\n"
	key := ""
	ss := newTestShopSet(t, func(id string) string { return key })
	s, err := ss.AddSource(context.Background(), keyed, true)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Meta().NeedsKey || s.Ready() {
		t.Fatalf("needs_key shop without key must not be ready")
	}
	if _, err := s.Search(context.Background(), ShopQuery{}); !errors.Is(err, ErrShopKeyMissing) {
		t.Fatalf("want ErrShopKeyMissing, got %v", err)
	}
	key = "k123"
	if !s.Ready() {
		t.Fatal("shop with key must be ready")
	}
	if _, err := s.Search(context.Background(), ShopQuery{}); err != nil {
		t.Fatalf("keyed search failed: %v", err)
	}
}

func TestShopUserCannotShadowBuiltin(t *testing.T) {
	ss := newTestShopSet(t, nil)
	if _, err := ss.AddSource(context.Background(), testShopSrc, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ss.AddSource(context.Background(), testShopSrc, false); !errors.Is(err, ErrProviderIDConflict) {
		t.Fatalf("want ErrProviderIDConflict, got %v", err)
	}
}
