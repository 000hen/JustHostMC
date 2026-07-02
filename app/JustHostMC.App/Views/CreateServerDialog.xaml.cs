using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>Collects the parameters for creating a new server.</summary>
public sealed partial class CreateServerDialog : UserControl
{
    private readonly MainViewModel _viewModel;
    private readonly ILocalizer _localizer = new LocalizationService();
    private bool _isLoadingVersions;
    private bool _versionLoadFailed;

    public bool CanSubmit { get; private set; }
    public event EventHandler? CanSubmitChanged;

    private sealed record TypeChoice(ProviderInfo Provider, string Display)
    {
        public string Id => Provider.Id;
    }

    public CreateServerDialog(MainViewModel viewModel)
    {
        _viewModel = viewModel;
        InitializeComponent();

        // Load the installed providers; selecting the first triggers a version load.
        _ = LoadProvidersAsync();
    }

    private async Task LoadProvidersAsync()
    {
        try
        {
            var providers = await _viewModel.GetProvidersAsync();
            TypeBox.ItemsSource = providers
                .Select(p => new TypeChoice(p, string.IsNullOrEmpty(p.Name) ? p.Id : p.Name))
                .ToList();
            if (TypeBox.Items.Count > 0)
                TypeBox.SelectedIndex = 0; // triggers OnTypeChanged -> version load
        }
        catch
        {
            _versionLoadFailed = true;
            VersionErrorBar.IsOpen = true;
            UpdatePrimaryButtonState();
        }
    }

    private async void OnTypeChanged(object sender, SelectionChangedEventArgs e)
    {
        UpdateProviderDetails();
        await LoadVersionsAsync();
    }

    /// <summary>Shows the selected provider's name, description, author and website.</summary>
    private void UpdateProviderDetails()
    {
        if (TypeBox.SelectedItem is not TypeChoice choice)
        {
            ProviderDetailsPanel.Visibility = Visibility.Collapsed;
            return;
        }

        var provider = choice.Provider;
        ProviderDetailsPanel.Visibility = Visibility.Visible;
        ProviderNameText.Text = choice.Display;

        var description = provider.Description?.Trim() ?? string.Empty;
        ProviderDescriptionText.Text = description;
        ProviderDescriptionText.Visibility = description.Length > 0
            ? Visibility.Visible : Visibility.Collapsed;

        var author = provider.Author?.Trim() ?? string.Empty;
        if (author.Length > 0)
        {
            ProviderAuthorText.Text = _localizer.Get("CreateServer_ProviderAuthor", ("author", author));
            ProviderAuthorText.Visibility = Visibility.Visible;
        }
        else
        {
            ProviderAuthorText.Visibility = Visibility.Collapsed;
        }

        var website = provider.Website?.Trim() ?? string.Empty;
        if (website.Length > 0 && Uri.TryCreate(website, UriKind.Absolute, out var uri)
            && (uri.Scheme == Uri.UriSchemeHttp || uri.Scheme == Uri.UriSchemeHttps))
        {
            ProviderWebsiteLink.NavigateUri = uri;
            ProviderWebsiteLink.Visibility = Visibility.Visible;
        }
        else
        {
            ProviderWebsiteLink.NavigateUri = null;
            ProviderWebsiteLink.Visibility = Visibility.Collapsed;
        }
    }

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
            var versions = await _viewModel.GetVersionsAsync(choice.Id);
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

    /// <summary>Enables the primary button only when versions are loaded and one is selected.</summary>
    private void UpdatePrimaryButtonState()
    {
        var canSubmit = !_isLoadingVersions
                        && !_versionLoadFailed
                        && VersionBox.SelectedItem is string;
        if (CanSubmit == canSubmit)
            return;

        CanSubmit = canSubmit;
        CanSubmitChanged?.Invoke(this, EventArgs.Empty);
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
            ProviderId = choice.Id,
            McVersion = version,
            MemoryMb = double.IsNaN(MemoryBox.Value) ? 2048 : (int)MemoryBox.Value,
            Port = double.IsNaN(PortBox.Value) ? 0 : (int)PortBox.Value,
            CustomJavaArgs = CustomJavaArgsBox.Text?.Trim() ?? string.Empty,
        };
    }
}
