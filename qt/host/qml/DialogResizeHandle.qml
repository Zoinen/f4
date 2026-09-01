pragma ComponentBehavior: Bound

import QtQuick

MouseArea {
    id: resizeHandle
    required property Item targetDialog
    property int edges: 0
    property point pressPoint: Qt.point(0, 0)
    property rect startGeometry: Qt.rect(0, 0, 0, 0)

    acceptedButtons: Qt.LeftButton
    hoverEnabled: true
    preventStealing: true
    cursorShape: {
        if (edges === 1 || edges === 2)
            return Qt.SizeHorCursor
        if (edges === 4 || edges === 8)
            return Qt.SizeVerCursor
        if (edges === 5 || edges === 10)
            return Qt.SizeFDiagCursor
        return Qt.SizeBDiagCursor
    }

    onPressed: function(mouse) {
        pressPoint = mapToItem(targetDialog.parent, mouse.x, mouse.y)
        startGeometry = Qt.rect(targetDialog.x, targetDialog.y,
                                targetDialog.width, targetDialog.height)
        mouse.accepted = true
    }
    onPositionChanged: function(mouse) {
        if (!pressed)
            return
        const point = mapToItem(targetDialog.parent, mouse.x, mouse.y)
        targetDialog.resizeFrom(edges,
                                point.x - pressPoint.x,
                                point.y - pressPoint.y,
                                startGeometry)
        mouse.accepted = true
    }
    onReleased: function(mouse) {
        targetDialog.commitGeometry()
        mouse.accepted = true
    }
}
