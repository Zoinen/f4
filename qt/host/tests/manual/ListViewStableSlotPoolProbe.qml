import QtQuick

// Proof-of-concept for a fixed-capacity, virtualized semantic row pool.  New
// bounded windows fill existing offscreen slots, so neither model geometry nor
// Flickable.contentY changes while native inertia is active.
Window {
    id: probe
    width: 1280
    height: 400
    visible: true

    property int completedRows: 0
    property int completedRuns: 0

    function runsFor(extent) {
        var result = []
        for (var run = 0; run < 40; ++run) {
            result.push({
                "text": String.fromCharCode(33 + ((extent + run) % 90)),
                "foreground": (run & 1) ? "#d8dee9" : "#88c0d0"
            })
        }
        return result
    }

    function fill(slot, extent) {
        rows.set(slot, {
            "loaded": true,
            "extent": extent,
            "label": "row " + extent,
            "runData": runsFor(extent)
        })
    }

    function topExtent() {
        var raw = Math.max(0, view.contentY) / 20
        var slot = Math.max(0, Math.min(rows.count - 1, Math.floor(raw)))
        var data = rows.get(slot)
        return data.loaded ? Number(data.extent) + raw - slot : -1
    }

    function report(label) {
        console.log("LISTVIEW_SLOT_POOL_PROBE", label,
                    "contentY=" + view.contentY.toFixed(3),
                    "extent=" + topExtent().toFixed(3),
                    "velocity=" + view.verticalVelocity.toFixed(3),
                    "moving=" + view.moving,
                    "flicking=" + view.flicking,
                    "rowDelegates=" + completedRows,
                    "runDelegates=" + completedRuns)
    }

    Component.onCompleted: {
        for (var slot = 0; slot < 360; ++slot) {
            rows.append({
                "loaded": false,
                "extent": -1,
                "label": "",
                "runData": []
            })
        }
        // Three loaded viewports in the centre of a twelve-viewport pool.
        for (var extent = 0; extent < 120; ++extent)
            fill(120 + extent, extent)
        startTimer.start()
    }

    ListModel {
        id: rows
        dynamicRoles: true
    }

    ListView {
        id: view
        anchors.fill: parent
        model: rows
        reuseItems: true
        cacheBuffer: 40
        boundsBehavior: Flickable.StopAtBounds

        delegate: Rectangle {
            required property bool loaded
            required property int extent
            required property string label
            required property var runData
            width: ListView.view.width
            height: 20
            color: loaded ? "#101419" : "#20252b"
            Component.onCompleted: ++probe.completedRows

            Row {
                height: parent.height
                visible: loaded

                Repeater {
                    model: runData
                    delegate: Text {
                        required property var modelData
                        text: modelData.text
                        color: modelData.foreground
                        font.family: "Monaco"
                        font.pixelSize: 13
                        renderType: Text.NativeRendering
                        Component.onCompleted: ++probe.completedRuns
                    }
                }
            }
        }
    }

    Timer {
        id: startTimer
        interval: 60
        onTriggered: {
            // Slot 160 is global extent 40.
            view.contentY = 160 * 20
            report("before-flick")
            view.flick(0, -900)
            fillTimer.start()
        }
    }

    Timer {
        id: fillTimer
        interval: 100
        onTriggered: {
            report("before-fill")
            var started = Date.now()
            // Fill the next viewport without changing count, indices or any
            // role belonging to the currently visible delegates.
            for (var extent = 120; extent < 150; ++extent)
                fill(120 + extent, extent)
            var elapsed = Date.now() - started
            report("after-fill")
            console.log("LISTVIEW_SLOT_POOL_PROBE fill30x40RunsMs=" + elapsed)
            settleTimer.start()
        }
    }

    Timer {
        id: settleTimer
        interval: 60
        onTriggered: {
            report("settled")
            Qt.quit()
        }
    }
}
