package grpcsvc

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	winio "github.com/Microsoft/go-winio"
	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/console"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type fakeInstance struct {
	id        string
	out       chan string
	done      chan struct{}
	exitErr   error
	emitSaved bool // echo "Saved the game" in response to "save-all flush"

	mu      sync.Mutex
	written []string
}

func (f *fakeInstance) ID() string            { return f.id }
func (f *fakeInstance) Output() <-chan string { return f.out }
func (f *fakeInstance) Done() <-chan struct{} { return f.done }
func (f *fakeInstance) ExitErr() error        { return f.exitErr }

func (f *fakeInstance) Running() bool {
	select {
	case <-f.done:
		return false
	default:
		return true
	}
}

func (f *fakeInstance) WriteStdin(line string) error {
	f.mu.Lock()
	f.written = append(f.written, line)
	emit := f.emitSaved && line == "save-all flush"
	f.mu.Unlock()
	if emit {
		f.out <- "[12:00:00] [Server thread/INFO]: Saved the game"
	}
	return nil
}

func (f *fakeInstance) wrote(line string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.written {
		if w == line {
			return true
		}
	}
	return false
}

func TestConsoleServiceStreamsOutputAndForwardsCommands(t *testing.T) {
	hub := console.NewHub()
	fake := &fakeInstance{out: make(chan string, 16), done: make(chan struct{})}
	hub.Register("s1", fake)

	pipeName := "test-jhmc-console-" + randomSuffix()
	lis, err := ListenPipe(pipeName)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Config{ConsoleService: NewConsoleService(hub)})
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
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := mcmanagerv1.NewConsoleServiceClient(conn)

	// Push a line and wait until it lands in the ring so attach replays it.
	fake.out <- "hello from server"
	waitForRing(t, hub, "s1")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.Attach(ctx)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := stream.Send(&mcmanagerv1.ConsoleInput{ServerId: "s1"}); err != nil {
		t.Fatalf("send subscribe: %v", err)
	}

	ev, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if ev.Line != "hello from server" {
		t.Fatalf("first line = %q, want history replay", ev.Line)
	}

	// A live line after subscription is streamed too.
	fake.out <- "live line"
	ev, err = stream.Recv()
	if err != nil {
		t.Fatalf("recv live: %v", err)
	}
	if ev.Line != "live line" {
		t.Errorf("live line = %q", ev.Line)
	}

	// Commands are forwarded to the instance stdin.
	if err := stream.Send(&mcmanagerv1.ConsoleInput{ServerId: "s1", Command: "say hi"}); err != nil {
		t.Fatalf("send command: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !fake.wrote("say hi") {
		if time.Now().After(deadline) {
			t.Fatal("command 'say hi' was not forwarded to stdin")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForRing(t *testing.T, hub *console.Hub, id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		history, _, cancel := hub.Subscribe(id)
		cancel()
		if len(history) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("line never reached the console ring buffer")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
