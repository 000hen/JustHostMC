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

// ParserService implements ParserServiceServer: it lists installed mod/plugin
// metadata parser scripts (built-in + user-imported), imports a user parser,
// removes user parsers, and manages per-script permission grants. It mirrors
// ProviderService but drives a scripting.ParserSet.
type ParserService struct {
	mcmanagerv1.UnimplementedParserServiceServer
	parsers *scripting.ParserSet
	grants  *scripting.GrantStore
	config  *scripting.ConfigStore
	dir     string // root dir where user parsers are persisted
}

// NewParserService builds a ParserService. dir is where imported user parser
// scripts are stored; grants persists permission decisions.
func NewParserService(parsers *scripting.ParserSet, grants *scripting.GrantStore, config *scripting.ConfigStore, dir string) *ParserService {
	return &ParserService{parsers: parsers, grants: grants, config: config, dir: dir}
}

func (s *ParserService) List(_ context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.ParserList, error) {
	entries := s.parsers.List()
	out := make([]*mcmanagerv1.ParserInfo, 0, len(entries))
	for _, p := range entries {
		out = append(out, s.info(p))
	}
	return &mcmanagerv1.ParserList{Parsers: out}, nil
}

func (s *ParserService) Import(ctx context.Context, req *mcmanagerv1.ImportParserRequest) (*mcmanagerv1.ParserInfo, error) {
	if strings.TrimSpace(req.LuaSource) == "" {
		return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "parser script is empty", nil)
	}
	// Compile first so a bad script is rejected before anything is written.
	p, err := s.parsers.AddSource(ctx, req.LuaSource, false)
	if err != nil {
		return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	if err := os.WriteFile(s.parserPath(p.Meta().ID), []byte(req.LuaSource), 0o644); err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	return s.info(p), nil
}

func (s *ParserService) Remove(_ context.Context, ref *mcmanagerv1.ProviderRef) (*mcmanagerv1.Empty, error) {
	p, ok := s.parsers.Get(ref.Id)
	if !ok {
		return &mcmanagerv1.Empty{}, nil
	}
	if p.Builtin() {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "cannot remove a built-in parser", nil)
	}
	s.parsers.Remove(ref.Id)
	if s.grants != nil {
		_ = s.grants.Forget(ref.Id)
	}
	if s.config != nil {
		_ = s.config.Forget(ref.Id)
	}
	_ = os.Remove(s.parserPath(ref.Id))
	return &mcmanagerv1.Empty{}, nil
}

func (s *ParserService) SetPermissions(_ context.Context, req *mcmanagerv1.SetPermissionsRequest) (*mcmanagerv1.ParserInfo, error) {
	p, ok := s.parsers.Get(req.Id)
	if !ok {
		return nil, errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "parser not found", nil)
	}
	if s.grants != nil {
		// Clamp the request to what the script declared, so a parser can never
		// be granted a capability the user was never shown a reason for.
		declared := make(map[mcmanagerv1.PermissionKind]bool, len(p.Meta().Permissions))
		for _, perm := range p.Meta().Permissions {
			declared[perm.Kind] = true
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
	return s.info(p), nil
}

// parserPath returns the on-disk path of a user parser keyed by its id.
func (s *ParserService) parserPath(id string) string {
	return filepath.Join(s.dir, id+".lua")
}

// info maps a parser to the proto ParserInfo, resolving the currently
// effective permission grants.
func (s *ParserService) info(p *scripting.LuaParser) *mcmanagerv1.ParserInfo {
	meta := p.Meta()
	perms := make([]*mcmanagerv1.Permission, 0, len(meta.Permissions))
	for _, perm := range meta.Permissions {
		perms = append(perms, &mcmanagerv1.Permission{Kind: perm.Kind, Reason: perm.Reason})
	}
	var granted []mcmanagerv1.PermissionKind
	for k := range s.parsers.EffectiveGrants(meta.ID) {
		granted = append(granted, k)
	}
	slices.Sort(granted)
	return &mcmanagerv1.ParserInfo{
		Id:            meta.ID,
		Name:          meta.Name,
		Website:       meta.Website,
		Description:   meta.Description,
		Version:       meta.Version,
		Author:        meta.Author,
		Builtin:       p.Builtin(),
		Permissions:   perms,
		Granted:       granted,
		Formats:       meta.Formats,
		ConfigOptions: configOptions(meta.Config),
	}
}

func (s *ParserService) GetConfig(_ context.Context, ref *mcmanagerv1.ProviderRef) (*mcmanagerv1.ScriptConfig, error) {
	p, ok := s.parsers.Get(ref.Id)
	if !ok {
		return nil, errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "parser not found", nil)
	}
	return getConfigView(ref.Id, p.Meta().Config, s.config), nil
}

func (s *ParserService) SetConfig(_ context.Context, req *mcmanagerv1.SetConfigRequest) (*mcmanagerv1.ScriptConfig, error) {
	p, ok := s.parsers.Get(req.Id)
	if !ok {
		return nil, errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "parser not found", nil)
	}
	return applyConfig(req.Id, p.Meta().Config, s.config, req)
}
