package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/store"
)

func TestRemoveAllDataWipesEverything(t *testing.T) {
	paths := appdata.New(t.TempDir())
	reg := store.NewMemory()
	_ = reg.Put(&store.Server{ID: "s1", Name: "One", Status: mcmanagerv1.ServerStatus_STOPPED})

	// Seed files across every data root plus the registry db at the base.
	seed := map[string]string{
		filepath.Join(paths.ServerDir("s1"), "server.properties"): "x",
		filepath.Join(paths.BackupsRoot(), "s1", "b.zip"):         "x",
		filepath.Join(paths.LogsRoot(), "s1", "console.log"):      "x",
		filepath.Join(paths.JRECache(), "21", "java.exe"):         "x",
		filepath.Join(paths.Base, "registry.db"):                  "db",
	}
	for p, c := range seed {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	closedLogs := false
	svc := NewServerService(ServerServiceConfig{
		Store:     reg,
		Backend:   &fakeBackend{},
		Paths:     paths,
		CloseLogs: func() { closedLogs = true },
	})

	if _, err := svc.RemoveAllData(context.Background(), &mcmanagerv1.Empty{}); err != nil {
		t.Fatalf("RemoveAllData: %v", err)
	}

	if !closedLogs {
		t.Error("RemoveAllData should release log handles before wiping logs")
	}
	if len(reg.List()) != 0 {
		t.Errorf("registry not cleared: %d records remain", len(reg.List()))
	}
	for _, dir := range []string{paths.ServersRoot(), paths.BackupsRoot(), paths.LogsRoot(), paths.JRECache()} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed (err=%v)", dir, err)
		}
	}
	// The base dir is recreated so the engine keeps working.
	if _, err := os.Stat(paths.Base); err != nil {
		t.Errorf("base dir should still exist: %v", err)
	}
}
