pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.Button {
    id: dialogButton
    required property ApplicationWindow hostWindow
    property bool semanticFocus: false
    property string mnemonicHotkey: ""

    focusPolicy: Qt.NoFocus
    hoverEnabled: true
    padding: 0
    implicitHeight: 34
    implicitWidth: 84

    contentItem: Text {
        text: hostWindow.mnemonicText(dialogButton.text,
                                dialogButton.mnemonicHotkey)
        textFormat: Text.StyledText
        color: dialogButton.enabled
               ? (dialogButton.semanticFocus ? "#f4f8fc" : hostWindow.textColor)
               : hostWindow.mutedText
        opacity: dialogButton.enabled ? 1 : 0.55
        font.pixelSize: 13
        font.weight: Font.Medium
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }

    background: Rectangle {
        radius: 4
        color: !dialogButton.enabled ? hostWindow.dialogBg
               : dialogButton.down ? hostWindow.controlPressedBg
               : dialogButton.hovered ? hostWindow.controlHoverBg
               : dialogButton.semanticFocus ? "#274b68"
               : hostWindow.controlBg
        border.width: 1
        border.color: dialogButton.semanticFocus
                      ? hostWindow.dialogAccent : hostWindow.controlBorder

        Behavior on color { ColorAnimation { duration: 90 } }
        Behavior on border.color { ColorAnimation { duration: 90 } }
    }
}
