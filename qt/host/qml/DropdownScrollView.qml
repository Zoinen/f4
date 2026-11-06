pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.ScrollView {
    id: view
    required property ApplicationWindow hostWindow
    property string menuObjectName: "dropdown"
    objectName: menuObjectName + "ScrollView"
    readonly property bool overflow: contentHeight > availableHeight
    readonly property real rowWidth: Math.max(0, availableWidth - (overflow ? verticalBar.width : 0))
    implicitHeight: contentHeight
    contentWidth: availableWidth
    clip: true
    DropdownWheelHandler { viewport: view.contentItem }
    ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
    ScrollBar.vertical: F4ScrollBar {
        id: verticalBar
        hostWindow: view.hostWindow
        objectName: view.menuObjectName + "ScrollBar"
        parent: view
        x: view.width - width
        y: view.topPadding
        height: view.availableHeight
        thickness: 8
        visible: view.overflow
        policy: ScrollBar.AsNeeded
    }
    Component.onCompleted: {
        contentItem.objectName = view.menuObjectName + "Viewport"
        contentItem.boundsBehavior = Flickable.StopAtBounds
    }
    function reveal(row) {
        if (!row || !contentItem || !contentItem.contentItem) return
        const top = row.mapToItem(contentItem.contentItem, 0, 0).y
        if (top < contentItem.contentY) contentItem.contentY = top
        else if (top + row.height > contentItem.contentY + contentItem.height)
            contentItem.contentY = top + row.height - contentItem.height
    }
    onOverflowChanged: if (overflow) console.debug("[FIX:menu-overflow] scrolling", objectName)
}
