pragma ComponentBehavior: Bound

import QtQuick
import ZoinGallery 1.0 as ZG

Item {
    id: router

    required property int side
    required property var panel
    required property GalleryPanelHostAdapter adapter
    required property ZG.GalleryPanel galleryPanel
    property var bridge: null
    property var keySink: null
    property var hostCapabilities: ({
        cursor: true,
        open: true,
        selection: true,
        viewer: true
    })
    property bool panelActive: false
    property bool commandLineHasText: false
    property bool fastFindActive: false
    property bool pendingCommanderInput: false
    property int pendingCommanderInputTimeoutMs: 2000
    property bool pendingPointerActivation: false
    property string pendingPointerActivationPanelId: ""
    property int pendingPointerActivationTimeoutMs: 6000
    property var forwardedKeysDown: ({})

    readonly property bool commanderInputActive:
        commandLineHasText || fastFindActive || pendingCommanderInput
    readonly property var effectiveHostCapabilities: ({
        cursor: hostCapabilities.cursor,
        open: hostCapabilities.open,
        selection: hostCapabilities.selection,
        viewer: hostCapabilities.viewer,
        galleryOwnsPanelInput: !commanderInputActive,
        galleryOwnsReturn: !commanderInputActive
    })

    signal pointerActivationPreviewRequested(int side)

    visible: false
    width: 0
    height: 0

    function reconcilePanelIdentity(panelState) {
        const state = panelState === undefined ? panel : panelState
        if (pendingPointerActivation
                && pendingPointerActivationPanelId
                   !== adapter.panelId(state))
            finishPendingPointerActivation()
    }

    function hasCommanderTextModifiers(modifiers) {
        // Ctrl/Meta combinations are commander shortcuts, not text entry.
        // Alt+printable is included because it starts f4 fast-find.
        return !(modifiers & (Qt.ControlModifier | Qt.MetaModifier))
    }

    function hasPrintableText(text) {
        if (!text || text.length === 0)
            return false
        for (let index = 0; index < text.length; ++index) {
            const code = text.charCodeAt(index)
            if (code >= 0x20 && code !== 0x7f
                    && !(code >= 0x80 && code <= 0x9f))
                return true
        }
        return false
    }

    function noteForwardedText(text, modifiers) {
        // Bridge only the first idle-to-commander scene transition. Re-arming
        // while Go already owns input would outlive the final Backspace.
        if (commandLineHasText || fastFindActive
                || !text || text.length === 0
                || !hasCommanderTextModifiers(modifiers))
            return
        pendingCommanderInput = true
        pendingCommanderInputTimer.interval = Math.max(
                    1, pendingCommanderInputTimeoutMs)
        pendingCommanderInputTimer.restart()
    }

    function noteForwardedKey(key, text, modifiers) {
        if (commandLineHasText || fastFindActive
                || !hasCommanderTextModifiers(modifiers))
            return
        if (hasPrintableText(text)
                || (key >= Qt.Key_Space && key <= Qt.Key_AsciiTilde)) {
            pendingCommanderInput = true
            pendingCommanderInputTimer.interval = Math.max(
                        1, pendingCommanderInputTimeoutMs)
            pendingCommanderInputTimer.restart()
        }
    }

    function acknowledgePendingCommanderInput() {
        pendingCommanderInput = false
        pendingCommanderInputTimer.stop()
    }

    function finishPendingCommanderInput() {
        pendingCommanderInput = false
        pendingCommanderInputTimer.stop()
    }

    function beginPendingPointerActivation(semanticPanelId) {
        if (panelActive)
            return
        pendingPointerActivationPanelId = String(semanticPanelId || "")
        pendingPointerActivation = true
        pendingPointerActivationTimer.restart()
    }

    function finishPendingPointerActivation() {
        pendingPointerActivation = false
        pendingPointerActivationPanelId = ""
        pendingPointerActivationTimer.stop()
    }

    function ownsZoomShortcut(event) {
        const zoomKey = event.key === Qt.Key_Plus
                || event.key === Qt.Key_Equal
                || event.key === Qt.Key_Minus || event.key === Qt.Key_0
        if (!zoomKey)
            return false
        const modifiers = event.modifiers
                & (Qt.ShiftModifier | Qt.ControlModifier
                   | Qt.AltModifier | Qt.MetaModifier)
        const control = Boolean(modifiers & Qt.ControlModifier)
        const meta = Boolean(modifiers & Qt.MetaModifier)
        if (control === meta || (modifiers & Qt.AltModifier))
            return false
        if (modifiers & Qt.ShiftModifier)
            return event.key === Qt.Key_Plus || event.key === Qt.Key_Equal
        return true
    }

    function handleZoom(event) {
        if (!galleryPanel || !ownsZoomShortcut(event))
            return false
        if (event.key === Qt.Key_0) {
            const defaultDensity = adapter.defaultDensityFor(
                        adapter.requestedPresentationMode)
            if (typeof galleryPanel.resetDensity === "function")
                galleryPanel.resetDensity(defaultDensity)
            else
                adapter.commitDensity(defaultDensity)
            return true
        }

        const zoomIn = event.key === Qt.Key_Plus
                || event.key === Qt.Key_Equal
        const zoomOut = event.key === Qt.Key_Minus
        if (!zoomIn && !zoomOut)
            return false

        if (typeof galleryPanel.stepDensity === "function") {
            galleryPanel.stepDensity(zoomIn)
            return true
        }

        // Compatibility with older lightweight embedders. Production uses the
        // discrete native step path above.
        const step = adapter.densityStep
        const currentValue = typeof galleryPanel.density !== "undefined"
                ? galleryPanel.density : galleryPanel.thumbnailHeight
        const current = Math.max(adapter.minimumDensity,
                     Math.min(adapter.maximumDensity,
                              Math.round(Number(currentValue
                                                || adapter.requestedDensity))))
        const target = zoomIn
                ? Math.min(adapter.maximumDensity, current + step)
                : Math.max(adapter.minimumDensity, current - step)

        if (target !== current) {
            if (typeof galleryPanel.beginThumbnailPinch === "function"
                    && typeof galleryPanel.updateThumbnailPinch === "function"
                    && typeof galleryPanel.finishThumbnailPinch === "function") {
                galleryPanel.beginThumbnailPinch()
                galleryPanel.updateThumbnailPinch(target / current)
                galleryPanel.finishThumbnailPinch()
            } else {
                galleryPanel.thumbnailHeight = target
            }
        }
        return true
    }

    function isPasteShortcut(event) {
        const modifiers = event.modifiers
                & (Qt.ShiftModifier | Qt.ControlModifier
                   | Qt.AltModifier | Qt.MetaModifier)
        const commandModifier = modifiers
                & (Qt.ControlModifier | Qt.MetaModifier)
        if (event.key === Qt.Key_V && commandModifier
                && !(modifiers & (Qt.AltModifier | Qt.ShiftModifier)))
            return true
        return event.key === Qt.Key_Insert
                && modifiers === Qt.ShiftModifier
    }

    function rememberForwardedKey(key) {
        const keys = Object.assign({}, forwardedKeysDown)
        keys[String(key)] = true
        forwardedKeysDown = keys
    }

    function takeForwardedKey(key) {
        const name = String(key)
        if (!forwardedKeysDown[name])
            return false
        const keys = Object.assign({}, forwardedKeysDown)
        delete keys[name]
        forwardedKeysDown = keys
        return true
    }

    function forwardQtKey(event, down) {
        if (!keySink)
            return
        if (typeof keySink.sendQtKeyEvent === "function") {
            keySink.sendQtKeyEvent(event.key, event.text, down,
                                   event.modifiers, event.nativeScanCode,
                                   event.isAutoRepeat)
        } else {
            keySink.sendQtKey(event.key, event.text, down,
                              event.modifiers, event.nativeScanCode)
        }
    }

    function forwardConsoleWheel(x, y, angleDeltaY, modifiers) {
        if (!galleryPanel || !keySink
                || typeof keySink.sendQtWheelAt !== "function")
            return
        const point = galleryPanel.mapToItem(keySink, x, y)
        keySink.sendQtWheelAt(point.x, point.y, angleDeltaY, modifiers)
    }

    function forwardConsoleMouseButton(x, y, button, down, modifiers) {
        if (!galleryPanel || !keySink
                || typeof keySink.sendQtMouseAt !== "function")
            return
        const point = galleryPanel.mapToItem(keySink, x, y)
        keySink.sendQtMouseAt(point.x, point.y, button, down, modifiers)
    }

    function commanderOwnsKey(event) {
        if (event.key === Qt.Key_Shift)
            return false
        if (event.key === Qt.Key_Plus || event.key === Qt.Key_Equal
                || event.key === Qt.Key_Minus || event.key === Qt.Key_0)
            return !ownsZoomShortcut(event)
        if (commanderInputActive)
            return true

        const routingModifiers = event.modifiers
                & (Qt.ShiftModifier | Qt.ControlModifier
                   | Qt.AltModifier | Qt.MetaModifier)
        const spatialKey = event.key === Qt.Key_Left
                || event.key === Qt.Key_Right || event.key === Qt.Key_Up
                || event.key === Qt.Key_Down
        const pageKey = event.key === Qt.Key_PageUp
                || event.key === Qt.Key_PageDown
        const edgeKey = event.key === Qt.Key_Home || event.key === Qt.Key_End
        if ((spatialKey || pageKey || edgeKey)
                && (routingModifiers === Qt.NoModifier
                    || routingModifiers === Qt.ShiftModifier))
            return false
        if (routingModifiers === Qt.NoModifier
                && (event.key === Qt.Key_Return || event.key === Qt.Key_Enter))
            return commandLineHasText
        if (routingModifiers === Qt.NoModifier
                && (event.key === Qt.Key_Space || event.key === Qt.Key_Insert))
            return false
        return true
    }

    function handlePressed(event) {
        if (isPasteShortcut(event)) {
            if (keySink)
                keySink.sendClipboardPaste()
            event.accepted = true
        } else if (handleZoom(event)) {
            event.accepted = true
        } else if (commanderOwnsKey(event)) {
            forwardQtKey(event, true)
            rememberForwardedKey(event.key)
            noteForwardedKey(event.key, event.text, event.modifiers)
            if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter
                    || event.key === Qt.Key_Escape)
                finishPendingCommanderInput()
            event.accepted = true
        }
    }

    function handleReleased(event) {
        if (isPasteShortcut(event)) {
            event.accepted = true
        } else if (forwardedKeysDown[String(event.key)]) {
            forwardQtKey(event, false)
            if (!event.isAutoRepeat)
                takeForwardedKey(event.key)
            event.accepted = true
        } else if (commanderOwnsKey(event)) {
            // Focus may have moved before release; Go still needs the matching
            // commander key-up event.
            forwardQtKey(event, false)
            event.accepted = true
        }
    }

    Timer {
        id: pendingCommanderInputTimer
        interval: Math.max(1, router.pendingCommanderInputTimeoutMs)
        repeat: false
        onTriggered: router.pendingCommanderInput = false
    }

    Timer {
        id: pendingPointerActivationTimer
        interval: Math.max(1, router.pendingPointerActivationTimeoutMs)
        repeat: false
        onTriggered: router.finishPendingPointerActivation()
    }

    Connections {
        target: router.keySink
        ignoreUnknownSignals: true

        function onCommanderTextInputForwarded(text, modifiers) {
            if (router.panelActive)
                router.noteForwardedText(text, modifiers)
        }
    }
}
