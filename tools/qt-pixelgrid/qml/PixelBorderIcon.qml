import QtQuick

Image {
    id: root

    property int physicalPixels: 56

    // Native raster sizing only; all positions remain deliberately unsnapped.
    width: physicalPixels / Screen.devicePixelRatio
    height: physicalPixels / Screen.devicePixelRatio
    source: "image://pixel/border/" + physicalPixels
    sourceSize: Qt.size(width, height)
    fillMode: Image.Stretch
    smooth: true
    mipmap: false
    Accessible.ignored: true
}
