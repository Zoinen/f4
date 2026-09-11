pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

// The same natural choice sizes are used by the dialog's measuring pass and
// its visible controls, independent of terminal columns and wrapped lines.
Item {
    id: root
    required property ApplicationWindow hostWindow
    required property var widget
    property bool measuring: false
    readonly property real gap: hostWindow.snapPx(12)
    FontMetrics { id: fontMetrics; font: root.hostWindow.font }
    readonly property var optionWidths: (widget.items || []).map(text => hostWindow.snapPx(fontMetrics.advanceWidth(hostWindow.cleanText(text)) + 29))
    readonly property real rowWidth: optionWidths.reduce((sum, value) => sum + value, 0) + Math.max(0, optionWidths.length - 1) * gap
    // Use the actual Text layout's natural extent, and never round below it:
    // even a fraction of a physical pixel can make the last word wrap.
    readonly property real captionWidth: Math.ceil(caption.implicitWidth * hostWindow.dpr) / hostWindow.dpr
    readonly property int layoutMode: !widget.title ? 2 : captionWidth + gap + rowWidth <= width ? 0 : rowWidth <= width ? 1 : 2
    implicitHeight: Math.max(caption.height, choices.y + choices.height)
    Text {
        id: caption
        objectName: root.measuring ? "" : "dialogWidget-" + root.hostWindow.cleanText(root.widget.id) + "ChoiceCaption"
        visible: !!root.widget.title
        text: root.widget.title || ""
        textFormat: Text.PlainText
        font: root.hostWindow.font
        color: root.widget.disabled ? root.hostWindow.mutedText : root.hostWindow.textColor
        width: root.layoutMode === 0 ? root.captionWidth : root.width
        height: root.widget.title ? root.hostWindow.snapPx(implicitHeight) : 0
        y: root.layoutMode === 0 ? root.hostWindow.snapPx((choices.height - height) / 2) : 0
        wrapMode: Text.Wrap
        transform: Translate {
            x: root.hostWindow.dialogPixelOffsetX(caption, root.hostWindow.contentItem)
            y: root.hostWindow.dialogPixelOffsetY(caption, root.hostWindow.contentItem)
        }
    }

    Flow {
        id: choices
        x: root.layoutMode === 0 ? root.captionWidth + root.gap : 0
        y: root.layoutMode === 0 || !root.widget.title ? 0 : caption.height + root.gap
        width: Math.max(1, parent.width - x)
        spacing: root.layoutMode === 2 ? 0 : root.gap
        Repeater {
            model: root.widget.items || []
            delegate: Item {
                id: option
                required property var modelData
                required property int index
                width: root.layoutMode === 2 ? choices.width : root.optionWidths[index]
                height: button.height
                DialogRadioButton {
                    id: button
                    objectName: root.measuring ? "" : "dialogWidget-"
                                + root.hostWindow.cleanText(root.widget.id) + "Radio-" + option.index
                    hostWindow: root.hostWindow
                    width: parent.width
                    height: implicitHeight
                    implicitHeight: root.hostWindow.snapPx(Math.max(25, contentItem.implicitHeight))
                    topPadding: 0
                    bottomPadding: 0
                    rightPadding: 0
                    autoExclusive: false
                    enabled: root.widget.disabled !== true
                             && !(root.widget.disabledItems || []).includes(option.index)
                    text: root.hostWindow.cleanText(option.modelData)
                    wrapText: root.widget.wrapText === true
                    plainText: root.widget.wrapText === true
                    checked: root.widget.kind === "radioGroup" ? option.index === root.widget.selected
                             : !!(root.widget.states && root.widget.states[option.index])
                    semanticFocus: root.widget.focused === true
                                   && option.index === (root.widget.focusIndex !== undefined
                                       ? root.widget.focusIndex : (root.widget.selected || 0))
                    onClicked: root.hostWindow.action({target: root.widget.id,
                        action: "control.select", index: option.index})
                }
                // Outside the enabled control: unavailable choices still explain.
                HoverHandler {
                    enabled: !root.measuring && !!root.widget.explainTarget
                    onHoveredChanged: if (hovered) root.hostWindow.action({
                        target: root.widget.explainTarget, action: "control.explain", index: option.index
                    }, true)
                }
            }
        }
    }
}
