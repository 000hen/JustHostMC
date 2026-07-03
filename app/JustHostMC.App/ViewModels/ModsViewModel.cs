using System;
using System.Collections.ObjectModel;
using System.IO;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using Google.Protobuf;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;
using Windows.Storage;

namespace JustHostMC.App.ViewModels;

/// <summary>Lists, uploads, and removes plugin/mod jars for a server via ModService.
/// Uploads and removals are only allowed while the server is stopped.</summary>
public sealed partial class ModsViewModel : ObservableObject
{
    private const int ChunkSize = 64 * 1024;

    private readonly string _serverId;
    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;
    private bool _serverStopped;
    private bool _loaded;
    private Task? _refreshTask;

    public ModsViewModel(string serverId, DispatcherQueue dispatcher, ILocalizer localizer)
    {
        _serverId = serverId;
        _dispatcher = dispatcher;
        _localizer = localizer;
    }

    public ObservableCollection<ModFileItem> Files { get; } = new();

    [ObservableProperty]
    public partial bool Supported { get; private set; } = true;

    [ObservableProperty]
    public partial bool CanModify { get; private set; }

    [ObservableProperty]
    public partial bool IsBusy { get; private set; }

    [ObservableProperty]
    public partial string KindLabel { get; private set; } = "";

    [ObservableProperty]
    public partial string StatusMessage { get; private set; } = "";

    partial void OnSupportedChanged(bool value) => RecomputeCanModify();

    /// <summary>Updates the stopped-server gate that allows upload/remove.</summary>
    public void SetServerStopped(bool stopped)
    {
        _serverStopped = stopped;
        RecomputeCanModify();
    }

    /// <summary>Reloads the jar list from the engine.</summary>
    public async Task RefreshAsync()
    {
        if (_refreshTask is { IsCompleted: false })
        {
            await _refreshTask;
            return;
        }

        _refreshTask = RefreshCoreAsync();
        await _refreshTask;
    }

    /// <summary>Loads the jar list once; repeated tab visits reuse cached data.</summary>
    public async Task EnsureLoadedAsync()
    {
        if (_loaded)
            return;

        await RefreshAsync();
    }

    private async Task RefreshCoreAsync()
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            var list = await daemon.Mods.ListAsync(new ServerId { Id = _serverId });
            RunOnUI(() =>
            {
                Supported = list.Supported;
                KindLabel = _localizer.Get(list.Kind == ModKind.Mod ? "Mods_KindMods" : "Mods_KindPlugins");
                Files.Clear();
                foreach (var file in list.Files)
                    Files.Add(new ModFileItem(file.Name, file.SizeBytes));
                _loaded = true;
            });
        }
        catch (RpcException)
        {
            // Transient; a later refresh reconciles.
        }
    }

    /// <summary>Streams a chosen jar to the engine, then refreshes.</summary>
    public async Task UploadAsync(StorageFile file)
    {
        RunOnUI(() => { IsBusy = true; StatusMessage = ""; });
        try
        {
            var daemon = await App.Current.DaemonReady;
            using var call = daemon.Mods.Upload();
            await call.RequestStream.WriteAsync(new UploadModRequest
            {
                Init = new UploadModInit { ServerId = _serverId, Filename = file.Name },
            });

            using var stream = (await file.OpenReadAsync()).AsStreamForRead();
            var buffer = new byte[ChunkSize];
            int read;
            while ((read = await stream.ReadAsync(buffer)) > 0)
            {
                await call.RequestStream.WriteAsync(new UploadModRequest
                {
                    Chunk = ByteString.CopyFrom(buffer, 0, read),
                });
            }

            await call.RequestStream.CompleteAsync();
            await call.ResponseAsync;
            await RefreshAsync();
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    /// <summary>Deletes one jar, then refreshes.</summary>
    public async Task RemoveAsync(ModFileItem item)
    {
        RunOnUI(() => IsBusy = true);
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Mods.RemoveAsync(new RemoveModRequest { ServerId = _serverId, Name = item.Name });
            await RefreshAsync();
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    private void RecomputeCanModify() => CanModify = Supported && _serverStopped;

    private static string MapErrorKey(RpcException ex) => ex.StatusCode switch
    {
        StatusCode.FailedPrecondition => "Mods_StoppedRequired",
        _ => "Mods_OperationFailed",
    };

    private void RunOnUI(Action action)
    {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
