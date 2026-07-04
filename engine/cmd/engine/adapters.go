package main

import (
	"context"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	grpcsvc "github.com/000hen/justhostmc/engine/internal/grpc"
	"github.com/000hen/justhostmc/engine/internal/players"
	"github.com/000hen/justhostmc/engine/internal/scripting/automation"
	"github.com/000hen/justhostmc/engine/internal/store"
)

// serverQueryAdapter exposes the persisted server registry to automation
// scripts (server.list / server.info).
type serverQueryAdapter struct{ store store.Store }

func (a *serverQueryAdapter) ListServers() []automation.ServerInfo {
	recs := a.store.List()
	out := make([]automation.ServerInfo, 0, len(recs))
	for _, r := range recs {
		out = append(out, serverInfoFromRecord(r))
	}
	return out
}

func (a *serverQueryAdapter) GetServer(id string) (automation.ServerInfo, bool) {
	r, ok := a.store.Get(id)
	if !ok {
		return automation.ServerInfo{}, false
	}
	return serverInfoFromRecord(r), true
}

func serverInfoFromRecord(r *store.Server) automation.ServerInfo {
	return automation.ServerInfo{
		ID:        r.ID,
		Name:      r.Name,
		Provider:  r.ProviderID,
		McVersion: r.McVersion,
		Status:    r.Status.String(),
		Port:      r.Port,
		MemoryMB:  r.MemoryMB,
	}
}

// playerManagerAdapter bridges automation scripts to the player event bus
// (live roster) and the PlayerService (ban lists — reusing its validation,
// e.g. refusing ban edits while the server runs).
type playerManagerAdapter struct {
	events  *players.EventBus
	players *grpcsvc.PlayerService
}

func (a *playerManagerAdapter) OnlinePlayers(serverID string) []string {
	return a.events.OnlinePlayers(serverID)
}

func (a *playerManagerAdapter) ListBans(serverID string) ([]automation.BanInfo, error) {
	resp, err := a.players.ListBans(context.Background(), &mcmanagerv1.ServerId{Id: serverID})
	if err != nil {
		return nil, err
	}
	out := make([]automation.BanInfo, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		out = append(out, automation.BanInfo{
			Type:    banTypeName(e.Type),
			Target:  e.Target,
			Reason:  e.Reason,
			Created: e.Created,
		})
	}
	return out, nil
}

func (a *playerManagerAdapter) AddBan(serverID, target, reason string) error {
	_, err := a.players.AddBan(context.Background(), &mcmanagerv1.AddBanRequest{
		ServerId: serverID,
		Target:   target,
		Reason:   reason,
	})
	return err
}

func (a *playerManagerAdapter) RemoveBan(serverID, target string) error {
	_, err := a.players.RemoveBan(context.Background(), &mcmanagerv1.RemoveBanRequest{
		ServerId: serverID,
		Target:   target,
	})
	return err
}

func banTypeName(t mcmanagerv1.BanListType) string {
	if t == mcmanagerv1.BanListType_IP_BANS {
		return "ip"
	}
	return "player"
}
