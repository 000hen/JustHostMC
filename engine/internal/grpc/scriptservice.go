package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"google.golang.org/grpc/codes"
)

// ScriptService implements ScriptServiceServer: it lists automation scripts,
// imports a user script (persisting it to the scripts dir), enables/disables a
// script (starting/stopping its hooks), manages per-script permission grants,
// removes user scripts, and streams the engine-wide automation log. It mirrors
// ProviderService but drives a scripting.Manager instead of a Registry.
type ScriptService struct {
	mcmanagerv1.UnimplementedScriptServiceServer
	mgr     *scripting.Manager
	grants  *scripting.GrantStore
	enabled *scripting.EnabledStore
	dir     string // root dir where user scripts are persisted
}

// NewScriptService builds a ScriptService. dir is where imported user scripts are
// stored; grants persists permission decisions; enabled persists which scripts
// the user has turned on.
func NewScriptService(mgr *scripting.Manager, grants *scripting.GrantStore, enabled *scripting.EnabledStore, dir string) *ScriptService {
	return &ScriptService{mgr: mgr, grants: grants, enabled: enabled, dir: dir}
}

func (s *ScriptService) List(_ context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.ScriptList, error) {
	autos := s.mgr.List()
	out := make([]*mcmanagerv1.ScriptInfo, 0, len(autos))
	for _, a := range autos {
		out = append(out, s.info(a))
	}
	return &mcmanagerv1.ScriptList{Scripts: out}, nil
}

func (s *ScriptService) Import(_ context.Context, req *mcmanagerv1.ImportScriptRequest) (*mcmanagerv1.ScriptInfo, error) {
	if strings.TrimSpace(req.LuaSource) == "" {
		return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "automation script is empty", nil)
	}
	// Compile first so a bad script is rejected before anything is written.
	a, err := s.mgr.AddSource(req.LuaSource, false)
	if err != nil {
		return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	if err := os.WriteFile(s.scriptPath(a.Meta().ID), []byte(req.LuaSource), 0o644); err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	return s.info(a), nil
}

func (s *ScriptService) SetEnabled(_ context.Context, req *mcmanagerv1.SetScriptEnabledRequest) (*mcmanagerv1.ScriptInfo, error) {
	a, ok := s.mgr.Get(req.Id)
	if !ok {
		return nil, errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "script not found", nil)
	}
	if req.Enabled {
		if err := s.mgr.Enable(req.Id); err != nil {
			return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
		}
	} else {
		s.mgr.Disable(req.Id)
	}
	if s.enabled != nil {
		if err := s.enabled.Set(req.Id, req.Enabled); err != nil {
			return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
		}
	}
	return s.info(a), nil
}

func (s *ScriptService) SetPermissions(_ context.Context, req *mcmanagerv1.SetPermissionsRequest) (*mcmanagerv1.ScriptInfo, error) {
	a, ok := s.mgr.Get(req.Id)
	if !ok {
		return nil, errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "script not found", nil)
	}
	if s.grants != nil {
		// Clamp the request to what the script declared, so it can never be
		// granted a capability the user was never shown a reason for.
		declared := make(map[mcmanagerv1.PermissionKind]bool, len(a.Meta().Permissions))
		for _, p := range a.Meta().Permissions {
			declared[p.Kind] = true
		}
		allowed := make([]mcmanagerv1.PermissionKind, 0, len(req.Granted))
		for _, k := range req.Granted {
			if declared[k] {
				allowed = append(allowed, k)
			}
		}
		if err := s.grants.Set(req.Id, allowed); err != nil {
			return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
		}
	}
	// Re-enabling picks up the new grants for any running hooks.
	if s.mgr.Enabled(req.Id) {
		s.mgr.Disable(req.Id)
		if err := s.mgr.Enable(req.Id); err != nil {
			return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
		}
	}
	return s.info(a), nil
}

func (s *ScriptService) Remove(_ context.Context, ref *mcmanagerv1.ProviderRef) (*mcmanagerv1.Empty, error) {
	a, ok := s.mgr.Get(ref.Id)
	if !ok {
		return &mcmanagerv1.Empty{}, nil
	}
	if a.Builtin() {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "cannot remove a built-in script", nil)
	}
	s.mgr.Remove(ref.Id)
	if s.grants != nil {
		_ = s.grants.Forget(ref.Id)
	}
	if s.enabled != nil {
		_ = s.enabled.Forget(ref.Id)
	}
	_ = os.Remove(s.scriptPath(ref.Id))
	return &mcmanagerv1.Empty{}, nil
}

func (s *ScriptService) StreamLog(_ *mcmanagerv1.Empty, stream mcmanagerv1.ScriptService_StreamLogServer) error {
	history, live, cancel := s.mgr.Logs().Subscribe()
	defer cancel()
	for _, ll := range history {
		if err := stream.Send(scriptLogProto(ll)); err != nil {
			return err
		}
	}
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ll, ok := <-live:
			if !ok {
				return nil
			}
			if err := stream.Send(scriptLogProto(ll)); err != nil {
				return err
			}
		}
	}
}

func scriptLogProto(line scripting.LogLine) *mcmanagerv1.ScriptLogLine {
	return &mcmanagerv1.ScriptLogLine{
		ScriptId:         line.ScriptID,
		Line:             line.Line,
		Timestamp:        line.Timestamp.Format(time.RFC3339Nano),
		SessionId:        line.SessionID,
		SessionStartedAt: line.SessionStartedAt.Format(time.RFC3339Nano),
		CurrentSession:   line.CurrentSession,
	}
}

// scriptPath returns the on-disk path of a user script keyed by its id.
func (s *ScriptService) scriptPath(id string) string {
	return filepath.Join(s.dir, id+".lua")
}

// info maps an automation to the proto ScriptInfo, resolving its current enabled
// state and effective permission grants.
func (s *ScriptService) info(a *scripting.Automation) *mcmanagerv1.ScriptInfo {
	meta := a.Meta()
	perms := make([]*mcmanagerv1.Permission, 0, len(meta.Permissions))
	for _, p := range meta.Permissions {
		perms = append(perms, &mcmanagerv1.Permission{Kind: p.Kind, Reason: p.Reason})
	}
	var granted []mcmanagerv1.PermissionKind
	for k := range s.mgr.EffectiveGrants(meta.ID) {
		granted = append(granted, k)
	}
	slices.Sort(granted)
	return &mcmanagerv1.ScriptInfo{
		Id:          meta.ID,
		Name:        meta.Name,
		Website:     meta.Website,
		Description: meta.Description,
		Version:     meta.Version,
		Author:      meta.Author,
		Enabled:     s.mgr.Enabled(meta.ID),
		Permissions: perms,
		Granted:     granted,
	}
}
