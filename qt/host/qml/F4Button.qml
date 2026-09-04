pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.Button {
    id: control

    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property string variant: "standard" // "standard", "accent", "tool", "flat"
    property string mnemonicHotkey: ""
    property url iconSource: ""
    property real iconSize: 14
    property bool colorfulIcon: false
    property string toolTipText: ""
    property bool semanticFocus: false

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    focusPolicy: Qt.NoFocus
    hoverEnabled: true

    implicitHeight: snap(variant === "tool" ? 28 : 30)
    implicitWidth: {
        if (variant === "tool" && text === "")
            return implicitHeight
        const contentW = btnRow.implicitWidth + (variant === "tool" ? 12 : 20)
        return snap(Math.max(contentW, variant === "tool" ? 0 : 80))
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

    contentItem: RowLayout {
        id: btnRow
        spacing: control.snap(6)

        HostPixelAlignedImage {
            id: btnIcon
            objectName: control.objectName ? (control.objectName + "Icon") : "buttonIcon"
            hostWindow: control.hostWindow
            width: control.snap(control.iconSize)
            height: control.snap(control.iconSize)
            sourceSize: Qt.size(control.iconSize, control.iconSize)
            source: control.iconSource
            visible: control.iconSource !== ""
            smooth: false
            mipmap: false
        }

        Text {
            id: btnText
            objectName: control.objectName ? (control.objectName + "Text") : "buttonText"
            text: {
                if (control.hostWindow && control.mnemonic !== "")
                    return control.hostWindow.mnemonicText(control.text, control.mnemonic)
                return control.text
            }
            textFormat: control.mnemonic !== "" ? Text.StyledText : Text.PlainText
            color: control.variant === "accent"
                   ? "#ffffff"
                   : (control.enabled
                      ? (control.hostWindow ? control.hostWindow.textColor : "#ffffff")
                      : (control.hostWindow ? control.hostWindow.mutedText : "#888888"))
            font.family: control.hostWindow ? control.hostWindow.guiMonospaceFontFamily : "monospace"
            font.pixelSize: control.variant === "tool" ? 12 : 11
            font.weight: (control.highlighted || control.variant === "accent") ? Font.Bold : Font.Normal
            visible: control.text !== ""
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
            Layout.alignment: Qt.AlignVCenter
        }
    }

    background: Rectangle {
        radius: control.snap(4)
        color: {
            const isAccent = control.highlighted || control.variant === "accent"
            if (!control.enabled)
                return control.hostWindow ? control.hostWindow.dialogBg : "#1e2630"
            if (isAccent) {
                const baseAccent = control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
                if (control.down)
                    return Qt.darker(baseAccent, 1.2)
                if (control.hovered)
                    return Qt.lighter(baseAccent, 1.1)
                return baseAccent
            }
            if (control.variant === "flat" || control.variant === "tool") {
                if (control.down)
                    return control.hostWindow ? control.hostWindow.controlPressedBg : "#334455"
                if (control.hovered)
                    return control.hostWindow ? control.hostWindow.controlHoverBg : "#223344"
                return "transparent"
            }
            // Standard
            if (control.down)
                return control.hostWindow ? control.hostWindow.controlPressedBg : "#334455"
            if (control.hovered)
                return control.hostWindow ? control.hostWindow.controlHoverBg : "#223344"
            if (control.semanticFocus)
                return control.hostWindow ? control.hostWindow.controlPressedBg : "#274b68"
            return control.hostWindow ? control.hostWindow.controlBg : "#18202a"
        }

        border.width: {
            if (control.variant === "flat" && !control.hovered && !control.semanticFocus)
                return 0
            return control.hostWindow ? control.hostWindow.separatorWidth : 1
        }
        border.color: {
            if (control.semanticFocus || control.highlighted || control.variant === "accent")
                return control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
            if (control.hovered)
                return control.hostWindow ? control.hostWindow.controlHoverBg : "#334455"
            return control.hostWindow ? control.hostWindow.controlBorder : "#25303d"
        }

        Behavior on color { ColorAnimation { duration: 90 } }
        Behavior on border.color { ColorAnimation { duration: 90 } }
    }
}
