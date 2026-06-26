namespace JustHostMC.App.Models;

/// <summary>A plugin/mod jar shown in the server page's Plugins/Mods panel.</summary>
public sealed class ModFileItem
{
    public ModFileItem(string name, long sizeBytes)
    {
        Name = name;
        SizeBytes = sizeBytes;
    }

    public string Name { get; }
    public long SizeBytes { get; }

    public string SizeText => SizeBytes switch
    {
        >= 1 << 20 => $"{SizeBytes / (double)(1 << 20):0.0} MB",
        >= 1 << 10 => $"{SizeBytes / (double)(1 << 10):0.0} KB",
        _ => $"{SizeBytes} B",
    };
}
