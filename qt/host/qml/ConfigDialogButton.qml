pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.impl
import QtQuick.Layouts
import ZoinGallery 1.0 as ZG

Rectangle {
    id: cBtn
    required property ApplicationWindow hostWindow
    property string text: ""
    property string iconSource: ""
    property string toolTipText: text
    property bool highlighted: false
    signal clicked()

    implicitHeight: hostWindow.snapPx(30)
    implicitWidth: hostWindow.snapPx(cBtnRow.implicitWidth + 20)
    Layout.preferredHeight: implicitHeight
    Layout.preferredWidth: implicitWidth
    Layout.minimumHeight: implicitHeight
    Layout.maximumHeight: implicitHeight
    radius: hostWindow.snapPx(4)

    color: cBtnMouse.pressed
           ? (highlighted ? Qt.darker(hostWindow.dialogAccent, 1.2) : "#38ffffff")
           : cBtnMouse.containsMouse
             ? (highlighted ? Qt.lighter(hostWindow.dialogAccent, 1.1) : "#24ffffff")
             : (highlighted ? hostWindow.dialogAccent : "#14ffffff")

    border.width: hostWindow.separatorWidth
    border.color: highlighted
                  ? hostWindow.dialogAccent
                  : (cBtnMouse.containsMouse ? hostWindow.controlHoverBg : hostWindow.controlBorder)
    Accessible.role: Accessible.Button
    Accessible.name: toolTipText

    ZG.ToolTip {
        visible: cBtnMouse.containsMouse && cBtn.toolTipText !== ""
        delay: 500
        timeout: 5000
        text: cBtn.toolTipText
    }

    RowLayout {
        id: cBtnRow
        x: hostWindow.snapPx((parent.width - width) / 2)
        y: hostWindow.snapPx((parent.height - height) / 2)
        spacing: hostWindow.snapPx(6)

        IconLabel {
            icon.source: cBtn.iconSource
            icon.width: hostWindow.snapPx(14)
            icon.height: hostWindow.snapPx(14)
            icon.color: cBtn.highlighted ? "#ffffff" : hostWindow.textColor
            visible: cBtn.iconSource !== ""
        }

        Text {
            text: cBtn.text
            color: cBtn.highlighted ? "#ffffff" : hostWindow.textColor
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: 11
            font.weight: cBtn.highlighted ? Font.Bold : Font.Normal
            verticalAlignment: Text.AlignVCenter
        }
    }

    MouseArea {
        id: cBtnMouse
        anchors.fill: parent
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: cBtn.clicked()
    }
}
