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

    public ModsViewModel(string serverId, DispatcherQueue dispatcher, ILocalizer localizer)
    {
        _serverId = serverId;
        _dispatcher = dispatcher;
        _localizer = localizer;
    }

    public ObservableCollection<ModFileItem> Files { get; } = new();

    private bool _supported = true;
    public bool Supported
    {
        get => _supported;
        private set
        {
            if (SetProperty(ref _supported, value))
                RecomputeCanModify();
        }
    }

    private bool _canModify;
    public bool CanModify
    {
        get => _canModify;
        private set => SetProperty(ref _canModify, value);
    }

    private bool _isBusy;
    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    private string _kindLabel = "";
    public string KindLabel
    {
        get => _kindLabel;
        private set => SetProperty(ref _kindLabel, value);
    }

    private string _statusMessage = "";
    public string StatusMessage
    {
        get => _statusMessage;
        private set => SetProperty(ref _statusMessage, value);
    }

    /// <summary>Updates the stopped-server gate that allows upload/remove.</summary>
    public void SetServerStopped(bool stopped)
    {
        _serverStopped = stopped;
        RecomputeCanModify();
    }

    /// <summary>Reloads the jar list from the engine.</summary>
    public async Task RefreshAsync()
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

    private void RecomputeCanModify() => CanModify = _supported && _serverStopped;

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
