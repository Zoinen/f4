pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

// The viewport owns the grab, not a row delegate that a semantic update can
// replace. Mouse drag selects rows; touch remains available to the Flickable.
Item {
    id: root
    required property ApplicationWindow hostWindow
    required property ListView view
    required property var widget
    property bool activateOnDoubleClick: false
    enabled: widget.disabled !== true && widget.readOnly !== true
    property int lastPointerIndex: -1

    function selectAt(y) {
        if (y < 0 || y >= height)
            return
        // A captured TUI list follows Y even when the pointer leaves sideways.
        const position = mapToItem(view.contentItem, width / 2, y)
        const index = view.indexAt(position.x, position.y)
        if (index < 0 || index === lastPointerIndex)
            return
        lastPointerIndex = index
        hostWindow.action({target: widget.id, action: "control.select", index: index})
    }

    TapHandler {
        id: mouseSelection
        objectName: root.objectName + "Mouse"
        acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
        acceptedButtons: Qt.LeftButton
        onPressedChanged: {
            if (pressed) {
                root.lastPointerIndex = -1
                root.selectAt(point.position.y)
            }
        }
        onDoubleTapped: if (root.activateOnDoubleClick && root.lastPointerIndex >= 0)
            root.hostWindow.action({target: root.widget.id, action: "control.activate", index: root.lastPointerIndex})
    }
    DragHandler {
        objectName: root.objectName + "Drag"
        acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
        acceptedButtons: Qt.LeftButton
        target: null
        dragThreshold: 0
        grabPermissions: PointerHandler.CanTakeOverFromAnything
        // DragHandler also keeps its parent item's mouse grab. TapHandler's
        // exclusive grab alone does not prevent an ancestor Flickable stealing.
        onActiveChanged: if (active) root.selectAt(centroid.position.y)
        onCentroidChanged: if (active) root.selectAt(centroid.position.y)
    }
    TapHandler {
        acceptedDevices: PointerDevice.TouchScreen
        onTapped: {
            root.lastPointerIndex = -1
            root.selectAt(point.position.y)
        }
    }
}
