// Package provider implements server-type downloader adapters. Each server type
// (vanilla, Paper, Forge, …) is a Provider that resolves versions, downloads the
// required files, runs any installer, and reports the resulting launch spec.
package provider

import "context"

// Progress is reported continuously during Install and maps onto the gRPC
// InstallProgress message. Step changes carry a localization key; Fraction is
// 0..1 for known-size work (negative = indeterminate); LogLine is raw installer
// or download output and is passed through verbatim (never localized).
type Progress struct {
	Step     string  // localization key "namespace.method.type", empty if unchanged
	Fraction float64 // 0..1, or < 0 for indeterminate
	LogLine  string  // raw stdout/stderr line, optional
}

// LaunchSpec is what a provider returns so the engine knows how to run a server.
type LaunchSpec struct {
	JavaMajor int      // required Java feature version: 8 / 17 / 21 …
	Args      []string // JVM/program args, e.g. ["-jar", "server.jar", "nogui"]
	// McVersion, when non-empty, is the concrete Minecraft version the install
	// resolved to; it overrides the request's version on the server record (a
	// modpack provider takes an opaque "packId/versionId" and resolves the real
	// MC version here). Loader is the effective mod loader ("fabric"/"forge"/…).
	McVersion string
	Loader    string
}

// JavaResolver resolves (downloading if needed) a java.exe for a Java major
// version, reporting progress. jre.Manager.EnsureJRE satisfies this; injecting it
// keeps the provider package decoupled from the jre package. The scripting host
// uses it to back jhmc.run_jar / jhmc.resolve_java.
type JavaResolver func(ctx context.Context, major int, progress func(Progress)) (string, error)

// Provider downloads and prepares one server type.
type Provider interface {
	// Versions lists the installable Minecraft versions for this type.
	Versions(ctx context.Context) ([]string, error)
	// Install resolves and downloads files into dir, runs any installer, and
	// returns the launch spec. progress may be nil.
	Install(ctx context.Context, dir, version string, progress func(Progress)) (LaunchSpec, error)
}

// report sends progress if a sink is set; it keeps call sites terse.
func report(progress func(Progress), p Progress) {
	if progress != nil {
		progress(p)
	}
}
