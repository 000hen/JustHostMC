using System.Linq;
using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>View model for one installed mod/plugin metadata parser in the
/// Scripts page list.</summary>
public sealed class ParserItem : ScriptEntryItem {
    public ParserItem(ParserInfo info, ILocalizer localizer)
        : base(info.Id, info.Name, info.Author, info.Version,
               ComposeDescription(info, localizer), info.Permissions,
               info.Granted, localizer) {
        Builtin = info.Builtin;
    }

    public bool Builtin { get; }
    public override bool IsBuiltIn => Builtin;

    /// <summary>Built-in parsers cannot be removed.</summary>
    public override bool CanRemove => !Builtin;

    /// <summary>Appends the descriptor formats the parser reads to its
    /// description, so the shared entry card surfaces them.</summary>
    private static string ComposeDescription(ParserInfo info,
                                             ILocalizer localizer) {
        if (info.Formats.Count == 0)
            return info.Description;
        var formats = localizer.Get(
            "Parsers_Formats", ("formats", string.Join(", ", info.Formats)));
        return info.Description.Length == 0 ? formats
                                            : $"{info.Description}\n{formats}";
    }
}
