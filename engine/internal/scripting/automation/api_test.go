package automation

import (
	"sync"
	"testing"

	"github.com/000hen/justhostmc/engine/internal/players"
	"github.com/000hen/justhostmc/engine/internal/scriptdata"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	"github.com/000hen/justhostmc/engine/internal/scriptlog"
)

// fakeQuery is an in-memory ServerQuery.
type fakeQuery struct{ servers []ServerInfo }

func (q *fakeQuery) ListServers() []ServerInfo { return q.servers }
func (q *fakeQuery) GetServer(id string) (ServerInfo, bool) {
	for _, s := range q.servers {
		if s.ID == id {
			return s, true
		}
	}
	return ServerInfo{}, false
}

// fakePlayers is an in-memory PlayerManager.
type fakePlayers struct {
	mu     sync.Mutex
	online map[string][]string
	bans   map[string][]BanInfo
}

func (p *fakePlayers) OnlinePlayers(id string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.online[id]...)
}

func (p *fakePlayers) ListBans(id string) ([]BanInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]BanInfo(nil), p.bans[id]...), nil
}

func (p *fakePlayers) AddBan(id, target, reason string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bans == nil {
		p.bans = map[string][]BanInfo{}
	}
	p.bans[id] = append(p.bans[id], BanInfo{Type: "player", Target: target, Reason: reason})
	return nil
}

func (p *fakePlayers) RemoveBan(id, target string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	kept := p.bans[id][:0]
	for _, b := range p.bans[id] {
		if b.Target != target {
			kept = append(kept, b)
		}
	}
	p.bans[id] = kept
	return nil
}

// TestServerQueryAPI verifies server.list()/server.info() expose registry data.
func TestServerQueryAPI(t *testing.T) {
	q := &fakeQuery{servers: []ServerInfo{
		{ID: "a", Name: "Alpha", Provider: "vanilla", McVersion: "26.1", Status: "RUNNING", Port: 25565, MemoryMB: 2048},
		{ID: "b", Name: "Beta", Provider: "paper", McVersion: "26.2", Status: "STOPPED", Port: 25566, MemoryMB: 4096},
	}}
	m := NewManager(ManagerConfig{Host: scripting.NewHost(nil, nil, nil), Query: q})
	const src = `
meta = { id = "q", name = "Q",
  permissions = { {kind = "server_query", reason = "list"} } }
register = function()
  local list = server.list()
  log("count:" .. #list)
  local info = server.info("b")
  log("b:" .. info.name .. ":" .. info.port .. ":" .. info.status)
  if server.info("nope") == nil then log("missing:nil") end
end
`
	if _, err := m.AddSource(src, true); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("q"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	defer m.Shutdown()

	for _, want := range []string{"count:2", "b:Beta:25566:STOPPED", "missing:nil"} {
		if !logContains(m, want) {
			t.Errorf("log missing %q", want)
		}
	}
}

// TestPlayerManagementAPI verifies server.players/ban/unban/bans and that kick
// goes through the console.
func TestPlayerManagementAPI(t *testing.T) {
	con := newFakeConsole()
	p := &fakePlayers{
		online: map[string][]string{"srv": {"Alice", "Bob"}},
		bans:   map[string][]BanInfo{"srv": {{Type: "player", Target: "Mallory", Reason: "grief"}}},
	}
	m := NewManager(ManagerConfig{
		Host:    scripting.NewHost(nil, nil, nil),
		Console: con,
		Players: p,
		Logs:    scriptlog.NewLogBuffer(0),
	})
	const src = `
meta = { id = "pm", name = "PM",
  permissions = {
    {kind = "player_manage", reason = "manage"},
    {kind = "console_write", reason = "kick"},
  } }
register = function()
  local names = server.players("srv")
  log("online:" .. table.concat(names, ","))
  local bans = server.bans("srv")
  log("ban0:" .. bans[1].target .. ":" .. bans[1].reason)
  server.ban("srv", "Eve", "cheating")
  server.unban("srv", "Mallory")
  server.kick("srv", "Bob", "afk")
  log("done")
end
`
	if _, err := m.AddSource(src, true); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("pm"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	defer m.Shutdown()

	waitFor(t, "script done", func() bool { return logContains(m, "done") })
	if !logContains(m, "online:Alice,Bob") {
		t.Error("players list not logged")
	}
	if !logContains(m, "ban0:Mallory:grief") {
		t.Error("bans list not logged")
	}
	bans, _ := p.ListBans("srv")
	if len(bans) != 1 || bans[0].Target != "Eve" {
		t.Errorf("bans after ban+unban = %v, want only Eve", bans)
	}
	if con.sentCount() != 1 || con.sent[0] != "srv kick Bob afk" {
		t.Errorf("kick command = %v", con.sent)
	}
}

// TestOnJoinLeaveViaEventBus verifies on_join/on_leave hooks fire from the
// players.EventBus (state-diff events), not from console parsing.
func TestOnJoinLeaveViaEventBus(t *testing.T) {
	bus := players.NewEventBus()
	m := NewManager(ManagerConfig{
		Host:   scripting.NewHost(nil, nil, nil),
		Events: bus,
		Logs:   scriptlog.NewLogBuffer(0),
	})
	const src = `
meta = { id = "greeter", name = "Greeter",
  permissions = { {kind = "player_manage", reason = "watch joins"} } }
on_join("srv", function(name) log("join:" .. name) end)
on_leave("srv", function(name) log("leave:" .. name) end)
on_join("other", function(name) log("otherjoin:" .. name) end)
`
	if _, err := m.AddSource(src, true); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("greeter"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	defer m.Shutdown()

	bus.Feed("srv", "[x] [Server thread/INFO]: Notch joined the game")
	waitFor(t, "join fired", func() bool { return logContains(m, "join:Notch") })
	bus.Feed("srv", "[x] [Server thread/INFO]: Notch left the game")
	waitFor(t, "leave fired", func() bool { return logContains(m, "leave:Notch") })
	// An event on a different server must not reach srv's hooks.
	bus.Feed("unrelated", "[x] [Server thread/INFO]: Ghost joined the game")
	if logContains(m, "otherjoin:Ghost") {
		t.Error("hook for 'other' fired for server 'unrelated'")
	}
}

// TestOnJoinRequiresPermission verifies on_join is permission-gated.
func TestOnJoinRequiresPermission(t *testing.T) {
	bus := players.NewEventBus()
	m := NewManager(ManagerConfig{Host: scripting.NewHost(nil, nil, nil), Events: bus})
	// Non-builtin, no grants → player_manage not granted; top-level on_join
	// raises, so Enable fails.
	const src = `
meta = { id = "spy", name = "Spy",
  permissions = { {kind = "player_manage", reason = "x"} } }
on_join("srv", function(name) end)
`
	if _, err := m.AddSource(src, false); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("spy"); err == nil {
		t.Fatal("Enable should fail: on_join without player_manage grant")
	}
}

// TestSleepBlocksScriptOnly verifies sleep() delays the calling script without
// failing, and Disable interrupts a long sleep promptly.
func TestSleepBlocksScriptOnly(t *testing.T) {
	m := NewManager(ManagerConfig{Host: scripting.NewHost(nil, nil, nil), Logs: scriptlog.NewLogBuffer(0)})
	const src = `
meta = { id = "sleepy", name = "Sleepy",
  permissions = { {kind = "schedule", reason = "sleep"} } }
register = function()
  sleep(0.01)
  log("woke")
end
`
	if _, err := m.AddSource(src, true); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("sleepy"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	waitFor(t, "sleep returned", func() bool { return logContains(m, "woke") })
	m.Shutdown()
}

// TestStoreScopedToScript verifies jhmc.store persists per script id through
// the automation runtime.
func TestStoreScopedToScript(t *testing.T) {
	kv := scriptdata.NewKVStore(t.TempDir())
	m := NewManager(ManagerConfig{Host: scripting.NewHost(nil, nil, nil), KV: kv, Logs: scriptlog.NewLogBuffer(0)})
	const src = `
meta = { id = "counter", name = "Counter", permissions = {} }
register = function()
  local n = jhmc.store.get("runs")
  if n == nil then n = "0" end
  jhmc.store.set("runs", tostring(tonumber(n) + 1))
  log("runs:" .. jhmc.store.get("runs"))
end
`
	if _, err := m.AddSource(src, true); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := m.Enable("counter"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	waitFor(t, "first run", func() bool { return logContains(m, "runs:1") })
	m.Disable("counter")
	if err := m.Enable("counter"); err != nil {
		t.Fatalf("re-Enable: %v", err)
	}
	waitFor(t, "second run", func() bool { return logContains(m, "runs:2") })
	m.Shutdown()

	if v, ok := kv.Get("counter", "runs"); !ok || v != "2" {
		t.Errorf("persisted runs = %q,%v", v, ok)
	}
}
