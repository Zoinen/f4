import QtQuick
import QtQuick.Controls
import F4QtHost 1.0

ApplicationWindow {
    id: root

    width: Math.max(640, Math.ceil(qtShell.initialCols * grid.cellWidth))
    height: Math.max(420, Math.ceil(qtShell.initialRows * grid.cellHeight))
    visible: true
    title: "f4"
    color: "black"

    onClosing: qtShell.sendQuit()

    VtuiGridItem {
        id: grid
        anchors.fill: parent
        controller: qtShell
        focus: true

        Component.onCompleted: forceActiveFocus()
    }
}
