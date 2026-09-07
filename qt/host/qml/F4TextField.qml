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
    property int cursorActivityRevision: 0

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
        objectName: control.objectName ? (control.objectName + "Background")
                                       : "textFieldBackground"
        readonly property color testBorderColor: border.color
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
                objectName: control.objectName
                            ? (control.objectName + "Placeholder")
                            : "textFieldPlaceholder"
                anchors.fill: parent
                verticalAlignment: Text.AlignVCenter
                visible: innerInput.text === "" && !innerInput.inputMethodComposing
                color: control.hostWindow ? control.hostWindow.mutedText : "#666666"
                font: innerInput.font
                elide: Text.ElideRight
                transform: Translate {
                    x: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetX(
                             placeholderLabel,
                             control.hostWindow.contentItem) : 0
                    y: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetY(
                             placeholderLabel,
                             control.hostWindow.contentItem) : 0
                }
            }

            TextInput {
                id: innerInput
                objectName: control.objectName
                            ? (control.objectName + "TextInput")
                            : "textFieldTextInput"
                anchors.fill: parent
                verticalAlignment: TextInput.AlignVCenter
                color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
                selectionColor: control.hostWindow ? control.hostWindow.selectedBg : "#2c7be5"
                selectedTextColor: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
                font: control.hostWindow ? control.hostWindow.font : Qt.font({})
                selectByMouse: !control.remoteControlled
                clip: true
                transform: Translate {
                    x: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetX(
                             innerInput, control.hostWindow.contentItem) : 0
                    y: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetY(
                             innerInput, control.hostWindow.contentItem) : 0
                }

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
                onTextEdited: {
                    ++control.cursorActivityRevision
                    control.textEdited()
                }
                onEditingFinished: control.editingFinished()
                onCursorPositionChanged: ++control.cursorActivityRevision

                cursorDelegate: Rectangle {
                    id: customCursor
                    objectName: control.objectName
                                ? (control.objectName + "Cursor")
                                : "textFieldCursor"
                    property alias blinkOn: textCursorBlinkController.blinkOn
                    property alias blinkInterval:
                        textCursorBlinkController.interval
                    readonly property bool blinkTimerRunning:
                        textCursorBlinkController.running
                    width: control.hostWindow
                           ? control.hostWindow.separatorWidth : 1
                    color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
                    opacity: blinkOn ? 1.0 : 0.0
                    function restartBlink() {
                        textCursorBlinkController.restart()
                    }
                    transform: Translate {
                        x: control.hostWindow
                           ? control.hostWindow.dialogPixelOffsetX(
                                 customCursor,
                                 control.hostWindow.contentItem) : 0
                        y: control.hostWindow
                           ? control.hostWindow.dialogPixelOffsetY(
                                 customCursor,
                                 control.hostWindow.contentItem) : 0
                    }

                    ActivityBoundedCursorBlink {
                        id: textCursorBlinkController
                        objectName: control.objectName
                                    ? (control.objectName
                                       + "CursorBlinkController")
                                    : "textFieldCursorBlinkController"
                        interval: 480
                        active: customCursor.visible
                                && control.visible
                                && innerInput.cursorVisible
                                && (innerInput.activeFocus
                                    || control.semanticFocus)
                                && (!control.hostWindow
                                    || control.hostWindow.active)
                                && !editMenu.visible
                        activityRevision:
                            control.cursorActivityRevision
                            + (control.hostWindow
                               ? control.hostWindow.keyboardActivityRevision
                               : 0)
                    }
                }
            }
        }
    }

    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.RightButton
        hoverEnabled: true
        // The field remains remote-controlled for semantic dialogs, but it is
        // still a text-edit surface.  Keep the native I-beam cursor while
        // hovering it instead of exposing the default arrow.
        cursorShape: Qt.IBeamCursor
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
