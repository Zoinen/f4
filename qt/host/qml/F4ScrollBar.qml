pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.ScrollBar {
    id: control

    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property real thickness: 16
    property real margin: Math.max(1, Math.round(thickness * 4 / 16))
    property real radius: Math.max(2, Math.round(thickness * 4 / 16))

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    readonly property color handleColor:
        (control.hostWindow && control.hostWindow.galleryScrollBarHandleColor !== undefined)
        ? control.hostWindow.galleryScrollBarHandleColor : "#434b57"
    readonly property color handleBackgroundHoveredColor:
        (control.hostWindow && control.hostWindow.galleryScrollBarBackgroundHoverColor !== undefined)
        ? control.hostWindow.galleryScrollBarBackgroundHoverColor : "#5f6875"
    readonly property color handleHoveredColor:
        (control.hostWindow && control.hostWindow.galleryScrollBarHoverColor !== undefined)
        ? control.hostWindow.galleryScrollBarHoverColor : "#7f8896"
    readonly property color handlePressedColor:
        (control.hostWindow && control.hostWindow.galleryScrollBarPressedColor !== undefined)
        ? control.hostWindow.galleryScrollBarPressedColor : "#47515d"
    readonly property color trackHoveredColor:
        (control.hostWindow && control.hostWindow.galleryScrollBarTrackHoverColor !== undefined)
        ? control.hostWindow.galleryScrollBarTrackHoverColor : "#0fffffff"

    hoverEnabled: true
    implicitWidth: orientation === Qt.Vertical ? snap(thickness) : snap(100)
    implicitHeight: orientation === Qt.Vertical ? snap(100) : snap(thickness)

    leftPadding: 0
    rightPadding: 0
    topPadding: 0
    bottomPadding: 0

    contentItem: MouseArea {
        id: handleMouseArea
        implicitWidth: control.orientation === Qt.Vertical ? control.snap(control.thickness) : control.snap(30)
        implicitHeight: control.orientation === Qt.Vertical ? control.snap(30) : control.snap(control.thickness)
        hoverEnabled: true
        acceptedButtons: Qt.NoButton

        Rectangle {
            anchors.fill: parent
            anchors.margins: control.snap(control.margin)
            radius: control.snap(control.radius)
            color: control.pressed ? control.handlePressedColor
                   : control.hovered
                     ? (handleMouseArea.containsMouse
                        ? control.handleHoveredColor
                        : control.handleBackgroundHoveredColor)
                     : control.handleColor
        }
    }

    background: Rectangle {
        implicitWidth: control.orientation === Qt.Vertical ? control.snap(control.thickness) : control.snap(100)
        implicitHeight: control.orientation === Qt.Vertical ? control.snap(100) : control.snap(control.thickness)
        color: (control.hovered || control.pressed) ? control.trackHoveredColor : "transparent"
        radius: control.snap(control.thickness)
    }
}
