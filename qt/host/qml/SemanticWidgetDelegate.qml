pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

Item {
    id: widgetRoot
    required property ApplicationWindow hostWindow
    property var widget: ({})
    property var dialogLayout: null
    property var siblingWidgets: []
    readonly property var inlineLabel: {
        if (widget.kind !== "edit" && widget.kind !== "comboBox")
            return null
        let previous = null
        for (const sibling of siblingWidgets) {
            if (!sibling || sibling.visible === false || sibling.y !== widget.y
                    || Number(sibling.x) >= Number(widget.x))
                continue
            if (!previous || Number(sibling.x) > Number(previous.x))
                previous = sibling
        }
        if (!previous || previous.kind !== "text" || Number(previous.h || 1) !== 1)
            return null
        const gap = Number(widget.x) - Number(previous.x) - Number(previous.w || 1)
        return gap >= 0 && gap <= 3 ? previous : null
    }
    // Use the same styled text and font as the visible label, including mnemonics.
    Text {
        id: inlineLabelMeasure
        visible: false
        width: widgetRoot.inlineLabel ? hostWindow.pxW(widgetRoot.inlineLabel.w || 1) : 0
        elide: Text.ElideRight
        text: widgetRoot.inlineLabel
              ? hostWindow.mnemonicText(widgetRoot.inlineLabel.text, widgetRoot.inlineLabel.hotkey) : ""
        textFormat: Text.StyledText
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
    width: widget.kind === "text" && widget.wrapText === true
           ? maximumWidth : Math.min(hostWindow.pxW(widget.w || 1), maximumWidth)
    height: visualHeight
    visible: widget.visible !== false
    clip: false

    // Correct the whole control subtree in scene space so backgrounds,
    // hit regions, text, and icons share the same physical-pixel origin.
    transform: Translate {
        x: widgetRoot.visuallyOverflows
           ? hostWindow.dialogPixelOffsetX(
                 widgetRoot, hostWindow.contentItem) : 0
        y: widgetRoot.visuallyOverflows
           ? hostWindow.dialogPixelOffsetY(
                 widgetRoot, hostWindow.contentItem) : 0
    }

    Loader {
        anchors.fill: parent
        sourceComponent: {
            switch (widget.kind) {
            case "button": return buttonDelegate
            case "checkbox": return checkboxDelegate
            case "edit": return editDelegate
            case "text": return textDelegate
            case "progressBar": return progressDelegate
            case "radioGroup": return choiceDelegate
            case "checkGroup": return choiceDelegate
            case "listBox": return listDelegate
            case "table": return tableDelegate
            case "comboBox": return comboDelegate
            case "group": return groupDelegate
            default: return textDelegate
            }
        }
    }

    Component {
        id: tableDelegate
        Item {
            id: tableControl
            objectName: "dialogWidget-" + widget.id + "Table"
            readonly property var columns: widget.columns || []
            readonly property real rowHeight: hostWindow.snapPx(hostWindow.ch + 8)
            function columnWidth(index) {
                let weight = 0
                for (const col of columns) weight += Math.max(8, Number(col.width) > 0 ? Number(col.width) : Number(col.minWidth || 24))
                return hostWindow.snapPx(width * Math.max(8, Number(columns[index].width) > 0 ? Number(columns[index].width) : Number(columns[index].minWidth || 24)) / Math.max(1, weight))
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
                ScrollBar.vertical: ScrollBar {}
                delegate: Rectangle {
                    id: tableRow
                    required property var modelData
                    required property int index
                    width: tableRows.width
                    height: tableControl.rowHeight
                    color: index === tableRows.currentIndex ? hostWindow.selectedBg : "transparent"
                    Repeater {
                        model: tableControl.columns
                        Text {
                            id: cellText
                            required property int index
                            objectName: "dialogWidget-" + widget.id + "TableCell-" + tableRow.index + "-" + index
                            x: tableControl.columnX(index) + hostWindow.snapPx(6)
                            width: Math.max(0, tableControl.columnWidth(index) - hostWindow.snapPx(12))
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
                    MouseArea {
                        anchors.fill: parent
                        onClicked: hostWindow.action({target: widget.id, action: "control.select", index: tableRow.index})
                        onDoubleClicked: hostWindow.action({target: widget.id, action: "control.activate", index: tableRow.index})
                    }
                }
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
            text: widget.wrapText === true ? String(widget.text || "")
                  : hostWindow.mnemonicText(widget.text || widget.typeName, widget.hotkey)
            textFormat: widget.wrapText === true ? Text.PlainText : Text.StyledText
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
        Column {
            anchors.fill: parent
            spacing: 0

            Repeater {
                model: widget.items || []
                delegate: DialogRadioButton {
                    required property var modelData
                    required property int index
                    hostWindow: widgetRoot.hostWindow
                    objectName: "dialogWidget-"
                                + hostWindow.cleanText(widget.id)
                                + "Radio-" + index
                    width: parent.width
                    height: widgetRoot.height
                            / Math.max(1, (widget.items || []).length)
                    text: hostWindow.cleanText(modelData)
                    checked: widget.kind === "radioGroup" ? index === widget.selected : !!(widget.states && widget.states[index])
                    semanticFocus: widget.focused === true
                                   && index === (widget.focusIndex !== undefined
                                                 ? widget.focusIndex
                                                 : (widget.selected !== undefined ? widget.selected : 0))
                    onClicked: hostWindow.action({ "target": widget.id, "action": "control.select", "index": index })
                }
            }
        }
    }

    Component {
        id: listDelegate
        Item {
            id: listControl
            objectName: "dialogWidget-" + hostWindow.cleanText(widget.id)
                        + "ListBox"
            property bool semanticFocus: widget.focused === true

            ListView {
                anchors.fill: parent
                clip: true
                model: widget.items || []
                delegate: Rectangle {
                    id: listRow
                    required property var modelData
                    required property int index
                    width: ListView.view.width
                    height: widget.wrapText === true
                            ? hostWindow.snapPx(Math.max(21, listRowText.contentHeight) + 12)
                            : Math.max(21, hostWindow.ch)
                    radius: 4
                    color: widget.readOnly === true ? "transparent"
                           : index === widget.cursor
                           ? hostWindow.selectedBg
                           : listMouse.containsMouse
                             ? hostWindow.controlHoverBg : "transparent"
                    Behavior on color { ColorAnimation { duration: 70 } }
                    Text {
                        id: listRowText
                        objectName: "dialogWidget-"
                                    + hostWindow.cleanText(widget.id)
                                    + "ListItemText-" + listRow.index
                        anchors.fill: parent
                        anchors.leftMargin: 8
                        anchors.rightMargin: 8
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
                    MouseArea {
                        id: listMouse
                        enabled: widget.readOnly !== true
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: hostWindow.action({
                            "target": widget.id,
                            "action": "control.select",
                            "index": index
                        })
                    }
                }
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
                anchors.left: parent.left
                anchors.top: parent.top
                anchors.leftMargin: 8
                anchors.topMargin: -9
                width: groupTitle.implicitWidth + 12
                height: 18
                radius: 3
                color: hostWindow.dialogBg
                visible: hostWindow.cleanText(widget.title) !== ""

                Text {
                    id: groupTitle
                    objectName: "dialogWidget-"
                                + hostWindow.cleanText(widget.id)
                                + "GroupTitle"
                    anchors.centerIn: parent
                    text: hostWindow.mnemonicText(widget.title, widget.hotkey)
                    textFormat: Text.StyledText
                    color: hostWindow.mutedText
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
                model: widget.children || []
                delegate: Loader {
                    required property var modelData

                    // A URL-backed loader keeps recursion dynamic. QML rejects
                    // a component that names itself directly in its compiled
                    // object tree, while the semantic dialog hierarchy is
                    // intentionally recursive and bounded by the wire model.
                    Component.onCompleted: setSource(
                        Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {
                            "hostWindow": widgetRoot.hostWindow,
                            "dialogLayout": widgetRoot.dialogLayout,
                            "siblingWidgets": widgetRoot.widget.children || [],
                            "widget": modelData,
                            // Nested semantic coordinates remain absolute in
                            // the frame. Rebase them to this group; the -1
                            // cancels the delegate's own legacy row offset.
                            "originX": Number(widgetRoot.widget.x || 0),
                            "originY": Number(widgetRoot.widget.y || 0) - 1
                        })
                }
            }
        }
    }
}
