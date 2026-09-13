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
    readonly property var displayModel: buildDisplayModel()
    function buildDisplayModel() {
        const source = hostWindow.cleanText(commandLine.text)
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
    readonly property real textLineHeight: commandInput.lineCount > 0
        ? Math.max(1, commandInput.contentHeight / commandInput.lineCount) : rowHeight
    readonly property real textTopInset:
        hostWindow.snapPx(Math.max(0, (rowHeight - textLineHeight) / 2))
    readonly property real inputContentHeight: hostWindow.snapPx(Math.max(rowHeight,
        Math.min(multiline ? commandInput.contentHeight + textTopInset * 2 : rowHeight,
                 hostWindow.height / 2)))
    readonly property real scrollTop: multiline
        ? hostWindow.snapPx(Math.max(0, commandInput.cursorRectangle.y
            + commandInput.cursorRectangle.height - inputContentHeight + textTopInset * 2)) : 0
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
    onCommandLineChanged: commandInput.scheduleSemanticCursorSync()

    x: nativeLayout ? 0 : hostWindow.pxX(commandLine.x)
    y: nativeLayout ? hostWindow.snapPx(hostWindow.height - hostWindow.keyBarHeight() - hostWindow.commandLineHeight(shell)) : hostWindow.pxY(commandLine.y)
    width: nativeLayout ? hostWindow.width : hostWindow.pxW(commandLine.w)
    height: nativeLayout ? hostWindow.commandLineHeight(shell)
                         : Math.max(hostWindow.ch, hostWindow.pxH(commandLine.h))
    visible: commandLine.visible !== false
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

        TextEdit {
            id: commandInput
            objectName: "commandLineInput"
            x: commandLineRoot.multiline ? 0 : -hostWindow.snapPx(Math.max(0,
                   cursorRectangle.x + commandLineFontMetrics.advanceWidth("M") - inputViewport.width))
            y: commandLineRoot.textTopInset - commandLineRoot.scrollTop
            width: inputViewport.width
            height: Math.max(commandLineRoot.rowHeight, contentHeight)
            text: commandLineRoot.multiline && commandLine.wordWrap === true
                    ? commandLineRoot.displayModel.html : commandLineRoot.displayModel.plain
            textFormat: commandLineRoot.multiline && commandLine.wordWrap === true
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
                var position = semanticCursorPosition()
                if (cursorPosition !== position)
                    cursorPosition = position
            }

            function scheduleSemanticCursorSync() {
                commandCursorSyncTimer.restart()
            }

            onTextChanged: scheduleSemanticCursorSync()
            Component.onCompleted: scheduleSemanticCursorSync()

            Timer {
                id: commandCursorSyncTimer
                interval: 0
                repeat: false
                onTriggered: commandInput.syncSemanticCursor()
            }
        }
        }
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
        objectName: "commandLineFocusArea"
        anchors.fill: parent
        z: 3
        enabled: commandLineRoot.dropInputEnabled
        acceptedButtons: Qt.LeftButton
        cursorShape: Qt.IBeamCursor
        onPressed: hostWindow.action({action: "commandLine.focus"})
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
