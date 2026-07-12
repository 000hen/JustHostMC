using JustHostMC.Core;
using McManager.Grpc;
using Xunit;

namespace JustHostMC.Core.Tests;

public sealed class ShopPresentationPolicyTests {
    [Theory]
    [InlineData(ShopDistribution.Unknown, "", ShopPrimaryActionKind.Install,
                true)]
    [InlineData(ShopDistribution.Direct, "", ShopPrimaryActionKind.Install,
                true)]
    [InlineData(ShopDistribution.WebsiteOnly,
                "https://example.test/mod",
                ShopPrimaryActionKind.Website, true)]
    [InlineData(ShopDistribution.WebsiteOnly, "",
                ShopPrimaryActionKind.Website, false)]
    [InlineData(ShopDistribution.WebsiteOnly, "file:///tmp/mod",
                ShopPrimaryActionKind.Website, false)]
    public void DeterminePrimaryActionMapsPolicy(
        ShopDistribution distribution, string website,
        ShopPrimaryActionKind kind, bool enabled) {
        var result = ShopPresentationPolicy.DeterminePrimaryAction(
            distribution, website);

        Assert.Equal(kind, result.Kind);
        Assert.Equal(enabled, result.IsEnabled);
    }

    [Fact]
    public void ResolveCategoryLabelUsesLocalizedValue() {
        var category = new ShopCategory {
            Name            = "Technology",
            LocalizationKey = "shop.category.curseforge.technology",
        };

        var result = ShopPresentationPolicy.ResolveCategoryLabel(
            category, _ => "科技");

        Assert.Equal("科技", result);
    }

    [Fact]
    public void ResolveCategoryLabelFallsBackWhenKeyIsMissing() {
        var category = new ShopCategory {
            Name            = "Technology",
            LocalizationKey = "missing.key",
        };

        var result = ShopPresentationPolicy.ResolveCategoryLabel(
            category, key => key);

        Assert.Equal("Technology", result);
    }
}
