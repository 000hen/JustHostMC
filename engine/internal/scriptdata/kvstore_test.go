package scriptdata

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSetGetDeleteKeys(t *testing.T) {
	kv := NewKVStore(filepath.Join(t.TempDir(), "script-data"))

	if _, ok := kv.Get("a", "missing"); ok {
		t.Fatal("Get on empty store should miss")
	}
	if err := kv.Set("a", "k1", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := kv.Set("a", "k2", "v2"); err != nil {
		t.Fatal(err)
	}
	if v, ok := kv.Get("a", "k1"); !ok || v != "v1" {
		t.Fatalf("Get k1 = %q,%v", v, ok)
	}
	if got := kv.Keys("a"); !reflect.DeepEqual(got, []string{"k1", "k2"}) {
		t.Fatalf("Keys = %v", got)
	}
	if err := kv.Delete("a", "k1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := kv.Get("a", "k1"); ok {
		t.Fatal("k1 should be deleted")
	}
	if err := kv.Delete("a", "never-existed"); err != nil {
		t.Fatalf("deleting absent key: %v", err)
	}
}

func TestScriptIsolation(t *testing.T) {
	kv := NewKVStore(filepath.Join(t.TempDir(), "script-data"))
	if err := kv.Set("alpha", "k", "from-alpha"); err != nil {
		t.Fatal(err)
	}
	if _, ok := kv.Get("beta", "k"); ok {
		t.Fatal("beta must not see alpha's data")
	}
}

func TestPersistsAcrossInstances(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "script-data")
	if err := NewKVStore(dir).Set("a", "k", "v"); err != nil {
		t.Fatal(err)
	}
	if v, ok := NewKVStore(dir).Get("a", "k"); !ok || v != "v" {
		t.Fatalf("reload Get = %q,%v", v, ok)
	}
}

func TestScriptIDSanitized(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "script-data")
	kv := NewKVStore(dir)
	if err := kv.Set("../../evil", "k", "v"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file inside store dir, got %d", len(entries))
	}
	if _, err := os.Stat(filepath.Join(base, "evil.json")); err == nil {
		t.Fatal("store file escaped the store dir")
	}
}
