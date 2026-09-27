pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls.Basic as T

// Qt owns text editing, prefix completion and accepting a model entry.
// Keep this separate from textEditable, whose edits are owned by Go.
F4ComboBox {
    id: control
    editable: true
    focusPolicy: Qt.StrongFocus

    contentItem: T.TextField {
        id: input
        objectName: control.objectName + "TextInput"
        text: control.editText
        font: control.font
        color: control.hostWindow
               ? (control.enabled ? control.hostWindow.textColor : control.hostWindow.mutedText)
               : "white"
        selectionColor: control.hostWindow ? control.hostWindow.selectedBg : "#2c7be5"
        selectedTextColor: control.hostWindow ? control.hostWindow.textColor : "white"
        selectByMouse: true
        leftPadding: control.snap(10)
        rightPadding: control.snap(30)
        topPadding: 0
        bottomPadding: 0
        verticalAlignment: TextInput.AlignVCenter
        background: null
        transform: Translate {
            x: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetX(input, control.hostWindow.contentItem) : 0
            y: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetY(input, control.hostWindow.contentItem) : 0
        }
    }
}
