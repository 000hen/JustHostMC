using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.Core;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

public enum ShopDetailStatus {
    None,
    LoadFailed,
    InstallDone,
    InstallFailed,
}

/// <summary>One project's detail page: header card, gallery, rendered body,
/// compatible versions, and the install flow (dependency prompt + streamed
/// progress).</summary>
public sealed partial class ShopDetailViewModel : ObservableObject {
    private readonly ShopViewModel _shop;
    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;
    private readonly string _shopId;
    private readonly string _projectId;

    public ShopDetailViewModel(ShopViewModel shop, ShopProjectItem card,
                               DispatcherQueue dispatcher,
                               ILocalizer localizer) {
        _shop        = shop;
        _dispatcher  = dispatcher;
        _localizer   = localizer;
        _shopId      = card.Project.ShopId;
        _projectId   = card.Project.ProjectId;
        Card         = card;
        var shopInfo = shop.Shops.FirstOrDefault(s => s.Id == _shopId);
        SourceName   = shopInfo?.Name ?? _shopId;
        IsModpack    = shopInfo?.Kinds.Contains("modpack") ?? false;
    }

    /// <summary>True when this project is a modpack: its versions create whole
    /// new servers (via the ftb provider) instead of installing a file into
    /// one.</summary>
    public bool IsModpack { get; }

    /// <summary>Label for the header action button, matching the project
    /// kind.</summary>
    public string InstallActionLabel => _localizer.Get(
        IsModpack ? "Shop_CreateServerButton" : "Shop_InstallLatestButtonText");

    /// <summary>The card the user clicked; the header shows it immediately
    /// while the full detail loads.</summary>
    [ObservableProperty]
    public partial ShopProjectItem Card { get; private set; }

    /// <summary>"on Modrinth" / "on CurseForge" source attribution.</summary>
    public string SourceName { get; }

    public ObservableCollection<ShopVersionItem> Versions { get; } = new();
    public ObservableCollection<ShopGalleryItem> Gallery { get; }  = new();

    [ObservableProperty]
    public partial ShopVersionItem? LatestRelease {
        get; private set;
    }

    public ShopPrimaryAction PrimaryAction =>
        ShopPresentationPolicy.DeterminePrimaryAction(Card.Project.Distribution,
                                                      WebsiteUrl);

    public bool IsWebsiteAction =>
        PrimaryAction.Kind == ShopPrimaryActionKind.Website;

    public string WebsiteActionLabel =>
        IsWebsiteAction
            ? _localizer.Get("Shop_GetOnSource", ("source", SourceName))
            : "";

    public bool CanInstallLatest =>
        LatestRelease is not null && PrimaryAction.IsEnabled && !IsInstalling;

    [ObservableProperty]
    public partial string BodyHtml { get; private set; } = "";

    [ObservableProperty]
    public partial bool IsLoading {
        get; private set;
    } = true;

    [ObservableProperty]
    public partial bool IsVersionsLoading {
        get; private set;
    } = true;

    [ObservableProperty]
    public partial bool IsInstalling {
        get; private set;
    }

    [ObservableProperty]
    public partial double InstallProgress {
        get; private set;
    }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(HasStatus))]
    public partial string StatusMessage {
        get; private set;
    } = "";

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(HasStatus))]
    [NotifyPropertyChangedFor(nameof(IsLoadFailedStatus))]
    [NotifyPropertyChangedFor(nameof(IsInstallDoneStatus))]
    [NotifyPropertyChangedFor(nameof(IsInstallFailedStatus))]
    public partial ShopDetailStatus DetailStatus {
        get; private set;
    }

    public bool HasStatus => DetailStatus != ShopDetailStatus.None ||
                             !string.IsNullOrWhiteSpace(StatusMessage);
    public bool IsLoadFailedStatus =>
        DetailStatus == ShopDetailStatus.LoadFailed;
    public bool IsInstallDoneStatus =>
        DetailStatus == ShopDetailStatus.InstallDone;
    public bool IsInstallFailedStatus =>
        DetailStatus == ShopDetailStatus.InstallFailed;

    [ObservableProperty]
    public partial bool InstallSucceeded { get; private set; }

    [ObservableProperty]
    public partial string WebsiteUrl {
        get; private set;
    } = "";

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(HasProjectCompatWarning))]
    public partial string ProjectCompatMessage {
        get; private set;
    } = "";

    /// <summary>Whether to show the project-level "may not fit your server"
    /// banner.</summary>
    public bool HasProjectCompatWarning => ProjectCompatMessage.Length > 0;

    public bool HasGallery => Gallery.Count > 0;

    // A project-level mismatch means none of the project's loaders/versions
    // fit the server, so both mismatch kinds collapse to one summary banner.
    private string ProjectCompatText(
        ShopCompatVerdict compat) => compat switch {
        ShopCompatVerdict.LoaderMismatch or ShopCompatVerdict.VersionMismatch =>
            _localizer.Get("Shop_CompatProjectWarning",
                           ("serverLoader", _shop.Context.Loader),
                           ("serverVersion", _shop.Context.McVersion)),
        _ => "",
    };

    /// <summary>Loads detail + versions concurrently.</summary>
    public Task LoadAsync(bool darkTheme) =>
        Task.WhenAll(LoadDetailAsync(darkTheme), LoadVersionsAsync());

    private async Task LoadDetailAsync(bool darkTheme) {
        try {
            var daemon = await App.Current.DaemonReady;
            var detail = await daemon.Shop.GetProjectAsync(
                new ShopProjectRequest {
                    ShopId    = _shopId,
                    ProjectId = _projectId,
                },
                deadline: DateTime.UtcNow.AddSeconds(30));

            var html = detail.Body.Length > 0
                           ? ShopBodyRenderer.ToHtml(
                                 detail.Body, detail.BodyFormat, darkTheme)
                           : "";
            // The detail lists every loader/version the project supports; a
            // mismatch here means none of them fits this server. Suppressed
            // while filtering.
            var filtersActive = _shop.UseVersionFilter || _shop.UseLoaderFilter;
            var projectCompat =
                filtersActive
                    ? ShopCompatVerdict.Unknown
                    : ShopCompat.Evaluate(_shop.Context.Loader,
                                          _shop.Context.McVersion,
                                          detail.Loaders, detail.GameVersions);
            var compatMessage = ProjectCompatText(projectCompat);
            await RunOnUIAsync(() => {
                if (detail.Project is not null &&
                    detail.Project.Title.Length > 0)
                    Card = new ShopProjectItem(detail.Project);
                WebsiteUrl = detail.Links?.Website ?? "";
                Gallery.Clear();
                foreach (var image in detail.Gallery)
                    Gallery.Add(new ShopGalleryItem(image));
                OnPropertyChanged(nameof(HasGallery));
                BodyHtml             = html;
                ProjectCompatMessage = compatMessage;
                RefreshPrimaryAction();
            });
        } catch {
            await RunOnUIAsync(() => DetailStatus =
                                   ShopDetailStatus.LoadFailed);
        } finally {
            await RunOnUIAsync(() => IsLoading = false);
        }
    }

    private async Task LoadVersionsAsync() {
        try {
            var daemon = await App.Current.DaemonReady;
            var list   = await daemon.Shop.GetVersionsAsync(
                new ShopVersionsRequest {
                    ShopId    = _shopId,
                    ProjectId = _projectId,
                    McVersion =
                        _shop.UseVersionFilter ? _shop.Context.McVersion : "",
                    Loader = _shop.UseLoaderFilter ? _shop.Context.Loader : "",
                },
                deadline: DateTime.UtcNow.AddSeconds(30));
            // With a filter on, the engine already narrowed the list to
            // matches, so a per-row badge would be redundant noise.
            var showBadge = !_shop.UseVersionFilter && !_shop.UseLoaderFilter;
            await RunOnUIAsync(() => {
                Versions.Clear();
                foreach (var version in list.Versions)
                    Versions.Add(new ShopVersionItem(version, _shop.Context,
                                                     showBadge, IsModpack,
                                                     _localizer));
                LatestRelease =
                    Versions
                        .Where(version => version.Version.Channel ==
                                          ShopChannel.Release)
                        .OrderByDescending(version => version.Version.Date,
                                           StringComparer.Ordinal)
                        .FirstOrDefault();
                RefreshPrimaryAction();
            });
        } catch {
            await RunOnUIAsync(() => DetailStatus =
                                   ShopDetailStatus.LoadFailed);
        } finally {
            await RunOnUIAsync(() => IsVersionsLoading = false);
        }
    }

    /// <summary>Required dependencies of a version that are not already in the
    /// server's mods folder (matched by installed jar filename heuristics being
    /// unavailable, we surface every required dependency and let the user
    /// uncheck ones they know they have; deps whose title matches an installed
    /// jar name are pre-marked).</summary>
    public IReadOnlyList<ShopDependency> MissingDependencies(
        ShopVersionItem version) {
        var installed = _shop.Context.InstalledFileNames();
        return version.RequiredDependencies
            .Where(d => !installed.Any(
                       f => d.Title.Length > 0 &&
                            f.Contains(Slug(d.Title),
                                       StringComparison.OrdinalIgnoreCase)))
            .ToArray();

        static string Slug(string title) =>
            title.Replace(" ", "").ToLowerInvariant() is { Length : > 0 } s
                ? s
                : title;
    }

    /// <summary>Streams the install (selected version + confirmed
    /// deps).</summary>
    public async Task InstallAsync(ShopVersionItem version,
                                   IReadOnlyList<ShopDependency> dependencies) {
        using var backgroundTask =
            App.Current.BackgroundTasks.Begin("mod-download");
        await RunOnUIAsync(() => {
            IsInstalling     = true;
            InstallSucceeded = false;
            InstallProgress  = 0;
            StatusMessage    = "";
            DetailStatus     = ShopDetailStatus.None;
        });
        try {
            var daemon  = await App.Current.DaemonReady;
            var request = new ShopInstallRequest {
                ShopId    = _shopId,
                ServerId  = _shop.Context.ServerId,
                ProjectId = _projectId,
                VersionId = version.Version.Id,
            };
            foreach (var dependency in dependencies)
                request.Dependencies.Add(
                    new ShopDependencyInstall { ProjectId =
                                                    dependency.ProjectId });

            using var call = daemon.Shop.Install(
                request, deadline: DateTime.UtcNow.AddMinutes(10));
            await foreach (var progress in call.ResponseStream.ReadAllAsync()) {
                var fraction = progress.Fraction;
                await RunOnUIAsync(() => InstallProgress = fraction);
            }
            await RunOnUIAsync(() => {
                InstallSucceeded = true;
                DetailStatus     = ShopDetailStatus.InstallDone;
            });
            _shop.Context.OnInstalled();
        } catch (RpcException ex) {
            var key = ErrorKey(ex);
            await RunOnUIAsync(() => {
                DetailStatus  = ShopDetailStatus.None;
                StatusMessage = _localizer.Get(key);
            });
        } catch {
            await RunOnUIAsync(() => DetailStatus =
                                   ShopDetailStatus.InstallFailed);
        } finally {
            await RunOnUIAsync(() => { IsInstalling = false; });
        }
    }

    partial void OnIsInstallingChanged(bool value) => RefreshPrimaryAction();

    private void RefreshPrimaryAction() {
        OnPropertyChanged(nameof(PrimaryAction));
        OnPropertyChanged(nameof(IsWebsiteAction));
        OnPropertyChanged(nameof(WebsiteActionLabel));
        OnPropertyChanged(nameof(CanInstallLatest));
        var label   = WebsiteActionLabel;
        var enabled = PrimaryAction.IsEnabled && !IsInstalling;
        foreach (var version in Versions) {
            version.ActionLabel   = label;
            version.ActionEnabled = enabled;
        }
    }

    /// <summary>Creates a brand-new server from a modpack version by handing
    /// the request to the main window's global install flow, which owns the
    /// stream: the install survives this window closing, and the global
    /// progress UI shows steps, logs, and errors.</summary>
    public async Task CreateServerAsync(ShopVersionItem version, string name,
                                        int memoryMb) {
        if (_shop.Context.CreateServer is null) {
            await RunOnUIAsync(() => StatusMessage =
                                   _localizer.Get("Shop_CreateServerFailed"));
            return;
        }
        var request = new CreateServerRequest {
            Name = name,
            // A modpack shop's id doubles as its whole-server provider id
            // (e.g. "ftb"); the provider parses "{projectId}/{versionId}".
            ProviderId = _shopId,
            McVersion  = $"{_projectId}/{version.Version.Id}",
            MemoryMb   = memoryMb,
            Port       = 0,
        };
        _ = _shop.Context.CreateServer(request);
        await RunOnUIAsync(() => {
            InstallSucceeded = true;
            StatusMessage    = _localizer.Get("Shop_ServerCreateStarted");
        });
    }

    // Follows the app's StatusCode-level mapping convention (see
    // ModsViewModel.MapErrorKey); the diagnostic text disambiguates the two
    // FailedPrecondition causes without unpacking ErrorDetail.
    private static string ErrorKey(RpcException ex) => ex.StatusCode switch {
        StatusCode.NotFound => "Shop_ErrorNotFound",
        StatusCode.FailedPrecondition when ex.Status.Detail.Contains(
            "not distributable",
            StringComparison.OrdinalIgnoreCase) => "Shop_ErrorNotDistributable",
        StatusCode.FailedPrecondition when ex.Status.Detail.Contains(
            "key",
            StringComparison.OrdinalIgnoreCase) => "Shop_ErrorKeyMissing",
        StatusCode.FailedPrecondition           => "Shop_ErrorServerRunning",
        _                                       => "Shop_InstallFailed",
    };

    private Task RunOnUIAsync(Action action) {
        if (_dispatcher.HasThreadAccess) {
            action();
            return Task.CompletedTask;
        }
        var completion = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        if (!_dispatcher.TryEnqueue(() => {
                try {
                    action();
                    completion.SetResult();
                } catch (Exception ex) {
                    completion.SetException(ex);
                }
            })) {
            completion.SetException(new InvalidOperationException(
                "The UI dispatcher is unavailable."));
        }
        return completion.Task;
    }
}
