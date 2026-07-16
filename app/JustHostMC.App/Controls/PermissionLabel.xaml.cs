using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>Renders a semantic permission kind through localized XAML
/// branches. Code chooses the branch but never resolves or composes localized
/// text.</summary>
public sealed partial class PermissionLabel : UserControl {
    public static readonly DependencyProperty KindProperty =
        DependencyProperty.Register(
            nameof(Kind), typeof(PermissionKind), typeof(PermissionLabel),
            new PropertyMetadata(default(PermissionKind), OnKindChanged));

    public PermissionLabel() {
        InitializeComponent();
        ApplyKind(Kind);
    }

    public PermissionKind Kind {
        get => (PermissionKind)GetValue(KindProperty);
        set => SetValue(KindProperty, value);
    }

    private static void OnKindChanged(DependencyObject sender,
                                      DependencyPropertyChangedEventArgs args) {
        if (sender is PermissionLabel label &&
            args.NewValue is PermissionKind kind)
            label.ApplyKind(kind);
    }

    private void ApplyKind(PermissionKind kind) {
        NetworkLabel.Visibility = VisibilityFor(
            kind, PermissionKind.PermissionNetwork);
        InstallLabel.Visibility = VisibilityFor(
            kind, PermissionKind.PermissionInstall);
        FsServerLabel.Visibility = VisibilityFor(
            kind, PermissionKind.PermissionFsServer);
        ConsoleReadLabel.Visibility = VisibilityFor(
            kind, PermissionKind.PermissionConsoleRead);
        ConsoleWriteLabel.Visibility = VisibilityFor(
            kind, PermissionKind.PermissionConsoleWrite);
        ServerControlLabel.Visibility = VisibilityFor(
            kind, PermissionKind.PermissionServerControl);
        ScheduleLabel.Visibility = VisibilityFor(
            kind, PermissionKind.PermissionSchedule);
        ServerQueryLabel.Visibility = VisibilityFor(
            kind, PermissionKind.PermissionServerQuery);
        PlayerManageLabel.Visibility = VisibilityFor(
            kind, PermissionKind.PermissionPlayerManage);
        UnknownLabel.Visibility = IsKnown(kind) ? Visibility.Collapsed
                                                : Visibility.Visible;
    }

    private static Visibility VisibilityFor(PermissionKind actual,
                                            PermissionKind expected) =>
        actual == expected ? Visibility.Visible : Visibility.Collapsed;

    private static bool IsKnown(PermissionKind kind) => kind is
        PermissionKind.PermissionNetwork or
        PermissionKind.PermissionInstall or
        PermissionKind.PermissionFsServer or
        PermissionKind.PermissionConsoleRead or
        PermissionKind.PermissionConsoleWrite or
        PermissionKind.PermissionServerControl or
        PermissionKind.PermissionSchedule or
        PermissionKind.PermissionServerQuery or
        PermissionKind.PermissionPlayerManage;
}
