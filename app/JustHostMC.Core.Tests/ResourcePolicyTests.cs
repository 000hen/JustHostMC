using System.Text.RegularExpressions;
using System.Xml.Linq;
using Xunit;

namespace JustHostMC.Core.Tests;

public sealed class ResourcePolicyTests {
    private static readonly string Root = FindRepositoryRoot();
    private static readonly string AppRoot =
        Path.Combine(Root, "app", "JustHostMC.App");

    [Fact]
    public void LocalesExposeTheSameResourceNames() {
        var english = LoadResources("en-US").Select(ResourceName).ToHashSet();
        var chinese = LoadResources("zh-TW").Select(ResourceName).ToHashSet();
        Assert.Empty(english.Except(chinese));
        Assert.Empty(chinese.Except(english));
    }

    [Theory]
    [InlineData("en-US")]
    [InlineData("zh-TW")]
    public void ResourceNamesAreUnique(string language) {
        var duplicates = LoadResources(language)
            .GroupBy(ResourceName, StringComparer.OrdinalIgnoreCase)
            .Where(group => group.Count() > 1)
            .Select(group => group.Key);
        Assert.Empty(duplicates);
    }

    [Fact]
    public void LocalePlaceholdersMatch() {
        var english = LoadResourceMap("en-US");
        var chinese = LoadResourceMap("zh-TW");
        foreach (var key in english.Keys) {
            Assert.Equal(Placeholders(english[key]), Placeholders(chinese[key]));
        }
    }

    [Fact]
    public void DynamicLookupUsesMrtSlashPaths() {
        var source = File.ReadAllText(Path.Combine(
            AppRoot, "Services", "LocalizationService.cs"));
        Assert.Contains("key.Replace('.', '/')", source, StringComparison.Ordinal);
        Assert.DoesNotContain("key.Replace('.', '_')", source, StringComparison.Ordinal);
    }

    [Fact]
    public void StaticControlsDoNotConstructLocalizationService() {
        string[] files = [
            "Controls/Server/ServerConfigPanel.xaml.cs",
            "Controls/Server/ServerModsPanel.xaml.cs",
            "Controls/Server/ServerPerformancePanel.xaml.cs",
        ];
        foreach (var relativePath in files) {
            var source = File.ReadAllText(Path.Combine(
                AppRoot, relativePath.Replace('/', Path.DirectorySeparatorChar)));
            Assert.DoesNotContain("LocalizationService", source,
                                  StringComparison.Ordinal);
            Assert.DoesNotContain("_localizer.Get", source,
                                  StringComparison.Ordinal);
        }
    }

    [Theory]
    [InlineData("Controls/Server/ServerConfigPanel.xaml", "ServerSectionConfig")]
    [InlineData("Controls/Server/ServerModsPanel.xaml", "ServerSectionMods")]
    [InlineData("Controls/Server/ServerPerformancePanel.xaml", "ServerSectionPerformance")]
    public void ServerPanelLayoutUidsDoNotReuseNavigationItemUids(
        string relativePath, string navigationUid) {
        var source = File.ReadAllText(Path.Combine(
            AppRoot, relativePath.Replace('/', Path.DirectorySeparatorChar)));
        Assert.DoesNotContain($"x:Uid=\"{navigationUid}\"", source,
                              StringComparison.Ordinal);
    }

    [Fact]
    public void ProgrammaticKeysDoNotUseUnderscoreSeparators() {
        var resourceNames = LoadResources("en-US")
            .Select(ResourceName)
            .ToHashSet(StringComparer.Ordinal);
        var offenders = Directory.EnumerateFiles(AppRoot, "*.cs", SearchOption.AllDirectories)
            .SelectMany(path => ResourceKeyLiterals(File.ReadAllText(path), resourceNames)
                .Select(item => $"{Path.GetRelativePath(Root, path)}:{item.Line} ({item.Key})"));
        Assert.Empty(offenders);
    }

    [Fact]
    public void ProgrammaticKeyPolicyCoversMappedAndPrefixedLiterals() {
        const string source = """
            localizer.Get(state switch {
                State.Running => "ServerStatus_Running",
                _ => ResourceKey("ServerStatus_", state),
            });
            """;
        var resources = new HashSet<string>(StringComparer.Ordinal) {
            "ServerStatus.Running",
            "ServerStatus.Stopped",
        };

        Assert.Equal(
            ["ServerStatus_Running", "ServerStatus_"],
            ResourceKeyLiterals(source, resources).Select(item => item.Key));
    }

    [Theory]
    [InlineData("PermissionConsentDialog_Title")]
    [InlineData("Scripts_RemoveConfirmTitle")]
    [InlineData("Scripts_RemoveConfirmCancel")]
    public void ObsoleteResourceAliasesAreAbsent(string alias) {
        Assert.DoesNotContain(alias, LoadResourceMap("en-US").Keys);
        Assert.DoesNotContain(alias, LoadResourceMap("zh-TW").Keys);
    }

    private static IReadOnlyList<XElement> LoadResources(string language) =>
        XDocument.Load(Path.Combine(AppRoot, "Strings", language, "Resources.resw"))
            .Root!.Elements("data").ToArray();

    private static Dictionary<string, string> LoadResourceMap(string language) =>
        LoadResources(language).ToDictionary(
            ResourceName,
            element => element.Element("value")?.Value ?? string.Empty,
            StringComparer.OrdinalIgnoreCase);

    private static string ResourceName(XElement element) =>
        element.Attribute("name")!.Value;

    private static IEnumerable<(string Key, int Line)> ResourceKeyLiterals(
        string source,
        IReadOnlySet<string> resourceNames) =>
        Regex.Matches(source, @"""(?:\\.|[^""\\])*""")
            .Select(match => (
                Key: match.Value[1..^1],
                Line: source.AsSpan(0, match.Index).Count('\n') + 1))
            .Where(item => IsUnderscoreSeparatedResource(item.Key, resourceNames));

    private static bool IsUnderscoreSeparatedResource(
        string candidate,
        IReadOnlySet<string> resourceNames) {
        if (!candidate.Contains('_', StringComparison.Ordinal))
            return false;
        if (resourceNames.Contains(candidate))
            return !candidate.Contains('.', StringComparison.Ordinal) ||
                Regex.IsMatch(candidate, @"_[A-Z]");
        return resourceNames.Any(resource =>
            candidate.Length <= resource.Length &&
            candidate.Select((character, index) => (character, index)).All(item =>
                item.character == resource[item.index] ||
                item.character == '_' && resource[item.index] == '.') &&
            candidate.Select((character, index) =>
                character == '_' && resource[index] == '.').Any(matches => matches));
    }

    private static string[] Placeholders(string value) =>
        Regex.Matches(value, @"\{[A-Za-z][A-Za-z0-9_]*\}")
            .Select(match => match.Value)
            .Distinct(StringComparer.Ordinal)
            .Order(StringComparer.Ordinal)
            .ToArray();

    private static string FindRepositoryRoot() {
        for (var directory = new DirectoryInfo(AppContext.BaseDirectory);
             directory is not null;
             directory = directory.Parent) {
            if (File.Exists(Path.Combine(directory.FullName, "JustHostMC.sln")))
                return directory.FullName;
        }
        throw new DirectoryNotFoundException("Could not locate JustHostMC.sln");
    }
}
