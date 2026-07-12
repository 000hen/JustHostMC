using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

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
        _shop       = shop;
        _dispatcher = dispatcher;
        _localizer  = localizer;
        _shopId     = card.Project.ShopId;
        _projectId  = card.Project.ProjectId;
        Card        = card;
        SourceName =
            shop.Shops.FirstOrDefault(s => s.Id == _shopId)?.Name ?? _shopId;
    }

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

    public bool CanInstallLatest => LatestRelease is not null && !IsInstalling;

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
    public partial string StatusMessage {
        get; private set;
    } = "";

    [ObservableProperty]
    public partial bool InstallSucceeded {
        get; private set;
    }

    [ObservableProperty]
    public partial string WebsiteUrl {
        get; private set;
    } = "";

    public bool HasGallery => Gallery.Count > 0;

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
            await RunOnUIAsync(() => {
                if (detail.Project is not null &&
                    detail.Project.Title.Length > 0)
                    Card = new ShopProjectItem(detail.Project);
                WebsiteUrl = detail.Links?.Website ?? "";
                Gallery.Clear();
                foreach (var image in detail.Gallery)
                    Gallery.Add(new ShopGalleryItem(image));
                OnPropertyChanged(nameof(HasGallery));
                BodyHtml = html;
            });
        } catch {
            await RunOnUIAsync(() => StatusMessage =
                                   _localizer.Get("Shop.LoadFailed"));
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
            await RunOnUIAsync(() => {
                Versions.Clear();
                foreach (var version in list.Versions)
                    Versions.Add(new ShopVersionItem(version));
                LatestRelease =
                    Versions
                        .Where(version => version.Version.Channel ==
                                          ShopChannel.Release)
                        .OrderByDescending(version => version.Version.Date,
                                           StringComparer.Ordinal)
                        .FirstOrDefault();
                OnPropertyChanged(nameof(CanInstallLatest));
            });
        } catch {
            await RunOnUIAsync(() => StatusMessage =
                                   _localizer.Get("Shop.LoadFailed"));
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
        await RunOnUIAsync(() => {
            IsInstalling = true;
            OnPropertyChanged(nameof(CanInstallLatest));
            InstallSucceeded = false;
            InstallProgress  = 0;
            StatusMessage    = "";
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
                StatusMessage    = _localizer.Get("Shop.InstallDone");
            });
            _shop.Context.OnInstalled();
        } catch (RpcException ex) {
            var key = ErrorKey(ex);
            await RunOnUIAsync(() => StatusMessage = _localizer.Get(key));
        } catch {
            await RunOnUIAsync(() => StatusMessage =
                                   _localizer.Get("Shop.InstallFailed"));
        } finally {
            await RunOnUIAsync(() => {
                IsInstalling = false;
                OnPropertyChanged(nameof(CanInstallLatest));
            });
        }
    }

    // Follows the app's StatusCode-level mapping convention (see
    // ModsViewModel.MapErrorKey); the diagnostic text disambiguates the two
    // FailedPrecondition causes without unpacking ErrorDetail.
    private static string ErrorKey(RpcException ex) => ex.StatusCode switch {
        StatusCode.NotFound => "Shop.ErrorNotFound",
        StatusCode.FailedPrecondition when ex.Status.Detail.Contains(
            "not distributable",
            StringComparison.OrdinalIgnoreCase) => "Shop.ErrorNotDistributable",
        StatusCode.FailedPrecondition when ex.Status.Detail.Contains(
            "key",
            StringComparison.OrdinalIgnoreCase) => "Shop.ErrorKeyMissing",
        StatusCode.FailedPrecondition           => "Shop.ErrorServerRunning",
        _                                       => "Shop.InstallFailed",
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
