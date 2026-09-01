pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: queueController
    required property ApplicationWindow hostWindow
    required property Item menuBar
    required property ListView listView
    required property real surfaceWidth
    property Item headerClearButton: null
    property Item cancelButton: null
    readonly property alias rowsModel: queueRowsModel
    visible: false
    width: 0
    height: 0

    property var queue: ({})
    property bool interactionActive: true
    property int localSelectedTaskId: -1
    property int pendingSelectedTaskId: -1
    property int lastSemanticSelectedTaskId: -1
    property bool syncingModel: false
    readonly property real topInset: menuBar.visible ? menuBar.height : 0
    readonly property real bottomInset:
                                         Object.keys(hostWindow.keyBarModel).length > 0
                                         ? hostWindow.keyBarHeight() : 0
    readonly property real rowHeight: Math.max(60, hostWindow.ch * 2.8)
    readonly property bool compactColumns: surfaceWidth < 900
    readonly property real idColumnWidth: compactColumns ? 0 : 54
    readonly property real stateColumnWidth: compactColumns ? 92 : 116
    readonly property real typeColumnWidth: compactColumns ? 92 : 116
    readonly property real progressColumnWidth: compactColumns ? 132 : 184
    readonly property real speedColumnWidth: compactColumns ? 0 : 112
    readonly property int selectedIndex:
        indexForTaskId(localSelectedTaskId)


    function normalizedItem(item, fallbackIndex) {
        item = item || ({})
        return {
            "stableId": hostWindow.cleanText(item.id || ("queue-task-" + item.taskId)),
            "taskId": Number(item.taskId || 0),
            "itemIndex": Number(item.index !== undefined
                                  ? item.index : fallbackIndex),
            "taskType": hostWindow.cleanText(item.type),
            "description": hostWindow.cleanText(item.description),
            "state": hostWindow.cleanText(item.state),
            "stateClass": hostWindow.cleanText(item.stateClass),
            "currentFile": hostWindow.cleanText(item.currentFile),
            "displayText": hostWindow.cleanText(item.displayText),
            "currentProgress": item.currentProgress === undefined
                               ? -1 : Math.max(-1, Math.min(100,
                                            Number(item.currentProgress))),
            "progress": item.progress === undefined
                        ? -1 : Math.max(-1, Math.min(100,
                                       Number(item.progress))),
            "totalText": hostWindow.cleanText(item.totalText),
            "speed": hostWindow.cleanText(item.speed),
            "error": hostWindow.cleanText(item.error),
            "cancellable": item.cancellable === true,
            "hasDetails": item.hasDetails === true,
            "terminal": item.terminal === true,
            "active": item.active === true
        }
    }

    function modelIndexForStableId(stableId, first) {
        for (var i = Math.max(0, Number(first || 0));
                i < queueRowsModel.count; ++i) {
            if (queueRowsModel.get(i).stableId === stableId)
                return i
        }
        return -1
    }

    function indexForTaskId(taskId) {
        for (var i = 0; i < queueRowsModel.count; ++i) {
            if (Number(queueRowsModel.get(i).taskId) === Number(taskId))
                return i
        }
        return -1
    }

    function updateRole(index, role, value) {
        if (queueRowsModel.get(index)[role] !== value)
            queueRowsModel.setProperty(index, role, value)
    }

    function updateRow(index, row) {
        updateRole(index, "stableId", row.stableId)
        updateRole(index, "taskId", row.taskId)
        updateRole(index, "itemIndex", row.itemIndex)
        updateRole(index, "taskType", row.taskType)
        updateRole(index, "description", row.description)
        updateRole(index, "state", row.state)
        updateRole(index, "stateClass", row.stateClass)
        updateRole(index, "currentFile", row.currentFile)
        updateRole(index, "displayText", row.displayText)
        updateRole(index, "currentProgress", row.currentProgress)
        updateRole(index, "progress", row.progress)
        updateRole(index, "totalText", row.totalText)
        updateRole(index, "speed", row.speed)
        updateRole(index, "error", row.error)
        updateRole(index, "cancellable", row.cancellable)
        updateRole(index, "hasDetails", row.hasDetails)
        updateRole(index, "terminal", row.terminal)
        updateRole(index, "active", row.active)
    }

    function syncItems() {
        if (syncingModel)
            return
        syncingModel = true
        var incoming = queue.items || []
        var previousSelectedTaskId = localSelectedTaskId
        var wasEmpty = queueRowsModel.count === 0
        for (var index = 0; index < incoming.length; ++index) {
            var row = normalizedItem(incoming[index], index)
            var existing = modelIndexForStableId(row.stableId, index)
            if (existing < 0)
                queueRowsModel.insert(index, row)
            else {
                if (existing !== index)
                    queueRowsModel.move(existing, index, 1)
                updateRow(index, row)
            }
        }
        if (queueRowsModel.count > incoming.length) {
            // Clamp the viewport before shrinking the model.  Qt can
            // otherwise keep incubating a delegate at the old tail and
            // emit DelegateModel::cancel out-of-range during the batch.
            var nextMaximumY = Math.max(0, incoming.length * rowHeight
                                        - listView.height)
            if (listView.contentY > nextMaximumY) {
                listView.cancelFlick()
                listView.contentY = nextMaximumY
            }
            if (incoming.length === 0)
                queueRowsModel.clear()
            else
                queueRowsModel.remove(incoming.length,
                                      queueRowsModel.count - incoming.length)
        }

        var semanticTaskId = Number(queue.selectedTaskId || 0)
        if (pendingSelectedTaskId >= 0
                && indexForTaskId(pendingSelectedTaskId) >= 0) {
            if (semanticTaskId === pendingSelectedTaskId) {
                pendingSelectedTaskId = -1
                selectionAckTimer.stop()
            }
        } else {
            pendingSelectedTaskId = -1
            var semanticIndex = Math.max(0,
                                Math.min(incoming.length - 1,
                                         Number(queue.selected || 0)))
            localSelectedTaskId = semanticTaskId > 0
                    ? semanticTaskId
                    : incoming.length > 0
                      ? Number(incoming[semanticIndex].taskId || 0) : -1
        }
        if (incoming.length === 0)
            localSelectedTaskId = -1
        else if (indexForTaskId(localSelectedTaskId) < 0)
            localSelectedTaskId = Number(incoming[0].taskId || 0)
        var semanticSelectionChanged = semanticTaskId > 0
                && semanticTaskId !== lastSemanticSelectedTaskId
        lastSemanticSelectedTaskId = semanticTaskId
        syncingModel = false
        if (wasEmpty && incoming.length > 0)
            Qt.callLater(queueController.applySemanticTop)
        else if (semanticSelectionChanged
                 && previousSelectedTaskId
                    !== localSelectedTaskId)
            Qt.callLater(queueController.revealSelection)
    }

    function selectedItem() {
        return selectedIndex >= 0 && selectedIndex < queueRowsModel.count
                ? queueRowsModel.get(selectedIndex) : null
    }

    function controlOwnsActivation() {
        return headerClearButton.activeFocus || cancelButton.activeFocus
    }

    function delegateForTaskId(taskId) {
        var rowIndex = indexForTaskId(taskId)
        return rowIndex >= 0
                ? listView.itemAtIndex(rowIndex) : null
    }

    function selectIndex(index, notifyBackend) {
        if (queueRowsModel.count < 1)
            return false
        index = Math.max(0, Math.min(queueRowsModel.count - 1, index))
        var row = queueRowsModel.get(index)
        localSelectedTaskId = Number(row.taskId)
        revealSelection()
        if (notifyBackend === true) {
            pendingSelectedTaskId = localSelectedTaskId
            selectionAckTimer.restart()
            hostWindow.action({
                "target": hostWindow.cleanText(queue.id),
                "action": "queue.select",
                "taskId": Number(row.taskId)
            }, true)
        }
        return true
    }

    function revealSelection() {
        if (selectedIndex >= 0 && listView.visible)
            listView.positionViewAtIndex(selectedIndex,
                                                     ListView.Contain)
    }

    function applySemanticTop() {
        if (queueRowsModel.count < 1 || !listView.visible)
            return
        var top = Math.max(0, Math.min(queueRowsModel.count - 1,
                                       Number(queue.top || 0)))
        listView.positionViewAtIndex(top, ListView.Beginning)
    }

    function navigate(command) {
        if (queueRowsModel.count < 1)
            return false
        var index = selectedIndex >= 0 ? selectedIndex : 0
        var page = Math.max(1, Math.floor(listView.height
                                          / rowHeight) - 1)
        if (command === "up")
            index -= 1
        else if (command === "down")
            index += 1
        else if (command === "pageUp")
            index -= page
        else if (command === "pageDown")
            index += page
        else if (command === "home")
            index = 0
        else if (command === "end")
            index = queueRowsModel.count - 1
        return selectIndex(index, true)
    }

    function activateItem(row) {
        if (!row)
            return false
        hostWindow.action({
            "target": hostWindow.cleanText(queue.id),
            "action": "queue.activate",
            "taskId": Number(row.taskId)
        }, true)
        return true
    }

    function activateSelection() {
        return activateItem(selectedItem())
    }

    function cancelSelection() {
        var row = selectedItem()
        if (!row || row.cancellable !== true)
            return false
        hostWindow.action({
            "target": hostWindow.cleanText(queue.id),
            "action": "queue.cancel",
            "taskId": Number(row.taskId)
        }, true)
        return true
    }

    function clearCompleted() {
        if (queue.canClear !== true)
            return false
        hostWindow.action({
            "target": hostWindow.cleanText(queue.id),
            "action": "queue.clearCompleted"
        }, true)
        return true
    }

    function stateColor(stateClass, state) {
        var value = hostWindow.cleanText(stateClass || state).toLowerCase()
        if (value === "error")
            return "#ee6a6a"
        if (value === "done" || value === "completed"
                || value === "success")
            return "#75c991"
        if (value === "running" || value === "scanning"
                || value === "active")
            return hostWindow.dialogAccent
        if (value === "queued" || value === "starting")
            return "#d9b866"
        return hostWindow.mutedText
    }

    function stateIconName(stateClass, state) {
        var value = hostWindow.cleanText(stateClass || state).toLowerCase()
        if (value === "error")
            return "triangle-alert"
        if (value === "done" || value === "completed"
                || value === "success")
            return "circle-check"
        if (value === "running" || value === "scanning"
                || value === "active")
            return "loader-circle"
        if (value === "queued" || value === "starting")
            return "clock-3"
        if (value === "cancelled" || value === "cancelling")
            return "circle-x"
        return "clock-3"
    }

    function columnTitle(id, fallback) {
        var columns = queue.columns || []
        for (var i = 0; i < columns.length; ++i) {
            if (hostWindow.cleanText(columns[i].id) === id)
                return hostWindow.cleanText(columns[i].title || fallback)
        }
        return fallback
    }

    onQueueChanged: syncItems()
    onInteractionActiveChanged: {
        if (interactionActive)
            return
        listView.cancelFlick()
        selectionAckTimer.stop()
        pendingSelectedTaskId = -1
    }
    Component.onCompleted: syncItems()

    Timer {
        id: selectionAckTimer
        interval: 700
        onTriggered: {
            queueController.pendingSelectedTaskId = -1
            queueController.syncItems()
        }
    }

    ListModel {
        id: queueRowsModel
        objectName: "operationsQueueRowsModel"
        dynamicRoles: true
    }
}
