pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl
import QtQuick.Layouts
import ZoinGallery 1.0 as ZG

Rectangle {
    id: panelHeader
    required property ApplicationWindow hostWindow
    required property Item panelView
    required property QtObject galleryController
    required property Item focusTarget
    required property var panel
    objectName: "panelHeader-" + Number(panel.side || 0)
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: parent.top
    height: Math.max(25, hostWindow.ch * 1.25)
            + hostWindow.verticalContentSpacing
            + hostWindow.pathRowExtraHeight
    // Keep the panel header color as a translucent foreground over
    // the same chrome surface used by the title bar.
    color: hostWindow.titleBarBg
    z: 2

    Rectangle {
        id: panelHeaderPanelBackground
        objectName: "panelHeaderPanelBackground-"
                    + Number(panel.side || 0)
        anchors.fill: parent
        color: hostWindow.panelPathBg
        z: 0
    }

    Rectangle {
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: hostWindow.separatorWidth
        color: hostWindow.separatorColor
    }

    Item {
        id: panelPathArea
        anchors.left: parent.left
        anchors.right: sortButton.left
        anchors.verticalCenter: parent.verticalCenter
        anchors.leftMargin: hostWindow.panelTextInset
        anchors.rightMargin: hostWindow.panelTextInset
        height: Math.min(parent.height - 4, 32)
        clip: true

        ZG.Button {
            id: panelDriveButton
            objectName: "panelDriveButton-" + Number(panel.side || 0)
            anchors.left: parent.left
            anchors.top: parent.top
            anchors.bottom: parent.bottom
            width: hostWindow.snapPx(24)
            leftPadding: 0
            topPadding: 0
            rightPadding: 0
            bottomPadding: 0
            focusPolicy: Qt.NoFocus
            hoverEnabled: true

            contentItem: Item {
                HostPixelAlignedImage {
                hostWindow: panelView.hostWindow
                    id: panelDriveButtonIcon
                    objectName: "panelDriveButtonIcon-"
                                + Number(panel.side || 0)
                    anchors.centerIn: parent
                    width: panelPathControl.driveIconSize
                    height: panelPathControl.driveIconSize
                    smooth: false
                    mipmap: false
                    alignmentRevision: panelDriveButton.x
                                       + panelDriveButton.y
                                       + panelDriveButton.width
                                       + panelDriveButton.height
                                       + panelPathArea.width
                                       + panelPathArea.height
                    source: panelPathControl.currentDriveIconSource
                }
            }

            background: Rectangle {
                radius: 4
                color: panelDriveButton.down
                       ? hostWindow.galleryPathItemPressedColor
                       : panelDriveButton.hovered
                         ? hostWindow.galleryPathHoverColor : "transparent"
            }

            onClicked: hostWindow.action({
                "action": "panel.driveMenu",
                "side": Number(panel.side || 0)
            })
        }

        ZG.PathControl {
            id: panelPathControl
            objectName: "panelPathTitle-" + Number(panel.side || 0)
            anchors.left: panelDriveButton.right
            anchors.leftMargin: hostWindow.snapPx(4)
            anchors.top: parent.top
            anchors.bottom: parent.bottom
            anchors.right: parent.right
            anchors.rightMargin: panelView.loadingIndicatorVisible ? 18 : 0
            backgroundOnHoverOnly: true
            // panelPathArea already begins on the shared 16 px
            // content line; do not add the standalone control's inset.
            leadingInset: 0
            showDriveIcon: false
            // Remote panels can expose a Windows-native path even
            // when the Qt host itself is running on another OS.
            windowsPathSeparators:
                Qt.platform.os === "windows"
                || String(panel.path || "").indexOf("\\") >= 0
            breadcrumbFontPixelSize: hostWindow.semanticTextFontPixelSize
            pathBackgroundColor: hostWindow.galleryPathBackgroundColor
            pathTextColor: hostWindow.galleryPathTextColor
            pathHoveredColor: hostWindow.controlBg
            pathItemHoveredColor: hostWindow.galleryPathItemHoverColor
            pathItemPressedColor: hostWindow.galleryPathItemPressedColor
            devicePixelRatio: hostWindow.iconDevicePixelRatio
            alignmentRevision: panelView.x + panelView.y
                               + panelView.width + panelView.height
                               + hostWindow.panelSplitRatio
            breadcrumbSeparatorIconSource:
                hostWindow.lucideIconSource(
                    "chevron-right", 12, hostWindow.galleryPathTextColor)
            localDriveIconSource:
                hostWindow.lucideIconSource(
                    "hard-drive", 18, hostWindow.galleryPathTextColor)
            networkDriveIconSource:
                hostWindow.lucideIconSource(
                    "network", 18, hostWindow.galleryPathTextColor)
            text: hostWindow.cleanText(panel.title || panel.path)
            navigationPath: String(panel.path || "")
            navigationHandler: function(path) {
                hostWindow.action({
                    "action": "panel.navigatePath",
                    "side": Number(panel.side || 0),
                    "path": String(path)
                })
            }
            onEditModeChanged: {
                if (!editMode)
                    focusTarget.forceActiveFocus()
            }
        }

        Text {
            id: panelLoadingIndicator
            objectName: "panelLoadingIndicator-"
                        + Number(panel.side || 0)
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            visible: panelView.loadingIndicatorVisible
            text: panelView.loadingIndicatorFrames[
                      panelView.loadingIndicatorFrame]
            color: hostWindow.mutedText
            font.pixelSize: 13
        }
    }

    ToolButton {
        id: sortButton
        objectName: "panelSortButton-" + Number(panel.side || 0)
        anchors.right: presentationButton.left
        anchors.rightMargin: hostWindow.snapPx(4)
        y: hostWindow.snapPx((parent.height - height) / 2)
        width: hostWindow.snapPx(sortButtonContent.implicitWidth
                           + hostWindow.actionButtonHorizontalMargin * 2)
        height: hostWindow.snapPx(Math.min(parent.height - 4, 28))
        hoverEnabled: true
        focusPolicy: Qt.NoFocus

        ZG.ToolTip {
            visible: sortButton.hovered && !sortMenu.opened
            delay: 500
            timeout: 5000
            text: panelView.sortModeName() === "unsorted"
                  ? "Sorting: Unsorted"
                  : "Sort by " + panelView.sortModeLabel()
                    + (panelView.sortIsAscending()
                       ? " · Ascending" : " · Descending")
        }

        contentItem: Row {
            id: sortButtonContent
            objectName: "panelSortButtonContent-"
                        + Number(panel.side || 0)
            x: hostWindow.snapPx((parent.width - width) / 2)
            y: hostWindow.snapPx((parent.height - height) / 2)
            width: hostWindow.snapPx(implicitWidth)
            height: hostWindow.snapPx(implicitHeight)
            spacing: hostWindow.snapPx(5)
            property real alignmentRevision:
                sortButton.x + sortButton.y
                + sortButton.width + sortButton.height
            transform: Translate {
                id: sortButtonContentPixelTranslation
                x: hostWindow.iconPixelOffsetX(sortButtonContent)
                y: hostWindow.iconPixelOffsetY(sortButtonContent)
            }

            HostPixelAlignedImage {
                hostWindow: panelView.hostWindow
                objectName: "panelSortDirectionIcon-"
                            + Number(panel.side || 0)
                readonly property string lucideName:
                    panelView.sortDirectionIconName()
                visible: panelView.sortModeName() !== "unsorted"
                width: visible ? hostWindow.snapPx(14) : 0
                height: hostWindow.snapPx(14)
                alignmentRevision: sortButton.x + sortButton.y
                                   + sortButton.width + sortButton.height
                                   + sortButtonContent.x
                                   + sortButtonContent.y
                                   + sortButtonContentPixelTranslation.x
                                   + sortButtonContentPixelTranslation.y
                y: hostWindow.snapPx((parent.height - height) / 2)
                smooth: false
                source: hostWindow.lucideIconSource(
                            lucideName, 14,
                            sortButton.enabled
                            ? hostWindow.chromeText : hostWindow.mutedText)
            }

            Text {
                id: sortButtonLabel
                objectName: "panelSortLabel-"
                            + Number(panel.side || 0)
                y: hostWindow.snapPx((parent.height - height) / 2)
                text: panelView.sortModeLabel()
                color: sortButton.enabled
                       ? hostWindow.chromeText : hostWindow.mutedText
                font.pixelSize: 12
            }

            HostPixelAlignedImage {
                hostWindow: panelView.hostWindow
                objectName: "panelSortChevron-"
                            + Number(panel.side || 0)
                width: hostWindow.snapPx(11)
                height: hostWindow.snapPx(11)
                alignmentRevision: sortButton.x + sortButton.y
                                   + sortButton.width + sortButton.height
                                   + sortButtonContent.x
                                   + sortButtonContent.y
                                   + sortButtonContentPixelTranslation.x
                                   + sortButtonContentPixelTranslation.y
                y: hostWindow.snapPx((parent.height - height) / 2)
                smooth: false
                source: hostWindow.lucideIconSource(
                            "chevron-down", 11,
                            sortButton.enabled
                            ? hostWindow.chromeText : hostWindow.mutedText)
            }
        }

        background: Rectangle {
            radius: 5
            color: sortButton.down
                   ? hostWindow.controlPressedBg
                   : sortButton.hovered || sortMenu.opened
                     ? hostWindow.controlHoverBg : "transparent"
            border.width: sortMenu.opened ? 1 : 0
            border.color: hostWindow.panelSelectionBorder
        }

        onClicked: {
            if (sortMenu.opened) {
                sortMenu.close()
                return
            }
            rendererMenu.close()
            sortMenu.open()
        }
    }

    Popup {
        id: sortMenu
        objectName: "panelSortMenu-" + Number(panel.side || 0)
        parent: Overlay.overlay
        width: Math.max(160, hostWindow.repeaterMaxImplicitWidth(
                                  sortChoiceRepeater))
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
            const point = sortButton.mapToItem(
                            hostWindow.contentItem, sortButton.width,
                            sortButton.height + 3)
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
            id: sortMenuColumn
            spacing: 2

            Repeater {
                id: sortChoiceRepeater
                model: panelView.sortChoices

                delegate: Rectangle {
                    id: sortChoice
                    required property var modelData
                    objectName: "panelSortChoice-"
                                + hostWindow.cleanText(modelData.mode)
                                + "-" + Number(panel.side || 0)
                    width: sortMenu.availableWidth
                    // Content determines the menu's width (see
                    // sortMenu.contentWidth below); this row must
                    // never be narrower than what it needs.
                    implicitWidth: 10 + sortChoiceLeading.implicitWidth
                                   + 24 + sortChoiceShortcut.implicitWidth
                                   + 10
                    height: 31
                    radius: 5
                    readonly property bool choiceActive:
                        panelView.sortModeName()
                        === hostWindow.cleanText(modelData.mode)
                    color: sortChoicePointer.containsMouse
                           ? hostWindow.controlHoverBg : "transparent"

                    Row {
                        id: sortChoiceLeading
                        anchors.left: parent.left
                        anchors.leftMargin: 10
                        anchors.verticalCenter: parent.verticalCenter
                        spacing: 8

                        // Always reserves its slot so the icon/label
                        // stay aligned across rows regardless of
                        // whether this particular row is the active
                        // choice.
                        Item {
                            width: hostWindow.snapPx(14)
                            height: hostWindow.snapPx(14)
                            anchors.verticalCenter: parent.verticalCenter

                            HostPixelAlignedImage {
                hostWindow: panelView.hostWindow
                                objectName: "panelSortChoiceCheck-"
                                            + hostWindow.cleanText(
                                                  sortChoice.modelData.mode)
                                            + "-" + Number(panel.side || 0)
                                anchors.fill: parent
                                alignmentRevision: sortMenu.x + sortMenu.y
                                                   + sortMenu.width
                                                   + sortMenu.height
                                visible: sortChoice.choiceActive
                                smooth: false
                                source: hostWindow.lucideIconSource(
                                            "check", 14,
                                            hostWindow.dialogAccent)
                            }
                        }

                        HostPixelAlignedImage {
                hostWindow: panelView.hostWindow
                            objectName: "panelSortChoiceIcon-"
                                        + hostWindow.cleanText(
                                              sortChoice.modelData.mode)
                                        + "-" + Number(panel.side || 0)
                            width: hostWindow.snapPx(16)
                            height: hostWindow.snapPx(16)
                            alignmentRevision: sortMenu.x + sortMenu.y
                                               + sortMenu.width
                                               + sortMenu.height
                            anchors.verticalCenter: parent.verticalCenter
                            smooth: false
                            source: hostWindow.lucideIconSource(
                                             hostWindow.cleanText(
                                                 sortChoice.modelData.icon),
                                             16, hostWindow.textColor)
                        }

                        Text {
                            objectName: "panelSortChoiceLabel-"
                                        + hostWindow.cleanText(
                                              sortChoice.modelData.mode)
                                        + "-" + Number(panel.side || 0)
                            anchors.verticalCenter: parent.verticalCenter
                            text: hostWindow.cleanText(sortChoice.modelData.label)
                            color: hostWindow.textColor
                            font.pixelSize: 12
                        }
                    }

                    Text {
                        id: sortChoiceShortcut
                        anchors.right: parent.right
                        anchors.rightMargin: 10
                        anchors.verticalCenter: parent.verticalCenter
                        text: hostWindow.cleanText(
                                  sortChoice.modelData.shortcut)
                        color: hostWindow.mutedText
                        font.pixelSize: 10
                    }

                    MouseArea {
                        id: sortChoicePointer
                        anchors.fill: parent
                        hoverEnabled: true
                        cursorShape: Qt.PointingHandCursor
                        onClicked: {
                            panelView.chooseSort(sortChoice.modelData)
                            sortMenu.close()
                        }
                    }
                }
            }
        }
    }

    ToolButton {
        id: presentationButton
        objectName: "panelRendererButton-" + Number(panel.side || 0)
        anchors.right: parent.right
        anchors.rightMargin: hostWindow.panelTextInset
        anchors.verticalCenter: parent.verticalCenter
        width: presentationButtonContent.implicitWidth
               + hostWindow.actionButtonHorizontalMargin * 2
        height: Math.min(parent.height - 4, 28)
        hoverEnabled: true
        focusPolicy: Qt.NoFocus
        // Once the renderer popup is open its contents are the
        // active explanation.  Keeping the delayed button tooltip
        // alive above the popup creates a detached black label that
        // overlaps the menu and obscures the first interaction.
        ZG.ToolTip {
            objectName: "panelRendererToolTip-"
                        + Number(panel.side || 0)
            visible: presentationButton.hovered
                     && !rendererMenu.opened
            delay: 500
            timeout: 5000
            text: "Panel view mode"
        }

        contentItem: Row {
            id: presentationButtonContent
            objectName: "panelRendererButtonContent-"
                        + Number(panel.side || 0)
            anchors.centerIn: parent
            spacing: 5

            HostPixelAlignedImage {
                hostWindow: panelView.hostWindow
                objectName: "panelRendererButtonIcon-"
                            + Number(panel.side || 0)
                readonly property string lucideName:
                    panelView.rendererButtonIconName()
                width: hostWindow.snapPx(16)
                height: hostWindow.snapPx(16)
                alignmentRevision: presentationButton.x
                                   + presentationButton.y
                                   + presentationButton.width
                                   + presentationButton.height
                                   + presentationButtonContent.x
                                   + presentationButtonContent.y
                y: hostWindow.snapPx((parent.height - height) / 2)
                smooth: false
                source: hostWindow.lucideIconSource(
                            lucideName, 16, hostWindow.chromeText)
            }

            HostPixelAlignedImage {
                hostWindow: panelView.hostWindow
                objectName: "panelRendererButtonChevron-"
                            + Number(panel.side || 0)
                readonly property string lucideName: "chevron-down"
                width: hostWindow.snapPx(11)
                height: hostWindow.snapPx(11)
                alignmentRevision: presentationButton.x
                                   + presentationButton.y
                                   + presentationButton.width
                                   + presentationButton.height
                                   + presentationButtonContent.x
                                   + presentationButtonContent.y
                y: hostWindow.snapPx((parent.height - height) / 2)
                smooth: false
                source: hostWindow.lucideIconSource(
                            lucideName, 11, hostWindow.chromeText)
            }
        }

        background: Rectangle {
            radius: 5
            color: presentationButton.down
                   ? hostWindow.controlPressedBg
                   : presentationButton.hovered || rendererMenu.opened
                     ? hostWindow.controlHoverBg : "transparent"
            border.width: rendererMenu.opened ? 1 : 0
            border.color: hostWindow.panelSelectionBorder
        }

        onClicked: {
            if (rendererMenu.opened) {
                rendererMenu.close()
                return
            }
            sortMenu.close()
            rendererMenu.open()
        }
    }

    PanelRendererMenu {
        id: rendererMenu
        hostWindow: panelHeader.hostWindow
        panelView: panelHeader.panelView
        galleryController: panelHeader.galleryController
        focusTarget: panelHeader.focusTarget
        anchorItem: presentationButton
        panel: panelHeader.panel
    }

    MouseArea {
        // Popup.CloseOnPressOutside is not delivered reliably when a
        // non-windowed popup is parented to Overlay.overlay above the
        // persistent panel/grid pointer layers. Put a transparent
        // dismiss plane immediately below the popup instead: popup
        // contents still receive their input, every outside press is
        // consumed here and cannot also move the file-panel cursor.
        parent: Overlay.overlay
        anchors.fill: parent
        visible: rendererMenu.opened
        enabled: visible
        z: 1000
        acceptedButtons: Qt.LeftButton | Qt.RightButton
                         | Qt.MiddleButton
        onPressed: rendererMenu.close()
    }

    MouseArea {
        parent: Overlay.overlay
        anchors.fill: parent
        visible: sortMenu.opened
        enabled: visible
        z: 1000
        acceptedButtons: Qt.LeftButton | Qt.RightButton
                         | Qt.MiddleButton
        onPressed: sortMenu.close()
    }

}
