using System.Diagnostics;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Windows.Storage.Pickers;

namespace JustHostMC.App.Views;

/// <summary>Modal backup manager for one server: list, create, restore,
/// delete.</summary>
public sealed partial class BackupsDialog : UserControl {
    private readonly string _serverId;
    private readonly string _serverName;

    public BackupsViewModel ViewModel { get; }

    public BackupsDialog(string serverId, string serverName, bool serverRunning,
                         DispatcherQueue dispatcher) {
        _serverId   = serverId;
        _serverName = serverName;
        ViewModel   = new BackupsViewModel(serverId, serverRunning, dispatcher,
                                           new LocalizationService());
        InitializeComponent();
    }

    public System.Threading.Tasks.Task LoadAsync() => ViewModel.LoadAsync();

    /// <summary>Helper for x:Bind to enable controls only when not
    /// busy.</summary>
    public bool Not(bool value) => !value;

    public Visibility ShowWhenEmpty(int count) =>
        count == 0 ? Visibility.Visible : Visibility.Collapsed;

    private async void OnExportBackupClick(object sender, RoutedEventArgs e) {
        if (ViewModel.IsBusy)
            return;

        if (GetBackupItem(sender) is not {} item)
            return;

        var source = FindBackupFile(item);
        if (source is null) {
            ViewModel.ReportExportSourceMissing();
            return;
        }

        var picker = new FileSavePicker {
            SuggestedFileName      = SuggestedBackupFileName(item),
            SuggestedStartLocation = PickerLocationId.DocumentsLibrary,
        };
        picker.FileTypeChoices.Add("Zip archive", new List<string> { ".zip" });
        var hwnd =
            WinRT.Interop.WindowNative.GetWindowHandle(App.Current.MainWindow);
        WinRT.Interop.InitializeWithWindow.Initialize(picker, hwnd);

        var file = await picker.PickSaveFileAsync();
        if (file is null)
            return;

        try {
            File.Copy(source, file.Path, overwrite: true);
            ViewModel.ReportExported(file.Path);
        } catch (Exception ex)
            when (ex is IOException or UnauthorizedAccessException) {
            ViewModel.ReportExportFailed();
        }
    }

    private void OnRestoreBackupConfirmClick(object sender, RoutedEventArgs e) {
        if (GetBackupItem(sender) is not {} item)
            return;

        if (ViewModel.IsBusy)
            return;

        if (!ViewModel.CanRestore) {
            ViewModel.ReportRestoreBlocked();
            return;
        }

        if (ViewModel.RestoreCommand.CanExecute(item))
            ViewModel.RestoreCommand.Execute(item);
    }

    private void OnDeleteBackupConfirmClick(object sender, RoutedEventArgs e) {
        if (GetBackupItem(sender) is not {} item || !ViewModel.CanRunActions)
            return;

        if (ViewModel.DeleteCommand.CanExecute(item))
            ViewModel.DeleteCommand.Execute(item);
    }

    private static BackupItem? GetBackupItem(object sender) =>
        sender is FrameworkElement element
            ? element.Tag as BackupItem ?? element.DataContext as BackupItem
            : null;

    private void OnOpenBackupFolderClick(object sender, RoutedEventArgs e) {
        var folder = BackupRoots().FirstOrDefault(Directory.Exists);
        if (folder is null) {
            ViewModel.ReportFolderMissing();
            return;
        }

        Process.Start(new ProcessStartInfo {
            FileName        = folder,
            UseShellExecute = true,
        });
    }

    private string? FindBackupFile(BackupItem item) {
        foreach (var root in BackupRoots().Where(Directory.Exists)) {
            var match =
                Directory.EnumerateFiles(root, "*", SearchOption.AllDirectories)
                    .FirstOrDefault(
                        path => Path.GetFileName(path).Contains(
                            item.Id, StringComparison.OrdinalIgnoreCase));
            if (match is not null)
                return match;
        }

        return null;
    }

    private string SuggestedBackupFileName(BackupItem item) {
        var fileName = $"{_serverName}-{item.Id}";
        foreach (var ch in Path.GetInvalidFileNameChars())
            fileName = fileName.Replace(ch, '-');
        return fileName;
    }

    private IEnumerable<string> BackupRoots() {
        foreach (var root in DataRoots()) {
            yield return Path.Combine(root, "backups");
            yield return Path.Combine(root, "servers", _serverId, "backups");
            yield return Path.Combine(root, "instances", _serverId, "backups");
            yield return Path.Combine(root, _serverId, "backups");
            yield return Path.Combine(root, "servers", _serverName, "backups");
            yield return Path.Combine(root, "instances", _serverName,
                                      "backups");
        }
    }

    private static IEnumerable<string> DataRoots() {
        var packaged = GetPackagedDataRoot();
        if (!string.IsNullOrWhiteSpace(packaged))
            yield return packaged;

        yield return Path.Combine(
            Environment.GetFolderPath(
                Environment.SpecialFolder.LocalApplicationData),
            "JustHostMC");
    }

    private static string? GetPackagedDataRoot() {
        try {
            return Windows.Storage.ApplicationData.Current.LocalFolder.Path;
        } catch {
            return null;
        }
    }
}
