import QtQuick

Rectangle {
    id: root
    width: 720
    height: 420
    color: "#171c22"

    readonly property int flatLeafCount: 200
    readonly property int nestedBranchCount: 20
    readonly property int nestedLeavesPerBranch: 10
    property real phase: 0
    property bool autoAnimate: true

    NumberAnimation on phase {
        running: root.autoAnimate
        from: 0
        to: 6
        duration: 1200
        loops: Animation.Infinite
    }

    Item {
        objectName: "flatGroup"
        x: 18.37 + root.phase
        y: 38.29

        Repeater {
            model: root.flatLeafCount

            delegate: Rectangle {
                x: (index % 20) * 10.0 + 0.37
                y: Math.floor(index / 20) * 10.0 + 0.21
                width: 8
                height: 8
                color: index % 2 ? "#568eb6" : "#d1884d"
            }
        }
    }

    Item {
        objectName: "nestedRoot"
        x: 278.37 - root.phase * 0.5
        y: 38.71

        Repeater {
            model: root.nestedBranchCount

            delegate: Item {
                x: (index % 4) * 65.0 + 0.31
                y: Math.floor(index / 4) * 62.0 + 0.17

                Repeater {
                    model: root.nestedLeavesPerBranch

                    delegate: Rectangle {
                        x: (index % 5) * 10.0 + 0.43
                        y: Math.floor(index / 5) * 10.0 + 0.39
                        width: 8
                        height: 8
                        color: index % 2 ? "#7fa765" : "#9d6cae"
                    }
                }
            }
        }
    }
}
