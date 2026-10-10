pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl
import QtQuick.Layouts
import ZoinGallery 1.0 as ZG

Rectangle {
    id: panelRoot
    objectName: "filePanel-" + Number(panel.side || 0)
    required property ApplicationWindow hostWindow
    required property QtObject galleryController
    required property Item focusTarget
    required property Item menuBar
    required property ZG.GalleryThemePalette galleryTheme
    required property ZG.GalleryPresentationMetrics galleryMetrics
    required property var panel
    property var layoutState: null
    readonly property bool layoutStateMatchesPanel:
        layoutState !== null
        && String(layoutState.id || "") === String(panel.id || "")
        && Number(layoutState.catalogRevision || 0)
           === Number(panel.catalogRevision || 0)
    readonly property string effectiveGalleryLayoutMode:
        hostWindow.cleanText(layoutStateMatchesPanel
                       && layoutState.galleryLayoutMode !== undefined
                       ? layoutState.galleryLayoutMode
                       : panel.galleryLayoutMode)
    readonly property int effectiveGalleryColumnCount:
        Number(layoutStateMatchesPanel
               && layoutState.galleryColumnCount !== undefined
               ? layoutState.galleryColumnCount
               : panel.galleryColumnCount) || 2
    readonly property bool backendLoading: panel.loading === true
    property bool loadingIndicatorVisible: false
    property int loadingIndicatorFrame: 0
    readonly property bool loadingIndicatorPulseRunning:
        loadingIndicatorPulse.running
    readonly property bool loadingIndicatorDelayRunning:
        loadingIndicatorDelay.running
    readonly property bool loadingIndicatorActive:
        backendLoading && visible && hostWindow.active
        && hostWindow.nativeTwoPanelSurfaceActive
    readonly property var loadingIndicatorFrames: [
        "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"
    ]
    property bool nativeLayout: hostWindow.isAppScene()
    property real topChromeOffset: nativeLayout ? 0 : ((panel.y || 0) <= 0 ? hostWindow.menuBarHeight : 0)
    readonly property bool panelIsActive:
        hostWindow.panelIsEffectivelyActive(panel)
    TapHandler {
        objectName: "panelCommandFocusReturn-" + Number(panelRoot.panel.side || 0)
        enabled: panelRoot.visible
                 && hostWindow.commandLineFrame().ownsNavigation === true
                 && !hostWindow.hasBlockingOverlay()
                 && !panelRoot.viewerVisible
        acceptedButtons: Qt.LeftButton | Qt.RightButton | Qt.MiddleButton
        onPressedChanged: {
            if (pressed)
                hostWindow.action({action: "panel.activate", side: Number(panelRoot.panel.side || 0)}, true)
        }
    }
    readonly property bool viewerVisible: galleryController.viewerVisible === true
    property var registeredGalleryPanelHost: null
    readonly property var rendererChoices: [
        { "label": "Columns · 2", "layoutMode": "columns", "columnCount": 2, "icon": "columns-2", "shortcut": "Ctrl+1" },
        { "label": "Columns · 3", "layoutMode": "columns", "columnCount": 3, "icon": "columns-3", "shortcut": "Ctrl+2" },
        { "label": "Details", "layoutMode": "details", "icon": "list", "shortcut": "Ctrl+3" },
        { "label": "Icons", "layoutMode": "icons", "icon": "images", "shortcut": "Ctrl+5" },
        { "label": "Grid", "layoutMode": "grid", "icon": "grid-3x3", "shortcut": "Ctrl+6" },
        { "label": "Masonry", "layoutMode": "masonry", "icon": "layout-dashboard", "shortcut": "Ctrl+7" },
        { "heading": true, "label": "Layout" },
        { "label": "Wide panel", "wideToggle": true, "icon": "panel-left", "shortcut": "Ctrl+4" },
        { "heading": true, "label": "File fields" },
        { "label": "Columns…", "mode": "file-field-columns", "icon": "columns-3", "shortcut": "" }
    ]
    readonly property var sortChoices: fileFieldTools.sortChoices
    readonly property var groupChoices: [
        { "label": qsTr("Off"), "mode": "None", "icon": "list" },
        { "label": qsTr("Name"), "mode": "Name", "icon": "arrow-down-a-z" },
        { "label": qsTr("Extension"), "mode": "Extension", "icon": "file-type" },
        { "label": qsTr("Size"), "mode": "Size", "icon": "arrow-down-wide-narrow" },
        { "label": qsTr("Size on disk"), "mode": "PhysicalSize", "icon": "hard-drive" },
        { "label": qsTr("Modification time"), "mode": "Modified", "icon": "file-clock" },
        { "label": qsTr("Access time"), "mode": "Accessed", "icon": "clock-3" },
        { "label": qsTr("Creation / metadata change time"), "mode": "Changed", "icon": "calendar-plus" },
        { "label": qsTr("Owner"), "mode": "Owner", "icon": "user-round" },
        { "label": qsTr("Owner group"), "mode": "OwnerGroup", "icon": "users-round" },
        { "label": qsTr("Unix permissions"), "mode": "Permissions", "icon": "file-lock" },
        { "label": qsTr("Windows attributes"), "mode": "Attributes", "icon": "list-checks" },
        { "label": qsTr("VFS type / mode"), "mode": "ModeText", "icon": "binary" },
        { "label": qsTr("Object kind"), "mode": "Kind", "icon": "blocks" },
        { "label": qsTr("Hidden"), "mode": "Hidden", "icon": "eye-off" },
        { "label": qsTr("Executable"), "mode": "Executable", "icon": "square-terminal" },
        { "separator": true, "label": qsTr("Options") },
        { "label": qsTr("Separate folders"), "special": "folders", "icon": "folder" },
        { "label": qsTr("Size thresholds…"), "special": "thresholds", "icon": "settings-2" }
    ]

    function rendererChoiceEnabled(choice) {
        if (!choice || choice.heading === true)
            return false
        if (choice.wideToggle === true)
            return true
        return galleryController.available
    }

    function rendererChoiceActive(choice) {
        if (!choice || choice.heading === true)
            return false
        if (choice.wideToggle === true)
            return hostWindow.widePanelSide() === Number(panel.side || 0)
        if (effectiveGalleryLayoutMode !== choice.layoutMode)
            return false
        return choice.layoutMode !== "columns"
                || effectiveGalleryColumnCount
                   === Number(choice.columnCount || 2)
    }

    function rendererButtonIconName() {
        for (var i = 0; i < rendererChoices.length; ++i) {
            const choice = rendererChoices[i]
            if (choice.wideToggle !== true
                    && rendererChoiceActive(choice))
                return hostWindow.cleanText(choice.icon)
        }
        return "layout-dashboard"
    }

    function sortModeName() {
        const mode = hostWindow.cleanText(panel.sortModeName).toLowerCase()
        return mode !== "" ? mode : "name"
    }

    function sortModeLabel() {
        return fileFieldTools.sortModeLabel(sortModeName())
    }

    function openFileFieldColumns() {
        fileFieldTools.openFileFieldColumns()
    }

    function openFileFieldFilter() {
        fileFieldTools.openFileFieldFilter()
    }

    function sortIsAscending() {
        if (typeof panel.sortAscending === "boolean")
            return panel.sortAscending
        const mode = sortModeName()
        const reversed = panel.sortReverse === true
        return mode === "time" || mode === "size"
                ? reversed : !reversed
    }

    property string lastSortDirectionDiagnostic: ""
    onPanelChanged: {
        if (typeof panel.sortAscending !== "boolean")
            return
        const mode = sortModeName()
        const legacyAscending = mode === "time" || mode === "size"
                ? panel.sortReverse === true : panel.sortReverse !== true
        if (legacyAscending === panel.sortAscending)
            return
        const state = mode + ":" + panel.sortReverse + ":" + panel.sortAscending
        if (state !== lastSortDirectionDiagnostic) {
            console.debug("[FIX:sort-direction] panel", panel.side,
                          "mode", mode, "reverse", panel.sortReverse,
                          "ascending", panel.sortAscending)
            lastSortDirectionDiagnostic = state
        }
    }

    function sortDirectionIconName() {
        return sortIsAscending() ? "arrow-up" : "arrow-down"
    }

    function chooseSort(choice) {
        if (choice.mode === "groups") {
            hostWindow.action({ "action": "panel.sortGroups", "side": panel.side,
                                "enabled": panel.useSortGroups !== true })
            return
        }

        if (choice.mode === "group-none" || choice.mode === "group-reverse"
                || String(choice.mode).startsWith("group:")) {
            const currentField = String(panel.groupFileField || "")
            const fieldId = choice.mode === "group-none" ? ""
                    : choice.mode === "group-reverse" ? currentField
                    : String(choice.mode).slice("group:".length)
            hostWindow.action({
                "action": "panel.fileFields.group",
                "side": panel.side,
                "fieldId": fieldId,
                "reverse": choice.mode === "group-reverse"
                           ? panel.groupReverse !== true
                           : panel.groupReverse === true
            })
            return
        }

        if (choice.mode === "file-field-columns") {
            fileFieldTools.openFileFieldColumns()
            return
        }
        if (choice.mode === "file-field-filter") {
            fileFieldTools.openFileFieldFilter()
            return
        }

        hostWindow.action({
            "action": "panel.sort",
            "side": panel.side,
            "mode": choice.mode
        })
    }

    function groupModeName() {
        const mode = hostWindow.cleanText(panel.groupBy)
        return mode !== "" ? mode : "None"
    }

    function groupModeLabel() {
        const mode = groupModeName()
        for (var i = 0; i < groupChoices.length; ++i) {
            if (groupChoices[i].mode === mode)
                return groupChoices[i].label
        }
        return "Off"
    }

    function groupChoiceActive(choice) {
        if (!choice || choice.separator === true)
            return false
        if (choice.special === "folders")
            return panel.groupFoldersSeparately === true
        if (choice.special === "thresholds")
            return false
        return groupModeName() === String(choice.mode || "None")
    }

    function groupDirectionIconName() {
        const mode = groupModeName()
        // Date groups are newest-first by default; size and other ranks ascend.
        const newestFirst = mode === "Modified" || mode === "Accessed" || mode === "Changed"
        const ascending = newestFirst ? panel.groupReverse === true : panel.groupReverse !== true
        return ascending ? "arrow-up" : "arrow-down"
    }

    function thumbnailsEnabled() {
        const preferences = galleryController.panelPreferences
        const values = preferences ? preferences.values : null
        const key = Number(panel.side || 0) === 0
                ? "leftThumbnailsEnabled" : "rightThumbnailsEnabled"
        return !values || values[key] !== false
    }

    function toggleThumbnails() {
        return galleryController.setPanelThumbnailsEnabled(
                    Number(panel.side || 0), !thumbnailsEnabled())
    }

    function chooseGrouping(choice) {
        if (!choice || choice.separator === true)
            return
        if (choice.special === "thresholds") {
            hostWindow.action({ "action": "panel.groupSettings",
                                "side": panel.side,
                                "panelId": panel.id,
                                "path": panel.path,
                                "catalogRevision": panel.catalogRevision }, true)
            return
        }
        const mode = choice.special ? groupModeName()
                                    : String(choice.mode || "None")
        const reverse = !choice.special && mode !== "None" && mode === groupModeName()
                ? panel.groupReverse !== true : panel.groupReverse === true
        const folders = choice.special === "folders"
                ? panel.groupFoldersSeparately !== true
                : panel.groupFoldersSeparately === true
        hostWindow.action({ "action": "panel.setGrouping", "side": panel.side,
                            "panelId": panel.id, "path": panel.path,
                            "catalogRevision": panel.catalogRevision,
                            "mode": mode, "reverse": reverse,
                            "foldersSeparately": folders }, true)
    }

    function chooseRenderer(choice) {
        if (!rendererChoiceEnabled(choice))
            return
        if (choice.mode === "file-field-columns") {
            fileFieldTools.openFileFieldColumns()
            return
        }
        if (choice.wideToggle === true) {
            hostWindow.action({
                "action": "panel.setWide",
                "side": panel.side,
                "enabled": hostWindow.widePanelSide()
                           !== Number(panel.side || 0)
            }, true)
        } else {
            galleryController.requestGalleryLayout(
                        panel.side, choice.layoutMode,
                        Number(choice.columnCount || 0))
        }
    }

    function galleryHost() {
        return galleryPanelContent.item
    }

    function updateRegisteredGalleryPanelHost() {
        var nextHost = galleryPanelContent.item
        if (registeredGalleryPanelHost
                && registeredGalleryPanelHost !== nextHost) {
            hostWindow.clearGalleryPanelHost(panel.side,
                                       registeredGalleryPanelHost)
        }
        registeredGalleryPanelHost = nextHost
        if (nextHost) {
            hostWindow.setGalleryPanelHost(panel.side, nextHost)
            if (typeof nextHost.registerDragPanel === "function")
                nextHost.registerDragPanel()
        }
    }

    readonly property real nativeSplitPosition: hostWindow.nativePanelSplitPosition()

    function synchronizeLoadingIndicator() {
        if (loadingIndicatorActive) {
            loadingIndicatorDelay.restart()
            return
        }
        loadingIndicatorDelay.stop()
        loadingIndicatorPulse.stop()
        loadingIndicatorVisible = false
        loadingIndicatorFrame = 0
    }

    onLoadingIndicatorActiveChanged: synchronizeLoadingIndicator()
    Component.onCompleted: synchronizeLoadingIndicator()

    Timer {
        id: loadingIndicatorDelay
        interval: 120
        repeat: false
        onTriggered: {
            if (!panelRoot.loadingIndicatorActive)
                return
            panelRoot.loadingIndicatorFrame = 0
            panelRoot.loadingIndicatorVisible = true
            loadingIndicatorPulse.restart()
        }
    }

    Timer {
        id: loadingIndicatorPulse
        interval: 100
        repeat: true
        onTriggered: {
            if (!panelRoot.loadingIndicatorActive) {
                panelRoot.synchronizeLoadingIndicator()
                return
            }
            panelRoot.loadingIndicatorFrame =
                    (panelRoot.loadingIndicatorFrame + 1)
                    % panelRoot.loadingIndicatorFrames.length
        }
    }

    x: nativeLayout
       ? hostWindow.nativePanelX(Number(panel.side || 0))
       : hostWindow.pxX(panel.x)
    y: nativeLayout ? hostWindow.menuBarHeight : hostWindow.pxY(panel.y) + topChromeOffset
    width: nativeLayout
           ? hostWindow.nativePanelWidth(Number(panel.side || 0))
           : hostWindow.pxW(panel.w)
    height: nativeLayout ? hostWindow.nativePanelHeight(Number(panel.side || 0), hostWindow.menuBarHeight) : Math.max(1, hostWindow.pxH(panel.h) - topChromeOffset)
    color: "transparent"
    border.width: 0
    clip: true

    FilePanelChrome {
        id: panelHeader
        hostWindow: panelRoot.hostWindow
        panelView: panelRoot
        galleryController: panelRoot.galleryController
        focusTarget: panelRoot.focusTarget
        panel: panelRoot.panel
    }

    Rectangle {
        id: columnHeader
        objectName: "panelColumnHeader-" + Number(panel.side || 0)
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: panelHeader.bottom
        readonly property bool showsGalleryDetails:
            galleryPanelContent.item
            && typeof galleryPanelContent.item.appliedPresentationMode
                    !== "undefined"
            && String(galleryPanelContent.item.appliedPresentationMode)
                    === "details"
        height: showsGalleryDetails
                ? Math.max(22, hostWindow.ch, hostWindow.typography.panelLineHeight)
                  + hostWindow.verticalContentSpacing : 0
        visible: showsGalleryDetails
        color: "transparent"
        z: 2

        readonly property var columns:
            galleryPanelContent.item
            && typeof galleryPanelContent.item.appliedColumnSchema
                    !== "undefined"
            ? (galleryPanelContent.item.appliedColumnSchema || []) : []
        readonly property real totalColumnWidth: {
            var total = 0
            for (var i = 0; i < columns.length; ++i)
                total += Math.max(1, Number(columns[i].width || 1))
            return Math.max(1, total)
        }

        function columnX(index) {
            const gallery = galleryPanelContent.item
                    ? galleryPanelContent.item.galleryPanel : null
            if (gallery && gallery.galleryLayout) {
                const layout = gallery.galleryLayout
                return layout.mapToItem(columnHeader,
                    layout.paddingLeft
                    + gallery.fileFieldPresentationHelper.detailsColumnX(index), 0).x
            }
            var before = 0
            for (var i = 0; i < index; ++i)
                before += Math.max(1, Number(columns[i].width || 1))
            var contentWidth = Math.max(1, width
                                        - hostWindow.panelContentSpacing * 2)
            const local = hostWindow.panelContentSpacing
                    + Math.round(contentWidth * before / totalColumnWidth)
            const scene = columnHeader.mapToItem(
                              hostWindow.contentItem, local, 0)
            const snapped = hostWindow.snapPx(scene.x)
            return columnHeader.mapFromItem(
                        hostWindow.contentItem, snapped, scene.y).x
        }

        function columnWidth(index) {
            var start = columnX(index)
            return index === columns.length - 1
                    ? width - hostWindow.panelContentSpacing - start
                    : columnX(index + 1) - start
        }

        Repeater {
            model: columnHeader.columns

            delegate: Rectangle {
                id: columnHeaderCell
                required property int index
                required property var modelData
                x: columnHeader.columnX(index)
                width: columnHeader.columnWidth(index)
                height: columnHeader.height
                color: columnMouse.containsMouse && modelData.sortable
                       ? hostWindow.controlHoverBg : "transparent"

                Behavior on color { ColorAnimation { duration: 70 } }

                Text {
                    id: columnHeaderTitle
                    font.family: hostWindow.typography.effectivePanelFamily
                    objectName: "panelColumnHeaderText-" + index
                                + "-" + Number(panel.side || 0)
                    anchors.fill: parent
                    anchors.leftMargin: hostWindow.snapPx(hostWindow.panelColumnPadding)
                    anchors.rightMargin: hostWindow.snapPx(hostWindow.panelColumnPadding)
                    text: hostWindow.cleanText(modelData.title)
                    color: modelData.sortable
                           ? hostWindow.chromeText : hostWindow.mutedText
                    font.pixelSize: hostWindow.typography.effectivePanelSize
                    verticalAlignment: Text.AlignVCenter
                    horizontalAlignment: index > 0
                                         ? Text.AlignRight
                                         : Text.AlignLeft
                    elide: Text.ElideRight
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               columnHeaderTitle, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               columnHeaderTitle, hostWindow.contentItem)
                    }
                }

                Rectangle {
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    objectName: "panelHeaderVerticalSeparator-" + index + "-" + Number(panel.side || 0)
                    width: 1
                    height: Math.max(1, parent.height
                                     - (hostWindow.headerVerticalSeparatorSpacing
                                        ? hostWindow.columnSeparatorVerticalMargin * 2 : 0))
                    color: hostWindow.separatorColor
                    opacity: index < columnHeader.columns.length - 1
                             ? 0.65 : 0
                }

                MouseArea {
                    id: columnMouse
                    anchors.fill: parent
                    acceptedButtons: Qt.LeftButton | Qt.RightButton
                    hoverEnabled: true
                    enabled: modelData.sortable === true
                    cursorShape: enabled ? Qt.PointingHandCursor
                                         : Qt.ArrowCursor
                    onClicked: mouse => {
                        if (mouse.button === Qt.RightButton) {
                            hostWindow.action({
                                "action": "panel.sortMenu",
                                "side": panel.side
                            })
                        } else {
                            hostWindow.action({
                                "action": "panel.sort",
                                "side": panel.side,
                                "mode": modelData.sortMode
                            })
                        }
                        mouse.accepted = true
                    }
                }
            }
        }

        Repeater {
            model: Math.max(0, columnHeader.columns.length - 1)
            delegate: MouseArea {
                id: columnResizeHandle
                required property int index
                objectName: "panelColumnResizeHandle-" + index + "-" + Number(panel.side || 0)
                readonly property var gallery: galleryPanelContent.item
                    ? galleryPanelContent.item.galleryPanel : null
                property real pointerStartX: 0
                property var widthsAtPress: []
                property var previewColumns: []
                x: columnHeader.columnX(index + 1) - hostWindow.snapPx(4)
                width: hostWindow.snapPx(8)
                height: columnHeader.height
                z: 10
                acceptedButtons: Qt.LeftButton
                hoverEnabled: true
                preventStealing: true
                cursorShape: Qt.SplitHCursor
                onPressed: mouse => {
                    if (!gallery) {
                        mouse.accepted = false
                        return
                    }
                    pointerStartX = mapToItem(columnHeader, mouse.x, mouse.y).x
                    widthsAtPress = gallery.detailsHeader.columns.map(column =>
                        Math.max(1, Number(column.width || 1)))
                    previewColumns = []
                    mouse.accepted = true
                }
                onPositionChanged: mouse => {
                    if (!(mouse.buttons & Qt.LeftButton) || !gallery
                            || widthsAtPress.length !== columnHeader.columns.length)
                        return
                    const delta = hostWindow.snapPx(
                        mapToItem(columnHeader, mouse.x, mouse.y).x - pointerStartX)
                    if (Math.abs(delta) < 0.01)
                        return
                    const columns = gallery.detailsHeader.resizedColumnsForDrag(widthsAtPress, index, delta)
                    if (columns.length !== widthsAtPress.length)
                        return
                    previewColumns = columns
                    gallery.previewColumnSchema(columns)
                }
                onReleased: mouse => {
                    if (gallery && previewColumns.length > 0)
                        gallery.columnResizeRequested(previewColumns)
                    widthsAtPress = []
                    previewColumns = []
                    mouse.accepted = true
                }
                onCanceled: {
                    widthsAtPress = []
                    previewColumns = []
                }
            }
        }

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            objectName: "panelHeaderHorizontalSeparator-" + Number(panel.side || 0)
            anchors.leftMargin: hostWindow.headerHorizontalSeparatorSpacing
                                ? hostWindow.columnSeparatorVerticalMargin : 0
            anchors.rightMargin: anchors.leftMargin
            height: 1
            color: hostWindow.separatorColor
            opacity: 0.7
        }
    }

    Repeater {
        model: hostWindow.showColumnSeparators && columnHeader.visible
                ? Math.max(0, columnHeader.columns.length - 1) : 0
        delegate: Rectangle {
            required property int index
            objectName: "panelColumnSeparator-" + index + "-" + Number(panel.side || 0)
            x: columnHeader.x + columnHeader.columnX(index + 1) - width
            readonly property real inset: hostWindow.columnSeparatorSpacing
                                          ? hostWindow.columnSeparatorVerticalMargin : 0
            y: galleryPanelContent.y + inset
            width: hostWindow.separatorWidth
            height: Math.max(0, galleryPanelContent.height
                             - inset * 2)
            color: hostWindow.separatorColor
            opacity: 0.65
            z: 2
        }
    }

    RetainedGalleryPanelContent {
        id: galleryPanelContent
        panelSurface: panelRoot
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.topMargin: panelHeader.height + columnHeader.height
        anchors.bottom: parent.bottom
        z: 1
    }

    Connections {
        target: galleryPanelContent.item
        ignoreUnknownSignals: true
        function onPointerActivationPreviewRequested(side) {
            hostWindow.beginPointerPanelActivation(side)
        }
    }

    Rectangle {
        id: rendererFailure
        objectName: "panelRendererFailure-" + Number(panel.side || 0)
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.leftMargin: hostWindow.panelContentSpacing
        anchors.rightMargin: hostWindow.panelContentSpacing
        anchors.top: parent.top
        anchors.topMargin: panelHeader.height + columnHeader.height
        anchors.bottom: parent.bottom
        z: 2
        visible: panelRoot.visible
                 && (!galleryController.available
                     || galleryPanelContent.status === Loader.Error)
        color: "transparent"

        Column {
            anchors.centerIn: parent
            width: Math.min(parent.width - 32, 420)
            spacing: 8

            IconLabel {
                anchors.horizontalCenter: parent.horizontalCenter
                width: 24
                height: 24
                icon.source: hostWindow.lucideIconSource(
                                 "triangle-alert", 24, hostWindow.activeBorder)
                icon.width: 24
                icon.height: 24
                icon.color: hostWindow.activeBorder
            }

            Text {
                width: parent.width
                text: galleryController.available
                      ? "The unified panel renderer could not be loaded."
                      : "The unified panel renderer is unavailable in this build."
                color: hostWindow.textColor
                font.pixelSize: (hostWindow ? hostWindow.uiTextSize(13) : 13)
                horizontalAlignment: Text.AlignHCenter
                wrapMode: Text.Wrap
            }
        }
    }

    onPanelIsActiveChanged: {
        if (!visible || !panelIsActive || galleryController.viewerVisible
                || hostWindow.hasBlockingOverlay() || hostWindow.needsFallbackGrid()
                || hostWindow.hasDocumentSurface()
                || hostWindow.hasOperationsQueueSurface())
            return
        if (galleryPanelContent.item)
            galleryPanelContent.item.forceActiveFocus()
        else
            focusTarget.forceActiveFocus()
    }

    onViewerVisibleChanged: {
        if (viewerVisible || hostWindow.hasBlockingOverlay()
                || hostWindow.needsFallbackGrid()
                || hostWindow.hasDocumentSurface()
                || hostWindow.hasOperationsQueueSurface())
            return
        if (panelRoot.visible && panelRoot.panelIsActive
                && galleryPanelContent.item) {
            galleryPanelContent.item.forceActiveFocus()
        } else if (panelRoot.visible && panelRoot.panelIsActive) {
            focusTarget.forceActiveFocus()
        }
    }

    PanelFastFindOverlay {
        panelSurface: panelRoot
        statusOverlay: status
    }

    FileFieldToolsOverlay {
        id: fileFieldTools
        hostWindow: panelRoot.hostWindow
        panelItem: panelRoot
        panelHeader: panelHeader
        panel: panelRoot.panel
    }

    PanelStatusOverlay {
        id: status
        objectName: "panelStatus-" + Number(panel.side || 0)
        hostWindow: panelRoot.hostWindow
        panel: panelRoot.panel
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.margins: hostWindow.snapPx(8)
        width: Math.min(implicitWidth, Math.max(1, parent.width - 2 * anchors.margins))
        height: implicitHeight
        z: 3
    }
}
