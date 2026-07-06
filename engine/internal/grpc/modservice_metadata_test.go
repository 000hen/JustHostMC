package grpcsvc

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/store"
)

// countingParser wraps a ParserSet and counts ParseJar calls (for cache tests).
type countingParser struct {
	inner *scripting.ParserSet
	calls atomic.Int64
}

func (c *countingParser) ParseJar(ctx context.Context, serverDir, jarRel string) (scripting.ModMeta, string, bool, error) {
	c.calls.Add(1)
	return c.inner.ParseJar(ctx, serverDir, jarRel)
}

func writeJar(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func newMetadataTestService(t *testing.T) (*ModService, *countingParser, appdata.Paths) {
	t.Helper()
	st := store.NewMemory()
	paths := appdata.Paths{Base: t.TempDir()}
	_ = st.Put(&store.Server{ID: "s1", ProviderID: "fabric", ModLayout: "mods", McVersion: "1.20.1", Status: mcmanagerv1.ServerStatus_STOPPED})

	ps := scripting.NewParserSet(scripting.NewHost(nil, nil, nil), nil)
	if err := scripting.LoadBuiltinParsers(ps); err != nil {
		t.Fatalf("LoadBuiltinParsers: %v", err)
	}
	parser := &countingParser{inner: ps}
	return NewModService(st, paths, parser), parser, paths
}

func TestListPopulatesMetadata(t *testing.T) {
	svc, _, paths := newMetadataTestService(t)
	writeJar(t, filepath.Join(paths.ServerDir("s1"), "mods", "example.jar"), map[string]string{
		"fabric.mod.json": `{"id":"example","name":"Example","version":"1.2","authors":["Alice"],"description":"d","contact":{"homepage":"https://e.x"},"icon":"icon.png","depends":{"minecraft":">=1.20 <1.21"}}`,
		"icon.png":        "PNGBYTES",
	})
	// A jar no parser understands lists with Parsed=false.
	writeJar(t, filepath.Join(paths.ServerDir("s1"), "mods", "opaque.jar"), map[string]string{"a.class": "x"})
	// A corrupt jar is retained as a failed row rather than failing the whole list.
	if err := os.WriteFile(filepath.Join(paths.ServerDir("s1"), "mods", "broken.jar"), []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := svc.List(context.Background(), &mcmanagerv1.ServerId{Id: "s1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Files) != 3 {
		t.Fatalf("files = %d", len(list.Files))
	}
	byName := map[string]*mcmanagerv1.ModFile{}
	for _, f := range list.Files {
		byName[f.Name] = f
	}
	m := byName["example.jar"].Metadata
	if m == nil || !m.Parsed || m.ParserId != "parser-fabric" || m.Loader != "fabric" ||
		m.ModId != "example" || m.Name != "Example" || m.Version != "1.2" ||
		m.GameVersionRequirement != ">=1.20 <1.21" || m.LoaderMismatch || m.GameVersionMismatch ||
		len(m.Authors) != 1 || m.Authors[0] != "Alice" || m.Website != "https://e.x" ||
		string(m.Icon) != "PNGBYTES" {
		t.Errorf("example.jar metadata = %+v", m)
	}
	if om := byName["opaque.jar"].Metadata; om == nil || om.Parsed {
		t.Errorf("opaque.jar metadata = %+v, want Parsed=false", om)
	}
	if bm := byName["broken.jar"].Metadata; bm == nil || bm.Parsed || bm.ParseError == "" {
		t.Errorf("broken.jar metadata = %+v, want a per-file parse error", bm)
	}
}

func TestListMarksTypeAndVersionMismatches(t *testing.T) {
	svc, _, paths := newMetadataTestService(t)
	writeJar(t, filepath.Join(paths.ServerDir("s1"), "mods", "wrong.jar"), map[string]string{
		"META-INF/mods.toml": `
[[mods]]
modId = "wrong"
version = "1"
[[dependencies.wrong]]
modId = "minecraft"
versionRange = "[1.19,1.20)"
`,
	})

	list, err := svc.List(context.Background(), &mcmanagerv1.ServerId{Id: "s1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	meta := list.Files[0].Metadata
	if meta == nil || !meta.LoaderMismatch || !meta.GameVersionMismatch {
		t.Fatalf("compatibility metadata = %+v", meta)
	}
}

func TestListMetadataCache(t *testing.T) {
	svc, parser, paths := newMetadataTestService(t)
	jar := filepath.Join(paths.ServerDir("s1"), "mods", "example.jar")
	writeJar(t, jar, map[string]string{"fabric.mod.json": `{"id":"example","version":"1"}`})

	for i := 0; i < 3; i++ {
		if _, err := svc.List(context.Background(), &mcmanagerv1.ServerId{Id: "s1"}); err != nil {
			t.Fatalf("List #%d: %v", i, err)
		}
	}
	if got := parser.calls.Load(); got != 1 {
		t.Errorf("ParseJar calls = %d, want 1 (cache hit on repeats)", got)
	}

	// Rewriting the jar bumps its mtime → cache invalidates → re-parse.
	writeJar(t, jar, map[string]string{"fabric.mod.json": `{"id":"example","version":"2"}`})
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(jar, future, future); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(context.Background(), &mcmanagerv1.ServerId{Id: "s1"})
	if err != nil {
		t.Fatalf("List after rewrite: %v", err)
	}
	if got := parser.calls.Load(); got != 2 {
		t.Errorf("ParseJar calls after rewrite = %d, want 2", got)
	}
	if list.Files[0].Metadata.Version != "2" {
		t.Errorf("version after rewrite = %q", list.Files[0].Metadata.Version)
	}
}

func TestListNilParserDegrades(t *testing.T) {
	st := store.NewMemory()
	paths := appdata.Paths{Base: t.TempDir()}
	_ = st.Put(&store.Server{ID: "s1", ProviderID: "paper", ModLayout: "plugins", Status: mcmanagerv1.ServerStatus_STOPPED})
	writeJar(t, filepath.Join(paths.ServerDir("s1"), "plugins", "p.jar"), map[string]string{"plugin.yml": "name: X\nversion: '1'"})

	svc := NewModService(st, paths, nil)
	list, err := svc.List(context.Background(), &mcmanagerv1.ServerId{Id: "s1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if m := list.Files[0].Metadata; m == nil || m.Parsed {
		t.Errorf("metadata with nil parser = %+v, want Parsed=false", m)
	}
}
