pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Rectangle {
    id: card
    required property ApplicationWindow hostWindow
    required property var widget
    property bool stacked: false
    objectName: "settingsHelpCard"
    readonly property var paragraph: (widget.children || []).find(child => child.kind === "text") || ({})
    readonly property string fullText: hostWindow.cleanText(paragraph.text)
    readonly property string title: hostWindow.cleanText(widget.title)
    readonly property string timing: hostWindow.cleanText(widget.helpTiming)
    readonly property string description: {
        let parts = fullText.split("\n\n")
        if (title !== "" && parts[0] === title) parts.shift()
        return parts.filter(part => timing === "" || part !== timing).join("\n\n")
    }
    radius: hostWindow.snapPx(10)
    color: Qt.tint(hostWindow.dialogBg, Qt.rgba(1, 1, 1, 0.035))
    border.width: hostWindow.snapPx(1)
    border.color: hostWindow.separatorColor
    clip: true

    HostPixelAlignedImage {
        id: informationIcon
        hostWindow: card.hostWindow
        objectName: "settingsHelpIcon"
        x: card.hostWindow.snapPx(card.stacked ? 16 : 18)
        y: card.hostWindow.snapPx(card.stacked ? 16 : 20)
        width: card.hostWindow.snapPx(32)
        height: width
        source: card.hostWindow.lucideIconSource("info", 32, card.hostWindow.dialogAccent)
    }
    Flickable {
        id: viewport
        objectName: "settingsHelpViewport"
        anchors.fill: parent
        anchors.leftMargin: card.hostWindow.snapPx(card.stacked ? 16 : 66)
        anchors.rightMargin: card.hostWindow.snapPx(16)
        anchors.topMargin: card.stacked
            ? informationIcon.y + informationIcon.height + card.hostWindow.snapPx(12)
            : card.hostWindow.snapPx(16)
        anchors.bottomMargin: card.hostWindow.snapPx(16)
        contentWidth: width
        contentHeight: paragraphs.height
        boundsBehavior: Flickable.StopAtBounds
        clip: true
        interactive: contentHeight > height
        DialogWheelScroll { viewport: viewport; hostWindow: card.hostWindow }
        Column {
            id: paragraphs
            width: viewport.width - (scrollBar.visible ? scrollBar.width + card.hostWindow.snapPx(6) : 0)
            spacing: card.hostWindow.snapPx(5)
            Text {
                objectName: "settingsHelpTitle"
                width: parent.width
                height: card.hostWindow.snapPx(Math.ceil(implicitHeight * card.hostWindow.dpr) / card.hostWindow.dpr)
                visible: text !== ""
                text: card.title
                textFormat: Text.PlainText
                wrapMode: Text.Wrap
                font.family: card.hostWindow.font.family
                font.pixelSize: card.hostWindow.font.pixelSize
                font.bold: true
                color: card.hostWindow.textColor
            }
            Text {
                objectName: "dialogWidget-" + card.hostWindow.cleanText(card.paragraph.id) + "Text"
                width: parent.width
                height: card.hostWindow.snapPx(Math.ceil(implicitHeight * card.hostWindow.dpr) / card.hostWindow.dpr)
                text: card.description
                textFormat: Text.PlainText
                wrapMode: Text.Wrap
                font: card.hostWindow.font
                color: card.hostWindow.mutedText
            }
            Text {
                objectName: "settingsHelpTiming"
                width: parent.width
                height: card.hostWindow.snapPx(Math.ceil(implicitHeight * card.hostWindow.dpr) / card.hostWindow.dpr)
                visible: text !== ""
                text: card.timing
                textFormat: Text.PlainText
                wrapMode: Text.Wrap
                font: card.hostWindow.font
                color: Qt.darker(card.hostWindow.mutedText, 1.18)
            }
        }
        ScrollBar.vertical: F4ScrollBar {
            id: scrollBar
            hostWindow: card.hostWindow
            thickness: 8
            visible: viewport.contentHeight > viewport.height
            policy: ScrollBar.AsNeeded
        }
    }
    onFullTextChanged: viewport.contentY = 0
}
