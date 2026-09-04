pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

Item {
    id: widgetRoot
    required property ApplicationWindow hostWindow
    property var widget: ({})
    property int originX: 0
    property int originY: 0
    property real maximumWidth: Number.POSITIVE_INFINITY

    x: hostWindow.pxX((widget.x || 0) - originX)
    y: hostWindow.pxY((widget.y || 0) - originY - 1)
    width: Math.min(hostWindow.pxW(widget.w || 1), maximumWidth)
    height: Math.max(22, hostWindow.pxH(widget.h || 1))
    visible: widget.visible !== false

    Loader {
        anchors.fill: parent
        sourceComponent: {
            switch (widget.kind) {
            case "button": return buttonDelegate
            case "checkbox": return checkboxDelegate
            case "edit": return editDelegate
            case "text": return textDelegate
            case "progressBar": return progressDelegate
            case "radioGroup": return choiceDelegate
            case "checkGroup": return choiceDelegate
            case "listBox": return listDelegate
            case "comboBox": return comboDelegate
            case "group": return groupDelegate
            default: return textDelegate
            }
        }
    }

    Component {
        id: textDelegate
        Text {
            text: hostWindow.mnemonicText(widget.text || widget.typeName,
                                    widget.hotkey)
            textFormat: Text.StyledText
            color: widget.disabled ? hostWindow.mutedText : hostWindow.textColor
            font.pixelSize: 13
            elide: Text.ElideRight
            verticalAlignment: Text.AlignVCenter
        }
    }

    Component {
        id: editDelegate
        DialogTextField {
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
        }
    }

    Component {
        id: buttonDelegate
        DialogButton {
            hostWindow: widgetRoot.hostWindow
            text: hostWindow.cleanText(widget.text)
            mnemonicHotkey: hostWindow.cleanText(widget.hotkey)
            enabled: widget.disabled !== true
            semanticFocus: widget.focused === true
            onClicked: hostWindow.action({ "target": widget.id, "action": "control.activate" })
        }
    }

    Component {
        id: checkboxDelegate
        DialogCheckBox {
            hostWindow: widgetRoot.hostWindow
            text: hostWindow.cleanText(widget.text)
            mnemonicHotkey: hostWindow.cleanText(widget.hotkey)
            checked: widget.state === 1
            tristate: widget.threeState === true
            enabled: widget.disabled !== true
            semanticFocus: widget.focused === true
            onClicked: hostWindow.action({ "target": widget.id, "action": "control.toggle" })
        }
    }

    Component {
        id: progressDelegate
        DialogProgressBar {
            hostWindow: widgetRoot.hostWindow
            from: 0
            to: 100
            value: widget.percent || 0
        }
    }

    Component {
        id: choiceDelegate
        Column {
            anchors.fill: parent
            spacing: 0

            Repeater {
                model: widget.items || []
                delegate: DialogRadioButton {
                    required property var modelData
                    required property int index
                    hostWindow: widgetRoot.hostWindow
                    width: parent.width
                    height: widgetRoot.height
                            / Math.max(1, (widget.items || []).length)
                    text: hostWindow.cleanText(modelData)
                    checked: widget.kind === "radioGroup" ? index === widget.selected : !!(widget.states && widget.states[index])
                    semanticFocus: widget.focused === true
                                   && (index === widget.selected || widget.selected === undefined)
                    onClicked: hostWindow.action({ "target": widget.id, "action": "control.select", "index": index })
                }
            }
        }
    }

    Component {
        id: listDelegate
        ListView {
            clip: true
            model: widget.items || []
            delegate: Rectangle {
                id: listRow
                required property var modelData
                required property int index
                width: ListView.view.width
                height: Math.max(21, hostWindow.ch)
                radius: 4
                color: index === widget.cursor
                       ? hostWindow.selectedBg
                       : listMouse.containsMouse ? hostWindow.controlHoverBg
                       : "transparent"
                Behavior on color { ColorAnimation { duration: 70 } }
                Text {
                    anchors.fill: parent
                    anchors.leftMargin: 8
                    anchors.rightMargin: 8
                    text: hostWindow.mnemonicText(modelData, "")
                    textFormat: Text.StyledText
                    color: hostWindow.textColor
                    font.pixelSize: 13
                    verticalAlignment: Text.AlignVCenter
                    elide: Text.ElideRight
                }
                MouseArea {
                    id: listMouse
                    anchors.fill: parent
                    hoverEnabled: true
                    onClicked: hostWindow.action({ "target": widget.id, "action": "control.select", "index": index })
                }
            }
        }
    }

    Component {
        id: comboDelegate
        DialogComboBox {
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
        }
    }

    Component {
        id: groupDelegate
        Item {
            Rectangle {
                anchors.fill: parent
                color: "transparent"
                border.width: widget.title ? 1 : 0
                border.color: hostWindow.controlBorder
                radius: 5
            }

            Rectangle {
                anchors.left: parent.left
                anchors.top: parent.top
                anchors.leftMargin: 8
                anchors.topMargin: -9
                width: groupTitle.implicitWidth + 12
                height: 18
                radius: 3
                color: hostWindow.dialogBg
                visible: hostWindow.cleanText(widget.title) !== ""

                Text {
                    id: groupTitle
                    anchors.centerIn: parent
                    text: hostWindow.mnemonicText(widget.title, widget.hotkey)
                    textFormat: Text.StyledText
                    color: hostWindow.mutedText
                    font.pixelSize: 12
                }
            }

            Repeater {
                model: widget.children || []
                delegate: Loader {
                    required property var modelData

                    // A URL-backed loader keeps recursion dynamic. QML rejects
                    // a component that names itself directly in its compiled
                    // object tree, while the semantic dialog hierarchy is
                    // intentionally recursive and bounded by the wire model.
                    Component.onCompleted: setSource(
                        Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {
                            "hostWindow": widgetRoot.hostWindow,
                            "widget": modelData,
                            "originX": widgetRoot.originX,
                            "originY": widgetRoot.originY
                        })
                }
            }
        }
    }
}
