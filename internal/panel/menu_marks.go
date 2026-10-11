package panel

// MenuCheckMark marks the chosen row of a menu. It is the square-root sign the
// side menus of the panels already use, because the check mark ✓ is missing
// from the fonts of many consoles and shows up as a question mark (f4#1769).
const MenuCheckMark = "√"

// SortModeMarker marks the sort mode in force in the sort menu with its
// direction: ▲ for ascending, ▼ for descending, as the arrows in the column
// header do.
func SortModeMarker(ascending bool) string {
	if ascending {
		return "▲"
	}
	return "▼"
}
