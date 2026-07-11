using JustHostMC.App.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Windows.System;

namespace JustHostMC.App.Controls.Server;

public sealed partial class ServerConsolePanel : UserControl {
    public static readonly DependencyProperty ConsoleProperty =
        DependencyProperty.Register(
            nameof(Console), typeof(ConsoleViewModel),
            typeof(ServerConsolePanel),
            new PropertyMetadata(null, OnConsoleChanged));

    public ConsoleViewModel Console {
        get => (ConsoleViewModel)GetValue(ConsoleProperty);
        set => SetValue(ConsoleProperty, value);
    }

    public ServerConsolePanel() {
        InitializeComponent();
    }

    private static void OnConsoleChanged(DependencyObject d,
                                         DependencyPropertyChangedEventArgs e) {
        var panel = (ServerConsolePanel)d;

        if (e.NewValue is ConsoleViewModel)
            panel.Bindings.Update();
    }

    private void OnCommandKeyDown(object sender, KeyRoutedEventArgs e) {
        if (e.Key == VirtualKey.Enter &&
            Console?.SendCommand.CanExecute(null) == true) {
            Console.SendCommand.Execute(null);
            e.Handled = true;
        }
    }
}
