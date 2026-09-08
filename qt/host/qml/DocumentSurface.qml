pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import ZoinGallery 1.0 as ZG

Rectangle {
    id: documentRoot
    required property ApplicationWindow hostWindow
    required property Item menuBar
    property string surfaceObjectName: "documentSurface"
    objectName: surfaceObjectName
    property var frame: ({})
    // Standalone F3/F4 owns the full application surface. Embedded
    // Quick View is already laid out between its own header and footer,
    // so applying the global menu/keybar insets there would double-pad it.
    property bool embedded: false
    property bool interactionActive: true
    property real scrollBarRightInset: 0
    property alias displayedRows: viewportController.displayedRows
    property alias windowInitialized: viewportController.windowInitialized
    property alias rebasingWindow: viewportController.rebasingWindow
    property alias windowRequestPending: viewportController.windowRequestPending
    property alias pendingWindowIntent: viewportController.pendingWindowIntent
    property alias requestedExtent: viewportController.requestedExtent
    property alias requestedFraction: viewportController.requestedFraction
    property alias requestedGeneration: viewportController.requestedGeneration
    property alias resumeVelocity: viewportController.resumeVelocity
    property alias requestPreservesLiveAnchor: viewportController.requestPreservesLiveAnchor
    property alias wheelGestureActive: viewportController.wheelGestureActive
    property alias middleAutoScrollActive: viewportController.middleAutoScrollActive
    property alias stableTopExtent: viewportController.stableTopExtent
    property alias stableTopFraction: viewportController.stableTopFraction
    property alias lastViewportStart: viewportController.lastViewportStart
    property alias wheelTarget: viewportController.wheelTarget
    property alias queuedScrollBarPosition: viewportController.queuedScrollBarPosition
    property alias appliedWindowSignature: viewportController.appliedWindowSignature
    property alias appliedDocumentKey: viewportController.appliedDocumentKey
    property alias appliedLayoutRevision: viewportController.appliedLayoutRevision
    property alias appliedWindowGeneration: viewportController.appliedWindowGeneration
    property alias canceledWindowGeneration: viewportController.canceledWindowGeneration
    property alias reportedViewportColumns: viewportController.reportedViewportColumns
    property alias geometryRevision: viewportController.geometryRevision
    property alias loadedSlotStart: viewportController.loadedSlotStart
    property alias loadedSlotEnd: viewportController.loadedSlotEnd
    property int liveRowDelegateCount: 0
    property alias poolSlotWriteCount: viewportController.poolSlotWriteCount
    property alias latestWindowRows: viewportController.latestWindowRows
    property alias reportedViewportRows: viewportController.reportedViewportRows
    property alias reportedViewportTarget: viewportController.reportedViewportTarget
    property alias reportedViewportAction: viewportController.reportedViewportAction
    property alias lastEditorMouseColumn: editorPointerController.lastMouseColumn
    property alias lastEditorMouseRow: editorPointerController.lastMouseRow
    property alias pendingEditorMouseMove: editorPointerController.pendingMouseMove
    property alias terminalSelectionVisible: terminalSelectionController.selectionVisible
    property alias terminalSelectionDragging: terminalSelectionController.selectionDragging
    property alias terminalSelectionAnchorRow: terminalSelectionController.anchorRow
    property alias terminalSelectionAnchorColumn: terminalSelectionController.anchorColumn
    property alias terminalSelectionFocusRow: terminalSelectionController.focusRow
    property alias terminalSelectionFocusColumn: terminalSelectionController.focusColumn
    property alias terminalClickCount: terminalSelectionController.clickCount
    property alias terminalLastClickAt: terminalSelectionController.lastClickAt
    property alias terminalLastClickRow: terminalSelectionController.lastClickRow
    property alias terminalLastClickColumn: terminalSelectionController.lastClickColumn
    property alias terminalSelectionPointerX: terminalSelectionController.pointerX
    property alias terminalSelectionPointerY: terminalSelectionController.pointerY
    property alias terminalSelectionAutoScrollDistance: terminalSelectionController.autoScrollDistance
    property alias terminalSelectionAutoScrollLastTick: terminalSelectionController.autoScrollLastTick
    property alias terminalFollowTailIntent: viewportController.terminalFollowTailIntent
    property alias terminalFollowTailInitialized: viewportController.terminalFollowTailInitialized
    readonly property var presentationFrame: viewportController.presentationFrame
    readonly property var cursorFrame:
        hostWindow.documentSurfaceStateOverride !== null
        && hostWindow.cleanText(hostWindow.documentSurfaceStateOverride.id)
           === hostWindow.cleanText(frame.id)
        && hostWindow.cleanText(hostWindow.documentSurfaceStateOverride.documentKey)
           === hostWindow.cleanText(documentKey)
        && (!standaloneViewport || (frame.layoutPending !== true
            && (hostWindow.documentSurfaceStateOverride.layoutRevision === undefined
                || Number(hostWindow.documentSurfaceStateOverride.layoutRevision)
                    === appliedLayoutRevision)
            && (hostWindow.documentSurfaceStateOverride.windowGeneration === undefined
                || Number(hostWindow.documentSurfaceStateOverride.windowGeneration)
                    === appliedWindowGeneration)))
        ? hostWindow.documentSurfaceStateOverride : presentationFrame
    readonly property bool showsConsoleTopBar:
        !embedded && (frame.kind === "viewer" || frame.kind === "editor")
    readonly property bool terminalSurface: frame.kind === "terminal"
    readonly property bool standaloneViewport:
        !embedded && !terminalSurface && surfaceObjectName === "documentSurface"
    readonly property bool inputPresentationActive:
        interactionActive && visible && hostWindow.active
        && !hostWindow.hasBlockingOverlay()
    readonly property bool middleAutoScrollAllowed:
        (standaloneViewport || terminalSurface) && inputPresentationActive
        && hostWindow.mouseWheelMode === "gui"
    readonly property bool terminalSelectionEnabled:
        terminalSurface && frame.selectionEnabled === true
    readonly property string topBarLeftText: {
        const path = hostWindow.cleanText(presentationFrame.path).trim()
        const legacyCaption = hostWindow.cleanText(
                    presentationFrame.topBarLeft).trim()
        const baseName = hostWindow.cleanText(
                    presentationFrame.baseName).trim()
        // Generated/plugin editor buffers deliberately replace their
        // temporary file name with a meaningful caption. Ordinary files use
        // the full logical path supplied by the core.
        const explicitEditorCaption = presentationFrame.kind === "editor"
                && baseName !== "" && legacyCaption !== baseName
        if (showsConsoleTopBar && path !== "" && !explicitEditorCaption)
            return path
        return legacyCaption
    }
    readonly property string topBarRightText:
        viewportController.loadError !== ""
        ? "Read error: " + viewportController.loadError
        : hostWindow.cleanText(cursorFrame.topBarRight !== undefined
                       ? cursorFrame.topBarRight : presentationFrame.topBarRight).trim()
    readonly property string documentFileName:
        hostWindow.cleanText(presentationFrame.baseName).trim() !== ""
        ? hostWindow.cleanText(presentationFrame.baseName).trim() : topBarLeftText
    readonly property string documentFileLocalPath:
        hostWindow.cleanText(presentationFrame.localPath)
    readonly property int documentFileIconLogicalSize: 16
    readonly property url documentFileIconSource:
        showsConsoleTopBar
        ? hostWindow.fileIconSource(documentFileLocalPath, documentFileName,
                              false, documentFileIconLogicalSize, 0)
        : ""
    readonly property bool documentFileIconFullColor:
        qtIcons.fileIconsAreFullColor === true
    readonly property color documentFileIconColor:
        hostWindow.cleanText(presentationFrame.iconColor).trim() !== ""
        ? hostWindow.cleanText(presentationFrame.iconColor).trim()
        : hostWindow.galleryMutedTextColor
    readonly property bool documentFileIconAvailable:
        documentFileIconSource.toString() !== ""
    readonly property real surfaceMenuInset:
        embedded ? 0 : (menuBar.visible ? menuBar.height : 0)
    readonly property real documentHeaderHeight:
        showsConsoleTopBar
        ? prospectiveHeaderHeight
        : 0
    readonly property real prospectiveHeaderHeight: hostWindow.snapPx(
        Math.max(25, hostWindow.ch * 1.25) + hostWindow.verticalContentSpacing
        + hostWindow.pathRowExtraHeight)
    readonly property real documentGutterWidth: standaloneViewport
        ? hostWindow.snapPx(15) + Math.max(0, scrollBarRightInset) : 0
    readonly property real prospectiveViewportWidth:
        Math.max(0, width - 2 * textHorizontalInset - documentGutterWidth)
    readonly property real prospectiveViewportHeight: Math.max(0,
        height - surfaceMenuInset - prospectiveHeaderHeight - bottomInset)
    readonly property bool kineticActive: viewportController.kineticActive
    readonly property bool hasWindowProtocol:
        viewportController.hasWindowProtocol
    readonly property string documentKey: viewportController.documentKey
    readonly property real contentExtent: viewportController.contentExtent
    readonly property bool contentExtentKnown:
        viewportController.contentExtentKnown
    readonly property real topInset:
        surfaceMenuInset + documentHeaderHeight
    readonly property real bottomInset: embedded ? 0
        : hostWindow.keyBarHeight()
    readonly property real rowHeight: standaloneViewport
        ? hostWindow.snapPx(Math.max(20, hostWindow.ch)) : Math.max(20, hostWindow.ch)
    readonly property real textHorizontalInset: terminalSurface ? 0
        : standaloneViewport ? hostWindow.snapPx(10) : 10
    readonly property real terminalCellWidth:
        Math.max(1, standaloneViewport ? documentCellProbe.implicitWidth / 64
                                      : documentFontMetrics.advanceWidth("M"))
    // Share the ancestor-to-window dependency chain once. Body text depends
    // only on this origin and its known local layout; repeating a 12-parent
    // dependency walk for every glyph run turns a row commit into a web of
    // unnecessary binding reevaluations.
    readonly property point pixelGridOrigin: {
        let revision = hostWindow.width + hostWindow.height + hostWindow.dpr
        let ancestor = documentRoot
        while (ancestor && ancestor !== hostWindow.contentItem) {
            revision += ancestor.x + ancestor.y + ancestor.width + ancestor.height
            ancestor = ancestor.parent
        }
        const origin = documentRoot.mapToItem(hostWindow.contentItem, 0, 0)
        return Qt.point(origin.x + revision * 0, origin.y)
    }

    function bodyPixelOffsetX(localX) {
        const origin = pixelGridOrigin.x + localX
        return standaloneViewport && !kineticActive ? hostWindow.snapPx(origin) - origin : 0
    }
    function bodyPixelOffsetY(localY) {
        const origin = pixelGridOrigin.y + localY
        return standaloneViewport && !kineticActive ? hostWindow.snapPx(origin) - origin : 0
    }

    function pixelOffsetX(item) {
        return standaloneViewport && !kineticActive
                ? hostWindow.dialogPixelOffsetX(item, hostWindow.contentItem) : 0
    }
    function pixelOffsetY(item) {
        return standaloneViewport && !kineticActive
                ? hostWindow.dialogPixelOffsetY(item, hostWindow.contentItem) : 0
    }

    function runBackground(value) {
        var background = hostWindow.cleanText(value).toLowerCase()
        var defaultBackground = hostWindow.cleanText(
                    presentationFrame.defaultBackground).toLowerCase()
        if ((defaultBackground !== ""
                && background === defaultBackground)
                || background === "#000000"
                || background === "#ff000000"
                || background === "black")
            return "transparent"
        return background !== "" ? value : "transparent"
    }

    function editorSelectionRangeForRow(visualRow, visualWidth, caretState) {
        const caret = caretState || cursorFrame
        let empty = ({ "valid": false, "start": 0, "end": 0 })
        if (frame.kind !== "editor" || presentationFrame.hexMode === true
                || presentationFrame.decodeMode === true
                || caret.selection !== true || visualRow < 0)
            return empty
        let anchorRow = Number(caret.selectionAnchorRow || 0)
        let anchorColumn = Number(caret.selectionAnchorColumn || 0)
        let focusRow = Number(caret.cursorAbsoluteRow || 0)
        let focusColumn = Number(caret.cursorAbsoluteColumn || 0)
        if (anchorRow > focusRow
                || (anchorRow === focusRow && anchorColumn > focusColumn)) {
            let swapRow = anchorRow
            let swapColumn = anchorColumn
            anchorRow = focusRow
            anchorColumn = focusColumn
            focusRow = swapRow
            focusColumn = swapColumn
        }
        if (visualRow < anchorRow || visualRow > focusRow)
            return empty
        let start = visualRow === anchorRow ? anchorColumn : 0
        let end = visualRow === focusRow ? focusColumn : Math.max(0, visualWidth)
        const scrollLeft = Math.max(0, Number(presentationFrame.scrollLeft || 0))
        const rowEnd = Math.max(0, Number(visualWidth || 0) - scrollLeft)
        const viewportEnd = Math.max(0, Number(presentationFrame.viewportColumns || 0))
        start = Math.max(0, start - scrollLeft)
        end = Math.max(0, end - scrollLeft)
        if (viewportEnd > 0) {
            start = Math.min(start, viewportEnd)
            end = Math.min(end, viewportEnd)
        }
        start = Math.min(start, rowEnd)
        end = Math.min(end, rowEnd)
        return ({ "valid": end > start, "start": start, "end": end })
    }


    function editorMouseButton(buttons) {
        return editorPointerController.mouseButton(buttons)
    }
    function editorMouseAction(mouse, phase, moved, doubleClick) {
        return editorPointerController.mouseAction(mouse, phase, moved,
                                                   doubleClick)
    }
    function flushEditorMouseMove() {
        editorPointerController.flushMouseMove()
    }
    function sendEditorMouse(mouse, phase, moved, doubleClick) {
        editorPointerController.sendMouse(mouse, phase, moved, doubleClick)
    }
    function releaseEditorMouse() {
        editorPointerController.releaseMouse()
    }

    function terminalSelectionPointAt(pointX, pointY) {
        return terminalSelectionController.pointAt(pointX, pointY)
    }
    function terminalSelectionPoint(mouse) {
        return terminalSelectionController.point(mouse)
    }
    function terminalSelectionPointAtViewportEdge() {
        return terminalSelectionController.pointAtViewportEdge()
    }
    function terminalColumnCount(rowWidth) {
        return terminalSelectionController.columnCount(rowWidth)
    }
    function handleTerminalSelectionPressAt(row, column, cellColumn, timestamp) {
        return terminalSelectionController.handlePressAt(
                    row, column, cellColumn, timestamp)
    }
    function terminalSelectionAutoScrollVelocity(distance) {
        return terminalSelectionController.autoScrollVelocity(distance)
    }
    function stopTerminalSelectionAutoScroll() {
        terminalSelectionController.stopAutoScroll()
    }
    function updateTerminalSelectionPointer(pointX, pointY) {
        terminalSelectionController.updatePointer(pointX, pointY)
    }
    function beginTerminalSelectionAt(row, column) {
        terminalSelectionController.beginAt(row, column)
    }
    function extendTerminalSelectionTo(row, column) {
        terminalSelectionController.extendTo(row, column)
    }
    function commitTerminalSelection() {
        terminalSelectionController.commit()
    }
    function terminalSelectionRangeForRow(row, rowWidth) {
        return terminalSelectionController.rangeForRow(row, rowWidth)
    }

    function rowExtent(index, rows) {
        return viewportController.rowExtent(index, rows)
    }
    function indexForExtent(extent, rows) {
        return viewportController.indexForExtent(extent, rows)
    }
    function topState() {
        return viewportController.topState()
    }
    function sendWindowRequest(extent, fraction, velocity,
                               preserveLiveAnchor) {
        return viewportController.sendWindowRequest(
                    extent, fraction, velocity, preserveLiveAnchor)
    }
    function maybeRequestWindow() {
        viewportController.maybeRequestWindow()
    }
    function applyFrameWindow() {
        viewportController.applyFrameWindow()
    }
    function handleWheel(wheel) {
        viewportController.handleWheel(wheel)
    }
    function beginMiddleAutoScroll() {
        return viewportController.beginMiddleAutoScroll()
    }
    function endMiddleAutoScroll(commitPosition) {
        return viewportController.endMiddleAutoScroll(commitPosition)
    }
    function cancelPendingWindowIntent() {
        viewportController.cancelPendingIntent()
    }

    color: "transparent"

    DocumentHeader {
        id: documentHeader
        hostWindow: documentRoot.hostWindow
        documentRoot: documentRoot
    }

    FontMetrics {
        id: documentFontMetrics
        font.family: hostWindow.guiMonospaceFontFamily
        font.pixelSize: hostWindow.semanticTextFontPixelSize
    }

    // FontMetrics resolves a different native font/DPI size from QQuickText
    // on Windows (11 px vs the actual 7 px Consolas 13 advance). Measure the
    // same renderer and resolved font as the real leaves, before opening any
    // document. A long string avoids rounding one fractional glyph advance.
    Text {
        id: documentCellProbe
        objectName: "documentCellProbe"
        visible: false
        text: "M".repeat(64)
        textFormat: Text.PlainText
        font.family: hostWindow.guiMonospaceFontFamily
        font.pixelSize: hostWindow.semanticTextFontPixelSize
        renderType: hostWindow.fontRenderType
    }

    ListView {
        id: documentList
        objectName: "documentList"
        // Never expose the previous file during a new document's first load.
        // Keep its slots untouched until the new ready window commits once.
        visible: !documentRoot.standaloneViewport
                 || (documentRoot.interactionActive
                     && documentRoot.windowInitialized
                     && documentRoot.documentKey
                        === documentRoot.appliedDocumentKey)
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.topMargin: documentRoot.topInset
        anchors.bottom: parent.bottom
        anchors.bottomMargin: documentRoot.bottomInset
        clip: true
        model: viewportController.rowsModel
        interactive: documentRoot.interactionActive
        boundsBehavior: Flickable.StopAtBounds
        reuseItems: true
        cacheBuffer: documentRoot.rowHeight * 2

        delegate: DocumentRowDelegate {
            hostWindow: documentRoot.hostWindow
            documentRoot: documentRoot
            documentList: documentList
            viewportController: viewportController
        }

        onContentYChanged: viewportController.contentYChanged()
        onHeightChanged: viewportController.scheduleNativeViewportSync()
        onMovementEnded: viewportController.movementEnded()
    }

    Rectangle {
        id: editorCursor
        objectName: "editorCursor"
        parent: documentList.contentItem
        property alias blinkOn: editorCursorBlinkController.blinkOn
        property alias blinkInterval: editorCursorBlinkController.interval
        readonly property bool blinkTimerRunning:
            editorCursorBlinkController.running
        readonly property bool block:
            documentRoot.cursorFrame.cursorShape === "block"
        readonly property int windowRow:
            frame.kind === "editor" || frame.kind === "terminal"
            ? documentRoot.indexForExtent(
                  Number(documentRoot.cursorFrame.cursorAbsoluteRow || 0),
                                          documentRoot.displayedRows)
            : -1
        readonly property var rowDelegate:
            windowRow >= 0 ? viewportController.rowItem(windowRow) : null
        x: documentRoot.textHorizontalInset + Math.max(0, Number(
                frame.kind === "terminal"
                ? documentRoot.cursorFrame.cursorX || 0
                : documentRoot.cursorFrame.cursorVisualColumn || 0))
                * documentRoot.terminalCellWidth
        y: rowDelegate !== null
           ? rowDelegate.y + (block ? 1 : 2)
           : -documentRoot.rowHeight
        width: block ? documentRoot.terminalCellWidth
                     : 2
        height: documentRoot.rowHeight - (block ? 2 : 4)
        color: hostWindow.textColor
        opacity: blinkOn ? 1 : 0
        visible: (frame.kind === "editor" || frame.kind === "terminal")
                 && (!documentRoot.standaloneViewport
                     || documentRoot.cursorFrame.layoutRevision === undefined
                     || Number(documentRoot.cursorFrame.layoutRevision)
                        === documentRoot.appliedLayoutRevision)
                 && documentRoot.cursorFrame.cursorVisible === true
                 && windowRow >= 0
                 && rowDelegate !== null
                 && Number(frame.kind === "terminal"
                           ? documentRoot.cursorFrame.cursorX
                           : documentRoot.cursorFrame.cursorVisualColumn) >= 0
        z: 5
        transform: Translate {
            x: documentRoot.pixelOffsetX(editorCursor)
            y: documentRoot.pixelOffsetY(editorCursor)
        }

        function restartBlink() {
            editorCursorBlinkController.restart()
        }

        ActivityBoundedCursorBlink {
            id: editorCursorBlinkController
            objectName: documentRoot.surfaceObjectName
                        + "CursorBlinkController"
            active: editorCursor.visible
                    && documentRoot.inputPresentationActive
            activityRevision: hostWindow.keyboardActivityRevision
        }
    }

    Repeater {
        model: frame.kind === "editor" ? (documentRoot.cursorFrame.secondaryCarets || []).length : 0
        delegate: Rectangle {
            id: secondaryCursor
            required property int index
            readonly property var modelData: (documentRoot.cursorFrame.secondaryCarets || [])[index] || {}
            objectName: "editorSecondaryCursor-" + index
            parent: documentList.contentItem
            readonly property int windowRow: documentRoot.indexForExtent(
                Number(modelData.cursorAbsoluteRow), documentRoot.displayedRows)
            readonly property var rowDelegate: windowRow >= 0 ? viewportController.rowItem(windowRow) : null
            readonly property real column: Number(modelData.cursorAbsoluteColumn)
                - Number(documentRoot.presentationFrame.scrollLeft || 0)
            x: documentRoot.textHorizontalInset + column * documentRoot.terminalCellWidth
            y: rowDelegate !== null ? rowDelegate.y + (editorCursor.block ? 1 : 2) : 0
            width: hostWindow.snapPx(editorCursor.width)
            height: hostWindow.snapPx(editorCursor.height)
            color: editorCursor.color
            opacity: editorCursor.opacity
            visible: rowDelegate !== null && column >= 0
                && column < Number(documentRoot.presentationFrame.viewportColumns || 1000000)
                && (!documentRoot.standaloneViewport
                    || documentRoot.cursorFrame.layoutRevision === undefined
                    || Number(documentRoot.cursorFrame.layoutRevision) === documentRoot.appliedLayoutRevision)
            z: 5
            transform: Translate {
                x: documentRoot.pixelOffsetX(secondaryCursor)
                y: documentRoot.pixelOffsetY(secondaryCursor)
            }
        }
    }

    MouseArea {
        anchors.left: documentList.left
        anchors.right: documentScrollBar.visible
                       ? documentScrollBar.left : documentList.right
        anchors.top: documentList.top
        anchors.bottom: documentList.bottom
        acceptedButtons: frame.kind === "editor"
                         ? Qt.LeftButton | Qt.RightButton
                           | (documentRoot.standaloneViewport
                              && hostWindow.mouseWheelMode === "gui"
                              ? Qt.NoButton : Qt.MiddleButton)
                         : documentRoot.terminalSelectionEnabled
                           ? Qt.LeftButton
                         : Qt.NoButton
        preventStealing: frame.kind === "editor"
                         || documentRoot.terminalSelectionEnabled
        propagateComposedEvents: true
        enabled: documentRoot.inputPresentationActive
        cursorShape: frame.kind === "editor"
                     || documentRoot.terminalSelectionEnabled
                     ? Qt.IBeamCursor : Qt.ArrowCursor
        z: 8
        onPressed: mouse => {
            editorCursor.restartBlink()
            if (frame.kind === "editor") {
                documentRoot.sendEditorMouse(mouse, "press", false, false)
            } else if (documentRoot.terminalSelectionEnabled) {
                documentList.cancelFlick()
                viewportController.stopMotion()
                var point = documentRoot.terminalSelectionPoint(mouse)
                documentRoot.handleTerminalSelectionPressAt(
                            point.row, point.column,
                            point.cellColumn, Date.now())
                documentRoot.updateTerminalSelectionPointer(mouse.x,
                                                              mouse.y)
            }
            mouse.accepted = true
        }
        onPositionChanged: mouse => {
            if (frame.kind === "editor" && mouse.buttons !== Qt.NoButton) {
                editorCursor.restartBlink()
                documentRoot.sendEditorMouse(mouse, "move", true, false)
            } else if (documentRoot.terminalSelectionDragging
                     && mouse.buttons !== Qt.NoButton) {
                editorCursor.restartBlink()
                documentRoot.updateTerminalSelectionPointer(mouse.x,
                                                              mouse.y)
                var point = documentRoot.terminalSelectionPointAtViewportEdge()
                documentRoot.extendTerminalSelectionTo(point.row,
                                                        point.column)
            }
        }
        onReleased: mouse => {
            editorCursor.restartBlink()
            if (frame.kind === "editor") {
                documentRoot.sendEditorMouse(mouse, "release", false, false)
            } else if (documentRoot.terminalSelectionDragging) {
                documentRoot.updateTerminalSelectionPointer(mouse.x,
                                                              mouse.y)
                var point = documentRoot.terminalSelectionPointAtViewportEdge()
                documentRoot.extendTerminalSelectionTo(point.row,
                                                        point.column)
                documentRoot.commitTerminalSelection()
            }
            mouse.accepted = true
        }
        onCanceled: {
            if (frame.kind === "editor")
                documentRoot.releaseEditorMouse()
            else {
                documentRoot.terminalSelectionDragging = false
                documentRoot.stopTerminalSelectionAutoScroll()
            }
        }
        onDoubleClicked: mouse => {
            editorCursor.restartBlink()
            if (frame.kind === "editor")
                documentRoot.sendEditorMouse(mouse, "press", false, true)
            else if (documentRoot.terminalSelectionEnabled
                     && documentRoot.terminalClickCount === 1) {
                // Most Qt platforms emit onPressed for the second press;
                // this fallback covers backends that surface only the
                // composed double-click signal.
                var point = documentRoot.terminalSelectionPoint(mouse)
                documentRoot.handleTerminalSelectionPressAt(
                            point.row, point.column,
                            point.cellColumn, Date.now())
            }
            mouse.accepted = true
        }
        // Wheel gestures stay in the native QML scrolling pipeline for
        // both viewers and editors.  Only button/drag selection events
        // need the canonical Go editor mouse handler.
        onWheel: wheel => documentRoot.handleWheel(wheel)
    }

    // The browser-style middle gesture owns a stable panel-level pointer
    // area. hoverEnabled is essential: a stationary click releases the
    // button while leaving auto-scroll armed, and later buttonless movement
    // must continue updating the shared frame-driven controller.
    MouseArea {
        id: documentMiddleButtonArea
        objectName: "documentMiddleButtonArea"
        anchors.left: documentList.left
        anchors.right: documentScrollBar.visible
                       ? documentScrollBar.left : documentList.right
        anchors.top: documentList.top
        anchors.bottom: documentList.bottom
        acceptedButtons: Qt.MiddleButton
        hoverEnabled: enabled
        preventStealing: true
        enabled: documentRoot.middleAutoScrollAllowed
        cursorShape: middleAutoScrollController.scrollingMode
                     ? Qt.SizeVerCursor
                     : frame.kind === "editor" ? Qt.IBeamCursor
                                                : Qt.ArrowCursor
        z: 9

        onPressed: mouse => {
            if (middleAutoScrollController.scrollingMode)
                documentRoot.endMiddleAutoScroll(false)
            else
                documentRoot.beginMiddleAutoScroll()
            mouse.accepted = true
        }
        onPositionChanged: middleAutoScrollController.updatePointerMotion()
        onReleased: mouse => {
            if (middleAutoScrollController.scrollingStarted)
                documentRoot.endMiddleAutoScroll(true)
            mouse.accepted = true
        }
        onCanceled: documentRoot.endMiddleAutoScroll(true)
    }

    // AutoScrollController also drives the gallery's middle gesture. This
    // zero-size adapter gives its reusable contentY/setScrollingMode contract
    // to ListView without duplicating its frame-rate-independent speed math.
    Item {
        id: documentAutoScrollLayout
        visible: false
        width: 0
        height: 0
        property alias contentY: documentList.contentY
        property bool scrollingMode: false
        property int scrollingDirection: 0

        // Keep the panel's cursor transition on the shared native path. The
        // adapter deliberately coalesces repeated frame callbacks: changing
        // the SVG cursor is a presentation transition, not per-frame work.
        function setScrollingMode(active, direction) {
            const nextMode = active === true
            const nextDirection = nextMode
                    ? Number(direction || 0) : 0
            if (scrollingMode === nextMode
                    && scrollingDirection === nextDirection)
                return
            scrollingMode = nextMode
            scrollingDirection = nextDirection

            const gallery = documentRoot.hostWindow.galleryControllerApi
            if (gallery && typeof gallery.setScrollingMouseCursor
                    === "function") {
                gallery.setScrollingMouseCursor(
                    nextMode, nextDirection,
                    Number(documentRoot.hostWindow.dpr || 1))
            }
        }

        Component.onDestruction: {
            if (scrollingMode)
                setScrollingMode(false, 0)
        }
    }

    ZG.AutoScrollController {
        id: middleAutoScrollController
        objectName: "documentMouseAutoScrollController"
        layout: documentAutoScrollLayout
        pointerSource: documentMiddleButtonArea
        horizontal: false
        scrollExtent: documentRoot.hostWindow.height
    }

    F4ScrollBar {
        id: documentScrollBar
        objectName: "documentScrollBar"
        hostWindow: documentRoot.hostWindow
        parent: documentRoot
        anchors.top: documentList.top
        anchors.bottom: documentList.bottom
        anchors.right: documentList.right
        anchors.rightMargin: Math.max(0,
                                      documentRoot.scrollBarRightInset)
        thickness: 15
        orientation: Qt.Vertical
        policy: T.ScrollBar.AlwaysOn
        visible: documentRoot.hasWindowProtocol
                 && documentRoot.interactionActive
                 && documentRoot.contentExtentKnown
                 && documentRoot.contentExtent
                    > Math.max(0, Number(frame.viewportSpan || 0))
        z: 10

        onPositionChanged: viewportController.scrollBarPositionChanged()
        onPressedChanged: viewportController.scrollBarPressedChanged()
    }

    DocumentTerminalSelectionController {
        id: terminalSelectionController
        hostWindow: documentRoot.hostWindow
        documentList: documentList
        fontMetrics: documentFontMetrics
        viewportController: viewportController
        frame: documentRoot.frame
        interactionActive: documentRoot.inputPresentationActive
        rowHeight: documentRoot.rowHeight
        textHorizontalInset: documentRoot.textHorizontalInset
        terminalCellWidth: documentRoot.terminalCellWidth
    }

    DocumentEditorPointerController {
        id: editorPointerController
        hostWindow: documentRoot.hostWindow
        documentList: documentList
        fontMetrics: documentFontMetrics
        viewportController: viewportController
        frame: documentRoot.frame
        rowHeight: documentRoot.rowHeight
        textHorizontalInset: documentRoot.textHorizontalInset
        textViewportWidth: documentRoot.prospectiveViewportWidth
        documentCellWidth: documentRoot.terminalCellWidth
    }

    DocumentViewportController {
        id: viewportController
        hostWindow: documentRoot.hostWindow
        documentList: documentList
        documentScrollBar: documentScrollBar
        terminalSelectionController: terminalSelectionController
        editorPointerController: editorPointerController
        middleAutoScrollController: middleAutoScrollController
        frame: documentRoot.frame
        embedded: documentRoot.embedded
        interactionActive: documentRoot.interactionActive
        showsConsoleTopBar: documentRoot.showsConsoleTopBar
        terminalSurface: documentRoot.terminalSurface
        rowHeight: documentRoot.rowHeight
        standaloneViewport: documentRoot.standaloneViewport
        standaloneViewportWidth: documentRoot.prospectiveViewportWidth
        standaloneViewportHeight: documentRoot.prospectiveViewportHeight
        documentCellWidth: documentRoot.terminalCellWidth
        middleAutoScrollAllowed: documentRoot.middleAutoScrollAllowed
    }

}
