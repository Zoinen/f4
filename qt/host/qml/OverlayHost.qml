pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: overlayHost
    objectName: "semanticOverlayHost"

    required property ApplicationWindow hostWindow
    required property Item menuBar
    required property Item semanticLayer
    required property QtObject shellController
    property var frames: []

    // Keep the overlay rows stable while their semantic payload changes.
    // Repeater models backed by a freshly-created JS array can tear down and
    // recreate every Loader on each scene update. That loses local dialog
    // interaction state (most notably the geometry kept during a drag), even
    // when the incoming frame is for the same overlay id.
    ListModel {
        id: overlayFrameModel
        objectName: "semanticOverlayFrameModel"
    }

    function frameKey(frame) {
        if (!frame)
            return ""
        return String(frame.kind || "") + "|"
                + String(frame.role || "") + "|"
                + String(frame.id || "")
    }

    function modelIndexForKey(key) {
        if (key === "")
            return -1
        for (let index = 0; index < overlayFrameModel.count; ++index) {
            if (String(overlayFrameModel.get(index).key || "") === key)
                return index
        }
        return -1
    }

    function frameIsWanted(key) {
        const wanted = frames || []
        for (let index = 0; index < wanted.length; ++index) {
            if (frameKey(wanted[index]) === key)
                return true
        }
        return false
    }

    function finishFrameExit(key) {
        if (frameIsWanted(key))
            return
        const index = modelIndexForKey(key)
        if (index >= 0 && overlayFrameModel.get(index).isClosing === true)
            overlayFrameModel.remove(index)
    }

    function syncFrameModel() {
        const wanted = frames || []

        // Remove rows that no longer exist. Work backwards so indexes stay
        // valid and existing Loader instances are left untouched.
        for (let index = overlayFrameModel.count - 1; index >= 0; --index) {
            const key = String(overlayFrameModel.get(index).key || "")
            let present = false
            for (let wantedIndex = 0; wantedIndex < wanted.length;
                 ++wantedIndex) {
                if (frameKey(wanted[wantedIndex]) === key) {
                    present = true
                    break
                }
            }
            if (!present) {
                const row = overlayFrameModel.get(index)
                const frame = row.modelFrame || ({})
                if (String(frame.presentation || "") === "dropdown") {
                    if (row.isClosing !== true)
                        overlayFrameModel.setProperty(index, "isClosing", true)
                } else {
                    overlayFrameModel.remove(index)
                }
            }
        }

        // Reorder, insert, and update only the rows whose semantic payload
        // actually changed. ListModel.move preserves the delegate identity.
        for (let index = 0; index < wanted.length; ++index) {
            const wantedFrame = wanted[index] || ({})
            const key = frameKey(wantedFrame)
            if (key === "")
                continue
            let currentIndex = modelIndexForKey(key)
            if (currentIndex < 0) {
                overlayFrameModel.insert(index, {
                    "key": key,
                    "modelFrame": wantedFrame,
                    "isClosing": false
                })
                continue
            }
            if (currentIndex !== index) {
                overlayFrameModel.move(currentIndex, index, 1)
                currentIndex = index
            }
            if (overlayFrameModel.get(currentIndex).modelFrame !== wantedFrame)
                overlayFrameModel.setProperty(currentIndex, "modelFrame",
                                               wantedFrame)
            if (overlayFrameModel.get(currentIndex).isClosing === true)
                overlayFrameModel.setProperty(currentIndex, "isClosing", false)
        }
    }

    onFramesChanged: syncFrameModel()
    Component.onCompleted: syncFrameModel()

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
                    overlayHost.finishFrameExit(
                                overlayHost.frameKey(overlayLoader.frame))
                }
            }
        }
    }
}
