package automation

import (
	"context"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// runner holds the live state of one enabled automation: its dedicated LState
// (gopher-lua is not concurrent-safe, so every callback is serialized through
// jobs), the cancel func that tears down all its goroutines, and the registered
// hooks discovered when the script first ran.
type runner struct {
	cancel     context.CancelFunc
	jobs       chan func(*lua.LState)
	wg         sync.WaitGroup
	logHooks   map[string][]*lua.LFunction // server id -> on_log callbacks
	startHooks map[string][]*lua.LFunction
	stopHooks  map[string][]*lua.LFunction
	joinHooks  map[string][]*lua.LFunction // server id -> on_join callbacks
	leaveHooks map[string][]*lua.LFunction
}

// stop cancels the runner and waits for all its goroutines to drain.
func (r *runner) stop() {
	r.cancel()
	r.wg.Wait()
}

// fire enqueues a call to each callback with the given arg onto the job pump.
func (r *runner) fire(ctx context.Context, fns []*lua.LFunction, arg lua.LValue) {
	for _, fn := range fns {
		fn := fn
		select {
		case <-ctx.Done():
			return
		case r.jobs <- func(L *lua.LState) {
			if err := L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, arg); err != nil {
				_ = err // errors surfaced via print/log are captured; ignore the raw raise here
			}
		}:
		}
	}
}
