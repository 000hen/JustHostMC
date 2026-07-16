using Xunit;

namespace JustHostMC.App.ResourceTests;

public sealed class ResourceCatalogTests {
    [Fact]
    public void LoadRejectsMalformedXml() {
        var path = WriteTemp("<root><data></root>");
        try {
            Assert.ThrowsAny<Exception>(() => ResourceCatalog.Load(path));
        } finally {
            File.Delete(path);
        }
    }

    [Fact]
    public void DuplicateNamesAreCaseInsensitive() {
        var entries = new[] {
            new ResourceEntry("Example.Text", "One", ""),
            new ResourceEntry("example.text", "Two", ""),
            new ResourceEntry("Other.Text", "Three", ""),
        };

        Assert.Equal(["Example.Text"], ResourceCatalog.DuplicateNames(entries));
    }

    private static string WriteTemp(string content) {
        var path = Path.GetTempFileName();
        File.WriteAllText(path, content);
        return path;
    }
}
