using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>Maps <see cref="PermissionKind"/> values to localization keys for
/// their human-readable labels (shown in the consent dialog and script
/// cards).</summary>
public static class PermissionLabels {
    public static string LabelKey(PermissionKind kind) => kind switch {
        PermissionKind.PermissionNetwork       => "Permission_Network",
        PermissionKind.PermissionInstall       => "Permission_Install",
        PermissionKind.PermissionFsServer      => "Permission_FsServer",
        PermissionKind.PermissionConsoleRead   => "Permission_ConsoleRead",
        PermissionKind.PermissionConsoleWrite  => "Permission_ConsoleWrite",
        PermissionKind.PermissionServerControl => "Permission_ServerControl",
        PermissionKind.PermissionSchedule      => "Permission_Schedule",
        PermissionKind.PermissionServerQuery   => "Permission_ServerQuery",
        PermissionKind.PermissionPlayerManage  => "Permission_PlayerManage",
        _                                      => "Permission_Unknown",
    };

    /// <summary>Resolves the localized label for a permission kind.</summary>
    public static string Label(PermissionKind kind, ILocalizer localizer) =>
        localizer.Get(LabelKey(kind));
}
