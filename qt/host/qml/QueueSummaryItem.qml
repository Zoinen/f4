pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.impl

Row {
    id: queueSummaryItem
    required property ApplicationWindow hostWindow
    property string statusName: ""
    property string iconName: ""
    property int count: 0
    property color accent: hostWindow.mutedText
    readonly property string lucideName: iconName

    spacing: 5
    Accessible.role: Accessible.StaticText
    Accessible.name: statusName + ": " + count

    Image {
        id: summaryIconLabel
        objectName: queueSummaryItem.objectName + "IconLabel"
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(summaryIconLabel,hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(summaryIconLabel,hostWindow.contentItem)
        }
        width: hostWindow.snapPx(14)
        height: hostWindow.snapPx(14)
        anchors.verticalCenter: parent.verticalCenter
        source: hostWindow.lucideIconSource(
                         queueSummaryItem.iconName, 14,
                         queueSummaryItem.accent)
    }

    Text {
        id: summaryText
        objectName: queueSummaryItem.objectName + "Text"
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(summaryText,hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(summaryText,hostWindow.contentItem)
        }
        anchors.verticalCenter: parent.verticalCenter
        text: String(queueSummaryItem.count)
        color: hostWindow.mutedText
        font.pixelSize: 12
    }
}
