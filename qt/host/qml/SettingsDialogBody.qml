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
    property bool sectionsOnly: false
    property string selectedNativePage: ""
    readonly property var nativePages: sectionsOnly ? [] : hostWindow.nativeSettingsPages || []
    readonly property var nativePage: nativePages.find(page => page.pageId === selectedNativePage) || null
    readonly property string sectionTitle: hostWindow.cleanText(nativePage ? nativePage.title
        : (widgets.find(widget => widget.layoutRole === "content-title" && widget.visible !== false) || {}).text || "")
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
    readonly property var searchToolIcons: ({
        "search-clear": "x", "search-previous": "chevron-up", "search-next": "chevron-down"
    })
    function displayWidget(widget) {
        if (widget.layoutRole === "description")
            return Object.assign({}, widget, {helpStacked: wideHelp})
        if (widget.layoutRole === "content")
            return Object.assign({}, widget, {
                nativeScrollOwner: sectionsOnly,
                scrollbarContentGap: hostWindow.snapPx(16),
                unscrolledRightInset: hasNavigation ? (wideHelp ? 2 * gap - helpGap : gap) : 0
            })
        if (widget.layoutRole === "search") {
            const label = widgets.find(w => w.layoutRole === "search-label") || {}
            return Object.assign({}, widget, {iconName: hasClear ? "" : "search",
                leadingInset: hasClear ? searchToolSize - hostWindow.snapPx(4) : 0,
                trailingInset: hasMatches ? 2 * searchToolSize : 0,
                cornerRadius: searchHeight / 2,
                focusRingWidth: 3,
                placeholderText: hostWindow.cleanText(label.text || qsTr("Search")).replace(/[:：]\s*$/, "")})
        }
        const toolIcon = searchToolIcons[widget.layoutRole]
        if (toolIcon)
            return Object.assign({}, widget, {text: "", iconName: toolIcon, toolTipText: widget.text,
                variant: "tool", flat: true, cornerRadius: searchToolSize / 2})
        if (widget.layoutRole === "navigation") return categoryWidget(widget)
        if (nativePage && widget.layoutRole === "content-title")
            return Object.assign({}, widget, {text: nativePage.title, hotkey: "", dimmed: false})
        if (nativePage && widget.layoutRole === "footer-status")
            return Object.assign({}, widget, {text: nativeContent.item ? nativeContent.item.status : ""})
        return widget
    }
    readonly property var displayedWidgets: {
        const fields = widgets.filter(widget => !isFooterButton(widget))
        return !fields.some(w => w.layoutRole === "footer-status")
            ? fields.concat([{id: "native-settings-status", kind: "text", text: "", layoutRole: "footer-status"}]) : fields
    }
    // Compose frontend pages into the category view only. Core row indices stay
    // unchanged, and local rows never become semantic actions or Go providers.
    function categoryWidget(core) {
        const rows = core.rows || []
        const localIndex = nativePages.findIndex(page => page.pageId === selectedNativePage)
        return Object.assign({}, core, {
            settingsNavigation: true,
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
    Keys.onEscapePressed: event => {
        console.debug("[FIX:settings] Escape closes settings from focused section")
        resetNativePages()
        closeRequested()
        event.accepted = true
    }
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
    readonly property real navigationWidth: hostWindow.snapPx(Math.min(232, width * .42))
    readonly property bool hasNavigation: widgets.some(w => w.layoutRole === "navigation" && w.visible !== false)
    readonly property real mainX: hasNavigation ? navigationWidth + gap * 2 : 0
    readonly property real mainWidth: Math.max(1, width - mainX)
    readonly property real footerY: Math.max(0, height - controlHeight)
    readonly property real paneBottom: Math.max(0, footerY - gap)
    readonly property bool wideHelp: mainWidth >= 720
    readonly property real helpWidth: hostWindow.snapPx(Math.min(280, mainWidth * .3))
    readonly property real helpHeight: hostWindow.snapPx(Math.min(112, Math.max(90, paneBottom * .22)))
    readonly property real contentStartY: hasNavigation ? gap : 0
    readonly property bool hasEditorPreview: widgets.some(w => w.layoutRole === "editor-preview")
    readonly property real editorPreviewHeight: hasEditorPreview ? hostWindow.snapPx(mainWidth < 560 ? 112 : 64) : 0
    readonly property real pageY: contentStartY + (hasEditorPreview ? editorPreviewHeight + gap : 0)
    readonly property real helpGap: hostWindow.snapPx(4)
    readonly property real pageWidth: Math.max(1, mainWidth - (wideHelp ? helpWidth + helpGap : 0))
    readonly property bool fileGroupsOverview: widgets.some(w => w.layoutRole === "content" && w.fileGroupsOverview === true)
    readonly property bool fullPage: widgets.some(w => w.layoutRole === "content" && w.fullPage === true)
    readonly property real pageHeight: Math.max(1, paneBottom - pageY - (wideHelp ? 0 : helpHeight + gap))
    readonly property real buttonWidth: hostWindow.snapPx(Math.min(104, (width - gap * 2) / 3))
    function isFooterButton(widget) {
        return ["apply", "accept", "cancel"].includes(widget.layoutRole)
    }
    readonly property bool hasMatches: widgets.some(w => w.layoutRole === "search-matches" && w.visible !== false)
    readonly property bool hasClear: widgets.some(w => w.layoutRole === "search-clear" && w.visible !== false)
    readonly property real sidebarInset: hostWindow.snapPx(10)
    readonly property real searchY: sidebarInset
    readonly property real searchHeight: hostWindow.snapPx(Math.max(28, hostWindow.uiTextSize(13) + 12))
    readonly property real searchToolSize: hostWindow.snapPx(searchHeight - 4)
    readonly property real navigationY: searchY + searchHeight + sidebarInset + (hasMatches ? lineHeight : 0)

    Rectangle {
        objectName: "settingsSidebarSeparator"
        visible: body.hasNavigation
        x: body.navigationWidth
        y: 0
        width: hostWindow.snapPx(1)
        height: body.height + body.rightGutter - hostWindow.snapPx(1)
        color: hostWindow.separatorColor
        opacity: 0.55
    }

    Rectangle {
        objectName: "settingsSidebarBackground"
        visible: body.hasNavigation
        x: hostWindow.snapPx(1)
        y: 0
        width: body.navigationWidth - x
        height: body.height + body.rightGutter - hostWindow.snapPx(1)
        color: body.hostWindow.dialogHeaderBg
        bottomLeftRadius: hostWindow.snapPx(8)
    }

    function rectangle(role) {
        const toolY = searchY + hostWindow.snapPx((searchHeight - searchToolSize) / 2)
        switch (role) {
        case "search-label": return Qt.rect(0, 0, 0, 0)
        case "search": return Qt.rect(sidebarInset, searchY, Math.max(1, navigationWidth - 2 * sidebarInset), searchHeight)
        case "search-clear": return Qt.rect(sidebarInset + hostWindow.snapPx(2), toolY, searchToolSize, searchToolSize)
        case "search-previous": return Qt.rect(navigationWidth - sidebarInset - 2 * searchToolSize - hostWindow.snapPx(2), toolY, searchToolSize, searchToolSize)
        case "search-next": return Qt.rect(navigationWidth - sidebarInset - searchToolSize - hostWindow.snapPx(2), toolY, searchToolSize, searchToolSize)
        case "search-matches": return Qt.rect(sidebarInset, searchY + searchHeight, navigationWidth - 2 * sidebarInset, lineHeight)
        case "navigation": return Qt.rect(sidebarInset, navigationY, navigationWidth - sidebarInset,
            Math.max(1, height + rightGutter - hostWindow.snapPx(1) - sidebarInset - navigationY))
        case "content-title": return Qt.rect(mainX, gap, nativePage ? mainWidth : pageWidth, lineHeight)
        case "editor-preview": return Qt.rect(mainX, contentStartY, mainWidth, editorPreviewHeight)
        case "content": return Qt.rect(mainX, pageY, (fileGroupsOverview || sectionsOnly || fullPage) ? mainWidth : pageWidth, (fileGroupsOverview || sectionsOnly || fullPage) ? paneBottom - pageY : pageHeight)
        case "description": return wideHelp
            ? Qt.rect(mainX + pageWidth + helpGap, gap, helpWidth, paneBottom - gap)
            : Qt.rect(mainX, paneBottom - helpHeight, mainWidth, helpHeight)
        case "footer-status": return Qt.rect(mainX, footerY, Math.max(0, footerButtons.x - mainX - gap), controlHeight)
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
            z: body.searchToolIcons[widgetData.layoutRole] ? 2 : 0
            y: hostWindow.snapPx(placement.y)
            width: hostWindow.snapPx(placement.width)
            height: hostWindow.snapPx(placement.height)
            maximumWidth: width
            visible: widgetData.visible !== false && placement.width > 0 && !body.isFooterButton(widgetData)
                     && !(body.hasNavigation && widgetData.layoutRole === "content-title"
                          && !body.nativePage)
                     && !((body.fileGroupsOverview || body.sectionsOnly || body.fullPage) && widgetData.layoutRole === "description")
                     && !(body.sectionsOnly && widgetData.layoutRole === "apply")
                     && (!body.nativePage || !["content", "description"].includes(widgetData.layoutRole))
        }
    }
    DialogButtonBox {
        id: footerButtons
        objectName: "settingsButtonBox"
        // Deliberately leave buttonLayout unset: Qt obtains the platform's
        // policy, including the active desktop convention on Linux.
        x: body.hostWindow.snapPx(body.width - width)
        y: body.footerY
        width: implicitWidth
        height: body.controlHeight
        padding: 0
        spacing: body.gap
        background: null
        alignment: Qt.AlignRight
        Repeater {
            model: SemanticChildrenModel {
                widgets: body.widgets.filter(widget => body.isFooterButton(widget)
                    && widget.visible !== false && !(body.sectionsOnly && widget.layoutRole === "apply"))
            }
            delegate: DialogButton {
                required property var widgetData
                readonly property var modelData: widgetData
                objectName: "dialogWidget-" + body.hostWindow.cleanText(modelData.id) + "Button"
                hostWindow: body.hostWindow
                implicitWidth: body.buttonWidth
                implicitHeight: body.controlHeight
                text: body.hostWindow.cleanText(modelData.text)
                mnemonicHotkey: body.hostWindow.cleanText(modelData.hotkey)
                toolTipText: body.hostWindow.cleanText(modelData.toolTipText)
                enabled: modelData.disabled !== true
                semanticFocus: modelData.focused === true
                DialogButtonBox.buttonRole: modelData.layoutRole === "apply" ? DialogButtonBox.ApplyRole
                    : modelData.layoutRole === "accept" ? DialogButtonBox.AcceptRole : DialogButtonBox.RejectRole
                onClicked: body.activateWidget(modelData)
            }
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
        interactive: contentHeight > height || contentWidth > width
        DialogWheelScroll { viewport: nativeViewport; hostWindow: body.hostWindow }
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
