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
    property bool closing: false
    property bool componentReady: false
    property real revealProgress: 1
    property int openingSelectedIndex: 0
    property int closingSelectedIndex: 0
    property real closeContentShiftTarget: 0
    property bool dropdownOpenSettled: false
    property bool dropdownPresentationInitialized: false
    readonly property bool dropdownAnimationRunning:
        dropdownOpenAnimation.running || dropdownCloseAnimation.running
    signal closeAnimationFinished()

    readonly property bool dropdownMode:
        hostWindow.cleanText(frame.presentation) === "dropdown"
        && hostWindow.cleanText(frame.ownerId) !== ""
    readonly property var dropdownAnchor: {
        const revision = hostWindow.dropdownAnchorRevision
        return revision >= 0 && dropdownMode
                ? hostWindow.dropdownAnchorForId(frame.ownerId) : null
    }
    readonly property rect dropdownAnchorRect: {
        const anchor = dropdownAnchor
        if (anchor) {
            const mapped = anchor.mapToItem(hostWindow.contentItem, 0, 0)
            return Qt.rect(hostWindow.snapPx(mapped.x),
                           hostWindow.snapPx(mapped.y),
                           hostWindow.snapPx(anchor.width),
                           hostWindow.snapPx(anchor.height))
        }
        const fallbackHeight = hostWindow.dialogControlHeight
        return Qt.rect(hostWindow.snapPx(hostWindow.pxX(frame.x)),
                       hostWindow.snapPx(hostWindow.pxY(frame.y)
                                         - fallbackHeight),
                       hostWindow.snapPx(hostWindow.pxW(frame.w)),
                       hostWindow.snapPx(fallbackHeight))
    }
    readonly property bool fromMenuBar: frame.menuBarSubmenu === true
    readonly property bool hasParentMenu:
        hostWindow.cleanText(frame.parentId) !== ""
    readonly property int effectiveMenuIndex:
        fromMenuBar && hostWindow.menuBarPreviewIndex >= 0
        ? hostWindow.menuBarPreviewIndex
        : Number(hostWindow.menuBarModel.selected || 0)
    // Keep the model dependency visible to QML.  Looking the item up through
    // hostWindow.menuBarItem() hides the read from the binding engine; when a
    // popup is created in the same turn as the menu-bar update it then keeps a
    // null preview until an unrelated key event (such as Alt) re-evaluates it.
    readonly property var previewMenuItem: {
        if (!fromMenuBar)
            return null
        const items = hostWindow.menuBarModel.items || []
        for (var i = 0; i < items.length; ++i) {
            if (Number(items[i].index) === Number(effectiveMenuIndex))
                return items[i]
        }
        return null
    }
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
    readonly property real effectiveMenuRowHeight:
        dropdownMode ? dropdownAnchorRect.height : menuRowHeight
    // Section labels are intentionally separated from the preceding
    // item. They are menu chrome, not rows that need to align with a
    // leading icon or tag dot.
    readonly property real menuHeaderTopPadding: hostWindow.snapPx(6)
    readonly property real menuHeaderHeight:
        hostWindow.snapPx(Math.max(23, hostWindow.ch * 0.9))
        + menuHeaderTopPadding
    readonly property real menuSeparatorHeight: hostWindow.snapPx(11)
    readonly property real menuEdgeInset: hostWindow.snapPx(5)
    readonly property real menuContentHeight: {
        var height = 0
        for (var i = 0; i < effectiveItems.length; ++i) {
            height += effectiveItems[i].separator
                      ? menuSeparatorHeight
                      : effectiveItems[i].header === true
                        ? menuHeaderHeight : effectiveMenuRowHeight
        }
        return height
    }
    readonly property real dropdownMinimumY: hostWindow.snapPx(4)
    readonly property real dropdownMaximumY:
        hostWindow.snapPx(hostWindow.height - hostWindow.keyBarHeight() - 4)
    readonly property real dropdownHeightBeforeSelection:
        heightBeforeIndex(openingSelectedIndex)
    readonly property real dropdownSelectedHeight:
        itemHeightAt(openingSelectedIndex)
    readonly property real dropdownDesiredTop:
        dropdownAnchorRect.y - dropdownHeightBeforeSelection - menuEdgeInset
    readonly property real dropdownDesiredBottom:
        dropdownAnchorRect.y + dropdownSelectedHeight
        + menuContentHeight - dropdownHeightBeforeSelection
        - dropdownSelectedHeight + menuEdgeInset
    readonly property real dropdownOpenTop:
        hostWindow.snapPx(Math.max(dropdownMinimumY, dropdownDesiredTop))
    readonly property real dropdownOpenBottom:
        hostWindow.snapPx(Math.min(dropdownMaximumY, dropdownDesiredBottom))
    readonly property real dropdownOpenHeight:
        hostWindow.snapPx(Math.max(dropdownAnchorRect.height,
                                   dropdownOpenBottom - dropdownOpenTop))
    readonly property real dropdownViewportHeight:
        hostWindow.snapPx(Math.max(dropdownAnchorRect.height,
                                   dropdownOpenHeight - 2 * menuEdgeInset))
    readonly property real dropdownInitialContentY: {
        const wanted = dropdownOpenTop + menuEdgeInset
                + dropdownHeightBeforeSelection - dropdownAnchorRect.y
        return Math.max(0, Math.min(
            Math.max(0, menuContentHeight - dropdownViewportHeight), wanted))
    }
    readonly property real dropdownContentShift:
        dropdownMode && closing
        ? (1 - revealProgress) * closeContentShiftTarget : 0
    // The list has a five-pixel presentation inset on its left edge.  Make
    // the expanding frame one inset wider and move it left by that same
    // amount.  This keeps the selected row's text and the collapsed combo's
    // text on the exact same screen x while preserving the chevron's x.
    readonly property real dropdownFrameWidth:
        hostWindow.snapPx(dropdownAnchorRect.width + menuEdgeInset)
    readonly property real dropdownFrameX:
        hostWindow.snapPx(dropdownAnchorRect.x - menuEdgeInset)
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

    function boundedItemIndex(index) {
        return Math.max(0, Math.min(Math.max(0, effectiveItems.length - 1),
                                    Number(index || 0)))
    }

    function itemHeightAt(index) {
        if (index < 0 || index >= effectiveItems.length)
            return effectiveMenuRowHeight
        const item = effectiveItems[index]
        return item.separator ? menuSeparatorHeight
             : item.header === true ? menuHeaderHeight
             : effectiveMenuRowHeight
    }

    function heightBeforeIndex(index) {
        const end = Math.max(0, Math.min(effectiveItems.length,
                                         Number(index || 0)))
        var height = 0
        for (var i = 0; i < end; ++i)
            height += itemHeightAt(i)
        return height
    }

    function dropdownRowWindowY(index) {
        const row = popupMenuList.itemAtIndex(index)
        if (row)
            return row.mapToItem(hostWindow.contentItem, 0, 0).y
        return dropdownOpenTop + menuEdgeInset - popupMenuList.contentY
                + heightBeforeIndex(index)
    }

    function initializeDropdownPosition() {
        if (!dropdownMode || popupMenuList.count <= 0)
            return
        popupMenuList.contentY = dropdownInitialContentY
    }

    function beginDropdownOpening() {
        if (!dropdownMode)
            return
        dropdownCloseAnimation.stop()
        closeContentShiftTarget = 0
        dropdownOpenSettled = false
        initializeDropdownPosition()
        dropdownOpenAnimation.start()
    }

    function initializeDropdownPresentation() {
        if (!componentReady || !dropdownMode
                || dropdownPresentationInitialized)
            return
        dropdownPresentationInitialized = true
        openingSelectedIndex = boundedItemIndex(
                    Math.max(0, Number(frame.selected || 0)))
        closingSelectedIndex = openingSelectedIndex
        closeContentShiftTarget = 0
        revealProgress = 0
        dropdownOpenSettled = false
        syncListSelection()
        initializeDropdownPosition()
        if (closing)
            beginDropdownClosing()
        else
            beginDropdownOpening()
    }

    function beginDropdownClosing() {
        if (!dropdownMode) {
            closeAnimationFinished()
            return
        }
        dropdownOpenAnimation.stop()
        closingSelectedIndex = boundedItemIndex(visualSelectedIndex)
        closeContentShiftTarget = dropdownAnchorRect.y
                - dropdownRowWindowY(closingSelectedIndex)
        dropdownOpenSettled = false
        if (revealProgress <= 0.001) {
            closeAnimationFinished()
            return
        }
        dropdownCloseAnimation.start()
    }

    function syncFrameState() {
        semanticSelectedIndex = Math.max(0,
            Number(frame.selected || 0))
        semanticTopIndex = Math.max(0, Number(frame.top || 0))
    }

    function syncListSelection() {
        const wanted = Math.max(-1, Number(visualSelectedIndex))
        if (popupMenuList.currentIndex !== wanted)
            popupMenuList.currentIndex = wanted
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
        Qt.callLater(syncListSelection)
        if (!fromMenuBar)
            pointerSelectedIndex = -1
        else
            Qt.callLater(reconcilePointerState)
        initializeDropdownPresentation()
    }
    onDropdownModeChanged: {
        if (dropdownMode) {
            Qt.callLater(initializeDropdownPresentation)
            return
        }
        dropdownOpenAnimation.stop()
        dropdownCloseAnimation.stop()
        dropdownPresentationInitialized = false
        dropdownOpenSettled = false
        revealProgress = 1
    }
    onVisualSelectedIndexChanged: Qt.callLater(syncListSelection)
    onClosingChanged: {
        if (!componentReady || !dropdownMode)
            return
        if (closing)
            beginDropdownClosing()
        else
            beginDropdownOpening()
    }
    Component.onCompleted: {
        syncFrameState()
        componentReady = true
        Qt.callLater(syncListSelection)
        Qt.callLater(reconcilePointerState)
        initializeDropdownPresentation()
    }

    NumberAnimation {
        id: dropdownOpenAnimation
        target: menuOverlay
        property: "revealProgress"
        to: 1
        duration: 190
        easing.type: Easing.OutCubic
        onFinished: {
            if (!menuOverlay.closing) {
                menuOverlay.revealProgress = 1
                menuOverlay.dropdownOpenSettled = true
            }
        }
    }

    NumberAnimation {
        id: dropdownCloseAnimation
        target: menuOverlay
        property: "revealProgress"
        to: 0
        duration: 150
        easing.type: Easing.InCubic
        onFinished: {
            if (menuOverlay.closing) {
                menuOverlay.revealProgress = 0
                menuOverlay.closeAnimationFinished()
            }
        }
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
        return menuContentHeight + menuEdgeInset
               + (hostWindow.cleanText(frame.bottomHint) !== ""
                  ? hostWindow.ch : menuEdgeInset)
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

    function nativeTopIndex() {
        if (popupMenuList.count <= 0)
            return 0
        // indexAt() consumes content coordinates, while contentY is the
        // native viewport's current origin. Probe just inside its top edge so
        // separators and section headers participate in the same mapping as
        // ordinary rows.
        var index = popupMenuList.indexAt(
                    hostWindow.snapPx(1),
                    popupMenuList.contentY + hostWindow.snapPx(1))
        if (index >= 0)
            return index
        return Math.max(0, popupMenuList.count - 1)
    }

    function commitNativeScrollTop() {
        const top = nativeTopIndex()
        semanticTopIndex = top
        popupMenuList.positionViewAtIndex(top, ListView.Beginning)
        hostWindow.action({
            "target": menuOverlay.frame.id,
            "action": "menu.scroll",
            "top": top
        }, true)
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
        // Only the root popup owns the chain-wide backdrop. A child Loader is
        // stacked above its parent and also fills the window, so an enabled
        // backdrop here would intercept every pointer event intended for the
        // parent popup. The child's own popup surface remains interactive;
        // clicks elsewhere fall through to this chain's root backdrop.
        enabled: !menuOverlay.hasParentMenu && !menuOverlay.closing
        visible: enabled
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
        width: menuOverlay.dropdownMode
               ? menuOverlay.dropdownFrameWidth
               : hostWindow.snapPx(menuOverlay.preferredMenuWidth())
        height: menuOverlay.dropdownMode
                ? menuOverlay.dropdownAnchorRect.height
                  + (menuOverlay.dropdownOpenHeight
                     - menuOverlay.dropdownAnchorRect.height)
                    * menuOverlay.revealProgress
                : hostWindow.snapPx(Math.min(
                    hostWindow.height - hostWindow.keyBarHeight() - 8,
                    Math.max(hostWindow.ch + 10,
                             menuOverlay.preferredMenuHeight())))
        x: menuOverlay.dropdownMode
           ? menuOverlay.dropdownFrameX
           : hostWindow.snapPx(menuOverlay.preferredPopupX(width))
        y: menuOverlay.dropdownMode
           ? menuOverlay.dropdownAnchorRect.y
             + (menuOverlay.dropdownOpenTop
                - menuOverlay.dropdownAnchorRect.y)
               * menuOverlay.revealProgress
           : hostWindow.snapPx(menuOverlay.preferredPopupY(height))
        objectName: "semanticMenuPopup-"
                    + hostWindow.cleanText(menuOverlay.frame.id)
        color: hostWindow.dialogHeaderBg
        border.width: menuOverlay.dropdownMode
                      ? hostWindow.separatorWidth : 1
        border.color: hostWindow.controlBorder
        radius: menuOverlay.dropdownMode
                ? 4 + 3 * menuOverlay.revealProgress : 7
        clip: true
        enabled: !menuOverlay.closing
        z: 160

        ListView {
            id: popupMenuList
            objectName: "semanticMenuList-"
                        + hostWindow.cleanText(menuOverlay.frame.id)
            x: menuOverlay.menuEdgeInset
            y: menuOverlay.dropdownMode
               ? menuOverlay.dropdownOpenTop + menuOverlay.menuEdgeInset
                 - popupSurface.y
               : menuOverlay.menuEdgeInset
            width: Math.max(1, popupSurface.width - menuOverlay.menuEdgeInset)
            height: menuOverlay.dropdownMode
                    ? menuOverlay.dropdownViewportHeight
                    : Math.max(1, popupSurface.height
                               - menuOverlay.menuEdgeInset
                               - (hostWindow.cleanText(
                                      menuOverlay.frame.bottomHint) !== ""
                                  ? hostWindow.ch
                                  : menuOverlay.menuEdgeInset))
            // The list reaches the popup edge so its attached scrollbar can
            // sit flush right. Delegates retain the visual five-pixel inset.
            anchors.rightMargin: 0
            model: menuOverlay.effectiveItems
            clip: true
            // ListView resets currentIndex while installing a model. Keep the
            // visual cursor synchronized explicitly with the authoritative Go
            // menu selection instead of allowing that local reset to win.
            currentIndex: -1
            boundsBehavior: Flickable.StopAtBounds
            interactive: popupMenuScrollBar.nativeOverflow
            transform: Translate {
                y: menuOverlay.dropdownContentShift
            }

            function syncTopPosition() {
                if (menuOverlay.dropdownMode
                        && !menuOverlay.dropdownOpenSettled) {
                    menuOverlay.initializeDropdownPosition()
                    return
                }
                if (count > 0 && !popupMenuScrollBar.pressed)
                    positionViewAtIndex(menuOverlay.semanticTopIndex,
                                        ListView.Beginning)
            }

            Component.onCompleted: {
                Qt.callLater(menuOverlay.syncListSelection)
                Qt.callLater(syncTopPosition)
            }
            onModelChanged: {
                Qt.callLater(menuOverlay.syncListSelection)
                Qt.callLater(syncTopPosition)
            }
            onCountChanged: {
                if (menuOverlay.dropdownMode
                        && !menuOverlay.dropdownOpenSettled)
                    menuOverlay.initializeDropdownPosition()
            }

            delegate: SemanticMenuItemDelegate {
                hostWindow: menuOverlay.hostWindow
                overlayController: menuOverlay
                popupSurfaceItem: popupSurface
                popupList: popupMenuList
                scrollBar: popupMenuScrollBar
            }

            ScrollBar.vertical: F4ScrollBar {
                id: popupMenuScrollBar
                objectName: "semanticMenuScrollBar-"
                            + hostWindow.cleanText(menuOverlay.frame.id)
                hostWindow: menuOverlay.hostWindow
                thickness: 8
                readonly property bool nativeOverflow:
                    menuOverlay.menuContentHeight
                    > popupMenuList.height
                      + 0.5 / Math.max(1, menuOverlay.hostWindow.dpr)
                policy: nativeOverflow
                        ? ScrollBar.AlwaysOn : ScrollBar.AlwaysOff
                z: 3
                property bool nativeDragActive: false

                onPressedChanged: {
                    if (pressed) {
                        nativeDragActive = true
                    } else if (nativeDragActive) {
                        nativeDragActive = false
                        menuOverlay.commitNativeScrollTop()
                    }
                }
            }
        }

        IconLabel {
            id: dropdownChevronDown
            objectName: "semanticDropdownChevronDown-"
                        + hostWindow.cleanText(menuOverlay.frame.id)
            readonly property url rasterizedIconSource:
                hostWindow.lucideIconSource(
                    "chevron-down", 14, hostWindow.textColor)
            x: hostWindow.snapPx(parent.width - width - 10)
            y: menuOverlay.dropdownAnchorRect.y - popupSurface.y
               + hostWindow.snapPx(
                   (menuOverlay.dropdownAnchorRect.height - height) / 2)
            width: hostWindow.snapPx(14)
            height: hostWindow.snapPx(14)
            icon.source: rasterizedIconSource
            icon.width: hostWindow.snapPx(14)
            icon.height: hostWindow.snapPx(14)
            icon.color: hostWindow.textColor
            opacity: 1 - menuOverlay.revealProgress
            visible: menuOverlay.dropdownMode && opacity > 0
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                       dropdownChevronDown, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                       dropdownChevronDown, hostWindow.contentItem)
            }
            z: 5
        }

        IconLabel {
            id: dropdownChevronUp
            objectName: "semanticDropdownChevronUp-"
                        + hostWindow.cleanText(menuOverlay.frame.id)
            readonly property url rasterizedIconSource:
                hostWindow.lucideIconSource(
                    "chevron-up", 14, hostWindow.textColor)
            x: hostWindow.snapPx(parent.width - width - 10)
            y: menuOverlay.dropdownAnchorRect.y - popupSurface.y
               + hostWindow.snapPx(
                   (menuOverlay.dropdownAnchorRect.height - height) / 2)
            width: hostWindow.snapPx(14)
            height: hostWindow.snapPx(14)
            icon.source: rasterizedIconSource
            icon.width: hostWindow.snapPx(14)
            icon.height: hostWindow.snapPx(14)
            icon.color: hostWindow.textColor
            opacity: menuOverlay.revealProgress
            visible: menuOverlay.dropdownMode && opacity > 0
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                       dropdownChevronUp, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                       dropdownChevronUp, hostWindow.contentItem)
            }
            z: 5
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

    }
}
