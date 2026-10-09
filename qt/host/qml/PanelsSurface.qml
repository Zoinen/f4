pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import ZoinGallery 1.0 as ZG

Item {
    id: panels

    required property ApplicationWindow hostWindow
    required property Item menuBar
    required property Item focusTarget
    required property var galleryController
    required property ZG.GalleryThemePalette galleryTheme
    required property ZG.GalleryPresentationMetrics galleryMetrics

    property Item panelChromeLayer: panels
    property bool splitterHovered: false
    property bool splitterDragging: false
    readonly property alias panelPair: panelPairLoader.item
    property var frame: hostWindow.shellFrame()
    property var panelList: frame.panels || []
    property var leftPanelDescriptor: ({ "side": 0 })
    property var rightPanelDescriptor: ({ "side": 1 })
    property string leftPanelSignature: ""
    property string rightPanelSignature: ""

    onPanelListChanged: {
        updatePanelDescriptor(0)
        updatePanelDescriptor(1)
    }

    function sourcePanelForSide(side) {
        const compactPanel = side === 0
                ? hostWindow.leftPanelPresentationOverride
                : hostWindow.rightPanelPresentationOverride
        if (compactPanel !== null)
            return compactPanel
        for (let index = 0; index < panelList.length; ++index) {
            if (Number(panelList[index].side) === side)
                return panelList[index]
        }
        return ({ "side": side })
    }

    function updatePanelDescriptor(side) {
        const descriptor = sourcePanelForSide(side)
        // Descriptors are row-free. A shell/terminal update can carry fresh
        // objects for the same panel, or repeat an accepted compact catalog.
        // Keep each panel's observable object until its own values change.
        const signature = JSON.stringify(descriptor)
        if (side === 0) {
            if (signature === leftPanelSignature)
                return
            leftPanelSignature = signature
            leftPanelDescriptor = descriptor
        } else {
            if (signature === rightPanelSignature)
                return
            rightPanelSignature = signature
            rightPanelDescriptor = descriptor
        }
    }

    function panelForSide(side) {
        return side === 0 ? leftPanelDescriptor : rightPanelDescriptor
    }

    Connections {
        target: panels.hostWindow
        function onLeftPanelPresentationOverrideChanged() {
            panels.updatePanelDescriptor(0)
        }
        function onRightPanelPresentationOverrideChanged() {
            panels.updatePanelDescriptor(1)
        }
    }

    function hasPanelForSide(side) {
        for (let index = 0; index < panelList.length; ++index) {
            if (Number(panelList[index].side) === side)
                return true
        }
        return false
    }

    function infoPanelForSide(side) {
        return hostWindow.infoPanelForSide(side)
    }

    function quickViewForSide(side) {
        return hostWindow.quickViewForSide(side)
    }

    function altPanelForSide(side) {
        return infoPanelForSide(side) || quickViewForSide(side)
    }

    // The terminal is the one persistent surface underneath Commander panels.
    TerminalBackdrop {
        hostWindow: panels.hostWindow
        menuBar: panels.menuBar
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.topMargin: panels.hostWindow.menuBarHeight
        anchors.bottom: parent.bottom
        anchors.bottomMargin: panels.hostWindow.keyBarHeight()
                              + panels.hostWindow.commandLineHeight(panels.frame)
        visible: panels.frame.terminalActive === true
                 || (panels.hostWindow.panelSideVisible(0)
                     && panels.hostWindow.panelBottomInset(0) > 0)
                 || (panels.hostWindow.panelSideVisible(1)
                     && panels.hostWindow.panelBottomInset(1) > 0)
                 || (panels.hostWindow.widePanelSide() < 0
                     && (panels.frame.showLeftPanel === false
                         || panels.frame.showRightPanel === false))
        shell: panels.frame
        terminal: panels.frame.terminal || ({})
    }

    Loader {
        id: panelPairLoader
        objectName: "persistentPanelPair"
        anchors.fill: parent
        active: true
        visible: panels.frame.terminalActive !== true
        sourceComponent: PanelPairSurface {
            hostWindow: panels.hostWindow
            panelsSurface: panels
            menuBar: panels.menuBar
            focusTarget: panels.focusTarget
            galleryController: panels.galleryController
            galleryTheme: panels.galleryTheme
            galleryMetrics: panels.galleryMetrics
        }
    }

    CommandLineView {
        hostWindow: panels.hostWindow
        commandLine: panels.hostWindow.commandLineFrame()
    }
}
