pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import ZoinGallery 1.0 as ZG
import QWindowKit 1.0

Item {
    id: surfaces

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

    readonly property alias titleBarItem: titleBar
    readonly property alias menuBar: titleBar.menuBar
    readonly property alias appIconButton: titleBar.appIconButton
    readonly property alias workspaceBarItem: titleBar.workspaceBarItem
    readonly property alias macSystemButtonAreaItem:
        titleBar.macSystemButtonAreaItem
    readonly property alias galleryViewerLoader: galleryViewerLayer
    readonly property alias operationsQueueLoader: operationsQueueLayer
    readonly property alias overlayController: overlayHost

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
        z: 20
    }

    Item {
        anchors.fill: parent
        opacity: surfaces.hostWindow.normalSurfaceOpacity

        Loader {
            objectName: "persistentPanelsLayer"
            anchors.fill: parent
            active: surfaces.hostWindow.retainedShellSurfaceCreated
            visible: !surfaces.hostWindow.hasOperationsQueueSurface()
            opacity: surfaces.hostWindow.hasStandaloneDocumentSurface() ? 0 : 1
            sourceComponent: PanelsSurface {
                hostWindow: surfaces.hostWindow
                menuBar: surfaces.menuBar
                focusTarget: surfaces.focusTarget
                galleryController: surfaces.galleryController
                galleryTheme: surfaces.galleryTheme
                galleryMetrics: surfaces.galleryMetrics
            }
        }

        Loader {
            objectName: "persistentDocumentLayer"
            anchors.fill: parent
            active: surfaces.hostWindow.retainedDocumentSurfaceCreated
                    || surfaces.hostWindow.documentSurfacePrewarmed
            visible: surfaces.hostWindow.hasStandaloneDocumentSurface()
            sourceComponent: DocumentSurface {
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

        Loader {
            id: operationsQueueLayer
            objectName: "operationsQueueLayer"
            anchors.fill: parent
            active: surfaces.hostWindow.retainedOperationsQueueCreated
            visible: surfaces.hostWindow.hasOperationsQueueSurface()
            sourceComponent: OperationsQueueSurface {
                hostWindow: surfaces.hostWindow
                menuBar: surfaces.menuBar
                queue: surfaces.hostWindow.operationsQueueFrame()
                interactionActive:
                    surfaces.hostWindow.hasOperationsQueueSurface()
            }
            z: 20
        }
    }

    Loader {
        id: galleryViewerLayer
        anchors.fill: parent
        active: surfaces.galleryController.viewerVisible
                && !surfaces.hostWindow.hasDocumentSurface()
                && !surfaces.hostWindow.needsFallbackGrid()
        visible: active && !surfaces.hostWindow.hasOperationsQueueSurface()
        sourceComponent: active ? galleryViewerSurface : undefined
        z: 60
    }

    Component {
        id: galleryViewerSurface

        Loader {
            anchors.fill: parent
            source: surfaces.galleryController.viewerComponentUrl

            onLoaded: {
                if (!item)
                    return
                item.session = Qt.binding(
                            () => surfaces.galleryController.viewerSession)
                item.sourcePanel = Qt.binding(
                            () => surfaces.hostWindow.galleryPanelHost(
                                surfaces.galleryController.viewerSide))
                item.bridge = surfaces.galleryController
                item.keySink = surfaces.focusTarget
                item.theme = surfaces.galleryTheme
                item.surfaceActive = Qt.binding(
                            () => surfaces.galleryController.viewerVisible
                              && !surfaces.hostWindow.hasBlockingOverlay()
                              && !surfaces.hostWindow.hasDocumentSurface()
                              && !surfaces.hostWindow.hasOperationsQueueSurface()
                              && !surfaces.hostWindow.needsFallbackGrid())
                item.devicePixelRatio = Qt.binding(
                            () => surfaces.hostWindow.screen
                                  ? surfaces.hostWindow.screen.devicePixelRatio
                                  : 1.0)
                item.forceActiveFocus()
            }
        }
    }

    OverlayHost {
        id: overlayHost
        hostWindow: surfaces.hostWindow
        menuBar: surfaces.menuBar
        semanticLayer: surfaces
        shellController: surfaces.shellController
        frames: surfaces.hostWindow.overlayFrames()
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
        y: surfaces.menuBar.height + 8
        opacity: surfaces.hostWindow.normalSurfaceOpacity
        z: 200

        onDismissRequested: surfaces.hostWindow.action({
            "action": "toast.dismiss",
            "target": "toast"
        })
    }
}
