pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Row {
    id: control
    required property ApplicationWindow hostWindow
    required property QtObject nativeWindowAgent
    required property bool nativeWindowAgentReady
    required property bool usesQwk
    property string kind: "horizontal"
    readonly property bool pressed: slider.pressed
    readonly property string namePrefix: kind === "caption" ? "galleryCellCaptionSpacing"
        : kind === "vertical" ? "galleryCellVerticalPadding" : "galleryCellPadding"
    readonly property string setting: kind === "caption" ? "galleryCellCaptionSpacing"
        : kind === "vertical" ? "galleryCellVerticalPadding" : "galleryCellHorizontalPadding"
    height: hostWindow.snapPx(28)
    spacing: hostWindow.snapPx(8)

    Text {
        objectName: control.namePrefix + "Label"
        text: control.kind === "caption" ? qsTr("Caption gap")
            : control.kind === "vertical" ? qsTr("Vertical") : qsTr("Horizontal")
        width: control.hostWindow.snapPx(implicitWidth)
        height: control.height
        color: control.hostWindow.chromeText
        font.family: control.hostWindow.uiFontFamily
        font.pixelSize: control.hostWindow.semanticTextFontPixelSize
        renderType: control.hostWindow.fontRenderType
        verticalAlignment: Text.AlignVCenter
    }

    F4Slider {
        id: slider
        objectName: control.namePrefix + "Slider"
        hostWindow: control.hostWindow
        width: hostWindow.snapPx(90)
        height: control.height
        from: 0
        to: 32
        stepSize: 1
        value: hostWindow[control.setting]
        Accessible.name: control.kind === "caption" ? qsTr("Icon to caption spacing")
            : control.kind === "vertical" ? qsTr("Cell vertical padding") : qsTr("Cell horizontal padding")
        onMoved: {
            hostWindow[control.setting] = Math.round(value)
            if (!pressed)
                hostWindow.saveThemeToPersistence()
        }
        onPressedChanged: {
            if (!pressed)
                hostWindow.saveThemeToPersistence()
        }
        function registerNativeHitTarget() {
            if (control.usesQwk && control.nativeWindowAgentReady)
                control.nativeWindowAgent.setHitTestVisible(slider, true)
        }
        Component.onCompleted: registerNativeHitTarget()
        Connections {
            target: control
            function onNativeWindowAgentReadyChanged() {
                slider.registerNativeHitTarget()
            }
        }
    }

    Text {
        objectName: control.namePrefix + "Value"
        text: control.hostWindow[control.setting] + " px"
        width: control.hostWindow.snapPx(45)
        height: control.height
        color: control.hostWindow.chromeText
        font.family: control.hostWindow.uiFontFamily
        font.pixelSize: control.hostWindow.semanticTextFontPixelSize
        renderType: control.hostWindow.fontRenderType
        verticalAlignment: Text.AlignVCenter
    }
}
