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

func (s *PlayerService) GetData(_ context.Context, req *mcmanagerv1.PlayerLookup) (*mcmanagerv1.PlayerData, error) {
	if req.ServerId == "" {
		return nil, status.Error(codes.InvalidArgument, "server_id required")
	}
	if _, ok := s.store.Get(req.ServerId); !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	name := strings.TrimSpace(req.Name)
	uuid := normalizeUUID(req.Uuid)
	if uuid == "" && name != "" {
		uuid = readUserCache(s.paths.ServerDir(req.ServerId)).uuidForName(name)
	}
	if uuid == "" {
		return nil, status.Error(codes.NotFound, "player UUID is not known yet")
	}

	path := filepath.Join(s.paths.ServerDir(req.ServerId), "world", "playerdata", uuid+".dat")
	var raw nbt.RawMessage
	if _, err := readNBTFile(path, &raw); err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "player data not found")
		}
		return nil, status.Errorf(codes.Internal, "read player data: %v", err)
	}

	var data playerNBT
	if _, err := readNBTFile(path, &data); err != nil {
		return nil, status.Errorf(codes.Internal, "decode player data: %v", err)
	}
	return &mcmanagerv1.PlayerData{
		ServerId:   req.ServerId,
		Name:       name,
		Uuid:       uuid,
		RawSnbt:    raw.String(),
		Inventory:  convertInventory(data.Inventory, false),
		EnderChest: convertInventory(data.EnderItems, true),
	}, nil
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
	Slot  int8   `nbt:"Slot"`
	ID    string `nbt:"id"`
	Count int8   `nbt:"Count"`
}

func convertInventory(items []nbt.RawMessage, ender bool) []*mcmanagerv1.PlayerInventoryItem {
	out := make([]*mcmanagerv1.PlayerInventoryItem, 0, len(items))
	for _, raw := range items {
		var item inventoryNBT
		if err := raw.Unmarshal(&item); err != nil {
			continue
		}
		slot := int32(item.Slot)
		out = append(out, &mcmanagerv1.PlayerInventoryItem{
			Slot:     slot,
			SlotName: slotName(slot, ender),
			ItemId:   item.ID,
			Count:    int32(item.Count),
			RawSnbt:  raw.String(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
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
