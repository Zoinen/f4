//go:build !windows

package vtui

// consoleFontIsRaster: only the classic Windows console has raster fonts.
func consoleFontIsRaster() bool { return false }
