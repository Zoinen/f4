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
    // Do not paint a guessed chevron while the owning combo is still being
    // incubated.  The guess is often one physical pixel away from the final
    // indicator at 175% DPR, so showing it first creates a visible jump when
    // the anchor registration arrives a turn later.
    property bool dropdownAnchorIndicatorReady: false
    property rect dropdownAnchorIndicatorStableRect: Qt.rect(0, 0, 0, 0)
    property bool dropdownAnchorGeometryReady: false
    property rect dropdownAnchorStableRect: Qt.rect(0, 0, 0, 0)
    readonly property bool dropdownAnchorReady:
        dropdownAnchor !== null && dropdownAnchorIndicatorReady
    function fallbackDropdownAnchorRect() {
        const fallbackHeight = hostWindow.dialogControlHeight
        return Qt.rect(hostWindow.snapPx(hostWindow.pxX(frame.x)),
                       hostWindow.snapPx(hostWindow.pxY(frame.y)
                                         - fallbackHeight),
                       hostWindow.snapPx(hostWindow.pxW(frame.w)),
                       hostWindow.snapPx(fallbackHeight))
    }
    readonly property rect dropdownAnchorRect: {
        if (dropdownAnchorGeometryReady)
            return dropdownAnchorStableRect
        const anchor = dropdownAnchor
        if (anchor) {
            const mapped = anchor.mapToItem(hostWindow.contentItem, 0, 0)
            return Qt.rect(hostWindow.snapPx(mapped.x),
                           hostWindow.snapPx(mapped.y),
                           hostWindow.snapPx(anchor.width),
                           hostWindow.snapPx(anchor.height))
        }
        return fallbackDropdownAnchorRect()
    }
    // Read the indicator after the combo template has laid it out, then snap
    // its *scene* position once. The stable value is deliberately retained
    // while the native ComboBox template is being reparented (for example
    // when the host window loses activation). During that short interval
    // mapToItem can report (0, 0); publishing that transient result is what
    // makes the expanded chevron jump to the popup's top-left corner.
    readonly property rect dropdownAnchorIndicatorRect: {
        if (dropdownAnchorIndicatorReady)
            return dropdownAnchorIndicatorStableRect
        const size = hostWindow.snapPx(14)
        const rightInset = hostWindow.snapPx(10)
        return Qt.rect(
            hostWindow.snapPx(dropdownAnchorRect.x
                              + hostWindow.snapPx(dropdownAnchorRect.width - size
                                                  - rightInset)),
            hostWindow.snapPx(dropdownAnchorRect.y
                              + (dropdownAnchorRect.height - size) / 2),
            size, size)
    }

    function refreshDropdownAnchorIndicator() {
        // QQuickItem mapping is not stable while the native window is
        // deactivated.  In particular, the ComboBox template can briefly
        // report its indicator at (0, 0) while it is detached from the scene
        // graph.  Keep the last coherent presentation geometry until the
        // window is active again instead of publishing that transient point.
        if (!hostWindow || !hostWindow.active)
            return
        const anchor = dropdownAnchor
        if (!anchor || !anchor.parent || !hostWindow.contentItem
                || typeof anchor.mapToItem !== "function")
            return
        const anchorMapped = anchor.mapToItem(hostWindow.contentItem, 0, 0)
        const anchorX = Number(anchorMapped.x)
        const anchorY = Number(anchorMapped.y)
        const anchorWidth = Number(anchor.width)
        const anchorHeight = Number(anchor.height)
        if (!isFinite(anchorX) || !isFinite(anchorY)
                || !isFinite(anchorWidth) || !isFinite(anchorHeight)
                || anchorWidth <= 0 || anchorHeight <= 0)
            return
        const nextAnchor = Qt.rect(hostWindow.snapPx(anchorX),
                                   hostWindow.snapPx(anchorY),
                                   hostWindow.snapPx(anchorWidth),
                                   hostWindow.snapPx(anchorHeight))
        const previousAnchor = dropdownAnchorStableRect
        const anchorChanged = !dropdownAnchorGeometryReady
                || Math.abs(previousAnchor.x - nextAnchor.x) > 0.0001
                || Math.abs(previousAnchor.y - nextAnchor.y) > 0.0001
                || Math.abs(previousAnchor.width - nextAnchor.width) > 0.0001
                || Math.abs(previousAnchor.height - nextAnchor.height) > 0.0001
        if (anchorChanged)
            dropdownAnchorStableRect = nextAnchor
        dropdownAnchorGeometryReady = true

        const indicator = anchor ? anchor.indicator : null
        if (!indicator || !indicator.parent || !hostWindow.contentItem
                || typeof indicator.mapToItem !== "function"
                || Number(indicator.width) <= 0
                || Number(indicator.height) <= 0)
            return
        const mapped = indicator.mapToItem(hostWindow.contentItem, 0, 0)
        const x = Number(mapped.x)
        const y = Number(mapped.y)
        const width = Number(indicator.width)
        const height = Number(indicator.height)
        if (!isFinite(x) || !isFinite(y) || !isFinite(width)
                || !isFinite(height) || width <= 0 || height <= 0)
            return
        // A detached ComboBox can briefly map both the indicator and its
        // owner to (0, 0) while the host window is being deactivated.  That
        // pair is finite and positive, but it is not a usable presentation
        // coordinate.  The indicator is right-aligned by F4ComboBox, so it
        // must remain inside the owner and in its trailing half.  Rejecting
        // impossible mappings keeps the last coherent rect instead of
        // moving the chevron to the popup's top-left corner.
        const tolerance = hostWindow.snapPx(1)
        const insideAnchor = x >= anchorX - tolerance
                && y >= anchorY - tolerance
                && x + width <= anchorX + anchorWidth + tolerance
                && y + height <= anchorY + anchorHeight + tolerance
        const inTrailingHalf = x + width / 2
                >= anchorX + anchorWidth / 2 - tolerance
        if (!insideAnchor || !inTrailingHalf)
            return
        const next = Qt.rect(hostWindow.snapPx(x),
                             hostWindow.snapPx(y),
                             hostWindow.snapPx(width),
                             hostWindow.snapPx(height))
        const previous = dropdownAnchorIndicatorStableRect
        const changed = !dropdownAnchorIndicatorReady
                || Math.abs(previous.x - next.x) > 0.0001
                || Math.abs(previous.y - next.y) > 0.0001
                || Math.abs(previous.width - next.width) > 0.0001
                || Math.abs(previous.height - next.height) > 0.0001
        if (changed)
            dropdownAnchorIndicatorStableRect = next
        dropdownAnchorIndicatorReady = true
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
                    || (effectiveItems[i].details && effectiveItems[i].details.icon)
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
            height += itemHeightAt(i)
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
    // The list has a presentation inset on both sides of the expanded
    // control.  Keep the selected row exactly the collapsed control's size,
    // then leave one inset visible at the popup's right edge as well.  The
    // frame is moved left by the leading inset so the selected row remains
    // anchored to the collapsed control's screen x.
    readonly property real dropdownFrameWidth:
        hostWindow.snapPx(dropdownAnchorRect.width + 2 * menuEdgeInset)
    readonly property real dropdownFrameX:
        hostWindow.snapPx(dropdownAnchorRect.x - menuEdgeInset)
    readonly property real dropdownListX:
        // Both values are already snapped to the same physical grid.  Taking
        // their difference keeps the first delegate's left edge exactly at
        // the collapsed control's left edge instead of introducing a second
        // rounding error.
        hostWindow.snapPx(dropdownAnchorRect.x - dropdownFrameX)
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
             : item.details ? driveRowHeight : effectiveMenuRowHeight
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
        Qt.callLater(refreshDropdownAnchorIndicator)
        initializeDropdownPresentation()
    }
    onDropdownAnchorChanged: {
        // A new owner must not inherit the previous combo's indicator until
        // its own template has produced a valid scene mapping.
        dropdownAnchorIndicatorReady = false
        dropdownAnchorGeometryReady = false
        Qt.callLater(refreshDropdownAnchorIndicator)
    }
    onDropdownAnchorRectChanged: Qt.callLater(refreshDropdownAnchorIndicator)
    onDropdownModeChanged: {
        if (dropdownMode) {
            Qt.callLater(refreshDropdownAnchorIndicator)
            Qt.callLater(initializeDropdownPresentation)
            return
        }
        dropdownOpenAnimation.stop()
        dropdownCloseAnimation.stop()
        dropdownPresentationInitialized = false
        dropdownOpenSettled = false
        dropdownAnchorIndicatorReady = false
        dropdownAnchorGeometryReady = false
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
        Qt.callLater(refreshDropdownAnchorIndicator)
        Qt.callLater(syncListSelection)
        Qt.callLater(reconcilePointerState)
        initializeDropdownPresentation()
    }

    Connections {
        target: menuOverlay.dropdownAnchor
        ignoreUnknownSignals: true
        function onIndicatorChanged() {
            Qt.callLater(menuOverlay.refreshDropdownAnchorIndicator)
        }
        function onXChanged() {
            Qt.callLater(menuOverlay.refreshDropdownAnchorIndicator)
        }
        function onYChanged() {
            Qt.callLater(menuOverlay.refreshDropdownAnchorIndicator)
        }
        function onWidthChanged() {
            Qt.callLater(menuOverlay.refreshDropdownAnchorIndicator)
        }
        function onHeightChanged() {
            Qt.callLater(menuOverlay.refreshDropdownAnchorIndicator)
        }
        function onVisibleChanged() {
            Qt.callLater(menuOverlay.refreshDropdownAnchorIndicator)
        }
        function onParentChanged() {
            Qt.callLater(menuOverlay.refreshDropdownAnchorIndicator)
        }
    }

    Connections {
        target: menuOverlay.hostWindow
        ignoreUnknownSignals: true
        function onActiveChanged() {
            if (menuOverlay.hostWindow.active)
                Qt.callLater(menuOverlay.refreshDropdownAnchorIndicator)
        }
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
        font: hostWindow.font
    }

    readonly property real menuLabelInset: hostWindow.snapPx(hasLeadingIndicator ? 32 : 10)
    readonly property real driveRowHeight: effectiveMenuRowHeight
    function driveName(details) {
        return String(details.name || "") + (details.label ? " (" + details.label + ")" : "")
            + (details.network ? " — " + details.network : "")
    }
    function isDriveDetails(details) {
        return details && details.isDrive === "true"
    }
    property int captionMetricsRevision: 0
    Repeater {
        id: driveCaptionMetrics
        model: effectiveItems.filter(item => isDriveDetails(item.details))
        onItemAdded: menuOverlay.captionMetricsRevision++
        onItemRemoved: menuOverlay.captionMetricsRevision++
        delegate: Text {
            required property var modelData
            objectName: "driveCaptionMeasurement-" + Number(modelData.index)
            visible: false
            font: hostWindow.font
            textFormat: Text.StyledText
            text: hostWindow.mnemonicText(menuOverlay.driveName(modelData.details), modelData.hotkey)
        }
    }
    readonly property real driveNameWidth: {
        const revision = captionMetricsRevision
        let width = 0
        for (let i = 0; i < driveCaptionMetrics.count; ++i) {
            const item = driveCaptionMetrics.itemAt(i)
            if (item) width = Math.max(width, item.implicitWidth)
        }
        return hostWindow.snapPx(width + 2) + revision * 0
    }
    readonly property real driveFilesystemWidth: hostWindow.snapPx(Math.max(0,
        ...effectiveItems.map(item => item.details ? popupMenuMetrics.advanceWidth(String(item.details.filesystem || "")) : 0)))
    readonly property bool driveHasCapacity: effectiveItems.some(item => item.details && item.details.total)
    function driveCapacityText(details, styled = false) {
        const format = qsTr("%1 free of %2")
        if (!styled) return format.arg(details.free || "").arg(details.total || "")
        function size(value) {
            return '<font color="' + hostWindow.textColor + '">'
                + hostWindow.richTextEscape(value || "") + '</font>'
        }
        return hostWindow.richTextEscape(format).arg(size(details.free)).arg(size(details.total))
    }
    readonly property real driveCapacityTextWidth: hostWindow.snapPx(Math.max(0,
        ...effectiveItems.map(item => item.details && item.details.total
            ? popupMenuMetrics.advanceWidth(driveCapacityText(item.details)) + 2 : 0)))
    readonly property real driveCapacityWidth: driveHasCapacity
        ? hostWindow.snapPx(108) + driveCapacityTextWidth : 0

    function preferredMenuWidth() {
        if (effectiveItems.some(item => item.details !== undefined)) {
            let preferred = menuLabelInset + hostWindow.snapPx(16 + 16 + 24) + driveNameWidth + driveCapacityWidth + driveFilesystemWidth
            for (const item of effectiveItems) {
                if (!isDriveDetails(item.details)) preferred = Math.max(preferred, popupMenuMetrics.advanceWidth(
                    item.details ? driveName(item.details) : String(item.text || ""))
                    + menuLabelInset + hostWindow.snapPx(24) + 2 * menuEdgeInset)
            }
            return Math.min(hostWindow.width - 12, preferred)
        }

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
            y += itemHeightAt(i)
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
            x: menuOverlay.dropdownMode
               ? menuOverlay.dropdownListX : menuOverlay.menuEdgeInset
            y: menuOverlay.dropdownMode
               ? menuOverlay.dropdownOpenTop + menuOverlay.menuEdgeInset
                 - popupSurface.y
               : menuOverlay.menuEdgeInset
            width: Math.max(1, popupSurface.width
                               - (menuOverlay.dropdownMode
                                  ? menuOverlay.dropdownListX
                                  : menuOverlay.menuEdgeInset))
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
