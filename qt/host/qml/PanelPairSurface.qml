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

    property var pendingSplitUpdate: null
    function flushSplitUpdate() {
        splitUpdateTimer.stop()
        if (!pendingSplitUpdate)
            return
        const update = pendingSplitUpdate
        pendingSplitUpdate = null
        hostWindow.action(update, true)
    }
    function publishSplitRatio(ratio) {
        hostWindow.panelSplitRatio = ratio
        pendingSplitUpdate = {
            action: "panel.setSplit",
            target: String(hostWindow.shellFrame().id || ""),
            ratioMillionths: Math.round(ratio * 1000000)
        }
        if (!splitUpdateTimer.running)
            splitUpdateTimer.start()
    }
    Timer {
        id: splitUpdateTimer
        interval: 16
        onTriggered: pair.flushSplitUpdate()
    }

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

    // Keep these hit areas above the divider, including while their visuals
    // are hidden. They occupy only the existing inset after the View button.
    property int expandButtonHoverMask: 0
    readonly property bool expandButtonsHovered: expandButtonHoverMask !== 0

    Repeater {
        id: expandButtons
        parent: pair.panelsSurface.panelChromeLayer
        model: 2
        delegate: ToolButton {
            id: expandButton
            opacity: pair.hostWindow.normalSurfaceOpacity
            required property int index
            objectName: "panelExpandButton-" + index
            readonly property bool expanded: pair.hostWindow.widePanelSide() === index
            readonly property bool revealed: hovered || pair.expandButtonsHovered
                                             || pair.panelsSurface.splitterHovered
            readonly property string iconName: (index === 0) !== expanded
                ? "arrow-right-from-line" : "arrow-left-from-line"
            x: pair.hostWindow.nativePanelX(index)
               + (index === 0 ? pair.hostWindow.nativePanelWidth(index) - width : 0)
            y: pair.hostWindow.snapPx(pair.hostWindow.menuBarHeight
               + (pair.hostWindow.panelPathRowHeight - height) / 2)
            width: pair.hostWindow.snapPx(pair.hostWindow.panelTextInset)
            height: Math.min(pair.hostWindow.panelPathRowHeight - 4, 28)
            z: 62
            padding: 0
            hoverEnabled: true
            onHoveredChanged: {
                if (hovered)
                    pair.expandButtonHoverMask |= (1 << index)
                else
                    pair.expandButtonHoverMask &= ~(1 << index)
            }
            focusPolicy: Qt.NoFocus
            visible: pair.hostWindow.panelPathBarsVisible
                     && pair.hostWindow.panelSideVisible(index)
                     && !pair.panelsSurface.altPanelForSide(index)
                     && pair.panelsSurface.hasPanelForSide(index)
            enabled: pair.hostWindow.nativeTwoPanelSurfaceActive
            Accessible.name: expanded ? qsTr("Restore split panels")
                                     : qsTr("Expand panel to full size")
            contentItem: Item {
                HostPixelAlignedImage {
                    hostWindow: pair.hostWindow
                    objectName: "panelExpandIcon-" + expandButton.index
                    width: pair.hostWindow.snapPx(12)
                    height: width
                    x: pair.hostWindow.snapPx((parent.width - width) / 2)
                    y: pair.hostWindow.snapPx((parent.height - height) / 2)
                    visible: expandButton.revealed
                    smooth: false
                    source: pair.hostWindow.lucideIconSource(
                                expandButton.iconName,
                                12, pair.hostWindow.galleryPathTextColor)
                }
            }
            background: Rectangle {
                objectName: "panelExpandBackground-" + expandButton.index
                radius: 5
                visible: expandButton.revealed
                color: expandButton.down ? pair.hostWindow.controlPressedBg
                     : expandButton.hovered ? pair.hostWindow.controlHoverBg
                                            : "transparent"
            }
            ZG.ToolTip {
                id: expandTip
                objectName: "panelExpandToolTip-" + expandButton.index
                visible: expandButton.hovered
                delay: 600
                text: expandButton.Accessible.name
                contentItem: Text {
                    id: expandTipText
                    objectName: "panelExpandToolTipText-" + expandButton.index
                    text: expandTip.text
                    font: expandTip.font
                    color: pair.hostWindow.chromeText
                    transform: Translate {
                        x: pair.hostWindow.dialogPixelOffsetX(expandTipText, pair.hostWindow.contentItem)
                        y: pair.hostWindow.dialogPixelOffsetY(expandTipText, pair.hostWindow.contentItem)
                    }
                }
            }
            onClicked: pair.hostWindow.action({
                action: "panel.setWide", side: index, enabled: !expanded
            }, true)
        }
    }

}
