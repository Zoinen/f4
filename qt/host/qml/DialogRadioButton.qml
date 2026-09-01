pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.RadioButton {
    id: dialogRadio
    required property ApplicationWindow hostWindow
    property bool semanticFocus: false
    property string mnemonicHotkey: ""

    focusPolicy: Qt.NoFocus
    hoverEnabled: true
    spacing: 9
    leftPadding: 0

    indicator: Rectangle {
        x: 0
        anchors.verticalCenter: parent.verticalCenter
        width: 18
        height: 18
        radius: 9
        color: dialogRadio.down ? hostWindow.controlPressedBg
               : dialogRadio.hovered ? hostWindow.controlHoverBg
               : hostWindow.controlBg
        border.width: 1
        border.color: dialogRadio.checked || dialogRadio.semanticFocus
                      ? hostWindow.dialogAccent : hostWindow.controlBorder

        Rectangle {
            anchors.centerIn: parent
            width: 8
            height: 8
            radius: 4
            visible: dialogRadio.checked
            color: hostWindow.dialogAccent
        }

        Behavior on color { ColorAnimation { duration: 90 } }
        Behavior on border.color { ColorAnimation { duration: 90 } }
    }

    contentItem: Text {
        leftPadding: dialogRadio.indicator.width + dialogRadio.spacing
        text: hostWindow.mnemonicText(dialogRadio.text,
                                dialogRadio.mnemonicHotkey)
        textFormat: Text.StyledText
        color: dialogRadio.enabled ? hostWindow.textColor : hostWindow.mutedText
        font.pixelSize: 13
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }
}
