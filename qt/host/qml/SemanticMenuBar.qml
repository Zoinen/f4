pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls


Rectangle {
    id: menuBarRoot
    required property ApplicationWindow hostWindow
    required property Item semanticLayer
    required property QtObject nativeWindowAgent
    required property bool nativeWindowAgentReady
    required property bool usesQwk
    property var menu: ({})
    property int pointerHoverIndex: -1
    readonly property int effectiveSelected:
        hostWindow.menuBarPreviewIndex >= 0
        ? hostWindow.menuBarPreviewIndex : Number(menu.selected || 0)
    color: "transparent"
    visible: menu.items !== undefined

    function registerNativeHitTargets() {
        if (!usesQwk || !nativeWindowAgentReady)
            return
        for (var i = 0; i < menuItemRepeater.count; ++i) {
            var item = menuItemRepeater.itemAt(i)
            if (item)
                item.registerNativeHitTarget()
        }
    }

    onNativeWindowAgentReadyChanged: registerNativeHitTargets()

    Timer {
        id: menuBarHoverSyncTimer
        interval: 12
        onTriggered: {
            var preview = hostWindow.menuBarPreviewIndex
            if (menu.active !== true || preview < 0
                    || Number(menu.selected || 0) === preview)
                return
            // The preview has already been painted locally. Synchronize
            // Go on the next event-loop turn so IPC and scene generation
            // never delay the menu-bar hover frame.
            hostWindow.action({
                "action": "menuBar.activate",
                "index": preview
            }, true)
        }
    }

    onMenuChanged: {
        if (menu.active !== true) {
            menuBarHoverSyncTimer.stop()
            hostWindow.menuBarPreviewIndex = -1
            hostWindow.menuBarOpenedByPointer = false
            hostWindow.menuBarPointerHasSelectedItem = false
            hostWindow.clearMenuPointerSelection()
            return
        }

        var selected = Number(menu.selected || 0)
        if (hostWindow.menuBarPreviewIndex === selected) {
            // Go acknowledged the exact item already shown locally. Drop
            // the preview without changing a pixel so later Left/Right
            // scenes can become authoritative.
            menuBarHoverSyncTimer.stop()
            hostWindow.menuBarPreviewIndex = -1
        }
        if (hostWindow.menuPointerMenuIndex >= 0
                && hostWindow.menuPointerMenuIndex !== selected
                && hostWindow.menuPointerMenuIndex
                   !== hostWindow.menuBarPreviewIndex)
            hostWindow.clearMenuPointerSelection()
    }

    function activateItem(item, hoverOnly) {
        if (!item || item.disabled === true)
            return true
        if (hoverOnly) {
            if (menu.active === true
                    && item.index !== effectiveSelected) {
                // Paint first; the short single-shot lets QML process the
                // local state and coalesces rapid passes over adjacent
                // top-level items before notifying Go.
                hostWindow.clearMenuPointerSelection()
                hostWindow.menuBarPointerHasSelectedItem = false
                hostWindow.menuBarPreviewIndex = item.index
                menuBarHoverSyncTimer.restart()
            }
        } else {
            menuBarHoverSyncTimer.stop()
            var closing = menu.active === true
                          && item.index === effectiveSelected
            hostWindow.clearMenuPointerSelection()
            hostWindow.menuBarOpenedByPointer = !closing
            hostWindow.menuBarPointerHasSelectedItem = false
            hostWindow.menuBarPreviewIndex = closing ? -1 : item.index
            hostWindow.action({
                "action": "menuBar.toggle",
                "index": item.index
            }, true)
        }
        return true
    }

    function activateAt(localX, hoverOnly) {
        for (var i = 0; i < menuItemRepeater.count; ++i) {
            var visualItem = menuItemRepeater.itemAt(i)
            if (!visualItem)
                continue
            var x1 = visualItem.mapToItem(menuBarRoot, 0, 0).x
            var x2 = x1 + visualItem.width
            if (localX >= x1 && localX < x2) {
                pointerHoverIndex = visualItem.menuIndex
                return activateItem(visualItem.menuData, hoverOnly)
            }
        }
        pointerHoverIndex = -1
        return false
    }

    function itemWindowX(menuIndex) {
        for (var i = 0; i < menuItemRepeater.count; ++i) {
            var visualItem = menuItemRepeater.itemAt(i)
            if (visualItem
                    && Number(visualItem.menuIndex) === Number(menuIndex))
                return visualItem.mapToItem(semanticLayer, 0, 0).x
        }
        var fallbackItem = hostWindow.menuBarItem(menuIndex)
        return mapToItem(semanticLayer,
                         fallbackItem ? hostWindow.pxX(fallbackItem.x) : 0,
                         0).x
    }

    function windowBottom() {
        return mapToItem(semanticLayer, 0, height).y
    }

    Row {
        id: menuItemsRow
        anchors.left: parent.left
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        spacing: 2

        Repeater {
            id: menuItemRepeater
            model: menu.items || []
            delegate: Item {
                id: menuItemHitTarget
                required property var modelData
                readonly property var menuData: modelData
                readonly property int menuIndex: Number(modelData.index)
                height: parent.height
                width: label.implicitWidth
                       + hostWindow.menuItemHorizontalPadding * 2

                function registerNativeHitTarget() {
                    if (menuItemHitTarget.nativeHitTargetRegistered
                            || !menuBarRoot.usesQwk
                            || !menuBarRoot.nativeWindowAgentReady)
                        return
                    menuBarRoot.nativeWindowAgent.setHitTestVisible(
                                menuItemHitTarget)
                    menuItemHitTarget.nativeHitTargetRegistered = true
                }

                property bool nativeHitTargetRegistered: false

                Component.onCompleted: registerNativeHitTarget()

                Rectangle {
                    anchors.fill: parent
                    anchors.leftMargin: 2
                    anchors.rightMargin: 2
                    anchors.topMargin: 3
                                       + hostWindow.titleBarContentVerticalOffset
                    anchors.bottomMargin: 3
                                          - hostWindow.titleBarContentVerticalOffset
                    radius: 5
                    color: modelData.index
                           === menuBarRoot.effectiveSelected
                           && menu.active
                           ? hostWindow.selectedBg
                           : modelData.index
                             === menuBarRoot.pointerHoverIndex
                             ? hostWindow.panelSelectionBg : "transparent"
                }

                Text {
                    id: label
                    anchors.centerIn: parent
                    anchors.verticalCenterOffset:
                        hostWindow.titleBarContentVerticalOffset
                    text: hostWindow.mnemonicText(modelData.text,
                                            modelData.hotkey)
                    textFormat: Text.StyledText
                    color: modelData.disabled ? hostWindow.mutedText : hostWindow.chromeText
                }
            }
        }
    }

    MouseArea {
        anchors.fill: parent
        acceptedButtons: Qt.LeftButton
        hoverEnabled: true
        onClicked: (mouse) => {
            if (!parent.activateAt(mouse.x, false) && menu.active === true) {
                menuBarHoverSyncTimer.stop()
                hostWindow.menuBarPreviewIndex = -1
                hostWindow.clearMenuPointerSelection()
                hostWindow.action({
                    "action": "menuBar.toggle",
                    "index": menu.selected
                }, true)
            }
        }
        onPositionChanged: (mouse) => parent.activateAt(mouse.x, true)
        onExited: parent.pointerHoverIndex = -1
    }
}
