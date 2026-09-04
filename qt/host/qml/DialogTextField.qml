pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

F4TextField {
    id: dialogEdit

    required property var widget
    remoteControlled: true
    readOnly: true
    text: hostWindow.cleanText(widget.text)
    echoMode: widget.password ? TextInput.Password : TextInput.Normal
    remoteCursorPosition: Math.max(0, Math.min(text.length, Number(widget.cursor || 0)))
    remoteCursorVisible: widget.focused === true
    semanticFocus: widget.focused === true

    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.LeftButton
        onPressed: dialogEdit.hostWindow.action({
            "target": dialogEdit.widget.id,
            "action": "control.focus"
        })
    }
}
