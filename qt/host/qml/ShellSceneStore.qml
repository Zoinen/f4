pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: store
    objectName: "shellSceneStore"

    required property ApplicationWindow hostWindow
    required property var shellController

    readonly property var typedShellState:
        shellController.shellState
    readonly property var chromeState: shellController.chromeState
    readonly property var workspaceState: shellController.workspaceState
    readonly property var overlayState: shellController.overlayState
    // A popup frame is exposed through overlayFrames(), which is a helper
    // function rather than a QML property.  Keep an explicit revision for
    // bindings that consume that helper; otherwise a menu opened/closed while
    // the fallback (console) surface is visible can leave the hidden overlay
    // model unchanged until an unrelated binding happens to reevaluate.
    readonly property string overlayFramesRevision:
        String(overlayState ? overlayState.menuRevision : 0) + ":"
        + String(overlayState ? overlayState.dialogRevision : 0)
        + ":" + String(chromeState ? chromeState.revision : 0)
    readonly property bool hasDialogs:
        (overlayState.dialogs || []).length > 0
    readonly property var commandLineState: shellController.commandLineState
    readonly property var surfaceRegistry: shellController.surfaceRegistry
    property var workspaceTabsOverride: null
    property var menuBarOverride: null
    property var keyBarOverride: null
    property var toastOverride: null
    property var leftPanelPresentationOverride: null
    property var rightPanelPresentationOverride: null
    property var leftPanelLayoutStateOverride: null
    property var rightPanelLayoutStateOverride: null

    property var retainedShellFrame: ({})
    property var retainedDocumentFrame: ({})
    property var retainedOperationsQueue: ({})
    property bool shellPresentationOverrideSet: false
    property var shellPresentationOverride: null
    property bool documentPresentationOverrideSet: false
    property var documentPresentationOverride: null
    property var documentSurfaceStateOverride: null
    property bool retainedShellSurfaceCreated: false
    property bool retainedDocumentSurfaceCreated: false
    property bool documentSurfaceActive: false
    property bool documentSurfacePrewarmed: false
    property bool retainedOperationsQueueCreated: false
    property int panelActivationOverride: -1

    readonly property var workspaceTabs:
        workspaceTabsOverride !== null
        ? workspaceTabsOverride : (workspaceState.tabs || ({}))
    readonly property var menuBarModel:
        menuBarOverride !== null ? menuBarOverride : (overlayState.menuBar || ({}))
    readonly property var keyBarModel:
        keyBarOverride !== null ? keyBarOverride : (chromeState.keyBar || ({}))
    readonly property var toastModel:
        toastOverride !== null ? toastOverride : (chromeState.toast || ({}))
    readonly property var workspaces: workspaceTabs.tabs || []

    signal sceneReset()
    signal activePanelAcknowledged(int side)
    signal structuralSurfaceUpdated()
    signal commandMenusUpdated()

    // Dialogs are part of the Go-owned input stack, but their typed state is
    // stored independently from shell/document surfaces.  Crossing the
    // no-dialog/dialog boundary must therefore participate in the same focus
    // hand-off as other structural surfaces; otherwise a native panel keeps
    // Qt active focus and consumes navigation keys before the grid can forward
    // them to the modal vtui frame.
    onHasDialogsChanged: structuralSurfaceUpdated()

    visible: false
    width: 0
    height: 0

    function mergePanelLayoutState(currentState, delta) {
        const next = delta || ({})
        const current = currentState || ({})
        const samePanel = String(current.id || "") !== ""
                && String(current.id || "") === String(next.id || "")
                && Number(current.catalogRevision || 0)
                   === Number(next.catalogRevision || 0)
        return Object.assign({}, samePanel ? current : ({}), next)
    }

    function commandLineFrame() {
        return commandLineState.frame || ({})
    }

    function isAppScene() {
        return chromeState.schema === "app"
    }

    function currentOperationsQueue() {
        return surfaceRegistry.hasOperationsQueue
                ? surfaceRegistry.operationsQueue : null
    }

    function operationsQueueFrame() {
        return currentOperationsQueue() || retainedOperationsQueue || ({})
    }

    function hasOperationsQueueSurface() {
        const queue = currentOperationsQueue()
        return queue !== null && queue.kind === "operationsQueue"
    }

    function currentShellFrame() {
        if (shellPresentationOverrideSet)
            return shellPresentationOverride
        return surfaceRegistry.hasShell ? surfaceRegistry.shell : null
    }

    function currentDocumentFrame() {
        return documentFrame
    }

    // Share one observable document projection between visibility, focus and
    // the persistent document loader.
    readonly property var documentFrame: {
        if (documentPresentationOverrideSet) {
            return isDocumentSurface(documentPresentationOverride)
                    ? documentPresentationOverride : null
        }
        if (!surfaceRegistry.hasDocument)
            return null
        const frame = surfaceRegistry.documentMetadata
        return isDocumentSurface(frame) ? frame : null
    }

    function captureShellSurface() {
        const shell = currentShellFrame()
        if (shell !== null) {
            retainedShellFrame = shell
            retainedShellSurfaceCreated = true
        }
    }

    function captureDocumentSurface() {
        const document = currentDocumentFrame()
        documentSurfaceActive = document !== null
        if (document !== null) {
            retainedDocumentFrame = document
            retainedDocumentSurfaceCreated = true
        }
    }

    function captureOperationsSurface() {
        const queue = currentOperationsQueue()
        if (queue !== null) {
            retainedOperationsQueue = queue
            retainedOperationsQueueCreated = true
        }
    }

    function captureRetainedSurfaces() {
        captureShellSurface()
        captureDocumentSurface()
        captureOperationsSurface()
    }

    function frames() {
        return []
    }

    function overlayFrames() {
        const result = []
        const menus = overlayState.commandMenus || []
        const dialogs = overlayState.dialogs || []
        // Command menus belong to the currently focused Go frame. A ComboBox
        // inside a dialog therefore pushes its VMenu after the dialog. Keep
        // that input-stack order in QML: DialogOverlay fills the window and
        // would otherwise paint over (and intercept input from) the dropdown.
        for (let index = 0; index < dialogs.length; ++index)
            result.push(dialogs[index])
        for (let index = 0; index < menus.length; ++index)
            result.push(menus[index])
        return result
    }

    function hasBlockingOverlay() {
        return overlayFrames().length > 0
    }

    function shellFrame() {
        // Every non-null shell publication is captured here already. Keep
        // hidden panels on that one presentation object when the active
        // shell is cleared for a standalone document; switching from the
        // native map to its equivalent retained map invalidates every panel.
        return retainedShellFrame || ({})
    }

    function quickViewForSide(side) {
        const shell = shellFrame()
        const quickViews = shell && shell.quickViews ? shell.quickViews : []
        for (let index = 0; index < quickViews.length; ++index) {
            if (Number(quickViews[index].side) === Number(side))
                return quickViews[index]
        }
        return null
    }

    function infoPanelForSide(side) {
        const shell = shellFrame()
        const infoPanels = shell && shell.infoPanels ? shell.infoPanels : []
        for (let index = 0; index < infoPanels.length; ++index) {
            if (Number(infoPanels[index].side) === Number(side))
                return infoPanels[index]
        }
        return null
    }

    function panelSideCovered(side) {
        return infoPanelForSide(side) !== null
                || quickViewForSide(side) !== null
    }

    function activeSurface() {
        const queue = currentOperationsQueue()
        if (queue)
            return queue
        const document = currentDocumentFrame()
        if (document)
            return document
        return shellFrame()
    }

    function isDocumentSurface(frame) {
        return frame && (frame.kind === "viewer" || frame.kind === "editor"
                         || frame.kind === "terminal")
    }

    function hasDocumentSurface() {
        const shell = currentShellFrame()
        return hasStandaloneDocumentSurface()
                || (typedShellState
                    ? typedShellState.terminalActive === true
                    : shell && shell.terminalActive === true)
    }

    function terminalActive() {
        if (typedShellState)
            return typedShellState.terminalActive === true
        const shell = shellFrame()
        return shell && shell.terminalActive === true
    }

    function hasStandaloneDocumentSurface() {
        return documentFrame !== null
    }

    function firstFrame(kind) {
        const list = frames()
        for (let index = list.length - 1; index >= 0; --index) {
            if (list[index].kind === kind)
                return list[index]
        }
        return null
    }

    function topFrame() {
        const list = frames()
        return list.length > 0 ? list[list.length - 1] : null
    }

    function needsFallbackGrid() {
        if (chromeState.presentation === "text")
            return true
        if (hasOperationsQueueSurface())
            return false
        const shell = shellFrame()
        if (shell && shell.fallback === true)
            return true
        const top = activeSurface()
        if (!top)
            return true
        return top.fallback === true || top.kind === "fallback"
                || containsFallback(top)
    }

    function fallbackReasonForNode(node) {
        if (!node)
            return ""
        const isFallback = node.fallback === true || node.kind === "fallback"
                || node.kind === "fallbackWidget"
        const ownReason = String(node.reason || "").trim()
        if (isFallback && ownReason !== "")
            return ownReason
        const children = node.children || []
        for (let index = 0; index < children.length; ++index) {
            const childReason = fallbackReasonForNode(children[index])
            if (childReason !== "")
                return childReason
        }
        return isFallback
                ? "the native QML model is unavailable for this surface" : ""
    }

    function semanticFallbackReason() {
        const shellReason = fallbackReasonForNode(shellFrame())
        if (shellReason !== "")
            return shellReason
        return fallbackReasonForNode(activeSurface())
    }

    function containsFallback(node) {
        if (!node)
            return false
        if (node.fallback === true || node.kind === "fallback"
                || node.kind === "fallbackWidget")
            return true
        const children = node.children || []
        for (let index = 0; index < children.length; ++index) {
            if (containsFallback(children[index]))
                return true
        }
        return false
    }

    function workspaceTabCanClose(tab) {
        if (!tab || tab.closable !== true)
            return false
        const queue = currentOperationsQueue()
        if (!queue)
            return true
        const queueTabId = String(queue.tabId || "")
        if (queue.hasActive === true && queueTabId !== ""
                && String(tab.id || "") === queueTabId)
            return false
        return true
    }

    function resetSceneProjection() {
        shellPresentationOverrideSet = false
        shellPresentationOverride = null
        documentPresentationOverrideSet = false
        documentPresentationOverride = null
        documentSurfaceStateOverride = null
        workspaceTabsOverride = null
        menuBarOverride = null
        keyBarOverride = null
        toastOverride = null
        leftPanelPresentationOverride = null
        rightPanelPresentationOverride = null
        leftPanelLayoutStateOverride = null
        rightPanelLayoutStateOverride = null
        panelActivationOverride = -1
        captureRetainedSurfaces()
        sceneReset()
    }

    function resetShellProjection() {
        shellPresentationOverrideSet = false
        shellPresentationOverride = null
        leftPanelPresentationOverride = null
        rightPanelPresentationOverride = null
        leftPanelLayoutStateOverride = null
        rightPanelLayoutStateOverride = null
        panelActivationOverride = -1
        captureShellSurface()
        sceneReset()
    }

    function resetDocumentProjection() {
        const previous = retainedDocumentFrame || ({})
        const wasActive = documentSurfaceActive
        documentPresentationOverrideSet = false
        documentPresentationOverride = null
        documentSurfaceStateOverride = null
        const current = currentDocumentFrame()
        captureDocumentSurface()
        // A ready row/style window is not a new shell interaction context.
        // Only opening, closing or switching documents needs focus/gallery
        // coordination; command-line/autocomplete state is unaffected.
        if (wasActive !== documentSurfaceActive
                || (current !== null
                    && (String(previous.documentKey || previous.id || "")
                        !== String(current.documentKey || current.id || "")
                        || previous.kind !== current.kind)))
            structuralSurfaceUpdated()
    }

    function resetOperationsQueueProjection() {
        captureOperationsSurface()
        sceneReset()
    }

    function applyCompactPatch(patch) {
        if (!patch)
            return
        let structuralSurfaceChanged = false
        const replaceShell = patch.replaceShell === true
        if (replaceShell) {
            // In production the typed shell store has already published the
            // replacement. The compact lane only invalidates older overrides.
            shellPresentationOverrideSet = false
            shellPresentationOverride = null
            leftPanelPresentationOverride = null
            rightPanelPresentationOverride = null
            leftPanelLayoutStateOverride = null
            rightPanelLayoutStateOverride = null
            panelActivationOverride = -1
            activePanelAcknowledged(-1)
        }
        if (patch.shellPresent === true
                && (replaceShell || currentShellFrame() === null)) {
            shellPresentationOverride = patch.shell
            shellPresentationOverrideSet = true
            captureShellSurface()
            structuralSurfaceChanged = true
        }
        if (patch.surfacePresent !== undefined) {
            // Publish the value before enabling its override. Otherwise the
            // binding briefly observes a null document, clears its native
            // viewport, then registers it again: every reply repeats the loop.
            documentPresentationOverride = patch.surfacePresent === true
                    ? patch.surface : null
            documentPresentationOverrideSet = true
            documentSurfaceStateOverride = null
            captureDocumentSurface()
            structuralSurfaceChanged = true
        }
        if (patch.surfaceState !== undefined && patch.surfaceState !== null)
            documentSurfaceStateOverride = patch.surfaceState
        const activePanel = Number(patch.activePanel)
        if (activePanel === 0 || activePanel === 1) {
            panelActivationOverride = activePanel
            activePanelAcknowledged(activePanel)
        }
        if (patch.panel !== undefined && patch.panel !== null) {
            const side = Number(patch.side)
            if (side === 0) {
                leftPanelLayoutStateOverride = null
                leftPanelPresentationOverride = patch.panel
            } else if (side === 1) {
                rightPanelLayoutStateOverride = null
                rightPanelPresentationOverride = patch.panel
            }
        }
        if (patch.panelLayoutState !== undefined
                && patch.panelLayoutState !== null) {
            const side = Number(patch.side)
            if (side === 0) {
                leftPanelLayoutStateOverride = mergePanelLayoutState(
                            leftPanelLayoutStateOverride,
                            patch.panelLayoutState)
            } else if (side === 1) {
                rightPanelLayoutStateOverride = mergePanelLayoutState(
                            rightPanelLayoutStateOverride,
                            patch.panelLayoutState)
            }
        }
        if (patch.workspaceTabs !== undefined && patch.workspaceTabs !== null)
            workspaceTabsOverride = patch.workspaceTabs
        if (patch.menuBar !== undefined && patch.menuBar !== null)
            menuBarOverride = patch.menuBar
        if (patch.keyBar !== undefined && patch.keyBar !== null)
            keyBarOverride = patch.keyBar
        if (patch.toast !== undefined && patch.toast !== null)
            toastOverride = patch.toast
        if (structuralSurfaceChanged) {
            structuralSurfaceUpdated()
        }
    }

    Component.onCompleted: resetSceneProjection()

    Connections {
        target: store.shellController
        ignoreUnknownSignals: true

        function onPanelActivationChanged(activePanel, revision) {
            store.panelActivationOverride = Number(activePanel)
            store.activePanelAcknowledged(Number(activePanel))
        }

        function onCompactPresentationChanged(patch) {
            store.applyCompactPatch(patch)
        }

    }

    Connections {
        target: store.surfaceRegistry

        function onShellChanged() { store.resetShellProjection() }
        function onDocumentChanged() { store.resetDocumentProjection() }
        function onOperationsQueueChanged() { store.resetOperationsQueueProjection() }
    }

    Connections {
        target: store.overlayState

        function onMenuBarChanged() { store.menuBarOverride = null }
        function onCommandMenusChanged() { store.commandMenusUpdated() }
    }

    Connections {
        target: store.workspaceState

        function onTabsChanged() { store.workspaceTabsOverride = null }
    }

    Connections {
        target: store.chromeState

        function onKeyBarChanged() { store.keyBarOverride = null }
        function onToastChanged() { store.toastOverride = null }
    }

}
