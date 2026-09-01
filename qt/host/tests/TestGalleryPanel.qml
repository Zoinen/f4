import QtQuick
import ZoinGallery 1.0 as ZG

FocusScope {
    property int side: 0
    property var panel: ({})
    property var layoutState: null
    property var bridge: null
    property var keySink: null
    property ZG.GalleryThemePalette theme: ZG.GalleryThemePalette {}
    property ZG.GalleryPresentationMetrics metrics:
        ZG.GalleryPresentationMetrics {}
    property real devicePixelRatio: 1
    property real defaultListDensity: 22
    property bool panelActive: false
    property bool commandLineHasText: false
    property bool fastFindActive: false
    readonly property bool showCursor: panelActive
    signal pointerActivationPreviewRequested(int side)
    readonly property bool layoutStateMatchesPanel:
        layoutState !== null
        && String(layoutState.id || "") === String(panel.id || "")
        && Number(layoutState.catalogRevision || 0)
           === Number(panel.catalogRevision || 0)
    readonly property string appliedPresentationMode:
        String(layoutStateMatchesPanel
               && layoutState.galleryLayoutMode !== undefined
               ? layoutState.galleryLayoutMode
               : (panel.galleryLayoutMode || "masonry"))
    readonly property var appliedColumnSchema:
        layoutStateMatchesPanel && layoutState.galleryColumns !== undefined
        ? layoutState.galleryColumns : (panel.galleryColumns || [])

    readonly property real minimumDensity: 22
    readonly property real maximumDensity: 500
    readonly property real densityStep: 2
    readonly property real currentDensity: 30
    readonly property bool densityAdjustable: true

    function beginPointerActivationPreview() {
        pointerActivationPreviewRequested(side)
    }

    Rectangle {
        anchors.fill: parent
        color: "#17202a"
    }
}
