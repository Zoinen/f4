pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.impl

// Native menu viewport and chrome. Selection and protocol state stay in the overlay.
Item {
    id: menuSurface
    required property Item menuOverlay
    readonly property var hostWindow: menuOverlay.hostWindow
    readonly property var menuBar: menuOverlay.menuBar
    property alias surfaceItem: popupSurface
    property alias listItem: popupMenuList
    property alias titleItem: popupMenuTitle
    anchors.fill: parent

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
        return y + menuOverlay.heightBeforeIndex(index)
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
        menuOverlay.semanticTopIndex = top
        popupMenuList.positionViewAtIndex(top, ListView.Beginning)
        hostWindow.action({
            "target": menuOverlay.frame.id,
            "action": "menu.scroll",
            "top": top
        }, true)
    }

    MouseArea {
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.topMargin: menuOverlay.fromMenuBar ? menuBar.windowBottom() : hostWindow.menuBarHeight
        anchors.bottomMargin: hostWindow.keyBarHeight()
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
        // Keep the displayed pointer selection until Go acknowledges close.
        // Clearing it on press exposes the older semantic row while IPC runs.
        onWheel: (wheel) => { wheel.accepted = true }
    }

    Rectangle {
        id: popupSurface
        parent: menuSurface.menuOverlay
        width: windowChrome.item ? windowChrome.item.width : menuOverlay.dropdownMode
               ? menuOverlay.dropdownFrameWidth
               : menuOverlay.frame.presentation === "fullWidth" ? hostWindow.snapPx(hostWindow.width)
               : hostWindow.snapPx(menuOverlay.preferredMenuWidth())
        height: windowChrome.item ? windowChrome.item.height : menuOverlay.dropdownMode
                ? menuOverlay.dropdownAnchorRect.height
                  + (menuOverlay.dropdownOpenHeight
                     - menuOverlay.dropdownAnchorRect.height)
                    * menuOverlay.revealProgress
                : hostWindow.snapPx(Math.min(
                    hostWindow.height - hostWindow.keyBarHeight() - 8,
                    Math.max(hostWindow.ch + 10,
                             menuOverlay.preferredMenuHeight())))
        x: windowChrome.item ? windowChrome.item.x : menuOverlay.dropdownMode
           ? menuOverlay.dropdownFrameX
           : menuOverlay.frame.presentation === "fullWidth" ? 0
           : hostWindow.snapPx(menuOverlay.preferredPopupX(width))
        y: windowChrome.item ? windowChrome.item.y : menuOverlay.dropdownMode
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

        Text {
            id: popupMenuTitle
            objectName: "semanticMenuTitle-"
                        + hostWindow.cleanText(menuOverlay.frame.id)
            visible: menuOverlay.showMenuTitle && !menuOverlay.windowMode
            x: hostWindow.snapPx(12)
            y: hostWindow.snapPx(menuOverlay.menuEdgeInset
                                + (menuOverlay.menuTitleHeight - height) / 2)
            width: Math.max(0, parent.width - 2 * hostWindow.snapPx(12))
            height: Math.ceil(implicitHeight * hostWindow.dpr) / hostWindow.dpr
            text: menuOverlay.menuTitleText
            textFormat: Text.PlainText
            color: hostWindow.textColor
            font.family: hostWindow.font.family
            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(14) : 14)
            font.weight: Font.DemiBold
            elide: Text.ElideMiddle
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(popupMenuTitle, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(popupMenuTitle, hostWindow.contentItem)
            }
        }

        ListView {
            id: popupMenuList
            objectName: "semanticMenuList-"
                        + hostWindow.cleanText(menuOverlay.frame.id)
            x: menuOverlay.dropdownMode
               ? menuOverlay.dropdownListX : menuOverlay.menuEdgeInset
            y: menuOverlay.dropdownMode
               ? menuOverlay.dropdownOpenTop + menuOverlay.menuEdgeInset
                 - popupSurface.y
               : menuOverlay.menuEdgeInset + menuOverlay.menuTitleHeight
            width: Math.max(1, popupSurface.width
                               - (menuOverlay.dropdownMode
                                  ? menuOverlay.dropdownListX
                                  : menuOverlay.menuEdgeInset))
            height: menuOverlay.dropdownMode
                    ? menuOverlay.dropdownViewportHeight
                    : Math.max(1, popupSurface.height
                               - menuOverlay.menuEdgeInset
                               - menuOverlay.menuTitleHeight
                               - menuOverlay.menuFooterHeight)
            // The list reaches the popup edge so its attached scrollbar can
            // sit flush right. Delegates retain the visual five-pixel inset.
            anchors.rightMargin: 0
            model: menuOverlay.effectiveItems
            reuseItems: historyMenu
            // Flickable's pixelAligned means logical pixels, which is the
            // wrong grid at fractional DPR. History rows use physical pixels.
            pixelAligned: !historyMenu
            readonly property bool historyMenu: menuOverlay.historyMenu
            property bool initialPositionReady: false
            opacity: historyMenu && !initialPositionReady ? 0 : 1
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

            function syncTopPosition(revealSelection = true, applyTopHint = true) {
                if (menuOverlay.dropdownMode
                        && !menuOverlay.dropdownOpenSettled) {
                    menuOverlay.initializeDropdownPosition()
                    return
                }
                if (count > 0 && !popupMenuScrollBar.pressed) {
                    if (applyTopHint)
                        positionViewAtIndex(menuOverlay.semanticTopIndex,
                                            ListView.Beginning)
                    // Console top/viewHeight do not include native title and
                    // row metrics. Use its hint for opening/explicit scrolling,
                    // but preserve the native viewport on keyboard selection.
                    // Reveal the cursor only when it leaves that viewport.
                    // Scroll-only acknowledgements must not pull the user back.
                    if (revealSelection) {
                        const visibleRow = itemAtIndex(menuOverlay.semanticSelectedIndex)
                        if (visibleRow && visibleRow.y >= contentY - .01
                                && visibleRow.y + visibleRow.height <= contentY + height + .01)
                            return
                        positionViewAtIndex(menuOverlay.semanticSelectedIndex,
                                            ListView.Contain)
                        if (historyMenu)
                            forceLayout()
                        // Qt's positioning can round contentY to logical
                        // pixels. Round outward so a fractional-DPR last row
                        // is not clipped by the remaining fraction of a pixel.
                        const row = itemAtIndex(menuOverlay.semanticSelectedIndex)
                        // Contain rounds to logical pixels. At 175%, its
                        // leading-edge remainder varies by row, making the
                        // first line jump even after the glyphs are snapped.
                        if (historyMenu && row && Math.abs(row.y - contentY) <= 1.01)
                            contentY = row.y
                        else if (historyMenu && row
                                 && Math.abs(row.y + row.height - contentY - height) <= 1.01)
                            contentY = row.y + row.height - height
                        else if (row && row.y + row.height > contentY + height)
                            contentY = Math.ceil((row.y + row.height - height)
                                                * hostWindow.dpr) / hostWindow.dpr
                        else if (row && row.y < contentY)
                            contentY = Math.floor(row.y * hostWindow.dpr) / hostWindow.dpr
                    }
                }
            }

            function prepareInitialPosition() {
                if (!menuOverlay.componentReady || height <= 0)
                    return
                // Realize row geometry before positioning; never expose the
                // default top-of-list viewport while waiting for ListView polish.
                forceLayout()
                menuOverlay.syncListSelection()
                syncTopPosition()
                initialPositionReady = true
            }

            onHeightChanged: {
                if (!initialPositionReady)
                    Qt.callLater(prepareInitialPosition)
                else
                    Qt.callLater(syncTopPosition)
            }

            Component.onCompleted: {
                Qt.callLater(prepareInitialPosition)
            }
            onModelChanged: {
                initialPositionReady = false
                Qt.callLater(prepareInitialPosition)
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
                thickness: menuOverlay.historyMenu ? 16 : 8
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
            x: menuOverlay.dropdownAnchorIndicatorRect.x - popupSurface.x
            y: menuOverlay.dropdownAnchorIndicatorRect.y - popupSurface.y
            width: menuOverlay.dropdownAnchorIndicatorRect.width
            height: menuOverlay.dropdownAnchorIndicatorRect.height
            icon.source: rasterizedIconSource
            icon.width: hostWindow.snapPx(14)
            icon.height: hostWindow.snapPx(14)
            icon.color: hostWindow.textColor
            opacity: 1 - menuOverlay.revealProgress
            visible: menuOverlay.dropdownMode
                     && menuOverlay.dropdownAnchorReady && opacity > 0
            z: 5
        }

        IconLabel {
            id: dropdownChevronUp
            objectName: "semanticDropdownChevronUp-"
                        + hostWindow.cleanText(menuOverlay.frame.id)
            readonly property url rasterizedIconSource:
                hostWindow.lucideIconSource(
                    "chevron-up", 14, hostWindow.textColor)
            x: menuOverlay.dropdownAnchorIndicatorRect.x - popupSurface.x
            y: menuOverlay.dropdownAnchorIndicatorRect.y - popupSurface.y
            width: menuOverlay.dropdownAnchorIndicatorRect.width
            height: menuOverlay.dropdownAnchorIndicatorRect.height
            icon.source: rasterizedIconSource
            icon.width: hostWindow.snapPx(14)
            icon.height: hostWindow.snapPx(14)
            icon.color: hostWindow.textColor
            opacity: menuOverlay.revealProgress
            visible: menuOverlay.dropdownMode
                     && menuOverlay.dropdownAnchorReady && opacity > 0
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
            id: popupMenuBottomHint
            objectName: "semanticMenuBottomHint-"
                        + hostWindow.cleanText(menuOverlay.frame.id)
            // Center the leaf itself, not the glyphs inside an odd-width text
            // box: both the origin and native glyph translation must be static.
            width: Math.max(0, Math.min(
                parent.width - 2 * hostWindow.snapPx(8),
                Math.ceil(implicitWidth * hostWindow.dpr) / hostWindow.dpr))
            height: Math.ceil(implicitHeight * hostWindow.dpr) / hostWindow.dpr
            x: hostWindow.snapPx((parent.width - width) / 2)
            y: hostWindow.snapPx(parent.height - menuOverlay.menuFooterHeight
                                + (menuOverlay.menuFooterHeight - height) / 2)
            text: hostWindow.cleanText(menuOverlay.frame.bottomHint)
            textFormat: Text.PlainText
            color: hostWindow.mutedText
            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(11) : 11)
            elide: Text.ElideMiddle
            visible: text !== ""
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(popupMenuBottomHint, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(popupMenuBottomHint, hostWindow.contentItem)
            }
        }

    }

    Loader {
        id: windowChrome
        parent: menuSurface.menuOverlay
        active: menuOverlay.windowMode
        z: 161
        sourceComponent: GenericDialog {
            hostWindow: menuOverlay.hostWindow
            menuBar: menuOverlay.menuBar
            externalBody: true
            frame: Object.assign({}, menuOverlay.frame, {showClose: true, showZoom: true})
            closeAction: "menu.close"
            geometryAction: "menu.geometry"
            geometryLeft: 0
            geometryRight: hostWindow.width
            preferredWidth: menuOverlay.frame.presentation === "fullWidth"
                ? availableWidth : Math.min(availableWidth, menuOverlay.preferredMenuWidth())
            preferredHeight: Math.min(availableHeight, menuOverlay.preferredMenuHeight())
        }
    }
}
