//go:build windows

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/backup"
	"github.com/000hen/justhostmc/engine/internal/console"
	grpcsvc "github.com/000hen/justhostmc/engine/internal/grpc"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/jre"
	"github.com/000hen/justhostmc/engine/internal/store"
)

// TestSafeOnlineBackupEndToEnd boots a real vanilla server, takes a safe-online
// backup while it is running (PROMPT §10.4: pause saves, flush, snapshot, resume),
// asserts the server never stopped, and restores the snapshot — the M5 acceptance.
func TestSafeOnlineBackupEndToEnd(t *testing.T) {
	if os.Getenv("JHMC_INTEGRATION") != "1" {
		t.Skip("set JHMC_INTEGRATION=1 to run (downloads ~100 MB and runs a real server)")
	}

	const (
		id      = "backup-itest"
		version = "1.21.1"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	paths := appdata.New(t.TempDir())
	dir := paths.ServerDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Install the server + JRE and boot it.
	spec, err := vanillaProvider(t).Install(ctx, dir, version, nil)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	javaPath, err := jre.NewManager(paths.JRECache()).EnsureJRE(ctx, spec.JavaMajor, nil)
	if err != nil {
		t.Fatalf("EnsureJRE: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "eula.txt"), []byte("eula=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte("server-port=25603\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := isolation.NewJobObjectBackend()
	inst, err := backend.Start(ctx, isolation.InstanceSpec{
		ID:       id,
		Dir:      dir,
		JavaPath: javaPath,
		Args:     append([]string{"-Xmx1024M"}, spec.Args...),
		MemoryMB: 2048,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 2. Read output directly until the server is ready (the hub is attached after,
	//    so there is only ever one reader of the output channel).
	waitFor(t, inst, "Done (")

	// 3. Attach the console hub and wire the BackupService over a RUNNING record.
	hub := console.NewHub()
	hub.Register(id, inst)
	reg := store.NewMemory()
	_ = reg.Put(&store.Server{ID: id, Name: "Backup ITest", Status: mcmanagerv1.ServerStatus_RUNNING})

	svc := grpcsvc.NewBackupService(grpcsvc.BackupServiceConfig{
		Manager: backup.NewManager(paths.BackupsRoot()),
		Store:   reg,
		Paths:   paths,
		Console: hub,
	})

	// 4. Take a safe-online backup. The server must keep running throughout.
	b, err := svc.Create(ctx, &mcmanagerv1.CreateBackupRequest{ServerId: id, SafeOnline: true})
	if err != nil {
		t.Fatalf("Create (safe online): %v", err)
	}
	if b.SizeBytes <= 0 {
		t.Fatalf("backup is empty: %+v", b)
	}
	if !inst.Running() {
		t.Fatal("server stopped during a safe-online backup (expected zero downtime)")
	}
	t.Logf("safe-online backup %s: %d bytes, server still running", b.Id, b.SizeBytes)

	// 5. Stop the server, then restore the snapshot world-consistently.
	if err := backend.Stop(ctx, id, true); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-inst.Done():
	case <-time.After(90 * time.Second):
		t.Fatal("server did not stop")
	}

	rec, _ := reg.Get(id)
	rec.Status = mcmanagerv1.ServerStatus_STOPPED
	_ = reg.Put(rec)

	if _, err := svc.Restore(ctx, &mcmanagerv1.RestoreBackupRequest{ServerId: id, BackupId: b.Id}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	// The restored data folder must contain the world that was flushed before the snapshot.
	if _, err := os.Stat(filepath.Join(dir, "world", "level.dat")); err != nil {
		t.Errorf("restored backup is missing world/level.dat: %v", err)
	}
}

// waitFor reads an instance's output until a line contains want (or it times out).
func waitFor(t *testing.T, inst isolation.Instance, want string) {
	t.Helper()
	deadline := time.After(6 * time.Minute)
	for {
		select {
		case line, ok := <-inst.Output():
			if !ok {
				t.Fatalf("output closed before %q", want)
			}
			if strings.Contains(line, want) {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q", want)
		}
	}
}
