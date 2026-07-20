using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

public enum BackupStatus {
    None,
    Creating,
    Created,
    Restoring,
    Restored,
    Deleting,
    Deleted,
    ExportSourceMissing,
    Exported,
    ExportFailed,
    RestoreBlocked,
    FolderMissing,
    Error,
}

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
    [NotifyPropertyChangedFor(nameof(IsCreatingStatus))]
    [NotifyPropertyChangedFor(nameof(IsCreatedStatus))]
    [NotifyPropertyChangedFor(nameof(IsRestoringStatus))]
    [NotifyPropertyChangedFor(nameof(IsRestoredStatus))]
    [NotifyPropertyChangedFor(nameof(IsDeletingStatus))]
    [NotifyPropertyChangedFor(nameof(IsDeletedStatus))]
    [NotifyPropertyChangedFor(nameof(IsExportSourceMissingStatus))]
    [NotifyPropertyChangedFor(nameof(IsExportedStatus))]
    [NotifyPropertyChangedFor(nameof(IsExportFailedStatus))]
    [NotifyPropertyChangedFor(nameof(IsRestoreBlockedStatus))]
    [NotifyPropertyChangedFor(nameof(IsFolderMissingStatus))]
    [NotifyPropertyChangedFor(nameof(IsErrorStatus))]
    public partial BackupStatus Status {
        get; private set;
    }

    [ObservableProperty]
    public partial string ErrorMessage {
        get; private set;
    } = "";

    [ObservableProperty]
    public partial string ExportPath {
        get; private set;
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

    public bool IsCreatingStatus  => Status == BackupStatus.Creating;
    public bool IsCreatedStatus   => Status == BackupStatus.Created;
    public bool IsRestoringStatus => Status == BackupStatus.Restoring;
    public bool IsRestoredStatus  => Status == BackupStatus.Restored;
    public bool IsDeletingStatus  => Status == BackupStatus.Deleting;
    public bool IsDeletedStatus   => Status == BackupStatus.Deleted;
    public bool IsExportSourceMissingStatus =>
        Status == BackupStatus.ExportSourceMissing;
    public bool IsExportedStatus       => Status == BackupStatus.Exported;
    public bool IsExportFailedStatus   => Status == BackupStatus.ExportFailed;
    public bool IsRestoreBlockedStatus => Status == BackupStatus.RestoreBlocked;
    public bool IsFolderMissingStatus  => Status == BackupStatus.FolderMissing;
    public bool IsErrorStatus          => Status == BackupStatus.Error;

    public void ReportExportSourceMissing() =>
        SetStatus(BackupStatus.ExportSourceMissing);

    public void ReportExported(string path) {
        ExportPath = path;
        SetStatus(BackupStatus.Exported);
    }

    public void ReportExportFailed() => SetStatus(BackupStatus.ExportFailed);

    public void ReportRestoreBlocked() =>
        SetStatus(BackupStatus.RestoreBlocked);

    public void ReportFolderMissing() => SetStatus(BackupStatus.FolderMissing);

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
            RunOnUI(() => SetError(ex));
        }
    }

    [RelayCommand]
    private async Task CreateBackup() {
        using var backgroundTask =
            App.Current.BackgroundTasks.Begin("backup-create");
        RunOnUI(() => {
            IsBusy = true;
            SetStatus(BackupStatus.Creating);
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Backups.CreateAsync(
                new CreateBackupRequest { ServerId   = _serverId,
                                          SafeOnline = SafeOnline },
                deadline: DateTime.UtcNow.AddMinutes(10));
            await LoadAsync();
            RunOnUI(() => SetStatus(BackupStatus.Created));
        } catch (RpcException ex) {
            RunOnUI(() => SetError(ex));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    [RelayCommand]
    private async Task Restore(BackupItem? item) {
        if (item is null)
            return;
        using var backgroundTask =
            App.Current.BackgroundTasks.Begin("backup-restore");
        RunOnUI(() => {
            IsBusy = true;
            SetStatus(BackupStatus.Restoring);
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Backups.RestoreAsync(
                new RestoreBackupRequest { ServerId = _serverId,
                                           BackupId = item.Id },
                deadline: DateTime.UtcNow.AddMinutes(10));
            RunOnUI(() => SetStatus(BackupStatus.Restored));
        } catch (RpcException ex) {
            RunOnUI(() => SetError(ex));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    [RelayCommand]
    private async Task Delete(BackupItem? item) {
        if (item is null)
            return;
        using var backgroundTask =
            App.Current.BackgroundTasks.Begin("backup-delete");
        RunOnUI(() => {
            IsBusy = true;
            SetStatus(BackupStatus.Deleting);
        });
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Backups.DeleteAsync(
                item.ToProto(), deadline: DateTime.UtcNow.AddSeconds(30));
            await LoadAsync();
            RunOnUI(() => SetStatus(BackupStatus.Deleted));
        } catch (RpcException ex) {
            RunOnUI(() => SetError(ex));
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

    private void SetStatus(BackupStatus status) {
        ErrorMessage = "";
        if (status != BackupStatus.Exported)
            ExportPath = "";
        Status = status;
    }

    private void SetError(RpcException ex) {
        ErrorMessage = _localizer.Get(MapBackupError(ex));
        Status       = BackupStatus.Error;
    }

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
