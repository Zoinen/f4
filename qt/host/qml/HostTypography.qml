pragma ComponentBehavior: Bound

import QtQuick

Item {
    id: typography
    property string uiFamily: ""
    property int uiSize: 13
    property string panelFamily: ""
    property int panelSize: 13
    property bool panelsLinked: true
    property string monoFamily: ""
    property int monoSize: 0
    property string defaultUiFamily: "sans-serif"
    property string defaultMonoFamily: "monospace"
    property int defaultMonoSize: 16
    property string platformMonoFamily: defaultMonoFamily
    property int platformMonoSize: defaultMonoSize
    readonly property string effectiveUiFamily: uiFamily || defaultUiFamily
    readonly property string effectivePanelFamily:
        panelsLinked ? effectiveUiFamily : (panelFamily || effectiveUiFamily)
    readonly property int effectivePanelSize: panelsLinked ? uiSize : panelSize
    readonly property string effectiveMonoFamily: monoFamily || defaultMonoFamily
    readonly property int effectiveMonoSize: monoSize > 0 ? monoSize : defaultMonoSize
    readonly property font panelFont: Qt.font({family: effectivePanelFamily, pixelSize: effectivePanelSize})
    readonly property real panelLineHeight: panelMetrics.height
    readonly property real uiLineHeight: uiMetrics.height
    FontMetrics {
        id: uiMetrics
        font: Qt.font({family: typography.effectiveUiFamily, pixelSize: typography.uiSize})
    }
    // One font-dependent measurement, shared by every panel row. No per-file cache.
    readonly property real extensionWidth: {
        const selectedFont = panelMetrics.font
        return panelMetrics.advanceWidth("MMM") * 57 / 67
    }
    FontMetrics { id: panelMetrics; font: typography.panelFont }

    function snapshot() {
        return {uiFamily, uiSize, panelFamily, panelSize, panelsLinked, monoFamily, monoSize}
    }
    function restore(values) {
        const v = values || {}
        uiFamily = String(v.uiFamily || "")
        uiSize = validSize(v.uiSize, 13)
        panelFamily = String(v.panelFamily || "")
        panelSize = validSize(v.panelSize, 13)
        panelsLinked = v.panelsLinked !== false
        monoFamily = String(v.monoFamily || "")
        monoSize = validSize(v.monoSize, 0)
    }
    function validSize(value, fallback) {
        const n = Number(value)
        return Number.isFinite(n) && n >= 8 && n <= 36 ? Math.round(n) : fallback
    }
    function setLinked(linked) {
        if (panelsLinked && !linked) {
            panelFamily = effectiveUiFamily
            panelSize = uiSize
        }
        panelsLinked = linked
    }
    function resetFont(role) {
        if (role === 0) {
            uiFamily = ""
            uiSize = 13
        } else if (role === 1 && !panelsLinked) {
            panelFamily = defaultUiFamily
            panelSize = 13
        } else if (role === 2) {
            monoFamily = platformMonoFamily
            monoSize = platformMonoSize
        }
    }
}
