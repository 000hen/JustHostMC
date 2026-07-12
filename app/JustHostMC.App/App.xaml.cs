using JustHostMC.App.Services;
using JustHostMC.App.Views;
using JustHostMC.Core;
using Microsoft.UI.Xaml;

namespace JustHostMC.App;

/// <summary>
/// Application entry point. Owns the engine child-process lifecycle: it
/// launches the engine on startup, exposes the connected <see
/// cref="DaemonClient"/> via <see cref="DaemonReady"/>, and shuts the engine
/// down when the window closes.
/// </summary>
public partial class App : Application {
    private Window? _window;
    private EngineHost? _engineHost;
    private DaemonClient? _daemon;
    private EngineStdioWindow? _engineStdioWindow;
    private readonly TaskCompletionSource<DaemonClient> _daemonReady = new();

    /// <summary>Completes once the engine is up and the gRPC client is
    /// ready.</summary>
    public Task<DaemonClient> DaemonReady => _daemonReady.Task;

    public static new App Current => (App)Application.Current;

    /// <summary>Tracks engine-backed work that continues independently of the
    /// main window.</summary>
    public BackgroundTaskService BackgroundTasks { get; } = new();

    /// <summary>The main window, exposed so pages can obtain its HWND for
    /// pickers/dialogs.</summary>
    public Window? MainWindow => _window;

    /// <summary>Opens or activates the single engine standard-stream
    /// monitor.</summary>
    public void ShowEngineStdioWindow() {
        if (_engineHost is null)
            return;

        if (_engineStdioWindow is null) {
            var window               = new EngineStdioWindow(_engineHost);
            window.Closed += (_, _) => {
                if (ReferenceEquals(_engineStdioWindow, window))
                    _engineStdioWindow = null;
            };
            _engineStdioWindow = window;
        }

        _engineStdioWindow.Activate();
    }

    public App() => InitializeComponent();

    protected override void OnLaunched(LaunchActivatedEventArgs args) {
        _window = new MainWindow();
        _window.Closed += OnWindowClosed;
        _window.Activate();

        _ = StartEngineAsync();
    }

    private async Task StartEngineAsync() {
        try {
            var enginePath =
                Path.Combine(AppContext.BaseDirectory, "engine", "engine.exe");
            _engineHost = new EngineHost(new EngineHostOptions {
                EnginePath = enginePath,
                DataDir    = ResolveDataDir(),
            });
            var connection =
                await _engineHost.StartAsync().ConfigureAwait(false);
            _daemon = new DaemonClient(connection);
            _daemonReady.TrySetResult(_daemon);
        } catch (Exception ex) {
            _daemonReady.TrySetException(ex);
        }
    }

    /// <summary>
    /// Resolves the engine's data directory. When packaged (MSIX), all data
    /// lives under the package's local store so an uninstall removes it cleanly
    /// (PROMPT §8); when running unpackaged (dev/test), returns null so the
    /// engine uses its %LOCALAPPDATA% default.
    /// </summary>
    private static string? ResolveDataDir() {
        try {
            return Windows.Storage.ApplicationData.Current.LocalFolder.Path;
        } catch {
            return null;  // not running in a packaged context
        }
    }

    private async void OnWindowClosed(object sender, WindowEventArgs args) {
        if (_daemon is not null)
            await _daemon.DisposeAsync();
        if (_engineHost is not null)
            await _engineHost.DisposeAsync();
    }
}
