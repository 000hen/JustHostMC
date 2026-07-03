using System;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace JustHostMC.App.Controls;

/// <summary>A ComboBox whose editable text filters the available items.</summary>
public sealed class FilterComboBox : ComboBox
{
    private readonly List<object> _allItems = [];
    private readonly List<object> _visibleItems = [];
    private TextBox? _editableTextBox;
    private bool _isUpdatingItems;

    public FilterComboBox()
    {
        IsEditable = true;
        RegisterPropertyChangedCallback(ItemsSourceProperty, OnItemsSourceChanged);
        DropDownOpened += OnDropDownOpened;
        TextSubmitted += OnTextSubmitted;
    }

    protected override void OnApplyTemplate()
    {
        if (_editableTextBox is not null)
            _editableTextBox.TextChanged -= OnFilterTextChanged;

        base.OnApplyTemplate();

        _editableTextBox = FindEditableTextBox(this);
        if (_editableTextBox is not null)
            _editableTextBox.TextChanged += OnFilterTextChanged;
    }

    private void OnItemsSourceChanged(DependencyObject sender, DependencyProperty property)
    {
        if (_isUpdatingItems)
            return;

        _allItems.Clear();
        if (ItemsSource is IEnumerable items)
            _allItems.AddRange(items.Cast<object>());

        _visibleItems.Clear();
        _visibleItems.AddRange(_allItems);
    }

    private void OnFilterTextChanged(object sender, TextChangedEventArgs e)
    {
        if (_isUpdatingItems || sender is not TextBox textBox)
            return;

        var query = textBox.Text.Trim();
        if (SelectedItem is object selected
            && string.Equals(GetFilterText(selected), query, StringComparison.OrdinalIgnoreCase))
        {
            return;
        }

        ApplyFilter(query, textBox.Text);
        IsDropDownOpen = true;
    }

    private void ApplyFilter(string query, string enteredText)
    {
        IEnumerable<object> matches = _allItems;
        if (query.Length > 0)
        {
            matches = matches.Where(item =>
                GetFilterText(item).Contains(query, StringComparison.OrdinalIgnoreCase));
        }

        _visibleItems.Clear();
        _visibleItems.AddRange(matches);
        ReplaceItems(_visibleItems, selectedItem: null, enteredText);
    }

    private void OnDropDownOpened(object? sender, object e)
    {
        if (SelectedItem is not object selected || _visibleItems.Count == _allItems.Count)
            return;

        var selectedText = GetFilterText(selected);
        if (!string.Equals(_editableTextBox?.Text, selectedText, StringComparison.OrdinalIgnoreCase))
            return;

        _visibleItems.Clear();
        _visibleItems.AddRange(_allItems);
        ReplaceItems(_visibleItems, selected, selectedText);
    }

    private void OnTextSubmitted(ComboBox sender, ComboBoxTextSubmittedEventArgs args)
    {
        var query = args.Text.Trim();
        var match = _allItems.FirstOrDefault(item =>
            string.Equals(GetFilterText(item), query, StringComparison.OrdinalIgnoreCase));

        if (match is null && _visibleItems.Count == 1)
            match = _visibleItems[0];

        if (match is null)
            return;

        _isUpdatingItems = true;
        SelectedItem = match;
        Text = GetFilterText(match);
        _isUpdatingItems = false;
        IsDropDownOpen = false;
        args.Handled = true;
    }

    private void ReplaceItems(IEnumerable<object> items, object? selectedItem, string text)
    {
        _isUpdatingItems = true;
        ItemsSource = items.ToList();
        SelectedItem = selectedItem;
        Text = text;
        if (_editableTextBox is not null)
            _editableTextBox.Text = text;
        _isUpdatingItems = false;
    }

    private string GetFilterText(object item) => item.ToString() ?? string.Empty;

    private static TextBox? FindEditableTextBox(DependencyObject parent)
    {
        var childCount = VisualTreeHelper.GetChildrenCount(parent);
        for (var index = 0; index < childCount; index++)
        {
            var child = VisualTreeHelper.GetChild(parent, index);
            if (child is TextBox textBox)
                return textBox;

            var descendant = FindEditableTextBox(child);
            if (descendant is not null)
                return descendant;
        }

        return null;
    }
}
