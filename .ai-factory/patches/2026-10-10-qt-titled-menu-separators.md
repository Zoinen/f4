# Preserve titled menu separators in Qt

The Go disks menu exports section captions as `separator: true` with nonempty
`text`, not necessarily as `header: true`. Qt previously hid every separator's
text and allocated only the thin-rule height.

Render nonempty separator captions left-aligned in the rule, with the line
continuing after the text, rather than putting a heading below the rule. Use one
row-height function for delegates and scroll/anchor geometry,
passing the actual item rather than using its stable semantic index as an array
offset. Keep plain separators thin and both variants nonselectable.

Regression checks must exercise `separator: true` with text as well as empty
separators and explicit headers. Check visible text leaves and physical pixel
alignment at fractional DPR; protocol-only assertions cannot catch hidden text.
