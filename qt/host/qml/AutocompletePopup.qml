pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: autocompleteOverlay
    required property ApplicationWindow hostWindow
    required property Item menuBar
    property var frame: ({})
    property var items: frame.items || []
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
    readonly property real preferredX: Math.max(0, inputTextX - 12)
    readonly property real rowHeight: Math.max(22, hostWindow.ch * 1.15)
    readonly property real maxHeight: Math.max(rowHeight + 8,
                                               commandLineY - menuBar.height)
    readonly property real availableWidth: Math.max(1,
                                                     hostWindow.width - preferredX - 6)
    readonly property real contentWidth: {
        var widest = 0
        for (var i = 0; i < items.length; ++i)
            widest = Math.max(widest, autocompleteFontMetrics.advanceWidth(
                                  hostWindow.cleanText(items[i].text)))
        return widest + 24
    }

    FontMetrics {
        id: autocompleteFontMetrics
        font.family: hostWindow.guiMonospaceFontFamily
        font.pixelSize: 13
    }

    FontMetrics {
        id: commandLineFontMetrics
        font.family: hostWindow.guiMonospaceFontFamily
        font.pixelSize: autocompleteOverlay.commandLine.runs
                        && autocompleteOverlay.commandLine.runs.length > 0
                        ? 13 : 18
    }

    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.AllButtons
        hoverEnabled: true
        preventStealing: true
        onPressed: (mouse) => { mouse.accepted = true }
        onReleased: (mouse) => { mouse.accepted = true }
        onPositionChanged: (mouse) => { mouse.accepted = true }
        onWheel: (wheel) => { wheel.accepted = false }
    }

    Rectangle {
        x: autocompleteOverlay.preferredX
        y: Math.max(menuBar.height,
                    autocompleteOverlay.commandLineY - height)
        width: Math.min(autocompleteOverlay.availableWidth,
                        Math.max(80, autocompleteOverlay.contentWidth))
        height: Math.min(autocompleteOverlay.maxHeight, Math.max(1, Math.min(12, autocompleteList.count)) * autocompleteOverlay.rowHeight + 8)
        color: "#202833"
        radius: 4
        border.width: 1
        border.color: hostWindow.dialogAccent
        clip: true
        z: 170

        ListView {
            id: autocompleteList
            anchors.fill: parent
            anchors.margins: 4
            model: autocompleteOverlay.items
            clip: true
            currentIndex: hostWindow.autocompleteMenuId === hostWindow.cleanText(autocompleteOverlay.frame.id)
                          ? hostWindow.autocompleteSelectedIndex : -1
            boundsBehavior: Flickable.StopAtBounds
            interactive: contentHeight > height
            onCurrentIndexChanged: {
                if (currentIndex >= 0)
                    positionViewAtIndex(currentIndex, ListView.Contain)
            }

            delegate: Rectangle {
                required property int index
                required property var modelData
                width: ListView.view.width
                height: autocompleteOverlay.rowHeight
                radius: 3
                color: index === autocompleteList.currentIndex
                       ? hostWindow.selectedBg : "transparent"

                Row {
                    id: completionTextRow
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    anchors.leftMargin: 8
                    anchors.rightMargin: 8
                    readonly property string fullText: hostWindow.cleanText(modelData.text)
                    readonly property string query: hostWindow.autocompleteQuery
                    readonly property int matchingLength:
                        fullText.toLocaleLowerCase().indexOf(query.toLocaleLowerCase()) === 0
                        ? Math.min(query.length, fullText.length) : 0

                    Text {
                        id: completionPrefixLabel
                        text: completionTextRow.fullText.substring(
                                  0, completionTextRow.matchingLength)
                        color: hostWindow.dialogAccent
                        font.family: hostWindow.guiMonospaceFontFamily
                        font.pixelSize: 13
                    }

                    Text {
                        width: Math.max(0, completionTextRow.width
                                        - completionPrefixLabel.implicitWidth)
                        text: completionTextRow.fullText.substring(
                                  completionTextRow.matchingLength)
                        color: hostWindow.textColor
                        font.family: hostWindow.guiMonospaceFontFamily
                        font.pixelSize: 13
                        elide: Text.ElideRight
                    }
                }

                MouseArea {
                    id: mouseArea
                    anchors.fill: parent
                    hoverEnabled: true
                    acceptedButtons: Qt.LeftButton
                    onPositionChanged: (mouse) => {
                        if (containsMouse)
                            hostWindow.autocompleteSelectedIndex = index
                    }
                    onPressed: (mouse) => {
                        hostWindow.autocompleteSelectedIndex = index
                        mouse.accepted = true
                    }
                    onClicked: hostWindow.submitAutocomplete()
                }
            }
        }
    }
}
