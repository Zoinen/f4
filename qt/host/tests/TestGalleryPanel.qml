import QtQuick

FocusScope {
    property int side: 0
    property var panel: ({})
    property var bridge: null
    property var keySink: null
    property var theme: ({})
    property var metrics: ({})
    property real devicePixelRatio: 1
    property real defaultListDensity: 22
    property bool panelActive: false
    property bool commandLineHasText: false
    property bool fastFindActive: false
    readonly property string appliedPresentationMode:
        String(panel.galleryLayoutMode || "masonry")
    readonly property var appliedColumnSchema: panel.galleryColumns || []

    readonly property real minimumDensity: 22
    readonly property real maximumDensity: 500
    readonly property real densityStep: 2
    readonly property real currentDensity: 30
    readonly property bool densityAdjustable: {
        const mode = String(panel.galleryLayoutMode || "masonry")
        return mode !== "columns" && mode !== "details"
    }

    Rectangle {
        anchors.fill: parent
        color: "#17202a"
    }
}
