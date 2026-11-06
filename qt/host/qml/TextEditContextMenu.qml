pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQml.Models

F4Menu {
    id: menu
    required property ApplicationWindow hostWindow
    property var editor: null
    property var actionHandler: null
    property bool editable: editor !== null && !editor.readOnly
    property bool hasSelection: editor !== null && editor.selectedText.length > 0
    property bool hasText: editor !== null && editor.text.length > 0
    property bool undoAvailable: editable && editor !== null && editor.canUndo
    property bool redoAvailable: editable && editor !== null && editor.canRedo
    property bool pasteAvailable: editable && editor !== null && editor.canPaste
    property point popupPoint: Qt.point(0, 0)
    property bool prepared: false
    readonly property var entries: [
        {name: "undo", text: qsTr("Undo"), icon: "undo-2", enabled: undoAvailable},
        {name: "redo", text: qsTr("Redo"), icon: "redo-2", enabled: redoAvailable},
        {name: "first-separator", separator: true},
        {name: "cut", text: qsTr("Cut"), icon: "scissors", enabled: editable && hasSelection},
        {name: "copy", text: qsTr("Copy"), icon: "copy", enabled: hasSelection},
        {name: "paste", text: qsTr("Paste"), icon: "clipboard-paste", enabled: pasteAvailable},
        {name: "delete", text: qsTr("Delete"), icon: "trash-2", enabled: editable && hasSelection},
        {name: "last-separator", separator: true},
        {name: "selectAll", text: qsTr("Select All"), icon: "text-cursor-input", enabled: hasText}
    ]
    parent: Overlay.overlay
    popupType: Popup.Item
    width: hostWindow.snapPx(220)
    x: hostWindow.snapPx(Math.max(6, Math.min(hostWindow.width - width - 6, popupPoint.x)))
    y: hostWindow.snapPx(Math.max(6, Math.min(hostWindow.height - height - 6, popupPoint.y)))
    padding: hostWindow.snapPx(6)
    modal: false
    dim: false
    focus: true
    z: 1100
    // Menu closes before MenuItem.triggered on some Qt styles. Retire the
    // lazy delegates after that activation finishes, not during its release.
    onClosed: Qt.callLater(function() { if (!menu.visible) menu.prepared = false })

    function openAt(target, point) {
        prepared = true
        popupPoint = target.mapToItem(hostWindow.contentItem, point.x, point.y)
        console.debug("[FIX:text-context-menu] open", target.objectName)
        open()
    }
    function activate(name) {
        console.debug("[FIX:text-context-menu] action", name)
        if (actionHandler) {
            actionHandler(name)
            return
        }
        activateNative(name)
    }
    function activateNative(name) {
        if (!editor) return
        switch (name) {
        case "undo": editor.undo(); break
        case "redo": editor.redo(); break
        case "cut": editor.cut(); break
        case "copy": editor.copy(); break
        case "paste": editor.paste(); break
        case "delete": editor.remove(editor.selectionStart, editor.selectionEnd); break
        case "selectAll": editor.selectAll(); break
        }
        editor.forceActiveFocus()
    }
    background: Rectangle {
        objectName: menu.objectName + "Background"
        color: menu.hostWindow.controlBg
        radius: menu.hostWindow.snapPx(8)
        border.width: menu.hostWindow.snapPx(1)
        border.color: menu.hostWindow.controlBorder
    }
    Instantiator {
        active: menu.prepared || menu.visible
        model: menu.entries
        onObjectAdded: (index, object) => menu.insertItem(index, object)
        onObjectRemoved: (index, object) => menu.removeItem(object)
        delegate: T.MenuItem {
            id: entry
            required property var modelData
            objectName: menu.objectName + "-" + modelData.name
            width: menu.availableWidth
            implicitWidth: menu.hostWindow.snapPx(208)
            implicitHeight: menu.hostWindow.snapPx(modelData.separator ? 9 : 31)
            text: modelData.text || ""
            enabled: modelData.enabled === true
            hoverEnabled: true
            padding: menu.hostWindow.snapPx(8)
            indicator: null
            arrow: null
            onTriggered: menu.activate(modelData.name)
            background: Rectangle {
                radius: menu.hostWindow.snapPx(4)
                color: entry.highlighted && entry.enabled ? menu.hostWindow.controlHoverBg : "transparent"
                Rectangle {
                    visible: entry.modelData.separator === true
                    x: menu.hostWindow.snapPx(2)
                    width: parent.width - 2 * x
                    y: menu.hostWindow.snapPx(parent.height / 2)
                    height: menu.hostWindow.snapPx(1)
                    color: menu.hostWindow.separatorColor
                }
            }
            contentItem: Item {
                visible: entry.modelData.separator !== true
                opacity: entry.enabled ? 1 : .4
                HostPixelAlignedImage {
                    id: icon
                    hostWindow: menu.hostWindow
                    objectName: entry.objectName + "Icon"
                    width: menu.hostWindow.snapPx(16)
                    height: width
                    y: menu.hostWindow.snapPx((parent.height - height) / 2)
                    source: entry.modelData.icon ? menu.hostWindow.lucideIconSource(entry.modelData.icon, 16, menu.hostWindow.textColor) : ""
                }
                Text {
                    id: label
                    x: icon.width + menu.hostWindow.snapPx(8)
                    width: parent.width - x
                    y: (parent.height - height) / 2
                    text: entry.text
                    color: menu.hostWindow.textColor
                    font.family: menu.hostWindow.uiFontFamily
                    font.pixelSize: menu.hostWindow.uiTextSize(12)
                    renderType: menu.hostWindow.fontRenderType
                    transform: Translate {
                        x: menu.hostWindow.dialogPixelOffsetX(label, menu.hostWindow.contentItem)
                        y: menu.hostWindow.dialogPixelOffsetY(label, menu.hostWindow.contentItem)
                    }
                }
            }
        }
    }
}
