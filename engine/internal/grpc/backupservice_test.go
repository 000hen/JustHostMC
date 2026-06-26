package grpcsvc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/backup"
	"github.com/000hen/justhostmc/engine/internal/console"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errorCodeOf extracts the typed ErrorCode the backend packs into a gRPC status's
// details (PROMPT §4.1, §14), or ERROR_CODE_UNSPECIFIED if none is present.
func errorCodeOf(err error) mcmanagerv1.ErrorCode {
	st, ok := status.FromError(err)
	if !ok {
		return mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED
	}
	for _, d := range st.Details() {
		if det, ok := d.(*mcmanagerv1.ErrorDetail); ok {
			return det.Code
		}
	}
	return mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED
}

func TestSafeOnlineSnapshotPausesFlushesResumes(t *testing.T) {
	hub := console.NewHub()
	fake := &fakeInstance{out: make(chan string, 16), done: make(chan struct{}), emitSaved: true}
	hub.Register("srv1", fake)

	resumedAtSnapshot := true
	snapshot := func() error {
		// At snapshot time saves must be paused+flushed but not yet resumed.
		if !fake.wrote("save-off") || !fake.wrote("save-all flush") {
			t.Errorf("snapshot ran without pausing+flushing saves first")
		}
		resumedAtSnapshot = fake.wrote("save-on")
		return nil
	}
	if err := safeOnlineSnapshot(context.Background(), hub, "srv1", snapshot, 5*time.Second); err != nil {
		t.Fatalf("safeOnlineSnapshot: %v", err)
	}
	if resumedAtSnapshot {
		t.Error("saves were resumed before the snapshot ran")
	}
	if !fake.wrote("save-on") {
		t.Error("saves were not resumed after the snapshot")
	}
}

func TestSafeOnlineSnapshotResumesSavesOnError(t *testing.T) {
	hub := console.NewHub()
	fake := &fakeInstance{out: make(chan string, 16), done: make(chan struct{}), emitSaved: true}
	hub.Register("srv1", fake)

	wantErr := errors.New("disk full")
	err := safeOnlineSnapshot(context.Background(), hub, "srv1", func() error { return wantErr }, 5*time.Second)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if !fake.wrote("save-on") {
		t.Error("save-on must be sent even when the snapshot fails")
	}
}

func TestSafeOnlineSnapshotProceedsAfterFlushTimeout(t *testing.T) {
	hub := console.NewHub()
	fake := &fakeInstance{out: make(chan string, 16), done: make(chan struct{})} // never emits "Saved the game"
	hub.Register("srv1", fake)

	done := make(chan struct{})
	snapped := false
	go func() {
		_ = safeOnlineSnapshot(context.Background(), hub, "srv1",
			func() error { snapped = true; return nil }, 150*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("safeOnlineSnapshot did not return after the flush timeout")
	}
	if !snapped {
		t.Error("snapshot should still run (best effort) after a flush timeout")
	}
	if !fake.wrote("save-on") {
		t.Error("save-on must be sent after timeout")
	}
}

// newBackupService builds a BackupService over a temp data dir and seeds one
// server record with the given status and a small world to snapshot.
func newBackupService(t *testing.T, st mcmanagerv1.ServerStatus, hub *console.Hub) (*BackupService, BackupServiceConfig, string) {
	t.Helper()
	paths := appdata.New(t.TempDir())
	reg := store.NewMemory()
	const id = "srv1"
	_ = reg.Put(&store.Server{ID: id, Name: "Test", Status: st})

	dir := paths.ServerDir(id)
	if err := os.MkdirAll(filepath.Join(dir, "world"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "world", "level.dat"), []byte("WORLD"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := BackupServiceConfig{
		Manager: backup.NewManager(paths.BackupsRoot()),
		Store:   reg,
		Paths:   paths,
		Console: hub,
	}
	return NewBackupService(cfg), cfg, id
}

func TestBackupServiceCreateAndList(t *testing.T) {
	svc, _, id := newBackupService(t, mcmanagerv1.ServerStatus_STOPPED, nil)
	ctx := context.Background()

	b, err := svc.Create(ctx, &mcmanagerv1.CreateBackupRequest{ServerId: id})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if b.Id == "" || b.SizeBytes <= 0 || b.ServerId != id || b.CreatedAt == "" {
		t.Fatalf("bad backup: %+v", b)
	}

	list, err := svc.List(ctx, &mcmanagerv1.ServerId{Id: id})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Backups) != 1 || list.Backups[0].Id != b.Id {
		t.Fatalf("List = %+v, want one entry %s", list.Backups, b.Id)
	}
}

func TestBackupServiceRestoreRejectsRunningServer(t *testing.T) {
	svc, _, id := newBackupService(t, mcmanagerv1.ServerStatus_RUNNING, nil)
	_, err := svc.Restore(context.Background(), &mcmanagerv1.RestoreBackupRequest{ServerId: id, BackupId: "whatever"})
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Fatalf("Restore on running server code = %v, want FailedPrecondition", code)
	}
	if ec := errorCodeOf(err); ec != mcmanagerv1.ErrorCode_SERVER_RUNNING {
		t.Errorf("ErrorCode = %v, want SERVER_RUNNING", ec)
	}
}

func TestBackupServiceRestoreRoundTrip(t *testing.T) {
	svc, cfg, id := newBackupService(t, mcmanagerv1.ServerStatus_STOPPED, nil)
	ctx := context.Background()

	b, err := svc.Create(ctx, &mcmanagerv1.CreateBackupRequest{ServerId: id})
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the live world, then restore and confirm it reverts.
	dir := cfg.Paths.ServerDir(id)
	if err := os.WriteFile(filepath.Join(dir, "world", "level.dat"), []byte("CORRUPT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Restore(ctx, &mcmanagerv1.RestoreBackupRequest{ServerId: id, BackupId: b.Id}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "world", "level.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "WORLD" {
		t.Errorf("level.dat = %q, want WORLD", got)
	}
}

func TestBackupServiceRestoreUnknownBackup(t *testing.T) {
	svc, _, id := newBackupService(t, mcmanagerv1.ServerStatus_STOPPED, nil)
	_, err := svc.Restore(context.Background(), &mcmanagerv1.RestoreBackupRequest{ServerId: id, BackupId: "missing"})
	if ec := errorCodeOf(err); ec != mcmanagerv1.ErrorCode_BACKUP_NOT_FOUND {
		t.Errorf("ErrorCode = %v, want BACKUP_NOT_FOUND", ec)
	}
}

func TestBackupServiceDelete(t *testing.T) {
	svc, _, id := newBackupService(t, mcmanagerv1.ServerStatus_STOPPED, nil)
	ctx := context.Background()

	b, err := svc.Create(ctx, &mcmanagerv1.CreateBackupRequest{ServerId: id})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Delete(ctx, b); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ := svc.List(ctx, &mcmanagerv1.ServerId{Id: id})
	if len(list.Backups) != 0 {
		t.Errorf("List after Delete = %+v, want empty", list.Backups)
	}
}

func TestBackupServiceCreateSafeOnline(t *testing.T) {
	hub := console.NewHub()
	fake := &fakeInstance{out: make(chan string, 16), done: make(chan struct{}), emitSaved: true}
	hub.Register("srv1", fake)

	svc, _, id := newBackupService(t, mcmanagerv1.ServerStatus_RUNNING, hub)
	b, err := svc.Create(context.Background(), &mcmanagerv1.CreateBackupRequest{ServerId: id, SafeOnline: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if b.SizeBytes <= 0 {
		t.Errorf("empty backup: %+v", b)
	}
	if !fake.wrote("save-off") || !fake.wrote("save-all flush") || !fake.wrote("save-on") {
		t.Error("safe-online create did not run the save-off/flush/save-on dance")
	}
}

func TestBackupServiceCreateUnknownServer(t *testing.T) {
	svc, _, _ := newBackupService(t, mcmanagerv1.ServerStatus_STOPPED, nil)
	_, err := svc.Create(context.Background(), &mcmanagerv1.CreateBackupRequest{ServerId: "ghost"})
	if code := status.Code(err); code != codes.NotFound {
		t.Errorf("Create unknown server code = %v, want NotFound", code)
	}
}
