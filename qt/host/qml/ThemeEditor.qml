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


    onClosing: themeColorConfigurator.stopAllFlashing()

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
            anchors.margins: hostWindow.snapPx(14)
            spacing: hostWindow.snapPx(10)

            // Header
            RowLayout {
                id: themeDialogHeader
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

                IconLabel {
                    icon.source: hostWindow.lucideIconSource(
                                     "palette", 18, hostWindow.dialogAccent)
                    icon.width: hostWindow.snapPx(18)
                    icon.height: hostWindow.snapPx(18)
                    icon.color: hostWindow.dialogAccent
                }

                Text {
                    text: "Theme Color Configurator"
                    color: hostWindow.textColor
                    font.family: hostWindow.guiMonospaceFontFamily
                    font.pixelSize: 14
                    font.weight: Font.Bold
                    Layout.fillWidth: true
                }

                Text {
                    text: themeColorConfigurator.themePersistence
                          ? themeColorConfigurator.themePersistence.themeFilePath
                          : "gui_theme.ini"
                    color: hostWindow.mutedText
                    font.family: hostWindow.guiMonospaceFontFamily
                    font.pixelSize: 10
                    elide: Text.ElideMiddle
                    Layout.maximumWidth: 320
                }
            }

            Rectangle {
                id: themeHeaderDivider
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

            Rectangle {
                id: themeFontRenderTypePanel
                objectName: "themeFontRenderTypePanel"
                Layout.fillWidth: true
                Layout.preferredHeight: hostWindow.snapPx(42)
                implicitHeight: hostWindow.snapPx(42)
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                        themeFontRenderTypePanel,
                        themeColorConfigurator.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                        themeFontRenderTypePanel,
                        themeColorConfigurator.contentItem)
                }
                radius: hostWindow.snapPx(4)
                color: hostWindow.dialogHeaderBg
                border.width: hostWindow.separatorWidth
                border.color: hostWindow.controlBorder

                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: hostWindow.snapPx(8)
                    anchors.rightMargin: hostWindow.snapPx(8)
                    spacing: hostWindow.snapPx(8)

                    ColumnLayout {
                        id: themeFontRenderTypeLabels
                        objectName: "themeFontRenderTypeLabels"
                        Layout.fillWidth: true
                        spacing: hostWindow.snapPx(1)
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                themeFontRenderTypeLabels,
                                themeColorConfigurator.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeFontRenderTypeLabels,
                                themeColorConfigurator.contentItem)
                        }

                        Text {
                            id: themeFontRenderTypeTitle
                            objectName: "themeFontRenderTypeTitle"
                            text: "Font rendering"
                            color: hostWindow.textColor
                            font.family: hostWindow.guiMonospaceFontFamily
                            font.pixelSize: 11
                            font.weight: Font.Bold
                        }

                        Text {
                            id: themeFontRenderTypeDescription
                            objectName: "themeFontRenderTypeDescription"
                            text: hostWindow.fontRenderTypeDescription
                            color: hostWindow.mutedText
                            font.family: hostWindow.guiMonospaceFontFamily
                            font.pixelSize: 9
                            elide: Text.ElideRight
                            Layout.fillWidth: true
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                    themeFontRenderTypeDescription,
                                    themeColorConfigurator.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                    themeFontRenderTypeDescription,
                                    themeColorConfigurator.contentItem)
                            }
                        }
                    }

                    ThemeRenderTypeComboBox {
                        id: themeFontRenderTypeCombo
                        hostWindow: themeColorConfigurator.hostWindow
                        objectName: "themeFontRenderTypeCombo"
                        options: hostWindow.fontRenderTypeOptions
                        selectedRenderType: hostWindow.fontRenderType
                        Layout.preferredWidth: hostWindow.snapPx(174)
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                themeFontRenderTypeCombo,
                                themeColorConfigurator.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeFontRenderTypeCombo,
                                themeColorConfigurator.contentItem)
                        }
                        onRenderTypeActivated: function(value) {
                            if (hostWindow.setFontRenderType(value))
                                themeColorConfigurator.statusToast =
                                    "Font rendering: " + hostWindow.fontRenderTypeName
                        }
                    }
                }
            }

            Rectangle {
                id: themeMouseWheelPanel
                objectName: "themeMouseWheelPanel"
                Layout.fillWidth: true
                Layout.preferredHeight: hostWindow.snapPx(42)
                implicitHeight: hostWindow.snapPx(42)
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                        themeMouseWheelPanel,
                        themeColorConfigurator.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                        themeMouseWheelPanel,
                        themeColorConfigurator.contentItem)
                }
                radius: hostWindow.snapPx(4)
                color: hostWindow.dialogHeaderBg
                border.width: hostWindow.separatorWidth
                border.color: hostWindow.controlBorder

                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: hostWindow.snapPx(8)
                    anchors.rightMargin: hostWindow.snapPx(8)
                    spacing: hostWindow.snapPx(8)

                    ColumnLayout {
                        id: themeMouseWheelLabels
                        objectName: "themeMouseWheelLabels"
                        Layout.fillWidth: true
                        spacing: hostWindow.snapPx(1)
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                themeMouseWheelLabels,
                                themeColorConfigurator.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeMouseWheelLabels,
                                themeColorConfigurator.contentItem)
                        }

                        Text {
                            id: themeMouseWheelTitle
                            objectName: "themeMouseWheelTitle"
                            text: "Mouse wheel control"
                            color: hostWindow.textColor
                            font.family: hostWindow.guiMonospaceFontFamily
                            font.pixelSize: 11
                            font.weight: Font.Bold
                        }

                        Text {
                            id: themeMouseWheelDescription
                            objectName: "themeMouseWheelDescription"
                            text: hostWindow.mouseWheelModeDescription
                            color: hostWindow.mutedText
                            font.family: hostWindow.guiMonospaceFontFamily
                            font.pixelSize: 9
                            elide: Text.ElideRight
                            Layout.fillWidth: true
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                    themeMouseWheelDescription,
                                    themeColorConfigurator.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                    themeMouseWheelDescription,
                                    themeColorConfigurator.contentItem)
                            }
                        }
                    }

                    ThemeRenderTypeComboBox {
                        id: themeMouseWheelCombo
                        hostWindow: themeColorConfigurator.hostWindow
                        objectName: "themeMouseWheelCombo"
                        options: hostWindow.mouseWheelModeOptions
                        selectedValue: hostWindow.mouseWheelMode
                        Layout.preferredWidth: hostWindow.snapPx(174)
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                themeMouseWheelCombo,
                                themeColorConfigurator.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeMouseWheelCombo,
                                themeColorConfigurator.contentItem)
                        }
                        onOptionActivated: function(value) {
                            if (hostWindow.setMouseWheelMode(value))
                                themeColorConfigurator.statusToast =
                                    "Mouse wheel: " + hostWindow.mouseWheelModeName
                        }
                    }
                }
            }

            ThemeBooleanOption {
                hostWindow: themeColorConfigurator.hostWindow
                pixelGridRoot: themeColorConfigurator.contentItem
                namePrefix: "themeNeutralFileText"
                title: "Panel file and folder text"
                description: "Use neutral text colors; semantic colors still tint icons"
                checked: hostWindow.galleryNeutralFileTextColors
                onToggled: function(checked) {
                    hostWindow.galleryNeutralFileTextColors = checked
                    themeColorConfigurator.statusToast = checked
                            ? "Neutral panel text enabled" : "Semantic panel text enabled"
                }
            }

            ThemeBooleanOption {
                hostWindow: themeColorConfigurator.hostWindow
                pixelGridRoot: themeColorConfigurator.contentItem
                namePrefix: "themeSelectionBorder"
                title: "Selection borders"
                description: "Outline marked items in every view; disable for text-color marking only"
                checked: hostWindow.galleryShowSelectionBorders
                onToggled: function(checked) {
                    hostWindow.galleryShowSelectionBorders = checked
                    themeColorConfigurator.statusToast = checked
                            ? "Selection borders enabled" : "Text-only selection enabled"
                }
            }

            // Main Body: Left (List of items) + Right (Editor)
            RowLayout {
                Layout.fillWidth: true
                Layout.fillHeight: true
                spacing: hostWindow.snapPx(12)

                // Left Column: Items List (Takes remaining width)
                ColumnLayout {
                    Layout.fillWidth: true
                    Layout.minimumWidth: 200
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
                                            text: itemDelegate.def.name
                                            color: hostWindow.textColor
                                            font.pixelSize: 11
                                            font.weight: itemDelegate.isSelected ? Font.Bold : Font.Normal
                                            elide: Text.ElideRight
                                            Layout.fillWidth: true
                                        }

                                        Text {
                                            text: itemDelegate.def.group
                                            color: hostWindow.mutedText
                                            font.pixelSize: 9
                                            elide: Text.ElideRight
                                            Layout.fillWidth: true
                                        }
                                    }

                                    // Hex preview
                                    Text {
                                        text: hostWindow.formatColorHex(hostWindow[itemDelegate.def.id])
                                        color: hostWindow.mutedText
                                        font.family: hostWindow.guiMonospaceFontFamily
                                        font.pixelSize: 10
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
                hostWindow: themeColorConfigurator.hostWindow
                editorWindow: themeColorConfigurator
                draft: themeDraft
            }
        }
    }
}
