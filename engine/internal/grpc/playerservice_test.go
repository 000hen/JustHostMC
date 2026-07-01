package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/console"
	"github.com/000hen/justhostmc/engine/internal/store"
	"github.com/Tnze/go-mc/nbt"
)

const testPlayerUUID = "12345678-1234-1234-1234-123456789abc"

func TestLocatePlayerDataUsesConfiguredLevelName(t *testing.T) {
	serverDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(serverDir, "server.properties"), []byte("level-name=survival\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := createPlayerDataFile(t, serverDir, "survival")

	got, found, err := locatePlayerData(serverDir, testPlayerUUID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got != want {
		t.Fatalf("locatePlayerData() = %q, %v; want %q, true", got, found, want)
	}
}

func TestLocatePlayerDataDiscoversWorldFolder(t *testing.T) {
	serverDir := t.TempDir()
	want := createPlayerDataFile(t, serverDir, "legacy-world")

	got, found, err := locatePlayerData(serverDir, testPlayerUUID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got != want {
		t.Fatalf("locatePlayerData() = %q, %v; want %q, true", got, found, want)
	}
}

func TestLocatePlayerDataUsesModernPlayersDataLayout(t *testing.T) {
	serverDir := t.TempDir()
	want := createPlayerDataFileAt(t, serverDir, "world", "players", "data")

	got, found, err := locatePlayerData(serverDir, testPlayerUUID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got != want {
		t.Fatalf("locatePlayerData() = %q, %v; want %q, true", got, found, want)
	}
}

func TestLocatePlayerDataRejectsEscapingLevelName(t *testing.T) {
	serverDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(serverDir, "server.properties"), []byte("level-name=../outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, found, err := locatePlayerData(serverDir, testPlayerUUID)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("locatePlayerData() unexpectedly found escaping path %q", got)
	}
}

func TestGetDataFlushesUnsavedOnlinePlayer(t *testing.T) {
	paths := appdata.New(t.TempDir())
	st := store.NewMemory()
	if err := st.Put(&store.Server{ID: "s1", Status: mcmanagerv1.ServerStatus_RUNNING}); err != nil {
		t.Fatal(err)
	}

	playerPath := filepath.Join(paths.ServerDir("s1"), "world", "players", "data", testPlayerUUID+".dat")
	instance := newPlayerDataTestInstance(func(command string) error {
		if command != "save-all flush" {
			t.Fatalf("command = %q, want save-all flush", command)
		}
		return writeNBTFile(playerPath, "", playerNBT{})
	})
	hub := console.NewHub()
	hub.Register("s1", instance)

	service := NewPlayerService(hub, st, paths)
	data, err := service.GetData(context.Background(), &mcmanagerv1.PlayerLookup{
		ServerId: "s1",
		Name:     "Alice",
		Uuid:     testPlayerUUID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.Uuid != testPlayerUUID {
		t.Fatalf("UUID = %q, want %q", data.Uuid, testPlayerUUID)
	}
}

func TestConvertInventorySupportsLegacyAndModernCounts(t *testing.T) {
	items := []nbt.RawMessage{
		rawNBT(t, `{Slot:0B,id:"minecraft:stone",Count:2B}`),
		rawNBT(t, `{Slot:1B,id:"minecraft:wooden_axe",count:1}`),
	}

	got := convertInventory(items, false)
	if len(got) != 2 {
		t.Fatalf("converted item count = %d, want 2", len(got))
	}
	if got[0].Count != 2 || got[1].Count != 1 {
		t.Fatalf("converted counts = %d, %d; want 2, 1", got[0].Count, got[1].Count)
	}
}

func createPlayerDataFile(t *testing.T, serverDir, world string) string {
	return createPlayerDataFileAt(t, serverDir, world, "playerdata")
}

func createPlayerDataFileAt(t *testing.T, serverDir, world string, dataPath ...string) string {
	t.Helper()
	parts := append([]string{serverDir, world}, dataPath...)
	parts = append(parts, testPlayerUUID+".dat")
	path := filepath.Join(parts...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func rawNBT(t *testing.T, snbt string) nbt.RawMessage {
	t.Helper()
	encoded, err := nbt.Marshal(nbt.StringifiedMessage(snbt))
	if err != nil {
		t.Fatal(err)
	}
	var raw nbt.RawMessage
	if err := nbt.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

type playerDataTestInstance struct {
	output  chan string
	done    chan struct{}
	onWrite func(string) error
}

func newPlayerDataTestInstance(onWrite func(string) error) *playerDataTestInstance {
	return &playerDataTestInstance{
		output:  make(chan string),
		done:    make(chan struct{}),
		onWrite: onWrite,
	}
}

func (i *playerDataTestInstance) ID() string                   { return "s1" }
func (i *playerDataTestInstance) WriteStdin(line string) error { return i.onWrite(line) }
func (i *playerDataTestInstance) Output() <-chan string        { return i.output }
func (i *playerDataTestInstance) Done() <-chan struct{}        { return i.done }
func (i *playerDataTestInstance) Running() bool                { return true }
func (i *playerDataTestInstance) ExitErr() error               { return nil }
