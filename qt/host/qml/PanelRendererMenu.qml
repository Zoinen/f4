pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Layouts
import ZoinGallery 1.0 as ZG

Popup {
    id: rendererMenu
    required property ApplicationWindow hostWindow
    required property Item panelView
    required property QtObject galleryController
    required property Item focusTarget
    required property Item anchorItem
    required property var panel
    objectName: "panelRendererMenu-" + Number(panel.side || 0)
    parent: Overlay.overlay
    width: Math.max(160, hostWindow.repeaterMaxImplicitWidth(
                              rendererChoiceRepeater))
           + leftPadding + rightPadding
    padding: 6
    modal: false
    dim: false
    z: 1001
    focus: false
    closePolicy: Popup.CloseOnEscape
                 | Popup.CloseOnPressOutside
                 | Popup.CloseOnPressOutsideParent

    onAboutToShow: {
        const point = anchorItem.mapToItem(
                        hostWindow.contentItem, anchorItem.width,
                        anchorItem.height + 3)
        x = Math.max(6, Math.min(hostWindow.width - width - 6,
                                point.x - width))
        y = Math.max(6, Math.min(hostWindow.height - height - 6,
                                point.y))
    }
    onClosed: Qt.callLater(function() {
        if (!panelView.panelIsActive || galleryController.viewerVisible
                || hostWindow.hasBlockingOverlay()
                || hostWindow.needsFallbackGrid()
                || hostWindow.hasDocumentSurface()
                || hostWindow.hasOperationsQueueSurface())
            return
        const galleryHost = panelView.galleryHost()
        if (galleryHost)
            galleryHost.forceActiveFocus()
        else
            focusTarget.forceActiveFocus()
    })

    background: Rectangle {
        color: hostWindow.controlBg
        radius: 8
        border.width: 1
        border.color: hostWindow.controlBorder
    }

    contentItem: Column {
        id: rendererMenuColumn
        spacing: 2

        Repeater {
            id: rendererChoiceRepeater
            model: panelView.rendererChoices

            delegate: Rectangle {
                id: rendererChoice
                required property int index
                required property var modelData
                readonly property bool isHeading:
                    modelData.heading === true
                width: rendererMenu.availableWidth
                implicitWidth: isHeading
                    ? 16 + rendererHeadingLabel.implicitWidth
                    : 8 + rendererChoiceLeading.implicitWidth
                      + 24 + rendererChoiceShortcut.implicitWidth
                      + 8
                height: isHeading ? 25 : 31
                radius: 5
                readonly property bool choiceEnabled:
                    panelView.rendererChoiceEnabled(modelData)
                readonly property bool choiceActive:
                    panelView.rendererChoiceActive(modelData)
                color: isHeading ? "transparent"
                       : rendererChoicePointer.containsMouse
                         && choiceEnabled
                         ? hostWindow.controlHoverBg : "transparent"

                Rectangle {
                    visible: rendererChoice.isHeading && index > 0
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.top: parent.top
                    height: 1
                    color: hostWindow.separatorColor
                }

                Text {
                    id: rendererHeadingLabel
                    visible: rendererChoice.isHeading
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.leftMargin: 8
                    anchors.rightMargin: 8
                    height: 20
                    text: hostWindow.cleanText(rendererChoice.modelData.label)
                    color: hostWindow.mutedText
                    font.pixelSize: 10
                    font.weight: Font.DemiBold
                    verticalAlignment: Text.AlignVCenter
                }

                Row {
                    id: rendererChoiceLeading
                    visible: !rendererChoice.isHeading
                    anchors.left: parent.left
                    anchors.leftMargin: 8
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: 8

                    // Always reserves its slot so the icon/label
                    // stay aligned across rows regardless of
                    // whether this particular row is active.
                    Item {
                        width: hostWindow.snapPx(14)
                        height: hostWindow.snapPx(14)
                        anchors.verticalCenter: parent.verticalCenter

                        HostPixelAlignedImage {
            hostWindow: panelView.hostWindow
                            objectName: "panelRendererChoiceCheck-"
                                        + hostWindow.cleanText(
                                              rendererChoice.modelData.mode)
                                        + "-" + Number(panel.side || 0)
                            anchors.fill: parent
                            alignmentRevision: rendererMenu.x
                                               + rendererMenu.y
                                               + rendererMenu.width
                                               + rendererMenu.height
                            visible: rendererChoice.choiceActive
                            smooth: false
                            source: hostWindow.lucideIconSource(
                                        "check", 14,
                                        hostWindow.dialogAccent)
                        }
                    }

                    HostPixelAlignedImage {
            hostWindow: panelView.hostWindow
                        objectName: "panelRendererChoiceIcon-"
                                    + hostWindow.cleanText(
                                          rendererChoice.modelData.mode)
                                    + "-" + Number(panel.side || 0)
                        width: hostWindow.snapPx(16)
                        height: hostWindow.snapPx(16)
                        alignmentRevision: rendererMenu.x
                                           + rendererMenu.y
                                           + rendererMenu.width
                                           + rendererMenu.height
                        anchors.verticalCenter: parent.verticalCenter
                        smooth: false
                        source: hostWindow.lucideIconSource(
                                         hostWindow.cleanText(
                                             rendererChoice.modelData.icon),
                                         16,
                                         rendererChoice.choiceEnabled
                                         ? hostWindow.textColor
                                         : hostWindow.mutedText)
                    }

                    Text {
                        text: hostWindow.cleanText(rendererChoice.modelData.label)
                        color: rendererChoice.choiceEnabled
                               ? hostWindow.textColor : hostWindow.mutedText
                        opacity: rendererChoice.choiceEnabled ? 1 : 0.5
                        font.pixelSize: 12
                    }
                }

                Text {
                    id: rendererChoiceShortcut
                    visible: !rendererChoice.isHeading
                    anchors.right: parent.right
                    anchors.rightMargin: 8
                    anchors.verticalCenter: parent.verticalCenter
                    text: hostWindow.cleanText(
                              rendererChoice.modelData.shortcut)
                    color: hostWindow.mutedText
                    opacity: rendererChoice.choiceEnabled ? 1 : 0.5
                    font.pixelSize: 10
                }

                MouseArea {
                        id: rendererChoicePointer
                        anchors.fill: parent
                        hoverEnabled: true
                        enabled: rendererChoice.choiceEnabled
                        cursorShape: enabled ? Qt.PointingHandCursor
                                             : Qt.ArrowCursor
                        onClicked: {
                            panelView.chooseRenderer(rendererChoice.modelData)
                            rendererMenu.close()
                        }
                    }
                }
            }

            Rectangle {
                width: rendererMenu.availableWidth
                height: 1
                color: hostWindow.separatorColor
                visible: rendererZoomRow.visible
            }

            Item {
                id: rendererZoomRow
                objectName: "panelRendererZoomRow-"
                            + Number(panel.side || 0)
                width: rendererMenu.availableWidth
                height: visible ? 48 : 0
                visible: galleryController.available
                         && panelView.galleryHost()
                         && panelView.galleryHost().densityAdjustable

                Text {
                    id: rendererZoomLabel
                    anchors.left: parent.left
                    anchors.leftMargin: 8
                    anchors.top: parent.top
                    anchors.topMargin: 5
                    text: "Zoom"
                    color: hostWindow.mutedText
                    font.pixelSize: 10
                    font.weight: Font.DemiBold
                }

                Text {
                    id: rendererZoomReset
                    objectName: "panelRendererZoomReset-"
                                + Number(panel.side || 0)
                    anchors.right: rendererZoomValue.left
                    anchors.rightMargin: 8
                    anchors.baseline: rendererZoomLabel.baseline
                    text: "Reset"
                    color: rendererZoomResetPointer.containsMouse
                           ? hostWindow.panelSelectionBorder : hostWindow.mutedText
                    font.pixelSize: 10
                    font.underline: rendererZoomResetPointer.containsMouse

                    MouseArea {
                        id: rendererZoomResetPointer
                        anchors.fill: parent
                        anchors.margins: -3
                        hoverEnabled: true
                        cursorShape: Qt.PointingHandCursor
                        onClicked: {
                            hostWindow.action({
                                "action": "panel.resetGalleryDensity",
                                "side": panel.side,
                                "layoutMode": panelView.effectiveGalleryLayoutMode
                            }, true)
                        }
                    }
                }

                Text {
                    id: rendererZoomValue
                    anchors.right: parent.right
                    anchors.rightMargin: 8
                    anchors.baseline: rendererZoomLabel.baseline
                    text: Math.round(rendererZoomSlider.value) + " px"
                    color: hostWindow.mutedText
                    font.pixelSize: 10
                }

                T.Slider {
                    id: rendererZoomSlider
                    objectName: "panelRendererZoomSlider-"
                                + Number(panel.side || 0)
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.leftMargin: 8
                    anchors.rightMargin: 8
                    height: 27
                    focusPolicy: Qt.NoFocus
                    hoverEnabled: true
                    from: panelView.galleryHost()
                          ? panelView.galleryHost().minimumDensity : 0
                    to: panelView.galleryHost()
                        ? panelView.galleryHost().maximumDensity : 1
                    stepSize: panelView.galleryHost()
                              ? panelView.galleryHost().densityStep : 1
                    value: panelView.galleryHost()
                           ? panelView.galleryHost().currentDensity : 0
                    snapMode: T.Slider.SnapAlways

                    onMoved: {
                        const host = panelView.galleryHost()
                        if (host)
                            host.previewDensity(value)
                    }
                    onPressedChanged: {
                        if (pressed)
                            return
                        const host = panelView.galleryHost()
                        if (host)
                            host.commitDensity(value)
                    }

                    background: Rectangle {
                        x: rendererZoomSlider.leftPadding
                        y: Math.round((rendererZoomSlider.height
                                       - height) / 2)
                        width: rendererZoomSlider.availableWidth
                        height: 4
                        radius: 2
                        color: hostWindow.controlBorder

                        Rectangle {
                            width: rendererZoomSlider.visualPosition
                                   * parent.width
                            height: parent.height
                            radius: parent.radius
                            color: hostWindow.panelSelectionBorder
                        }
                    }

                    handle: Rectangle {
                        x: rendererZoomSlider.leftPadding
                           + rendererZoomSlider.visualPosition
                             * (rendererZoomSlider.availableWidth
                                - width)
                        y: Math.round((rendererZoomSlider.height
                                       - height) / 2)
                        width: 14
                        height: 14
                        radius: 7
                        color: rendererZoomSlider.pressed
                               ? hostWindow.panelSelectionBorder
                               : hostWindow.chromeText
                        border.width: 2
                        border.color: hostWindow.controlBg
                    }
                }
            }
        }
    }
