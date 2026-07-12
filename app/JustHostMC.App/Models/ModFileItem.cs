using CommunityToolkit.Mvvm.ComponentModel;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media;

namespace JustHostMC.App.Models;

/// <summary>A plugin/mod jar shown in the server page's Plugins/Mods panel,
/// enriched with metadata parsed from the jar's embedded descriptor when an
/// installed parser recognized it.</summary>
public sealed class ModFileItem : ObservableObject {
    public ModFileItem(string name, long sizeBytes, ModMetadata? metadata,
                       ImageSource? icon, ILocalizer localizer) {
        Name      = name;
        SizeBytes = sizeBytes;
        _icon     = icon;
        if (metadata is not null && metadata.ParseError.Length > 0) {
            ParseError = localizer.Get("Mods.ParseFailed",
                                       ("error", metadata.ParseError));
        }
        if (metadata is { Parsed : true }) {
            HasMetadata = true;
            DisplayName = metadata.Name;
            ModId       = metadata.ModId;
            Version     = metadata.Version;
            Description = metadata.Description;
            Website     = metadata.Website;
            Loader      = metadata.Loader;
            Authors =
                string.Join(", ", metadata.Authors.Where(a => a.Length > 0));
            CompatibilityWarning =
                (metadata.LoaderMismatch, metadata.GameVersionMismatch) switch {
                    (true, true) => localizer.Get(
                        "Mods.TypeAndVersionMismatch", ("loader", Loader),
                        ("version", metadata.GameVersionRequirement)),
                    (true, false) =>
                        localizer.Get("Mods.TypeMismatch", ("loader", Loader)),
                    (false, true) => localizer.Get(
                        "Mods.VersionMismatch",
                        ("version", metadata.GameVersionRequirement)),
                    _ => "",
                };
        }
    }

    public string Name { get; }
    public long SizeBytes { get; }

    public bool HasMetadata { get; }
    public string DisplayName { get; }          = "";
    public string ModId { get; }                = "";
    public string Version { get; }              = "";
    public string Description { get; }          = "";
    public string Website { get; }              = "";
    public string Loader { get; }               = "";
    public string Authors { get; }              = "";
    public string CompatibilityWarning { get; } = "";
    public string ParseError { get; }           = "";

    /// <summary>Decoded jar icon; null when the jar has none (a glyph shows
    /// instead).</summary>
    private ImageSource? _icon;
    public ImageSource? Icon => _icon;

    public void SetIcon(ImageSource icon) {
        if (!SetProperty(ref _icon, icon, nameof(Icon)))
            return;
        OnPropertyChanged(nameof(IconVisibility));
        OnPropertyChanged(nameof(FallbackIconVisibility));
    }

    /// <summary>Parsed display name, falling back to the jar
    /// filename.</summary>
    public string Title =>
        HasMetadata && DisplayName.Length > 0 ? DisplayName : Name;

    /// <summary>Locale-neutral detail line: "version · authors" (either part
    /// optional).</summary>
    public string InfoLine =>
        string.Join(" · ", new[] { Version, Authors }.Where(s => s.Length > 0));

    public Visibility IconVisibility =>
        Icon is null ? Visibility.Collapsed : Visibility.Visible;
    public Visibility FallbackIconVisibility =>
        Icon is null ? Visibility.Visible : Visibility.Collapsed;
    public Visibility FileNameVisibility =>
        HasMetadata && DisplayName.Length > 0? Visibility.Visible
        : Visibility.Collapsed;
    public Visibility InfoLineVisibility =>
        InfoLine.Length > 0? Visibility.Visible : Visibility.Collapsed;
    public Visibility DescriptionVisibility =>
        Description.Length > 0? Visibility.Visible : Visibility.Collapsed;
    public Visibility WebsiteVisibility =>
        WebsiteUri is null ? Visibility.Collapsed : Visibility.Visible;
    public Visibility CompatibilityWarningVisibility =>
        CompatibilityWarning.Length > 0? Visibility.Visible
        : Visibility.Collapsed;
    public Visibility ParseErrorVisibility =>
        ParseError.Length > 0? Visibility.Visible : Visibility.Collapsed;

    /// <summary>Website as a Uri for HyperlinkButton.NavigateUri; null when
    /// absent or not an absolute http(s) URL.</summary>
    public System.Uri? WebsiteUri =>
        System.Uri.TryCreate(Website, System.UriKind.Absolute, out var uri) &&
                (uri.Scheme == System.Uri.UriSchemeHttp ||
                 uri.Scheme == System.Uri.UriSchemeHttps)
            ? uri
            : null;

    public string SizeText => SizeBytes switch {
        >= 1 <<
                20 => $"{SizeBytes / (double)(1 << 20):0.0} MB",
        >= 1 <<
                10 => $"{SizeBytes / (double)(1 << 10):0.0} KB",
        _          => $"{SizeBytes} B",
    };
}
