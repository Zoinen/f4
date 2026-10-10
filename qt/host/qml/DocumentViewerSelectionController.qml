pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: controller

    required property ApplicationWindow hostWindow
    required property ListView documentList
    required property DocumentViewportController viewportController
    property var frame: ({})
    property real textHorizontalInset: 0
    property real terminalCellWidth: 1
    objectName: "viewerSelectionController"
    visible: false
    width: 0
    height: 0
    property bool selectionVisible: false
    property bool selectionDragging: false
    property bool blockSelection: false
    property int anchorOffset: -1
    property int anchorColumn: 0
    property int focusOffset: -1
    property int focusColumn: 0

    function columnCount() {
        const columns = Math.floor(Number(frame.viewportColumns || 0))
        if (columns > 0)
            return columns
        return Math.max(1, Math.floor((documentList.width
            - textHorizontalInset) / terminalCellWidth))
    }

    function pointAt(pointX, pointY) {
        const index = viewportController.windowIndexAtViewportY(pointY)
        const rows = viewportController.displayedRows || []
        if (index < 0 || index >= rows.length)
            return null
        const row = rows[index] || ({})
        const rawColumn = (pointX - textHorizontalInset)
            / Math.max(1, terminalCellWidth)
        return {
            "offset": Number(row.offset || 0),
            "column": Math.max(0, Math.min(columnCount(), Math.floor(rawColumn + 0.5))),
            "row": row
        }
    }

    function point(pointX, pointY) {
        return pointAt(pointX, pointY)
    }

    function pointAtViewportEdge(pointX, pointY) {
        return pointAt(pointX, Math.max(0, Math.min(documentList.height - 0.001, pointY)))
    }

    function beginAt(point, block) {
        if (!point)
            return
        blockSelection = block === true
        anchorOffset = point.offset
        anchorColumn = point.column
        focusOffset = point.offset
        focusColumn = point.column
        selectionVisible = true
        selectionDragging = true
    }

    function extendTo(point) {
        if (!selectionDragging || !point)
            return
        focusOffset = point.offset
        focusColumn = point.column
    }

    function normalized() {
        let startOffset = anchorOffset
        let startColumn = anchorColumn
        let endOffset = focusOffset
        let endColumn = focusColumn
        if (startOffset > endOffset
                || (startOffset === endOffset && startColumn > endColumn)) {
            const offset = startOffset
            const column = startColumn
            startOffset = endOffset
            startColumn = endColumn
            endOffset = offset
            endColumn = column
        }
        return {
            "startOffset": startOffset,
            "startColumn": startColumn,
            "endOffset": endOffset,
            "endColumn": endColumn,
            "blockStartColumn": Math.min(anchorColumn, focusColumn),
            "blockEndColumn": Math.max(anchorColumn, focusColumn)
        }
    }

    function rangeForRow(rowData) {
        if (!selectionVisible || !rowData || rowData.offset === undefined)
            return {"valid": false, "start": 0, "end": 0}
        const selection = normalized()
        const offset = Number(rowData.offset)
        if (offset < selection.startOffset || offset > selection.endOffset)
            return {"valid": false, "start": 0, "end": 0}
        const columns = columnCount()
        let start = blockSelection ? selection.blockStartColumn
            : offset === selection.startOffset ? selection.startColumn : 0
        let end = blockSelection ? selection.blockEndColumn
            : offset === selection.endOffset ? selection.endColumn : columns
        start = Math.max(0, Math.min(columns, start))
        end = Math.max(start, Math.min(columns, end))
        return {"valid": end > start, "start": start, "end": end}
    }

    function commit() {
        if (!selectionDragging)
            return
        selectionDragging = false
        if (anchorOffset === focusOffset && anchorColumn === focusColumn) {
            selectionVisible = false
            return
        }
        const selectedRows = []
        const rows = viewportController.displayedRows || []
        for (let index = 0; index < rows.length; ++index) {
            const row = rows[index] || ({})
            const range = rangeForRow(row)
            if (!range.valid)
                continue
            selectedRows.push({
                "text": hostWindow.rowText(row),
                "origin": Number(row.displayColumn || 0),
                "width": columnCount(),
                "start": range.start,
                "end": range.end
            })
        }
        hostWindow.action({
            "target": hostWindow.cleanText(frame.id),
            "action": "viewer.copySelection",
            "block": blockSelection,
            "rows": selectedRows
        }, true)
    }

    function cancel() {
        selectionDragging = false
    }
}
