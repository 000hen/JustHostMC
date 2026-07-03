# MVVM Toolkit conventions

The WinUI frontend uses `CommunityToolkit.Mvvm` source generators. View models
and observable UI models should declare state and commands; they should not
repeat the notification and command plumbing that the toolkit generates.

## Observable properties

Use an `[ObservableProperty]` partial property inside a `partial` class:

```csharp
public sealed partial class ExampleViewModel : ObservableObject
{
    [ObservableProperty]
    public partial string StatusMessage { get; private set; } = "";
}
```

The app targets .NET 9 and sets `LangVersion` to `preview` because
CommunityToolkit.Mvvm 8.4 requires that combination for partial properties.
Partial properties are preferred over annotated fields in this WinUI project:
the Toolkit reports field-generated properties as incompatible with WinRT AOT
scenarios (`MVVMTK0045`). They also preserve the intended setter accessibility.

Do not hand-write a backing field and call `SetProperty` for an ordinary
observable property.

## Dependent properties

Use `[NotifyPropertyChangedFor]` when a generated property affects a computed
property:

```csharp
[ObservableProperty]
[NotifyPropertyChangedFor(nameof(CanSave))]
public partial bool IsBusy { get; private set; }

public bool CanSave => !IsBusy;
```

Use a generated partial change hook when a change needs behavior in addition to
notifications:

```csharp
[ObservableProperty]
public partial bool UseDocker { get; set; }

partial void OnUseDockerChanged(bool value) => _ = SavePreferenceAsync(value);
```

Keep direct `OnPropertyChanged` calls only for state that is not driven by one
generated property, such as changes within an `ObservableCollection` or a batch
of externally updated values.

## Commands

Use `[RelayCommand]` on the method and bind to the generated `...Command`
property:

```csharp
[RelayCommand]
private async Task Refresh()
{
    await RefreshAsync();
}
```

Use the attribute's `CanExecute` option and
`[NotifyCanExecuteChangedFor]` when command availability depends on observable
state. Do not allocate `RelayCommand` or `AsyncRelayCommand` manually unless the
command truly requires runtime composition that an attribute cannot express.

All containing types in the source-generation chain must be `partial`. A clean
solution build should have no MVVM Toolkit analyzer warnings.
