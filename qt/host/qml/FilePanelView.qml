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
    readonly property var loadingIndicatorFrames: [
        "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"
    ]
    property bool nativeLayout: hostWindow.isAppScene()
    property real topChromeOffset: nativeLayout ? 0 : ((panel.y || 0) <= 0 ? menuBar.height : 0)
    readonly property bool panelIsActive:
        hostWindow.panelIsEffectivelyActive(panel)
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
        { "label": "Wide panel", "wideToggle": true, "icon": "panel-left", "shortcut": "Ctrl+4" }
    ]
    readonly property var sortChoices: [
        { "label": "Name", "mode": "name", "icon": "arrow-down-a-z", "shortcut": "Ctrl+F3" },
        { "label": "Extension", "mode": "extension", "icon": "file-type", "shortcut": "Ctrl+F4" },
        { "label": "Time", "mode": "time", "icon": "clock-3", "shortcut": "Ctrl+F5" },
        { "label": "Size", "mode": "size", "icon": "arrow-down-wide-narrow", "shortcut": "Ctrl+F6" },
        { "label": "Unsorted", "mode": "unsorted", "icon": "list", "shortcut": "Ctrl+F7" }
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
        switch (sortModeName()) {
        case "extension": return "Extension"
        case "time": return "Time"
        case "size": return "Size"
        case "unsorted": return "Unsorted"
        default: return "Name"
        }
    }

    function sortIsAscending() {
        const mode = sortModeName()
        const reversed = panel.sortReverse === true
        return mode === "time" || mode === "size"
                ? reversed : !reversed
    }

    function sortDirectionIconName() {
        return sortIsAscending() ? "arrow-up" : "arrow-down"
    }

    function chooseSort(choice) {
        hostWindow.action({
            "action": "panel.sort",
            "side": panel.side,
            "mode": choice.mode
        })
    }

    function chooseRenderer(choice) {
        if (!rendererChoiceEnabled(choice))
            return
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
        if (nextHost)
            hostWindow.setGalleryPanelHost(panel.side, nextHost)
    }

    readonly property real nativeSplitPosition: hostWindow.nativePanelSplitPosition()

    function synchronizeLoadingIndicator() {
        if (backendLoading) {
            loadingIndicatorDelay.restart()
            return
        }
        loadingIndicatorDelay.stop()
        loadingIndicatorPulse.stop()
        loadingIndicatorVisible = false
        loadingIndicatorFrame = 0
    }

    onBackendLoadingChanged: synchronizeLoadingIndicator()
    Component.onCompleted: synchronizeLoadingIndicator()

    Timer {
        id: loadingIndicatorDelay
        interval: 120
        repeat: false
        onTriggered: {
            if (!panelRoot.backendLoading)
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
            if (!panelRoot.backendLoading) {
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
    y: nativeLayout ? menuBar.height : hostWindow.pxY(panel.y) + topChromeOffset
    width: nativeLayout
           ? hostWindow.nativePanelWidth(Number(panel.side || 0))
           : hostWindow.pxW(panel.w)
    height: nativeLayout ? Math.max(1, hostWindow.height - menuBar.height - hostWindow.commandLineHeight(hostWindow.shellFrame()) - hostWindow.keyBarHeight()) : Math.max(1, hostWindow.pxH(panel.h) - topChromeOffset)
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
                ? Math.max(22, hostWindow.ch) + hostWindow.verticalContentSpacing : 0
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
            var before = 0
            for (var i = 0; i < index; ++i)
                before += Math.max(1, Number(columns[i].width || 1))
            var contentWidth = Math.max(1, width
                                        - hostWindow.panelContentSpacing * 2)
            return hostWindow.panelContentSpacing
                    + Math.round(contentWidth * before
                                 / totalColumnWidth)
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
                    anchors.fill: parent
                    anchors.leftMargin: hostWindow.panelRowInnerSpacing
                    anchors.rightMargin: hostWindow.panelRowInnerSpacing
                    text: hostWindow.cleanText(modelData.title)
                    color: modelData.sortable
                           ? hostWindow.chromeText : hostWindow.mutedText
                    font.pixelSize: 12
                    verticalAlignment: Text.AlignVCenter
                    horizontalAlignment: index > 0
                                         ? Text.AlignRight
                                         : Text.AlignLeft
                    elide: Text.ElideRight
                }

                Rectangle {
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    width: 1
                    height: Math.max(1, parent.height
                                     - hostWindow.columnSeparatorVerticalMargin * 2)
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

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: 1
            color: hostWindow.separatorColor
            opacity: 0.7
        }
    }

    Loader {
        id: galleryPanelContent
        objectName: "galleryPanelContent-" + Number(panel.side || 0)
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.leftMargin: hostWindow.panelContentSpacing
        anchors.rightMargin: hostWindow.panelContentSpacing
        anchors.top: parent.top
        anchors.topMargin: panelHeader.height + columnHeader.height
        anchors.bottom: status.top
        z: 1
        // Each side owns one persistent instance of the unified renderer.
        // Covering a panel, hiding it with Ctrl+O, or switching layout mode
        // only changes visibility/state; it never reconstructs the host.
        active: true
        // `status` is also the id of the panel footer below. Qualify the
        // Loader property explicitly so QML cannot resolve that sibling
        // object here and hide an otherwise ready, populated renderer.
        visible: panelRoot.visible
                 && galleryPanelContent.status === Loader.Ready
        source: galleryController.available ? galleryController.panelComponentUrl : ""

        onItemChanged: panelRoot.updateRegisteredGalleryPanelHost()

        onLoaded: {
            if (!item)
                return
            item.side = panel.side
            item.panel = Qt.binding(() => panelRoot.panel)
            if (typeof item.layoutState !== "undefined")
                item.layoutState = Qt.binding(() => panelRoot.layoutState)
            item.bridge = qtGallery
            item.keySink = grid
            item.theme = Qt.binding(() => panelRoot.galleryTheme)
            item.metrics = Qt.binding(() => panelRoot.galleryMetrics)
            item.devicePixelRatio = Qt.binding(
                () => hostWindow.screen ? hostWindow.screen.devicePixelRatio : 1.0)
            item.defaultListDensity = Qt.binding(
                () => hostWindow.snapPx(Math.max(22, hostWindow.ch * 1.1)))
            // Lightweight QML test embedders may supply an older panel
            // host without this optional input property.
            if (typeof item.mouseWheelMode !== "undefined")
                item.mouseWheelMode = Qt.binding(
                    () => hostWindow.mouseWheelMode)
            item.panelActive = Qt.binding(
                () => panelRoot.visible
                      && panelRoot.panelIsActive
                      && !galleryController.viewerVisible
                      && !hostWindow.needsFallbackGrid()
                      && !hostWindow.hasDocumentSurface()
                      && !hostWindow.hasOperationsQueueSurface()
                      && !hostWindow.hasBlockingOverlay())
            item.commandLineHasText = Qt.binding(() => {
                var commandLine = hostWindow.commandLineFrame()
                return hostWindow.cleanText(commandLine.text).length > 0
            })
            item.fastFindActive = Qt.binding(
                () => panelRoot.panel.fastFind === true)
            if (item.panelActive)
                item.forceActiveFocus()
            panelRoot.updateRegisteredGalleryPanelHost()
        }
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
        anchors.bottom: status.top
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
                font.pixelSize: 13
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

    Rectangle {
        id: fastFindOverlay
        objectName: "panelFastFindOverlay-"
                    + Number(panel.side || 0)
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: status.top
        anchors.bottomMargin: hostWindow.panelContentSpacing
        readonly property real desiredWidth:
            Math.max(hostWindow.snapPx(220),
                     fastFindQuery.implicitWidth + hostWindow.snapPx(64))
        width: Math.max(1,
                        Math.min(parent.width
                                 - hostWindow.panelContentSpacing * 2,
                                 desiredWidth))
        height: hostWindow.snapPx(36)
        visible: panel.fastFind === true
        z: 4
        clip: true
        radius: hostWindow.snapPx(8)
        color: hostWindow.dialogBg
        border.width: hostWindow.separatorWidth
        border.color: hostWindow.controlBorder

        FontMetrics {
            id: fastFindFontMetrics
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: 13
        }

        HostPixelAlignedImage {
                    hostWindow: panelRoot.hostWindow
            id: fastFindIcon
            objectName: "panelFastFindIcon-"
                        + Number(panel.side || 0)
            anchors.left: parent.left
            anchors.leftMargin: hostWindow.snapPx(10)
            anchors.verticalCenter: parent.verticalCenter
            width: hostWindow.snapPx(15)
            height: hostWindow.snapPx(15)
            smooth: false
            source: hostWindow.lucideIconSource("search", 15,
                                         hostWindow.dialogAccent)
        }

        Text {
            id: fastFindQuery
            objectName: "panelFastFindText-"
                        + Number(panel.side || 0)
            anchors.left: fastFindIcon.right
            anchors.leftMargin: hostWindow.snapPx(8)
            anchors.right: parent.right
            anchors.rightMargin: hostWindow.snapPx(10)
            anchors.verticalCenter: parent.verticalCenter
            text: hostWindow.cleanText(panel.fastFindText)
            color: hostWindow.textColor
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: 13
            elide: Text.ElideLeft
            verticalAlignment: Text.AlignVCenter
        }

        Rectangle {
            id: fastFindCursor
            objectName: "panelFastFindCursor-"
                        + Number(panel.side || 0)
            property bool blinkOn: true
            readonly property real textAdvance:
                fastFindFontMetrics.advanceWidth(fastFindQuery.text)
            x: fastFindQuery.x
               + Math.min(fastFindQuery.width, textAdvance)
            y: fastFindQuery.y + hostWindow.snapPx(2)
            width: hostWindow.snapPx(2)
            height: Math.max(hostWindow.snapPx(1),
                             fastFindQuery.height - hostWindow.snapPx(4))
            color: hostWindow.textColor
            visible: panel.fastFind === true
            opacity: blinkOn ? 1 : 0
            z: 2

            function restartBlink() {
                blinkOn = true
                if (visible)
                    fastFindCursorBlinkTimer.restart()
            }

            onVisibleChanged: {
                if (visible)
                    restartBlink()
            }

            Connections {
                target: root
                function onKeyboardActivityRevisionChanged() {
                    fastFindCursor.restartBlink()
                }
            }

            Timer {
                id: fastFindCursorBlinkTimer
                interval: 520
                running: fastFindCursor.visible
                repeat: true
                onTriggered: fastFindCursor.blinkOn = !fastFindCursor.blinkOn
            }
        }
    }

    Rectangle {
        id: status
        objectName: "panelStatus-" + Number(panel.side || 0)
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        visible: panel.showFileInfo === true
        height: visible
                ? Math.max(24, hostWindow.ch * 1.15)
                  + hostWindow.verticalContentSpacing
                : 0
        color: "transparent"
        // Keep the footer above dynamically loaded semantic content even
        // while Loader/anchor geometry is settling after scene changes.
        z: 3

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: hostWindow.separatorWidth
            color: hostWindow.separatorColor
        }

        Text {
            objectName: "panelStatusSelection-"
                        + Number(panel.side || 0)
            anchors.left: parent.left
            anchors.verticalCenter: parent.verticalCenter
            anchors.leftMargin: hostWindow.panelTextInset
            text: hostWindow.cleanText(panel.selectedCount) + " selected"
            color: hostWindow.mutedText
            font.pixelSize: 12
        }

        Text {
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            anchors.rightMargin: hostWindow.panelTextInset
            text: hostWindow.cleanText(panel.totalCount) + " items"
            color: hostWindow.mutedText
            font.pixelSize: 12
        }
    }
}
