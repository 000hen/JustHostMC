package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestScriptService builds a ScriptService backed by an automation manager
// with no console/control (List/Import/grant flows don't need them) and a temp
// scripts dir.
func newTestScriptService(t *testing.T) (*ScriptService, *scripting.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	grants := scripting.NewGrantStore(filepath.Join(dir, "grants.json"))
	mgr := scripting.NewManager(scripting.NewHost(nil, nil, nil), grants, nil, nil, scripting.NewLogBuffer(0))
	enabled := scripting.NewEnabledStore(filepath.Join(dir, "enabled.json"))
	return NewScriptService(mgr, grants, enabled, dir), mgr, dir
}

const validScript = `
meta = {
  id = "auto1", name = "Auto One", author = "me", version = "1.0",
  permissions = { {kind = "console_read", reason = "watch"} },
}
on_log("srv1", function(line) end)
`

func TestScriptImportAndList(t *testing.T) {
	s, _, dir := newTestScriptService(t)
	ctx := context.Background()

	info, err := s.Import(ctx, &mcmanagerv1.ImportScriptRequest{LuaSource: validScript})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if info.Id != "auto1" || info.Name != "Auto One" {
		t.Errorf("info = %+v, want id=auto1 name=Auto One", info)
	}
	if info.Enabled {
		t.Error("freshly imported script should be disabled")
	}
	// Persisted to disk.
	if _, err := os.Stat(filepath.Join(dir, "auto1.lua")); err != nil {
		t.Errorf("script not persisted: %v", err)
	}

	list, err := s.List(ctx, &mcmanagerv1.Empty{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Scripts) != 1 || list.Scripts[0].Id != "auto1" {
		t.Errorf("List = %+v, want one auto1", list.Scripts)
	}
}

func TestScriptImportEmptyRejected(t *testing.T) {
	s, _, _ := newTestScriptService(t)
	_, err := s.Import(context.Background(), &mcmanagerv1.ImportScriptRequest{LuaSource: "  "})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("Import empty: code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestScriptImportBadScriptRejected(t *testing.T) {
	s, _, dir := newTestScriptService(t)
	_, err := s.Import(context.Background(), &mcmanagerv1.ImportScriptRequest{LuaSource: "this is not lua ("})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("Import bad: code = %v, want InvalidArgument", status.Code(err))
	}
	// Nothing should have been written.
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("bad script left files behind: %v", entries)
	}
}

func TestScriptSetEnabled(t *testing.T) {
	s, mgr, _ := newTestScriptService(t)
	ctx := context.Background()
	if _, err := s.Import(ctx, &mcmanagerv1.ImportScriptRequest{LuaSource: validScript}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	// on_log registration requires console_read; grant it so Enable succeeds.
	if _, err := s.SetPermissions(ctx, &mcmanagerv1.SetPermissionsRequest{
		Id:      "auto1",
		Granted: []mcmanagerv1.PermissionKind{mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_READ},
	}); err != nil {
		t.Fatalf("SetPermissions: %v", err)
	}

	info, err := s.SetEnabled(ctx, &mcmanagerv1.SetScriptEnabledRequest{Id: "auto1", Enabled: true})
	if err != nil {
		t.Fatalf("SetEnabled true: %v", err)
	}
	if !info.Enabled || !mgr.Enabled("auto1") {
		t.Error("script should be enabled")
	}

	info, err = s.SetEnabled(ctx, &mcmanagerv1.SetScriptEnabledRequest{Id: "auto1", Enabled: false})
	if err != nil {
		t.Fatalf("SetEnabled false: %v", err)
	}
	if info.Enabled || mgr.Enabled("auto1") {
		t.Error("script should be disabled")
	}
}

func TestScriptSetEnabledUnknown(t *testing.T) {
	s, _, _ := newTestScriptService(t)
	_, err := s.SetEnabled(context.Background(), &mcmanagerv1.SetScriptEnabledRequest{Id: "nope", Enabled: true})
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

func TestScriptSetPermissions(t *testing.T) {
	s, mgr, _ := newTestScriptService(t)
	ctx := context.Background()
	if _, err := s.Import(ctx, &mcmanagerv1.ImportScriptRequest{LuaSource: validScript}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	want := []mcmanagerv1.PermissionKind{mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_READ}
	info, err := s.SetPermissions(ctx, &mcmanagerv1.SetPermissionsRequest{Id: "auto1", Granted: want})
	if err != nil {
		t.Fatalf("SetPermissions: %v", err)
	}
	if len(info.Granted) != 1 || info.Granted[0] != mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_READ {
		t.Errorf("granted = %v, want console_read", info.Granted)
	}
	if !mgr.EffectiveGrants("auto1").Has(mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_READ) {
		t.Error("grant not effective in manager")
	}
}

func TestScriptRemove(t *testing.T) {
	s, mgr, dir := newTestScriptService(t)
	ctx := context.Background()
	if _, err := s.Import(ctx, &mcmanagerv1.ImportScriptRequest{LuaSource: validScript}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, err := s.Remove(ctx, &mcmanagerv1.ProviderRef{Id: "auto1"}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := mgr.Get("auto1"); ok {
		t.Error("script still registered after Remove")
	}
	if _, err := os.Stat(filepath.Join(dir, "auto1.lua")); !os.IsNotExist(err) {
		t.Errorf("script file not deleted: %v", err)
	}
	// Removing an unknown id is a no-op.
	if _, err := s.Remove(ctx, &mcmanagerv1.ProviderRef{Id: "ghost"}); err != nil {
		t.Errorf("Remove unknown: %v", err)
	}
}

func TestScriptRemoveBuiltinRejected(t *testing.T) {
	s, mgr, _ := newTestScriptService(t)
	if _, err := mgr.AddSource(validScript, true); err != nil {
		t.Fatalf("AddSource builtin: %v", err)
	}
	_, err := s.Remove(context.Background(), &mcmanagerv1.ProviderRef{Id: "auto1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", status.Code(err))
	}
}

// fakeLogStream is a minimal grpc.ServerStreamingServer[ScriptLogLine] that
// collects sent lines and is cancelled via its context. Send is guarded so the
// test goroutine can read recv safely.
type fakeLogStream struct {
	grpc.ServerStream
	ctx  context.Context
	mu   sync.Mutex
	recv []*mcmanagerv1.ScriptLogLine
	got  chan struct{} // signalled after each Send
}

func (f *fakeLogStream) Context() context.Context { return f.ctx }
func (f *fakeLogStream) Send(l *mcmanagerv1.ScriptLogLine) error {
	f.mu.Lock()
	f.recv = append(f.recv, l)
	f.mu.Unlock()
	select {
	case f.got <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakeLogStream) lines() []*mcmanagerv1.ScriptLogLine {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*mcmanagerv1.ScriptLogLine(nil), f.recv...)
}

func TestScriptStreamLogReplaysHistory(t *testing.T) {
	s, mgr, _ := newTestScriptService(t)
	mgr.Logs().Append("auto1", "hello")
	mgr.Logs().Append("auto1", "world")

	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeLogStream{ctx: ctx, got: make(chan struct{}, 8)}
	done := make(chan error, 1)
	go func() { done <- s.StreamLog(&mcmanagerv1.Empty{}, stream) }()

	// Wait for both history lines to be sent, then cancel to end the stream.
	for i := 0; i < 2; i++ {
		select {
		case <-stream.got:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for history replay")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StreamLog did not return after cancel")
	}

	lines := stream.lines()
	if len(lines) < 2 || lines[0].Line != "hello" || lines[1].Line != "world" {
		t.Errorf("recv = %+v, want hello/world history", lines)
	}
	if lines[0].ScriptId != "auto1" {
		t.Errorf("script id = %q, want auto1", lines[0].ScriptId)
	}
	if _, err := time.Parse(time.RFC3339Nano, lines[0].Timestamp); err != nil {
		t.Errorf("timestamp = %q, want RFC3339: %v", lines[0].Timestamp, err)
	}
	if lines[0].SessionId == "" || lines[0].SessionStartedAt == "" || !lines[0].CurrentSession {
		t.Errorf("session metadata = %+v, want current session id/start", lines[0])
	}
}
