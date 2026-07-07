using System.Collections.ObjectModel;
using System.Collections.Specialized;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using Microsoft.UI.Xaml;
using Windows.Graphics;

namespace JustHostMC.App.Views;

/// <summary>Displays automation output grouped by application
/// session.</summary>
public sealed partial class ScriptLogsWindow : Window {
    private readonly Window? _owner;

    public ObservableCollection<ScriptLogSession> LogSessions { get; }

    public ScriptLogsWindow(
        ObservableCollection<ScriptLogSession> logSessions) {
        LogSessions = logSessions;
        InitializeComponent();
        Root.DataContext = this;

        var title = new LocalizationService().Get("ScriptLogsWindow_Title");
        Title     = title;
        LogsTitleBar.Title         = title;
        ExtendsContentIntoTitleBar = true;
        AppWindow.Resize(new SizeInt32(960, 640));

        _owner = App.Current.MainWindow;
        if (_owner is not null)
            _owner.Closed += OnOwnerClosed;
        Closed += OnClosed;

        LogSessions.CollectionChanged += OnLogSessionsChanged;
        if (LogSessions.Count > 0)
            LogList.SelectedIndex = 0;
    }

    private void OnOwnerClosed(object sender, WindowEventArgs args) => Close();

    private void OnClosed(object sender, WindowEventArgs args) {
        LogSessions.CollectionChanged -= OnLogSessionsChanged;
        if (_owner is not null)
            _owner.Closed -= OnOwnerClosed;
    }

    private void OnLogSessionsChanged(object? sender,
                                      NotifyCollectionChangedEventArgs e) {
        if (LogList.SelectedIndex < 0 && LogSessions.Count > 0)
            LogList.SelectedIndex = 0;
    }
}
