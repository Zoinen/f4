pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Rectangle {
    id: status
    required property ApplicationWindow hostWindow
    required property var panel
    readonly property bool selectionActive: Number(panel.selectedCount || 0) > 0
    readonly property color summaryColor: selectionActive ? hostWindow.gallerySelectionColor : hostWindow.mutedText
    readonly property real padding: hostWindow.snapPx(8)
    readonly property real gap: hostWindow.snapPx(8)
    readonly property real usedFraction: capacityKnown
        ? 1 - Math.max(0, Math.min(1, Number(panel.freeSpace || 0) / Number(panel.diskTotalSpace))) : 0
    readonly property bool capacityKnown: panel.freeSpaceKnown === true && Number(panel.diskTotalSpace || 0) > 0
    readonly property real desiredWidth: 2 * padding + Math.max(
        summary.implicitWidth,
        disk.visible ? disk.naturalWidth : 0)
    implicitWidth: Math.ceil(desiredWidth * hostWindow.dpr) / hostWindow.dpr
    implicitHeight: hostWindow.snapPx(content.height + 2 * padding)
    color: Qt.rgba(hostWindow.dialogBg.r, hostWindow.dialogBg.g, hostWindow.dialogBg.b, 0.96)
    border.color: hostWindow.separatorColor
    border.width: hostWindow.separatorWidth
    radius: hostWindow.snapPx(6)
    transform: Translate {
        x: hostWindow.dialogPixelOffsetX(status, hostWindow.contentItem)
        y: hostWindow.dialogPixelOffsetY(status, hostWindow.contentItem)
    }

    function bytes(value) {
        const count = Math.max(0, Number(value || 0))
        const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"]
        const unit = count > 0 ? Math.min(units.length - 1, Math.floor(Math.log(count) / Math.log(1024))) : 0
        return (count / Math.pow(1024, unit)).toLocaleString(Qt.locale(), 'f', unit > 0 ? 1 : 0) + " " + units[unit]
    }

    Column {
        id: content
        x: status.padding
        y: status.padding
        width: Math.max(1, status.width - 2 * status.padding)
        spacing: status.hostWindow.snapPx(3)
        Row {
            id: summary
            width: content.width
            spacing: status.gap
            opacity: status.selectionActive ? 0.95 : 1
            PanelStatusMetric {
                id: files
                objectName: "panelStatusFiles-" + Number(status.panel.side || 0)
                hostWindow: status.hostWindow
                width: naturalWidth
                height: implicitHeight
                iconName: "file"
                textColor: status.summaryColor
                text: Number(status.selectionActive ? status.panel.selectedFiles || 0 : status.panel.totalFiles || 0).toLocaleString(Qt.locale(), 'f', 0)
            }
            PanelStatusMetric {
                id: folders
                objectName: "panelStatusFolders-" + Number(status.panel.side || 0)
                hostWindow: status.hostWindow
                width: naturalWidth
                height: implicitHeight
                iconName: "folder"
                textColor: status.summaryColor
                text: Number(status.selectionActive ? status.panel.selectedDirectories || 0 : status.panel.totalDirectories || 0).toLocaleString(Qt.locale(), 'f', 0)
            }
            PanelStatusMetric {
                id: size
                objectName: "panelStatusSize-" + Number(status.panel.side || 0)
                hostWindow: status.hostWindow
                width: naturalWidth
                height: implicitHeight
                iconName: "database"
                textColor: status.summaryColor
                text: status.bytes(status.selectionActive ? status.panel.selectedSize : status.panel.totalSize)
            }
        }
        PanelStatusMetric {
            id: disk
            objectName: "panelStatusDisk-" + Number(status.panel.side || 0)
            hostWindow: status.hostWindow
            visible: status.panel.freeSpaceKnown === true
            width: content.width
            height: implicitHeight
            iconName: "hard-drive"
            text: status.capacityKnown
                ? qsTr("%1 free of %2").arg(status.bytes(status.panel.freeSpace)).arg(status.bytes(status.panel.diskTotalSpace))
                : qsTr("%1 free").arg(status.bytes(status.panel.freeSpace))
        }
        Rectangle {
            id: track
            objectName: "panelStatusSpaceTrack-" + Number(status.panel.side || 0)
            visible: status.capacityKnown
            width: content.width
            height: status.hostWindow.snapPx(4)
            radius: height / 2
            color: Qt.darker(status.hostWindow.controlBorder, 1.6)
            Rectangle {
                objectName: "panelStatusSpaceFill-" + Number(status.panel.side || 0)
                width: status.hostWindow.snapPx(track.width * status.usedFraction)
                height: track.height
                radius: track.radius
                color: status.hostWindow.controlBorder
            }
        }
        PanelStatusMetric {
            objectName: "panelStatusSymlink-" + Number(status.panel.side || 0)
            hostWindow: status.hostWindow
            visible: status.panel.showFileInfo === true && !status.selectionActive && text !== ""
            width: content.width
            height: implicitHeight
            iconName: "folder-symlink"
            text: String(status.panel.symlinkTarget || "")
        }
    }
}
