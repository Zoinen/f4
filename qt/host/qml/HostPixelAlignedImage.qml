pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Image {
    id: image

    required property ApplicationWindow hostWindow
    property real alignmentRevision: 0

    transform: Translate {
        x: image.hostWindow.iconPixelOffsetX(image)
        y: image.hostWindow.iconPixelOffsetY(image)
    }
}
