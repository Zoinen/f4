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
    readonly property real semanticTop: hostWindow.pxY(
        (widget.y || 0) - originY - 1)
    readonly property real semanticHeight:
        hostWindow.dialogWidgetSemanticHeight(widget)
    readonly property real visualHeight:
        hostWindow.dialogWidgetVisualHeight(widget)
    readonly property bool visuallyOverflows:
        visualHeight > semanticHeight + 0.001

    objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "Root"

    x: hostWindow.pxX((widget.x || 0) - originX)
    y: hostWindow.dialogWidgetVisualTop(
           (widget.y || 0) - originY - 1, widget)
    width: Math.min(hostWindow.pxW(widget.w || 1), maximumWidth)
    height: visualHeight
    visible: widget.visible !== false
    clip: false

    // A one-row semantic rectangle remains the stable layout anchor. Only
    // its native control paints and receives input outside that rectangle.
    // Correct the whole overflowing subtree in scene space so backgrounds,
    // hit regions, text, and icons share the same physical-pixel origin.
    transform: Translate {
        x: widgetRoot.visuallyOverflows
           ? hostWindow.dialogPixelOffsetX(
                 widgetRoot, hostWindow.contentItem) : 0
        y: widgetRoot.visuallyOverflows
           ? hostWindow.dialogPixelOffsetY(
                 widgetRoot, hostWindow.contentItem) : 0
    }

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
            id: dialogText
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "Text"
            text: hostWindow.mnemonicText(widget.text || widget.typeName,
                                    widget.hotkey)
            textFormat: Text.StyledText
            color: widget.disabled ? hostWindow.mutedText : hostWindow.textColor
            font: hostWindow.font
            elide: Text.ElideRight
            verticalAlignment: Text.AlignVCenter
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                       dialogText, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                       dialogText, hostWindow.contentItem)
            }
        }
    }

    Component {
        id: editDelegate
        DialogTextField {
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "Edit"
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
        }
    }

    Component {
        id: buttonDelegate
        DialogButton {
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "Button"
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
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "CheckBox"
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
                    objectName: "dialogWidget-"
                                + hostWindow.cleanText(widget.id)
                                + "Radio-" + index
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
        Item {
            id: listControl
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "ListBox"
            property bool semanticFocus: widget.focused === true

            ListView {
                anchors.fill: parent
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
                           : listMouse.containsMouse
                             ? hostWindow.controlHoverBg : "transparent"
                    Behavior on color { ColorAnimation { duration: 70 } }
                    Text {
                        id: listRowText
                        objectName: "dialogWidget-"
                                    + hostWindow.cleanText(widget.id)
                                    + "ListItemText-" + listRow.index
                        anchors.fill: parent
                        anchors.leftMargin: 8
                        anchors.rightMargin: 8
                        text: hostWindow.mnemonicText(modelData, "")
                        textFormat: Text.StyledText
                        color: hostWindow.textColor
                        font: hostWindow.font
                        verticalAlignment: Text.AlignVCenter
                        elide: Text.ElideRight
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                   listRowText, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                   listRowText, hostWindow.contentItem)
                        }
                    }
                    MouseArea {
                        id: listMouse
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: hostWindow.action({
                            "target": widget.id,
                            "action": "control.select",
                            "index": index
                        })
                    }
                }
            }

            Rectangle {
                id: listFocusFrame
                objectName: "dialogWidget-"
                            + hostWindow.cleanText(widget.id)
                            + "ListFocusFrame"
                readonly property color testBorderColor: border.color
                readonly property real testBorderWidth: border.width
                x: 0
                y: 0
                width: hostWindow.snapPx(listControl.width)
                height: hostWindow.snapPx(listControl.height)
                z: 2
                color: "transparent"
                radius: hostWindow.snapPx(4)
                border.width: listControl.semanticFocus
                              ? hostWindow.separatorWidth : 0
                border.color: listControl.semanticFocus
                              ? hostWindow.dialogAccent
                              : hostWindow.controlBorder
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                           listFocusFrame, hostWindow.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                           listFocusFrame, hostWindow.contentItem)
                }

                Behavior on border.color {
                    ColorAnimation { duration: 90 }
                }
            }
        }
    }

    Component {
        id: comboDelegate
        DialogComboBox {
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "ComboBox"
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
        }
    }

    Component {
        id: groupDelegate
        Item {
            Rectangle {
                objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                            + "GroupBorder"
                readonly property color testBorderColor: border.color
                readonly property real testBorderWidth: border.width
                anchors.fill: parent
                color: "transparent"
                border.width: widget.bordered === true ? 1 : 0
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
                    objectName: "dialogWidget-"
                                + hostWindow.cleanText(widget.id)
                                + "GroupTitle"
                    anchors.centerIn: parent
                    text: hostWindow.mnemonicText(widget.title, widget.hotkey)
                    textFormat: Text.StyledText
                    color: hostWindow.mutedText
                    font: hostWindow.font
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               groupTitle, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               groupTitle, hostWindow.contentItem)
                    }
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
                            // Nested semantic coordinates remain absolute in
                            // the frame. Rebase them to this group; the -1
                            // cancels the delegate's own legacy row offset.
                            "originX": Number(widgetRoot.widget.x || 0),
                            "originY": Number(widgetRoot.widget.y || 0) - 1
                        })
                }
            }
        }
    }
}
