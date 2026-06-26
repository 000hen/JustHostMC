using System.Collections.Generic;
using System.Threading.Tasks;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>Collects the parameters for creating a new server.</summary>
public sealed partial class CreateServerDialog : ContentDialog
{
    private readonly MainViewModel _viewModel;
    private bool _isLoadingVersions;
    private bool _versionLoadFailed;

    private sealed record TypeChoice(ServerType Type, string Display);

    public CreateServerDialog(MainViewModel viewModel)
    {
        _viewModel = viewModel;
        InitializeComponent();

        var loc = new LocalizationService();
        TypeBox.ItemsSource = new List<TypeChoice>
        {
            new(ServerType.Vanilla, loc.Get("ServerType_Vanilla")),
            new(ServerType.Paper, loc.Get("ServerType_Paper")),
            new(ServerType.Forge, loc.Get("ServerType_Forge")),
            new(ServerType.Neoforge, loc.Get("ServerType_NeoForge")),
            new(ServerType.Fabric, loc.Get("ServerType_Fabric")),
        };

        // Disable the primary button until a version is successfully loaded.
        IsPrimaryButtonEnabled = false;

        // Selecting the first type triggers OnTypeChanged, which loads versions.
        TypeBox.SelectedIndex = 0;
    }

    private async void OnTypeChanged(object sender, SelectionChangedEventArgs e)
        => await LoadVersionsAsync();

    private void OnVersionSelectionChanged(object sender, SelectionChangedEventArgs e)
        => UpdatePrimaryButtonState();

    private async Task LoadVersionsAsync()
    {
        if (TypeBox.SelectedItem is not TypeChoice choice)
            return;

        _isLoadingVersions = true;
        _versionLoadFailed = false;
        VersionErrorBar.IsOpen = false;
        VersionLoading.IsActive = true;
        VersionLoading.Visibility = Visibility.Visible;
        VersionBox.ItemsSource = null;
        UpdatePrimaryButtonState();

        try
        {
            var versions = await _viewModel.GetVersionsAsync(choice.Type);
            VersionBox.ItemsSource = versions;
            if (versions.Length > 0)
                VersionBox.SelectedIndex = 0;
        }
        catch
        {
            _versionLoadFailed = true;
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

    /// <summary>
    /// Enables the primary button only when versions are loaded and one is selected.
    /// </summary>
    private void UpdatePrimaryButtonState()
    {
        IsPrimaryButtonEnabled = !_isLoadingVersions
                                 && !_versionLoadFailed
                                 && VersionBox.SelectedItem is string;
    }

    /// <summary>Builds the request from the form, or null if required fields are missing.</summary>
    public CreateServerRequest? BuildRequest()
    {
        if (TypeBox.SelectedItem is not TypeChoice choice)
            return null;

        if (VersionBox.SelectedItem is not string version)
            return null;

        var name = NameBox.Text?.Trim();
        if (string.IsNullOrEmpty(name))
            name = _viewModel.SuggestDefaultServerName();

        return new CreateServerRequest
        {
            Name = name,
            Type = choice.Type,
            McVersion = version,
            MemoryMb = double.IsNaN(MemoryBox.Value) ? 2048 : (int)MemoryBox.Value,
            Port = double.IsNaN(PortBox.Value) ? 0 : (int)PortBox.Value,
            CustomJavaArgs = CustomJavaArgsBox.Text?.Trim() ?? string.Empty,
        };
    }
}

