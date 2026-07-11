using System.Collections.ObjectModel;
using System.Collections.Specialized;
using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;
using Windows.ApplicationModel.DataTransfer;

namespace JustHostMC.App.Views;

/// <summary>The Home page: a grid of server cards plus an "add" card, with live
/// install progress. Shares the shell's <see cref="MainViewModel"/>.</summary>
public sealed partial class HomePage : Page {
    private readonly FirstRunService _firstRun = new();
    private readonly ILocalizer _localizer     = new LocalizationService();
    private readonly AddCard _addCard          = new();
    private NavShellViewModel _shell           = null!;

    public HomePage() => InitializeComponent();

    public MainViewModel Main { get; private set; } = null!;

    /// <summary>Server cards followed by the trailing add card.</summary>
    public ObservableCollection<object> Cards { get; } = new();

    protected override void OnNavigatedTo(NavigationEventArgs e) {
        _shell = (NavShellViewModel)e.Parameter;
        Main   = _shell.Main;

        OnMachineInfoBar.IsOpen = _firstRun.ShouldShowOnMachineNotice();
        Main.Servers.CollectionChanged += OnServersChanged;
        RebuildCards();
        Bindings.Update();
    }

    protected override void OnNavigatedFrom(NavigationEventArgs e) {
        Main.Servers.CollectionChanged -= OnServersChanged;
    }

    private void OnServersChanged(
        object? sender, NotifyCollectionChangedEventArgs e) => RebuildCards();

    private void RebuildCards() {
        Cards.Clear();
        foreach (var server in Main.Servers) Cards.Add(server);
        Cards.Add(_addCard);
    }

    private void OnCardOpenClick(object sender, RoutedEventArgs e) {
        if (TryGetServerItem(sender, out var item))
            _shell.RequestOpenServer(item);
    }

    private async void OnCardRenameClick(object sender, RoutedEventArgs e) {
        if (TryGetServerItem(sender, out var item))
            await ShowRenameDialogAsync(item);
    }

    private async void OnCardEditClick(object sender, RoutedEventArgs e) {
        if (TryGetServerItem(sender, out var item))
            await ShowEditDialogAsync(item);
    }

    private void OnCardStateClick(object sender, RoutedEventArgs e) {
        if (!TryGetServerItem(sender, out var item))
            return;

        if (item.CanStart)
            Main.StartServerCommand.Execute(item);
        else if (item.CanStop)
            Main.StopServerCommand.Execute(item);
    }

    private async void OnCardMoveUpClick(object sender, RoutedEventArgs e) {
        if (TryGetServerItem(sender, out var item))
            await Main.MoveServerAsync(item, -1);
    }

    private async void OnCardMoveDownClick(object sender, RoutedEventArgs e) {
        if (TryGetServerItem(sender, out var item))
            await Main.MoveServerAsync(item, 1);
    }

    private async void OnCardDeleteClick(object sender, RoutedEventArgs e) {
        if (!TryGetServerItem(sender, out var item))
            return;

        var confirm = new ContentDialog {
            XamlRoot          = XamlRoot,
            Title             = _localizer.Get("ServerDelete_Title"),
            Content           = _localizer.Get("ServerDelete_Body"),
            PrimaryButtonText = _localizer.Get("ServerDelete_Confirm"),
            CloseButtonText   = _localizer.Get("Common_Cancel"),
            DefaultButton     = ContentDialogButton.Close,
        };
        if (await confirm.ShowAsync() == ContentDialogResult.Primary)
            Main.DeleteServerCommand.Execute(item);
    }

    private void OnCardCopyAddressClick(object sender, RoutedEventArgs e) {
        if (!TryGetServerItem(sender, out var item))
            return;

        var package = new DataPackage();
        package.SetText(item.EndpointText);
        Clipboard.SetContent(package);
    }

    private async void OnAddCardClick(object sender, RoutedEventArgs e) {
        var content = new ServerDialog(Main, ServerDialogMode.Create);
        var dialog  = new ContentDialog {
            XamlRoot = XamlRoot,
            Style    = Application.Current
                           .Resources["DefaultContentDialogStyle"] as Style,
            Title    = _localizer.Get("CreateServerDialog_Title"),
            Content  = content,
            PrimaryButtonText =
                _localizer.Get("CreateServerDialog_PrimaryButtonText"),
            CloseButtonText =
                _localizer.Get("CreateServerDialog_CloseButtonText"),
            DefaultButton          = ContentDialogButton.Primary,
            IsPrimaryButtonEnabled = content.CanSubmit,
        };
        content.CanSubmitChanged += (_, _) => dialog.IsPrimaryButtonEnabled =
            content.CanSubmit;
        ContentDialogSizing.Apply(dialog);

        if (await dialog.ShowAsync() != ContentDialogResult.Primary)
            return;
        if (content.BuildCreateRequest() is {} request)
            await Main.InstallServerAsync(request);
    }

    private void OnMachineNoticeClosed(InfoBar sender, object args) =>
        _firstRun.MarkOnMachineNoticeShown();

    private async Task ShowEditDialogAsync(ServerItem item) {
        var content = new ServerDialog(Main, ServerDialogMode.Edit, item);
        var dialog  = new ContentDialog {
            XamlRoot = XamlRoot,
            Style    = Application.Current
                           .Resources["DefaultContentDialogStyle"] as Style,
            Title    = _localizer.Get("EditServerDialog_Title"),
            Content  = content,
            PrimaryButtonText =
                _localizer.Get("EditServerDialog_PrimaryButtonText"),
            CloseButtonText =
                _localizer.Get("EditServerDialog_CloseButtonText"),
            DefaultButton          = ContentDialogButton.Primary,
            IsPrimaryButtonEnabled = content.CanSubmit,
        };
        content.CanSubmitChanged += (_, _) => dialog.IsPrimaryButtonEnabled =
            content.CanSubmit;
        ContentDialogSizing.Apply(dialog);

        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
            await Main.UpdateServerAsync(content.BuildUpdateRequest());
    }

    private async Task ShowRenameDialogAsync(ServerItem item) {
        var nameBox = new TextBox {
            Text            = item.Name,
            Header          = _localizer.Get("EditServerName_Header"),
            SelectionStart  = 0,
            SelectionLength = item.Name.Length,
        };
        var dialog = new ContentDialog {
            XamlRoot          = XamlRoot,
            Title             = _localizer.Get("RenameServerDialog_Title"),
            Content           = nameBox,
            PrimaryButtonText = _localizer.Get("Common_Save"),
            CloseButtonText   = _localizer.Get("Common_Cancel"),
            DefaultButton     = ContentDialogButton.Primary,
        };
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
            await Main.RenameServerAsync(item, nameBox.Text);
    }

    private static bool TryGetServerItem(object sender, out ServerItem item) {
        if (sender is FrameworkElement { Tag : ServerItem tagged }) {
            item = tagged;
            return true;
        }
        if (sender is
            FrameworkElement { DataContext : ServerItem dataContext }) {
            item = dataContext;
            return true;
        }

        item = null!;
        return false;
    }
}
