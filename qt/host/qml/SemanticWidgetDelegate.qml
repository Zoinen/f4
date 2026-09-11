pragma ComponentBehavior: Bound

import QtQuick
import F4QtHost 1.0
import QtQuick.Controls
import QtQuick.Controls.Basic as T

Item {
    id: widgetRoot
    required property ApplicationWindow hostWindow
    property var widget: ({})
    property var dialogLayout: null
    property var labelData: ({})
    readonly property var inlineLabel: labelData && labelData.kind === "text" ? labelData : null
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
                            + inlineLabelMeasure.contentWidth + 12)
        : hostWindow.pxX((widget.x || 0) - originX)
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
    width: Math.min(hostWindow.pxW(widget.w || 1), maximumWidth)
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
        sourceComponent: {
            switch (widget.kind) {
            case "button": return buttonDelegate
            case "checkbox": return checkboxDelegate
            case "edit": return editDelegate
            case "multiLineEdit": return multilineDelegate
            case "text": return textDelegate
            case "progressBar": return progressDelegate
            case "radioGroup": return choiceDelegate
            case "checkGroup": return choiceDelegate
            case "listBox": return listDelegate
            case "table": return tableDelegate
            case "comboBox": return comboDelegate
            case "group": return widget.scrollable === true ? viewportDelegate : groupDelegate
            default: return textDelegate
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
            readonly property real rowHeight: hostWindow.snapPx(hostWindow.ch + 8)
            readonly property real availableWidth: width - (tableScrollBar.visible ? tableScrollBar.width : 0)
            Rectangle {
                objectName: "dialogWidget-" + widget.id + "TableBackground"
                anchors.fill: parent
                color: hostWindow.controlPressedBg
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
                visible: widget.quickSearch === true
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
                        MouseArea { anchors.fill: parent; onClicked: hostWindow.action({target: widget.id, action: "control.sort", index: headerText.index}) }
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
                model: widget.rows || []
                currentIndex: Number(widget.cursor || 0)
                onCurrentIndexChanged: positionViewAtIndex(currentIndex, ListView.Contain)
                onModelChanged: Qt.callLater(function() { positionViewAtIndex(currentIndex, ListView.Contain) })
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
                    radius: hostWindow.snapPx(4)
                    required property var modelData
                    required property int index
                    width: tableControl.availableWidth
                    height: tableControl.rowHeight
                    color: index === tableRows.currentIndex ? hostWindow.selectedBg : "transparent"
                    readonly property string iconName: (widget.itemIcons || [])[index] || ""
                    readonly property real iconSpace: iconName !== "" ? hostWindow.snapPx(24) : 0
                    HostPixelAlignedImage {
                        objectName: "dialogWidget-" + widget.id + "TableItemIcon-" + tableRow.index
                        hostWindow: widgetRoot.hostWindow
                        x: hostWindow.snapPx(6)
                        y: hostWindow.snapPx((tableRow.height - height) / 2)
                        width: hostWindow.snapPx(16)
                        height: width
                        visible: tableRow.iconName !== ""
                        source: visible ? hostWindow.lucideIconSource(tableRow.iconName, 16, hostWindow.textColor) : ""
                        sourceSize: Qt.size(width * hostWindow.dpr, height * hostWindow.dpr)
                    }
                    Repeater {
                        model: tableControl.columns
                        Text {
                            id: cellText
                            required property int index
                            objectName: "dialogWidget-" + widget.id + "TableCell-" + tableRow.index + "-" + index
                            readonly property real iconSpace: index === 0 ? tableRow.iconSpace : 0
                            x: tableControl.columnX(index) + hostWindow.snapPx(6) + iconSpace
                            width: Math.max(0, tableControl.columnWidth(index) - hostWindow.snapPx(12) - iconSpace)
                            height: tableRow.height
                            text: tableRow.modelData.cells[index] || ""
                            font: hostWindow.font
                            color: hostWindow.textColor
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
            text: widget.wrapText === true || !widget.hotkey ? String(widget.text || widget.typeName || "")
                  : hostWindow.mnemonicText(widget.text || widget.typeName, widget.hotkey)
            textFormat: widget.wrapText === true || !widget.hotkey ? Text.PlainText : Text.StyledText
            wrapMode: widget.wrapText === true ? Text.Wrap : Text.NoWrap
            color: widget.disabled ? hostWindow.mutedText : hostWindow.textColor
            font: hostWindow.font
            elide: widget.wrapText === true ? Text.ElideNone : Text.ElideRight
            verticalAlignment: Text.AlignVCenter
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                       dialogText, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                       dialogText, hostWindow.contentItem)
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
            mnemonicHotkey: hostWindow.cleanText(widget.hotkey)
            enabled: widget.disabled !== true
            semanticFocus: widget.focused === true
            onClicked: hostWindow.action({ "target": widget.id, "action": "control.activate" })
        }
    }

    Component {
        id: checkboxDelegate
        DialogCheckBox {
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "CheckBox"
            hostWindow: widgetRoot.hostWindow
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
                    width: listView.width - (listScrollBar.visible ? listScrollBar.width : 0)
                    height: widget.wrapText === true
                            ? hostWindow.snapPx(Math.max(21, listRowText.contentHeight) + 12)
                            : hostWindow.snapPx(Math.max(21, hostWindow.ch))
                    radius: 4
                    color: widget.readOnly === true ? "transparent"
                           : index === widget.cursor
                           ? hostWindow.selectedBg
                           : listHover.hovered
                             ? hostWindow.controlHoverBg : "transparent"
                    Behavior on color { ColorAnimation { duration: 70 } }
                    HostPixelAlignedImage {
                        objectName: "dialogWidget-" + widget.id + "ListItemIcon-" + listRow.index
                        hostWindow: widgetRoot.hostWindow
                        x: hostWindow.snapPx(8)
                        y: hostWindow.snapPx((listRow.height - height) / 2)
                        width: hostWindow.snapPx(16)
                        height: width
                        visible: listRow.iconName !== ""
                        source: visible ? hostWindow.lucideIconSource(listRow.iconName, 16, hostWindow.textColor) : ""
                        sourceSize: Qt.size(width * hostWindow.dpr, height * hostWindow.dpr)
                    }
                    Text {
                        id: listRowText
                        objectName: "dialogWidget-"
                                    + hostWindow.cleanText(widget.id)
                                    + "ListItemText-" + listRow.index
                        anchors.fill: parent
                        anchors.leftMargin: hostWindow.snapPx(listRow.iconName !== "" ? 32 : 8)
                        anchors.rightMargin: hostWindow.snapPx(8)
                        text: widget.wrapText === true ? String(modelData)
                              : hostWindow.mnemonicText(modelData, "")
                        textFormat: widget.wrapText === true ? Text.PlainText : Text.StyledText
                        wrapMode: widget.wrapText === true ? Text.Wrap : Text.NoWrap
                        color: hostWindow.textColor
                        font: hostWindow.font
                        verticalAlignment: Text.AlignVCenter
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
        id: groupDelegate
        Item {
            Rectangle {
                objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                            + "GroupBorder"
                readonly property color testBorderColor: border.color
                readonly property real testBorderWidth: border.width
                anchors.fill: parent
                color: "transparent"
                border.width: widget.bordered === true ? 1 : 0
                border.color: hostWindow.controlBorder
                radius: 5
            }

            Rectangle {
                objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "GroupTitleBackground"
                anchors.left: parent.left
                anchors.top: parent.top
                anchors.leftMargin: hostWindow.snapPx(8)
                anchors.topMargin: -hostWindow.snapPx(9)
                width: hostWindow.snapPx(groupTitle.implicitWidth + 12)
                height: hostWindow.snapPx(18)
                radius: hostWindow.snapPx(3)
                color: hostWindow.dialogBg
                visible: hostWindow.cleanText(widget.title) !== ""

                Text {
                    id: groupTitle
                    objectName: "dialogWidget-"
                                + hostWindow.cleanText(widget.id)
                                + "GroupTitle"
                    anchors.centerIn: parent
                    text: widget.hotkey ? hostWindow.mnemonicText(widget.title, widget.hotkey) : String(widget.title || "")
                    textFormat: widget.hotkey ? Text.StyledText : Text.PlainText
                    color: widget.dimmed === true ? hostWindow.controlBorder : hostWindow.mutedText
                    font: hostWindow.font
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               groupTitle, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               groupTitle, hostWindow.contentItem)
                    }
                }
            }

            Repeater {
                model: SemanticChildrenModel { widgets: widgetRoot.widget.children || [] }
                delegate: Loader {
                    id: childLoader
                    required property var widgetData
                    required property var labelData

                    readonly property real leftInset: item ? item.x
                        : hostWindow.pxX(Number(widgetData.x || 0) - Number(widgetRoot.widget.x || 0))
                    readonly property real availableWidth: Math.max(1, widgetRoot.width - leftInset
                        - (widgetRoot.widget.bordered === true ? hostWindow.pxW(2) : 0))
                    width: widgetData.fillWidth === true ? availableWidth
                        : Math.min(hostWindow.pxW(widgetData.w || 1), availableWidth)

                    // A URL-backed loader keeps recursion dynamic. QML rejects
                    // a component that names itself directly in its compiled
                    // object tree, while the semantic dialog hierarchy is
                    // intentionally recursive and bounded by the wire model.
                    Component.onCompleted: setSource(
                        Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {
                            "hostWindow": widgetRoot.hostWindow,
                            "dialogLayout": widgetRoot.dialogLayout,
                            "labelData": Qt.binding(() => childLoader.labelData),
                            "widget": Qt.binding(() => childLoader.widgetData),
                            // Nested semantic coordinates remain absolute in
                            // the frame. Rebase them to this group; the -1
                            // cancels the delegate's own legacy row offset.
                            "originX": Qt.binding(() => Number(widgetRoot.widget.x || 0)),
                            "originY": Qt.binding(() => Number(widgetRoot.widget.y || 0) - 1),
                            "maximumWidth": Qt.binding(() => childLoader.availableWidth)
                        })
                }
            }
        }
    }
}
