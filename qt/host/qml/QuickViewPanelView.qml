pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Layouts

Rectangle {
    id: quickRoot
    required property ApplicationWindow hostWindow
    required property Item menuBar
    required property var quickView
    readonly property int side: Number(quickView.side || 0)
    readonly property var documentFrame:
        quickView.surface || ({})
    readonly property string previewKind:
        hostWindow.cleanText(quickView.previewKind).toLowerCase()
    readonly property bool showsDocument:
        previewKind === "text" || previewKind === "binary"
        || previewKind === "hex" || previewKind === "provider"
        || previewKind === "document"
        || (previewKind === "" && documentFrame.rows !== undefined)
    // Go exports the exact fixed header used by the text frontend.
    // Directory stats deliberately have no separate header because their
    // first body row already contains the selected folder name.
    readonly property var effectiveHeaderRows:
        quickView.headerRows || []
    readonly property var directoryRows: {
        if (documentFrame.windowRows !== undefined)
            return documentFrame.windowRows || []
        return documentFrame.rows || []
    }

    objectName: "quickViewPanel-" + side
    x: hostWindow.nativePanelX(side)
    y: menuBar.height
    width: hostWindow.nativePanelWidth(side)
    height: Math.max(1, hostWindow.height - menuBar.height
                     - hostWindow.commandLineHeight(hostWindow.shellFrame())
                     - hostWindow.keyBarHeight())
    color: "transparent"
    border.width: 0
    clip: true
    z: 3

    Rectangle {
        id: quickTitle
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        height: Math.max(25, hostWindow.ch * 1.25)
        color: "transparent"

        Text {
            anchors.fill: parent
            anchors.leftMargin: 8
            anchors.rightMargin: 8
            text: hostWindow.cleanText(quickRoot.quickView.title)
            color: quickRoot.quickView.active === true
                   ? hostWindow.activeBorder : hostWindow.textColor
            font.pixelSize: 13
            font.bold: true
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideMiddle
        }

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: hostWindow.separatorWidth
            color: hostWindow.separatorColor
        }
    }

    Item {
        id: quickHeader
        objectName: "quickViewHeader-" + quickRoot.side
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: quickTitle.bottom
        height: Math.min(4, quickRoot.effectiveHeaderRows.length)
                * Math.max(20, hostWindow.ch)
        clip: true

        Repeater {
            model: quickRoot.effectiveHeaderRows
            delegate: ConsoleRunRow {
                required property int index
                required property var modelData
                hostWindow: quickRoot.hostWindow
                x: 0
                y: index * Math.max(20, hostWindow.ch)
                width: quickHeader.width
                height: Math.max(20, hostWindow.ch)
                runs: modelData.runs || []
                fallbackText: hostWindow.rowText(modelData)
            }
        }
    }

    Item {
        id: quickContent
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: quickHeader.bottom
        anchors.bottom: quickFooter.top
        clip: true

        DocumentSurface {
            id: quickDocument
            hostWindow: quickRoot.hostWindow
            menuBar: quickRoot.menuBar
            frame: quickRoot.documentFrame
            embedded: true
            interactionActive: quickRoot.visible
                               && quickRoot.showsDocument
            surfaceObjectName: "quickViewDocumentSurface-"
                               + quickRoot.side
            anchors.fill: parent
            visible: quickRoot.showsDocument
        }

        ListView {
            id: quickDirectoryList
            objectName: "quickViewDirectoryList-" + quickRoot.side
            anchors.fill: parent
            anchors.leftMargin: 8
            anchors.rightMargin: 8
            visible: quickRoot.previewKind === "directory"
            model: quickRoot.directoryRows
            clip: true
            spacing: 1
            interactive: visible
            boundsBehavior: Flickable.StopAtBounds
            reuseItems: true

            delegate: ConsoleRunRow {
                required property var modelData
                hostWindow: quickRoot.hostWindow
                width: ListView.view.width
                height: Math.max(20, hostWindow.ch)
                runs: modelData.runs || []
                fallbackText: hostWindow.rowText(modelData)
            }
        }

        Image {
            id: quickImage
            objectName: "quickViewImage-" + quickRoot.side
            anchors.fill: parent
            anchors.margins: 10
            visible: quickRoot.previewKind === "image"
            source: hostWindow.cleanText(quickRoot.quickView.imageSource)
            sourceSize.width: Number(quickRoot.quickView.imageWidth || 0)
            sourceSize.height: Number(quickRoot.quickView.imageHeight || 0)
            asynchronous: true
            cache: true
            fillMode: Image.PreserveAspectFit
            smooth: true
            mipmap: true
        }

        Text {
            objectName: "quickViewLoading-" + quickRoot.side
            anchors.centerIn: parent
            width: Math.max(0, parent.width - 24)
            visible: (quickRoot.previewKind === "loading"
                      && quickRoot.effectiveHeaderRows.length < 4)
                     || (quickRoot.previewKind === "image"
                         && quickRoot.quickView.loading === true)
            text: hostWindow.cleanText(quickRoot.quickView.label) !== ""
                  ? hostWindow.cleanText(quickRoot.quickView.label)
                  : "Loading…"
            color: hostWindow.mutedText
            font.pixelSize: 13
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.WrapAtWordBoundaryOrAnywhere
        }

        Text {
            objectName: "quickViewError-" + quickRoot.side
            anchors.centerIn: parent
            width: Math.max(0, parent.width - 24)
            visible: quickRoot.previewKind === "error"
                     && quickRoot.effectiveHeaderRows.length < 4
                     && hostWindow.cleanText(quickRoot.quickView.error) !== ""
            text: hostWindow.cleanText(quickRoot.quickView.error)
            color: hostWindow.textColor
            font.pixelSize: 13
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.WrapAtWordBoundaryOrAnywhere
        }
    }

    Rectangle {
        id: quickFooter
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: Math.max(24, hostWindow.ch * 1.15)
        color: "transparent"

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: hostWindow.separatorWidth
            color: hostWindow.separatorColor
        }

        Text {
            anchors.fill: parent
            anchors.leftMargin: 8
            anchors.rightMargin: 8
            text: hostWindow.cleanText(quickRoot.quickView.bottomHint)
            color: hostWindow.mutedText
            font.pixelSize: 11
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideMiddle
        }
    }

    TapHandler {
        acceptedButtons: Qt.LeftButton
        gesturePolicy: TapHandler.ReleaseWithinBounds
        onTapped: hostWindow.action({
            "action": "panel.activate",
            "side": quickRoot.side
        })
    }
}
