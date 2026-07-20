using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class ShopBrowsingContractTests {
    [Fact]
    public void BrowseModeComesFromContextRatherThanMergedSourceCapabilities() {
        var shop =
            RepositoryLayout.ReadAppFile("ViewModels", "ShopViewModel.cs");
        var detail = RepositoryLayout.ReadAppFile("ViewModels",
                                                  "ShopDetailViewModel.cs");

        Assert.Contains(
            "public bool SelectedShopIsModpack => Context.Kind == ModKind.Modpack;",
            shop, StringComparison.Ordinal);
        Assert.DoesNotContain("SelectedShop?.Kinds.Contains(\"modpack\")", shop,
                              StringComparison.Ordinal);
        Assert.Contains("IsModpack    = shop.SelectedShopIsModpack;", detail,
                        StringComparison.Ordinal);
        Assert.DoesNotContain(
            "IsModpack    = shopInfo?.Kinds.Contains(\"modpack\")", detail,
            StringComparison.Ordinal);
    }
}
