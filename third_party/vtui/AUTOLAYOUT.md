# AutoLayout Engine (Discrete Cassowary)

The `vtui` framework provides a powerful declarative layout engine called `AutoLayout`. It is built on top of **Discrete Cassowary** (`kiwi-go`), allowing developers to build complex, responsive Terminal User Interfaces without writing brittle coordinate math or manual recalculation logic.

## Why Discrete Cassowary?

Standard layout engines (like CSS Flexbox) solve constraints in a continuous (floating-point) mathematical space. However, TUI applications render on a strict integer grid of character cells. Naïve rounding of continuous layouts leads to severe visual defects:

1. **Fractional Gaps & Overflows:** If you split an 80-character dialog into 3 equal columns, standard math yields `26.666`. Truncating gives `26+26+26=78` (2 chars of dead space). Rounding gives `27+27+27=81` (overflows the dialog).
2. **Double-width Misalignment:** Certain CJK characters take 2 grid cells. TUI elements snapping to boundaries must respect this step.

The `AutoLayout` engine solves this by borrowing **Font Hinting heuristics** (FreeType/TrueType algorithms) to map continuous solutions onto the discrete grid perfectly, distributing remainders automatically.

---

## Basic Example

Creating an `AutoLayout` involves wrapping a bounding box inside your dialog, registering `UIElements`, specifying relationships via a fluent API, and executing `Apply()`.

```go
dlg := vtui.NewCenteredDialog(40, 10, " Simple Login ")

// 1. Instantiate UI elements with dummy (0, 0) coordinates
lbl := vtui.NewLabel(0, 0, "User:", nil)
edit := vtui.NewEdit(0, 0, 10, "")
btn := vtui.NewButton(0, 0, "&Submit")

// 2. Add elements to the dialog (required for event routing/focus)
dlg.AddItem(lbl)
dlg.AddItem(edit)
dlg.AddItem(btn)

// 3. Create an AutoLayout bounds context (inside dialog margins)
layout := vtui.NewAutoLayout(dlg.X1+2, dlg.Y1+2, 36, 6)

// 4. Define constraints fluently
layout.
    PinTop(lbl, 0).PinLeft(lbl, 0).
    StackVertical(1, lbl, edit).FillWidth(edit, 0, 0).
    PinBottom(btn, 0).CenterHorizontal(btn)

// 5. Compute the integer grid
layout.Apply()
```

---

## API Reference

### Edge Pinning
Constrains an element's edges to the outer boundaries of the `AutoLayout` container.
* `PinLeft(el, margin)`
* `PinRight(el, margin)`
* `PinTop(el, margin)`
* `PinBottom(el, margin)`
* `PinEdges(el, margins)`

### Expansion
Forces elements to stretch up to the layout's defined margins.
* `FillWidth(el, marginLeft, marginRight)`
* `FillHeight(el, marginTop, marginBottom)`

### Stacking
Positions a series of elements sequentially, ensuring consistent padding between them.
* `StackVertical(spacing, elements...)`
* `StackHorizontal(spacing, elements...)`

### Alignment
Forces elements to align to each other's specific edges.
* `AlignLeft(elements...)` / `AlignRight(elements...)`
* `AlignTop(elements...)` / `AlignBottom(elements...)`
* `CenterHorizontal(el)` / `CenterVertical(el)`
* `CenterHorizontalGroup(firstEl, lastEl)`: Aligns the center of a *block* of elements to the container.

---

## Font Hinting and Discrete Adjustments

The real power of `AutoLayout` comes from explicitly handling terminal grid rounding.

### `ApportionWidths` (FreeType Autohinting)
Distributes rounding remainders across multiple elements to perfectly match a target width.
```go
// 3 columns aiming to perfectly fill 76 chars with zero gaps/overflows.
layout.StackHorizontal(0, col1, col2, col3).
       ApportionWidths(76, col1, col2, col3)
```

### `EqualizeWidthsGroup`
Forces multiple items to calculate to the exact same discrete integer size (averages out differences).
```go
// Make two side-by-side buttons exactly the same width
layout.EqualizeWidthsGroup(btnOk, btnCancel)
```

### `SnapWidthToGrid`
Forces an element's width to round to a multiple of a given step (useful for double-width grids).
```go
// Force column width to be an even number
layout.SnapWidthToGrid(cjkPanel, 2)
```

---

## Responsive Resizing (`GrowMode` Integration)

`AutoLayout` is deeply integrated with `vtui`'s window resizing architecture (`GrowMode`).
To make your constraints respond dynamically when a user drags the corner of a dialog, simply:

1. Define your rules and call `layout.Apply()` once.
2. Tell the layout container how it stretches relative to its parent using `SetGrowMode`.
3. Add the `AutoLayout` object itself to the dialog/window.

```go
layout := vtui.NewAutoLayout(dlg.X1+2, dlg.Y1+2, 46, 6)

// ... setup constraints ...
layout.Apply()

// Instruct the layout container to stretch down and to the right
layout.SetGrowMode(vtui.GrowHiX | vtui.GrowHiY)

// Register the layout engine as a child of the dialog
dlg.AddItem(layout)
```

When the user resizes the window, the `Dialog` routes size deltas to all children based on their `GrowMode`. The `AutoLayout` object intercepts this, updates its internal constraint variables for its boundaries, and automatically calls `Apply()` to seamlessly re-solve the entire user interface.

**Important Note on Child Elements:**
The actual UI widgets (`Button`, `Edit`, `Text`) managed by the `AutoLayout` should **not** have their own `GrowMode` set. Leave them as `GrowNone`. The `AutoLayout` engine overrides their positions absolutely during the resize cycle, effectively handling their responsiveness on their behalf.
