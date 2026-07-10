using CommunityToolkit.Mvvm.ComponentModel;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media.Imaging;

namespace JustHostMC.App.Models;

/// <summary>Everything the shop window needs to know about the server it was
/// opened for: the compatibility pre-filter, the installed-jar lookup used to
/// mark dependencies as already present, and the refresh hook fired after an
/// install so the server's mod list updates.</summary>
public sealed record ShopContext(
    string ServerId, string McVersion, string Loader, ModKind Kind,
    Func<IReadOnlyCollection<string>> InstalledFileNames, Action OnInstalled,
    // Invoked after a modpack shop creates a brand-new server so the opener
    // can refresh its server list; null for a per-server mod shop, which only
    // installs into the existing server.
    Action? OnServerCreated = null);

/// <summary>One project card (home/search results).</summary>
public sealed class ShopProjectItem {
    public ShopProjectItem(ShopProject project) {
        Project = project;
        if (Uri.TryCreate(project.IconUrl, UriKind.Absolute, out var uri) &&
            (uri.Scheme == Uri.UriSchemeHttps ||
             uri.Scheme == Uri.UriSchemeHttp)) {
            Icon = new BitmapImage(uri) { DecodePixelWidth = 96 };
        }
    }

    public ShopProject Project { get; }
    public string Title => Project.Title;
    public string Initial =>
        string.IsNullOrWhiteSpace(Title) ? "?" : Title[..1].ToUpperInvariant();
    public string Author        => Project.Author;
    public string AuthorLine    => Author;
    public string Summary       => Project.Summary;
    public string DownloadsText => ShopFormat.Count(Project.Downloads);
    public string ProjectTypeLabel =>
        Project.ProjectType.Length == 0
            ? ""
            : char.ToUpperInvariant(Project.ProjectType[0]) +
                  Project.ProjectType[1..];
    public string CategoriesText =>
        string.Join("  ·  ", Project.Categories.Take(4));
    public BitmapImage? Icon { get; }
    public Visibility IconVisibility =>
        Icon is null ? Visibility.Collapsed : Visibility.Visible;
    public Visibility FallbackIconVisibility =>
        Icon is null ? Visibility.Visible : Visibility.Collapsed;
    public Visibility AuthorVisibility =>
        Author.Length == 0 ? Visibility.Collapsed : Visibility.Visible;
    public Visibility ProjectTypeVisibility => ProjectTypeLabel.Length == 0
                                                   ? Visibility.Collapsed
                                                   : Visibility.Visible;
    public Visibility CategoriesVisibility =>
        CategoriesText.Length == 0 ? Visibility.Collapsed : Visibility.Visible;
}

/// <summary>One home-page section (a localized title plus its cards).</summary>
public sealed class ShopSectionItem {
    public ShopSectionItem(string title, string description,
                           IReadOnlyList<ShopProjectItem> projects) {
        Title       = title;
        Description = description;
        Projects    = projects;
    }

    public string Title { get; }
    public string Description { get; }
    public IReadOnlyList<ShopProjectItem> Projects { get; }
}

/// <summary>One screenshot in the detail-page gallery.</summary>
public sealed class ShopGalleryItem {
    public ShopGalleryItem(ShopGalleryImage image) {
        Title = image.Title;
        if (Uri.TryCreate(image.Url, UriKind.Absolute, out var uri) &&
            (uri.Scheme == Uri.UriSchemeHttps ||
             uri.Scheme == Uri.UriSchemeHttp)) {
            Image = new BitmapImage(uri);
        }
    }

    public string Title { get; }
    public BitmapImage? Image { get; }
    public Visibility TitleVisibility =>
        Title.Length == 0 ? Visibility.Collapsed : Visibility.Visible;
}

/// <summary>One installable version row on the detail page.</summary>
public sealed partial class ShopVersionItem : ObservableObject {
    public ShopVersionItem(ShopVersion version) {
        Version = version;
        RequiredDependencies =
            version.Dependencies.Where(d => d.Required).ToArray();
    }

    /// <summary>Overload that computes a compatibility badge against the
    /// server the shop was opened for. <paramref name="showBadge"/> is false
    /// when the user's version/loader filters are active — the engine already
    /// narrowed the list, so a badge would be noise.</summary>
    public ShopVersionItem(ShopVersion version, ShopContext context,
                           bool showBadge, bool isModpack, ILocalizer localizer)
        : this(version) {
        ActionLabel = localizer.Get(isModpack ? "Shop_CreateServerButton"
                                              : "Shop_InstallButtonText");
        if (showBadge) {
            Compat = ShopCompat.Evaluate(context.Loader, context.McVersion,
                                         version.Loaders, version.GameVersions);
            CompatToolTip =
                BuildCompatToolTip(Compat, version, context, localizer);
        }
    }

    public ShopVersion Version { get; }
    public IReadOnlyList<ShopDependency> RequiredDependencies { get; }

    [ObservableProperty]
    public partial string ActionLabel {
        get; set;
    } = "";

    [ObservableProperty]
    public partial bool ActionEnabled {
        get; set;
    }

    public string Name =>
        Version.Name.Length > 0? Version.Name : Version.VersionNumber;
    public string VersionNumber => Version.VersionNumber;
    public string ChannelText   => Version.Channel switch {
        ShopChannel.Release => "Release",
        ShopChannel.Beta    => "Beta",
        ShopChannel.Alpha   => "Alpha",
        _                   => "",
    };
    public Visibility ChannelVisibility =>
        ChannelText.Length == 0 ? Visibility.Collapsed : Visibility.Visible;
    public string GameVersionsText =>
        string.Join(", ", Version.GameVersions.Take(6)) +
        (Version.GameVersions.Count > 6 ? "…" : "");
    public string InfoLine {
        get {
            var parts = new List<string>(3);
            if (Version.Date.Length >= 10)
                parts.Add(Version.Date[..10]);
            if (Version.SizeBytes > 0)
                parts.Add(ShopFormat.Bytes(Version.SizeBytes));
            if (Version.Downloads > 0)
                parts.Add(ShopFormat.Count(Version.Downloads));
            return string.Join("  ·  ", parts);
        }
    }

    /// <summary>Client-side compatibility verdict for this listing against the
    /// server the shop was opened for; Unknown when not evaluated.</summary>
    public ShopCompatVerdict Compat { get; } = ShopCompatVerdict.Unknown;

    /// <summary>Localized explanation shown on the warning badge; empty unless
    /// a mismatch was detected.</summary>
    public string CompatToolTip { get; } = "";

    public Visibility CompatBadgeVisibility =>
        Compat is ShopCompatVerdict.LoaderMismatch or
                ShopCompatVerdict.VersionMismatch
            ? Visibility.Visible
            : Visibility.Collapsed;

    private static string BuildCompatToolTip(
        ShopCompatVerdict compat, ShopVersion version, ShopContext context,
        ILocalizer localizer) => compat switch {
        ShopCompatVerdict.LoaderMismatch =>
            localizer.Get("Shop_CompatLoaderMismatch",
                          ("modLoaders", string.Join(", ", version.Loaders)),
                          ("serverLoader", context.Loader)),
        ShopCompatVerdict.VersionMismatch => localizer.Get(
            "Shop_CompatVersionMismatch",
            ("modVersions", string.Join(", ", version.GameVersions.Take(6))),
            ("serverVersion", context.McVersion)),
        _ => "",
    };
}

/// <summary>Display formatting shared by the shop UI.</summary>
public static class ShopFormat {
    public static string Count(long n) => n switch {
        >=
            1_000_000 => $"{n / 1_000_000.0:0.#}M",
        >=
            1_000 => $"{n / 1_000.0:0.#}K",
        _         => n.ToString(),
    };

    public static string Bytes(long n) => n switch {
        >= 1 <<
                30 => $"{n / (double)(1 << 30):0.#} GB",
        >= 1 <<
                20 => $"{n / (double)(1 << 20):0.#} MB",
        >= 1 <<
                10 => $"{n / (double)(1 << 10):0.#} KB",
        _          => $"{n} B",
    };
}
