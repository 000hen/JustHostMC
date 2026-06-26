package provider

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/000hen/justhostmc/engine/internal/dl"
)

const (
	defaultForgePromotions = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"
	defaultForgeMaven      = "https://maven.minecraftforge.net/net/minecraftforge/forge"
)

// Forge installs Minecraft Forge by running the official installer's
// --installServer step. Forge builds are resolved from the promotions feed.
type Forge struct {
	client        *http.Client
	promotionsURL string
	mavenBase     string
	java          JavaResolver
}

type ForgeOption func(*Forge)

func WithForgeHTTPClient(c *http.Client) ForgeOption  { return func(f *Forge) { f.client = c } }
func WithForgePromotionsURL(u string) ForgeOption     { return func(f *Forge) { f.promotionsURL = u } }
func WithForgeMavenBase(u string) ForgeOption         { return func(f *Forge) { f.mavenBase = u } }

// NewForge builds a Forge provider. java resolves the JRE needed to run the
// installer (and later the server).
func NewForge(java JavaResolver, opts ...ForgeOption) *Forge {
	f := &Forge{
		client:        http.DefaultClient,
		promotionsURL: defaultForgePromotions,
		mavenBase:     defaultForgeMaven,
		java:          java,
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

type forgePromotions struct {
	Promos map[string]string `json:"promos"`
}

// Versions lists Minecraft versions that have a Forge build, newest first.
func (f *Forge) Versions(ctx context.Context) ([]string, error) {
	promos, err := f.fetchPromos(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var out []string
	for key := range promos.Promos {
		if i := strings.LastIndex(key, "-"); i > 0 {
			mc := key[:i]
			if _, dup := seen[mc]; !dup {
				seen[mc] = struct{}{}
				out = append(out, mc)
			}
		}
	}
	sortMCDesc(out)
	return out, nil
}

func (f *Forge) fetchPromos(ctx context.Context) (*forgePromotions, error) {
	var p forgePromotions
	if err := getJSON(ctx, f.client, f.promotionsURL, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (f *Forge) resolveBuild(ctx context.Context, mc string) (string, error) {
	promos, err := f.fetchPromos(ctx)
	if err != nil {
		return "", err
	}
	if b, ok := promos.Promos[mc+"-recommended"]; ok {
		return b, nil
	}
	if b, ok := promos.Promos[mc+"-latest"]; ok {
		return b, nil
	}
	return "", fmt.Errorf("forge %q: %w", mc, ErrVersionNotFound)
}

func (f *Forge) Install(ctx context.Context, dir, version string, progress func(Progress)) (LaunchSpec, error) {
	report(progress, Progress{Step: "install.progress.resolving_version", Fraction: -1})
	build, err := f.resolveBuild(ctx, version)
	if err != nil {
		return LaunchSpec{}, err
	}
	major := JavaMajorForMC(version)

	// The installer itself needs a JRE; fetch it first (also used to run later).
	javaPath, err := f.java(ctx, major, progress)
	if err != nil {
		return LaunchSpec{}, err
	}

	full := version + "-" + build
	installerURL := fmt.Sprintf("%s/%s/forge-%s-installer.jar", f.mavenBase, full, full)
	return installViaInstaller(ctx, f.client, javaPath, installerURL, dir, major, progress)
}

// installViaInstaller is shared by Forge and NeoForge: download the installer,
// run --installServer (streaming output), then detect the launch command.
func installViaInstaller(ctx context.Context, client *http.Client, javaPath, installerURL, dir string, major int, progress func(Progress)) (LaunchSpec, error) {
	installerPath := filepath.Join(dir, "installer.jar")
	report(progress, Progress{Step: "install.progress.downloading_installer", Fraction: 0})
	_, _, err := dl.Download(ctx, client, installerURL, installerPath, nil, func(done, total int64) {
		frac := -1.0
		if total > 0 {
			frac = float64(done) / float64(total)
		}
		report(progress, Progress{Fraction: frac})
	})
	if err != nil {
		return LaunchSpec{}, err
	}

	report(progress, Progress{Step: "install.progress.running_installer", Fraction: -1})
	if err := runInstaller(ctx, javaPath, installerPath, dir, progress); err != nil {
		return LaunchSpec{}, err
	}

	args, err := detectServerLaunch(dir)
	if err != nil {
		return LaunchSpec{}, err
	}
	report(progress, Progress{Step: "install.progress.done", Fraction: 1})
	return LaunchSpec{JavaMajor: major, Args: args}, nil
}
