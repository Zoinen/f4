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
        ? Math.min(availableWidth, Math.max(420, hostWindow.pxW(frame.w || 60)))
        : Math.min(availableWidth, hostWindow.pxW(frame.w))
    readonly property real preferredHeight: nativeLayout
        ? Math.min(availableHeight, Math.max(180, hostWindow.pxH(frame.h)))
        : Math.min(availableHeight, hostWindow.pxH(frame.h))

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
        if (!widget || widget.visible === false)
            return 0

        var bottom = hostWindow.pxY(Number(widget.y || 0) - Number(frame.y || 0) - 1)
                     + Math.max(22, hostWindow.pxH(Number(widget.h || 1)))
        var children = widget.children || []
        for (var i = 0; i < children.length; ++i)
            bottom = Math.max(bottom, widgetBottom(children[i]))
        return bottom
    }

    function calculateBodyContentHeight() {
        var bottom = 0
        var children = frame.children || []
        for (var i = 0; i < children.length; ++i)
            bottom = Math.max(bottom, widgetBottom(children[i]))
        return bottom + 14
    }

    function focusedWidget(widgets) {
        for (var i = 0; i < widgets.length; ++i) {
            var widget = widgets[i]
            if (!widget || widget.visible === false)
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

        var top = hostWindow.pxY(Number(widget.y || 0) - Number(frame.y || 0) - 1)
        var bottom = top + Math.max(22, hostWindow.pxH(Number(widget.h || 1)))
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
        anchors.left: parent.left
        anchors.right: dialogWindowButtons.left
        anchors.top: parent.top
        height: dialogHeader.height
        anchors.leftMargin: 18
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
        anchors.right: parent.right
        height: dialogHeader.height
        spacing: 0

        ZG.TitleButton {
            id: dialogMaximizeButton
            objectName: "dialogMaximizeButton"
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
            implicitWidth: 42
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
            onClicked: hostWindow.action({
                "target": frame.id,
                "action": "dialog.close"
            })
        }
    }

    Flickable {
        id: dialogBody
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.topMargin: dialogHeader.height + 10
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

        T.ScrollBar.vertical: T.ScrollBar {
            policy: T.ScrollBar.AsNeeded
            width: 10
            rightPadding: 2
            contentItem: Rectangle {
                implicitWidth: 5
                radius: width / 2
                color: parent.pressed || parent.hovered ? hostWindow.dialogAccent : hostWindow.mutedText
                opacity: parent.active ? 0.85 : 0.55
                Behavior on color { ColorAnimation { duration: 90 } }
                Behavior on opacity { NumberAnimation { duration: 90 } }
            }
            background: Item { }
        }

        Item {
            width: dialogBody.width
            height: dialogBody.contentHeight

            Repeater {
                model: frame.children || []
                delegate: SemanticWidgetDelegate {
                    hostWindow: dialogRoot.hostWindow
                    widget: modelData
                    originX: frame.x || 0
                    originY: frame.y || 0
                    maximumWidth: Math.max(1, dialogBody.width
                                          - hostWindow.pxX((modelData.x || 0)
                                                     - (frame.x || 0))
                                          - 16)
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
