pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Window
import ZoinGallery 1.0 as ZG

FocusScope {
    id: host

    property var session: null
    // The persistent panel host remains loaded below the full-area viewer and
    // supplies the thumbnail geometry/source for both halves of the shared
    // expand/collapse transition.
    property var sourcePanel: null
    property var bridge: null
    // Kept for source compatibility with older host loaders.  The full-area
    // viewer never calls this sink: its keyboard surface is modal.
    property var keySink: null
    property ZG.GalleryThemePalette theme: ZG.GalleryThemePalette {}
    // The host keeps this Loader alive beneath commander dialogs. Reacquire
    // focus when the viewer becomes the top input surface again.
    property bool surfaceActive: true
    property var hostCapabilities: ({
        cursor: true,
        open: true,
        selection: true,
        viewer: true
    })
    property real devicePixelRatio: 1.0
    property int nonFullscreenVisibility: Window.Windowed
    // Expose the viewer's exact animation/gesture progress so the embedding
    // shell can fade its chrome in lockstep with the image transition.
    readonly property real surfaceProgress: galleryViewer.surfaceProgress

    onSurfaceActiveChanged: {
        if (surfaceActive)
            forceActiveFocus()
    }

    function toggleFullscreen() {
        const targetWindow = host.Window.window
        if (!targetWindow)
            return
        if (targetWindow.visibility === Window.FullScreen) {
            const restoreVisibility =
                    host.nonFullscreenVisibility === Window.Maximized
                    ? Window.Maximized : Window.Windowed
            targetWindow.visibility = restoreVisibility
        } else {
            host.nonFullscreenVisibility =
                    targetWindow.visibility === Window.Maximized
                    ? Window.Maximized : Window.Windowed
            targetWindow.visibility = Window.FullScreen
        }
        Qt.callLater(() => {
            if (host.surfaceActive)
                host.forceActiveFocus()
        })
    }

    // GalleryViewer dispatches its local key map first.  This parent handler is
    // the safety net for an older module or a future unhandled key: no press or
    // release may bubble into f4 while the full-area viewer owns the surface.
    Keys.priority: Keys.AfterItem
    Keys.onPressed: event => { event.accepted = true }
    Keys.onReleased: event => { event.accepted = true }

    ZG.GalleryViewer {
        id: galleryViewer
        objectName: "embeddedGalleryViewer"
        anchors.fill: parent
        focus: host.surfaceActive
        autoFocus: host.surfaceActive
        session: host.session
        sourcePanel: host.sourcePanel
        theme: host.theme
        hostCapabilities: host.hostCapabilities
        devicePixelRatio: host.devicePixelRatio
        onNavigationRequested: (entryId, sourceIndex) => {
            if (!host.bridge || !host.session || entryId === "")
                return
            host.bridge.requestCursor(host.bridge.viewerSide, entryId,
                                      sourceIndex,
                                      Number(host.session.catalogRevision || 0))
        }
        onSelectionRequested: (mode, entryIds) => {
            if (!host.bridge || !host.session)
                return
            host.bridge.requestSelection(host.bridge.viewerSide, mode, entryIds,
                                         Number(host.session.catalogRevision || 0))
        }
        onFullscreenToggleRequested: host.toggleFullscreen()
        // requestClose() deliberately keeps the session and Loader alive while
        // the image animates back into its panel tile. Only completion may tear
        // down the bridge-owned full-area surface.
        onCloseCompleted: {
            // Do not synchronously destroy this Loader while GalleryViewer is
            // still emitting its completion signals.
            const owningBridge = host.bridge
            if (owningBridge)
                Qt.callLater(() => owningBridge.closeViewer())
        }
    }
}
