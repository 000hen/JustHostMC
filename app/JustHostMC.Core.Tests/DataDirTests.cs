using JustHostMC.Core;
using Xunit;

namespace JustHostMC.Core.Tests;

/// <summary>
/// M6 store-compliance: the app can redirect the engine's data directory so a
/// packaged build keeps everything under the package's local store (removed on
/// uninstall). Verified end-to-end by launching the real engine.
/// </summary>
public class DataDirTests {
    [Fact]
    public async Task StartAsync_HonorsConfiguredDataDir() {
        var dataDir = Path.Combine(
            Path.GetTempPath(), "jhmc-datadir-" + Guid.NewGuid().ToString("N"));
        try {
            await using var host = new EngineHost(new EngineHostOptions {
                EnginePath = EngineFixture.EnginePath(),
                DataDir    = dataDir,
            });

            await host.StartAsync();

            // The engine creates its SQLite registry under the data dir's base
            // before it reports a port, so by now the file must exist there.
            Assert.True(
                File.Exists(Path.Combine(dataDir, "registry.db")),
                $"engine should create registry.db under the configured data dir '{dataDir}'");
        } finally {
            try {
                Directory.Delete(dataDir, recursive: true);
            } catch { /* best effort */
            }
        }
    }
}
