package provider

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const defaultNeoForgeMaven = "https://maven.neoforged.net/releases/net/neoforged/neoforge"

// NeoForge installs NeoForge via its installer's --installServer step. NeoForge
// versions encode the Minecraft version. Legacy "A.B.C" maps to MC "1.A.B"
// (B==0 -> "1.A", e.g. 21.0.x -> 1.21). The current "A.B.C.D" scheme maps to MC
// "A.B.C" (C==0 -> "A.B", e.g. 26.2.0.x -> 26.2), with D the build number.
type NeoForge struct {
	client    *http.Client
	mavenBase string
	java      JavaResolver
}

type NeoForgeOption func(*NeoForge)

func WithNeoForgeHTTPClient(c *http.Client) NeoForgeOption { return func(n *NeoForge) { n.client = c } }
func WithNeoForgeMavenBase(u string) NeoForgeOption        { return func(n *NeoForge) { n.mavenBase = u } }

func NewNeoForge(java JavaResolver, opts ...NeoForgeOption) *NeoForge {
	n := &NeoForge{client: http.DefaultClient, mavenBase: defaultNeoForgeMaven, java: java}
	for _, o := range opts {
		o(n)
	}
	return n
}

func (n *NeoForge) fetchVersions(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.mavenBase+"/maven-metadata.xml", nil)
	if err != nil {
		return nil, err
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("neoforge maven-metadata: unexpected status %s", resp.Status)
	}
	var meta struct {
		Versions []string `xml:"versioning>versions>version"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}
	return meta.Versions, nil
}

// Versions lists the Minecraft versions NeoForge supports, newest first.
func (n *NeoForge) Versions(ctx context.Context) ([]string, error) {
	versions, err := n.fetchVersions(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var out []string
	for _, v := range versions {
		mc := mcForNeoForge(v)
		if mc == "" {
			continue
		}
		if _, dup := seen[mc]; !dup {
			seen[mc] = struct{}{}
			out = append(out, mc)
		}
	}
	sortMCDesc(out)
	return out, nil
}

func (n *NeoForge) resolveVersion(ctx context.Context, mc string) (string, error) {
	versions, err := n.fetchVersions(ctx)
	if err != nil {
		return "", err
	}
	prefix, ok := neoForgePrefix(mc)
	if !ok {
		return "", fmt.Errorf("neoforge %q: %w", mc, ErrVersionNotFound)
	}
	var matches []string
	for _, v := range versions {
		if strings.HasPrefix(v, prefix) {
			matches = append(matches, v)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("neoforge %q: %w", mc, ErrVersionNotFound)
	}
	sort.Slice(matches, func(i, j int) bool {
		return neoPatch(matches[i]) > neoPatch(matches[j])
	})
	return matches[0], nil
}

func (n *NeoForge) Install(ctx context.Context, dir, version string, progress func(Progress)) (LaunchSpec, error) {
	report(progress, Progress{Step: "install.progress.resolving_version", Fraction: -1})
	nfVersion, err := n.resolveVersion(ctx, version)
	if err != nil {
		return LaunchSpec{}, err
	}
	major := JavaMajorForMC(version)

	javaPath, err := n.java(ctx, major, progress)
	if err != nil {
		return LaunchSpec{}, err
	}

	installerURL := fmt.Sprintf("%s/%s/neoforge-%s-installer.jar", n.mavenBase, nfVersion, nfVersion)
	return installViaInstaller(ctx, n.client, javaPath, installerURL, dir, major, progress)
}

// mcForNeoForge maps a NeoForge version to its Minecraft version. Legacy 3-part
// versions ("21.1.66") map to MC "1.A.B" (B==0 -> "1.A"); current 4-part
// versions ("26.1.2.76") map to MC "A.B.C" (C==0 -> "A.B", e.g. 26.2.0.7 -> 26.2).
func mcForNeoForge(v string) string {
	parts := strings.Split(v, ".")
	switch len(parts) {
	case 3: // legacy A.B.<build> -> MC 1.A.B
		a, err1 := strconv.Atoi(parts[0])
		b, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return ""
		}
		if b == 0 {
			return fmt.Sprintf("1.%d", a)
		}
		return fmt.Sprintf("1.%d.%d", a, b)
	case 4: // current A.B.C.<build> -> MC A.B.C
		a, err1 := strconv.Atoi(parts[0])
		b, err2 := strconv.Atoi(parts[1])
		c, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return ""
		}
		if c == 0 {
			return fmt.Sprintf("%d.%d", a, b)
		}
		return fmt.Sprintf("%d.%d.%d", a, b, c)
	default:
		return ""
	}
}

// neoForgePrefix maps an MC version to the NeoForge version prefix to match.
// Legacy MC 1.x -> "minor.patch." (1.21.1 -> "21.1."); the current MC scheme,
// which has no leading "1." -> "major.minor.patch." (26.2 -> "26.2.0.").
func neoForgePrefix(mc string) (string, bool) {
	major, minor, patch, ok := parseMC(mc)
	if !ok {
		return "", false
	}
	if major == 1 {
		return fmt.Sprintf("%d.%d.", minor, patch), true
	}
	return fmt.Sprintf("%d.%d.%d.", major, minor, patch), true
}

// neoPatch returns the trailing build number of a NeoForge version. It is the
// last dot-separated segment in both the legacy ("21.1.66") and current
// ("26.1.2.76") schemes, so picking the highest works across both.
func neoPatch(v string) int {
	parts := strings.Split(v, ".")
	p, _ := strconv.Atoi(leadingDigits(parts[len(parts)-1]))
	return p
}
