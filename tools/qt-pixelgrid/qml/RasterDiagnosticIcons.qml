import QtQuick

Item {
    width: 570
    height: 140

    Text {
        objectName: "rasterBorderTitleLeaf"
        text: qsTr("Raster 1:1 · white fill · cyan border: 1 physical pixel")
        color: "#b8e7ff"
        font.pixelSize: 11
    }

    // Depths count additional ancestors inside this diagnostic group.
    PixelBorderIcon {
        objectName: "borderIconDepth0Leaf"
        x: 0.37
        y: 32.19
        physicalPixels: 48
    }

    Item {
        x: 125.13
        y: 31.71
        width: 90
        height: 90

        PixelBorderIcon {
            objectName: "borderIconDepth1Leaf"
            x: 0.29
            y: 0.67
            physicalPixels: 56
        }
    }

    Item {
        x: 250.31
        y: 31.29
        width: 90
        height: 90

        Item {
            x: 0.43
            y: 0.37
            width: 90
            height: 90

            PixelBorderIcon {
                objectName: "borderIconDepth2Leaf"
                x: 0.17
                y: 0.73
                physicalPixels: 64
            }
        }
    }

    Item {
        x: 375.23
        y: 30.13
        width: 90
        height: 90

        Item {
            x: 0.41
            y: 0.31
            width: 90
            height: 90

            Item {
                x: 0.29
                y: 0.47
                width: 90
                height: 90

                Item {
                    x: 0.37
                    y: 0.59
                    width: 90
                    height: 90

                    PixelBorderIcon {
                        objectName: "borderIconDepth4Leaf"
                        x: 0.19
                        y: 0.83
                        physicalPixels: 72
                    }
                }
            }
        }
    }

    Text {
        objectName: "borderIconDepth0LabelLeaf"
        x: 0.37
        y: 108.23
        text: qsTr("Depth 0 · 48 px")
        color: "white"
        font.pixelSize: 10
    }
    Text {
        objectName: "borderIconDepth1LabelLeaf"
        x: 125.13
        y: 108.23
        text: qsTr("Depth 1 · 56 px")
        color: "white"
        font.pixelSize: 10
    }
    Text {
        objectName: "borderIconDepth2LabelLeaf"
        x: 250.31
        y: 108.23
        text: qsTr("Depth 2 · 64 px")
        color: "white"
        font.pixelSize: 10
    }
    Text {
        objectName: "borderIconDepth4LabelLeaf"
        x: 375.23
        y: 108.23
        text: qsTr("Depth 4 · 72 px")
        color: "white"
        font.pixelSize: 10
    }
}
