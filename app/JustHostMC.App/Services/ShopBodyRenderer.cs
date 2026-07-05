using Markdig;
using McManager.Grpc;

namespace JustHostMC.App.Services;

/// <summary>Turns a project's long description (Modrinth Markdown or
/// CurseForge HTML) into a self-contained, script-free HTML document for
/// WebView2. A CSP meta tag blocks scripts/frames/fetch; only remote images
/// and inline styles are allowed.</summary>
public static class ShopBodyRenderer
{
    private static readonly MarkdownPipeline Pipeline =
        new MarkdownPipelineBuilder().UseAdvancedExtensions().DisableHtml().Build();

    public static string ToHtml(string body, ShopBodyFormat format, bool darkTheme)
    {
        var content = format == ShopBodyFormat.ShopBodyMarkdown
            ? Markdown.ToHtml(body, Pipeline)
            : body;

        var fg = darkTheme ? "#e8e8e8" : "#1a1a1a";
        var secondary = darkTheme ? "#9e9e9e" : "#5c5c5c";
        var accent = darkTheme ? "#4cc2ff" : "#0067c0";
        var codeBg = darkTheme ? "#2b2b2b" : "#f3f3f3";

        return $$"""
            <!DOCTYPE html>
            <html>
            <head>
            <meta charset="utf-8">
            <meta http-equiv="Content-Security-Policy"
                  content="default-src 'none'; img-src https: http: data:; style-src 'unsafe-inline'; media-src https:">
            <style>
              html, body { background: transparent; margin: 0; }
              body {
                font-family: 'Segoe UI Variable Text', 'Segoe UI', sans-serif;
                font-size: 14px; line-height: 1.55; color: {{fg}};
                padding: 4px 12px 24px 2px; word-wrap: break-word;
              }
              img { max-width: 100%; height: auto; border-radius: 4px; }
              a { color: {{accent}}; }
              h1, h2, h3 { font-weight: 600; line-height: 1.3; }
              hr { border: none; border-top: 1px solid {{secondary}}; opacity: .4; }
              code, pre { background: {{codeBg}}; border-radius: 4px; font-family: Consolas, monospace; }
              pre { padding: 10px; overflow-x: auto; }
              code { padding: 1px 4px; }
              blockquote { border-left: 3px solid {{secondary}}; margin-left: 0; padding-left: 12px; color: {{secondary}}; }
              table { border-collapse: collapse; }
              th, td { border: 1px solid {{secondary}}; padding: 4px 8px; }
              iframe, script, object, embed { display: none !important; }
            </style>
            </head>
            <body>{{content}}</body>
            </html>
            """;
    }
}
