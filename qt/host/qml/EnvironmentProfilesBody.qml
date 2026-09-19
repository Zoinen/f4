pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import F4QtHost 1.0

// Terminal rectangles describe the source scene, not the live native viewport.
// Dock both expanding panes above the same action row throughout resize drags.
Item {
    id: body
    objectName: "environmentProfilesBody"
    required property ApplicationWindow hostWindow
    property var widgets: []
    readonly property real gap: hostWindow.snapPx(12)
    readonly property real smallGap: hostWindow.snapPx(6)
    readonly property real lineHeight: hostWindow.snapPx(Math.max(22, hostWindow.font.pixelSize + 4))
    readonly property real controlHeight: hostWindow.snapPx(Math.max(32, hostWindow.font.pixelSize + 16))
    readonly property real splitX: hostWindow.snapPx(width / 3)
    readonly property real leftWidth: Math.max(1, splitX - gap)
    readonly property real rightX: splitX + gap
    readonly property real rightWidth: Math.max(1, width - rightX)
    readonly property real nameLabelWidth: Math.ceil(nameLabelMetrics.implicitWidth * hostWindow.dpr) / hostWindow.dpr
    readonly property real enabledWidth: Math.ceil((enabledMetrics.implicitWidth + 35) * hostWindow.dpr) / hostWindow.dpr
    readonly property real nameX: rightX + nameLabelWidth + smallGap
    readonly property real enabledX: width - enabledWidth
    readonly property real minimumBodyWidth: hostWindow.snapPx(1.5 * (nameLabelWidth + enabledWidth + 64 + 2 * smallGap + gap))
    readonly property real variablesLabelY: controlHeight + smallGap
    readonly property real variablesY: variablesLabelY + lineHeight + smallGap
    readonly property real minimumBodyHeight: variablesY + hostWindow.snapPx(32) + gap + controlHeight
    readonly property real actionY: height - controlHeight
    readonly property real paneBottom: actionY - gap
    readonly property real buttonWidth: hostWindow.snapPx(Math.min(120, (rightWidth - smallGap) / 2))

    function caption(role) {
        for (const widget of widgets) {
            if (widget.layoutRole === role)
                return hostWindow.cleanText(widget.text)
        }
        return ""
    }
    // Measure with the same Text renderer as the visible controls.
    Text { id: nameLabelMetrics; objectName: "environmentNameMeasure"; visible: false; font: body.hostWindow.font; text: body.caption("name-label") }
    Text { id: enabledMetrics; objectName: "environmentEnabledMeasure"; visible: false; font: body.hostWindow.font; text: body.caption("enabled") }

    function rectangle(role) {
        switch (role) {
        case "profiles": return Qt.rect(0, 0, leftWidth, paneBottom)
        case "name-label": return Qt.rect(rightX, hostWindow.snapPx((controlHeight - lineHeight) / 2), nameLabelWidth, lineHeight)
        case "name": return Qt.rect(nameX, 0, Math.max(1, enabledX - smallGap - nameX), controlHeight)
        case "enabled": return Qt.rect(enabledX, 0, enabledWidth, controlHeight)
        case "variables-label": return Qt.rect(rightX, variablesLabelY, rightWidth, lineHeight)
        case "variables": return Qt.rect(rightX, variablesY, rightWidth, Math.max(1, paneBottom - variablesY))
        case "add": return Qt.rect(0, actionY, leftWidth, controlHeight)
        case "save": return Qt.rect(width - 2 * buttonWidth - smallGap, actionY, buttonWidth, controlHeight)
        case "cancel": return Qt.rect(width - buttonWidth, actionY, buttonWidth, controlHeight)
        }
        return Qt.rect(0, 0, 0, 0)
    }
    Repeater {
        model: SemanticChildrenModel { widgets: body.widgets }
        delegate: SemanticWidgetDelegate {
            required property var widgetData
            required labelData
            readonly property rect allocation: body.rectangle(widgetData.layoutRole || "")
            hostWindow: body.hostWindow
            widget: widgetData
            x: allocation.x
            y: allocation.y
            width: allocation.width
            height: allocation.height
            visible: widgetData.visible !== false && allocation.width > 0
        }
    }
}
