package store

import (
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

func TestMemoryStoreCRUD(t *testing.T) {
	m := NewMemory()

	if _, ok := m.Get("nope"); ok {
		t.Fatal("Get on empty store returned ok")
	}

	_ = m.Put(&Server{ID: "b", Name: "Bravo", ProviderID: "vanilla", Status: mcmanagerv1.ServerStatus_STOPPED})
	_ = m.Put(&Server{ID: "a", Name: "Alpha", ProviderID: "vanilla", Status: mcmanagerv1.ServerStatus_STOPPED})

	got, ok := m.Get("a")
	if !ok || got.Name != "Alpha" {
		t.Fatalf("Get(a) = %v, ok=%v", got, ok)
	}

	// List is sorted by name.
	list := m.List()
	if len(list) != 2 || list[0].Name != "Alpha" || list[1].Name != "Bravo" {
		t.Fatalf("List = %v, want [Alpha Bravo]", list)
	}

	// Stored values are cloned (mutating the returned copy must not leak back).
	got.Name = "Mutated"
	again, _ := m.Get("a")
	if again.Name != "Alpha" {
		t.Errorf("store value mutated via returned pointer: %q", again.Name)
	}

	_ = m.Delete("a")
	if _, ok := m.Get("a"); ok {
		t.Error("Get(a) ok after Delete")
	}
}

func TestServerProtoProjection(t *testing.T) {
	s := &Server{ID: "x", Name: "N", ProviderID: "paper", McVersion: "1.21", MemoryMB: 2048, Port: 25565, Status: mcmanagerv1.ServerStatus_RUNNING}
	p := s.Proto()
	if p.Id != "x" || p.MemoryMb != 2048 || p.Port != 25565 || p.ProviderId != "paper" || p.Status != mcmanagerv1.ServerStatus_RUNNING {
		t.Errorf("Proto projection wrong: %+v", p)
	}
}
