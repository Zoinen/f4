pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    required property ApplicationWindow hostWindow
    property var runs: []
    property string fallbackText: ""
    property bool transparentBlackBackground: false
    property bool ignoreRunBackground: false
    property real fallbackFontPixelSize: hostWindow.guiMonospaceFontPixelSize
    property real horizontalInset: 8

    function runBackground(value) {
        if (ignoreRunBackground)
            return "transparent"
        var background = hostWindow.cleanText(value).toLowerCase()
        if (transparentBlackBackground
                && (background === "#000000"
                    || background === "#ff000000"
                    || background === "black"))
            return "transparent"
        return background !== "" ? value : "transparent"
    }

    // Use the data model, rather than effective visibility, to select the
    // measured content. Measurement-only rows are deliberately hidden;
    // inherited visibility must not collapse their reported run width.
    readonly property real contentWidth: horizontalInset
                                         + (runs && runs.length > 0
                                            ? runRow.implicitWidth
                                            : fallbackRunLabel.implicitWidth)

    Row {
        id: runRow
        anchors.left: parent.left
        anchors.leftMargin: horizontalInset
        height: parent.height
        visible: !!runs && runs.length > 0

        Repeater {
            model: runs || []

            delegate: Rectangle {
                required property var modelData
                height: parent ? parent.height : hostWindow.ch
                width: consoleRunLabel.implicitWidth
                color: runBackground(modelData.background)

                Text {
                    id: consoleRunLabel
                    anchors.verticalCenter: parent.verticalCenter
                    text: hostWindow.cleanText(modelData.text)
                    color: hostWindow.cleanText(modelData.foreground) !== ""
                           ? modelData.foreground : hostWindow.textColor
                    font.family: hostWindow.guiMonospaceFontFamily
                    font.pixelSize: hostWindow.semanticTextFontPixelSize
                    font.bold: modelData.bold === true
                    font.underline: modelData.underline === true
                    font.strikeout: modelData.strikeout === true
                }
            }
        }
    }

    Text {
        id: fallbackRunLabel
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.leftMargin: horizontalInset
        anchors.rightMargin: horizontalInset
        anchors.verticalCenter: parent.verticalCenter
        visible: !runs || runs.length === 0
        text: fallbackText
        color: hostWindow.textColor
        font.family: hostWindow.guiMonospaceFontFamily
        font.pixelSize: fallbackFontPixelSize
        elide: Text.ElideRight
    }
}
