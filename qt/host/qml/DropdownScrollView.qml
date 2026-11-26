import QtQuick
import QtQuick.Controls
import ZoinGallery 1.0 as ZG

ZG.GalleryDropdownScrollView {
    id: view
    required property ApplicationWindow hostWindow
    ScrollBar.vertical: F4ScrollBar {
        id: verticalBar
        hostWindow: view.hostWindow
        objectName: view.menuObjectName + "ScrollBar"
        parent: view
        x: view.width - width
        y: view.topPadding
        height: view.availableHeight
        thickness: 8
        visible: view.overflow
        policy: ScrollBar.AsNeeded
    }
}
