pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.Slider {
    id: control

    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property Component trackComponent: null
    property real trackHeight: 6
    property real handleSize: 14

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    hoverEnabled: true
    implicitHeight: snap(Math.max(handleSize, trackHeight) + 4)
    implicitWidth: snap(160)

    handle: Rectangle {
        x: control.snap(control.leftPadding + control.visualPosition * (control.availableWidth - width))
        y: control.snap(control.topPadding + (control.availableHeight - height) / 2)
        width: control.snap(control.handleSize)
        height: control.snap(control.handleSize)
        radius: width / 2
        color: {
            if (control.pressed)
                return control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
            if (control.hovered)
                return control.hostWindow ? Qt.lighter(control.hostWindow.dialogAccent, 1.2) : "#ffffff"
            return "#ffffff"
        }
        border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
        border.color: control.hostWindow ? control.hostWindow.controlBorder : "#334455"

        Behavior on color { ColorAnimation { duration: 90 } }
    }

    background: Item {
        x: control.snap(control.leftPadding + control.handle.width / 2)
        y: control.snap(control.topPadding + (control.availableHeight - control.trackHeight) / 2)
        width: control.snap(control.availableWidth - control.handle.width)
        height: control.snap(control.trackHeight)

        Loader {
            anchors.fill: parent
            sourceComponent: control.trackComponent ? control.trackComponent : defaultTrack
        }

        Component {
            id: defaultTrack
            Rectangle {
                anchors.fill: parent
                radius: control.snap(height / 2)
                color: control.hostWindow ? control.hostWindow.controlBg : "#18202a"
                border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
                border.color: control.hostWindow ? control.hostWindow.controlBorder : "#25303d"

                Rectangle {
                    width: control.snap(control.visualPosition * parent.width)
                    height: parent.height
                    radius: control.snap(height / 2)
                    color: control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
                }
            }
        }
    }
}
