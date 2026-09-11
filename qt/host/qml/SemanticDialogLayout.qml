pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

// Shared row metrics for a dialog and each independently scrollable group.
// Content stays in semantic character coordinates until this presentation step.
Item {
    id: layout
    required property ApplicationWindow hostWindow
    property var widgets: []
    property int originX: 0
    property int originY: 0
    property real maximumWidth: 1
    property bool naturalText: false
    readonly property var rowEdges: calculateRowEdges()
    readonly property real contentHeight: {
        let bottom = 0
        for (const widget of widgets)
            bottom = Math.max(bottom, widgetBottom(widget))
        return bottom
    }
    visible: false

    function isContent(widget) {
        return widget && widget.visible !== false
            && (widget.kind !== "text" || String(widget.text || widget.typeName || "").trim().length > 0)
    }

    function availableWidth(widget) {
        if (widget.fillWidth === true)
            return Math.max(1, maximumWidth - hostWindow.pxX(Number(widget.x || 0) - originX) - hostWindow.pxW(2))
        if (naturalText && widget.kind === "text" && widget.wrapText === true)
            return Math.max(1, maximumWidth)
        return Math.max(1, Math.min(hostWindow.pxW(Number(widget.w || 1)),
            maximumWidth - hostWindow.pxX(Number(widget.x || 0) - originX)))
    }

    readonly property var paragraphs: {
        const result = []
        function collect(children) {
            for (const widget of children) {
                if (!isContent(widget) || widget.scrollable === true)
                    continue
                if (widget.kind === "text" && widget.wrapText === true)
                    result.push(widget)
                collect(widget.children || [])
            }
        }
        collect(widgets)
        return result
    }
    Repeater {
        id: measures
        model: layout.paragraphs
        delegate: Text {
            required property var modelData
            width: layout.availableWidth(modelData)
            font: layout.hostWindow.font
            text: String(modelData.text || "")
            textFormat: Text.PlainText
            wrapMode: Text.Wrap
        }
    }

    readonly property var choiceGroups: {
        const result = []
        function collect(children) {
            for (const widget of children) {
                if (!isContent(widget) || widget.scrollable === true)
                    continue
                if (widget.kind === "radioGroup" || widget.kind === "checkGroup")
                    result.push(widget)
                collect(widget.children || [])
            }
        }
        collect(widgets)
        return result
    }
    Repeater {
        id: choiceMeasures
        model: layout.choiceGroups
        delegate: SemanticChoiceGroup {
            required property var modelData
            hostWindow: layout.hostWindow
            widget: modelData
            measuring: true
            width: layout.availableWidth(widget)
        }
    }

    function visualHeight(widget) {
        if (widget.kind === "radioGroup" || widget.kind === "checkGroup") {
            const measure = choiceMeasures.itemAt(choiceGroups.findIndex(group => group.id === widget.id))
            if (measure)
                return hostWindow.snapPx(measure.implicitHeight)
        }

        if (widget.kind === "text" && widget.wrapText === true) {
            const index = paragraphs.findIndex(p => p.id === widget.id)
            const measure = measures.itemAt(index)
            if (measure)
                return hostWindow.snapPx(measure.implicitHeight)
        }
        return hostWindow.dialogWidgetVisualHeight(widget)
    }

    function calculateRowEdges() {
        const rows = []
        const controls = []
        const adaptiveChoices = []
        const base = hostWindow.snapPx(Math.max(22, hostWindow.ch))
        function collect(children) {
            for (const widget of children) {
                if (!isContent(widget))
                    continue
                const start = Math.max(0, Number(widget.y || 0) - originY)
                const flowing = naturalText && widget.kind === "text" && widget.wrapText === true
                const span = flowing ? 1 : Math.max(1, Number(widget.h || 1))
                if ((widget.kind === "radioGroup" || widget.kind === "checkGroup") && widget.title)
                    adaptiveChoices.push({start: start, span: span, widget: widget})
                while (rows.length < start + span)
                    rows.push(0)
                for (let row = start; row < start + span; ++row) {
                    const bordered = widget.kind === "group" && widget.bordered === true
                    const height = flowing ? visualHeight(widget)
                        : bordered ? hostWindow.snapPx(row === start ? 12 : 8) : base
                    rows[row] = Math.max(rows[row], height)
                }
                if (widget.kind !== "group")
                    controls.push({start: start, span: span, widget: widget})
                if (widget.scrollable !== true)
                    collect(widget.children || [])
            }
        }
        collect(widgets)
        // A compound choice field owns its entire terminal row span. Its
        // native layout may occupy one row even when the TUI needed several.
        for (const field of adaptiveChoices) {
            for (let row = field.start; row < field.start + field.span; ++row)
                rows[row] = row === field.start ? visualHeight(field.widget) + 8 : 0
        }
        // Trim outer whitespace and compact each internal blank run.
        const first = rows.findIndex(height => height > 0)
        for (let row = Math.max(0, first); row < rows.length; ++row) {
            if (rows[row] === 0 && !adaptiveChoices.some(field => row >= field.start && row < field.start + field.span)) {
                rows[row] = hostWindow.snapPx(16)
                while (row + 1 < rows.length && rows[row + 1] === 0)
                    ++row
            }
        }
        controls.sort((a, b) => a.span - b.span)
        for (const control of controls) {
            let allocated = 0
            for (let i = control.start; i < control.start + control.span; ++i)
                allocated += rows[i]
            const needed = visualHeight(control.widget)
                    + (hostWindow.dialogWidgetUsesControlHeight(control.widget) ? 8 : 0)
            if (needed > allocated)
                rows[control.start + control.span - 1] += needed - allocated
        }
        const edges = [0]
        for (const height of rows)
            edges.push(hostWindow.snapPx(edges[edges.length - 1] + height))
        return edges
    }

    function rowTop(absoluteRow) {
        const row = Number(absoluteRow) - originY
        if (row < 0)
            return 0
        return rowEdges[Math.min(row, rowEdges.length - 1)]
    }
    function widgetHeight(widget) {
        return widget.kind === "group"
            ? rowTop(Number(widget.y || 0) + Math.max(1, Number(widget.h || 1))) - rowTop(Number(widget.y || 0))
            : visualHeight(widget)
    }
    function widgetTop(widget) {
        const start = rowTop(Number(widget.y || 0))
        if (naturalText && widget.kind === "text" && widget.wrapText === true) return start
        const end = rowTop(Number(widget.y || 0) + Math.max(1, Number(widget.h || 1)))
        return hostWindow.snapPx(start + (widget.kind === "group" ? 0 : (end - start - widgetHeight(widget)) / 2))
    }
    function widgetBottom(widget) {
        if (!isContent(widget))
            return 0
        let bottom = widgetTop(widget) + widgetHeight(widget)
        if (widget.scrollable !== true) {
            for (const child of widget.children || [])
                bottom = Math.max(bottom, widgetBottom(child))
        }
        return bottom
    }
    function focusedWidget(children) {
        for (const widget of children) {
            if (!isContent(widget))
                continue
            const nested = focusedWidget(widget.children || [])
            if (nested)
                return nested
            if (widget.focused === true)
                return widget
        }
        return null
    }
}
