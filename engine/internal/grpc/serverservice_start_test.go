package grpcsvc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/store"
)

func TestStartUpgradesStaleJavaMajorForCurrentMinecraft(t *testing.T) {
	paths := appdata.New(t.TempDir())
	cacheJava(t, paths.JRECache(), 25)

	st := store.NewMemory()
	_ = st.Put(&store.Server{
		ID: "s1", Name: "s1", ProviderID: "fabric", McVersion: "26.2",
		MemoryMB: 1024, Port: 25565, Status: mcmanagerv1.ServerStatus_STOPPED,
		JavaMajor: 21, LaunchArgs: []string{"-jar", "server.jar", "nogui"},
	})

	backend := &fakeBackend{}
	svc := NewServerService(ServerServiceConfig{
		Store:   st,
		JRE:     jre.NewManager(paths.JRECache()),
		Backend: backend,
		Paths:   paths,
	})

	if _, err := svc.Start(context.Background(), &mcmanagerv1.ServerId{Id: "s1"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer close(backend.startInst.done)

	if backend.startSpec.JavaMajor != 25 {
		t.Fatalf("JavaMajor = %d, want upgraded Java 25", backend.startSpec.JavaMajor)
	}
	if !strings.HasPrefix(backend.startSpec.JavaPath, filepath.Join(paths.JRECache(), "25")) {
		t.Fatalf("JavaPath = %q, want cached Java 25", backend.startSpec.JavaPath)
	}
	rec, _ := st.Get("s1")
	if rec.JavaMajor != 25 {
		t.Fatalf("persisted JavaMajor = %d, want 25", rec.JavaMajor)
	}
}

func cacheJava(t *testing.T, cache string, major int) {
	t.Helper()
	binDir := filepath.Join(cache, fmt.Sprint(major), fmt.Sprintf("jdk-%d-jre", major), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "java.exe"), []byte("fake java"), 0o755); err != nil {
		t.Fatal(err)
	}
}
