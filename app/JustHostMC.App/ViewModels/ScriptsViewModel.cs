using CommunityToolkit.Mvvm.ComponentModel;
using Google.Protobuf;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;
using System.Collections.ObjectModel;
using System.Globalization;

namespace JustHostMC.App.ViewModels;

/// <summary>
/// Lists installed Lua providers and automation scripts, imports user scripts
/// (with a permission-consent step handled by the page), toggles automation
/// scripts on/off, removes them, and streams the engine-wide automation log.
/// The runtime automation engine ships separately, so every call degrades
/// gracefully (status messages) if the engine returns an error.
/// </summary>
public sealed partial class ScriptsViewModel : ObservableObject, IAsyncDisposable {
    private const int MaxLogLines = 2000;
    private const int MaxPerScriptLogLines = 500;
    private const int MaxLineLength = 2000;

    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;
    private readonly Dictionary<string, List<string>> _logsByScript = new(StringComparer.Ordinal);

    private CancellationTokenSource? _logCts;
    private bool _loaded;

    public ScriptsViewModel(DispatcherQueue dispatcher, ILocalizer localizer) {
        _dispatcher = dispatcher;
        _localizer = localizer;
    }

    public ObservableCollection<ProviderItem> Providers { get; } = new();
    public ObservableCollection<ScriptItem> Scripts { get; } = new();
    public ObservableCollection<ScriptLogEntry> LogEntries { get; } = new();

    [ObservableProperty]
    public partial bool IsBusy { get; private set; }

    [ObservableProperty]
    [NotifyPropertyChangedFor(nameof(HasStatusMessage))]
    public partial string StatusMessage { get; private set; } = "";

    public bool HasStatusMessage => !string.IsNullOrWhiteSpace(StatusMessage);

    /// <summary>Sets a localized status message (used by the page for picker/IO errors).
    /// Marshals to the UI thread.</summary>
    public void SetStatus(string message) => RunOnUI(() => StatusMessage = message);

    /// <summary>Loads providers + scripts once, then starts the log stream.</summary>
    public async Task EnsureLoadedAsync() {
        if (_loaded)
            return;
        _loaded = true;
        await RefreshAsync();
        StartLogStream();
    }

    public async Task RefreshAsync() {
        RunOnUI(() => IsBusy = true);
        try {
            var daemon = await App.Current.DaemonReady;
            await RefreshProvidersAsync(daemon);
            await RefreshScriptsAsync(daemon);
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    private async Task RefreshProvidersAsync(JustHostMC.Core.DaemonClient daemon) {
        try {
            var list = await daemon.Providers.ListAsync(new Empty());
            RunOnUI(() => {
                Providers.Clear();
                foreach (var p in list.Providers)
                    Providers.Add(new ProviderItem(p, _localizer));
            });
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
    }

    private async Task RefreshScriptsAsync(JustHostMC.Core.DaemonClient daemon) {
        try {
            var list = await daemon.Scripts.ListAsync(new Empty());
            RunOnUI(() => {
                Scripts.Clear();
                foreach (var s in list.Scripts) {
                    var item = new ScriptItem(s, _localizer);
                    if (_logsByScript.TryGetValue(item.Id, out var lines)) {
                        foreach (var line in lines)
                            item.LogLines.Add(line);
                    }
                    Scripts.Add(item);
                }
            });
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
    }

    /// <summary>Imports a provider script (+ optional jar) with the granted permissions.</summary>
    public async Task ImportProviderAsync(
        string luaSource, byte[]? jar, string? jarFilename, IReadOnlyList<PermissionKind> granted) {
        RunOnUI(() => { IsBusy = true; StatusMessage = ""; });
        try {
            var daemon = await App.Current.DaemonReady;
            var req = new ImportProviderRequest { LuaSource = luaSource };
            if (jar is { Length: > 0 }) {
                req.Jar = ByteString.CopyFrom(jar);
                req.JarFilename = jarFilename ?? "";
            }
            var info = await daemon.Providers.ImportAsync(req);

            // Persist the user's consent choices (the imported provider defaults to
            // requesting everything; narrow it to what the user allowed).
            await SetProviderPermissionsAsync(daemon, info.Id, granted);
            await RefreshProvidersAsync(daemon);
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    /// <summary>Imports an automation script with the granted permissions.</summary>
    public async Task ImportScriptAsync(string luaSource, IReadOnlyList<PermissionKind> granted) {
        RunOnUI(() => { IsBusy = true; StatusMessage = ""; });
        try {
            var daemon = await App.Current.DaemonReady;
            var info = await daemon.Scripts.ImportAsync(new ImportScriptRequest { LuaSource = luaSource });
            await SetScriptPermissionsAsync(daemon, info.Id, granted);
            await RefreshScriptsAsync(daemon);
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        } finally {
            RunOnUI(() => IsBusy = false);
        }
    }

    public async Task SetScriptEnabledAsync(ScriptItem item, bool enabled) {
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Scripts.SetEnabledAsync(new SetScriptEnabledRequest { Id = item.Id, Enabled = enabled });
            // Record the new known state so the page handler treats later identical
            // Toggled events as no-ops.
            item.Enabled = enabled;
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
            // Re-sync so the toggle reflects the engine's actual state.
            await RefreshAsync();
        }
    }

    public async Task RemoveProviderAsync(ProviderItem item) {
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Providers.RemoveAsync(new ProviderRef { Id = item.Id });
            await RefreshProvidersAsync(daemon);
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
    }

    public async Task RemoveScriptAsync(ScriptItem item) {
        try {
            var daemon = await App.Current.DaemonReady;
            await daemon.Scripts.RemoveAsync(new ProviderRef { Id = item.Id });
            await RefreshScriptsAsync(daemon);
        } catch (RpcException ex) {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
    }

    private static async Task SetProviderPermissionsAsync(
        JustHostMC.Core.DaemonClient daemon, string id, IReadOnlyList<PermissionKind> granted) {
        var req = new SetPermissionsRequest { Id = id };
        req.Granted.AddRange(granted);
        await daemon.Providers.SetPermissionsAsync(req);
    }

    private static async Task SetScriptPermissionsAsync(
        JustHostMC.Core.DaemonClient daemon, string id, IReadOnlyList<PermissionKind> granted) {
        var req = new SetPermissionsRequest { Id = id };
        req.Granted.AddRange(granted);
        await daemon.Scripts.SetPermissionsAsync(req);
    }

    private void StartLogStream() {
        _logCts = new CancellationTokenSource();
        _ = LogLoopAsync(_logCts.Token);
    }

    private async Task LogLoopAsync(CancellationToken token) {
        try {
            var daemon = await App.Current.DaemonReady;
            using var call = daemon.Scripts.StreamLog(new Empty(), cancellationToken: token);
            await foreach (var ev in call.ResponseStream.ReadAllAsync(token).ConfigureAwait(false)) {
                var timestamp = ParseLogTimestamp(ev.Timestamp);
                RunOnUI(() => AppendLogLine(ev.ScriptId, ev.Line, timestamp));
            }
        } catch (OperationCanceledException) {
        } catch (RpcException) {
            // The runtime automation engine may not be available yet; the rest of
            // the page (import/list/remove) still works.
        }
    }

    private void AppendLogLine(string scriptId, string line, DateTimeOffset timestamp) {
        if (line.Length > MaxLineLength)
            line = line[..MaxLineLength] + "…";

        var scriptName = Scripts.FirstOrDefault(item => item.Id == scriptId)?.Name;
        if (string.IsNullOrEmpty(scriptName)) {
            scriptName = string.IsNullOrEmpty(scriptId)
                ? _localizer.Get("Scripts_SystemLogName")
                : scriptId;
        }
        var displayId = string.IsNullOrEmpty(scriptId) ? "—" : scriptId;
        LogEntries.Add(new ScriptLogEntry(
            displayId,
            scriptName,
            line,
            timestamp,
            _localizer.Get("Scripts_LogEntryFallbackTitle")));
        while (LogEntries.Count > MaxLogLines)
            LogEntries.RemoveAt(0);

        if (string.IsNullOrEmpty(scriptId))
            return;

        if (!_logsByScript.TryGetValue(scriptId, out var scriptLines)) {
            scriptLines = new List<string>();
            _logsByScript[scriptId] = scriptLines;
        }
        scriptLines.Add(line);
        while (scriptLines.Count > MaxPerScriptLogLines)
            scriptLines.RemoveAt(0);

        var script = Scripts.FirstOrDefault(item => item.Id == scriptId);
        if (script is null)
            return;

        script.LogLines.Add(line);
        while (script.LogLines.Count > MaxPerScriptLogLines)
            script.LogLines.RemoveAt(0);
    }

    private static DateTimeOffset ParseLogTimestamp(string value) =>
        DateTimeOffset.TryParse(
            value,
            CultureInfo.InvariantCulture,
            DateTimeStyles.AssumeUniversal | DateTimeStyles.AdjustToUniversal,
            out var timestamp)
            ? timestamp
            : DateTimeOffset.UtcNow;

    private static string MapErrorKey(RpcException ex) => ex.StatusCode switch {
        StatusCode.InvalidArgument => "Scripts_ImportInvalid",
        StatusCode.AlreadyExists => "Scripts_AlreadyExists",
        StatusCode.FailedPrecondition => "Scripts_BuiltinProtected",
        StatusCode.Unimplemented => "Scripts_NotAvailable",
        _ => "Scripts_OperationFailed",
    };

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }

    public async ValueTask DisposeAsync() {
        try {
            _logCts?.Cancel();
        } catch {
            // Best-effort teardown.
        } finally {
            _logCts?.Dispose();
        }
        await Task.CompletedTask;
    }
}
