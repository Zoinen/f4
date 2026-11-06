import QtQuick
import QtQuick.Controls.Basic
import QtPixelGrid.Proof

Item {
    id: root
    required property bool moving
    readonly property real panelHeight: 96 / Screen.devicePixelRatio
    width: 600
    height: panelHeight + 82

    Text {
        objectName: "galleryShaderTitleLeaf"
        text: qsTr("Gallery · ShaderEffect · 1 px strokes")
        color: "#b8e7ff"
        font.pixelSize: 10
    }
    Text {
        objectName: "viewerShaderTitleLeaf"
        x: 280.31
        text: qsTr("Viewer · F4 shader · 1 px strokes")
        color: "#b8e7ff"
        font.pixelSize: 10
    }

    Loader {
        objectName: "galleryShaderLoader"
        x: 0.37
        y: 24.19
        active: shaderRenderingAvailable
        sourceComponent: ShaderEffect {
            objectName: "galleryShaderLeaf"
            // No component-specific snapping: the window policy handles this.
            x: 0.23
            y: 0.31
            width: 192 / Screen.devicePixelRatio
            height: 96 / Screen.devicePixelRatio
            property size viewportSize: Qt.size(192, 96)
            fragmentShader: "qrc:/ZoinGallery/resources/gallery_pixel_pattern.frag.qsb"
            Accessible.ignored: true
        }
    }

    Item {
        x: 279.73
        y: 23.81
        width: 220
        height: 100

        Loader {
            objectName: "viewerShaderLoader"
            x: 0.41
            y: 0.27
            active: shaderRenderingAvailable
            sourceComponent: ViewerResampleEffect {
                objectName: "viewerShaderLeaf"
                x: 0.19
                y: 0.43
                width: 192 / Screen.devicePixelRatio
                height: 96 / Screen.devicePixelRatio
                viewportSize: Qt.size(192, 96)
                pixelAligned: demoState.viewerShaderSnap && !root.moving
                Accessible.ignored: true
            }
        }
    }

    Text {
        objectName: "shaderCoverageNoteLeaf"
        y: root.panelHeight + 64.23
        text: shaderRenderingAvailable
            ? qsTr("Gallery: window alignment · Viewer: its own vertex snapping")
            : qsTr("GPU shaders unavailable in the software renderer")
        color: "#f3d15b"
        font.pixelSize: 9
    }

    CheckBox {
        id: viewerSnapControl
        objectName: "viewerShaderSnapControl"
        x: 280.31
        y: root.panelHeight + 37.23
        text: qsTr("F4 viewer's own pixel snapping")
        checked: demoState.viewerShaderSnap
        enabled: shaderRenderingAvailable
        onToggled: demoState.viewerShaderSnap = checked
        contentItem: Text {
            objectName: "viewerShaderSnapTextLeaf"
            text: viewerSnapControl.text
            color: "#e9edf0"
            font.pixelSize: 9
            verticalAlignment: Text.AlignVCenter
            leftPadding: 20
        }
        indicator: Rectangle {
            objectName: "viewerShaderSnapIndicatorLeaf"
            x: 0
            y: (viewerSnapControl.height - height) / 2
            width: 13
            height: 13
            color: viewerSnapControl.checked ? "#69ae70" : "#303840"
            border.width: 1
            border.color: "#c4cdd4"
        }
    }
}
