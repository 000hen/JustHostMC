using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using JustHostMC.App.Models;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>Edits server fields backed by ServerService.Update.</summary>
public sealed partial class EditServerDialog : UserControl
{
    private readonly MainViewModel _viewModel;
    private readonly ServerItem _server;
    private bool _isLoadingVersions;

    public bool CanSubmit { get; private set; }
    public event EventHandler? CanSubmitChanged;

    public EditServerDialog(MainViewModel viewModel, ServerItem server)
    {
        _viewModel = viewModel;
        _server = server;
        InitializeComponent();

        NameBox.Text = server.Name;
        TypeBox.Text = server.TypeText;
        VersionBox.ItemsSource = new[] { server.McVersion };
        VersionBox.SelectedIndex = 0;
        MemoryBox.Value = server.MemoryMb > 0 ? server.MemoryMb : 2048;
        CustomJavaArgsBox.Text = server.CustomJavaArgs;
        PortBox.Value = server.Port;

        VersionBox.IsEnabled = server.CanEditLaunchSettings;
        MemoryBox.IsEnabled = server.CanEditLaunchSettings;
        CustomJavaArgsBox.IsEnabled = server.CanEditLaunchSettings;
        PortBox.IsEnabled = server.CanEditLaunchSettings;
        LaunchLockedInfo.IsOpen = !server.CanEditLaunchSettings;

        UpdatePrimaryButtonState();
        _ = LoadVersionsAsync();
    }

    private void OnNameChanged(object sender, TextChangedEventArgs e) => UpdatePrimaryButtonState();

    private void OnVersionSelectionChanged(object sender, SelectionChangedEventArgs e) => UpdatePrimaryButtonState();

    private void OnMemoryChanged(NumberBox sender, NumberBoxValueChangedEventArgs args) => UpdatePrimaryButtonState();

    private void OnCustomJavaArgsChanged(object sender, TextChangedEventArgs e) => UpdatePrimaryButtonState();

    private void OnPortChanged(NumberBox sender, NumberBoxValueChangedEventArgs args) => UpdatePrimaryButtonState();

    private async Task LoadVersionsAsync()
    {
        if (!_server.CanEditLaunchSettings)
        {
            VersionLoading.IsActive = false;
            VersionLoading.Visibility = Visibility.Collapsed;
            return;
        }

        _isLoadingVersions = true;
        VersionErrorBar.IsOpen = false;
        VersionLoading.IsActive = true;
        VersionLoading.Visibility = Visibility.Visible;
        UpdatePrimaryButtonState();

        try
        {
            var versions = (await _viewModel.GetVersionsAsync(_server.ProviderId)).ToList();
            if (!versions.Contains(_server.McVersion))
                versions.Insert(0, _server.McVersion);
            VersionBox.ItemsSource = versions;
            VersionBox.SelectedItem = _server.McVersion;
        }
        catch
        {
            VersionErrorBar.IsOpen = true;
        }
        finally
        {
            _isLoadingVersions = false;
            VersionLoading.IsActive = false;
            VersionLoading.Visibility = Visibility.Collapsed;
            UpdatePrimaryButtonState();
        }
    }

    private void UpdatePrimaryButtonState()
    {
        var canSubmit = !_isLoadingVersions
                        && !string.IsNullOrWhiteSpace(NameBox.Text)
                        && VersionBox.SelectedItem is string
                        && IsValidNumber(MemoryBox)
                        && IsValidNumber(PortBox);
        if (CanSubmit == canSubmit)
            return;

        CanSubmit = canSubmit;
        CanSubmitChanged?.Invoke(this, EventArgs.Empty);
    }

    private static bool IsValidNumber(NumberBox box) =>
        !double.IsNaN(box.Value) && box.Value >= box.Minimum && box.Value <= box.Maximum;

    public UpdateServerRequest BuildRequest()
    {
        var version = _server.CanEditLaunchSettings && VersionBox.SelectedItem is string selectedVersion
            ? selectedVersion
            : _server.McVersion;

        var port = _server.CanEditLaunchSettings && !double.IsNaN(PortBox.Value)
            ? (int)PortBox.Value
            : _server.Port;

        var memoryMb = _server.CanEditLaunchSettings && !double.IsNaN(MemoryBox.Value)
            ? (int)MemoryBox.Value
            : _server.MemoryMb;

        var customJavaArgs = _server.CanEditLaunchSettings
            ? CustomJavaArgsBox.Text.Trim()
            : _server.CustomJavaArgs;

        return new UpdateServerRequest
        {
            Id = _server.Id,
            Name = NameBox.Text.Trim(),
            McVersion = version,
            Port = port,
            SortOrder = _server.SortOrder,
            MemoryMb = memoryMb,
            CustomJavaArgs = customJavaArgs,
        };
    }
}
