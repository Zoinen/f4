pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

FocusScope {
    id: guiSettings
    objectName: "guiSettingsPage"
    required property ApplicationWindow hostWindow
    readonly property Item contentItem: guiSettings
    property string statusToast: ""
    readonly property string status: statusToast
    property var draftBaseline: ({})
    readonly property var preferenceKeys: [
        "fontRenderType", "mouseWheelMode", "galleryNeutralFileTextColors",
        "galleryShowSelectionBorders", "commandLineGraphicalCursor", "compactBreadcrumbs",
        "showColumnSeparators", "panelColumnPadding", "iconSetName",
        "headerVerticalSeparatorSpacing", "headerHorizontalSeparatorSpacing", "columnSeparatorSpacing"
    ]
    implicitWidth: hostWindow.snapPx(535)
    implicitHeight: options.implicitHeight
    function captureBaseline() {
        const values = {}
        for (const key of preferenceKeys) values[key] = hostWindow[key]
        draftBaseline = values
    }
    function applyDraft() {
        if (!hostWindow.saveThemeToPersistence()) {
            statusToast = qsTr("Failed to save GUI settings")
            return false
        }
        captureBaseline()
        statusToast = qsTr("GUI settings saved")
        return true
    }
    function resetDraft() {
        for (const key of Object.keys(draftBaseline)) {
            if (key === "fontRenderType") hostWindow.setFontRenderType(draftBaseline[key])
            else if (key === "iconSetName") hostWindow.setIconSet(draftBaseline[key])
            else hostWindow[key] = draftBaseline[key]
        }
        statusToast = ""
    }
    Component.onCompleted: captureBaseline()
    Component.onDestruction: if (hostWindow) resetDraft()
    ColumnLayout {
        id: options
        width: parent.width
        spacing: hostWindow.snapPx(10)
            Rectangle {
                id: themeFontRenderTypePanel
                objectName: "themeFontRenderTypePanel"
                Layout.fillWidth: true
                Layout.preferredHeight: hostWindow.snapPx(42)
                implicitHeight: hostWindow.snapPx(42)
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                        themeFontRenderTypePanel,
                        guiSettings.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                        themeFontRenderTypePanel,
                        guiSettings.contentItem)
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
                                guiSettings.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeFontRenderTypeLabels,
                                guiSettings.contentItem)
                        }

                        Text {
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(themeFontRenderTypeTitle, hostWindow.contentItem)
                                y: hostWindow.dialogPixelOffsetY(themeFontRenderTypeTitle, hostWindow.contentItem)
                            }
                            id: themeFontRenderTypeTitle
                            objectName: "themeFontRenderTypeTitle"
                            text: "Font rendering"
                            color: hostWindow.textColor
                            font.family: hostWindow.uiFontFamily
                            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(11) : 11)
                            font.weight: Font.Bold
                        }

                        Text {
                            id: themeFontRenderTypeDescription
                            objectName: "themeFontRenderTypeDescription"
                            text: hostWindow.fontRenderTypeDescription
                            color: hostWindow.mutedText
                            font.family: hostWindow.uiFontFamily
                            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(9) : 9)
                            elide: Text.ElideRight
                            Layout.fillWidth: true
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                    themeFontRenderTypeDescription,
                                    guiSettings.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                    themeFontRenderTypeDescription,
                                    guiSettings.contentItem)
                            }
                        }
                    }

                    ThemeRenderTypeComboBox {
                        id: themeFontRenderTypeCombo
                        hostWindow: guiSettings.hostWindow
                        objectName: "themeFontRenderTypeCombo"
                        options: hostWindow.fontRenderTypeOptions
                        selectedRenderType: hostWindow.fontRenderType
                        Layout.preferredWidth: hostWindow.snapPx(174)
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                themeFontRenderTypeCombo,
                                guiSettings.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeFontRenderTypeCombo,
                                guiSettings.contentItem)
                        }
                        onRenderTypeActivated: function(value) {
                            if (hostWindow.setFontRenderType(value))
                                guiSettings.statusToast =
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
                        guiSettings.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                        themeMouseWheelPanel,
                        guiSettings.contentItem)
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
                                guiSettings.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeMouseWheelLabels,
                                guiSettings.contentItem)
                        }

                        Text {
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(themeMouseWheelTitle, hostWindow.contentItem)
                                y: hostWindow.dialogPixelOffsetY(themeMouseWheelTitle, hostWindow.contentItem)
                            }
                            id: themeMouseWheelTitle
                            objectName: "themeMouseWheelTitle"
                            text: "Mouse wheel control"
                            color: hostWindow.textColor
                            font.family: hostWindow.uiFontFamily
                            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(11) : 11)
                            font.weight: Font.Bold
                        }

                        Text {
                            id: themeMouseWheelDescription
                            objectName: "themeMouseWheelDescription"
                            text: hostWindow.mouseWheelModeDescription
                            color: hostWindow.mutedText
                            font.family: hostWindow.uiFontFamily
                            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(9) : 9)
                            elide: Text.ElideRight
                            Layout.fillWidth: true
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                    themeMouseWheelDescription,
                                    guiSettings.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                    themeMouseWheelDescription,
                                    guiSettings.contentItem)
                            }
                        }
                    }

                    ThemeRenderTypeComboBox {
                        id: themeMouseWheelCombo
                        hostWindow: guiSettings.hostWindow
                        objectName: "themeMouseWheelCombo"
                        options: hostWindow.mouseWheelModeOptions
                        selectedValue: hostWindow.mouseWheelMode
                        Layout.preferredWidth: hostWindow.snapPx(174)
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                themeMouseWheelCombo,
                                guiSettings.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeMouseWheelCombo,
                                guiSettings.contentItem)
                        }
                        onOptionActivated: function(value) {
                            if (hostWindow.setMouseWheelMode(value))
                                guiSettings.statusToast =
                                    "Mouse wheel: " + hostWindow.mouseWheelModeName
                        }
                    }
                }
            }

            Rectangle {
                id: themeIconSetPanel
                objectName: "themeIconSetPanel"
                Layout.fillWidth: true
                Layout.preferredHeight: hostWindow.snapPx(42)
                implicitHeight: hostWindow.snapPx(42)
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                        themeIconSetPanel,
                        guiSettings.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                        themeIconSetPanel,
                        guiSettings.contentItem)
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
                        id: themeIconSetLabels
                        objectName: "themeIconSetLabels"
                        Layout.fillWidth: true
                        spacing: hostWindow.snapPx(1)
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                themeIconSetLabels,
                                guiSettings.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeIconSetLabels,
                                guiSettings.contentItem)
                        }

                        Text {
                            id: themeIconSetTitle
                            objectName: "themeIconSetTitle"
                            text: "Icon set"
                            color: hostWindow.textColor
                            font.family: hostWindow.guiMonospaceFontFamily
                            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(11) : 11)
                            font.weight: Font.Bold
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                    themeIconSetTitle,
                                    guiSettings.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                    themeIconSetTitle,
                                    guiSettings.contentItem)
                            }
                        }

                        Text {
                            id: themeIconSetDescription
                            objectName: "themeIconSetDescription"
                            text: hostWindow.iconSetDescription
                            color: hostWindow.mutedText
                            font.family: hostWindow.guiMonospaceFontFamily
                            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(9) : 9)
                            elide: Text.ElideRight
                            Layout.fillWidth: true
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                    themeIconSetDescription,
                                    guiSettings.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                    themeIconSetDescription,
                                    guiSettings.contentItem)
                            }
                        }
                    }

                    ThemeRenderTypeComboBox {
                        id: themeIconSetCombo
                        hostWindow: guiSettings.hostWindow
                        objectName: "themeIconSetCombo"
                        options: hostWindow.iconSetOptions
                        selectedValue: hostWindow.iconSetName
                        Layout.preferredWidth: hostWindow.snapPx(174)
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                themeIconSetCombo,
                                guiSettings.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                themeIconSetCombo,
                                guiSettings.contentItem)
                        }
                        onOptionActivated: function(value) {
                            if (hostWindow.setIconSet(value))
                                guiSettings.statusToast =
                                    "Icon set: " + hostWindow.iconSetOption(value).name
                        }
                    }
                }
            }

            ThemeBooleanOption {
                hostWindow: guiSettings.hostWindow
                pixelGridRoot: guiSettings.contentItem
                namePrefix: "themeNeutralFileText"
                title: "Panel file and folder text"
                description: "Use neutral text colors; semantic colors still tint icons"
                checked: hostWindow.galleryNeutralFileTextColors
                onToggled: function(checked) {
                    hostWindow.galleryNeutralFileTextColors = checked
                    guiSettings.statusToast = checked
                            ? "Neutral panel text enabled" : "Semantic panel text enabled"
                }
            }

            ThemeBooleanOption {
                hostWindow: guiSettings.hostWindow
                pixelGridRoot: guiSettings.contentItem
                namePrefix: "themeSelectionBorder"
                title: "Selection borders"
                description: "Outline marked items in every view; disable for text-color marking only"
                checked: hostWindow.galleryShowSelectionBorders
                onToggled: function(checked) {
                    hostWindow.galleryShowSelectionBorders = checked
                    guiSettings.statusToast = checked
                            ? "Selection borders enabled" : "Text-only selection enabled"
                }
            }

            ThemeBooleanOption {
                hostWindow: guiSettings.hostWindow
                pixelGridRoot: guiSettings.contentItem
                namePrefix: "themeCommandLineCaret"
                title: qsTr("Graphical command-line caret")
                description: qsTr("Use a thin vertical caret; disable for the console underline")
                checked: hostWindow.commandLineGraphicalCursor
                onToggled: checked => hostWindow.commandLineGraphicalCursor = checked
            }

            ThemeBooleanOption {
                hostWindow: guiSettings.hostWindow
                pixelGridRoot: guiSettings.contentItem
                namePrefix: "themeCompactBreadcrumbs"
                title: qsTr("Compact breadcrumbs when the path does not fit")
                description: qsTr("Shorten ancestor names; disable to keep full names and scroll horizontally")
                checked: hostWindow.compactBreadcrumbs
                onToggled: checked => hostWindow.compactBreadcrumbs = checked
            }

            ThemeBooleanOption {
                hostWindow: guiSettings.hostWindow
                pixelGridRoot: guiSettings.contentItem
                namePrefix: "themeColumnSeparators"
                title: qsTr("Show column separators")
                description: qsTr("Draw vertical lines between columns in Details view")
                checked: hostWindow.showColumnSeparators
                onToggled: checked => hostWindow.showColumnSeparators = checked
            }

            ThemeBooleanOption {
                hostWindow: guiSettings.hostWindow
                pixelGridRoot: guiSettings.contentItem
                namePrefix: "themeheaderVerticalSeparatorSpacing"
                title: qsTr("Inset vertical header separators")
                description: qsTr("Add space above and below separators between column titles")
                checked: hostWindow.headerVerticalSeparatorSpacing
                onToggled: checked => hostWindow.headerVerticalSeparatorSpacing = checked
            }

            ThemeBooleanOption {
                hostWindow: guiSettings.hostWindow
                pixelGridRoot: guiSettings.contentItem
                namePrefix: "themeheaderHorizontalSeparatorSpacing"
                title: qsTr("Inset horizontal header separator")
                description: qsTr("Add space at both ends of the line below column titles")
                checked: hostWindow.headerHorizontalSeparatorSpacing
                onToggled: checked => hostWindow.headerHorizontalSeparatorSpacing = checked
            }

            ThemeBooleanOption {
                hostWindow: guiSettings.hostWindow
                pixelGridRoot: guiSettings.contentItem
                namePrefix: "themecolumnSeparatorSpacing"
                title: qsTr("Inset panel column separators")
                description: qsTr("Add space above and below the vertical lines between columns")
                checked: hostWindow.columnSeparatorSpacing
                onToggled: checked => hostWindow.columnSeparatorSpacing = checked
            }

            RowLayout {
                Layout.fillWidth: true
                Layout.preferredHeight: hostWindow.snapPx(42)
                spacing: hostWindow.snapPx(12)
                Text {
                    id: paddingTitle
                    objectName: "themeColumnPaddingTitle"
                    text: qsTr("Column padding")
                    font.family: hostWindow.uiFontFamily
                    font.pixelSize: hostWindow.uiTextSize(11)
                    color: hostWindow.textColor
                    Layout.fillWidth: true
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(paddingTitle, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(paddingTitle, hostWindow.contentItem)
                    }
                }
                F4Slider {
                    id: paddingSlider
                    objectName: "themeColumnPaddingSlider"
                    hostWindow: guiSettings.hostWindow
                    Layout.preferredWidth: hostWindow.snapPx(180)
                    from: 0
                    to: 24
                    stepSize: 1
                    snapMode: Slider.SnapAlways
                    value: hostWindow.panelColumnPadding
                    onMoved: hostWindow.panelColumnPadding = Math.round(value)
                    Accessible.name: qsTr("Column padding in Details and two/three columns")
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(paddingSlider, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(paddingSlider, hostWindow.contentItem)
                    }
                }
                Text {
                    id: paddingValue
                    objectName: "themeColumnPaddingValue"
                    text: hostWindow.panelColumnPadding + " px"
                    font.family: hostWindow.uiFontFamily
                    font.pixelSize: hostWindow.uiTextSize(11)
                    color: hostWindow.mutedText
                    Layout.preferredWidth: hostWindow.snapPx(42)
                    horizontalAlignment: Text.AlignRight
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(paddingValue, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(paddingValue, hostWindow.contentItem)
                    }
                }
            }


    }
}
