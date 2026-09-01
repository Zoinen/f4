pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

RowLayout {
    id: themeColorFooter
    required property ApplicationWindow hostWindow
    required property Window editorWindow
    required property ThemeDraftModel draft
    objectName: "themeColorFooter"
    readonly property bool compactActions:
        width < hostWindow.snapPx(675)
        || draft.statusToast !== ""
    Layout.fillWidth: false
    Layout.preferredWidth: hostWindow.snapPx(parent.width)
    Layout.preferredHeight: hostWindow.snapPx(30)
    Layout.minimumHeight: hostWindow.snapPx(30)
    Layout.maximumHeight: hostWindow.snapPx(30)
    spacing: hostWindow.snapPx(8)
    transform: Translate {
        x: hostWindow.dialogPixelOffsetX(
            themeColorFooter,
            editorWindow.contentItem)
        y: hostWindow.dialogPixelOffsetY(
            themeColorFooter,
            editorWindow.contentItem)
    }

    ConfigDialogButton {
        id: themeResetElementButton
        hostWindow: themeColorFooter.hostWindow
        objectName: "themeResetElementButton"
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeResetElementButton,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeResetElementButton,
                editorWindow.contentItem)
        }
        text: themeColorFooter.compactActions
              ? "" : "Reset Element"
        toolTipText: "Reset current element to default"
        iconSource: hostWindow.lucideIconSource(
                        "rotate-ccw", 14, hostWindow.textColor)
        onClicked: {
            if (draft.currentItem) {
                hostWindow[draft.currentItem.id] = Qt.color(draft.currentItem.defaultColor)
                draft.setFromColor(hostWindow[draft.currentItem.id])
                draft.flash(draft.currentItem.id)
            }
        }
    }

    ConfigDialogButton {
        id: themeResetAllButton
        hostWindow: themeColorFooter.hostWindow
        objectName: "themeResetAllButton"
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeResetAllButton,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeResetAllButton,
                editorWindow.contentItem)
        }
        text: themeColorFooter.compactActions
              ? "" : "Reset All"
        toolTipText: "Reset all theme settings to defaults"
        iconSource: hostWindow.lucideIconSource(
                        "refresh-cw", 14, hostWindow.textColor)
        onClicked: {
            draft.stopAllFlashing()
            hostWindow.resetThemeToDefaults()
            if (draft.currentItem)
                draft.setFromColor(hostWindow[draft.currentItem.id])
            draft.statusToast = "Reset all colors to default"
        }
    }

    ConfigDialogButton {
        id: themeRestoreSavedButton
        hostWindow: themeColorFooter.hostWindow
        objectName: "themeRestoreSavedButton"
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeRestoreSavedButton,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeRestoreSavedButton,
                editorWindow.contentItem)
        }
        text: "Restore"
        toolTipText: "Restore the last saved theme"
        iconSource: hostWindow.lucideIconSource(
                        "clock-3", 14, hostWindow.textColor)
        onClicked: {
            draft.stopAllFlashing()
            if (hostWindow.loadThemeFromPersistence()) {
                if (draft.currentItem) {
                    draft.setFromColor(
                        hostWindow[draft.currentItem.id])
                }
                draft.statusToast =
                    "Restored saved theme"
            } else {
                draft.statusToast =
                    "No saved theme found"
            }
        }
    }

    Text {
        text: draft.statusToast
        color: hostWindow.activeBorder
        font.pixelSize: 11
        Layout.fillWidth: true
        Layout.minimumWidth: 0
        horizontalAlignment: Text.AlignHCenter
        elide: Text.ElideRight
        visible: draft.statusToast !== ""
    }

    Item { Layout.fillWidth: true; visible: draft.statusToast === "" }

    ConfigDialogButton {
        id: themeSaveButton
        hostWindow: themeColorFooter.hostWindow
        objectName: "themeSaveButton"
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeSaveButton,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeSaveButton,
                editorWindow.contentItem)
        }
        text: "Save"
        toolTipText: "Save the current theme"
        highlighted: true
        iconSource: hostWindow.lucideIconSource(
                        "save", 14, "#ffffff")
        onClicked: {
            if (hostWindow.saveThemeToPersistence()) {
                draft.statusToast = "Theme saved to gui_theme.ini!"
            } else {
                draft.statusToast = "Failed to save theme"
            }
        }
    }

    ConfigDialogButton {
        id: themeCloseButton
        hostWindow: themeColorFooter.hostWindow
        objectName: "themeCloseButton"
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeCloseButton,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeCloseButton,
                editorWindow.contentItem)
        }
        text: themeColorFooter.compactActions ? "" : "Close"
        toolTipText: "Close the theme editor"
        iconSource: hostWindow.lucideIconSource(
                        "x", 14, hostWindow.textColor)
        onClicked: editorWindow.close()
    }
}
