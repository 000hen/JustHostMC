using System.Text.RegularExpressions;
using System.Xml.Linq;
using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed partial class ResourceIntegrityTests {
    private static readonly string[] Locales = ["en-US", "zh-TW"];

    [Fact]
    public void LocaleFilesHaveUniqueNonEmptyEntries() {
        foreach (var locale in Locales) {
            var entries = Entries(locale);
            Assert.Empty(ResourceCatalog.DuplicateNames(entries));
            Assert.DoesNotContain(entries, entry =>
                string.IsNullOrWhiteSpace(entry.Name) ||
                string.IsNullOrWhiteSpace(entry.Value));
        }
    }

    [Fact]
    public void LocaleFilesHaveIdenticalIdentifiers() {
        var english = Entries("en-US").Select(entry => entry.Name).ToHashSet(
            StringComparer.OrdinalIgnoreCase);
        var chinese = Entries("zh-TW").Select(entry => entry.Name).ToHashSet(
            StringComparer.OrdinalIgnoreCase);

        Assert.True(english.SetEquals(chinese),
            $"en-US only: {string.Join(", ", english.Except(chinese))}; " +
            $"zh-TW only: {string.Join(", ", chinese.Except(english))}");
    }

    [Fact]
    public void DuplicateVisibleValuesHaveTranslatorContext() {
        foreach (var locale in Locales) {
            var undocumented = Entries(locale)
                .GroupBy(entry => entry.Value, StringComparer.Ordinal)
                .Where(group => group.Count() > 1)
                .SelectMany(group => group)
                .Where(entry => string.IsNullOrWhiteSpace(entry.Comment))
                .Select(entry => entry.Name)
                .Order(StringComparer.OrdinalIgnoreCase)
                .ToArray();

            Assert.True(undocumented.Length == 0,
                $"{locale} has undocumented duplicate visible values: " +
                string.Join(", ", undocumented));
        }
    }

    [Fact]
    public void EveryXamlUidHasAResourcePropertyInEveryLocale() {
        var uids = Directory.EnumerateFiles(RepositoryLayout.AppPath(), "*.xaml",
                                           SearchOption.AllDirectories)
            .Where(path => !IsGenerated(path))
            .SelectMany(path => XamlUidRegex().Matches(File.ReadAllText(path)))
            .Select(match => match.Groups[1].Value)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);

        foreach (var locale in Locales) {
            var names = Entries(locale).Select(entry => entry.Name).ToArray();
            var missing = uids.Where(uid => !names.Any(name =>
                    name.StartsWith(uid + ".", StringComparison.OrdinalIgnoreCase)))
                .Order(StringComparer.OrdinalIgnoreCase)
                .ToArray();
            Assert.True(missing.Length == 0,
                $"{locale} has no property resource for x:Uid values: " +
                string.Join(", ", missing));
        }
    }

    [Fact]
    public void BackendResourceIdentifiersUseDotsNotLegacyUnderscores() {
        foreach (var locale in Locales) {
            var legacy = Entries(locale).Select(entry => entry.Name)
                .Where(name => name.StartsWith("install_progress_",
                                               StringComparison.Ordinal) ||
                               name == "install_ready_to_run" ||
                               name.StartsWith("shop_category_",
                                               StringComparison.Ordinal) ||
                               name.StartsWith("shop.category.curseforge_",
                                               StringComparison.Ordinal) ||
                               name.StartsWith("shop_home_",
                                               StringComparison.Ordinal) ||
                               name == "error_server_running")
                .ToArray();
            Assert.Empty(legacy);
        }
    }

    [Fact]
    public void ProgrammaticDottedKeysUseSlashLookupPaths() {
        var source = File.ReadAllText(RepositoryLayout.AppPath(
            "Services", "LocalizationService.cs"));
        Assert.Contains("key.Replace('.', '/')", source,
                        StringComparison.Ordinal);
        Assert.DoesNotContain("key.Replace('.', '_')", source,
                              StringComparison.Ordinal);
    }

    [Fact]
    public void LiteralProgrammaticResourceKeysExistInEveryLocale() {
        var referenced = SourceFiles("*.cs")
            .SelectMany(path => LiteralLocalizerKeyRegex()
                .Matches(File.ReadAllText(path)))
            .Select(match => match.Groups[1].Value)
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .Order(StringComparer.OrdinalIgnoreCase)
            .ToArray();

        foreach (var locale in Locales) {
            var names = Entries(locale).Select(entry => entry.Name).ToHashSet(
                StringComparer.OrdinalIgnoreCase);
            var missing = referenced.Where(key => !names.Contains(key)).ToArray();
            Assert.True(missing.Length == 0,
                $"{locale} is missing literal C# resource keys: " +
                string.Join(", ", missing));
        }
    }

    [Fact]
    public void ResourceArchitectureIsDocumentedForFutureChanges() {
        var guide = RepositoryLayout.RootPath("docs", "resources.md");
        Assert.True(File.Exists(guide), "docs/resources.md is required");
        Assert.Contains("docs/resources.md",
            File.ReadAllText(RepositoryLayout.RootPath("AGENTS.md")),
            StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void EveryProgrammaticLocalizationSourceIsDocumented() {
        var guide = File.ReadAllText(RepositoryLayout.RootPath(
            "docs", "resources.md"));
        var undocumented = SourceFiles("*.cs")
            .Where(path => ProgrammaticLocalizerCallRegex().IsMatch(
                File.ReadAllText(path)))
            .Select(path => Path.GetRelativePath(RepositoryLayout.Root, path)
                .Replace('\\', '/'))
            .Where(path => !guide.Contains($"`{path}`",
                StringComparison.Ordinal))
            .Order(StringComparer.OrdinalIgnoreCase)
            .ToArray();

        Assert.True(undocumented.Length == 0,
            "Programmatic localization sources missing from the approved " +
            "exception inventory: " + string.Join(", ", undocumented));
    }

    [Fact]
    public void LocalizedDialogsAreDeclaredInXaml() {
        var offenders = SourceFiles("*.cs")
            .Where(path => NewContentDialogRegex().IsMatch(File.ReadAllText(path)))
            .Select(path => Path.GetRelativePath(RepositoryLayout.Root, path))
            .ToArray();
        Assert.Empty(offenders);
    }

    [Fact]
    public void LocalizedTooltipsAreNotInjectedFromCode() {
        var offenders = SourceFiles("*.cs")
            .Where(path => File.ReadAllText(path).Contains(
                "ToolTipService.SetToolTip", StringComparison.Ordinal))
            .Select(path => Path.GetRelativePath(RepositoryLayout.Root, path))
            .ToArray();
        Assert.Empty(offenders);
    }

    [Fact]
    public void LocalizedXamlElementsAreNotUsedAsStringCarriers() {
        var offenders = SourceFiles("*.xaml.cs")
            .Where(path => XamlTextCarrierRegex().IsMatch(File.ReadAllText(path)))
            .Select(path => Path.GetRelativePath(RepositoryLayout.Root, path))
            .ToArray();
        Assert.Empty(offenders);
    }

    [Fact]
    public void PermissionLabelsAreRenderedFromSemanticKindsInXaml() {
        var controlXaml = File.ReadAllText(RepositoryLayout.AppPath(
            "Controls", "PermissionLabel.xaml"));
        var expectedUids = new[] {
            "PermissionLabelNetwork",
            "PermissionLabelInstall",
            "PermissionLabelFsServer",
            "PermissionLabelConsoleRead",
            "PermissionLabelConsoleWrite",
            "PermissionLabelServerControl",
            "PermissionLabelSchedule",
            "PermissionLabelServerQuery",
            "PermissionLabelPlayerManage",
            "PermissionLabelUnknown",
        };
        foreach (var uid in expectedUids)
            Assert.Contains($"x:Uid=\"{uid}\"", controlXaml,
                            StringComparison.Ordinal);

        var scriptCard = File.ReadAllText(RepositoryLayout.AppPath(
            "Controls", "ScriptEntryCard.xaml"));
        Assert.Contains("Item.Granted", scriptCard, StringComparison.Ordinal);
        Assert.Contains("controls:PermissionLabel", scriptCard,
                        StringComparison.Ordinal);

        var consentDialog = File.ReadAllText(RepositoryLayout.AppPath(
            "Views", "PermissionConsentDialog.xaml"));
        Assert.Contains("controls:PermissionLabel", consentDialog,
                        StringComparison.Ordinal);
        Assert.Contains("Kind=\"{x:Bind Kind}\"", consentDialog,
                        StringComparison.Ordinal);

        var sources = SourceFiles("*.cs")
            .Select(path => (Path: path, Source: File.ReadAllText(path)))
            .ToArray();
        Assert.DoesNotContain(sources, item =>
            item.Source.Contains("PermissionLabels.Label(",
                                 StringComparison.Ordinal) ||
            item.Source.Contains("GrantedSummary", StringComparison.Ordinal));
        Assert.DoesNotContain("public string Label { get; }",
            File.ReadAllText(RepositoryLayout.AppPath(
                "Views", "PermissionConsentDialog.xaml.cs")),
            StringComparison.Ordinal);
    }

    [Fact]
    public void RawExceptionDiagnosticsAreNotDisplayedToUsers() {
        var forbidden = new[] {
            @"ex\.Status\.Detail\.Length\s*>\s*0",
            @"\(\""detail\""\s*,\s*ex\.Message\)",
            @"ex\.GetType\(\)\.Name",
        };
        var offenders = SourceFiles("*.cs")
            .Where(path => forbidden.Any(pattern => Regex.IsMatch(
                File.ReadAllText(path), pattern)))
            .Select(path => Path.GetRelativePath(RepositoryLayout.Root, path))
            .ToArray();
        Assert.Empty(offenders);
    }

    private static IEnumerable<string> SourceFiles(string pattern) =>
        Directory.EnumerateFiles(RepositoryLayout.AppPath(), pattern,
                                 SearchOption.AllDirectories)
            .Where(path => !IsGenerated(path));

    private static bool IsGenerated(string path) =>
        path.Contains($"{Path.DirectorySeparatorChar}obj{Path.DirectorySeparatorChar}",
                      StringComparison.OrdinalIgnoreCase) ||
        path.Contains($"{Path.DirectorySeparatorChar}bin{Path.DirectorySeparatorChar}",
                      StringComparison.OrdinalIgnoreCase);

    private static IReadOnlyList<ResourceEntry> Entries(string locale) =>
        ResourceCatalog.Load(RepositoryLayout.AppPath(
            "Strings", locale, "Resources.resw"));

    [GeneratedRegex("x:Uid\\s*=\\s*[\"']([^\"']+)[\"']")]
    private static partial Regex XamlUidRegex();

    [GeneratedRegex("new\\s+ContentDialog\\s*\\{")]
    private static partial Regex NewContentDialogRegex();

    [GeneratedRegex("(?:\\b\\w*[Ll]ocalizer|new\\s+LocalizationService\\(\\))\\.Get\\(\\s*[\"']([^\"']+)[\"']",
        RegexOptions.IgnoreCase)]
    private static partial Regex LiteralLocalizerKeyRegex();

    [GeneratedRegex("(?:\\b\\w*[Ll]ocalizer|new\\s+LocalizationService\\(\\))\\.Get\\(", RegexOptions.IgnoreCase)]
    private static partial Regex ProgrammaticLocalizerCallRegex();

    [GeneratedRegex("(?:=\\s*|string\\.Format\\()\\w+(?:Text|Label|Header)\\.Text\\b")]
    private static partial Regex XamlTextCarrierRegex();
}
