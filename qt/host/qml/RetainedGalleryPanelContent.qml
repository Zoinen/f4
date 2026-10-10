pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: galleryPanelContent
    required property Item panelSurface
    readonly property ApplicationWindow hostWindow: panelSurface.hostWindow
    readonly property QtObject galleryController: panelSurface.galleryController
    objectName: "galleryPanelContent-" + Number(panelSurface.panel.side || 0)
    property int currentIndex: -1
    readonly property var currentLoader: retainedViews.itemAt(currentIndex)
    readonly property var item: currentLoader ? currentLoader.item : null
    readonly property int status: currentLoader ? currentLoader.status : Loader.Null
    onItemChanged: panelSurface.updateRegisteredGalleryPanelHost()
    function activatePanel() {
        const id = String(panelSurface.panel.id || "")
        let found = -1
        for (let index = 0; index < retainedIdentities.count; ++index) {
            if (retainedIdentities.get(index).panelId === id) {
                found = index
                break
            }
        }
        if (found < 0) {
            retainedIdentities.append({ "panelId": id })
            found = retainedIdentities.count - 1
        }
        currentIndex = found
        if (currentLoader)
            currentLoader.refreshDescriptor()
        if (retainedIdentities.count > 8) {
            const evicted = currentIndex === 0 ? 1 : 0
            retainedIdentities.remove(evicted)
            if (currentIndex > evicted)
                currentIndex -= 1
        }
    }
    Component.onCompleted: activatePanel()
    Connections {
        target: panelSurface
        function onPanelChanged() { galleryPanelContent.activatePanel() }
        function onLayoutStateChanged() {
            if (galleryPanelContent.currentLoader)
                galleryPanelContent.currentLoader.refreshDescriptor()
        }
    }
    ListModel { id: retainedIdentities }
    Repeater {
        id: retainedViews
        model: retainedIdentities
        delegate: Loader {
            id: retainedLoader
            required property string panelId
            required property int index
            objectName: "retainedGalleryPanel-" + panelId
            anchors.fill: parent
            property var retainedPanel: ({})
            property var retainedLayoutState: null
            function refreshDescriptor() {
                if (String(panelSurface.panel.id || "") !== panelId)
                    return
                retainedPanel = panelSurface.panel
                retainedLayoutState = panelSurface.layoutState
            }
            Component.onCompleted: refreshDescriptor()
            active: true
            visible: panelSurface.visible && index === galleryPanelContent.currentIndex
            source: galleryController.available ? galleryController.panelComponentUrl : ""
            onLoaded: {
                refreshDescriptor()
                if (!item)
                    return
                item.side = retainedLoader.retainedPanel.side
                item.panel = Qt.binding(() => retainedLoader.retainedPanel)
                if (typeof item.layoutState !== "undefined")
                    item.layoutState = Qt.binding(() => retainedLoader.retainedLayoutState)
                if (typeof item.dropPanelSurface !== "undefined")
                    item.dropPanelSurface = panelSurface
                item.bridge = panelSurface.galleryController
                if (typeof item.iconProvider !== "undefined")
                    item.iconProvider = Qt.binding(() => hostWindow.iconProvider)
                if (typeof item.dropInputEnabled !== "undefined")
                    item.dropInputEnabled = Qt.binding(() => retainedLoader.visible && panelSurface.visible
                        && !galleryController.viewerVisible && !hostWindow.needsFallbackGrid()
                        && !hostWindow.hasDocumentSurface() && !hostWindow.hasOperationsQueueSurface()
                        && !hostWindow.hasBlockingOverlay())
                item.keySink = panelSurface.focusTarget
                item.theme = Qt.binding(() => panelSurface.galleryTheme)
                item.metrics = Qt.binding(() => panelSurface.galleryMetrics)
                if (typeof item.groupHeaderBackdropColor !== "undefined")
                    item.groupHeaderBackdropColor = Qt.binding(() => {
                        const color = hostWindow.windowBackgroundColor
                        return Qt.rgba(color.r, color.g, color.b, 1)
                    })
                item.devicePixelRatio = Qt.binding(
                    () => hostWindow.screen ? hostWindow.screen.devicePixelRatio : 1.0)
                item.defaultListDensity = Qt.binding(
                    () => hostWindow.snapPx(Math.max(22, hostWindow.ch * 1.1)))
                // Lightweight QML test embedders may supply an older panel
                // host without this optional input property.
                if (typeof item.mouseWheelMode !== "undefined")
                    item.mouseWheelMode = Qt.binding(
                        () => hostWindow.mouseWheelMode)
                if (typeof item.contentHorizontalInset !== "undefined")
                    item.contentHorizontalInset = Qt.binding(
                        () => hostWindow.panelContentSpacing)
                item.panelActive = Qt.binding(
                    () => retainedLoader.visible && panelSurface.visible
                          && panelSurface.panelIsActive
                          && !galleryController.viewerVisible
                          && !hostWindow.needsFallbackGrid()
                          && !hostWindow.hasDocumentSurface()
                          && !hostWindow.hasOperationsQueueSurface()
                          && !hostWindow.hasBlockingOverlay())
                if (typeof item.panelCursorVisible !== "undefined")
                    item.panelCursorVisible = Qt.binding(
                        () => retainedLoader.visible && panelSurface.visible
                              && panelSurface.panelIsActive
                              && !galleryController.viewerVisible
                              && !hostWindow.needsFallbackGrid()
                              && !hostWindow.hasDocumentSurface()
                              && !hostWindow.hasOperationsQueueSurface()
                              && hostWindow.overlayFrames().every(
                                  frame => frame.kind === "menu"))
                item.commandLineHasText = Qt.binding(() => {
                    var commandLine = hostWindow.commandLineFrame()
                    return hostWindow.cleanText(commandLine.text).length > 0
                })
                item.commandLineFocused = Qt.binding(
                    () => hostWindow.commandLineFrame().focused === true)
                if (typeof item.commandLineOwnsNavigation !== "undefined")
                    item.commandLineOwnsNavigation = Qt.binding(
                        () => hostWindow.commandLineFrame().ownsNavigation === true)
                item.fastFindActive = Qt.binding(
                    () => retainedLoader.retainedPanel.fastFind === true)
                if (item.panelActive)
                    item.forceActiveFocus()
                panelSurface.updateRegisteredGalleryPanelHost()
            }
        }
    }
}
