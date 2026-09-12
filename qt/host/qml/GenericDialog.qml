pragma ComponentBehavior: Bound

import QtQuick
import F4QtHost 1.0
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
    // Optional native content uses the same chrome, geometry and resize path as
    // semantic form controls. The content component owns only its body.
    property Component contentComponent: null
    readonly property bool customContent: contentComponent !== null
    property string backAction: hostWindow.cleanText(frame.backAction)
    property bool canGoBack: frame.canGoBack === true
    readonly property bool settingsLayout: frame.layout === "settings"
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
    readonly property real bodyContentHeight: settingsLayout || customContent ? 0 : calculateBodyContentHeight()
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
    readonly property real preferredWidth: customContent
        ? Math.min(availableWidth, Math.max(320, (customBody.item ? customBody.item.implicitWidth : 0) + 2 * contentPadding)) : settingsLayout
        ? Math.min(availableWidth, Math.max(640, hostWindow.pxW(frame.w))) : nativeLayout
        ? Math.min(availableWidth, Math.max(320, hostWindow.pxW(contentRight - contentLeft) + 2 * contentPadding))
        : Math.min(availableWidth, hostWindow.pxW(frame.w))
    readonly property real preferredHeight: customContent
        ? Math.min(availableHeight, Math.max(160, (customBody.item ? customBody.item.implicitHeight : 0) + dialogHeader.height + 2 * contentPadding)) : settingsLayout
        ? Math.min(availableHeight, Math.max(400, hostWindow.pxH(frame.h))) : nativeLayout
        ? Math.min(availableHeight, Math.max(100, bodyContentHeight + dialogHeader.height + contentPadding))
        : Math.min(availableHeight, hostWindow.pxH(frame.h))

    SemanticDialogLayout {
        id: contentLayout
        hostWindow: dialogRoot.hostWindow
        widgets: dialogRoot.settingsLayout || dialogRoot.customContent ? [] : frame.children || []
        originX: dialogRoot.contentLeft
        originY: Number(frame.y || 0) + 1
        maximumWidth: Math.max(1, dialogRoot.width - 2 * dialogRoot.contentPadding)
    }

    function isContent(widget) { return contentLayout.isContent(widget) }
    function visualHeight(widget) { return contentLayout.visualHeight(widget) }

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

    function widgetBottom(widget) { return contentLayout.widgetBottom(widget) }
    function calculateRowEdges() { return contentLayout.rowEdges }
    function rowTop(row) { return contentLayout.rowTop(row) }
    function widgetHeight(widget) { return contentLayout.widgetHeight(widget) }
    function widgetTop(widget) { return contentLayout.widgetTop(widget) }

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
            if (widget.focused === true || (widget.scrollable === true && contentLayout.focusedWidget(widget.children || [])))
                return widget
            var nested = focusedWidget(widget.children || [])
            if (nested)
                return nested
        }
        return null
    }

    function ensureFocusedWidgetVisible() {
        if (settingsLayout || customContent || !dialogBody || dialogBody.height <= 0)
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

    width: hostWindow.snapPx(maximized ? availableWidth
                     : userGeometrySet
                       ? clamped(userWidth, minimumDialogWidth,
                                 availableWidth)
                       : preferredWidth)
    height: hostWindow.snapPx(maximized ? availableHeight
                      : userGeometrySet
                        ? clamped(userHeight, minimumDialogHeight,
                                  availableHeight)
                        : preferredHeight)
    x: hostWindow.snapPx(maximized ? geometryLeft
                 : userGeometrySet
                   ? clamped(userX, geometryLeft,
                             Math.max(geometryLeft, geometryRight - width))
                   : clamped(nativeLayout
                             ? Math.round((hostWindow.width - width) / 2)
                             : hostWindow.pxX(frame.x),
                             geometryLeft,
                             Math.max(geometryLeft, geometryRight - width)))
    y: hostWindow.snapPx(maximized ? geometryTop
                 : userGeometrySet
                   ? clamped(userY, geometryTop,
                             Math.max(geometryTop, geometryBottom - height))
                   : clamped(nativeLayout
                             ? Math.round((hostWindow.height - height) / 2)
                             : hostWindow.pxY(frame.y),
                             geometryTop,
                             Math.max(geometryTop, geometryBottom - height)))
    color: hostWindow.dialogBg
    border.width: hostWindow.snapPx(1)
    border.color: "#46586b"
    radius: hostWindow.snapPx(9)
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
        x: hostWindow.snapPx(1)
        y: hostWindow.snapPx(1)
        width: parent.width - 2 * x
        height: hostWindow.snapPx(43)
        color: hostWindow.dialogHeaderBg
        radius: hostWindow.snapPx(8)

        Rectangle {
            objectName: "dialogHeaderFill"
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: parent.radius
            color: parent.color
        }

        Rectangle {
            objectName: "dialogHeaderSeparator"
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: hostWindow.snapPx(1)
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
        height: Math.ceil(implicitHeight * hostWindow.dpr) / hostWindow.dpr
        anchors.topMargin: hostWindow.snapPx((dialogHeader.height - height) / 2)
        anchors.leftMargin: dialogBackButton.visible
                            ? hostWindow.snapPx(dialogBackButton.x + dialogBackButton.width + 8)
                            : hostWindow.snapPx(18)
        text: hostWindow.cleanText(frame.title)
        color: hostWindow.textColor
        font.pixelSize: 14
        font.weight: Font.DemiBold
        elide: Text.ElideMiddle
    }

    F4Button {
        id: dialogBackButton
        objectName: "dialogBackButton"
        hostWindow: dialogRoot.hostWindow
        visible: dialogRoot.canGoBack && dialogRoot.backAction !== ""
        variant: "tool"
        flat: true
        x: hostWindow.snapPx(8)
        y: hostWindow.snapPx(dialogHeader.y + (dialogHeader.height - height) / 2)
        iconSource: hostWindow.semanticMenuIconSource("arrow-left", iconSize, hostWindow.textColor)
        Accessible.name: "Back"
        onClicked: hostWindow.action({ "target": frame.id, "action": dialogRoot.backAction }, true)
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

    SettingsDialogBody {
        hostWindow: dialogRoot.hostWindow
        visible: dialogRoot.settingsLayout
        widgets: visible ? dialogRoot.frame.children || [] : []
        anchors.fill: parent
        anchors.margins: dialogRoot.contentPadding
        anchors.topMargin: dialogHeader.height + dialogRoot.contentPadding
    }

    Loader {
        id: customBody
        objectName: "dialogCustomBody"
        active: dialogRoot.customContent
        sourceComponent: dialogRoot.contentComponent
        x: dialogRoot.contentPadding
        y: dialogHeader.height + dialogRoot.contentPadding
        width: Math.max(0, dialogRoot.width - 2 * dialogRoot.contentPadding)
        height: Math.max(0, dialogRoot.height - y - dialogRoot.contentPadding)
    }

    Flickable {
        id: dialogBody
        visible: !dialogRoot.settingsLayout && !dialogRoot.customContent
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
                model: SemanticChildrenModel { widgets: dialogRoot.settingsLayout || dialogRoot.customContent ? [] : frame.children || [] }
                delegate: SemanticWidgetDelegate {
                    required property var widgetData
                    required labelData
                    hostWindow: dialogRoot.hostWindow
                    dialogLayout: dialogRoot
                    widget: widgetData
                    visible: dialogRoot.isContent(widgetData)
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
