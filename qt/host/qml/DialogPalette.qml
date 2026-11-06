pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

QtObject {
    id: colors
    property ApplicationWindow hostWindow: null
    property color accent: referenceAccent
    readonly property color referenceAccent: hostWindow ? hostWindow.dialogAccent : "#4e9bd4"

    // Preserve every existing theme value exactly until the scope changes accent.
    // Rebase its hue/chroma while retaining each role's brightness and alpha.
    function tone(base) {
        if (accent.toString() === referenceAccent.toString())
            return base
        const value = Qt.tint(base, "transparent")
        const saturationScale = referenceAccent.hsvSaturation > 0
            ? accent.hsvSaturation / referenceAccent.hsvSaturation : 1
        return Qt.hsva(Math.max(0, accent.hsvHue),
                       Math.min(1, value.hsvSaturation * saturationScale),
                       value.hsvValue, value.a)
    }

    readonly property color dialogAccent: accent
    readonly property color dialogBg: tone(hostWindow ? hostWindow.dialogBg : "#171e27")
    readonly property color dialogHeaderBg: tone(hostWindow ? hostWindow.dialogHeaderBg : "#1b242f")
    readonly property color chromeBorder: tone(Qt.rgba(70/255, 88/255, 107/255, 1))
    readonly property color controlBg: tone(hostWindow ? hostWindow.controlBg : "#222c38")
    readonly property color controlHoverBg: tone(hostWindow ? hostWindow.controlHoverBg : "#2a3745")
    readonly property color controlPressedBg: tone(hostWindow ? hostWindow.controlPressedBg : "#10161e")
    readonly property color inputHoverBg: tone(hostWindow ? hostWindow.inputHoverBg : "#1a222d")
    readonly property color controlBorder: tone(hostWindow ? hostWindow.controlBorder : "#3a495b")
    readonly property color selectedBg: tone(hostWindow ? hostWindow.selectedBg : "#285d8f")
    readonly property color separatorColor: tone(hostWindow ? hostWindow.separatorColor : "#2d3642")
    readonly property color textColor: hostWindow ? hostWindow.textColor : "#e8edf2"
    readonly property color mutedText: hostWindow ? hostWindow.mutedText : "#9aa7b5"
}
