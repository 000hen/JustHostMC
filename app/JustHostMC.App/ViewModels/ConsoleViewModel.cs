using System;
using System.Collections.ObjectModel;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using Grpc.Core;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>
/// Drives one server's live console over the bidirectional ConsoleService stream.
/// Output lines are appended to a bounded buffer, marshaled to the UI thread.
/// </summary>
public partial class ConsoleViewModel : ObservableObject, IAsyncDisposable
{
    private const int MaxLines = 2000;
    private const int MaxLineLength = 2000;

    private readonly string _serverId;
    private readonly DispatcherQueue _dispatcher;

    private AsyncDuplexStreamingCall<ConsoleInput, ConsoleEvent>? _call;
    private CancellationTokenSource? _cts;

    public ObservableCollection<string> Lines { get; } = new();

    [ObservableProperty]
    public partial string ServerName { get; set; }

    [ObservableProperty]
    public partial string CommandText { get; set; } = "";

    public ConsoleViewModel(string serverId, string serverName, DispatcherQueue dispatcher)
    {
        _serverId = serverId;
        ServerName = serverName;
        _dispatcher = dispatcher;
    }

    /// <summary>Opens the stream, subscribes to the server, and starts reading output.</summary>
    public async Task AttachAsync()
    {
        var daemon = await App.Current.DaemonReady;
        _cts = new CancellationTokenSource();
        _call = daemon.Console.Attach(cancellationToken: _cts.Token);

        // The first message selects which server's console to attach to.
        await _call.RequestStream.WriteAsync(new ConsoleInput { ServerId = _serverId });

        _ = ReadLoopAsync(_call, _cts.Token);
    }

    private async Task ReadLoopAsync(AsyncDuplexStreamingCall<ConsoleInput, ConsoleEvent> call, CancellationToken token)
    {
        try
        {
            await foreach (var ev in call.ResponseStream.ReadAllAsync(token).ConfigureAwait(false))
            {
                var line = ev.Line;
                RunOnUI(() => AppendLine(line));
            }
        }
        catch (OperationCanceledException)
        {
        }
        catch (RpcException)
        {
        }
    }

    [RelayCommand]
    private async Task Send()
    {
        var text = CommandText?.Trim();
        if (string.IsNullOrEmpty(text) || _call is null)
            return;
        try
        {
            await _call.RequestStream.WriteAsync(new ConsoleInput { ServerId = _serverId, Command = text });
        }
        catch (RpcException)
        {
        }
        CommandText = "";
    }

    private void AppendLine(string line)
    {
        if (line.Length > MaxLineLength)
            line = line[..MaxLineLength] + "…";
        Lines.Add(line);
        while (Lines.Count > MaxLines)
            Lines.RemoveAt(0);
    }

    public void AppendExternalLine(string line) => RunOnUI(() => AppendLine(line));

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
            _cts?.Cancel();
            if (_call is not null)
            {
                try { await _call.RequestStream.CompleteAsync(); }
                catch (RpcException) { }
                _call.Dispose();
            }
        }
        catch
        {
            // Best-effort teardown.
        }
        finally
        {
            _cts?.Dispose();
        }
    }
}
