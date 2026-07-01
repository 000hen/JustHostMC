package grpcsvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"testing"
	"time"

	winio "github.com/Microsoft/go-winio"
	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func randomSuffix() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func TestListenPipeCreatesNamedPipe(t *testing.T) {
	name := "test-jhmc-" + randomSuffix()
	lis, err := ListenPipe(name)
	if err != nil {
		t.Fatalf("ListenPipe: %v", err)
	}
	if lis == nil {
		t.Fatal("listener is nil")
	}
	if err := lis.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// startTestServer spins up the real gRPC server on a named pipe and returns
// a client plus the raw connection.
func startTestServer(t *testing.T) (mcmanagerv1.EngineServiceClient, *grpc.ClientConn) {
	t.Helper()

	pipeName := "test-jhmc-srv-" + randomSuffix()
	lis, err := ListenPipe(pipeName)
	if err != nil {
		t.Fatalf("ListenPipe: %v", err)
	}
	srv := New()
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	pipePath := `\\.\pipe\` + pipeName
	conn, err := grpc.NewClient(
		"passthrough:///pipe",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return winio.DialPipeContext(ctx, pipePath)
		}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return mcmanagerv1.NewEngineServiceClient(conn), conn
}

func TestHealthSucceeds(t *testing.T) {
	client, _ := startTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Health(ctx, &mcmanagerv1.Empty{}); err != nil {
		t.Fatalf("Health: %v", err)
	}
}
