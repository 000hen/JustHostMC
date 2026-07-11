using System.Collections.ObjectModel;
using System.Collections.Specialized;
using System.ComponentModel;

namespace JustHostMC.App.Collections;

/// <summary>
/// An observable collection optimized for appending bursts to a bounded log.
/// A whole burst raises range notifications so WinUI can retain realized rows
/// instead of recreating the entire list.
/// </summary>
public sealed class BoundedObservableCollection<T>(int capacity)
    : ObservableCollection<T> {
    public void AddRange(IEnumerable<T> items) {
        CheckReentrancy();

        var added = items.ToList();
        if (added.Count == 0)
            return;

        if (added.Count > capacity)
            added.RemoveRange(0, added.Count - capacity);

        var removeCount = Math.Max(0, Items.Count + added.Count - capacity);
        if (removeCount > 0) {
            var removed = Items.Take(removeCount).ToList();
            for (var i = 0; i < removeCount; i++) Items.RemoveAt(0);

            NotifyCountAndItemsChanged();
            OnCollectionChanged(new NotifyCollectionChangedEventArgs(
                NotifyCollectionChangedAction.Remove, removed, 0));
        }

        var startIndex = Items.Count;
        foreach (var item in added) Items.Add(item);

        NotifyCountAndItemsChanged();
        OnCollectionChanged(new NotifyCollectionChangedEventArgs(
            NotifyCollectionChangedAction.Add, added, startIndex));
    }

    private void NotifyCountAndItemsChanged() {
        OnPropertyChanged(new PropertyChangedEventArgs(nameof(Count)));
        OnPropertyChanged(new PropertyChangedEventArgs("Item[]"));
    }
}
