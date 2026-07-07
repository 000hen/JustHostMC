using System;
using System.Collections.ObjectModel;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using Grpc.Core;
using McManager.Grpc;
using Microsoft.UI.Dispatching;

namespace JustHostMC.App.ViewModels;

/// <summary>Streams ~1 Hz resource samples from the MetricsService and exposes
/// them as bounded normalized series (for sparklines) plus current-value
/// readouts.</summary>
public sealed partial class MetricsViewModel : ObservableObject,
                                               IAsyncDisposable {
    private const int Capacity = 60;

    private readonly string _serverId;
    private readonly DispatcherQueue _dispatcher;
    private CancellationTokenSource? _cts;
    private double _memMaxSeen = 1;
    private double _netMaxSeen = 1;

    public MetricsViewModel(string serverId, DispatcherQueue dispatcher) {
        _serverId   = serverId;
        _dispatcher = dispatcher;
    }

    /// <summary>Normalized 0..1 history for the sparklines.</summary>
    public ObservableCollection<double> CpuSeries { get; } = new();
    public ObservableCollection<double> MemSeries { get; } = new();
    public ObservableCollection<double> NetSeries { get; } = new();

    [ObservableProperty]
    public partial string CpuText {
        get; private set;
    } = "—";

    [ObservableProperty]
    public partial string MemText {
        get; private set;
    } = "—";

    [ObservableProperty]
    public partial string NetText {
        get; private set;
    } = "—";

    [ObservableProperty]
    public partial bool NetworkAvailable {
        get; private set;
    }

    public async Task AttachAsync() {
        var daemon = await App.Current.DaemonReady;
        _cts       = new CancellationTokenSource();
        _          = ReadLoopAsync(daemon, _cts.Token);
    }

    private async Task ReadLoopAsync(JustHostMC.Core.DaemonClient daemon,
                                     CancellationToken token) {
        try {
            using var call = daemon.Metrics.Watch(
                new ServerId { Id = _serverId }, cancellationToken: token);
            await foreach (var sample in call.ResponseStream.ReadAllAsync(token)
                               .ConfigureAwait(false)) {
                var snapshot = sample;
                RunOnUI(() => Apply(snapshot));
            }
        } catch (OperationCanceledException) {
        } catch (RpcException) {
        }
    }

    private void Apply(ResourceSample s) {
        Push(CpuSeries, Math.Clamp(s.CpuPercent / 100.0, 0, 1));
        CpuText = $"{s.CpuPercent:0}%";

        double memNormalized;
        if (s.MemoryLimitBytes > 0) {
            memNormalized =
                Math.Clamp(s.MemoryBytes / (double)s.MemoryLimitBytes, 0, 1);
            MemText = $"{Mb(s.MemoryBytes)} / {Mb(s.MemoryLimitBytes)} MB";
        } else {
            _memMaxSeen   = Math.Max(_memMaxSeen, s.MemoryBytes);
            memNormalized = s.MemoryBytes / _memMaxSeen;
            MemText       = $"{Mb(s.MemoryBytes)} MB";
        }
        Push(MemSeries, memNormalized);

        NetworkAvailable = s.NetworkAvailable;
        if (s.NetworkAvailable) {
            double total = s.NetRxBytesPerSec + s.NetTxBytesPerSec;
            _netMaxSeen  = Math.Max(_netMaxSeen, total);
            Push(NetSeries, total / _netMaxSeen);
            NetText =
                $"↓ {Kb(s.NetRxBytesPerSec)}  ↑ {Kb(s.NetTxBytesPerSec)} /s";
        }
    }

    private static void Push(ObservableCollection<double> series,
                             double value) {
        series.Add(value);
        while (series.Count > Capacity) series.RemoveAt(0);
    }

    private static long Mb(long bytes)   => bytes / (1 << 20);
    private static string Kb(long bytes) => $"{bytes / 1024.0:0.0} KB";

    private void RunOnUI(Action action) {
        if (_dispatcher.HasThreadAccess)
            action();
        else
            _dispatcher.TryEnqueue(() => action());
    }

    public ValueTask DisposeAsync() {
        _cts?.Cancel();
        _cts?.Dispose();
        return ValueTask.CompletedTask;
    }
}
