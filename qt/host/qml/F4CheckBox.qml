pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.CheckBox {
    id: control

    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property bool semanticFocus: false
    property string mnemonicHotkey: ""
    readonly property bool focusHighlighted:
        control.enabled
        && (control.semanticFocus || control.visualFocus || control.activeFocus)

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    focusPolicy: Qt.NoFocus
    hoverEnabled: true
    font: control.hostWindow ? control.hostWindow.font : Qt.font({})
    spacing: snap(9)
    leftPadding: 0
    implicitHeight: snap(25)

    background: Rectangle {
        id: focusFrame
        objectName: control.objectName ? (control.objectName + "FocusFrame")
                                       : "checkBoxFocusFrame"
        readonly property color testBorderColor: border.color
        readonly property real testBorderWidth: border.width
        x: 0
        y: 0
        width: control.snap(control.width)
        height: control.snap(control.height)
        color: "transparent"
        radius: control.snap(4)
        border.width: control.focusHighlighted
                      ? (control.hostWindow
                         ? control.hostWindow.separatorWidth : 1)
                      : 0
        border.color: control.focusHighlighted
                      ? (control.hostWindow
                         ? control.hostWindow.dialogAccent : "#2c7be5")
                      : (control.hostWindow
                         ? control.hostWindow.controlBorder : "#25303d")
        transform: Translate {
            x: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetX(
                     focusFrame, control.hostWindow.contentItem) : 0
            y: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetY(
                     focusFrame, control.hostWindow.contentItem) : 0
        }

        Behavior on border.color { ColorAnimation { duration: 90 } }
    }

    indicator: Rectangle {
        objectName: control.objectName ? (control.objectName + "Indicator")
                                       : "checkBoxIndicator"
        readonly property color testBorderColor: border.color
        x: 0
        anchors.verticalCenter: parent.verticalCenter
        width: control.snap(18)
        height: control.snap(18)
        radius: control.snap(3)
        transform: Translate {
            x: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetX(
                     control.indicator, control.hostWindow.contentItem) : 0
            y: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetY(
                     control.indicator, control.hostWindow.contentItem) : 0
        }
        color: {
            if (control.checked)
                return control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
            if (control.down)
                return control.hostWindow ? control.hostWindow.controlPressedBg : "#334455"
            if (control.hovered)
                return control.hostWindow ? control.hostWindow.controlHoverBg : "#223344"
            return control.hostWindow ? control.hostWindow.controlBg : "#18202a"
        }
        border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
        border.color: {
            if (control.checked || control.semanticFocus)
                return control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
            return control.hostWindow ? control.hostWindow.controlBorder : "#25303d"
        }

        HostPixelAlignedImage {
            id: checkMark
            objectName: control.objectName ? (control.objectName + "CheckMark")
                                           : "checkBoxCheckMark"
            readonly property real opticalVerticalOffset:
                control.hostWindow
                ? 1 / Math.max(1, control.hostWindow.iconDevicePixelRatio) : 1
            hostWindow: control.hostWindow
            anchors.centerIn: parent
            anchors.verticalCenterOffset: opticalVerticalOffset
            width: control.snap(12)
            height: control.snap(12)
            sourceSize: Qt.size(12, 12)
            source: control.hostWindow
                    ? control.hostWindow.lucideIconSource(
                          "check", 12, control.hostWindow.dialogBg) : ""
            visible: control.checkState === Qt.Checked
            smooth: false
            mipmap: false
        }

        Rectangle {
            id: partialMark
            objectName: control.objectName ? (control.objectName + "PartialMark")
                                           : "checkBoxPartialMark"
            anchors.centerIn: parent
            width: control.snap(9)
            height: control.snap(2)
            radius: control.snap(1)
            visible: control.tristate
                     && control.checkState === Qt.PartiallyChecked
            color: control.hostWindow ? control.hostWindow.dialogBg : "#0e1318"
            transform: Translate {
                x: control.hostWindow
                   ? control.hostWindow.dialogPixelOffsetX(
                         partialMark, control.hostWindow.contentItem) : 0
                y: control.hostWindow
                   ? control.hostWindow.dialogPixelOffsetY(
                         partialMark, control.hostWindow.contentItem) : 0
            }
        }

        Behavior on color { ColorAnimation { duration: 90 } }
        Behavior on border.color { ColorAnimation { duration: 90 } }
    }

    contentItem: Text {
        id: checkBoxText
        objectName: control.objectName ? (control.objectName + "Text")
                                       : "checkBoxText"
        leftPadding: control.indicator.width + control.spacing
        text: control.hostWindow
              ? control.hostWindow.mnemonicText(control.text, control.mnemonicHotkey)
              : control.text
        textFormat: Text.StyledText
        color: {
            if (!control.enabled)
                return control.hostWindow ? control.hostWindow.mutedText : "#666666"
            return control.hostWindow ? control.hostWindow.textColor : "#ffffff"
        }
        opacity: control.enabled ? 1.0 : 0.55
        font: control.font
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
        transform: Translate {
            x: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetX(
                     checkBoxText, control.hostWindow.contentItem) : 0
            y: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetY(
                     checkBoxText, control.hostWindow.contentItem) : 0
        }
    }
}
