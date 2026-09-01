pragma ComponentBehavior: Bound

import QtQuick
import ZoinGallery 1.0 as ZG
import ZoinGallery.Native 1.0 as ZGN

FocusScope {
    id: host

    property int side: 0
    property var panel: ({})
    property var layoutState: null
    property var bridge: null
    property var keySink: null
    property string mouseWheelMode: "gui"
    property ZG.GalleryThemePalette theme: ZG.GalleryThemePalette {}
    property ZG.GalleryPresentationMetrics metrics:
        ZG.GalleryPresentationMetrics {}
    property var hostCapabilities: ({
        cursor: true,
        open: true,
        selection: true,
        viewer: true
    })
    property bool panelActive: false
    property bool commandLineHasText: false
    property bool fastFindActive: false
    property alias pendingCommanderInput: inputRouter.pendingCommanderInput
    property alias pendingCommanderInputTimeoutMs:
        inputRouter.pendingCommanderInputTimeoutMs
    property alias pendingPointerActivation:
        inputRouter.pendingPointerActivation
    property alias pendingPointerActivationPanelId:
        inputRouter.pendingPointerActivationPanelId
    property alias pendingPointerActivationTimeoutMs:
        inputRouter.pendingPointerActivationTimeoutMs
    property real devicePixelRatio: 1.0
    property real defaultListDensity: 22
    property bool viewerTransitionActive: false
    property string viewerTransitionEntryId: ""
    property alias forwardedKeysDown: inputRouter.forwardedKeysDown
    property alias panelSession: panelAdapter.panelSession

    readonly property alias galleryPanel: embeddedGalleryPanel
    readonly property var emptyQuickSearchMatches: ({})
    readonly property alias effectiveHostCapabilities:
        inputRouter.effectiveHostCapabilities
    readonly property alias commanderInputActive: inputRouter.commanderInputActive
    readonly property alias session: panelAdapter.panelSession
    readonly property alias benchmarkTracingEnabled:
        panelAdapter.benchmarkTracingEnabled
    readonly property alias benchmarkTraceOutputEnabled:
        panelAdapter.benchmarkTraceOutputEnabled
    readonly property alias catalogRevision: panelAdapter.catalogRevision
    readonly property alias requestedRendererState:
        panelAdapter.requestedRendererState
    readonly property alias requestedPresentationMode:
        panelAdapter.requestedPresentationMode
    readonly property alias requestedColumnCount:
        panelAdapter.requestedColumnCount
    readonly property alias densityAdjustable: panelAdapter.densityAdjustable
    readonly property alias requestedDensity: panelAdapter.requestedDensity
    readonly property alias currentDensity: panelAdapter.currentDensity
    readonly property alias minimumDensity: panelAdapter.minimumDensity
    readonly property alias maximumDensity: panelAdapter.maximumDensity
    readonly property alias densityStep: panelAdapter.densityStep
    property alias appliedPresentationMode:
        panelAdapter.appliedPresentationMode
    property alias appliedColumnSchema: panelAdapter.appliedColumnSchema
    property alias appliedRendererConfigSignature:
        panelAdapter.appliedRendererConfigSignature
    property bool applyingRendererState: false

    signal pointerActivationPreviewRequested(int side)

    // Host resource URLs terminate at this adapter boundary. ZoinGallery only
    // receives semantic icon keys plus an injected resolver.
    ZGN.GalleryIconResolver {
        id: f4GalleryIconResolver
        compactPrefix: "qrc:/F4QtHost/icons/lucide"
        largePrefix: "qrc:/F4QtHost/icons/lucide-gallery"
    }

    GalleryPanelHostAdapter {
        id: panelAdapter
        side: host.side
        panel: host.panel
        layoutState: host.layoutState
        bridge: host.bridge
        galleryPanel: embeddedGalleryPanel
        defaultListDensity: host.defaultListDensity
        onRendererTransactionStateChanged: (active) => {
            host.applyingRendererState = active
        }
    }

    GalleryPanelInputRouter {
        id: inputRouter
        side: host.side
        panel: host.panel
        adapter: panelAdapter
        galleryPanel: embeddedGalleryPanel
        bridge: host.bridge
        keySink: host.keySink
        hostCapabilities: host.hostCapabilities
        panelActive: host.panelActive
        commandLineHasText: host.commandLineHasText
        fastFindActive: host.fastFindActive
        onPointerActivationPreviewRequested: (requestedSide) => {
            host.pointerActivationPreviewRequested(requestedSide)
        }
    }

    function panelId(panelState) {
        return panelAdapter.panelId(panelState)
    }

    function legacyPanelId() {
        return "@legacy-side:" + side
    }

    function rendererStateFor(panelState) {
        return panelAdapter.rendererStateFor(panelState)
    }

    function densityIsAdjustable(mode) {
        return panelAdapter.densityIsAdjustable(mode)
    }

    function defaultDensityFor(mode) {
        return panelAdapter.defaultDensityFor(mode)
    }

    function previewDensity(value) {
        panelAdapter.previewDensity(value)
    }

    function commitDensity(value) {
        panelAdapter.commitDensity(value)
    }

    function applyRendererState() {
        panelAdapter.applyRendererState()
    }

    function currentItemImageGeometry(targetItem) {
        if (!targetItem
                || typeof embeddedGalleryPanel.currentItemImageGeometry
                        !== "function")
            return Qt.rect(0, 0, 0, 0)
        return embeddedGalleryPanel.currentItemImageGeometry(targetItem)
    }

    function currentItemImageSource() {
        if (typeof embeddedGalleryPanel.currentItemImageSource !== "function")
            return ""
        return embeddedGalleryPanel.currentItemImageSource()
    }

    function beginPendingPointerActivation(semanticPanelId) {
        inputRouter.beginPendingPointerActivation(semanticPanelId)
    }

    function finishPendingPointerActivation() {
        inputRouter.finishPendingPointerActivation()
    }

    function forwardConsoleWheel(x, y, angleDeltaY, modifiers) {
        inputRouter.forwardConsoleWheel(x, y, angleDeltaY, modifiers)
    }

    function forwardConsoleMouseButton(x, y, button, down, modifiers) {
        inputRouter.forwardConsoleMouseButton(
                    x, y, button, down, modifiers)
    }

    Keys.priority: Keys.BeforeItem
    Keys.onPressed: (event) => inputRouter.handlePressed(event)
    Keys.onReleased: (event) => inputRouter.handleReleased(event)

    onPanelChanged: {
        inputRouter.reconcilePanelIdentity(host.panel)
        panelAdapter.synchronizePanel(host.panel, host.layoutState)
    }
    onLayoutStateChanged: {
        panelAdapter.synchronizeLayout(host.panel, host.layoutState)
    }
    onDefaultListDensityChanged: panelAdapter.synchronizeLayout(
                                     host.panel, host.layoutState)
    onPanelActiveChanged: {
        if (host.panelActive)
            inputRouter.finishPendingPointerActivation()
        else
            inputRouter.finishPendingCommanderInput()
    }
    onCommandLineHasTextChanged: {
        if (host.commandLineHasText)
            inputRouter.acknowledgePendingCommanderInput()
    }
    onFastFindActiveChanged: {
        if (host.fastFindActive)
            inputRouter.acknowledgePendingCommanderInput()
    }

    Component.onCompleted: {
        panelAdapter.refreshPanelSession(host.panel)
        panelAdapter.synchronizeLayout(host.panel, host.layoutState)
    }

    ZG.GalleryPanel {
        id: embeddedGalleryPanel
        objectName: "embeddedGalleryPanel"
        anchors.fill: parent
        session: host.session
        iconResolver: f4GalleryIconResolver
        presentationDensities: ({})
        theme: host.theme
        metrics: host.metrics
        animateLayoutChanges: false
        emptyStateEnabled: host.session !== null
                           && host.panel.loading !== true
                           && host.panel.catalogProvisional !== true
        emptyStateText: qsTr("Folder is empty")
        scrollBarsReady: host.panel.catalogProvisional !== true
        quickSearchMatches: host.panel.fastFind === true
                            ? (host.panel.fastFindMatches
                               || host.emptyQuickSearchMatches)
                            : host.emptyQuickSearchMatches
        quickSearchMatchColor: {
            const supplied = String(host.panel.fastFindMatchColor || "")
            return supplied !== "" ? supplied : host.theme.quickSearchMatch
        }
        showDetailsHeader: false
        hostCapabilities: host.effectiveHostCapabilities
        devicePixelRatio: host.devicePixelRatio
        viewerTransitionActive: host.viewerTransitionActive
        viewerTransitionEntryId: host.viewerTransitionEntryId
        showCursor: host.panelActive
                    || (host.pendingPointerActivation
                        && host.pendingPointerActivationPanelId
                           === panelAdapter.panelId(host.panel))
        focus: true
        autoFocus: false
        mouseWheelMode: host.mouseWheelMode
        benchmarkTracingEnabled: host.benchmarkTracingEnabled

        onActivateRequested: {
            if (host.bridge && !host.panelActive)
                host.bridge.requestActivate(host.side)
        }
        onCursorRequested: (entryId, index, deferCommit) => {
            if (!host.bridge)
                return
            if (!host.panelActive) {
                inputRouter.beginPendingPointerActivation(
                            panelAdapter.panelId(host.panel))
                inputRouter.pointerActivationPreviewRequested(host.side)
            }
            const revision = Number(host.panel.catalogRevision || 0)
            host.bridge.requestCursor(
                        host.side, entryId, index, revision,
                        deferCommit,
                        deferCommit
                        && (embeddedGalleryPanel.keyboardShiftSelectionActive
                            || embeddedGalleryPanel.keyboardToggleSelectionActive))
        }
        onOpenRequested: (entryId, index, isImage, autoRepeat) => {
            if (host.bridge) {
                host.bridge.requestOpen(
                            host.side, entryId, index, isImage,
                            Number(host.panel.catalogRevision || 0), autoRepeat)
            }
        }
        onSelectionRequested: (mode, entryIds) => {
            if (host.bridge) {
                host.bridge.requestSelection(
                            host.side, mode, entryIds,
                            Number(host.panel.catalogRevision || 0))
            }
        }
        onSelectionTransactionRequested: (changes, cursorEntryId,
                                           cursorIndex) => {
            if (host.bridge) {
                host.bridge.requestSelectionTransaction(
                            host.side, changes, cursorEntryId, cursorIndex,
                            Number(host.panel.catalogRevision || 0))
            }
        }
        onMetadataVisibleRangeChanged: (firstRow, lastRow) => {
            if (host.bridge) {
                host.bridge.reportMetadataVisibleRange(
                            host.side, firstRow, lastRow,
                            Number(host.panel.catalogRevision || 0))
            }
        }
        onBenchmarkStage: (stage, metadata) => {
            panelAdapter.forwardBenchmarkStage(stage, metadata)
        }
        onConsoleWheelRequested: (x, y, angleDeltaY, modifiers) => {
            inputRouter.forwardConsoleWheel(x, y, angleDeltaY, modifiers)
        }
        onConsoleMouseButtonRequested: (x, y, button, down, modifiers) => {
            inputRouter.forwardConsoleMouseButton(
                        x, y, button, down, modifiers)
        }
    }
}
