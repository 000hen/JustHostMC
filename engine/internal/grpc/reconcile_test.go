package grpcsvc

import (
	"context"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/isolation"
	"github.com/000hen/justhostmc/engine/internal/store"
)

// fakeBackend lets reconcile tests control which instances appear "alive".
type fakeBackend struct {
	alive     []isolation.Instance
	startSpec isolation.InstanceSpec
	startInst *fakeInstance
}

func (b *fakeBackend) Start(_ context.Context, spec isolation.InstanceSpec) (isolation.Instance, error) {
	b.startSpec = spec
	if b.startInst == nil {
		b.startInst = &fakeInstance{id: spec.ID, out: make(chan string, 16), done: make(chan struct{})}
	}
	return b.startInst, nil
}
func (b *fakeBackend) Stop(context.Context, string, bool) error { return nil }
func (b *fakeBackend) Attach(context.Context, string) (isolation.Instance, error) {
	return nil, nil
}
func (b *fakeBackend) List(context.Context) ([]isolation.Instance, error) { return b.alive, nil }

func runningRecord(id string) *store.Server {
	return &store.Server{ID: id, Name: id, Status: mcmanagerv1.ServerStatus_RUNNING}
}

func TestReconcileMarksNonSurvivorsStopped(t *testing.T) {
	st := store.NewMemory()
	_ = st.Put(runningRecord("s1"))

	svc := NewServerService(ServerServiceConfig{Store: st, Backend: &fakeBackend{}})
	svc.Reconcile(context.Background())

	got, _ := st.Get("s1")
	if got.Status != mcmanagerv1.ServerStatus_STOPPED {
		t.Fatalf("status = %v, want STOPPED (did not survive restart)", got.Status)
	}
}

func TestReconcileReadoptsSurvivors(t *testing.T) {
	st := store.NewMemory()
	_ = st.Put(runningRecord("s1"))

	survivor := &fakeInstance{id: "s1", out: make(chan string), done: make(chan struct{})}
	svc := NewServerService(ServerServiceConfig{Store: st, Backend: &fakeBackend{alive: []isolation.Instance{survivor}}})
	svc.Reconcile(context.Background())

	got, _ := st.Get("s1")
	if got.Status != mcmanagerv1.ServerStatus_RUNNING {
		t.Fatalf("status = %v, want RUNNING (re-adopted)", got.Status)
	}
	svc.mu.Lock()
	_, adopted := svc.instances["s1"]
	svc.mu.Unlock()
	if !adopted {
		t.Error("survivor was not re-adopted into the instance map")
	}
	close(survivor.done) // let the watchExit goroutine finish
}
