pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.ComboBox {
    id: dialogCombo
    required property ApplicationWindow hostWindow
    required property var widget

    focusPolicy: Qt.NoFocus
    hoverEnabled: true
    padding: 0
    model: widget.items || []
    textRole: "text"
    currentIndex: Math.max(0, Number(widget.selected || 0))
    displayText: hostWindow.cleanText(widget.text)

    contentItem: Text {
        leftPadding: 10
        rightPadding: 32
        text: dialogCombo.displayText
        color: hostWindow.textColor
        font.pixelSize: 13
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }

    indicator: Text {
        x: dialogCombo.width - width - 11
        y: Math.round((dialogCombo.height - height) / 2) - 1
        text: "⌄"
        color: comboMouse.containsMouse ? hostWindow.textColor : hostWindow.mutedText
        font.pixelSize: 14
    }

    background: Rectangle {
        radius: 4
        color: comboMouse.pressed ? hostWindow.controlPressedBg
               : comboMouse.containsMouse ? hostWindow.controlHoverBg
               : hostWindow.controlBg
        border.width: 1
        border.color: dialogCombo.widget.focused
                      ? hostWindow.dialogAccent : hostWindow.controlBorder
        Behavior on color { ColorAnimation { duration: 90 } }
        Behavior on border.color { ColorAnimation { duration: 90 } }
    }

    popup: T.Popup {
        y: dialogCombo.height + 4
        width: dialogCombo.width
        implicitHeight: Math.min(contentItem.implicitHeight + 8, 240)
        padding: 4

        background: Rectangle {
            radius: 5
            color: hostWindow.dialogHeaderBg
            border.width: 1
            border.color: hostWindow.controlBorder
        }

        contentItem: ListView {
            clip: true
            implicitHeight: contentHeight
            model: dialogCombo.popup.visible
                   ? dialogCombo.delegateModel : null
            currentIndex: dialogCombo.highlightedIndex
            boundsBehavior: Flickable.StopAtBounds
        }
    }

    delegate: T.ItemDelegate {
        id: dialogComboItem
        required property int index
        required property var model
        width: dialogCombo.width - 8
        height: 30
        highlighted: dialogCombo.highlightedIndex === index

        contentItem: Text {
            leftPadding: 8
            text: hostWindow.mnemonicText(dialogComboItem.model.text,
                                    dialogComboItem.model.hotkey)
            textFormat: Text.StyledText
            color: hostWindow.textColor
            font.pixelSize: 13
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }

        background: Rectangle {
            radius: 3
            color: dialogComboItem.highlighted
                   ? hostWindow.selectedBg
                   : dialogComboItem.hovered ? hostWindow.controlHoverBg
                   : "transparent"
        }
    }

    MouseArea {
        id: comboMouse
        anchors.fill: parent
        z: 2
        acceptedButtons: Qt.LeftButton
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: {
            if (dialogCombo.popup.visible)
                dialogCombo.popup.close()
            else
                dialogCombo.popup.open()
        }
    }

    onActivated: (index) => hostWindow.action({
        "target": widget.id,
        "action": "control.select",
        "index": index
    })
}
