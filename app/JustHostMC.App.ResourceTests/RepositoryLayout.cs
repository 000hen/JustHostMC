namespace JustHostMC.App.ResourceTests;

internal static class RepositoryLayout {
    public static string Root { get; } = FindRoot(AppContext.BaseDirectory);

    public static string AppPath(params string[] segments) => Path.Combine([
        Root, "app", "JustHostMC.App", ..segments
    ]);

    public static string RootPath(params string[] segments) => Path.Combine([
        Root, ..segments
    ]);

    public static string ReadAppFile(params string[] segments) =>
        File.ReadAllText(AppPath(segments));

    private static string FindRoot(string startPath) {
        var directory = new DirectoryInfo(startPath);
        while (directory is not null) {
            if (File.Exists(Path.Combine(directory.FullName, "JustHostMC.sln")))
                return directory.FullName;
            directory = directory.Parent;
        }

        throw new DirectoryNotFoundException(
            $"Could not find the repository root above '{startPath}'.");
    }
}
