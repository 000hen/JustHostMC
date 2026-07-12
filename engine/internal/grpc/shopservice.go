package grpcsvc

import (
	"context"
	"crypto/sha1" //nolint:gosec // upstream sources publish SHA-1 checksums
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"slices"
	"strings"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/dl"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// shopSearchMaxLimit clamps one search page; both sources cap lower or equal
// (CurseForge 50, Modrinth 100).
const shopSearchMaxLimit = 50

// ModDirResolver validates a server and returns its writable plugins/mods
// dir + kind. *ModService satisfies it, so shop installs share the exact
// same guards (server stopped, layout supported, safe filenames).
type ModDirResolver interface {
	writableDir(serverID string) (string, mcmanagerv1.ModKind, error)
}

// ShopService implements ShopServiceServer over a scripting.ShopSet: browse,
// search, project detail, version listing, and streamed installs, plus the
// usual script management (import/remove/permissions) mirroring ParserService.
type ShopService struct {
	mcmanagerv1.UnimplementedShopServiceServer
	shops  *scripting.ShopSet
	grants *scripting.GrantStore
	dir    string // root dir where user shops are persisted
	store  store.Store
	mods   ModDirResolver
}

// NewShopService builds a ShopService. dir is where imported user shop
// scripts are stored; st resolves per-server pre-filters (MC version +
// provider/loader); mods supplies the guarded target directory for installs.
func NewShopService(shops *scripting.ShopSet, grants *scripting.GrantStore, dir string, st store.Store, mods ModDirResolver) *ShopService {
	return &ShopService{shops: shops, grants: grants, dir: dir, store: st, mods: mods}
}

func (s *ShopService) List(_ context.Context, _ *mcmanagerv1.Empty) (*mcmanagerv1.ShopList, error) {
	entries := s.shops.List()
	out := make([]*mcmanagerv1.ShopInfo, 0, len(entries))
	for _, sh := range entries {
		out = append(out, s.info(sh))
	}
	return &mcmanagerv1.ShopList{Shops: out}, nil
}

func (s *ShopService) GetCategories(ctx context.Context, req *mcmanagerv1.ShopCategoriesRequest) (*mcmanagerv1.ShopCategoryList, error) {
	sh, err := s.shop(req.ShopId)
	if err != nil {
		return nil, err
	}
	categories, err := sh.Categories(ctx, kindString(req.Kind))
	if err != nil {
		return nil, mapShopError(err)
	}
	return &mcmanagerv1.ShopCategoryList{
		Categories: categoriesToProto(categories),
	}, nil
}

func (s *ShopService) Home(ctx context.Context, req *mcmanagerv1.ShopHomeRequest) (*mcmanagerv1.ShopHomeReply, error) {
	sh, err := s.shop(req.ShopId)
	if err != nil {
		return nil, err
	}
	sections, err := sh.Home(ctx, scripting.ShopQuery{
		MCVersion: req.McVersion,
		Loader:    req.Loader,
		Kind:      kindString(req.Kind),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	reply := &mcmanagerv1.ShopHomeReply{}
	for _, sec := range sections {
		reply.Sections = append(reply.Sections, &mcmanagerv1.ShopHomeSection{
			Title:    &mcmanagerv1.LocalizedMessage{Key: sec.TitleKey},
			Projects: projectsToProto(req.ShopId, sec.Projects),
		})
	}
	return reply, nil
}

func (s *ShopService) Search(ctx context.Context, req *mcmanagerv1.ShopSearchRequest) (*mcmanagerv1.ShopPage, error) {
	sh, err := s.shop(req.ShopId)
	if err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 || limit > shopSearchMaxLimit {
		limit = shopSearchMaxLimit
	}
	page, err := sh.Search(ctx, scripting.ShopQuery{
		Query:      req.Query,
		MCVersion:  req.McVersion,
		Loader:     req.Loader,
		Kind:       kindString(req.Kind),
		Categories: slices.Clone(req.Categories),
		Sort:       sortString(req.Sort),
		Offset:     int(req.Offset),
		Limit:      limit,
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return &mcmanagerv1.ShopPage{
		Projects: projectsToProto(req.ShopId, page.Projects),
		Total:    page.Total,
		Offset:   page.Offset,
	}, nil
}

func (s *ShopService) GetProject(ctx context.Context, req *mcmanagerv1.ShopProjectRequest) (*mcmanagerv1.ShopProjectDetail, error) {
	sh, err := s.shop(req.ShopId)
	if err != nil {
		return nil, err
	}
	d, err := sh.Detail(ctx, req.ProjectId)
	if err != nil {
		return nil, mapShopError(err)
	}
	format := mcmanagerv1.ShopBodyFormat_SHOP_BODY_FORMAT_UNSPECIFIED
	switch d.BodyFormat {
	case "markdown":
		format = mcmanagerv1.ShopBodyFormat_SHOP_BODY_MARKDOWN
	case "html":
		format = mcmanagerv1.ShopBodyFormat_SHOP_BODY_HTML
	}
	detail := &mcmanagerv1.ShopProjectDetail{
		Project:      projectToProto(req.ShopId, d.Project),
		Body:         d.Body,
		BodyFormat:   format,
		GameVersions: d.GameVersions,
		Loaders:      d.Loaders,
		License:      d.License,
		Updated:      d.Updated,
		Links: &mcmanagerv1.ShopLinks{
			Website: d.Links.Website,
			Source:  d.Links.Source,
			Issues:  d.Links.Issues,
			Wiki:    d.Links.Wiki,
			Discord: d.Links.Discord,
		},
	}
	for _, g := range d.Gallery {
		detail.Gallery = append(detail.Gallery, &mcmanagerv1.ShopGalleryImage{
			Url:         g.URL,
			Title:       g.Title,
			Description: g.Description,
		})
	}
	return detail, nil
}

func (s *ShopService) GetVersions(ctx context.Context, req *mcmanagerv1.ShopVersionsRequest) (*mcmanagerv1.ShopVersionList, error) {
	sh, err := s.shop(req.ShopId)
	if err != nil {
		return nil, err
	}
	versions, err := sh.Versions(ctx, req.ProjectId, req.McVersion, req.Loader)
	if err != nil {
		return nil, mapShopError(err)
	}
	out := &mcmanagerv1.ShopVersionList{}
	for _, v := range versions {
		channel := mcmanagerv1.ShopChannel_SHOP_CHANNEL_UNSPECIFIED
		switch v.Channel {
		case "release":
			channel = mcmanagerv1.ShopChannel_SHOP_CHANNEL_RELEASE
		case "beta":
			channel = mcmanagerv1.ShopChannel_SHOP_CHANNEL_BETA
		case "alpha":
			channel = mcmanagerv1.ShopChannel_SHOP_CHANNEL_ALPHA
		}
		pv := &mcmanagerv1.ShopVersion{
			Id:            v.ID,
			Name:          v.Name,
			VersionNumber: v.VersionNumber,
			Channel:       channel,
			GameVersions:  v.GameVersions,
			Loaders:       v.Loaders,
			Date:          v.Date,
			Downloads:     v.Downloads,
			Filename:      v.Filename,
			SizeBytes:     v.SizeBytes,
		}
		for _, d := range v.Dependencies {
			pv.Dependencies = append(pv.Dependencies, &mcmanagerv1.ShopDependency{
				ProjectId: d.ProjectID,
				Title:     d.Title,
				Required:  d.Required,
			})
		}
		out.Versions = append(out.Versions, pv)
	}
	return out, nil
}

// Install downloads the requested version (and any confirmed dependencies)
// into the server's plugins/mods folder, streaming progress. The target dir
// goes through ModService's guards, so the server must be stopped.
func (s *ShopService) Install(req *mcmanagerv1.ShopInstallRequest, stream grpc.ServerStreamingServer[mcmanagerv1.InstallProgress]) error {
	ctx := stream.Context()
	sh, err := s.shop(req.ShopId)
	if err != nil {
		return err
	}
	rec, ok := s.store.Get(req.ServerId)
	if !ok {
		return status.Error(codes.NotFound, "server not found")
	}
	mcVersion, loader := rec.McVersion, rec.ProviderID
	dir, kind, err := s.mods.writableDir(req.ServerId)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return status.Errorf(codes.Internal, "create dir: %v", err)
	}

	// The selected version first, then each confirmed dependency.
	type item struct{ projectID, versionID string }
	items := []item{{req.ProjectId, req.VersionId}}
	for _, d := range req.Dependencies {
		if d.ProjectId != "" {
			items = append(items, item{d.ProjectId, d.VersionId})
		}
	}

	for i, it := range items {
		base := float64(i) / float64(len(items))
		span := 1.0 / float64(len(items))

		send(stream, "shop.install.resolving", base)
		file, err := sh.ResolveFile(ctx, it.projectID, it.versionID, mcVersion, loader)
		if err != nil {
			return mapShopError(err)
		}
		name, err := safeModFileName(file.Filename, kind)
		if err != nil {
			return err
		}

		send(stream, "shop.install.downloading", base+span*0.1)
		var h hash.Hash
		var want string
		switch {
		case file.SHA512 != "":
			h, want = sha512.New(), strings.ToLower(file.SHA512)
		case file.SHA1 != "":
			h, want = sha1.New(), strings.ToLower(file.SHA1) //nolint:gosec
		}
		dest := filepath.Join(dir, name)
		sum, _, err := dl.Download(ctx, nil, file.URL, dest, h, func(done, total int64) {
			if total > 0 {
				send(stream, "shop.install.downloading", base+span*(0.1+0.85*float64(done)/float64(total)))
			}
		})
		if err != nil {
			return errorStatus(codes.Internal, mcmanagerv1.ErrorCode_INSTALL_FAILED, err.Error(), nil)
		}
		if want != "" && !strings.EqualFold(sum, want) {
			_ = os.Remove(dest)
			return errorStatus(codes.Internal, mcmanagerv1.ErrorCode_INSTALL_FAILED,
				fmt.Sprintf("checksum mismatch for %s", name), nil)
		}
	}
	send(stream, "shop.install.done", 1)
	return nil
}

// send streams one progress update; send failures surface on the next Recv.
func send(stream grpc.ServerStreamingServer[mcmanagerv1.InstallProgress], key string, fraction float64) {
	_ = stream.Send(&mcmanagerv1.InstallProgress{
		Step:     &mcmanagerv1.LocalizedMessage{Key: key},
		Fraction: fraction,
	})
}

func (s *ShopService) Import(ctx context.Context, req *mcmanagerv1.ImportShopRequest) (*mcmanagerv1.ShopInfo, error) {
	if strings.TrimSpace(req.LuaSource) == "" {
		return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "shop script is empty", nil)
	}
	sh, err := s.shops.AddSource(ctx, req.LuaSource, false)
	if err != nil {
		return nil, errorStatus(codes.InvalidArgument, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	if err := os.WriteFile(s.shopPath(sh.Meta().ID), []byte(req.LuaSource), 0o644); err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
	return s.info(sh), nil
}

func (s *ShopService) Remove(_ context.Context, ref *mcmanagerv1.ProviderRef) (*mcmanagerv1.Empty, error) {
	sh, ok := s.shops.Get(ref.Id)
	if !ok {
		return &mcmanagerv1.Empty{}, nil
	}
	if sh.Builtin() {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "cannot remove a built-in shop", nil)
	}
	s.shops.Remove(ref.Id)
	if s.grants != nil {
		_ = s.grants.Forget(ref.Id)
	}
	_ = os.Remove(s.shopPath(ref.Id))
	return &mcmanagerv1.Empty{}, nil
}

func (s *ShopService) SetPermissions(_ context.Context, req *mcmanagerv1.SetPermissionsRequest) (*mcmanagerv1.ShopInfo, error) {
	sh, ok := s.shops.Get(req.Id)
	if !ok {
		return nil, errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, "shop not found", nil)
	}
	if s.grants != nil {
		// Clamp to declared permissions, mirroring ParserService.
		declared := make(map[mcmanagerv1.PermissionKind]bool, len(sh.Meta().Permissions))
		for _, perm := range sh.Meta().Permissions {
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
	return s.info(sh), nil
}

// shop resolves a shop id or returns a typed NotFound.
func (s *ShopService) shop(id string) (*scripting.LuaShop, error) {
	sh, ok := s.shops.Get(id)
	if !ok {
		return nil, errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_SHOP_PROJECT_NOT_FOUND, "shop not found: "+id, nil)
	}
	return sh, nil
}

// shopPath returns the on-disk path of a user shop keyed by its id.
func (s *ShopService) shopPath(id string) string {
	return filepath.Join(s.dir, id+".lua")
}

// info maps a shop to the proto ShopInfo with effective grants + readiness.
func (s *ShopService) info(sh *scripting.LuaShop) *mcmanagerv1.ShopInfo {
	meta := sh.Meta()
	perms := make([]*mcmanagerv1.Permission, 0, len(meta.Permissions))
	for _, perm := range meta.Permissions {
		perms = append(perms, &mcmanagerv1.Permission{Kind: perm.Kind, Reason: perm.Reason})
	}
	var granted []mcmanagerv1.PermissionKind
	for k := range s.shops.EffectiveGrants(meta.ID) {
		granted = append(granted, k)
	}
	slices.Sort(granted)
	return &mcmanagerv1.ShopInfo{
		Id:          meta.ID,
		Name:        meta.Name,
		Website:     meta.Website,
		Description: meta.Description,
		Version:     meta.Version,
		Author:      meta.Author,
		Builtin:     sh.Builtin(),
		Permissions: perms,
		Granted:     granted,
		NeedsKey:    meta.NeedsKey,
		Ready:       sh.Ready(),
	}
}

// mapShopError converts shop script failures into typed gRPC statuses.
func mapShopError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return status.FromContextError(context.Canceled).Err()
	case errors.Is(err, scripting.ErrShopKeyMissing):
		return errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SHOP_KEY_MISSING, err.Error(), nil)
	case errors.Is(err, scripting.ErrShopNotDistributable):
		return errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SHOP_FILE_NOT_DISTRIBUTABLE, err.Error(), nil)
	case errors.Is(err, scripting.ErrShopNotFound):
		return errorStatus(codes.NotFound, mcmanagerv1.ErrorCode_SHOP_PROJECT_NOT_FOUND, err.Error(), nil)
	default:
		return errorStatus(codes.Internal, mcmanagerv1.ErrorCode_ERROR_CODE_UNSPECIFIED, err.Error(), nil)
	}
}

// kindString maps ModKind to the script-facing string.
func kindString(k mcmanagerv1.ModKind) string {
	if k == mcmanagerv1.ModKind_PLUGIN {
		return "plugin"
	}
	return "mod"
}

// sortString maps ShopSort to the script-facing string.
func sortString(s mcmanagerv1.ShopSort) string {
	switch s {
	case mcmanagerv1.ShopSort_SHOP_SORT_DOWNLOADS:
		return "downloads"
	case mcmanagerv1.ShopSort_SHOP_SORT_FOLLOWS:
		return "follows"
	case mcmanagerv1.ShopSort_SHOP_SORT_NEWEST:
		return "newest"
	case mcmanagerv1.ShopSort_SHOP_SORT_UPDATED:
		return "updated"
	default:
		return "relevance"
	}
}

func projectsToProto(shopID string, in []scripting.ShopProject) []*mcmanagerv1.ShopProject {
	out := make([]*mcmanagerv1.ShopProject, 0, len(in))
	for _, p := range in {
		out = append(out, projectToProto(shopID, p))
	}
	return out
}

func projectToProto(shopID string, p scripting.ShopProject) *mcmanagerv1.ShopProject {
	return &mcmanagerv1.ShopProject{
		ShopId:       shopID,
		ProjectId:    p.ID,
		Slug:         p.Slug,
		Title:        p.Title,
		Summary:      p.Summary,
		IconUrl:      p.IconURL,
		Author:       p.Author,
		Downloads:    p.Downloads,
		Follows:      p.Follows,
		Categories:   p.Categories,
		ProjectType:  p.ProjectType,
		Distribution: distributionToProto(p.Distribution),
	}
}

func distributionToProto(distribution scripting.ShopDistribution) mcmanagerv1.ShopDistribution {
	switch distribution {
	case scripting.ShopDistributionDirect:
		return mcmanagerv1.ShopDistribution_SHOP_DISTRIBUTION_DIRECT
	case scripting.ShopDistributionWebsiteOnly:
		return mcmanagerv1.ShopDistribution_SHOP_DISTRIBUTION_WEBSITE_ONLY
	default:
		return mcmanagerv1.ShopDistribution_SHOP_DISTRIBUTION_UNKNOWN
	}
}

func categoriesToProto(categories []scripting.ShopCategory) []*mcmanagerv1.ShopCategory {
	out := make([]*mcmanagerv1.ShopCategory, 0, len(categories))
	for _, category := range categories {
		out = append(out, &mcmanagerv1.ShopCategory{
			Id:              category.ID,
			Name:            category.Name,
			Slug:            category.Slug,
			LocalizationKey: category.LocalizationKey,
		})
	}
	return out
}
