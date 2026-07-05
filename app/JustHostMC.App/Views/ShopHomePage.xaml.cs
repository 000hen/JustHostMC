using JustHostMC.App.Models;
using JustHostMC.App.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace JustHostMC.App.Views;

/// <summary>Shop landing page: trending / most-downloaded sections.</summary>
public sealed partial class ShopHomePage : Page
{
    private ShopWindow? _window;

    public ShopViewModel ViewModel { get; private set; } = null!;

    public IReadOnlyList<int> SkeletonItems { get; } = [0, 1, 2, 3, 4, 5];

    public ShopHomePage()
    {
        InitializeComponent();
    }

    protected override void OnNavigatedTo(NavigationEventArgs e)
    {
        base.OnNavigatedTo(e);
        var args = (ShopNavArgs)e.Parameter;
        _window = args.Window;
        ViewModel = args.Shop;
        Bindings.Update();
        if (ViewModel.HomeSections.Count == 0)
            _ = ViewModel.LoadHomeAsync();
    }

    private void OnProjectClick(object sender, RoutedEventArgs e)
    {
        if (sender is FrameworkElement { Tag: ShopProjectItem item })
            _window?.ShowProject(item);
    }

    private void OnFilterChanged(object sender, RoutedEventArgs e) =>
        _ = ViewModel.LoadHomeAsync();

    private void OnDismissWelcome(object sender, RoutedEventArgs e) =>
        WelcomeBanner.Visibility = Visibility.Collapsed;

    public Visibility HasStatus(string status) =>
        status.Length > 0 ? Visibility.Visible : Visibility.Collapsed;

    public static Visibility InvertVisibility(bool value) =>
        value ? Visibility.Collapsed : Visibility.Visible;

    public Visibility LoadingVisibility(bool value) =>
        value ? Visibility.Visible : Visibility.Collapsed;
}
