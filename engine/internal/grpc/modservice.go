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
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maxModBytes caps a single uploaded plugin/mod archive; individual files are far
// smaller, so this just guards against a runaway or malicious upload.
const maxModBytes = 512 << 20 // 512 MiB

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
	// unchanged jars. Re-uploading a jar changes its mtime, which
	// self-invalidates the old entry (stale keys are dropped per server).
	metaMu    sync.Mutex
	metaCache map[string]map[string]*mcmanagerv1.ModMetadata // server id -> key -> metadata
}

// ModParser extracts embedded metadata from one jar (path relative to the
// server dir). *scripting.ParserSet satisfies it; matched=false means no
// installed parser recognized the jar.
type ModParser interface {
	ParseJar(ctx context.Context, serverDir, jarRel string) (scripting.ModMeta, string, bool)
}

// NewModService builds a ModService over the registry and data paths. parser
// may be nil (metadata then reports parsed=false for every jar).
func NewModService(st store.Store, paths appdata.Paths, parser ModParser) *ModService {
	return &ModService{
		store:     st,
		paths:     paths,
		parser:    parser,
		metaCache: map[string]map[string]*mcmanagerv1.ModMetadata{},
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
// with parsed metadata (name/version/authors/icon/...) where a parser matches.
func (s *ModService) List(ctx context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.ModList, error) {
	rec, ok := s.store.Get(req.Id)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	subdir, kind, ok := modLayout(rec.ModLayout)
	if !ok {
		return &mcmanagerv1.ModList{ServerId: req.Id, Supported: false}, nil
	}

	dir := filepath.Join(s.paths.ServerDir(req.Id), subdir)
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "read %s: %v", subdir, err)
	}

	list := &mcmanagerv1.ModList{ServerId: req.Id, Kind: kind, Supported: true}
	fresh := map[string]*mcmanagerv1.ModMetadata{}
	for _, e := range entries {
		if e.IsDir() || !supportedModFile(e.Name(), kind) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		meta := s.jarMetadata(ctx, req.Id, subdir, e.Name(), info.Size(), info.ModTime().UnixNano(), fresh)
		list.Files = append(list.Files, &mcmanagerv1.ModFile{
			Name:      e.Name(),
			SizeBytes: info.Size(),
			Metadata:  meta,
		})
	}
	// Replace the server's cache with only the keys seen this pass so removed
	// or re-uploaded jars don't accumulate stale entries.
	s.metaMu.Lock()
	s.metaCache[req.Id] = fresh
	s.metaMu.Unlock()
	sort.Slice(list.Files, func(i, j int) bool { return list.Files[i].Name < list.Files[j].Name })
	return list, nil
}

// jarMetadata returns the (possibly cached) parsed metadata for one jar and
// records it in fresh under its cache key.
func (s *ModService) jarMetadata(ctx context.Context, serverID, subdir, name string, size, mtime int64, fresh map[string]*mcmanagerv1.ModMetadata) *mcmanagerv1.ModMetadata {
	key := fmt.Sprintf("%s|%d|%d", name, size, mtime)
	s.metaMu.Lock()
	cached, ok := s.metaCache[serverID][key]
	s.metaMu.Unlock()
	if ok {
		fresh[key] = cached
		return cached
	}

	meta := &mcmanagerv1.ModMetadata{}
	if s.parser != nil {
		if m, parserID, matched := s.parser.ParseJar(ctx, s.paths.ServerDir(serverID), subdir+"/"+name); matched {
			meta = &mcmanagerv1.ModMetadata{
				Parsed:      true,
				ParserId:    parserID,
				Loader:      m.Loader,
				ModId:       m.ModID,
				Name:        m.Name,
				Version:     m.Version,
				Authors:     m.Authors,
				Description: m.Description,
				Website:     m.Website,
				Icon:        m.Icon,
			}
		}
	}
	fresh[key] = meta
	return meta
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
