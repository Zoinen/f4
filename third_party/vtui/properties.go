package vtui

import (
	"errors"
	"fmt"
)

var (
	ErrUnknownProperty = errors.New("vtui: unknown property")
	ErrPropertyType    = errors.New("vtui: property type mismatch")
)

// PropKind enumerates data types supported by the declarative property system.
type PropKind int

const (
	PropString PropKind = iota
	PropInt
	PropBool
	PropColor
	PropStringList
	PropRect
)

func (k PropKind) String() string {
	switch k {
	case PropString:
		return "string"
	case PropInt:
		return "int"
	case PropBool:
		return "bool"
	case PropColor:
		return "color"
	case PropStringList:
		return "stringList"
	case PropRect:
		return "rect"
	default:
		return "unknown"
	}
}

// PropValue represents a typed property value without reflection overhead.
type PropValue struct {
	Kind PropKind
	S    string
	I    int
	B    bool
	C    uint64
	L    []string
	R    Rect
}

func (p PropValue) String() string {
	switch p.Kind {
	case PropString:
		return fmt.Sprintf("PropValue(string: %q)", p.S)
	case PropInt:
		return fmt.Sprintf("PropValue(int: %d)", p.I)
	case PropBool:
		return fmt.Sprintf("PropValue(bool: %v)", p.B)
	case PropColor:
		return fmt.Sprintf("PropValue(color: %#x)", p.C)
	case PropStringList:
		return fmt.Sprintf("PropValue(stringList: %v)", p.L)
	case PropRect:
		return fmt.Sprintf("PropValue(rect: %d,%d-%d,%d)", p.R.X1, p.R.Y1, p.R.X2, p.R.Y2)
	default:
		return "PropValue(invalid)"
	}
}

func PropValString(v string) PropValue       { return PropValue{Kind: PropString, S: v} }
func PropValInt(v int) PropValue             { return PropValue{Kind: PropInt, I: v} }
func PropValBool(v bool) PropValue           { return PropValue{Kind: PropBool, B: v} }
func PropValColor(v uint64) PropValue        { return PropValue{Kind: PropColor, C: v} }
func PropValStringList(v []string) PropValue { return PropValue{Kind: PropStringList, L: v} }
func PropValRect(v Rect) PropValue           { return PropValue{Kind: PropRect, R: v} }

// PropertyAccess allows inspecting and mutating widget properties by name.
type PropertyAccess interface {
	SetProperty(name string, v PropValue) error
	GetProperty(name string) (PropValue, bool)
}

// SetProperty implements base PropertyAccess on ScreenObject.
func (so *ScreenObject) SetProperty(name string, v PropValue) error {
	switch name {
	case "id":
		if v.Kind != PropString {
			return ErrPropertyType
		}
		so.SetID(v.S)
		return nil
	case "visible":
		if v.Kind != PropBool {
			return ErrPropertyType
		}
		so.SetVisible(v.B)
		return nil
	case "enabled":
		if v.Kind != PropBool {
			return ErrPropertyType
		}
		so.SetDisabled(!v.B)
		return nil
	case "help":
		if v.Kind != PropString {
			return ErrPropertyType
		}
		so.SetHelp(v.S)
		return nil
	case "grow":
		if v.Kind != PropInt {
			return ErrPropertyType
		}
		so.SetGrowMode(GrowMode(v.I))
		return nil
	case "align":
		if v.Kind != PropString {
			return ErrPropertyType
		}
		so.align = v.S
		return nil
	case "stretch":
		if v.Kind != PropInt {
			return ErrPropertyType
		}
		so.stretch = v.I
		return nil
	case "minWidth":
		if v.Kind != PropInt {
			return ErrPropertyType
		}
		so.minW = v.I
		return nil
	case "minHeight":
		if v.Kind != PropInt {
			return ErrPropertyType
		}
		so.minH = v.I
		return nil
	case "maxWidth":
		if v.Kind != PropInt {
			return ErrPropertyType
		}
		so.maxW = v.I
		return nil
	case "maxHeight":
		if v.Kind != PropInt {
			return ErrPropertyType
		}
		so.maxH = v.I
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrUnknownProperty, name)
	}
}

// GetProperty implements base PropertyAccess on ScreenObject.
func (so *ScreenObject) GetProperty(name string) (PropValue, bool) {
	switch name {
	case "id":
		return PropValString(so.ID()), true
	case "visible":
		return PropValBool(so.IsVisible()), true
	case "enabled":
		return PropValBool(!so.IsDisabled()), true
	case "help":
		return PropValString(so.helpTopic), true
	case "grow":
		return PropValInt(int(so.GetGrowMode())), true
	case "align":
		align := so.align
		if align == "" {
			align = "fill"
		}
		return PropValString(align), true
	case "stretch":
		stretch := so.stretch
		if stretch == 0 {
			stretch = 1
		}
		return PropValInt(stretch), true
	case "minWidth":
		return PropValInt(so.minW), true
	case "minHeight":
		return PropValInt(so.minH), true
	case "maxWidth":
		return PropValInt(so.maxW), true
	case "maxHeight":
		return PropValInt(so.maxH), true
	default:
		return PropValue{}, false
	}
}
