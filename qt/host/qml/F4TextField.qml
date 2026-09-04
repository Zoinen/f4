pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import QtQuick.Controls
import QtQuick.Controls.Basic as T

Item {
    id: control

    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property alias text: innerInput.text
    property alias placeholderText: placeholderLabel.text
    property alias readOnly: innerInput.readOnly
    property alias echoMode: innerInput.echoMode
    property alias inputMethodHints: innerInput.inputMethodHints
    property alias validator: innerInput.validator
    property alias font: innerInput.font
    property alias horizontalAlignment: innerInput.horizontalAlignment
    property alias cursorPosition: innerInput.cursorPosition
    property alias selectedText: innerInput.selectedText
    property alias selectionStart: innerInput.selectionStart
    property alias selectionEnd: innerInput.selectionEnd
    property bool acceptableInput: innerInput.acceptableInput

    property bool hasBackground: true
    property url leadingIconSource: ""
    property real leadingIconSize: 14
    property bool semanticFocus: false
    property bool remoteControlled: false
    property int remoteCursorPosition: 0
    property bool remoteCursorVisible: false

    signal accepted()
    signal textEdited()
    signal editingFinished()

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    function selectAll() { innerInput.selectAll() }
    function select(start, end) { innerInput.select(start, end) }
    function copy() { innerInput.copy() }
    function cut() { innerInput.cut() }
    function paste() { innerInput.paste() }
    function forceActiveFocus() { innerInput.forceActiveFocus() }

    implicitHeight: snap(32)
    implicitWidth: snap(180)

    Rectangle {
        id: bgRect
        anchors.fill: parent
        visible: control.hasBackground
        radius: control.snap(4)
        color: control.hostWindow ? control.hostWindow.controlPressedBg : "#18202a"
        border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
        border.color: {
            if (innerInput.activeFocus || control.semanticFocus)
                return control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
            return control.hostWindow ? control.hostWindow.controlBorder : "#25303d"
        }

        Behavior on border.color { ColorAnimation { duration: 90 } }
    }

    RowLayout {
        anchors.fill: parent
        anchors.leftMargin: control.snap(8)
        anchors.rightMargin: control.snap(8)
        spacing: control.snap(6)

        HostPixelAlignedImage {
            id: leadIcon
            hostWindow: control.hostWindow
            visible: control.leadingIconSource.toString() !== ""
            width: control.snap(control.leadingIconSize)
            height: control.snap(control.leadingIconSize)
            sourceSize: Qt.size(control.leadingIconSize, control.leadingIconSize)
            smooth: false
            mipmap: false
            source: control.leadingIconSource
            Layout.alignment: Qt.AlignVCenter
        }

        Item {
            Layout.fillWidth: true
            Layout.fillHeight: true

            Text {
                id: placeholderLabel
                anchors.fill: parent
                verticalAlignment: Text.AlignVCenter
                visible: innerInput.text === "" && !innerInput.inputMethodComposing
                color: control.hostWindow ? control.hostWindow.mutedText : "#666666"
                font: innerInput.font
                elide: Text.ElideRight
            }

            TextInput {
                id: innerInput
                anchors.fill: parent
                verticalAlignment: TextInput.AlignVCenter
                color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
                selectionColor: control.hostWindow ? control.hostWindow.selectedBg : "#2c7be5"
                selectedTextColor: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
                font.pixelSize: 13
                selectByMouse: !control.remoteControlled
                clip: true

                Binding {
                    target: innerInput
                    property: "cursorPosition"
                    value: Math.max(0, Math.min(innerInput.length, Number(control.remoteCursorPosition || 0)))
                    when: control.remoteControlled
                }
                Binding {
                    target: innerInput
                    property: "cursorVisible"
                    value: control.remoteCursorVisible
                    when: control.remoteControlled
                }

                onAccepted: control.accepted()
                onTextEdited: control.textEdited()
                onEditingFinished: control.editingFinished()

                cursorDelegate: Rectangle {
                    id: customCursor
                    property bool blinkOn: true
                    width: 1
                    color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
                    opacity: blinkOn ? 1.0 : 0.0

                    Timer {
                        interval: 480
                        running: innerInput.cursorVisible
                        repeat: true
                        onTriggered: customCursor.blinkOn = !customCursor.blinkOn
                    }
                }
            }
        }
    }

    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.RightButton
        onClicked: (mouse) => {
            if (mouse.button === Qt.RightButton && !control.readOnly) {
                editMenu.popup()
            }
        }
    }

    T.Menu {
        id: editMenu
        T.MenuItem {
            text: qsTr("Cut")
            enabled: innerInput.selectedText.length > 0 && !innerInput.readOnly
            onTriggered: {
                innerInput.cut()
                innerInput.forceActiveFocus()
            }
        }
        T.MenuItem {
            text: qsTr("Copy")
            enabled: innerInput.selectedText.length > 0
            onTriggered: {
                innerInput.copy()
                innerInput.forceActiveFocus()
            }
        }
        T.MenuItem {
            text: qsTr("Paste")
            enabled: innerInput.canPaste && !innerInput.readOnly
            onTriggered: {
                innerInput.paste()
                innerInput.forceActiveFocus()
            }
        }
        T.MenuSeparator {}
        T.MenuItem {
            text: qsTr("Select All")
            enabled: innerInput.text.length > 0
            onTriggered: {
                innerInput.selectAll()
                innerInput.forceActiveFocus()
            }
        }
    }
}
