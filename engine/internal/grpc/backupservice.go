package grpcsvc

import (
	"context"
	"errors"
	"strings"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/backup"
	"github.com/000hen/justhostmc/engine/internal/console"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// defaultSaveTimeout bounds how long a safe-online backup waits for the server
// to confirm its world flush before snapshotting anyway (best effort).
const defaultSaveTimeout = 30 * time.Second

// BackupServiceConfig wires the BackupService to its collaborators.
type BackupServiceConfig struct {
	Manager     *backup.Manager
	Store       store.Store
	Paths       appdata.Paths
	Console     *console.Hub // for safe-online save-off/flush/save-on; may be nil in tests
	SaveTimeout time.Duration
}

// BackupService implements the BackupService RPCs: it snapshots, lists, restores,
// and deletes a server's data folder as portable zip archives. For a running
// server, a safe-online backup pauses world saves, flushes, snapshots, then
// resumes saves so the archive is consistent with no downtime (PROMPT §10.4).
type BackupService struct {
	mcmanagerv1.UnimplementedBackupServiceServer
	cfg BackupServiceConfig
}

// NewBackupService builds a BackupService.
func NewBackupService(cfg BackupServiceConfig) *BackupService {
	return &BackupService{cfg: cfg}
}

func (s *BackupService) saveTimeout() time.Duration {
	if s.cfg.SaveTimeout > 0 {
		return s.cfg.SaveTimeout
	}
	return defaultSaveTimeout
}

// Create snapshots a server's data folder into a new backup archive.
func (s *BackupService) Create(ctx context.Context, req *mcmanagerv1.CreateBackupRequest) (*mcmanagerv1.Backup, error) {
	rec, ok := s.cfg.Store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	srcDir := s.cfg.Paths.ServerDir(req.ServerId)

	var info backup.Info
	snapshot := func() error {
		var e error
		info, e = s.cfg.Manager.Create(req.ServerId, srcDir)
		return e
	}

	running := rec.Status == mcmanagerv1.ServerStatus_RUNNING
	var err error
	if req.SafeOnline && running && s.cfg.Console != nil {
		err = safeOnlineSnapshot(ctx, s.cfg.Console, req.ServerId, snapshot, s.saveTimeout())
	} else {
		err = snapshot()
	}
	if err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_BACKUP_FAILED, err.Error(), nil)
	}
	return backupProto(info), nil
}

// List returns a server's backups, newest first.
func (s *BackupService) List(_ context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.BackupList, error) {
	infos, err := s.cfg.Manager.List(req.Id)
	if err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_BACKUP_FAILED, err.Error(), nil)
	}
	out := &mcmanagerv1.BackupList{Backups: make([]*mcmanagerv1.Backup, 0, len(infos))}
	for _, info := range infos {
		out.Backups = append(out.Backups, backupProto(info))
	}
	return out, nil
}

// Restore extracts a backup over a server's data folder. The server must be
// stopped so the restore is world-consistent.
func (s *BackupService) Restore(_ context.Context, req *mcmanagerv1.RestoreBackupRequest) (*mcmanagerv1.Empty, error) {
	rec, ok := s.cfg.Store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	switch rec.Status {
	case mcmanagerv1.ServerStatus_RUNNING, mcmanagerv1.ServerStatus_STARTING, mcmanagerv1.ServerStatus_STOPPING:
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SERVER_RUNNING,
			"stop the server before restoring a backup", nil)
	}

	destDir := s.cfg.Paths.ServerDir(req.ServerId)
	if err := s.cfg.Manager.Restore(req.ServerId, req.BackupId, destDir); err != nil {
		return nil, mapBackupError(err)
	}
	return &mcmanagerv1.Empty{}, nil
}

// Delete removes a stored backup archive.
func (s *BackupService) Delete(_ context.Context, req *mcmanagerv1.Backup) (*mcmanagerv1.Empty, error) {
	if err := s.cfg.Manager.Delete(req.ServerId, req.Id); err != nil {
		return nil, mapBackupError(err)
	}
	return &mcmanagerv1.Empty{}, nil
}

// backupProto maps a backup.Info to its proto form (created_at as RFC3339 UTC).
func backupProto(info backup.Info) *mcmanagerv1.Backup {
	return &mcmanagerv1.Backup{
		Id:        info.ID,
		ServerId:  info.ServerID,
		SizeBytes: info.SizeBytes,
		CreatedAt: info.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// mapBackupError converts backup errors into typed gRPC statuses.
func mapBackupError(err error) error {
	if errors.Is(err, backup.ErrBackupNotFound) {
		return errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_BACKUP_NOT_FOUND, err.Error(), nil)
	}
	return errorStatus(codes.Internal, mcmanagerv1.ErrorCode_BACKUP_FAILED, err.Error(), nil)
}

// safeOnlineSnapshot pauses world saves, forces a flush, runs snapshot once the
// server confirms the flush (or the timeout elapses), and always resumes saves
// afterwards — even if the snapshot fails. If saves can't be paused (e.g. the
// server just died), it falls back to a direct snapshot.
func safeOnlineSnapshot(ctx context.Context, hub *console.Hub, serverID string, snapshot func() error, timeout time.Duration) error {
	_, live, cancel := hub.Subscribe(serverID)
	defer cancel()

	if err := hub.Send(serverID, "save-off"); err != nil {
		return snapshot() // can't pause saves; best-effort cold snapshot
	}
	defer func() { _ = hub.Send(serverID, "save-on") }()
	_ = hub.Send(serverID, "save-all flush")

	waitForFlush(ctx, live, timeout)
	return snapshot()
}

// waitForFlush blocks until the server reports its world was saved, the timeout
// elapses, or the context is cancelled.
func waitForFlush(ctx context.Context, live <-chan string, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		case line, ok := <-live:
			if !ok {
				return
			}
			// Modern servers print "Saved the game"; older ones "Saved the world".
			if strings.Contains(line, "Saved the") {
				return
			}
		}
	}
}
