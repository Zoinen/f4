pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Layouts

Rectangle {
    id: option
    required property ApplicationWindow hostWindow
    required property Item pixelGridRoot
    required property string namePrefix
    required property string title
    required property string description
    required property bool checked
    signal toggled(bool checked)

    objectName: namePrefix + "Panel"
    Layout.fillWidth: true
    Layout.preferredHeight: hostWindow.snapPx(42)
    Layout.minimumHeight: Layout.preferredHeight
    Layout.maximumHeight: Layout.preferredHeight
    implicitHeight: Layout.preferredHeight
    radius: hostWindow.snapPx(4)
    color: hostWindow.dialogHeaderBg
    border.width: hostWindow.separatorWidth
    border.color: hostWindow.controlBorder
    transform: Translate {
        x: option.hostWindow.dialogPixelOffsetX(option, option.pixelGridRoot)
        y: option.hostWindow.dialogPixelOffsetY(option, option.pixelGridRoot)
    }

    Item {
        id: labels
        objectName: option.namePrefix + "Labels"
        x: option.hostWindow.snapPx(8)
        y: option.hostWindow.snapPx((option.height - height) / 2)
        width: option.hostWindow.snapPx(checkBox.x - x - 8)
        height: titleText.height + option.hostWindow.snapPx(1) + descriptionText.height

        Text {
            id: titleText
            objectName: option.namePrefix + "Title"
            width: parent.width
            height: option.hostWindow.snapPx(implicitHeight)
            text: option.title
            color: option.hostWindow.textColor
            font.family: option.hostWindow.guiMonospaceFontFamily
            font.pixelSize: 11
            font.weight: Font.Bold
            elide: Text.ElideRight
        }
        Text {
            id: descriptionText
            objectName: option.namePrefix + "Description"
            y: titleText.height + option.hostWindow.snapPx(1)
            width: parent.width
            height: option.hostWindow.snapPx(implicitHeight)
            text: option.description
            color: option.hostWindow.mutedText
            font.family: option.hostWindow.guiMonospaceFontFamily
            font.pixelSize: 9
            elide: Text.ElideRight
        }
    }

    T.CheckBox {
        id: checkBox
        objectName: option.namePrefix + "CheckBox"
        x: option.hostWindow.snapPx(option.width - width - 8)
        y: option.hostWindow.snapPx((option.height - height) / 2)
        width: option.hostWindow.snapPx(100)
        height: option.hostWindow.snapPx(26)
        padding: 0
        spacing: option.hostWindow.snapPx(9)
        focusPolicy: Qt.StrongFocus
        hoverEnabled: true
        checked: option.checked
        text: "Enabled"
        onClicked: option.toggled(checked)

        indicator: Rectangle {
            objectName: option.namePrefix + "Indicator"
            width: option.hostWindow.snapPx(18)
            height: width
            y: option.hostWindow.snapPx((checkBox.height - height) / 2)
            radius: option.hostWindow.snapPx(3)
            color: checkBox.checked ? option.hostWindow.dialogAccent
                   : checkBox.down ? option.hostWindow.controlPressedBg
                   : checkBox.hovered ? option.hostWindow.controlHoverBg
                   : option.hostWindow.controlBg
            border.width: option.hostWindow.separatorWidth
            border.color: checkBox.checked || checkBox.activeFocus
                          ? option.hostWindow.dialogAccent : option.hostWindow.controlBorder

            Rectangle {
                objectName: option.namePrefix + "CheckMark"
                width: option.hostWindow.snapPx(6)
                height: width
                x: option.hostWindow.snapPx((parent.width - width) / 2)
                y: option.hostWindow.snapPx((parent.height - height) / 2)
                radius: option.hostWindow.snapPx(1)
                visible: checkBox.checked
                color: option.hostWindow.dialogBg
            }
        }
        contentItem: Text {
            objectName: option.namePrefix + "CheckBoxText"
            leftPadding: checkBox.indicator.width + checkBox.spacing
            text: checkBox.text
            color: option.hostWindow.textColor
            font.family: option.hostWindow.guiMonospaceFontFamily
            font.pixelSize: 13
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }
    }
}
