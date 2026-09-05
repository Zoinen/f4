# vtui Widget and Property Reference

This file is generated automatically from `vocabulary.json` by `cmd/vtui-gen`. **DO NOT EDIT.**

## Widgets

### `BorderedFrame`

Single or double bordered frame with title

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 5 cells
- `minSize`: 4 × 3 cells
- `sizePolicy`: h=`preferred`, v=`preferred`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `boxType` | `int` | `2` | Border type: 1 (SingleBox) or 2 (DoubleBox) |
| `showClose` | `bool` | `false` | Show close button |
| `title` | `string` | `""` | Frame title |

**Localizable Properties:** `title`

---

### `Button`

Action push button

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 10 × 1 cells
- `minSize`: 6 × 1 cells
- `sizePolicy`: h=`fixed`, v=`fixed`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `command` | `int` | `0` | Command ID sent when clicked |
| `default` | `bool` | `false` | Default button activated on Enter |
| `text` | `string` | `"&Ok"` | Button caption with mnemonic (&) |

**Signals:** `clicked`

**Localizable Properties:** `text`

---

### `CheckGroup`

Independent checkbox cluster in a grid

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 3 cells
- `minSize`: 8 × 1 cells
- `sizePolicy`: h=`preferred`, v=`preferred`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `columns` | `int` | `1` | Number of grid columns |
| `data` | `int` | `0` | Bitmask of checked states |
| `items` | `stringList` | `[]` | List of checkbox labels |

**Signals:** `changed`

**Localizable Properties:** `items`

---

### `Checkbox`

Two- or three-state checkbox

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 16 × 1 cells
- `minSize`: 6 × 1 cells
- `sizePolicy`: h=`preferred`, v=`fixed`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `state` | `int` | `0` | 0 = unchecked, 1 = checked, 2 = indeterminate |
| `text` | `string` | `""` | Checkbox label with mnemonic |
| `threeState` | `bool` | `false` | Enable third indeterminate state |

**Signals:** `changed`

**Localizable Properties:** `text`

---

### `ComboBox`

Dropdown list with optional text entry

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 1 cells
- `minSize`: 6 × 1 cells
- `sizePolicy`: h=`expanding`, v=`fixed`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `dropdownOnly` | `bool` | `false` | Disable manual text typing |
| `items` | `stringList` | `[]` | List of dropdown items |
| `text` | `string` | `""` | Current text in edit field |

**Signals:** `changed`, `selected`

**Localizable Properties:** `items`, `text`

---

### `Desktop`

Background desktop canvas

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 80 × 25 cells
- `minSize`: 10 × 5 cells
- `sizePolicy`: h=`expanding`, v=`expanding`

---

### `Dialog`

Modal dialog window

*Inherits:* `Window`

**Default Geometry:**
- `sizeHint`: 40 × 10 cells
- `minSize`: 10 × 5 cells
- `sizePolicy`: h=`preferred`, v=`preferred`

**Signals:** `closed`

**Localizable Properties:** `title`

---

### `Edit`

Single-line text input field

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 1 cells
- `minSize`: 3 × 1 cells
- `sizePolicy`: h=`expanding`, v=`fixed`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `historyId` | `string` | `""` | Input history list identifier |
| `password` | `bool` | `false` | Mask input characters with asterisks |
| `showHistoryButton` | `bool` | `false` | Show history dropdown arrow [v] |
| `text` | `string` | `""` | Entered text content |

**Signals:** `changed`, `activated`

**Localizable Properties:** `text`

---

### `GroupBox`

Framed group container with header title

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 6 cells
- `minSize`: 6 × 3 cells
- `sizePolicy`: h=`expanding`, v=`preferred`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `title` | `string` | `""` | Group header title |

**Localizable Properties:** `title`

---

### `KeyBar`

Bottom function key bar F1-F12

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 80 × 1 cells
- `minSize`: 36 × 1 cells
- `sizePolicy`: h=`expanding`, v=`fixed`

**Signals:** `clicked`

---

### `Label`

Text label bound to an input field

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 10 × 1 cells
- `minSize`: 1 × 1 cells
- `sizePolicy`: h=`preferred`, v=`fixed`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `buddy` | `string` | `""` | ID of the associated input widget |
| `text` | `string` | `""` | Label text with mnemonic |

**Localizable Properties:** `text`

---

### `ListBox`

Single-column scrollable list

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 8 cells
- `minSize`: 5 × 3 cells
- `sizePolicy`: h=`expanding`, v=`expanding`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `items` | `stringList` | `[]` | List of displayed string items |
| `multiSelect` | `bool` | `false` | Enable multi-selection |
| `selected` | `int` | `0` | Selected item index |

**Signals:** `selected`, `activated`

**Localizable Properties:** `items`

---

### `MenuBar`

Top horizontal menu bar

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 80 × 1 cells
- `minSize`: 10 × 1 cells
- `sizePolicy`: h=`expanding`, v=`fixed`

**Signals:** `activated`

---

### `MultiLineEdit`

Multi-line text input field

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 30 × 5 cells
- `minSize`: 10 × 2 cells
- `sizePolicy`: h=`expanding`, v=`expanding`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `text` | `string` | `""` | Text content with line breaks |

**Signals:** `changed`

**Localizable Properties:** `text`

---

### `ProgressBar`

Progress bar indicator

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 1 cells
- `minSize`: 5 × 1 cells
- `sizePolicy`: h=`expanding`, v=`fixed`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `percent` | `int` | `0` | Completion percentage from 0 to 100 |

---

### `RadioButton`

Individual radio button

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 16 × 1 cells
- `minSize`: 6 × 1 cells
- `sizePolicy`: h=`preferred`, v=`fixed`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `selected` | `bool` | `false` | Selected state |
| `text` | `string` | `""` | Radio button label with mnemonic |

**Signals:** `changed`

**Localizable Properties:** `text`

---

### `RadioGroup`

Mutually exclusive radio button cluster

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 3 cells
- `minSize`: 8 × 1 cells
- `sizePolicy`: h=`preferred`, v=`preferred`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `columns` | `int` | `1` | Number of grid columns |
| `items` | `stringList` | `[]` | List of option labels |
| `selected` | `int` | `0` | Index of selected item |

**Signals:** `changed`

**Localizable Properties:** `items`

---

### `Separator`

Horizontal divider line

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 1 cells
- `minSize`: 2 × 1 cells
- `sizePolicy`: h=`expanding`, v=`fixed`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `connectLeft` | `bool` | `true` | Join left edge with surrounding frame |
| `connectRight` | `bool` | `true` | Join right edge with surrounding frame |

---

### `Spacer`

Flexible layout spacer

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 1 × 1 cells
- `minSize`: 0 × 0 cells
- `sizePolicy`: h=`expanding`, v=`expanding`

---

### `StatusLine`

Contextual status line and hotkey hints

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 80 × 1 cells
- `minSize`: 10 × 1 cells
- `sizePolicy`: h=`expanding`, v=`fixed`

---

### `Table`

Multi-column data table

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 40 × 10 cells
- `minSize`: 10 × 4 cells
- `sizePolicy`: h=`expanding`, v=`expanding`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `quickSearch` | `bool` | `false` | Enable fuzzy quick search |
| `selected` | `int` | `0` | Selected row index |
| `showHeader` | `bool` | `true` | Show column headers |
| `showSeparators` | `bool` | `true` | Show vertical column separators |
| `sortable` | `bool` | `false` | Enable column header click sorting |

**Signals:** `selected`, `activated`

---

### `Text`

Static text label

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 10 × 1 cells
- `minSize`: 1 × 1 cells
- `sizePolicy`: h=`preferred`, v=`fixed`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `text` | `string` | `""` | Displayed text |

**Localizable Properties:** `text`

---

### `VMenu`

Vertical context or popup menu

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 20 × 8 cells
- `minSize`: 8 × 3 cells
- `sizePolicy`: h=`preferred`, v=`preferred`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `selected` | `int` | `0` | Selected menu item index |
| `title` | `string` | `""` | Menu title |

**Signals:** `selected`, `activated`

**Localizable Properties:** `title`

---

### `Widget`

Base abstract UI element

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `align` | `string` | `"fill"` | Alignment within the layout cell (fill, start, center, end) |
| `enabled` | `bool` | `true` | Interactive availability state |
| `grow` | `int` | `0` | GrowMode anchor flags for absolute layout |
| `help` | `string` | `""` | Contextual F1 help topic identifier |
| `id` | `string` | `""` | Unique string identifier of the element |
| `maxHeight` | `int` | `0` | Maximum height (0 = unconstrained) |
| `maxWidth` | `int` | `0` | Maximum width (0 = unconstrained) |
| `minHeight` | `int` | `0` | Minimum height in character cells |
| `minWidth` | `int` | `0` | Minimum width in character cells |
| `stretch` | `int` | `1` | Stretch weight for extra space distribution |
| `visible` | `bool` | `true` | Visibility state of the element |

**Signals:** `focus`, `blur`

---

### `Window`

Non-modal movable window or dialog

*Inherits:* `Widget`

**Default Geometry:**
- `sizeHint`: 60 × 20 cells
- `minSize`: 10 × 5 cells
- `sizePolicy`: h=`preferred`, v=`preferred`

**Properties:**

| Property | Type | Default | Description |
|---|---|---|---|
| `autoSize` | `bool` | `false` | Automatically compute window size from contents |
| `center` | `bool` | `true` | Center window on screen |
| `isWarning` | `bool` | `false` | Use warning (red) color palette |
| `showClose` | `bool` | `true` | Show close button [x] |
| `showZoom` | `bool` | `true` | Show zoom/maximize button [↕] |
| `title` | `string` | `""` | Window title |

**Signals:** `closed`

---

## Layout Containers

### `Absolute`

Classic absolute positioning with GrowMode anchors

### `Form`

Two-column form layout (label + field)

| Property | Type | Default | Description |
|---|---|---|---|
| `margins` | `rect` | `[0 0 0 0]` | Form margins |
| `spacing` | `int` | `1` | Row spacing |

### `Grid`

Two-dimensional grid layout

| Property | Type | Default | Description |
|---|---|---|---|
| `margins` | `rect` | `[0 0 0 0]` | Grid margins |
| `spacing` | `rect` | `[1 1]` | Horizontal and vertical spacing |

### `HBox`

Horizontal sequential layout

| Property | Type | Default | Description |
|---|---|---|---|
| `align` | `string` | `left` | Block alignment (left, center, right) |
| `margins` | `rect` | `[0 0 0 0]` | Margins [top, right, bottom, left] |
| `spacing` | `int` | `1` | Spacing between elements |

### `Stack`

Multi-layer stacked container (displays one child at a time)

| Property | Type | Default | Description |
|---|---|---|---|
| `currentIndex` | `int` | `0` | Active layer index |

### `VBox`

Vertical sequential layout

| Property | Type | Default | Description |
|---|---|---|---|
| `margins` | `rect` | `[0 0 0 0]` | Margins [top, right, bottom, left] |
| `spacing` | `int` | `1` | Spacing between elements |
