pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQml.Models

T.Menu {
    id: menu
    readonly property var viewportWindow: parent ? parent.Window.window : null
    height: Math.min(implicitHeight, viewportWindow ? Math.max(1, viewportWindow.height - 12) : implicitHeight)
    margins: 6

    contentItem: ListView {
        id: menuList
        objectName: menu.objectName + "Viewport"
        implicitHeight: contentHeight
        model: menu.contentModel
        currentIndex: menu.currentIndex
        highlightMoveDuration: 0
        interactive: contentHeight > height
        boundsBehavior: Flickable.StopAtBounds
        clip: true
        DropdownWheelHandler { viewport: menuList }
        ScrollBar.vertical: F4ScrollBar {
            id: menuScrollBar
            objectName: menu.objectName + "ScrollBar"
            thickness: 8
            policy: ScrollBar.AsNeeded
            visible: size < 1
        }
    }

    // Disabled MenuItems cannot receive hover events themselves. Observe the
    // content from outside them without stealing presses or keyboard focus.
    property Instantiator inactiveRowObservers: Instantiator {
        active: menu.visible
        model: menu.count
        delegate: Item {
            id: observer
            required property int index
            readonly property Item row: { menu.count; return menu.itemAt(index) }
            // Keep guards outside the ListView's content model and physically
            // cover only disabled rows, never an enabled MenuItem's hover area.
            parent: menu.contentItem ? menu.contentItem.parent : null
            visible: row !== null && row.visible && !row.enabled
            readonly property point origin: {
                const scroll = menu.contentItem ? Number(menu.contentItem.contentY || 0) : 0
                const revision = row && menu.contentItem
                    ? row.x + row.y + menu.contentItem.x + menu.contentItem.y : 0
                const mapped = row && parent ? row.mapToItem(parent, 0, 0) : Qt.point(0, 0)
                return Qt.point(mapped.x + revision * 0, mapped.y + scroll * 0)
            }
            readonly property point viewportOrigin: menu.contentItem && parent
                ? menu.contentItem.mapToItem(parent, menu.contentItem.x * 0, menu.contentItem.y * 0) : Qt.point(0, 0)
            x: origin.x
            y: Math.max(origin.y, viewportOrigin.y)
            width: row ? Math.max(0, row.width - (menuScrollBar.visible ? menuScrollBar.width : 0)) : 0
            height: row ? Math.max(0, Math.min(origin.y + row.height,
                viewportOrigin.y + menuList.height) - y) : 0
            z: 1000
            property point lastPointerPosition: Qt.point(NaN, NaN)
            HoverHandler {
                target: null
                blocking: false
                onPointChanged: clearInactiveRow()
                onHoveredChanged: {
                    if (hovered) clearInactiveRow()
                    else observer.lastPointerPosition = Qt.point(NaN, NaN)
                }
                function clearInactiveRow() {
                    if (!hovered) return
                    const previous = observer.lastPointerPosition
                    const current = point.scenePosition
                    if (previous.x === current.x && previous.y === current.y) return
                    observer.lastPointerPosition = current
                    if (menu.currentIndex !== -1) {
                        console.debug("[FIX:menu-hover] clear disabled row", menu.objectName, observer.index)
                        menu.currentIndex = -1
                    }
                }
            }
        }
    }
}
