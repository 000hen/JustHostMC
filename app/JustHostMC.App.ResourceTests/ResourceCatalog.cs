using System.Xml.Linq;

namespace JustHostMC.App.ResourceTests;

internal sealed record ResourceEntry(string Name, string Value, string Comment);

internal static class ResourceCatalog {
    public static IReadOnlyList<ResourceEntry> Load(string path) {
        var document = XDocument.Load(path, LoadOptions.SetLineInfo);
        return document.Root?.Elements("data")
                   .Select(element => new ResourceEntry(
                       (string?)element.Attribute("name") ?? "",
                       element.Element("value")?.Value ?? "",
                       element.Element("comment")?.Value ?? ""))
                   .ToArray()
               ?? [];
    }

    public static IReadOnlyList<string> DuplicateNames(
        IEnumerable<ResourceEntry> entries) =>
        entries.GroupBy(entry => entry.Name, StringComparer.OrdinalIgnoreCase)
            .Where(group => group.Count() > 1)
            .Select(group => group.Key)
            .Order(StringComparer.OrdinalIgnoreCase)
            .ToArray();
}
