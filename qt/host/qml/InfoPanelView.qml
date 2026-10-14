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
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: infoHeader.bottom
        anchors.bottom: infoFooter.top
        anchors.margins: 8
        model: panel.rows || []
        clip: true
        spacing: 2
        boundsBehavior: Flickable.StopAtBounds

        delegate: Item {
            id: infoRow
            required property var modelData
            required property int index
            width: ListView.view.width
            height: modelData.kind === "blank" ? 10
                    : modelData.kind === "section" ? 30
                    : modelData.kind === "usage" ? usageContent.implicitHeight + 8
                    : Math.max(25, infoValue.implicitHeight + 8)

            RowLayout {
                anchors.fill: parent
                spacing: 10
                visible: modelData.kind === "row"

                Text {
                    text: hostWindow.cleanText(modelData.label)
                    color: hostWindow.mutedText
                    font.pixelSize: (hostWindow ? hostWindow.uiTextSize(12) : 12)
                    verticalAlignment: Text.AlignTop
                    Layout.alignment: Qt.AlignTop
                    Layout.preferredWidth: Math.min(150, Math.max(80, infoRows.width * 0.34))
                    elide: Text.ElideRight
                }

                Text {
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
                Text {
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

                Text {
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

                    Text {
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
                            Text {
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
                        Text {
                            text: hostWindow.cleanText(infoRow.modelData.usedText)
                            color: hostWindow.textColor
                            font.pixelSize: hostWindow.uiTextSize(12)
                            Layout.fillWidth: true
                            elide: Text.ElideMiddle
                        }
                        Text {
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
