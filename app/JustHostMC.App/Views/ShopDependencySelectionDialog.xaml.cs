using McManager.Grpc;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class ShopDependencySelectionDialog : ContentDialog {
    private readonly IReadOnlyList<CheckBox> _choices;

    public ShopDependencySelectionDialog(
        IReadOnlyList<ShopDependency> dependencies) {
        InitializeComponent();

        _choices = dependencies
                       .Select(dependency => new CheckBox {
                           Content   = dependency.Title.Length > 0
                                           ? dependency.Title
                                           : dependency.ProjectId,
                           IsChecked = true,
                           Tag       = dependency,
                       })
                       .ToArray();
        foreach (var choice in _choices) ChoicesPanel.Children.Add(choice);
    }

    public IReadOnlyList<ShopDependency> SelectedDependencies =>
        _choices.Where(choice => choice.IsChecked == true)
            .Select(choice => (ShopDependency)choice.Tag)
            .ToArray();
}
