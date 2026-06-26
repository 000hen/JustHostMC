using System;
using System.Collections.ObjectModel;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>Loads and saves server.properties plus world gamerules.</summary>
public sealed partial class ServerConfigViewModel : ObservableObject
{
    private readonly string _serverId;
    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;
    private bool _serverStopped;
    private bool _loaded;
    private Task? _refreshTask;

    public ServerConfigViewModel(string serverId, DispatcherQueue dispatcher, ILocalizer localizer)
    {
        _serverId = serverId;
        _dispatcher = dispatcher;
        _localizer = localizer;
    }

    public ObservableCollection<ConfigEntryItem> Properties { get; } = new();
    public ObservableCollection<ConfigEntryItem> GameRules { get; } = new();

    private bool _canModify;
    public bool CanModify
    {
        get => _canModify;
        private set
        {
            if (SetProperty(ref _canModify, value))
                OnPropertyChanged(nameof(CanSaveGameRules));
        }
    }

    private bool _gameRulesWorldExists;
    public bool GameRulesWorldExists
    {
        get => _gameRulesWorldExists;
        private set
        {
            if (SetProperty(ref _gameRulesWorldExists, value))
                OnPropertyChanged(nameof(CanSaveGameRules));
        }
    }

    public bool CanSaveGameRules => CanModify && GameRulesWorldExists;

    private bool _isBusy;
    public bool IsBusy
    {
        get => _isBusy;
        private set
        {
            if (SetProperty(ref _isBusy, value))
                OnPropertyChanged(nameof(IsInitialLoading));
        }
    }

    public bool IsInitialLoading => IsBusy && !_loaded;

    private string _statusMessage = "";
    public string StatusMessage
    {
        get => _statusMessage;
        private set => SetProperty(ref _statusMessage, value);
    }

    private string _gameRulesMessage = "";
    public string GameRulesMessage
    {
        get => _gameRulesMessage;
        private set => SetProperty(ref _gameRulesMessage, value);
    }

    public void SetServerStopped(bool stopped)
    {
        _serverStopped = stopped;
        CanModify = stopped;
    }

    public void PrepareInitialLoad()
    {
        if (_loaded)
            return;

        RunOnUI(() =>
        {
            IsBusy = true;
            StatusMessage = "";
        });
    }

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

    public async Task EnsureLoadedAsync()
    {
        if (_loaded)
            return;

        await RefreshAsync();
    }

    private async Task RefreshCoreAsync()
    {
        RunOnUI(() =>
        {
            IsBusy = true;
            StatusMessage = "";
        });
        await Task.Yield();

        try
        {
            var daemon = await App.Current.DaemonReady;
            var propsTask = daemon.Config.GetServerPropertiesAsync(new ServerId { Id = _serverId }).ResponseAsync;
            var rulesTask = daemon.Config.GetGameRulesAsync(new ServerId { Id = _serverId }).ResponseAsync;
            await Task.WhenAll(propsTask, rulesTask);
            RunOnUI(() =>
            {
                Replace(Properties, propsTask.Result.Entries, _localizer);
                Replace(GameRules, rulesTask.Result.Entries, _localizer);
                GameRulesWorldExists = rulesTask.Result.WorldExists;
                GameRulesMessage = rulesTask.Result.Message;
                _loaded = true;
                OnPropertyChanged(nameof(IsInitialLoading));
            });
        }
        catch (RpcException)
        {
            RunOnUI(() => StatusMessage = _localizer.Get("Config_LoadFailed"));
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    public async Task SavePropertiesAsync()
    {
        if (!_serverStopped)
            return;

        RunOnUI(() =>
        {
            IsBusy = true;
            StatusMessage = "";
        });
        try
        {
            var daemon = await App.Current.DaemonReady;
            var req = new UpdateServerPropertiesRequest { ServerId = _serverId };
            foreach (var item in Properties)
                req.Entries.Add(item.ToUpdate());
            var saved = await daemon.Config.UpdateServerPropertiesAsync(req);
            RunOnUI(() =>
            {
                Replace(Properties, saved.Entries, _localizer);
                StatusMessage = _localizer.Get("Config_Saved");
            });
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = ex.Status.Detail.Length > 0
                ? ex.Status.Detail
                : _localizer.Get("Config_SaveFailed"));
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    public async Task SaveGameRulesAsync()
    {
        if (!CanSaveGameRules)
            return;

        RunOnUI(() =>
        {
            IsBusy = true;
            StatusMessage = "";
        });
        try
        {
            var daemon = await App.Current.DaemonReady;
            var req = new UpdateGameRulesRequest { ServerId = _serverId };
            foreach (var item in GameRules)
                req.Entries.Add(item.ToUpdate());
            var saved = await daemon.Config.UpdateGameRulesAsync(req);
            RunOnUI(() =>
            {
                Replace(GameRules, saved.Entries, _localizer);
                GameRulesWorldExists = saved.WorldExists;
                GameRulesMessage = saved.Message;
                StatusMessage = _localizer.Get("Config_Saved");
            });
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = ex.Status.Detail.Length > 0
                ? ex.Status.Detail
                : _localizer.Get("Config_SaveFailed"));
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    private static void Replace(ObservableCollection<ConfigEntryItem> target,
        Google.Protobuf.Collections.RepeatedField<ConfigEntry> entries,
        ILocalizer localizer)
    {
        target.Clear();
        foreach (var entry in entries)
            target.Add(new ConfigEntryItem(entry, localizer));
    }

    private void RunOnUI(Action action)
    {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
