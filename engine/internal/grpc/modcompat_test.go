package grpcsvc

import (
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

func TestLoaderMatchesServer(t *testing.T) {
	tests := []struct {
		loader, provider string
		kind             mcmanagerv1.ModKind
		want             bool
	}{
		{"fabric", "fabric", mcmanagerv1.ModKind_MOD, true},
		{"fabric", "forge", mcmanagerv1.ModKind_MOD, false},
		{"forge-legacy", "forge", mcmanagerv1.ModKind_MOD, true},
		{"bukkit", "paper", mcmanagerv1.ModKind_PLUGIN, true},
		{"bukkit", "spigot", mcmanagerv1.ModKind_PLUGIN, true},
		{"paper", "paper", mcmanagerv1.ModKind_PLUGIN, true},
		{"paper", "spigot", mcmanagerv1.ModKind_PLUGIN, false},
	}
	for _, tt := range tests {
		if got := loaderMatchesServer(tt.loader, tt.provider, tt.kind); got != tt.want {
			t.Errorf("loaderMatchesServer(%q, %q) = %v, want %v", tt.loader, tt.provider, got, tt.want)
		}
	}
}

func TestMinecraftVersionMatches(t *testing.T) {
	tests := []struct {
		version, requirement string
		match, known         bool
	}{
		{"1.20.1", "1.20.1", true, true},
		{"1.20.2", "1.20.1", false, true},
		{"1.20.4", ">=1.20 <1.21", true, true},
		{"1.21", ">=1.20 <1.21", false, true},
		{"1.20.1", "[1.20,1.21)", true, true},
		{"1.21", "[1.20,1.21)", false, true},
		{"1.20.1", "[1.20.1]", true, true},
		{"1.20.4", "1.19.x || 1.20.x", true, true},
		{"1.12.2", "1.12.r2", true, true},
		{"1.20.1", "not-a-version", false, false},
		{"1.20.1", "1.19 || future-syntax", false, false},
		{"1.20.1", "", false, false},
	}
	for _, tt := range tests {
		match, known := minecraftVersionMatches(tt.version, tt.requirement)
		if match != tt.match || known != tt.known {
			t.Errorf("minecraftVersionMatches(%q, %q) = (%v, %v), want (%v, %v)",
				tt.version, tt.requirement, match, known, tt.match, tt.known)
		}
	}
}

func TestModCompatibilityDoesNotMutateCachedMetadata(t *testing.T) {
	meta := &mcmanagerv1.ModMetadata{
		Parsed: true, Loader: "fabric", GameVersionRequirement: "1.20.1",
	}
	first := modCompatibility(meta, "forge", "1.21", mcmanagerv1.ModKind_MOD)
	if !first.LoaderMismatch || !first.GameVersionMismatch {
		t.Fatalf("first compatibility = %+v", first)
	}
	second := modCompatibility(meta, "fabric", "1.20.1", mcmanagerv1.ModKind_MOD)
	if second.LoaderMismatch || second.GameVersionMismatch {
		t.Fatalf("second compatibility = %+v", second)
	}
	if meta.LoaderMismatch || meta.GameVersionMismatch {
		t.Fatalf("cached metadata was mutated: %+v", meta)
	}
}
