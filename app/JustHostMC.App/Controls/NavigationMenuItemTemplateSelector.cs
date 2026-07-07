using JustHostMC.App.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>Selects the static Home row or a data-bound server row in the
/// navigation menu.</summary>
public sealed partial class NavigationMenuItemTemplateSelector
    : DataTemplateSelector {
    public DataTemplate? HomeTemplate { get; set; }
    public DataTemplate? ServerTemplate { get; set; }
    public DataTemplate? AddServerTemplate { get; set; }
    public DataTemplate? ScriptsTemplate { get; set; }
    public DataTemplate? SettingsTemplate { get; set; }

    protected override DataTemplate? SelectTemplateCore(object item) =>
        item switch {
            ServerItem                      => ServerTemplate,
            NavigationDestination.AddServer => AddServerTemplate,
            NavigationDestination.Scripts   => ScriptsTemplate,
            NavigationDestination.Settings  => SettingsTemplate,
            _                               => HomeTemplate,
        };

    protected override DataTemplate? SelectTemplateCore(
        object item, DependencyObject container) => SelectTemplateCore(item);
}
