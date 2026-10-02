pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl
import QtQuick.Layouts
import QtQuick.Shapes
import ZoinGallery 1.0 as ZG

FocusScope {
    id: themeColorConfigurator
    required property ApplicationWindow hostWindow
    required property QtObject themePersistence
    objectName: "themeConfiguratorContent"
    readonly property Item contentItem: themeColorConfigurator
    property bool embeddedSettings: false
    property var draftBaseline: ({})
    readonly property string status: statusToast
    function captureDraftBaseline() {
        let values = {}
        for (const definition of hostWindow.themeColorDefinitions)
            values[definition.id] = hostWindow[definition.id].toString()
        draftBaseline = values
    }
    function applyDraft() {
        stopAllFlashing()
        if (!hostWindow.saveThemeToPersistence()) {
            statusToast = "Failed to save theme"
            return false
        }
        captureDraftBaseline()
        statusToast = "Theme saved"
        return true
    }
    function resetDraft() {
        stopAllFlashing()
        for (const key of Object.keys(draftBaseline)) {
            if (key === "fontRenderType") hostWindow.setFontRenderType(draftBaseline[key])
            else if (key === "iconSetName") hostWindow.setIconSet(draftBaseline[key])
            else hostWindow[key] = draftBaseline[key]
        }
        if (currentItem) setFromColor(hostWindow[currentItem.id])
        statusToast = ""
    }
    Component.onCompleted: captureDraftBaseline()
    signal closeRequested()
    function close() { stopAllFlashing(); closeRequested() }
    Keys.onPressed: event => {
        if (event.key === Qt.Key_Escape) close()
        event.accepted = true
    }
    implicitWidth: hostWindow.snapPx(embeddedSettings ? 535 : 720)
    implicitHeight: hostWindow.snapPx(embeddedSettings ? 480 : 544)


    ThemeDraftModel {
        id: themeDraft
        hostWindow: themeColorConfigurator.hostWindow
        editorVisible: themeColorConfigurator.visible
    }

    property alias selectedIndex: themeDraft.selectedIndex
    readonly property var currentItem: themeDraft.currentItem
    readonly property real maxOklchChroma: themeDraft.maxOklchChroma
    readonly property real wheelOklchChroma: themeDraft.wheelOklchChroma
    readonly property color activeFlashColor: themeDraft.activeFlashColor
    property alias selectedHue: themeDraft.selectedHue
    property alias selectedChroma: themeDraft.selectedChroma
    property alias selectedLightness: themeDraft.selectedLightness
    property alias selectedAlpha: themeDraft.selectedAlpha
    property alias filterQuery: themeDraft.filterQuery
    property alias statusToast: themeDraft.statusToast

    function parseHex(text) { return themeDraft.parseHex(text) }
    function oklchColorValue(lightness, chroma, hue, alpha) {
        return themeDraft.oklchColorValue(lightness, chroma, hue, alpha)
    }
    function oklchDisplayRgb(lightness, chroma, hue) {
        return themeDraft.oklchDisplayRgb(lightness, chroma, hue)
    }
    function setFromColor(colorValue) { themeDraft.setFromColor(colorValue) }
    function setFromRgb(red, green, blue, alpha) {
        themeDraft.setFromRgb(red, green, blue, alpha)
    }
    function applyCurrentColor() { themeDraft.applyCurrentColor() }
    function selectItem(index, shouldFlash) {
        themeDraft.selectItem(index, shouldFlash)
    }
    function flash(propertyId) { themeDraft.flash(propertyId) }
    function endHoverFlash(propertyId) {
        themeDraft.endHoverFlash(propertyId)
    }
    function startPressFlash(propertyId) {
        themeDraft.startPressFlash(propertyId)
    }
    function endPressFlash(propertyId) {
        themeDraft.endPressFlash(propertyId)
    }
    function stopAllFlashing() { themeDraft.stopAllFlashing() }


    Component.onDestruction: {
        if (embeddedSettings && hostWindow && typeof hostWindow.setFontRenderType === "function") resetDraft()
        else stopAllFlashing()
    }

    onVisibleChanged: {
        if (visible) {
            selectItem(selectedIndex, false)
            statusToast = ""
        } else {
            themeColorConfigurator.stopAllFlashing()
        }
    }

    Rectangle {
        anchors.fill: parent
        color: hostWindow.dialogBg

        ColumnLayout {
            anchors.fill: parent
            anchors.margins: embeddedSettings ? 0 : hostWindow.snapPx(14)
            spacing: hostWindow.snapPx(10)

            // Header
            RowLayout {
                id: themeDialogHeader
                visible: !themeColorConfigurator.embeddedSettings
                objectName: "themeDialogHeader"
                Layout.fillWidth: true
                Layout.preferredHeight: hostWindow.snapPx(18)
                Layout.minimumHeight: hostWindow.snapPx(18)
                Layout.maximumHeight: hostWindow.snapPx(18)
                spacing: hostWindow.snapPx(8)
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                        themeDialogHeader,
                        themeColorConfigurator.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                        themeDialogHeader,
                        themeColorConfigurator.contentItem)
                }

                HostPixelAlignedImage {
                    objectName: "themeConfiguratorHeaderIcon"
                    hostWindow: themeColorConfigurator.hostWindow
                    Layout.preferredWidth: hostWindow.snapPx(18)
                    Layout.minimumWidth: Layout.preferredWidth
                    Layout.maximumWidth: Layout.preferredWidth
                    Layout.preferredHeight: hostWindow.snapPx(18)
                    Layout.minimumHeight: Layout.preferredHeight
                    Layout.maximumHeight: Layout.preferredHeight
                    width: hostWindow.snapPx(18)
                    height: width
                    source: hostWindow.lucideIconSource("palette", 18, hostWindow.dialogAccent)
                    sourceSize: Qt.size(width * hostWindow.dpr, height * hostWindow.dpr)
                }

                Text {
                    id: themeEditorContentLeaf0
                    objectName: "themeEditorContentLeaf0"
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(themeEditorContentLeaf0, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(themeEditorContentLeaf0, hostWindow.contentItem)
                    }
                    text: "Theme Color Configurator"
                    color: hostWindow.textColor
                    font.family: hostWindow.uiFontFamily
                    font.pixelSize: (hostWindow ? hostWindow.uiTextSize(14) : 14)
                    font.weight: Font.Bold
                    Layout.fillWidth: true
                }

                Text {
                    id: themeEditorContentLeaf1
                    objectName: "themeEditorContentLeaf1"
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(themeEditorContentLeaf1, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(themeEditorContentLeaf1, hostWindow.contentItem)
                    }
                    text: themeColorConfigurator.themePersistence
                          ? themeColorConfigurator.themePersistence.themeFilePath
                          : "gui_theme.ini"
                    color: hostWindow.mutedText
                    font.family: hostWindow.uiFontFamily
                    font.pixelSize: (hostWindow ? hostWindow.uiTextSize(10) : 10)
                    elide: Text.ElideMiddle
                    Layout.maximumWidth: 320
                }
            }

            Rectangle {
                id: themeHeaderDivider
                visible: !themeColorConfigurator.embeddedSettings
                objectName: "themeHeaderDivider"
                Layout.fillWidth: true
                Layout.preferredHeight: hostWindow.separatorWidth
                Layout.minimumHeight: hostWindow.separatorWidth
                Layout.maximumHeight: hostWindow.separatorWidth
                implicitHeight: hostWindow.separatorWidth
                color: hostWindow.separatorColor
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                        themeHeaderDivider,
                        themeColorConfigurator.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                        themeHeaderDivider,
                        themeColorConfigurator.contentItem)
                }
            }


            RowLayout {
                Layout.fillWidth: true
                Layout.fillHeight: true
                spacing: hostWindow.snapPx(12)

                // Left Column: Items List (Takes remaining width)
                ColumnLayout {
                    Layout.fillWidth: true
                    Layout.minimumWidth: hostWindow.snapPx(embeddedSettings ? 190 : 200)
                    Layout.fillHeight: true
                    spacing: hostWindow.snapPx(6)

                    // Filter box with search icon
                    Item {
                        Layout.fillWidth: true
                        Layout.preferredHeight: hostWindow.snapPx(28)
                        Layout.minimumHeight: hostWindow.snapPx(28)
                        Layout.maximumHeight: hostWindow.snapPx(28)
                        implicitHeight: hostWindow.snapPx(28)

                        F4TextField {
                            id: themeColorFilter
                            objectName: "themeColorFilter"
                            hostWindow: themeColorConfigurator.hostWindow
                            width: hostWindow.snapPx(parent.width)
                            height: parent.height
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                    themeColorFilter,
                                    themeColorConfigurator.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                    themeColorFilter,
                                    themeColorConfigurator.contentItem)
                            }
                            leadingIconSource: hostWindow.lucideIconSource(
                                                   "search", 14,
                                                   hostWindow.mutedText)
                            placeholderText: "Filter elements..."
                            onTextEdited: themeColorConfigurator.filterQuery = text.toLowerCase().trim()
                        }
                    }

                    // List View with ScrollBar
                    Item {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        Layout.minimumHeight: hostWindow.snapPx(36)

                        ListView {
                            id: themeItemsList
                            objectName: "themeItemsList"
                            width: hostWindow.snapPx(parent.width)
                            height: hostWindow.snapPx(parent.height)
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                    themeItemsList,
                                    themeColorConfigurator.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                    themeItemsList,
                                    themeColorConfigurator.contentItem)
                            }
                            clip: true
                            boundsBehavior: Flickable.StopAtBounds
                            model: hostWindow.themeColorDefinitions.length

                            ScrollBar.vertical: F4ScrollBar {
                                id: themeListScrollBar
                                objectName: "themeListScrollBar"
                                hostWindow: themeColorConfigurator.hostWindow
                                policy: ScrollBar.AsNeeded
                            }

                        delegate: Item {
                            id: itemDelegate
                            required property int index
                            readonly property var def: hostWindow.themeColorDefinitions[index]
                            readonly property bool isSelected: themeColorConfigurator.selectedIndex === index
                            readonly property bool matchesFilter: {
                                if (!themeColorConfigurator.filterQuery)
                                    return true
                                return def.name.toLowerCase().includes(themeColorConfigurator.filterQuery)
                                    || def.group.toLowerCase().includes(themeColorConfigurator.filterQuery)
                            }

                            width: hostWindow.snapPx(themeItemsList.width - (themeListScrollBar.visible ? (themeListScrollBar.width + 6) : 0))
                            height: matchesFilter ? hostWindow.snapPx(36) : 0
                            visible: matchesFilter

                            Rectangle {
                                anchors.fill: parent
                                radius: hostWindow.snapPx(4)
                                color: itemDelegate.isSelected ? hostWindow.panelSelectionBg
                                       : itemMouse.containsMouse ? hostWindow.controlHoverBg : "transparent"
                                border.width: itemDelegate.isSelected ? hostWindow.separatorWidth : 0
                                border.color: hostWindow.panelSelectionBorder

                                RowLayout {
                                    anchors.fill: parent
                                    anchors.leftMargin: hostWindow.snapPx(6)
                                    anchors.rightMargin: hostWindow.snapPx(6)
                                    spacing: hostWindow.snapPx(8)

                                    // Color swatch
                                    Rectangle {
                                        implicitWidth: hostWindow.snapPx(20)
                                        implicitHeight: hostWindow.snapPx(20)
                                        radius: hostWindow.snapPx(3)
                                        color: hostWindow.controlBg
                                        border.width: hostWindow.separatorWidth
                                        border.color: hostWindow.controlBorder
                                        clip: true

                                        Canvas {
                                            anchors.fill: parent
                                            onPaint: {
                                                const ctx = getContext("2d")
                                                const sz = 3
                                                for (let x = 0; x < width; x += sz) {
                                                    for (let y = 0; y < height; y += sz) {
                                                        ctx.fillStyle = ((Math.floor(x / sz) + Math.floor(y / sz)) % 2 === 0) ? "#404b5a" : "#222c38"
                                                        ctx.fillRect(x, y, sz, sz)
                                                    }
                                                }
                                            }
                                        }

                                        Rectangle {
                                            anchors.fill: parent
                                            color: hostWindow[itemDelegate.def.id]
                                        }
                                    }

                                    // Name & group
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: hostWindow.snapPx(1)

                                        Text {
                                            id: themeEditorContentLeaf6
                                            objectName: "themeEditorContentLeaf6"
                                            transform: Translate {
                                                x: hostWindow.dialogPixelOffsetX(themeEditorContentLeaf6, hostWindow.contentItem)
                                                y: hostWindow.dialogPixelOffsetY(themeEditorContentLeaf6, hostWindow.contentItem)
                                            }
                                            text: itemDelegate.def.name
                                            color: hostWindow.textColor
                                            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(11) : 11)
                                            font.weight: itemDelegate.isSelected ? Font.Bold : Font.Normal
                                            elide: Text.ElideRight
                                            Layout.fillWidth: true
                                        }

                                        Text {
                                            id: themeEditorContentLeaf7
                                            objectName: "themeEditorContentLeaf7"
                                            transform: Translate {
                                                x: hostWindow.dialogPixelOffsetX(themeEditorContentLeaf7, hostWindow.contentItem)
                                                y: hostWindow.dialogPixelOffsetY(themeEditorContentLeaf7, hostWindow.contentItem)
                                            }
                                            text: itemDelegate.def.group
                                            color: hostWindow.mutedText
                                            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(9) : 9)
                                            elide: Text.ElideRight
                                            Layout.fillWidth: true
                                        }
                                    }

                                    // Hex preview
                                    Text {
                                        id: themeEditorContentLeaf8
                                        objectName: "themeEditorContentLeaf8"
                                        transform: Translate {
                                            x: hostWindow.dialogPixelOffsetX(themeEditorContentLeaf8, hostWindow.contentItem)
                                            y: hostWindow.dialogPixelOffsetY(themeEditorContentLeaf8, hostWindow.contentItem)
                                        }
                                        text: hostWindow.formatColorHex(hostWindow[itemDelegate.def.id])
                                        color: hostWindow.mutedText
                                        font.family: hostWindow.uiFontFamily
                                        font.pixelSize: (hostWindow ? hostWindow.uiTextSize(10) : 10)
                                    }
                                }

                                MouseArea {
                                    id: itemMouse
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    cursorShape: Qt.PointingHandCursor
                                    onEntered: {
                                        if (!pressed)
                                            themeColorConfigurator.flash(itemDelegate.def.id)
                                    }
                                    onExited: {
                                        themeColorConfigurator.endHoverFlash(
                                            itemDelegate.def.id)
                                    }
                                    onPressed: function(mouse) {
                                        themeColorConfigurator.selectItem(index, false)
                                        themeColorConfigurator.startPressFlash(itemDelegate.def.id)
                                    }
                                    onReleased: function(mouse) {
                                        themeColorConfigurator.endPressFlash(itemDelegate.def.id)
                                    }
                                    onCanceled: {
                                        themeColorConfigurator.endPressFlash(itemDelegate.def.id)
                                    }
                                }
                            }
                        }
                    }
                }
                }

                // Vertical Divider
                Item {
                    Layout.fillHeight: true
                    Layout.preferredWidth: hostWindow.separatorWidth
                    Layout.minimumWidth: hostWindow.separatorWidth
                    Layout.maximumWidth: hostWindow.separatorWidth
                    implicitWidth: hostWindow.separatorWidth

                    Rectangle {
                        id: themeColorDivider
                        objectName: "themeColorDivider"
                        width: parent.width
                        height: hostWindow.snapPx(parent.height)
                        color: hostWindow.separatorColor
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                themeColorDivider,
                                themeColorConfigurator.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeColorDivider,
                                themeColorConfigurator.contentItem)
                        }
                    }
                }

                // Right Column: Color Editor - STRICTLY FIXED WIDTH
                ThemeColorEditorPane {
                    hostWindow: themeColorConfigurator.hostWindow
                    editorWindow: themeColorConfigurator
                    draft: themeDraft
                }
            }

            Rectangle {
                id: themeFooterDivider
                objectName: "themeFooterDivider"
                Layout.fillWidth: true
                Layout.preferredHeight: hostWindow.separatorWidth
                Layout.minimumHeight: hostWindow.separatorWidth
                Layout.maximumHeight: hostWindow.separatorWidth
                implicitHeight: hostWindow.separatorWidth
                color: hostWindow.separatorColor
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                        themeFooterDivider,
                        themeColorConfigurator.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                        themeFooterDivider,
                        themeColorConfigurator.contentItem)
                }
            }

            // Footer Actions
            ThemeEditorFooter {
                sharedSettingsFooter: themeColorConfigurator.embeddedSettings
                hostWindow: themeColorConfigurator.hostWindow
                editorWindow: themeColorConfigurator
                draft: themeDraft
            }
        }
    }
}
