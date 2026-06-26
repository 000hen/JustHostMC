using System.Collections.ObjectModel;
using Grpc.Core;
using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>Modal editor for vanilla banned-players.json and banned-ips.json.</summary>
public sealed partial class BanListDialog : FluentContentDialog
{
    private readonly string _serverId;
    private readonly ILocalizer _localizer = new LocalizationService();

    public ObservableCollection<BanEntryItem> Bans { get; } = new();
    public bool CanModify { get; }
    public string StoppedNoticeTitle { get; }
    public string StoppedNoticeMessage { get; }

    public BanListDialog(string serverId, bool canModify)
    {
        _serverId = serverId;
        CanModify = canModify;
        StoppedNoticeTitle = _localizer.Get("BanListStoppedNotice_Title");
        StoppedNoticeMessage = _localizer.Get("BanListStoppedNotice_Message");
        InitializeComponent();
        Title = _localizer.Get("BanListDialog_Title");
        Opened += async (_, _) => await LoadAsync();
    }

    private async Task LoadAsync()
    {
        BusyBar.Visibility = Visibility.Visible;
        StatusText.Text = "";
        try
        {
            var daemon = await App.Current.DaemonReady;
            var list = await daemon.Players.ListBansAsync(new ServerId { Id = _serverId });
            Bans.Clear();
            foreach (var entry in list.Entries)
                Bans.Add(new BanEntryItem(entry, _localizer));
            UpdateEmptyState();
        }
        catch (RpcException ex)
        {
            StatusText.Text = ex.Status.Detail.Length > 0 ? ex.Status.Detail : _localizer.Get("BanList_LoadFailed");
        }
        finally
        {
            BusyBar.Visibility = Visibility.Collapsed;
        }
    }

    private async void OnAddClick(object sender, RoutedEventArgs e)
    {
        if (!CanModify)
            return;
        var target = TargetBox.Text.Trim();
        if (target.Length == 0)
        {
            StatusText.Text = _localizer.Get("BanList_TargetRequired");
            return;
        }

        BusyBar.Visibility = Visibility.Visible;
        StatusText.Text = "";
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Players.AddBanAsync(new AddBanRequest
            {
                ServerId = _serverId,
                Type = TypeBox.SelectedIndex == 1 ? BanListType.IpBans : BanListType.PlayerBans,
                Target = target,
                Reason = ReasonBox.Text.Trim(),
            });
            TargetBox.Text = "";
            ReasonBox.Text = "";
            await LoadAsync();
        }
        catch (RpcException ex)
        {
            StatusText.Text = ex.Status.Detail.Length > 0 ? ex.Status.Detail : _localizer.Get("BanList_AddFailed");
        }
        finally
        {
            BusyBar.Visibility = Visibility.Collapsed;
        }
    }

    private async void OnRemoveClick(object sender, RoutedEventArgs e)
    {
        if (!CanModify)
        {
            StatusText.Text = _localizer.Get("BanList_StoppedRequired");
            return;
        }
        if (sender is not FrameworkElement { Tag: BanEntryItem item })
            return;

        BusyBar.Visibility = Visibility.Visible;
        StatusText.Text = "";
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Players.RemoveBanAsync(new RemoveBanRequest
            {
                ServerId = _serverId,
                Type = item.Type,
                Target = item.Target,
            });
            await LoadAsync();
        }
        catch (RpcException ex)
        {
            StatusText.Text = ex.Status.Detail.Length > 0 ? ex.Status.Detail : _localizer.Get("BanList_RemoveFailed");
        }
        finally
        {
            BusyBar.Visibility = Visibility.Collapsed;
        }
    }

    private void UpdateEmptyState() => EmptyHint.Visibility = Bans.Count == 0 ? Visibility.Visible : Visibility.Collapsed;
}
