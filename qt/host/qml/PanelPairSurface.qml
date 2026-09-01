pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import ZoinGallery 1.0 as ZG

Item {
    id: pair

    required property ApplicationWindow hostWindow
    required property Item panelsSurface
    required property Item menuBar
    required property Item focusTarget
    required property var galleryController
    required property ZG.GalleryThemePalette galleryTheme
    required property ZG.GalleryPresentationMetrics galleryMetrics

    property bool leftInfoCreated: false
    property bool rightInfoCreated: false
    property bool leftQuickViewCreated: false
    property bool rightQuickViewCreated: false

    function retainOptionalPanels() {
        if (panelsSurface.infoPanelForSide(0) !== null)
            leftInfoCreated = true
        if (panelsSurface.infoPanelForSide(1) !== null)
            rightInfoCreated = true
        if (panelsSurface.quickViewForSide(0) !== null)
            leftQuickViewCreated = true
        if (panelsSurface.quickViewForSide(1) !== null)
            rightQuickViewCreated = true
    }

    Component.onCompleted: retainOptionalPanels()

    Connections {
        target: pair.panelsSurface

        function onFrameChanged() {
            pair.retainOptionalPanels()
        }
    }

    FilePanelView {
        hostWindow: pair.hostWindow
        galleryController: pair.galleryController
        focusTarget: pair.focusTarget
        menuBar: pair.menuBar
        galleryTheme: pair.galleryTheme
        galleryMetrics: pair.galleryMetrics
        panel: pair.panelsSurface.panelForSide(0)
        layoutState: pair.hostWindow.leftPanelLayoutStateOverride
        visible: pair.hostWindow.panelSideVisible(0)
                 && !pair.panelsSurface.altPanelForSide(0)
    }

    FilePanelView {
        hostWindow: pair.hostWindow
        galleryController: pair.galleryController
        focusTarget: pair.focusTarget
        menuBar: pair.menuBar
        galleryTheme: pair.galleryTheme
        galleryMetrics: pair.galleryMetrics
        panel: pair.panelsSurface.panelForSide(1)
        layoutState: pair.hostWindow.rightPanelLayoutStateOverride
        visible: pair.hostWindow.panelSideVisible(1)
                 && !pair.panelsSurface.altPanelForSide(1)
    }

    Loader {
        active: pair.leftInfoCreated
        sourceComponent: Component {
            InfoPanelView {
                hostWindow: pair.hostWindow
                menuBar: pair.menuBar
                panel: pair.panelsSurface.infoPanelForSide(0)
                       || ({ "side": 0 })
                visible: pair.hostWindow.panelSideVisible(0)
                         && pair.panelsSurface.infoPanelForSide(0) !== null
            }
        }
    }

    Loader {
        active: pair.rightInfoCreated
        sourceComponent: Component {
            InfoPanelView {
                hostWindow: pair.hostWindow
                menuBar: pair.menuBar
                panel: pair.panelsSurface.infoPanelForSide(1)
                       || ({ "side": 1 })
                visible: pair.hostWindow.panelSideVisible(1)
                         && pair.panelsSurface.infoPanelForSide(1) !== null
            }
        }
    }

    // Keep native Quick View hosts alive with the persistent panel pair.
    Loader {
        active: pair.leftQuickViewCreated
        sourceComponent: Component {
            QuickViewPanelView {
                hostWindow: pair.hostWindow
                menuBar: pair.menuBar
                quickView: pair.panelsSurface.quickViewForSide(0)
                           || ({ "side": 0 })
                visible: pair.hostWindow.panelSideVisible(0)
                         && pair.panelsSurface.quickViewForSide(0) !== null
            }
        }
    }

    Loader {
        active: pair.rightQuickViewCreated
        sourceComponent: Component {
            QuickViewPanelView {
                hostWindow: pair.hostWindow
                menuBar: pair.menuBar
                quickView: pair.panelsSurface.quickViewForSide(1)
                           || ({ "side": 1 })
                visible: pair.hostWindow.panelSideVisible(1)
                         && pair.panelsSurface.quickViewForSide(1) !== null
            }
        }
    }

    PanelSplitter {
        objectName: "mainPanelSplitter"
        y: pair.menuBar.height
        height: Math.max(1, parent.height - y
                         - pair.hostWindow.commandLineHeight(
                             pair.panelsSurface.frame)
                         - pair.hostWindow.keyBarHeight())
        availableWidth: parent.width
        minimumPanelWidth: pair.hostWindow.panelMinimumWidth
        ratio: pair.hostWindow.panelSplitRatio
        defaultRatio: 0.5
        keySink: pair.focusTarget
        surfaceActive: pair.hostWindow.nativeTwoPanelSurfaceActive
                       && pair.hostWindow.widePanelSide() < 0
                       && pair.panelsSurface.hasPanelForSide(0)
                       && pair.panelsSurface.hasPanelForSide(1)
        surfaceVisible: pair.hostWindow.nativeTwoPanelSurfaceVisible
                        && pair.hostWindow.widePanelSide() < 0
                        && pair.panelsSurface.hasPanelForSide(0)
                        && pair.panelsSurface.hasPanelForSide(1)
        hoverLineColor: pair.hostWindow.separatorHoverColor
        activeLineColor: pair.hostWindow.separatorActiveColor
        trackColor: "transparent"
        separatorColor: pair.hostWindow.separatorColor
        separatorWidth: pair.hostWindow.separatorWidth
        gutterWidth: pair.hostWindow.panelContentSpacing * 2
        leadingHitInset: pair.hostWindow.panelContentSpacing
        z: 10

        onRatioRequested: (nextRatio) => {
            pair.hostWindow.panelSplitRatio = nextRatio
        }
        onFocusReleaseRequested: {
            Qt.callLater(pair.hostWindow.restoreSurfaceFocus)
        }
    }
}
