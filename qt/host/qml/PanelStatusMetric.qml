pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: metric
    required property ApplicationWindow hostWindow
    property string iconName: ""
    property string text: ""
    property color textColor: hostWindow.mutedText
    property color iconColor: textColor
    readonly property real naturalWidth: hostWindow.snapPx(20)
        + Math.ceil(label.implicitWidth * hostWindow.dpr) / hostWindow.dpr
    implicitWidth: naturalWidth
    implicitHeight: hostWindow.snapPx(20)

    HostPixelAlignedImage {
        id: icon
        objectName: metric.objectName + "Icon"
        hostWindow: metric.hostWindow
        width: hostWindow.snapPx(14)
        height: width
        y: hostWindow.snapPx((metric.height - height) / 2)
        source: hostWindow.lucideIconSource(metric.iconName, 14, metric.iconColor)
        smooth: false
        mipmap: false
    }
    Text {
        id: label
        objectName: metric.objectName + "Text"
        x: metric.hostWindow.snapPx(20)
        y: metric.hostWindow.snapPx((metric.height - height) / 2)
        width: Math.max(0, metric.width - x)
        text: metric.text
        color: metric.textColor
        font.family: metric.hostWindow.font.family
        font.pixelSize: 12
        elide: Text.ElideMiddle
        transform: Translate {
            x: metric.hostWindow.dialogPixelOffsetX(label, metric.hostWindow.contentItem)
            y: metric.hostWindow.dialogPixelOffsetY(label, metric.hostWindow.contentItem)
        }
    }
}
