package numeric

import (
	"fmt"
)

// FormatSize renders a byte count the way a file manager shows it. The file
// operations and the media player both print sizes, and two copies are two
// roundings.
// formatSize formats a byte count into a human-readable string.
func FormatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
