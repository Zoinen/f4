pragma ComponentBehavior: Bound

import QtQuick
import F4QtHost 1.0
import QtQuick.Controls

Item {
    id: overlayHost
    objectName: "semanticOverlayHost"

    required property ApplicationWindow hostWindow
    required property Item menuBar
    required property Item semanticLayer
    required property QtObject shellController
    property var frames: []

    SemanticOverlayModel {
        id: overlayFrameModel
        objectName: "semanticOverlayFrameModel"
        frames: overlayHost.frames
    }

    function frameKey(frame) {
        return String(frame.kind || "") + "|" + String(frame.role || "")
                + "|" + String(frame.id || "")
    }

    function routeMenuBarPointer(windowX, windowY, released) {
        let handled = false
        for (let i = overlayRepeater.count - 1; i >= 0; --i) {
            const loader = overlayRepeater.itemAt(i)
            const popup = loader ? loader.item : null
            if (!popup || typeof popup.handleGrabbedPointer !== "function") continue
            if (!handled) handled = popup.handleGrabbedPointer(windowX, windowY, released)
            else popup.clearGrabbedPointer()
        }
        return handled
    }

    function menuOverlayForId(menuId) {
        const wanted = String(menuId || "")
        if (wanted === "")
            return null
        for (let i = 0; i < overlayRepeater.count; ++i) {
            const loader = overlayRepeater.itemAt(i)
            if (!loader || !loader.item || loader.item.frame === undefined)
                continue
            if (String(loader.item.frame.id || "") === wanted)
                return loader.item
        }
        return null
    }

    function createDialogOverlay(frame) {
        return dialogOverlayComponent.createObject(overlayHost,
                                                   { "frame": frame })
    }

    Component {
        id: dialogOverlayComponent
        DialogOverlay {
            hostWindow: overlayHost.hostWindow
            menuBar: overlayHost.menuBar
        }
    }

    Component {
        id: autocompletePopupComponent
        AutocompletePopup {
            hostWindow: overlayHost.hostWindow
            menuBar: overlayHost.menuBar
        }
    }

    Component {
        id: menuPopupComponent
        SemanticMenuPopup {
            hostWindow: overlayHost.hostWindow
            menuBar: overlayHost.menuBar
            semanticLayer: overlayHost.semanticLayer
            shellController: overlayHost.shellController
        }
    }

    Repeater {
        id: overlayRepeater
        objectName: "semanticOverlayRepeater"
        model: overlayFrameModel

        delegate: Loader {
            id: overlayLoader
            required property int index
            required property var modelFrame
            required property bool isClosing
            property var frame: modelFrame || ({})

            function bindFrame() {
                if (item && item.frame !== undefined) {
                    item.frame = Qt.binding(function() {
                        return overlayLoader.frame
                    })
                }
                if (item && item.closing !== undefined) {
                    item.closing = Qt.binding(function() {
                        return overlayLoader.isClosing
                    })
                }
            }

            anchors.fill: parent
            active: true
            sourceComponent: frame.kind === "menu"
                             ? (frame.role === "autocomplete"
                                ? autocompletePopupComponent
                                : menuPopupComponent)
                             : dialogOverlayComponent
            onLoaded: bindFrame()
            z: 100 + index

            Connections {
                target: overlayLoader.item
                ignoreUnknownSignals: true
                function onCloseAnimationFinished() {
                    overlayFrameModel.finishExit(
                                overlayHost.frameKey(overlayLoader.frame))
                }
            }
        }
    }
}
