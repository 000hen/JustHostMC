using System.Net;
using System.Net.NetworkInformation;
using System.Net.Sockets;
using CommunityToolkit.Mvvm.ComponentModel;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media;

namespace JustHostMC.App.Models;

/// <summary>Observable wrapper around a server, exposing localized, bindable
/// state.</summary>
public sealed partial class ServerItem : ObservableObject {
    private static readonly Lazy<string?> ConnectHost = new(FindConnectHost);

    private readonly ILocalizer _localizer;
    private readonly ProviderCatalog? _providers;
    private readonly DispatcherQueue? _dispatcher;
    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(NavigationAutomationName))]
    public partial string Name { get; private set; } = "";

    [ObservableProperty]
    public partial string McVersion {
        get; private set;
    } = "";

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(TypeText))]
    [NotifyPropertyChangedFor(nameof(HasTypeText))]
    [NotifyPropertyChangedFor(nameof(IsVanillaProvider))]
    [NotifyPropertyChangedFor(nameof(IsPaperProvider))]
    [NotifyPropertyChangedFor(nameof(IsSpigotProvider))]
    [NotifyPropertyChangedFor(nameof(IsForgeProvider))]
    [NotifyPropertyChangedFor(nameof(IsNeoForgeProvider))]
    [NotifyPropertyChangedFor(nameof(IsFabricProvider))]
    [NotifyPropertyChangedFor(nameof(IsTypeUnknown))]
    [NotifyPropertyChangedFor(nameof(EffectiveLoader))]
    public partial string ProviderId {
        get; private set;
    } = "";

    /// <summary>Effective mod loader recorded at install ("fabric"/"forge"/…);
    /// empty for providers whose loader equals their id.</summary>
    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(EffectiveLoader))]
    public partial string Loader {
        get; private set;
    } = "";

    /// <summary>The loader to reason about compatibility with: the recorded
    /// loader when set, else the provider id (which doubles as the loader for
    /// plain providers like fabric/paper/forge).</summary>
    public string EffectiveLoader => Loader.Length > 0 ? Loader : ProviderId;

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(CanStart))]
    [NotifyPropertyChangedFor(nameof(CanStop))]
    [NotifyPropertyChangedFor(nameof(CanToggleState))]
    [NotifyPropertyChangedFor(nameof(IsTransitional))]
    [NotifyPropertyChangedFor(nameof(IsRunning))]
    [NotifyPropertyChangedFor(nameof(IsStopped))]
    [NotifyPropertyChangedFor(nameof(IsInstalling))]
    [NotifyPropertyChangedFor(nameof(IsStarting))]
    [NotifyPropertyChangedFor(nameof(IsStopping))]
    [NotifyPropertyChangedFor(nameof(IsCrashed))]
    [NotifyPropertyChangedFor(nameof(IsStatusUnknown))]
    [NotifyPropertyChangedFor(nameof(ShowStartAction))]
    [NotifyPropertyChangedFor(nameof(CanEditLaunchSettings))]
    [NotifyPropertyChangedFor(nameof(StateActionGlyph))]
    [NotifyPropertyChangedFor(nameof(NavigationAutomationName))]
    [NotifyPropertyChangedFor(nameof(NavigationStatusBrush))]
    [NotifyPropertyChangedFor(nameof(IsIncompleteInstallation))]
    public partial ServerStatus Status {
        get; private set;
    }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(EndpointText))]
    [NotifyPropertyChangedFor(nameof(HasEndpoint))]
    public partial int Port {
        get; private set;
    }

    [ObservableProperty]
    public partial int MemoryMb {
        get; private set;
    }

    [ObservableProperty]
    public partial int SortOrder {
        get; private set;
    }

    [ObservableProperty]
    public partial string CustomJavaArgs {
        get; private set;
    } = "";

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(NavigationAutomationName))]
    [NotifyPropertyChangedFor(nameof(NavigationInfoBadgeVisibility))]
    public partial bool HasUnreadStateChange {
        get; set;
    }

    public ServerItem(Server proto, ILocalizer localizer,
                      ProviderCatalog? providers  = null,
                      DispatcherQueue? dispatcher = null) {
        _localizer  = localizer;
        _providers  = providers;
        _dispatcher = dispatcher;
        Id          = proto.Id;
        Apply(proto);

        // The provider name + capabilities arrive asynchronously; refresh the
        // friendly type label (on the UI thread) once the catalog loads. If it
        // already loaded, TypeText resolves synchronously and no refresh is
        // needed.
        if (_providers is not null && !_providers.IsLoaded)
            _providers.Loaded += OnProvidersLoaded;
    }

    private void OnProvidersLoaded() {
        // The catalog populates once; after that TypeText resolves
        // synchronously, so drop the subscription to avoid keeping deleted
        // ServerItems alive.
        if (_providers is not null)
            _providers.Loaded -= OnProvidersLoaded;

        void Refresh() {
            OnPropertyChanged(nameof(TypeText));
            OnPropertyChanged(nameof(HasTypeText));
        }
        if (_dispatcher is not null)
            _dispatcher.TryEnqueue(Refresh);
        else
            Refresh();
    }

    public string Id { get; }
    public ServerProgressTracker ProgressTracker { get; set; } = null!;

    private string LocalizedStatusText => _localizer.Get(Status switch {
        ServerStatus.Running    => "ServerStatus_Running",
        ServerStatus.Stopped    => "ServerStatus_Stopped",
        ServerStatus.Installing => "ServerStatus_Installing",
        ServerStatus.Starting   => "ServerStatus_Starting",
        ServerStatus.Stopping   => "ServerStatus_Stopping",
        ServerStatus.Crashed    => "ServerStatus_Crashed",
        _                       => "ServerStatus_Unknown",
    });

    public Visibility NavigationInfoBadgeVisibility =>
        HasUnreadStateChange ? Visibility.Visible : Visibility.Collapsed;

    public Brush NavigationStatusBrush =>
        (Brush)Application.Current.Resources[Status switch {
            ServerStatus.Running => "SystemFillColorSuccessBrush",
            ServerStatus.Crashed => "SystemFillColorCriticalBrush",
            ServerStatus.Starting or ServerStatus.Stopping or
                ServerStatus.Installing => "SystemFillColorCautionBrush",
            _                           => "TextFillColorDisabledBrush",
        }];

    public string NavigationAutomationName => _localizer.Get(
        HasUnreadStateChange ? "ServerNav_StateChangedAutomationName"
                             : "ServerNav_AutomationName",
        ("name", Name), ("status", LocalizedStatusText));

    // Custom provider names are runtime script data. Built-in and unknown
    // provider labels are rendered by x:Uid-backed branches in each view.
    public string TypeText {
        get {
            if (IsBuiltInProvider)
                return "";
            if (_providers?.NameFor(ProviderId) is { Length : > 0 } name)
                return name;
            return ProviderId;
        }
    }
    public bool HasTypeText        => !string.IsNullOrWhiteSpace(TypeText);
    public bool IsVanillaProvider  => ProviderId == "vanilla";
    public bool IsPaperProvider    => ProviderId == "paper";
    public bool IsSpigotProvider   => ProviderId == "spigot";
    public bool IsForgeProvider    => ProviderId == "forge";
    public bool IsNeoForgeProvider => ProviderId == "neoforge";
    public bool IsFabricProvider   => ProviderId == "fabric";
    public bool IsTypeUnknown      => string.IsNullOrWhiteSpace(ProviderId);
    private bool IsBuiltInProvider => IsVanillaProvider || IsPaperProvider ||
                                      IsSpigotProvider || IsForgeProvider ||
                                      IsNeoForgeProvider || IsFabricProvider;

    public bool CanStart =>
        Status is ServerStatus.Stopped or ServerStatus.Crashed;
    public bool CanStop =>
        Status is ServerStatus.Running or ServerStatus.Starting;
    public bool CanToggleState  => CanStart || CanStop;
    public bool IsRunning       => Status is ServerStatus.Running;
    public bool IsStopped       => Status is ServerStatus.Stopped;
    public bool IsInstalling    => Status is ServerStatus.Installing;
    public bool IsStarting      => Status is ServerStatus.Starting;
    public bool IsStopping      => Status is ServerStatus.Stopping;
    public bool IsCrashed       => Status is ServerStatus.Crashed;
    public bool IsStatusUnknown => Status is not(
        ServerStatus.Running or ServerStatus.Stopped or
            ServerStatus.Installing or ServerStatus.Starting or
                ServerStatus.Stopping or ServerStatus.Crashed);
    public bool ShowStartAction => Status is not(
        ServerStatus.Running or ServerStatus.Installing or
            ServerStatus.Starting or ServerStatus.Stopping);
    public bool IsIncompleteInstallation => Status is ServerStatus.Installing;
    public bool CanEditLaunchSettings =>
        Status is ServerStatus.Stopped or ServerStatus.Crashed;
    public bool HasEndpoint => Port > 0;
    public string EndpointText =>
        Port <= 0 ? ""
        : ConnectHost.Value is { Length : > 0 } host
            ? $"{host}:{Port}"
            : _localizer.Get("Server_PortLabel", ("port", Port.ToString()));
    public string StateActionGlyph => Status switch {
        ServerStatus.Running => "\uE71A",
        ServerStatus.Starting or ServerStatus.Stopping or
            ServerStatus.Installing => "\uE895",
        _                           => "\uE768",
    };

    /// <summary>True while the server is changing state
    /// (Starting/Stopping/Installing) — drives the blinking status dot and the
    /// disabled, spinning state button.</summary>
    public bool IsTransitional =>
        Status is ServerStatus.Starting or ServerStatus.Stopping or
            ServerStatus.Installing;

    /// <summary>Refreshes this item from a fresh server snapshot.</summary>
    public void Apply(Server proto) {
        Name           = proto.Name;
        McVersion      = proto.McVersion;
        ProviderId     = proto.ProviderId;
        Loader         = proto.Loader;
        Status         = proto.Status;
        Port           = proto.Port;
        MemoryMb       = proto.MemoryMb;
        SortOrder      = proto.SortOrder;
        CustomJavaArgs = proto.CustomJavaArgs;
    }

    public void ApplyLocal(UpdateServerRequest request) {
        Name      = request.Name;
        McVersion = request.McVersion;
        Port      = request.Port;
        SortOrder = request.SortOrder;
        if (request.MemoryMb > 0)
            MemoryMb = request.MemoryMb;
        CustomJavaArgs = request.CustomJavaArgs;
    }

    private static string? FindConnectHost() {
        try {
            foreach (var adapter in NetworkInterface
                         .GetAllNetworkInterfaces()) {
                if (adapter.OperationalStatus != OperationalStatus.Up ||
                    adapter.NetworkInterfaceType is NetworkInterfaceType
                        .Loopback or NetworkInterfaceType.Tunnel)
                    continue;

                var props = adapter.GetIPProperties();
                if (props.GatewayAddresses.Count == 0)
                    continue;

                foreach (var address in props.UnicastAddresses) {
                    var ip = address.Address;
                    if (ip.AddressFamily == AddressFamily.InterNetwork &&
                        !IPAddress.IsLoopback(ip) &&
                        !ip.ToString().StartsWith("169.254.",
                                                  StringComparison.Ordinal))
                        return ip.ToString();
                }
            }
        } catch {
            // Fall back to the port-only label when local network discovery
            // fails.
        }

        return null;
    }
}
