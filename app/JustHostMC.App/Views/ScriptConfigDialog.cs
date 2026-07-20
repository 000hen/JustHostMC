using System.Collections.Generic;
using System.Globalization;
using McManager.Grpc;
using Microsoft.UI.Text;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace JustHostMC.App.Views;

/// <summary>Editor for a script's typed config options. Built imperatively
/// (the row control varies by option type), it is used as the content of a
/// ContentDialog. On save, <see cref="BuildRequest"/> yields only the values
/// the user actually changed.</summary>
public sealed class ScriptConfigDialog : UserControl {
    private sealed class Field {
        public Field(string key, ConfigOptionType type, FrameworkElement input,
                     string original) {
            Key      = key;
            Type     = type;
            Input    = input;
            Original = original;
        }

        public string Key { get; }
        public ConfigOptionType Type { get; }
        public FrameworkElement Input { get; }
        public string Original { get; }

        public bool TryGetChange(out string value) {
            value = Read();
            // Secrets start blank and are write-only: only persist a non-empty
            // entry, so leaving the box empty keeps any existing secret intact.
            if (Type == ConfigOptionType.ConfigOptionSecret)
                return value.Length > 0;
            return value != Original;
        }

        private string Read() => Input switch {
            PasswordBox pb  => pb.Password,
            ToggleSwitch ts => ts.IsOn ? "true" : "false",
            NumberBox nb =>
                double.IsNaN(nb.Value)
                    ? ""
                    : nb.Value.ToString(CultureInfo.InvariantCulture),
            TextBox tb => tb.Text,
            _          => "",
        };
    }

    private readonly string _id;
    private readonly List<Field> _fields = new();

    public ScriptConfigDialog(string id, IReadOnlyList<ConfigOption> options,
                              ScriptConfig current) {
        _id = id;

        var stored = new Dictionary<string, ScriptConfigValue>();
        foreach (var v in current.Values) stored[v.Key] = v;

        var panel = new StackPanel { Spacing = 16 };
        foreach (var opt in options) {
            stored.TryGetValue(opt.Key, out var value);
            var (input, original) = BuildInput(opt, value);
            _fields.Add(new Field(opt.Key, opt.Type, input, original));

            var row = new StackPanel { Spacing = 4 };
            row.Children.Add(new TextBlock {
                Text = string.IsNullOrEmpty(opt.Name) ? opt.Key : opt.Name,
                FontWeight = FontWeights.SemiBold,
            });
            if (!string.IsNullOrEmpty(opt.Description))
                row.Children.Add(new TextBlock {
                    Text         = opt.Description,
                    Opacity      = 0.7,
                    TextWrapping = TextWrapping.WrapWholeWords,
                });
            row.Children.Add(input);
            panel.Children.Add(row);
        }

        Content = new ScrollViewer {
            VerticalScrollBarVisibility   = ScrollBarVisibility.Auto,
            HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled,
            MinWidth                      = 360,
            Content                       = panel,
        };
    }

    private static (FrameworkElement input, string original)
        BuildInput(ConfigOption opt, ScriptConfigValue? value) {
        var hasValue    = value?.HasValue ?? false;
        var storedValue = value?.Value ?? "";

        switch (opt.Type) {
            case ConfigOptionType.ConfigOptionSecret:
                var pb = new PasswordBox();
                return (pb, "");

            case ConfigOptionType.ConfigOptionBoolean:
                var effective = hasValue ? storedValue : opt.Default;
                var on        = bool.TryParse(effective, out var b) && b;
                return (new ToggleSwitch { IsOn = on }, on ? "true" : "false");

            case ConfigOptionType.ConfigOptionNumber:
                var nb = new NumberBox {
                    SpinButtonPlacementMode =
                        NumberBoxSpinButtonPlacementMode.Compact,
                    PlaceholderText = opt.Default,
                };
                if (hasValue &&
                    double.TryParse(storedValue, NumberStyles.Any,
                                    CultureInfo.InvariantCulture, out var d))
                    nb.Value = d;
                else
                    nb.Value = double.NaN;
                return (nb, hasValue ? storedValue : "");

            default:  // string
                var tb = new TextBox {
                    Text            = hasValue ? storedValue : "",
                    PlaceholderText = opt.Default,
                };
                return (tb, hasValue ? storedValue : "");
        }
    }

    /// <summary>Builds a SetConfigRequest with only the values the user changed
    /// (secrets only when a new one was entered).</summary>
    public SetConfigRequest BuildRequest() {
        var req = new SetConfigRequest { Id = _id };
        foreach (var f in _fields)
            if (f.TryGetChange(out var value))
                req.Values.Add(
                    new ScriptConfigValue { Key = f.Key, Value = value });
        return req;
    }
}
