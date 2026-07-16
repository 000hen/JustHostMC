using JustHostMC.App.Models;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls;

public sealed partial class InventoryItemDetails : UserControl {
    public InventoryItemDetails(PlayerInventoryItemView item) {
        InitializeComponent();
        DataContext = item;
    }
}
