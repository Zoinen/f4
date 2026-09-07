pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: pool

    required property ApplicationWindow hostWindow
    property real rowHeight: 20
    property real viewportHeight: 0
    property int slotWriteCount: 0
    property bool nativeRows: false
    // Exactly one model belongs to this physical pool, even if its surface
    // mode changes. Rebinding nativeRows must not allocate another row store.
    readonly property var nativeModel: qtShell.surfaceRegistry.createDocumentRowsModel(pool)
    readonly property var rowsModel: nativeRows ? nativeModel : legacyRowsModel
    readonly property int count: rowsModel.count

    visible: false
    width: 0
    height: 0

    function rowSignature(row) {
        const source = row || ({})
        const contentKey = hostWindow.cleanText(source.contentKey)
        return contentKey !== "" ? contentKey : JSON.stringify(source)
    }

    function emptySlot() {
        return { "loaded": false, "rowData": ({}) }
    }

    function setSlot(slot, row, forceReplacement) {
        if (slot < 0 || slot >= rowsModel.count)
            return false
        const current = rowsModel.get(slot)
        if (forceReplacement !== true && current.loaded === true
                && rowSignature(current.rowData)
                   === rowSignature(row || ({})))
            return false
        rowsModel.set(slot, {
            "loaded": true,
            "rowData": row || ({})
        })
        ++slotWriteCount
        return true
    }

    function clearSlot(slot) {
        if (slot < 0 || slot >= rowsModel.count
                || rowsModel.get(slot).loaded !== true)
            return false
        rowsModel.set(slot, emptySlot())
        ++slotWriteCount
        return true
    }

    function capacityFor(rows) {
        const viewportRows = Math.max(
                    1, Math.ceil(viewportHeight / rowHeight))
        // Physical slots are bounded by viewport capacity and semantic
        // overscan. The count never depends on total document size.
        return Math.max(120, viewportRows * 12,
                        (rows ? rows.length : 0) + viewportRows * 6)
    }

    function ensureCapacity(capacity) {
        if (nativeRows) {
            rowsModel.ensureCapacity(capacity)
            return
        }
        while (rowsModel.count < capacity)
            rowsModel.append(emptySlot())
    }

    function commit() {
        if (nativeRows)
            rowsModel.commit()
    }

    function replaceWindow(start, rows, previousStart, previousEnd,
                           forceReplacement, clearOutside) {
        if (!nativeRows)
            return -1
        const writes = rowsModel.replaceWindow(start, rows,
            previousStart, previousEnd, forceReplacement === true,
            clearOutside === true)
        slotWriteCount += writes
        return writes
    }

    ListModel {
        id: legacyRowsModel
        objectName: pool.nativeRows ? "legacyDocumentRowsModel" : "documentRowsModel"
        dynamicRoles: true
    }
}
