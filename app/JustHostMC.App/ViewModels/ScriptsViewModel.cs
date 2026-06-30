using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using Google.Protobuf;
using Grpc.Core;
using JustHostMC.App.Models;
using JustHostMC.App.Services;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>
/// Lists installed Lua providers and automation scripts, imports user scripts
/// (with a permission-consent step handled by the page), toggles automation
/// scripts on/off, removes them, and streams the engine-wide automation log.
/// The runtime automation engine ships separately, so every call degrades
/// gracefully (status messages) if the engine returns an error.
/// </summary>
public sealed class ScriptsViewModel : ObservableObject, IAsyncDisposable
{
    private const int MaxLogLines = 2000;
    private const int MaxLineLength = 2000;

    private readonly DispatcherQueue _dispatcher;
    private readonly ILocalizer _localizer;

    private CancellationTokenSource? _logCts;
    private bool _loaded;

    public ScriptsViewModel(DispatcherQueue dispatcher, ILocalizer localizer)
    {
        _dispatcher = dispatcher;
        _localizer = localizer;
    }

    public ObservableCollection<ProviderItem> Providers { get; } = new();
    public ObservableCollection<ScriptItem> Scripts { get; } = new();
    public ObservableCollection<string> LogLines { get; } = new();

    private bool _isBusy;
    public bool IsBusy
    {
        get => _isBusy;
        private set => SetProperty(ref _isBusy, value);
    }

    private string _statusMessage = "";
    public string StatusMessage
    {
        get => _statusMessage;
        private set => SetProperty(ref _statusMessage, value);
    }

    /// <summary>Sets a localized status message (used by the page for picker/IO errors).
    /// Marshals to the UI thread.</summary>
    public void SetStatus(string message) => RunOnUI(() => StatusMessage = message);

    /// <summary>Loads providers + scripts once, then starts the log stream.</summary>
    public async Task EnsureLoadedAsync()
    {
        if (_loaded)
            return;
        _loaded = true;
        await RefreshAsync();
        StartLogStream();
    }

    public async Task RefreshAsync()
    {
        RunOnUI(() => IsBusy = true);
        try
        {
            var daemon = await App.Current.DaemonReady;
            await RefreshProvidersAsync(daemon);
            await RefreshScriptsAsync(daemon);
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    private async Task RefreshProvidersAsync(JustHostMC.Core.DaemonClient daemon)
    {
        try
        {
            var list = await daemon.Providers.ListAsync(new Empty());
            RunOnUI(() =>
            {
                Providers.Clear();
                foreach (var p in list.Providers)
                    Providers.Add(new ProviderItem(p, _localizer));
            });
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
    }

    private async Task RefreshScriptsAsync(JustHostMC.Core.DaemonClient daemon)
    {
        try
        {
            var list = await daemon.Scripts.ListAsync(new Empty());
            RunOnUI(() =>
            {
                Scripts.Clear();
                foreach (var s in list.Scripts)
                    Scripts.Add(new ScriptItem(s, _localizer));
            });
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
    }

    /// <summary>Imports a provider script (+ optional jar) with the granted permissions.</summary>
    public async Task ImportProviderAsync(
        string luaSource, byte[]? jar, string? jarFilename, IReadOnlyList<PermissionKind> granted)
    {
        RunOnUI(() => { IsBusy = true; StatusMessage = ""; });
        try
        {
            var daemon = await App.Current.DaemonReady;
            var req = new ImportProviderRequest { LuaSource = luaSource };
            if (jar is { Length: > 0 })
            {
                req.Jar = ByteString.CopyFrom(jar);
                req.JarFilename = jarFilename ?? "";
            }
            var info = await daemon.Providers.ImportAsync(req);

            // Persist the user's consent choices (the imported provider defaults to
            // requesting everything; narrow it to what the user allowed).
            await SetProviderPermissionsAsync(daemon, info.Id, granted);
            await RefreshProvidersAsync(daemon);
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    /// <summary>Imports an automation script with the granted permissions.</summary>
    public async Task ImportScriptAsync(string luaSource, IReadOnlyList<PermissionKind> granted)
    {
        RunOnUI(() => { IsBusy = true; StatusMessage = ""; });
        try
        {
            var daemon = await App.Current.DaemonReady;
            var info = await daemon.Scripts.ImportAsync(new ImportScriptRequest { LuaSource = luaSource });
            await SetScriptPermissionsAsync(daemon, info.Id, granted);
            await RefreshScriptsAsync(daemon);
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
        finally
        {
            RunOnUI(() => IsBusy = false);
        }
    }

    public async Task SetScriptEnabledAsync(ScriptItem item, bool enabled)
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Scripts.SetEnabledAsync(new SetScriptEnabledRequest { Id = item.Id, Enabled = enabled });
            // Record the new known state so the page handler treats later identical
            // Toggled events as no-ops.
            item.Enabled = enabled;
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
            // Re-sync so the toggle reflects the engine's actual state.
            await RefreshAsync();
        }
    }

    public async Task RemoveProviderAsync(ProviderItem item)
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Providers.RemoveAsync(new ProviderRef { Id = item.Id });
            await RefreshProvidersAsync(daemon);
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
    }

    public async Task RemoveScriptAsync(ScriptItem item)
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            await daemon.Scripts.RemoveAsync(new ProviderRef { Id = item.Id });
            await RefreshScriptsAsync(daemon);
        }
        catch (RpcException ex)
        {
            RunOnUI(() => StatusMessage = _localizer.Get(MapErrorKey(ex)));
        }
    }

    private static async Task SetProviderPermissionsAsync(
        JustHostMC.Core.DaemonClient daemon, string id, IReadOnlyList<PermissionKind> granted)
    {
        var req = new SetPermissionsRequest { Id = id };
        req.Granted.AddRange(granted);
        await daemon.Providers.SetPermissionsAsync(req);
    }

    private static async Task SetScriptPermissionsAsync(
        JustHostMC.Core.DaemonClient daemon, string id, IReadOnlyList<PermissionKind> granted)
    {
        var req = new SetPermissionsRequest { Id = id };
        req.Granted.AddRange(granted);
        await daemon.Scripts.SetPermissionsAsync(req);
    }

    private void StartLogStream()
    {
        _logCts = new CancellationTokenSource();
        _ = LogLoopAsync(_logCts.Token);
    }

    private async Task LogLoopAsync(CancellationToken token)
    {
        try
        {
            var daemon = await App.Current.DaemonReady;
            using var call = daemon.Scripts.StreamLog(new Empty(), cancellationToken: token);
            await foreach (var ev in call.ResponseStream.ReadAllAsync(token).ConfigureAwait(false))
            {
                var prefix = string.IsNullOrEmpty(ev.ScriptId) ? "" : $"[{ev.ScriptId}] ";
                var line = prefix + ev.Line;
                RunOnUI(() => AppendLogLine(line));
            }
        }
        catch (OperationCanceledException)
        {
        }
        catch (RpcException)
        {
            // The runtime automation engine may not be available yet; the rest of
            // the page (import/list/remove) still works.
        }
    }

    private void AppendLogLine(string line)
    {
        if (line.Length > MaxLineLength)
            line = line[..MaxLineLength] + "…";
        LogLines.Add(line);
        while (LogLines.Count > MaxLogLines)
            LogLines.RemoveAt(0);
    }

    private static string MapErrorKey(RpcException ex) => ex.StatusCode switch
    {
        StatusCode.InvalidArgument => "Scripts_ImportInvalid",
        StatusCode.AlreadyExists => "Scripts_AlreadyExists",
        StatusCode.FailedPrecondition => "Scripts_BuiltinProtected",
        StatusCode.Unimplemented => "Scripts_NotAvailable",
        _ => "Scripts_OperationFailed",
    };

    private void RunOnUI(Action action)
    {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }

    public async ValueTask DisposeAsync()
    {
        try
        {
            _logCts?.Cancel();
        }
        catch
        {
            // Best-effort teardown.
        }
        finally
        {
            _logCts?.Dispose();
        }
        await Task.CompletedTask;
    }
}
