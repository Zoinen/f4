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

    // Keep the modal backdrop input out of the dialog rectangle. A full
    // window MouseArea can become the press grabber before deeply nested
    // native controls (notably TextInput's padded margins) get a chance to
    // handle the event. Four outside bands preserve modality without
    // intercepting any point inside the dialog surface.
    Item {
        id: backdropInput
        anchors.fill: parent
        z: 0

        component OutsideBand: MouseArea {
            acceptedButtons: Qt.AllButtons
            hoverEnabled: true
            preventStealing: true
            onPressed: mouse => { mouse.accepted = true }
            onReleased: mouse => { mouse.accepted = true }
            onPositionChanged: mouse => { mouse.accepted = true }
            onWheel: wheel => { wheel.accepted = true }
        }

        OutsideBand {
            x: 0
            y: 0
            width: parent.width
            height: Math.max(0, dialogSurface.y)
        }
        OutsideBand {
            x: 0
            y: dialogSurface.y
            width: Math.max(0, dialogSurface.x)
            height: dialogSurface.height
        }
        OutsideBand {
            x: dialogSurface.x + dialogSurface.width
            y: dialogSurface.y
            width: Math.max(0, parent.width - x)
            height: dialogSurface.height
        }
        OutsideBand {
            x: 0
            y: dialogSurface.y + dialogSurface.height
            width: parent.width
            height: Math.max(0, parent.height - y)
        }
    }

    GenericDialog {
        id: dialogSurface
        z: 1
        hostWindow: dialogOverlay.hostWindow
        menuBar: dialogOverlay.menuBar
        frame: dialogOverlay.frame
    }
}
