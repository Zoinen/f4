pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Layouts

Rectangle {
    id: toastRoot
    required property ApplicationWindow hostWindow
    required property var toast
    signal dismissRequested()
    readonly property string message: hostWindow.cleanText(toast.message)
    width: Math.min(hostWindow.width - 32,
                    toastText.implicitWidth + toastCloseButton.implicitWidth + 44)
    height: Math.max(32, toastText.implicitHeight + 14)
    radius: 6
    color: hostWindow.dialogBg
    border.width: 1
    border.color: hostWindow.controlBorder
    visible: toast.message !== undefined && message !== ""

    RowLayout {
        anchors.fill: parent
        anchors.leftMargin: 12
        anchors.rightMargin: 6
        spacing: 8

        Text {
            id: toastText
            Layout.fillWidth: true
            Layout.maximumWidth: Math.max(0, hostWindow.width - 92)
            text: toastRoot.message
            color: hostWindow.textColor
            font.pixelSize: 13
            elide: Text.ElideRight
            verticalAlignment: Text.AlignVCenter
        }

        T.Button {
            id: toastCloseButton
            objectName: "toastCloseButton"
            implicitWidth: 24
            implicitHeight: 24
            padding: 0
            hoverEnabled: true
            Accessible.name: "Close notification"
            Accessible.role: Accessible.Button

            background: Rectangle {
                radius: 4
                color: toastCloseButton.down
                       ? hostWindow.controlPressedBg
                       : (toastCloseButton.hovered
                          ? hostWindow.controlHoverBg : "transparent")
                border.width: toastCloseButton.activeFocus ? 1 : 0
                border.color: hostWindow.dialogAccent
            }

            contentItem: Text {
                text: "\u00d7"
                color: hostWindow.textColor
                font.pixelSize: 17
                horizontalAlignment: Text.AlignHCenter
                verticalAlignment: Text.AlignVCenter
            }

            onClicked: toastRoot.dismissRequested()
        }
    }
}
