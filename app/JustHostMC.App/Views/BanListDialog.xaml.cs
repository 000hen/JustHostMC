using System.Collections.ObjectModel;
using Grpc.Core;
using JustHostMC.App.Models;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>Modal editor for vanilla banned-players.json and
/// banned-ips.json.</summary>
public sealed partial class BanListDialog : UserControl {
    private readonly string _serverId;

    public ObservableCollection<BanEntryItem> Bans { get; } = new();
    public bool CanModify { get; }

    public BanListDialog(string serverId, bool canModify) {
        _serverId = serverId;
        CanModify = canModify;
        InitializeComponent();
    }

    public async Task LoadAsync() {
        BusyBar.Visibility = Visibility.Visible;
        ClearStatus();
        try {
            var daemon = await App.Current.DaemonReady;
            var list   = await daemon.Players.ListBansAsync(
                new ServerId { Id = _serverId });
            Bans.Clear();
            foreach (var entry in list.Entries)
                Bans.Add(new BanEntryItem(entry));
            UpdateEmptyState();
        } catch (RpcException) {
            ShowStatus(LoadFailedText);
        } finally {
            BusyBar.Visibility = Visibility.Collapsed;
        }
    }

    private async void OnAddClick(object sender, RoutedEventArgs e) {
        if (!CanModify)
            return;
        var target = TargetBox.Text.Trim();
        if (target.Length == 0) {
            ShowStatus(TargetRequiredText);
            return;
        }

        BusyBar.Visibility = Visibility.Visible;
        ClearStatus();
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Players.AddBanAsync(new AddBanRequest {
                ServerId = _serverId,
                Type     = TypeBox.SelectedIndex == 1 ? BanListType.IpBans
                                                      : BanListType.PlayerBans,
                Target   = target,
                Reason   = ReasonBox.Text.Trim(),
            });
            TargetBox.Text = "";
            ReasonBox.Text = "";
            await LoadAsync();
        } catch (RpcException) {
            ShowStatus(AddFailedText);
        } finally {
            BusyBar.Visibility = Visibility.Collapsed;
        }
    }

    private async void OnRemoveClick(object sender, RoutedEventArgs e) {
        if (!CanModify) {
            ShowStatus(StoppedRequiredText);
            return;
        }
        if (sender is not FrameworkElement { Tag : BanEntryItem item })
            return;

        BusyBar.Visibility = Visibility.Visible;
        ClearStatus();
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Players.RemoveBanAsync(new RemoveBanRequest {
                ServerId = _serverId,
                Type     = item.Type,
                Target   = item.Target,
            });
            await LoadAsync();
        } catch (RpcException) {
            ShowStatus(RemoveFailedText);
        } finally {
            BusyBar.Visibility = Visibility.Collapsed;
        }
    }

    private void UpdateEmptyState() => EmptyHint.Visibility =
        Bans.Count == 0 ? Visibility.Visible : Visibility.Collapsed;

    private void ClearStatus() {
        foreach (var text in StatusElements)
            text.Visibility = Visibility.Collapsed;
    }

    private void ShowStatus(TextBlock status) {
        ClearStatus();
        status.Visibility = Visibility.Visible;
    }

    private IEnumerable<TextBlock>
        StatusElements => [LoadFailedText, TargetRequiredText, AddFailedText,
                           StoppedRequiredText, RemoveFailedText,
    ];
}
