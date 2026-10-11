pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Rectangle {
    id: fastFindOverlay
    required property Item panelSurface
    required property Item statusOverlay
    readonly property ApplicationWindow hostWindow: panelSurface.hostWindow
    readonly property var panel: panelSurface.panel
    objectName: "panelFastFindOverlay-"
                + Number(panel.side || 0)
    anchors.bottom: parent.bottom
    anchors.bottomMargin: margin
    readonly property real margin: hostWindow.snapPx(hostWindow.panelContentSpacing)
    readonly property real availableRight: statusOverlay.visible
        ? statusOverlay.x - margin : parent.width - margin
    readonly property real desiredWidth:
        Math.max(hostWindow.snapPx(220),
                 fastFindQuery.implicitWidth + hostWindow.snapPx(64))
    width: Math.max(1, Math.floor(Math.min(availableRight - margin, desiredWidth)
                                 * hostWindow.dpr)) / hostWindow.dpr
    x: hostWindow.snapPx(Math.max(margin,
                        Math.min((parent.width - width) / 2, availableRight - width)))
    height: hostWindow.snapPx(36)
    visible: panel.fastFind === true
    z: 4
    clip: true
    radius: hostWindow.snapPx(8)
    color: hostWindow.dialogBg
    border.width: hostWindow.separatorWidth
    border.color: hostWindow.controlBorder
    transform: Translate {
        x: hostWindow.dialogPixelOffsetX(fastFindOverlay, hostWindow.contentItem)
        y: hostWindow.dialogPixelOffsetY(fastFindOverlay, hostWindow.contentItem)
    }

    FontMetrics {
        id: fastFindFontMetrics
        font.family: hostWindow.uiFontFamily
        font.pixelSize: (hostWindow ? hostWindow.uiTextSize(13) : 13)
    }

    HostPixelAlignedImage {
        hostWindow: panelSurface.hostWindow
        id: fastFindIcon
        objectName: "panelFastFindIcon-"
                    + Number(panel.side || 0)
        anchors.left: parent.left
        anchors.leftMargin: hostWindow.snapPx(10)
        anchors.verticalCenter: parent.verticalCenter
        width: hostWindow.snapPx(15)
        height: hostWindow.snapPx(15)
        smooth: false
        source: hostWindow.lucideIconSource("search", 15,
                                     hostWindow.dialogAccent)
    }

    Text {
        id: fastFindQuery
        objectName: "panelFastFindText-"
                    + Number(panel.side || 0)
        x: fastFindIcon.x + fastFindIcon.width + hostWindow.snapPx(8)
        y: hostWindow.snapPx((parent.height - height) / 2)
        width: Math.max(0, parent.width - x - hostWindow.snapPx(10))
        height: Math.ceil(implicitHeight * hostWindow.dpr) / hostWindow.dpr
        text: hostWindow.cleanText(panel.fastFindText)
        color: panel.fastFindNoMatch === true
            ? Qt.tint(hostWindow.textColor, "#66e07070")
            : hostWindow.textColor
        font.family: hostWindow.uiFontFamily
        font.pixelSize: (hostWindow ? hostWindow.uiTextSize(13) : 13)
        elide: Text.ElideLeft
        verticalAlignment: Text.AlignVCenter
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                   fastFindQuery, hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                   fastFindQuery, hostWindow.contentItem)
        }
    }

    Rectangle {
        id: fastFindCursor
        objectName: "panelFastFindCursor-"
                    + Number(panel.side || 0)
        property alias blinkOn: fastFindCursorBlinkController.blinkOn
        property alias blinkInterval:
            fastFindCursorBlinkController.interval
        readonly property bool blinkTimerRunning:
            fastFindCursorBlinkController.running
        readonly property real textAdvance:
            fastFindFontMetrics.advanceWidth(fastFindQuery.text)
        x: hostWindow.snapPx(fastFindQuery.x
                            + Math.min(fastFindQuery.width, textAdvance))
        y: fastFindQuery.y + hostWindow.snapPx(2)
        width: hostWindow.snapPx(2)
        height: Math.max(hostWindow.snapPx(1),
                         fastFindQuery.height - hostWindow.snapPx(4))
        color: fastFindQuery.color
        visible: panel.fastFind === true
        opacity: blinkOn ? 1 : 0
        z: 2
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(fastFindCursor, hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(fastFindCursor, hostWindow.contentItem)
        }

        function restartBlink() {
            fastFindCursorBlinkController.restart()
        }

        ActivityBoundedCursorBlink {
            id: fastFindCursorBlinkController
            objectName: "panelFastFindCursorBlinkController-"
                        + Number(panel.side || 0)
            active: fastFindCursor.visible
                    && fastFindOverlay.visible
                    && panelSurface.visible
                    && panelSurface.panelIsActive
                    && hostWindow.active
                    && hostWindow.nativeTwoPanelSurfaceActive
            activityRevision: hostWindow.keyboardActivityRevision
        }
    }
}
