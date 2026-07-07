using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Data;

namespace JustHostMC.App.Converters;

/// <summary>
/// Maps a bool to Visibility. Pass ConverterParameter="invert" to flip it.
/// </summary>
public sealed partial class BoolToVisibilityConverter : IValueConverter {
    public object Convert(object value, Type targetType, object parameter,
                          string language) {
        var flag = value switch {
            bool b => b,
            int i  => i > 0,
            _      => value is not null,
        };
        if (parameter is string s &&
            string.Equals(s, "invert", StringComparison.OrdinalIgnoreCase))
            flag = !flag;
        return flag ? Visibility.Visible : Visibility.Collapsed;
    }

    public object ConvertBack(object value, Type targetType, object parameter,
                              string language) =>
        throw new NotSupportedException();
}
