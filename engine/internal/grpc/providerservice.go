package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"google.golang.org/grpc/codes"
)

// ProviderService implements ProviderServiceServer: it lists installed provider
// scripts (built-in + user-imported), imports a user script with an optional
// bundled jar, removes user providers, and manages per-script permission grants.
type ProviderService struct {
	mcmanagerv1.UnimplementedProviderServiceServer
	registry *scripting.Registry
	grants   *scripting.GrantStore
	dir      string // root dir where user providers are persisted
}

// NewProviderService builds a ProviderService. dir is where imported user
// scripts (and any bundled jars) are stored.
func NewProviderService(reg *scripting.Registry, grants *scripting.GrantStore, dir string) *ProviderService {
	return &ProviderService{registry: reg, grants: grants, dir: dir}
}

func (s *ProviderService) List(_ context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.ProviderList, error) {
	entries := s.registry.List()
	out := make([]*mcmanagerv1.ProviderInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, s.info(e))
	}
	return &mcmanagerv1.ProviderList{Providers: out}, nil
}

func (s *ProviderService) Import(_ context.Context, req *mcmanagerv1.ImportProviderRequest) (*mcmanagerv1.ProviderInfo, error) {
	if strings.TrimSpace(req.LuaSource) == "" {
		return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "provider script is empty", nil)
	}
	// Compile first so a bad script is rejected before anything is written.
	e, err := s.registry.AddSource(req.LuaSource, false)
	if err != nil {
		return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}

	pdir := filepath.Join(s.dir, e.Meta.ID)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	if err := os.WriteFile(filepath.Join(pdir, "provider.lua"), []byte(req.LuaSource), 0o644); err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	if len(req.Jar) > 0 && req.JarFilename != "" {
		jarName := filepath.Base(req.JarFilename)
		if !strings.HasSuffix(strings.ToLower(jarName), ".jar") {
			return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "bundled jar filename must end in .jar", nil)
		}
		if err := os.WriteFile(filepath.Join(pdir, jarName), req.Jar, 0o644); err != nil {
			return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
		}
	}
	// Re-register from the persisted dir so the script gains its asset dir.
	e2, err := s.registry.AddProviderDir(pdir, false)
	if err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	return s.info(e2), nil
}

func (s *ProviderService) Remove(_ context.Context, ref *mcmanagerv1.ProviderRef) (*mcmanagerv1.Empty, error) {
	e, ok := s.registry.Get(ref.Id)
	if !ok {
		return &mcmanagerv1.Empty{}, nil
	}
	if e.Builtin {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "cannot remove a built-in provider", nil)
	}
	s.registry.Remove(ref.Id)
	if s.grants != nil {
		_ = s.grants.Forget(ref.Id)
	}
	_ = os.RemoveAll(filepath.Join(s.dir, ref.Id))
	return &mcmanagerv1.Empty{}, nil
}

func (s *ProviderService) SetPermissions(_ context.Context, req *mcmanagerv1.SetPermissionsRequest) (*mcmanagerv1.ProviderInfo, error) {
	e, ok := s.registry.Get(req.Id)
	if !ok {
		return nil, errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "provider not found", nil)
	}
	if s.grants != nil {
		// Clamp the request to what the script actually declared, so a provider
		// can never be granted a capability the user was never shown a reason for.
		declared := make(map[mcmanagerv1.PermissionKind]bool, len(e.Meta.Permissions))
		for _, p := range e.Meta.Permissions {
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
	return s.info(e), nil
}

// info maps a registry entry to the proto ProviderInfo, resolving the currently
// effective permission grants.
func (s *ProviderService) info(e *scripting.Entry) *mcmanagerv1.ProviderInfo {
	perms := make([]*mcmanagerv1.Permission, 0, len(e.Meta.Permissions))
	for _, p := range e.Meta.Permissions {
		perms = append(perms, &mcmanagerv1.Permission{Kind: p.Kind, Reason: p.Reason})
	}
	var granted []mcmanagerv1.PermissionKind
	for k := range s.registry.EffectiveGrants(e.Meta.ID) {
		granted = append(granted, k)
	}
	slices.Sort(granted)
	return &mcmanagerv1.ProviderInfo{
		Id:           e.Meta.ID,
		Name:         e.Meta.Name,
		Website:      e.Meta.Website,
		Description:  e.Meta.Description,
		Version:      e.Meta.Version,
		Author:       e.Meta.Author,
		Builtin:      e.Builtin,
		Permissions:  perms,
		Granted:      granted,
		Capabilities: &mcmanagerv1.ProviderCapabilities{ModLayout: e.Meta.ModLayout},
	}
}
