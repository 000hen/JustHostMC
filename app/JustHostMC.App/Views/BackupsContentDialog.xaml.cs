using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class BackupsContentDialog : ContentDialog {
    private readonly BackupsDialog _content;

    public BackupsContentDialog(string serverId, string serverName,
                                bool serverRunning,
                                DispatcherQueue dispatcher) {
        InitializeComponent();
        Title = serverName;

        _content = new BackupsDialog(serverId, serverName, serverRunning,
                                     dispatcher);
        DialogBody.Content = _content;
        Opened += async (_, _) => await _content.LoadAsync();
    }
}
