pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.RadioButton {
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
                                       : "radioButtonFocusFrame"
        readonly property color testBorderColor: border.color
        readonly property real testBorderWidth: border.width
        // Derive the ring from the indicator, not the row: fractional-DPR
        // centering can otherwise split its vertical space into unequal pixels.
        readonly property real indicatorGap: control.snap(2)
        x: control.indicator.x - indicatorGap
        y: control.indicator.y - indicatorGap
        width: control.snap(control.width) + indicatorGap
        height: control.indicator.height + 2 * indicatorGap
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
                                       : "radioButtonIndicator"
        readonly property color testBorderColor: border.color
        x: 0
        anchors.verticalCenter: parent.verticalCenter
        width: control.snap(18)
        height: control.snap(18)
        radius: width / 2
        transform: Translate {
            x: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetX(
                     control.indicator, control.hostWindow.contentItem) : 0
            y: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetY(
                     control.indicator, control.hostWindow.contentItem) : 0
        }
        color: {
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

        Rectangle {
            id: selectionMark
            objectName: control.objectName ? (control.objectName + "SelectionMark")
                                           : "radioButtonSelectionMark"
            anchors.centerIn: parent
            width: control.snap(8)
            height: control.snap(8)
            radius: width / 2
            visible: control.checked
            color: control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
            transform: Translate {
                x: control.hostWindow
                   ? control.hostWindow.dialogPixelOffsetX(
                         selectionMark, control.hostWindow.contentItem) : 0
                y: control.hostWindow
                   ? control.hostWindow.dialogPixelOffsetY(
                         selectionMark, control.hostWindow.contentItem) : 0
            }
        }

        Behavior on color { ColorAnimation { duration: 90 } }
        Behavior on border.color { ColorAnimation { duration: 90 } }
    }

    contentItem: Text {
        id: radioButtonText
        objectName: control.objectName ? (control.objectName + "Text")
                                       : "radioButtonText"
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
                     radioButtonText, control.hostWindow.contentItem) : 0
            y: control.hostWindow
               ? control.hostWindow.dialogPixelOffsetY(
                     radioButtonText, control.hostWindow.contentItem) : 0
        }
    }
}
