using System.Collections.ObjectModel;
using System.Collections.Specialized;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using Microsoft.UI.Xaml;
using Windows.Graphics;

namespace JustHostMC.App.Views;

/// <summary>Displays the combined live output from all automation scripts.</summary>
public sealed partial class ScriptLogsWindow : Window
{
    private readonly Window? _owner;

    public ObservableCollection<ScriptLogEntry> LogEntries { get; }

    public ScriptLogsWindow(ObservableCollection<ScriptLogEntry> logEntries)
    {
        LogEntries = logEntries;
        InitializeComponent();
        Root.DataContext = this;

        var title = new LocalizationService().Get("ScriptLogsWindow_Title");
        Title = title;
        LogsTitleBar.Title = title;
        ExtendsContentIntoTitleBar = true;
        AppWindow.Resize(new SizeInt32(960, 640));

        _owner = App.Current.MainWindow;
        if (_owner is not null)
            _owner.Closed += OnOwnerClosed;
        Closed += OnClosed;

        LogEntries.CollectionChanged += OnLogEntriesChanged;
        if (LogEntries.Count > 0)
            LogList.SelectedIndex = 0;
    }

    private void OnOwnerClosed(object sender, WindowEventArgs args) => Close();

    private void OnClosed(object sender, WindowEventArgs args)
    {
        LogEntries.CollectionChanged -= OnLogEntriesChanged;
        if (_owner is not null)
            _owner.Closed -= OnOwnerClosed;
    }

    private void OnLogEntriesChanged(object? sender, NotifyCollectionChangedEventArgs e)
    {
        if (LogList.SelectedIndex < 0 && LogEntries.Count > 0)
            LogList.SelectedIndex = 0;
    }
}
