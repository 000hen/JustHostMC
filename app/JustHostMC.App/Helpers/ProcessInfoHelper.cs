using System.Diagnostics;
using Windows.ApplicationModel;

namespace JustHostMC.App.Helpers;

public static partial class ProcessInfoHelper {
    private static readonly FileVersionInfo? _fileVersionInfo;
    private static readonly Process _process;
    private static readonly string? _packageDisplayName;
    private static readonly Version? _packageVersion;
    public static string GitBranch { get; }
    public static string GitSha { get; }

    static ProcessInfoHelper() {
        _process         = Process.GetCurrentProcess();
        _fileVersionInfo = _process.MainModule?.FileVersionInfo;

        try {
            var package         = Package.Current;
            _packageDisplayName = package.DisplayName;
            var version         = package.Id.Version;
            _packageVersion     = new Version(version.Major, version.Minor,
                                              version.Build, version.Revision);
        } catch {
            // Package.Current is unavailable for unpackaged development builds.
        }

        string gitBranch = "";
        string gitSha    = "";
        var assembly     = System.Reflection.Assembly.GetEntryAssembly();
        if (assembly != null) {
            var attributes = assembly.GetCustomAttributes(
                typeof(System.Reflection.AssemblyMetadataAttribute), false);
            foreach (System.Reflection
                         .AssemblyMetadataAttribute attr in attributes) {
                if (attr.Key == "GitBranch")
                    gitBranch = attr.Value ?? "";
                if (attr.Key == "GitSha")
                    gitSha = attr.Value ?? "";
            }
        }
        GitBranch = gitBranch;
        GitSha    = gitSha;
    }

    /// <summary>
    /// Returns the full version string including the Git SHA and branch if
    /// available.
    /// </summary>
    public static string FullVersion =>
        _packageVersion is not null ? VersionWithPrefix
        : (!string.IsNullOrEmpty(GitBranch) && !string.IsNullOrEmpty(GitSha))
            ? $"v{Version}+{GitSha} ({GitBranch})"
            : VersionWithPrefix;

    /// <summary>
    /// Returns the version string.
    /// </summary>
    public static string Version =>
        GetVersion() is Version version
            ? string.Format("{0}.{1}.{2}.{3}", version.Major, version.Minor,
                            version.Build, version.Revision)
            : string.Empty;

    /// <summary>
    /// Returns the version string prefixed with 'v'.
    /// </summary>
    public static string VersionWithPrefix => $"v{Version}";

    /// <summary>
    /// Retrieves the product name. If not available, it returns 'Unknown
    /// Product'.
    /// </summary>
    public static string ProductName =>
        !string.IsNullOrWhiteSpace(_packageDisplayName)
            ? _packageDisplayName
            : _fileVersionInfo?.ProductName ?? "JustHostMC";

    /// <summary>
    /// Combines the product name and version into a single string. The version
    /// includes a prefix.
    /// </summary>
    public static string ProductNameAndVersion =>
        $"{ProductName} {VersionWithPrefix}";

    /// <summary>
    /// Returns the company name of the publisher. If not available, it defaults
    /// to 'Unknown Publisher'.
    /// </summary>
    public static string Publisher =>
        _fileVersionInfo?.CompanyName ?? "Unknown Publisher";

    public static Version? GetVersion() {
        return _packageVersion ??
               (_fileVersionInfo is null
                    ? null
                    : new Version(_fileVersionInfo.FileMajorPart,
                                  _fileVersionInfo.FileMinorPart,
                                  _fileVersionInfo.FileBuildPart,
                                  _fileVersionInfo.FilePrivatePart));
    }

    /// <summary>
    /// Retrieves the file version information for the current assembly.
    /// </summary>
    /// <returns>Returns a FileVersionInfo object containing version
    /// details.</returns>
    public static FileVersionInfo? GetFileVersionInfo() {
        return _fileVersionInfo;
    }

    /// <summary>
    /// Retrieves the current process instance.
    /// </summary>
    /// <returns>Returns the current Process object.</returns>
    public static Process GetProcess() {
        return _process;
    }
}
