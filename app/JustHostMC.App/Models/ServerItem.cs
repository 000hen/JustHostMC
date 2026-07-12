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
    public partial string ProviderId {
        get; private set;
    } = "";

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(StatusText))]
    [NotifyPropertyChangedFor(nameof(CanStart))]
    [NotifyPropertyChangedFor(nameof(CanStop))]
    [NotifyPropertyChangedFor(nameof(CanToggleState))]
    [NotifyPropertyChangedFor(nameof(IsTransitional))]
    [NotifyPropertyChangedFor(nameof(IsRunning))]
    [NotifyPropertyChangedFor(nameof(CanEditLaunchSettings))]
    [NotifyPropertyChangedFor(nameof(StateActionText))]
    [NotifyPropertyChangedFor(nameof(StateActionGlyph))]
    [NotifyPropertyChangedFor(nameof(NavigationAutomationName))]
    [NotifyPropertyChangedFor(nameof(NavigationStatusBrush))]
    [NotifyPropertyChangedFor(nameof(IsIncompleteInstallation))]
    [NotifyPropertyChangedFor(nameof(DeleteActionText))]
    public partial ServerStatus Status {
        get; private set;
    }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(EndpointText))]
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

        void Refresh() => OnPropertyChanged(nameof(TypeText));
        if (_dispatcher is not null)
            _dispatcher.TryEnqueue(Refresh);
        else
            Refresh();
    }

    public string Id { get; }
    public ServerProgressTracker ProgressTracker { get; set; } = null!;

    public string StatusText => _localizer.Get(Status switch {
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
        ("name", Name), ("status", StatusText));

    // Prefer the live provider list's friendly name; fall back to the built-in
    // id→localized-name map, then to the raw id.
    public string TypeText {
        get {
            if (_providers?.NameFor(ProviderId) is { Length : > 0 } name)
                return name;
            return ProviderId switch {
                "vanilla"  => _localizer.Get("ServerType_Vanilla"),
                "paper"    => _localizer.Get("ServerType_Paper"),
                "spigot"   => _localizer.Get("ServerType_Spigot"),
                "forge"    => _localizer.Get("ServerType_Forge"),
                "neoforge" => _localizer.Get("ServerType_NeoForge"),
                "fabric"   => _localizer.Get("ServerType_Fabric"),
                ""         => _localizer.Get("ServerType_Unknown"),
                _          => ProviderId,
            };
        }
    }

    public bool CanStart =>
        Status is ServerStatus.Stopped or ServerStatus.Crashed;
    public bool CanStop =>
        Status is ServerStatus.Running or ServerStatus.Starting;
    public bool CanToggleState           => CanStart || CanStop;
    public bool IsRunning                => Status is ServerStatus.Running;
    public bool IsIncompleteInstallation => Status is ServerStatus.Installing;
    public string DeleteActionText =>
        _localizer.Get(IsIncompleteInstallation ? "ServerInstallRemove_Action"
                                                : "ServerDelete_Action");
    public bool CanEditLaunchSettings =>
        Status is ServerStatus.Stopped or ServerStatus.Crashed;
    public string EndpointText     => Port > 0? ConnectHost.Value
                                              is { Length : > 0 } host
                                          ? $"{host}:{Port}"
                                          : _localizer
                                                .Get("Server_PortLabel",
                                                     ("port", Port.ToString()))
        : _localizer.Get("Server_PortAutoValue");
    public string StateActionText  => _localizer.Get(Status switch {
        ServerStatus.Running    => "ServerState_Stop",
        ServerStatus.Starting   => "ServerState_Starting",
        ServerStatus.Stopping   => "ServerState_Stopping",
        ServerStatus.Installing => "ServerState_Installing",
        _                       => "ServerState_Start",
    });
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
