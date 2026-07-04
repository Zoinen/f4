package vtui

import (
	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
)

func gridNav(idx, count, cols int, vk uint16) (int, bool) {
	if cols < 1 {
		cols = 1
	}
	row := idx / cols
	col := idx % cols
	rows := (count + cols - 1) / cols

	switch vk {
	case vtinput.VK_UP:
		if row > 0 {
			return idx - cols, true
		} else if col > 0 {
			// Snake: bottom of previous column
			lastRowInPrev := rows - 1
			newIdx := lastRowInPrev*cols + (col - 1)
			for newIdx >= count {
				newIdx -= cols
			}
			return newIdx, true
		}
	case vtinput.VK_DOWN:
		if idx+cols < count {
			return idx + cols, true
		} else if col < cols-1 && col+1 < count {
			// Snake: top of next column
			return col + 1, true
		}
	case vtinput.VK_LEFT:
		if col > 0 {
			return idx - 1, true
		} else if row > 0 {
			// Snake: end of previous row
			return idx - 1, true
		}
	case vtinput.VK_RIGHT:
		if col < cols-1 && idx+1 < count {
			return idx + 1, true
		} else if row < rows-1 && idx+1 < count {
			// Snake: start of next row
			return idx + 1, true
		}
	}
	return idx, false
}

// calcGridColWidths calculates column widths for grid-based UI groups.
func calcGridColWidths(cols int, items []string) []int {
	widths := make([]int, cols)
	for i, itm := range items {
		c := i % cols
		clean, _, _ := ParseAmpersandString(itm)
		w := 6 + runewidth.StringWidth(clean) // 4 for prefix + 2 padding
		if w > widths[c] {
			widths[c] = w
		}
	}
	return widths
}

// getGridIndexFromMouse maps a mouse click coordinate to a grid index.
func getGridIndexFromMouse(x1, y1, mx, my, columns int, colWidths []int) int {
	row := my - y1
	col := 0
	cx := x1
	for c := 0; c < columns; c++ {
		if mx >= cx && mx < cx+colWidths[c] {
			col = c
			break
		}
		cx += colWidths[c]
	}
	return row*columns + col
}

// handleGridBoundaryNav determines if a navigation key should be swallowed
// or allowed to pass through for exiting the group.
func handleGridBoundaryNav(vk uint16, currentIndex, itemCount int) bool {
	if vk == vtinput.VK_UP || vk == vtinput.VK_DOWN ||
		vk == vtinput.VK_LEFT || vk == vtinput.VK_RIGHT {
		movingBack := vk == vtinput.VK_UP || vk == vtinput.VK_LEFT
		movingForward := vk == vtinput.VK_DOWN || vk == vtinput.VK_RIGHT

		if movingBack && currentIndex == 0 {
			return false // Exit to previous control
		}
		if movingForward && currentIndex == itemCount-1 {
			return false // Exit to next control
		}
		return true // Stay in group (swallow the key)
	}
	return false
}
