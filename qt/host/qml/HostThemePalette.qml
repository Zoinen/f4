pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import ZoinGallery 1.0 as ZG

Item {
    id: palette

    required property ApplicationWindow hostWindow
    property var persistence: null
    property var textRendering: null

    readonly property int schemaVersion: 1

    property color windowBackgroundColor: "#191d23"
    property color panelPathBg: "#26576478"
    property color commandLineBg: "#141921"
    property color activeBorder: "#f0c95a"
    property color textColor: "#e8edf2"
    property color mutedText: "#9aa7b5"
    property color selectedBg: "#285d8f"
    property color panelSelectionBg: "#18456e"
    property color panelSelectionBorder: "#1d5888"
    property color titleBarBg: "#19202b"
    property color fBarBg: "#19202b"
    property color chromeText: "#d7e0ea"
    property color dialogBg: "#171e27"
    property color dialogHeaderBg: "#1b242f"
    property color controlBg: "#222c38"
    property color controlHoverBg: "#2a3745"
    property color controlPressedBg: "#10161e"
    property color controlBorder: "#3a495b"
    property color separatorColor: "#2d3642"
    property color separatorHoverColor: "#464d55"
    property color separatorActiveColor: "#59616a"
    property color dialogAccent: "#4e9bd4"

    property color galleryPanelBackgroundColor: "#00000000"
    property color galleryViewerBackgroundColor: "#00000000"
    property color galleryTextColor: "#e8edf2"
    property color galleryMutedTextColor: "#9aa7b5"
    property color galleryFileTextColor: "#c4cbd3"
    property color galleryFolderTextColor: "#ffffff"
    property bool galleryNeutralFileTextColors: true
    property bool galleryShowSelectionBorders: true
    property color galleryQuickSearchMatchColor: "#e8edf2"
    property color galleryDirectoryTextColor: "#98d8ff"
    property color galleryFolderIconColor: "#5ab2f1"
    property color galleryCursorColor: "#1d5888"
    property color galleryCursorBackgroundColor: "#18456e"
    property color galleryCursorBorderColor: "#1d5888"
    property color galleryCardCursorBorderColor: "#2777b8"
    property color gallerySelectionColor: "#ffd43b"
    property color galleryMarkedBackgroundColor: "#4f5037"
    property color galleryMarkedTextColor: "#ffd43b"
    property color galleryItemBackgroundColor: "#00000000"
    property color galleryDirectoryBackgroundColor: "#00000000"
    property color galleryItemHoverColor: "#1a75afe5"
    property color galleryLabelBackgroundColor: "#aa101216"
    property color galleryPreviewBackdropColor: "#4d000000"
    property color gallerySeparatorColor: "#30363d"
    property color galleryHeaderTextColor: "#d7e0ea"
    property color galleryControlHoverColor: "#2a3745"
    property color galleryScrollBarHandleColor: "#434b57"
    property color galleryScrollBarBackgroundHoverColor: "#5f6875"
    property color galleryScrollBarHoverColor: "#7f8896"
    property color galleryScrollBarPressedColor: "#47515d"
    property color galleryScrollBarTrackHoverColor: "#0fffffff"
    property color galleryPathBackgroundColor: "#00000000"
    property color galleryPathTextColor: "#e8edf2"
    property color galleryPathHoverColor: "#2a3745"
    property color galleryPathItemHoverColor: "#2a3745"
    property color galleryPathItemPressedColor: "#10161e"

    readonly property var colorDefinitions: [
        { id: "windowBackgroundColor", name: "Window Background", group: "Window", defaultColor: "#191d23" },
        { id: "titleBarBg", name: "Chrome / Title Bar Background", group: "Window", defaultColor: "#19202b" },
        { id: "fBarBg", name: "Chrome / F-Bar Background", group: "Window", defaultColor: "#19202b" },
        { id: "chromeText", name: "Chrome Text", group: "Window", defaultColor: "#d7e0ea" },
        { id: "panelPathBg", name: "Panel Header Background", group: "Panels", defaultColor: "#26576478" },
        { id: "commandLineBg", name: "Command Line Background", group: "Terminal", defaultColor: "#141921" },
        { id: "textColor", name: "Primary Text Color", group: "Text", defaultColor: "#e8edf2" },
        { id: "mutedText", name: "Secondary / Muted Text Color", group: "Text", defaultColor: "#9aa7b5" },
        { id: "selectedBg", name: "General Selection Background", group: "Selection", defaultColor: "#285d8f" },
        { id: "panelSelectionBg", name: "Interactive Selection Background", group: "Selection", defaultColor: "#18456e" },
        { id: "panelSelectionBorder", name: "Interactive Selection Border", group: "Selection", defaultColor: "#1d5888" },
        { id: "dialogAccent", name: "Highlight / Accent Color", group: "Icons & Accents", defaultColor: "#4e9bd4" },
        { id: "activeBorder", name: "Attention / Active Label", group: "Icons & Accents", defaultColor: "#f0c95a" },
        { id: "dialogBg", name: "Dialog Background", group: "Dialogs", defaultColor: "#171e27" },
        { id: "dialogHeaderBg", name: "Dialog Header Background", group: "Dialogs", defaultColor: "#1b242f" },
        { id: "controlBg", name: "Control Background", group: "Controls", defaultColor: "#222c38" },
        { id: "controlHoverBg", name: "Control Hover Background", group: "Controls", defaultColor: "#2a3745" },
        { id: "controlPressedBg", name: "Control Pressed Background", group: "Controls", defaultColor: "#10161e" },
        { id: "controlBorder", name: "Control Border", group: "Controls", defaultColor: "#3a495b" },
        { id: "separatorColor", name: "Separator Color", group: "Separators", defaultColor: "#2d3642" },
        { id: "separatorHoverColor", name: "Separator Hover Color", group: "Separators", defaultColor: "#464d55" },
        { id: "separatorActiveColor", name: "Separator Active Color", group: "Separators", defaultColor: "#59616a" },
        { id: "galleryPanelBackgroundColor", name: "Gallery Panel Background", group: "Panel Colors", defaultColor: "#00000000" },
        { id: "galleryViewerBackgroundColor", name: "Gallery Viewer Background", group: "Panel Colors", defaultColor: "#00000000" },
        { id: "galleryTextColor", name: "File Text", group: "Panel Colors", defaultColor: "#e8edf2" },
        { id: "galleryMutedTextColor", name: "Secondary File Text", group: "Panel Colors", defaultColor: "#9aa7b5" },
        { id: "galleryFileTextColor", name: "Neutral File Text", group: "Panel Colors", defaultColor: "#c4cbd3" },
        { id: "galleryFolderTextColor", name: "Neutral Folder Text", group: "Panel Colors", defaultColor: "#ffffff" },
        { id: "galleryQuickSearchMatchColor", name: "Quick Search Match", group: "Panel Colors", defaultColor: "#e8edf2" },
        { id: "galleryDirectoryTextColor", name: "Directory Text", group: "Panel Colors", defaultColor: "#98d8ff" },
        { id: "galleryFolderIconColor", name: "Folder Icon", group: "Panel Colors", defaultColor: "#5ab2f1" },
        { id: "galleryCursorColor", name: "Card Cursor Fill", group: "Panel Colors", defaultColor: "#1d5888" },
        { id: "galleryCursorBackgroundColor", name: "Details Cursor Fill", group: "Panel Colors", defaultColor: "#18456e" },
        { id: "galleryCursorBorderColor", name: "Details Cursor Border", group: "Panel Colors", defaultColor: "#1d5888" },
        { id: "galleryCardCursorBorderColor", name: "Card Cursor Border", group: "Panel Colors", defaultColor: "#2777b8" },
        { id: "gallerySelectionColor", name: "Selection Border", group: "Panel Colors", defaultColor: "#ffd43b" },
        { id: "galleryMarkedBackgroundColor", name: "Marked Row Background", group: "Panel Colors", defaultColor: "#4f5037" },
        { id: "galleryMarkedTextColor", name: "Marked Item Text", group: "Panel Colors", defaultColor: "#ffd43b" },
        { id: "galleryItemBackgroundColor", name: "Image Card Background", group: "Panel Colors", defaultColor: "#00000000" },
        { id: "galleryDirectoryBackgroundColor", name: "Directory Card Background", group: "Panel Colors", defaultColor: "#00000000" },
        { id: "galleryItemHoverColor", name: "Item Hover", group: "Panel Colors", defaultColor: "#1a75afe5" },
        { id: "galleryLabelBackgroundColor", name: "Thumbnail Label Background", group: "Panel Colors", defaultColor: "#aa101216" },
        { id: "galleryPreviewBackdropColor", name: "Preview Placeholder", group: "Panel Colors", defaultColor: "#4d000000" },
        { id: "gallerySeparatorColor", name: "Gallery Separator", group: "Panel Colors", defaultColor: "#30363d" },
        { id: "galleryHeaderTextColor", name: "Gallery Header Text", group: "Panel Colors", defaultColor: "#d7e0ea" },
        { id: "galleryControlHoverColor", name: "Gallery Control Hover", group: "Panel Colors", defaultColor: "#2a3745" },
        { id: "galleryScrollBarHandleColor", name: "Scrollbar Handle", group: "Panel Colors", defaultColor: "#434b57" },
        { id: "galleryScrollBarBackgroundHoverColor", name: "Scrollbar Hover Background", group: "Panel Colors", defaultColor: "#5f6875" },
        { id: "galleryScrollBarHoverColor", name: "Scrollbar Handle Hover", group: "Panel Colors", defaultColor: "#7f8896" },
        { id: "galleryScrollBarPressedColor", name: "Scrollbar Handle Pressed", group: "Panel Colors", defaultColor: "#47515d" },
        { id: "galleryScrollBarTrackHoverColor", name: "Scrollbar Track Hover", group: "Panel Colors", defaultColor: "#0fffffff" },
        { id: "galleryPathBackgroundColor", name: "Path Background", group: "Panel Colors", defaultColor: "#00000000" },
        { id: "galleryPathTextColor", name: "Path Text", group: "Panel Colors", defaultColor: "#e8edf2" },
        { id: "galleryPathHoverColor", name: "Path Hover", group: "Panel Colors", defaultColor: "#2a3745" },
        { id: "galleryPathItemHoverColor", name: "Breadcrumb Hover", group: "Panel Colors", defaultColor: "#2a3745" },
        { id: "galleryPathItemPressedColor", name: "Breadcrumb Pressed", group: "Panel Colors", defaultColor: "#10161e" }
    ]

    readonly property ZG.GalleryThemePalette galleryTheme:
        ZG.GalleryThemePalette {
            panelBackground: palette.galleryPanelBackgroundColor
            viewerBackground: palette.galleryViewerBackgroundColor
            cursor: palette.galleryCursorColor
            cursorBackground: palette.galleryCursorBackgroundColor
            cursorBorder: palette.galleryCursorBorderColor
            cardCursorBorder: palette.galleryCardCursorBorderColor
            text: palette.galleryTextColor
            mutedText: palette.galleryMutedTextColor
            fileText: palette.galleryFileTextColor
            folderText: palette.galleryFolderTextColor
            neutralFileTextColors: palette.galleryNeutralFileTextColors
            showSelectionBorders: palette.galleryShowSelectionBorders
            quickSearchMatch: palette.galleryQuickSearchMatchColor
            selection: palette.gallerySelectionColor
            markedBackground: palette.galleryMarkedBackgroundColor
            markedText: palette.galleryMarkedTextColor
            directoryText: palette.galleryDirectoryTextColor
            folderIcon: palette.galleryFolderIconColor
            itemBackground: palette.galleryItemBackgroundColor
            directoryBackground: palette.galleryDirectoryBackgroundColor
            itemHover: palette.galleryItemHoverColor
            labelBackground: palette.galleryLabelBackgroundColor
            previewBackdrop: palette.galleryPreviewBackdropColor
            separator: palette.gallerySeparatorColor
            headerText: palette.galleryHeaderTextColor
            controlHover: palette.galleryControlHoverColor
            scrollBarHandle: palette.galleryScrollBarHandleColor
            scrollBarHandleBackgroundHovered:
                palette.galleryScrollBarBackgroundHoverColor
            scrollBarHandleHovered: palette.galleryScrollBarHoverColor
            scrollBarHandlePressed: palette.galleryScrollBarPressedColor
            scrollBarTrackHovered: palette.galleryScrollBarTrackHoverColor
        }

    readonly property ZG.GalleryPresentationMetrics galleryMetrics:
        ZG.GalleryPresentationMetrics {
            detailsRowInset: palette.hostWindow.snapPx(
                                 palette.hostWindow.panelRowInnerSpacing)
            detailsRowSpacing: palette.hostWindow.snapPx(8)
            detailsIconSlotSize: palette.hostWindow.snapPx(16)
            detailsIconSize: palette.hostWindow.snapPx(16)
            detailsNameFontPixelSize: 13
            detailsSecondaryFontPixelSize: 12
            detailsExtensionMinimumWidth: palette.hostWindow.snapPx(40)
            detailsExtensionMaximumWidth: palette.hostWindow.snapPx(80)
            detailsSizeColumnWidth: palette.hostWindow.snapPx(96)
            detailsHeaderHeight: palette.hostWindow.snapPx(
                                     Math.max(22, palette.hostWindow.ch)
                                     + palette.hostWindow.verticalContentSpacing)
            detailsHeaderCellInset: palette.hostWindow.snapPx(
                                        palette.hostWindow.panelRowInnerSpacing)
            detailsHeaderFontPixelSize: 12
            detailsSeparatorVerticalMargin: palette.hostWindow.snapPx(
                                                palette.hostWindow.columnSeparatorVerticalMargin)
            detailsSeparatorWidth: palette.hostWindow.separatorWidth
            detailsScrollBarWidth: palette.hostWindow.snapPx(16)
        }

    visible: false
    width: 0
    height: 0

    function setFontRenderType(value) {
        if (!textRendering)
            return false
        textRendering.renderType = Number(value)
        return textRendering.renderType === Number(value)
    }

    function loadFromPersistence() {
        if (!persistence)
            return false
        try {
            const saved = persistence.loadTheme()
            const savedSchemaVersion = Number(saved.themeSchemaVersion || 0)
            let applied = false
            for (let index = 0; index < colorDefinitions.length; ++index) {
                const definition = colorDefinitions[index]
                if (!Object.prototype.hasOwnProperty.call(saved, definition.id)
                        || !saved[definition.id])
                    continue
                const value = Qt.color(saved[definition.id])
                if (!value)
                    continue
                const legacyTransparentItemHover = savedSchemaVersion < 1
                        && definition.id === "galleryItemHoverColor"
                        && Number(value.a) === 0
                palette[definition.id] = legacyTransparentItemHover
                        ? Qt.color(definition.defaultColor) : value
                applied = true
            }
            if (Object.prototype.hasOwnProperty.call(saved, "chromeBg")
                    && saved.chromeBg) {
                const legacyChromeBg = Qt.color(saved.chromeBg)
                if (legacyChromeBg) {
                    if (!saved.titleBarBg) {
                        titleBarBg = legacyChromeBg
                        applied = true
                    }
                    if (!saved.fBarBg) {
                        fBarBg = legacyChromeBg
                        applied = true
                    }
                }
            }
            if (saved.fontRenderType !== undefined
                    && saved.fontRenderType !== "" && textRendering) {
                applied = textRendering.setRenderTypeByName(
                              String(saved.fontRenderType)) || applied
            }
            if (saved.mouseWheelMode !== undefined
                    && saved.mouseWheelMode !== "") {
                applied = hostWindow.setMouseWheelMode(
                              String(saved.mouseWheelMode)) || applied
            }
            if (saved.neutralFileTextColors !== undefined) {
                const value = saved.neutralFileTextColors
                galleryNeutralFileTextColors = value === true
                        || String(value).toLowerCase() === "true"
                applied = true
            }
            if (saved.showSelectionBorders !== undefined) {
                const value = saved.showSelectionBorders
                galleryShowSelectionBorders = value === true
                        || String(value).toLowerCase() === "true"
                applied = true
            } else if (Object.keys(saved).length > 0) {
                // Older saved themes predate the option. Restoring one must
                // also undo an unsaved switch to text-only selection.
                galleryShowSelectionBorders = true
            }
            return applied
        } catch (error) {
            console.warn("Unable to load the saved theme:", error)
            return false
        }
    }

    function saveToPersistence() {
        if (!persistence)
            return false
        const values = ({})
        for (let index = 0; index < colorDefinitions.length; ++index) {
            const definition = colorDefinitions[index]
            values[definition.id] = formatColorHex(palette[definition.id])
        }
        values.fontRenderType = hostWindow.fontRenderTypeName
        values.mouseWheelMode = hostWindow.mouseWheelMode
        values.neutralFileTextColors = galleryNeutralFileTextColors
        values.showSelectionBorders = galleryShowSelectionBorders
        values.themeSchemaVersion = schemaVersion
        return persistence.saveTheme(values)
    }

    function resetToDefaults() {
        for (let index = 0; index < colorDefinitions.length; ++index) {
            const definition = colorDefinitions[index]
            palette[definition.id] = Qt.color(definition.defaultColor)
        }
        if (textRendering)
            textRendering.setRenderTypeByName("NativeRendering")
        hostWindow.mouseWheelMode = "gui"
        galleryNeutralFileTextColors = true
        galleryShowSelectionBorders = true
    }

    function formatColorHex(colorValue) {
        if (!colorValue)
            return "#000000"
        const red = Math.round(colorValue.r * 255)
        const green = Math.round(colorValue.g * 255)
        const blue = Math.round(colorValue.b * 255)
        const alpha = Math.round((colorValue.a !== undefined
                                  ? colorValue.a : 1) * 255)
        const hex2 = (value) => (value < 16 ? "0" : "")
                + value.toString(16)
        if (alpha < 255)
            return "#" + hex2(alpha) + hex2(red) + hex2(green) + hex2(blue)
        return "#" + hex2(red) + hex2(green) + hex2(blue)
    }
}
