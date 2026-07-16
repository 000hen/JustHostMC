using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class ShopLocalizationContractTests {
    [Fact]
    public void HomeSectionDescriptionUsesCatalogKeyConvention() {
        var source = File.ReadAllText(
            RepositoryLayout.AppPath("ViewModels", "ShopViewModel.cs"));

        Assert.Contains("var descriptionKey = $\"{titleKey}_description\";",
                        source);
    }
}
