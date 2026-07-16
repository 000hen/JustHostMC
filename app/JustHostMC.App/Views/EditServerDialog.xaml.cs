using JustHostMC.App.Controls;
using JustHostMC.App.Models;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class EditServerDialog : ContentDialog {
    private readonly ServerDialog _content;

    public EditServerDialog(MainViewModel viewModel, ServerItem server) {
        InitializeComponent();
        ContentDialogSizing.Apply(this);
        _content = new ServerDialog(viewModel, ServerDialogMode.Edit, server);
        DialogBody.Content = _content;

        IsPrimaryButtonEnabled               = _content.CanSubmit;
        _content.CanSubmitChanged += (_, _) => IsPrimaryButtonEnabled =
            _content.CanSubmit;
    }

    public UpdateServerRequest BuildUpdateRequest() =>
        _content.BuildUpdateRequest();
}
