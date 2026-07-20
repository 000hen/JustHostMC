package grpcsvc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/backup"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maxModBytes caps a single uploaded plugin/mod archive; individual files are far
// smaller, so this just guards against a runaway or malicious upload.
const (
	maxModBytes         = 512 << 20 // 512 MiB
	maxModIconBytes     = 512 << 10 // 512 KiB
	defaultModListLimit = 20
	maxModListLimit     = 50
)

type modFileInfo struct {
	name  string
	size  int64
	mtime int64
}

// ModService manages mod archives in a server's plugins or mods folder. Uploads
// and removals require the server to be stopped so files aren't changed beneath a
// running JVM.
type ModService struct {
	mcmanagerv1.UnimplementedModServiceServer
	store  store.Store
	paths  appdata.Paths
	parser ModParser

	// metaCache memoizes parsed jar metadata keyed by
	// serverID|name|size|mtimeUnixNano, so repeated List calls don't re-parse
	// unchanged jars. It stores every parser candidate, not the final
	// compatibility-ranked metadata, so server version/provider changes update
	// warnings without re-reading every jar. Re-uploading a jar changes its
	// mtime, which self-invalidates the old entry (stale keys are dropped per
	// server).
	metaMu    sync.Mutex
	metaCache map[string]map[string]*modMetadataParseResult // server id -> key -> parser result
}

// ModParser extracts embedded metadata candidates from one jar (path relative
// to the server dir). *scripting.ParserSet satisfies it; an empty candidate
// slice means no installed parser recognized the jar.
type ModParser interface {
	ParseJarCandidates(ctx context.Context, serverDir, jarRel string) ([]scripting.ModParseCandidate, error)
}

type modMetadataParseResult struct {
	candidates []*mcmanagerv1.ModMetadata
	parseError string
}

// NewModService builds a ModService over the registry and data paths. parser
// may be nil (metadata then reports parsed=false for every jar).
func NewModService(st store.Store, paths appdata.Paths, parser ModParser) *ModService {
	return &ModService{
		store:     st,
		paths:     paths,
		parser:    parser,
		metaCache: map[string]map[string]*modMetadataParseResult{},
	}
}

// modLayout maps a provider's declared mod layout (captured on the server record
// at create time) to its jar folder and ModKind. ok is false for providers with
// no plugins/mods folder (e.g. Vanilla, mod_layout "none").
func modLayout(layout string) (subdir string, kind mcmanagerv1.ModKind, ok bool) {
	switch layout {
	case "plugins":
		return "plugins", mcmanagerv1.ModKind_PLUGIN, true
	case "mods":
		return "mods", mcmanagerv1.ModKind_MOD, true
	default:
		return "", mcmanagerv1.ModKind_MOD_KIND_UNSPECIFIED, false
	}
}

// List returns the archives in the server's plugins/mods folder, enriched
// with parsed metadata where a parser matches. Raw icons are deliberately
// omitted so one image cannot inflate the list response past gRPC's limit.
func (s *ModService) List(ctx context.Context, req *mcmanagerv1.ModListRequest) (*mcmanagerv1.ModList, error) {
	rec, ok := s.store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	subdir, kind, ok := modLayout(rec.ModLayout)
	if !ok {
		return &mcmanagerv1.ModList{ServerId: req.ServerId, Supported: false}, nil
	}

	dir := filepath.Join(s.paths.ServerDir(req.ServerId), subdir)
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "read %s: %v", subdir, err)
	}

	// The effective loader is the server's recorded loader (set by providers that
	// resolve one, e.g. a modpack), falling back to the provider id.
	serverLoader := rec.Loader
	if serverLoader == "" {
		serverLoader = rec.ProviderID
	}

	files := make([]modFileInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !supportedModFile(e.Name(), kind) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, modFileInfo{name: e.Name(), size: info.Size(), mtime: info.ModTime().UnixNano()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	offset, end := modPageBounds(req.Offset, req.Limit, len(files))
	list := &mcmanagerv1.ModList{
		ServerId: req.ServerId, Kind: kind, Supported: true,
		Total: int32(len(files)), Offset: int32(offset), NextOffset: int32(end), HasMore: end < len(files),
	}
	fresh := s.currentMetadataCache(req.ServerId, files)
	for _, file := range files[offset:end] {
		parsed := s.jarMetadata(ctx, req.ServerId, subdir, file.name, file.size, file.mtime, fresh)
		meta := selectModMetadata(parsed, serverLoader, rec.McVersion, kind)
		meta.Icon = nil
		list.Files = append(list.Files, &mcmanagerv1.ModFile{
			Name:      file.name,
			SizeBytes: file.size,
			Metadata:  meta,
		})
	}
	// Replace the server's cache with only the keys seen this pass so removed
	// or re-uploaded jars don't accumulate stale entries.
	s.metaMu.Lock()
	s.metaCache[req.ServerId] = fresh
	s.metaMu.Unlock()
	return list, nil
}

func modPageBounds(requestedOffset, requestedLimit int32, total int) (int, int) {
	offset := max(0, min(int(requestedOffset), total))
	limit := int(requestedLimit)
	if limit <= 0 {
		limit = defaultModListLimit
	} else if limit > maxModListLimit {
		limit = maxModListLimit
	}
	return offset, min(offset+limit, total)
}

func modMetadataCacheKey(file modFileInfo) string {
	return fmt.Sprintf("%s|%d|%d", file.name, file.size, file.mtime)
}

func (s *ModService) currentMetadataCache(serverID string, files []modFileInfo) map[string]*modMetadataParseResult {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	current := make(map[string]*modMetadataParseResult, len(files))
	for _, file := range files {
		key := modMetadataCacheKey(file)
		if cached, ok := s.metaCache[serverID][key]; ok {
			current[key] = cached
		}
	}
	return current
}

// jarMetadata returns the (possibly cached) parsed metadata for one jar and
// records it in fresh under its cache key.
func (s *ModService) jarMetadata(ctx context.Context, serverID, subdir, name string, size, mtime int64, fresh map[string]*modMetadataParseResult) *modMetadataParseResult {
	key := fmt.Sprintf("%s|%d|%d", name, size, mtime)
	s.metaMu.Lock()
	cached, ok := s.metaCache[serverID][key]
	s.metaMu.Unlock()
	if ok {
		fresh[key] = cached
		return cached
	}

	parsed := &modMetadataParseResult{}
	if s.parser != nil {
		candidates, err := s.parser.ParseJarCandidates(ctx, s.paths.ServerDir(serverID), subdir+"/"+name)
		if err != nil {
			parsed.parseError = err.Error()
		}
		for _, c := range candidates {
			parsed.candidates = append(parsed.candidates, metadataFromModMeta(c.Meta, c.ParserID))
		}
	}
	fresh[key] = parsed
	return parsed
}

func metadataFromModMeta(m scripting.ModMeta, parserID string) *mcmanagerv1.ModMetadata {
	icon := m.Icon
	if len(icon) > maxModIconBytes {
		icon = nil
	}
	return &mcmanagerv1.ModMetadata{
		Parsed:                 true,
		ParserId:               parserID,
		Loader:                 m.Loader,
		GameVersionRequirement: m.GameVersion,
		ModId:                  m.ModID,
		Name:                   m.Name,
		Version:                m.Version,
		Authors:                append([]string(nil), m.Authors...),
		Description:            m.Description,
		Website:                m.Website,
		Icon:                   append([]byte(nil), icon...),
		HasIcon:                len(icon) > 0,
	}
}

// GetIcon returns one bounded icon independently from paged metadata so raw
// image bytes can never accumulate into an oversized List response.
func (s *ModService) GetIcon(ctx context.Context, req *mcmanagerv1.ModIconRequest) (*mcmanagerv1.ModIcon, error) {
	rec, ok := s.store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	subdir, kind, ok := modLayout(rec.ModLayout)
	if !ok {
		return nil, status.Error(codes.FailedPrecondition, "this server type has no plugins/mods folder")
	}
	name, err := safeModFileName(req.Name, kind)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filepath.Join(s.paths.ServerDir(req.ServerId), subdir, name))
	if os.IsNotExist(err) {
		return nil, status.Error(codes.NotFound, "mod file not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "stat mod file: %v", err)
	}
	file := modFileInfo{name: name, size: info.Size(), mtime: info.ModTime().UnixNano()}
	fresh := s.currentMetadataCache(req.ServerId, []modFileInfo{file})
	parsed := s.jarMetadata(ctx, req.ServerId, subdir, name, file.size, file.mtime, fresh)
	s.metaMu.Lock()
	if s.metaCache[req.ServerId] == nil {
		s.metaCache[req.ServerId] = make(map[string]*modMetadataParseResult)
	}
	s.metaCache[req.ServerId][modMetadataCacheKey(file)] = parsed
	s.metaMu.Unlock()
	serverLoader := rec.Loader
	if serverLoader == "" {
		serverLoader = rec.ProviderID
	}
	meta := selectModMetadata(parsed, serverLoader, rec.McVersion, kind)
	if !meta.HasIcon || len(meta.Icon) == 0 {
		return &mcmanagerv1.ModIcon{}, nil
	}
	return &mcmanagerv1.ModIcon{Data: append([]byte(nil), meta.Icon...)}, nil
}

func selectModMetadata(parsed *modMetadataParseResult, providerID, mcVersion string, kind mcmanagerv1.ModKind) *mcmanagerv1.ModMetadata {
	if parsed == nil {
		return &mcmanagerv1.ModMetadata{}
	}
	if len(parsed.candidates) == 0 {
		return &mcmanagerv1.ModMetadata{ParseError: parsed.parseError}
	}

	best := parsed.candidates[0]
	bestRank := modMetadataRank(best, providerID, mcVersion, kind, 0)
	for i, candidate := range parsed.candidates[1:] {
		rank := modMetadataRank(candidate, providerID, mcVersion, kind, i+1)
		if rank.less(bestRank) {
			best = candidate
			bestRank = rank
		}
	}
	return modCompatibility(best, providerID, mcVersion, kind)
}

type modMetadataCandidateRank struct {
	loader  int
	version int
	order   int
}

func (r modMetadataCandidateRank) less(other modMetadataCandidateRank) bool {
	if r.loader != other.loader {
		return r.loader < other.loader
	}
	if r.version != other.version {
		return r.version < other.version
	}
	return r.order < other.order
}

func modMetadataRank(meta *mcmanagerv1.ModMetadata, providerID, mcVersion string, kind mcmanagerv1.ModKind, order int) modMetadataCandidateRank {
	return modMetadataCandidateRank{
		loader:  loaderMatchRank(meta.GetLoader(), providerID, kind),
		version: versionMatchRank(mcVersion, meta.GetGameVersionRequirement()),
		order:   order,
	}
}

func loaderMatchRank(loader, providerID string, kind mcmanagerv1.ModKind) int {
	loader = strings.ToLower(strings.TrimSpace(loader))
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if loader == "" {
		return 2 // unknown; better than a definite mismatch but worse than a declared match
	}
	if loaderExactlyMatchesProvider(loader, providerID, kind) {
		return 0
	}
	if loaderMatchesServer(loader, providerID, kind) {
		return 1
	}
	return 3
}

func loaderExactlyMatchesProvider(loader, providerID string, kind mcmanagerv1.ModKind) bool {
	if providerID == "" {
		return false
	}
	if kind == mcmanagerv1.ModKind_MOD && providerID == "forge" {
		return loader == "forge"
	}
	return loader == providerID
}

func versionMatchRank(mcVersion, requirement string) int {
	if strings.TrimSpace(requirement) == "" {
		return 1
	}
	matches, known := minecraftVersionMatches(mcVersion, requirement)
	if !known {
		return 1
	}
	if matches {
		return 0
	}
	return 2
}

// Remove deletes one jar by name. The server must be stopped.
func (s *ModService) Remove(_ context.Context, req *mcmanagerv1.RemoveModRequest) (*mcmanagerv1.Empty, error) {
	dir, kind, err := s.writableDir(req.ServerId)
	if err != nil {
		return nil, err
	}
	name, err := safeModFileName(req.Name, kind)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "remove %s: %v", name, err)
	}
	return &mcmanagerv1.Empty{}, nil
}

// Upload streams an archive into the server's plugins/mods folder. The first message
// carries the init (server id + filename); the rest carry raw bytes. It writes to
// a temp file and renames into place only on success. The server must be stopped.
func (s *ModService) Upload(stream grpc.ClientStreamingServer[mcmanagerv1.UploadModRequest, mcmanagerv1.ModFile]) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	init := first.GetInit()
	if init == nil {
		return status.Error(codes.InvalidArgument, "first upload message must carry init")
	}

	dir, kind, err := s.writableDir(init.ServerId)
	if err != nil {
		return err
	}
	name, err := safeModFileName(init.Filename, kind)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return status.Errorf(codes.Internal, "create dir: %v", err)
	}

	tmp, err := os.CreateTemp(dir, ".upload-*.jar.part")
	if err != nil {
		return status.Errorf(codes.Internal, "create temp: %v", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	var written int64
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		chunk := msg.GetChunk()
		if len(chunk) == 0 {
			continue
		}
		written += int64(len(chunk))
		if written > maxModBytes {
			return status.Errorf(codes.InvalidArgument, "upload exceeds %d bytes", int64(maxModBytes))
		}
		if _, err := tmp.Write(chunk); err != nil {
			return status.Errorf(codes.Internal, "write: %v", err)
		}
	}

	if err := tmp.Close(); err != nil {
		return status.Errorf(codes.Internal, "close temp: %v", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, name)); err != nil {
		return status.Errorf(codes.Internal, "finalize %s: %v", name, err)
	}
	committed = true
	return stream.SendAndClose(&mcmanagerv1.ModFile{Name: name, SizeBytes: written})
}

// ExportAll zips the server's whole plugins/mods folder to req.DestPath (a
// user-picked absolute .zip path). Read-only on the server dir, so a running
// server is fine.
func (s *ModService) ExportAll(_ context.Context, req *mcmanagerv1.ExportModsRequest) (*mcmanagerv1.Empty, error) {
	rec, ok := s.store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	subdir, _, ok := modLayout(rec.ModLayout)
	if !ok {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_MOD_UNSUPPORTED,
			"this server type has no plugins/mods folder", nil)
	}
	dest := req.DestPath
	if !filepath.IsAbs(dest) || !strings.EqualFold(filepath.Ext(dest), ".zip") {
		return nil, status.Error(codes.InvalidArgument, "dest_path must be an absolute .zip path")
	}
	dir := filepath.Join(s.paths.ServerDir(req.ServerId), subdir)
	if _, err := os.Stat(dir); err != nil {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_MOD_EXPORT_FAILED,
			"nothing to export", nil)
	}
	if err := backup.Archive(dir, dest); err != nil {
		return nil, errorStatus(codes.Internal, mcmanagerv1.ErrorCode_MOD_EXPORT_FAILED, err.Error(), nil)
	}
	return &mcmanagerv1.Empty{}, nil
}

// writableDir validates that the server exists, supports plugins/mods, and is
// stopped, then returns its jar directory.
func (s *ModService) writableDir(serverID string) (string, mcmanagerv1.ModKind, error) {
	rec, ok := s.store.Get(serverID)
	if !ok {
		return "", mcmanagerv1.ModKind_MOD_KIND_UNSPECIFIED, status.Error(codes.NotFound, "server not found")
	}
	subdir, kind, ok := modLayout(rec.ModLayout)
	if !ok {
		return "", mcmanagerv1.ModKind_MOD_KIND_UNSPECIFIED, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_MOD_UNSUPPORTED,
			"this server type has no plugins/mods folder", nil)
	}
	if rec.Status != mcmanagerv1.ServerStatus_STOPPED && rec.Status != mcmanagerv1.ServerStatus_CRASHED {
		return "", mcmanagerv1.ModKind_MOD_KIND_UNSPECIFIED, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SERVER_RUNNING,
			"stop the server before changing plugins/mods", nil)
	}
	return filepath.Join(s.paths.ServerDir(serverID), subdir), kind, nil
}

// supportedModFile accepts normal jars everywhere and legacy .litemod archives
// only in mod directories.
func supportedModFile(name string, kind mcmanagerv1.ModKind) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".jar") ||
		(kind == mcmanagerv1.ModKind_MOD && strings.HasSuffix(lower, ".litemod"))
}

// safeModFileName reduces a client-supplied filename to a safe basename,
// validates its archive extension, and rejects path traversal.
func safeModFileName(name string, kind mcmanagerv1.ModKind) (string, error) {
	base := filepath.Base(filepath.FromSlash(strings.ReplaceAll(name, `\`, "/")))
	if base == "." || base == string(filepath.Separator) || strings.Contains(base, "..") {
		return "", status.Error(codes.InvalidArgument, "invalid filename")
	}
	if !supportedModFile(base, kind) {
		return "", status.Error(codes.InvalidArgument, "filename must end in .jar (or .litemod for mods)")
	}
	return base, nil
}
