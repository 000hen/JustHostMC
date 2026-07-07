using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using JustHostMC.App.Views;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls.Server;

public sealed partial class ServerPlayersPanel : UserControl {
    private readonly ILocalizer _localizer = new LocalizationService();

    public static readonly DependencyProperty PlayersProperty = DependencyProperty.Register(
        nameof(Players),
        typeof(PlayersViewModel),
        typeof(ServerPlayersPanel),
        new PropertyMetadata(null, OnViewModelChanged));

    public static readonly DependencyProperty ServerProperty = DependencyProperty.Register(
        nameof(Server),
        typeof(ServerItem),
        typeof(ServerPlayersPanel),
        new PropertyMetadata(null, OnViewModelChanged));

    public static readonly DependencyProperty ConsoleProperty = DependencyProperty.Register(
        nameof(Console),
        typeof(ConsoleViewModel),
        typeof(ServerPlayersPanel),
        new PropertyMetadata(null));

    public PlayersViewModel Players {
        get => (PlayersViewModel)GetValue(PlayersProperty);
        set => SetValue(PlayersProperty, value);
    }

    public ServerItem Server {
        get => (ServerItem)GetValue(ServerProperty);
        set => SetValue(ServerProperty, value);
    }

    public ConsoleViewModel Console {
        get => (ConsoleViewModel)GetValue(ConsoleProperty);
        set => SetValue(ConsoleProperty, value);
    }

    public ServerPlayersPanel() {
        InitializeComponent();
    }

    private static void OnViewModelChanged(DependencyObject d, DependencyPropertyChangedEventArgs e) {
        var panel = (ServerPlayersPanel)d;
        panel.Bindings.Update();
    }

    private string PlayersHeader(int count) => _localizer.Get("Players_Header", ("count", count.ToString()));

    private Visibility HasNoPlayers(int count) => count == 0 ? Visibility.Visible : Visibility.Collapsed;

    private void OnPlayerOpClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "op {0}");
    private void OnPlayerDeopClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "deop {0}");
    private void OnPlayerKickClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "kick {0}");
    private void OnPlayerBanClick(object sender, RoutedEventArgs e) => SendPlayerCommand(sender, "ban {0}");
    private async void OnPlayerRawClick(object sender, RoutedEventArgs e) => await ShowPlayerDataDialogAsync(sender);
    private async void OnPlayerInventoryClick(object sender, RoutedEventArgs e) => await ShowPlayerInventoryDialogAsync(sender);

    private async Task ShowPlayerDataDialogAsync(object sender) {
        if (GetPlayer(sender) is not { } player || Server == null)
            return;

        var view = new PlayerDataDialog(Server.Id, player);
        var content = new PlayerDialogBase(player, view);
        var dialog = CreatePlayerDialog(_localizer.Get("PlayerDataDialog_ActionName"), player, content);
        view.OnHeaderUpdated = content.UpdateHeader;
        await dialog.ShowAsync();
    }

    private async Task ShowPlayerInventoryDialogAsync(object sender) {
        if (GetPlayer(sender) is not { } player || Server == null)
            return;

        var view = new PlayerInventoryDialog(Server.Id, player);
        var content = new PlayerDialogBase(player, view);
        var dialog = CreatePlayerDialog(_localizer.Get("PlayerInventoryDialog_ActionName"), player, content);
        view.OnHeaderUpdated = content.UpdateHeader;
        await dialog.ShowAsync();
    }

    private ContentDialog CreatePlayerDialog(string actionName, PlayerItem player, PlayerDialogBase content) {
        var dialog = new ContentDialog {
            XamlRoot = XamlRoot,
            Style = Application.Current.Resources["DefaultContentDialogStyle"] as Style,
            Title = string.Format(_localizer.Get("PlayerDialogBase_TitleFormat"), actionName, player.Name),
            Content = content,
            CloseButtonText = _localizer.Get("PlayerDialogBase_CloseButtonText"),
            DefaultButton = ContentDialogButton.Close,
        };
        ContentDialogSizing.Apply(dialog, useWideLayout: true);
        return dialog;
    }

    private async void OnManageBansClick(object sender, RoutedEventArgs e) {
        if (Server == null)
            return;

        var isStopped = Server.Status is ServerStatus.Stopped or ServerStatus.Crashed;
        var content = new BanListDialog(Server.Id, isStopped);
        var dialog = new ContentDialog {
            XamlRoot = XamlRoot,
            Style = Application.Current.Resources["DefaultContentDialogStyle"] as Style,
            Title = _localizer.Get("BanListDialog_Title"),
            Content = content,
            CloseButtonText = _localizer.Get("BanListDialog_CloseButtonText"),
            DefaultButton = ContentDialogButton.Close,
        };
        dialog.Opened += async (_, _) => await content.LoadAsync();
        ContentDialogSizing.Apply(dialog, useWideLayout: true);
        await dialog.ShowAsync();
    }

    private void SendPlayerCommand(object sender, string format) {
        if (Console == null)
            return;

        var player = GetPlayer(sender)?.Name ?? GetPlayerName(sender);
        if (string.IsNullOrWhiteSpace(player))
            return;

        Console.CommandText = string.Format(format, player);
        if (Console.SendCommand.CanExecute(null))
            Console.SendCommand.Execute(null);
    }

    private static PlayerItem? GetPlayer(object sender) => sender switch {
        FrameworkElement { Tag: PlayerItem taggedPlayer } => taggedPlayer,
        FrameworkElement { DataContext: PlayerItem dataPlayer } => dataPlayer,
        _ => null,
    };

    private static string? GetPlayerName(object sender) => sender switch {
        FrameworkElement { Tag: string taggedName } => taggedName,
        FrameworkElement { DataContext: string dataName } => dataName,
        _ => null,
    };
}
