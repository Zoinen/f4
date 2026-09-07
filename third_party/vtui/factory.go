package vtui

import (
	"errors"
	"fmt"
	"sync"
)

var ErrUnknownType = errors.New("vtui: unknown type name")

var (
	typeRegistryMu sync.RWMutex
	typeRegistry   = make(map[string]func() UIElement)
)

// RegisterType registers a constructor factory for a widget type name.
func RegisterType(typeName string, ctor func() UIElement) {
	typeRegistryMu.Lock()
	defer typeRegistryMu.Unlock()
	typeRegistry[typeName] = ctor
}

// NewByType creates a new UIElement by its registered type name.
func NewByType(typeName string) (UIElement, error) {
	typeRegistryMu.RLock()
	ctor, ok := typeRegistry[typeName]
	typeRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownType, typeName)
	}
	return ctor(), nil
}

func init() {
	RegisterType("Dialog", func() UIElement { return NewDialog(0, 0, 40, 10, "") })
	RegisterType("Window", func() UIElement { return NewWindow(0, 0, 40, 10, "") })
	RegisterType("BorderedFrame", func() UIElement { return NewBorderedFrame(0, 0, 10, 5, DoubleBox, "") })
	RegisterType("GroupBox", func() UIElement { return NewGroupBox(0, 0, 10, 5, "") })
	RegisterType("Button", func() UIElement { return NewButton(0, 0, "") })
	RegisterType("Checkbox", func() UIElement { return NewCheckbox(0, 0, "", false) })
	RegisterType("RadioButton", func() UIElement { return NewRadioButton(0, 0, "", false) })
	RegisterType("RadioGroup", func() UIElement { return NewRadioGroup(0, 0, 1, nil) })
	RegisterType("CheckGroup", func() UIElement { return NewCheckGroup(0, 0, 1, nil) })
	RegisterType("Edit", func() UIElement { return NewEdit(0, 0, 10, "") })
	RegisterType("MultiLineEdit", func() UIElement { return NewMultiLineEdit(0, 0, 10, 3, "") })
	RegisterType("Label", func() UIElement { return NewLabel(0, 0, "", nil) })
	RegisterType("Text", func() UIElement { return NewText(0, 0, "", 0) })
	RegisterType("ListBox", func() UIElement { return NewListBox(0, 0, 10, 5, nil) })
	RegisterType("ComboBox", func() UIElement { return NewComboBox(0, 0, 10, nil) })
	RegisterType("Table", func() UIElement { return NewTable(0, 0, 10, 5, nil) })
	RegisterType("VMenu", func() UIElement { return NewVMenu("") })
	RegisterType("MenuBar", func() UIElement { return NewMenuBar(nil) })
	RegisterType("KeyBar", func() UIElement { return NewKeyBar() })
	RegisterType("StatusLine", func() UIElement { return NewStatusLine() })
	RegisterType("ProgressBar", func() UIElement { return NewProgressBar(0, 0, 10) })
	RegisterType("Separator", func() UIElement { return NewSeparator(0, 0, 10, true, true) })
	RegisterType("Spacer", func() UIElement { return NewSpacer() })
	RegisterType("Desktop", func() UIElement { return NewDesktop() })
}
