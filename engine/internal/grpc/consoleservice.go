package grpcsvc

import (
	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/console"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ConsoleService bridges a server's console to the frontend over a bidirectional
// stream: inbound ConsoleInput carries commands (written to stdin), outbound
// ConsoleEvent carries output lines (history first, then live).
type ConsoleService struct {
	mcmanagerv1.UnimplementedConsoleServiceServer
	hub *console.Hub
}

// NewConsoleService builds a ConsoleService over the given hub.
func NewConsoleService(hub *console.Hub) *ConsoleService {
	return &ConsoleService{hub: hub}
}

// Attach subscribes the caller to a server's console. The first ConsoleInput
// selects the server (server_id); subsequent ones send commands.
func (s *ConsoleService) Attach(stream grpc.BidiStreamingServer[mcmanagerv1.ConsoleInput, mcmanagerv1.ConsoleEvent]) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	serverID := first.ServerId
	if serverID == "" {
		return status.Error(codes.InvalidArgument, "first ConsoleInput must set server_id")
	}

	history, live, cancel := s.hub.Subscribe(serverID)
	defer cancel()

	// Replay buffered history so the client sees recent output immediately.
	for _, line := range history {
		if err := stream.Send(&mcmanagerv1.ConsoleEvent{ServerId: serverID, Line: line}); err != nil {
			return err
		}
	}
	if first.Command != "" {
		_ = s.hub.Send(serverID, first.Command)
	}

	// Read further commands concurrently (one reader, one writer is allowed).
	ctx := stream.Context()
	go func() {
		for {
			in, recvErr := stream.Recv()
			if recvErr != nil {
				return
			}
			if in.Command != "" {
				_ = s.hub.Send(in.ServerId, in.Command)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-live:
			if !ok {
				return nil
			}
			if err := stream.Send(&mcmanagerv1.ConsoleEvent{ServerId: serverID, Line: line}); err != nil {
				return err
			}
		}
	}
}
