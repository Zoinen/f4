pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

// The same natural choice sizes are used by the dialog's measuring pass and
// its visible controls, independent of terminal columns and wrapped lines.
Item {
    id: root

    readonly property var dialogColors: dialogColorContext.colors
    DialogColorContext {
        id: dialogColorContext
        control: root
    }
    required property ApplicationWindow hostWindow
    required property var widget
    property bool measuring: false
    readonly property bool buttonStyle: widget.choicePresentation === "buttons"
    readonly property real gap: hostWindow.snapPx(12)
    readonly property real optionGap: buttonStyle ? 0 : gap
    readonly property bool compact: buttonStyle && widget.compactChoices === true
    readonly property real buttonPadding: 20
    FontMetrics { id: fontMetrics; font: root.hostWindow.font }
    readonly property var optionWidths: (widget.items || []).map(text => {
        const textWidth = fontMetrics.advanceWidth(hostWindow.cleanText(text))
        return buttonStyle ? hostWindow.capacityPx(Math.max(compact ? 32 : 42, textWidth + buttonPadding))
                           : hostWindow.snapPx(textWidth + 29)
    })
    readonly property real naturalRowWidth: optionWidths.reduce((sum, value) => sum + value, 0)
        + Math.max(0, optionWidths.length - 1) * optionGap
    readonly property real rowWidth: compact ? Math.min(hostWindow.snapPx(width), naturalRowWidth) : naturalRowWidth
    function optionWidth(index) {
        if (!compact || rowWidth >= naturalRowWidth)
            return optionWidths[index]
        const before = optionWidths.slice(0, index).reduce((sum, value) => sum + value, 0)
        return hostWindow.snapPx(rowWidth * (before + optionWidths[index]) / naturalRowWidth)
             - hostWindow.snapPx(rowWidth * before / naturalRowWidth)
    }
    // Use the actual Text layout's natural extent, and never round below it:
    // even a fraction of a physical pixel can make the last word wrap.
    readonly property real captionWidth: Math.ceil(caption.implicitWidth * hostWindow.dpr) / hostWindow.dpr
    readonly property int layoutMode: compact ? 1 : !widget.title ? 2
        : captionWidth + gap + rowWidth <= width ? 0 : rowWidth <= width ? 1 : 2
    readonly property real buttonHeight: hostWindow.snapPx(Math.max(30, fontMetrics.height + (compact ? 16 : 12)))
    readonly property int buttonRows: layoutMode === 2 ? optionWidths.length : Math.min(1, optionWidths.length)
    readonly property real choicesHeight: buttonStyle ? buttonRows * buttonHeight : choices.height
    implicitHeight: Math.max(caption.height, choices.y + choicesHeight)
    Text {
        id: caption
        objectName: root.measuring ? "" : "dialogWidget-" + root.hostWindow.cleanText(root.widget.id) + "ChoiceCaption"
        visible: !!root.widget.title
        text: root.widget.title || ""
        textFormat: Text.PlainText
        font: root.hostWindow.font
        readonly property bool labelEnabled: enabled && root.widget.disabled !== true
        color: labelEnabled ? root.hostWindow.textColor : root.hostWindow.mutedText
        opacity: labelEnabled ? 1.0 : 0.55
        width: root.layoutMode === 0 ? root.captionWidth : root.width
        height: root.widget.title ? root.hostWindow.snapPx(implicitHeight) : 0
        y: root.layoutMode === 0 ? root.hostWindow.snapPx((root.choicesHeight - height) / 2) : 0
        wrapMode: Text.Wrap
        transform: Translate {
            x: root.hostWindow.dialogPixelOffsetX(caption, root.hostWindow.contentItem)
            y: root.hostWindow.dialogPixelOffsetY(caption, root.hostWindow.contentItem)
        }
    }

    Rectangle {
        objectName: root.measuring ? "" : "dialogWidget-" + root.hostWindow.cleanText(root.widget.id) + "ChoiceOutline"
        visible: root.buttonStyle && root.buttonRows > 0
        x: choices.x
        y: choices.y
        width: root.layoutMode === 2 ? choices.width : root.rowWidth
        height: root.choicesHeight
        radius: root.hostWindow.snapPx(4)
        color: "transparent"
        border.width: root.hostWindow.separatorWidth
        border.color: root.dialogColors.controlBorder
        z: 1
    }

    Flow {
        id: choices
        x: root.layoutMode === 0 ? root.captionWidth + root.gap : 0
        y: root.layoutMode === 0 || !root.widget.title ? 0 : caption.height + (root.compact ? root.hostWindow.snapPx(6) : root.gap)
        width: Math.max(1, parent.width - x) + (root.compact ? 0.001 : 0)
        spacing: root.layoutMode === 2 && !root.buttonStyle ? 0 : root.optionGap
        Repeater {
            model: root.widget.items || []
            delegate: Item {
                id: option
                required property var modelData
                required property int index
                width: root.layoutMode === 2 ? choices.width : root.optionWidth(index)
                height: root.buttonStyle ? root.buttonHeight : button.height
                readonly property bool first: index === 0
                readonly property bool last: index === root.optionWidths.length - 1
                readonly property bool vertical: root.layoutMode === 2
                readonly property string optionText: root.hostWindow.cleanText(modelData)
                readonly property bool optionEnabled: root.widget.disabled !== true
                    && !(root.widget.disabledItems || []).includes(index)
                readonly property bool optionFocused: root.widget.focused === true
                    && index === (root.widget.focusIndex !== undefined
                        ? root.widget.focusIndex : (root.widget.selected || 0))
                function select() {
                    root.hostWindow.action({target: root.widget.id,
                        action: "control.select", index: index})
                }
                Loader {
                    id: buttonLoader
                    active: root.buttonStyle
                    width: option.width
                    height: option.height
                    sourceComponent: DialogButton {
                        objectName: root.measuring ? "" : "dialogWidget-"
                                    + root.hostWindow.cleanText(root.widget.id) + "Option-" + option.index
                        hostWindow: root.hostWindow
                        text: option.width >= root.optionWidths[option.index] ? option.optionText
                            : fontMetrics.elidedText(option.optionText, Qt.ElideRight, Math.max(1, option.width - root.buttonPadding))
                        Accessible.name: option.optionText
                        Accessible.checkable: true
                        Accessible.checked: highlighted
                        highlighted: option.index === root.widget.selected
                        flat: !focusHighlighted
                        topLeftRadius: option.first ? cornerRadius : 0
                        topRightRadius: (option.vertical ? option.first : option.last) ? cornerRadius : 0
                        bottomLeftRadius: (option.vertical ? option.last : option.first) ? cornerRadius : 0
                        bottomRightRadius: option.last ? cornerRadius : 0
                        enabled: option.optionEnabled
                        semanticFocus: option.optionFocused
                        onClicked: option.select()
                    }
                }
                Rectangle {
                    visible: root.buttonStyle && !option.first && !option.optionFocused
                    width: option.vertical ? option.width : root.hostWindow.separatorWidth
                    height: option.vertical ? root.hostWindow.separatorWidth : option.height
                    color: root.dialogColors.controlBorder
                }
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
                    visible: !root.buttonStyle
                    enabled: option.optionEnabled
                    text: option.optionText
                    wrapText: root.widget.wrapText === true
                    plainText: root.widget.wrapText === true
                    checked: root.widget.kind === "radioGroup" ? option.index === root.widget.selected
                             : !!(root.widget.states && root.widget.states[option.index])
                    semanticFocus: option.optionFocused
                    onClicked: option.select()
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
