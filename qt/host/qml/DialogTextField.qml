pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Rectangle {
    id: dialogEdit
    required property ApplicationWindow hostWindow
    required property var widget

    color: hostWindow.controlPressedBg
    border.width: 1
    border.color: widget.focused ? hostWindow.dialogAccent : hostWindow.controlBorder
    radius: 4
    clip: true

    Behavior on border.color { ColorAnimation { duration: 90 } }

    TextInput {
        id: dialogTextInput
        anchors.fill: parent
        anchors.leftMargin: 10
        anchors.rightMargin: 10
        readOnly: true
        focus: false
        text: hostWindow.cleanText(dialogEdit.widget.text)
        color: hostWindow.textColor
        selectionColor: hostWindow.selectedBg
        selectedTextColor: hostWindow.textColor
        font.pixelSize: 13
        verticalAlignment: TextInput.AlignVCenter
        echoMode: dialogEdit.widget.password
                  ? TextInput.Password : TextInput.Normal
        cursorPosition: Math.max(0, Math.min(length,
                                  Number(dialogEdit.widget.cursor || 0)))
        cursorVisible: dialogEdit.widget.focused === true

        cursorDelegate: Rectangle {
            id: dialogCursor
            property bool blinkOn: true
            width: 1
            color: hostWindow.textColor
            opacity: blinkOn ? 1 : 0

            Timer {
                interval: 480
                running: dialogTextInput.cursorVisible
                repeat: true
                onTriggered: dialogCursor.blinkOn = !dialogCursor.blinkOn
            }
        }
    }

    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.LeftButton
        onPressed: hostWindow.action({
            "target": dialogEdit.widget.id,
            "action": "control.focus"
        })
    }
}
