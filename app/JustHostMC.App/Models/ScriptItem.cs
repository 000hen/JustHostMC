using JustHostMC.App.Services;
using McManager.Grpc;

namespace JustHostMC.App.Models;

/// <summary>View model for one installed automation script in the Scripts page list.</summary>
public sealed class ScriptItem : ScriptEntryItem
{
    public ScriptItem(ScriptInfo info, ILocalizer localizer)
        : base(
            info.Id,
            info.Name,
            info.Author,
            info.Version,
            info.Description,
            info.Permissions,
            info.Granted,
            localizer)
    {
        Enabled = info.Enabled;
    }

    public override bool SupportsToggle => true;

    public override bool SupportsLogs => true;
}
