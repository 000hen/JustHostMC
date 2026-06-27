package grpcsvc

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/store"
	"github.com/Tnze/go-mc/nbt/dynbt"
)

func TestConfigServiceUpdatesServerPropertiesAndSyncsPort(t *testing.T) {
	paths := appdata.New(t.TempDir())
	dir := paths.ServerDir("s1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	newPort := resolvePort(0)
	oldPort := newPort + 1
	if err := os.WriteFile(filepath.Join(dir, "server.properties"),
		[]byte("# keep this\nmotd=Hello\nserver-port="+strconv.Itoa(oldPort)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	_ = st.Put(&store.Server{
		ID: "s1", Name: "One", ProviderID: "vanilla", McVersion: "1.21",
		MemoryMB: 2048, Port: oldPort, Status: mcmanagerv1.ServerStatus_STOPPED,
	})
	svc := NewConfigService(st, paths)

	if _, err := svc.UpdateServerProperties(context.Background(), &mcmanagerv1.UpdateServerPropertiesRequest{
		ServerId: "s1",
		Entries: []*mcmanagerv1.ConfigUpdate{
			{Key: "server-port", Value: strconv.Itoa(newPort)},
		},
	}); err != nil {
		t.Fatalf("UpdateServerProperties: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "server.properties"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "# keep this") ||
		!strings.Contains(text, "motd=Hello") ||
		!strings.Contains(text, "server-port="+strconv.Itoa(newPort)) ||
		!strings.Contains(text, "query.port="+strconv.Itoa(newPort)) {
		t.Fatalf("server.properties = %q", text)
	}
	rec, _ := st.Get("s1")
	if rec.Port != newPort {
		t.Fatalf("stored port = %d, want %d", rec.Port, newPort)
	}
}

func TestConfigServiceUpdatesGameRuleInLevelDat(t *testing.T) {
	paths := appdata.New(t.TempDir())
	levelPath := filepath.Join(paths.ServerDir("s1"), "world", "level.dat")
	root := dynbt.NewCompound()
	data := dynbt.NewCompound()
	rules := dynbt.NewCompound()
	rules.Set("keepInventory", dynbt.NewString("false"))
	data.Set("GameRules", rules)
	root.Set("Data", data)
	if err := writeNBTFile(levelPath, "", root); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	_ = st.Put(&store.Server{
		ID: "s1", Name: "One", ProviderID: "vanilla", McVersion: "1.21",
		MemoryMB: 2048, Port: 25565, Status: mcmanagerv1.ServerStatus_STOPPED,
	})
	svc := NewConfigService(st, paths)
	got, err := svc.UpdateGameRules(context.Background(), &mcmanagerv1.UpdateGameRulesRequest{
		ServerId: "s1",
		Entries: []*mcmanagerv1.ConfigUpdate{
			{Key: "keepInventory", Value: "true"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGameRules: %v", err)
	}
	for _, entry := range got.Entries {
		if entry.Key == "keepInventory" && entry.Value == "true" {
			return
		}
	}
	t.Fatalf("keepInventory was not updated in %+v", got.Entries)
}
