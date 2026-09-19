pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls

Item {
    id: controller
    required property ApplicationWindow hostWindow
    required property var bridge
    readonly property var preferences: bridge && bridge.quickViewPreferences !== undefined
                                       ? bridge.quickViewPreferences : null
    readonly property var shell: hostWindow.shellFrame() || ({})
    readonly property var views: shell.quickViews || []
    readonly property var view: {
        const candidate = views.length ? views[0] : ({})
        const sideVisible = Number(candidate.side) === 0 ? shell.showLeftPanel !== false : shell.showRightPanel !== false
        return shell.showPanels !== false && sideVisible && !shell.terminalActive ? candidate : ({})
    }
    readonly property var sourceHost: view.id ? hostWindow.galleryPanelHost(Number(view.sourceSide)) : null
    readonly property var sourceGallery: sourceHost ? sourceHost.galleryPanel : null
    readonly property var sourceSession: sourceHost ? sourceHost.session : null
    readonly property bool blocked: !view.id || !preferences || !preferences.values.previewOnHover
        || bridge.viewerVisible || hostWindow.queueDropdownOpen || (sourceGallery && sourceGallery.dragCursorActive) || hostWindow.hasBlockingOverlay()
        || hostWindow.hasDocumentSurface() || hostWindow.hasOperationsQueueSurface()
        || hostWindow.needsFallbackGrid() || !hostWindow.nativeTwoPanelSurfaceActive
    readonly property string viewIdentity: String(view.id || "")
    readonly property bool nativeRendering: !hostWindow.needsFallbackGrid()
    onNativeRenderingChanged: configure()
    onViewIdentityChanged: {
        requestedEntry = ""
        if (viewIdentity !== "") {
            hoverArmed = true
            Qt.callLater(hover)
        }
    }
    property string configuredShell: ""
    property string requestedEntry: ""
    property real generation: 0
    property bool hoverArmed: true
    visible: false

    function configure() {
        if (!preferences || !shell.id) return
        const nativeImages = !preferences.values.useBuiltinF4Viewer && !hostWindow.needsFallbackGrid()
        const key = shell.id + ":" + nativeImages
        if (configuredShell === key) return
        configuredShell = key
        hostWindow.action({action: "quickView.configure", target: shell.id,
            nativeImages: nativeImages}, true)
    }
    function synchronize() {
        configure()
        if (typeof bridge.synchronizeQuickView === "function")
            bridge.synchronizeQuickView(view)
    }
    function preview(entryId, index) {
        if (!view.id || !sourceSession || requestedEntry === entryId) return
        requestedEntry = entryId
        generation = Math.max(generation, Number(view.previewGeneration || 0)) + 1
        hostWindow.action({action: "quickView.preview", target: view.id,
            sourcePanelId: view.sourcePanelId, catalogRevision: sourceSession.catalogRevision,
            entryId: entryId, index: index, generation: generation}, true)
    }
    function hover() {
        const index = sourceGallery ? sourceGallery.hoveredIndex : -1
        if (blocked || !hoverArmed || index < 0 || !sourceSession) {
            preview("", -1)
            return
        }
        preview(sourceSession.entryIdAt(index), sourceSession.sourceIndexAt(index))
    }
    function keyboardNavigation() {
        hoverArmed = false
        preview("", -1)
    }
    Connections {
        target: controller.sourceHost
        ignoreUnknownSignals: true
        function onKeyboardInput() { controller.keyboardNavigation() }
    }
    onViewChanged: synchronize()
    onShellChanged: configure()
    onSourceSessionChanged: synchronize()
    onBlockedChanged: { if (blocked) { hoverArmed = false; preview("", -1) } }
    Component.onCompleted: synchronize()
    Connections {
        target: controller.preferences
        function onChanged() { controller.synchronize(); controller.hover() }
    }
    Connections {
        target: controller.sourceGallery
        function onHoveredIndexChanged() { controller.hover() }
        function onHoverPointerXChanged() { controller.hoverArmed = true; controller.hover() }
        function onHoverPointerYChanged() { controller.hoverArmed = true; controller.hover() }
    }
    Connections {
        target: controller.sourceSession
        function onCurrentIndexChanged() { controller.keyboardNavigation() }
        function onCatalogRevisionChanged() {
            controller.requestedEntry = ""
            controller.hoverArmed = false
            controller.synchronize()
        }
    }
}
