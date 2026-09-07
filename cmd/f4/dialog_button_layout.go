package main

import "github.com/unxed/vtui"

// dialogButtonRows packs fixed-width buttons into centered rows that fit the
// available dialog content width. Long translations therefore wrap instead of
// painting over the dialog border.
func dialogButtonRows(buttons []*vtui.Button, availableWidth, spacing int) []*vtui.HBoxLayout {
	if availableWidth < 1 {
		availableWidth = 1
	}
	if spacing < 0 {
		spacing = 0
	}

	rows := make([]*vtui.HBoxLayout, 0, 1)
	row := vtui.NewHBoxLayout(0, 0, availableWidth, 1)
	row.HorizontalAlign = vtui.AlignCenter
	row.Spacing = spacing
	rowWidth := 0

	for _, button := range buttons {
		if button == nil {
			continue
		}
		x1, _, x2, _ := button.GetPosition()
		buttonWidth := x2 - x1 + 1
		if len(row.Items) > 0 && rowWidth+spacing+buttonWidth > availableWidth {
			rows = append(rows, row)
			row = vtui.NewHBoxLayout(0, 0, availableWidth, 1)
			row.HorizontalAlign = vtui.AlignCenter
			row.Spacing = spacing
			rowWidth = 0
		}
		row.Add(button, vtui.Margins{}, vtui.AlignTop)
		if rowWidth > 0 {
			rowWidth += spacing
		}
		rowWidth += buttonWidth
	}
	if len(row.Items) > 0 {
		rows = append(rows, row)
	}
	return rows
}
