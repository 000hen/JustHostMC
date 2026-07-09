using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Controls.Server;

/// <summary>Reusable section frame for server-page tab panels.
/// Provides a uniform header with <see cref="IconGlyph"/>,
/// <see cref="Title"/>, <see cref="Description"/> and an optional
/// <see cref="HeaderContent"/> slot for action buttons, plus a
/// <see cref="Body"/> content area that fills remaining space.
/// The header row auto-collapses when both Title and HeaderContent
/// are empty.</summary>
public sealed partial class ServerSectionLayout : UserControl {
    public static readonly DependencyProperty IconGlyphProperty =
        DependencyProperty.Register(
            nameof(IconGlyph), typeof(string), typeof(ServerSectionLayout),
            new PropertyMetadata(null, OnHeaderPartChanged));

    public static readonly DependencyProperty TitleProperty =
        DependencyProperty.Register(
            nameof(Title), typeof(string), typeof(ServerSectionLayout),
            new PropertyMetadata(null, OnHeaderPartChanged));

    public static readonly DependencyProperty DescriptionProperty =
        DependencyProperty.Register(
            nameof(Description), typeof(string), typeof(ServerSectionLayout),
            new PropertyMetadata(null, OnHeaderPartChanged));

    public static readonly DependencyProperty HeaderContentProperty =
        DependencyProperty.Register(
            nameof(HeaderContent), typeof(object), typeof(ServerSectionLayout),
            new PropertyMetadata(null, OnHeaderPartChanged));

    public static readonly DependencyProperty BodyProperty =
        DependencyProperty.Register(
            nameof(Body), typeof(object), typeof(ServerSectionLayout),
            new PropertyMetadata(null));

    public string IconGlyph {
        get => (string)GetValue(IconGlyphProperty);
        set => SetValue(IconGlyphProperty, value);
    }

    public string Title {
        get => (string)GetValue(TitleProperty);
        set => SetValue(TitleProperty, value);
    }

    public string Description {
        get => (string)GetValue(DescriptionProperty);
        set => SetValue(DescriptionProperty, value);
    }

    public object HeaderContent {
        get => GetValue(HeaderContentProperty);
        set => SetValue(HeaderContentProperty, value);
    }

    public object Body {
        get => GetValue(BodyProperty);
        set => SetValue(BodyProperty, value);
    }

    public ServerSectionLayout() {
        InitializeComponent();
    }

    private static void OnHeaderPartChanged(DependencyObject d,
                                            DependencyPropertyChangedEventArgs e) {
        var layout = (ServerSectionLayout)d;
        layout.UpdateHeaderVisibility();
    }

    private void UpdateHeaderVisibility() {
        var hasTitle       = !string.IsNullOrEmpty(Title);
        var hasIcon        = !string.IsNullOrEmpty(IconGlyph);
        var hasDescription = !string.IsNullOrEmpty(Description);
        var hasContent     = HeaderContent is not null;

        HeaderRow.Visibility       = (hasTitle || hasContent) ? Visibility.Visible : Visibility.Collapsed;
        HeaderIcon.Visibility      = hasIcon ? Visibility.Visible : Visibility.Collapsed;
        TitleArea.Visibility       = hasTitle ? Visibility.Visible : Visibility.Collapsed;
        DescriptionText.Visibility = hasDescription ? Visibility.Visible : Visibility.Collapsed;
    }
}
