using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace JustHostMC.App.Controls.Server;

public sealed partial class ServerHeaderPanel : UserControl {
    private readonly ILocalizer _localizer = new LocalizationService();

    public static readonly DependencyProperty ServerProperty =
        DependencyProperty.Register(
            nameof(Server), typeof(ServerItem), typeof(ServerHeaderPanel),
            new PropertyMetadata(null, OnServerChanged));

    public ServerItem Server {
        get => (ServerItem)GetValue(ServerProperty);
        set => SetValue(ServerProperty, value);
    }

    public event RoutedEventHandler? TitleRenameClick;
    public event RoutedEventHandler? StateButtonClick;
    public event RoutedEventHandler? EditClick;
    public event RoutedEventHandler? BackupsClick;
    public event RoutedEventHandler? OpenInstanceFolderClick;
    public event RoutedEventHandler? DeleteClick;
    public event RoutedEventHandler? UpdateModpackClick;
    public event RoutedEventHandler? ExportModpackClick;

    public ServerHeaderPanel() {
        InitializeComponent();
    }

    private static void OnServerChanged(DependencyObject d,
                                        DependencyPropertyChangedEventArgs e) {
        if (d is ServerHeaderPanel panel) {
            panel.Bindings.Update();
        }
    }

    private void OnTitleRenameClick(object sender, RoutedEventArgs e) =>
        TitleRenameClick?.Invoke(this, e);
    private void OnStateButtonClick(object sender, RoutedEventArgs e) =>
        StateButtonClick?.Invoke(this, e);
    private void OnEditClick(object sender,
                             RoutedEventArgs e) => EditClick?.Invoke(this, e);
    private void OnBackupsClick(object sender,
                                RoutedEventArgs e) => BackupsClick?.Invoke(this,
                                                                           e);
    private void OnOpenInstanceFolderClick(object sender, RoutedEventArgs e) =>
        OpenInstanceFolderClick?.Invoke(this, e);
    private void OnDeleteClick(object sender,
                               RoutedEventArgs e) => DeleteClick?.Invoke(this,
                                                                         e);
    private void OnUpdateModpackClick(object sender, RoutedEventArgs e) =>
        UpdateModpackClick?.Invoke(this, e);
    private void OnExportModpackClick(object sender, RoutedEventArgs e) =>
        ExportModpackClick?.Invoke(this, e);

    /// <summary>Modpack actions only exist for servers installed from a
    /// modpack (non-empty pack version).</summary>
    public Visibility ModpackItemVisibility(string providerVersion) =>
        string.IsNullOrEmpty(providerVersion) ? Visibility.Collapsed
                                              : Visibility.Visible;

    /// <summary>Updating replaces pack files, so the server must not be
    /// running.</summary>
    public bool UpdateModpackEnabled(ServerStatus s) =>
        s is ServerStatus.Stopped or ServerStatus.Crashed;

    private void OnPanelSizeChanged(object sender, SizeChangedEventArgs e) =>
        UpdateResponsiveLayout(e.NewSize.Width);

    private void UpdateResponsiveLayout(double width) {
        var wide   = width >= 900;
        var medium = !wide && width >= 620;

        if (wide) {
            MetaGrid.ColumnDefinitions.Clear();
            for (int i = 0; i < 4; i++)
                MetaGrid.ColumnDefinitions.Add(new ColumnDefinition {
                    Width = new GridLength(1, GridUnitType.Star)
                });
            MetaGrid.RowDefinitions.Clear();
            MetaGrid.RowDefinitions.Add(
                new RowDefinition { Height = GridLength.Auto });

            SetGrid(MetaType, 0, 0, new Thickness(0, 0, 1, 0));
            SetGrid(MetaVersion, 0, 1, new Thickness(0, 0, 1, 0));
            SetGrid(MetaPort, 0, 2, new Thickness(0, 0, 1, 0));
            SetGrid(MetaMemory, 0, 3, new Thickness(0));
        } else if (medium) {
            MetaGrid.ColumnDefinitions.Clear();
            for (int i = 0; i < 2; i++)
                MetaGrid.ColumnDefinitions.Add(new ColumnDefinition {
                    Width = new GridLength(1, GridUnitType.Star)
                });
            MetaGrid.RowDefinitions.Clear();
            for (int i = 0; i < 2; i++)
                MetaGrid.RowDefinitions.Add(
                    new RowDefinition { Height = GridLength.Auto });

            SetGrid(MetaType, 0, 0, new Thickness(0, 0, 1, 1));
            SetGrid(MetaVersion, 0, 1, new Thickness(0, 0, 0, 1));
            SetGrid(MetaPort, 1, 0, new Thickness(0, 0, 1, 0));
            SetGrid(MetaMemory, 1, 1, new Thickness(0));
        } else {
            MetaGrid.ColumnDefinitions.Clear();
            MetaGrid.ColumnDefinitions.Add(new ColumnDefinition {
                Width = new GridLength(1, GridUnitType.Star)
            });
            MetaGrid.RowDefinitions.Clear();
            for (int i = 0; i < 4; i++)
                MetaGrid.RowDefinitions.Add(
                    new RowDefinition { Height = GridLength.Auto });

            SetGrid(MetaType, 0, 0, new Thickness(0, 0, 0, 1));
            SetGrid(MetaVersion, 1, 0, new Thickness(0, 0, 0, 1));
            SetGrid(MetaPort, 2, 0, new Thickness(0, 0, 0, 1));
            SetGrid(MetaMemory, 3, 0, new Thickness(0));
        }
    }

    private void SetGrid(FrameworkElement el, int row, int col,
                         Thickness border) {
        Grid.SetRow(el, row);
        Grid.SetColumn(el, col);
        if (el is Grid g)
            g.BorderThickness = border;
    }

    private Brush StateBrush(ServerStatus s) =>
        (Brush)Application.Current.Resources[s switch {
            ServerStatus.Running => "SystemFillColorCriticalBrush",
            ServerStatus.Starting or ServerStatus.Stopping or
                ServerStatus.Installing => "ControlFillColorDisabledBrush",
            _                           => "SystemFillColorSuccessBrush",
        }];

    private Brush StateForeground(ServerStatus s) =>
        (Brush)Application.Current
            .Resources[s is ServerStatus.Starting or
                               ServerStatus.Stopping or ServerStatus.Installing
                           ? "TextFillColorDisabledBrush"
                           : "TextOnAccentFillColorPrimaryBrush"];

    private bool StateEnabled(ServerStatus s) =>
        s is ServerStatus.Stopped or ServerStatus.Crashed or
        ServerStatus.Running;

    private Visibility PositiveValueVisibility(int value) =>
        value > 0? Visibility.Visible : Visibility.Collapsed;

    private Visibility NonPositiveValueVisibility(int value) =>
        value > 0? Visibility.Collapsed : Visibility.Visible;

    private string ValueText(int value) => value.ToString();

    private string MemoryText(int memoryMb) =>
        _localizer.Get("Server_MemoryValue", ("memory", memoryMb.ToString()));
}
