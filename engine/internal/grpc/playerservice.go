package grpcsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/console"
	"github.com/000hen/justhostmc/engine/internal/players"
	"github.com/000hen/justhostmc/engine/internal/store"
	"github.com/Tnze/go-mc/nbt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PlayerService streams a server's online roster, reconstructs player file data,
// and manages vanilla ban-list JSON files.
type PlayerService struct {
	mcmanagerv1.UnimplementedPlayerServiceServer
	hub   *console.Hub
	store store.Store
	paths appdata.Paths
}

// NewPlayerService builds a PlayerService over the console hub and server data
// paths. The hub supplies the live roster; paths supply playerdata and ban files.
func NewPlayerService(hub *console.Hub, st store.Store, paths appdata.Paths) *PlayerService {
	return &PlayerService{hub: hub, store: st, paths: paths}
}

// Watch subscribes to a server's console, seeds the roster from buffered history
// plus a fresh "list" command, then streams the roster whenever it changes.
func (s *PlayerService) Watch(req *mcmanagerv1.ServerId, stream grpc.ServerStreamingServer[mcmanagerv1.PlayerList]) error {
	if req.Id == "" {
		return status.Error(codes.InvalidArgument, "server_id required")
	}

	roster := players.NewRoster()
	history, live, cancel := s.hub.Subscribe(req.Id)
	defer cancel()

	for _, line := range history {
		roster.Apply(line)
	}
	// Best-effort: ask the server to report its roster so we don't rely solely on
	// join/leave events that may have scrolled out of the ring buffer. Ignored when
	// the server isn't running.
	_ = s.hub.Send(req.Id, "list")

	if err := s.sendRoster(stream, req.Id, roster); err != nil {
		return err
	}

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-live:
			if !ok {
				return nil
			}
			if roster.Apply(line) {
				if err := s.sendRoster(stream, req.Id, roster); err != nil {
					return err
				}
			}
		}
	}
}

func (s *PlayerService) sendRoster(stream grpc.ServerStreamingServer[mcmanagerv1.PlayerList], id string, roster *players.Roster) error {
	names := roster.Names()
	list := &mcmanagerv1.PlayerList{ServerId: id, Players: make([]*mcmanagerv1.PlayerInfo, 0, len(names))}
	cache := readUserCache(s.paths.ServerDir(id))
	for _, n := range names {
		list.Players = append(list.Players, &mcmanagerv1.PlayerInfo{Name: n, Uuid: cache.uuidForName(n)})
	}
	return stream.Send(list)
}

func (s *PlayerService) GetData(ctx context.Context, req *mcmanagerv1.PlayerLookup) (*mcmanagerv1.PlayerData, error) {
	if req.ServerId == "" {
		return nil, status.Error(codes.InvalidArgument, "server_id required")
	}
	rec, ok := s.store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	serverDir := s.paths.ServerDir(req.ServerId)
	name := strings.TrimSpace(req.Name)
	uuid := normalizeUUID(req.Uuid)
	if uuid == "" && name != "" {
		uuid = readUserCache(serverDir).uuidForName(name)
	}
	if uuid == "" {
		return nil, status.Error(codes.NotFound, "player UUID is not known yet")
	}

	var raw nbt.RawMessage
	var data playerNBT
	liveDataLoaded := false
	// A save file can still lag behind an online player's entity state (notably
	// the offhand and item components). Query the live entity after flushing and
	// use the file only as the offline/fallback source.
	if rec.Status == mcmanagerv1.ServerStatus_RUNNING {
		_, live, cancel := s.hub.Subscribe(req.ServerId)
		if err := s.hub.Send(req.ServerId, "save-all flush"); err == nil {
			waitForFlush(ctx, live, 5*time.Second)
		}
		if validPlayerCommandName(name) {
			if err := s.hub.Send(req.ServerId, "data get entity "+name); err == nil {
				if liveRaw, ok := waitForEntityData(ctx, live, name, 2*time.Second); ok {
					if err := liveRaw.Unmarshal(&data); err == nil {
						raw = liveRaw
						liveDataLoaded = true
					}
				}
			}
		}
		cancel()
	}

	if !liveDataLoaded {
		path, found, err := locatePlayerData(serverDir, uuid)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "locate player data: %v", err)
		}
		// Some server implementations report save completion just before the atomic
		// file rename becomes visible. Briefly poll only when the file is still absent.
		if !found && rec.Status == mcmanagerv1.ServerStatus_RUNNING {
			path, found, err = waitForPlayerData(ctx, serverDir, uuid, 3*time.Second)
			if err != nil {
				return nil, err
			}
		}
		if !found {
			return nil, status.Error(codes.NotFound, "player data not found")
		}

		if _, err := readNBTFile(path, &raw); err != nil {
			if os.IsNotExist(err) {
				return nil, status.Error(codes.NotFound, "player data not found")
			}
			return nil, status.Errorf(codes.Internal, "read player data: %v", err)
		}
		if _, err := readNBTFile(path, &data); err != nil {
			return nil, status.Errorf(codes.Internal, "decode player data: %v", err)
		}
	}
	// Item models are client resources and are not present in a dedicated server
	// jar. Cache the exact matching official client archive once; server-local
	// resource packs and mod jars are indexed afterward and retain precedence.
	clientArchive := localMinecraftClient(rec.McVersion)
	if clientArchive == "" {
		clientArchive, _ = ensureMinecraftClient(ctx, s.paths, rec.McVersion)
	}
	assets := newItemAssetResolver(serverDir, rec.McVersion, clientArchive)
	return &mcmanagerv1.PlayerData{
		ServerId:   req.ServerId,
		Name:       name,
		Uuid:       uuid,
		RawSnbt:    raw.String(),
		Inventory:  convertInventory(data.Inventory, false, assets),
		EnderChest: convertInventory(data.EnderItems, true, assets),
	}, nil
}

func validPlayerCommandName(name string) bool {
	if len(name) < 1 || len(name) > 16 {
		return false
	}
	for _, character := range name {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func waitForEntityData(ctx context.Context, live <-chan string, name string, timeout time.Duration) (nbt.RawMessage, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	marker := name + " has the following entity data: "
	for {
		select {
		case <-ctx.Done():
			return nbt.RawMessage{}, false
		case <-timer.C:
			return nbt.RawMessage{}, false
		case line, ok := <-live:
			if !ok {
				return nbt.RawMessage{}, false
			}
			index := strings.Index(line, marker)
			if index < 0 {
				continue
			}
			snbt := strings.TrimSpace(line[index+len(marker):])
			encoded, err := nbt.Marshal(nbt.StringifiedMessage(snbt))
			if err != nil {
				continue
			}
			var raw nbt.RawMessage
			if err := nbt.Unmarshal(encoded, &raw); err == nil {
				return raw, true
			}
		}
	}
}

// locatePlayerData honors server.properties' level-name and also discovers an
// existing world folder. Discovery keeps older/custom instances working when
// their properties file is absent or no longer reflects the folder on disk.
func locatePlayerData(serverDir, uuid string) (string, bool, error) {
	fileName := uuid + ".dat"
	levelName := "world"
	props, err := readPropertiesFile(filepath.Join(serverDir, "server.properties"))
	if err != nil {
		return "", false, err
	}
	if configured := strings.TrimSpace(props["level-name"]); configured != "" {
		levelName = configured
	}

	worldDir := filepath.Join(serverDir, filepath.FromSlash(levelName))
	expected := filepath.Join(worldDir, "playerdata", fileName)
	if insideDir(serverDir, worldDir) {
		for _, candidate := range playerDataCandidates(worldDir, fileName) {
			if found, err := regularFileExists(candidate); err != nil || found {
				return candidate, found, err
			}
		}
	}

	entries, err := os.ReadDir(serverDir)
	if err != nil {
		if os.IsNotExist(err) {
			return expected, false, nil
		}
		return "", false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidateWorld := filepath.Join(serverDir, entry.Name())
		if candidateWorld == worldDir {
			continue
		}
		for _, candidate := range playerDataCandidates(candidateWorld, fileName) {
			if found, err := regularFileExists(candidate); err != nil || found {
				return candidate, found, err
			}
		}
	}
	return expected, false, nil
}

func playerDataCandidates(worldDir, fileName string) []string {
	return []string{
		filepath.Join(worldDir, "playerdata", fileName),      // Minecraft through 1.21.x
		filepath.Join(worldDir, "players", "data", fileName), // Minecraft 26.1+
	}
}

func waitForPlayerData(ctx context.Context, serverDir, uuid string, timeout time.Duration) (string, bool, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		path, found, err := locatePlayerData(serverDir, uuid)
		if err != nil || found {
			return path, found, err
		}
		select {
		case <-ctx.Done():
			return "", false, status.FromContextError(ctx.Err()).Err()
		case <-timer.C:
			return path, false, nil
		case <-ticker.C:
		}
	}
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func insideDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (s *PlayerService) ListBans(_ context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.BanList, error) {
	if _, ok := s.store.Get(req.Id); !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	entries, err := s.readBans(req.Id)
	if err != nil {
		return nil, err
	}
	return &mcmanagerv1.BanList{ServerId: req.Id, Entries: entries}, nil
}

func (s *PlayerService) AddBan(_ context.Context, req *mcmanagerv1.AddBanRequest) (*mcmanagerv1.BanEntry, error) {
	rec, ok := s.store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	if !isEditableStopped(rec.Status) {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SERVER_RUNNING,
			"stop the server before changing ban lists", nil)
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		return nil, status.Error(codes.InvalidArgument, "ban target is required")
	}
	entry := &mcmanagerv1.BanEntry{
		Type:    req.Type,
		Target:  target,
		Created: time.Now().UTC().Format("2006-01-02 15:04:05 -0700"),
		Source:  "JustHostMC",
		Expires: "forever",
		Reason:  strings.TrimSpace(req.Reason),
	}
	if entry.Reason == "" {
		entry.Reason = "Banned by an operator."
	}
	if entry.Type == mcmanagerv1.BanListType_BAN_LIST_TYPE_UNSPECIFIED {
		if strings.Contains(target, ".") || strings.Contains(target, ":") {
			entry.Type = mcmanagerv1.BanListType_IP_BANS
		} else {
			entry.Type = mcmanagerv1.BanListType_PLAYER_BANS
		}
	}
	if entry.Type == mcmanagerv1.BanListType_PLAYER_BANS {
		entry.Name = target
		if uuid := normalizeUUID(target); uuid != "" {
			entry.Uuid = uuid
			entry.Name = ""
		} else if uuid := readUserCache(s.paths.ServerDir(req.ServerId)).uuidForName(target); uuid != "" {
			entry.Uuid = uuid
		}
	} else {
		entry.Target = target
	}

	if err := s.upsertBan(req.ServerId, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *PlayerService) RemoveBan(_ context.Context, req *mcmanagerv1.RemoveBanRequest) (*mcmanagerv1.Empty, error) {
	rec, ok := s.store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	if !isEditableStopped(rec.Status) {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SERVER_RUNNING,
			"stop the server before changing ban lists", nil)
	}
	if err := s.removeBan(req.ServerId, req.Type, req.Target); err != nil {
		return nil, err
	}
	return &mcmanagerv1.Empty{}, nil
}

type playerNBT struct {
	Inventory  []nbt.RawMessage `nbt:"Inventory"`
	EnderItems []nbt.RawMessage `nbt:"EnderItems"`
}

type inventoryNBT struct {
	Slot        int8   `nbt:"Slot"`
	ID          string `nbt:"id"`
	Count       int32  `nbt:"count"`
	LegacyCount int8   `nbt:"Count"`
}

func convertInventory(items []nbt.RawMessage, ender bool, assets *itemAssetResolver) []*mcmanagerv1.PlayerInventoryItem {
	out := make([]*mcmanagerv1.PlayerInventoryItem, 0, len(items))
	for _, raw := range items {
		var item inventoryNBT
		if err := raw.Unmarshal(&item); err != nil {
			continue
		}
		slot := canonicalInventorySlot(int32(item.Slot), ender)
		count := item.Count
		if count == 0 {
			count = int32(item.LegacyCount)
		}
		converted := &mcmanagerv1.PlayerInventoryItem{
			Slot:     slot,
			SlotName: slotName(slot, ender),
			ItemId:   item.ID,
			Count:    count,
			RawSnbt:  raw.String(),
			Details:  extractItemDetails(raw),
		}
		if assets != nil {
			asset := assets.Resolve(item.ID)
			converted.ModelJson = asset.ModelJSON
			refs := make([]string, 0, len(asset.Textures))
			for ref := range asset.Textures {
				refs = append(refs, ref)
			}
			sort.Strings(refs)
			for _, ref := range refs {
				converted.Textures = append(converted.Textures, &mcmanagerv1.PlayerItemTexture{
					Id:  ref,
					Png: asset.Textures[ref],
				})
			}
		}
		out = append(out, converted)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

func canonicalInventorySlot(slot int32, ender bool) int32 {
	if !ender {
		// Vanilla player NBT uses signed byte -106 (the byte value 150), while
		// inventory APIs and some server implementations serialize logical slot
		// 40. Normalize every representation before it reaches WinUI.
		if slot == -106 || slot == 40 || slot == 150 {
			return -106
		}
	}
	return slot
}

func slotName(slot int32, ender bool) string {
	if ender {
		return fmt.Sprintf("Ender chest %d", slot+1)
	}
	switch {
	case slot >= 0 && slot <= 8:
		return fmt.Sprintf("Hotbar %d", slot+1)
	case slot >= 9 && slot <= 35:
		return fmt.Sprintf("Inventory %d", slot-8)
	case slot == 100:
		return "Boots"
	case slot == 101:
		return "Leggings"
	case slot == 102:
		return "Chestplate"
	case slot == 103:
		return "Helmet"
	case slot == -106:
		return "Offhand"
	default:
		return fmt.Sprintf("Slot %d", slot)
	}
}

type userCache []struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

func readUserCache(serverDir string) userCache {
	data, err := os.ReadFile(filepath.Join(serverDir, "usercache.json"))
	if err != nil {
		return nil
	}
	var cache userCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	return cache
}

func (c userCache) uuidForName(name string) string {
	for _, entry := range c {
		if strings.EqualFold(entry.Name, name) {
			return normalizeUUID(entry.UUID)
		}
	}
	return ""
}

func normalizeUUID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 32 {
		return ""
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ""
		}
	}
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

type banJSON struct {
	UUID    string `json:"uuid,omitempty"`
	Name    string `json:"name,omitempty"`
	IP      string `json:"ip,omitempty"`
	Created string `json:"created"`
	Source  string `json:"source"`
	Expires string `json:"expires"`
	Reason  string `json:"reason"`
}

func (s *PlayerService) readBans(serverID string) ([]*mcmanagerv1.BanEntry, error) {
	var entries []*mcmanagerv1.BanEntry
	playerBans, err := readBanFile(filepath.Join(s.paths.ServerDir(serverID), "banned-players.json"))
	if err != nil {
		return nil, err
	}
	for _, b := range playerBans {
		target := b.Name
		if target == "" {
			target = b.UUID
		}
		entries = append(entries, &mcmanagerv1.BanEntry{
			Type:    mcmanagerv1.BanListType_PLAYER_BANS,
			Target:  target,
			Name:    b.Name,
			Uuid:    normalizeUUID(b.UUID),
			Created: b.Created,
			Source:  b.Source,
			Expires: b.Expires,
			Reason:  b.Reason,
		})
	}
	ipBans, err := readBanFile(filepath.Join(s.paths.ServerDir(serverID), "banned-ips.json"))
	if err != nil {
		return nil, err
	}
	for _, b := range ipBans {
		entries = append(entries, &mcmanagerv1.BanEntry{
			Type:    mcmanagerv1.BanListType_IP_BANS,
			Target:  b.IP,
			Created: b.Created,
			Source:  b.Source,
			Expires: b.Expires,
			Reason:  b.Reason,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type < entries[j].Type
		}
		return strings.ToLower(entries[i].Target) < strings.ToLower(entries[j].Target)
	})
	return entries, nil
}

func (s *PlayerService) upsertBan(serverID string, entry *mcmanagerv1.BanEntry) error {
	path := banPath(s.paths.ServerDir(serverID), entry.Type)
	items, err := readBanFile(path)
	if err != nil {
		return err
	}
	next := banJSON{
		UUID:    entry.Uuid,
		Name:    entry.Name,
		Created: entry.Created,
		Source:  entry.Source,
		Expires: entry.Expires,
		Reason:  entry.Reason,
	}
	if entry.Type == mcmanagerv1.BanListType_IP_BANS {
		next.IP = entry.Target
	}
	replaced := false
	for i := range items {
		if banMatches(items[i], entry.Type, entry.Target) {
			items[i] = next
			replaced = true
			break
		}
	}
	if !replaced {
		items = append(items, next)
	}
	return writeBanFile(path, items)
}

func (s *PlayerService) removeBan(serverID string, typ mcmanagerv1.BanListType, target string) error {
	if typ == mcmanagerv1.BanListType_BAN_LIST_TYPE_UNSPECIFIED {
		if strings.Contains(target, ".") || strings.Contains(target, ":") {
			typ = mcmanagerv1.BanListType_IP_BANS
		} else {
			typ = mcmanagerv1.BanListType_PLAYER_BANS
		}
	}
	path := banPath(s.paths.ServerDir(serverID), typ)
	items, err := readBanFile(path)
	if err != nil {
		return err
	}
	filtered := items[:0]
	for _, item := range items {
		if !banMatches(item, typ, target) {
			filtered = append(filtered, item)
		}
	}
	return writeBanFile(path, filtered)
}

func banPath(serverDir string, typ mcmanagerv1.BanListType) string {
	if typ == mcmanagerv1.BanListType_IP_BANS {
		return filepath.Join(serverDir, "banned-ips.json")
	}
	return filepath.Join(serverDir, "banned-players.json")
}

func readBanFile(path string) ([]banJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, status.Errorf(codes.Internal, "read %s: %v", filepath.Base(path), err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var out []banJSON
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, status.Errorf(codes.Internal, "parse %s: %v", filepath.Base(path), err)
	}
	return out, nil
}

func writeBanFile(path string, items []banJSON) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return status.Errorf(codes.Internal, "create ban dir: %v", err)
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return status.Errorf(codes.Internal, "encode ban list: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return status.Errorf(codes.Internal, "write %s: %v", filepath.Base(path), err)
	}
	return nil
}

func banMatches(item banJSON, typ mcmanagerv1.BanListType, target string) bool {
	target = strings.TrimSpace(target)
	if typ == mcmanagerv1.BanListType_IP_BANS {
		return strings.EqualFold(item.IP, target)
	}
	uuid := normalizeUUID(target)
	if uuid != "" && normalizeUUID(item.UUID) == uuid {
		return true
	}
	return strings.EqualFold(item.Name, target) || strings.EqualFold(item.UUID, target)
}
