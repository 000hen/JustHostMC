package settings

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "settings.json"))
	got, err := st.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("Load on missing file = %+v, want defaults %+v", got, Defaults())
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "sub", "settings.json"))
	want := Settings{KeepLogDays: 30, MaxLogTotalBytes: 1 << 30}
	if err := st.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestSaveOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st := NewStore(path)
	if err := st.Save(Settings{KeepLogDays: 7}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(Settings{KeepLogDays: 99, MaxLogTotalBytes: 5}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.KeepLogDays != 99 || got.MaxLogTotalBytes != 5 {
		t.Errorf("Load = %+v, want KeepLogDays=99 MaxLogTotalBytes=5", got)
	}
}
