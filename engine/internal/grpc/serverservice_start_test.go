package grpcsvc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type blockingStartBackend struct {
	fakeBackend
	started chan struct{}
}

func (b *blockingStartBackend) Start(ctx context.Context, spec isolation.InstanceSpec) (isolation.Instance, error) {
	b.startSpec = spec
	close(b.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

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

func TestDeleteCancelsAndWaitsForStart(t *testing.T) {
	svc, st, paths, backend := startRaceService(t)
	startDone := make(chan error, 1)
	go func() {
		_, err := svc.Start(context.Background(), &mcmanagerv1.ServerId{Id: "s1"})
		startDone <- err
	}()
	waitForStart(t, backend.started)

	if _, err := svc.Delete(context.Background(), &mcmanagerv1.ServerId{Id: "s1"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := <-startDone; status.Code(err) != codes.Canceled {
		t.Fatalf("Start error = %v, want context canceled", err)
	}
	if _, ok := st.Get("s1"); ok {
		t.Fatal("canceled Start resurrected deleted server")
	}
	if _, err := os.Stat(paths.ServerDir("s1")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("server directory still exists after delete: %v", err)
	}
}

func TestRemoveAllDataCancelsAndWaitsForStart(t *testing.T) {
	svc, st, paths, backend := startRaceService(t)
	startDone := make(chan error, 1)
	go func() {
		_, err := svc.Start(context.Background(), &mcmanagerv1.ServerId{Id: "s1"})
		startDone <- err
	}()
	waitForStart(t, backend.started)

	if _, err := svc.RemoveAllData(context.Background(), &mcmanagerv1.Empty{}); err != nil {
		t.Fatalf("RemoveAllData: %v", err)
	}
	if err := <-startDone; status.Code(err) != codes.Canceled {
		t.Fatalf("Start error = %v, want context canceled", err)
	}
	if _, ok := st.Get("s1"); ok {
		t.Fatal("canceled Start resurrected server after RemoveAllData")
	}
	if _, err := os.Stat(paths.ServersRoot()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("servers root still exists after RemoveAllData: %v", err)
	}
}

func TestStopCancelsAndWaitsForStart(t *testing.T) {
	svc, st, _, backend := startRaceService(t)
	startDone := make(chan error, 1)
	go func() {
		_, err := svc.Start(context.Background(), &mcmanagerv1.ServerId{Id: "s1"})
		startDone <- err
	}()
	waitForStart(t, backend.started)

	if _, err := svc.Stop(context.Background(), &mcmanagerv1.ServerId{Id: "s1"}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := <-startDone; status.Code(err) != codes.Canceled {
		t.Fatalf("Start error = %v, want context canceled", err)
	}
	rec, ok := st.Get("s1")
	if !ok {
		t.Fatal("Stop deleted the server record")
	}
	if rec.Status != mcmanagerv1.ServerStatus_STOPPED {
		t.Fatalf("status = %v, want STOPPED", rec.Status)
	}
	if _, running := svc.Instance("s1"); running {
		t.Fatal("canceled Start published a running instance after Stop")
	}
}

func startRaceService(t *testing.T) (*ServerService, *store.Memory, appdata.Paths, *blockingStartBackend) {
	t.Helper()
	paths := appdata.New(t.TempDir())
	cacheJava(t, paths.JRECache(), 21)
	if err := os.MkdirAll(paths.ServerDir("s1"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	_ = st.Put(&store.Server{
		ID: "s1", Name: "s1", McVersion: "1.21.1", MemoryMB: 1024,
		Port: 25565, Status: mcmanagerv1.ServerStatus_STOPPED,
		JavaMajor: 21, LaunchArgs: []string{"-jar", "server.jar", "nogui"},
	})
	backend := &blockingStartBackend{started: make(chan struct{})}
	svc := NewServerService(ServerServiceConfig{
		Store: st, JRE: jre.NewManager(paths.JRECache()), Backend: backend, Paths: paths,
	})
	return svc, st, paths, backend
}

func waitForStart(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not reach the backend")
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
