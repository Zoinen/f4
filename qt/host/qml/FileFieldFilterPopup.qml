pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Popup {
    id: fileFieldFilterPopup
    required property ApplicationWindow hostWindow
    required property Item panelItem
    required property Item panelHeader
    required property var panel
    property var fileFieldFilterDraft: []
    property bool fileFieldFilterDraftAny: false

    function fileFieldDescriptor(fieldId) {
        const fields = panel.fileFieldDescriptors || []
        for (let index = 0; index < fields.length; ++index) {
            if (String(fields[index].id || "") === String(fieldId || ""))
                return fields[index]
        }
        return null
    }
    function openFileFieldFilter() {
        fileFieldFilterDraft = (panel.fileFieldFilters || []).map(function(filter) {
            return { fieldId: String(filter.fieldId || "exif.iso"),
                     operation: String(filter.operation || "has"),
                     value: String(filter.value || "") }
        })
        fileFieldFilterDraftAny = panel.fileFieldFilterAny === true
        fileFieldFilterPopup.open()
    }

    function mutateFileFieldFilter(index, propertyName, value) {
        const draft = fileFieldFilterDraft.slice()
        if (index < 0 || index >= draft.length)
            return
        const row = Object.assign({}, draft[index])
        row[propertyName] = value
        draft[index] = row
        fileFieldFilterDraft = draft
    }

    function cycleFileFieldFilterField(index) {
        const fields = panel.fileFieldDescriptors || []
        if (fields.length === 0)
            return
        const current = String(fileFieldFilterDraft[index].fieldId || "")
        let next = 0
        for (let candidate = 0; candidate < fields.length; ++candidate) {
            if (String(fields[candidate].id) === current) {
                next = (candidate + 1) % fields.length
                break
            }
        }
        mutateFileFieldFilter(index, "fieldId", String(fields[next].id))
        let defaultOperation = "has"
        const operations = fields[next].operations || []
        if (operations.indexOf("has") < 0 && operations.length > 0)
            defaultOperation = String(operations[0])
        mutateFileFieldFilter(index, "operation", defaultOperation)
        mutateFileFieldFilter(index, "value", "")
    }

    function cycleFileFieldFilterOperation(index) {
        const descriptor = fileFieldDescriptor(fileFieldFilterDraft[index].fieldId)
        if (!descriptor || !descriptor.operations || descriptor.operations.length === 0)
            return
        const operations = descriptor.operations
        const current = String(fileFieldFilterDraft[index].operation || "")
        let next = 0
        for (let candidate = 0; candidate < operations.length; ++candidate) {
            if (String(operations[candidate]) === current) {
                next = (candidate + 1) % operations.length
                break
            }
        }
        mutateFileFieldFilter(index, "operation", String(operations[next]))
        if (operations[next] === "has" || operations[next] === "missing")
            mutateFileFieldFilter(index, "value", "")
    }

    function fileFieldFilterFieldLabel(fieldId) {
        const descriptor = fileFieldDescriptor(fieldId)
        return descriptor ? String(descriptor.title || fieldId) : String(fieldId)
    }

    function fileFieldFilterOperationLabel(operation) {
        const labels = { eq: "=", ne: "≠", lt: "<", le: "≤", gt: ">",
                         ge: "≥", contains: "contains", has: "has value",
                         missing: "is missing" }
        return labels[String(operation)] || String(operation)
    }

    function addFileFieldFilter() {
        const draft = fileFieldFilterDraft.slice()
        draft.push({ fieldId: "exif.iso", operation: "has", value: "" })
        fileFieldFilterDraft = draft
    }

    function removeFileFieldFilter(index) {
        const draft = fileFieldFilterDraft.slice()
        draft.splice(index, 1)
        fileFieldFilterDraft = draft
    }

    function applyFileFieldFilters() {
        hostWindow.action({
            action: "panel.fileFields.filter", side: panel.side,
            filters: fileFieldFilterDraft, any: fileFieldFilterDraftAny
        })
        fileFieldFilterPopup.close()
    }

        objectName: "fileFieldFilterPopup-" + Number(panel.side || 0)
        parent: Overlay.overlay
        width: hostWindow.snapPx(590)
        height: hostWindow.snapPx(390)
        padding: hostWindow.snapPx(10)
        modal: false
        dim: false
        z: 1002
        focus: false
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutside
                     | Popup.CloseOnPressOutsideParent
        onAboutToShow: {
            const point = panelItem.mapToItem(
                            hostWindow.contentItem,
                            hostWindow.snapPx(Math.max(0, panelItem.width - width - 8)),
                            hostWindow.snapPx(panelHeader.height + 4))
            x = hostWindow.snapPx(Math.max(6, Math.min(
                hostWindow.width - width - 6, point.x)))
            y = hostWindow.snapPx(Math.max(6, Math.min(
                hostWindow.height - height - 6, point.y)))
        }
        background: Rectangle {
            color: hostWindow.controlBg
            radius: hostWindow.snapPx(8)
            border.width: hostWindow.snapPx(1)
            border.color: hostWindow.controlBorder
        }
        contentItem: Column {
            spacing: hostWindow.snapPx(6)

            Row {
                width: fileFieldFilterPopup.availableWidth
                height: hostWindow.snapPx(28)
                spacing: hostWindow.snapPx(10)
                Text {
                    id: fileFieldFilterTitle
                    objectName: "fileFieldFilterTitle-" + Number(panel.side || 0)
                    width: hostWindow.snapPx(210)
                    height: parent.height
                    text: "Filter by file fields"
                    color: hostWindow.textColor
                    font.pixelSize: 14
                    font.weight: Font.DemiBold
                    verticalAlignment: Text.AlignVCenter
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               fileFieldFilterTitle, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               fileFieldFilterTitle, hostWindow.contentItem)
                    }
                }
                Text {
                    id: fileFieldFilterCombine
                    objectName: "fileFieldFilterCombine-" + Number(panel.side || 0)
                    width: hostWindow.snapPx(116)
                    height: parent.height
                    text: fileFieldFilterDraftAny ? "Match any" : "Match all"
                    color: hostWindow.dialogAccent
                    font.pixelSize: 12
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               fileFieldFilterCombine, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               fileFieldFilterCombine, hostWindow.contentItem)
                    }
                    MouseArea {
                        anchors.fill: parent
                        onClicked: fileFieldFilterDraftAny = !fileFieldFilterDraftAny
                    }
                }
                Text {
                    id: fileFieldFilterPending
                    objectName: "fileFieldFilterPending-" + Number(panel.side || 0)
                    width: hostWindow.snapPx(parent.width - 346)
                    height: parent.height
                    text: Number(panel.fileFieldPendingCount || 0) > 0
                          ? "Checking " + Number(panel.fileFieldPendingCount)
                            + " files…" : ""
                    color: hostWindow.mutedText
                    font.pixelSize: 11
                    horizontalAlignment: Text.AlignRight
                    verticalAlignment: Text.AlignVCenter
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               fileFieldFilterPending, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               fileFieldFilterPending, hostWindow.contentItem)
                    }
                }
            }

            Flickable {
                id: fileFieldFilterScroll
                objectName: "fileFieldFilterScroll-" + Number(panel.side || 0)
                width: fileFieldFilterPopup.availableWidth
                height: hostWindow.snapPx(274)
                clip: true
                contentWidth: width
                contentHeight: fileFieldFilterColumn.implicitHeight
                boundsBehavior: Flickable.StopAtBounds
                Column {
                    id: fileFieldFilterColumn
                    width: fileFieldFilterScroll.width
                    spacing: hostWindow.snapPx(5)
                    Repeater {
                        id: fileFieldFilterRepeater
                        model: fileFieldFilterPopup.fileFieldFilterDraft
                        delegate: Rectangle {
                            id: fileFieldFilterRow
                            required property int index
                            required property var modelData
                            width: fileFieldFilterColumn.width
                            height: hostWindow.snapPx(42)
                            radius: 4
                            color: "transparent"

                            Rectangle {
                                id: filterFieldButton
                                objectName: "fileFieldFilterField-"
                                            + fileFieldFilterRow.index
                                            + "-" + Number(panel.side || 0)
                                x: 0
                                y: hostWindow.snapPx((parent.height - height) / 2)
                                width: hostWindow.snapPx(158)
                                height: hostWindow.snapPx(34)
                                radius: 4
                                color: filterFieldPointer.containsMouse
                                       ? hostWindow.controlHoverBg
                                       : hostWindow.panelPathBg
                                Text {
                                    id: filterFieldLabel
                                    objectName: "fileFieldFilterFieldLabel-"
                                                + fileFieldFilterRow.index
                                                + "-" + Number(panel.side || 0)
                                    anchors.fill: parent
                                    anchors.leftMargin: hostWindow.snapPx(8)
                                    anchors.rightMargin: hostWindow.snapPx(8)
                                    text: fileFieldFilterPopup.fileFieldFilterFieldLabel(
                                              String(fileFieldFilterRow.modelData.fieldId))
                                    color: hostWindow.textColor
                                    font.pixelSize: 11
                                    verticalAlignment: Text.AlignVCenter
                                    elide: Text.ElideRight
                                    transform: Translate {
                                        x: hostWindow.dialogPixelOffsetX(
                                               filterFieldLabel,
                                               hostWindow.contentItem)
                                        y: hostWindow.dialogPixelOffsetY(
                                               filterFieldLabel,
                                               hostWindow.contentItem)
                                    }
                                }
                                MouseArea {
                                    id: filterFieldPointer
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    onClicked: fileFieldFilterPopup.cycleFileFieldFilterField(
                                                   fileFieldFilterRow.index)
                                }
                            }

                            Rectangle {
                                id: filterOperationButton
                                objectName: "fileFieldFilterOperation-"
                                            + fileFieldFilterRow.index
                                            + "-" + Number(panel.side || 0)
                                x: hostWindow.snapPx(166)
                                y: hostWindow.snapPx((parent.height - height) / 2)
                                width: hostWindow.snapPx(116)
                                height: hostWindow.snapPx(34)
                                radius: 4
                                color: filterOperationPointer.containsMouse
                                       ? hostWindow.controlHoverBg
                                       : hostWindow.panelPathBg
                                Text {
                                    id: filterOperationLabel
                                    objectName: "fileFieldFilterOperationLabel-"
                                                + fileFieldFilterRow.index
                                                + "-" + Number(panel.side || 0)
                                    anchors.fill: parent
                                    anchors.leftMargin: hostWindow.snapPx(6)
                                    anchors.rightMargin: hostWindow.snapPx(6)
                                    text: fileFieldFilterPopup.fileFieldFilterOperationLabel(
                                              String(fileFieldFilterRow.modelData.operation))
                                    color: hostWindow.textColor
                                    font.pixelSize: 11
                                    horizontalAlignment: Text.AlignHCenter
                                    verticalAlignment: Text.AlignVCenter
                                    elide: Text.ElideRight
                                    transform: Translate {
                                        x: hostWindow.dialogPixelOffsetX(
                                               filterOperationLabel,
                                               hostWindow.contentItem)
                                        y: hostWindow.dialogPixelOffsetY(
                                               filterOperationLabel,
                                               hostWindow.contentItem)
                                    }
                                }
                                MouseArea {
                                    id: filterOperationPointer
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    onClicked: fileFieldFilterPopup.cycleFileFieldFilterOperation(
                                                   fileFieldFilterRow.index)
                                }
                            }

                            Rectangle {
                                x: hostWindow.snapPx(290)
                                y: hostWindow.snapPx((parent.height - height) / 2)
                                width: hostWindow.snapPx(parent.width - 330)
                                height: hostWindow.snapPx(34)
                                radius: 4
                                color: hostWindow.panelPathBg
                                border.width: 1
                                border.color: hostWindow.controlBorder
                                TextInput {
                                    id: filterValueInput
                                    objectName: "fileFieldFilterValue-"
                                                + fileFieldFilterRow.index
                                                + "-" + Number(panel.side || 0)
                                    anchors.fill: parent
                                    anchors.leftMargin: hostWindow.snapPx(8)
                                    anchors.rightMargin: hostWindow.snapPx(8)
                                    enabled: fileFieldFilterRow.modelData.operation
                                             !== "has"
                                             && fileFieldFilterRow.modelData.operation
                                                !== "missing"
                                    text: String(fileFieldFilterRow.modelData.value || "")
                                    color: hostWindow.textColor
                                    selectionColor: hostWindow.dialogAccent
                                    selectedTextColor: hostWindow.textColor
                                    font.pixelSize: 12
                                    verticalAlignment: TextInput.AlignVCenter
                                    clip: true
                                    transform: Translate {
                                        x: hostWindow.dialogPixelOffsetX(
                                               filterValueInput,
                                               hostWindow.contentItem)
                                        y: hostWindow.dialogPixelOffsetY(
                                               filterValueInput,
                                               hostWindow.contentItem)
                                    }
                                    onTextEdited: fileFieldFilterPopup.mutateFileFieldFilter(
                                                      fileFieldFilterRow.index,
                                                      "value", text)
                                }
                            }

                            Rectangle {
                                id: removeFilterButton
                                objectName: "fileFieldFilterRemove-"
                                            + fileFieldFilterRow.index
                                            + "-" + Number(panel.side || 0)
                                x: hostWindow.snapPx(parent.width - 32)
                                y: hostWindow.snapPx((parent.height - height) / 2)
                                width: hostWindow.snapPx(28)
                                height: hostWindow.snapPx(28)
                                radius: 4
                                color: removeFilterPointer.containsMouse
                                       ? hostWindow.controlHoverBg : "transparent"
                                Text {
                                    id: removeFilterLabel
                                    objectName: "fileFieldFilterRemoveLabel-"
                                                + fileFieldFilterRow.index
                                                + "-" + Number(panel.side || 0)
                                    anchors.fill: parent
                                    text: "×"
                                    color: hostWindow.mutedText
                                    font.pixelSize: 16
                                    horizontalAlignment: Text.AlignHCenter
                                    verticalAlignment: Text.AlignVCenter
                                    transform: Translate {
                                        x: hostWindow.dialogPixelOffsetX(
                                               removeFilterLabel,
                                               hostWindow.contentItem)
                                        y: hostWindow.dialogPixelOffsetY(
                                               removeFilterLabel,
                                               hostWindow.contentItem)
                                    }
                                }
                                MouseArea {
                                    id: removeFilterPointer
                                    anchors.fill: parent
                                    hoverEnabled: true
                                    onClicked: fileFieldFilterPopup.removeFileFieldFilter(
                                                   fileFieldFilterRow.index)
                                }
                            }
                        }
                    }
                }
            }

            Row {
                width: fileFieldFilterPopup.availableWidth
                height: hostWindow.snapPx(30)
                spacing: hostWindow.snapPx(8)
                Rectangle {
                    width: hostWindow.snapPx(148)
                    height: hostWindow.snapPx(28)
                    radius: 4
                    color: addFilterPointer.containsMouse
                           ? hostWindow.controlHoverBg : "transparent"
                    Text {
                        id: addFilterText
                        objectName: "fileFieldFilterAddText-"
                                    + Number(panel.side || 0)
                        anchors.fill: parent
                        text: "+ Add condition"
                        color: hostWindow.dialogAccent
                        font.pixelSize: 12
                        verticalAlignment: Text.AlignVCenter
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                   addFilterText, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                   addFilterText, hostWindow.contentItem)
                        }
                    }
                    MouseArea {
                        id: addFilterPointer
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: fileFieldFilterPopup.addFileFieldFilter()
                    }
                }
                Item {
                    width: hostWindow.snapPx(Math.max(0, parent.width - 336))
                    height: hostWindow.snapPx(1)
                }
                Rectangle {
                    width: hostWindow.snapPx(78)
                    height: hostWindow.snapPx(28)
                    radius: 4
                    color: clearFilterPointer.containsMouse
                           ? hostWindow.controlHoverBg : "transparent"
                    Text {
                        id: clearFilterText
                        objectName: "fileFieldFilterClearText-"
                                    + Number(panel.side || 0)
                        anchors.centerIn: parent
                        text: "Clear"
                        color: hostWindow.textColor
                        width: hostWindow.snapPx(implicitWidth)
                        height: hostWindow.snapPx(implicitHeight)
                        font.pixelSize: 12
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                   clearFilterText, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                   clearFilterText, hostWindow.contentItem)
                        }
                    }
                    MouseArea {
                        id: clearFilterPointer
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: {
                            fileFieldFilterDraft = []
                            fileFieldFilterPopup.applyFileFieldFilters()
                        }
                    }
                }
                Rectangle {
                    width: hostWindow.snapPx(78)
                    height: hostWindow.snapPx(28)
                    radius: 4
                    color: hostWindow.dialogAccent
                    Text {
                        id: applyFilterText
                        objectName: "fileFieldFilterApplyText-"
                                    + Number(panel.side || 0)
                        anchors.centerIn: parent
                        text: "Apply"
                        color: hostWindow.textColor
                        width: hostWindow.snapPx(implicitWidth)
                        height: hostWindow.snapPx(implicitHeight)
                        font.pixelSize: 12
                        font.weight: Font.DemiBold
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                   applyFilterText, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                   applyFilterText, hostWindow.contentItem)
                        }
                    }
                    MouseArea {
                        anchors.fill: parent
                        onClicked: fileFieldFilterPopup.applyFileFieldFilters()
                    }
                }
            }
        }
    }
