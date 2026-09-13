pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.Menu {
    id: applicationMenu
    required property ApplicationWindow hostWindow
    objectName: "applicationMenuPopup"
    popupType: Popup.Item
    modal: true
    dim: false
    width: hostWindow.snapPx(230)
    padding: hostWindow.snapPx(4)
    delegate: menuEntry
    background: Loader { sourceComponent: menuBackground }
    onClosed: if (hostWindow.focusTarget) hostWindow.focusTarget.forceActiveFocus()

    Component {
        id: menuBackground
        Rectangle {
            color: applicationMenu.hostWindow.titleBarBg
            border.color: applicationMenu.hostWindow.separatorColor
            border.width: applicationMenu.hostWindow.separatorWidth
            radius: applicationMenu.hostWindow.snapPx(5)
        }
    }

    Component {
        id: menuEntry
        T.MenuItem {
            id: entry
            objectName: "applicationMenuEntry-" + text
            property string shortcutText: ""
            property string iconName: ""
            property bool separatorRow: false
            implicitWidth: applicationMenu.hostWindow.snapPx(300)
            implicitHeight: applicationMenu.hostWindow.snapPx(separatorRow ? 9 : 32)
            leftPadding: applicationMenu.hostWindow.snapPx(iconName !== "" ? 48 : 24)
            rightPadding: applicationMenu.hostWindow.snapPx(24)
            topPadding: 0
            bottomPadding: 0
            arrow: null
            indicator: null
            background: Rectangle {
                color: entry.highlighted ? applicationMenu.hostWindow.selectedBg : "transparent"
                Rectangle {
                    visible: entry.separatorRow
                    x: applicationMenu.hostWindow.snapPx(4)
                    y: applicationMenu.hostWindow.snapPx(parent.height / 2)
                    width: parent.width - 2 * x
                    height: applicationMenu.hostWindow.separatorWidth
                    color: applicationMenu.hostWindow.separatorColor
                }
            }
            contentItem: Item {
                HostPixelAlignedImage {
                    hostWindow: applicationMenu.hostWindow
                    objectName: entry.objectName + "-icon"
                    visible: entry.iconName !== ""
                    x: -applicationMenu.hostWindow.snapPx(24)
                    y: applicationMenu.hostWindow.snapPx((parent.height - height) / 2)
                    width: applicationMenu.hostWindow.snapPx(16)
                    height: width
                    source: visible ? applicationMenu.hostWindow.lucideIconSource(entry.iconName, 16,
                        applicationMenu.hostWindow.chromeText) : ""
                }
                Text {
                    id: caption
                    objectName: entry.objectName + "-text"
                    visible: !entry.separatorRow
                    anchors.left: parent.left
                    anchors.right: shortcut.left
                    anchors.rightMargin: applicationMenu.hostWindow.snapPx(12)
                    y: (parent.height - height) / 2
                    text: entry.text
                    elide: Text.ElideRight
                    color: entry.enabled ? applicationMenu.hostWindow.chromeText : applicationMenu.hostWindow.mutedText
                    font.pixelSize: applicationMenu.hostWindow.semanticTextFontPixelSize
                    renderType: applicationMenu.hostWindow.fontRenderType
                    transform: Translate {
                        x: applicationMenu.hostWindow.dialogPixelOffsetX(caption, applicationMenu.hostWindow.contentItem)
                        y: applicationMenu.hostWindow.dialogPixelOffsetY(caption, applicationMenu.hostWindow.contentItem)
                    }
                }
                Text {
                    id: shortcut
                    objectName: entry.objectName + "-shortcut"
                    visible: text !== ""
                    anchors.right: parent.right
                    y: (parent.height - height) / 2
                    text: entry.shortcutText
                    color: applicationMenu.hostWindow.mutedText
                    font.pixelSize: applicationMenu.hostWindow.semanticTextFontPixelSize
                    renderType: applicationMenu.hostWindow.fontRenderType
                    transform: Translate {
                        x: applicationMenu.hostWindow.dialogPixelOffsetX(shortcut, applicationMenu.hostWindow.contentItem)
                        y: applicationMenu.hostWindow.dialogPixelOffsetY(shortcut, applicationMenu.hostWindow.contentItem)
                    }
                }
                Text {
                    id: marker
                    objectName: entry.objectName + "-marker"
                    visible: entry.subMenu !== null || entry.checked
                    x: entry.subMenu ? parent.width + applicationMenu.hostWindow.snapPx(8)
                                    : -applicationMenu.hostWindow.snapPx(entry.iconName !== "" ? 42 : 18)
                    y: (parent.height - height) / 2
                    text: entry.subMenu ? "›" : "✓"
                    color: applicationMenu.hostWindow.chromeText
                    font.pixelSize: applicationMenu.hostWindow.semanticTextFontPixelSize
                    renderType: applicationMenu.hostWindow.fontRenderType
                    transform: Translate {
                        x: applicationMenu.hostWindow.dialogPixelOffsetX(marker, applicationMenu.hostWindow.contentItem)
                        y: applicationMenu.hostWindow.dialogPixelOffsetY(marker, applicationMenu.hostWindow.contentItem)
                    }
                }
            }
        }
    }

    Instantiator {
        model: applicationMenu.hostWindow.menuBarModel.items || []
        delegate: T.Menu {
            id: category
            required property var modelData
            objectName: "applicationSubmenu-" + Number(modelData.index)
            title: modelData.text || ""
            enabled: modelData.disabled !== true
            popupType: Popup.Item
            width: applicationMenu.hostWindow.snapPx(360)
            padding: applicationMenu.hostWindow.snapPx(4)
            delegate: menuEntry
            background: Loader { sourceComponent: menuBackground }
            Instantiator {
                model: category.modelData.items || []
                delegate: Loader {
                    required property var modelData
                    sourceComponent: menuEntry
                    onLoaded: {
                        item.text = Qt.binding(() => modelData.text || "")
                        item.shortcutText = Qt.binding(() => modelData.shortcut || "")
                        item.iconName = Qt.binding(() => modelData.icon || "")
                        item.separatorRow = Qt.binding(() => modelData.separator === true)
                        item.enabled = Qt.binding(() => !modelData.disabled && !modelData.separator && !modelData.header)
                        item.checked = Qt.binding(() => modelData.checked === true)
                        item.triggered.connect(() => applicationMenu.hostWindow.action({
                            action: "menuBar.itemActivate", menuIndex: Number(category.modelData.index),
                            index: Number(modelData.index)
                        }, true))
                    }
                }
                onObjectAdded: (index, object) => category.insertItem(index, object.item)
                onObjectRemoved: (index, object) => category.removeItem(object.item)
            }
        }
        onObjectAdded: (index, object) => applicationMenu.insertMenu(index, object)
        onObjectRemoved: (index, object) => applicationMenu.removeMenu(object)
    }
}
