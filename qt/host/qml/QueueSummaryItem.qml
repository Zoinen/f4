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

    IconLabel {
        width: 14
        height: 14
        anchors.verticalCenter: parent.verticalCenter
        icon.source: hostWindow.lucideIconSource(
                         queueSummaryItem.iconName, 14,
                         queueSummaryItem.accent)
        icon.width: 14
        icon.height: 14
        icon.color: queueSummaryItem.accent
    }

    Text {
        anchors.verticalCenter: parent.verticalCenter
        text: String(queueSummaryItem.count)
        color: hostWindow.mutedText
        font.pixelSize: 12
    }
}
