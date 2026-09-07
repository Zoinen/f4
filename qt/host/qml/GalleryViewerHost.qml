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
        animationDuration: 150
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
            if (owningBridge) {
                if (galleryViewer.immediateCloseRequested
                        && typeof owningBridge.suppressKeyRelease
                           === "function") {
                    owningBridge.suppressKeyRelease(Qt.Key_Escape)
                }
                Qt.callLater(() => owningBridge.closeViewer())
            }
        }
    }

    Rectangle {
        id: transitionBorder
        objectName: "galleryViewerTransitionBorder"
        readonly property var sourceSurface: host.sourcePanel
                && typeof host.sourcePanel.currentItemSelectionSurface === "function"
                ? host.sourcePanel.currentItemSelectionSurface() : null
        readonly property real progress: galleryViewer.transitionProgress
        readonly property rect sourceRect: sourceSurface
                ? sourceSurface.parent.mapToItem(host, sourceSurface.x, sourceSurface.y,
                                                sourceSurface.width, sourceSurface.height)
                : Qt.rect(0, 0, 0, 0)
        readonly property rect targetRect: transitionCaption.imageRect
        visible: sourceSurface !== null && galleryViewer.transitionHasGeometry
                 && galleryViewer.viewerContentVisible && progress < 1
        opacity: 1 - progress
        x: galleryViewer.lerp(sourceRect.x, targetRect.x, progress)
        y: galleryViewer.lerp(sourceRect.y, targetRect.y, progress)
        width: galleryViewer.lerp(sourceRect.width, targetRect.width, progress)
        height: galleryViewer.lerp(sourceRect.height, targetRect.height, progress)
        color: "transparent"
        // The panel suppresses its cursor stroke while handing the image to
        // the viewer. Preserve the semantic stroke, not that temporary zero.
        border.width: sourceSurface
                ? Math.max(sourceSurface.nominalBorderWidth,
                           sourceSurface.selectionBorderVisible
                           || (sourceSurface.entry && sourceSurface.entry.current) ? 1 : 0)
                : 0
        border.color: sourceSurface ? sourceSurface.visualBorderColor : "transparent"
        radius: sourceSurface ? sourceSurface.radius * (1 - progress) : 0
        antialiasing: true
        Binding {
            target: transitionBorder.sourceSurface
            property: "opacity"
            value: 0
            when: transitionBorder.visible
            restoreMode: Binding.RestoreBindingOrValue
        }
    }

    // Carry the source caption above the expanding image. Leaving it in the
    // panel makes the opaque viewer image cover it on the first frame.
    Item {
        id: transitionCaption
        objectName: "galleryViewerTransitionCaption"
        readonly property var sourceLabel: host.sourcePanel
                && typeof host.sourcePanel.currentItemCaption === "function"
                ? host.sourcePanel.currentItemCaption() : null
        readonly property var sourceEntry: sourceLabel
                ? sourceLabel.parent.parent.entry : null
        readonly property real progress: galleryViewer.transitionProgress
        readonly property rect sourceRect: sourceLabel
                ? sourceLabel.parent.parent.mapToItem(host,
                    sourceLabel.parent.x, sourceLabel.parent.y,
                    sourceLabel.parent.width, sourceLabel.parent.height)
                : Qt.rect(0, 0, 0, 0)
        // Use the mathematical endpoint, not the live image item: its fit,
        // translation and decoded dimensions settle independently at startup.
        readonly property rect imageRect:
            galleryViewer.flickableArea.imageRectFittedInRect(
                Qt.rect(0, 0, galleryViewer.width, galleryViewer.height))
        visible: sourceLabel !== null && galleryViewer.transitionHasGeometry
                 && progress < 1 && galleryViewer.viewerContentVisible
        opacity: 1 - progress
        x: galleryViewer.lerp(sourceRect.x, imageRect.x, progress)
        y: galleryViewer.lerp(sourceRect.y,
                             imageRect.y + imageRect.height - height, progress)
        width: galleryViewer.lerp(sourceRect.width, imageRect.width, progress)
        height: sourceRect.height
        clip: true
        Binding {
            target: transitionCaption.sourceLabel ? transitionCaption.sourceLabel.parent : null
            property: "opacity"
            value: 0
            when: transitionCaption.visible
            restoreMode: Binding.RestoreBindingOrValue
        }
        Rectangle {
            objectName: "galleryViewerTransitionCaptionBackground"
            anchors.fill: parent
            color: transitionCaption.sourceEntry
                   ? transitionCaption.sourceEntry.highlightLabelBackground : "transparent"
            radius: 3
        }
        Text {
            objectName: "galleryViewerTransitionCaptionText"
            anchors.fill: parent
            anchors.margins: 6
            text: transitionCaption.sourceLabel ? transitionCaption.sourceLabel.text : ""
            textFormat: transitionCaption.sourceLabel
                        ? transitionCaption.sourceLabel.textFormat : Text.PlainText
            font: transitionCaption.sourceLabel ? transitionCaption.sourceLabel.font : Qt.font({pixelSize: 12})
            color: transitionCaption.sourceLabel ? transitionCaption.sourceLabel.color : "white"
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideMiddle
        }
    }
}
