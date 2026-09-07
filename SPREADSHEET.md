# Spreadsheet

`f4` ships with a built-in spreadsheet, reached with `Ctrl+Alt+S`, from the
Commands menu, or from the command palette. It is a deliberate re-creation of the spreadsheet that used to
live inside Dos Navigator: the same cell model, the same formula language and
the same key map, so muscle memory carries over.

The implementation is original Go code written against the published DN
documentation and observable behaviour. No Pascal sources were copied, and
nothing here derives from Turbo Vision.

## Files

The native format is a SQLite database, since `f4` already depends on
`ncruces/go-sqlite3`. A sheet is a plain `.f4s.sqlite` file holding three
tables. The extension is double on purpose: the second half is what makes every
SQLite tool -- f4's own client included -- open the file without being told,
and the first half keeps a sheet recognisable as a sheet. Files written under
the earlier `.f4s` name still open.

| table           | contents                                              |
| --------------- | ----------------------------------------------------- |
| `sheet_meta`    | schema version, title, separator display mode         |
| `sheet_columns` | per-column widths                                     |
| `sheet_cells`   | one row per cell: text, display format, justification, decimals, protection |

Only the raw text of a cell is stored; values are recomputed on load, so a file
never carries stale results. `IsSheetFile` checks the schema before offering to
open a database from the panel, which keeps unrelated `.db` files alone.

Import and export cover:

- **XLSX** — read and written directly with `archive/zip` and `encoding/xml`, so
  no new module dependency was added. Formulas are translated in both
  directions (`^` becomes `POWER`, function names are mapped, `@` markers are
  dropped); when a workbook formula cannot be parsed, the cached value is
  imported instead of failing the load.
- **CSV** — for exchanging data with anything else.
- **Plain text** — the "save text" of the original, laid out with the current
  column widths and honouring the separator mode.

## Cells

The kind of a cell follows from what was typed, exactly as it did in DN:

| input                | kind    |
| -------------------- | ------- |
| a decimal number     | value   |
| text starting with `=` | formula |
| anything else        | text    |

Each cell carries a display format (as is, decimal, comma, exponent, logical,
currency, percent, hidden), a justification, a number of decimals, and a
protection flag. Protected cells survive block clears.

## Formulas

The expression language is the DN calculator plus references:

- Infix operators by rising precedence: comparisons (`= # != <> < > >= <=`),
  additive (`+ - | or & and`), multiplicative (`* / div % mod \ >> shr << shl`),
  power (`^`), and time (`:`, where `a:b` is `a*60+b`).
- Prefix `+`, `-`, `~`, and every function, bind tighter than any infix
  operator, so `log 2+1` means `log(2, +1)`.
- Comparisons yield `0xFFFFFFFF` or zero, which lets the bitwise operators act
  as logical ones.
- Functions: trigonometric, hyperbolic, `exp fact lg ln log sqr sqrt root round
  sign`, and `if(cond, then, else)`.
- `sum` and `mul` require parentheses and commas. Their arguments may be
  expressions, references, or rectangular ranges such as `B1:C2`. Inside a
  range, empty and text cells are skipped — this is why `=mul(B1:C2)` and
  `=B1*B2*C1*C2` differ when a cell is blank.

References are relative by default and are rewritten when rows or columns are
inserted or deleted, and when a block is pasted somewhere else. A `@` before the
letter or the digits pins that part: `@A@2` never moves, `@A2` keeps its column,
`A@2` keeps its row. A reference whose target was deleted becomes `#REF`.

Cycles are detected during recalculation and reported on the offending cell
rather than hanging the UI.

## Keys

| key            | action                        |
| -------------- | ----------------------------- |
| `F1`           | help                          |
| `F2` / `Shift+F2` | save / save as             |
| `F3` / `Shift+F3` | open / export              |
| `F5`           | recalculate                   |
| `F6`           | go to cell                    |
| `F7` / `Ctrl+F7` / `Shift+F7` | find / replace / search again |
| `F10`          | menu                          |
| `Alt+I` / `Alt+C` | insert row / column        |
| `Alt+L` / `Alt+D` | delete row / column        |
| `Alt+O`        | cell format                   |
| `Alt+S`        | toggle column separators      |
| `Alt+←` / `Alt+→` | narrow / widen the column  |
| `Alt+BackSpace` | undo                         |
| `Ctrl+Ins` / `Shift+Ins` / `Shift+Del` | copy / paste / cut |
| `Ctrl+Del`, `Del` | clear the block            |
| `Shift`+arrows | mark a block                  |
| `Home` / `End` | first cell / last used cell   |

Typing a printable character starts editing the cell under the cursor; `Enter`
commits and steps down, `Esc` cancels.
