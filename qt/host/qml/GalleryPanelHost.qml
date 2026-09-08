pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls.Basic as T
import ZoinGallery 1.0 as ZG
import ZoinGallery.Native 1.0 as ZGN

FocusScope {
    id: host
    property bool dropInputEnabled: true
    property Item dropPanelSurface: null
    property bool dropTabHover: false
    property int dropHoverIndex: -2
    property point dropPointer: Qt.point(0, 0)
    // Shared scene-space bounds for painting and hit testing, including gutters.
    readonly property rect wholeDropSceneRect: {
        const layout = embeddedGalleryPanel.galleryLayout
        const surface = host.dropPanelSurface || host
        const top = layout.mapToItem(null, 0, 0)
        const left = host.dropPanelSurface ? surface.mapToItem(null, 0, 0).x : top.x
        const right = host.dropPanelSurface
            ? surface.mapToItem(null, surface.width, 0).x : top.x + layout.width
        const dpr = host.devicePixelRatio
        const x = Math.round(left * dpr) / dpr
        const y = Math.round(top.y * dpr) / dpr
        return Qt.rect(x, y, Math.round(right * dpr) / dpr - x,
                       Math.round((top.y + layout.height) * dpr) / dpr - y)
    }
    function dragHit(x, y) {
        const layout = embeddedGalleryPanel.galleryLayout
        const p = layout.mapFromItem(host, x, y)
        const scene = host.mapToItem(null, x, y)
        const bounds = host.wholeDropSceneRect
        if (scene.x < bounds.x || scene.y < bounds.y
                || scene.x >= bounds.x + bounds.width || scene.y >= bounds.y + bounds.height)
            return { valid: false }
        dropPointer = p
        const inViewport = p.x >= 0 && p.y >= 0 && p.x < layout.width && p.y < layout.height
        const index = inViewport ? layout.indexAtViewport(p.x, p.y) : -1
        return { valid: true, index: index < 0 ? -1
                 : embeddedGalleryPanel.controller.sourceIndexAt(index) }
    }
    function endNativeDragPointer() { embeddedGalleryPanel.endPointerDrag() }
    function registerDragPanel() {
        if (bridge && typeof bridge.registerDragPanel === "function")
            bridge.registerDragPanel(side, host)
    }
    onBridgeChanged: registerDragPanel()
    onSideChanged: registerDragPanel()

    // Only this new outline is painted. Its edges are snapped in scene space;
    // existing text and icons retain their original transforms.
    Rectangle {
        id: dropOutline
        parent: host.dropPanelSurface || host
        objectName: "panelDropOutline-" + host.side
        z: 100
        visible: host.dropInputEnabled && host.dropHoverIndex !== -2
        color: "transparent"
        border.color: host.theme.selection
        border.width: 2 / host.devicePixelRatio
        readonly property rect targetRect: {
            if (host.dropHoverIndex < 0) {
                const bounds = host.wholeDropSceneRect
                const a = dropOutline.parent.mapFromItem(null, bounds.x, bounds.y)
                const b = dropOutline.parent.mapFromItem(null, bounds.x + bounds.width, bounds.y + bounds.height)
                return Qt.rect(a.x, a.y, b.x - a.x, b.y - a.y)
            }
            const layout = embeddedGalleryPanel.galleryLayout
            const revision = layout.layoutRevision
            const contentY = layout.contentY
            let r = Qt.rect(0, 0, layout.width, layout.height)
            if (host.dropHoverIndex >= 0) {
                for (const idx of layout.visibleIndexes) {
                    if (embeddedGalleryPanel.controller.sourceIndexAt(idx) === host.dropHoverIndex) {
                        const g = layout.indexGeometry(idx)
                        r = Qt.rect(g.x, g.y - contentY, g.width, g.height)
                        break
                    }
                }
            }
            const right = Math.min(layout.width, r.x + r.width)
            const bottom = Math.min(layout.height, r.y + r.height)
            r = Qt.rect(Math.max(0, r.x), Math.max(0, r.y),
                        Math.max(0, right - Math.max(0, r.x)),
                        Math.max(0, bottom - Math.max(0, r.y)))
            let p = layout.mapToItem(dropOutline.parent, r.x, r.y)
            const scene = dropOutline.parent.mapToItem(null, p.x, p.y)
            const dpr = host.devicePixelRatio
            const a = dropOutline.parent.mapFromItem(null, Math.round(scene.x * dpr) / dpr,
                                       Math.round(scene.y * dpr) / dpr)
            const end = dropOutline.parent.mapFromItem(null,
                Math.round((scene.x + r.width) * dpr) / dpr,
                Math.round((scene.y + r.height) * dpr) / dpr)
            return Qt.rect(a.x, a.y, end.x - a.x, end.y - a.y)
        }
        x: targetRect.x
        y: targetRect.y
        width: targetRect.width
        height: targetRect.height
    }
    Timer {
        interval: 60
        repeat: true
        running: host.dropInputEnabled && !host.dropTabHover && host.dropHoverIndex !== -2
        onTriggered: {
            const layout = embeddedGalleryPanel.galleryLayout
            const y = host.dropPointer.y
            const direction = y < 28 ? -1 : y > layout.height - 28 ? 1 : 0
            if (direction) {
                layout.contentY = Math.max(0, Math.min(layout.contentHeight - layout.height,
                                                      layout.contentY + direction * 18))
                const index = layout.indexAtViewport(host.dropPointer.x, y)
                host.dropHoverIndex = index >= 0 && host.session.isDirectoryAt(index)
                    ? embeddedGalleryPanel.controller.sourceIndexAt(index) : -1
            }
        }
    }

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

    function currentItemCaption() {
        return currentItemDecoration("galleryMasonryLabel-")
    }

    function currentItemSelectionSurface() {
        return currentItemDecoration("gallerySelectionSurface-")
    }

    function currentItemDecoration(prefix) {
        const entry = embeddedGalleryPanel.currentTransitionItem()
        function findLabel(item) {
            if (!item)
                return null
            if (String(item.objectName).startsWith(prefix))
                return item
            for (const child of item.children) {
                const label = findLabel(child)
                if (label)
                    return label
            }
            return null
        }
        return findLabel(entry)
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
        registerDragPanel()
        panelAdapter.refreshPanelSession(host.panel)
        panelAdapter.synchronizeLayout(host.panel, host.layoutState)
    }

    // Read the complete name from the native catalog, independent of the
    // console's horizontal name offset and the current gallery presentation.
    readonly property string hoveredEntryName: session !== null
        && embeddedGalleryPanel.hoveredIndex >= 0
        ? session.entryNameAt(embeddedGalleryPanel.hoveredIndex) : ""
    TextMetrics {
        id: fullNameMetrics
        text: host.hoveredEntryName
        font: fullNameTip.font
    }
    T.ToolTip {
        id: fullNameTip
        objectName: "galleryFullNameTooltip-" + host.side
        parent: host
        visible: false // Full-name hover tooltips are disabled for now.
        delay: 650
        timeout: 6000
        text: host.hoveredEntryName
        x: Math.min(host.width - width, Math.max(0, embeddedGalleryPanel.hoverPointerX))
        y: Math.max(0, Math.min(host.height - height, embeddedGalleryPanel.hoverPointerY + 20))
        padding: 8
        width: Math.round(Math.min(host.width, 480, fullNameMetrics.advanceWidth + 2 * padding)
                          * host.devicePixelRatio) / host.devicePixelRatio
        function pixelOffset(item, horizontal) {
            const revision = x + y + host.x + host.y + width + height
            const origin = item.parent.mapToItem(null, item.x, item.y)
            const coordinate = horizontal ? origin.x : origin.y
            const dpr = host.devicePixelRatio > 0 ? host.devicePixelRatio : 1
            return Math.round(coordinate * dpr) / dpr - coordinate
        }
        background: Rectangle {
            color: host.theme.dialogBackground
            border.color: host.theme.separator
            border.width: 1 / host.devicePixelRatio
            radius: 4
        }
        contentItem: Text {
            id: fullNameText
            objectName: "galleryFullNameText-" + host.side
            text: fullNameTip.text
            textFormat: Text.PlainText
            font: fullNameTip.font
            color: host.theme.text
            wrapMode: Text.WrapAnywhere
            transform: Translate {
                x: fullNameTip.pixelOffset(fullNameText, true)
                y: fullNameTip.pixelOffset(fullNameText, false)
            }
        }
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
