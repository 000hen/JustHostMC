package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/provider"
	"github.com/000hen/justhostmc/engine/internal/store"
)

func TestUpdateRenamesAndReordersRunningServer(t *testing.T) {
	st := store.NewMemory()
	_ = st.Put(&store.Server{
		ID: "s1", Name: "Old", Type: mcmanagerv1.ServerType_VANILLA, McVersion: "1.21",
		MemoryMB: 2048, Port: 25565, Status: mcmanagerv1.ServerStatus_RUNNING,
		SortOrder: 2, LaunchArgs: []string{"-jar", "server.jar", "nogui"},
	})

	svc := NewServerService(ServerServiceConfig{Store: st})
	got, err := svc.Update(context.Background(), &mcmanagerv1.UpdateServerRequest{
		Id: "s1", Name: "New", McVersion: "1.21", Port: 25565, SortOrder: 0,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "New" || got.SortOrder != 0 {
		t.Fatalf("updated server = %+v, want name New and sort order 0", got)
	}
}

func TestUpdatePortPreservesServerProperties(t *testing.T) {
	paths := appdata.New(t.TempDir())
	dir := paths.ServerDir("s1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	newPort := resolvePort(0)
	oldPort := newPort + 1
	if err := os.WriteFile(filepath.Join(dir, "server.properties"),
		[]byte("motd=Hello\nserver-port="+strconv.Itoa(oldPort)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	_ = st.Put(&store.Server{
		ID: "s1", Name: "One", Type: mcmanagerv1.ServerType_VANILLA, McVersion: "1.21",
		MemoryMB: 2048, Port: oldPort, Status: mcmanagerv1.ServerStatus_STOPPED,
		LaunchArgs: []string{"-jar", "server.jar", "nogui"},
	})

	svc := NewServerService(ServerServiceConfig{Store: st, Paths: paths})
	if _, err := svc.Update(context.Background(), &mcmanagerv1.UpdateServerRequest{
		Id: "s1", Name: "One", McVersion: "1.21", Port: int32(newPort),
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "server.properties"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "motd=Hello") ||
		!strings.Contains(text, "server-port="+strconv.Itoa(newPort)) ||
		!strings.Contains(text, "query.port="+strconv.Itoa(newPort)) {
		t.Fatalf("server.properties = %q", text)
	}
}

func TestUpdateLaunchSettingsPersistsMemoryAndCustomArgs(t *testing.T) {
	st := store.NewMemory()
	_ = st.Put(&store.Server{
		ID: "s1", Name: "One", Type: mcmanagerv1.ServerType_VANILLA, McVersion: "1.21",
		MemoryMB: 2048, Port: 25565, Status: mcmanagerv1.ServerStatus_STOPPED,
		LaunchArgs: []string{"-jar", "server.jar", "nogui"},
	})

	svc := NewServerService(ServerServiceConfig{Store: st})
	got, err := svc.Update(context.Background(), &mcmanagerv1.UpdateServerRequest{
		Id: "s1", Name: "One", McVersion: "1.21", Port: 25565,
		MemoryMb: 4096, CustomJavaArgs: "-Ddemo=true -XX:MaxGCPauseMillis=150",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.MemoryMb != 4096 || got.CustomJavaArgs != "-Ddemo=true -XX:MaxGCPauseMillis=150" {
		t.Fatalf("updated server = %+v, want memory and custom Java args", got)
	}
	rec, _ := st.Get("s1")
	if rec.MemoryMB != 4096 || rec.CustomJavaArgs != "-Ddemo=true -XX:MaxGCPauseMillis=150" {
		t.Fatalf("record = %+v, want persisted launch settings", rec)
	}
}

func TestUpdateVersionRefreshesLaunchSpec(t *testing.T) {
	paths := appdata.New(t.TempDir())
	if err := os.MkdirAll(paths.ServerDir("s1"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	_ = st.Put(&store.Server{
		ID: "s1", Name: "One", Type: mcmanagerv1.ServerType_VANILLA, McVersion: "1.20.1",
		MemoryMB: 2048, Port: 25565, Status: mcmanagerv1.ServerStatus_STOPPED,
		JavaMajor: 17, LaunchArgs: []string{"-jar", "old.jar"},
	})
	prov := &installStubProvider{spec: provider.LaunchSpec{
		JavaMajor: 21,
		Args:      []string{"-jar", "server.jar", "nogui"},
	}}

	svc := NewServerService(ServerServiceConfig{
		Store:     st,
		Providers: map[mcmanagerv1.ServerType]provider.Provider{mcmanagerv1.ServerType_VANILLA: prov},
		Paths:     paths,
	})
	if _, err := svc.Update(context.Background(), &mcmanagerv1.UpdateServerRequest{
		Id: "s1", Name: "One", McVersion: "1.21", Port: 25565,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	rec, _ := st.Get("s1")
	if rec.McVersion != "1.21" || rec.JavaMajor != 21 || len(rec.LaunchArgs) != 3 {
		t.Fatalf("record = %+v, want refreshed version, Java major, and launch args", rec)
	}
}
