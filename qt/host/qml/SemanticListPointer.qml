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
    property bool semanticWheel: false
    property real checkboxLeft: 0
    property real checkboxWidth: 0

    function checkboxAt(position) {
        if (position.x < checkboxLeft || position.x >= checkboxLeft + checkboxWidth)
            return -1
        const contentPosition = mapToItem(view.contentItem, position.x, position.y)
        const index = view.indexAt(contentPosition.x, contentPosition.y)
        const state = (widget.itemStates || [])[index]
        return state && state.checkable === true ? index : -1
    }
    function tapAt(position) {
        const index = checkboxAt(position)
        if (index >= 0) dispatch("control.toggle", index)
    }

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
    property real wheelRowRemainder: 0

    MouseArea {
        enabled: root.semanticWheel
        anchors.fill: parent
        acceptedButtons: Qt.NoButton
        onWheel: wheel => {
            const rowHeight = root.view.currentItem
                            ? root.view.currentItem.height : Math.max(21, root.hostWindow.ch)
            // Category navigation selects one page per notch; ordinary table
            // scrolling continues to honor the system's line count.
            const lines = root.keyboardNavigation ? 1
                        : Math.max(1, Number(Qt.styleHints.wheelScrollLines || 3))
            root.wheelRowRemainder += wheel.pixelDelta.y !== 0
                    ? -wheel.pixelDelta.y / rowHeight
                    : -wheel.angleDelta.y / 120 * lines
            const rows = Math.trunc(root.wheelRowRemainder)
            if (rows !== 0) {
                root.wheelRowRemainder -= rows
                if (root.keyboardNavigation)
                    root.dispatch("control.select", Math.max(0, Math.min(
                                      root.view.count - 1, root.view.currentIndex + rows)))
                else
                    root.hostWindow.action({target: root.widget.id,
                                            action: "control.scroll", delta: rows}, true)
            }
            wheel.accepted = true
        }
    }

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
                if (root.checkboxAt(point.position) < 0)
                    root.selectAt(point.position.y)
            }
        }
        onTapped: root.tapAt(point.position)
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
            if (root.checkboxAt(point.position) >= 0) root.tapAt(point.position)
            else root.selectAt(point.position.y)
        }
    }
}
