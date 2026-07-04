// Package automation runs the sandboxed Lua automation scripts that react to
// and drive running servers (console hooks, timers, player events, server
// control). It builds on the parent scripting package's sandbox and jhmc host
// API; the scripting package never imports automation.
package automation

import (
	"context"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/players"
)

// Console is the subset of the console hub the automation host needs. A running
// server's hub (*console.Hub) satisfies it. It is injected to avoid an import
// cycle between scripting and the console/grpc packages.
type Console interface {
	// Subscribe returns the buffered history plus a live channel of subsequent
	// lines; cancel unsubscribes (closing the live channel).
	Subscribe(id string) (history []string, live <-chan string, cancel func())
	// Send writes a command line to the server's stdin.
	Send(id, command string) error
}

// ServerControl is the subset of the server service the automation host needs to
// start/stop servers. *grpcsvc.ServerService satisfies it; injecting an
// interface keeps this package free of a grpc import.
type ServerControl interface {
	Start(ctx context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.Empty, error)
	Stop(ctx context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.Empty, error)
}

// ServerInfo is the read-only view of a registered server exposed to scripts.
type ServerInfo struct {
	ID, Name, Provider, McVersion, Status string
	Port, MemoryMB                        int
}

// ServerQuery lets scripts list/inspect registered servers (server.list/info).
type ServerQuery interface {
	ListServers() []ServerInfo
	GetServer(id string) (ServerInfo, bool)
}

// BanInfo is one entry of a server's ban list as exposed to scripts.
type BanInfo struct {
	Type, Target, Reason, Created string
}

// PlayerManager lets scripts query and manage players without direct access to
// the filesystem or console parsing.
type PlayerManager interface {
	OnlinePlayers(serverID string) []string
	ListBans(serverID string) ([]BanInfo, error)
	AddBan(serverID, target, reason string) error
	RemoveBan(serverID, target string) error
}

// PlayerEvents is the source of structured join/leave events (on_join/on_leave).
// *players.EventBus satisfies it; events are derived from roster state diffs,
// never from scripts parsing console lines themselves.
type PlayerEvents interface {
	Subscribe() (live <-chan players.PlayerEvent, cancel func())
}
