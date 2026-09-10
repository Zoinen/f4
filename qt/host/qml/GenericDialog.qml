pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl
import QtQuick.Layouts
import ZoinGallery 1.0 as ZG

Rectangle {
    id: dialogRoot
    required property ApplicationWindow hostWindow
    required property Item menuBar
    objectName: "semanticDialog-" + hostWindow.cleanText(frame.id)
    property var frame: ({})
    property bool nativeLayout: hostWindow.isAppScene()
    property bool userGeometrySet: false
    property bool maximized: false
    property real userX: 0
    property real userY: 0
    property real userWidth: 0
    property real userHeight: 0
    property rect restoredGeometry: Qt.rect(0, 0, 0, 0)
    readonly property real contentLeft: {
        const items = (frame.children || []).filter(isContent)
        return items.length ? Math.min(...items.map(w => Number(w.x || 0))) : Number(frame.x || 0)
    }
    readonly property real contentRight: {
        const items = (frame.children || []).filter(isContent)
        return items.length ? Math.max(...items.map(w => Number(w.x || 0) + Math.max(1, Number(w.w || 1)))) : contentLeft
    }
    readonly property real contentPadding: hostWindow.snapPx(24)
    readonly property var rowEdges: calculateRowEdges()
    readonly property real bodyContentHeight: calculateBodyContentHeight()
    readonly property real geometryLeft: 12
    readonly property real geometryTop: menuBar.height + 8
    readonly property real geometryRight: hostWindow.width - 12
    readonly property real geometryBottom: hostWindow.height - 12
    readonly property real availableWidth: Math.max(
                                               1, geometryRight - geometryLeft)
    readonly property real availableHeight: Math.max(
                                                1, geometryBottom - geometryTop)
    readonly property real minimumDialogWidth: Math.min(320, availableWidth)
    readonly property real minimumDialogHeight: Math.min(160, availableHeight)
    readonly property real preferredWidth: nativeLayout
        ? Math.min(availableWidth, Math.max(320, hostWindow.pxW(contentRight - contentLeft) + 2 * contentPadding))
        : Math.min(availableWidth, hostWindow.pxW(frame.w))
    readonly property real preferredHeight: nativeLayout
        ? Math.min(availableHeight, Math.max(100, bodyContentHeight + dialogHeader.height + contentPadding))
        : Math.min(availableHeight, hostWindow.pxH(frame.h))

    Text {
        id: messageMeasure
        visible: false
        width: Math.max(1, dialogRoot.width - 2 * dialogRoot.contentPadding)
        font: hostWindow.font
        textFormat: Text.PlainText
        wrapMode: Text.Wrap
        text: {
            const body = (frame.children || []).find(w => w.kind === "text" && w.wrapText === true)
            return body ? String(body.text || "") : ""
        }
    }

    function visualHeight(widget) {
        return widget.kind === "text" && widget.wrapText === true
            ? hostWindow.snapPx(messageMeasure.implicitHeight)
            : hostWindow.dialogWidgetVisualHeight(widget)
    }

    function isContent(widget) {
        return widget && widget.visible !== false
                && (widget.kind !== "text" || String(widget.text || widget.typeName || "").trim().length > 0)
    }

    function clamped(value, minimum, maximum) {
        return Math.max(minimum, Math.min(maximum, value))
    }

    function setUserGeometry(nextX, nextY, nextWidth, nextHeight) {
        maximized = false
        userGeometrySet = true
        userWidth = clamped(nextWidth, minimumDialogWidth, availableWidth)
        userHeight = clamped(nextHeight, minimumDialogHeight,
                             availableHeight)
        userX = clamped(nextX, geometryLeft,
                        Math.max(geometryLeft, geometryRight - userWidth))
        userY = clamped(nextY, geometryTop,
                        Math.max(geometryTop, geometryBottom - userHeight))
    }

    function moveTo(nextX, nextY) {
        if (maximized) {
            const restore = restoredGeometry
            setUserGeometry(nextX, nextY,
                            restore.width > 0 ? restore.width : preferredWidth,
                            restore.height > 0 ? restore.height : preferredHeight)
            return
        }
        setUserGeometry(nextX, nextY, width, height)
    }

    function resizeFrom(edges, deltaX, deltaY, start) {
        if (maximized)
            return

        let left = start.x
        let top = start.y
        let right = start.x + start.width
        let bottom = start.y + start.height
        if ((edges & 1) !== 0)
            left = clamped(start.x + deltaX, geometryLeft,
                           right - minimumDialogWidth)
        if ((edges & 2) !== 0)
            right = clamped(start.x + start.width + deltaX,
                            left + minimumDialogWidth, geometryRight)
        if ((edges & 4) !== 0)
            top = clamped(start.y + deltaY, geometryTop,
                          bottom - minimumDialogHeight)
        if ((edges & 8) !== 0)
            bottom = clamped(start.y + start.height + deltaY,
                             top + minimumDialogHeight, geometryBottom)
        setUserGeometry(left, top, right - left, bottom - top)
    }

    function commitGeometry() {
        if (!frame || hostWindow.cleanText(frame.id) === "")
            return
        hostWindow.action({
            "target": frame.id,
            "action": "dialog.geometry",
            "x": Math.round(x / hostWindow.cw),
            "y": Math.round(y / hostWindow.ch),
            "w": Math.max(1, Math.round(width / hostWindow.cw)),
            "h": Math.max(1, Math.round(height / hostWindow.ch))
        }, true)
    }

    function toggleMaximized() {
        if (maximized) {
            const restore = restoredGeometry
            setUserGeometry(restore.x, restore.y,
                            restore.width, restore.height)
        } else {
            restoredGeometry = Qt.rect(x, y, width, height)
            maximized = true
        }
        Qt.callLater(commitGeometry)
    }

    function widgetBottom(widget) {
        if (!isContent(widget))
            return 0

        var bottom = widgetTop(widget) + widgetHeight(widget)
        var children = widget.children || []
        for (var i = 0; i < children.length; ++i)
            bottom = Math.max(bottom, widgetBottom(children[i]))
        return bottom
    }

    function calculateRowEdges() {
        const rows = []
        const controls = []
        const base = hostWindow.snapPx(Math.max(22, hostWindow.ch))
        function collect(widgets) {
            for (const widget of widgets) {
                if (!isContent(widget))
                    continue
                const start = Math.max(0, Number(widget.y || 0) - Number(frame.y || 0) - 1)
                const span = Math.max(1, Number(widget.h || 1))
                while (rows.length < start + span)
                    rows.push(0)
                for (let row = start; row < start + span; ++row)
                    rows[row] = base
                if (widget.kind !== "group")
                    controls.push({start: start, span: span, widget: widget})
                collect(widget.children || [])
            }
        }
        collect(frame.children || [])
        // Trim outer whitespace; each internal blank run becomes one section
        // gap (twice the 8-DIP allowance used by native controls).
        const first = rows.findIndex(height => height > 0)
        for (let row = Math.max(0, first); row < rows.length; ++row) {
            if (rows[row] === 0) {
                rows[row] = hostWindow.snapPx(16)
                while (row + 1 < rows.length && rows[row + 1] === 0)
                    ++row
            }
        }
        // Shared rows reserve the tallest control once.
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
        const row = Number(absoluteRow) - Number(frame.y || 0) - 1
        const base = hostWindow.snapPx(Math.max(22, hostWindow.ch))
        if (row < 0)
            return 0
        if (row < rowEdges.length)
            return rowEdges[row]
        return rowEdges[rowEdges.length - 1]
    }

    function widgetHeight(widget) {
        return widget.kind === "group"
                ? rowTop(Number(widget.y || 0) + Math.max(1, Number(widget.h || 1)))
                  - rowTop(Number(widget.y || 0))
                : visualHeight(widget)
    }

    function widgetTop(widget) {
        const start = rowTop(Number(widget.y || 0))
        const end = rowTop(Number(widget.y || 0) + Math.max(1, Number(widget.h || 1)))
        return hostWindow.snapPx(start + (widget.kind === "group"
                                         ? 0 : (end - start - widgetHeight(widget)) / 2))
    }

    function calculateBodyContentHeight() {
        var bottom = 0
        var children = frame.children || []
        for (var i = 0; i < children.length; ++i)
            bottom = Math.max(bottom, widgetBottom(children[i]))
        return bottom + contentPadding
    }

    function focusedWidget(widgets) {
        for (var i = 0; i < widgets.length; ++i) {
            var widget = widgets[i]
            if (!isContent(widget))
                continue
            if (widget.focused === true)
                return widget
            var nested = focusedWidget(widget.children || [])
            if (nested)
                return nested
        }
        return null
    }

    function ensureFocusedWidgetVisible() {
        if (!dialogBody || dialogBody.height <= 0)
            return
        var widget = focusedWidget(frame.children || [])
        if (!widget)
            return

        var top = widgetTop(widget)
        var bottom = top + widgetHeight(widget)
        var maximum = Math.max(0, dialogBody.contentHeight - dialogBody.height)
        if (top < dialogBody.contentY)
            dialogBody.contentY = Math.max(0, top - 6)
        else if (bottom > dialogBody.contentY + dialogBody.height)
            dialogBody.contentY = Math.min(maximum, bottom - dialogBody.height + 6)
    }

    onFrameChanged: Qt.callLater(ensureFocusedWidgetVisible)

    width: maximized ? availableWidth
                     : userGeometrySet
                       ? clamped(userWidth, minimumDialogWidth,
                                 availableWidth)
                       : preferredWidth
    height: maximized ? availableHeight
                      : userGeometrySet
                        ? clamped(userHeight, minimumDialogHeight,
                                  availableHeight)
                        : preferredHeight
    x: maximized ? geometryLeft
                 : userGeometrySet
                   ? clamped(userX, geometryLeft,
                             Math.max(geometryLeft, geometryRight - width))
                   : clamped(nativeLayout
                             ? Math.round((hostWindow.width - width) / 2)
                             : hostWindow.pxX(frame.x),
                             geometryLeft,
                             Math.max(geometryLeft, geometryRight - width))
    y: maximized ? geometryTop
                 : userGeometrySet
                   ? clamped(userY, geometryTop,
                             Math.max(geometryTop, geometryBottom - height))
                   : clamped(nativeLayout
                             ? Math.round((hostWindow.height - height) / 2)
                             : hostWindow.pxY(frame.y),
                             geometryTop,
                             Math.max(geometryTop, geometryBottom - height))
    color: hostWindow.dialogBg
    border.width: 1
    border.color: "#46586b"
    radius: 9
    clip: true

    MouseArea {
        z: -1
        anchors.fill: parent
        acceptedButtons: Qt.AllButtons
        hoverEnabled: true
        preventStealing: true
        onPressed: (mouse) => { mouse.accepted = true }
        onReleased: (mouse) => { mouse.accepted = true }
        onPositionChanged: (mouse) => { mouse.accepted = true }
        onWheel: (wheel) => { wheel.accepted = true }
    }

    Rectangle {
        id: dialogHeader
        objectName: "dialogMoveHandle"
        x: 1
        y: 1
        width: parent.width - 2
        height: 43
        color: hostWindow.dialogHeaderBg
        radius: 8

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: parent.radius
            color: parent.color
        }

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: 1
            color: hostWindow.separatorColor
            opacity: 0.55
        }

        MouseArea {
            id: dialogMoveArea
            anchors.fill: parent
            anchors.rightMargin: dialogWindowButtons.width
            acceptedButtons: Qt.LeftButton
            hoverEnabled: true
            preventStealing: true
            cursorShape: Qt.SizeAllCursor
            property point pressPoint: Qt.point(0, 0)
            property point startPosition: Qt.point(0, 0)

            onPressed: function(mouse) {
                pressPoint = mapToItem(dialogRoot.parent, mouse.x, mouse.y)
                startPosition = Qt.point(dialogRoot.x, dialogRoot.y)
                mouse.accepted = true
            }
            onPositionChanged: function(mouse) {
                if (!pressed)
                    return
                const point = mapToItem(dialogRoot.parent, mouse.x, mouse.y)
                dialogRoot.moveTo(startPosition.x + point.x - pressPoint.x,
                                  startPosition.y + point.y - pressPoint.y)
                mouse.accepted = true
            }
            onReleased: function(mouse) {
                dialogRoot.commitGeometry()
                mouse.accepted = true
            }
            onDoubleClicked: function(mouse) {
                dialogRoot.toggleMaximized()
                mouse.accepted = true
            }
        }
    }

    Text {
        id: dialogTitle
        objectName: "semanticDialogTitle"
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(dialogTitle, hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(dialogTitle, hostWindow.contentItem)
        }
        anchors.left: parent.left
        anchors.right: dialogWindowButtons.left
        anchors.top: dialogHeader.top
        height: dialogHeader.height
        anchors.leftMargin: hostWindow.snapPx(18)
        verticalAlignment: Text.AlignVCenter
        text: hostWindow.cleanText(frame.title)
        color: hostWindow.textColor
        font.pixelSize: 14
        font.weight: Font.DemiBold
        elide: Text.ElideMiddle
    }

    Row {
        id: dialogWindowButtons
        anchors.top: dialogHeader.top
        anchors.right: dialogHeader.right
        height: dialogHeader.height
        spacing: 0

        ZG.TitleButton {
            id: dialogMaximizeButton
            objectName: "dialogMaximizeButton"
            visible: false
            implicitWidth: 42
            implicitHeight: dialogHeader.height
            opacity: 1
            source: dialogRoot.maximized
                    ? "qrc:/ZoinGallery/resources/WindowRestore.svg"
                    : "qrc:/ZoinGallery/resources/WindowMaximize.svg"
            icon.color: ZG.Style.text
            onClicked: dialogRoot.toggleMaximized()
        }

        ZG.TitleButton {
            id: dialogCloseButton
            objectName: "dialogCloseButton"
            devicePixelRatio: hostWindow.iconDevicePixelRatio
            function iconPixelOffsetX(item) {
                return hostWindow.dialogPixelOffsetX(item, hostWindow.contentItem)
            }
            function iconPixelOffsetY(item) {
                return hostWindow.dialogPixelOffsetY(item, hostWindow.contentItem)
            }
            implicitWidth: hostWindow.snapPx(42)
            implicitHeight: dialogHeader.height
            opacity: 1
            visible: frame.showClose === true
            source: "qrc:/ZoinGallery/resources/WindowClose.svg"
            icon.color: hovered
                        ? ZG.Style.closeButtonHoveredIcon : ZG.Style.text
            backgroundColor: {
                if (!enabled)
                    return "gray"
                if (pressed)
                    return ZG.Style.closeButtonPressed
                if (hovered)
                    return ZG.Style.closeButtonHovered
                return "transparent"
            }
            background: Rectangle {
                id: closeBackground
                objectName: "dialogCloseBackground"
                color: dialogCloseButton.backgroundColor
                topRightRadius: hostWindow.snapPx(dialogHeader.radius)
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(closeBackground, hostWindow.contentItem)
                    y: hostWindow.dialogPixelOffsetY(closeBackground, hostWindow.contentItem)
                }
            }
            onClicked: hostWindow.action({
                "target": frame.id,
                "action": "dialog.close"
            })
        }
    }

    Flickable {
        id: dialogBody
        objectName: "dialogBody"
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.topMargin: dialogHeader.height + dialogRoot.contentPadding
        anchors.bottom: parent.bottom
        clip: true
        contentWidth: width
        contentHeight: Math.max(height, dialogRoot.bodyContentHeight)
        flickableDirection: Flickable.VerticalFlick
        boundsBehavior: Flickable.StopAtBounds
        boundsMovement: Flickable.StopAtBounds
        interactive: contentHeight > height

        onHeightChanged: contentY = Math.min(contentY, Math.max(0, contentHeight - height))
        onContentHeightChanged: contentY = Math.min(contentY, Math.max(0, contentHeight - height))

        ScrollBar.vertical: F4ScrollBar {
            objectName: "dialogBodyScrollBar"
            hostWindow: dialogRoot.hostWindow
            policy: ScrollBar.AsNeeded
            visible: dialogBody.contentHeight > dialogBody.height + 0.5
        }

        Item {
            width: dialogBody.width
            height: dialogBody.contentHeight

            Repeater {
                model: frame.children || []
                delegate: SemanticWidgetDelegate {
                    required property var modelData
                    hostWindow: dialogRoot.hostWindow
                    dialogLayout: dialogRoot
                    siblingWidgets: frame.children || []
                    widget: modelData
                    visible: dialogRoot.isContent(modelData)
                    originX: dialogRoot.contentLeft
                    originY: frame.y || 0
                    x: dialogRoot.contentPadding + horizontalPosition
                    maximumWidth: Math.max(1, dialogBody.width - x - dialogRoot.contentPadding)
                }
            }
        }
    }

    DialogResizeHandle {
        targetDialog: dialogRoot
        edges: 1
        x: 0
        y: 9
        width: 6
        height: Math.max(1, dialogRoot.height - 18)
        z: 500
        visible: !dialogRoot.maximized
    }
    DialogResizeHandle {
        targetDialog: dialogRoot
        edges: 2
        x: dialogRoot.width - width
        y: 9
        width: 6
        height: Math.max(1, dialogRoot.height - 18)
        z: 500
        visible: !dialogRoot.maximized
    }
    DialogResizeHandle {
        targetDialog: dialogRoot
        edges: 4
        x: 9
        y: 0
        width: Math.max(1, dialogRoot.width - 18)
        height: 6
        z: 500
        visible: !dialogRoot.maximized
    }
    DialogResizeHandle {
        targetDialog: dialogRoot
        edges: 8
        x: 9
        y: dialogRoot.height - height
        width: Math.max(1, dialogRoot.width - 18)
        height: 6
        z: 500
        visible: !dialogRoot.maximized
    }
    DialogResizeHandle {
        objectName: "dialogResizeTopLeft"
        targetDialog: dialogRoot
        edges: 5
        x: 0
        y: 0
        width: 10
        height: 10
        z: 501
        visible: !dialogRoot.maximized
    }
    DialogResizeHandle {
        objectName: "dialogResizeTopRight"
        targetDialog: dialogRoot
        edges: 6
        x: dialogRoot.width - width
        y: 0
        width: 10
        height: 10
        z: 501
        visible: !dialogRoot.maximized
    }
    DialogResizeHandle {
        objectName: "dialogResizeBottomLeft"
        targetDialog: dialogRoot
        edges: 9
        x: 0
        y: dialogRoot.height - height
        width: 10
        height: 10
        z: 501
        visible: !dialogRoot.maximized
    }
    DialogResizeHandle {
        objectName: "dialogResizeBottomRight"
        targetDialog: dialogRoot
        edges: 10
        x: dialogRoot.width - width
        y: dialogRoot.height - height
        width: 10
        height: 10
        z: 501
        visible: !dialogRoot.maximized
    }
}
