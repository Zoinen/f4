pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.ToolTip {
    id: control
    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property string mainTextObjectName: "f4ToolTipText"
    readonly property color surfaceColor: hostWindow ? hostWindow.tooltipBg : "#283c4e"
    readonly property color outlineColor: hostWindow ? hostWindow.tooltipBorder : "#3e5b76"

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    delay: 500
    timeout: 5000
    y: parent ? parent.height : 0
    font: control.hostWindow ? control.hostWindow.font : Qt.font({})
    topPadding: snap(6)
    bottomPadding: snap(6)
    leftPadding: snap(10)
    rightPadding: snap(10)

    contentItem: RowLayout {
        spacing: control.snap(8)
        Text {
            id: mainText
            objectName: control.mainTextObjectName
            text: {
                const parts = control.text.split("\t")
                return parts.length > 0 ? parts[0] : control.text
            }
            color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
            font: control.font
            textFormat: Text.PlainText
            wrapMode: Text.Wrap
            Layout.maximumWidth: control.snap(480)
            verticalAlignment: Text.AlignVCenter
            transform: Translate {
                x: control.hostWindow ? control.hostWindow.dialogPixelOffsetX(mainText, control.hostWindow.contentItem) : 0
                y: control.hostWindow ? control.hostWindow.dialogPixelOffsetY(mainText, control.hostWindow.contentItem) : 0
            }
        }
        Text {
            id: hotkeyText
            objectName: "f4ToolTipHotkey"
            visible: text !== ""
            text: {
                const parts = control.text.split("\t")
                return parts.length > 1 ? parts[1] : ""
            }
            color: control.hostWindow ? control.hostWindow.mutedText : "#888888"
            font.family: control.font.family
            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(10) : 10)
            textFormat: Text.PlainText
            verticalAlignment: Text.AlignVCenter
            transform: Translate {
                x: control.hostWindow ? control.hostWindow.dialogPixelOffsetX(hotkeyText, control.hostWindow.contentItem) : 0
                y: control.hostWindow ? control.hostWindow.dialogPixelOffsetY(hotkeyText, control.hostWindow.contentItem) : 0
            }
        }
    }

    background: Rectangle {
        objectName: "f4ToolTipBackground"
        radius: control.snap(6)
        color: control.surfaceColor
        border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
        border.color: control.outlineColor
    }
}
