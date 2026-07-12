package grpcsvc

import (
	"context"
	"os"
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/store"
)

func TestDeleteCancelsActiveInstallationBeforeRemovingData(t *testing.T) {
	paths := appdata.New(t.TempDir())
	dir := paths.ServerDir("installing")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	if err := st.Put(&store.Server{
		ID: "installing", Name: "Incomplete",
		Status: mcmanagerv1.ServerStatus_INSTALLING,
	}); err != nil {
		t.Fatal(err)
	}

	installCtx, cancel := context.WithCancel(context.Background())
	installation := &activeInstallation{cancel: cancel, done: make(chan struct{})}
	svc := NewServerService(ServerServiceConfig{Store: st, Paths: paths})
	svc.installations["installing"] = installation
	go func() {
		<-installCtx.Done()
		close(installation.done)
	}()

	if _, err := svc.Delete(context.Background(),
		&mcmanagerv1.ServerId{Id: "installing"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := st.Get("installing"); ok {
		t.Fatal("server record remains after Delete")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("server directory still exists: %v", err)
	}
}
