using JustHostMC.App.Controls;
using JustHostMC.App.ViewModels;
using McManager.Grpc;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

public sealed partial class CreateServerDialog : ContentDialog {
    private readonly ServerDialog _content;

    public CreateServerDialog(MainViewModel viewModel) {
        InitializeComponent();
        ContentDialogSizing.Apply(this);
        _content = new ServerDialog(viewModel, ServerDialogMode.Create);
        DialogBody.Content = _content;

        IsPrimaryButtonEnabled               = _content.CanSubmit;
        _content.CanSubmitChanged += (_, _) => IsPrimaryButtonEnabled =
            _content.CanSubmit;
    }

    public CreateServerRequest? BuildCreateRequest() =>
        _content.BuildCreateRequest();
}
