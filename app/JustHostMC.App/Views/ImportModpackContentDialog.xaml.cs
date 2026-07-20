using JustHostMC.App.Controls;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class ImportModpackContentDialog : ContentDialog {
    private static readonly int[] MemoryOptions = [
        2048,
        4096,
        6144,
        8192,
        12288,
        16384,
    ];

    private readonly string _defaultName;

    public ImportModpackContentDialog(string defaultName) {
        _defaultName = defaultName;
        InitializeComponent();
        ContentDialogSizing.Apply(this);
        NameBox.Text = defaultName;
    }

    public string ServerName {
        get {
            var name = NameBox.Text.Trim();
            return name.Length == 0 ? _defaultName : name;
        }
    }

    public int MemoryMb {
        get {
            var index = MemoryBox.SelectedIndex;
            return MemoryOptions[index >= 0 ? index : 1];
        }
    }
}
