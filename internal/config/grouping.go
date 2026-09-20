package config

import (
	"math"
	"strconv"
)

func parseGroupLimit(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

// ValidPanelGroupLimits validates the complete set before any setting is applied.
func ValidPanelGroupLimits(small, medium, large int) bool {
	return small > 0 && medium > small && large > medium && int64(large) <= math.MaxInt64/(1<<20)
}

// PanelGroupLimits returns safe inclusive byte limits even for an old config.
func PanelGroupLimits() [3]int64 {
	small, medium, large := App.PanelGroupSmallMiB, App.PanelGroupMediumMiB, App.PanelGroupLargeMiB
	if !ValidPanelGroupLimits(small, medium, large) {
		small, medium, large = 5, 10, 100
	}
	return [3]int64{int64(small) << 20, int64(medium) << 20, int64(large) << 20}
}
