using System;
using System.Collections.ObjectModel;
using System.Globalization;

namespace JustHostMC.App.Models;

/// <summary>
/// Automation output captured during one application run, from launch until exit.
/// </summary>
public sealed class ScriptLogSession {
    public ScriptLogSession(string id, DateTimeOffset startedAt, string title) {
        Id = id;
        StartedAt = startedAt;
        Title = title;
    }

    public string Id { get; }
    public string Title { get; }
    public DateTimeOffset StartedAt { get; }
    public string StartedAtFormatted => StartedAt.ToLocalTime().ToString("G", CultureInfo.CurrentCulture);
    public ObservableCollection<ScriptLogEntry> Entries { get; } = new();
}
