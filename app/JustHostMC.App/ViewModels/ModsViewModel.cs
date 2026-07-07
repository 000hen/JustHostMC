using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using Google.Protobuf;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using System.Runtime.InteropServices.WindowsRuntime;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;
using Windows.Storage;
using Windows.Storage.Streams;

namespace JustHostMC.App.ViewModels;

/// <summary>Lists, uploads, and removes plugin/mod jars for a server via
/// ModService. Uploads and removals are only allowed while the server is
/// stopped.</summary>
public sealed partial class ModsViewModel : ObservableObject {
    private const int ChunkSize = 64 * 1024;

    private readonly string _serverId;
    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;
    private bool _serverStopped;
    private bool _loaded;
    private Task? _refreshTask;

    public ModsViewModel(string serverId, DispatcherQueue dispatcher,
                         ILocalizer localizer) {
        _serverId   = serverId;
        _dispatcher = dispatcher;
        _localizer  = localizer;
    }

    public ObservableCollection<ModFileItem> Files { get; } = new();

    /// <summary>Stable placeholder count used by the loading
    /// skeleton.</summary>
    public IReadOnlyList<int> LoadingRows {
        get;
    } = Enumerable.Range(0, 5).ToArray();

    [ObservableProperty]
    public partial bool Supported {
        get; private set;
    } = true;

    [ObservableProperty]
    public partial bool CanModify {
        get; private set;
    }

    [ObservableProperty]
    public partial bool IsBusy {
        get; private set;
    }

    [ObservableProperty]
    public partial bool IsInitialLoading {
        get; private set;
    } = true;

    [ObservableProperty]
    public partial bool IsRefreshing {
        get; private set;
    }

    [ObservableProperty]
    public partial string KindLabel {
        get; private set;
    } = "";

    public bool AcceptsLiteMod { get; private set; }

    /// <summary>The folder kind reported by the engine (plugins vs mods),
    /// used as the shop's project-type pre-filter.</summary>
    public ModKind Kind { get; private set; } = ModKind.Mod;

    /// <summary>Installed jar filenames, for the shop's already-installed
    /// checks.</summary>
    public IReadOnlyCollection<string> InstalledFileNames() =>
        Files.Select(f => f.Name).ToArray();

    public bool ShowOperationProgress =>
        !IsInitialLoading && (IsBusy || IsRefreshing);

    [ObservableProperty]
    public partial string StatusMessage { get; private set; } = "";

    partial void OnSupportedChanged(bool value) => RecomputeCanModify();

    partial void OnIsBusyChanged(bool value) {
        OnPropertyChanged(nameof(ShowOperationProgress));
        RecomputeCanModify();
    }

    partial void OnIsInitialLoadingChanged(bool value) {
        OnPropertyChanged(nameof(ShowOperationProgress));
        RecomputeCanModify();
    }

    partial void OnIsRefreshingChanged(bool value) {
        OnPropertyChanged(nameof(ShowOperationProgress));
        RecomputeCanModify();
    }

    /// <summary>Updates the stopped-server gate that allows
    /// upload/remove.</summary>
    public void SetServerStopped(bool stopped) {
        _serverStopped = stopped;
        RecomputeCanModify();
    }

    /// <summary>Reloads the jar list from the engine.</summary>
    public async Task RefreshAsync() {
        if (_refreshTask is { IsCompleted : false }) {
            await _refreshTask;
            return;
        }

        _refreshTask = RefreshCoreAsync();
        await _refreshTask;
    }

    /// <summary>Loads the jar list once; repeated tab visits reuse cached
    /// data.</summary>
    public async Task EnsureLoadedAsync() {
        if (_loaded)
            return;

        await RefreshAsync();
    }

    private async Task RefreshCoreAsync() {
        var isInitialLoad = !_loaded;
        await RunOnUIAsync(() => {
            IsInitialLoading = isInitialLoad;
            IsRefreshing     = !isInitialLoad;
            StatusMessage    = "";
        });

        try {
            // Keep the potentially long parser RPC and its continuation away
            // from the UI synchronization context. The engine itself parses out
            // of process.
            var daemon = await App.Current.DaemonReady.ConfigureAwait(false);
            var call   = daemon.Mods.ListAsync(new ServerId { Id = _serverId });
            var list   = await call.ResponseAsync.ConfigureAwait(false);
            var files  = list.Files.ToArray();

            await RunOnUIAsync(() => {
                Supported      = list.Supported;
                Kind           = list.Kind;
                AcceptsLiteMod = list.Kind == ModKind.Mod;
                KindLabel      = _localizer.Get(list.Kind == ModKind.Mod
                                                    ? "Mods_KindMods"
                                                    : "Mods_KindPlugins");
                ApplyFileDiff(files);
                _loaded = true;
            });
        } catch (RpcException) {
            await RunOnUIAsync(() => StatusMessage =
                                   _localizer.Get("Mods_OperationFailed"))
                .ConfigureAwait(false);
        } catch {
            await RunOnUIAsync(() => StatusMessage =
                                   _localizer.Get("Mods_OperationFailed"))
                .ConfigureAwait(false);
        } finally {
            await RunOnUIAsync(() => {
                IsInitialLoading = false;
                IsRefreshing     = false;
            }).ConfigureAwait(false);
        }
    }

    /// <summary>Reconciles the fetched snapshot with the observable collection.
    /// Existing rows stay in place, while additions, removals, moves, and
    /// changed metadata produce only their corresponding collection
    /// notifications.</summary>
    private void ApplyFileDiff(IReadOnlyList<ModFile> files) {
        var desiredNames = files.Select(file => file.Name)
                               .ToHashSet(StringComparer.OrdinalIgnoreCase);

        for (var index = Files.Count - 1; index >= 0; index--) {
            if (!desiredNames.Contains(Files[index].Name))
                Files.RemoveAt(index);
        }

        for (var desiredIndex = 0; desiredIndex < files.Count; desiredIndex++) {
            var file          = files[desiredIndex];
            var existingIndex = -1;
            for (var index = desiredIndex; index < Files.Count; index++) {
                if (string.Equals(Files[index].Name, file.Name,
                                  StringComparison.OrdinalIgnoreCase)) {
                    existingIndex = index;
                    break;
                }
            }

            if (existingIndex < 0) {
                Files.Insert(desiredIndex, CreateItem(file));
                continue;
            }

            if (existingIndex != desiredIndex)
                Files.Move(existingIndex, desiredIndex);
            if (!Matches(Files[desiredIndex], file))
                Files[desiredIndex] = CreateItem(file);
        }
    }

    private static bool Matches(ModFileItem item, ModFile file) {
        var metadata    = file.Metadata;
        var hasMetadata = metadata is { Parsed : true };
        return item.Name == file.Name && item.SizeBytes == file.SizeBytes &&
               item.HasMetadata == hasMetadata &&
               (!hasMetadata ||
                (item.DisplayName == metadata!.Name &&
                 item.ModId == metadata.ModId &&
                 item.Version == metadata.Version &&
                 item.Description == metadata.Description &&
                 item.Website == metadata.Website &&
                 item.Loader == metadata.Loader &&
                 item.Authors ==
                     string.Join(", ", metadata.Authors.Where(
                                           author => author.Length > 0))));
    }

    /// <summary>Streams a chosen jar to the engine, then refreshes.</summary>
    public async Task UploadAsync(StorageFile file) {
        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = "";
        });
        try {
            var daemon     = await App.Current.DaemonReady;
            using var call = daemon.Mods.Upload();
            await call.RequestStream.WriteAsync(new UploadModRequest {
                Init = new UploadModInit { ServerId = _serverId,
                                          Filename = file.Name },
            });

            using var stream = (await file.OpenReadAsync()).AsStreamForRead();
            var buffer       = new byte[ChunkSize];
            int read;
            while ((read = await stream.ReadAsync(buffer)) > 0) {
                await call.RequestStream.WriteAsync(new UploadModRequest {
                    Chunk = ByteString.CopyFrom(buffer, 0, read),
                });
            }

            await call.RequestStream.CompleteAsync();
            await call.ResponseAsync;
            await RefreshAsync();
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    /// <summary>Deletes one jar, then refreshes.</summary>
    public async Task RemoveAsync(ModFileItem item) {
        RunOnUI(() => IsBusy = true);
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Mods.RemoveAsync(new RemoveModRequest {
                ServerId = _serverId, Name = item.Name
            });
            await RefreshAsync();
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    /// <summary>Builds a list item, decoding the parsed jar icon (if any) into
    /// a BitmapImage. Must run on the UI thread (BitmapImage is a UI object);
    /// the async decode fills the image in place while the list already
    /// shows.</summary>
    private ModFileItem CreateItem(ModFile file) {
        ImageSource? icon = null;
        if (file.Metadata is { Parsed : true } meta && meta.Icon.Length > 0) {
            var bitmap = new BitmapImage { DecodePixelWidth = 64 };
            _          = LoadIconAsync(bitmap, meta.Icon);
            icon       = bitmap;
        }
        return new ModFileItem(file.Name, file.SizeBytes, file.Metadata, icon,
                               _localizer);
    }

    private static async Task LoadIconAsync(BitmapImage bitmap,
                                            ByteString bytes) {
        try {
            using var stream = new InMemoryRandomAccessStream();
            await stream.WriteAsync(bytes.ToByteArray().AsBuffer());
            stream.Seek(0);
            await bitmap.SetSourceAsync(stream);
        } catch {
            // Undecodable icon bytes: the item keeps its fallback glyph area.
        }
    }

    private void RecomputeCanModify() => CanModify = Supported && _serverStopped
                                                     && !IsBusy &&
                                                     !IsInitialLoading &&
                                                     !IsRefreshing;

    /// <summary>Zips the whole plugins/mods folder to a user-picked .zip.
    /// Read-only on the server dir, so it works while the server
    /// runs.</summary>
    public async Task ExportAllAsync(string destPath) {
        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = "";
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Mods.ExportAllAsync(
                new ExportModsRequest {
                    ServerId = _serverId,
                    DestPath = destPath,
                },
                deadline: DateTime.UtcNow.AddMinutes(2));
            RunOnUI(() => StatusMessage = _localizer.Get("Mods_ExportDone"));
        } catch (RpcException) {
            RunOnUI(() => StatusMessage = _localizer.Get("Mods_ExportFailed"));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    private static string MapErrorKey(RpcException ex) => ex.StatusCode switch {
        StatusCode.FailedPrecondition => "Mods_StoppedRequired",
        _                             => "Mods_OperationFailed",
    };

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }

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
