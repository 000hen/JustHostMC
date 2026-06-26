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
        set
        {
            if (SetProperty(ref _serverName, value))
            {
                OnPropertyChanged(nameof(TooltipText));
            }
        }
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
                OnPropertyChanged(nameof(TooltipText));
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
                OnPropertyChanged(nameof(TooltipText));
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
        set
        {
            if (SetProperty(ref _hasFailed, value))
            {
                OnPropertyChanged(nameof(TooltipText));
            }
        }
    }

    private bool _isReadyToRun;
    public bool IsReadyToRun
    {
        get => _isReadyToRun;
        set
        {
            if (SetProperty(ref _isReadyToRun, value))
            {
                OnPropertyChanged(nameof(TooltipText));
            }
        }
    }

    public ObservableCollection<string> InstallLog { get; } = new();

    public double ProgressPercentage => ProgressFraction * 100;

    public string TooltipText
    {
        get
        {
            if (IsReadyToRun || IsActive || HasFailed)
                return string.IsNullOrEmpty(CurrentStep) ? ServerName : $"{ServerName}: {CurrentStep}";
            return ServerName;
        }
    }

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
public sealed partial class ServerProgressService : ObservableObject
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
                UpdatePaginationProps();
            }
        }
    }

    public bool HasActiveTracker => CurrentActiveTracker is not null;

    public int ActiveCount => ActiveTrackers.Count;
    public bool HasMultipleActive => ActiveTrackers.Count > 1;
    public string ActiveIndexText
    {
        get
        {
            if (ActiveTrackers.Count == 0 || CurrentActiveTracker == null)
                return "";
            var index = ActiveTrackers.IndexOf(CurrentActiveTracker) + 1;
            return $"({index}/{ActiveTrackers.Count})";
        }
    }

    public bool CanGoPrevious => ActiveTrackers.Count > 1 && CurrentActiveTracker != null && ActiveTrackers.IndexOf(CurrentActiveTracker) > 0;
    public bool CanGoNext => ActiveTrackers.Count > 1 && CurrentActiveTracker != null && ActiveTrackers.IndexOf(CurrentActiveTracker) < ActiveTrackers.Count - 1;

    [RelayCommand(CanExecute = nameof(CanGoPrevious))]
    public void Previous()
    {
        if (CurrentActiveTracker == null) return;
        var index = ActiveTrackers.IndexOf(CurrentActiveTracker);
        if (index > 0)
        {
            CurrentActiveTracker = ActiveTrackers[index - 1];
            UpdatePaginationProps();
        }
    }

    [RelayCommand(CanExecute = nameof(CanGoNext))]
    public void Next()
    {
        if (CurrentActiveTracker == null) return;
        var index = ActiveTrackers.IndexOf(CurrentActiveTracker);
        if (index >= 0 && index < ActiveTrackers.Count - 1)
        {
            CurrentActiveTracker = ActiveTrackers[index + 1];
            UpdatePaginationProps();
        }
    }

    private void UpdatePaginationProps()
    {
        OnPropertyChanged(nameof(ActiveCount));
        OnPropertyChanged(nameof(HasMultipleActive));
        OnPropertyChanged(nameof(ActiveIndexText));
        OnPropertyChanged(nameof(CanGoPrevious));
        OnPropertyChanged(nameof(CanGoNext));
        PreviousCommand.NotifyCanExecuteChanged();
        NextCommand.NotifyCanExecuteChanged();
    }

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

            if (newTracker.IsActive)
            {
                if (_dispatcher.HasThreadAccess)
                    ActiveTrackers.Add(newTracker);
                else
                    _dispatcher.TryEnqueue(() => ActiveTrackers.Add(newTracker));
            }

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
            var activeList = _trackers.Values.Where(t => t.IsActive).Distinct().ToList();
            RunOnUI(() =>
            {
                for (int i = ActiveTrackers.Count - 1; i >= 0; i--)
                {
                    if (!activeList.Contains(ActiveTrackers[i]))
                        ActiveTrackers.RemoveAt(i);
                }
                foreach (var t in activeList)
                {
                    if (!ActiveTrackers.Contains(t))
                        ActiveTrackers.Add(t);
                }

                if (CurrentActiveTracker == null || !ActiveTrackers.Contains(CurrentActiveTracker))
                {
                    CurrentActiveTracker = ActiveTrackers.FirstOrDefault();
                }
                else
                {
                    UpdatePaginationProps();
                }
            });
        }
    }

    private void RunOnUI(Action action)
    {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }
}
