pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import F4QtHost 1.0

// Pane roles come from the owner. Terminal rectangles remain content hints;
// the live native rectangle owns docking throughout a resize gesture.
Item {
    id: body
    objectName: "settingsDialogBody"
    required property ApplicationWindow hostWindow
    required property real rightGutter
    property var widgets: []
    property string selectedNativePage: ""
    readonly property var nativePages: hostWindow.nativeSettingsPages || []
    readonly property var nativePage: nativePages.find(page => page.pageId === selectedNativePage) || null
    property var visitedNativePages: []
    property int pageRevision: 0
    onSelectedNativePageChanged: {
        if (selectedNativePage && !visitedNativePages.includes(selectedNativePage))
            visitedNativePages = visitedNativePages.concat([selectedNativePage])
        nativeViewport.contentX = 0
        nativeViewport.contentY = 0
    }
    function resetNativePages() {
        for (let i = 0; i < pageLoaders.count; ++i) {
            const loader = pageLoaders.itemAt(i)
            if (loader && loader.item) loader.item.resetDraft()
        }
    }
    function activateWidget(widget) {
        const role = widget.layoutRole
        if (role === "apply" || role === "accept") {
            for (const operation of ["validateDraft", "applyDraft"]) {
                for (let i = 0; i < pageLoaders.count; ++i) {
                    const loader = pageLoaders.itemAt(i)
                    if (loader && loader.item && typeof loader.item[operation] === "function"
                            && !loader.item[operation]()) {
                        selectedNativePage = nativePages[i].pageId
                        return
                    }
                }
            }
        } else if (role === "cancel") {
            resetNativePages()
        }
        hostWindow.action({target: widget.id, action: "control.activate"})
    }
    function displayWidget(widget) {
        if (widget.layoutRole === "navigation") return categoryWidget(widget)
        if (nativePage && widget.layoutRole === "content-title")
            return Object.assign({}, widget, {text: nativePage.title, hotkey: "", dimmed: false})
        if (nativePage && widget.layoutRole === "footer-status")
            return Object.assign({}, widget, {text: nativeContent.item ? nativeContent.item.status : ""})
        return widget
    }
    readonly property var displayedWidgets: !widgets.some(w => w.layoutRole === "footer-status")
        ? widgets.concat([{id: "native-settings-status", kind: "text", text: "", layoutRole: "footer-status"}]) : widgets
    // Compose frontend pages into the category view only. Core row indices stay
    // unchanged, and local rows never become semantic actions or Go providers.
    function categoryWidget(core) {
        const rows = core.rows || []
        const localIndex = nativePages.findIndex(page => page.pageId === selectedNativePage)
        return Object.assign({}, core, {
            rows: rows.concat(nativePages.map(page => ({cells: [page.title]}))),
            itemIcons: rows.map((row, index) => (core.itemIcons || [])[index] || "")
                           .concat(nativePages.map(page => page.iconName)),
            cursor: localIndex >= 0 ? rows.length + localIndex : core.cursor
        })
    }
    function selectCategory(core, action, index) {
        const coreCount = (core.rows || []).length
        if (index >= coreCount) {
            const page = nativePages[index - coreCount]
            if (page) selectedNativePage = page.pageId
            return
        }
        selectedNativePage = ""
        hostWindow.action({target: core.id, action: action, index: index})
    }
    signal closeRequested()
    Connections {
        target: body.hostWindow
        function onNativeSettingsPageRequested(pageId) {
            if (!body.visible) return
            body.selectedNativePage = pageId
            body.hostWindow.requestedNativeSettingsPage = ""
        }
    }
    onVisibleChanged: {
        if (visible && hostWindow.requestedNativeSettingsPage !== "") {
            selectedNativePage = hostWindow.requestedNativeSettingsPage
            hostWindow.requestedNativeSettingsPage = ""
        } else if (!visible) {
            resetNativePages()
            selectedNativePage = ""
            visitedNativePages = []
        }
    }
    Component.onCompleted: {
        if (visible && hostWindow.requestedNativeSettingsPage !== "") {
            selectedNativePage = hostWindow.requestedNativeSettingsPage
            hostWindow.requestedNativeSettingsPage = ""
        }
    }
    readonly property real gap: hostWindow.snapPx(12)
    readonly property real lineHeight: hostWindow.snapPx(Math.max(22, hostWindow.font.pixelSize + 4))
    readonly property real controlHeight: hostWindow.snapPx(Math.max(32, hostWindow.font.pixelSize + 16))
    readonly property real navigationWidth: hostWindow.snapPx(Math.min(260, Math.max(170, width * .26)))
    readonly property bool hasNavigation: widgets.some(w => w.layoutRole === "navigation" && w.visible !== false)
    readonly property real mainX: hasNavigation ? navigationWidth + gap : 0
    readonly property real mainWidth: Math.max(1, width - mainX)
    readonly property real footerY: Math.max(0, height - controlHeight)
    readonly property real paneBottom: Math.max(0, footerY - gap)
    readonly property bool wideHelp: mainWidth >= 720
    readonly property real helpWidth: hostWindow.snapPx(Math.min(280, mainWidth * .3))
    readonly property real helpHeight: hostWindow.snapPx(Math.min(110, Math.max(50, paneBottom * .22)))
    readonly property real pageY: hasNavigation ? lineHeight + gap : 0
    readonly property real pageWidth: Math.max(1, mainWidth - (wideHelp ? helpWidth + gap : 0))
    readonly property real pageHeight: Math.max(1, paneBottom - pageY - (wideHelp ? 0 : helpHeight + gap))
    readonly property real buttonWidth: hostWindow.snapPx(Math.min(104, (width - gap * 2) / 3))
    readonly property bool hasMatches: widgets.some(w => w.layoutRole === "search-matches" && w.visible !== false)
    readonly property bool hasClear: widgets.some(w => w.layoutRole === "search-clear" && w.visible !== false)
    readonly property real searchY: lineHeight + hostWindow.snapPx(4)
    readonly property real navigationY: searchY + controlHeight + gap + (hasMatches ? lineHeight : 0)

    function rectangle(role) {
        const tools = (hasClear ? controlHeight : 0) + (hasMatches ? 2 * controlHeight : 0)
        switch (role) {
        case "search-label": return Qt.rect(0, 0, navigationWidth, lineHeight)
        case "search": return Qt.rect(0, searchY, Math.max(1, navigationWidth - tools), controlHeight)
        case "search-clear": return Qt.rect(navigationWidth - tools, searchY, controlHeight, controlHeight)
        case "search-previous": return Qt.rect(navigationWidth - 2 * controlHeight, searchY, controlHeight, controlHeight)
        case "search-next": return Qt.rect(navigationWidth - controlHeight, searchY, controlHeight, controlHeight)
        case "search-matches": return Qt.rect(0, searchY + controlHeight + gap, navigationWidth, lineHeight)
        case "navigation": return Qt.rect(0, navigationY, navigationWidth, Math.max(1, paneBottom - navigationY))
        case "content-title": return Qt.rect(mainX, 0, nativePage ? mainWidth : pageWidth, lineHeight)
        case "content": return Qt.rect(mainX, pageY, pageWidth, pageHeight)
        case "description": return wideHelp
            ? Qt.rect(mainX + pageWidth + gap, 0, helpWidth, paneBottom)
            : Qt.rect(mainX, paneBottom - helpHeight, mainWidth, helpHeight)
        case "apply": return Qt.rect(width - 3 * buttonWidth - 2 * gap, footerY, buttonWidth, controlHeight)
        case "accept": return Qt.rect(width - 2 * buttonWidth - gap, footerY, buttonWidth, controlHeight)
        case "cancel": return Qt.rect(width - buttonWidth, footerY, buttonWidth, controlHeight)
        case "footer-status": return Qt.rect(0, footerY, Math.max(1, width - 3 * (buttonWidth + gap)), controlHeight)
        default: return Qt.rect(0, 0, 0, 0)
        }
    }

    Repeater {
        model: SemanticChildrenModel { widgets: body.displayedWidgets }
        delegate: SemanticWidgetDelegate {
            required property var widgetData
            required property var labelData
            readonly property rect placement: body.rectangle(widgetData.layoutRole || "")
            hostWindow: body.hostWindow
            widget: body.displayWidget(widgetData)
            buttonAction: () => body.activateWidget(widgetData)
            tableKeyboardNavigation: widgetData.layoutRole === "navigation"
            tableRowAction: (action, index) => {
                if (widgetData.layoutRole === "navigation") body.selectCategory(widgetData, action, index)
                else hostWindow.action({target: widgetData.id, action: action, index: index})
            }
            x: hostWindow.snapPx(placement.x)
            y: hostWindow.snapPx(placement.y)
            width: hostWindow.snapPx(placement.width)
            height: hostWindow.snapPx(placement.height)
            maximumWidth: width
            visible: widgetData.visible !== false && placement.width > 0
                     && (!body.nativePage || !["content", "description"].includes(widgetData.layoutRole))
        }
    }
    Flickable {
        id: nativeViewport
        objectName: "nativeSettingsViewport"
        visible: !!body.nativePage
        x: hostWindow.snapPx(body.mainX)
        y: hostWindow.snapPx(body.pageY)
        width: hostWindow.snapPx(body.mainWidth)
        height: hostWindow.snapPx(Math.max(1, body.paneBottom - body.pageY))
        clip: true
        pixelAligned: true
        contentWidth: nativeContent.width
        contentHeight: nativeContent.height
        boundsBehavior: Flickable.StopAtBounds
        Item {
            id: nativeContent
            objectName: "nativeSettingsContent"
            readonly property var item: {
                const revision = body.pageRevision
                const index = body.nativePages.findIndex(page => page.pageId === body.selectedNativePage)
                const loader = pageLoaders.itemAt(index)
                return loader ? loader.item : null
            }
            width: hostWindow.snapPx(Math.max(nativeViewport.width, item ? item.implicitWidth : 0))
            height: hostWindow.snapPx(Math.max(nativeViewport.height, item ? item.implicitHeight : 0))
            Repeater {
                id: pageLoaders
                model: body.nativePages
                delegate: Loader {
                    id: pageLoader
                    required property var modelData
                    anchors.fill: parent
                    active: body.visitedNativePages.includes(modelData.pageId)
                    visible: body.selectedNativePage === modelData.pageId
                    sourceComponent: modelData.content
                    onLoaded: { ++body.pageRevision; if (visible) item.forceActiveFocus() }
                    Connections {
                        target: pageLoader.item
                        ignoreUnknownSignals: true
                        function onCloseRequested() { body.resetNativePages(); body.closeRequested() }
                    }
                }
            }
        }
        ScrollBar.vertical: F4ScrollBar {
            objectName: "nativeSettingsVerticalScrollBar"
            // Keep the attached scroll ratios, but place the control outside
            // the clipped viewport in the dialog's existing right gutter.
            parent: body
            x: body.hostWindow.snapPx(body.width + body.rightGutter - width - 4)
            y: nativeViewport.y
            height: nativeViewport.height
            hostWindow: body.hostWindow
            policy: ScrollBar.AsNeeded
            visible: nativeViewport.visible && nativeViewport.contentHeight > nativeViewport.height + 0.5 / body.hostWindow.dpr
        }
        ScrollBar.horizontal: F4ScrollBar {
            objectName: "nativeSettingsHorizontalScrollBar"
            hostWindow: body.hostWindow
            policy: ScrollBar.AsNeeded
            visible: nativeViewport.contentWidth > nativeViewport.width + 0.5 / body.hostWindow.dpr
        }
    }
}
