pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

F4ComboBox {
    id: dialogCombo
    required property var widget

    model: widget.items || []
    textRole: "text"
    currentIndex: Math.max(0, Number(widget.selected || 0))
    displayText: hostWindow.cleanText(widget.text)
    semanticFocus: widget.focused === true

    onActivated: (index) => hostWindow.action({
        "target": widget.id,
        "action": "control.select",
        "index": index
    })
}
