using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>Loads and saves server.properties plus world gamerules.</summary>
public sealed partial class ServerConfigViewModel : ObservableObject {
    private readonly string _serverId;
    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;
    private bool _serverStopped;
    private bool _loaded;
    private Task? _refreshTask;

    public ServerConfigViewModel(string serverId, DispatcherQueue dispatcher,
                                 ILocalizer localizer) {
        _serverId   = serverId;
        _dispatcher = dispatcher;
        _localizer  = localizer;
    }

    public ObservableCollection<ConfigEntryItem> Properties { get; } = new();
    public ObservableCollection<ConfigEntryItem> GameRules { get; }  = new();

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(CanSaveGameRules))]
    [NotifyPropertyChangedFor(nameof(CanSaveModifiedConfiguration))]
    public partial bool CanModify {
        get; private set;
    }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(CanSaveGameRules))]
    [NotifyPropertyChangedFor(nameof(CanSaveModifiedConfiguration))]
    public partial bool GameRulesWorldExists {
        get; private set;
    }

    public bool CanSaveGameRules   => CanModify && GameRulesWorldExists;
    public bool PropertiesModified => Properties.Any(item => item.IsModified);
    public bool GameRulesModified  => GameRules.Any(item => item.IsModified);
    public bool HasModifiedConfiguration =>
        PropertiesModified || GameRulesModified;
    public bool CanDiscardChanges => !IsBusy && HasModifiedConfiguration;
    public bool CanSaveModifiedConfiguration =>
        !IsBusy && CanModify &&
        (PropertiesModified || (GameRulesWorldExists && GameRulesModified));

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(IsInitialLoading))]
    [NotifyPropertyChangedFor(nameof(CanDiscardChanges))]
    [NotifyPropertyChangedFor(nameof(CanSaveModifiedConfiguration))]
    public partial bool IsBusy { get; private set; }

    public bool IsInitialLoading => IsBusy && !_loaded;

    [ObservableProperty]
    public partial string StatusMessage { get; private set; } = "";

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(HasGameRulesMessage))]
    public partial string GameRulesMessage {
        get; private set;
    } = "";

    public bool HasGameRulesMessage =>
        !string.IsNullOrWhiteSpace(GameRulesMessage);

    public void SetServerStopped(bool stopped) {
        _serverStopped = stopped;
        CanModify      = stopped;
    }

    public void PrepareInitialLoad() {
        if (_loaded)
            return;

        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = "";
        });
    }

    public async Task RefreshAsync() {
        if (_refreshTask is { IsCompleted : false }) {
            await _refreshTask;
            return;
        }

        _refreshTask = RefreshCoreAsync();
        await _refreshTask;
    }

    public async Task EnsureLoadedAsync() {
        if (_loaded)
            return;

        await RefreshAsync();
    }

    private async Task RefreshCoreAsync() {
        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = "";
        });
        await Task.Yield();

        try {
            var daemon = await App.Current.DaemonReady;
            var propsTask =
                daemon.Config
                    .GetServerPropertiesAsync(new ServerId { Id = _serverId })
                    .ResponseAsync;
            var rulesTask =
                daemon.Config.GetGameRulesAsync(new ServerId { Id = _serverId })
                    .ResponseAsync;
            await Task.WhenAll(propsTask, rulesTask);
            RunOnUI(() => {
                Replace(Properties, propsTask.Result.Entries, _localizer);
                Replace(GameRules, rulesTask.Result.Entries, _localizer);
                GameRulesWorldExists = rulesTask.Result.WorldExists;
                GameRulesMessage     = rulesTask.Result.Message;
                _loaded              = true;
                OnPropertyChanged(nameof(IsInitialLoading));
            });
        } catch (RpcException) {
            RunOnUI(() => StatusMessage = _localizer.Get("Config_LoadFailed"));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    public async Task SavePropertiesAsync() {
        if (!_serverStopped)
            return;

        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = "";
        });
        try {
            var daemon = await App.Current.DaemonReady;
            var req =
                new UpdateServerPropertiesRequest { ServerId = _serverId };
            foreach (var item in Properties) req.Entries.Add(item.ToUpdate());
            var saved = await daemon.Config.UpdateServerPropertiesAsync(req);
            RunOnUI(() => {
                Replace(Properties, saved.Entries, _localizer);
                StatusMessage = _localizer.Get("Config_Saved");
            });
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage =
                        ex.Status.Detail.Length > 0
                            ? ex.Status.Detail
                            : _localizer.Get("Config_SaveFailed"));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    public async Task SaveGameRulesAsync() {
        if (!CanSaveGameRules)
            return;

        RunOnUI(() => {
            IsBusy        = true;
            StatusMessage = "";
        });
        try {
            var daemon = await App.Current.DaemonReady;
            var req    = new UpdateGameRulesRequest { ServerId = _serverId };
            foreach (var item in GameRules) req.Entries.Add(item.ToUpdate());
            var saved = await daemon.Config.UpdateGameRulesAsync(req);
            RunOnUI(() => {
                Replace(GameRules, saved.Entries, _localizer);
                GameRulesWorldExists = saved.WorldExists;
                GameRulesMessage     = saved.Message;
                StatusMessage        = _localizer.Get("Config_Saved");
            });
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage =
                        ex.Status.Detail.Length > 0
                            ? ex.Status.Detail
                            : _localizer.Get("Config_SaveFailed"));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    public async Task SaveModifiedAsync() {
        var saveProperties = PropertiesModified;
        var saveGameRules  = GameRulesModified && CanSaveGameRules;

        if (saveProperties)
            await SavePropertiesAsync();
        if (saveGameRules)
            await SaveGameRulesAsync();
    }

    public void DiscardChanges() {
        foreach (var item in Properties.Where(item => item.IsModified))
            item.DiscardChanges();
        foreach (var item in GameRules.Where(item => item.IsModified))
            item.DiscardChanges();
        StatusMessage = "";
    }

    private void Replace(
        ObservableCollection<ConfigEntryItem> target,
        Google.Protobuf.Collections.RepeatedField<ConfigEntry> entries,
        ILocalizer localizer) {
        foreach (var item in target)
            item.PropertyChanged -= OnEntryPropertyChanged;
        target.Clear();
        foreach (var entry in entries) {
            var item = new ConfigEntryItem(entry, localizer);
            item.PropertyChanged += OnEntryPropertyChanged;
            target.Add(item);
        }
        NotifyModifiedStateChanged();
    }

    private void OnEntryPropertyChanged(object? sender,
                                        PropertyChangedEventArgs e) {
        if (e.PropertyName == nameof(ConfigEntryItem.IsModified))
            NotifyModifiedStateChanged();
    }

    private void NotifyModifiedStateChanged() {
        OnPropertyChanged(nameof(PropertiesModified));
        OnPropertyChanged(nameof(GameRulesModified));
        OnPropertyChanged(nameof(HasModifiedConfiguration));
        OnPropertyChanged(nameof(CanDiscardChanges));
        OnPropertyChanged(nameof(CanSaveModifiedConfiguration));
    }

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
