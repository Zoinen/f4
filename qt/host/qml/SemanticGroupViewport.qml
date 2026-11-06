pragma ComponentBehavior: Bound

import QtQuick
import F4QtHost 1.0
import QtQuick.Controls

Item {
    id: root
    required property ApplicationWindow hostWindow
    required property var widget
    objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "ViewportController"
    readonly property bool description: widget.layoutRole === "description"
    readonly property real padding: description ? 0 : hostWindow.snapPx(10)
    readonly property real scrollbarContentGap: hostWindow.snapPx(Number(widget.scrollbarContentGap || 0))
    readonly property real contentAreaWidth: Math.max(1, viewport.width
        - (scrollBar.visible ? scrollBar.width + scrollbarContentGap
                            : hostWindow.snapPx(Number(widget.unscrolledRightInset || 0))))
    readonly property real maximumScroll: Math.max(0, viewport.contentHeight - viewport.height)
    readonly property int semanticMaximum: Math.max(0, Number(widget.contentHeight || 0) - Number(widget.h || 1))
    property bool synchronizing: false
    readonly property int semanticScroll: Number(widget.scrollTop || 0)
    readonly property var focused: widget.focused === true ? metrics.focusedWidget(widget.children || []) : null
    readonly property string focusedID: focused ? String(focused.id) : ""

    SemanticDialogLayout {
        id: metrics
        hostWindow: root.hostWindow
        widgets: root.widget.children || []
        naturalText: root.description
        originX: Number(root.widget.x || 0)
        originY: Number(root.widget.y || 0)
        maximumWidth: root.contentAreaWidth
    }

    function synchronizeScroll() {
        synchronizing = true
        viewport.contentY = hostWindow.snapPx(semanticMaximum > 0
                            ? maximumScroll * semanticScroll / semanticMaximum : 0)
        synchronizing = false
    }
    function commitScroll() {
        if (synchronizing || maximumScroll <= 0 || semanticMaximum <= 0)
            return
        const value = Math.round(viewport.contentY / maximumScroll * semanticMaximum)
        if (value !== semanticScroll)
            hostWindow.action({"target": widget.id, "action": "control.scroll", "value": value}, true)
    }
    function clampScroll() {
        // Layout can settle after native scrolling starts, before its debounced
        // position reaches Go. New geometry must not restore that stale offset.
        const bounded = Math.max(0, Math.min(maximumScroll, viewport.contentY))
        if (Math.abs(bounded - viewport.contentY) < .001)
            return
        synchronizing = true
        viewport.contentY = hostWindow.snapPx(bounded)
        synchronizing = false
    }
    function ensureFocusVisible() {
        if (!focused)
            return
        const top = padding + metrics.widgetTop(focused)
        const bottom = top + metrics.widgetHeight(focused)
        if (top < viewport.contentY)
            viewport.contentY = hostWindow.snapPx(Math.max(0, top - padding))
        else if (bottom > viewport.contentY + viewport.height)
            viewport.contentY = hostWindow.snapPx(Math.min(maximumScroll, bottom - viewport.height + padding))
    }
    onSemanticScrollChanged: Qt.callLater(synchronizeScroll)
    onMaximumScrollChanged: Qt.callLater(clampScroll)
    onFocusedIDChanged: Qt.callLater(ensureFocusVisible)
    Component.onCompleted: Qt.callLater(synchronizeScroll)

    Flickable {
        id: viewport
        objectName: "dialogWidget-" + root.hostWindow.cleanText(root.widget.id) + "Viewport"
        anchors.fill: parent
        clip: true
        contentWidth: width
        contentHeight: Math.max(height, root.hostWindow.snapPx(metrics.contentHeight + 2 * root.padding))
        boundsBehavior: Flickable.StopAtBounds
        pixelAligned: true
        flickableDirection: Flickable.VerticalFlick
        onMovementEnded: root.commitScroll()
        onContentYChanged: {
            if (!root.synchronizing && (moving || scrollBar.pressed))
                scrollCommit.restart()
        }
        Timer {
            id: scrollCommit
            interval: 100
            onTriggered: root.commitScroll()
        }
        ScrollBar.vertical: F4ScrollBar {
            id: scrollBar
            objectName: "dialogWidget-" + root.hostWindow.cleanText(root.widget.id) + "ViewportScrollBar"
            policy: ScrollBar.AlwaysOn
            hostWindow: root.hostWindow
            visible: root.maximumScroll > 0
            onPressedChanged: if (!pressed) root.commitScroll()
        }
        Item {
            id: content
            y: root.padding
            width: root.contentAreaWidth
            height: metrics.contentHeight
            // Snap the scroll translation as a whole before native text, icons
            // and control backgrounds inherit it, including wheel rest states.
            readonly property point dialogPixelTranslation: Qt.point(pixelTranslation.x, pixelTranslation.y)
            transform: Translate {
                id: pixelTranslation
                x: root.hostWindow.dialogPixelOffsetX(content, root.hostWindow.contentItem)
                y: root.hostWindow.dialogPixelOffsetY(content, root.hostWindow.contentItem)
            }
            Repeater {
                model: SemanticChildrenModel { widgets: root.widget.children || [] }
                delegate: Loader {
                    id: childLoader
                    required property var widgetData
                    required property var labelData
                    readonly property real availableWidth: Math.max(1, content.width - root.hostWindow.pxX(Number(widgetData.x || 0) - Number(root.widget.x || 0)))
                    width: root.description || widgetData.kind === "group" ? availableWidth
                        : Math.min(root.hostWindow.pxW(widgetData.w || 1), availableWidth)
                    Component.onCompleted: setSource(Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {
                        "hostWindow": root.hostWindow,
                        "widget": Qt.binding(() => childLoader.widgetData),
                        "labelData": Qt.binding(() => childLoader.labelData),
                        "dialogLayout": metrics,
                        "originX": Qt.binding(() => Number(root.widget.x || 0)),
                        "originY": Qt.binding(() => Number(root.widget.y || 0) - 1),
                        "maximumWidth": Qt.binding(() => childLoader.availableWidth)
                    })
                }
            }
        }
    }
}
