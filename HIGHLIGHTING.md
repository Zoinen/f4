# Custom File Highlighting in f4

`f4` features a highly flexible file highlighting system modeled after `far2l` and Far Manager. It allows you to colorize file entries and prepend custom visual markers on panels based on file names, glob masks, file sizes, timestamps, and cross-platform attributes.

---

## 1. How It Works

Highlighting is configured via the `highlight.ini` file located in the `f4` configuration directory:
* **Linux/macOS/BSD:** `~/.config/f4/highlight.ini`
* **Windows:** `%APPDATA%\f4\highlight.ini` (or `f4/Profile/highlight.ini` in portable mode)

On the very first launch, `f4` automatically generates a default, balanced `highlight.ini` file with basic rules for executables, archives, hidden files, and directories. You can modify this file to customize colors and visual marker glyphs.

---

## 2. Configuration Format

The file is parsed as a standard INI file. Rules are defined in sections starting with `[Highlight_N]`, where `N` is a non-negative integer indicating the rule's precedence (lower indices are evaluated first).

### Available Parameters per Rule

| Parameter | Type | Description |
| :--- | :--- | :--- |
| `Name` | String | A descriptive label for the rule (used for logging and human reference). |
| `Mask` | String | Comma-separated list of glob patterns (e.g., `*.zip, *.tar.gz`). |
| `IncludeAttributes` | String | Comma-separated attributes that **must** be present (see below). |
| `ExcludeAttributes` | String | Comma-separated attributes that **must not** be present. |
| `SizeAbove` | Integer | Minimum file size in bytes (e.g., `1048576` for 1MB). |
| `SizeBelow` | Integer | Maximum file size in bytes. |
| `DateType` | String | Target timestamp: `Modified` (default), `Created`, or `Accessed`. |
| `DateRelative` | Boolean | If `1`, `DateAfter`/`DateBefore` are relative intervals (e.g., `2d`). |
| `DateAfter` | String | Match if file timestamp is after this. Absolute format: `YYYY-MM-DD HH:MM:SS`. |
| `DateBefore` | String | Match if file timestamp is before this. |
| `Mark` (or `MarkChar`) | String | A single character/glyph to prepend before the filename on panels. |
| `ContinueProcessing`| Boolean | If `1`, matching continues to subsequent rules, merging colors. |
| `NormalColor` | String | Color expression for unmodified files. |
| `SelectedColor` | String | Color expression for selected files. |
| `CursorColor` | String | Color expression for files currently under the cursor. |
| `SelectedCursorColor` | String| Color expression for selected files under the cursor. |

---

## 3. Supported Attributes

You can filter files by specifying the following flags in `IncludeAttributes` or `ExcludeAttributes` (comma-separated, case-insensitive):

* `Directory` (or `dir`, `d`): Match directories.
* `Hidden` (or `h`): Match hidden files.
* `Executable` (or `exec`, `e`): Match executable files.
* `ReadOnly` (or `ro`): Match write-protected files (lacking write perms on Unix, or having the read-only attribute on Windows).
* `System` (or `sys`): Match Windows system files.
* `Archive` (or `arc`): Match Windows archive files.

---

## 4. Date and Time Filtering

Dates can be evaluated either absolutely or relatively:

* **Absolute Filtering (`DateRelative = 0`):**
  Matches if the file's timestamp falls within an exact time frame.
  ```ini
  DateRelative = 0
  DateAfter = 2026-01-01 00:00:00
  ```
* **Relative Filtering (`DateRelative = 1`):**
  Matches files modified/created recently. The values are parsed as Go-style durations (`h` for hours, `m` for minutes, `s` for seconds) or with the custom `d` suffix representing days (e.g., `2d` or `7d`).
  ```ini
  DateRelative = 1
  DateAfter = 48h    # Modified within the last 48 hours
  DateBefore = 2h    # But excluding those modified in the last 2 hours
  ```

---

## 5. Color Expressions and Cascade Blending

Color expressions match the standard f4 format: `foreground:<color> | background:<color>`, where `<color>` is a hex RGB value (e.g. `#8AE234` or `#0000FF`).

### Cascade Blending (`ContinueProcessing = 1`)

Normally, `f4` evaluates rules from top to bottom and stops on the first match. However, if `ContinueProcessing = 1` is specified, `f4` merges the colors of this rule with subsequent matches.

Since f4 color parsing is transparent, if a rule only specifies `foreground:#FF0000`, the background will be inherited from the panels (or from subsequent matching rules), allowing layered styles.

---

## 6. Full Example Configuration

```ini
[Highlight_0]
Name = Temporary files
Mask = *.tmp, *.bak, *~
Mark = •
NormalColor = foreground:#888888

[Highlight_1]
Name = Huge Logs
Mask = *.log
SizeAbove = 104857600   # > 100MB
NormalColor = foreground:#FF5555 | background:#220000
SelectedColor = foreground:#FF5555 | background:#0000A0

[Highlight_2]
Name = Executables
Mask = *.sh, *.bash
IncludeAttributes = Executable
ExcludeAttributes = Directory
Mark = *
NormalColor = foreground:#8AE234

[Highlight_3]
Name = Read-Only Files
IncludeAttributes = ReadOnly
Mark = 🔒
NormalColor = foreground:#D3D7CF
ContinueProcessing = 1   # Apply the lock marker but let other extensions colorize

[Highlight_4]
Name = Archives
Mask = *.zip, *.tar, *.gz, *.7z
NormalColor = foreground:#AD7FA8
SelectedColor = foreground:#AD7FA8 | background:#0000A0
```