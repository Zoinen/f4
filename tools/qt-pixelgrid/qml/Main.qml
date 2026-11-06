import QtQuick
import QtQuick.Controls.Basic
import QtQuick.Layouts

Rectangle {
    id: root
    objectName: "demoRoot"
    width: 700
    height: 700
    color: "#20252b"

    property bool alignedView: pixelAligned

    ColumnLayout {
        objectName: "rootColumnLayout"
        anchors.fill: parent
        anchors.margins: 12
        spacing: 8

        RowLayout {
            objectName: "headerRowLayout"
            Layout.fillWidth: true
            Text {
                objectName: "viewTitleLeaf"
                text: alignedView ? "Pixel aligned" : "Original"
                color: "white"
                font.pixelSize: 18
                Layout.fillWidth: true
            }
            Text {
                objectName: "dprLeaf"
                text: "DPR " + Screen.devicePixelRatio.toFixed(3)
                color: "#a9d6ff"
                font.pixelSize: 10
            }
        }

        Text {
            objectName: "statusLeaf"
            Layout.fillWidth: true
            color: "#d7dde5"
            font.pixelSize: 10
            text: (alignedView ? "Qt pixel-grid policy enabled" : "Qt original rendering")
                  + " · motion " + demoState.motion.toFixed(2)
                  + " · unsupported " + (alignedView ? demoState.unsupportedCount : "n/a")
        }

        RowLayout {
            objectName: "toolbarRowLayout"
            Layout.fillWidth: true
            Button {
                objectName: "moveLeftControl"
                text: "Move −¼"
                onClicked: demoState.move(-0.25)
                contentItem: Text {
                    objectName: "moveLeftTextLeaf"
                    text: "Move −¼"
                    color: "white"
                    font.pixelSize: 10
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                }
                background: Rectangle {
                    objectName: "moveLeftBackgroundLeaf"
                    color: "#3c4854"
                    border.width: 1
                    border.color: "#8c9aa7"
                }
            }
            Button {
                objectName: "moveRightControl"
                text: "Move +¼"
                onClicked: demoState.move(0.25)
                contentItem: Text {
                    objectName: "moveRightTextLeaf"
                    text: "Move +¼"
                    color: "white"
                    font.pixelSize: 10
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                }
                background: Rectangle {
                    objectName: "moveRightBackgroundLeaf"
                    color: "#3c4854"
                    border.width: 1
                    border.color: "#8c9aa7"
                }
            }
            Button {
                objectName: "resizeControl"
                text: "Resize"
                onClicked: demoState.resizeBy(1)
                contentItem: Text {
                    objectName: "resizeTextLeaf"
                    text: "Resize"
                    color: "white"
                    font.pixelSize: 10
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                }
                background: Rectangle {
                    objectName: "resizeBackgroundLeaf"
                    color: "#3c4854"
                    border.width: 1
                    border.color: "#8c9aa7"
                }
            }
            Button {
                objectName: "contentControl"
                text: "Change text"
                onClicked: demoState.changeContent()
                contentItem: Text {
                    objectName: "contentTextLeaf"
                    text: "Change text"
                    color: "white"
                    font.pixelSize: 10
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                }
                background: Rectangle {
                    objectName: "contentBackgroundLeaf"
                    color: "#3c4854"
                    border.width: 1
                    border.color: "#8c9aa7"
                }
            }
        }
        RowLayout {
            objectName: "actionsRowLayout"
            Layout.fillWidth: true
            CheckBox {
                objectName: "animationControl"
                text: "Animate fractional motion"
                checked: demoState.animating
                onToggled: demoState.animating = checked
                contentItem: Text {
                    objectName: "animationTextLeaf"
                    text: "Animate fractional motion"
                    color: "#e9edf0"
                    font.pixelSize: 10
                    verticalAlignment: Text.AlignVCenter
                    leftPadding: 20
                }
                indicator: Rectangle {
                    objectName: "animationIndicatorLeaf"
                    x: 0
                    y: (parent.height - height) / 2
                    width: 13
                    height: 13
                    color: parent.checked ? "#69ae70" : "#303840"
                    border.width: 1
                    border.color: "#c4cdd4"
                }
            }
            Button {
                objectName: "captureControl"
                text: "Capture this window"
                onClicked: demoState.saveCapture(Window.window, alignedView)
                contentItem: Text {
                    objectName: "captureTextLeaf"
                    text: "Capture this window"
                    color: "white"
                    font.pixelSize: 10
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                }
                background: Rectangle {
                    objectName: "captureBackgroundLeaf"
                    color: "#3c4854"
                    border.width: 1
                    border.color: "#8c9aa7"
                }
            }
        }

        Rectangle {
            objectName: "clipViewport"
            Layout.fillWidth: true
            Layout.fillHeight: true
            Layout.minimumHeight: 230
            color: "#12171b"
            border.width: 1
            border.color: "#69727c"
            clip: true

            Item {
                id: nestedFractionalParent
                objectName: "nestedFractionalParent"
                x: 19.35 + demoState.motion
                y: 13.2
                width: 630
                height: 400
                transform: Translate { x: 0.37; y: 0.29 }

                Behavior on x {
                    enabled: demoState.animating
                    NumberAnimation {
                        id: fractionalMotionAnimation
                        duration: 420
                        easing.type: Easing.InOutQuad
                    }
                }

                Rectangle {
                    objectName: "sampleRectangle"
                    x: 9.4
                    y: 8.6
                    width: 111 + demoState.widthDelta
                    height: 61
                    color: "#3278a6"
                    border.width: 1
                    border.color: "#bfe8ff"
                }

                Text {
                    objectName: "smallCaptionLeaf"
                    x: 9.4
                    y: 77.4
                    text: demoState.content
                    color: "#fff4ce"
                    font.pixelSize: demoState.captionPixelSize
                }

                TextInput {
                    objectName: "editableTextLeaf"
                    x: 9.4
                    y: 98.6
                    width: 178
                    height: 24
                    text: "Editable cursor"
                    color: "white"
                    selectionColor: "#387dc4"
                    font.pixelSize: 11
                    clip: true
                }

                Image {
                    objectName: "rasterIconLeaf"
                    x: 9.4
                    y: 134.6
                    width: 24
                    height: 24
                    source: demoState.imageSource
                    smooth: false
                }

                Button {
                    objectName: "demoButton"
                    x: 45.7
                    y: 132.1
                    width: 111
                    height: 30
                    text: "Test button"
                    contentItem: Text {
                        objectName: "buttonTextLeaf"
                        text: parent.text
                        color: "white"
                        font.pixelSize: 10
                        horizontalAlignment: Text.AlignHCenter
                        verticalAlignment: Text.AlignVCenter
                    }
                    background: Rectangle {
                        objectName: "buttonBackgroundLeaf"
                        radius: 2
                        color: "#3c4854"
                        border.width: 1
                        border.color: "#8c9aa7"
                    }
                }

                CheckBox {
                    objectName: "demoCheckBox"
                    x: 45.7
                    y: 169.4
                    text: "Small check"
                    contentItem: Text {
                        objectName: "checkboxTextLeaf"
                        text: parent.text
                        color: "#e9edf0"
                        font.pixelSize: 9
                        verticalAlignment: Text.AlignVCenter
                        leftPadding: 20
                    }
                    indicator: Rectangle {
                        objectName: "checkboxIndicatorLeaf"
                        x: 0
                        y: (parent.height - height) / 2
                        width: 13
                        height: 13
                        color: parent.checked ? "#69ae70" : "#303840"
                        border.width: 1
                        border.color: "#c4cdd4"
                    }
                }

                Item {
                    objectName: "oddCenterContainer"
                    x: 203.3
                    y: 8.6
                    width: 80
                    height: 80
                    Rectangle {
                        objectName: "oddCenteredLeaf"
                        width: 37
                        height: 25
                        anchors.centerIn: parent
                        color: "#cf7e3f"
                    }
                }

                Item {
                    objectName: "clipInnerContainer"
                    x: 325.2
                    y: 19.3
                    width: 81
                    height: 45
                    clip: true
                    Rectangle {
                        objectName: "clipEdgeLeaf"
                        x: 4.3
                        y: 4.2
                        width: 90
                        height: 37
                        color: "#a54c55"
                    }
                }

                RasterDiagnosticIcons {
                    objectName: "rasterDiagnosticGroup"
                    x: 9.4
                    y: 224.6
                }

                ShaderDiagnosticScene {
                    objectName: "shaderDiagnosticGroup"
                    x: 9.4
                    y: 372.3
                    moving: fractionalMotionAnimation.running
                }
            }
        }
    }
}
