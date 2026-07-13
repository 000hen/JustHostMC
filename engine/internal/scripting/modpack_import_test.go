package scripting

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/provider"
)

func TestImportProviderHidden(t *testing.T) {
	e := builtinProvider(t, "import")
	if !e.Meta.Hidden {
		t.Error("import provider should be hidden")
	}
	if e.Meta.ModLayout != "mods" {
		t.Errorf("mod_layout = %q, want mods", e.Meta.ModLayout)
	}
	var hasKey bool
	for _, c := range e.Meta.Config {
		if c.Key == "curseforge_api_key" && c.Type == mcmanagerv1.ConfigOptionType_CONFIG_OPTION_SECRET {
			hasKey = true
		}
	}
	if !hasKey {
		t.Errorf("import should declare a curseforge_api_key secret, config=%+v", e.Meta.Config)
	}
}

// stageImportZip writes the staged import file the "import" provider reads.
func stageImportZip(t *testing.T, dir string, entries map[string]string) {
	t.Helper()
	writeTestJar(t, filepath.Join(dir, ".jhmc", "import.zip"), entries)
}

func TestImportProviderRejectsUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	stageImportZip(t, dir, map[string]string{"random.txt": "x"})
	e := builtinProvider(t, "import")
	_, err := e.Provider.Install(context.Background(), dir, "import/local", nil)
	if err == nil || !strings.Contains(err.Error(), "not a recognized modpack") {
		t.Fatalf("want unrecognized-format error, got %v", err)
	}
}

func TestImportProviderCurseForgeNeedsKey(t *testing.T) {
	dir := t.TempDir()
	stageImportZip(t, dir, map[string]string{
		"manifest.json": `{"minecraft":{"version":"1.20.1","modLoaders":[{"id":"forge-47.2.0","primary":true}]},"name":"P","version":"1","files":[]}`,
	})
	e := builtinProvider(t, "import")
	_, err := e.Provider.Install(context.Background(), dir, "import/local", nil)
	if err == nil || !strings.Contains(err.Error(), "CurseForge API key") {
		t.Fatalf("want CurseForge-key error, got %v", err)
	}
}

// TestImportProviderUpdateUnsupported confirms the import provider yields
// ErrUpdateUnsupported — imported packs can be exported but not updated (a local
// file has no upstream to diff against), which is how the RPC answers
// Unimplemented for them.
func TestImportProviderUpdateUnsupported(t *testing.T) {
	e := builtinProvider(t, "import")
	up, ok := e.Provider.(provider.Updater)
	if !ok {
		t.Fatal("Lua providers expose Update; import should report it unsupported at runtime")
	}
	_, err := up.Update(context.Background(), t.TempDir(), "import/local", "import/local", nil)
	if !errors.Is(err, provider.ErrUpdateUnsupported) {
		t.Fatalf("import Update err = %v, want ErrUpdateUnsupported", err)
	}
}
