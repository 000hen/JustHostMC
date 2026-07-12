using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>Maps <see cref="PermissionKind"/> values to localization keys for
/// their human-readable labels (shown in the consent dialog and script
/// cards).</summary>
public static class PermissionLabels {
    public static string LabelKey(PermissionKind kind) => kind switch {
        PermissionKind.PermissionNetwork       => "Permission.Network",
        PermissionKind.PermissionInstall       => "Permission.Install",
        PermissionKind.PermissionFsServer      => "Permission.FsServer",
        PermissionKind.PermissionConsoleRead   => "Permission.ConsoleRead",
        PermissionKind.PermissionConsoleWrite  => "Permission.ConsoleWrite",
        PermissionKind.PermissionServerControl => "Permission.ServerControl",
        PermissionKind.PermissionSchedule      => "Permission.Schedule",
        PermissionKind.PermissionServerQuery   => "Permission.ServerQuery",
        PermissionKind.PermissionPlayerManage  => "Permission.PlayerManage",
        _                                      => "Permission.Unknown",
    };

    /// <summary>Resolves the localized label for a permission kind.</summary>
    public static string Label(PermissionKind kind, ILocalizer localizer) =>
        localizer.Get(LabelKey(kind));
}
