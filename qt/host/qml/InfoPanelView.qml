pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Layouts

Rectangle {
    id: infoRoot
    required property ApplicationWindow hostWindow
    required property Item menuBar
    required property var panel
    readonly property int side: Number(panel.side || 0)
    readonly property var rows: panel.rows || []

    // Scene refreshes replace the row payload. Keep the view's model and
    // update its length in place so refreshing a value cannot reset scrolling.
    function syncRowCount() {
        if (!rowSlots)
            return
        if (rowSlots.count > rows.length)
            rowSlots.remove(rows.length, rowSlots.count - rows.length)
        while (rowSlots.count < rows.length)
            rowSlots.append({ "slot": rowSlots.count })
    }

    onRowsChanged: syncRowCount()
    Component.onCompleted: syncRowCount()

    ListModel { id: rowSlots }

    component InfoRowText: Text {
        id: leaf
        transform: Translate {
            x: infoRoot.hostWindow.dialogPixelOffsetX(leaf, infoRoot.hostWindow.contentItem)
            y: infoRoot.hostWindow.dialogPixelOffsetY(leaf, infoRoot.hostWindow.contentItem)
        }
    }

    x: hostWindow.nativePanelX(side)
    y: hostWindow.menuBarHeight
    width: hostWindow.nativePanelWidth(side)
    height: hostWindow.nativePanelHeight(side, hostWindow.menuBarHeight)
    color: "transparent"
    border.width: 0
    clip: true

    Rectangle {
        id: infoHeader
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        height: Math.max(25, hostWindow.ch * 1.25)
        color: "transparent"

        Text {
            anchors.fill: parent
            anchors.leftMargin: 8
            anchors.rightMargin: 8
            text: hostWindow.cleanText(panel.title)
            color: hostWindow.textColor
            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(13) : 13)
            font.bold: true
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideMiddle
        }
    }

    ListView {
        id: infoRows
        objectName: "infoPanelRows-" + infoRoot.side
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: infoHeader.bottom
        anchors.bottom: infoFooter.top
        anchors.margins: 8
        anchors.rightMargin: infoScrollBar.width
        model: rowSlots
        clip: true
        spacing: 2
        boundsBehavior: Flickable.StopAtBounds

        ScrollBar.vertical: F4ScrollBar {
            id: infoScrollBar
            objectName: "infoPanelScrollBar-" + infoRoot.side
            parent: infoRoot
            hostWindow: infoRoot.hostWindow
            x: infoRows.x + infoRows.width
            y: infoRows.y
            width: implicitWidth
            height: hostWindow.snapPx(infoRows.height)
            z: 1
            policy: T.ScrollBar.AlwaysOn
            visible: infoRows.contentHeight > infoRows.height + 0.5 / hostWindow.dpr
            transform: Translate {
                x: infoScrollBar.hostWindow.dialogPixelOffsetX(infoScrollBar, infoScrollBar.hostWindow.contentItem)
                y: infoScrollBar.hostWindow.dialogPixelOffsetY(infoScrollBar, infoScrollBar.hostWindow.contentItem)
            }
        }

        delegate: Item {
            id: infoRow
            required property int index
            readonly property var modelData: infoRoot.rows[index] || ({})
            width: ListView.view.width
            height: modelData.kind === "blank" ? 10
                    : modelData.kind === "section" ? 30
                    : modelData.kind === "usage" ? usageContent.implicitHeight + 8
                    : Math.max(25, infoValue.implicitHeight + 8)

            RowLayout {
                anchors.fill: parent
                spacing: 10
                visible: modelData.kind === "row"

                InfoRowText {
                    objectName: "infoPanelLabel-" + infoRoot.side + "-" + infoRow.index
                    text: hostWindow.cleanText(modelData.label)
                    color: hostWindow.mutedText
                    font.pixelSize: (hostWindow ? hostWindow.uiTextSize(12) : 12)
                    verticalAlignment: Text.AlignTop
                    Layout.alignment: Qt.AlignTop
                    Layout.preferredWidth: Math.min(150, Math.max(80, infoRows.width * 0.34))
                    elide: Text.ElideRight
                }

                InfoRowText {
                    objectName: "infoPanelValue-" + infoRoot.side + "-" + infoRow.index
                    id: infoValue
                    text: hostWindow.cleanText(modelData.value)
                    color: hostWindow.textColor
                    font.pixelSize: (hostWindow ? hostWindow.uiTextSize(12) : 12)
                    wrapMode: Text.WrapAtWordBoundaryOrAnywhere
                    horizontalAlignment: Text.AlignRight
                    verticalAlignment: Text.AlignTop
                    Layout.alignment: Qt.AlignTop
                    Layout.fillWidth: true
                }
            }

            RowLayout {
                anchors.fill: parent
                spacing: 8
                visible: modelData.kind === "section"

                Rectangle { height: 1; color: hostWindow.dialogAccent; Layout.fillWidth: true }
                InfoRowText {
                    objectName: "infoPanelSection-" + infoRoot.side + "-" + infoRow.index
                    text: hostWindow.cleanText(modelData.label)
                    color: hostWindow.activeBorder
                    font.pixelSize: (hostWindow ? hostWindow.uiTextSize(12) : 12)
                    font.bold: true
                }
                Rectangle { height: 1; color: hostWindow.dialogAccent; Layout.fillWidth: true }
            }

            RowLayout {
                anchors.fill: parent
                spacing: 10
                visible: modelData.kind === "usage"

                InfoRowText {
                    objectName: "infoPanelUsageLabel-" + infoRoot.side + "-" + infoRow.index
                    text: hostWindow.cleanText(infoRow.modelData.label)
                    color: hostWindow.mutedText
                    font.pixelSize: hostWindow.uiTextSize(12)
                    Layout.preferredWidth: Math.min(150, Math.max(80, infoRows.width * 0.34))
                    elide: Text.ElideRight
                }

                ColumnLayout {
                    id: usageContent
                    Layout.fillWidth: true
                    spacing: 4

                    InfoRowText {
                        objectName: "infoPanelTotal-" + infoRoot.side + "-" + infoRow.index
                        text: hostWindow.cleanText(infoRow.modelData.totalText)
                        color: hostWindow.textColor
                        font.pixelSize: hostWindow.uiTextSize(12)
                        Layout.fillWidth: true
                        horizontalAlignment: Text.AlignHCenter
                        elide: Text.ElideMiddle
                    }

                    T.ProgressBar {
                        id: usageBar
                        objectName: "infoUsageBar-" + infoRoot.side + "-" + infoRow.index
                        Layout.fillWidth: true
                        Layout.preferredHeight: Math.max(20, usagePercent.implicitHeight + 4)
                        from: 0
                        to: 100
                        value: Math.max(0, Math.min(100, Number(infoRow.modelData.percent) || 0))
                        padding: 0
                        Accessible.name: infoRow.modelData.label + ": " + value + "%"
                        background: Rectangle {
                            radius: 3
                            color: hostWindow.themePaletteObject.panelProgressTrackColor
                        }
                        contentItem: Item {
                            Rectangle {
                                width: parent.width * usageBar.visualPosition
                                height: parent.height
                                radius: 3
                                color: hostWindow.themePaletteObject.panelProgressFillColor
                            }
                            InfoRowText {
                                objectName: "infoPanelPercent-" + infoRoot.side + "-" + infoRow.index
                                id: usagePercent
                                anchors.centerIn: parent
                                text: Math.round(usageBar.value) + "%"
                                color: hostWindow.textColor
                                font.pixelSize: hostWindow.uiTextSize(12)
                            }
                        }
                        Component.onCompleted: {
                            console.debug("[FIX:info-usage]", infoRow.modelData.label, value)
                        }
                    }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: 8
                        InfoRowText {
                            objectName: "infoPanelUsed-" + infoRoot.side + "-" + infoRow.index
                            text: hostWindow.cleanText(infoRow.modelData.usedText)
                            color: hostWindow.textColor
                            font.pixelSize: hostWindow.uiTextSize(12)
                            Layout.fillWidth: true
                            elide: Text.ElideMiddle
                        }
                        InfoRowText {
                            objectName: "infoPanelFree-" + infoRoot.side + "-" + infoRow.index
                            text: hostWindow.cleanText(infoRow.modelData.freeText)
                            color: hostWindow.textColor
                            font.pixelSize: hostWindow.uiTextSize(12)
                            Layout.fillWidth: true
                            horizontalAlignment: Text.AlignRight
                            elide: Text.ElideMiddle
                        }
                    }
                }
            }
        }
    }

    Rectangle {
        id: infoFooter
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: Math.max(24, hostWindow.ch * 1.15)
        color: "transparent"

        Text {
            id: infoPanelFooterText
            objectName: "infoPanelFooterText-" + side
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(infoPanelFooterText, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(infoPanelFooterText, hostWindow.contentItem)
            }
            anchors.fill: parent
            anchors.leftMargin: 8
            anchors.rightMargin: 8
            text: hostWindow.cleanText(panel.bottomHint)
            color: hostWindow.mutedText
            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(11) : 11)
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideMiddle
        }
    }

    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.LeftButton
        propagateComposedEvents: true
        onClicked: (mouse) => {
            hostWindow.action({ "action": "panel.activate", "side": infoRoot.side })
            mouse.accepted = false
        }
        onWheel: (wheel) => { wheel.accepted = false }
    }
}
