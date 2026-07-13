# Main Window Close-Order Fix

## Problem

`MainWindow.OnClosed` awaits `Shell.Main.DisposeAsync()` before detaching window,
native, XAML, and server event handlers. The await can yield while WinUI tears
down the closed window. When the continuation resumes, `AppWindow` may no longer
be available, causing a `NullReferenceException` at the `Closing` event
unsubscription. The later cleanup operations are exposed to the same lifecycle
race.

## Design

Keep `OnClosed` as the shutdown entry point, but complete every operation that
depends on the live WinUI window synchronously before the first await:

1. Detach `AppWindow.Closing` and dispose the tray icon.
2. Remove the native window subclass.
3. Unregister the XAML property callback and collection event handler.
4. Untrack each cached server.
5. Await `Shell.Main.DisposeAsync()` only after window-dependent cleanup is
   complete.

This changes only cleanup ordering. It does not change close cancellation,
background-task prompting, tray behavior, server disposal, or daemon shutdown.

## Error Handling

No new exception handling is introduced. Existing cleanup and disposal errors
retain their current propagation behavior; the fix removes the invalid access
caused by resuming after window teardown.

## Verification

The WinUI window cannot be instantiated by the existing non-UI core test
project. Verification will therefore combine a focused source-level regression
check for the required ordering with the repository formatter, the full C# test
suite, and the x64 solution build.
