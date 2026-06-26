using System.Net;
using System.Net.NetworkInformation;
using System.Net.Sockets;
using CommunityToolkit.Mvvm.ComponentModel;
using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>Observable wrapper around a server, exposing localized, bindable state.</summary>
public sealed class ServerItem : ObservableObject
{
    private static readonly Lazy<string?> ConnectHost = new(FindConnectHost);

    private readonly ILocalizer _localizer;
    private string _name = "";
    private string _mcVersion = "";
    private ServerType _type;
    private ServerStatus _status;
    private int _port;
    private int _memoryMb;
    private int _sortOrder;
    private string _customJavaArgs = "";

    public ServerItem(Server proto, ILocalizer localizer)
    {
        _localizer = localizer;
        Id = proto.Id;
        Apply(proto);
    }

    public string Id { get; }

    public string Name
    {
        get => _name;
        private set => SetProperty(ref _name, value);
    }

    public string McVersion
    {
        get => _mcVersion;
        private set => SetProperty(ref _mcVersion, value);
    }

    public ServerType Type
    {
        get => _type;
        private set
        {
            if (SetProperty(ref _type, value))
                OnPropertyChanged(nameof(TypeText));
        }
    }

    public ServerStatus Status
    {
        get => _status;
        private set
        {
            if (SetProperty(ref _status, value))
            {
                OnPropertyChanged(nameof(StatusText));
                OnPropertyChanged(nameof(CanStart));
                OnPropertyChanged(nameof(CanStop));
                OnPropertyChanged(nameof(CanToggleState));
                OnPropertyChanged(nameof(IsTransitional));
                OnPropertyChanged(nameof(IsRunning));
                OnPropertyChanged(nameof(CanEditLaunchSettings));
                OnPropertyChanged(nameof(StateActionText));
                OnPropertyChanged(nameof(StateActionGlyph));
            }
        }
    }

    public int Port
    {
        get => _port;
        private set
        {
            if (SetProperty(ref _port, value))
                OnPropertyChanged(nameof(EndpointText));
        }
    }

    public int MemoryMb
    {
        get => _memoryMb;
        private set => SetProperty(ref _memoryMb, value);
    }

    public int SortOrder
    {
        get => _sortOrder;
        private set => SetProperty(ref _sortOrder, value);
    }

    public string CustomJavaArgs
    {
        get => _customJavaArgs;
        private set => SetProperty(ref _customJavaArgs, value);
    }

    public string StatusText => _localizer.Get(Status switch
    {
        ServerStatus.Running => "ServerStatus_Running",
        ServerStatus.Stopped => "ServerStatus_Stopped",
        ServerStatus.Installing => "ServerStatus_Installing",
        ServerStatus.Starting => "ServerStatus_Starting",
        ServerStatus.Stopping => "ServerStatus_Stopping",
        ServerStatus.Crashed => "ServerStatus_Crashed",
        _ => "ServerStatus_Unknown",
    });

    public string TypeText => _localizer.Get(Type switch
    {
        ServerType.Vanilla => "ServerType_Vanilla",
        ServerType.Paper => "ServerType_Paper",
        ServerType.Spigot => "ServerType_Spigot",
        ServerType.Forge => "ServerType_Forge",
        ServerType.Neoforge => "ServerType_NeoForge",
        ServerType.Fabric => "ServerType_Fabric",
        _ => "ServerType_Unknown",
    });

    public bool CanStart => Status is ServerStatus.Stopped or ServerStatus.Crashed;
    public bool CanStop => Status is ServerStatus.Running or ServerStatus.Starting;
    public bool CanToggleState => CanStart || CanStop;
    public bool IsRunning => Status is ServerStatus.Running;
    public bool CanEditLaunchSettings => Status is ServerStatus.Stopped or ServerStatus.Crashed;
    public string EndpointText => Port > 0
        ? ConnectHost.Value is { Length: > 0 } host
            ? $"{host}:{Port}"
            : _localizer.Get("Server_PortLabel", ("port", Port.ToString()))
        : _localizer.Get("Server_PortAutoValue");
    public string StateActionText => _localizer.Get(Status switch
    {
        ServerStatus.Running => "ServerState_Stop",
        ServerStatus.Starting => "ServerState_Starting",
        ServerStatus.Stopping => "ServerState_Stopping",
        ServerStatus.Installing => "ServerState_Installing",
        _ => "ServerState_Start",
    });
    public string StateActionGlyph => Status switch
    {
        ServerStatus.Running => "\uE71A",
        ServerStatus.Starting or ServerStatus.Stopping or ServerStatus.Installing => "\uE895",
        _ => "\uE768",
    };

    /// <summary>True while the server is changing state (Starting/Stopping/Installing) —
    /// drives the blinking status dot and the disabled, spinning state button.</summary>
    public bool IsTransitional => Status is ServerStatus.Starting or ServerStatus.Stopping or ServerStatus.Installing;

    /// <summary>Refreshes this item from a fresh server snapshot.</summary>
    public void Apply(Server proto)
    {
        Name = proto.Name;
        McVersion = proto.McVersion;
        Type = proto.Type;
        Status = proto.Status;
        Port = proto.Port;
        MemoryMb = proto.MemoryMb;
        SortOrder = proto.SortOrder;
        CustomJavaArgs = proto.CustomJavaArgs;
    }

    public void ApplyLocal(UpdateServerRequest request)
    {
        Name = request.Name;
        McVersion = request.McVersion;
        Port = request.Port;
        SortOrder = request.SortOrder;
        if (request.MemoryMb > 0)
            MemoryMb = request.MemoryMb;
        CustomJavaArgs = request.CustomJavaArgs;
    }

    private static string? FindConnectHost()
    {
        try
        {
            foreach (var adapter in NetworkInterface.GetAllNetworkInterfaces())
            {
                if (adapter.OperationalStatus != OperationalStatus.Up ||
                    adapter.NetworkInterfaceType is NetworkInterfaceType.Loopback or NetworkInterfaceType.Tunnel)
                    continue;

                var props = adapter.GetIPProperties();
                if (props.GatewayAddresses.Count == 0)
                    continue;

                foreach (var address in props.UnicastAddresses)
                {
                    var ip = address.Address;
                    if (ip.AddressFamily == AddressFamily.InterNetwork &&
                        !IPAddress.IsLoopback(ip) &&
                        !ip.ToString().StartsWith("169.254.", StringComparison.Ordinal))
                        return ip.ToString();
                }
            }
        }
        catch
        {
            // Fall back to the port-only label when local network discovery fails.
        }

        return null;
    }
}
