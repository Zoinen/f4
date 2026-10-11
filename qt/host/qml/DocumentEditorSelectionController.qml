pragma ComponentBehavior: Bound

import QtQml

QtObject {
    required property var frame
    required property var presentationFrame
    required property var selectionCursorFrame

    function rangeForRow(visualRow, visualWidth, caretState) {
        const caret = caretState || selectionCursorFrame
        let empty = ({ "valid": false, "start": 0, "end": 0 })
        const rectangular = caret.rectSelection === true
        if (frame.kind !== "editor" || presentationFrame.hexMode === true
                || presentationFrame.decodeMode === true
                || (caret.selection !== true && !rectangular) || visualRow < 0)
            return empty
        let anchorRow = Number(rectangular ? caret.rectAnchorRow : caret.selectionAnchorRow) || 0
        let anchorColumn = Number(rectangular ? caret.rectAnchorColumn : caret.selectionAnchorColumn) || 0
        let focusRow = Number(rectangular ? caret.rectFocusRow : caret.cursorAbsoluteRow) || 0
        let focusColumn = Number(rectangular ? caret.rectFocusColumn : caret.cursorAbsoluteColumn) || 0
        if (rectangular) {
            if (anchorRow > focusRow) {
                let swapRow = anchorRow
                anchorRow = focusRow
                focusRow = swapRow
            }
        } else if (anchorRow > focusRow
                || (anchorRow === focusRow && anchorColumn > focusColumn)) {
            let swapRow = anchorRow
            let swapColumn = anchorColumn
            anchorRow = focusRow
            anchorColumn = focusColumn
            focusRow = swapRow
            focusColumn = swapColumn
        }
        if (visualRow < anchorRow || visualRow > focusRow)
            return empty
        let start = rectangular ? Math.min(anchorColumn, focusColumn)
            : visualRow === anchorRow ? anchorColumn : 0
        let end = rectangular ? Math.max(anchorColumn, focusColumn)
            : visualRow === focusRow ? focusColumn : Math.max(0, visualWidth)
        const scrollLeft = Math.max(0, Number(presentationFrame.scrollLeft || 0))
        const rowEnd = Math.max(0, Number(visualWidth || 0) - scrollLeft)
        const viewportEnd = Math.max(0, Number(presentationFrame.viewportColumns || 0))
        start = Math.max(0, start - scrollLeft)
        end = Math.max(0, end - scrollLeft)
        if (viewportEnd > 0) {
            start = Math.min(start, viewportEnd)
            end = Math.min(end, viewportEnd)
        }
        if (!rectangular) {
            start = Math.min(start, rowEnd)
            end = Math.min(end, rowEnd)
        }
        return ({ "valid": end > start, "start": start, "end": end })
    }
}
