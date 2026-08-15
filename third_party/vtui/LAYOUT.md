# vtui Layout Engine

The `vtui` Layout Engine provides a simple, declarative way to arrange UI elements inside dialogs and windows. It eliminates the need for manual coordinate math (e.g., `x = dlg.X1 + 2; y = dlg.Y1 + 5`), making your UI code cleaner, easier to maintain, and less prone to overlapping bugs.

## Core Concepts

The engine is based on two primary containers:
1. **`VBoxLayout`**: Stacks elements vertically (top to bottom).
2. **`HBoxLayout`**: Stacks elements horizontally (left to right).

Instead of setting `X` and `Y` manually, you add elements to a layout container and specify **Margins** and **Alignment**.

### Margins
`Margins{Left, Top, Right, Bottom}` define the empty space around an element.
* In a `VBoxLayout`, `Top` and `Bottom` margins add vertical spacing between stacked items.
* In an `HBoxLayout`, `Left` and `Right` margins add horizontal spacing.

### Alignment
`vtui.Alignment` dictates how an element behaves within the layout's available space:
* `AlignLeft` / `AlignRight` / `AlignCenter`: Positions the element horizontally (VBox) or vertically (HBox) using its inherent width/height.
* `AlignFill`: Stretches the element to fill all available space in the cross-axis, minus the specified margins.

## Usage Example

Here is how you build a standard input dialog without calculating a single coordinate:

```go
dlg := vtui.NewCenteredDialog(40, 10, " User Info ")

// 1. Create elements with dummy coordinates (0, 0)
nameEdit := vtui.NewEdit(0, 0, 10, "")
ageEdit := vtui.NewEdit(0, 0, 10, "")
btnOk := vtui.NewButton(0, 0, "&Save")
btnCancel := vtui.NewButton(0, 0, "&Cancel")

// 2. Define the main vertical layout area
areaX, areaY := dlg.X1+2, dlg.Y1+2
areaW := 40 - 4

vbox := vtui.NewVBoxLayout(areaX, areaY, areaW, 6)

// Add items top-to-bottom
vbox.Add(vtui.NewLabel(0, 0, "Name:", nameEdit), vtui.Margins{}, vtui.AlignLeft)
vbox.Add(nameEdit, vtui.Margins{Top: 1}, vtui.AlignFill) // Stretches horizontally
vbox.Add(vtui.NewLabel(0, 0, "Age:", ageEdit), vtui.Margins{Top: 1}, vtui.AlignLeft)
vbox.Add(ageEdit, vtui.Margins{Top: 1}, vtui.AlignFill)

// 3. Apply coordinates to widgets
vbox.Apply()

// 4. Create a horizontal layout for buttons
hbox := vtui.NewHBoxLayout(areaX, dlg.Y1+8, areaW, 1)
hbox.HorizontalAlign = vtui.AlignCenter // Center the whole block of buttons
hbox.Spacing = 2                        // 2 spaces between buttons

hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)

hbox.Apply()

// 5. Add to Dialog
dlg.AddItem(...) // Add all elements to dlg
```

## Best Practices
* **Use Layouts for structured forms:** Forms with labels, inputs, and checkboxes benefit massively from `VBoxLayout`.
* **Use `GrowMode` for resizing:** The Layout engine is currently a "one-time calculator" used during initialization. If your dialog supports manual resizing by the user, combine the initial Layout setup with `SetGrowMode` (e.g., `GrowHiX | GrowHiY`) so widgets resize dynamically without re-running the layout engine.## Reference Implementation (Best Practice)

For a complete example of a complex layout with nested horizontal rows and a filling vertical center, see **`SelectFileDialog`** in `vtui/common_dialogs.go`.

Key patterns demonstrated there:
1. **The Root Stack**: A `VBoxLayout` that defines the overall vertical spacing of the dialog.
2. **The "Label + Input" Row**: Using an `HBoxLayout` where the Label has a fixed margin and the Edit field uses `AlignFill` to take up the remaining width.
3. **The Expansion Area**: Placing a `ListBox` or `Table` in the middle of a `VBoxLayout` with `AlignFill` so it scales with the dialog's height.
4. **Grouped Buttons**: An `HBoxLayout` with `HorizontalAlign = AlignCenter` and `Spacing = 2` to create a professional-looking button bar at the bottom.
