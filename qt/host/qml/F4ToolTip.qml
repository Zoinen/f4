pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.ToolTip {
    id: control
    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    delay: 500
    timeout: 5000
    topPadding: snap(5)
    bottomPadding: snap(5)
    leftPadding: snap(8)
    rightPadding: snap(8)

    contentItem: RowLayout {
        spacing: control.snap(8)
        Text {
            id: mainText
            text: {
                const parts = control.text.split("\t")
                return parts.length > 0 ? parts[0] : control.text
            }
            color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
            font.pixelSize: 11
            verticalAlignment: Text.AlignVCenter
        }
        Text {
            id: hotkeyText
            visible: text !== ""
            text: {
                const parts = control.text.split("\t")
                return parts.length > 1 ? parts[1] : ""
            }
            color: control.hostWindow ? control.hostWindow.mutedText : "#888888"
            font.pixelSize: 10
            verticalAlignment: Text.AlignVCenter
        }
    }

    background: Rectangle {
        radius: control.snap(4)
        color: control.hostWindow ? control.hostWindow.controlBg : "#222c38"
        border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
        border.color: control.hostWindow ? control.hostWindow.controlBorder : "#334455"
    }
}
