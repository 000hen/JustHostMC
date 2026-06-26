package grpcsvc

import (
	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/console"
	"github.com/000hen/justhostmc/engine/internal/players"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PlayerService streams a server's online roster, reconstructed from its console
// output by a players.Roster (Minecraft exposes no roster API).
type PlayerService struct {
	mcmanagerv1.UnimplementedPlayerServiceServer
	hub *console.Hub
}

// NewPlayerService builds a PlayerService over the given console hub.
func NewPlayerService(hub *console.Hub) *PlayerService {
	return &PlayerService{hub: hub}
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

	if err := sendRoster(stream, req.Id, roster); err != nil {
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
				if err := sendRoster(stream, req.Id, roster); err != nil {
					return err
				}
			}
		}
	}
}

func sendRoster(stream grpc.ServerStreamingServer[mcmanagerv1.PlayerList], id string, roster *players.Roster) error {
	names := roster.Names()
	list := &mcmanagerv1.PlayerList{ServerId: id, Players: make([]*mcmanagerv1.PlayerInfo, 0, len(names))}
	for _, n := range names {
		list.Players = append(list.Players, &mcmanagerv1.PlayerInfo{Name: n})
	}
	return stream.Send(list)
}
