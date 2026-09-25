pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Popup {
    id: fileFieldColumnsPopup
    required property ApplicationWindow hostWindow
    required property Item panelItem
    required property Item panelHeader
    required property var panel
    property var fileFieldColumnDraft: []
    function openFileFieldColumns() {
        const columns = panel.galleryColumns || []
        const draft = []
        for (let index = 0; index < columns.length; ++index) {
            const id = String(columns[index].id || columns[index].role || "")
            if (id.startsWith("exif."))
                draft.push({ fieldId: id, width: Number(columns[index].width || 12) })
        }
        fileFieldColumnDraft = draft
        fileFieldColumnsPopup.open()
    }

    function fileFieldColumnIndex(fieldId) {
        for (let index = 0; index < fileFieldColumnDraft.length; ++index) {
            if (String(fileFieldColumnDraft[index].fieldId) === fieldId)
                return index
        }
        return -1
    }

    function toggleFileFieldColumn(fieldId) {
        const draft = fileFieldColumnDraft.slice()
        const index = fileFieldColumnIndex(fieldId)
        if (index >= 0)
            draft.splice(index, 1)
        else
            draft.push({ fieldId: fieldId, width: 12 })
        fileFieldColumnDraft = draft
    }

    function moveFileFieldColumn(fieldId, delta) {
        const draft = fileFieldColumnDraft.slice()
        const index = fileFieldColumnIndex(fieldId)
        const destination = Math.max(0, Math.min(draft.length - 1,
                                                 index + delta))
        if (index < 0 || destination === index)
            return
        const value = draft[index]
        draft[index] = draft[destination]
        draft[destination] = value
        fileFieldColumnDraft = draft
    }

    function resizeFileFieldColumn(fieldId, delta) {
        const draft = fileFieldColumnDraft.slice()
        const index = fileFieldColumnIndex(fieldId)
        if (index < 0)
            return
        const column = Object.assign({}, draft[index])
        column.width = Math.max(6, Math.min(40,
                                             Number(column.width || 12) + delta))
        draft[index] = column
        fileFieldColumnDraft = draft
    }

    function saveFileFieldColumns() {
        hostWindow.action({
            action: "panel.fileFields.columns", side: panel.side,
            columns: fileFieldColumnDraft
        })
        fileFieldColumnsPopup.close()
    }

        objectName: "fileFieldColumnsPopup-" + Number(panel.side || 0)
        parent: Overlay.overlay
        width: hostWindow.snapPx(430)
        height: hostWindow.snapPx(356)
        padding: hostWindow.snapPx(8)
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
            spacing: hostWindow.snapPx(3)

            Text {
                id: fileFieldColumnsTitle
                objectName: "fileFieldColumnsTitle-" + Number(panel.side || 0)
                width: parent.width
                height: hostWindow.snapPx(28)
                text: "Details columns"
                color: hostWindow.textColor
                font.pixelSize: 14
                font.weight: Font.DemiBold
                verticalAlignment: Text.AlignVCenter
                transform: Translate {
                    x: hostWindow.dialogPixelOffsetX(
                           fileFieldColumnsTitle, hostWindow.contentItem)
                    y: hostWindow.dialogPixelOffsetY(
                           fileFieldColumnsTitle, hostWindow.contentItem)
                }
            }

            Repeater {
                id: fileFieldColumnRepeater
                model: panel.fileFieldDescriptors || []

                delegate: Rectangle {
                    id: fileFieldColumnRow
                    required property int index
                    required property var modelData
                    readonly property string fieldId:
                        String(modelData.id || "")
                    readonly property int orderIndex:
                        fileFieldColumnsPopup.fileFieldColumnIndex(fieldId)
                    readonly property bool selected: orderIndex >= 0
                    objectName: "fileFieldColumnRow-" + fieldId
                                + "-" + Number(panel.side || 0)
                    width: fileFieldColumnsPopup.availableWidth
                    height: hostWindow.snapPx(34)
                    radius: 4
                    color: columnRowPointer.containsMouse
                           ? hostWindow.controlHoverBg : "transparent"

                    HostPixelAlignedImage {
                        hostWindow: fileFieldColumnsPopup.hostWindow
                        objectName: "fileFieldColumnCheck-" + fieldId
                                    + "-" + Number(panel.side || 0)
                        x: hostWindow.snapPx(8)
                        anchors.verticalCenter: parent.verticalCenter
                        width: hostWindow.snapPx(15)
                        height: hostWindow.snapPx(15)
                        visible: fileFieldColumnRow.selected
                        smooth: false
                        alignmentRevision: fileFieldColumnsPopup.x
                                           + fileFieldColumnsPopup.y
                                           + fileFieldColumnsPopup.width
                                           + fileFieldColumnsPopup.height
                        source: hostWindow.lucideIconSource(
                                    "check", 15, hostWindow.dialogAccent)
                    }

                    Text {
                        id: fileFieldColumnLabel
                        objectName: "fileFieldColumnLabel-" + fieldId
                                    + "-" + Number(panel.side || 0)
                        anchors.left: parent.left
                        anchors.leftMargin: hostWindow.snapPx(32)
                        anchors.right: parent.right
                        anchors.rightMargin: hostWindow.snapPx(190)
                        anchors.verticalCenter: parent.verticalCenter
                        text: String(modelData.title || fieldId)
                        color: fileFieldColumnRow.selected
                               ? hostWindow.textColor : hostWindow.mutedText
                        font.pixelSize: 12
                        elide: Text.ElideRight
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                   fileFieldColumnLabel,
                                   hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                   fileFieldColumnLabel,
                                   hostWindow.contentItem)
                        }
                    }

                    Rectangle {
                        id: columnUpButton
                        objectName: "fileFieldColumnUp-" + fieldId
                                    + "-" + Number(panel.side || 0)
                        x: hostWindow.snapPx(parent.width - 180)
                        y: hostWindow.snapPx((parent.height - height) / 2)
                        width: hostWindow.snapPx(26)
                        height: hostWindow.snapPx(24)
                        radius: 4
                        color: columnUpPointer.containsMouse
                               ? hostWindow.controlPressedBg : "transparent"
                        opacity: fileFieldColumnRow.selected ? 1 : 0.35
                        Text {
                            id: columnUpText
                            objectName: "fileFieldColumnUpText-" + fieldId
                                        + "-" + Number(panel.side || 0)
                            anchors.fill: parent
                            text: "↑"
                            color: hostWindow.textColor
                            font.pixelSize: 13
                            horizontalAlignment: Text.AlignHCenter
                            verticalAlignment: Text.AlignVCenter
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                       columnUpText, hostWindow.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                       columnUpText, hostWindow.contentItem)
                            }
                        }
                        MouseArea {
                            id: columnUpPointer
                            anchors.fill: parent
                            enabled: fileFieldColumnRow.selected
                                     && fileFieldColumnRow.orderIndex > 0
                            hoverEnabled: true
                            onClicked: fileFieldColumnsPopup.moveFileFieldColumn(fieldId, -1)
                        }
                    }

                    Rectangle {
                        id: columnDownButton
                        objectName: "fileFieldColumnDown-" + fieldId
                                    + "-" + Number(panel.side || 0)
                        x: hostWindow.snapPx(parent.width - 151)
                        y: hostWindow.snapPx((parent.height - height) / 2)
                        width: hostWindow.snapPx(26)
                        height: hostWindow.snapPx(24)
                        radius: 4
                        color: columnDownPointer.containsMouse
                               ? hostWindow.controlPressedBg : "transparent"
                        opacity: fileFieldColumnRow.selected ? 1 : 0.35
                        Text {
                            id: columnDownText
                            objectName: "fileFieldColumnDownText-" + fieldId
                                        + "-" + Number(panel.side || 0)
                            anchors.fill: parent
                            text: "↓"
                            color: hostWindow.textColor
                            font.pixelSize: 13
                            horizontalAlignment: Text.AlignHCenter
                            verticalAlignment: Text.AlignVCenter
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                       columnDownText, hostWindow.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                       columnDownText, hostWindow.contentItem)
                            }
                        }
                        MouseArea {
                            id: columnDownPointer
                            anchors.fill: parent
                            enabled: fileFieldColumnRow.selected
                                     && fileFieldColumnRow.orderIndex
                                        < fileFieldColumnsPopup.fileFieldColumnDraft.length - 1
                            hoverEnabled: true
                            onClicked: fileFieldColumnsPopup.moveFileFieldColumn(fieldId, 1)
                        }
                    }

                    Rectangle {
                        id: columnNarrowButton
                        objectName: "fileFieldColumnNarrow-" + fieldId
                                    + "-" + Number(panel.side || 0)
                        x: hostWindow.snapPx(parent.width - 116)
                        y: hostWindow.snapPx((parent.height - height) / 2)
                        width: hostWindow.snapPx(24)
                        height: hostWindow.snapPx(24)
                        radius: 4
                        color: columnNarrowPointer.containsMouse
                               ? hostWindow.controlPressedBg : "transparent"
                        opacity: fileFieldColumnRow.selected ? 1 : 0.35
                        Text {
                            id: columnNarrowText
                            objectName: "fileFieldColumnNarrowText-" + fieldId
                                        + "-" + Number(panel.side || 0)
                            anchors.fill: parent
                            text: "−"
                            color: hostWindow.textColor
                            font.pixelSize: 13
                            horizontalAlignment: Text.AlignHCenter
                            verticalAlignment: Text.AlignVCenter
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                       columnNarrowText, hostWindow.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                       columnNarrowText, hostWindow.contentItem)
                            }
                        }
                        MouseArea {
                            id: columnNarrowPointer
                            anchors.fill: parent
                            enabled: fileFieldColumnRow.selected
                            hoverEnabled: true
                            onClicked: fileFieldColumnsPopup.resizeFileFieldColumn(fieldId, -1)
                        }
                    }

                    Text {
                        id: columnWidthValue
                        objectName: "fileFieldColumnWidth-" + fieldId
                                    + "-" + Number(panel.side || 0)
                        x: hostWindow.snapPx(parent.width - 87)
                        width: hostWindow.snapPx(34)
                        height: hostWindow.snapPx(implicitHeight)
                        anchors.verticalCenter: parent.verticalCenter
                        text: fileFieldColumnRow.selected
                              ? String(fileFieldColumnsPopup.fileFieldColumnDraft[
                                           fileFieldColumnRow.orderIndex].width)
                              : "—"
                        color: hostWindow.mutedText
                        font.pixelSize: 11
                        horizontalAlignment: Text.AlignHCenter
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                   columnWidthValue, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                   columnWidthValue, hostWindow.contentItem)
                        }
                    }

                    Rectangle {
                        id: columnWidenButton
                        objectName: "fileFieldColumnWiden-" + fieldId
                                    + "-" + Number(panel.side || 0)
                        x: hostWindow.snapPx(parent.width - 47)
                        y: hostWindow.snapPx((parent.height - height) / 2)
                        width: hostWindow.snapPx(24)
                        height: hostWindow.snapPx(24)
                        radius: 4
                        color: columnWidenPointer.containsMouse
                               ? hostWindow.controlPressedBg : "transparent"
                        opacity: fileFieldColumnRow.selected ? 1 : 0.35
                        Text {
                            id: columnWidenText
                            objectName: "fileFieldColumnWidenText-" + fieldId
                                        + "-" + Number(panel.side || 0)
                            anchors.fill: parent
                            text: "+"
                            color: hostWindow.textColor
                            font.pixelSize: 13
                            horizontalAlignment: Text.AlignHCenter
                            verticalAlignment: Text.AlignVCenter
                            transform: Translate {
                                x: hostWindow.dialogPixelOffsetX(
                                       columnWidenText, hostWindow.contentItem)
                                y: hostWindow.dialogPixelOffsetY(
                                       columnWidenText, hostWindow.contentItem)
                            }
                        }
                        MouseArea {
                            id: columnWidenPointer
                            anchors.fill: parent
                            enabled: fileFieldColumnRow.selected
                            hoverEnabled: true
                            onClicked: fileFieldColumnsPopup.resizeFileFieldColumn(fieldId, 1)
                        }
                    }

                    MouseArea {
                        id: columnRowPointer
                        anchors.fill: parent
                        anchors.rightMargin: hostWindow.snapPx(184)
                        hoverEnabled: true
                        onClicked: fileFieldColumnsPopup.toggleFileFieldColumn(fieldId)
                    }
                }
            }

            Rectangle {
                width: fileFieldColumnsPopup.availableWidth
                height: hostWindow.snapPx(1)
                color: hostWindow.separatorColor
            }

            Row {
                width: fileFieldColumnsPopup.availableWidth
                height: hostWindow.snapPx(34)
                spacing: hostWindow.snapPx(8)
                Item { width: hostWindow.snapPx(1); height: hostWindow.snapPx(1) }
                Rectangle {
                    width: hostWindow.snapPx(82)
                    height: hostWindow.snapPx(30)
                    radius: 5
                    color: cancelColumnsPointer.containsMouse
                           ? hostWindow.controlHoverBg : "transparent"
                    Text {
                        id: cancelColumnsText
                        objectName: "fileFieldColumnsCancelText-"
                                    + Number(panel.side || 0)
                        anchors.centerIn: parent
                        text: "Cancel"
                        color: hostWindow.textColor
                        width: hostWindow.snapPx(implicitWidth)
                        height: hostWindow.snapPx(implicitHeight)
                        font.pixelSize: 12
                        transform: Translate {
                            x: hostWindow.dialogPixelOffsetX(
                                   cancelColumnsText, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                   cancelColumnsText, hostWindow.contentItem)
                        }
                    }
                    MouseArea {
                        id: cancelColumnsPointer
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: fileFieldColumnsPopup.close()
                    }
                }
                Rectangle {
                    width: hostWindow.snapPx(82)
                    height: hostWindow.snapPx(30)
                    radius: 5
                    color: hostWindow.dialogAccent
                    Text {
                        id: applyColumnsText
                        objectName: "fileFieldColumnsApplyText-"
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
                                   applyColumnsText, hostWindow.contentItem)
                            y: hostWindow.dialogPixelOffsetY(
                                   applyColumnsText, hostWindow.contentItem)
                        }
                    }
                    MouseArea {
                        anchors.fill: parent
                        onClicked: fileFieldColumnsPopup.saveFileFieldColumns()
                    }
                }
            }
        }
    }
