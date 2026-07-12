package modpack

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/000hen/justhostmc/engine/internal/provider"
)

// TestExportLiveServer is an opt-in diagnostic: it exports a real installed
// modpack server against the real FTB API. Runs only when
// JHMC_LIVE_EXPORT_DIR (server directory) and JHMC_LIVE_EXPORT_PACK
// ("packId/versionId") are set.
func TestExportLiveServer(t *testing.T) {
	dir := os.Getenv("JHMC_LIVE_EXPORT_DIR")
	pack := os.Getenv("JHMC_LIVE_EXPORT_PACK")
	if dir == "" || pack == "" {
		t.Skip("JHMC_LIVE_EXPORT_DIR / JHMC_LIVE_EXPORT_PACK not set")
	}
	dest := filepath.Join(t.TempDir(), "live-export.zip")
	err := Export(context.Background(), http.DefaultClient, Options{
		ServerDir:   dir,
		DestZip:     dest,
		PackVersion: pack,
		ServerName:  "Live Export",
	}, func(p provider.Progress) {
		if p.Step != "" {
			fmt.Printf("[step] %s %.2f\n", p.Step, p.Fraction)
		}
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	fi, _ := os.Stat(dest)
	fmt.Printf("zip size: %d\n", fi.Size())
}
