# Main Window Close-Order Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent `MainWindow.OnClosed` from accessing WinUI window state after asynchronous view-model disposal yields.

**Architecture:** Keep the existing `OnClosed` entry point and cleanup operations. Add a source-order regression test because the non-UI test project cannot construct a WinUI window, then move the existing `DisposeAsync` await after all window-dependent cleanup.

**Tech Stack:** C# 13, .NET 9, WinUI 3, xUnit, PowerShell repository formatter

## Global Constraints

- Do not change close cancellation, background-task prompting, tray behavior, server disposal, or daemon shutdown.
- Perform every operation that depends on the live window before the first await in `OnClosed`.
- Use `format.ps1` for formatting and format verification.
- Do not add dependencies or create a WinUI test project.

---

### Task 1: Enforce synchronous window cleanup before asynchronous disposal

**Files:**
- Create: `app/JustHostMC.Core.Tests/MainWindowLifecycleTests.cs`
- Modify: `app/JustHostMC.App/MainWindow.xaml.cs:141-156`

**Interfaces:**
- Consumes: `MainWindow.OnClosed(object, WindowEventArgs)` and the existing `Shell.Main.DisposeAsync()` lifecycle.
- Produces: The invariant that all window-dependent cleanup in `OnClosed` precedes its first asynchronous suspension.

- [ ] **Step 1: Write the failing source-order regression test**

```csharp
using System.Runtime.CompilerServices;
using Xunit;

namespace JustHostMC.Core.Tests;

public class MainWindowLifecycleTests {
    [Fact]
    public void OnClosed_CleansUpWindowBeforeAwaitingDisposal() {
        var source = File.ReadAllText(MainWindowSourcePath());
        var methodStart = source.IndexOf(
            "private async void OnClosed", StringComparison.Ordinal);
        var methodEnd = source.IndexOf(
            "\n    private ", methodStart + 1, StringComparison.Ordinal);

        Assert.True(methodStart >= 0, "OnClosed method was not found.");
        Assert.True(methodEnd > methodStart, "OnClosed method end was not found.");

        var method = source[methodStart..methodEnd];
        var lastWindowCleanup = method.IndexOf(
            "UntrackServer(server);", StringComparison.Ordinal);
        var asynchronousDisposal = method.IndexOf(
            "await Shell.Main.DisposeAsync();", StringComparison.Ordinal);

        Assert.True(lastWindowCleanup >= 0,
                    "The final window cleanup operation was not found.");
        Assert.True(asynchronousDisposal > lastWindowCleanup,
                    "Window cleanup must complete before asynchronous disposal.");
    }

    private static string MainWindowSourcePath(
        [CallerFilePath] string testSourcePath = "") =>
        Path.GetFullPath(Path.Combine(
            Path.GetDirectoryName(testSourcePath)!, "..", "JustHostMC.App",
            "MainWindow.xaml.cs"));
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```powershell
rtk test dotnet test app\JustHostMC.Core.Tests\JustHostMC.Core.Tests.csproj --filter FullyQualifiedName~MainWindowLifecycleTests -p:SkipEngineBuild=true
```

Expected: FAIL with `Window cleanup must complete before asynchronous disposal.` because `DisposeAsync` currently precedes `UntrackServer`.

- [ ] **Step 3: Move asynchronous disposal after synchronous window cleanup**

Change `OnClosed` to:

```csharp
private async void OnClosed(object sender, WindowEventArgs args) {
    AppWindow.Closing -= OnAppWindowClosing;
    _trayIcon?.Dispose();
    if (_hwnd != IntPtr.Zero)
        RemoveWindowSubclass(_hwnd, _subclassProc, MinWindowSubclassId);

    PaneFooterGrid.UnregisterPropertyChangedCallback(
        UIElement.VisibilityProperty, _paneFooterVisibilityCallbackToken);
    Shell.Main.Servers.CollectionChanged -= OnServersChanged;

    foreach (var server in _trackedServers.Values.ToList())
        UntrackServer(server);

    await Shell.Main.DisposeAsync();
}
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```powershell
rtk test dotnet test app\JustHostMC.Core.Tests\JustHostMC.Core.Tests.csproj --filter FullyQualifiedName~MainWindowLifecycleTests -p:SkipEngineBuild=true
```

Expected: PASS, 1 test passed and 0 failed.

- [ ] **Step 5: Format and verify the repository**

Run:

```powershell
rtk proxy powershell -NoProfile -ExecutionPolicy Bypass -File .\format.ps1
rtk test dotnet test app\JustHostMC.Core.Tests\JustHostMC.Core.Tests.csproj
rtk dotnet build JustHostMC.sln -p:Platform=x64
rtk proxy powershell -NoProfile -ExecutionPolicy Bypass -File .\format.ps1 -Check
```

Expected: 35 C# tests pass, the x64 solution build has 0 errors and 0 warnings, and all format checks pass.

- [ ] **Step 6: Inspect and commit the focused change**

```powershell
rtk git diff --check
rtk git diff -- app/JustHostMC.App/MainWindow.xaml.cs app/JustHostMC.Core.Tests/MainWindowLifecycleTests.cs
rtk git add app/JustHostMC.App/MainWindow.xaml.cs app/JustHostMC.Core.Tests/MainWindowLifecycleTests.cs docs/superpowers/plans/2026-07-13-main-window-close-order.md
rtk git commit -m fix-main-window-close-order
```

Expected: the diff contains only the lifecycle regression test, the `OnClosed` reorder, and this plan; the commit succeeds.
