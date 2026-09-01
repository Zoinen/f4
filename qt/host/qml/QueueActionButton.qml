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
            }

            Text {
                anchors.verticalCenter: parent.verticalCenter
                text: hostWindow.mnemonicText(queueActionButton.text,
                                        queueActionButton.mnemonicHotkey)
                textFormat: Text.StyledText
                color: queueActionButton.enabled
                       ? (queueActionButton.semanticFocus
                          ? "#f4f8fc" : hostWindow.textColor)
                       : hostWindow.mutedText
                opacity: queueActionButton.enabled ? 1 : 0.52
                font.pixelSize: 13
                font.weight: Font.Medium
                elide: Text.ElideRight
            }
        }
    }
}
