pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

T.ProgressBar {
    id: dialogProgress

    readonly property var dialogColors: dialogColorContext.colors
    DialogColorContext {
        id: dialogColorContext
        control: dialogProgress
    }
    required property ApplicationWindow hostWindow

    background: Rectangle {
        implicitHeight: 8
        radius: 4
        color: dialogProgress.dialogColors.controlPressedBg
        border.width: 1
        border.color: dialogProgress.dialogColors.controlBorder
    }

    contentItem: Item {
        implicitHeight: 8
        clip: true

        Rectangle {
            width: dialogProgress.visualPosition * parent.width
            height: parent.height
            radius: 4
            color: dialogProgress.dialogColors.dialogAccent
        }
    }
}
