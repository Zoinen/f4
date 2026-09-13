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
    // A frontend may compose local rows while retaining the shared list renderer.
    property var rowAction: (action, index) => hostWindow.action({target: widget.id, action: action, index: index})
    property bool keyboardNavigation: false

    function dispatch(action, index) {
        rowAction(action, index)
        if (keyboardNavigation) forceActiveFocus()
    }
    Keys.onPressed: event => {
        if (!keyboardNavigation) return
        let index = view.currentIndex
        switch (event.key) {
        case Qt.Key_Up: index = Math.max(0, index - 1); break
        case Qt.Key_Down: index = Math.min(view.count - 1, index + 1); break
        case Qt.Key_Home: index = 0; break
        case Qt.Key_End: index = view.count - 1; break
        case Qt.Key_Return:
        case Qt.Key_Enter: dispatch("control.activate", index); event.accepted = true; return
        default: return
        }
        if (index >= 0) dispatch("control.select", index)
        event.accepted = true
    }
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
        dispatch("control.select", index)
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
            root.dispatch("control.activate", root.lastPointerIndex)
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
