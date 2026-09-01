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
    objectName: "commandLineView"

    // A command-line patch changes text and caret position together.  A
    // TextInput may apply its own cursor reset while accepting the new
    // text after a declarative cursor binding has already run.  Reapply
    // the semantic caret on the next event-loop turn, once both values
    // have settled.
    onCommandLineChanged: commandInput.scheduleSemanticCursorSync()

    x: nativeLayout ? 0 : hostWindow.pxX(commandLine.x)
    y: nativeLayout ? hostWindow.height - hostWindow.keyBarHeight() - hostWindow.commandLineHeight(shell) : hostWindow.pxY(commandLine.y)
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
            anchors.verticalCenter: parent.verticalCenter
            width: Math.min(contentWidth, commandPresentation.width * 0.5)
            height: parent.height
            clip: true
            horizontalInset: 0
            transparentBlackBackground: true
            ignoreRunBackground: true
            runs: commandLine.promptRuns || []
            fallbackText: hostWindow.cleanText(commandLine.prompt)
        }

        TextInput {
            id: commandInput
            objectName: "commandLineInput"
            anchors.left: commandPrompt.right
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            height: parent.height
            text: hostWindow.cleanText(commandLine.text)
            color: hostWindow.textColor
            selectionColor: hostWindow.selectedBg
            selectedTextColor: hostWindow.textColor
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: hostWindow.semanticTextFontPixelSize
            verticalAlignment: TextInput.AlignVCenter
            readOnly: true
            activeFocusOnPress: false
            cursorDelegate: Item { width: 0; height: 0 }

            function semanticCursorPosition() {
                return Math.max(0, Math.min(text.length,
                                Number(commandLine.cursorPosition || 0)))
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

    FontMetrics {
        id: commandLineFontMetrics
        font.family: hostWindow.guiMonospaceFontFamily
        font.pixelSize: hostWindow.guiMonospaceFontPixelSize
    }

    Rectangle {
        id: commandCursor
        objectName: "commandLineCursor"
        property bool blinkOn: true
        readonly property bool block: commandLine.cursorShape === "block"
        readonly property int textPosition: commandInput.cursorPosition
        readonly property rect caretRect: commandInput.cursorRectangle
        x: commandPresentation.x + commandInput.x + caretRect.x
        y: block ? commandPresentation.y
                 : commandPresentation.y + commandPresentation.height - 2
        width: Math.max(1, commandLineFontMetrics.advanceWidth("M"))
        height: block ? commandPresentation.height : 2
        color: "#ffffff"
        visible: commandLine.cursorVisible === true
        opacity: blinkOn ? 1 : 0
        z: 2

        function restartBlink() {
            blinkOn = true
            if (visible)
                commandCursorBlinkTimer.restart()
        }

        onVisibleChanged: {
            if (visible)
                restartBlink()
        }

        Connections {
            target: hostWindow
            function onKeyboardActivityRevisionChanged() {
                commandCursor.restartBlink()
            }
        }

        Timer {
            id: commandCursorBlinkTimer
            interval: 520
            running: commandCursor.visible
            repeat: true
            onTriggered: commandCursor.blinkOn = !commandCursor.blinkOn
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
