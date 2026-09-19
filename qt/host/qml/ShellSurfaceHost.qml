pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import ZoinGallery 1.0 as ZG
import QWindowKit 1.0

Item {
    id: surfaces

    function cancelDocumentWindowIntentForEdgeNavigation() {
        const document = documentLayer.item
        if (document && document.interactionActive
                && !surfaces.hostWindow.hasBlockingOverlay()
                && !surfaces.hostWindow.queueDropdownOpen
                && !surfaces.hostWindow.hasOperationsQueueSurface()
                && (document.frame.kind === "viewer"
                    || document.frame.kind === "editor"))
            document.cancelPendingWindowIntent()
    }

    required property ApplicationWindow hostWindow
    required property WindowAgent nativeWindowAgent
    required property bool nativeWindowAgentReady
    required property Item focusTarget
    required property var shellController
    required property var galleryController
    required property Window themeEditor
    required property ZG.GalleryThemePalette galleryTheme
    required property ZG.GalleryPresentationMetrics galleryMetrics
    property bool usesQwk: false
    readonly property bool fullscreenGallery: surfaces.galleryController.viewerVisible && galleryViewerLayer.visible
        && surfaces.hostWindow.visibility === Window.FullScreen

    readonly property alias titleBarItem: titleBar
    readonly property alias menuBar: semanticMenu
    readonly property alias appIconButton: titleBar.appIconButton
    readonly property alias workspaceBarItem: titleBar.workspaceBarItem
    readonly property alias macSystemButtonAreaItem:
        titleBar.macSystemButtonAreaItem
    readonly property alias galleryViewerLoader: galleryViewerLayer
    readonly property alias operationsQueueLoader: operationsQueueLayer
    readonly property alias overlayController: overlayHost

    QuickViewController {
        id: quickViewController
        hostWindow: surfaces.hostWindow
        bridge: surfaces.galleryController
    }

    Rectangle {
        anchors.fill: parent
        color: surfaces.hostWindow.useTransparentWindowBackground
               ? surfaces.hostWindow.windowBackgroundColor : "transparent"
    }

    ShellTitleBar {
        id: titleBar
        hostWindow: surfaces.hostWindow
        semanticLayer: surfaces
        nativeWindowAgent: surfaces.nativeWindowAgent
        nativeWindowAgentReady: surfaces.nativeWindowAgentReady
        usesQwk: surfaces.usesQwk
        themeEditor: surfaces.themeEditor
        anchors.left: parent.left
        anchors.right: parent.right
        height: surfaces.hostWindow.menuBarHeight
        visible: !surfaces.fullscreenGallery
        z: 20
        onApplicationMenuRequested: applicationMenu.popup(titleBar.appIconButton, 0,
                                                        titleBar.appIconButton.height)
    }

    SemanticMenuBar {
        id: semanticMenu
        objectName: "panelMenuBar"
        hostWindow: surfaces.hostWindow
        semanticLayer: surfaces
        nativeWindowAgent: surfaces.nativeWindowAgent
        nativeWindowAgentReady: surfaces.nativeWindowAgentReady
        usesQwk: false
        menu: surfaces.hostWindow.menuBarModel
        visible: menu.active === true && (menu.items || []).length > 0
        anchors.left: parent.left
        anchors.right: parent.right
        y: surfaces.hostWindow.menuBarHeight
        height: surfaces.hostWindow.panelPathRowHeight
        // Match the path row's translucent foreground over the title surface.
        color: surfaces.hostWindow.titleBarBg
        overlayColor: surfaces.hostWindow.panelPathBg
        z: 90

        Rectangle {
            objectName: "panelMenuBarSeparator"
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: surfaces.hostWindow.separatorWidth
            color: surfaces.hostWindow.separatorColor
        }
    }

    ApplicationMenuPopup {
        id: applicationMenu
        hostWindow: surfaces.hostWindow
    }

    Item {
        anchors.fill: parent
        opacity: surfaces.hostWindow.normalSurfaceOpacity

        Loader {
            id: panelsLayer
            objectName: "persistentPanelsLayer"
            anchors.fill: parent
            active: surfaces.hostWindow.retainedShellSurfaceCreated
            visible: !surfaces.hostWindow.hasOperationsQueueSurface()
            opacity: surfaces.hostWindow.hasStandaloneDocumentSurface() ? 0 : 1
            sourceComponent: PanelsSurface {
                enabled: !surfaces.hostWindow.queueDropdownOpen
                hostWindow: surfaces.hostWindow
                menuBar: surfaces.menuBar
                focusTarget: surfaces.focusTarget
                galleryController: surfaces.galleryController
                galleryTheme: surfaces.galleryTheme
                galleryMetrics: surfaces.galleryMetrics
            }
        }

        Loader {
            id: documentLayer
            objectName: "persistentDocumentLayer"
            anchors.fill: parent
            active: surfaces.hostWindow.retainedDocumentSurfaceCreated
                    || surfaces.hostWindow.documentSurfacePrewarmed
            visible: surfaces.hostWindow.hasStandaloneDocumentSurface()
            sourceComponent: DocumentSurface {
                enabled: !surfaces.hostWindow.queueDropdownOpen
                hostWindow: surfaces.hostWindow
                menuBar: surfaces.menuBar
                // The store publishes each non-null document here. Closing
                // changes visibility only: rebinding the hidden document to
                // an equivalent fallback re-evaluates every text run on Esc.
                frame: surfaces.hostWindow.retainedDocumentFrame
                interactionActive:
                    surfaces.hostWindow.hasStandaloneDocumentSurface()
                    && !surfaces.hostWindow.needsFallbackGrid()
            }
            z: 10
        }

        Timer {
            interval: 0
            running: surfaces.hostWindow.retainedShellSurfaceCreated
                     && !surfaces.hostWindow.documentSurfacePrewarmed
            onTriggered: surfaces.hostWindow.documentSurfacePrewarmed = true
        }

    }

    Popup {
        id: queueDropdown
        objectName: "operationsQueueDropdown"
        parent: Overlay.overlay
        x: surfaces.hostWindow.snapPx(Math.max(8, parent.width - width - 8))
        y: surfaces.hostWindow.menuBarHeight
        width: surfaces.hostWindow.snapPx(Math.min(880, parent.width - 16))
        height: surfaces.hostWindow.snapPx(Math.min(520, parent.height - y - 16,
            160 + Math.max(1,(surfaces.hostWindow.operationsQueueFrame().items || []).length) * 60))
        padding: surfaces.hostWindow.snapPx(1)
        modal: true
        dim: false
        focus: true
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutside
        Shortcut {
            sequence: "Escape"
            enabled: queueDropdown.visible
            onActivated: surfaces.hostWindow.queueDropdownOpen = false
        }
        visible: surfaces.hostWindow.nativeQueueDropdownEnabled && surfaces.hostWindow.queueDropdownOpen && !surfaces.hostWindow.hasBlockingOverlay()
        // Synchronize at the start of dismissal. A delayed closed signal can
        // otherwise clear a newer open request from the title-bar button.
        onAboutToHide: if (!surfaces.hostWindow.hasBlockingOverlay()) surfaces.hostWindow.queueDropdownOpen = false
        background: Rectangle {
            color: surfaces.hostWindow.windowBackgroundColor
            border.color: surfaces.hostWindow.separatorColor
            radius: surfaces.hostWindow.snapPx(8)
        }
        contentItem: Loader {
            id: operationsQueueLayer
            objectName: "operationsQueueLayer"
            active: surfaces.hostWindow.retainedOperationsQueueCreated || surfaces.hostWindow.queueDropdownOpen
            sourceComponent: OperationsQueueSurface {
                hostWindow: surfaces.hostWindow
                menuBar: surfaces.menuBar
                queue: surfaces.hostWindow.operationsQueueFrame()
                dropdown: true
                interactionActive: surfaces.hostWindow.queueDropdownOpen
            }
        }
    }

    Loader {
        id: galleryViewerLayer
        objectName: "galleryViewerLayer"
        readonly property int presentationState: surfaces.galleryController.viewerState === undefined ? 3 : surfaces.galleryController.viewerState
        readonly property int dockSide: surfaces.galleryController.quickViewSide === undefined ? -1 : surfaces.galleryController.quickViewSide
        property real fullProgress: 1
        function updatePresentation() {
            const target = dockSide < 0 || presentationState === 2 || presentationState === 3 ? 1 : 0
            presentationAnimation.stop()
            if (dockSide >= 0 && (presentationState === 2 || presentationState === 4)) {
                presentationAnimation.to = target
                presentationAnimation.start()
            } else {
                fullProgress = target
            }
        }
        onPresentationStateChanged: updatePresentation()
        onDockSideChanged: updatePresentation()
        Component.onCompleted: updatePresentation()
        NumberAnimation {
            id: presentationAnimation
            target: galleryViewerLayer
            property: "fullProgress"
            duration: 150
            onFinished: {
                surfaces.galleryController.settleViewer()
                if (galleryViewerLayer.presentationState === 1 && galleryViewerLayer.item && galleryViewerLayer.item.item)
                    galleryViewerLayer.item.item.focusSource()
            }
        }
        readonly property real dockX: dockSide < 0 ? 0 : surfaces.hostWindow.nativePanelX(dockSide)
        readonly property real dockY: surfaces.hostWindow.menuBarHeight
        readonly property real dockWidth: dockSide < 0 ? parent.width : surfaces.hostWindow.nativePanelWidth(dockSide)
        readonly property real dockHeight: dockSide < 0 ? parent.height - dockY : surfaces.hostWindow.nativePanelHeight(dockSide, dockY)
        readonly property real fullY: surfaces.fullscreenGallery ? 0 : titleBar.height
        x: surfaces.hostWindow.snapPx(dockX * (1 - fullProgress))
        y: surfaces.hostWindow.snapPx(dockY + (fullY - dockY) * fullProgress)
        width: surfaces.hostWindow.snapPx(dockWidth + (parent.width - dockWidth) * fullProgress)
        height: surfaces.hostWindow.snapPx(dockHeight + (parent.height - fullY - dockHeight) * fullProgress)
        clip: true
        active: (surfaces.galleryController.viewerMounted === undefined ? surfaces.galleryController.viewerVisible : surfaces.galleryController.viewerMounted)
                && !surfaces.hostWindow.hasDocumentSurface()
                && !surfaces.hostWindow.needsFallbackGrid()
        visible: active && !surfaces.hostWindow.hasOperationsQueueSurface()
        sourceComponent: active ? galleryViewerSurface : undefined
        z: 60
    }

    // Docked Quick View sits above the panels, including their overlapping
    // gutter. Keep the divider at shell level so it owns that pointer region.
    PanelSplitter {
        objectName: "mainPanelSplitter"
        devicePixelRatio: surfaces.hostWindow.dpr
        x: surfaces.hostWindow.snapPx(splitPosition - width / 2)
        y: surfaces.hostWindow.menuBarHeight
        height: Math.max(surfaces.hostWindow.nativePanelHeight(0, y),
                         surfaces.hostWindow.nativePanelHeight(1, y))
        availableWidth: parent.width
        minimumPanelWidth: surfaces.hostWindow.panelMinimumWidth
        ratio: surfaces.hostWindow.panelSplitRatio
        defaultRatio: 0.5
        keySink: surfaces.focusTarget
        surfaceActive: panelsLayer.item !== null && surfaces.hostWindow.nativeTwoPanelSurfaceActive
                       && !surfaces.hostWindow.queueDropdownOpen
                       && surfaces.hostWindow.widePanelSide() < 0
                       && panelsLayer.item.hasPanelForSide(0)
                       && panelsLayer.item.hasPanelForSide(1)
        surfaceVisible: panelsLayer.item !== null && surfaces.hostWindow.nativeTwoPanelSurfaceVisible
                        && surfaces.hostWindow.widePanelSide() < 0
                        && panelsLayer.item.hasPanelForSide(0)
                        && panelsLayer.item.hasPanelForSide(1)
        hoverLineColor: surfaces.hostWindow.separatorHoverColor
        activeLineColor: surfaces.hostWindow.separatorActiveColor
        trackColor: "transparent"
        separatorColor: surfaces.hostWindow.separatorColor
        separatorWidth: surfaces.hostWindow.separatorWidth
        gutterWidth: surfaces.hostWindow.panelContentSpacing * 2
        leadingHitInset: surfaces.hostWindow.panelContentSpacing
        opacity: surfaces.hostWindow.normalSurfaceOpacity
        z: 61

        onRatioRequested: (nextRatio) => {
            surfaces.hostWindow.panelSplitRatio = nextRatio
        }
        onFocusReleaseRequested: {
            Qt.callLater(surfaces.hostWindow.restoreSurfaceFocus)
        }
    }

    Component {
        id: galleryViewerSurface

        Loader {
            anchors.fill: parent
            source: surfaces.galleryController.viewerComponentUrl

            onLoaded: {
                if (!item)
                    return
                // Cached sessions publish image dimensions synchronously.
                // Set the screen scale before converting them to logical units.
                item.devicePixelRatio = Qt.binding(
                            () => surfaces.hostWindow.screen
                                  ? surfaces.hostWindow.screen.devicePixelRatio
                                  : 1.0)
                item.session = Qt.binding(
                            () => surfaces.galleryController.viewerSession)
                item.sourcePanel = Qt.binding(
                            () => surfaces.hostWindow.galleryPanelHost(
                                surfaces.galleryController.viewerSide))
                if (item.hostWindow !== undefined) item.hostWindow = surfaces.hostWindow
                if (item.fullViewProgress !== undefined) item.fullViewProgress = Qt.binding(() => galleryViewerLayer.fullProgress)
                item.bridge = surfaces.galleryController
                item.keySink = surfaces.focusTarget
                item.theme = surfaces.galleryTheme
                item.surfaceActive = Qt.binding(
                            () => (surfaces.galleryController.viewerVisible
                                   || (surfaces.galleryController.quickView && surfaces.galleryController.quickView.active === true))
                              && !surfaces.hostWindow.hasBlockingOverlay()
                              && !surfaces.hostWindow.hasDocumentSurface()
                              && !surfaces.hostWindow.hasOperationsQueueSurface()
                              && !surfaces.hostWindow.needsFallbackGrid())
                if (item.surfaceActive) item.forceActiveFocus()
            }
        }
    }

    OverlayHost {
        id: overlayHost
        hostWindow: surfaces.hostWindow
        menuBar: surfaces.menuBar
        semanticLayer: surfaces
        shellController: surfaces.shellController
        // overlayFrames() is a helper function, so make its stream revisions
        // an explicit binding dependency. This is important when a popup is
        // closed in the fallback/console presentation and the native surface
        // becomes visible again: the overlay model must not resurrect that
        // old frame on the next GUI presentation change.
        frames: {
            const sceneStore = surfaces.hostWindow.sceneStoreApi
            // Keep the payload properties as direct dependencies as well.
            // A legacy peer can deliver a cross-stream overlay payload at the
            // current owner revision; the helper revision string then keeps
            // the same value even though its frame list changed.
            const overlayState = sceneStore ? sceneStore.overlayState : null
            const menuPayload = overlayState
                    ? overlayState.commandMenus : []
            const dialogPayload = overlayState ? overlayState.dialogs : []
            if (surfaces.hostWindow.overlayFramesRevision === "")
                return []
            // The reads above intentionally participate in this binding's
            // dependency graph; frames() remains the single projection path.
            void menuPayload
            void dialogPayload
            return surfaces.hostWindow.overlayFrames()
        }
        anchors.fill: parent
        z: 100
    }

    KeyBarView {
        hostWindow: surfaces.hostWindow
        shellController: surfaces.shellController
        focusTarget: surfaces.focusTarget
        keyBar: surfaces.hostWindow.keyBarModel
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: surfaces.hostWindow.keyBarHeight()
        opacity: surfaces.hostWindow.normalSurfaceOpacity
        z: 40
    }

    ToastView {
        hostWindow: surfaces.hostWindow
        toast: surfaces.hostWindow.toastModel
        anchors.horizontalCenter: parent.horizontalCenter
        y: surfaces.hostWindow.menuBarHeight + 8
        opacity: surfaces.hostWindow.normalSurfaceOpacity
        z: 200

        onDismissRequested: surfaces.hostWindow.action({
            "action": "toast.dismiss",
            "target": "toast"
        })
    }
}
