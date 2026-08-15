import QtQuick

// Standalone diagnostic for the bounded DocumentSurface row tree.  It mirrors
// the relevant Repeater/Rectangle/Text topology without depending on f4's IPC
// test harness.  Run with QSG_RHI_BACKEND=software qml this-file.qml.
Window {
    id: probe
    width: 1600
    height: 900
    visible: true

    property var rows: []
    property int completedRows: 0
    property int completedRuns: 0
    property int scenarioIndex: -1
    property int repetition: 0
    property double assignmentStarted: 0
    property double assignmentFinished: 0
    property var scenarios: [
        { "rows": 90, "runs": 0 },
        { "rows": 90, "runs": 10 },
        { "rows": 90, "runs": 40 },
        { "rows": 90, "runs": 160 },
        { "rows": 300, "runs": 40 }
    ]

    function makeRows(rowCount, runCount, salt) {
        var result = []
        for (var row = 0; row < rowCount; ++row) {
            var runs = []
            for (var run = 0; run < runCount; ++run) {
                runs.push({
                    "text": String.fromCharCode(33 + ((run + salt) % 90)),
                    "foreground": ((run + salt) & 1) ? "#d8dee9" : "#88c0d0",
                    "background": "transparent",
                    "bold": (run % 7) === 0
                })
            }
            result.push({
                "visualRow": row + salt * rowCount,
                "text": runCount === 0
                        ? "plain row " + row + " generation " + salt : "",
                "runs": runs
            })
        }
        return result
    }

    function advance() {
        ++repetition
        if (repetition >= 4) {
            repetition = 0
            ++scenarioIndex
        }
        if (scenarioIndex >= scenarios.length) {
            Qt.quit()
            return
        }

        completedRows = 0
        completedRuns = 0
        gc()
        var scenario = scenarios[scenarioIndex]
        var nextRows = makeRows(scenario.rows, scenario.runs,
                                scenarioIndex * 10 + repetition)
        assignmentStarted = Date.now()
        rows = nextRows
        assignmentFinished = Date.now()
        settleTimer.restart()
    }

    Component.onCompleted: {
        // First pass warms QML types and the font machinery and is excluded by
        // using four repetitions per scenario in the output.
        scenarioIndex = 0
        repetition = -1
        advanceTimer.start()
    }

    Timer {
        id: settleTimer
        // Delegate creation is synchronous for Repeater.  One event-loop turn
        // additionally captures binding/layout work queued by the assignment;
        // GPU timings can be added externally with QSG_RENDER_TIMING=1.
        interval: 0
        onTriggered: {
        var scenario = scenarios[scenarioIndex]
        console.log("DOCUMENT_REPEATER_PROBE",
                    "rows=" + scenario.rows,
                    "runsPerRow=" + scenario.runs,
                    "repetition=" + repetition,
                    "assignMs=" + (assignmentFinished - assignmentStarted),
                    "settledMs=" + (Date.now() - assignmentStarted),
                    "rowDelegates=" + completedRows,
                    "runDelegates=" + completedRuns,
                    "estimatedQuickItems="
                    + (1 + scenario.rows * (4 + 2 * scenario.runs)))
        advanceTimer.restart()
        }
    }

    Timer {
        id: advanceTimer
        interval: 40
        onTriggered: probe.advance()
    }

    Flickable {
        anchors.fill: parent
        contentWidth: width
        contentHeight: Math.max(height, rowLayer.height)

        Item {
            id: rowLayer
            width: parent.width
            height: probe.rows.length * 20

            Repeater {
                model: probe.rows

                delegate: Rectangle {
                    required property int index
                    required property var modelData
                    x: 0
                    y: index * 20
                    width: rowLayer.width
                    height: 20
                    color: "#101419"
                    Component.onCompleted: ++probe.completedRows

                    Row {
                        anchors.left: parent.left
                        height: parent.height

                        Repeater {
                            model: modelData.runs || []

                            delegate: Rectangle {
                                required property var modelData
                                height: 20
                                width: runLabel.implicitWidth
                                color: modelData.background
                                Component.onCompleted: ++probe.completedRuns

                                Text {
                                    id: runLabel
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: modelData.text
                                    color: modelData.foreground
                                    font.family: "Monaco"
                                    font.pixelSize: 13
                                    font.bold: modelData.bold
                                    renderType: Text.NativeRendering
                                }
                            }
                        }
                    }

                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        text: modelData.text
                        visible: !modelData.runs || modelData.runs.length === 0
                        color: "white"
                        font.family: "Monaco"
                        font.pixelSize: 13
                        renderType: Text.NativeRendering
                    }
                }
            }
        }
    }
}
