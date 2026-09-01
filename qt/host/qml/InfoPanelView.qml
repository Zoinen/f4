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
    y: menuBar.height
    width: hostWindow.nativePanelWidth(side)
    height: Math.max(1, hostWindow.height - menuBar.height
                     - hostWindow.commandLineHeight(hostWindow.shellFrame())
                     - hostWindow.keyBarHeight())
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
            font.pixelSize: 13
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
            width: ListView.view.width
            height: modelData.kind === "blank" ? 10
                    : modelData.kind === "section" ? 30
                    : Math.max(25, infoValue.implicitHeight + 8)

            RowLayout {
                anchors.fill: parent
                spacing: 10
                visible: modelData.kind === "row"

                Text {
                    text: hostWindow.cleanText(modelData.label)
                    color: hostWindow.mutedText
                    font.pixelSize: 12
                    verticalAlignment: Text.AlignTop
                    Layout.alignment: Qt.AlignTop
                    Layout.preferredWidth: Math.min(150, Math.max(80, infoRows.width * 0.34))
                    elide: Text.ElideRight
                }

                Text {
                    id: infoValue
                    text: hostWindow.cleanText(modelData.value)
                    color: hostWindow.textColor
                    font.pixelSize: 12
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
                    font.pixelSize: 12
                    font.bold: true
                }
                Rectangle { height: 1; color: hostWindow.dialogAccent; Layout.fillWidth: true }
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
            anchors.fill: parent
            anchors.leftMargin: 8
            anchors.rightMargin: 8
            text: hostWindow.cleanText(panel.bottomHint)
            color: hostWindow.mutedText
            font.pixelSize: 11
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
