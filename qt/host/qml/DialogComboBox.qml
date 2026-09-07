pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

F4ComboBox {
    id: dialogCombo
    required property var widget
    property string registeredOwnerId: ""

    function syncDropdownRegistration() {
        if (!hostWindow
                || typeof hostWindow.registerDropdownAnchor !== "function"
                || typeof hostWindow.unregisterDropdownAnchor !== "function")
            return
        const nextId = widget.dropdownOnly === true
                ? hostWindow.cleanText(widget.id) : ""
        if (registeredOwnerId !== "" && registeredOwnerId !== nextId)
            hostWindow.unregisterDropdownAnchor(registeredOwnerId, dialogCombo)
        registeredOwnerId = nextId
        if (registeredOwnerId !== "")
            hostWindow.registerDropdownAnchor(registeredOwnerId, dialogCombo)
    }

    model: widget.items || []
    textRole: "text"
    currentIndex: Math.max(0, Number(widget.selected || 0))
    displayText: hostWindow.cleanText(widget.text)
    // Go's ComboBox keeps the edit field live even when the Qt surface is
    // only a projection.  Reflect that distinction to the native control so
    // editable combos get the text-edit pointer, while DropdownOnly combos
    // retain their pointing-hand affordance and semantic popup path.
    editable: widget.dropdownOnly !== true
    semanticFocus: widget.focused === true
    externallyOwnedPopup: widget.dropdownOnly === true

    Component.onCompleted: syncDropdownRegistration()
    onWidgetChanged: syncDropdownRegistration()

    onExternalPopupRequested: hostWindow.action({
        "target": widget.id,
        "action": "control.open"
    }, true)

    onActivated: (index) => hostWindow.action({
        "target": widget.id,
        "action": "control.select",
        "index": index
    })
}
