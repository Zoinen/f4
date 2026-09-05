package main

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

// infoUsageMeter is the reusable two-line capacity element used by provider
// data (such as Android), local filesystems, physical memory and paging space.
// It owns both layout and copy semantics; InfoPanel only decides where the
// element belongs and how its filled/unfilled cells are painted.
type infoUsageMeter struct {
	Label     string
	Total     uint64
	Available uint64
}

func (meter infoUsageMeter) rows(section string, innerW, y int) []infoRow {
	return meter.rowsWithWidth(section, innerW, y, 0)
}

// rowsWithWidth lays out a meter at its natural width when meterWidth is zero,
// or at the requested right-aligned width during the panel-wide alignment
// pass.
func (meter infoUsageMeter) rowsWithWidth(section string, innerW, y, requestedMeterWidth int) []infoRow {
	used := panelInfoUsedBytes(meter.Total, meter.Available)
	usedText := formatBytes(used)
	totalText := formatBytes(meter.Total)
	usedLabel := Msg("InfoPanel.UsedShort")
	totalLabel := Msg("InfoPanel.TotalShort")
	copyValue := fmt.Sprintf("%s: %s; %s: %s", usedLabel, usedText, totalLabel, totalText)

	labelPad := " " + meter.Label
	labelWidth := runewidth.StringWidth(labelPad)
	const preferredMeterWidth = 8
	if innerW-labelWidth-1 < preferredMeterWidth {
		labelWidth = innerW - preferredMeterWidth - 1
		if labelWidth < 1 {
			labelWidth = innerW / 3
		}
		if labelWidth < 1 {
			labelWidth = 1
		}
		labelPad = runewidth.Truncate(labelPad, labelWidth, "…")
		labelWidth = runewidth.StringWidth(labelPad)
	}

	meterX := labelWidth + 1
	meterWidth := innerW - meterX
	if requestedMeterWidth > 0 && requestedMeterWidth < meterWidth {
		meterWidth = requestedMeterWidth
		meterX = innerW - meterWidth
		labelRoom := meterX - 1
		if labelRoom < 1 {
			labelRoom = 1
		}
		labelPad = runewidth.Truncate(" "+meter.Label, labelRoom, "…")
	}
	if meterWidth < 1 {
		// Degenerate one-cell interiors cannot display a meaningful meter,
		// but still retain the semantic two-line row without crossing a border.
		meterX = innerW
		meterWidth = 0
	}
	firstText := runewidth.Truncate(labelPad, innerW, "…")
	if meterWidth > 0 {
		firstText += strings.Repeat(" ", meterX-runewidth.StringWidth(firstText)) +
			panelInfoUsageBar(meter.Total, meter.Available, meterWidth)
	}
	barStart, barWidth, barFilled := 0, 0, 0
	if meterWidth > 0 {
		barStart = meterX
		barWidth = meterWidth
		barFilled, _ = panelInfoUsageMetrics(meter.Total, meter.Available, barWidth)
	}

	secondText := ""
	if meterWidth > 0 {
		secondText = strings.Repeat(" ", meterX) + panelInfoUsageLegend(
			usedLabel, usedText, totalLabel, totalText, meterWidth)
	}

	return []infoRow{
		{
			section: section,
			label:   meter.Label, value: copyValue, copyable: true,
			text:            runewidth.Truncate(firstText, innerW, "…"),
			usageBarStart:   barStart,
			usageBarWidth:   barWidth,
			usageBarFilled:  barFilled,
			usageMeterWidth: meterWidth,
			usageTotal:      meter.Total,
			usageAvailable:  meter.Available,
			y:               y,
		},
		{
			section:           section,
			label:             meter.Label,
			text:              runewidth.Truncate(secondText, innerW, "…"),
			usageMeterWidth:   meterWidth,
			usageTotal:        meter.Total,
			usageAvailable:    meter.Available,
			usageContinuation: true,
			y:                 y + 1,
		},
	}
}

// alignInfoUsageMeters makes every meter as narrow as the naturally narrowest
// one and pins all of their right edges to the panel border. The second lines
// are rebuilt too, so their Used/Total legends form the same visual column.
func alignInfoUsageMeters(rows []infoRow, innerW int) {
	minWidth := 0
	for _, row := range rows {
		if row.usageContinuation || row.usageMeterWidth <= 0 {
			continue
		}
		if minWidth == 0 || row.usageMeterWidth < minWidth {
			minWidth = row.usageMeterWidth
		}
	}
	if minWidth == 0 {
		return
	}
	for i := 0; i+1 < len(rows); i++ {
		row := rows[i]
		if row.usageContinuation || row.usageMeterWidth <= 0 {
			continue
		}
		meter := infoUsageMeter{Label: row.label, Total: row.usageTotal, Available: row.usageAvailable}
		aligned := meter.rowsWithWidth(row.section, innerW, row.y, minWidth)
		rows[i], rows[i+1] = aligned[0], aligned[1]
		i++
	}
}

func panelInfoUsedBytes(total, available uint64) uint64 {
	if available > total {
		available = total
	}
	return total - available
}

func panelInfoUsageMetrics(total, available uint64, width int) (filled, percent int) {
	if width <= 0 {
		return 0, 0
	}
	used := panelInfoUsedBytes(total, available)
	if total != 0 {
		filled = int(float64(used)/float64(total)*float64(width) + 0.5)
		percent = int(float64(used)/float64(total)*100 + 0.5)
	}
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return filled, percent
}

func panelInfoUsageBar(total, available uint64, width int) string {
	if width <= 0 {
		return ""
	}
	_, percent := panelInfoUsageMetrics(total, available, width)
	cells := []rune(strings.Repeat(" ", width))
	percentText := []rune(fmt.Sprintf("%d%%", percent))
	if width >= 5 && len(percentText) <= width {
		start := (width - len(percentText)) / 2
		copy(cells[start:], percentText)
	}
	return string(cells)
}

func panelInfoUsageLegend(usedLabel, used, totalLabel, total string, width int) string {
	if width <= 0 {
		return ""
	}
	left := usedLabel + ": " + used
	right := totalLabel + ": " + total
	if runewidth.StringWidth(left)+1+runewidth.StringWidth(right) > width {
		left, right = used, total
	}
	if width == 1 {
		return runewidth.Truncate(left, width, "…")
	}
	leftLimit := (width - 1) / 2
	rightLimit := width - 1 - leftLimit
	left = runewidth.Truncate(left, leftLimit, "…")
	right = runewidth.Truncate(right, rightLimit, "…")
	gap := width - runewidth.StringWidth(left) - runewidth.StringWidth(right)
	if gap < 1 {
		gap = 1
	}
	return runewidth.Truncate(left+strings.Repeat(" ", gap)+right, width, "…")
}
