using System.Collections.ObjectModel;
using JustHostMC.App.Services;
using JustHostMC.Core;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Controls.Primitives;
using Windows.ApplicationModel.DataTransfer;
using Windows.Graphics;

namespace JustHostMC.App.Views;

/// <summary>Displays the bounded, live standard-stream history of the engine
/// process.</summary>
public sealed partial class EngineStdioWindow : Window {
    private const int MaxLocalEntries = 5000;

    private readonly EngineHost _host;
    private readonly Window? _owner;
    private readonly ILocalizer _localizer = new LocalizationService();
    private readonly List<EngineStdioEntry> _allEntries = [];
    private readonly HashSet<long> _seenSequences       = [];
    private FilterGroup<EngineStdioStream> _streamFilters =
        FilterGroup<EngineStdioStream>.Empty;
    private FilterGroup<EngineDiagnosticLevel> _levelFilters =
        FilterGroup<EngineDiagnosticLevel>.Empty;
    private bool _isInitialized;
    private bool _isPaused;
    private bool _updatingFilterControls;
    private string _messageFilter = "";
    private long _ignoreThroughSequence;

    public ObservableCollection<EngineStdioDisplayEntry> VisibleEntries {
        get;
    } = [];

    public EngineStdioWindow(EngineHost host) {
        _host = host;
        InitializeComponent();
        InitializeFilterGroups();
        _isInitialized = true;

        ExtendsContentIntoTitleBar = true;
        AppWindow.Resize(new SizeInt32(1040, 700));

        _owner = App.Current.MainWindow;
        if (_owner is not null)
            _owner.Closed += OnOwnerClosed;
        Closed += OnClosed;

        // Subscribe before taking the snapshot. Sequence de-duplication closes
        // the race where a line is both queued by the event and present in the
        // snapshot.
        _host.StdioReceived += OnStdioReceived;
        foreach (var entry in _host.GetStdioSnapshot()) AppendEntry(entry);
        UpdateStatus();
    }

    private void OnStdioReceived(object? sender, EngineStdioEntry entry) =>
        DispatcherQueue.TryEnqueue(() => AppendEntry(entry));

    private void AppendEntry(EngineStdioEntry entry) {
        if (entry.Sequence <= _ignoreThroughSequence ||
            !_seenSequences.Add(entry.Sequence))
            return;

        _allEntries.Add(entry);
        if (_allEntries.Count > MaxLocalEntries) {
            var removed = _allEntries[0];
            _allEntries.RemoveAt(0);
            _seenSequences.Remove(removed.Sequence);
        }

        if (!_isPaused && MatchesFilter(entry)) {
            VisibleEntries.Add(new EngineStdioDisplayEntry(entry));
        }

        UpdateStatus();
    }

    private bool MatchesFilter(EngineStdioEntry entry) {
        if (!_streamFilters.Allows(entry.Stream))
            return false;

        if (!_levelFilters.Allows(entry.Level))
            return false;

        return _messageFilter.Length == 0 ||
               entry.DisplayMessage.Contains(
                   _messageFilter, StringComparison.CurrentCultureIgnoreCase);
    }

    private void RefreshVisibleEntries() {
        VisibleEntries.Clear();
        if (!_isPaused) {
            foreach (var entry in _allEntries.Where(MatchesFilter))
                VisibleEntries.Add(new EngineStdioDisplayEntry(entry));
        }

        UpdateStatus();
    }

    private void OnFilterChanged(object sender, RoutedEventArgs e) {
        if (!_isInitialized || _updatingFilterControls ||
            sender is not CheckBox checkBox)
            return;

        if (!_streamFilters.TryUpdate(checkBox) &&
            !_levelFilters.TryUpdate(checkBox))
            return;

        SyncFilterControls();
        if (!_isPaused)
            RefreshVisibleEntries();
    }

    private void OnSearchTextChanged(object sender, TextChangedEventArgs e) {
        if (!_isInitialized || _updatingFilterControls ||
            sender is not TextBox textBox)
            return;

        _messageFilter = textBox.Text.Trim();
        SyncFilterControls();
        if (!_isPaused)
            RefreshVisibleEntries();
    }

    private void OnCompactFilterOpening(object sender,
                                        object e) => SyncFilterControls();

    private void InitializeFilterGroups() {
        _streamFilters = new FilterGroup<EngineStdioStream>(
            [
                EngineStdioStream.StdOut,
                EngineStdioStream.StdErr,
                EngineStdioStream.StdIn,
            ],
            [
                new(EngineStdioStream.StdOut, WideStdOutCheckBox,
                    CompactStdOutCheckBox),
                new(EngineStdioStream.StdErr, WideStdErrCheckBox,
                    CompactStdErrCheckBox),
                new(EngineStdioStream.StdIn, WideStdInCheckBox,
                    CompactStdInCheckBox),
            ]);

        _levelFilters = new FilterGroup<EngineDiagnosticLevel>(
            [
                EngineDiagnosticLevel.Debug,
                EngineDiagnosticLevel.Information,
                EngineDiagnosticLevel.Warning,
                EngineDiagnosticLevel.Error,
            ],
            [
                new(EngineDiagnosticLevel.Debug, WideDebugCheckBox,
                    CompactDebugCheckBox),
                new(EngineDiagnosticLevel.Information, WideInfoCheckBox,
                    CompactInfoCheckBox),
                new(EngineDiagnosticLevel.Warning, WideWarningCheckBox,
                    CompactWarningCheckBox),
                new(EngineDiagnosticLevel.Error, WideErrorCheckBox,
                    CompactErrorCheckBox),
            ]);
    }

    private void SyncFilterControls() {
        _updatingFilterControls = true;
        try {
            _streamFilters.SyncControls();
            _levelFilters.SyncControls();
            WideSearchBox.Text = CompactSearchBox.Text = _messageFilter;
        } finally {
            _updatingFilterControls = false;
        }
    }

    private void OnPauseChanged(object sender, RoutedEventArgs e) {
        _isPaused = sender is ToggleButton { IsChecked : true };
        if (_isPaused)
            UpdateStatus();
        else
            RefreshVisibleEntries();
    }

    private void OnClearClick(object sender, RoutedEventArgs e) {
        _ignoreThroughSequence = _host.LastStdioSequence;
        _host.ClearStdioHistory();
        _allEntries.Clear();
        _seenSequences.Clear();
        VisibleEntries.Clear();
        UpdateStatus();
    }

    private void OnCopyClick(object sender, RoutedEventArgs e) {
        if (VisibleEntries.Count == 0)
            return;

        var package = new DataPackage();
        package.SetText(
            string.Join(Environment.NewLine,
                        VisibleEntries.Select(entry => entry.CopyText)));
        Clipboard.SetContent(package);
    }

    private void UpdateStatus() {
        var pid         = _host.ProcessId?.ToString() ?? "—";
        StatusText.Text = _localizer.Get(
            _isPaused ? "EngineMonitor_StatusPaused" : "EngineMonitor_Status",
            ("pid", pid), ("visible", VisibleEntries.Count.ToString()),
            ("total", _allEntries.Count.ToString()));
    }

    private void OnOwnerClosed(object sender, WindowEventArgs args) => Close();

    private void OnClosed(object sender, WindowEventArgs args) {
        _host.StdioReceived -= OnStdioReceived;
        if (_owner is not null)
            _owner.Closed -= OnOwnerClosed;
    }

    /// <summary>Connects a domain filter value to every checkbox that toggles
    /// it.</summary>
    private sealed class FilterControlBinding<T>
        where T : notnull {
        public FilterControlBinding(T value, params CheckBox[] controls) {
            Value    = value;
            Controls = controls;
        }

        public T Value { get; }
        public IReadOnlyList<CheckBox> Controls { get; }
    }

    /// <summary>Tracks one monitor filter group and keeps duplicate UI controls
    /// in sync.</summary>
    private sealed class FilterGroup<T>
        where T : notnull {
        private readonly HashSet<T> _visibleValues;
        private readonly IReadOnlyList<FilterControlBinding<T>>
            _controlBindings;

        public FilterGroup(
            IEnumerable<T> visibleValues,
            IReadOnlyList<FilterControlBinding<T>> controlBindings) {
            _visibleValues   = new HashSet<T>(visibleValues);
            _controlBindings = controlBindings;

            foreach (var binding in _controlBindings) {
                foreach (var control in binding.Controls)
                    control.Tag = binding.Value;
            }
        }

        public static FilterGroup<T> Empty {
            get;
        } = new(Array.Empty<T>(), Array.Empty<FilterControlBinding<T>>());

        public bool Allows(T value) => _visibleValues.Contains(value);

        public bool TryUpdate(CheckBox control) {
            if (control.Tag is not T value)
                return false;

            if (control.IsChecked == true)
                _visibleValues.Add(value);
            else
                _visibleValues.Remove(value);

            return true;
        }

        public void SyncControls() {
            foreach (var binding in _controlBindings) {
                var isVisible = _visibleValues.Contains(binding.Value);
                foreach (var control in binding.Controls)
                    control.IsChecked = isVisible;
            }
        }
    }
}

/// <summary>Presentation wrapper for a captured engine stdio entry.</summary>
public sealed class EngineStdioDisplayEntry {
    private readonly EngineStdioEntry _entry;

    public EngineStdioDisplayEntry(EngineStdioEntry entry) {
        _entry  = entry;
        Message = entry.DisplayMessage;
    }

    public string StreamLabel => _entry.Stream switch {
        EngineStdioStream.StdOut => "STDOUT",
        EngineStdioStream.StdErr => "STDERR",
        EngineStdioStream.StdIn  => "STDIN",
        _                        => "STDIO",
    };

    public string LevelLabel => _entry.Level switch {
        EngineDiagnosticLevel.Debug       => "DEBUG",
        EngineDiagnosticLevel.Information => "INFO",
        EngineDiagnosticLevel.Warning     => "WARN",
        EngineDiagnosticLevel.Error       => "ERROR",
        _                                 => "INFO",
    };

    public string TimestampText =>
        _entry.Timestamp.ToLocalTime().ToString("HH:mm:ss.fff");
    public string Message { get; }
    public string CopyText =>
        $"{TimestampText} | {LevelLabel} | {StreamLabel} | {Message}";
}
