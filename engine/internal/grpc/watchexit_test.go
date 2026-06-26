package grpcsvc

import (
	"errors"
	"testing"
	"time"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/store"
)

func waitStatus(t *testing.T, st store.Store, id string, want mcmanagerv1.ServerStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if rec, ok := st.Get(id); ok && rec.Status == want {
			return
		}
		if time.Now().After(deadline) {
			rec, _ := st.Get(id)
			t.Fatalf("status = %v, want %v", rec.Status, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWatchExitMarksCrashedOnUnexpectedExit(t *testing.T) {
	st := store.NewMemory()
	_ = st.Put(runningRecord("s1"))
	svc := NewServerService(ServerServiceConfig{Store: st, Backend: &fakeBackend{}})

	inst := &fakeInstance{id: "s1", done: make(chan struct{}), exitErr: errors.New("killed")}
	svc.mu.Lock()
	svc.instances["s1"] = inst
	svc.mu.Unlock()

	go svc.watchExit("s1", inst)
	close(inst.done) // simulate the process dying on its own

	waitStatus(t, st, "s1", mcmanagerv1.ServerStatus_CRASHED)
}

func TestWatchExitMarksStoppedOnRequestedStop(t *testing.T) {
	st := store.NewMemory()
	_ = st.Put(runningRecord("s1"))
	svc := NewServerService(ServerServiceConfig{Store: st, Backend: &fakeBackend{}})

	inst := &fakeInstance{id: "s1", done: make(chan struct{}), exitErr: errors.New("terminated")}
	svc.mu.Lock()
	svc.instances["s1"] = inst
	svc.stopping["s1"] = true // user asked to stop
	svc.mu.Unlock()

	go svc.watchExit("s1", inst)
	close(inst.done)

	waitStatus(t, st, "s1", mcmanagerv1.ServerStatus_STOPPED)
}
