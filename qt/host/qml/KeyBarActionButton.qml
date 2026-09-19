pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Rectangle {
    id: button
    required property ApplicationWindow hostWindow
    property string label: ""
    property int labelFormat: Text.StyledText
    property string shortcut: ""
    property string iconName: ""
    property string labelObjectName: ""
    property string shortcutObjectName: ""
    property string iconObjectName: ""
    property string separatorObjectName: ""
    property bool separatorVisible: true
    property bool highlighted: false
    property color labelColor: hostWindow.chromeText
    property color shortcutColor: hostWindow.mutedText
    property color shortcutHoverColor: hostWindow.textColor
    property int shortcutWeight: Font.Normal
    property alias labelFont: actionLabel.font
    property alias shortcutFont: shortcutLabel.font
    readonly property real naturalWidth: hostWindow.snapPx(
        labelMetrics.advanceWidth + shortcutMetrics.advanceWidth + 7
        + 2 * hostWindow.actionButtonHorizontalMargin
        + (iconName !== "" ? 21 : 0)) + hostWindow.separatorWidth
    signal clicked(var mouse)

    radius: hostWindow.snapPx(5)
    color: pointer.pressed ? hostWindow.panelSelectionBorder
           : pointer.containsMouse || highlighted ? hostWindow.panelSelectionBg : "transparent"
    opacity: enabled ? 1 : 0.5

    TextMetrics { id: labelMetrics; text: button.label; font: button.labelFont }
    TextMetrics { id: shortcutMetrics; text: button.shortcut; font: button.shortcutFont }

    HostPixelAlignedImage {
        id: actionIcon
        hostWindow: button.hostWindow
        objectName: button.iconObjectName
        anchors.left: parent.left
        anchors.verticalCenter: parent.verticalCenter
        anchors.leftMargin: hostWindow.snapPx(hostWindow.actionButtonHorizontalMargin)
        width: visible ? hostWindow.snapPx(14) : 0
        height: width
        visible: button.iconName !== ""
        smooth: false
        mipmap: false
        alignmentRevision: button.x + button.y + button.width + button.height
        source: button.iconName === "" ? "" : hostWindow.lucideIconSource(button.iconName, 14,
                    pointer.containsMouse ? hostWindow.textColor : hostWindow.chromeText)
    }
    Text {
        id: actionLabel
        objectName: button.labelObjectName
        anchors.left: actionIcon.visible ? actionIcon.right : parent.left
        anchors.leftMargin: actionIcon.visible ? hostWindow.snapPx(7) : hostWindow.actionButtonHorizontalMargin
        anchors.right: shortcutLabel.left
        anchors.rightMargin: hostWindow.snapPx(7)
        anchors.verticalCenter: parent.verticalCenter
        text: button.label
        textFormat: button.labelFormat
        color: button.labelColor
        font.pixelSize: 11
        elide: Text.ElideRight
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(actionLabel, hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(actionLabel, hostWindow.contentItem)
        }
    }
    Text {
        id: shortcutLabel
        objectName: button.shortcutObjectName
        anchors.right: parent.right
        anchors.rightMargin: hostWindow.actionButtonHorizontalMargin
        anchors.verticalCenter: parent.verticalCenter
        text: button.shortcut
        color: pointer.containsMouse ? button.shortcutHoverColor : button.shortcutColor
        font.pixelSize: 11
        font.weight: button.shortcutWeight
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(shortcutLabel, hostWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(shortcutLabel, hostWindow.contentItem)
        }
    }
    Rectangle {
        objectName: button.separatorObjectName
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.topMargin: hostWindow.snapPx(hostWindow.actionSeparatorVerticalMargin)
        anchors.bottomMargin: hostWindow.snapPx(hostWindow.actionSeparatorVerticalMargin)
        width: hostWindow.separatorWidth
        color: hostWindow.separatorColor
        visible: button.separatorVisible
        antialiasing: false
    }
    MouseArea {
        id: pointer
        anchors.fill: parent
        hoverEnabled: true
        acceptedButtons: Qt.LeftButton | Qt.RightButton
        cursorShape: Qt.PointingHandCursor
        onClicked: function(mouse) { button.clicked(mouse) }
    }
}
