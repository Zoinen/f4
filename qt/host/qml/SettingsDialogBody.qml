pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import F4QtHost 1.0

// Pane roles come from the owner. Terminal rectangles remain content hints;
// the live native rectangle owns docking throughout a resize gesture.
Item {
    id: body
    objectName: "settingsDialogBody"
    required property ApplicationWindow hostWindow
    property var widgets: []
    readonly property real gap: hostWindow.snapPx(12)
    readonly property real lineHeight: hostWindow.snapPx(Math.max(22, hostWindow.font.pixelSize + 4))
    readonly property real controlHeight: hostWindow.snapPx(Math.max(32, hostWindow.font.pixelSize + 16))
    readonly property real navigationWidth: hostWindow.snapPx(Math.min(260, Math.max(170, width * .26)))
    readonly property real mainX: navigationWidth + gap
    readonly property real mainWidth: Math.max(1, width - mainX)
    readonly property real footerY: Math.max(0, height - controlHeight)
    readonly property real paneBottom: Math.max(0, footerY - gap)
    readonly property bool wideHelp: mainWidth >= 720
    readonly property real helpWidth: hostWindow.snapPx(Math.min(280, mainWidth * .3))
    readonly property real helpHeight: hostWindow.snapPx(Math.min(110, Math.max(50, paneBottom * .22)))
    readonly property real pageY: lineHeight + gap
    readonly property real pageWidth: Math.max(1, mainWidth - (wideHelp ? helpWidth + gap : 0))
    readonly property real pageHeight: Math.max(1, paneBottom - pageY - (wideHelp ? 0 : helpHeight + gap))
    readonly property real buttonWidth: hostWindow.snapPx(Math.min(104, (width - gap * 2) / 3))
    readonly property bool hasMatches: widgets.some(w => w.layoutRole === "search-matches" && w.visible !== false)
    readonly property bool hasClear: widgets.some(w => w.layoutRole === "search-clear" && w.visible !== false)
    readonly property real searchY: lineHeight + hostWindow.snapPx(4)
    readonly property real navigationY: searchY + controlHeight + gap + (hasMatches ? lineHeight : 0)

    function rectangle(role) {
        const tools = (hasClear ? controlHeight : 0) + (hasMatches ? 2 * controlHeight : 0)
        switch (role) {
        case "search-label": return Qt.rect(0, 0, navigationWidth, lineHeight)
        case "search": return Qt.rect(0, searchY, Math.max(1, navigationWidth - tools), controlHeight)
        case "search-clear": return Qt.rect(navigationWidth - tools, searchY, controlHeight, controlHeight)
        case "search-previous": return Qt.rect(navigationWidth - 2 * controlHeight, searchY, controlHeight, controlHeight)
        case "search-next": return Qt.rect(navigationWidth - controlHeight, searchY, controlHeight, controlHeight)
        case "search-matches": return Qt.rect(0, searchY + controlHeight + gap, navigationWidth, lineHeight)
        case "navigation": return Qt.rect(0, navigationY, navigationWidth, Math.max(1, paneBottom - navigationY))
        case "content-title": return Qt.rect(mainX, 0, pageWidth, lineHeight)
        case "content": return Qt.rect(mainX, pageY, pageWidth, pageHeight)
        case "description": return wideHelp
            ? Qt.rect(mainX + pageWidth + gap, 0, helpWidth, paneBottom)
            : Qt.rect(mainX, paneBottom - helpHeight, mainWidth, helpHeight)
        case "apply": return Qt.rect(width - 3 * buttonWidth - 2 * gap, footerY, buttonWidth, controlHeight)
        case "accept": return Qt.rect(width - 2 * buttonWidth - gap, footerY, buttonWidth, controlHeight)
        case "cancel": return Qt.rect(width - buttonWidth, footerY, buttonWidth, controlHeight)
        case "footer-status": return Qt.rect(0, footerY, Math.max(1, width - 3 * (buttonWidth + gap)), controlHeight)
        default: return Qt.rect(0, 0, 0, 0)
        }
    }

    Repeater {
        model: SemanticChildrenModel { widgets: body.widgets }
        delegate: SemanticWidgetDelegate {
            required property var widgetData
            required property var labelData
            readonly property rect placement: body.rectangle(widgetData.layoutRole || "")
            hostWindow: body.hostWindow
            widget: widgetData
            x: hostWindow.snapPx(placement.x)
            y: hostWindow.snapPx(placement.y)
            width: hostWindow.snapPx(placement.width)
            height: hostWindow.snapPx(placement.height)
            maximumWidth: width
            visible: widgetData.visible !== false && placement.width > 0
        }
    }
}
