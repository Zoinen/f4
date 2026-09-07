pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.impl

DialogButton {
    id: queueActionButton
    property string iconName: ""
    readonly property bool f4Themed: true

    focusPolicy: Qt.StrongFocus
    semanticFocus: activeFocus
    implicitWidth: Math.max(108, queueActionContent.implicitWidth + 24)

    contentItem: Item {
        id: queueActionContent
        implicitWidth: queueActionRow.implicitWidth
        implicitHeight: queueActionRow.implicitHeight

        Row {
            id: queueActionRow
            anchors.centerIn: parent
            spacing: 7

            IconLabel {
                id: queueActionIcon
                objectName: queueActionButton.objectName
                            ? (queueActionButton.objectName + "Icon")
                            : "queueActionButtonIcon"
                visible: queueActionButton.iconName !== ""
                width: visible ? 15 : 0
                height: 15
                anchors.verticalCenter: parent.verticalCenter
                icon.source: hostWindow.lucideIconSource(
                                 queueActionButton.iconName, 15,
                                 queueActionButton.enabled
                                 ? (queueActionButton.semanticFocus
                                    ? "#f4f8fc" : hostWindow.textColor)
                                 : hostWindow.mutedText)
                icon.width: 15
                icon.height: 15
                icon.color: queueActionButton.enabled
                            ? (queueActionButton.semanticFocus
                               ? "#f4f8fc" : hostWindow.textColor)
                            : hostWindow.mutedText
                opacity: queueActionButton.enabled ? 1 : 0.52
                transform: Translate {
                    x: queueActionButton.hostWindow
                       ? queueActionButton.hostWindow.dialogPixelOffsetX(
                             queueActionIcon,
                             queueActionButton.hostWindow.contentItem) : 0
                    y: queueActionButton.hostWindow
                       ? queueActionButton.hostWindow.dialogPixelOffsetY(
                             queueActionIcon,
                             queueActionButton.hostWindow.contentItem) : 0
                }
            }

            Text {
                id: queueActionText
                objectName: queueActionButton.objectName
                            ? (queueActionButton.objectName + "Text")
                            : "queueActionButtonText"
                anchors.verticalCenter: parent.verticalCenter
                text: hostWindow.mnemonicText(queueActionButton.text,
                                        queueActionButton.mnemonicHotkey)
                textFormat: Text.StyledText
                color: queueActionButton.enabled
                       ? (queueActionButton.semanticFocus
                          ? "#f4f8fc" : hostWindow.textColor)
                       : hostWindow.mutedText
                opacity: queueActionButton.enabled ? 1 : 0.52
                font: queueActionButton.font
                elide: Text.ElideRight
                transform: Translate {
                    x: queueActionButton.hostWindow
                       ? queueActionButton.hostWindow.dialogPixelOffsetX(
                             queueActionText,
                             queueActionButton.hostWindow.contentItem) : 0
                    y: queueActionButton.hostWindow
                       ? queueActionButton.hostWindow.dialogPixelOffsetY(
                             queueActionText,
                             queueActionButton.hostWindow.contentItem) : 0
                }
            }
        }
    }
}
