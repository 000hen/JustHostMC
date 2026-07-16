using System.Xml.Linq;
using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class ConverterArchitectureTests {
    [Fact]
    public void AppReferencesCommunityToolkitConverters() {
        var project = XDocument.Load(
            RepositoryLayout.AppPath("JustHostMC.App.csproj"));

        var packageNames = project.Descendants("PackageReference")
            .Select(element => (string?)element.Attribute("Include"));

        Assert.Contains("CommunityToolkit.WinUI.Converters", packageNames);
    }

    [Fact]
    public void AppDoesNotRetainCustomBoolToVisibilityConverter() {
        var converterPath = RepositoryLayout.AppPath(
            "Converters", "BoolToVisibilityConverter.cs");

        Assert.False(
            File.Exists(converterPath),
            $"Replace the custom converter with CommunityToolkit.WinUI.Converters: {converterPath}");
    }
}
