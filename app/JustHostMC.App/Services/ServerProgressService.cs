using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.Services;

/// <summary>
/// Tracks live download, install, and transitional state progress for a single server.
/// </summary>
public partial class ServerProgressTracker : ObservableObject
{
    private const int MaxLogLines = 2000;
    private const int MaxLogLineLength = 2000;

    private readonly DispatcherQueue _dispatcher;

    private string? _serverId;
    public string? ServerId
    {
        get => _serverId;
        set => SetProperty(ref _serverId, value);
    }

    private string _serverName = string.Empty;
    public string ServerName
    {
        get => _serverName;
        set => SetProperty(ref _serverName, value);
    }

    private bool _isActive;
    public bool IsActive
    {
        get => _isActive;
        set
        {
            if (SetProperty(ref _isActive, value))
            {
                OnPropertyChanged(nameof(NavigationText));
                IsActiveChanged?.Invoke(this);
            }
        }
    }

    private bool _isInstalling;
    public bool IsInstalling
    {
        get => _isInstalling;
        set => SetProperty(ref _isInstalling, value);
    }

    private string _currentStep = string.Empty;
    public string CurrentStep
    {
        get => _currentStep;
        set
        {
            if (SetProperty(ref _currentStep, value))
            {
                OnPropertyChanged(nameof(NavigationText));
            }
        }
    }

    private double _progressFraction;
    public double ProgressFraction
    {
        get => _progressFraction;
        set
        {
            if (SetProperty(ref _progressFraction, value))
            {
                OnPropertyChanged(nameof(NavigationText));
                OnPropertyChanged(nameof(ProgressPercentage));
            }
        }
    }

    private bool _isIndeterminate = true;
    public bool IsIndeterminate
    {
        get => _isIndeterminate;
        set => SetProperty(ref _isIndeterminate, value);
    }

    private bool _hasFailed;
    public bool HasFailed
    {
        get => _hasFailed;
        set => SetProperty(ref _hasFailed, value);
    }

    private bool _isReadyToRun;
    public bool IsReadyToRun
    {
        get => _isReadyToRun;
        set => SetProperty(ref _isReadyToRun, value);
    }

    public ObservableCollection<string> InstallLog { get; } = new();

    public double ProgressPercentage => ProgressFraction * 100;

    public string NavigationText
    {
        get
        {
            if (!IsActive)
                return "";
            if (ProgressFraction > 0 && ProgressFraction <= 1.0)
                return $"({Math.Round(ProgressFraction * 100)}%)";
            return string.IsNullOrEmpty(CurrentStep) ? "(…)" : $"({CurrentStep})";
        }
    }

    public ServerProgressTracker(DispatcherQueue dispatcher, string serverName)
    {
        _dispatcher = dispatcher;
        _serverName = serverName;
    }

    public void AppendLog(string line)
    {
        if (line.Length > MaxLogLineLength)
            line = line[..MaxLogLineLength] + "…";

        RunOnUI(() =>
        {
            InstallLog.Add(line);
            while (InstallLog.Count > MaxLogLines)
                InstallLog.RemoveAt(0);

            LogAppended?.Invoke(line);
        });
    }

    public event Action<string>? LogAppended;
    public event Action<ServerProgressTracker>? IsActiveChanged;

    [RelayCommand]
    public void Dismiss()
    {
        IsInstalling = false;
        IsActive = false;
        IsReadyToRun = false;
        HasFailed = false;
    }

    public void RunOnUI(Action action)
    {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}

/// <summary>
/// Centralized repository managing per-server progress trackers shared across pages and navigation menus.
/// </summary>
public sealed class ServerProgressService : ObservableObject
{
    private readonly DispatcherQueue _dispatcher;
    private readonly Dictionary<string, ServerProgressTracker> _trackers = new();
    private readonly object _gate = new();

    public ObservableCollection<ServerProgressTracker> ActiveTrackers { get; } = new();

    private ServerProgressTracker? _currentActiveTracker;
    public ServerProgressTracker? CurrentActiveTracker
    {
        get => _currentActiveTracker;
        private set
        {
            if (SetProperty(ref _currentActiveTracker, value))
            {
                OnPropertyChanged(nameof(HasActiveTracker));
            }
        }
    }

    public bool HasActiveTracker => CurrentActiveTracker is not null;

    public ServerProgressService(DispatcherQueue dispatcher)
    {
        _dispatcher = dispatcher;
    }

    public ServerProgressTracker GetOrCreateTracker(string? serverId, string serverName)
    {
        lock (_gate)
        {
            if (serverId is not null && _trackers.TryGetValue(serverId, out var trackerById))
            {
                trackerById.ServerName = serverName;
                return trackerById;
            }

            var keyByName = $"name:{serverName}";
            if (_trackers.TryGetValue(keyByName, out var trackerByName))
            {
                if (serverId is not null)
                {
                    trackerByName.ServerId = serverId;
                    _trackers[serverId] = trackerByName;
                }
                return trackerByName;
            }

            var newTracker = new ServerProgressTracker(_dispatcher, serverName);
            newTracker.IsActiveChanged += OnTrackerActiveChanged;
            if (serverId is not null)
            {
                newTracker.ServerId = serverId;
                _trackers[serverId] = newTracker;
            }
            _trackers[keyByName] = newTracker;

            if (_dispatcher.HasThreadAccess)
                ActiveTrackers.Add(newTracker);
            else
                _dispatcher.TryEnqueue(() => ActiveTrackers.Add(newTracker));

            return newTracker;
        }
    }

    private void OnTrackerActiveChanged(ServerProgressTracker tracker)
    {
        if (_dispatcher.HasThreadAccess)
            UpdateCurrentActiveTracker();
        else
            _dispatcher.TryEnqueue(UpdateCurrentActiveTracker);
    }

    private void UpdateCurrentActiveTracker()
    {
        lock (_gate)
        {
            var active = _trackers.Values.FirstOrDefault(t => t.IsActive);
            CurrentActiveTracker = active;
        }
    }
}
