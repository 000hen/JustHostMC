package store

import (
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

func TestSQLiteCRUDAndOrdering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.db")
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer s.Close()

	_ = s.Put(&Server{ID: "b", Name: "Bravo", ProviderID: "paper", Status: mcmanagerv1.ServerStatus_STOPPED})
	_ = s.Put(&Server{ID: "a", Name: "Alpha", ProviderID: "vanilla", McVersion: "1.21",
		MemoryMB: 2048, Port: 25565, Status: mcmanagerv1.ServerStatus_RUNNING, JavaMajor: 21,
		LaunchArgs: []string{"-jar", "server.jar", "nogui"}})

	got, ok := s.Get("a")
	if !ok || got.Name != "Alpha" || got.JavaMajor != 21 || len(got.LaunchArgs) != 3 || got.LaunchArgs[1] != "server.jar" {
		t.Fatalf("Get(a) = %+v, ok=%v", got, ok)
	}

	list := s.List()
	if len(list) != 2 || list[0].Name != "Alpha" || list[1].Name != "Bravo" {
		t.Fatalf("List = %v, want [Alpha Bravo]", list)
	}

	// Update in place.
	got.Status = mcmanagerv1.ServerStatus_CRASHED
	_ = s.Put(got)
	if again, _ := s.Get("a"); again.Status != mcmanagerv1.ServerStatus_CRASHED {
		t.Errorf("status after update = %v, want CRASHED", again.Status)
	}

	_ = s.Delete("b")
	if _, ok := s.Get("b"); ok {
		t.Error("Get(b) ok after Delete")
	}
}

func TestSQLitePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.db")

	s1, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Put(&Server{ID: "x", Name: "Persisted", ProviderID: "forge", ModLayout: "mods",
		McVersion: "1.20.1", ProviderVersion: "95/12695", MemoryMB: 4096, Port: 25570,
		Status:    mcmanagerv1.ServerStatus_RUNNING,
		JavaMajor: 17, LaunchArgs: []string{"-jar", "forge.jar"}})
	s1.Close()

	s2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	got, ok := s2.Get("x")
	if !ok {
		t.Fatal("server not found after reopen")
	}
	if got.Name != "Persisted" || got.JavaMajor != 17 || got.MemoryMB != 4096 ||
		got.ProviderID != "forge" || len(got.LaunchArgs) != 2 ||
		got.ProviderVersion != "95/12695" {
		t.Errorf("reopened server = %+v", got)
	}
}
