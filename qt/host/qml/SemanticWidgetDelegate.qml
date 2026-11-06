pragma ComponentBehavior: Bound

import QtQuick
import F4QtHost 1.0
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl as Impl

Item {
    id: widgetRoot
    required property ApplicationWindow hostWindow
    property var widget: ({})
    property var tableRowAction: (action, index) => hostWindow.action({target: widget.id, action: action, index: index})
    property var buttonAction: () => hostWindow.action({target: widget.id, action: "control.activate"})
    property bool tableKeyboardNavigation: false
    property bool groupHeading: false
    property var dialogLayout: null
    property var labelData: ({})
    readonly property var inlineLabel: labelData && labelData.kind === "text" ? labelData : null
    FontMetrics { id: matrixLabelMetrics; font: widgetRoot.hostWindow.font }
    readonly property real matrixLabelWidth: (widget.items || []).length > 0
        ? Math.max(0, ...(widget.items || []).map(text => matrixLabelMetrics.advanceWidth(String(text))))
        : inlineLabelMeasure.contentWidth
    // Use the same styled text and font as the visible label, including mnemonics.
    Text {
        id: inlineLabelMeasure
        visible: false
        width: widgetRoot.inlineLabel ? hostWindow.pxW(widgetRoot.inlineLabel.w || 1) : 0
        elide: Text.ElideRight
        text: widgetRoot.inlineLabel
              ? (widgetRoot.inlineLabel.hotkey
                 ? hostWindow.mnemonicText(widgetRoot.inlineLabel.text, widgetRoot.inlineLabel.hotkey)
                 : String(widgetRoot.inlineLabel.text || "")) : ""
        textFormat: widgetRoot.inlineLabel && widgetRoot.inlineLabel.hotkey ? Text.StyledText : Text.PlainText
        font: hostWindow.font
    }
    readonly property real horizontalPosition: inlineLabel
        ? hostWindow.snapPx(hostWindow.pxX(Number(inlineLabel.x) - originX)
                            + (inlineLabel.iconName ? hostWindow.snapPx(16)
                                                    : widget.colorMatrixCell === true ? matrixLabelWidth : inlineLabelMeasure.contentWidth) + 12)
        : hostWindow.pxX((widget.x || 0) - originX)
          + (dialogLayout && typeof dialogLayout.horizontalExpansionBefore === "function"
             ? dialogLayout.horizontalExpansionBefore(widget) : 0)
    property int originX: 0
    property int originY: 0
    property real maximumWidth: Number.POSITIVE_INFINITY
    readonly property real semanticTop: hostWindow.pxY(
        (widget.y || 0) - originY - 1)
    readonly property real semanticHeight:
        hostWindow.dialogWidgetSemanticHeight(widget)
    readonly property real visualHeight:
        dialogLayout ? dialogLayout.widgetHeight(widget)
                     : hostWindow.dialogWidgetVisualHeight(widget)
    readonly property bool visuallyOverflows:
        visualHeight > semanticHeight + 0.001

    objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "Root"

    x: horizontalPosition
    y: dialogLayout ? dialogLayout.widgetTop(widget) - dialogLayout.rowTop(originY + 1)
                    : hostWindow.dialogWidgetVisualTop(
           (widget.y || 0) - originY - 1, widget)
    width: widget.colorMatrixCell === true ? maximumWidth : Math.min(dialogLayout && typeof dialogLayout.presentationWidth === "function"
                    ? dialogLayout.presentationWidth(widget) : hostWindow.pxW(widget.w || 1), maximumWidth)
    height: visualHeight
    visible: widget.visible !== false
    clip: false
    opacity: widget.dimmed === true && widget.kind !== "group" ? 0.5 : 1

    // Correct the whole control subtree in scene space so backgrounds,
    // hit regions, text, and icons share the same physical-pixel origin.
    readonly property point dialogPixelTranslation: Qt.point(pixelTranslation.x, pixelTranslation.y)
    transform: Translate {
        id: pixelTranslation
        x: widgetRoot.visuallyOverflows
           ? hostWindow.dialogPixelOffsetX(
                 widgetRoot, hostWindow.contentItem) : 0
        y: widgetRoot.visuallyOverflows
           ? hostWindow.dialogPixelOffsetY(
                 widgetRoot, hostWindow.contentItem) : 0
    }

    HoverHandler {
        enabled: !!widgetRoot.widget.explainTarget
                 && widgetRoot.widget.kind !== "radioGroup"
                 && widgetRoot.widget.kind !== "checkGroup"
        onHoveredChanged: if (hovered) hostWindow.action({
            target: widgetRoot.widget.explainTarget, action: "control.explain"
        }, true)
    }

    Loader {
        anchors.fill: parent
        anchors.rightMargin: hostWindow.snapPx((widget.resettable === true ? 32 : 0) + (widget.colorMatrixCell === true ? 48 : 0))
        sourceComponent: {
            if (widget.layoutRole === "description") return settingsHelpDelegate
            switch (widget.kind) {
            case "button": return buttonDelegate
            case "checkbox": return checkboxDelegate
            case "edit": return editDelegate
            case "multiLineEdit": return multilineDelegate
            case "text": return textDelegate
            case "separator": return separatorDelegate
            case "progressBar": return progressDelegate
            case "radioGroup": return choiceDelegate
            case "checkGroup": return choiceDelegate
            case "listBox": return listDelegate
            case "table": return tableDelegate
            case "comboBox": return comboDelegate
            case "group": return widget.fileGroupSample === true ? fileGroupSampleDelegate : widget.fileGroupsOverview === true ? fileGroupsOverviewDelegate : widget.fullPage === true && (widget.children || []).some(child => child.kind === "table") ? fullPageTableDelegate : widget.scrollable === true ? viewportDelegate : groupDelegate
            default: return textDelegate
            }
        }
    }

    Component {
        id: settingsHelpDelegate
        SettingsHelpCard {
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
            stacked: widgetRoot.widget.helpStacked === true
        }
    }

    F4Button {
        hostWindow: widgetRoot.hostWindow
        visible: widgetRoot.widget.resettable === true
        anchors.right: parent.right
        anchors.top: parent.top
        width: hostWindow.snapPx(28)
        height: hostWindow.snapPx(28)
        variant: "tool"
        iconSource: hostWindow.lucideIconSource("rotate-ccw", 14, hostWindow.mutedText)
        toolTipText: "Restore the standard value (Ctrl+Backspace)"
        onClicked: hostWindow.action({target: widgetRoot.widget.id, action: "control.reset"})
    }

    Rectangle {
        objectName: "dialogWidget-" + widgetRoot.widget.id + "ColorSwatch"
        visible: widgetRoot.widget.colorMatrixCell === true
        anchors.right: parent.right
        anchors.rightMargin: hostWindow.snapPx(widgetRoot.widget.resettable === true ? 34 : 0)
        anchors.verticalCenter: parent.verticalCenter
        width: hostWindow.snapPx(42); height: hostWindow.snapPx(28)
        radius: hostWindow.snapPx(3)
        color: (widgetRoot.widget.colorPreview || {}).background || hostWindow.dialogBg
        border.color: hostWindow.controlBorder
        Text {
            anchors.centerIn: parent
            text: "Aa"; font: widgetRoot.hostWindow.font
            color: (widgetRoot.widget.colorPreview || {}).foreground || widgetRoot.hostWindow.textColor
        }
    }

    Component {
        id: fileGroupSampleDelegate
        Item {
            id: samplesRoot
            readonly property var style: widgetRoot.widget.previewStyle || ({})
            readonly property var patches: [style.normal || ({}), style.selected || ({}), style.cursor || ({}), style.selectedCursor || ({})]
            readonly property int columns: width < 560 ? 2 : 4
            readonly property real tileWidth: widgetRoot.hostWindow.snapPx((width - (columns - 1)*8) / columns)
            readonly property var resolved: widgetRoot.hostWindow.iconProvider
                && typeof widgetRoot.hostWindow.iconProvider.previewFileStyle === "function"
                ? widgetRoot.hostWindow.iconProvider.previewFileStyle(String(widgetRoot.widget.previewCatalog || ""), style,
                    {name: widgetRoot.widget.previewName || "", isDir: widgetRoot.widget.previewDirectory === true},
                    16, widgetRoot.hostWindow.dpr) : ({})
            Repeater {
                model: widgetRoot.widget.items || []
                delegate: Item {
                    id: sampleTile
                    required property int index
                    required property string modelData
                    readonly property var preview: samplesRoot.patches[index] || ({})
                    x: (index % samplesRoot.columns) * (samplesRoot.tileWidth + 8)
                    y: Math.floor(index / samplesRoot.columns) * 48
                    width: samplesRoot.tileWidth; height: 44
                    Text {
                        x: 4; width: parent.width - 8; height: 18
                        text: sampleTile.modelData; font: widgetRoot.hostWindow.font
                        color: widgetRoot.hostWindow.mutedText; elide: Text.ElideRight
                    }
                    Rectangle {
                        objectName: "fileGroupPinnedSample-" + sampleTile.index
                        y: widgetRoot.hostWindow.snapPx(19); width: parent.width; height: widgetRoot.hostWindow.snapPx(25)
                        radius: 3; color: sampleTile.preview.background || widgetRoot.hostWindow.controlBg
                        Impl.IconImage {
                            objectName: "fileGroupPinnedIcon-" + sampleTile.index
                            x: 4; y: 4; width: 16; height: 16
                            source: samplesRoot.resolved.icon || ""
                            sourceSize: Qt.size(16 * widgetRoot.hostWindow.dpr, 16 * widgetRoot.hostWindow.dpr)
                            color: samplesRoot.style.tintIcon === true && widgetRoot.widget.previewCatalog !== "system"
                                ? (sampleTile.preview.foreground || widgetRoot.hostWindow.textColor) : "transparent"
                            fillMode: Image.PreserveAspectFit
                        }
                        Text {
                            id: sampleText
                            x: widgetRoot.hostWindow.snapPx(25); width: parent.width - x - 4; anchors.verticalCenter: parent.verticalCenter
                            text: String(samplesRoot.style.marker || "") + String(widgetRoot.widget.previewName || "")
                            font: widgetRoot.hostWindow.font; elide: Text.ElideMiddle
                            color: sampleTile.preview.foreground || widgetRoot.hostWindow.textColor
                            transform: Translate {
                                x: widgetRoot.hostWindow.dialogPixelOffsetX(sampleText, widgetRoot.hostWindow.contentItem)
                                y: widgetRoot.hostWindow.dialogPixelOffsetY(sampleText, widgetRoot.hostWindow.contentItem)
                            }
                        }
                    }
                }
            }
            Text {
                x: 0; y: samplesRoot.columns === 2 ? 96 : 48; width: parent.width; height: 16
                text: String(widgetRoot.widget.text || ""); font: widgetRoot.hostWindow.font
                color: widgetRoot.hostWindow.mutedText; elide: Text.ElideRight
            }
        }
    }

    Component {
        id: fileGroupsOverviewDelegate
        Item {
            readonly property var childrenData: widgetRoot.widget.children || []
            readonly property bool hasColorTheme: childrenData.some(w => w.layoutRole === "overview-color-theme" && w.visible !== false)
            Loader {
                id: overviewTheme
                readonly property var projection: parent.childrenData.find(w => w.layoutRole === "overview-theme") || ({})
                x: 0; y: 0; width: Math.min(parent.width * (parent.hasColorTheme ? .34 : .58), 300); height: 34
                Component.onCompleted: setSource(Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {hostWindow: widgetRoot.hostWindow, widget: Qt.binding(() => overviewTheme.projection)})
                onLoaded: { item.x = 0; item.y = 0; item.width = Qt.binding(() => overviewTheme.width); item.height = Qt.binding(() => overviewTheme.height) }
            }
            Loader {
                id: overviewColors
                readonly property var projection: parent.childrenData.find(w => w.layoutRole === "overview-color-theme") || ({})
                active: parent.hasColorTheme
                visible: active
                x: overviewTheme.width + 8; y: 0
                width: active ? Math.min(parent.width * .34, 300) : 0
                height: 34
                Component.onCompleted: if (active) setSource(Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {hostWindow: widgetRoot.hostWindow, widget: Qt.binding(() => overviewColors.projection)})
                onLoaded: { item.x = 0; item.y = 0; item.width = Qt.binding(() => overviewColors.width); item.height = Qt.binding(() => overviewColors.height) }
            }
            Text {
                x: (parent.hasColorTheme ? overviewColors.x + overviewColors.width : overviewTheme.width) + 12; y: 0
                width: Math.max(0,parent.width-x); height: 34
                text: (overviewTable.projection.searchText ? "Search: " + overviewTable.projection.searchText : "Type to search groups")
                font: widgetRoot.hostWindow.font
                color: widgetRoot.hostWindow.mutedText
                verticalAlignment: Text.AlignVCenter
                elide: Text.ElideRight
                MouseArea { anchors.fill: parent; onClicked: widgetRoot.hostWindow.action({target: overviewTable.projection.id,action:"control.focus"}) }
            }
            Loader {
                id: overviewTable
                readonly property var projection: parent.childrenData.find(w => w.layoutRole === "overview-table") || ({})
                x: 0; y: 44; width: parent.width; height: Math.max(1, actionFlow.y - y - 10)
                Component.onCompleted: setSource(Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {hostWindow: widgetRoot.hostWindow, widget: Qt.binding(() => overviewTable.projection)})
                onLoaded: { item.x = 0; item.y = 0; item.width = Qt.binding(() => overviewTable.width); item.height = Qt.binding(() => overviewTable.height) }
            }
            Flow {
                id: actionFlow
                x: 0; y: parent.height - height
                width: parent.width
                spacing: widgetRoot.hostWindow.snapPx(6)
                Repeater {
                    model: (parent.parent.childrenData.find(w => w.layoutRole === "overview-actions") || {}).children || []
                    delegate: Loader {
                        id: actionLoader
                        required property var modelData
                        required property int index
                        readonly property var projected: index === 7
                            ? Object.assign({}, modelData, {iconName:"rotate-ccw"})
                            : index < 2 ? modelData : Object.assign({},modelData,{
                            text:"",hotkey:"",toolTipText:String(modelData.text || ""),iconName:["plus","pencil","copy","trash-2","chevron-up","chevron-down","menu"][index] || "menu"})
                        width: widgetRoot.hostWindow.snapPx(index === 7 ? 180 : index < 2 ? 80 : 34); height: 34
                        Component.onCompleted: setSource(Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {hostWindow: widgetRoot.hostWindow, widget: Qt.binding(() => actionLoader.projected)})
                        onLoaded: { item.x = 0; item.y = 0; item.width = Qt.binding(() => actionLoader.width); item.height = Qt.binding(() => actionLoader.height) }
                    }
                }
            }
        }
    }

    Component {
        id: tableDelegate
        Item {
            id: tableControl
            enabled: widget.disabled !== true
            objectName: "dialogWidget-" + widget.id + "Table"
            readonly property var columns: widget.columns || []
            readonly property bool settingsNavigation: widget.settingsNavigation === true
            readonly property real rowHeight: hostWindow.snapPx(settingsNavigation
                ? Math.max(32, hostWindow.uiTextSize(13) + 18) : hostWindow.ch + 8)
            readonly property real cellInset: hostWindow.snapPx(settingsNavigation ? 9 : 6)
            function categoryColor(icon) {
                const colors = {palette:"#a980dc", "circle-play":"#69b968",
                    "panels-top-left":"#619fe4", "columns-2":"#619fe4",
                    "hard-drive":"#a0a8b4", "copy":"#e1a04f", "files":"#e1a04f",
                    "file-pen-line":"#e1a04f", "file-code":"#66bfa7", "keyboard":"#a0a8b4",
                    "square-terminal":"#7ba88d", "clock-3":"#c588ce", "file-type":"#579ee8",
                    "network":"#56b2dc", "globe":"#56b2dc", "info":"#579ee8",
                    "refresh-cw":"#69b968", "download":"#69b968", "plug":"#e1a04f",
                    "sparkles":"#bf86df", "monitor":"#579ee8", "image":"#66bfa7",
                    "type":"#e1a04f", "file-cog":"#a0a8b4"}
                return colors[icon] || "#579ee8"
            }
            readonly property real availableWidth: width - (tableScrollBar.visible ? tableScrollBar.width : 0)
                - (settingsNavigation ? hostWindow.snapPx(8) : 0)
            Rectangle {
                objectName: "dialogWidget-" + widget.id + "TableBackground"
                anchors.fill: parent
                color: tableControl.settingsNavigation ? "transparent" : hostWindow.controlPressedBg
                radius: hostWindow.snapPx(4)
            }
            function columnWidth(index) {
                let weight = 0
                for (const col of columns) weight += Math.max(8, Number(col.width) > 0 ? Number(col.width) : Number(col.minWidth || 24))
                return hostWindow.snapPx(availableWidth * Math.max(8, Number(columns[index].width) > 0 ? Number(columns[index].width) : Number(columns[index].minWidth || 24)) / Math.max(1, weight))
            }
            function columnX(index) {
                let result = 0
                for (let i = 0; i < index; ++i) result += columnWidth(i)
                return result
            }
            Text {
                id: tableSearch
                objectName: "dialogWidget-" + widget.id + "TableSearch"
                visible: widget.quickSearch === true && widget.layoutRole !== "overview-table"
                width: parent.width
                height: visible ? tableControl.rowHeight : 0
                text: widget.searchText ? "Search: " + widget.searchText : "Type to search"
                font: hostWindow.font
                color: hostWindow.textColor
                verticalAlignment: Text.AlignVCenter
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(tableSearch, hostWindow.contentItem)
                    y: hostWindow.dialogPixelOffsetY(tableSearch, hostWindow.contentItem)
                }
                MouseArea { anchors.fill: parent; onClicked: hostWindow.action({target: widget.id, action: "control.focus"}) }
            }
            Item {
                id: tableHeader
                y: tableSearch.height
                width: parent.width
                height: widget.showHeader === false ? 0 : tableControl.rowHeight
                visible: height > 0
                Repeater {
                    model: tableControl.columns
                    Text {
                        id: headerText
                        required property var modelData
                        required property int index
                        objectName: "dialogWidget-" + widget.id + "TableHeader-" + index
                        x: tableControl.columnX(index) + hostWindow.snapPx(6)
                        width: Math.max(0, tableControl.columnWidth(index) - hostWindow.snapPx(12))
                        height: tableHeader.height
                        text: modelData.title + (widget.sortColumn === index ? (widget.sortAscending ? " ↑" : " ↓") : "")
                        font: hostWindow.font
                        color: hostWindow.textColor
                        elide: Text.ElideRight
                        verticalAlignment: Text.AlignVCenter
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(headerText, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(headerText, hostWindow.contentItem)
                        }
                        MouseArea { anchors.fill: parent; enabled: widget.sortable !== false; onClicked: hostWindow.action({target: widget.id, action: "control.sort", index: headerText.index}) }
                    }
                }
            }
            ListView {
                id: tableRows
                objectName: "dialogWidget-" + widget.id + "TableRows"
                y: tableHeader.y + tableHeader.height
                width: parent.width
                height: Math.max(0, parent.height - y)
                clip: true
                model: SemanticChildrenModel { widgets: widgetRoot.widget.rows || [] }
                currentIndex: Number(widget.cursor || 0)
                onCurrentIndexChanged: positionViewAtIndex(currentIndex, ListView.Contain)
                // Passive hover/help/mark updates must not snap back to the
                // keyboard cursor while the user scrolls the list independently.
                onModelChanged: if (widget.layoutRole !== "overview-table" && !tableControl.settingsNavigation)
                    Qt.callLater(function() { positionViewAtIndex(currentIndex, ListView.Contain) })
                pixelAligned: true
                boundsBehavior: Flickable.StopAtBounds
                ScrollBar.vertical: F4ScrollBar {
                    id: tableScrollBar
                    objectName: "dialogWidget-" + widget.id + "TableScrollBar"
                    hostWindow: widgetRoot.hostWindow
                    policy: ScrollBar.AlwaysOn
                    visible: tableRows.contentHeight > tableRows.height
                }
                delegate: Rectangle {
                    id: tableRow
                    objectName: "dialogWidget-" + widget.id + "TableRow-" + index
                    radius: hostWindow.snapPx(tableControl.settingsNavigation ? 8 : 4)
                    required property var widgetData
                    readonly property var modelData: widgetData
                    required property int index
                    width: tableControl.availableWidth
                    height: tableControl.rowHeight
                    color: index === tableRows.currentIndex
                        ? (tableControl.settingsNavigation ? hostWindow.controlHoverBg : hostWindow.selectedBg) : "transparent"
                    readonly property string iconName: (widget.itemIcons || [])[index] || ""
                    readonly property bool filePreview: widget.layoutRole === "overview-table"
                    readonly property bool separator: !!modelData.separator
                    readonly property var appearance: modelData.appearance || ({})
                    readonly property bool marked: modelData.marked === true
                    readonly property bool cursor: index === tableRows.currentIndex
                    readonly property var patch: (marked ? (cursor ? appearance.selectedCursor : appearance.selected)
                                                         : (cursor ? appearance.cursor : appearance.normal)) || ({})
                    readonly property var iconChoice: (appearance.iconChoices || {})[String(widget.previewCatalog || "")] || ({})
                    readonly property var resolvedPreview: filePreview && !separator && hostWindow.iconProvider
                        && typeof hostWindow.iconProvider.previewFileStyle === "function"
                        ? hostWindow.iconProvider.previewFileStyle(String(widget.previewCatalog || ""), appearance,
                            {name: modelData.sampleName || "", isDir: modelData.directory === true}, 16, hostWindow.dpr) : ({})
                    readonly property bool dimmed: ((widget.itemStates || [])[index] || {}).dimmed === true
                    readonly property color foregroundColor: dimmed ? hostWindow.mutedText : hostWindow.textColor
                    readonly property real iconSpace: filePreview || iconName !== ""
                        ? hostWindow.snapPx(tableControl.settingsNavigation ? 27 : 24) : 0
                    Rectangle {
                        objectName: "settingsCategoryIconBadge-" + tableRow.index
                        visible: tableControl.settingsNavigation && tableRow.iconName !== ""
                        x: tableControl.cellInset
                        y: hostWindow.snapPx((tableRow.height - height) / 2)
                        width: hostWindow.snapPx(20); height: width
                        radius: hostWindow.snapPx(5)
                        antialiasing: true
                        color: "#18191b"
                        gradient: Gradient {
                            GradientStop { position: 0; color: "#28292b" }
                            GradientStop { position: 1; color: "#111214" }
                        }
                        border.width: 1 / hostWindow.dpr
                        border.color: "#38393b"
                        opacity: tableRow.dimmed ? 0.35 : 1
                    }
                    Repeater {
                        model: tableRow.filePreview && !tableRow.separator ? Math.max(0, tableControl.columns.length - 1) : 0
                        Rectangle {
                            required property int index
                            x: tableControl.columnX(index + 1)
                            width: 1 / hostWindow.dpr
                            height: tableRow.height
                            color: hostWindow.controlBorder
                        }
                    }
                    Rectangle {
                        x: 0; y: 0; width: tableControl.columnWidth(0); height: parent.height
                        visible: tableRow.filePreview && !tableRow.separator
                        color: tableRow.patch.background || (tableRow.cursor ? hostWindow.selectedBg : "transparent")
                    }
                    Impl.IconImage {
                        objectName: "dialogWidget-" + widget.id + "TableItemIcon-" + tableRow.index
                        x: hostWindow.snapPx(tableControl.settingsNavigation ? 11 : 6)
                        y: hostWindow.snapPx((tableRow.height - height) / 2)
                        width: hostWindow.snapPx(16)
                        height: width
                        visible: !tableRow.separator && (tableRow.iconName !== "" || (tableRow.filePreview && !!tableRow.resolvedPreview.icon))
                        source: tableRow.filePreview && visible
                                ? tableRow.resolvedPreview.icon
                                : visible ? hostWindow.lucideIconSource(tableRow.iconName, 16,
                                    tableControl.settingsNavigation && !tableRow.dimmed
                                        ? tableControl.categoryColor(tableRow.iconName) : tableRow.foregroundColor) : ""
                        sourceSize: Qt.size(width * hostWindow.dpr, height * hostWindow.dpr)
                        color: tableRow.filePreview && tableRow.appearance.tintIcon === true && widget.previewCatalog !== "system"
                            ? (tableRow.patch.foreground || tableRow.foregroundColor) : "transparent"
                        fillMode: Image.PreserveAspectFit
                    }
                    Repeater {
                        model: tableControl.columns
                        Text {
                            id: cellText
                            required property int index
                            objectName: "dialogWidget-" + widget.id + "TableCell-" + tableRow.index + "-" + index
                            readonly property real iconSpace: index === 0 && !tableRow.separator ? tableRow.iconSpace : 0
                            x: tableControl.columnX(index) + tableControl.cellInset + iconSpace
                            width: tableRow.separator && index === 0 ? tableControl.availableWidth - 2 * tableControl.cellInset
                                : Math.max(0, tableControl.columnWidth(index) - 2 * tableControl.cellInset - iconSpace)
                            height: tableRow.height
                            text: tableRow.modelData.cells[index] || ""
                            visible: !tableRow.separator || index === 0
                            font: Qt.font({family: hostWindow.font.family, pixelSize: hostWindow.font.pixelSize,
                                bold: tableRow.separator || (tableControl.settingsNavigation && tableRow.cursor)})
                            color: index === 0 && tableRow.filePreview && !tableRow.separator ? (tableRow.patch.foreground || tableRow.foregroundColor) : tableRow.foregroundColor
                            elide: Text.ElideRight
                            verticalAlignment: Text.AlignVCenter
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(cellText, hostWindow.contentItem)
                                y: hostWindow.dialogPixelOffsetY(cellText, hostWindow.contentItem)
                            }
                        }
                    }
                }
            }
            SemanticListPointer {
                objectName: "dialogWidget-" + widget.id + "TablePointer"
                hostWindow: widgetRoot.hostWindow
                widget: widgetRoot.widget
                view: tableRows
                rowAction: widgetRoot.tableRowAction
                keyboardNavigation: widgetRoot.tableKeyboardNavigation
                semanticWheel: widget.layoutRole !== "overview-table" && !tableControl.settingsNavigation
                nativeWheel: tableControl.settingsNavigation
                y: tableRows.y
                width: tableControl.availableWidth
                height: tableRows.height
                activateOnDoubleClick: true
            }
            // A single overlay keeps column boundaries continuous while rows scroll.
            Repeater {
                model: Math.max(0, tableControl.columns.length - 1)
                Rectangle {
                    id: tableDivider
                    required property int index
                    objectName: "dialogWidget-" + widget.id + "TableDivider-" + (index + 1)
                    x: tableControl.columnX(index + 1)
                    y: tableHeader.y
                    width: 1 / hostWindow.dpr
                    height: hostWindow.snapPx(Math.max(0, tableControl.height - y))
                    visible: widget.layoutRole !== "overview-table"
                    color: hostWindow.controlBorder
                    z: 3
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(tableDivider, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(tableDivider, hostWindow.contentItem)
                    }
                }
            }
        }
    }

    Component {
        id: textDelegate
        Text {
            id: dialogText
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "Text"
            text: widget.iconName ? "" : (widget.wrapText === true || !widget.hotkey
                  ? String(widget.text || widget.typeName || "")
                  : hostWindow.mnemonicText(widget.text || widget.typeName, widget.hotkey))
            textFormat: widget.wrapText === true || !widget.hotkey ? Text.PlainText : Text.StyledText
            wrapMode: widget.wrapText === true ? Text.Wrap : Text.NoWrap
            color: widget.disabled ? hostWindow.mutedText : hostWindow.textColor
            font: hostWindow.font
            elide: widget.wrapText === true ? Text.ElideNone : Text.ElideRight
            verticalAlignment: Text.AlignVCenter
            HostPixelAlignedImage {
                objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "TextIcon"
                hostWindow: widgetRoot.hostWindow
                anchors.verticalCenter: parent.verticalCenter
                width: hostWindow.snapPx(16)
                height: width
                visible: !!widget.iconName
                source: visible ? hostWindow.lucideIconSource(widget.iconName, 16, dialogText.color) : ""
                sourceSize: Qt.size(width * hostWindow.dpr, height * hostWindow.dpr)
            }
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                       dialogText, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                       dialogText, hostWindow.contentItem)
            }
        }
    }

    Component {
        id: separatorDelegate
        Item {
            id: separatorControl
            readonly property bool hasCaption: String(widget.text || "").length > 0
            readonly property real captionGap: hostWindow.snapPx(8)

            Text {
                id: separatorCaption
                objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "SeparatorText"
                x: hostWindow.snapPx((separatorControl.width - width) / 2)
                y: hostWindow.snapPx((separatorControl.height - height) / 2)
                width: hostWindow.snapPx(Math.min(implicitWidth, separatorControl.width))
                height: Math.ceil(implicitHeight * hostWindow.dpr) / hostWindow.dpr
                visible: separatorControl.hasCaption
                text: String(widget.text || "")
                textFormat: Text.PlainText
                font: hostWindow.font
                color: hostWindow.mutedText
                elide: Text.ElideRight
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(separatorCaption, hostWindow.contentItem)
                    y: hostWindow.dialogPixelOffsetY(separatorCaption, hostWindow.contentItem)
                }
            }

            Repeater {
                model: 2
                Rectangle {
                    id: separatorLine
                    required property int index
                    objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "SeparatorLine-" + index
                    x: index === 0 ? 0
                        : hostWindow.snapPx(separatorCaption.x + separatorCaption.width + separatorControl.captionGap)
                    y: hostWindow.snapPx((separatorControl.height - height) / 2)
                    width: hostWindow.snapPx(Math.max(0, separatorControl.hasCaption
                        ? (index === 0 ? separatorCaption.x - separatorControl.captionGap
                           : separatorControl.width - x)
                        : (index === 0 ? separatorControl.width : 0)))
                    height: 1 / hostWindow.dpr
                    visible: width > 0
                    color: hostWindow.controlBorder
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(separatorLine, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(separatorLine, hostWindow.contentItem)
                    }
                }
            }
        }
    }

    Component {
        id: multilineDelegate
        DialogMultiLineEdit {
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "MultiLine"
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
        }
    }

    Component {
        id: editDelegate
        DialogTextField {
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "Edit"
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
        }
    }

    Component {
        id: buttonDelegate
        DialogButton {
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "Button"
            hostWindow: widgetRoot.hostWindow
            text: hostWindow.cleanText(widget.text)
            toolTipText: hostWindow.cleanText(widget.toolTipText)
            variant: widget.variant || "standard"
            flat: widget.flat === true
            cornerRadius: widget.cornerRadius !== undefined ? widget.cornerRadius : 4
            iconSource: widget.iconName
                ? hostWindow.lucideIconSource(widget.iconName, 14, hostWindow.textColor) : ""
            mnemonicHotkey: hostWindow.cleanText(widget.hotkey)
            enabled: widget.disabled !== true
            semanticFocus: widget.focused === true
            onClicked: widgetRoot.buttonAction()
        }
    }

    Component {
        id: checkboxDelegate
        DialogCheckBox {
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "CheckBox"
            hostWindow: widgetRoot.hostWindow
            font: widgetRoot.groupHeading
                ? Qt.font({family: hostWindow.font.family, pixelSize: hostWindow.font.pixelSize,
                           italic: hostWindow.font.italic, bold: true})
                : hostWindow.font
            text: hostWindow.cleanText(widget.text)
            mnemonicHotkey: hostWindow.cleanText(widget.hotkey)
            checked: widget.state === 1
            tristate: widget.threeState === true
            enabled: widget.disabled !== true
            semanticFocus: widget.focused === true
            onClicked: hostWindow.action({ "target": widget.id, "action": "control.toggle" })
        }
    }

    Component {
        id: progressDelegate
        DialogProgressBar {
            hostWindow: widgetRoot.hostWindow
            from: 0
            to: 100
            value: widget.percent || 0
        }
    }

    Component {
        id: choiceDelegate
        SemanticChoiceGroup {
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
        }
    }

    Component {
        id: listDelegate
        Item {
            id: listControl
            enabled: widget.disabled !== true
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "ListBox"
            property bool semanticFocus: widget.focused === true && widget.disabled !== true
            Rectangle {
                objectName: "dialogWidget-" + widget.id + "ListBackground"
                anchors.fill: parent
                color: hostWindow.controlPressedBg
                radius: hostWindow.snapPx(4)
            }
            ListView {
                id: listView
                objectName: "dialogWidget-" + widget.id + "ListView"
                anchors.fill: parent
                pixelAligned: true
                boundsBehavior: Flickable.StopAtBounds
                currentIndex: Number(widget.cursor || 0)
                onCurrentIndexChanged: positionViewAtIndex(currentIndex, ListView.Contain)
                ScrollBar.vertical: F4ScrollBar {
                    id: listScrollBar
                    objectName: "dialogWidget-" + widget.id + "ListScrollBar"
                    hostWindow: widgetRoot.hostWindow
                    policy: ScrollBar.AlwaysOn
                    visible: listView.contentHeight > listView.height
                }
                clip: true
                model: widget.items || []
                delegate: Rectangle {
                    id: listRow
                    required property var modelData
                    required property int index
                    readonly property string iconName: (widget.itemIcons || [])[index] || ""
                    readonly property var itemState: (widget.itemStates || [])[index] || ({})
                    readonly property var field: (widget.itemFields || [])[index] || ({})
                    readonly property bool hasField: String(field.name || "") !== ""
                    readonly property real fieldWidth: hasField
                        ? hostWindow.snapPx(Math.min(hostWindow.pxW(widget.fieldColumnWidth || 1), width * 0.45)) : 0
                    readonly property bool checkable: itemState.checkable === true
                    readonly property real contentOpacity: itemState.dimmed === true ? 0.75 : 1
                    width: listView.width - (listScrollBar.visible ? listScrollBar.width : 0)
                    height: widget.wrapText === true
                            ? hostWindow.snapPx(Math.max(21, listRowText.contentHeight,
                                                       hasField ? listFieldText.contentHeight : 0) + 12)
                            : hostWindow.snapPx(Math.max(21, hostWindow.ch))
                    radius: 4
                    color: widget.readOnly === true ? "transparent"
                           : index === widget.cursor
                           ? hostWindow.selectedBg
                           : listHover.hovered
                             ? hostWindow.controlHoverBg : "transparent"
                    // Keyboard repeat can outrun a color fade. Show the Go-owned
                    // cursor immediately, without fading the old and new rows.
                    F4CheckBox {
                        objectName: "dialogWidget-" + widget.id + "ListCheck-" + listRow.index
                        hostWindow: widgetRoot.hostWindow
                        x: hostWindow.snapPx(8)
                        y: 0
                        width: hostWindow.snapPx(18)
                        height: listRow.height
                        visible: listRow.checkable
                        checked: listRow.itemState.checked === true
                        opacity: listRow.contentOpacity
                        // The viewport owns pointer gestures and sends intent to Go.
                        // This control only presents the acknowledged row state.
                        enabled: false
                        contentItem: Item {}
                    }
                    HostPixelAlignedImage {
                        objectName: "dialogWidget-" + widget.id + "ListItemIcon-" + listRow.index
                        hostWindow: widgetRoot.hostWindow
                        x: hostWindow.snapPx(listRow.checkable ? 34 : 8)
                        opacity: listRow.contentOpacity
                        y: hostWindow.snapPx((listRow.height - height) / 2)
                        width: hostWindow.snapPx(16)
                        height: width
                        visible: listRow.iconName !== ""
                        source: visible ? hostWindow.lucideIconSource(listRow.iconName, 16, hostWindow.textColor) : ""
                        sourceSize: Qt.size(width * hostWindow.dpr, height * hostWindow.dpr)
                    }
                    Text {
                        id: listFieldText
                        objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                                    + "ListItemFieldText-" + listRow.index
                        visible: listRow.hasField
                        x: hostWindow.snapPx(8)
                        width: listRow.fieldWidth
                        height: listRow.height
                        text: String(listRow.field.name || "")
                        textFormat: Text.PlainText
                        wrapMode: Text.Wrap
                        opacity: 0.75 * listRow.contentOpacity
                        color: hostWindow.textColor
                        font: hostWindow.font
                        verticalAlignment: Text.AlignTop
                        topPadding: hostWindow.snapPx(6)
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(listFieldText, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(listFieldText, hostWindow.contentItem)
                        }
                    }
                    Text {
                        id: listRowText
                        objectName: "dialogWidget-"
                                    + hostWindow.cleanText(widget.id)
                                    + "ListItemText-" + listRow.index
                        anchors.fill: parent
                        anchors.leftMargin: hostWindow.snapPx((listRow.iconName !== "" ? 32 : 8)
                                                               + (listRow.checkable ? 26 : 0)) + listRow.fieldWidth
                        opacity: listRow.contentOpacity
                        anchors.rightMargin: hostWindow.snapPx(8)
                        text: listRow.hasField ? String(listRow.field.value || "")
                              : widget.wrapText === true || listRow.checkable ? String(modelData)
                              : hostWindow.mnemonicText(modelData, "")
                        textFormat: widget.wrapText === true || listRow.checkable ? Text.PlainText : Text.StyledText
                        wrapMode: widget.wrapText === true ? Text.Wrap : Text.NoWrap
                        color: hostWindow.textColor
                        font: hostWindow.font
                        verticalAlignment: listRow.hasField ? Text.AlignTop : Text.AlignVCenter
                        topPadding: listRow.hasField ? hostWindow.snapPx(6) : 0
                        elide: widget.wrapText === true ? Text.ElideNone : Text.ElideRight
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                   listRowText, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                   listRowText, hostWindow.contentItem)
                        }
                    }
                    HoverHandler {
                        id: listHover
                        enabled: widget.readOnly !== true
                    }
                }
            }

            SemanticListPointer {
                objectName: "dialogWidget-" + widget.id + "ListPointer"
                hostWindow: widgetRoot.hostWindow
                widget: widgetRoot.widget
                view: listView
                checkboxLeft: hostWindow.snapPx(8)
                checkboxWidth: hostWindow.snapPx(18)
                width: listView.width - (listScrollBar.visible ? listScrollBar.width : 0)
                height: listView.height
            }

            Rectangle {
                id: listFocusFrame
                objectName: "dialogWidget-"
                            + hostWindow.cleanText(widget.id)
                            + "ListFocusFrame"
                readonly property color testBorderColor: border.color
                readonly property real testBorderWidth: border.width
                x: 0
                y: 0
                width: hostWindow.snapPx(listControl.width)
                height: hostWindow.snapPx(listControl.height)
                z: 2
                color: "transparent"
                radius: hostWindow.snapPx(4)
                border.width: listControl.semanticFocus
                              ? hostWindow.separatorWidth : 0
                border.color: listControl.semanticFocus
                              ? hostWindow.dialogAccent
                              : hostWindow.controlBorder
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                           listFocusFrame, hostWindow.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                           listFocusFrame, hostWindow.contentItem)
                }

                Behavior on border.color {
                    ColorAnimation { duration: 90 }
                }
            }
        }
    }

    Component {
        id: comboDelegate
        DialogComboBox {
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "ComboBox"
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
        }
    }

    Component {
        id: viewportDelegate
        SemanticGroupViewport {
            hostWindow: widgetRoot.hostWindow
            widget: widgetRoot.widget
        }
    }

    Component {
        id: fullPageTableDelegate
        Item {
            id: tablePage
            readonly property var tableWidget: (widgetRoot.widget.children || []).find(child => child.kind === "table") || ({})
            Loader {
                id: pageTable
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.top: parent.top
                anchors.bottom: pageActions.top
                anchors.bottomMargin: hostWindow.snapPx(12)
                Component.onCompleted: setSource(Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {
                    hostWindow: widgetRoot.hostWindow,
                    widget: Qt.binding(() => tablePage.tableWidget),
                    tableKeyboardNavigation: true,
                    x: 0, y: 0,
                    width: Qt.binding(() => pageTable.width),
                    height: Qt.binding(() => pageTable.height)
                })
            }
            Row {
                id: pageActions
                anchors.left: parent.left
                anchors.bottom: parent.bottom
                spacing: hostWindow.snapPx(8)
                height: hostWindow.snapPx(Math.max(32, hostWindow.font.pixelSize + 16))
                Repeater {
                    model: SemanticChildrenModel { widgets: (widgetRoot.widget.children || []).filter(child => child.kind === "button") }
                    delegate: Loader {
                        id: pageAction
                        required property var widgetData
                        required property var labelData
                        width: Math.max(hostWindow.snapPx(80), hostWindow.pxW(widgetData.w || 1))
                        height: pageActions.height
                        Component.onCompleted: setSource(Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {
                            hostWindow: widgetRoot.hostWindow,
                            widget: Qt.binding(() => pageAction.widgetData),
                            x: 0, y: 0,
                            width: Qt.binding(() => pageAction.width),
                            height: Qt.binding(() => pageAction.height)
                        })
                    }
                }
            }
        }
    }

    Component {
        id: groupDelegate
        Item {
            id: groupContainer
            readonly property bool hasHeading: hostWindow.cleanText(widget.title) !== ""
            readonly property real headingHeight: hasHeading
                ? hostWindow.snapPx(widget.headerControl ? 32 : Math.max(22, matrixLabelMetrics.height)) : 0
            readonly property real contentPadding: hostWindow.snapPx(12)
            // Measure native control extents, not the terminal's spare rows:
            // an edit and a checkbox have different visual heights.
            readonly property rect contentBounds: {
                let top = Number.POSITIVE_INFINITY
                let bottom = 0
                for (let i = 0; i < groupChildren.count; ++i) {
                    const loader = groupChildren.itemAt(i)
                    const child = loader ? loader.item : null
                    if (child && child.visible) {
                        top = Math.min(top, child.y)
                        bottom = Math.max(bottom, child.y + child.height)
                    }
                }
                return Qt.rect(0, Number.isFinite(top) ? top : 0, 0,
                               Number.isFinite(top) ? bottom - top : 0)
            }
            Rectangle {
                id: groupSurface
                objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                            + "GroupBorder"
                readonly property color testBorderColor: border.color
                readonly property real testBorderWidth: border.width
                x: 0
                y: groupContainer.hasHeading ? groupContainer.headingHeight + hostWindow.snapPx(4) : 0
                width: parent.width
                height: widget.bordered === true && groupContainer.contentBounds.height > 0
                    ? hostWindow.snapPx(groupContainer.contentBounds.height + 2 * groupContainer.contentPadding)
                    : Math.max(0, parent.height - y)
                color: widget.bordered === true ? Qt.tint(hostWindow.dialogBg, Qt.rgba(1, 1, 1, 0.035)) : "transparent"
                border.width: 0
                border.color: hostWindow.controlBorder
                radius: hostWindow.snapPx(10)
            }

            Rectangle {
                objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "GroupTitleBackground"
                anchors.left: parent.left
                anchors.top: parent.top
                anchors.leftMargin: hostWindow.snapPx(8)
                width: Math.max(0, parent.width - hostWindow.snapPx(16))
                height: groupContainer.headingHeight
                color: "transparent"
                visible: groupContainer.hasHeading

                Text {
                    id: groupTitle
                    objectName: "dialogWidget-"
                                + hostWindow.cleanText(widget.id)
                                + "GroupTitle"
                    anchors.left: parent.left
                    anchors.verticalCenter: parent.verticalCenter
                    width: parent.width
                    elide: Text.ElideRight
                    text: widget.hotkey ? hostWindow.mnemonicText(widget.title, widget.hotkey) : String(widget.title || "")
                    textFormat: widget.hotkey ? Text.StyledText : Text.PlainText
                    color: widget.dimmed === true ? hostWindow.mutedText : hostWindow.textColor
                    font.family: hostWindow.font.family
                    font.pixelSize: hostWindow.font.pixelSize
                    font.italic: hostWindow.font.italic
                    font.bold: true
                    visible: !widget.headerControl
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               groupTitle, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               groupTitle, hostWindow.contentItem)
                    }
                }
                Loader {
                    id: headerToggle
                    anchors.fill: parent
                    active: !!widgetRoot.widget.headerControl
                    function loadHeader() { if (active && source.toString() === "") setSource(Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {
                        hostWindow: widgetRoot.hostWindow, groupHeading: true,
                        widget: Qt.binding(() => widgetRoot.widget.headerControl || ({}))
                    }) }
                    Component.onCompleted: loadHeader()
                    onActiveChanged: loadHeader()
                    onLoaded: { item.x = 0; item.y = 0; item.width = Qt.binding(() => headerToggle.width); item.height = Qt.binding(() => headerToggle.height) }
                }
            }

            Item {
                width: parent.width
                y: widget.bordered === true && groupContainer.contentBounds.height > 0
                    ? groupSurface.y + groupContainer.contentPadding - groupContainer.contentBounds.y : 0
                Repeater {
                    id: groupChildren
                    model: SemanticChildrenModel { widgets: (widgetRoot.widget.children || []).filter(child => !widgetRoot.widget.headerControl || child.id !== widgetRoot.widget.headerControl.id) }
                    delegate: Loader {
                        id: childLoader
                        required property var widgetData
                        required property var labelData

                        readonly property real leftInset: item ? item.x
                            : hostWindow.pxX(Number(widgetData.x || 0) - Number(widgetRoot.widget.x || 0))
                        readonly property real availableWidth: Math.max(1, widgetRoot.width - leftInset
                            - (widgetRoot.widget.bordered === true ? hostWindow.pxW(2) : 0))
                        width: widgetData.fillWidth === true || widgetData.colorMatrixCell === true ? availableWidth
                            : Math.min(hostWindow.pxW(widgetData.w || 1), availableWidth)

                        // URL-backed loading keeps this bounded hierarchy dynamic
                        // instead of embedding a recursive QML component tree.
                        Component.onCompleted: setSource(
                            Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {
                                "hostWindow": widgetRoot.hostWindow,
                                "dialogLayout": widgetRoot.dialogLayout,
                                "labelData": Qt.binding(() => childLoader.labelData),
                                "widget": Qt.binding(() => childLoader.widgetData),
                                // Coordinates remain absolute in the wire frame.
                                "originX": Qt.binding(() => Number(widgetRoot.widget.x || 0)),
                                "originY": Qt.binding(() => Number(widgetRoot.widget.y || 0) - 1),
                                "maximumWidth": Qt.binding(() => childLoader.availableWidth)
                            })
                    }
                }
            }
        }
    }
}
