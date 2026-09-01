pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: dialogOverlay
    required property ApplicationWindow hostWindow
    required property Item menuBar
    property var frame: ({})

    Rectangle {
        anchors.fill: parent
        color: "#05080c"
        opacity: 0.58
    }

    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.AllButtons
        hoverEnabled: true
        preventStealing: true
        onPressed: (mouse) => { mouse.accepted = true }
        onReleased: (mouse) => { mouse.accepted = true }
        onPositionChanged: (mouse) => { mouse.accepted = true }
        onWheel: (wheel) => { wheel.accepted = true }
    }

    GenericDialog {
        id: dialogSurface
        hostWindow: dialogOverlay.hostWindow
        menuBar: dialogOverlay.menuBar
        frame: parent.frame
    }
}
