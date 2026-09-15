//go:build !windows && !darwin

package filemenu

func PanelPoint(x, y, cols, rows int) Point { return Point{} }
