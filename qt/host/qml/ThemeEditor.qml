pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl
import QtQuick.Layouts
import QtQuick.Shapes
import ZoinGallery 1.0 as ZG

Window {
    id: themeColorConfigurator
    required property ApplicationWindow hostWindow
    required property QtObject themePersistence
    objectName: "themeColorConfigurator"
    title: "Theme Color Configurator"
    width: hostWindow.snapPx(720)
    height: hostWindow.snapPx(772)
    minimumWidth: hostWindow.snapPx(580)
    minimumHeight: hostWindow.snapPx(612)
    visible: false
    color: hostWindow.dialogBg
    flags: Qt.Window | Qt.WindowTitleHint | Qt.WindowSystemMenuHint | Qt.WindowMinMaxButtonsHint | Qt.WindowCloseButtonHint


    property alias selectedIndex: editorContent.selectedIndex
    readonly property var currentItem: editorContent.currentItem
    readonly property real maxOklchChroma: editorContent.maxOklchChroma
    readonly property real wheelOklchChroma: editorContent.wheelOklchChroma
    readonly property color activeFlashColor: editorContent.activeFlashColor
    property alias selectedHue: editorContent.selectedHue
    property alias selectedChroma: editorContent.selectedChroma
    property alias selectedLightness: editorContent.selectedLightness
    property alias selectedAlpha: editorContent.selectedAlpha
    property alias filterQuery: editorContent.filterQuery
    property alias statusToast: editorContent.statusToast

    function parseHex(text) { return editorContent.parseHex(text) }
    function oklchColorValue(lightness, chroma, hue, alpha) {
        return editorContent.oklchColorValue(lightness, chroma, hue, alpha)
    }
    function oklchDisplayRgb(lightness, chroma, hue) {
        return editorContent.oklchDisplayRgb(lightness, chroma, hue)
    }
    function setFromColor(colorValue) { editorContent.setFromColor(colorValue) }
    function setFromRgb(red, green, blue, alpha) {
        editorContent.setFromRgb(red, green, blue, alpha)
    }
    function applyCurrentColor() { editorContent.applyCurrentColor() }
    function selectItem(index, shouldFlash) {
        editorContent.selectItem(index, shouldFlash)
    }
    function flash(propertyId) { editorContent.flash(propertyId) }
    function endHoverFlash(propertyId) {
        editorContent.endHoverFlash(propertyId)
    }
    function startPressFlash(propertyId) {
        editorContent.startPressFlash(propertyId)
    }
    function endPressFlash(propertyId) {
        editorContent.endPressFlash(propertyId)
    }
    function stopAllFlashing() { editorContent.stopAllFlashing() }


    onClosing: editorContent.stopAllFlashing()
    ThemeEditorContent {
        id: editorContent
        anchors.fill: parent
        hostWindow: themeColorConfigurator.hostWindow
        themePersistence: themeColorConfigurator.themePersistence
        onCloseRequested: themeColorConfigurator.close()
    }
}
