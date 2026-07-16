using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class ModelLocalizationArchitectureTests {
    [Fact]
    public void BanEntryFiniteLabelsBelongToItsXamlTemplate() {
        var model = File.ReadAllText(RepositoryLayout.AppPath(
            "Models", "BanEntryItem.cs"));
        var xaml = File.ReadAllText(RepositoryLayout.AppPath(
            "Views", "BanListDialog.xaml"));

        Assert.DoesNotContain("ILocalizer", model, StringComparison.Ordinal);
        Assert.DoesNotContain("BanList_Type", model, StringComparison.Ordinal);
        Assert.DoesNotContain("BanList_NoReason", model, StringComparison.Ordinal);
        Assert.Contains("BanListEntryIpTypeText", xaml, StringComparison.Ordinal);
        Assert.Contains("BanListEntryPlayerTypeText", xaml,
                        StringComparison.Ordinal);
        Assert.Contains("BanListEntryNoReasonText", xaml,
                        StringComparison.Ordinal);
    }

    [Fact]
    public void ParserDiagnosticsAreNotFormattedIntoUserFacingModErrors() {
        var model = File.ReadAllText(RepositoryLayout.AppPath(
            "Models", "ModFileItem.cs"));

        Assert.DoesNotContain("(\"error\", metadata.ParseError)", model,
                              StringComparison.Ordinal);
    }
}
