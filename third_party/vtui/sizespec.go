package vtui

import "strings"

// Policy defines how a widget behaves when extra space is distributed or deficit occurs.
type Policy int

const (
	PolicyPreferred Policy = iota
	PolicyFixed
	PolicyMinimum
	PolicyMaximum
	PolicyExpanding
)

func (p Policy) String() string {
	switch p {
	case PolicyFixed:
		return "fixed"
	case PolicyMinimum:
		return "minimum"
	case PolicyMaximum:
		return "maximum"
	case PolicyPreferred:
		return "preferred"
	case PolicyExpanding:
		return "expanding"
	default:
		return "preferred"
	}
}

// ParsePolicy parses a policy string name.
func ParsePolicy(s string) Policy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "fixed":
		return PolicyFixed
	case "minimum":
		return PolicyMinimum
	case "maximum":
		return PolicyMaximum
	case "expanding":
		return PolicyExpanding
	default:
		return PolicyPreferred
	}
}

// SizeSpec defines size hints, minimums, maximums, and policy along one layout axis.
type SizeSpec struct {
	Hint    int
	Min     int
	Max     int
	Policy  Policy
	Stretch int
}

// SizeSpecH returns the horizontal size specification for the element.
func (so *ScreenObject) SizeSpecH() SizeSpec {
	if so.sizeSpecH != nil {
		return *so.sizeSpecH
	}
	minW := so.minW
	hint := 0
	if so.cleanText != "" {
		hint = StringWidth(so.cleanText)
	}
	if minW > hint {
		hint = minW
	}
	stretch := so.stretch
	if stretch <= 0 {
		stretch = 1
	}
	return SizeSpec{
		Hint:    hint,
		Min:     minW,
		Max:     so.maxW,
		Policy:  PolicyPreferred,
		Stretch: stretch,
	}
}

// SizeSpecV returns the vertical size specification for the element.
func (so *ScreenObject) SizeSpecV() SizeSpec {
	if so.sizeSpecV != nil {
		return *so.sizeSpecV
	}
	minH := so.minH
	if minH < 1 {
		minH = 1
	}
	stretch := so.stretch
	if stretch <= 0 {
		stretch = 1
	}
	return SizeSpec{
		Hint:    minH,
		Min:     minH,
		Max:     so.maxH,
		Policy:  PolicyFixed,
		Stretch: stretch,
	}
}

// SetSizeSpecH sets the horizontal size specification.
func (so *ScreenObject) SetSizeSpecH(s SizeSpec) {
	so.sizeSpecH = &s
}

// SetSizeSpecV sets the vertical size specification.
func (so *ScreenObject) SetSizeSpecV(s SizeSpec) {
	so.sizeSpecV = &s
}
