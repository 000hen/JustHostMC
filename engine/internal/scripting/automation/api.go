package automation

import (
	"fmt"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
	lua "github.com/yuin/gopher-lua"
)

// autoAPI binds the permission-gated server.* / on_* / schedule API to one
// running automation.
type autoAPI struct {
	mgr    *Manager
	runner *runner
	inv    *scripting.Invocation
	id     string
}

// serverTable builds the `server` table: send/logs/start/stop/restart plus the
// server-query (list/info) and player-management (players/kick/ban/unban/bans)
// surfaces.
func (a *autoAPI) serverTable(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("send", L.NewFunction(a.send))
	t.RawSetString("logs", L.NewFunction(a.logs))
	t.RawSetString("start", L.NewFunction(a.start))
	t.RawSetString("stop", L.NewFunction(a.stop))
	t.RawSetString("restart", L.NewFunction(a.restart))
	t.RawSetString("list", L.NewFunction(a.serverList))
	t.RawSetString("info", L.NewFunction(a.serverInfo))
	t.RawSetString("players", L.NewFunction(a.players))
	t.RawSetString("kick", L.NewFunction(a.kick))
	t.RawSetString("ban", L.NewFunction(a.ban))
	t.RawSetString("unban", L.NewFunction(a.unban))
	t.RawSetString("bans", L.NewFunction(a.listBans))
	return t
}

// installGlobals registers on_log/on_start/on_stop/on_join/on_leave/schedule/
// sleep plus a print/log that captures output into the engine-wide automation
// log.
func (a *autoAPI) installGlobals(L *lua.LState) {
	L.SetGlobal("on_log", L.NewFunction(a.onLog))
	L.SetGlobal("on_start", L.NewFunction(a.onStart))
	L.SetGlobal("on_stop", L.NewFunction(a.onStop))
	L.SetGlobal("on_join", L.NewFunction(a.onJoin))
	L.SetGlobal("on_leave", L.NewFunction(a.onLeave))
	L.SetGlobal("schedule", L.NewFunction(a.schedule))
	L.SetGlobal("sleep", L.NewFunction(a.sleep))
	logFn := L.NewFunction(a.log)
	L.SetGlobal("log", logFn)
	L.SetGlobal("print", logFn)
}

func (a *autoAPI) send(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_WRITE)
	id := L.CheckString(1)
	cmd := L.CheckString(2)
	if a.mgr.cfg.Console == nil {
		a.inv.Fail(L, fmt.Errorf("server.send: no console available"))
		return 0
	}
	if err := a.mgr.cfg.Console.Send(id, cmd); err != nil {
		a.inv.Fail(L, err)
		return 0
	}
	return 0
}

func (a *autoAPI) logs(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_READ)
	id := L.CheckString(1)
	if a.mgr.cfg.Console == nil {
		a.inv.Fail(L, fmt.Errorf("server.logs: no console available"))
		return 0
	}
	history, _, cancel := a.mgr.cfg.Console.Subscribe(id)
	cancel()
	out := L.NewTable()
	lines := L.NewTable()
	for _, line := range history {
		lines.Append(lua.LString(line))
	}
	out.RawSetString("lines", lines)
	L.Push(out)
	return 1
}

func (a *autoAPI) start(L *lua.LState) int { return a.control(L, "start") }
func (a *autoAPI) stop(L *lua.LState) int  { return a.control(L, "stop") }

func (a *autoAPI) restart(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_SERVER_CONTROL)
	id := L.CheckString(1)
	if a.mgr.cfg.Control == nil {
		a.inv.Fail(L, fmt.Errorf("server.restart: no server control available"))
		return 0
	}
	if _, err := a.mgr.cfg.Control.Stop(a.inv.Ctx(), &mcmanagerv1.ServerId{Id: id}); err != nil {
		a.inv.Fail(L, err)
		return 0
	}
	if _, err := a.mgr.cfg.Control.Start(a.inv.Ctx(), &mcmanagerv1.ServerId{Id: id}); err != nil {
		a.inv.Fail(L, err)
		return 0
	}
	return 0
}

func (a *autoAPI) control(L *lua.LState, action string) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_SERVER_CONTROL)
	id := L.CheckString(1)
	if a.mgr.cfg.Control == nil {
		a.inv.Fail(L, fmt.Errorf("server.%s: no server control available", action))
		return 0
	}
	var err error
	if action == "start" {
		_, err = a.mgr.cfg.Control.Start(a.inv.Ctx(), &mcmanagerv1.ServerId{Id: id})
	} else {
		_, err = a.mgr.cfg.Control.Stop(a.inv.Ctx(), &mcmanagerv1.ServerId{Id: id})
	}
	if err != nil {
		a.inv.Fail(L, err)
	}
	return 0
}

// -- server queries -------------------------------------------------------------

// serverInfoTable maps a ServerInfo to the table shape scripts see.
func serverInfoTable(L *lua.LState, info ServerInfo) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("id", lua.LString(info.ID))
	t.RawSetString("name", lua.LString(info.Name))
	t.RawSetString("provider", lua.LString(info.Provider))
	t.RawSetString("mc_version", lua.LString(info.McVersion))
	t.RawSetString("status", lua.LString(info.Status))
	t.RawSetString("port", lua.LNumber(info.Port))
	t.RawSetString("memory_mb", lua.LNumber(info.MemoryMB))
	return t
}

func (a *autoAPI) serverList(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_SERVER_QUERY)
	if a.mgr.cfg.Query == nil {
		a.inv.Fail(L, fmt.Errorf("server.list: no server query available"))
		return 0
	}
	out := L.NewTable()
	for _, info := range a.mgr.cfg.Query.ListServers() {
		out.Append(serverInfoTable(L, info))
	}
	L.Push(out)
	return 1
}

func (a *autoAPI) serverInfo(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_SERVER_QUERY)
	id := L.CheckString(1)
	if a.mgr.cfg.Query == nil {
		a.inv.Fail(L, fmt.Errorf("server.info: no server query available"))
		return 0
	}
	info, ok := a.mgr.cfg.Query.GetServer(id)
	if !ok {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(serverInfoTable(L, info))
	return 1
}

// -- player management ------------------------------------------------------------

func (a *autoAPI) requirePlayers(L *lua.LState, what string) bool {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_PLAYER_MANAGE)
	if a.mgr.cfg.Players == nil {
		a.inv.Fail(L, fmt.Errorf("server.%s: no player manager available", what))
		return false
	}
	return true
}

func (a *autoAPI) players(L *lua.LState) int {
	if !a.requirePlayers(L, "players") {
		return 0
	}
	id := L.CheckString(1)
	out := L.NewTable()
	for _, name := range a.mgr.cfg.Players.OnlinePlayers(id) {
		out.Append(lua.LString(name))
	}
	L.Push(out)
	return 1
}

// kick sends the console kick command; it needs console_write on top of
// player_manage because it acts through the server's stdin.
func (a *autoAPI) kick(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_PLAYER_MANAGE)
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_WRITE)
	id := L.CheckString(1)
	name := L.CheckString(2)
	reason := L.OptString(3, "")
	if a.mgr.cfg.Console == nil {
		a.inv.Fail(L, fmt.Errorf("server.kick: no console available"))
		return 0
	}
	cmd := "kick " + name
	if reason != "" {
		cmd += " " + reason
	}
	if err := a.mgr.cfg.Console.Send(id, cmd); err != nil {
		a.inv.Fail(L, err)
	}
	return 0
}

func (a *autoAPI) ban(L *lua.LState) int {
	if !a.requirePlayers(L, "ban") {
		return 0
	}
	id := L.CheckString(1)
	target := L.CheckString(2)
	reason := L.OptString(3, "")
	if err := a.mgr.cfg.Players.AddBan(id, target, reason); err != nil {
		a.inv.Fail(L, err)
	}
	return 0
}

func (a *autoAPI) unban(L *lua.LState) int {
	if !a.requirePlayers(L, "unban") {
		return 0
	}
	id := L.CheckString(1)
	target := L.CheckString(2)
	if err := a.mgr.cfg.Players.RemoveBan(id, target); err != nil {
		a.inv.Fail(L, err)
	}
	return 0
}

func (a *autoAPI) listBans(L *lua.LState) int {
	if !a.requirePlayers(L, "bans") {
		return 0
	}
	id := L.CheckString(1)
	bans, err := a.mgr.cfg.Players.ListBans(id)
	if err != nil {
		a.inv.Fail(L, err)
		return 0
	}
	out := L.NewTable()
	for _, b := range bans {
		t := L.NewTable()
		t.RawSetString("type", lua.LString(b.Type))
		t.RawSetString("target", lua.LString(b.Target))
		t.RawSetString("reason", lua.LString(b.Reason))
		t.RawSetString("created", lua.LString(b.Created))
		out.Append(t)
	}
	L.Push(out)
	return 1
}

// -- hooks ------------------------------------------------------------------------

func (a *autoAPI) onLog(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_CONSOLE_READ)
	id := L.CheckString(1)
	fn := L.CheckFunction(2)
	a.runner.logHooks[id] = append(a.runner.logHooks[id], fn)
	return 0
}

func (a *autoAPI) onStart(L *lua.LState) int {
	id := L.CheckString(1)
	fn := L.CheckFunction(2)
	a.runner.startHooks[id] = append(a.runner.startHooks[id], fn)
	return 0
}

func (a *autoAPI) onStop(L *lua.LState) int {
	id := L.CheckString(1)
	fn := L.CheckFunction(2)
	a.runner.stopHooks[id] = append(a.runner.stopHooks[id], fn)
	return 0
}

func (a *autoAPI) onJoin(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_PLAYER_MANAGE)
	id := L.CheckString(1)
	fn := L.CheckFunction(2)
	a.runner.joinHooks[id] = append(a.runner.joinHooks[id], fn)
	return 0
}

func (a *autoAPI) onLeave(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_PLAYER_MANAGE)
	id := L.CheckString(1)
	fn := L.CheckFunction(2)
	a.runner.leaveHooks[id] = append(a.runner.leaveHooks[id], fn)
	return 0
}

// schedule runs fn every `seconds` seconds until the automation is disabled.
func (a *autoAPI) schedule(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_SCHEDULE)
	seconds := float64(L.CheckNumber(1))
	fn := L.CheckFunction(2)
	if seconds <= 0 {
		a.inv.Fail(L, fmt.Errorf("schedule: interval must be positive"))
		return 0
	}
	r := a.runner
	ctx := a.inv.Ctx()
	d := time.Duration(seconds * float64(time.Second))
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		t := time.NewTicker(d)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.fire(ctx, []*lua.LFunction{fn}, lua.LNil)
			}
		}
	}()
	return 0
}

// sleep blocks the calling script for the given seconds (like time.sleep in
// Python). The script's LState is single-threaded, so its other hooks queue on
// the job pump and run when sleep returns; disabling the script interrupts it.
func (a *autoAPI) sleep(L *lua.LState) int {
	a.inv.Require(L, mcmanagerv1.PermissionKind_PERMISSION_SCHEDULE)
	secs := float64(L.CheckNumber(1))
	if secs <= 0 {
		return 0
	}
	t := time.NewTimer(time.Duration(secs * float64(time.Second)))
	defer t.Stop()
	select {
	case <-t.C:
	case <-a.inv.Ctx().Done():
		a.inv.Fail(L, a.inv.Ctx().Err())
	}
	return 0
}

// log appends a print/log line from the script to the engine-wide automation log.
func (a *autoAPI) log(L *lua.LState) int {
	n := L.GetTop()
	parts := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		parts = append(parts, L.ToStringMeta(L.Get(i)).String())
	}
	a.mgr.logs.Append(a.id, joinSpace(parts))
	return 0
}

func joinSpace(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}
