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

    property var frame: hostWindow.shellFrame()
    property var panelList: frame.panels || []

    function panelForSide(side) {
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
        anchors.topMargin: panels.menuBar.height
        anchors.bottom: parent.bottom
        anchors.bottomMargin: panels.hostWindow.keyBarHeight()
                              + panels.hostWindow.commandLineHeight(panels.frame)
        visible: panels.frame.terminalActive === true
                 || (panels.hostWindow.widePanelSide() < 0
                     && (panels.frame.showLeftPanel === false
                         || panels.frame.showRightPanel === false
                         || Number((panels.frame.panelLayout || {})
                                      .leftBottomInsetRows || 0) > 0
                         || Number((panels.frame.panelLayout || {})
                                      .rightBottomInsetRows || 0) > 0))
        shell: panels.frame
        terminal: panels.frame.terminal || ({})
    }

    Loader {
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
