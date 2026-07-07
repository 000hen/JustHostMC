using System;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>Display wrapper around a backup, with a human-readable size and
/// date.</summary>
public sealed class BackupItem {
    public BackupItem(Backup proto) {
        Id          = proto.Id;
        ServerId    = proto.ServerId;
        SizeText    = FormatSize(proto.SizeBytes);
        CreatedText = FormatDate(proto.CreatedAt);
    }

    public string Id { get; }
    public string ServerId { get; }
    public string SizeText { get; }
    public string CreatedText { get; }

    /// <summary>Reconstructs the proto needed to delete this backup.</summary>
    public Backup ToProto() => new() { Id = Id, ServerId = ServerId };

    private static string FormatSize(long bytes) {
        string[] units = { "B", "KB", "MB", "GB", "TB" };
        double value   = bytes;
        var unit       = 0;
        while (value >= 1024 && unit < units.Length - 1) {
            value /= 1024;
            unit++;
        }
        return $"{value:0.#} {units[unit]}";
    }

    private static string FormatDate(string rfc3339) =>
        DateTimeOffset.TryParse(rfc3339, out var dt)
            ? dt.LocalDateTime.ToString("g")
            : rfc3339;
}
