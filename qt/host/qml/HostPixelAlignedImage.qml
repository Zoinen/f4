pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Image {
    id: image

    required property ApplicationWindow hostWindow
    property real alignmentRevision: 0

    transform: Translate {
        x: image.hostWindow.dialogPixelOffsetX(image, image.hostWindow.contentItem)
        y: image.hostWindow.dialogPixelOffsetY(image, image.hostWindow.contentItem)
    }
}
