pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl

Rectangle {
    id: keyBarRoot
    required property ApplicationWindow hostWindow
    required property QtObject shellController
    required property Item focusTarget
    required property var keyBar
    objectName: "keyBar"
    color: hostWindow.fBarBg
    visible: keyBar.visible !== false && keyBar.items !== undefined

    function openAlternativeMenu(anchorItem, item, functionKey,
                                 functionIndex) {
        const alternatives = item && item.alternatives
                ? item.alternatives : []
        const activeModifier = hostWindow.cleanText(keyBar.modifier || "normal")
                .trim().toLowerCase()
        var rows = []
        for (var i = 0; i < alternatives.length; ++i) {
            const alternative = alternatives[i] || ({})
            const modifier = hostWindow.cleanText(alternative.modifier)
                    .trim().toLowerCase()
            const text = hostWindow.cleanText(alternative.text)
            if (text === "" || modifier === activeModifier)
                continue
            rows.push({
                "text": text,
                "icon": hostWindow.cleanText(alternative.icon),
                "modifier": modifier,
                "shortcut": hostWindow.keyBarModifierShortcut(
                                 functionKey, modifier)
            })
        }
        if (rows.length === 0)
            return false
        if (keyBarAlternativeMenu.opened)
            keyBarAlternativeMenu.close()
        keyBarAlternativeMenu.anchorItem = anchorItem
        keyBarAlternativeMenu.functionKey = functionKey
        keyBarAlternativeMenu.functionIndex = functionIndex
        keyBarAlternativeMenu.menuItems = rows
        keyBarAlternativeMenu.open()
        return true
    }

    // This is application chrome, not panel/document content. Keeping the
    // separator in the shared F-bar makes panels, viewer and editor end at
    // one identical boundary.
    Rectangle {
        objectName: "keyBarTopSeparator"
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        height: hostWindow.separatorWidth
        color: hostWindow.separatorColor
        antialiasing: false
        z: 2
    }

    Item {
        anchors.fill: parent
        anchors.topMargin: hostWindow.snapPx(hostWindow.actionBarVerticalMargin)
        anchors.bottomMargin: hostWindow.snapPx(hostWindow.actionBarVerticalMargin)
        Repeater {
            model: (keyBar.items || []).length
            delegate: KeyBarActionButton {
                id: actionButton
                required property int index
                readonly property var modelData: (keyBarRoot.keyBar.items || [])[index] || ({})
                readonly property string functionKey: hostWindow.cleanText(modelData.key) !== ""
                    ? hostWindow.cleanText(modelData.key) : "F" + String(index + 1)
                readonly property int functionIndex: hostWindow.keyBarFunctionIndex({ "key": functionKey }, index)
                hostWindow: keyBarRoot.hostWindow
                objectName: "key-bar-action-" + (functionIndex + 1)
                labelObjectName: "key-bar-label-" + (functionIndex + 1)
                shortcutObjectName: "key-bar-shortcut-" + (functionIndex + 1)
                iconObjectName: "key-bar-icon-" + (functionIndex + 1)
                separatorObjectName: "key-bar-separator-" + (functionIndex + 1)
                x: hostWindow.snapPx(index * parent.width / 12)
                width: hostWindow.snapPx((index + 1) * parent.width / 12) - x
                height: parent.height
                label: hostWindow.mnemonicText(modelData.text, modelData.hotkey)
                shortcut: functionKey
                iconName: hostWindow.cleanText(modelData.icon)
                separatorVisible: index < (keyBarRoot.keyBar.items || []).length - 1
                highlighted: keyBarAlternativeMenu.opened && keyBarAlternativeMenu.functionIndex === functionIndex
                onClicked: function(mouse) {
                    if (mouse.button === Qt.RightButton) {
                        keyBarRoot.openAlternativeMenu(actionButton, modelData, functionKey, functionIndex)
                        return
                    }
                    const keyNumber = Number(functionKey.replace(/^F/i, ""))
                    const clickedIndex = !isNaN(keyNumber) && keyNumber >= 1 && keyNumber <= 24
                        ? keyNumber - 1 : functionIndex
                    const vk = 0x70 + clickedIndex
                    const mods = hostWindow.vtuiKeyModifiers(mouse.modifiers)
                    shellController.sendKey(vk, 0, true, mods)
                    shellController.sendKey(vk, 0, false, mods)
                    focusTarget.forceActiveFocus()
                }
            }
        }
    }

    Popup {
        id: keyBarAlternativeMenu
        objectName: "keyBarAlternativeMenu"
        parent: Overlay.overlay
        property var anchorItem: null
        property string functionKey: ""
        property int functionIndex: -1
        property var menuItems: []
        readonly property real menuRowHeight: hostWindow.snapPx(31)

        function preferredWidth() {
            var widestLabel = 0
            var widestShortcut = 0
            for (var i = 0; i < menuItems.length; ++i) {
                const item = menuItems[i] || ({})
                widestLabel = Math.max(widestLabel,
                    keyBarAlternativeFontMetrics.advanceWidth(
                        hostWindow.cleanText(item.text)))
                widestShortcut = Math.max(widestShortcut,
                    keyBarAlternativeFontMetrics.advanceWidth(
                        hostWindow.cleanText(item.shortcut)))
            }
            return Math.max(hostWindow.snapPx(220),
                            hostWindow.snapPx(62) + widestLabel
                            + widestShortcut)
        }

        width: Math.min(hostWindow.width - hostWindow.snapPx(12), preferredWidth())
        implicitHeight: menuItems.length * menuRowHeight
                         + topPadding + bottomPadding
        height: Math.min(hostWindow.height - hostWindow.snapPx(12),
                         Math.max(menuRowHeight + topPadding + bottomPadding,
                                  implicitHeight))
        padding: hostWindow.snapPx(6)
        modal: false
        dim: false
        focus: false
        z: 1001
        closePolicy: Popup.CloseOnEscape
                     | Popup.CloseOnPressOutside
                     | Popup.CloseOnPressOutsideParent

        onAboutToShow: {
            if (!anchorItem) {
                close()
                return
            }
            const point = anchorItem.mapToItem(
                hostWindow.contentItem, anchorItem.width / 2, 0)
            x = Math.max(hostWindow.snapPx(6), Math.min(
                hostWindow.width - width - hostWindow.snapPx(6),
                point.x - width / 2))
            var popupY = point.y - height - hostWindow.snapPx(4)
            if (popupY < hostWindow.snapPx(6))
                popupY = point.y + anchorItem.height + hostWindow.snapPx(4)
            y = Math.max(hostWindow.snapPx(6), Math.min(
                hostWindow.height - height - hostWindow.snapPx(6), popupY))
        }

        onClosed: {
            anchorItem = null
            functionKey = ""
            functionIndex = -1
            menuItems = []
        }

        background: Rectangle {
            color: hostWindow.dialogHeaderBg
            radius: hostWindow.snapPx(8)
            border.width: hostWindow.separatorWidth
            border.color: hostWindow.controlBorder
        }

        FontMetrics {
            id: keyBarAlternativeFontMetrics
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: 12
        }

        contentItem: ListView {
            id: keyBarAlternativeList
            objectName: "keyBarAlternativeList"
            anchors.fill: parent
            clip: true
            model: keyBarAlternativeMenu.menuItems
            boundsBehavior: Flickable.StopAtBounds

            delegate: Rectangle {
                id: keyBarAlternativeRow
                required property var modelData
                objectName: "keyBarAlternative-"
                            + hostWindow.cleanText(modelData.modifier)
                width: keyBarAlternativeList.width
                height: keyBarAlternativeMenu.menuRowHeight
                radius: hostWindow.snapPx(5)
                color: keyBarAlternativeMouse.containsMouse
                       ? hostWindow.controlHoverBg : "transparent"

                Item {
                    id: keyBarAlternativeIconSlot
                    anchors.left: parent.left
                    anchors.leftMargin: hostWindow.snapPx(8)
                    anchors.verticalCenter: parent.verticalCenter
                    width: hostWindow.snapPx(18)
                    height: hostWindow.snapPx(18)

                    HostPixelAlignedImage {
                    hostWindow: keyBarRoot.hostWindow
                        objectName: "keyBarAlternativeIcon-"
                                    + hostWindow.cleanText(modelData.modifier)
                        anchors.centerIn: parent
                        width: hostWindow.snapPx(16)
                        height: hostWindow.snapPx(16)
                        visible: hostWindow.cleanText(modelData.icon) !== ""
                        smooth: false
                        mipmap: false
                        source: hostWindow.lucideIconSource(
                                    hostWindow.cleanText(modelData.icon), 16,
                                    keyBarAlternativeMouse.containsMouse
                                    ? hostWindow.textColor : hostWindow.chromeText)
                    }
                }

                Text {
                    id: keyBarAlternativeLabel
                    objectName: "keyBarAlternativeLabel-"
                                + hostWindow.cleanText(modelData.modifier)
                    anchors.left: keyBarAlternativeIconSlot.right
                    anchors.right: keyBarAlternativeShortcut.left
                    anchors.leftMargin: hostWindow.snapPx(8)
                    anchors.rightMargin: hostWindow.snapPx(12)
                    anchors.verticalCenter: parent.verticalCenter
                    text: hostWindow.cleanText(modelData.text)
                    color: hostWindow.textColor
                    font.family: hostWindow.guiMonospaceFontFamily
                    font.pixelSize: 12
                    elide: Text.ElideRight
                }

                Text {
                    id: keyBarAlternativeShortcut
                    objectName: "keyBarAlternativeShortcut-"
                                + hostWindow.cleanText(modelData.modifier)
                    anchors.right: parent.right
                    anchors.rightMargin: hostWindow.snapPx(10)
                    anchors.verticalCenter: parent.verticalCenter
                    text: hostWindow.cleanText(modelData.shortcut)
                    color: hostWindow.mutedText
                    font.family: hostWindow.guiMonospaceFontFamily
                    font.pixelSize: 11
                }

                MouseArea {
                    id: keyBarAlternativeMouse
                    anchors.fill: parent
                    acceptedButtons: Qt.LeftButton
                    hoverEnabled: true
                    onClicked: {
                        const keyNumber = Number(
                            keyBarAlternativeMenu.functionKey
                                .replace(/^F/i, ""))
                        const clickedIndex = !isNaN(keyNumber)
                                && keyNumber >= 1 && keyNumber <= 24
                                ? keyNumber - 1
                                : keyBarAlternativeMenu.functionIndex
                        const vk = 0x70 + clickedIndex
                        const modifiers = hostWindow.keyBarModifierFlags(
                            modelData.modifier)
                        shellController.sendKey(vk, 0, true, modifiers)
                        shellController.sendKey(vk, 0, false, modifiers)
                        keyBarAlternativeMenu.close()
                    }
                }
            }
        }
    }

    MouseArea {
        // Popup.CloseOnPressOutside is not delivered reliably when a
        // non-windowed popup is parented to Overlay.overlay above the
        // persistent panel/grid pointer layers. Keep the dismiss plane
        // immediately below the popup so outside presses always close it
        // without moving the file-panel cursor underneath.
        parent: Overlay.overlay
        anchors.fill: parent
        visible: keyBarAlternativeMenu.opened
        enabled: visible
        z: 1000
        acceptedButtons: Qt.LeftButton | Qt.RightButton
                         | Qt.MiddleButton
        onPressed: keyBarAlternativeMenu.close()
    }
}
