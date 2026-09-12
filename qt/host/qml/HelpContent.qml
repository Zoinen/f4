pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import F4QtHost 1.0

Item {
    id: helpContent
    required property ApplicationWindow hostWindow
    property var frame: ({})
    readonly property real lineHeight: hostWindow.snapPx(Math.max(hostWindow.ch, bodyMetrics.height + 4))
    readonly property int viewportRows: Math.max(1, Math.floor(body.height / lineHeight))
    property int reportedRows: 0
    property real wheelRemainder: 0
    readonly property string topic: String(frame.topic || "")
    onTopicChanged: wheelRemainder = 0
    implicitWidth: hostWindow.snapPx(bodyMetrics.advanceWidth("M") * 76)
    implicitHeight: hostWindow.snapPx(Number(frame.h || 25) * lineHeight)

    function send(name, fields) {
        const action = fields || ({})
        action.target = frame.id
        action.action = name
        hostWindow.action(action, true)
    }
    function ceilPx(value) {
        return Math.ceil(value * hostWindow.dpr) / hostWindow.dpr
    }
    function scrollWheel(pixelDelta, angleDelta) {
        wheelRemainder += pixelDelta !== 0 ? -pixelDelta / lineHeight : -angleDelta / 40
        const rows = Math.trunc(wheelRemainder + Math.sign(wheelRemainder) * 1e-9)
        if (rows === 0) return
        wheelRemainder -= rows
        if (Math.abs(wheelRemainder) < 1e-9) wheelRemainder = 0
        send("help.scroll", { "delta": rows })
    }
    function reportViewport() {
        if (frame.layout !== "help" || !frame.id || viewportRows === reportedRows) return
        reportedRows = viewportRows
        send("help.viewport", { "rows": viewportRows })
    }
    function escapeHtml(text) {
        return String(text).replace(/&/g, "&amp;").replace(/</g, "&lt;")
                .replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/ /g, "&nbsp;")
    }
    function lineHtml(line, hoveredLink) {
        return (line.spans || []).map(span => {
            let text = escapeHtml(span.text || "")
            if (span.bold) text = "<b>" + text + "</b>"
            if (span.searchMatch) {
                const matchColor = span.searchSelected ? "#ffff00" : hostWindow.dialogAccent
                // Ordinary matches change only the foreground. A transparent
                // brush here would erase the enclosing link's selected/hover fill.
                const matchBackground = span.searchSelected ? ';background-color:' + hostWindow.selectedBg : ''
                text = '<span style="color:' + matchColor + matchBackground + '">' + text + '</span>'
            }
            if (Number(span.link) >= 0) {
                const hovered = hoveredLink !== "" && Number(hoveredLink) === Number(span.link)
                const color = hovered || span.selected ? hostWindow.textColor : hostWindow.dialogAccent
                const background = hovered ? hostWindow.controlHoverBg
                                           : span.selected ? hostWindow.selectedBg : "transparent"
                text = '<a href="' + Number(span.link) + '" style="color:' + color
                        + ';background-color:' + background + '">' + text + '</a>'
            }
            return text
        }).join("")
    }
    onViewportRowsChanged: Qt.callLater(reportViewport)
    Component.onCompleted: Qt.callLater(reportViewport)

    FontMetrics {
        id: bodyMetrics
        font.family: f4GuiFontFamily
        font.pixelSize: f4GuiFontPixelSize
    }
    Item {
        id: body
        objectName: "semanticHelpBody"
        width: hostWindow.snapPx(Math.max(0, parent.width - (scrollBar.visible ? scrollBar.width + 6 : 0)))
        height: hostWindow.snapPx(Math.max(0, parent.height - (searchHint.visible ? searchHint.height + 10 : 0)))
        clip: true

        MouseArea {
            anchors.fill: parent
            acceptedButtons: Qt.NoButton
            onWheel: wheel => {
                helpContent.scrollWheel(wheel.pixelDelta.y, wheel.angleDelta.y)
                wheel.accepted = true
            }
        }
        Repeater {
            model: helpContent.frame.helpLines || []
            delegate: Text {
                id: helpLine
                required property int index
                required property var modelData
                objectName: "semanticHelpLine-" + modelData.index
                x: hostWindow.snapPx(modelData.centered ? Math.max(0, (body.width - implicitWidth) / 2) : 0)
                y: hostWindow.snapPx(index * helpContent.lineHeight)
                text: helpContent.lineHtml(modelData, hoveredLink)
                width: helpContent.ceilPx(implicitWidth)
                height: helpContent.ceilPx(implicitHeight)
                textFormat: Text.RichText
                color: hostWindow.textColor
                font: bodyMetrics.font
                renderType: Text.NativeRendering
                transform: Translate { x: helpLine.ScenePixelAlignment.offset.x; y: helpLine.ScenePixelAlignment.offset.y }
                onLinkActivated: link => helpContent.send("help.activate", { "index": Number(link), "topic": helpContent.frame.topic })
                HoverHandler { cursorShape: helpLine.hoveredLink !== "" ? Qt.PointingHandCursor : Qt.ArrowCursor }
            }
        }
    }
    Text {
        id: searchHint
        objectName: "semanticHelpSearchHint"
        visible: text !== ""
        text: String(helpContent.frame.searchHint || "")
        textFormat: Text.PlainText
        color: hostWindow.mutedText
        font: bodyMetrics.font
        renderType: Text.NativeRendering
        wrapMode: Text.WordWrap
        width: helpContent.width
        height: helpContent.ceilPx(implicitHeight)
        y: hostWindow.snapPx(helpContent.height - height)
        transform: Translate { x: searchHint.ScenePixelAlignment.offset.x; y: searchHint.ScenePixelAlignment.offset.y }
    }
    F4ScrollBar {
        id: scrollBar
        hostWindow: helpContent.hostWindow
        objectName: "semanticHelpScrollBar"
        x: hostWindow.snapPx(helpContent.width - width)
        height: body.height
        width: implicitWidth
        orientation: Qt.Vertical
        policy: ScrollBar.AlwaysOn
        visible: Number(helpContent.frame.totalRows || 0) > Number(helpContent.frame.pageRows || 0)
        size: Math.min(1, Number(helpContent.frame.pageRows || 1) / Math.max(1, Number(helpContent.frame.totalRows || 1)))
        position: Number(helpContent.frame.scrollTop || 0) / Math.max(1, Number(helpContent.frame.totalRows || 1))
        onPositionChanged: {
            if (pressed) helpContent.send("help.scroll", { "position": Math.round(position * Number(helpContent.frame.totalRows || 0)) })
        }
    }
}
