namespace JustHostMC.App.Services;

/// <summary>
/// Tracks whether the one-time "servers run directly on your machine" notice
/// has been shown (PROMPT §6, §10.7). Backed by a marker file under local app
/// data.
/// </summary>
public sealed class FirstRunService {
    private readonly string _markerPath;

    public FirstRunService() {
        var baseDir =
            Environment.GetEnvironmentVariable("MCMANAGER_DATA_DIR") ??
            Path.Combine(Environment.GetFolderPath(
                             Environment.SpecialFolder.LocalApplicationData),
                         "JustHostMC");
        _markerPath = Path.Combine(baseDir, ".onmachine-notice-acknowledged");
    }

    public bool ShouldShowOnMachineNotice() => !File.Exists(_markerPath);

    public void MarkOnMachineNoticeShown() {
        try {
            Directory.CreateDirectory(Path.GetDirectoryName(_markerPath)!);
            File.WriteAllText(_markerPath, DateTimeOffset.UtcNow.ToString("o"));
        } catch {
            // Non-fatal: worst case the notice shows again next launch.
        }
    }
}
