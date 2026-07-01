package grpcsvc

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/000hen/justhostmc/engine/internal/appdata"
)

func TestEnsureMinecraftClientDownloadsAndCachesExactVersion(t *testing.T) {
	clientJar := []byte("matching-client-assets")
	checksum := fmt.Sprintf("%x", sha1.Sum(clientJar))
	downloads := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/manifest":
			fmt.Fprintf(writer, `{"versions":[{"id":"26.2","url":%q}]}`, "http://"+request.Host+"/details")
		case "/details":
			fmt.Fprintf(writer, `{"downloads":{"client":{"url":%q,"sha1":%q}}}`, "http://"+request.Host+"/client", checksum)
		case "/client":
			downloads++
			writer.Header().Set("Content-Type", "application/java-archive")
			_, _ = writer.Write(clientJar)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	previous := minecraftVersionManifestURL
	minecraftVersionManifestURL = server.URL + "/manifest"
	defer func() { minecraftVersionManifestURL = previous }()

	paths := appdata.New(t.TempDir())
	first, err := ensureMinecraftClient(context.Background(), paths, "26.2")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureMinecraftClient(context.Background(), paths, "26.2")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || downloads != 1 {
		t.Fatalf("paths = %q, %q; downloads = %d", first, second, downloads)
	}
	got, err := os.ReadFile(first)
	if err != nil || string(got) != string(clientJar) {
		t.Fatalf("cached client = %q, %v", got, err)
	}
}
