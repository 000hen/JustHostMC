package grpcsvc

import (
	"context"
	"maps"
	"slices"
	"strings"
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
	CloseLogs  func() // close open log files before purging
	// BakedShopKeys are build-time default shop API keys (shop id -> key),
	// e.g. a CurseForge key injected via -ldflags. Reported as
	// has_builtin_key; never returned to the client.
	BakedShopKeys map[string]string
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
	v, err := s.cfg.Store.Load()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load settings: %v", err)
	}
	v.KeepLogDays = int(req.KeepDays)
	v.MaxLogTotalBytes = req.MaxTotalBytes
	err = s.cfg.Store.Save(v)
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
	if s.cfg.CloseLogs != nil {
		s.cfg.CloseLogs()
	}
	p := v.Policy()
	p.ForceAll = true
	removed, freed, err := logging.Purge(s.cfg.LogsRoot, p, time.Now())
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

// GetShopKeys reports, per shop id, whether a user key override and/or a
// baked-in build default exist. Key material itself never leaves the engine.
func (s *SettingsService) GetShopKeys(_ context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.ShopKeyList, error) {
	v, err := s.cfg.Store.Load()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load settings: %v", err)
	}
	ids := map[string]bool{}
	for id := range v.ShopKeys {
		ids[id] = true
	}
	for id, key := range s.cfg.BakedShopKeys {
		if key != "" {
			ids[id] = true
		}
	}
	out := &mcmanagerv1.ShopKeyList{}
	for _, id := range slices.Sorted(maps.Keys(ids)) {
		out.Keys = append(out.Keys, &mcmanagerv1.ShopKeyInfo{
			ShopId:        id,
			HasUserKey:    v.ShopKeys[id] != "",
			HasBuiltinKey: s.cfg.BakedShopKeys[id] != "",
		})
	}
	return out, nil
}

// SetShopKey persists (or clears, with an empty api_key) the user's key
// override for one shop.
func (s *SettingsService) SetShopKey(_ context.Context, req *mcmanagerv1.ShopKey) (*mcmanagerv1.Empty, error) {
	if strings.TrimSpace(req.ShopId) == "" {
		return nil, status.Error(codes.InvalidArgument, "shop_id is required")
	}
	v, err := s.cfg.Store.Load()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load settings: %v", err)
	}
	key := strings.TrimSpace(req.ApiKey)
	if key == "" {
		delete(v.ShopKeys, req.ShopId)
	} else {
		if v.ShopKeys == nil {
			v.ShopKeys = map[string]string{}
		}
		v.ShopKeys[req.ShopId] = key
	}
	if err := s.cfg.Store.Save(v); err != nil {
		return nil, status.Errorf(codes.Internal, "save settings: %v", err)
	}
	return &mcmanagerv1.Empty{}, nil
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
