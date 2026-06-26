package grpcsvc

import (
	"context"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/logging"
	"github.com/000hen/justhostmc/engine/internal/settings"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SettingsServiceConfig wires the SettingsService to its collaborators.
type SettingsServiceConfig struct {
	Store      *settings.Store
	LogsRoot   string
	ActiveMode string // the isolation backend chosen at startup, for reporting
	// Detector reports live Docker availability; defaults to the real probe when nil
	// (injectable for tests).
	Detector func(context.Context) isolation.Availability
}

// SettingsService implements the SettingsService RPCs: read/update the log
// retention policy and apply it on demand (PROMPT §15).
type SettingsService struct {
	mcmanagerv1.UnimplementedSettingsServiceServer
	cfg SettingsServiceConfig
}

// NewSettingsService builds a SettingsService.
func NewSettingsService(cfg SettingsServiceConfig) *SettingsService {
	return &SettingsService{cfg: cfg}
}

// GetLogRetention returns the current retention policy (defaults if unset).
func (s *SettingsService) GetLogRetention(_ context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.LogRetention, error) {
	v, err := s.cfg.Store.Load()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load settings: %v", err)
	}
	return &mcmanagerv1.LogRetention{KeepDays: int32(v.KeepLogDays), MaxTotalBytes: v.MaxLogTotalBytes}, nil
}

// SetLogRetention persists a new retention policy.
func (s *SettingsService) SetLogRetention(_ context.Context, req *mcmanagerv1.LogRetention) (*mcmanagerv1.Empty, error) {
	if req.KeepDays < 0 || req.MaxTotalBytes < 0 {
		return nil, status.Error(codes.InvalidArgument, "retention values must be non-negative")
	}
	err := s.cfg.Store.Save(settings.Settings{
		KeepLogDays:      int(req.KeepDays),
		MaxLogTotalBytes: req.MaxTotalBytes,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "save settings: %v", err)
	}
	return &mcmanagerv1.Empty{}, nil
}

// PurgeLogs applies the stored retention policy immediately.
func (s *SettingsService) PurgeLogs(_ context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.PurgeResult, error) {
	v, err := s.cfg.Store.Load()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load settings: %v", err)
	}
	removed, freed, err := logging.Purge(s.cfg.LogsRoot, v.Policy(), time.Now())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "purge logs: %v", err)
	}
	return &mcmanagerv1.PurgeResult{RemovedFiles: int32(removed), FreedBytes: freed}, nil
}

// GetBackendInfo reports the active isolation backend, live Docker availability,
// and the user's persisted Docker opt-in so the UI can show where servers run
// (PROMPT §10.7).
func (s *SettingsService) GetBackendInfo(ctx context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.BackendInfo, error) {
	v, err := s.cfg.Store.Load()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load settings: %v", err)
	}
	mode := s.cfg.ActiveMode
	if mode == "" {
		mode = string(isolation.ModeOnMachine)
	}
	detect := s.cfg.Detector
	if detect == nil {
		detect = func(c context.Context) isolation.Availability { return isolation.DetectDocker(c, nil) }
	}
	avail := detect(ctx)
	return &mcmanagerv1.BackendInfo{
		ActiveMode:      mode,
		DockerAvailable: avail.Available,
		DockerVersion:   avail.Version,
		UseDocker:       v.UseDocker,
	}, nil
}

// SetUseDocker persists the Docker opt-in. It takes effect on the next engine
// launch (running servers stay on the backend they were started with).
func (s *SettingsService) SetUseDocker(_ context.Context, req *mcmanagerv1.UseDocker) (*mcmanagerv1.Empty, error) {
	v, err := s.cfg.Store.Load()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load settings: %v", err)
	}
	v.UseDocker = req.Enabled
	if err := s.cfg.Store.Save(v); err != nil {
		return nil, status.Errorf(codes.Internal, "save settings: %v", err)
	}
	return &mcmanagerv1.Empty{}, nil
}
