pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: overlayHost

    required property ApplicationWindow hostWindow
    required property Item menuBar
    required property Item semanticLayer
    required property QtObject shellController
    property var frames: []

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
        model: overlayHost.frames

        delegate: Loader {
            id: overlayLoader
            required property int index
            required property var modelData
            property var frame: modelData || ({})

            function bindFrame() {
                if (item && item.frame !== undefined) {
                    item.frame = Qt.binding(function() {
                        return overlayLoader.frame
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
        }
    }
}
