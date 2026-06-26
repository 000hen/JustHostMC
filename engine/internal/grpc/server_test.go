package grpcsvc

import (
	"context"
	"net"
	"testing"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestListenBindsLoopbackRandomPort(t *testing.T) {
	lis, err := Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer lis.Close()

	addr, ok := lis.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr type = %T, want *net.TCPAddr", lis.Addr())
	}
	if !addr.IP.IsLoopback() {
		t.Errorf("bound to %v, want loopback only", addr.IP)
	}
	if addr.Port == 0 {
		t.Errorf("port = 0, want an OS-assigned port")
	}
}

// startTestServer spins up the real gRPC server on a loopback port and returns
// an authenticated client plus the raw connection for negative tests.
func startTestServer(t *testing.T, token string) (mcmanagerv1.EngineServiceClient, *grpc.ClientConn) {
	t.Helper()

	lis, err := Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := New(token)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return mcmanagerv1.NewEngineServiceClient(conn), conn
}

func TestHealthSucceedsWithToken(t *testing.T) {
	const token = "test-token"
	client, _ := startTestServer(t, token)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, tokenMetadataKey, token)

	if _, err := client.Health(ctx, &mcmanagerv1.Empty{}); err != nil {
		t.Fatalf("Health with valid token: %v", err)
	}
}

func TestHealthRejectedWithoutToken(t *testing.T) {
	client, _ := startTestServer(t, "test-token")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Health(ctx, &mcmanagerv1.Empty{})
	if got := status.Code(err); got != codes.Unauthenticated {
		t.Fatalf("Health without token: code = %v, want Unauthenticated", got)
	}
}
