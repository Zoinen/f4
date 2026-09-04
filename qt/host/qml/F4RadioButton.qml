pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.RadioButton {
    id: control

    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property bool semanticFocus: false
    property string mnemonicHotkey: ""

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    focusPolicy: Qt.NoFocus
    hoverEnabled: true
    spacing: snap(9)
    leftPadding: 0
    implicitHeight: snap(25)

    indicator: Rectangle {
        x: 0
        anchors.verticalCenter: parent.verticalCenter
        width: control.snap(18)
        height: control.snap(18)
        radius: width / 2
        color: {
            if (control.down)
                return control.hostWindow ? control.hostWindow.controlPressedBg : "#334455"
            if (control.hovered)
                return control.hostWindow ? control.hostWindow.controlHoverBg : "#223344"
            return control.hostWindow ? control.hostWindow.controlBg : "#18202a"
        }
        border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
        border.color: {
            if (control.checked || control.semanticFocus)
                return control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
            return control.hostWindow ? control.hostWindow.controlBorder : "#25303d"
        }

        Rectangle {
            anchors.centerIn: parent
            width: control.snap(8)
            height: control.snap(8)
            radius: width / 2
            visible: control.checked
            color: control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
        }

        Behavior on color { ColorAnimation { duration: 90 } }
        Behavior on border.color { ColorAnimation { duration: 90 } }
    }

    contentItem: Text {
        leftPadding: control.indicator.width + control.spacing
        text: control.hostWindow
              ? control.hostWindow.mnemonicText(control.text, control.mnemonicHotkey)
              : control.text
        textFormat: Text.StyledText
        color: {
            if (!control.enabled)
                return control.hostWindow ? control.hostWindow.mutedText : "#666666"
            return control.hostWindow ? control.hostWindow.textColor : "#ffffff"
        }
        opacity: control.enabled ? 1.0 : 0.55
        font.pixelSize: 13
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }
}
