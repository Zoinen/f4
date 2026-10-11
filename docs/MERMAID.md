# Formulas and Mermaid diagrams in Markdown

[f4#1664](https://github.com/unxed/f4/issues/1664): the terminal Markdown views show LaTeX formulas and Mermaid diagrams as plain Unicode text.

Where it applies: the formatted view of a `.md` file (F3), and in the editor the preview (Shift+F3) and the preview beside the editor (Alt+Shift+F3), which is rebuilt shortly after typing stops.

A terminal cannot typeset or draw, so both are turned into readable text before the document reaches the viewer. What cannot be converted with confidence is **not guessed at**: it stays as its source in a code span or block, which is exact.

## Formulas

`$...$` inside a line and `$$...$$` blocks become Unicode text: Greek letters, operators and relations, super- and subscripts, fractions, roots and the like. A formula that uses anything outside that subset stays as its source. Fenced code and inline code spans are never touched, and a lone dollar sign (a price) stays a dollar sign.

## Mermaid

A fenced block marked `mermaid` is converted to text: one line per connection, nodes in their own brackets, arrows drawn with box characters, edge labels inline. Supported diagram types:

* **flowchart / graph**: node shapes, edge styles (`-->`, `==>`, `-.->`, both directions, labels), `subgraph` (nested), `&` for several nodes at once and `:::` class markers;
* **sequenceDiagram**, **classDiagram**, **stateDiagram**, **erDiagram**;
* **pie** and **gantt**.

A diagram of any other type, with syntax the reader does not fully understand, or larger than the limit (400 lines, 600 edges), is shown as its source.
