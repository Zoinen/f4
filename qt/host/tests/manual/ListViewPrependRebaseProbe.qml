import QtQuick

// Symmetric upward-window probe for granular ListModel prepends.
Window {
    id: probe
    width: 720
    height: 400
    visible: true

    function topExtent() {
        var raw = Math.max(0, view.contentY) / 20
        var index = Math.max(0, Math.min(rows.count - 1, Math.floor(raw)))
        return Number(rows.get(index).extent) + raw - index
    }

    function report(label) {
        console.log("LISTVIEW_PREPEND_PROBE", label,
                    "count=" + rows.count,
                    "contentY=" + view.contentY.toFixed(3),
                    "extent=" + topExtent().toFixed(3),
                    "velocity=" + view.verticalVelocity.toFixed(3),
                    "moving=" + view.moving,
                    "flicking=" + view.flicking)
    }

    Component.onCompleted: {
        for (var row = 20; row < 140; ++row)
            rows.append({ "extent": row, "label": "row " + row })
        startTimer.start()
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
        id: startTimer
        interval: 50
        onTriggered: {
            // Local index 40 is global extent 60.  Start moving toward the
            // beginning before prepending the missing global rows 0..19.
            view.contentY = 800
            report("before-flick")
            view.flick(0, 900)
            rebaseTimer.start()
        }
    }

    Timer {
        id: rebaseTimer
        interval: 100
        onTriggered: {
            report("before-prepend")
            for (var row = 19; row >= 0; --row)
                rows.insert(0, { "extent": row, "label": "row " + row })
            report("after-prepend")
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
