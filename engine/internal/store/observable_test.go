package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestObservablePublishesSuccessfulMutations(t *testing.T) {
	base := NewMemory()
	observable := NewObservable(base, 4)
	subscription := observable.Subscribe()
	defer subscription.Cancel()

	rec := &Server{ID: "one", Name: "First", SortOrder: 2, LaunchArgs: []string{"server.jar"}}
	if err := observable.Put(rec); err != nil {
		t.Fatal(err)
	}
	rec.Name = "mutated after put"
	rec.LaunchArgs[0] = "mutated.jar"

	upsert := <-subscription.Events
	if upsert.Kind != ChangeUpsert || upsert.Server.Name != "First" {
		t.Fatalf("upsert = %#v", upsert)
	}
	if got := upsert.Server.LaunchArgs[0]; got != "server.jar" {
		t.Fatalf("launch args were not cloned: %q", got)
	}

	if err := observable.Delete("one"); err != nil {
		t.Fatal(err)
	}
	deleted := <-subscription.Events
	if deleted.Kind != ChangeDeleted || deleted.ServerID != "one" {
		t.Fatalf("deleted = %#v", deleted)
	}
}

func TestObservableDoesNotPublishFailedMutation(t *testing.T) {
	wantErr := errors.New("persistence failed")
	observable := NewObservable(failingStore{err: wantErr}, 2)
	subscription := observable.Subscribe()
	defer subscription.Cancel()

	if err := observable.Put(&Server{ID: "one"}); !errors.Is(err, wantErr) {
		t.Fatalf("Put error = %v, want %v", err, wantErr)
	}
	if err := observable.Delete("one"); !errors.Is(err, wantErr) {
		t.Fatalf("Delete error = %v, want %v", err, wantErr)
	}
	select {
	case change := <-subscription.Events:
		t.Fatalf("unexpected change: %#v", change)
	default:
	}
}

func TestObservableDropsOnlySlowSubscriber(t *testing.T) {
	observable := NewObservable(NewMemory(), 1)
	slow := observable.Subscribe()
	fast := observable.Subscribe()
	defer slow.Cancel()
	defer fast.Cancel()

	if err := observable.Put(&Server{ID: "one"}); err != nil {
		t.Fatal(err)
	}
	if change := <-fast.Events; change.Server.ID != "one" {
		t.Fatalf("first fast change = %#v", change)
	}

	if err := observable.Put(&Server{ID: "two"}); err != nil {
		t.Fatal(err)
	}
	if change := <-fast.Events; change.Server.ID != "two" {
		t.Fatalf("second fast change = %#v", change)
	}
	if change := <-slow.Events; change.Server.ID != "one" {
		t.Fatalf("buffered slow change = %#v", change)
	}
	if _, ok := <-slow.Events; ok {
		t.Fatal("slow subscription remained open after overflow")
	}
}

func TestSubscriptionCancelIsIdempotent(t *testing.T) {
	observable := NewObservable(NewMemory(), 1)
	subscription := observable.Subscribe()
	subscription.Cancel()
	subscription.Cancel()
	if _, ok := <-subscription.Events; ok {
		t.Fatal("subscription remained open after cancellation")
	}
}

func TestObservableSerializesPersistenceAndPublication(t *testing.T) {
	base := newControlledStore()
	observable := NewObservable(base, 2)
	subscription := observable.Subscribe()
	defer subscription.Cancel()

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- observable.Put(&Server{ID: "same", Name: "first"})
	}()
	<-base.firstPersisted

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- observable.Put(&Server{ID: "same", Name: "second"})
	}()

	select {
	case change := <-subscription.Events:
		t.Fatalf("published %q before the first persistence returned", change.Server.Name)
	case <-time.After(50 * time.Millisecond):
	}

	close(base.releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	first := <-subscription.Events
	second := <-subscription.Events
	if first.Server.Name != "first" || second.Server.Name != "second" {
		t.Fatalf("event order = %q, %q", first.Server.Name, second.Server.Name)
	}
	if stored, _ := base.Get("same"); stored.Name != "second" {
		t.Fatalf("stored server = %q, want second", stored.Name)
	}
}

type failingStore struct {
	err error
}

func (s failingStore) Put(*Server) error          { return s.err }
func (s failingStore) Get(string) (*Server, bool) { return nil, false }
func (s failingStore) List() []*Server            { return nil }
func (s failingStore) Delete(string) error        { return s.err }

type controlledStore struct {
	mu             sync.Mutex
	server         *Server
	firstPersisted chan struct{}
	releaseFirst   chan struct{}
	firstOnce      sync.Once
}

func newControlledStore() *controlledStore {
	return &controlledStore{
		firstPersisted: make(chan struct{}),
		releaseFirst:   make(chan struct{}),
	}
}

func (s *controlledStore) Put(server *Server) error {
	s.mu.Lock()
	clone := *server
	s.server = &clone
	s.mu.Unlock()
	if server.Name == "first" {
		s.firstOnce.Do(func() { close(s.firstPersisted) })
		<-s.releaseFirst
	}
	return nil
}

func (s *controlledStore) Get(string) (*Server, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return nil, false
	}
	clone := *s.server
	return &clone, true
}

func (s *controlledStore) List() []*Server { return nil }
func (s *controlledStore) Delete(string) error {
	return context.Canceled
}
