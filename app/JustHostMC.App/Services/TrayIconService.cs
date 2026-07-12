using System.Runtime.InteropServices;
using Windows.Graphics.Imaging;
using Windows.Storage;

namespace JustHostMC.App.Services;

/// <summary>Small Win32 notification-area icon used while the main window is
/// hidden. The window's existing subclass forwards the callback
/// message.</summary>
internal sealed class TrayIconService : IDisposable {
    public const uint CallbackMessage = 0x8001;

    private const uint NimAdd          = 0x00000000;
    private const uint NimDelete       = 0x00000002;
    private const uint NimSetVersion   = 0x00000004;
    private const uint NifMessage      = 0x00000001;
    private const uint NifIcon         = 0x00000002;
    private const uint NifTip          = 0x00000004;
    private const uint NotifyVersion   = 4;
    private const uint WmLButtonDblClk = 0x0203;
    private const uint WmRButtonUp     = 0x0205;
    private const uint WmContextMenu   = 0x007B;
    private const uint MfString        = 0x00000000;
    private const uint MfSeparator     = 0x00000800;
    private const uint TpmRightButton  = 0x0002;
    private const uint TpmReturnCmd    = 0x0100;
    private const uint RestoreCommand  = 1;
    private const uint ExitCommand     = 2;
    private const int IdiApplication   = 32512;

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct NotifyIconData {
        public int CbSize;
        public IntPtr HWnd;
        public uint UId;
        public uint UFlags;
        public uint UCallbackMessage;
        public IntPtr HIcon;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 128)]
        public string SzTip;
        public uint DwState;
        public uint DwStateMask;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 256)]
        public string SzInfo;
        public uint UTimeoutOrVersion;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 64)]
        public string SzInfoTitle;
        public uint DwInfoFlags;
        public Guid GuidItem;
        public IntPtr HBalloonIcon;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct Point {
        public int X;
        public int Y;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct IconInfo {
        [MarshalAs(UnmanagedType.Bool)]
        public bool IsIcon;
        public uint XHotspot;
        public uint YHotspot;
        public IntPtr MaskBitmap;
        public IntPtr ColorBitmap;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct BitmapInfoHeader {
        public uint Size;
        public int Width;
        public int Height;
        public ushort Planes;
        public ushort BitCount;
        public uint Compression;
        public uint SizeImage;
        public int XPelsPerMeter;
        public int YPelsPerMeter;
        public uint ClrUsed;
        public uint ClrImportant;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct BitmapInfo {
        public BitmapInfoHeader Header;
        public uint Colors;
    }

    [DllImport("shell32.dll", CharSet = CharSet.Unicode)]
    private static extern bool Shell_NotifyIcon(uint message,
                                                ref NotifyIconData data);

    [DllImport("user32.dll")]
    private static extern IntPtr LoadIcon(IntPtr instance, IntPtr iconName);

    [DllImport("gdi32.dll")]
    private static extern IntPtr CreateDIBSection(IntPtr deviceContext,
                                                  ref BitmapInfo bitmapInfo,
                                                  uint usage, out IntPtr bits,
                                                  IntPtr section, uint offset);

    [DllImport("gdi32.dll", EntryPoint = "CreateBitmap")]
    private static extern IntPtr CreateEmptyBitmap(int width, int height,
                                                   uint planes,
                                                   uint bitsPerPixel,
                                                   IntPtr pixels);

    [DllImport("gdi32.dll")]
    private static extern bool DeleteObject(IntPtr handle);

    [DllImport("user32.dll")]
    private static extern IntPtr CreateIconIndirect(ref IconInfo iconInfo);

    [DllImport("user32.dll")]
    private static extern bool DestroyIcon(IntPtr icon);

    [DllImport("user32.dll")]
    private static extern IntPtr CreatePopupMenu();

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern bool AppendMenu(IntPtr menu, uint flags, nuint item,
                                          string? text);

    [DllImport("user32.dll")]
    private static extern uint TrackPopupMenu(IntPtr menu, uint flags, int x,
                                              int y, int reserved,
                                              IntPtr window, IntPtr rect);

    [DllImport("user32.dll")]
    private static extern bool DestroyMenu(IntPtr menu);

    [DllImport("user32.dll")]
    private static extern bool GetCursorPos(out Point point);

    [DllImport("user32.dll")]
    private static extern bool SetForegroundWindow(IntPtr window);

    private readonly IntPtr _window;
    private readonly ILocalizer _localizer;
    private readonly Action _restore;
    private readonly Action _exit;
    private IntPtr _icon;
    private bool _visible;

    public TrayIconService(IntPtr window, ILocalizer localizer, Action restore,
                           Action exit) {
        _window    = window;
        _localizer = localizer;
        _restore   = restore;
        _exit      = exit;
    }

    public async Task ShowAsync() {
        if (_visible)
            return;

        if (_icon == IntPtr.Zero)
            _icon = await LoadApplicationIconAsync();

        var data = CreateData();
        if (!Shell_NotifyIcon(NimAdd, ref data))
            return;

        data.UTimeoutOrVersion = NotifyVersion;
        Shell_NotifyIcon(NimSetVersion, ref data);
        _visible = true;
    }

    public void Hide() {
        if (!_visible)
            return;

        var data = CreateData();
        Shell_NotifyIcon(NimDelete, ref data);
        _visible = false;
    }

    public bool HandleMessage(IntPtr lParam) {
        var message = (uint)(lParam.ToInt64() & 0xffff);
        switch (message) {
            case WmLButtonDblClk:
                _restore();
                return true;
            case WmRButtonUp:
            case WmContextMenu:
                ShowContextMenu();
                return true;
            default:
                return false;
        }
    }

    public void Dispose() {
        Hide();
        if (_icon != IntPtr.Zero) {
            DestroyIcon(_icon);
            _icon = IntPtr.Zero;
        }
    }

    private NotifyIconData CreateData() => new() {
        CbSize           = Marshal.SizeOf<NotifyIconData>(),
        HWnd             = _window,
        UId              = 1,
        UFlags           = NifMessage | NifIcon | NifTip,
        UCallbackMessage = CallbackMessage,
        HIcon       = _icon != IntPtr.Zero
                          ? _icon
                          : LoadIcon(IntPtr.Zero, new IntPtr(IdiApplication)),
        SzTip       = _localizer.Get("AppTitle"),
        SzInfo      = "",
        SzInfoTitle = "",
    };

    private static async Task<IntPtr> LoadApplicationIconAsync() {
        try {
            const int size   = 32;
            var path         = Path.Combine(AppContext.BaseDirectory, "Assets",
                                            "StoreLogo.png");
            var file         = await StorageFile.GetFileFromPathAsync(path);
            using var stream = await file.OpenReadAsync();
            var decoder      = await BitmapDecoder.CreateAsync(stream);
            var pixels       = await decoder.GetPixelDataAsync(
                BitmapPixelFormat.Bgra8, BitmapAlphaMode.Premultiplied,
                new BitmapTransform {
                    ScaledWidth  = size,
                    ScaledHeight = size,
                },
                ExifOrientationMode.IgnoreExifOrientation,
                ColorManagementMode.DoNotColorManage);

            var pixelBytes = pixels.DetachPixelData();
            var bitmapInfo = new BitmapInfo {
                Header =
                    new BitmapInfoHeader {
                                          Size      = (uint)Marshal.SizeOf<BitmapInfoHeader>(),
                                          Width     = size,
                                          Height    = -size,
                                          Planes    = 1,
                                          BitCount  = 32,
                                          SizeImage = (uint)pixelBytes.Length,
                                          },
            };
            var color = CreateDIBSection(IntPtr.Zero, ref bitmapInfo, 0,
                                         out var colorBits, IntPtr.Zero, 0);
            if (color != IntPtr.Zero && colorBits != IntPtr.Zero)
                Marshal.Copy(pixelBytes, 0, colorBits, pixelBytes.Length);
            var mask = CreateEmptyBitmap(size, size, 1, 1, IntPtr.Zero);
            if (color == IntPtr.Zero || mask == IntPtr.Zero) {
                if (color != IntPtr.Zero)
                    DeleteObject(color);
                if (mask != IntPtr.Zero)
                    DeleteObject(mask);
                return IntPtr.Zero;
            }

            try {
                var info = new IconInfo {
                    IsIcon      = true,
                    ColorBitmap = color,
                    MaskBitmap  = mask,
                };
                return CreateIconIndirect(ref info);
            } finally {
                DeleteObject(color);
                DeleteObject(mask);
            }
        } catch {
            return IntPtr.Zero;
        }
    }

    private void ShowContextMenu() {
        var menu = CreatePopupMenu();
        if (menu == IntPtr.Zero)
            return;

        try {
            AppendMenu(menu, MfString, RestoreCommand,
                       _localizer.Get("Tray_Restore"));
            AppendMenu(menu, MfSeparator, 0, null);
            AppendMenu(menu, MfString, ExitCommand,
                       _localizer.Get("Tray_Exit"));
            GetCursorPos(out var point);
            SetForegroundWindow(_window);
            var command =
                TrackPopupMenu(menu, TpmRightButton | TpmReturnCmd, point.X,
                               point.Y, 0, _window, IntPtr.Zero);
            if (command == RestoreCommand)
                _restore();
            else if (command == ExitCommand)
                _exit();
        } finally {
            DestroyMenu(menu);
        }
    }
}
