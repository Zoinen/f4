import QtQuick

// Verifies whether granular ListModel edits can slide an absolute-row window
// under a live ListView without resetting its native kinetic timeline.
Window {
    id: probe
    width: 720
    height: 400
    visible: true

    function topExtent() {
        if (rows.count === 0)
            return 0
        var raw = Math.max(0, view.contentY) / 20
        var index = Math.max(0, Math.min(rows.count - 1, Math.floor(raw)))
        return Number(rows.get(index).extent) + raw - index
    }

    function report(label) {
        console.log("LISTVIEW_INCREMENTAL_PROBE", label,
                    "count=" + rows.count,
                    "contentY=" + view.contentY.toFixed(3),
                    "extent=" + topExtent().toFixed(3),
                    "velocity=" + view.verticalVelocity.toFixed(3),
                    "moving=" + view.moving,
                    "flicking=" + view.flicking)
    }

    Component.onCompleted: {
        for (var row = 0; row < 120; ++row)
            rows.append({ "extent": row, "label": "row " + row })
        view.positionViewAtIndex(40, ListView.Beginning)
        Qt.callLater(function() {
            report("before-flick")
            view.flick(0, -900)
            rebaseTimer.start()
        })
    }

    ListModel { id: rows }

    ListView {
        id: view
        anchors.fill: parent
        model: rows
        boundsBehavior: Flickable.StopAtBounds
        delegate: Text {
            required property int extent
            required property string label
            width: ListView.view.width
            height: 20
            text: label
            color: "white"
        }
    }

    Timer {
        id: rebaseTimer
        interval: 100
        onTriggered: {
            report("before-rebase")
            for (var row = 120; row < 140; ++row)
                rows.append({ "extent": row, "label": "row " + row })
            report("after-append")
            rows.remove(0, 20)
            report("after-remove")
            settleTimer.start()
        }
    }

    Timer {
        id: settleTimer
        interval: 50
        onTriggered: {
            report("settled")
            Qt.quit()
        }
    }
}
