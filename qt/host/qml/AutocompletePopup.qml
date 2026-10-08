pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: autocompleteOverlay
    objectName: "autocompleteOverlay"
    required property ApplicationWindow hostWindow
    required property Item menuBar
    property var frame: ({})
    property var items: frame.items || []
    property point pointerSelectionPosition: hostWindow.focusTarget.pointerScreenPosition()
    onCommandLineChanged: pointerSelectionPosition = hostWindow.focusTarget.pointerScreenPosition()
    property Item hoveredRow: null
    property real hoveredSceneY: 0

    property bool pointerWarpPending: false
    readonly property var inputAnchor: {
        const revision = hostWindow.dropdownAnchorRevision
        return revision >= 0 ? hostWindow.dropdownAnchorForId(frame.ownerId) : null
    }
    property rect inputRect: Qt.rect(0, 0, 0, 0)
    function syncInputRect() {
        if (inputAnchor)
            inputRect = inputAnchor.mapToItem(hostWindow.contentItem, 0, 0,
                                             inputAnchor.width, inputAnchor.height)
    }
    onInputAnchorChanged: Qt.callLater(syncInputRect)

    function preserveHoveredRow() {
        if (!hoveredRow)
            return false
        if (hoveredRow.index !== hostWindow.autocompleteSelectedIndex) {
            hoveredRow = null
            pointerWarpPending = false
            return false
        }
        const sceneY = hoveredRow.mapToItem(null, 0, 0).y
        if (Math.abs(sceneY - hoveredSceneY) < 0.001)
            return false
        // Freeze selection immediately, but move the native cursor only once
        // the frame containing the popup's new position has been presented.
        pointerWarpPending = true
        return true
    }

    function hoverRow(row, x, y) {
        if (pointerWarpPending || preserveHoveredRow())
            return
        // A callLater is not a mouse-event barrier. Discard events queued at
        // the old position, even after the popup and pointer have settled.
        if (!hostWindow.pointerSelectionMoved(autocompleteOverlay, row, x, y))
            return
        hoveredRow = row
        hoveredSceneY = row.mapToItem(null, 0, 0).y
        hostWindow.autocompleteSelectedIndex = row.index
    }

    Connections {
        target: autocompleteOverlay.hostWindow
        function onAfterAnimating() { autocompleteOverlay.syncInputRect() }
        function onFrameSwapped() {
            if (!autocompleteOverlay.pointerWarpPending || !autocompleteOverlay.hoveredRow)
                return
            const row = autocompleteOverlay.hoveredRow
            const previousY = autocompleteOverlay.hoveredSceneY
            autocompleteOverlay.hoveredSceneY = row.mapToItem(null, 0, 0).y
            autocompleteOverlay.pointerWarpPending = false
            autocompleteOverlay.hostWindow.focusTarget.preservePointerRowOffset(row, previousY)
            autocompleteOverlay.pointerSelectionPosition = autocompleteOverlay.hostWindow.focusTarget.pointerScreenPosition()
        }
    }

    readonly property var commandLine: hostWindow.commandLineFrame()
    readonly property real commandLineX: hostWindow.isAppScene()
                                             ? 0 : hostWindow.pxX(commandLine.x || 0)
    readonly property real commandLineY: hostWindow.isAppScene()
                                             ? hostWindow.height - hostWindow.keyBarHeight()
                                               - hostWindow.commandLineHeight(hostWindow.shellFrame())
                                             : hostWindow.pxY(commandLine.y || 0)
    // CommandLine exports the authoritative Edit start column.  Its
    // prompt is monospaced, so translating that column with the same
    // Configured monospace metrics used by ConsoleRunRow land on the exact input x.
    readonly property real inputTextX: commandLineX
                                       + hostWindow.commandLineLeftMargin
                                       + Number(commandLine.inputX || 0)
                                         * commandLineFontMetrics.advanceWidth("M")
    // ListView has a 4 px inset and each row has another 8 px text
    // inset. Offset the panel itself so the glyphs—not its border—
    // align with the command-line input.
    readonly property real preferredX: hostWindow.snapPx(Math.max(0, inputAnchor ? inputRect.x : inputTextX - 12))
    readonly property real rowHeight: hostWindow.snapPx(Math.max(22, hostWindow.ch * 1.15))
    function itemText(item) {
        return hostWindow.cleanText(item.text).replace(/\r\n?|\u2028|\u2029/g, "\n")
    }
    readonly property real preferredHeight: {
        let result = 8
        for (let i = 0; i < Math.min(12, items.length); ++i) {
            const lines = itemText(items[i]).split("\n").length
            result += lines === 1 ? rowHeight : Math.max(rowHeight,
                Math.ceil(lines * autocompleteFontMetrics.height * hostWindow.dpr)
                    / hostWindow.dpr + hostWindow.snapPx(8))
        }
        return result
    }
    readonly property real maxHeight: Math.max(rowHeight + 8,
                                               commandLineY - hostWindow.menuBarHeight)
    readonly property real availableWidth: Math.max(1,
                                                     hostWindow.width - preferredX - 6)
    readonly property real contentWidth: {
        var widest = 0
        for (var i = 0; i < items.length; ++i) {
            const lines = itemText(items[i]).split("\n")
            for (const line of lines)
                widest = Math.max(widest, autocompleteFontMetrics.advanceWidth(line))
        }
        return widest + 24
    }

    FontMetrics {
        id: autocompleteFontMetrics
        font.family: hostWindow.uiFontFamily
        font.pixelSize: (hostWindow ? hostWindow.uiTextSize(13) : 13)
    }

    FontMetrics {
        id: commandLineFontMetrics
        font.family: hostWindow.uiFontFamily
        font.pixelSize: autocompleteOverlay.commandLine.runs
                        && autocompleteOverlay.commandLine.runs.length > 0
                        ? 13 : 18
    }

    PointHandler {
        target: null
        acceptedButtons: Qt.AllButtons
        onActiveChanged: {
            if (!active) return
            const mapped = autocompleteOverlay.mapToItem(hintsPanel, point.position.x, point.position.y)
            if (!hintsPanel.contains(mapped)) {
                const shell = hostWindow.shellFrame()
                hostWindow.action({
                    "target": inputAnchor ? frame.id : hostWindow.cleanText(shell.id) || frame.id,
                    "action": inputAnchor ? "autocomplete.dismiss" : "command.complete"
                }, true)
            }
        }
    }

    Rectangle {
        id: hintsPanel
        objectName: "autocompleteHintsPanel"
        onYChanged: autocompleteOverlay.preserveHoveredRow()
        x: autocompleteOverlay.preferredX
        y: hostWindow.snapPx(autocompleteOverlay.inputAnchor
            ? (autocompleteOverlay.inputRect.y + autocompleteOverlay.inputRect.height + height <= hostWindow.height - hostWindow.keyBarHeight()
               ? autocompleteOverlay.inputRect.y + autocompleteOverlay.inputRect.height
               : Math.max(hostWindow.menuBarHeight, autocompleteOverlay.inputRect.y - height))
            : Math.max(hostWindow.menuBarHeight, autocompleteOverlay.commandLineY - height))
        width: hostWindow.snapPx(Math.min(autocompleteOverlay.availableWidth,
                        Math.max(80, autocompleteOverlay.contentWidth)))
        height: hostWindow.snapPx(Math.min(autocompleteOverlay.maxHeight,
            Math.max(autocompleteOverlay.rowHeight + 8, autocompleteOverlay.preferredHeight)))
        color: "#202833"
        radius: 4
        border.width: 1
        border.color: hostWindow.dialogAccent
        clip: true
        z: 170

        ListView {
            id: autocompleteList
            objectName: "autocompleteList"
            anchors.fill: parent
            anchors.margins: 4
            model: autocompleteOverlay.items
            clip: true
            currentIndex: hostWindow.autocompleteMenuId === hostWindow.cleanText(autocompleteOverlay.frame.id)
                          ? hostWindow.autocompleteSelectedIndex : -1
            boundsBehavior: Flickable.StopAtBounds
            interactive: contentHeight > height
            ScrollBar.vertical: F4ScrollBar {
                hostWindow: autocompleteOverlay.hostWindow
                policy: ScrollBar.AsNeeded
            }
            onCurrentIndexChanged: {
                if (currentIndex >= 0)
                    positionViewAtIndex(currentIndex, ListView.Contain)
            }

            delegate: Rectangle {
                id: hintRow
                objectName: "autocompleteHint-" + index
                required property int index
                required property var modelData
                readonly property string fullText: autocompleteOverlay.itemText(modelData)
                readonly property bool multiline: fullText.indexOf("\n") >= 0
                width: ListView.view.width
                height: multiline
                    ? Math.max(autocompleteOverlay.rowHeight,
                        Math.ceil(completionMultilineLabel.implicitHeight * hostWindow.dpr)
                            / hostWindow.dpr + hostWindow.snapPx(8))
                    : autocompleteOverlay.rowHeight
                radius: 3
                color: index === autocompleteList.currentIndex
                       ? hostWindow.selectedBg : "transparent"

                Row {
                    id: completionTextRow
                    visible: !hintRow.multiline
                    anchors.left: parent.left
                    anchors.right: parent.right
                    y: hostWindow.snapPx((parent.height - height) / 2)
                    anchors.leftMargin: 8
                    anchors.rightMargin: 8
                    readonly property string fullText: hintRow.fullText
                    readonly property string query: hostWindow.autocompleteQuery
                    readonly property int matchingLength:
                        fullText.toLocaleLowerCase().indexOf(query.toLocaleLowerCase()) === 0
                        ? Math.min(query.length, fullText.length) : 0

                    Text {
                        id: completionPrefixLabel
                        objectName: "autocompletePrefix-" + hintRow.index
                        width: hostWindow.snapPx(implicitWidth)
                        height: hostWindow.snapPx(implicitHeight)
                        text: completionTextRow.fullText.substring(
                                  0, completionTextRow.matchingLength)
                        color: hostWindow.dialogAccent
                        font.family: hostWindow.uiFontFamily
                        font.pixelSize: (hostWindow ? hostWindow.uiTextSize(13) : 13)
                    }

                    Text {
                        objectName: "autocompleteSuffix-" + hintRow.index
                        height: hostWindow.snapPx(implicitHeight)
                        width: Math.max(0, completionTextRow.width
                                        - completionPrefixLabel.width)
                        text: completionTextRow.fullText.substring(
                                  completionTextRow.matchingLength)
                        color: hostWindow.textColor
                        font.family: hostWindow.uiFontFamily
                        font.pixelSize: (hostWindow ? hostWindow.uiTextSize(13) : 13)
                        elide: Text.ElideRight
                    }
                }

                Column {
                    id: completionMultilineLabel
                    objectName: "autocompleteMultiline-" + hintRow.index
                    visible: hintRow.multiline
                    x: hostWindow.snapPx(8)
                    y: hostWindow.snapPx(4)
                    width: Math.max(0, parent.width - hostWindow.snapPx(24))
                    height: Math.ceil(implicitHeight * hostWindow.dpr) / hostWindow.dpr
                    Repeater {
                        model: hintRow.fullText.split("\n")
                        delegate: Text {
                            required property int index
                            required property string modelData
                            objectName: "autocompleteLogicalLine-" + hintRow.index + "-" + index
                            width: completionMultilineLabel.width
                            height: Math.ceil(implicitHeight * hostWindow.dpr) / hostWindow.dpr
                            property real continuationLineX: 0
                            readonly property real continuationIndent: hostWindow.snapPx(
                                autocompleteFontMetrics.advanceWidth("  "))
                            text: {
                                const lines = hintRow.fullText.split("\n")
                                let start = 0
                                for (let i = 0; i < index; ++i)
                                    start += lines[i].length + 1
                                const length = Math.max(0, completionTextRow.matchingLength - start)
                                const prefix = hostWindow.richTextEscape(modelData.slice(0, length))
                                const suffix = hostWindow.richTextEscape(modelData.slice(length))
                                return '<font color="' + hostWindow.dialogAccent + '">'
                                    + prefix + '</font>' + suffix
                            }
                            textFormat: Text.StyledText
                            wrapMode: Text.Wrap
                            color: hostWindow.textColor
                            font.family: hostWindow.uiFontFamily
                            font.pixelSize: hostWindow.uiTextSize(13)
                            onLineLaidOut: (line) => {
                                if (line.number > 0) {
                                    line.x = continuationIndent
                                    line.width = Math.max(1, width - continuationIndent)
                                    continuationLineX = line.x
                                }
                            }
                        }
                    }
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(completionMultilineLabel, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(completionMultilineLabel, hostWindow.contentItem)
                    }
                }

                MouseArea {
                    id: mouseArea
                    anchors.fill: parent
                    hoverEnabled: true
                    acceptedButtons: Qt.LeftButton
                    onPositionChanged: (mouse) => {
                        if (containsMouse)
                            autocompleteOverlay.hoverRow(hintRow, mouse.x, mouse.y)
                    }
                    onPressed: (mouse) => {
                        hostWindow.autocompleteSelectedIndex = index
                        mouse.accepted = true
                    }
                    onClicked: hostWindow.completeAutocomplete()
                }
            }
        }
    }
}
