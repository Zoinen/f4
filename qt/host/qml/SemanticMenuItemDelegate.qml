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
    objectName: "semanticMenuItem-"
                + hostWindow.cleanText(overlayController.frame.id)
                + "-" + Number(modelData.index)
    width: ListView.view.width - overlayController.menuEdgeInset
    height: modelData.separator
            ? overlayController.menuSeparatorHeight
            : modelData.header === true
              ? overlayController.menuHeaderHeight
              : overlayController.effectiveMenuRowHeight
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
        anchors.right: shortcut.left
        anchors.verticalCenter: modelData.header === true
                               ? undefined
                               : parent.verticalCenter
        anchors.top: modelData.header === true
                     ? parent.top : undefined
        anchors.bottom: modelData.header === true
                        ? parent.bottom : undefined
        anchors.topMargin: modelData.header === true
                           ? overlayController.menuHeaderTopPadding
                           : 0
        anchors.leftMargin: modelData.header === true
                            ? 10
                            : overlayController.dropdownMode
                            ? 0
                            : overlayController.hasLeadingIndicator
                            ? 32 : 10
        verticalAlignment: Text.AlignVCenter
        text: {
            var label = hostWindow.cleanText(modelData.text)
            if (overlayController.hasLeadingIndicator)
                label = label.replace(/^\s+/, "")
            return hostWindow.mnemonicText(label,
                                     modelData.hotkey)
        }
        textFormat: Text.StyledText
        color: modelData.disabled || modelData.header === true
               ? hostWindow.mutedText : hostWindow.textColor
        font.pixelSize: modelData.header === true ? 12 : 13
        font.bold: modelData.header === true
        visible: !modelData.separator
        elide: Text.ElideRight
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                   menuItemText, hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                   menuItemText, hostWindow.contentItem)
        }
    }

    IconImage {
        id: leadingMenuIcon
        objectName: "semanticMenuItemIcon-"
                    + hostWindow.cleanText(overlayController.frame.id)
                    + "-" + Number(modelData.index)
        readonly property string semanticIconName:
            modelData.checked === true ? "check"
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
        font.pixelSize: 12
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
