pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.impl

Item {
    id: menuOverlay
    required property ApplicationWindow hostWindow
    required property Item menuBar
    required property Item semanticLayer
    required property QtObject shellController
    anchors.fill: parent
    property var frame: ({})
    readonly property bool fromMenuBar: frame.menuBarSubmenu === true
    readonly property int effectiveMenuIndex:
        fromMenuBar && hostWindow.menuBarPreviewIndex >= 0
        ? hostWindow.menuBarPreviewIndex
        : Number(hostWindow.menuBarModel.selected || 0)
    readonly property var previewMenuItem:
        fromMenuBar ? hostWindow.menuBarItem(effectiveMenuIndex) : null
    readonly property var effectiveItems:
        previewMenuItem && previewMenuItem.items
        ? previewMenuItem.items : (frame.items || [])
    readonly property bool previewIsAhead:
        fromMenuBar && previewMenuItem
        && effectiveMenuIndex
           !== Number(hostWindow.menuBarModel.selected || 0)
    readonly property bool hasLeadingIndicator: {
        for (var i = 0; i < effectiveItems.length; ++i) {
            if (effectiveItems[i].checked === true
                    || hostWindow.cleanText(effectiveItems[i].icon) !== ""
                    || hostWindow.cleanText(effectiveItems[i].iconColor) !== "")
                return true
        }
        return false
    }
    readonly property real menuRowHeight:
        hostWindow.snapPx(Math.max(27, hostWindow.ch * 1.02))
    // Section labels are intentionally separated from the preceding
    // item. They are menu chrome, not rows that need to align with a
    // leading icon or tag dot.
    readonly property real menuHeaderTopPadding: hostWindow.snapPx(6)
    readonly property real menuHeaderHeight:
        hostWindow.snapPx(Math.max(23, hostWindow.ch * 0.9))
        + menuHeaderTopPadding
    readonly property real menuSeparatorHeight: hostWindow.snapPx(11)
    property int pointerSelectedIndex: -1
    property int semanticSelectedIndex: 0
    property int semanticTopIndex: 0
    property bool pointerWindowPositionKnown: false
    property real pointerWindowX: 0
    property real pointerWindowY: 0
    readonly property int activePointerSelectedIndex:
        fromMenuBar
        && hostWindow.menuPointerMenuIndex === effectiveMenuIndex
        ? hostWindow.menuPointerItemIndex : pointerSelectedIndex
    readonly property int visualSelectedIndex:
        activePointerSelectedIndex >= 0
        ? activePointerSelectedIndex
        : fromMenuBar && hostWindow.menuBarOpenedByPointer
          && !hostWindow.menuBarPointerHasSelectedItem ? -1
        : previewIsAhead ? 0
        : semanticSelectedIndex

    function syncFrameState() {
        semanticSelectedIndex = Math.max(0,
            Number(frame.selected || 0))
        semanticTopIndex = Math.max(0, Number(frame.top || 0))
    }

    function applyCommandMenuStates(states) {
        const frameId = String(frame.id || "")
        for (var i = 0; i < states.length; ++i) {
            if (String(states[i].id || "") !== frameId)
                continue
            semanticSelectedIndex = Math.max(0,
                Number(states[i].selected || 0))
            semanticTopIndex = Math.max(0,
                Number(states[i].top || 0))
            if (!fromMenuBar)
                pointerSelectedIndex = -1
            else
                Qt.callLater(reconcilePointerState)
            Qt.callLater(popupMenuList.syncTopPosition)
            return
        }
    }

    function pointerActuallyMoved(area, mouse) {
        // MouseArea.positionChanged is expressed in delegate-local
        // coordinates. Qt also emits it when ListView moves that
        // delegate underneath a completely stationary cursor (for
        // example after keyboard selection scrolls the menu). Compare
        // in the stable window coordinate space so only a real mouse
        // move may take selection ownership away from the keyboard.
        const point = area.mapToItem(hostWindow.contentItem,
                                     mouse.x, mouse.y)
        const moved = pointerWindowPositionKnown
                && (Math.abs(point.x - pointerWindowX) >= 0.5
                    || Math.abs(point.y - pointerWindowY) >= 0.5)
        pointerWindowX = point.x
        pointerWindowY = point.y
        pointerWindowPositionKnown = true
        return moved
    }

    function reconcilePointerSelection() {
        if (!fromMenuBar || previewIsAhead
                || hostWindow.menuPointerMenuIndex !== effectiveMenuIndex
                || hostWindow.menuPointerItemIndex < 0
                || hostWindow.menuPointerSentItemIndex
                   !== hostWindow.menuPointerItemIndex
                || semanticSelectedIndex
                   !== hostWindow.menuPointerItemIndex)
            return
        // Go now owns exactly the row already painted by QML. Dropping
        // the local override is visually lossless and lets the next
        // keyboard Up/Down scene become authoritative immediately.
        hostWindow.clearMenuPointerSelection()
    }

    function retargetPointerSelection() {
        if (!fromMenuBar || previewIsAhead
                || hostWindow.menuPointerMenuIndex !== effectiveMenuIndex
                || hostWindow.menuPointerItemIndex < 0)
            return
        var frameId = String(frame.id || "")
        if (frameId === "" || hostWindow.menuPointerFrameId === frameId)
            return
        // A locally previewed top-level menu can receive pointer input
        // before Go has replaced the old submenu frame. Once the
        // matching frame arrives, retarget the pending row selection
        // instead of sending it with the stale popup id.
        hostWindow.menuPointerFrameId = frameId
        hostWindow.menuPointerSentItemIndex = -1
        hostWindow.scheduleMenuPointerSync()
    }

    function reconcilePointerState() {
        reconcilePointerSelection()
        // An exact acknowledgement clears the local state above. If
        // it was not an acknowledgement, keep the user's hovered row
        // and bind it to the newly materialized submenu frame.
        retargetPointerSelection()
    }

    onFrameChanged: {
        syncFrameState()
        if (!fromMenuBar)
            pointerSelectedIndex = -1
        else
            Qt.callLater(reconcilePointerState)
    }
    Component.onCompleted: {
        syncFrameState()
        Qt.callLater(reconcilePointerState)
    }

    Connections {
        target: menuOverlay.shellController
        ignoreUnknownSignals: true
        function onCommandMenuStatesChanged(states) {
            menuOverlay.applyCommandMenuStates(states)
        }
    }

    FontMetrics {
        id: popupMenuMetrics
        font.pixelSize: 13
    }

    function preferredMenuWidth() {
        if (!fromMenuBar || !previewMenuItem) {
            var semanticWidth = hostWindow.pxW(frame.w)
            return Math.min(hostWindow.width - 8, Math.max(150, semanticWidth))
        }
        var preferred = 150
        for (var i = 0; i < effectiveItems.length; ++i) {
            var item = effectiveItems[i]
            var width = popupMenuMetrics.advanceWidth(
                            hostWindow.cleanText(item.text)) + 32
            if (hostWindow.cleanText(item.shortcut) !== "")
                width += popupMenuMetrics.advanceWidth(
                             hostWindow.cleanText(item.shortcut)) + 24
            preferred = Math.max(preferred, width)
        }
        return Math.min(hostWindow.width - 12, preferred)
    }

    function preferredMenuHeight() {
        var height = 10
        for (var i = 0; i < effectiveItems.length; ++i) {
            height += effectiveItems[i].separator
                      ? menuSeparatorHeight
                      : effectiveItems[i].header === true
                        ? menuHeaderHeight : menuRowHeight
        }
        if (hostWindow.cleanText(frame.bottomHint) !== "")
            height += hostWindow.ch
        return height
    }

    function popupWindowX() { return popupSurface.x }
    function popupWindowY() { return popupSurface.y }
    function popupWindowWidth() { return popupSurface.width }

    function rowWindowY(index) {
        var row = popupMenuList.itemAtIndex(index)
        if (row) {
            var mapped = row.mapToItem(hostWindow.contentItem, 0, 0)
            return mapped.y
        }
        var y = popupSurface.y + popupMenuList.y
                - popupMenuList.contentY
        for (var i = 0; i < effectiveItems.length && i < index; ++i) {
            y += effectiveItems[i].separator
                 ? menuSeparatorHeight
                 : effectiveItems[i].header === true
                   ? menuHeaderHeight : menuRowHeight
        }
        return y
    }

    function preferredPopupX(popupWidth) {
        var parentMenu = hostWindow.menuOverlayForId(frame.parentId)
        if (parentMenu) {
            var right = parentMenu.popupWindowX()
                        + parentMenu.popupWindowWidth() - 1
            if (right + popupWidth > hostWindow.width - 4)
                right = parentMenu.popupWindowX() - popupWidth + 1
            return Math.max(4, Math.min(hostWindow.width - popupWidth - 4,
                                        right))
        }
        if (previewMenuItem)
            return menuBar.itemWindowX(effectiveMenuIndex)
        return Math.max(4, Math.min(hostWindow.width - popupWidth - 4,
                                    hostWindow.pxX(frame.x)))
    }

    function preferredPopupY(popupHeight) {
        var parentMenu = hostWindow.menuOverlayForId(frame.parentId)
        var desired = parentMenu
                ? parentMenu.rowWindowY(Number(frame.anchorIndex || 0))
                : fromMenuBar ? menuBar.windowBottom()
                              : hostWindow.pxY(frame.y)
        var minimum = fromMenuBar ? menuBar.windowBottom() : 4
        var maximum = hostWindow.height - hostWindow.keyBarHeight()
                      - popupHeight - 4
        return Math.max(minimum, Math.min(maximum, desired))
    }

    MouseArea {
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.topMargin: menuOverlay.fromMenuBar ? menuBar.height : 0
        acceptedButtons: Qt.AllButtons
        hoverEnabled: true
        preventStealing: true
        onClicked: hostWindow.action({
            "target": menuOverlay.frame.id,
            "action": "menu.closeChain"
        })
        onPressed: {
            hostWindow.menuBarPreviewIndex = -1
            hostWindow.clearMenuPointerSelection()
        }
        onWheel: (wheel) => { wheel.accepted = true }
    }

    Rectangle {
        id: popupSurface
        width: hostWindow.snapPx(menuOverlay.preferredMenuWidth())
        height: hostWindow.snapPx(Math.min(hostWindow.height - hostWindow.keyBarHeight() - 8,
                                     Math.max(hostWindow.ch + 10,
                                              menuOverlay.preferredMenuHeight())))
        x: hostWindow.snapPx(menuOverlay.preferredPopupX(width))
        y: hostWindow.snapPx(menuOverlay.preferredPopupY(height))
        objectName: "semanticMenuPopup-"
                    + hostWindow.cleanText(menuOverlay.frame.id)
        color: hostWindow.dialogHeaderBg
        border.width: 1
        border.color: hostWindow.controlBorder
        radius: 7
        clip: true
        z: 160

        ListView {
            id: popupMenuList
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.bottom: parent.bottom
            anchors.topMargin: 5
            anchors.bottomMargin: hostWindow.cleanText(menuOverlay.frame.bottomHint) !== ""
                                  ? hostWindow.ch : 5
            anchors.leftMargin: 5
            anchors.rightMargin: 5
            model: menuOverlay.effectiveItems
            clip: true
            currentIndex: menuOverlay.visualSelectedIndex
            boundsBehavior: Flickable.StopAtBounds
            interactive: contentHeight > height

            function syncTopPosition() {
                if (count > 0)
                    positionViewAtIndex(menuOverlay.semanticTopIndex,
                                        ListView.Beginning)
            }

            Component.onCompleted: Qt.callLater(syncTopPosition)
            onModelChanged: Qt.callLater(syncTopPosition)

            delegate: Rectangle {
                required property var modelData
                objectName: "semanticMenuItem-"
                            + hostWindow.cleanText(menuOverlay.frame.id)
                            + "-" + Number(modelData.index)
                width: ListView.view.width
                height: modelData.separator
                        ? menuOverlay.menuSeparatorHeight
                        : modelData.header === true
                          ? menuOverlay.menuHeaderHeight
                          : menuOverlay.menuRowHeight
                radius: 4
                color: modelData.index === menuOverlay.visualSelectedIndex
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
                    objectName: "semanticMenuItemText-"
                                + hostWindow.cleanText(menuOverlay.frame.id)
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
                                       ? menuOverlay.menuHeaderTopPadding
                                       : 0
                    anchors.leftMargin: modelData.header === true
                                        ? 10
                                        : menuOverlay.hasLeadingIndicator
                                        ? 32 : 10
                    verticalAlignment: Text.AlignVCenter
                    text: {
                        var label = hostWindow.cleanText(modelData.text)
                        if (menuOverlay.hasLeadingIndicator)
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
                }

                IconImage {
                    id: leadingMenuIcon
                    objectName: "semanticMenuItemIcon-"
                                + hostWindow.cleanText(menuOverlay.frame.id)
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
                        popupSurface.x + popupSurface.y
                        + popupMenuList.contentY + parent.y
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
                                + hostWindow.cleanText(menuOverlay.frame.id)
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
                        popupSurface.x + popupSurface.y
                        + popupMenuList.contentY + parent.y
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
                                + hostWindow.cleanText(menuOverlay.frame.id)
                                + "-" + Number(modelData.index)
                    x: hostWindow.snapPx(parent.width - width - 9)
                    y: 0
                    width: hostWindow.snapPx(15)
                    height: hostWindow.snapPx(parent.height)
                    text: "›"
                    color: modelData.disabled ? hostWindow.mutedText : hostWindow.textColor
                    font.pixelSize: 17
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                    property real alignmentRevision:
                        popupSurface.x + popupSurface.y
                        + popupMenuList.contentY + parent.y
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
                    anchors.rightMargin: modelData.hasSubmenu === true ? 28 : 10
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
                        "target": menuOverlay.frame.id,
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
                        if (menuOverlay.fromMenuBar) {
                            hostWindow.menuBarPointerHasSelectedItem = true
                            if (hostWindow.menuPointerItemIndex < 0
                                    && !menuOverlay.previewIsAhead
                                    && menuOverlay.semanticSelectedIndex
                                       === modelData.index)
                                return
                            if (hostWindow.menuPointerMenuIndex
                                    === menuOverlay.effectiveMenuIndex
                                    && hostWindow.menuPointerItemIndex
                                       === modelData.index)
                                return
                            hostWindow.menuPointerMenuIndex
                                    = menuOverlay.effectiveMenuIndex
                            hostWindow.menuPointerItemIndex = modelData.index
                            hostWindow.menuPointerFrameId
                                    = String(menuOverlay.frame.id || "")
                            hostWindow.scheduleMenuPointerSync()
                        } else {
                            if (menuOverlay.pointerSelectedIndex
                                    === modelData.index)
                                return
                            menuOverlay.pointerSelectedIndex = modelData.index
                            hostWindow.action({
                                "target": menuOverlay.frame.id,
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
                                && menuOverlay.pointerActuallyMoved(
                                    itemMouse, mouse))
                            selectFromPointer()
                    }
                    onEntered: {
                        if (modelData.hasSubmenu === true)
                            submenuHoverTimer.restart()
                    }
                    onExited: submenuHoverTimer.stop()
                    onClicked: {
                        submenuHoverTimer.stop()
                        if (menuOverlay.fromMenuBar) {
                            hostWindow.action({
                                "action": "menuBar.itemActivate",
                                "menuIndex": menuOverlay.effectiveMenuIndex,
                                "index": modelData.index
                            }, true)
                        } else {
                            hostWindow.action({
                                "target": menuOverlay.frame.id,
                                "action": "menu.activate",
                                "index": modelData.index
                            }, true)
                        }
                        hostWindow.menuBarPreviewIndex = -1
                        hostWindow.clearMenuPointerSelection()
                    }
                }
            }
        }

        MouseArea {
            anchors.fill: popupMenuList
            acceptedButtons: Qt.NoButton
            onWheel: (wheel) => {
                var delta = wheel.angleDelta.y > 0 ? -1 : 1
                hostWindow.action({
                    "target": menuOverlay.frame.id,
                    "action": "menu.scroll",
                    "delta": delta
                }, true)
                wheel.accepted = true
            }
        }

        Text {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: hostWindow.ch
            anchors.leftMargin: 8
            anchors.rightMargin: 8
            text: hostWindow.cleanText(menuOverlay.frame.bottomHint)
            color: hostWindow.mutedText
            font.pixelSize: 11
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideMiddle
            visible: text !== ""
        }

        Rectangle {
            readonly property int itemCount: menuOverlay.effectiveItems.length
            readonly property int pageSize: Math.max(1, Number(menuOverlay.frame.viewHeight || itemCount))
            anchors.right: parent.right
            anchors.rightMargin: 2
            width: 3
            height: Math.max(12, popupMenuList.height * Math.min(1, pageSize / Math.max(1, itemCount)))
            y: popupMenuList.y + (popupMenuList.height - height)
               * Math.max(0, Number(menuOverlay.frame.top || 0))
               / Math.max(1, itemCount - pageSize)
            radius: 2
            color: hostWindow.mutedText
            visible: itemCount > pageSize
            opacity: 0.75
        }
    }
}
