using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>Manages the backups of a single server: list, create, restore,
/// delete.</summary>
public sealed partial class BackupsViewModel : ObservableObject {
    private readonly string _serverId;
    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(CanRunActions))]
    [NotifyPropertyChangedFor(nameof(CanRestore))]
    public partial bool IsBusy { get; private set; }

    [ObservableProperty]
    public partial bool SafeOnline {
        get; set;
    }

    [ObservableProperty]
    public partial string StatusMessage {
        get; set;
    } = "";

    public BackupsViewModel(string serverId, bool serverRunning,
                            DispatcherQueue dispatcher, ILocalizer localizer) {
        _serverId     = serverId;
        _dispatcher   = dispatcher;
        _localizer    = localizer;
        ServerRunning = serverRunning;
        SafeOnline    = serverRunning;  // a running server defaults to a safe
                                        // online snapshot
    }

    public ObservableCollection<BackupItem> Backups { get; } = new();

    /// <summary>True when the server is running, so safe-online applies and
    /// restore is blocked.</summary>
    public bool ServerRunning { get; }

    public bool CanRunActions => !IsBusy;

    public bool CanRestore => !ServerRunning && !IsBusy;

    /// <summary>Loads (or reloads) the server's backups, newest
    /// first.</summary>
    public async Task LoadAsync() {
        try {
            var daemon = await App.Current.DaemonReady;
            var list   = await daemon.Backups.ListAsync(
                new ServerId { Id = _serverId },
                deadline: DateTime.UtcNow.AddSeconds(30));
            RunOnUI(() => {
                Backups.Clear();
                foreach (var b in list.Backups) Backups.Add(new BackupItem(b));
            });
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapBackupError(ex)));
        }
    }

    [RelayCommand]
    private async Task CreateBackup() {
        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = _localizer.Get("Backups_Creating");
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Backups.CreateAsync(
                new CreateBackupRequest { ServerId   = _serverId,
                                          SafeOnline = SafeOnline },
                deadline: DateTime.UtcNow.AddMinutes(10));
            await LoadAsync();
            RunOnUI(() => StatusMessage = _localizer.Get("Backups_Created"));
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapBackupError(ex)));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    [RelayCommand]
    private async Task Restore(BackupItem? item) {
        if (item is null)
            return;
        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = _localizer.Get("Backups_Restoring");
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Backups.RestoreAsync(
                new RestoreBackupRequest { ServerId = _serverId,
                                           BackupId = item.Id },
                deadline: DateTime.UtcNow.AddMinutes(10));
            RunOnUI(() => StatusMessage = _localizer.Get("Backups_Restored"));
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapBackupError(ex)));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    [RelayCommand]
    private async Task Delete(BackupItem? item) {
        if (item is null)
            return;
        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = _localizer.Get("Backups_Deleting");
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Backups.DeleteAsync(
                item.ToProto(), deadline: DateTime.UtcNow.AddSeconds(30));
            await LoadAsync();
            RunOnUI(() => StatusMessage = _localizer.Get("Backups_Deleted"));
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapBackupError(ex)));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    private static string MapBackupError(RpcException ex) =>
        ex.StatusCode switch {
            StatusCode.FailedPrecondition => "error.server_running",
            StatusCode.NotFound           => "error.backup_not_found",
            _                             => "error.backup_failed",
        };

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
