using System;
using System.Globalization;

namespace JustHostMC.App.Models;

/// <summary>One timestamped automation log event within an application
/// session.</summary>
public sealed class ScriptLogEntry {
    private const int MaxTitleLength = 80;

    public ScriptLogEntry(string scriptId, string scriptName, string detail,
                          DateTimeOffset timestamp, string fallbackTitle) {
        ScriptId   = scriptId;
        ScriptName = scriptName;
        Detail     = detail;
        Timestamp  = timestamp;

        var title = detail.ReplaceLineEndings(" ").Trim();
        if (title.Length == 0)
            title = fallbackTitle;
        Title = title.Length > MaxTitleLength ? title[..MaxTitleLength] + "…"
                                              : title;
    }

    public string Title { get; }
    public string Detail { get; }
    public string ScriptName { get; }
    public string ScriptId { get; }
    public DateTimeOffset Timestamp { get; }
    public string TimestampFormatted =>
        Timestamp.ToLocalTime().ToString("G", CultureInfo.CurrentCulture);
}
