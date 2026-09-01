pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.CheckBox {
    id: dialogCheck
    required property ApplicationWindow hostWindow
    property bool semanticFocus: false
    property string mnemonicHotkey: ""

    focusPolicy: Qt.NoFocus
    hoverEnabled: true
    spacing: hostWindow.snapPx(9)
    leftPadding: 0
    implicitHeight: hostWindow.snapPx(25)

    indicator: Rectangle {
        x: 0
        anchors.verticalCenter: parent.verticalCenter
        width: hostWindow.snapPx(18)
        height: hostWindow.snapPx(18)
        radius: hostWindow.snapPx(3)
        color: dialogCheck.checked
               ? hostWindow.dialogAccent
               : dialogCheck.down ? hostWindow.controlPressedBg
               : dialogCheck.hovered ? hostWindow.controlHoverBg
               : hostWindow.controlBg
        border.width: 1
        border.color: dialogCheck.checked || dialogCheck.semanticFocus
                      ? hostWindow.dialogAccent : hostWindow.controlBorder

        Rectangle {
            anchors.centerIn: parent
            width: dialogCheck.tristate
                   && dialogCheck.checkState === Qt.PartiallyChecked
                   ? hostWindow.snapPx(9) : hostWindow.snapPx(6)
            height: dialogCheck.tristate
                    && dialogCheck.checkState === Qt.PartiallyChecked
                    ? hostWindow.snapPx(2) : hostWindow.snapPx(6)
            radius: hostWindow.snapPx(1)
            visible: dialogCheck.checkState !== Qt.Unchecked
            color: hostWindow.dialogBg
        }

        Behavior on color { ColorAnimation { duration: 90 } }
        Behavior on border.color { ColorAnimation { duration: 90 } }
    }

    contentItem: Text {
        leftPadding: dialogCheck.indicator.width + dialogCheck.spacing
        text: hostWindow.mnemonicText(dialogCheck.text,
                                dialogCheck.mnemonicHotkey)
        textFormat: Text.StyledText
        color: dialogCheck.enabled ? hostWindow.textColor : hostWindow.mutedText
        opacity: dialogCheck.enabled ? 1 : 0.55
        font.pixelSize: 13
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }
}
