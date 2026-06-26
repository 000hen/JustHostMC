using JustHostMC.App.Services;
using JustHostMC.App.ViewModels;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Navigation;

namespace JustHostMC.App.Views;

/// <summary>Settings page: log retention, isolation backend (Docker opt-in), and the
/// destructive remove-all-data action. Hosts the shared <see cref="SettingsViewModel"/>.</summary>
public sealed partial class SettingsPage : Page
{
    public SettingsViewModel ViewModel { get; }

    public SettingsPage()
    {
        NavigationCacheMode = NavigationCacheMode.Required;
        ViewModel = new SettingsViewModel(DispatcherQueue, new LocalizationService());
        InitializeComponent();
        Loaded += async (_, _) => await ViewModel.LoadAsync();
    }
}
