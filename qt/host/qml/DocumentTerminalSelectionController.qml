pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: controller

    required property ApplicationWindow hostWindow
    required property ListView documentList
    required property FontMetrics fontMetrics
    required property DocumentViewportController viewportController
    property var frame: ({})
    property bool interactionActive: true
    property real rowHeight: 20
    property real textHorizontalInset: 0
    property real terminalCellWidth: 1

    property bool selectionVisible: false
    property bool selectionDragging: false
    property int anchorRow: -1
    property int anchorColumn: -1
    property int focusRow: -1
    property int focusColumn: -1
    property int clickCount: 0
    property real lastClickAt: 0
    property int lastClickRow: -1
    property int lastClickColumn: -1
    property real pointerX: 0
    property real pointerY: 0
    property real autoScrollDistance: 0
    property real autoScrollLastTick: 0
    readonly property bool autoScrollRunning: autoScrollTimer.running

    visible: false
    width: 0
    height: 0

    function pointAt(pointX, pointY) {
        const modelIndex = Math.floor(
                    (documentList.contentY + pointY) / rowHeight)
        const windowIndex = modelIndex - viewportController.loadedSlotStart
        const rows = viewportController.displayedRows
        const absoluteRow = windowIndex >= 0 && windowIndex < rows.length
                ? viewportController.rowExtent(windowIndex)
                : Number(frame.viewportStart || 0)
                  + Math.floor(pointY / rowHeight)
        const totalRows = Math.max(1, Number(frame.contentExtent || 1))
        const relativeColumn = (pointX - textHorizontalInset)
                / terminalCellWidth
        const columns = columnCount(documentList.width)
        return {
            "row": Math.max(0, Math.min(totalRows - 1,
                                          Math.floor(absoluteRow))),
            "column": Math.max(0, Math.min(columns,
                                  Math.floor(relativeColumn + 0.5))),
            "cellColumn": Math.max(0, Math.min(columns - 1,
                                      Math.floor(relativeColumn)))
        }
    }

    function point(mouse) {
        return pointAt(mouse.x, mouse.y)
    }

    function pointAtViewportEdge() {
        return pointAt(pointerX,
                       viewportController.clamp(
                           pointerY, 0,
                           Math.max(0, documentList.height - 0.001)))
    }

    function columnCount(rowWidth) {
        const semanticColumns = Math.floor(Number(frame.columns || 0))
        if (semanticColumns > 0)
            return semanticColumns
        return Math.max(1, Math.floor(
                    (Number(rowWidth || documentList.width)
                     - textHorizontalInset) / terminalCellWidth))
    }

    function rowDataAt(absoluteRow) {
        const rows = viewportController.displayedRows
        for (let index = 0; index < rows.length; ++index) {
            if (Number((rows[index] || {}).visualRow || 0)
                    === Number(absoluteRow))
                return rows[index]
        }
        return null
    }

    function characterAt(text, index) {
        const first = text.charCodeAt(index)
        if (first >= 0xD800 && first <= 0xDBFF
                && index + 1 < text.length) {
            const second = text.charCodeAt(index + 1)
            if (second >= 0xDC00 && second <= 0xDFFF)
                return text.slice(index, index + 2)
        }
        return text.charAt(index)
    }

    function characterCellWidth(character, column) {
        if (character === "\t")
            return 8 - (column % 8)
        const measured = Number(fontMetrics.advanceWidth(character))
        if (!isFinite(measured) || measured <= 0)
            return 1
        return Math.max(1, Math.round(measured / terminalCellWidth))
    }

    function selectWordAt(absoluteRow, cellColumn) {
        const rowData = rowDataAt(absoluteRow)
        const text = rowData ? hostWindow.rowText(rowData) : ""
        const cells = []
        let column = 0
        for (let index = 0; index < text.length;) {
            const character = characterAt(text, index)
            const width = characterCellWidth(character, column)
            cells.push({
                "start": column,
                "end": column + width,
                "word": character.trim().length > 0
            })
            column += width
            index += character.length
        }

        let hit = -1
        for (let cell = 0; cell < cells.length; ++cell) {
            if (cellColumn >= cells[cell].start
                    && cellColumn < cells[cell].end) {
                hit = cell
                break
            }
        }
        if (hit < 0 || !cells[hit].word) {
            selectionVisible = false
            selectionDragging = false
            return false
        }

        let first = hit
        let last = hit
        while (first > 0 && cells[first - 1].word)
            --first
        while (last + 1 < cells.length && cells[last + 1].word)
            ++last
        anchorRow = Math.floor(absoluteRow)
        anchorColumn = cells[first].start
        focusRow = anchorRow
        focusColumn = cells[last].end
        selectionVisible = true
        selectionDragging = false
        stopAutoScroll()
        return true
    }

    function selectParagraphAt(absoluteRow) {
        const rowData = rowDataAt(absoluteRow) || ({})
        const totalRows = Math.max(1, Number(frame.contentExtent || 1))
        let start = rowData.logicalRowStart !== undefined
                ? Number(rowData.logicalRowStart) : Number(absoluteRow)
        let end = rowData.logicalRowEnd !== undefined
                ? Number(rowData.logicalRowEnd) : Number(absoluteRow) + 1
        start = Math.max(0, Math.min(totalRows - 1, Math.floor(start)))
        end = Math.max(start + 1, Math.min(totalRows, Math.floor(end)))
        anchorRow = start
        anchorColumn = 0
        focusRow = end - 1
        focusColumn = columnCount(documentList.width)
        selectionVisible = true
        selectionDragging = false
        stopAutoScroll()
        return true
    }

    function registerClick(absoluteRow, cellColumn, timestamp) {
        const now = Number(timestamp || Date.now())
        const sameCell = Math.floor(absoluteRow) === lastClickRow
                && Math.floor(cellColumn) === lastClickColumn
        if (sameCell && now >= lastClickAt && now - lastClickAt <= 400)
            clickCount = Math.min(3, clickCount + 1)
        else
            clickCount = 1
        lastClickAt = now
        lastClickRow = Math.floor(absoluteRow)
        lastClickColumn = Math.floor(cellColumn)
        return clickCount
    }

    function handlePressAt(absoluteRow, boundaryColumn, cellColumn, timestamp) {
        selectionVisible = false
        stopAutoScroll()
        const count = registerClick(absoluteRow, cellColumn, timestamp)
        if (count === 1)
            beginAt(absoluteRow, boundaryColumn)
        else if (count === 2)
            selectWordAt(absoluteRow, cellColumn)
        else {
            selectParagraphAt(absoluteRow)
            clickCount = 0
        }
        return count
    }

    function autoScrollVelocity(distance) {
        const signedDistance = Number(distance || 0)
        if (Math.abs(signedDistance) < 0.001)
            return 0
        const rowsBeyond = Math.abs(signedDistance) / rowHeight
        const rowsPerSecond = Math.min(
                    240, 3 + rowsBeyond * 12 + rowsBeyond * rowsBeyond * 2)
        return (signedDistance < 0 ? -1 : 1)
                * rowsPerSecond * rowHeight
    }

    function stopAutoScroll() {
        autoScrollTimer.stop()
        autoScrollDistance = 0
        autoScrollLastTick = 0
    }

    function updatePointer(pointX, pointY) {
        pointerX = Number(pointX || 0)
        pointerY = Number(pointY || 0)
        let outside = 0
        if (pointerY < 0)
            outside = pointerY
        else if (pointerY > documentList.height)
            outside = pointerY - documentList.height
        autoScrollDistance = outside
        if (!selectionDragging || Math.abs(outside) < 0.001) {
            autoScrollTimer.stop()
            autoScrollLastTick = 0
            return
        }

        const state = viewportController.topState()
        viewportController.setTerminalFollowTailIntent(
                    outside > 0
                    && viewportController.requestReachesContentEnd(state.extent),
                    true)
        if (!autoScrollTimer.running) {
            autoScrollLastTick = Date.now()
            autoScrollTimer.start()
        }
    }

    function advanceAutoScroll(elapsedMs) {
        if (!selectionDragging || !interactionActive
                || Math.abs(autoScrollDistance) < 0.001) {
            stopAutoScroll()
            return 0
        }
        const seconds = viewportController.clamp(
                    Number(elapsedMs || 16), 1, 50) / 1000
        const previousY = documentList.contentY
        const nextY = viewportController.clamp(
                    previousY + autoScrollVelocity(autoScrollDistance)
                                * seconds,
                    viewportController.minimumLoadedY(),
                    viewportController.maximumLoadedY())
        if (Math.abs(nextY - previousY) > 0.001) {
            documentList.contentY = nextY
            viewportController.wheelTarget = nextY
            const state = viewportController.topState()
            viewportController.setTerminalFollowTailIntent(
                        autoScrollDistance > 0
                        && viewportController.requestReachesContentEnd(
                            state.extent), true)
        } else {
            viewportController.maybeRequestWindow()
        }
        const edgePoint = pointAtViewportEdge()
        extendTo(edgePoint.row, edgePoint.column)
        return nextY - previousY
    }

    function beginAt(row, column) {
        anchorRow = Math.max(0, Math.floor(row))
        anchorColumn = Math.max(0, Math.floor(column))
        focusRow = anchorRow
        focusColumn = anchorColumn
        selectionVisible = true
        selectionDragging = true
    }

    function extendTo(row, column) {
        if (!selectionDragging)
            return
        focusRow = Math.max(0, Math.floor(row))
        focusColumn = Math.max(0, Math.floor(column))
    }

    function commit() {
        if (!selectionDragging)
            return
        selectionDragging = false
        stopAutoScroll()
        if (anchorRow === focusRow && anchorColumn === focusColumn) {
            selectionVisible = false
            return
        }
        hostWindow.action({
            "target": hostWindow.cleanText(frame.id),
            "action": "terminal.copySelection",
            "startRow": anchorRow,
            "startColumn": anchorColumn,
            "endRow": focusRow,
            "endColumn": focusColumn,
            "endExclusive": true
        }, true)
    }

    function normalizedSelection() {
        let startRow = anchorRow
        let startColumn = anchorColumn
        let endRow = focusRow
        let endColumn = focusColumn
        if (startRow > endRow
                || (startRow === endRow && startColumn > endColumn)) {
            const swapRow = startRow
            const swapColumn = startColumn
            startRow = endRow
            startColumn = endColumn
            endRow = swapRow
            endColumn = swapColumn
        }
        return {
            "startRow": startRow,
            "startColumn": startColumn,
            "endRow": endRow,
            "endColumn": endColumn
        }
    }

    function rangeForRow(absoluteRow, rowWidth) {
        if (!selectionVisible || absoluteRow < 0)
            return { "valid": false, "start": 0, "end": 0 }
        const selection = normalizedSelection()
        if (absoluteRow < selection.startRow
                || absoluteRow > selection.endRow)
            return { "valid": false, "start": 0, "end": 0 }
        const maxColumns = columnCount(rowWidth)
        let start = absoluteRow === selection.startRow
                ? selection.startColumn : 0
        let end = absoluteRow === selection.endRow
                ? selection.endColumn : maxColumns
        start = Math.max(0, Math.min(maxColumns, start))
        end = Math.max(start, Math.min(maxColumns, end))
        return { "valid": end > start, "start": start, "end": end }
    }

    function resetForDocumentChange() {
        selectionDragging = false
        selectionVisible = false
        clickCount = 0
        lastClickAt = 0
        stopAutoScroll()
    }

    function cancelInteraction() {
        selectionDragging = false
        clickCount = 0
        lastClickAt = 0
        stopAutoScroll()
    }

    Timer {
        id: autoScrollTimer
        objectName: "terminalSelectionAutoScrollTimer"
        interval: 16
        repeat: true
        onTriggered: {
            const now = Date.now()
            const elapsed = controller.autoScrollLastTick > 0
                    ? now - controller.autoScrollLastTick : interval
            controller.autoScrollLastTick = now
            controller.advanceAutoScroll(elapsed)
        }
    }
}
