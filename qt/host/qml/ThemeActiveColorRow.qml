pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

// Active item title & badge
RowLayout {
    id: themeActiveColorRow
    required property ApplicationWindow hostWindow
    required property QtObject editorWindow
    required property ThemeDraftModel draft
    objectName: "themeActiveColorRow"
    Layout.fillWidth: false
    Layout.preferredWidth: hostWindow.snapPx(320)
    Layout.minimumWidth: hostWindow.snapPx(320)
    Layout.maximumWidth: hostWindow.snapPx(320)
    width: hostWindow.snapPx(320)
    Layout.preferredHeight: hostWindow.snapPx(26)
    Layout.minimumHeight: hostWindow.snapPx(26)
    Layout.maximumHeight: hostWindow.snapPx(26)
    spacing: hostWindow.snapPx(8)
    transform: Translate {
        x: hostWindow.dialogPixelOffsetX(
            themeActiveColorRow,
            editorWindow.contentItem)
        y: hostWindow.dialogPixelOffsetY(
            themeActiveColorRow,
            editorWindow.contentItem)
    }

    Text {
        id: themeColorEditorPaneLeaf0
        objectName: "themeColorEditorPaneLeaf0"
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(themeColorEditorPaneLeaf0, hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(themeColorEditorPaneLeaf0, hostWindow.contentItem)
        }
        text: draft.currentItem ? draft.currentItem.name : ""
        color: hostWindow.textColor
        font.family: hostWindow.uiFontFamily
        font.pixelSize: (hostWindow ? hostWindow.uiTextSize(12) : 12)
        font.weight: Font.Bold
        elide: Text.ElideRight
        Layout.fillWidth: true
    }

    Rectangle {
        id: themeColorGroupBadge
        objectName: "themeColorGroupBadge"
        radius: hostWindow.snapPx(3)
        color: hostWindow.controlBg
        border.width: hostWindow.separatorWidth
        border.color: hostWindow.controlBorder
        implicitHeight: hostWindow.snapPx(18)
        implicitWidth: hostWindow.snapPx(groupText.implicitWidth + 8)
        Layout.preferredHeight: hostWindow.snapPx(18)
        Layout.preferredWidth: hostWindow.snapPx(
            groupText.implicitWidth + 8)
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeColorGroupBadge,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeColorGroupBadge,
                editorWindow.contentItem)
        }

        Text {
            objectName: "groupText"
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(groupText, hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(groupText, hostWindow.contentItem)
            }
            id: groupText
            text: draft.currentItem ? draft.currentItem.group : ""
            color: hostWindow.mutedText
            font.pixelSize: (hostWindow ? hostWindow.uiTextSize(9) : 9)
            x: hostWindow.snapPx((parent.width - width) / 2)
            y: hostWindow.snapPx((parent.height - height) / 2)
        }
    }

    // Large preview swatch
    Rectangle {
        id: themeColorPreviewSwatch
        objectName: "themeColorPreviewSwatch"
        implicitWidth: hostWindow.snapPx(26)
        implicitHeight: hostWindow.snapPx(26)
        Layout.preferredWidth: hostWindow.snapPx(26)
        Layout.preferredHeight: hostWindow.snapPx(26)
        radius: hostWindow.snapPx(4)
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeColorPreviewSwatch,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeColorPreviewSwatch,
                editorWindow.contentItem)
        }
        color: hostWindow.controlBg
        border.width: hostWindow.separatorWidth
        border.color: hostWindow.controlBorder
        clip: true

        Canvas {
            anchors.fill: parent
            onPaint: {
                const ctx = getContext("2d")
                const sz = 4
                for (let x = 0; x < width; x += sz) {
                    for (let y = 0; y < height; y += sz) {
                        ctx.fillStyle = ((Math.floor(x / sz) + Math.floor(y / sz)) % 2 === 0) ? "#404b5a" : "#222c38"
                        ctx.fillRect(x, y, sz, sz)
                    }
                }
            }
        }

        Rectangle {
            anchors.fill: parent
            color: draft.currentItem ? hostWindow[draft.currentItem.id] : "transparent"
        }
    }
}
