pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Layouts

Rectangle {
    id: commandLineRoot
    required property ApplicationWindow hostWindow
    required property var commandLine
    property var shell: hostWindow.shellFrame()
    property bool nativeLayout: hostWindow.isAppScene()
    readonly property bool multiline: commandLine.multiline === true
    readonly property string sourceText: hostWindow.cleanText(commandLine.text)
    onSourceTextChanged: {
        preferredColumn = -1
        lastVerticalCursor = -1
        blockPreview = null
    }
    property var blockPreview: null
    readonly property var displayedBlock: blockPreview || commandLine
    function acknowledgeBlockPreview() {
        if (blockPreview === null || commandLineFocusArea.pressed)
            return
        if (commandLine.blockSelection === true
                && commandLine.blockAnchorRow === blockPreview.blockAnchorRow
                && commandLine.blockAnchorColumn === blockPreview.blockAnchorColumn
                && commandLine.blockFocusRow === blockPreview.blockFocusRow
                && commandLine.blockFocusColumn === blockPreview.blockFocusColumn)
            blockPreview = null
    }
    readonly property var displayModel: buildDisplayModel()
    property real preferredColumn: -1
    property int lastVerticalCursor: -1
    readonly property bool visualNavigationEnabled: dropInputEnabled && multiline
        && commandLine.focused === true && shell.terminalBusy !== true
        && (commandInput.lineCount > 1 || commandLine.ownsNavigation === true || shell.terminalActive === true)
        && !hostWindow.galleryViewerOwnsKeyboard()

    function sourcePosition(displayPosition) {
        const positions = displayModel.positions
        let lo = 0, hi = positions.length - 1
        while (lo < hi) {
            const mid = Math.floor((lo + hi) / 2)
            if (positions[mid] < displayPosition) lo = mid + 1
            else hi = mid
        }
        return lo
    }

    function blockPoint(x, y) {
        const cellWidth = Math.max(1, commandLineFontMetrics.advanceWidth("M"))
        return {
            "row": Math.max(0, Math.floor(y / textLineHeight)),
            "column": Math.max(0, Math.floor(x / cellWidth))
        }
    }

    function moveVisualCursor(direction) {
        const caret = commandInput.cursorRectangle
        const first = commandInput.positionToRectangle(0)
        const last = commandInput.positionToRectangle(displayModel.plain.length)
        const boundary = direction < 0 ? caret.y <= first.y : caret.y >= last.y
        if (boundary) {
            preferredColumn = -1
            lastVerticalCursor = -1
            if (commandInput.lineCount <= 1)
                hostWindow.action({action: "commandLine.history", direction: direction})
            return
        }
        if (preferredColumn < 0 || lastVerticalCursor !== commandInput.cursorPosition)
            preferredColumn = caret.x
        const position = commandInput.positionAt(preferredColumn,
                caret.y + direction * caret.height + caret.height / 2)
        lastVerticalCursor = position
        commandInput.cursorPosition = position
        hostWindow.action({action: "commandLine.cursor", cursorPosition: sourcePosition(position)})
    }

    Shortcut {
        sequence: "Up"
        context: Qt.WindowShortcut
        autoRepeat: true
        enabled: commandLineRoot.visualNavigationEnabled
        onActivated: commandLineRoot.moveVisualCursor(-1)
    }
    Shortcut {
        sequence: "Down"
        context: Qt.WindowShortcut
        autoRepeat: true
        enabled: commandLineRoot.visualNavigationEnabled
        onActivated: commandLineRoot.moveVisualCursor(1)
    }
    function buildDisplayModel() {
        const source = sourceText
        const splitArguments = multiline && commandLine.wordWrap === true
        let plain = "", html = "", quote = ""
        const positions = [], softStarts = []
        for (let i = 0; i < source.length; ++i) {
            const ch = source.charAt(i)
            if ((ch === "'" || ch === '"') && (i === 0 || source.charAt(i-1) !== "\\")) {
                if (quote === "") quote = ch
                else if (quote === ch) quote = ""
            }
            const argument = splitArguments && quote === "" && ch === "-"
                && i > 0 && /\s/.test(source.charAt(i-1)) && source.charAt(i-1) !== "\n"
            if (argument) {
                plain += "\n"
                html += "<br>"
                softStarts.push(plain.length)
            }
            positions.push(plain.length)
            const character = !multiline && (ch === "\n" || ch === "\r") ? " " : ch
            plain += character
            const escaped = character.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
            html += argument ? '<span style="color:#e6b450">-</span>'
                    : character === "\n" ? "<br>" : escaped
        }
        positions.push(plain.length)
        return { plain: plain, html: '<p style="margin:0;white-space:pre-wrap">' + html + '</p>',
                 positions: positions, softStarts: softStarts }
    }
    readonly property real rowHeight: hostWindow.snapPx(hostWindow.ch)
    // Empty TextEdit documents round their content height differently from
    // populated lines. Padding must depend on the font, not the document.
    readonly property real textLineHeight: commandLineMetrics.contentHeight
    readonly property real textTopInset:
        hostWindow.snapPx(Math.max(0, (rowHeight - textLineHeight) / 2))
    readonly property real inputContentHeight: hostWindow.snapPx(Math.max(rowHeight,
        Math.min(multiline ? Math.max(textLineHeight, commandInput.contentHeight)
                            + textTopInset * 2 : rowHeight,
                 hostWindow.height / 2)))
    property real scrollTop: 0
    readonly property real scrollExtent: commandInput.contentHeight + textTopInset * 2
    readonly property real maxScrollTop: multiline
        ? Math.max(0, scrollExtent - inputViewport.height) : 0
    function setScrollTop(value) {
        scrollTop = hostWindow.snapPx(Math.max(0, Math.min(maxScrollTop, value)))
    }
    function ensureCaretVisible() {
        if (!multiline) {
            setScrollTop(0)
            return
        }
        const caret = commandInput.cursorRectangle
        const top = scrollTop - textTopInset
        const bottom = top + inputViewport.height
        if (caret.y < top)
            setScrollTop(caret.y + textTopInset)
        else if (caret.y + caret.height > bottom)
            setScrollTop(caret.y + caret.height - inputViewport.height + textTopInset)
    }
    onMaxScrollTopChanged: setScrollTop(scrollTop)
    // TextEdit updates its content height and line count separately. Publish
    // only the settled height so one edit cannot resize both panels twice.
    function publishContentHeight() {
        const bridge = hostWindow.galleryControllerApi
        const tracing = bridge && bridge.benchmarkTraceEnabled === true
        if (tracing)
            bridge.recordBenchmarkStage(0, "command-line.resize.begin", {
                oldHeight: hostWindow.commandLineContentHeight, newHeight: inputContentHeight })
        hostWindow.commandLineContentHeight = inputContentHeight
        if (tracing)
            bridge.recordBenchmarkStage(0, "command-line.resize.end", {
                height: hostWindow.commandLineContentHeight })
    }
    onInputContentHeightChanged: Qt.callLater(publishContentHeight)
    objectName: "commandLineView"
    property bool dropHovered: false
    readonly property bool dropInputEnabled: visible && nativeLayout
        && !hostWindow.hasBlockingOverlay()
        && !hostWindow.hasStandaloneDocumentSurface()
        && !hostWindow.hasOperationsQueueSurface()

    Component.onCompleted: {
        Qt.callLater(publishContentHeight)
        if (typeof qtGallery !== "undefined"
                && typeof qtGallery.registerDragCommandLine === "function")
            qtGallery.registerDragCommandLine(commandLineRoot)
    }

    // A command-line patch changes text and caret position together.  A
    // TextInput may apply its own cursor reset while accepting the new
    // text after a declarative cursor binding has already run.  Reapply
    // the semantic caret on the next event-loop turn, once both values
    // have settled.
    onCommandLineChanged: {
        acknowledgeBlockPreview()
        commandInput.scheduleSemanticCursorSync()
    }

    x: nativeLayout ? 0 : hostWindow.pxX(commandLine.x)
    y: nativeLayout ? hostWindow.snapPx(hostWindow.height - hostWindow.keyBarHeight() - hostWindow.commandLineHeight(shell)) : hostWindow.pxY(commandLine.y)
    width: nativeLayout ? hostWindow.width : hostWindow.pxW(commandLine.w)
    height: nativeLayout ? hostWindow.commandLineHeight(shell)
                         : Math.max(hostWindow.ch, hostWindow.pxH(commandLine.h))
    visible: commandLine.visible !== false || (nativeLayout && hostWindow.commandLineReveal > 0)
    clip: true
    color: hostWindow.commandLineBg

    Item {
        id: commandPresentation
        objectName: "commandLinePresentation"
        anchors.fill: parent
        anchors.leftMargin: hostWindow.commandLineLeftMargin
        anchors.rightMargin: hostWindow.contentSpacing
        anchors.topMargin: hostWindow.commandLineVerticalMargin
                           + hostWindow.separatorWidth
        anchors.bottomMargin: hostWindow.commandLineVerticalMargin
                              + hostWindow.separatorWidth
        clip: true

        ConsoleRunRow {
            hostWindow: commandLineRoot.hostWindow
            id: commandPrompt
            objectName: "commandLinePrompt"
            anchors.left: parent.left
            y: -commandLineRoot.scrollTop
            width: hostWindow.snapPx(Math.min(contentWidth, commandPresentation.width * 0.5))
            height: commandLineRoot.rowHeight
            pixelAligned: true
            clip: true
            horizontalInset: 0
            transparentBlackBackground: true
            ignoreRunBackground: true
            runs: commandLine.promptRuns || []
            fallbackText: hostWindow.cleanText(commandLine.prompt)
        }

        Item {
            id: inputViewport
            objectName: "commandLineInputViewport"
            x: commandPrompt.width
            width: hostWindow.snapPx(Math.max(1, parent.width - x
                        - commandLineFontMetrics.advanceWidth("M")))
            height: parent.height
            clip: true

        WheelHandler {
            enabled: commandLineRoot.multiline && commandLineRoot.maxScrollTop > 0
            onWheel: event => commandLineRoot.setScrollTop(commandLineRoot.scrollTop
                - event.angleDelta.y / 120 * commandLineRoot.textLineHeight * 3)
        }

        TextEdit {
            id: commandInput
            objectName: "commandLineInput"
            z: 2
            x: commandLineRoot.multiline ? 0 : -hostWindow.snapPx(Math.max(0,
                   cursorRectangle.x + commandLineFontMetrics.advanceWidth("M") - inputViewport.width))
            y: commandLineRoot.textTopInset - commandLineRoot.scrollTop
            width: inputViewport.width
            height: Math.max(commandLineRoot.rowHeight, contentHeight)
            readonly property string presentationText: commandLineRoot.multiline && commandLine.wordWrap === true
                    ? commandLineRoot.displayModel.html : commandLineRoot.displayModel.plain
            readonly property int presentationFormat: commandLineRoot.multiline && commandLine.wordWrap === true
                        ? TextEdit.RichText : TextEdit.PlainText
            wrapMode: !commandLineRoot.multiline ? TextEdit.NoWrap
                      : commandLine.wordWrap === true ? TextEdit.WrapAtWordBoundaryOrAnywhere
                      : TextEdit.WrapAnywhere
            color: hostWindow.textColor
            selectionColor: hostWindow.selectedBg
            selectedTextColor: hostWindow.textColor
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: hostWindow.semanticTextFontPixelSize
            verticalAlignment: TextEdit.AlignTop
            readOnly: true
            activeFocusOnPress: false
            cursorDelegate: Item { width: 0; height: 0 }

            function semanticCursorPosition() {
                const positions = commandLineRoot.displayModel.positions
                return positions[Math.max(0, Math.min(positions.length - 1,
                                Number(commandLine.cursorPosition || 0)))]
            }

            function syncSemanticCursor() {
                // Older server echoes must not move the viewport under a drag.
                if (commandLineFocusArea.pressed || commandLineRoot.blockPreview !== null)
                    return
                // Apply format before text in one transaction. Switching a
                // rich document to plain text otherwise serializes its HTML
                // into the command when the two bindings settle separately.
                if (textFormat !== presentationFormat)
                    textFormat = presentationFormat
                if (text !== presentationText)
                    text = presentationText
                var position = semanticCursorPosition()
                const start = commandLine.blockSelection === true ? -1
                    : commandLine.selectionStart === undefined ? -1
                    : Number(commandLine.selectionStart)
                const end = commandLine.selectionEnd === undefined ? -1
                    : Number(commandLine.selectionEnd)
                if (cursorPosition !== position)
                    cursorPosition = position
                if (start >= 0 && end > start) {
                    const positions = commandLineRoot.displayModel.positions
                    const a = positions[Math.max(0, Math.min(positions.length - 1, start))]
                    const b = positions[Math.max(0, Math.min(positions.length - 1, end))]
                    if (selectionStart !== a || selectionEnd !== b
                            || cursorPosition !== position) {
                        // TextEdit.select() leaves its cursor at the second
                        // argument. Preserve Go's active end so backward
                        // Shift+Left selections survive synchronization.
                        if (position <= a)
                            select(b, a)
                        else
                            select(a, b)
                    }
                } else if (selectionStart !== selectionEnd) {
                    deselect()
                }
            }

            function scheduleSemanticCursorSync() {
                commandCursorSyncTimer.restart()
            }

            onPresentationTextChanged: scheduleSemanticCursorSync()
            onPresentationFormatChanged: scheduleSemanticCursorSync()
            onCursorRectangleChanged: Qt.callLater(commandLineRoot.ensureCaretVisible)
            Component.onCompleted: scheduleSemanticCursorSync()

            Timer {
                id: commandCursorSyncTimer
                interval: 0
                repeat: false
                onTriggered: commandInput.syncSemanticCursor()
            }
        }

        Item {
            id: commandBlockSelectionOverlay
            objectName: "commandLineBlockSelectionOverlay"
            anchors.fill: parent
            z: 1
            visible: commandLineRoot.multiline && commandLineRoot.displayedBlock.blockSelection === true
            clip: true
            readonly property int firstRow: Math.max(
                Math.floor(commandLineRoot.scrollTop / commandLineRoot.textLineHeight),
                Math.min(Number(commandLineRoot.displayedBlock.blockAnchorRow || 0),
                         Number(commandLineRoot.displayedBlock.blockFocusRow || 0)))
            readonly property int lastRow: Math.min(
                Math.ceil((commandLineRoot.scrollTop + inputViewport.height) / commandLineRoot.textLineHeight),
                Math.max(Number(commandLineRoot.displayedBlock.blockAnchorRow || 0),
                         Number(commandLineRoot.displayedBlock.blockFocusRow || 0)))
            readonly property int firstColumn: Math.min(
                Number(commandLineRoot.displayedBlock.blockAnchorColumn || 0),
                Number(commandLineRoot.displayedBlock.blockFocusColumn || 0))
            readonly property int lastColumn: Math.max(
                Number(commandLineRoot.displayedBlock.blockAnchorColumn || 0),
                Number(commandLineRoot.displayedBlock.blockFocusColumn || 0))

            Repeater {
                model: commandBlockSelectionOverlay.visible
                       ? Math.max(0, commandBlockSelectionOverlay.lastRow
                         - commandBlockSelectionOverlay.firstRow + 1) : 0
                delegate: Rectangle {
                    required property int index
                    objectName: "commandLineBlockSelectionRow" + index
                    x: commandBlockSelectionOverlay.firstColumn
                       * commandLineFontMetrics.advanceWidth("M")
                    y: commandLineRoot.textTopInset
                       + (commandBlockSelectionOverlay.firstRow + index)
                         * commandLineRoot.textLineHeight
                       - commandLineRoot.scrollTop
                    width: Math.max(0, commandBlockSelectionOverlay.lastColumn
                                       - commandBlockSelectionOverlay.firstColumn)
                           * commandLineFontMetrics.advanceWidth("M")
                    height: commandLineRoot.textLineHeight
                    visible: width > 0 && y + height > 0 && y < parent.height
                    color: commandLineRoot.hostWindow.selectedBg
                    opacity: 0.72
                }
            }
        }
        }
    }

    F4ScrollBar {
        id: commandScrollBar
        objectName: "commandLineScrollBar"
        hostWindow: commandLineRoot.hostWindow
        anchors.top: inputViewport.top
        anchors.bottom: inputViewport.bottom
        anchors.right: commandPresentation.right
        thickness: 12
        orientation: Qt.Vertical
        policy: T.ScrollBar.AlwaysOn
        visible: commandLineRoot.maxScrollTop > 0
        size: Math.min(1, inputViewport.height / Math.max(1, commandLineRoot.scrollExtent))
        position: commandLineRoot.maxScrollTop > 0
            ? commandLineRoot.scrollTop / commandLineRoot.scrollExtent : 0
        z: 4
        onPositionChanged: if (pressed)
            commandLineRoot.setScrollTop(position * commandLineRoot.scrollExtent)
    }

    Repeater {
        model: commandLineRoot.multiline
               ? Math.ceil(commandPresentation.height / Math.max(1, commandLineRoot.textLineHeight)) : 0
        delegate: Text {
            required property int index
            objectName: "commandLineWrapMarker" + index
            readonly property real lineY: (Math.floor(commandLineRoot.scrollTop
                    / commandLineRoot.textLineHeight) + index) * commandLineRoot.textLineHeight
            readonly property int nextPosition: {
                const revision = commandInput.width + commandInput.lineCount
                    + commandInput.contentHeight + commandLineRoot.displayModel.plain.length
                return commandInput.positionAt(0, lineY + commandLineRoot.textLineHeight + 1)
                    + revision * 0
            }
            visible: nextPosition > 0 && nextPosition < commandLineRoot.displayModel.plain.length
                     && (commandLineRoot.displayModel.plain.charAt(nextPosition - 1) !== "\n"
                         || commandLineRoot.displayModel.softStarts.indexOf(nextPosition) >= 0)
            x: hostWindow.snapPx(commandPresentation.x + commandPresentation.width - width)
            y: hostWindow.snapPx(commandPresentation.y + commandLineRoot.textTopInset
                                + lineY - commandLineRoot.scrollTop)
            text: "-"
            color: "#e6b450"
            font: commandInput.font
        }
    }

    FontMetrics {
        id: commandLineFontMetrics
        font: commandInput.font
    }

    // Use the same text engine as the input: FontMetrics omits the document's
    // line-height rounding. This also keeps an empty rich-text paragraph from
    // shrinking the command bar when its last character is removed.
    TextEdit {
        id: commandLineMetrics
        objectName: "commandLineMetrics"
        visible: false
        text: "M"
        font: commandInput.font
        textFormat: TextEdit.PlainText
        readOnly: true
    }

    Rectangle {
        id: commandCursor
        objectName: "commandLineCursor"
        property alias blinkOn: commandCursorBlinkController.blinkOn
        property alias blinkInterval: commandCursorBlinkController.interval
        readonly property bool blinkTimerRunning:
            commandCursorBlinkController.running
        readonly property bool block: commandLine.cursorShape === "block"
        readonly property bool graphical: hostWindow.commandLineGraphicalCursor
        readonly property int textPosition: commandInput.cursorPosition
        readonly property rect caretRect: commandInput.cursorRectangle
        x: hostWindow.snapPx(commandPresentation.x + inputViewport.x + commandInput.x + caretRect.x)
        y: hostWindow.snapPx(commandPresentation.y + commandInput.y + caretRect.y
                  + (block || graphical ? 0 : caretRect.height - height))
        width: hostWindow.snapPx(graphical && !block ? 2
                  : Math.max(1, commandLineFontMetrics.advanceWidth("M")))
        height: block || graphical ? hostWindow.snapPx(caretRect.height) : hostWindow.snapPx(2)
        color: graphical ? hostWindow.textColor : "#ffffff"
        visible: commandLine.cursorVisible === true
        opacity: blinkOn ? 1 : 0
        z: 2

        function restartBlink() {
            commandCursorBlinkController.restart()
        }

        ActivityBoundedCursorBlink {
            id: commandCursorBlinkController
            objectName: "commandLineCursorBlinkController"
            active: commandCursor.visible
                    && commandLineRoot.visible
                    && hostWindow.active
                    && hostWindow.isAppScene()
                    && !hostWindow.needsFallbackGrid()
                    && !hostWindow.hasBlockingOverlay()
                    && !hostWindow.hasStandaloneDocumentSurface()
                    && !hostWindow.hasOperationsQueueSurface()
                    && (!hostWindow.galleryControllerApi
                        || !hostWindow.galleryControllerApi.viewerVisible)
            activityRevision: hostWindow.keyboardActivityRevision
        }
    }

    Rectangle {
        objectName: "commandLineDropOutline"
        z: 100
        visible: commandLineRoot.dropHovered && commandLineRoot.dropInputEnabled
        color: "transparent"
        border.color: hostWindow.gallerySelectionColor
        border.width: 2 / hostWindow.dpr
        readonly property rect snappedBounds: {
            const revision = commandLineRoot.x + commandLineRoot.y
                + commandLineRoot.width + commandLineRoot.height
            const start = commandLineRoot.mapToItem(null, 0, 0)
            const end = commandLineRoot.mapToItem(null, commandLineRoot.width, commandLineRoot.height)
            const a = commandLineRoot.mapFromItem(null, hostWindow.snapPx(start.x), hostWindow.snapPx(start.y))
            const b = commandLineRoot.mapFromItem(null, hostWindow.snapPx(end.x), hostWindow.snapPx(end.y))
            return Qt.rect(a.x + revision * 0, a.y, b.x-a.x, b.y-a.y)
        }
        x: snappedBounds.x
        y: snappedBounds.y
        width: snappedBounds.width
        height: snappedBounds.height
    }

    MouseArea {
        id: commandLineFocusArea
        objectName: "commandLineFocusArea"
        anchors.fill: parent
        z: 3
        enabled: commandLineRoot.dropInputEnabled
        acceptedButtons: Qt.LeftButton
        cursorShape: Qt.IBeamCursor
        property int selectionAnchor: -1
        property int wordAnchorStart: -1
        property int wordAnchorEnd: -1
        property int clickCount: 0
        property bool blockGesture: false
        property int blockAnchorRow: 0
        property int blockAnchorColumn: 0
        property real lastClickX: -10000
        property real lastClickY: -10000
        property double lastClickAt: 0
        function selectAt(anchor, cursor, block, anchorRow, anchorColumn, focusRow, focusColumn) {
            if (block === true)
                commandInput.deselect()
            else
                commandInput.select(anchor, cursor)
            const action = {action: "commandLine.select",
                anchor: commandLineRoot.sourcePosition(anchor),
                cursorPosition: commandLineRoot.sourcePosition(cursor),
                block: block === true}
            if (block === true) {
                const cellWidth = Math.max(1, commandLineFontMetrics.advanceWidth("M"))
                action.blockAnchorRow = anchorRow
                action.blockAnchorColumn = anchorColumn
                action.blockFocusRow = focusRow
                action.blockFocusColumn = focusColumn
                action.blockWrapWidth = Math.max(1, Math.floor(inputViewport.width / cellWidth))
                commandLineRoot.blockPreview = {
                    blockSelection: true, blockAnchorRow: anchorRow,
                    blockAnchorColumn: anchorColumn, blockFocusRow: focusRow,
                    blockFocusColumn: focusColumn
                }
            } else {
                commandLineRoot.blockPreview = null
            }
            hostWindow.action(action)
        }
        onPressed: mouse => {
            const point = commandInput.mapFromItem(commandLineFocusArea, mouse.x, mouse.y)
            const position = commandInput.positionAt(Math.max(0, point.x), Math.max(0, point.y))
            const now = Date.now()
            clickCount = now - lastClickAt <= 400
                && Math.abs(mouse.x - lastClickX) < 5 && Math.abs(mouse.y - lastClickY) < 5
                ? Math.min(3, clickCount + 1) : 1
            lastClickAt = now
            lastClickX = mouse.x
            lastClickY = mouse.y
            commandLineRoot.preferredColumn = -1
            commandLineRoot.lastVerticalCursor = -1
            commandInput.cursorPosition = position
            hostWindow.action({action: "commandLine.focus",
                               cursorPosition: commandLineRoot.sourcePosition(position)})
            selectionAnchor = position
            wordAnchorStart = -1
            wordAnchorEnd = -1
            blockGesture = commandLineRoot.multiline
                && (mouse.modifiers & Qt.AltModifier) !== 0
            if (blockGesture) {
                clickCount = 0
                const blockAnchor = commandLineRoot.blockPoint(point.x, point.y)
                blockAnchorRow = blockAnchor.row
                blockAnchorColumn = blockAnchor.column
                selectAt(position, position, false)
            } else if (clickCount === 2) {
                commandInput.selectWord()
                wordAnchorStart = commandInput.selectionStart
                wordAnchorEnd = commandInput.selectionEnd
                selectAt(commandInput.selectionStart, commandInput.selectionEnd)
            } else if (clickCount === 3) {
                const plain = commandLineRoot.displayModel.plain
                const start = plain.lastIndexOf("\n", Math.max(0, position - 1)) + 1
                const next = plain.indexOf("\n", position)
                selectAt(start, next < 0 ? plain.length : next)
                clickCount = 0
            }
        }
        onPositionChanged: mouse => {
            if (!pressed || selectionAnchor < 0 || clickCount > 2)
                return
            const point = commandInput.mapFromItem(commandLineFocusArea, mouse.x, mouse.y)
            const position = commandInput.positionAt(Math.max(0, point.x), Math.max(0, point.y))
            if (blockGesture) {
                const blockFocus = commandLineRoot.blockPoint(point.x, point.y)
                selectAt(selectionAnchor, position, true,
                         blockAnchorRow, blockAnchorColumn,
                         blockFocus.row, blockFocus.column)
            } else if (clickCount === 2 && wordAnchorStart >= 0) {
                commandInput.cursorPosition = position
                commandInput.selectWord()
                const start = commandInput.selectionStart
                const end = commandInput.selectionEnd
                if (position < wordAnchorStart)
                    selectAt(wordAnchorEnd, start)
                else if (position >= wordAnchorEnd)
                    selectAt(wordAnchorStart, end)
                else
                    selectAt(wordAnchorStart, wordAnchorEnd)
            } else {
                selectAt(selectionAnchor, position)
            }
        }
        onReleased: {
            const wasBlock = blockGesture
            selectionAnchor = -1
            blockGesture = false
            if (wasBlock) {
                commandLineRoot.acknowledgeBlockPreview()
                commandInput.scheduleSemanticCursorSync()
            }
        }
        onCanceled: {
            selectionAnchor = -1
            blockGesture = false
            commandLineRoot.blockPreview = null
            commandInput.scheduleSemanticCursorSync()
        }
    }

    Rectangle {
        objectName: "commandLineTopSeparator"
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        height: hostWindow.separatorWidth
        color: hostWindow.separatorColor
    }
}
