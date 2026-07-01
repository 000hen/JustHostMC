using System.Text;

namespace JustHostMC.App.Services;

/// <summary>Indents SNBT without changing content inside quoted JSON/text values.</summary>
internal static class SnbtFormatter
{
    public static string Format(string source)
    {
        if (string.IsNullOrWhiteSpace(source))
            return source;

        var output = new StringBuilder(source.Length + source.Length / 4);
        var indent = 0;
        var quote = '\0';
        var escaped = false;

        for (var index = 0; index < source.Length; index++)
        {
            var current = source[index];
            if (quote != '\0')
            {
                output.Append(current);
                if (escaped)
                {
                    escaped = false;
                }
                else if (current == '\\')
                {
                    escaped = true;
                }
                else if (current == quote)
                {
                    quote = '\0';
                }
                continue;
            }

            if (current is '\'' or '"')
            {
                quote = current;
                output.Append(current);
                continue;
            }

            switch (current)
            {
                case '{':
                case '[':
                    output.Append(current);
                    indent++;
                    if (NextNonWhitespace(source, index + 1) != (current == '{' ? '}' : ']'))
                        AppendLine(output, indent);
                    break;
                case '}':
                case ']':
                    indent = Math.Max(0, indent - 1);
                    if (PreviousNonWhitespace(source, index - 1) != (current == '}' ? '{' : '['))
                        AppendLine(output, indent);
                    output.Append(current);
                    break;
                case ',':
                    output.Append(current);
                    AppendLine(output, indent);
                    break;
                case ':':
                    output.Append(": ");
                    break;
                default:
                    if (!char.IsWhiteSpace(current))
                        output.Append(current);
                    break;
            }
        }
        return output.ToString();
    }

    private static void AppendLine(StringBuilder output, int indent)
    {
        while (output.Length > 0 && output[^1] is ' ' or '\t' or '\r' or '\n')
            output.Length--;
        output.AppendLine();
        output.Append(' ', indent * 2);
    }

    private static char NextNonWhitespace(string source, int index)
    {
        while (index < source.Length && char.IsWhiteSpace(source[index]))
            index++;
        return index < source.Length ? source[index] : '\0';
    }

    private static char PreviousNonWhitespace(string source, int index)
    {
        while (index >= 0 && char.IsWhiteSpace(source[index]))
            index--;
        return index >= 0 ? source[index] : '\0';
    }
}
