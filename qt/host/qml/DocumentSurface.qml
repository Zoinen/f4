pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl

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
    property alias requestedExtent: viewportController.requestedExtent
    property alias requestedFraction: viewportController.requestedFraction
    property alias requestedGeneration: viewportController.requestedGeneration
    property alias resumeVelocity: viewportController.resumeVelocity
    property alias requestPreservesLiveAnchor: viewportController.requestPreservesLiveAnchor
    property alias wheelGestureActive: viewportController.wheelGestureActive
    property alias stableTopExtent: viewportController.stableTopExtent
    property alias stableTopFraction: viewportController.stableTopFraction
    property alias lastViewportStart: viewportController.lastViewportStart
    property alias wheelTarget: viewportController.wheelTarget
    property alias queuedScrollBarPosition: viewportController.queuedScrollBarPosition
    property alias appliedWindowSignature: viewportController.appliedWindowSignature
    property alias appliedDocumentKey: viewportController.appliedDocumentKey
    property alias initialPlacementPending: viewportController.initialPlacementPending
    property alias initialPlacementExtent: viewportController.initialPlacementExtent
    property alias initialPlacementFraction: viewportController.initialPlacementFraction
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
    readonly property var cursorFrame:
        hostWindow.documentSurfaceStateOverride !== null
        && hostWindow.cleanText(hostWindow.documentSurfaceStateOverride.id)
           === hostWindow.cleanText(frame.id)
        ? hostWindow.documentSurfaceStateOverride : frame
    readonly property bool showsConsoleTopBar:
        !embedded && (frame.kind === "viewer" || frame.kind === "editor")
    readonly property bool terminalSurface: frame.kind === "terminal"
    readonly property bool terminalSelectionEnabled:
        terminalSurface && frame.selectionEnabled === true
    readonly property string topBarLeftText:
        hostWindow.cleanText(frame.topBarLeft).trim()
    readonly property string topBarRightText:
        hostWindow.cleanText(cursorFrame.topBarRight !== undefined
                       ? cursorFrame.topBarRight : frame.topBarRight).trim()
    readonly property string documentFileName:
        hostWindow.cleanText(frame.baseName).trim() !== ""
        ? hostWindow.cleanText(frame.baseName).trim() : topBarLeftText
    readonly property string documentFileLocalPath:
        hostWindow.cleanText(frame.localPath)
    readonly property int documentFileIconLogicalSize: 16
    readonly property url documentFileIconSource:
        showsConsoleTopBar
        ? hostWindow.fileIconSource(documentFileLocalPath, documentFileName,
                              false, documentFileIconLogicalSize, 0)
        : ""
    readonly property bool documentFileIconFullColor:
        qtIcons.fileIconsAreFullColor === true
    readonly property color documentFileIconColor:
        hostWindow.cleanText(frame.iconColor).trim() !== ""
        ? hostWindow.cleanText(frame.iconColor).trim()
        : hostWindow.galleryMutedTextColor
    readonly property bool documentFileIconAvailable:
        documentFileIconSource.toString() !== ""
    readonly property real surfaceMenuInset:
        embedded ? 0 : (menuBar.visible ? menuBar.height : 0)
    readonly property real documentHeaderHeight:
        showsConsoleTopBar
        ? Math.max(25, hostWindow.ch * 1.25)
          + hostWindow.verticalContentSpacing
          + hostWindow.pathRowExtraHeight
        : 0
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
    readonly property real rowHeight: Math.max(20, hostWindow.ch)
    readonly property real textHorizontalInset: terminalSurface ? 0 : 10
    readonly property real terminalCellWidth:
        Math.max(1, documentFontMetrics.advanceWidth("M"))

    function runBackground(value) {
        var background = hostWindow.cleanText(value).toLowerCase()
        var defaultBackground = hostWindow.cleanText(
                    frame.defaultBackground).toLowerCase()
        if ((defaultBackground !== ""
                && background === defaultBackground)
                || background === "#000000"
                || background === "#ff000000"
                || background === "black")
            return "transparent"
        return background !== "" ? value : "transparent"
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

    color: "transparent"

    Rectangle {
        id: documentHeader
        objectName: documentRoot.surfaceObjectName === "documentSurface"
                    ? "documentHeader"
                    : documentRoot.surfaceObjectName + "Header"
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.topMargin: documentRoot.surfaceMenuInset
        height: documentRoot.documentHeaderHeight
        visible: documentRoot.showsConsoleTopBar
        color: hostWindow.titleBarBg
        z: 2

        Rectangle {
            id: documentHeaderBackground
            objectName: "documentHeaderBackground"
            anchors.fill: parent
            color: hostWindow.panelPathBg
            z: 0
        }

        Rectangle {
            id: documentHeaderSeparator
            objectName: "documentHeaderSeparator"
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: hostWindow.separatorWidth
            color: hostWindow.separatorColor
        }

        Item {
            id: documentHeaderIcon
            objectName: documentRoot.surfaceObjectName === "documentSurface"
                        ? "documentHeaderIcon"
                        : documentRoot.surfaceObjectName + "HeaderIcon"
            anchors.left: parent.left
            anchors.leftMargin: hostWindow.panelTextInset
            anchors.verticalCenter: parent.verticalCenter
            width: hostWindow.snapPx(18)
            height: hostWindow.snapPx(18)
            visible: documentRoot.documentFileIconAvailable
            z: 1

            IconLabel {
                id: documentHeaderLucideIcon
                objectName: documentRoot.surfaceObjectName === "documentSurface"
                            ? "documentHeaderLucideIcon"
                            : documentRoot.surfaceObjectName
                              + "HeaderLucideIcon"
                anchors.centerIn: parent
                width: hostWindow.snapPx(16)
                height: hostWindow.snapPx(16)
                visible: !documentRoot.documentFileIconFullColor
                icon.source: documentRoot.documentFileIconSource
                icon.width: width
                icon.height: height
                icon.color: documentRoot.documentFileIconColor
            }

            Image {
                id: documentHeaderSystemIcon
                objectName: "documentHeaderSystemIcon"
                anchors.centerIn: parent
                width: hostWindow.snapPx(16)
                height: hostWindow.snapPx(16)
                source: documentRoot.documentFileIconSource
                fillMode: Image.PreserveAspectFit
                smooth: false
                mipmap: false
                asynchronous: true
                cache: true
                retainWhileLoading: true
                visible: documentRoot.documentFileIconFullColor
            }

            IconLabel {
                id: documentHeaderSystemFallbackIcon
                objectName: "documentHeaderSystemFallbackIcon"
                anchors.centerIn: parent
                width: hostWindow.snapPx(16)
                height: hostWindow.snapPx(16)
                visible: documentRoot.documentFileIconFullColor
                         && documentHeaderSystemIcon.status !== Image.Ready
                icon.source: hostWindow.lucideIconSource(
                                 "file", 16,
                                 documentRoot.documentFileIconColor)
                icon.width: width
                icon.height: height
                icon.color: documentRoot.documentFileIconColor
            }
        }

        Text {
            id: documentHeaderRight
            objectName: documentRoot.surfaceObjectName === "documentSurface"
                        ? "documentHeaderRight"
                        : documentRoot.surfaceObjectName + "HeaderRight"
            anchors.right: parent.right
            anchors.rightMargin: hostWindow.panelTextInset
            anchors.verticalCenter: parent.verticalCenter
            width: Math.min(implicitWidth,
                           Math.max(0, parent.width
                                   - 2 * hostWindow.panelTextInset))
            text: documentRoot.topBarRightText
            color: hostWindow.galleryPathTextColor
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: hostWindow.semanticTextFontPixelSize
            horizontalAlignment: Text.AlignRight
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideLeft
        }

        Text {
            id: documentHeaderLeft
            objectName: documentRoot.surfaceObjectName === "documentSurface"
                        ? "documentHeaderLeft"
                        : documentRoot.surfaceObjectName + "HeaderLeft"
            anchors.left: documentHeaderIcon.visible
                           ? documentHeaderIcon.right : parent.left
            anchors.right: documentHeaderRight.left
            anchors.leftMargin: documentHeaderIcon.visible
                               ? hostWindow.snapPx(7) : hostWindow.panelTextInset
            anchors.rightMargin: hostWindow.panelTextInset
            anchors.verticalCenter: parent.verticalCenter
            text: documentRoot.topBarLeftText
            color: hostWindow.galleryPathTextColor
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: hostWindow.semanticTextFontPixelSize
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }
    }

    FontMetrics {
        id: documentFontMetrics
        font.family: hostWindow.guiMonospaceFontFamily
        font.pixelSize: 13
    }

    ListView {
        id: documentList
        objectName: "documentList"
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

        delegate: Rectangle {
            id: documentRow
            objectName: "documentRowDelegate"
            required property bool loaded
            required property var rowData
            property bool countedAsLive: true
            width: ListView.view.width
            height: documentRoot.rowHeight
            color: "transparent"
            Component.onCompleted: ++documentRoot.liveRowDelegateCount
            Component.onDestruction: {
                if (countedAsLive)
                    --documentRoot.liveRowDelegateCount
            }
            ListView.onPooled: {
                if (countedAsLive) {
                    countedAsLive = false
                    --documentRoot.liveRowDelegateCount
                }
            }
            ListView.onReused: {
                if (!countedAsLive) {
                    countedAsLive = true
                    ++documentRoot.liveRowDelegateCount
                }
            }

            Row {
                id: runRow
                anchors.left: parent.left
                anchors.leftMargin: documentRoot.textHorizontalInset
                height: parent.height
                z: 1
                visible: documentRow.loaded
                         && documentRow.rowData.runs !== undefined
                         && documentRow.rowData.runs.length > 0

                Repeater {
                    model: documentRow.loaded
                           ? documentRow.rowData.runs || [] : []

                    delegate: Rectangle {
                        height: runRow.height
                        width: runLabel.implicitWidth
                        color: documentRoot.runBackground(
                                   modelData.background)

                        Text {
                            id: runLabel
                            anchors.verticalCenter: parent.verticalCenter
                            text: hostWindow.cleanText(modelData.text)
                            color: hostWindow.cleanText(modelData.foreground) !== ""
                                   ? modelData.foreground : hostWindow.textColor
                            font.family: hostWindow.guiMonospaceFontFamily
                            font.pixelSize: 13
                            font.bold: modelData.bold === true
                            font.underline: modelData.underline === true
                            font.strikeout: modelData.strikeout === true
                        }
                    }
                }
            }

            Text {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                anchors.leftMargin: documentRoot.textHorizontalInset
                anchors.rightMargin: documentRoot.textHorizontalInset
                visible: documentRow.loaded
                         && (!documentRow.rowData.runs
                             || documentRow.rowData.runs.length === 0)
                text: hostWindow.rowText(documentRow.rowData)
                color: hostWindow.textColor
                font.family: hostWindow.guiMonospaceFontFamily
                font.pixelSize: 13
                elide: Text.ElideRight
                z: 1
            }

            readonly property var terminalSelectionRange:
                documentRoot.terminalSelectionRangeForRow(
                    loaded ? Number(rowData.visualRow || 0) : -1, width)

            Rectangle {
                x: documentRoot.textHorizontalInset
                   + documentRow.terminalSelectionRange.start
                     * documentRoot.terminalCellWidth
                y: 0
                width: Math.max(0,
                    (documentRow.terminalSelectionRange.end
                     - documentRow.terminalSelectionRange.start)
                    * documentRoot.terminalCellWidth)
                height: parent.height
                visible: documentRoot.terminalSurface
                         && documentRow.loaded
                         && documentRow.terminalSelectionRange.valid
                color: hostWindow.selectedBg
                opacity: 0.72
                z: 0
            }
        }

        onContentYChanged: viewportController.contentYChanged()
        onHeightChanged: viewportController.scheduleNativeViewportSync()
        onMovementEnded: viewportController.movementEnded()
    }

    Rectangle {
        id: editorCursor
        objectName: "editorCursor"
        parent: documentList.contentItem
        property bool blinkOn: true
        readonly property bool block:
            documentRoot.cursorFrame.cursorShape === "block"
        readonly property int windowRow:
            frame.kind === "editor" || frame.kind === "terminal"
            ? documentRoot.indexForExtent(
                  Number(documentRoot.cursorFrame.cursorAbsoluteRow || 0),
                                          documentRoot.displayedRows)
            : -1
        x: documentRoot.textHorizontalInset + Math.max(0, Number(
                frame.kind === "terminal"
                ? documentRoot.cursorFrame.cursorX || 0
                : documentRoot.cursorFrame.cursorVisualColumn || 0))
                * documentFontMetrics.advanceWidth("M")
        y: (documentRoot.loadedSlotStart + Math.max(0, windowRow))
           * documentRoot.rowHeight
           + (block ? 1 : 2)
        width: block ? Math.max(1, documentFontMetrics.advanceWidth("M"))
                     : 2
        height: documentRoot.rowHeight - (block ? 2 : 4)
        color: "#ffffff"
        opacity: blinkOn ? 1 : 0
        visible: (frame.kind === "editor" || frame.kind === "terminal")
                 && documentRoot.cursorFrame.cursorVisible === true
                 && windowRow >= 0
                 && Number(frame.kind === "terminal"
                           ? documentRoot.cursorFrame.cursorX
                           : documentRoot.cursorFrame.cursorVisualColumn) >= 0
        z: 5

        onVisibleChanged: {
            if (visible)
                restartBlink()
        }

        function restartBlink() {
            blinkOn = true
            if (visible)
                editorCursorBlinkTimer.restart()
        }

        Connections {
            target: hostWindow
            function onKeyboardActivityRevisionChanged() {
                editorCursor.restartBlink()
            }
        }

        Timer {
            id: editorCursorBlinkTimer
            interval: 520
            running: editorCursor.visible
            repeat: true
            onTriggered: editorCursor.blinkOn = !editorCursor.blinkOn
        }
    }

    MouseArea {
        anchors.left: documentList.left
        anchors.right: documentScrollBar.visible
                       ? documentScrollBar.left : documentList.right
        anchors.top: documentList.top
        anchors.bottom: documentList.bottom
        acceptedButtons: frame.kind === "editor"
                         ? Qt.LeftButton | Qt.RightButton | Qt.MiddleButton
                         : documentRoot.terminalSelectionEnabled
                           ? Qt.LeftButton
                         : Qt.NoButton
        preventStealing: frame.kind === "editor"
                         || documentRoot.terminalSelectionEnabled
        propagateComposedEvents: true
        enabled: documentRoot.interactionActive
        cursorShape: frame.kind === "editor"
                     || documentRoot.terminalSelectionEnabled
                     ? Qt.IBeamCursor : Qt.ArrowCursor
        z: 8
        onPressed: mouse => {
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
            if (frame.kind === "editor" && mouse.buttons !== Qt.NoButton)
                documentRoot.sendEditorMouse(mouse, "move", true, false)
            else if (documentRoot.terminalSelectionDragging
                     && mouse.buttons !== Qt.NoButton) {
                documentRoot.updateTerminalSelectionPointer(mouse.x,
                                                              mouse.y)
                var point = documentRoot.terminalSelectionPointAtViewportEdge()
                documentRoot.extendTerminalSelectionTo(point.row,
                                                        point.column)
            }
        }
        onReleased: mouse => {
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

    T.ScrollBar {
        id: documentScrollBar
        objectName: "documentScrollBar"
        parent: documentRoot
        anchors.top: documentList.top
        anchors.bottom: documentList.bottom
        anchors.right: documentList.right
        anchors.rightMargin: Math.max(0,
                                      documentRoot.scrollBarRightInset)
        width: 15
        orientation: Qt.Vertical
        policy: T.ScrollBar.AlwaysOn
        hoverEnabled: true
        visible: documentRoot.hasWindowProtocol
                 && documentRoot.interactionActive
                 && documentRoot.contentExtentKnown
                 && documentRoot.contentExtent
                    > Math.max(0, Number(frame.viewportSpan || 0))
        z: 10

        contentItem: Rectangle {
            implicitWidth: 15
            implicitHeight: 15
            anchors.margins: 4
            radius: 4
            color: documentScrollBar.pressed ? "#505050"
                 : documentScrollBar.hovered ? "#676767" : "#4a4a4a"
        }
        background: Rectangle {
            color: documentScrollBar.hovered || documentScrollBar.pressed
                   ? Qt.rgba(1, 1, 1, 0.06) : "transparent"
            radius: 15
        }

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
        interactionActive: documentRoot.interactionActive
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
    }

    DocumentViewportController {
        id: viewportController
        hostWindow: documentRoot.hostWindow
        documentList: documentList
        documentScrollBar: documentScrollBar
        terminalSelectionController: terminalSelectionController
        editorPointerController: editorPointerController
        frame: documentRoot.frame
        embedded: documentRoot.embedded
        interactionActive: documentRoot.interactionActive
        showsConsoleTopBar: documentRoot.showsConsoleTopBar
        terminalSurface: documentRoot.terminalSurface
        rowHeight: documentRoot.rowHeight
    }
}
