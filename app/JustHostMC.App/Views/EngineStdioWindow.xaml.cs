using System.Collections.ObjectModel;
using JustHostMC.App.Services;
using JustHostMC.Core;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Controls.Primitives;
using Windows.ApplicationModel.DataTransfer;
using Windows.Graphics;

namespace JustHostMC.App.Views;

/// <summary>Displays the bounded, live standard-stream history of the engine process.</summary>
public sealed partial class EngineStdioWindow : Window
{
    private const int MaxLocalEntries = 5000;

    private readonly EngineHost _host;
    private readonly Window? _owner;
    private readonly ILocalizer _localizer = new LocalizationService();
    private readonly List<EngineStdioEntry> _allEntries = [];
    private readonly HashSet<long> _seenSequences = [];
    private bool _isInitialized;
    private bool _isPaused;
    private long _ignoreThroughSequence;

    public ObservableCollection<EngineStdioDisplayEntry> VisibleEntries { get; } = [];

    public EngineStdioWindow(EngineHost host)
    {
        _host = host;
        InitializeComponent();
        _isInitialized = true;

        var title = _localizer.Get("EngineMonitor_Title");
        Title = title;
        MonitorTitleBar.Title = title;
        ExtendsContentIntoTitleBar = true;
        AppWindow.Resize(new SizeInt32(1040, 700));

        _owner = App.Current.MainWindow;
        if (_owner is not null)
            _owner.Closed += OnOwnerClosed;
        Closed += OnClosed;

        // Subscribe before taking the snapshot. Sequence de-duplication closes the
        // race where a line is both queued by the event and present in the snapshot.
        _host.StdioReceived += OnStdioReceived;
        foreach (var entry in _host.GetStdioSnapshot())
            AppendEntry(entry);
        UpdateStatus();
    }

    private void OnStdioReceived(object? sender, EngineStdioEntry entry)
        => DispatcherQueue.TryEnqueue(() => AppendEntry(entry));

    private void AppendEntry(EngineStdioEntry entry)
    {
        if (entry.Sequence <= _ignoreThroughSequence || !_seenSequences.Add(entry.Sequence))
            return;

        _allEntries.Add(entry);
        if (_allEntries.Count > MaxLocalEntries)
        {
            var removed = _allEntries[0];
            _allEntries.RemoveAt(0);
            _seenSequences.Remove(removed.Sequence);
        }

        if (!_isPaused && MatchesFilter(entry))
        {
            VisibleEntries.Add(new EngineStdioDisplayEntry(entry));
            if (AutoScrollCheckBox.IsChecked == true)
                LogListView.ScrollIntoView(VisibleEntries[^1]);
        }

        UpdateStatus();
    }

    private bool MatchesFilter(EngineStdioEntry entry)
    {
        var streamVisible = entry.Stream switch
        {
            EngineStdioStream.StdOut => StdOutCheckBox.IsChecked == true,
            EngineStdioStream.StdErr => StdErrCheckBox.IsChecked == true,
            EngineStdioStream.StdIn => StdInCheckBox.IsChecked == true,
            _ => false,
        };
        if (!streamVisible)
            return false;

        var search = SearchBox.Text?.Trim();
        return string.IsNullOrEmpty(search)
            || entry.Message.Contains(search, StringComparison.CurrentCultureIgnoreCase);
    }

    private void RefreshVisibleEntries()
    {
        VisibleEntries.Clear();
        if (!_isPaused)
        {
            foreach (var entry in _allEntries.Where(MatchesFilter))
                VisibleEntries.Add(new EngineStdioDisplayEntry(entry));
        }

        if (VisibleEntries.Count > 0 && AutoScrollCheckBox.IsChecked == true)
            LogListView.ScrollIntoView(VisibleEntries[^1]);
        UpdateStatus();
    }

    private void OnFilterChanged(object sender, RoutedEventArgs e)
    {
        if (_isInitialized && !_isPaused)
            RefreshVisibleEntries();
    }

    private void OnSearchTextChanged(object sender, TextChangedEventArgs e)
    {
        if (_isInitialized && !_isPaused)
            RefreshVisibleEntries();
    }

    private void OnPauseChanged(object sender, RoutedEventArgs e)
    {
        _isPaused = sender is ToggleButton { IsChecked: true };
        if (_isPaused)
            UpdateStatus();
        else
            RefreshVisibleEntries();
    }

    private void OnClearClick(object sender, RoutedEventArgs e)
    {
        _ignoreThroughSequence = _host.LastStdioSequence;
        _host.ClearStdioHistory();
        _allEntries.Clear();
        _seenSequences.Clear();
        VisibleEntries.Clear();
        UpdateStatus();
    }

    private void OnCopyClick(object sender, RoutedEventArgs e)
    {
        if (VisibleEntries.Count == 0)
            return;

        var package = new DataPackage();
        package.SetText(string.Join(Environment.NewLine, VisibleEntries.Select(entry => entry.CopyText)));
        Clipboard.SetContent(package);
    }

    private void UpdateStatus()
    {
        EmptyText.Visibility = VisibleEntries.Count == 0
            ? Visibility.Visible
            : Visibility.Collapsed;
        var pid = _host.ProcessId?.ToString() ?? "—";
        StatusText.Text = _localizer.Get(
            _isPaused ? "EngineMonitor_StatusPaused" : "EngineMonitor_Status",
            ("pid", pid),
            ("visible", VisibleEntries.Count.ToString()),
            ("total", _allEntries.Count.ToString()));
    }

    private void OnOwnerClosed(object sender, WindowEventArgs args) => Close();

    private void OnClosed(object sender, WindowEventArgs args)
    {
        _host.StdioReceived -= OnStdioReceived;
        if (_owner is not null)
            _owner.Closed -= OnOwnerClosed;
    }
}

/// <summary>Presentation wrapper for a captured engine stdio entry.</summary>
public sealed class EngineStdioDisplayEntry
{
    private readonly EngineStdioEntry _entry;

    public EngineStdioDisplayEntry(EngineStdioEntry entry) => _entry = entry;

    public string StreamLabel => _entry.Stream switch
    {
        EngineStdioStream.StdOut => "STDOUT",
        EngineStdioStream.StdErr => "STDERR",
        EngineStdioStream.StdIn => "STDIN",
        _ => "STDIO",
    };

    public string TimestampText => _entry.Timestamp.ToLocalTime().ToString("HH:mm:ss.fff");
    public string Message => _entry.Message;
    public string CopyText => $"{StreamLabel} | {TimestampText} | {Message}";
}
