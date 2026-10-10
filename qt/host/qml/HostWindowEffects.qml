pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

QtObject {
    id: effects
    required property ApplicationWindow hostWindow
    required property QtObject windowAgent
    required property bool usesQwk

    function applyPlatformWindowEffects() {
        if (!hostWindow.windowAgentReady || !hostWindow.useMacNativeTitleBar
                || !hostWindow.supportsTransparentWindowBackground)
            return
        windowAgent.setWindowAttribute("blur-effect", "none")
        windowAgent.setWindowAttribute("glass-corner-radius", 0)
        windowAgent.setWindowAttribute("glass-tint-color", "none")
        const glassApplied = windowAgent.setWindowAttribute(
                                 "glass-effect", hostWindow.macWindowGlassEffect) === true
        const applied = glassApplied || windowAgent.setWindowAttribute(
                            "blur-effect", hostWindow.macWindowFallbackBlurEffect) === true
        hostWindow.isQWKLegacy = !applied
        if (!applied && hostWindow.macWindowEffectApplyAttempts < 10) {
            ++hostWindow.macWindowEffectApplyAttempts
            macWindowEffectRetryTimer.restart()
        }
    }

    function initialize() {
        if (!usesQwk)
            return
        windowAgent.setup(hostWindow)
        hostWindow.windowAgentReady = true
        if (Qt.platform.os === "windows") {
            hostWindow.isQWKLegacy = hostWindow.supportsTransparentWindowBackground
                    ? windowAgent.setWindowAttribute("mica-alt", true) !== true
                    : true
        } else if (hostWindow.useMacNativeTitleBar) {
            if (hostWindow.supportsTransparentWindowBackground)
                applyPlatformWindowEffects()
            else
                hostWindow.isQWKLegacy = true
        }
        if (hostWindow.titleBarItem)
            windowAgent.setTitleBar(hostWindow.titleBarItem)
        if (Qt.platform.os !== "osx" && hostWindow.appIconItem)
            windowAgent.setHitTestVisible(hostWindow.appIconItem)
        if (hostWindow.workspaceBarItem) {
            windowAgent.setHitTestVisible(hostWindow.workspaceBarItem)
            hostWindow.workspaceBarHitTestRegistered = true
        }
        if (hostWindow.useMacNativeTitleBar && hostWindow.macSystemButtonAreaItem)
            windowAgent.setSystemButtonArea(hostWindow.macSystemButtonAreaItem)
    }

    readonly property Timer macWindowEffectRetryTimer: Timer {
        interval: 100
        repeat: false
        onTriggered: effects.applyPlatformWindowEffects()
    }
}
