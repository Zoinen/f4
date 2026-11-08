pragma ComponentBehavior: Bound

import QtQuick
import ZoinGallery.PixelGrid 1.0 as Grid
import QtQuick.Layouts
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.Button {
    id: control

    readonly property var dialogColors: dialogColorContext.colors
    DialogColorContext {
        id: dialogColorContext
        control: control
    }

    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property string variant: "standard" // "standard", "accent", "tool", "flat"
    property string mnemonicHotkey: ""
    property url iconSource: ""
    property real iconSize: 14
    property real cornerRadius: 4
    property real topLeftRadius: cornerRadius
    property real topRightRadius: cornerRadius
    property real bottomLeftRadius: cornerRadius
    property real bottomRightRadius: cornerRadius
    property bool colorfulIcon: false
    property string toolTipText: ""
    property bool semanticFocus: false
    readonly property bool focusHighlighted: semanticFocus || visualFocus || activeFocus
    readonly property bool dropdownSurface: dialogColorContext.inherited !== null
        && variant === "standard" && !highlighted
    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    focusPolicy: Qt.NoFocus
    hoverEnabled: true
    font: control.hostWindow ? control.hostWindow.font : Qt.font({})

    implicitHeight: snap(Math.max(variant === "tool" ? 28 : 30, btnRow.implicitHeight + 12))
    implicitWidth: {
        if (variant === "tool" && text === "")
            return implicitHeight
        const contentW = btnRow.implicitWidth + (variant === "tool" ? 12 : 20)
        return hostWindow ? hostWindow.capacityPx(Math.max(contentW, variant === "tool" ? 0 : 80))
                          : Math.ceil(Math.max(contentW, variant === "tool" ? 0 : 80))
    }
    Layout.preferredHeight: implicitHeight
    Layout.preferredWidth: implicitWidth
    Layout.minimumHeight: implicitHeight
    Layout.maximumHeight: implicitHeight

    Accessible.role: Accessible.Button
    Accessible.name: toolTipText !== "" ? toolTipText : text

    F4ToolTip {
        visible: control.hovered && control.toolTipText !== ""
        text: control.toolTipText
        hostWindow: control.hostWindow
    }

    contentItem: Grid.PixelGridScope {
        id: buttonContentItem
        objectName: control.objectName ? (control.objectName + "ContentItem")
                                       : "buttonContentItem"
        implicitWidth: btnRow.implicitWidth
        implicitHeight: btnRow.implicitHeight

        RowLayout {
            id: btnRow
            objectName: control.objectName ? (control.objectName + "Content")
                                           : "buttonContent"
            width: implicitWidth
            height: implicitHeight
            // Capacity and centering use integer physical pixels. Leaf fitting
            // handles the remaining Qt layout distribution only when it changes.
            x: control.hostWindow ? control.hostWindow.centerPx(parent.width, width)
                                  : Math.floor((parent.width - width) / 2)
            y: control.hostWindow ? control.hostWindow.centerPx(parent.height, height)
                                  : Math.floor((parent.height - height) / 2)
            spacing: btnIcon.visible && btnText.visible ? control.snap(6) : 0

            HostPixelAlignedImage {
                id: btnIcon
                objectName: control.objectName
                            ? (control.objectName + "Icon") : "buttonIcon"
                hostWindow: control.hostWindow
                width: visible ? control.snap(control.iconSize) : 0
                height: visible ? control.snap(control.iconSize) : 0
                Layout.preferredWidth: control.snap(control.iconSize)
                Layout.preferredHeight: control.snap(control.iconSize)
                sourceSize: Qt.size(control.iconSize, control.iconSize)
                source: control.iconSource
                visible: control.iconSource.toString() !== ""
                smooth: false
                mipmap: false
                Layout.alignment: Qt.AlignVCenter
            }

            Text {
                id: btnText
                objectName: control.objectName
                            ? (control.objectName + "Text") : "buttonText"
                text: control.hostWindow
                      ? control.hostWindow.mnemonicTextWithAccent(control.text, control.mnemonicHotkey, control.dialogColors.dialogAccent)
                      : control.text
                textFormat: control.hostWindow ? Text.StyledText : Text.PlainText
                color: control.variant === "accent"
                       ? "#ffffff"
                       : (control.enabled
                          ? (control.hostWindow
                             ? control.hostWindow.textColor : "#ffffff")
                          : (control.hostWindow
                             ? control.hostWindow.mutedText : "#888888"))
                font: control.font
                visible: control.text !== ""
                verticalAlignment: Text.AlignVCenter
                elide: Text.ElideRight
                Layout.alignment: Qt.AlignVCenter
                transform: Translate {
                    x: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetX(
                             btnText, control.hostWindow.contentItem) : 0
                    y: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetY(
                             btnText, control.hostWindow.contentItem) : 0
                }
            }
        }
    }

    background: Rectangle {
        objectName: control.objectName ? (control.objectName + "Background")
                                       : "buttonBackground"
        readonly property color testBorderColor: border.color
        radius: control.snap(control.cornerRadius)
        topLeftRadius: control.snap(control.topLeftRadius)
        topRightRadius: control.snap(control.topRightRadius)
        bottomLeftRadius: control.snap(control.bottomLeftRadius)
        bottomRightRadius: control.snap(control.bottomRightRadius)
        color: {
            const isAccent = control.highlighted || control.variant === "accent"
            if (!control.enabled)
                return control.hostWindow ? control.dialogColors.dialogBg : "#1e2630"
            if (isAccent) {
                const baseAccent = control.hostWindow ? control.dialogColors.dialogAccent : "#2c7be5"
                if (control.down)
                    return Qt.darker(baseAccent, 1.2)
                if (control.hovered)
                    return Qt.lighter(baseAccent, 1.1)
                return baseAccent
            }
            if (control.variant === "flat" || control.variant === "tool") {
                if (control.down)
                    return control.hostWindow ? control.dialogColors.controlPressedBg : "#334455"
                if (control.hovered)
                    return control.hostWindow ? control.dialogColors.controlHoverBg : "#223344"
                return "transparent"
            }
            // Standard
            if (control.down)
                return control.hostWindow ? control.dialogColors.controlPressedBg : "#334455"
            if (control.hovered)
                return control.hostWindow ? control.dialogColors.controlHoverBg : "#223344"
            return control.hostWindow ? control.dialogColors.controlBg : "#18202a"
        }

        border.width: {
            if (control.flat)
                return 0
            if (control.variant === "flat" && !control.hovered && !control.focusHighlighted)
                return 0
            return control.hostWindow ? control.hostWindow.separatorWidth : 1
        }
        border.color: {
            if (control.dropdownSurface)
                return control.enabled && control.focusHighlighted
                    ? (control.hostWindow ? control.dialogColors.dialogAccent : "#2c7be5")
                    : (control.hostWindow ? control.dialogColors.controlBorder : "#25303d")
            if (control.focusHighlighted || control.highlighted || control.variant === "accent")
                return control.hostWindow ? control.dialogColors.dialogAccent : "#2c7be5"
            if (control.hovered)
                return control.hostWindow ? control.dialogColors.controlHoverBg : "#334455"
            return control.hostWindow ? control.dialogColors.controlBorder : "#25303d"
        }

        Behavior on color {
            enabled: !control.dropdownSurface
            ColorAnimation { duration: 90 }
        }
        Behavior on border.color {
            enabled: !control.dropdownSurface
            ColorAnimation { duration: 90 }
        }
    }
}
