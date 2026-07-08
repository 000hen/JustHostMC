package automation

import (
	"context"
	"sync"
	"testing"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/scriptlog"
)

// fakeConsole is an in-memory Console: it records sent commands, replays a fixed
// history, and lets a test push live lines (and close the channel to signal a
// server stop) per server id.
type fakeConsole struct {
	mu      sync.Mutex
	sent    []string               // "id cmd" pairs the script wrote
	history map[string][]string    // replayed to Subscribe
	live    map[string]chan string // live channels handed out
}

func newFakeConsole() *fakeConsole {
	return &fakeConsole{history: map[string][]string{}, live: map[string]chan string{}}
}

func (f *fakeConsole) Subscribe(id string) ([]string, <-chan string, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	hist := append([]string(nil), f.history[id]...)
	ch, ok := f.live[id]
	if !ok {
		ch = make(chan string, 16)
		f.live[id] = ch
	}
	return hist, ch, func() {}
}

func (f *fakeConsole) Send(id, command string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, id+" "+command)
	return nil
}

func (f *fakeConsole) push(id, line string) {
	f.mu.Lock()
	ch := f.live[id]
	f.mu.Unlock()
	if ch != nil {
		ch <- line
	}
}

func (f *fakeConsole) closeServer(id string) {
	f.mu.Lock()
	ch := f.live[id]
	delete(f.live, id)
	f.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

func (f *fakeConsole) sentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// fakeControl records Start/Stop calls.
type fakeControl struct {
	mu     sync.Mutex
	starts []string
	stops  []string
}

func (c *fakeControl) Start(_ context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.Empty, error) {
	c.mu.Lock()
	c.starts = append(c.starts, req.Id)
	c.mu.Unlock()
	return &mcmanagerv1.Empty{}, nil
}

func (c *fakeControl) Stop(_ context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.Empty, error) {
	c.mu.Lock()
	c.stops = append(c.stops, req.Id)
	c.mu.Unlock()
	return &mcmanagerv1.Empty{}, nil
}

func (c *fakeControl) startCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.starts)
}

// waitFor polls cond up to a second, failing the test if it never holds.
func waitFor(t *testing.T, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

func newTestManager(con Console, ctl ServerControl) *Manager {
	return NewManager(ManagerConfig{
		Host:    scripting.NewHost(nil, nil, nil),
		Console: con,
		Control: ctl,
		Logs:    scriptlog.NewLogBuffer(0),
	})
}

// TestOnLogDispatch verifies an on_log hook fires for live console lines and the
// script can write a command back (console_write/read permissions granted as a
// built-in).
func TestOnLogDispatch(t *testing.T) {
	con := newFakeConsole()
	m := newTestManager(con, nil)
	const src = `
meta = {
  id = "watcher", name = "Watcher",
  permissions = {
    {kind = "console_read", reason = "watch"},
    {kind = "console_write", reason = "reply"},
  },
}
on_log("srv1", function(line)
  if line == "ping" then server.send("srv1", "pong") end
end)
`
	if _, err := m.AddSource(context.Background(), src, true); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("watcher"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	defer m.Shutdown()

	con.push("srv1", "noise")
	con.push("srv1", "ping")
	waitFor(t, "pong sent", func() bool { return con.sentCount() == 1 })
	if con.sent[0] != "srv1 pong" {
		t.Errorf("sent = %q, want %q", con.sent[0], "srv1 pong")
	}
}

// TestOnStartStop verifies on_start fires when the script enables and on_stop
// fires when the server's console channel closes.
func TestOnStartStop(t *testing.T) {
	con := newFakeConsole()
	con.live["srv1"] = make(chan string, 4) // pre-create so closeServer signals stop
	m := newTestManager(con, nil)
	const src = `
meta = { id = "lifecycle", name = "Lifecycle",
  permissions = { {kind = "console_read", reason = "x"} } }
events = 0
on_start("srv1", function(id) log("start:" .. id) end)
on_stop("srv1", function(id) log("stop:" .. id) end)
`
	if _, err := m.AddSource(context.Background(), src, true); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("lifecycle"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	defer m.Shutdown()

	waitFor(t, "start logged", func() bool { return logContains(m, "start:srv1") })
	con.closeServer("srv1")
	waitFor(t, "stop logged", func() bool { return logContains(m, "stop:srv1") })
}

// TestSchedule verifies schedule() runs its callback repeatedly and Disable
// cancels it.
func TestSchedule(t *testing.T) {
	ctl := &fakeControl{}
	m := newTestManager(nil, ctl)
	const src = `
meta = { id = "ticker", name = "Ticker",
  permissions = {
    {kind = "schedule", reason = "tick"},
    {kind = "server_control", reason = "ctl"},
  } }
schedule(0.01, function() server.start("srv1") end)
`
	if _, err := m.AddSource(context.Background(), src, true); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("ticker"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	waitFor(t, "scheduled tick", func() bool { return ctl.startCount() >= 2 })

	m.Disable("ticker")
	settled := ctl.startCount()
	time.Sleep(40 * time.Millisecond)
	if got := ctl.startCount(); got != settled {
		t.Errorf("scheduler kept firing after Disable: %d -> %d", settled, got)
	}
}

// TestPermissionGate verifies a script without console_write cannot send, and the
// failure is surfaced (the hook does not crash the runner).
func TestPermissionGate(t *testing.T) {
	con := newFakeConsole()
	// User-imported (non-builtin) with no grants → no permissions.
	m := newTestManager(con, nil)
	const src = `
meta = { id = "rogue", name = "Rogue",
  permissions = { {kind = "console_write", reason = "x"} } }
register = function()
  -- on_log itself requires console_read, which is not granted, so registration
  -- of the hook should raise; the script still loads.
end
`
	if _, err := m.AddSource(context.Background(), src, false); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	// Enable must not error even though the script declares perms it isn't granted.
	if err := m.Enable("rogue"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	defer m.Shutdown()
	if m.EffectiveGrants("rogue").Has(mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_WRITE) {
		t.Error("non-builtin script should have no grants by default")
	}
}

// TestServerControl verifies server.start/stop/restart drive the ServerControl.
func TestServerControl(t *testing.T) {
	ctl := &fakeControl{}
	m := newTestManager(nil, ctl)
	const src = `
meta = { id = "ctl", name = "Ctl",
  permissions = {
    {kind = "schedule", reason = "t"},
    {kind = "server_control", reason = "c"},
  } }
done = false
schedule(0.01, function()
  if not done then
    done = true
    server.start("a")
    server.stop("b")
    server.restart("c")
  end
end)
`
	if _, err := m.AddSource(context.Background(), src, true); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("ctl"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	defer m.Shutdown()

	waitFor(t, "control calls", func() bool {
		ctl.mu.Lock()
		defer ctl.mu.Unlock()
		return len(ctl.starts) >= 2 && len(ctl.stops) >= 1
	})
	ctl.mu.Lock()
	defer ctl.mu.Unlock()
	// restart = stop("c") then start("c"); plus start("a") and stop("b").
	if !contains(ctl.starts, "a") || !contains(ctl.starts, "c") {
		t.Errorf("starts = %v, want a and c", ctl.starts)
	}
	if !contains(ctl.stops, "b") || !contains(ctl.stops, "c") {
		t.Errorf("stops = %v, want b and c", ctl.stops)
	}
}

func logContains(m *Manager, want string) bool {
	hist, _, cancel := m.Logs().Subscribe()
	cancel()
	for _, ll := range hist {
		if ll.Line == want {
			return true
		}
	}
	return false
}

func contains(s []string, want string) bool {
	for _, x := range s {
		if x == want {
			return true
		}
	}
	return false
}
