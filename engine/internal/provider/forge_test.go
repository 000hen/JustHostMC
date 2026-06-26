package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newForgePromoStub(t *testing.T) *Forge {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"promos": map[string]string{
				"26.2-latest":        "60.0.1",
				"26.1.2-latest":      "59.1.2",
				"1.21.1-latest":      "52.0.2",
				"1.21.1-recommended": "52.0.0",
				"1.20.1-latest":      "47.3.5",
				"1.20.1-recommended": "47.3.0",
			},
		})
	}))
	t.Cleanup(srv.Close)
	return NewForge(nil, WithForgePromotionsURL(srv.URL))
}

func TestForgeVersionsDistinctNewestFirst(t *testing.T) {
	f := newForgePromoStub(t)
	versions, err := f.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"26.2", "26.1.2", "1.21.1", "1.20.1"}
	if len(versions) != len(want) {
		t.Fatalf("Versions = %v, want %v", versions, want)
	}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("Versions = %v, want %v", versions, want)
		}
	}
}

func TestForgeResolveBuildPrefersRecommended(t *testing.T) {
	f := newForgePromoStub(t)
	build, err := f.resolveBuild(context.Background(), "1.21.1")
	if err != nil {
		t.Fatal(err)
	}
	if build != "52.0.0" {
		t.Errorf("build = %q, want recommended 52.0.0", build)
	}
	build, err = f.resolveBuild(context.Background(), "26.2")
	if err != nil {
		t.Fatal(err)
	}
	if build != "60.0.1" {
		t.Errorf("build = %q, want latest 60.0.1", build)
	}

	if _, err := f.resolveBuild(context.Background(), "9.9"); !errors.Is(err, ErrVersionNotFound) {
		t.Errorf("err = %v, want ErrVersionNotFound", err)
	}
}
