using JustHostMC.App.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

/// <summary>Picks the Home grid template: a server card vs. the trailing "add"
/// card.</summary>
public sealed partial class HomeCardTemplateSelector : DataTemplateSelector {
    public DataTemplate? ServerTemplate { get; set; }
    public DataTemplate? AddTemplate { get; set; }

    protected override DataTemplate? SelectTemplateCore(object item) =>
        item is AddCard ? AddTemplate : ServerTemplate;

    protected override DataTemplate? SelectTemplateCore(
        object item, DependencyObject container) => SelectTemplateCore(item);
}
