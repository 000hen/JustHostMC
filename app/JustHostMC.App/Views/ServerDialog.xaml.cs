using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public enum ServerDialogMode {
    Create,
    Edit,
}

/// <summary>Collects server fields for either a create or edit
/// operation.</summary>
public sealed partial class ServerDialog : UserControl {
    private readonly MainViewModel _viewModel;
    private readonly ServerItem? _server;
    private readonly ILocalizer _localizer = new LocalizationService();
    private bool _isInitialized;
    private bool _isLoadingVersions;
    private bool _versionLoadFailed;

    public ServerDialogMode Mode { get; }
    public bool CanSubmit { get; private set; }
    public event EventHandler? CanSubmitChanged;

    private sealed record TypeChoice(ProviderInfo Provider, string Display) {
        public string Id => Provider.Id;
    }

    public ServerDialog(MainViewModel viewModel, ServerDialogMode mode,
                        ServerItem? server = null) {
        if (mode == ServerDialogMode.Edit && server is null)
            throw new ArgumentNullException(nameof(server),
                                            "Edit mode requires a server.");
        if (mode == ServerDialogMode.Create && server is not null)
            throw new ArgumentException(
                "Create mode does not accept an existing server.",
                nameof(server));

        _viewModel = viewModel;
        _server    = server;
        Mode       = mode;
        InitializeComponent();
        _isInitialized = true;

        if (Mode == ServerDialogMode.Create)
            InitializeCreateMode();
        else
            InitializeEditMode(server!);
    }

    private void InitializeCreateMode() {
        _ = LoadProvidersAsync();
    }

    private void InitializeEditMode(ServerItem server) {
        CreateTypePanel.Visibility  = Visibility.Collapsed;
        EditTypeBox.Visibility      = Visibility.Visible;
        VersionHint.Visibility      = Visibility.Collapsed;
        LaunchLockedInfo.Visibility = Visibility.Visible;

        NameBox.Text             = server.Name;
        EditTypeBox.Text         = string.IsNullOrWhiteSpace(server.TypeText)
                                       ? server.ProviderId
                                       : server.TypeText;
        VersionBox.IsEditable    = false;
        VersionBox.ItemsSource   = new[] { server.McVersion };
        VersionBox.SelectedIndex = 0;
        MemoryBox.Value          = server.MemoryMb > 0 ? server.MemoryMb : 2048;
        CustomJavaArgsBox.Text   = server.CustomJavaArgs;
        PortBox.Value            = server.Port;

        VersionBox.IsEnabled        = server.CanEditLaunchSettings;
        MemoryBox.IsEnabled         = server.CanEditLaunchSettings;
        CustomJavaArgsBox.IsEnabled = server.CanEditLaunchSettings;
        PortBox.IsEnabled           = server.CanEditLaunchSettings;
        LaunchLockedInfo.IsOpen     = !server.CanEditLaunchSettings;

        UpdatePrimaryButtonState();
        _ = LoadVersionsAsync(server.ProviderId, server.McVersion);
    }

    private async Task LoadProvidersAsync() {
        try {
            var providers = await _viewModel.GetProvidersAsync();
            TypeBox.ItemsSource =
                providers
                    // Hidden providers (e.g. the modpack installer) are driven
                    // by their own shop flow, not the manual create dialog.
                    .Where(p => p.Capabilities?.Hidden != true)
                    .Select(p => new TypeChoice(p, string.IsNullOrEmpty(p.Name)
                                                       ? p.Id
                                                       : p.Name))
                    .ToList();
            if (TypeBox.Items.Count > 0)
                TypeBox.SelectedIndex = 0;
        } catch {
            _versionLoadFailed     = true;
            VersionErrorBar.IsOpen = true;
            UpdatePrimaryButtonState();
        }
    }

    private async void OnTypeChanged(object sender,
                                     SelectionChangedEventArgs e) {
        UpdateProviderDetails();
        if (TypeBox.SelectedItem is TypeChoice choice)
            await LoadVersionsAsync(choice.Id);
    }

    private void UpdateProviderDetails() {
        if (TypeBox.SelectedItem is not TypeChoice choice) {
            ProviderDetailsPanel.Visibility = Visibility.Collapsed;
            return;
        }

        var provider                    = choice.Provider;
        ProviderDetailsPanel.Visibility = Visibility.Visible;
        ProviderNameText.Text           = choice.Display;

        var description = provider.Description?.Trim() ?? string.Empty;
        ProviderDescriptionText.Text = description;
        ProviderDescriptionText.Visibility =
            description.Length > 0 ? Visibility.Visible : Visibility.Collapsed;

        var author = provider.Author?.Trim() ?? string.Empty;
        if (author.Length > 0) {
            ProviderAuthorText.Text = _localizer.Get(
                "CreateServer_ProviderAuthor", ("author", author));
            ProviderAuthorText.Visibility = Visibility.Visible;
        } else {
            ProviderAuthorText.Visibility = Visibility.Collapsed;
        }

        var website = provider.Website?.Trim() ?? string.Empty;
        if (website.Length > 0 &&
            Uri.TryCreate(website, UriKind.Absolute, out var uri) &&
            (uri.Scheme == Uri.UriSchemeHttp ||
             uri.Scheme == Uri.UriSchemeHttps)) {
            ProviderWebsiteLink.NavigateUri = uri;
            ProviderWebsiteLink.Visibility  = Visibility.Visible;
        } else {
            ProviderWebsiteLink.NavigateUri = null;
            ProviderWebsiteLink.Visibility  = Visibility.Collapsed;
        }
    }

    private void OnTextChanged(object sender, TextChangedEventArgs e) {
        if (_isInitialized)
            UpdatePrimaryButtonState();
    }

    private void OnVersionSelectionChanged(object sender,
                                           SelectionChangedEventArgs e) {
        if (_isInitialized)
            UpdatePrimaryButtonState();
    }

    private void OnNumberChanged(NumberBox sender,
                                 NumberBoxValueChangedEventArgs args) {
        if (_isInitialized)
            UpdatePrimaryButtonState();
    }

    private async Task LoadVersionsAsync(string providerId,
                                         string? selectedVersion = null) {
        if (Mode == ServerDialogMode.Edit && !_server!.CanEditLaunchSettings) {
            VersionLoading.IsActive   = false;
            VersionLoading.Visibility = Visibility.Collapsed;
            return;
        }

        _isLoadingVersions        = true;
        _versionLoadFailed        = false;
        VersionErrorBar.IsOpen    = false;
        VersionLoading.IsActive   = true;
        VersionLoading.Visibility = Visibility.Visible;
        if (selectedVersion is null)
            VersionBox.ItemsSource = null;
        UpdatePrimaryButtonState();

        try {
            var versions =
                (await _viewModel.GetVersionsAsync(providerId)).ToList();
            if (selectedVersion is not null &&
                !versions.Contains(selectedVersion))
                versions.Insert(0, selectedVersion);

            VersionBox.ItemsSource = versions;
            if (selectedVersion is not null)
                VersionBox.SelectedItem = selectedVersion;
            else if (versions.Count > 0)
                VersionBox.SelectedIndex = 0;
        } catch {
            _versionLoadFailed     = Mode == ServerDialogMode.Create;
            VersionErrorBar.IsOpen = true;
        } finally {
            _isLoadingVersions        = false;
            VersionLoading.IsActive   = false;
            VersionLoading.Visibility = Visibility.Collapsed;
            UpdatePrimaryButtonState();
        }
    }

    private void UpdatePrimaryButtonState() {
        var canSubmit = Mode switch {
            ServerDialogMode.Create => !_isLoadingVersions &&
                                       !_versionLoadFailed &&
                                       VersionBox.SelectedItem is string,
            ServerDialogMode.Edit => !_isLoadingVersions &&
                                     !string.IsNullOrWhiteSpace(NameBox.Text) &&
                                     VersionBox.SelectedItem is string &&
                                     IsValidNumber(MemoryBox) &&
                                     IsValidNumber(PortBox),
            _                     => false,
        };

        if (CanSubmit == canSubmit)
            return;

        CanSubmit = canSubmit;
        CanSubmitChanged?.Invoke(this, EventArgs.Empty);
    }

    private static bool IsValidNumber(NumberBox box) =>
        !double.IsNaN(box.Value) && box.Value >= box.Minimum
        && box.Value <= box.Maximum;

    public CreateServerRequest? BuildCreateRequest() {
        EnsureMode(ServerDialogMode.Create);

        if (TypeBox.SelectedItem is not TypeChoice choice ||
            VersionBox.SelectedItem is not string version)
            return null;

        var name = NameBox.Text?.Trim();
        if (string.IsNullOrEmpty(name))
            name = _viewModel.SuggestDefaultServerName();

        return new CreateServerRequest {
            Name       = name,
            ProviderId = choice.Id,
            McVersion  = version,
            MemoryMb =
                double.IsNaN(MemoryBox.Value) ? 2048 : (int)MemoryBox.Value,
            Port = double.IsNaN(PortBox.Value) ? 0 : (int)PortBox.Value,
            CustomJavaArgs = CustomJavaArgsBox.Text?.Trim() ?? string.Empty,
        };
    }

    public UpdateServerRequest BuildUpdateRequest() {
        EnsureMode(ServerDialogMode.Edit);
        var server = _server!;

        var version = server.CanEditLaunchSettings &&
                              VersionBox.SelectedItem is string selectedVersion
                          ? selectedVersion
                          : server.McVersion;
        var port = server.CanEditLaunchSettings && !double.IsNaN(PortBox.Value)
                       ? (int)PortBox.Value
                       : server.Port;
        var memoryMb =
            server.CanEditLaunchSettings && !double.IsNaN(MemoryBox.Value)
                ? (int)MemoryBox.Value
                : server.MemoryMb;
        var customJavaArgs = server.CanEditLaunchSettings
                                 ? CustomJavaArgsBox.Text.Trim()
                                 : server.CustomJavaArgs;

        return new UpdateServerRequest {
            Id             = server.Id,
            Name           = NameBox.Text.Trim(),
            McVersion      = version,
            Port           = port,
            SortOrder      = server.SortOrder,
            MemoryMb       = memoryMb,
            CustomJavaArgs = customJavaArgs,
        };
    }

    private void EnsureMode(ServerDialogMode expectedMode) {
        if (Mode != expectedMode)
            throw new InvalidOperationException(
                $"Cannot build a {expectedMode} request in {Mode} mode.");
    }
}
