pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.impl

Rectangle {
    id: menuItem
    required property ApplicationWindow hostWindow
    required property Item overlayController
    required property Item popupSurfaceItem
    required property Item popupList
    required property var scrollBar
    required property var modelData
    readonly property var dropdownTextLayout: overlayController.dropdownMode
        && overlayController.dropdownAnchor
        ? overlayController.dropdownAnchor.contentItem : null
    objectName: "semanticMenuItem-"
                + hostWindow.cleanText(overlayController.frame.id)
                + "-" + Number(modelData.index)
    width: overlayController.dropdownMode
           ? overlayController.dropdownAnchorRect.width
           : ListView.view.width - overlayController.menuEdgeInset
    height: modelData.separator
            ? overlayController.menuSeparatorHeight
            : modelData.header === true
              ? overlayController.menuHeaderHeight
              : modelData.details ? overlayController.driveRowHeight : overlayController.effectiveMenuRowHeight
    radius: 4
    color: modelData.index === overlayController.visualSelectedIndex
           && !modelData.separator
           && modelData.header !== true
           ? hostWindow.selectedBg : "transparent"

    Rectangle {
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.leftMargin: 8
        anchors.rightMargin: 8
        anchors.verticalCenter: parent.verticalCenter
        height: 1
        color: hostWindow.separatorColor
        opacity: 0.68
        visible: modelData.separator
    }

    Text {
        id: menuItemText
        objectName: "semanticMenuItemText-"
                    + hostWindow.cleanText(overlayController.frame.id)
                    + "-" + Number(modelData.index)
        anchors.left: parent.left
        anchors.right: overlayController.dropdownMode ? parent.right : shortcut.left
        anchors.rightMargin: menuItem.dropdownTextLayout
            ? parent.width - menuItem.dropdownTextLayout.x
              - menuItem.dropdownTextLayout.width : 0
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.topMargin: menuItem.dropdownTextLayout
                           ? menuItem.dropdownTextLayout.y
                           : modelData.header === true
                           ? overlayController.menuHeaderTopPadding
                           : 0
        // Match the collapsed ComboBox's Text layout exactly. Moving the
        // padding into an anchor margin changes Qt's glyph raster origin at
        // fractional DPR even when the two mapped origins compare equal.
        anchors.bottomMargin: menuItem.dropdownTextLayout
            ? parent.height - menuItem.dropdownTextLayout.y
              - menuItem.dropdownTextLayout.height : 0
        leftPadding: menuItem.dropdownTextLayout ? menuItem.dropdownTextLayout.leftPadding : 0
        rightPadding: menuItem.dropdownTextLayout ? menuItem.dropdownTextLayout.rightPadding : 0
        anchors.leftMargin: menuItem.dropdownTextLayout ? menuItem.dropdownTextLayout.x
                            : overlayController.dropdownMode ? 0
                            : modelData.header === true
                            ? hostWindow.snapPx(10)
                            : overlayController.menuLabelInset
        verticalAlignment: Text.AlignVCenter
        text: {
            var label = hostWindow.cleanText(modelData.text)
            if (overlayController.hasLeadingIndicator)
                label = label.replace(/^\s+/, "")
            if (overlayController.dropdownMode)
                return label
            return hostWindow.mnemonicText(label,
                                     modelData.hotkey)
        }
        textFormat: overlayController.dropdownMode ? Text.PlainText : Text.StyledText
        color: modelData.disabled || modelData.header === true
               ? hostWindow.mutedText : hostWindow.textColor
        font: {
            if (menuItem.dropdownTextLayout)
                return menuItem.dropdownTextLayout.font
            var defaultFont = hostWindow.font
            defaultFont.bold = modelData.header === true
            return defaultFont
        }
        visible: !modelData.separator && !modelData.details
        elide: Text.ElideRight
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                   menuItemText, hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                   menuItemText, hostWindow.contentItem)
        }
    }

    Item {
        id: driveContent
        visible: !!menuItem.modelData.details
        readonly property var details: menuItem.modelData.details || ({})
        readonly property real gap: hostWindow.snapPx(24)
        readonly property real captionGap: hostWindow.snapPx(16)
        readonly property real nameX: overlayController.menuLabelInset
        readonly property real fsWidth: overlayController.driveFilesystemWidth
        readonly property real fsX: hostWindow.snapPx(menuItem.width - 16 - fsWidth)
        readonly property real columnNameWidth: hostWindow.snapPx(Math.min(overlayController.driveNameWidth,
            Math.max(0, fsX - nameX - captionGap - gap - Math.min(overlayController.driveCapacityWidth, hostWindow.snapPx(220)))))
        readonly property real capacityX: nameX + columnNameWidth + captionGap
        readonly property real capacityWidth: hostWindow.snapPx(Math.max(0, fsX - (fsWidth > 0 ? gap : 0) - capacityX))
        readonly property real capacityTextWidth: hostWindow.snapPx(Math.min(overlayController.driveCapacityTextWidth, capacityWidth * 0.8))
        readonly property real barGap: hostWindow.snapPx(Math.min(8, capacityWidth * 0.05))
        readonly property real barX: capacityX
        readonly property real barWidth: hostWindow.snapPx(Math.max(0, capacityWidth - capacityTextWidth - barGap))
        readonly property real nameWidth: overlayController.isDriveDetails(details)
            ? columnNameWidth : hostWindow.snapPx(Math.max(0, menuItem.width - nameX - 16))
        readonly property bool capacityKnown: details.usedFraction !== undefined && isFinite(Number(details.usedFraction))
        width: menuItem.width
        height: menuItem.height

        Repeater {
            model: ["name", "capacity", "filesystem"]
            delegate: Text {
                id: driveText
                required property string modelData
                objectName: "semanticMenuDetail-" + hostWindow.cleanText(overlayController.frame.id)
                            + "-" + Number(menuItem.modelData.index) + "-" + modelData
                readonly property bool capacity: modelData === "capacity"
                x: modelData === "filesystem" ? driveContent.fsX
                   : capacity ? driveContent.barX + driveContent.barWidth + driveContent.barGap
                   : driveContent.nameX
                y: hostWindow.snapPx((driveContent.height - height) / 2)
                width: modelData === "filesystem" ? driveContent.fsWidth
                       : capacity ? driveContent.capacityTextWidth : driveContent.nameWidth
                text: modelData === "name" ? hostWindow.mnemonicText(overlayController.driveName(driveContent.details), menuItem.modelData.hotkey)
                      : capacity ? overlayController.driveCapacityText(driveContent.details, true)
                      : String(driveContent.details[modelData] || "")
                textFormat: modelData === "name" || capacity ? Text.StyledText : Text.PlainText
                horizontalAlignment: Text.AlignLeft
                font: hostWindow.font
                color: modelData === "name" ? hostWindow.textColor : hostWindow.mutedText
                elide: Text.ElideMiddle
                visible: capacity ? !!driveContent.details.total : text !== ""
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(driveText, hostWindow.contentItem)
                    y: hostWindow.dialogPixelOffsetY(driveText, hostWindow.contentItem)
                }
            }
        }
        Rectangle {
            id: capacityTrack
            objectName: "semanticMenuCapacity-" + hostWindow.cleanText(overlayController.frame.id) + "-" + Number(menuItem.modelData.index)
            x: driveContent.barX
            y: hostWindow.snapPx((driveContent.height - height) / 2)
            width: driveContent.barWidth
            height: hostWindow.snapPx(5)
            radius: height / 2
            color: hostWindow.separatorColor
            visible: driveContent.capacityKnown
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(capacityTrack, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(capacityTrack, hostWindow.contentItem)
            }
            Rectangle {
                objectName: capacityTrack.objectName + "-used"
                width: hostWindow.snapPx(parent.width * Math.max(0, Math.min(1, Number(driveContent.details.usedFraction || 0))))
                height: parent.height
                radius: parent.radius
                color: hostWindow.dialogAccent
            }
        }
    }

    IconImage {
        id: leadingMenuIcon
        objectName: "semanticMenuItemIcon-"
                    + hostWindow.cleanText(overlayController.frame.id)
                    + "-" + Number(modelData.index)
        readonly property string semanticIconName:
            modelData.checked === true ? "check"
            : modelData.details ? String(modelData.details.icon || "hard-drive")
            : hostWindow.cleanText(modelData.icon)
        readonly property url semanticIconSource:
            semanticIconName === ""
            || semanticIconName === "tag-dot" ? ""
            : hostWindow.semanticMenuIconSource(
                  semanticIconName, 15,
                  semanticIconColor)
        readonly property color semanticIconColor:
            modelData.disabled ? hostWindow.mutedText
            : hostWindow.cleanText(modelData.iconColor) !== ""
              ? hostWindow.cleanText(modelData.iconColor)
              : hostWindow.textColor
        x: hostWindow.snapPx(10)
        y: hostWindow.snapPx((parent.height - height) / 2)
        width: hostWindow.snapPx(15)
        height: hostWindow.snapPx(15)
        property real alignmentRevision:
            popupSurfaceItem.x + popupSurfaceItem.y
            + popupList.contentY + parent.y
        transform: Translate {
            x: hostWindow.iconPixelOffsetX(leadingMenuIcon)
            y: hostWindow.iconPixelOffsetY(leadingMenuIcon)
        }
        visible: !modelData.separator
                 && modelData.header !== true
                 && semanticIconName !== "tag-dot"
                 && semanticIconName !== ""
        sourceSize: Qt.size(15, 15)
        smooth: false
        mipmap: false
        source: semanticIconSource
        color: semanticIconColor
    }

    Rectangle {
        id: menuItemColor
        objectName: "semanticMenuItemColor-"
                    + hostWindow.cleanText(overlayController.frame.id)
                    + "-" + Number(modelData.index)
        x: hostWindow.snapPx(13)
        y: hostWindow.snapPx((parent.height - height) / 2)
        width: hostWindow.snapPx(10)
        height: width
        radius: width / 2
        color: hostWindow.cleanText(modelData.iconColor) !== ""
               ? hostWindow.cleanText(modelData.iconColor)
               : hostWindow.textColor
        property real alignmentRevision:
            popupSurfaceItem.x + popupSurfaceItem.y
            + popupList.contentY + parent.y
        transform: Translate {
            x: hostWindow.iconPixelOffsetX(menuItemColor)
            y: hostWindow.iconPixelOffsetY(menuItemColor)
        }
        visible: !modelData.separator
                 && modelData.header !== true
                 && hostWindow.cleanText(modelData.icon) === "tag-dot"
    }

    Text {
        id: menuItemChevron
        objectName: "semanticMenuItemChevron-"
                    + hostWindow.cleanText(overlayController.frame.id)
                    + "-" + Number(modelData.index)
        x: hostWindow.snapPx(
               parent.width - width - 9
               - (scrollBar.visible
                  ? scrollBar.width : 0))
        y: 0
        width: hostWindow.snapPx(15)
        height: hostWindow.snapPx(parent.height)
        text: "›"
        color: modelData.disabled ? hostWindow.mutedText : hostWindow.textColor
        font.pixelSize: 17
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
        property real alignmentRevision:
            popupSurfaceItem.x + popupSurfaceItem.y
            + popupList.contentY + parent.y
        transform: Translate {
            x: hostWindow.iconPixelOffsetX(menuItemChevron)
            y: hostWindow.iconPixelOffsetY(menuItemChevron)
        }
        visible: !modelData.separator
                 && modelData.header !== true
                 && modelData.hasSubmenu === true
    }

    Text {
        id: shortcut
        anchors.right: parent.right
        anchors.verticalCenter: parent.verticalCenter
        anchors.rightMargin:
            (modelData.hasSubmenu === true ? 28 : 10)
            + (overlayController.dropdownMode ? 26 : 0)
            + (scrollBar.visible
               ? scrollBar.width : 0)
        text: hostWindow.cleanText(modelData.shortcut)
        color: hostWindow.mutedText
        font: hostWindow.font
        visible: !modelData.separator
                 && modelData.header !== true
    }

    Timer {
        id: submenuHoverTimer
        interval: 180
        repeat: false
        onTriggered: hostWindow.action({
            "target": overlayController.frame.id,
            "action": "menu.openSubmenu",
            "index": modelData.index
        }, true)
    }

    MouseArea {
        id: itemMouse
        anchors.fill: parent
        hoverEnabled: true
        enabled: !modelData.separator
                 && modelData.header !== true
                 && !modelData.disabled
        function selectFromPointer() {
            if (overlayController.fromMenuBar) {
                hostWindow.menuBarPointerHasSelectedItem = true
                if (hostWindow.menuPointerItemIndex < 0
                        && !overlayController.previewIsAhead
                        && overlayController.semanticSelectedIndex
                           === modelData.index)
                    return
                if (hostWindow.menuPointerMenuIndex
                        === overlayController.effectiveMenuIndex
                        && hostWindow.menuPointerItemIndex
                           === modelData.index)
                    return
                hostWindow.menuPointerMenuIndex
                        = overlayController.effectiveMenuIndex
                hostWindow.menuPointerItemIndex = modelData.index
                hostWindow.menuPointerFrameId
                        = String(overlayController.frame.id || "")
                hostWindow.scheduleMenuPointerSync()
            } else {
                if (overlayController.pointerSelectedIndex
                        === modelData.index)
                    return
                overlayController.pointerSelectedIndex = modelData.index
                hostWindow.action({
                    "target": overlayController.frame.id,
                    "action": "menu.select",
                    "index": modelData.index
                }, true)
            }
        }
        // Delegate creation and ListView scrolling both
        // produce local position changes under a stationary
        // cursor. Only movement in window coordinates is
        // allowed to take selection ownership.
        onPositionChanged: (mouse) => {
            if (containsMouse
                    && overlayController.pointerActuallyMoved(
                        itemMouse, mouse))
                selectFromPointer()
        }
        onEntered: {
            if (modelData.hasSubmenu === true)
                submenuHoverTimer.restart()
        }
        onExited: submenuHoverTimer.stop()
        onPressed: {
            if (overlayController.dropdownMode)
                selectFromPointer()
        }
        onClicked: {
            submenuHoverTimer.stop()
            if (overlayController.fromMenuBar) {
                hostWindow.action({
                    "action": "menuBar.itemActivate",
                    "menuIndex": overlayController.effectiveMenuIndex,
                    "index": modelData.index
                }, true)
            } else {
                hostWindow.action({
                    "target": overlayController.frame.id,
                    "action": "menu.activate",
                    "index": modelData.index
                }, true)
            }
            hostWindow.menuBarPreviewIndex = -1
            hostWindow.clearMenuPointerSelection()
        }
    }
}
