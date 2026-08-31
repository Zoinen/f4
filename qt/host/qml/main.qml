import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl
import QtQuick.Layouts
import QtQuick.Shapes
import F4QtHost 1.0
import ZoinGallery 1.0 as ZG
import QWindowKit 1.0

ApplicationWindow {
    id: root

    width: Math.max(720, Math.ceil(qtShell.initialCols * grid.cellWidth))
    height: Math.max(460, Math.ceil(qtShell.initialRows * grid.cellHeight))
    minimumWidth: 320
    minimumHeight: 240
    topPadding: 0
    leftPadding: 0
    rightPadding: 0
    bottomPadding: 0
    // main.cpp restores the saved geometry while this window is hidden and
    // exposes it only after the first semantic scene has populated the native
    // surface.  Avoid presenting the empty QML shell during startup.
    visible: false
    title: fallbackExplanation !== ""
           ? "f4 [Using text presentation: " + fallbackExplanation + "]"
           : "f4"
    property bool isQWKLegacy: false
    property bool windowAgentReady: false
    property bool workspaceBarHitTestRegistered: false
    property int macWindowEffectApplyAttempts: 0
    property int keyboardActivityRevision: 0
    readonly property string guiMonospaceFontFamily:
        String(f4GuiFontFamily || "").length > 0 ? String(f4GuiFontFamily) : "Monaco"
    readonly property int guiMonospaceFontPixelSize:
        Number(f4GuiFontPixelSize) > 0 ? Number(f4GuiFontPixelSize)
                                       : (Qt.platform.os === "osx" ? 17 : 16)
    // Keep the QWK window surface opaque temporarily while the native
    // Windows/macOS transparency paths are unstable. QML content may still
    // use translucent colors inside the opaque window.
    readonly property bool useSystemTransparentWindowBackground: false
    readonly property bool supportsTransparentWindowBackground:
        useSystemTransparentWindowBackground && f4UsesQwk
        && (Qt.platform.os === "windows" || Qt.platform.os === "osx")
    readonly property bool useTransparentWindowBackground:
        supportsTransparentWindowBackground && !isQWKLegacy
    readonly property bool useMacNativeTitleBar:
        f4UsesQwk && Qt.platform.os === "osx"
    readonly property string macWindowGlassEffect:
        useMacNativeTitleBar ? "regular" : "none"
    readonly property string macWindowFallbackBlurEffect:
        useMacNativeTitleBar ? "dark" : "none"
    readonly property int macTitleBarLeftPadding:
        useMacNativeTitleBar && visibility !== Window.FullScreen
        ? macSystemButtonAreaLeftMargin + 74 : 0
    readonly property int macSystemButtonAreaLeftMargin: panelTextInset
    readonly property real titleBarContentVerticalOffset: 1
    property color windowBackgroundColor: "#1f242c"
    color: useTransparentWindowBackground ? "transparent" : root.windowBackgroundColor

    // Gallery receives full catalogs directly in C++. Keep those row payloads
    // out of QML, where QVariant-to-JavaScript conversion would otherwise run
    // synchronously on every directory transition.
    property var scene: qtShell.presentationScene || ({})
    // Compact panel/chrome updates keep the controller's scene caches
    // authoritative without invalidating the heavyweight presentation
    // binding. Project only their row-free state until a real scene replaces
    // it; catalog rows continue to live exclusively in the C++ Gallery model.
    property var workspaceTabsOverride: null
    property var menuBarOverride: null
    property var keyBarOverride: null
    property var toastOverride: null
    property var leftPanelPresentationOverride: null
    property var rightPanelPresentationOverride: null
    property var leftPanelLayoutStateOverride: null
    property var rightPanelLayoutStateOverride: null

    function mergePanelLayoutState(currentState, delta) {
        const next = delta || ({})
        const current = currentState || ({})
        const samePanel = String(current.id || "") !== ""
                && String(current.id || "") === String(next.id || "")
                && Number(current.catalogRevision || 0)
                   === Number(next.catalogRevision || 0)
        return Object.assign({}, samePanel ? current : ({}), next)
    }
    readonly property var workspaceTabs:
        workspaceTabsOverride !== null
        ? workspaceTabsOverride : (scene.workspaceTabs || ({}))
    readonly property var menuBarModel:
        menuBarOverride !== null ? menuBarOverride : (scene.menuBar || ({}))
    readonly property var keyBarModel:
        keyBarOverride !== null ? keyBarOverride : (scene.keyBar || ({}))
    readonly property var toastModel:
        toastOverride !== null ? toastOverride : (scene.toast || ({}))
    readonly property var workspaces: workspaceTabs.tabs || []
    // vtui exports only the active workspace.  Keep the last complete model
    // for each heavyweight native surface so opening the Operations Queue tab
    // is a visibility change, not destruction/recreation of the panel,
    // Gallery, Viewer or Editor object tree underneath it.
    property var retainedShellFrame: ({})
    property var retainedDocumentFrame: ({})
    property var retainedOperationsQueue: ({})
    property bool shellPresentationOverrideSet: false
    property var shellPresentationOverride: null
    property bool documentPresentationOverrideSet: false
    property var documentPresentationOverride: null
    property var documentSurfaceStateOverride: null
    property bool retainedShellSurfaceCreated: false
    property bool retainedDocumentSurfaceCreated: false
    property bool documentSurfacePrewarmed: false
    property bool retainedOperationsQueueCreated: false
    readonly property real dpr: root.screen ? root.screen.devicePixelRatio : 1.0
    // Qt exposes the render-type policy globally through QQuickWindow. Keep
    // the QML surface bound to the same policy so the setting can be edited
    // live and persisted with the rest of the theme.
    readonly property int fontRenderType:
        typeof qtTextRendering !== "undefined" && qtTextRendering
        ? qtTextRendering.renderType : Text.NativeRendering
    readonly property var fontRenderTypeOptions:
        typeof qtTextRendering !== "undefined" && qtTextRendering
        ? qtTextRendering.options
        : [
            { value: Text.QtRendering,
              name: "QtRendering",
              description: "Qt distance-field rendering" },
            { value: Text.NativeRendering,
              name: "NativeRendering",
              description: "Native platform text rasterization" }
        ]
    function fontRenderTypeOption(value) {
        const options = root.fontRenderTypeOptions || []
        for (let i = 0; i < options.length; ++i) {
            if (Number(options[i].value) === Number(value))
                return options[i]
        }
        return options.length > 0 ? options[0] : ({})
    }
    readonly property string fontRenderTypeName:
        typeof qtTextRendering !== "undefined" && qtTextRendering
        ? qtTextRendering.renderTypeName
        : String(root.fontRenderTypeOption(root.fontRenderType).name || "NativeRendering")
    readonly property string fontRenderTypeDescription:
        String(root.fontRenderTypeOption(root.fontRenderType).description || "")
    // Panel wheel input has two deliberately different contracts.  GUI mode
    // is the product default: wheel steps scroll the current presentation and
    // middle-click enters the reusable auto-scroll gesture.  Console mode is
    // opt-in and forwards native wheel/middle-button messages to Go.
    property string mouseWheelMode: "gui"
    readonly property var mouseWheelModeOptions: [
        { value: "console", name: "F4 console",
          description: "Send wheel and middle-button events to the F4 panel" },
        { value: "gui", name: "GUI scrolling",
          description: "Smoothly scroll the panel and use middle-button auto-scroll" }
    ]
    function mouseWheelModeOption(value) {
        const options = root.mouseWheelModeOptions || []
        for (let i = 0; i < options.length; ++i) {
            if (String(options[i].value) === String(value))
                return options[i]
        }
        return options.length > 0 ? options[options.length - 1] : ({})
    }
    readonly property string mouseWheelModeName:
        String(root.mouseWheelModeOption(root.mouseWheelMode).name
               || "GUI scrolling")
    readonly property string mouseWheelModeDescription:
        String(root.mouseWheelModeOption(root.mouseWheelMode).description || "")

    function setMouseWheelMode(value) {
        const normalized = String(value || "").toLowerCase()
        for (let i = 0; i < root.mouseWheelModeOptions.length; ++i) {
            if (String(root.mouseWheelModeOptions[i].value) === normalized) {
                root.mouseWheelMode = normalized
                return true
            }
        }
        return false
    }

    function showApplicationSettings() {
        themeColorConfigurator.show()
        themeColorConfigurator.requestActivate()
        themeColorConfigurator.raise()
    }

    function snapPx(val) {
        return Math.round(Number(val || 0) * root.dpr) / root.dpr
    }
    function dialogPixelOffsetX(item, target) {
        if (!item || !item.parent || !target)
            return 0
        let layoutRevision = target.width + target.height
        let ancestor = item
        for (let depth = 0; ancestor && depth < 12; ++depth) {
            layoutRevision += ancestor.x + ancestor.y
                    + ancestor.width + ancestor.height
            ancestor = ancestor.parent
        }
        const windowRoot = item.window ? item.window.contentItem : target
        const localPoint = item.parent.mapToItem(windowRoot, item.x, item.y)
        return root.snapPx(localPoint.x) - localPoint.x
                + layoutRevision * 0
    }
    function dialogPixelOffsetY(item, target) {
        if (!item || !item.parent || !target)
            return 0
        let layoutRevision = target.width + target.height
        let ancestor = item
        for (let depth = 0; ancestor && depth < 12; ++depth) {
            layoutRevision += ancestor.x + ancestor.y
                    + ancestor.width + ancestor.height
            ancestor = ancestor.parent
        }
        const windowRoot = item.window ? item.window.contentItem : target
        const localPoint = item.parent.mapToItem(windowRoot, item.x, item.y)
        return root.snapPx(localPoint.y) - localPoint.y
                + layoutRevision * 0
    }
    function iconPixelOffsetX(item) {
        if (!item || !item.parent || !root.contentItem)
            return 0
        // These reads keep the binding live when an ancestor is repositioned
        // by the window layout without changing the image's local x/y.
        const layoutRevision = root.width + root.height
                + root.panelSplitRatio + item.alignmentRevision
        const scenePoint = item.parent.mapToItem(root.contentItem,
                                                 item.x, item.y)
        return root.snapPx(scenePoint.x) - scenePoint.x
                + layoutRevision * 0
    }
    function iconPixelOffsetY(item) {
        if (!item || !item.parent || !root.contentItem)
            return 0
        const layoutRevision = root.width + root.height
                + root.panelSplitRatio + item.alignmentRevision
        const scenePoint = item.parent.mapToItem(root.contentItem,
                                                 item.x, item.y)
        return root.snapPx(scenePoint.y) - scenePoint.y
                + layoutRevision * 0
    }

    component PixelAlignedImage: Image {
        id: pixelAlignedImage
        property real alignmentRevision: 0
        transform: Translate {
            x: root.iconPixelOffsetX(pixelAlignedImage)
            y: root.iconPixelOffsetY(pixelAlignedImage)
        }
    }
    // Crisp pixel-grid separator width (e.g. 1px @ 100%, 2px @ 150%/175%/200%)
    readonly property real physicalSeparatorPixels: Math.max(1, Math.round(1 * root.dpr))
    readonly property real separatorWidth: physicalSeparatorPixels / root.dpr
    property real cw: Math.max(8, grid.cellWidth)
    property real ch: Math.max(17, grid.cellHeight)
    readonly property real menuBarHeight: snapPx(42)
    readonly property real workspaceTabMinWidth: 92
    readonly property real workspaceTabMaxWidth: 280
    readonly property real contentSpacing: 16
    readonly property real panelContentSpacing: 8
    readonly property real panelTextInset: 16
    readonly property real panelRowInnerSpacing:
        panelTextInset - panelContentSpacing
    readonly property real verticalContentSpacing: 8
    readonly property real pathRowExtraHeight: 4
    readonly property real columnSeparatorVerticalMargin: 6
    // Align the prompt glyphs with the leading icons in both file panels.
    readonly property real commandLineLeftMargin: panelTextInset
    readonly property real commandLineVerticalMargin: 8
    readonly property real semanticTextFontPixelSize: 13
    readonly property real actionBarVerticalMargin: 3
    readonly property real actionSeparatorVerticalMargin: 5
    readonly property real actionButtonHorizontalMargin: 8
    readonly property real menuItemHorizontalPadding: 14
    property color panelPathBg: "#26314152"
    property color commandLineBg: "#19000000"
    property color activeBorder: "#f0c95a"
    property color textColor: "#e8edf2"
    property color mutedText: "#9aa7b5"
    property color selectedBg: "#285d8f"
    property color panelSelectionBg: "#18456e"
    property color panelSelectionBorder: "#1d5888"
    property color titleBarBg: "#202833"
    property color fBarBg: "#202833"
    property color chromeText: "#d7e0ea"
    property color dialogBg: "#171e27"
    property color dialogHeaderBg: "#1b242f"
    property color controlBg: "#222c38"
    property color controlHoverBg: "#2a3745"
    property color controlPressedBg: "#10161e"
    property color controlBorder: "#3a495b"
    property color separatorColor: "#30363d"
    property color separatorHoverColor: "#464d55"
    property color separatorActiveColor: "#59616a"
    property color dialogAccent: "#4e9bd4"

    // ZoinGallery is a reusable QML module with its own visual states. Keep
    // those colors as first-class root properties so the configurator can
    // preview, persist, and reset them through the same path as f4's native
    // QML colors instead of handing the gallery a one-time palette snapshot.
    property color galleryPanelBackgroundColor: "transparent"
    property color galleryViewerBackgroundColor: "transparent"
    property color galleryTextColor: "#e8edf2"
    property color galleryMutedTextColor: "#9aa7b5"
    property color galleryFileTextColor: "#c4cbd3"
    property color galleryFolderTextColor: "#ffffff"
    property bool galleryNeutralFileTextColors: true
    property color galleryQuickSearchMatchColor: "#e8edf2"
    property color galleryDirectoryTextColor: "#98d8ff"
    property color galleryFolderIconColor: "#5ab2f1"
    property color galleryCursorColor: "#1d5888"
    property color galleryCursorBackgroundColor: "#18456e"
    property color galleryCursorBorderColor: "#1d5888"
    property color galleryCardCursorBorderColor: "#2777b8"
    property color gallerySelectionColor: "#ffd43b"
    property color galleryMarkedBackgroundColor: "#4f5037"
    property color galleryMarkedTextColor: "#ffd43b"
    property color galleryItemBackgroundColor: "transparent"
    property color galleryDirectoryBackgroundColor: "transparent"
    // A low-alpha white overlay lightens the actual surface beneath a brick,
    // including the intentionally transparent default panel/card surfaces.
    property color galleryItemHoverColor: "#0cffffff"
    property color galleryLabelBackgroundColor: "#aa101216"
    property color galleryPreviewBackdropColor: "#4d000000"
    property color gallerySeparatorColor: "#30363d"
    property color galleryHeaderTextColor: "#d7e0ea"
    property color galleryControlHoverColor: "#2a3745"
    property color galleryScrollBarHandleColor: "#4a4a4a"
    property color galleryScrollBarBackgroundHoverColor: "#676767"
    property color galleryScrollBarHoverColor: "#878787"
    property color galleryScrollBarPressedColor: "#505050"
    property color galleryScrollBarTrackHoverColor: "#0fffffff"
    property color galleryPathBackgroundColor: "transparent"
    property color galleryPathTextColor: "#e8edf2"
    property color galleryPathHoverColor: "#222c38"
    property color galleryPathItemHoverColor: "#2a3745"
    property color galleryPathItemPressedColor: "#10161e"
    readonly property real iconDevicePixelRatio:
        root.screen ? root.screen.devicePixelRatio : 1.0
    readonly property string fallbackExplanation: semanticFallbackReason()
    // Go supplies the saved keyboard/configuration split as four scalar shell
    // fields. Pointer dragging remains a presentation-local override: the
    // first assignment intentionally replaces this binding for the lifetime
    // of the window and never touches either heavyweight panel catalog.
    property real panelSplitRatio: semanticPanelSplitRatio()
    readonly property real panelMinimumWidth:
        Math.max(180, Math.min(280, cw * 22))
    readonly property bool nativeTwoPanelSurfaceActive:
        isAppScene()
        && !needsFallbackGrid()
        && !hasBlockingOverlay()
        && !hasDocumentSurface()
        && !hasOperationsQueueSurface()
        && !qtGallery.viewerVisible
        && shellFrame().terminalActive !== true
    readonly property bool nativeTwoPanelSurfaceVisible:
        isAppScene()
        && !needsFallbackGrid()
        && !hasDocumentSurface()
        && !hasOperationsQueueSurface()
        && !qtGallery.viewerVisible
        && shellFrame().terminalActive !== true
    readonly property real galleryViewerProgress: {
        // galleryViewerLayer loads galleryViewerSurface, which in turn loads
        // GalleryViewerHost. Keep the normal surface fully visible until that
        // host exists and its original transition actually starts.
        var surfaceLoader = galleryViewerLayer.item
        var viewerHost = surfaceLoader && surfaceLoader.item
                         ? surfaceLoader.item : null
        if (!viewerHost)
            return 0
        return Math.max(0, Math.min(1,
                    Number(viewerHost.surfaceProgress || 0)))
    }
    readonly property real normalSurfaceOpacity:
        hasDocumentSurface() || hasOperationsQueueSurface()
        ? 1 : 1 - galleryViewerProgress
    // The viewer needs the active persistent panel host throughout its reverse
    // animation. Retain non-owning QML object references by side and clear them
    // only when the corresponding panel tree is destroyed.
    property var leftGalleryPanelHost: null
    property var rightGalleryPanelHost: null
    // A Tab activation patch updates the controller's authoritative scene,
    // but deliberately does not invalidate the whole QML scene graph. Keep
    // the tiny active-side projection here until the next real scene update.
    // This preserves both persistent panel/Gallery instances and avoids
    // running every panel binding at keyboard-repeat frequency.
    property int panelActivationOverride: -1
    // Pointer drag keeps its semantic cursor+activation action deferred until
    // mouse-up, but cursor ownership is a two-panel visual invariant. Preview
    // the target side here so one mouse-down simultaneously reveals its new
    // cursor and hides the former side's cursor. Authoritative compact/full
    // scene state acknowledges this projection; the watchdog rolls it back if
    // no acknowledgement arrives.
    property int pointerPanelActivationOverride: -1
    property int pointerPanelActivationTimeoutMs: 6000
    // While a menu-bar submenu is open, pointer traversal must feel local.
    // Go remains authoritative for activation, but waiting for a complete
    // semantic-scene round trip just to paint the adjacent submenu creates a
    // visible delay and used to enqueue one action per mouse-move event.
    // Keep the last pointer selection across transient popup lifecycles until
    // the matching Go snapshot acknowledges it (or the menu closes/switches).
    // Loader destruction must never be allowed to clear this application state.
    property int menuBarPreviewIndex: -1
    property int menuPointerMenuIndex: -1
    property int menuPointerItemIndex: -1
    property int menuPointerSentItemIndex: -1
    property string menuPointerFrameId: ""
    property bool menuBarOpenedByPointer: false
    property bool menuBarPointerHasSelectedItem: false
    // Command history completion is intentionally presentation-local.  The
    // user's text remains authoritative until a real navigation gesture picks
    // one of these rows; this also makes held Up/Down keys immediate.
    property string autocompleteMenuId: ""
    property string autocompleteQuery: ""
    property string autocompleteItemsSignature: ""
    property int autocompleteSelectedIndex: -1

    WindowAgent {
        id: windowAgent
    }

    Component.onCompleted: {
        loadThemeFromPersistence()
        captureRetainedSurfaces()
        if (!f4UsesQwk)
            return

        windowAgent.setup(root)
        windowAgentReady = true

        if (Qt.platform.os === "windows") {
            if (supportsTransparentWindowBackground) {
                isQWKLegacy = windowAgent.setWindowAttribute("mica-alt", true) !== true
            } else {
                isQWKLegacy = true
            }
        } else if (useMacNativeTitleBar) {
            if (supportsTransparentWindowBackground) {
                applyPlatformWindowEffects()
            } else {
                isQWKLegacy = true
            }
        }

        windowAgent.setTitleBar(titleBar)
        if (Qt.platform.os !== "osx") {
            windowAgent.setHitTestVisible(appIcon)
        }
        windowAgent.setHitTestVisible(workspaceBar)
        workspaceBarHitTestRegistered = true
        if (useMacNativeTitleBar) {
            windowAgent.setSystemButtonArea(macSystemButtonArea)
        }
    }

    Timer {
        id: macWindowEffectRetryTimer
        interval: 100
        repeat: false
        onTriggered: root.applyPlatformWindowEffects()
    }

    function applyPlatformWindowEffects() {
        if (!windowAgentReady || !useMacNativeTitleBar
                || !supportsTransparentWindowBackground)
            return

        windowAgent.setWindowAttribute("blur-effect", "none")
        windowAgent.setWindowAttribute("glass-corner-radius", 0)
        windowAgent.setWindowAttribute("glass-tint-color", "none")

        const glassApplied = windowAgent.setWindowAttribute(
                                 "glass-effect", macWindowGlassEffect) === true
        const applied = glassApplied || windowAgent.setWindowAttribute(
                            "blur-effect", macWindowFallbackBlurEffect) === true
        isQWKLegacy = !applied
        if (!applied && macWindowEffectApplyAttempts < 10) {
            macWindowEffectApplyAttempts += 1
            macWindowEffectRetryTimer.restart()
        }
    }

    function activeAutocompleteFrame() {
        var menus = qtShell.commandMenus !== undefined
                && qtShell.commandMenus !== null
                ? qtShell.commandMenus : (scene.menus || [])
        for (var i = menus.length - 1; i >= 0; --i) {
            if (menus[i].role === "autocomplete")
                return menus[i]
        }
        return null
    }

    function autocompleteSignature(frame) {
        var items = frame && frame.items ? frame.items : []
        var parts = []
        for (var i = 0; i < items.length; ++i)
            parts.push(cleanText(items[i].text))
        return parts.join("\u001f")
    }

    function syncAutocompleteSelection() {
        var frame = activeAutocompleteFrame()
        if (!frame) {
            autocompleteMenuId = ""
            autocompleteQuery = ""
            autocompleteItemsSignature = ""
            autocompleteSelectedIndex = -1
            return
        }
        var id = cleanText(frame.id)
        var query = cleanText(frame.query)
        var signature = autocompleteSignature(frame)
        if (id !== autocompleteMenuId || query !== autocompleteQuery
                || signature !== autocompleteItemsSignature) {
            autocompleteMenuId = id
            autocompleteQuery = query
            autocompleteItemsSignature = signature
            autocompleteSelectedIndex = -1
        } else if (autocompleteSelectedIndex >= (frame.items || []).length) {
            autocompleteSelectedIndex = -1
        }
    }

    function navigateAutocomplete(delta) {
        var frame = activeAutocompleteFrame()
        var count = frame && frame.items ? frame.items.length : 0
        if (count < 1)
            return
        if (autocompleteSelectedIndex < 0)
            autocompleteSelectedIndex = delta < 0 ? count - 1 : 0
        else
            autocompleteSelectedIndex = (autocompleteSelectedIndex + delta
                                         + count) % count
    }

    function autocompleteSelectedText() {
        var frame = activeAutocompleteFrame()
        var items = frame && frame.items ? frame.items : []
        var index = autocompleteSelectedIndex
        return index >= 0 && index < items.length
                ? cleanText(items[index].text) : ""
    }

    function submitAutocomplete() {
        var frame = activeAutocompleteFrame()
        if (!frame)
            return
        var shell = shellFrame()
        var actionMap = {
            "target": cleanText(shell.id) !== "" ? shell.id : frame.id,
            "action": "command.submit"
        }
        if (autocompleteSelectedIndex >= 0)
            actionMap.text = autocompleteSelectedText()
        action(actionMap, true)
    }

    function completeAutocomplete() {
        var frame = activeAutocompleteFrame()
        if (!frame)
            return
        var shell = shellFrame()
        var actionMap = {
            "target": cleanText(shell.id) !== "" ? shell.id : frame.id,
            "action": "command.complete"
        }
        if (autocompleteSelectedIndex >= 0)
            actionMap.text = autocompleteSelectedText()
        action(actionMap, true)
    }

    function clearMenuPointerSelection() {
        menuPointerSyncTimer.stop()
        menuPointerMenuIndex = -1
        menuPointerItemIndex = -1
        menuPointerSentItemIndex = -1
        menuPointerFrameId = ""
    }

    Timer {
        id: menuPointerSyncTimer
        interval: 90
        onTriggered: {
            if (root.menuPointerMenuIndex < 0
                    || root.menuPointerItemIndex < 0
                    || root.menuPointerSentItemIndex
                       === root.menuPointerItemIndex)
                return
            root.menuPointerSentItemIndex = root.menuPointerItemIndex
            root.action({
                "action": "menuBar.itemSelect",
                "target": root.menuPointerFrameId,
                "menuIndex": root.menuPointerMenuIndex,
                "index": root.menuPointerItemIndex
            }, true)
        }
    }

    function menuBarItem(index) {
        var items = menuBarModel.items || []
        for (var i = 0; i < items.length; ++i) {
            if (Number(items[i].index) === Number(index))
                return items[i]
        }
        return null
    }

    function setGalleryPanelHost(side, panelHost) {
        if (Number(side) === 0)
            leftGalleryPanelHost = panelHost
        else if (Number(side) === 1)
            rightGalleryPanelHost = panelHost
    }

    function clearGalleryPanelHost(side, panelHost) {
        if (Number(side) === 0 && leftGalleryPanelHost === panelHost)
            leftGalleryPanelHost = null
        else if (Number(side) === 1 && rightGalleryPanelHost === panelHost)
            rightGalleryPanelHost = null
    }

    function galleryPanelHost(side) {
        if (Number(side) === 0)
            return leftGalleryPanelHost
        if (Number(side) === 1)
            return rightGalleryPanelHost
        return null
    }

    function commandLineFrame() {
        if (qtShell.commandLine !== undefined
                && qtShell.commandLine !== null)
            return qtShell.commandLine
        var shell = shellFrame()
        return shell && shell.commandLine ? shell.commandLine : ({})
    }

    function isAppScene() {
        return scene.schema === "app"
                && (scene.shell !== undefined
                    || scene.surface !== undefined
                    || scene.operationsQueue !== undefined)
    }

    function currentOperationsQueue() {
        if (scene.operationsQueue !== undefined
                && scene.operationsQueue !== null)
            return scene.operationsQueue
        var legacyTop = topFrame()
        return legacyTop && legacyTop.kind === "operationsQueue"
                ? legacyTop : null
    }

    function operationsQueueFrame() {
        return currentOperationsQueue() || retainedOperationsQueue || ({})
    }

    function hasOperationsQueueSurface() {
        var queue = currentOperationsQueue()
        return queue !== null && queue.kind === "operationsQueue"
    }

    function currentShellFrame() {
        if (shellPresentationOverrideSet)
            return shellPresentationOverride
        if (scene.shell !== undefined && scene.shell !== null)
            return scene.shell
        return firstFrame("shell") || firstFrame("panels") || null
    }

    function currentDocumentFrame() {
        if (documentPresentationOverrideSet)
            return isDocumentSurface(documentPresentationOverride)
                    ? documentPresentationOverride : null
        if (isDocumentSurface(scene.surface))
            return scene.surface
        var legacyTop = topFrame()
        return isDocumentSurface(legacyTop) ? legacyTop : null
    }

    function captureRetainedSurfaces() {
        var shell = currentShellFrame()
        if (shell !== null) {
            retainedShellFrame = shell
            retainedShellSurfaceCreated = true
        }

        var document = currentDocumentFrame()
        if (document !== null) {
            retainedDocumentFrame = document
            retainedDocumentSurfaceCreated = true
        }

        var queue = currentOperationsQueue()
        if (queue !== null) {
            retainedOperationsQueue = queue
            retainedOperationsQueueCreated = true
        }
    }

    function activeOperationsQueueView() {
        if (!hasOperationsQueueSurface() || hasBlockingOverlay())
            return null
        return operationsQueueLayer.item
    }

    function navigateOperationsQueue(command) {
        var view = activeOperationsQueueView()
        return view ? view.navigate(command) : false
    }

    function activateOperationsQueueSelection() {
        var view = activeOperationsQueueView()
        return view ? view.activateSelection() : false
    }

    function operationsQueueShortcutCanActivate() {
        var view = activeOperationsQueueView()
        return view !== null && !view.controlOwnsActivation()
    }

    function workspaceTabCanClose(tab) {
        if (!tab || tab.closable !== true)
            return false
        // A retained queue snapshot is deliberately not live while another
        // workspace is active.  The fresh workspaceTabs model is authoritative
        // there; only cross-check the queue model while its own tab is current.
        var queue = currentOperationsQueue()
        if (!queue)
            return true
        var queueTabId = cleanText(queue.tabId)
        if (queue.hasActive === true && queueTabId !== ""
                && cleanText(tab.id) === queueTabId)
            return false
        return true
    }

    function frames() {
        return scene.frames || []
    }

    function overlayFrames() {
        var out = []
        if (!isAppScene()) {
            var legacyFrames = frames()
            for (var k = 0; k < legacyFrames.length; ++k) {
                var kind = legacyFrames[k].kind
                if (kind === "menu" || kind === "dialog"
                        || kind === "window")
                    out.push(legacyFrames[k])
            }
        }
        var menus = qtShell.commandMenus !== undefined
                && qtShell.commandMenus !== null
                ? qtShell.commandMenus : (scene.menus || [])
        var dialogs = scene.dialogs || []
        for (var i = 0; i < menus.length; ++i)
            out.push(menus[i])
        for (var j = 0; j < dialogs.length; ++j)
            out.push(dialogs[j])
        return out
    }

    function menuOverlayForId(menuId) {
        var wanted = String(menuId || "")
        if (wanted === "" || typeof overlayRepeater === "undefined")
            return null
        for (var i = 0; i < overlayRepeater.count; ++i) {
            var loader = overlayRepeater.itemAt(i)
            if (!loader || !loader.item || loader.item.frame === undefined)
                continue
            if (String(loader.item.frame.id || "") === wanted)
                return loader.item
        }
        return null
    }

    function createDialogOverlay(frame) {
        return dialogOverlayComponent.createObject(mainSurface,
                                                   { "frame": frame })
    }

    function hasBlockingOverlay() {
        return overlayFrames().length > 0
    }

    function activePanelHasGalleryHost() {
        if (!qtGallery.available)
            return false
        var shell = shellFrame()
        var panels = shell && shell.panels ? shell.panels : []
        var activeSide = effectiveActivePanelSide()
        for (var i = 0; i < panels.length; ++i) {
            if (Number(panels[i].side) === activeSide
                    && panelSideVisible(panels[i].side)
                    && !panelSideCovered(panels[i].side)) {
                return galleryPanelHost(panels[i].side) !== null
            }
        }
        return false
    }

    function effectiveActivePanelSide() {
        if (pointerPanelActivationOverride === 0
                || pointerPanelActivationOverride === 1)
            return pointerPanelActivationOverride
        if (panelActivationOverride === 0 || panelActivationOverride === 1)
            return panelActivationOverride
        var shell = shellFrame()
        var shellSide = Number(shell && shell.activePanel)
        if (shellSide === 0 || shellSide === 1)
            return shellSide
        var panels = shell && shell.panels ? shell.panels : []
        for (var i = 0; i < panels.length; ++i) {
            if (panels[i].active === true)
                return Number(panels[i].side)
        }
        return -1
    }

    function panelIsEffectivelyActive(panel) {
        var activeSide = effectiveActivePanelSide()
        return activeSide >= 0
               ? Number(panel && panel.side) === activeSide
               : Boolean(panel && panel.active === true)
    }

    function beginPointerPanelActivation(side) {
        const normalized = Number(side)
        if (normalized !== 0 && normalized !== 1)
            return
        pointerPanelActivationOverride = normalized
        pointerPanelActivationTimer.restart()
    }

    function finishPointerPanelActivation() {
        pointerPanelActivationOverride = -1
        pointerPanelActivationTimer.stop()
    }

    Timer {
        id: pointerPanelActivationTimer
        interval: Math.max(1, root.pointerPanelActivationTimeoutMs)
        repeat: false
        onTriggered: root.finishPointerPanelActivation()
    }

    function galleryInputRoutingActive() {
        if (hasBlockingOverlay() || needsFallbackGrid()
                || hasDocumentSurface() || hasOperationsQueueSurface())
            return false
        // The full-area GalleryViewer is a modal keyboard/text surface.  Its
        // key handlers already swallow every press/release; disabling the
        // hidden grid's window-level IME filter closes the equivalent commit-
        // string path into Go as well.
        if (qtGallery.viewerVisible)
            return false
        return activePanelHasGalleryHost()
    }

    function galleryViewerOwnsKeyboard() {
        return qtGallery.viewerVisible
                && !hasBlockingOverlay()
                && !needsFallbackGrid()
                && !hasDocumentSurface()
                && !hasOperationsQueueSurface()
    }

    function restoreSurfaceFocus() {
        // Commander overlays and non-panel surfaces always own keys, even
        // when the integrated viewer remains loaded underneath them.
        if (hasBlockingOverlay() || needsFallbackGrid()
                || hasDocumentSurface() || hasOperationsQueueSurface()) {
            grid.forceActiveFocus()
            return
        }
        if (qtGallery.viewerVisible) {
            // The viewer Loader stays alive while a commander dialog is on
            // top. Hiding the dialog therefore does not retrigger onLoaded,
            // and the dialog's former focus falls back to the hidden grid.
            // Explicitly return it to the persistent viewer surface.
            var viewerLoader = galleryViewerLayer.item
            if (viewerLoader && viewerLoader.item)
                viewerLoader.item.forceActiveFocus()
            return
        }

        var shell = shellFrame()
        var panels = shell && shell.panels ? shell.panels : []
        var activeSide = effectiveActivePanelSide()
        for (var i = 0; i < panels.length; ++i) {
            if (Number(panels[i].side) !== activeSide)
                continue
            if (!panelSideVisible(panels[i].side)
                    || panelSideCovered(panels[i].side)) {
                // Alt panels deliberately keep keyboard ownership in Go. In
                // particular, a Quick View that became active through Tab
                // must not return focus to the covered Gallery/List host.
                grid.forceActiveFocus()
                return
            }
            var panelHost = galleryPanelHost(panels[i].side)
            if (panelHost) {
                panelHost.forceActiveFocus()
                return
            }
        }
        grid.forceActiveFocus()
    }

    function shellFrame() {
        return currentShellFrame() || retainedShellFrame || ({})
    }

    function quickViewForSide(side) {
        var shell = shellFrame()
        var quickViews = shell && shell.quickViews ? shell.quickViews : []
        for (var i = 0; i < quickViews.length; ++i) {
            if (Number(quickViews[i].side) === Number(side))
                return quickViews[i]
        }
        return null
    }

    function infoPanelForSide(side) {
        var shell = shellFrame()
        var infoPanels = shell && shell.infoPanels ? shell.infoPanels : []
        for (var i = 0; i < infoPanels.length; ++i) {
            if (Number(infoPanels[i].side) === Number(side))
                return infoPanels[i]
        }
        return null
    }

    function panelSideCovered(side) {
        return infoPanelForSide(side) !== null
                || quickViewForSide(side) !== null
    }

    function activeSurface() {
        var queue = currentOperationsQueue()
        if (queue)
            return queue
        var document = currentDocumentFrame()
        if (document)
            return document
        return topFrame()
    }

    function isDocumentSurface(frame) {
        return frame && (frame.kind === "viewer" || frame.kind === "editor"
                         || frame.kind === "terminal")
    }

    function hasDocumentSurface() {
        var shell = currentShellFrame()
        return hasStandaloneDocumentSurface()
                || (shell && shell.terminalActive === true)
    }

    function hasStandaloneDocumentSurface() {
        return currentDocumentFrame() !== null
    }

    function keyBarHeight() {
        return root.keyBarModel.visible !== false
                && Object.keys(root.keyBarModel).length > 0
                ? root.ch + root.commandLineVerticalMargin * 2
                  + root.separatorWidth * 2
                : 0
    }

    function commandLineHeight(shell) {
        var cmd = commandLineFrame()
        return cmd && cmd.visible !== false
                ? root.ch + root.commandLineVerticalMargin * 2
                  + root.separatorWidth * 2
                : 0
    }

    function firstFrame(kind) {
        var list = frames()
        for (var i = list.length - 1; i >= 0; --i) {
            if (list[i].kind === kind)
                return list[i]
        }
        return null
    }

    function topFrame() {
        var list = frames()
        return list.length > 0 ? list[list.length - 1] : null
    }

    function needsFallbackGrid() {
		if (scene.presentation === "text")
			return true
        if (hasOperationsQueueSurface())
            return false
        var shell = shellFrame()
        if (shell && shell.fallback === true)
            return true
        var top = isAppScene() ? activeSurface() : topFrame()
        if (!top)
            return !isAppScene()
        return top.fallback === true || top.kind === "fallback" || containsFallback(top)
    }

    function fallbackReasonForNode(node) {
        if (!node)
            return ""

        var isFallback = node.fallback === true
                || node.kind === "fallback"
                || node.kind === "fallbackWidget"
        var ownReason = cleanText(node.reason).trim()
        if (isFallback && ownReason !== "")
            return ownReason

        var children = node.children || []
        for (var i = 0; i < children.length; ++i) {
            var childReason = fallbackReasonForNode(children[i])
            if (childReason !== "")
                return childReason
        }

        return isFallback
                ? "the native QML model is unavailable for this surface"
                : ""
    }

    function semanticFallbackReason() {
        var shell = shellFrame()
        var shellReason = fallbackReasonForNode(shell)
        if (shellReason !== "")
            return shellReason

        var top = isAppScene() ? activeSurface() : topFrame()
        return fallbackReasonForNode(top)
    }

    function containsFallback(node) {
        if (!node)
            return false
        if (node.fallback === true || node.kind === "fallback" || node.kind === "fallbackWidget")
            return true
        var children = node.children || []
        for (var i = 0; i < children.length; ++i) {
            if (containsFallback(children[i]))
                return true
        }
        return false
    }

    function pxX(value) { return Math.round((value || 0) * cw) }
    function pxY(value) { return Math.round((value || 0) * ch) }
    function pxW(value) { return Math.max(1, Math.round((value || 0) * cw)) }
    function pxH(value) { return Math.max(1, Math.round((value || 0) * ch)) }

    function nativePanelSplitPosition() {
        var minimum = Math.min(panelMinimumWidth, Math.max(0, width / 2))
        return Math.round(Math.max(minimum,
                                  Math.min(width - minimum,
                                           width * panelSplitRatio)))
    }

    function widePanelSide() {
        var shell = shellFrame()
        if (shell.wide === true)
            return Number(shell.widePanel || 0)
        return -1
    }

    function panelSideVisible(side) {
        var wideSide = widePanelSide()
        if (wideSide >= 0)
            return Number(side) === wideSide
        var shell = shellFrame()
        return Number(side) === 0
                ? shell.showLeftPanel !== false
                : shell.showRightPanel !== false
    }

    function nativePanelX(side) {
        if (widePanelSide() >= 0)
            return 0
        return Number(side) === 0 ? 0 : nativePanelSplitPosition()
    }

    function nativePanelWidth(side) {
        if (widePanelSide() >= 0)
            return width
        var split = nativePanelSplitPosition()
        return Number(side) === 0 ? split : width - split
    }

    function cleanText(value) {
        return value === undefined || value === null ? "" : String(value)
    }

    // A menu that must size itself to its content cannot bind each row's
    // width to the menu's own width (that is circular). Rows instead expose
    // their natural implicitWidth, and the menu's own width binds to the
    // widest one via this helper, which is a one-way dependency.
    function repeaterMaxImplicitWidth(repeater) {
        let maxWidth = 0
        for (let i = 0; i < repeater.count; ++i) {
            const item = repeater.itemAt(i)
            if (item)
                maxWidth = Math.max(maxWidth, item.implicitWidth)
        }
        return maxWidth
    }

    function resolvedIconSource(name, logicalSize) {
        // Reading revision makes every binding below refresh after an icon-set
        // or desktop-theme change; it is also encoded into system image URLs
        // so Qt Quick cannot serve an obsolete cached texture.
        const generation = qtIcons.revision
        return qtIcons.iconSource(name, logicalSize,
                                  root.iconDevicePixelRatio)
    }

    function lucideIconSource(name, size, tint) {
        const value = root.cleanText(name)
        if (value === "")
            return ""
        const s = Number(size) > 0 ? Number(size) : 16
        const color = tint === undefined || tint === null
                    ? root.chromeText : tint
        return qtIcons.rasterizedLucideSource(
                    value, s, root.iconDevicePixelRatio, color)
    }

    function semanticPanelSplitRatio() {
        var shell = shellFrame()
        var layout = shell && shell.panelLayout ? shell.panelLayout : ({})
        var columns = Number(layout.columns || 0)
        var splitColumn = Number(layout.splitColumn || 0)
        if (columns > 0 && splitColumn > 0 && splitColumn < columns)
            return splitColumn / columns
        return 0.5
    }

    function workspaceTabIconName(tab) {
        tab = tab || ({})
        const configured = cleanText(tab.iconName)
        if (configured !== "")
            return configured
        switch (cleanText(tab.surfaceKind)) {
        case "operationsQueue": return "list-checks"
        case "editor": return "file-pen-line"
        case "viewer": return "file-text"
        case "terminal": return "square-terminal"
        case "panels": return "panels-top-left"
        default: return "panels-top-left"
        }
    }

    function workspaceTabLabel(tab) {
        tab = tab || ({})
        const number = Number(tab.number || 0)
        const title = cleanText(tab.text).trim()
        if (number <= 0)
            return title
        return title === "" ? String(number) : title + " " + String(number)
    }

    function workspaceTabTextColor(active) {
        return active === true ? textColor : mutedText
    }

    function workspaceTabNumberColor() {
        return mutedText
    }

    function workspaceTabShortcut(tab, platformName) {
        tab = tab || ({})
        const number = Number(tab.number || 0)
        if (number < 1 || number > 9 || tab.shortcutAvailable === false)
            return ""
        return cleanText(platformName) === "osx"
                ? "\u2325" + String(number)
                : "Alt+" + String(number)
    }

    function workspaceTabToolTip(tab, platformName) {
        tab = tab || ({})
        var primary = cleanText(tab.tooltipPrimary).trim()
        const secondary = cleanText(tab.tooltipSecondary).trim()
        if (primary === "")
            primary = cleanText(tab.text).trim()
        if (secondary !== "")
            primary = primary === "" ? secondary
                                      : primary + "\n" + secondary
        const shortcut = workspaceTabShortcut(tab, platformName)
        if (primary === "")
            return shortcut
        return shortcut === "" ? primary : primary + "\t" + shortcut
    }

    function workspaceTabFontWeight() {
        return Font.Normal
    }

    function vtuiKeyModifiers(modifiers) {
        var result = 0
        if ((modifiers & Qt.ShiftModifier) !== 0)
            result |= 0x0010
        if ((modifiers & Qt.ControlModifier) !== 0
                || (Qt.platform.os === "osx"
                    && (modifiers & Qt.MetaModifier) !== 0))
            result |= 0x0008
        if ((modifiers & Qt.AltModifier) !== 0)
            result |= 0x0002
        return result
    }

    function keyBarFunctionIndex(item, fallbackIndex) {
        item = item || ({})
        const match = /^F([0-9]+)$/i.exec(cleanText(item.key).trim())
        if (match !== null) {
            const keyNumber = Number(match[1])
            if (keyNumber >= 1 && keyNumber <= 24)
                return keyNumber - 1
        }
        const semanticIndex = Number(item["index"])
        if (!isNaN(semanticIndex) && semanticIndex >= 0)
            return semanticIndex
        return Math.max(0, Number(fallbackIndex || 0))
    }

    function keyBarModifierShortcut(functionKey, modifier) {
        const key = cleanText(functionKey).trim()
        const normalized = cleanText(modifier).toLowerCase().trim()
        if (normalized === "shift")
            return "Shift+" + key
        if (normalized === "ctrl" || normalized === "control")
            return "Ctrl+" + key
        if (normalized === "alt")
            return "Alt+" + key
        return key
    }

    function keyBarModifierFlags(modifier) {
        const normalized = cleanText(modifier).toLowerCase().trim()
        var result = 0
        if (normalized.indexOf("shift") >= 0)
            result |= 0x0010
        if (normalized.indexOf("ctrl") >= 0
                || normalized.indexOf("control") >= 0)
            result |= 0x0008
        if (normalized.indexOf("alt") >= 0)
            result |= 0x0002
        return result
    }

    function preferredWorkspaceTabWidth(titleWidth, closeEnabled) {
        const chromeWidth = closeEnabled === true ? 64 : 46
        return snapPx(Math.min(workspaceTabMaxWidth,
                               Math.max(workspaceTabMinWidth,
                                        Number(titleWidth || 0) + chromeWidth)))
    }

    function richTextEscape(value) {
        return cleanText(value).replace(/&/g, "&amp;")
                               .replace(/</g, "&lt;")
                               .replace(/>/g, "&gt;")
                               .replace(/\"/g, "&quot;")
    }

    // vtui follows the usual commander convention: '&' marks the mnemonic,
    // while '&&' means a literal ampersand. Some semantic nodes retain the
    // marker and others already expose clean text plus a separate hotkey, so
    // support both representations here.
    function mnemonicText(value, hotkey) {
        var source = cleanText(value)
        var characters = []
        var mnemonicIndex = -1
        for (var i = 0; i < source.length; ++i) {
            if (source[i] !== "&") {
                characters.push(source[i])
                continue
            }
            if (i + 1 >= source.length) {
                characters.push("&")
                continue
            }
            if (source[i + 1] === "&") {
                characters.push("&")
                ++i
                continue
            }
            mnemonicIndex = characters.length
            characters.push(source[++i])
        }

        if (mnemonicIndex < 0 && cleanText(hotkey) !== "") {
            var wanted = cleanText(hotkey).toLocaleLowerCase()
            for (var j = 0; j < characters.length; ++j) {
                if (characters[j].toLocaleLowerCase() === wanted) {
                    mnemonicIndex = j
                    break
                }
            }
        }

        var result = ""
        for (var k = 0; k < characters.length; ++k) {
            var escaped = richTextEscape(characters[k])
            result += k === mnemonicIndex
                    ? "<font color=\"" + root.dialogAccent + "\">"
                      + escaped + "</font>"
                    : escaped
        }
        return result
    }

    function runsText(runs) {
        if (!runs)
            return ""
        var out = ""
        for (var i = 0; i < runs.length; ++i)
            out += cleanText(runs[i].text)
        return out
    }

    function rowText(row) {
        if (!row)
            return ""
        if (row.text !== undefined)
            return cleanText(row.text)
        return runsText(row.runs)
    }

    function galleryTheme() {
        return {
            "panelBackground": galleryPanelBackgroundColor,
            "viewerBackground": galleryViewerBackgroundColor,
            "cursor": galleryCursorColor,
            "cursorBackground": galleryCursorBackgroundColor,
            "cursorBorder": galleryCursorBorderColor,
            "cardCursorBorder": galleryCardCursorBorderColor,
            "text": galleryTextColor,
            "mutedText": galleryMutedTextColor,
            "fileText": galleryFileTextColor,
            "folderText": galleryFolderTextColor,
            "neutralFileTextColors": galleryNeutralFileTextColors,
            "quickSearchMatch": galleryQuickSearchMatchColor,
            "selection": gallerySelectionColor,
            "markedBackground": galleryMarkedBackgroundColor,
            "markedText": galleryMarkedTextColor,
            "directoryText": galleryDirectoryTextColor,
            "folderIcon": galleryFolderIconColor,
            "itemBackground": galleryItemBackgroundColor,
            "directoryBackground": galleryDirectoryBackgroundColor,
            "itemHover": galleryItemHoverColor,
            "labelBackground": galleryLabelBackgroundColor,
            "previewBackdrop": galleryPreviewBackdropColor,
            "separator": gallerySeparatorColor,
            "headerText": galleryHeaderTextColor,
            "controlHover": galleryControlHoverColor,
            "scrollBarHandle": galleryScrollBarHandleColor,
            "scrollBarHandleBackgroundHovered": galleryScrollBarBackgroundHoverColor,
            "scrollBarHandleHovered": galleryScrollBarHoverColor,
            "scrollBarHandlePressed": galleryScrollBarPressedColor,
            "scrollBarTrackHovered": galleryScrollBarTrackHoverColor
        }
    }

    function galleryMetrics() {
        return {
            "detailsRowInset": root.snapPx(panelRowInnerSpacing),
            "detailsRowSpacing": root.snapPx(8),
            "detailsIconSlotSize": root.snapPx(16),
            "detailsIconSize": root.snapPx(16),
            "detailsNameFontPixelSize": 13,
            "detailsSecondaryFontPixelSize": 12,
            "detailsExtensionMinimumWidth": root.snapPx(40),
            "detailsExtensionMaximumWidth": root.snapPx(80),
            "detailsSizeColumnWidth": root.snapPx(96),
            "detailsHeaderHeight": root.snapPx(Math.max(22, ch)
                                   + verticalContentSpacing),
            "detailsHeaderCellInset": root.snapPx(panelRowInnerSpacing),
            "detailsHeaderFontPixelSize": 12,
            "detailsSeparatorVerticalMargin":
                root.snapPx(columnSeparatorVerticalMargin),
            "detailsSeparatorWidth": separatorWidth,
            "detailsScrollBarWidth": root.snapPx(16)
        }
    }

    function action(map, preserveFocus) {
        qtShell.sendUiAction(map)
        if (preserveFocus !== true)
            grid.forceActiveFocus()
    }

    onSceneChanged: {
        shellPresentationOverrideSet = false
        shellPresentationOverride = null
        documentPresentationOverrideSet = false
        documentPresentationOverride = null
        documentSurfaceStateOverride = null
        workspaceTabsOverride = null
        menuBarOverride = null
        keyBarOverride = null
        toastOverride = null
        leftPanelPresentationOverride = null
        rightPanelPresentationOverride = null
        leftPanelLayoutStateOverride = null
        rightPanelLayoutStateOverride = null
        panelActivationOverride = -1
        finishPointerPanelActivation()
        captureRetainedSurfaces()
        syncAutocompleteSelection()
        if (qtGallery.viewerVisible
                && (hasDocumentSurface() || needsFallbackGrid()))
            qtGallery.closeViewer()
        Qt.callLater(root.restoreSurfaceFocus)
    }

    Connections {
        target: qtShell
        // Several lightweight embedding/test shells intentionally implement
        // only the scene API. The production controller exposes compact
        // presentation/activation signals; missing optional signals must not
        // break loading.
        ignoreUnknownSignals: true
        function onPanelActivationChanged(activePanel, revision) {
            root.panelActivationOverride = Number(activePanel)
            root.finishPointerPanelActivation()
            // FilePanelView transfers focus synchronously when its compact
            // panelIsActive binding flips. Only alternate/covered surfaces
            // need the generic deferred focus router.
            if (!root.activePanelHasGalleryHost())
                Qt.callLater(root.restoreSurfaceFocus)
        }
        function onCompactPresentationChanged(patch) {
            if (!patch)
                return
            var structuralSurfaceChanged = false
            const replaceShell = patch.replaceShell === true
            if (patch.shellPresent === true
                    && (replaceShell
                        || root.currentShellFrame() === null)) {
                // A standalone document normally covers an already-retained
                // Commander shell. Keep that exact panel object graph current
                // underneath it, so returning with Escape is only a surface
                // reveal. The override is needed only when this QML scene was
                // born on a document and has never received a shell.
                root.shellPresentationOverrideSet = true
                root.shellPresentationOverride = patch.shell
                if (replaceShell) {
                    // Panel-scoped overrides belong to the previous shell.
                    // Clear both before exposing the replacement so one side
                    // can never paint a new session under stale old chrome.
                    root.leftPanelPresentationOverride = null
                    root.rightPanelPresentationOverride = null
                    root.leftPanelLayoutStateOverride = null
                    root.rightPanelLayoutStateOverride = null
                    root.panelActivationOverride = -1
                    root.finishPointerPanelActivation()
                }
                structuralSurfaceChanged = true
            }
            if (patch.surfacePresent !== undefined) {
                root.documentPresentationOverrideSet = true
                root.documentPresentationOverride = patch.surfacePresent === true
                        ? patch.surface : null
                root.documentSurfaceStateOverride = null
                structuralSurfaceChanged = true
            }
            if (patch.surfaceState !== undefined
                    && patch.surfaceState !== null)
                root.documentSurfaceStateOverride = patch.surfaceState
            const activePanel = Number(patch.activePanel)
            if (activePanel === 0 || activePanel === 1) {
                root.panelActivationOverride = activePanel
                root.finishPointerPanelActivation()
            }
            if (patch.panel !== undefined && patch.panel !== null) {
                const side = Number(patch.side)
                if (side === 0) {
                    root.leftPanelLayoutStateOverride = null
                    root.leftPanelPresentationOverride = patch.panel
                } else if (side === 1) {
                    root.rightPanelLayoutStateOverride = null
                    root.rightPanelPresentationOverride = patch.panel
                }
            }
            if (patch.panelLayoutState !== undefined
                    && patch.panelLayoutState !== null) {
                const layoutSide = Number(patch.side)
                if (layoutSide === 0) {
                    root.leftPanelLayoutStateOverride =
                            root.mergePanelLayoutState(
                                root.leftPanelLayoutStateOverride,
                                patch.panelLayoutState)
                } else if (layoutSide === 1) {
                    root.rightPanelLayoutStateOverride =
                            root.mergePanelLayoutState(
                                root.rightPanelLayoutStateOverride,
                                patch.panelLayoutState)
                }
            }
            if (patch.workspaceTabs !== undefined
                    && patch.workspaceTabs !== null)
                root.workspaceTabsOverride = patch.workspaceTabs
            if (patch.menuBar !== undefined && patch.menuBar !== null)
                root.menuBarOverride = patch.menuBar
            if (patch.keyBar !== undefined && patch.keyBar !== null)
                root.keyBarOverride = patch.keyBar
            if (patch.toast !== undefined && patch.toast !== null)
                root.toastOverride = patch.toast
            if (structuralSurfaceChanged) {
                root.captureRetainedSurfaces()
                if (qtGallery.viewerVisible
                        && (root.hasDocumentSurface()
                            || root.needsFallbackGrid()))
                    qtGallery.closeViewer()
                Qt.callLater(root.restoreSurfaceFocus)
            }
        }
        function onCommandMenusChanged() {
            root.syncAutocompleteSelection()
            // Structural menu changes alter the authoritative top surface.
            // Transfer keyboard ownership synchronously: a persistent Gallery
            // may otherwise consume the first bare arrow before the popup's
            // deferred Loader has completed. Selection-only menu state updates
            // deliberately use commandMenuStatesChanged and skip this path.
            root.restoreSurfaceFocus()
        }
    }

    onClosing: qtShell.sendQuit()

    Shortcut {
        sequence: "Shift+F12"
		context: Qt.ApplicationShortcut
		enabled: !qtGallery.viewerVisible
		onActivated: root.action({"target": "app", "action": "presentation.toggle"}, true)
	}

    Shortcut {
        sequence: "Up"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeAutocompleteFrame() !== null
        onActivated: root.navigateAutocomplete(-1)
    }

    Shortcut {
        sequence: "Down"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeAutocompleteFrame() !== null
        onActivated: root.navigateAutocomplete(1)
    }

    Shortcut {
        sequence: "Return"
        context: Qt.ApplicationShortcut
        enabled: root.activeAutocompleteFrame() !== null
        onActivated: root.submitAutocomplete()
    }

    Shortcut {
        sequence: "Enter"
        context: Qt.ApplicationShortcut
        enabled: root.activeAutocompleteFrame() !== null
        onActivated: root.submitAutocomplete()
    }

    Shortcut {
        sequence: "Tab"
        context: Qt.ApplicationShortcut
        enabled: root.activeAutocompleteFrame() !== null
        onActivated: root.completeAutocomplete()
    }

    // The queue remains a vtui workspace, so modified keys, Escape, F10 and
    // Ctrl+W continue through the hidden grid to the authoritative QueueFrame.
    // Plain table navigation is mirrored locally for native, autorepeating
    // selection and then synchronized by stable task identity.
    Shortcut {
        sequence: "Up"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeOperationsQueueView() !== null
        onActivated: root.navigateOperationsQueue("up")
    }
    Shortcut {
        sequence: "Down"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeOperationsQueueView() !== null
        onActivated: root.navigateOperationsQueue("down")
    }
    Shortcut {
        sequence: "PgUp"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeOperationsQueueView() !== null
        onActivated: root.navigateOperationsQueue("pageUp")
    }
    Shortcut {
        sequence: "PgDown"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeOperationsQueueView() !== null
        onActivated: root.navigateOperationsQueue("pageDown")
    }
    Shortcut {
        sequence: "Home"
        context: Qt.ApplicationShortcut
        enabled: root.activeOperationsQueueView() !== null
        onActivated: root.navigateOperationsQueue("home")
    }
    Shortcut {
        sequence: "End"
        context: Qt.ApplicationShortcut
        enabled: root.activeOperationsQueueView() !== null
        onActivated: root.navigateOperationsQueue("end")
    }
    Shortcut {
        sequence: "Return"
        context: Qt.ApplicationShortcut
        enabled: root.operationsQueueShortcutCanActivate()
        onActivated: root.activateOperationsQueueSelection()
    }
    Shortcut {
        sequence: "Enter"
        context: Qt.ApplicationShortcut
        enabled: root.operationsQueueShortcutCanActivate()
        onActivated: root.activateOperationsQueueSelection()
    }

    VtuiGridItem {
        id: grid
        objectName: "vtuiGrid"
        anchors.fill: parent
        // The pure text renderer has no semantic menu/title row of its own.
        // Reserve the native macOS title area so the first terminal row never
        // sits underneath QWK's traffic-light controls.
        anchors.topMargin: root.needsFallbackGrid()
                           && root.useMacNativeTitleBar
                           ? root.menuBarHeight : 0
        controller: qtShell
        fontFamily: f4GuiFontFamily
        fontPixelSize: root.guiMonospaceFontPixelSize
        focus: true
        // The grid remains callable as the semantic layer's explicit key
        // sink, but it must not be a pointer target behind QML surfaces.
        // Otherwise an unhandled right-click/wheel event mutates the hidden
        // text panel underneath Gallery (or any other semantic control).
        pointerInputEnabled: root.needsFallbackGrid()
        // Gallery panels own focus while embedded, so their committed IME text
        // still uses the terminal protocol. galleryInputRoutingActive() keeps
        // that window-level path disabled for the modal full-area viewer.
        inputMethodForwardingEnabled: root.galleryInputRoutingActive()
        // QML focus is not a security boundary: a Loader rebuild or native
        // activation race can briefly focus this hidden grid. Block every
        // terminal key/text/paste path while GalleryViewer is the top surface;
        // semantic cursor/selection still travel through F4GalleryBridge.
        terminalInputEnabled: !root.galleryViewerOwnsKeyboard()
        // Continue ingesting terminal frames while semantic surfaces own the
        // window, but do not repaint/upload the fully covered compatibility
        // texture on every backend update.
        renderingEnabled: root.needsFallbackGrid()
        z: 0
        opacity: root.needsFallbackGrid() ? 1.0 : 0.0
        visible: true
        Component.onCompleted: forceActiveFocus()
    }

    Connections {
        target: grid
        ignoreUnknownSignals: true
        function onKeyboardActivity() {
            ++root.keyboardActivityRevision
        }
    }

    readonly property int themeSchemaVersion: 1
    readonly property var themeColorDefinitions: [
        // Window
        { id: "windowBackgroundColor", name: "Window Background", group: "Window", defaultColor: "#1f242c" },
        { id: "titleBarBg", name: "Chrome / Title Bar Background", group: "Window", defaultColor: "#202833" },
        { id: "fBarBg", name: "Chrome / F-Bar Background", group: "Window", defaultColor: "#202833" },
        { id: "chromeText", name: "Chrome Text", group: "Window", defaultColor: "#d7e0ea" },

        // Panels
        { id: "panelPathBg", name: "Panel Header Background", group: "Panels", defaultColor: "#26314152" },

        // Terminal
        { id: "commandLineBg", name: "Command Line Background", group: "Terminal", defaultColor: "#19000000" },

        // Text
        { id: "textColor", name: "Primary Text Color", group: "Text", defaultColor: "#e8edf2" },
        { id: "mutedText", name: "Secondary / Muted Text Color", group: "Text", defaultColor: "#9aa7b5" },

        // Selection
        { id: "selectedBg", name: "General Selection Background", group: "Selection", defaultColor: "#285d8f" },
        { id: "panelSelectionBg", name: "Interactive Selection Background", group: "Selection", defaultColor: "#18456e" },
        { id: "panelSelectionBorder", name: "Interactive Selection Border", group: "Selection", defaultColor: "#1d5888" },

        // Icons & Accents
        { id: "dialogAccent", name: "Highlight / Accent Color", group: "Icons & Accents", defaultColor: "#4e9bd4" },
        { id: "activeBorder", name: "Attention / Active Label", group: "Icons & Accents", defaultColor: "#f0c95a" },

        // Dialogs
        { id: "dialogBg", name: "Dialog Background", group: "Dialogs", defaultColor: "#171e27" },
        { id: "dialogHeaderBg", name: "Dialog Header Background", group: "Dialogs", defaultColor: "#1b242f" },

        // Controls
        { id: "controlBg", name: "Control Background", group: "Controls", defaultColor: "#222c38" },
        { id: "controlHoverBg", name: "Control Hover Background", group: "Controls", defaultColor: "#2a3745" },
        { id: "controlPressedBg", name: "Control Pressed Background", group: "Controls", defaultColor: "#10161e" },
        { id: "controlBorder", name: "Control Border", group: "Controls", defaultColor: "#3a495b" },

        // Separators
        { id: "separatorColor", name: "Separator Color", group: "Separators", defaultColor: "#30363d" },
        { id: "separatorHoverColor", name: "Separator Hover Color", group: "Separators", defaultColor: "#464d55" },
        { id: "separatorActiveColor", name: "Separator Active Color", group: "Separators", defaultColor: "#59616a" },

        // ZoinGallery panel and viewer
        { id: "galleryPanelBackgroundColor", name: "Gallery Panel Background", group: "Panel Colors", defaultColor: "transparent" },
        { id: "galleryViewerBackgroundColor", name: "Gallery Viewer Background", group: "Panel Colors", defaultColor: "transparent" },
        { id: "galleryTextColor", name: "File Text", group: "Panel Colors", defaultColor: "#e8edf2" },
        { id: "galleryMutedTextColor", name: "Secondary File Text", group: "Panel Colors", defaultColor: "#9aa7b5" },
        { id: "galleryFileTextColor", name: "Neutral File Text", group: "Panel Colors", defaultColor: "#c4cbd3" },
        { id: "galleryFolderTextColor", name: "Neutral Folder Text", group: "Panel Colors", defaultColor: "#ffffff" },
        { id: "galleryQuickSearchMatchColor", name: "Quick Search Match", group: "Panel Colors", defaultColor: "#e8edf2" },
        { id: "galleryDirectoryTextColor", name: "Directory Text", group: "Panel Colors", defaultColor: "#98d8ff" },
        { id: "galleryFolderIconColor", name: "Folder Icon", group: "Panel Colors", defaultColor: "#5ab2f1" },
        { id: "galleryCursorColor", name: "Card Cursor Fill", group: "Panel Colors", defaultColor: "#1d5888" },
        { id: "galleryCursorBackgroundColor", name: "Details Cursor Fill", group: "Panel Colors", defaultColor: "#18456e" },
        { id: "galleryCursorBorderColor", name: "Details Cursor Border", group: "Panel Colors", defaultColor: "#1d5888" },
        { id: "galleryCardCursorBorderColor", name: "Card Cursor Border", group: "Panel Colors", defaultColor: "#2777b8" },
        { id: "gallerySelectionColor", name: "Selection Border", group: "Panel Colors", defaultColor: "#ffd43b" },
        { id: "galleryMarkedBackgroundColor", name: "Marked Row Background", group: "Panel Colors", defaultColor: "#4f5037" },
        { id: "galleryMarkedTextColor", name: "Marked Item Text", group: "Panel Colors", defaultColor: "#ffd43b" },
        { id: "galleryItemBackgroundColor", name: "Image Card Background", group: "Panel Colors", defaultColor: "transparent" },
        { id: "galleryDirectoryBackgroundColor", name: "Directory Card Background", group: "Panel Colors", defaultColor: "transparent" },
        { id: "galleryItemHoverColor", name: "Item Hover", group: "Panel Colors", defaultColor: "#0cffffff" },
        { id: "galleryLabelBackgroundColor", name: "Thumbnail Label Background", group: "Panel Colors", defaultColor: "#aa101216" },
        { id: "galleryPreviewBackdropColor", name: "Preview Placeholder", group: "Panel Colors", defaultColor: "#4d000000" },
        { id: "gallerySeparatorColor", name: "Gallery Separator", group: "Panel Colors", defaultColor: "#30363d" },
        { id: "galleryHeaderTextColor", name: "Gallery Header Text", group: "Panel Colors", defaultColor: "#d7e0ea" },
        { id: "galleryControlHoverColor", name: "Gallery Control Hover", group: "Panel Colors", defaultColor: "#2a3745" },
        { id: "galleryScrollBarHandleColor", name: "Scrollbar Handle", group: "Panel Colors", defaultColor: "#4a4a4a" },
        { id: "galleryScrollBarBackgroundHoverColor", name: "Scrollbar Hover Background", group: "Panel Colors", defaultColor: "#676767" },
        { id: "galleryScrollBarHoverColor", name: "Scrollbar Handle Hover", group: "Panel Colors", defaultColor: "#878787" },
        { id: "galleryScrollBarPressedColor", name: "Scrollbar Handle Pressed", group: "Panel Colors", defaultColor: "#505050" },
        { id: "galleryScrollBarTrackHoverColor", name: "Scrollbar Track Hover", group: "Panel Colors", defaultColor: "#0fffffff" },
        { id: "galleryPathBackgroundColor", name: "Path Background", group: "Panel Colors", defaultColor: "transparent" },
        { id: "galleryPathTextColor", name: "Path Text", group: "Panel Colors", defaultColor: "#e8edf2" },
        { id: "galleryPathHoverColor", name: "Path Hover", group: "Panel Colors", defaultColor: "#222c38" },
        { id: "galleryPathItemHoverColor", name: "Breadcrumb Hover", group: "Panel Colors", defaultColor: "#2a3745" },
        { id: "galleryPathItemPressedColor", name: "Breadcrumb Pressed", group: "Panel Colors", defaultColor: "#10161e" }
    ]

    function setFontRenderType(value) {
        if (typeof qtTextRendering === "undefined" || !qtTextRendering)
            return false
        qtTextRendering.renderType = Number(value)
        return qtTextRendering.renderType === Number(value)
    }

    function loadThemeFromPersistence() {
        if (typeof qtTheme === "undefined" || !qtTheme)
            return false

        try {
            const saved = qtTheme.loadTheme()
            const savedSchemaVersion = Number(saved.themeSchemaVersion || 0)
            let applied = false
            for (let i = 0; i < themeColorDefinitions.length; ++i) {
                const def = themeColorDefinitions[i]
                if (Object.prototype.hasOwnProperty.call(saved, def.id)
                        && saved[def.id]) {
                    const c = Qt.color(saved[def.id])
                    if (c) {
                        // Before hover was visible in every presentation its
                        // shipped value was transparent, and Save persisted
                        // that inert default. Upgrade only those legacy files;
                        // schema-aware themes may still deliberately disable
                        // hover by choosing a transparent color.
                        const legacyTransparentItemHover =
                            savedSchemaVersion < 1
                            && def.id === "galleryItemHoverColor"
                            && Number(c.a) === 0
                        root[def.id] = legacyTransparentItemHover
                                ? Qt.color(def.defaultColor) : c
                        applied = true
                    }
                }
            }
            // Preserve the old shared chrome color when loading a theme
            // saved before the title-bar/F-bar split.
            if (Object.prototype.hasOwnProperty.call(saved, "chromeBg")
                    && saved.chromeBg) {
                const legacyChromeBg = Qt.color(saved.chromeBg)
                const hasTitleBarBg =
                    Object.prototype.hasOwnProperty.call(saved,
                                                         "titleBarBg")
                    && saved.titleBarBg
                const hasFBarBg =
                    Object.prototype.hasOwnProperty.call(saved, "fBarBg")
                    && saved.fBarBg
                if (legacyChromeBg) {
                    if (!hasTitleBarBg) {
                        root.titleBarBg = legacyChromeBg
                        applied = true
                    }
                    if (!hasFBarBg) {
                        root.fBarBg = legacyChromeBg
                        applied = true
                    }
                }
            }
            if (saved.fontRenderType !== undefined
                    && saved.fontRenderType !== "") {
                if (typeof qtTextRendering !== "undefined"
                        && qtTextRendering) {
                    applied = qtTextRendering.setRenderTypeByName(
                                  String(saved.fontRenderType)) || applied
                }
            }
            if (saved.mouseWheelMode !== undefined
                    && saved.mouseWheelMode !== "") {
                applied = root.setMouseWheelMode(
                              String(saved.mouseWheelMode)) || applied
            }
            if (saved.neutralFileTextColors !== undefined) {
                const value = saved.neutralFileTextColors
                root.galleryNeutralFileTextColors = value === true
                        || String(value).toLowerCase() === "true"
                applied = true
            }
            return applied
        } catch (error) {
            console.warn("Unable to load the saved theme:", error)
            return false
        }
    }

    function saveThemeToPersistence() {
        if (typeof qtTheme !== "undefined" && qtTheme) {
            const map = {}
            for (let i = 0; i < themeColorDefinitions.length; ++i) {
                const def = themeColorDefinitions[i]
                map[def.id] = formatColorHex(root[def.id])
            }
            map.fontRenderType = root.fontRenderTypeName
            map.mouseWheelMode = root.mouseWheelMode
            map.neutralFileTextColors = root.galleryNeutralFileTextColors
            map.themeSchemaVersion = root.themeSchemaVersion
            return qtTheme.saveTheme(map)
        }
        return false
    }

    function resetThemeToDefaults() {
        for (let i = 0; i < themeColorDefinitions.length; ++i) {
            const def = themeColorDefinitions[i]
            root[def.id] = Qt.color(def.defaultColor)
        }
        if (typeof qtTextRendering !== "undefined" && qtTextRendering)
            qtTextRendering.setRenderTypeByName("NativeRendering")
        root.mouseWheelMode = "gui"
        root.galleryNeutralFileTextColors = true
    }

    function formatColorHex(clr) {
        if (!clr)
            return "#000000"
        const r = Math.round(clr.r * 255)
        const g = Math.round(clr.g * 255)
        const b = Math.round(clr.b * 255)
        const a = Math.round((clr.a !== undefined ? clr.a : 1) * 255)
        const hex2 = (n) => (n < 16 ? "0" : "") + n.toString(16)
        if (a < 255) {
            return "#" + hex2(a) + hex2(r) + hex2(g) + hex2(b)
        }
        return "#" + hex2(r) + hex2(g) + hex2(b)
    }

    component ConfigDialogButton: Rectangle {
        id: cBtn
        property string text: ""
        property string iconSource: ""
        property string toolTipText: text
        property bool highlighted: false
        signal clicked()

        implicitHeight: root.snapPx(30)
        implicitWidth: root.snapPx(cBtnRow.implicitWidth + 20)
        Layout.preferredHeight: implicitHeight
        Layout.preferredWidth: implicitWidth
        Layout.minimumHeight: implicitHeight
        Layout.maximumHeight: implicitHeight
        radius: root.snapPx(4)

        color: cBtnMouse.pressed
               ? (highlighted ? Qt.darker(root.dialogAccent, 1.2) : "#38ffffff")
               : cBtnMouse.containsMouse
                 ? (highlighted ? Qt.lighter(root.dialogAccent, 1.1) : "#24ffffff")
                 : (highlighted ? root.dialogAccent : "#14ffffff")

        border.width: root.separatorWidth
        border.color: highlighted
                      ? root.dialogAccent
                      : (cBtnMouse.containsMouse ? root.controlHoverBg : root.controlBorder)
        Accessible.role: Accessible.Button
        Accessible.name: toolTipText

        ZG.ToolTip {
            visible: cBtnMouse.containsMouse && cBtn.toolTipText !== ""
            delay: 500
            timeout: 5000
            text: cBtn.toolTipText
        }

        RowLayout {
            id: cBtnRow
            x: root.snapPx((parent.width - width) / 2)
            y: root.snapPx((parent.height - height) / 2)
            spacing: root.snapPx(6)

            IconLabel {
                icon.source: cBtn.iconSource
                icon.width: root.snapPx(14)
                icon.height: root.snapPx(14)
                icon.color: cBtn.highlighted ? "#ffffff" : root.textColor
                visible: cBtn.iconSource !== ""
            }

            Text {
                text: cBtn.text
                color: cBtn.highlighted ? "#ffffff" : root.textColor
                font.family: root.guiMonospaceFontFamily
                font.pixelSize: 11
                font.weight: cBtn.highlighted ? Font.Bold : Font.Normal
                verticalAlignment: Text.AlignVCenter
            }
        }

        MouseArea {
            id: cBtnMouse
            anchors.fill: parent
            hoverEnabled: true
            cursorShape: Qt.PointingHandCursor
            onClicked: cBtn.clicked()
        }
    }

    component ThemeRenderTypeComboBox: T.ComboBox {
        id: themeRenderCombo
        property var options: []
        property int selectedRenderType: Text.NativeRendering
        // The component is also used by non-rendering theme preferences. Keep
        // selectedRenderType/renderTypeActivated for the existing font
        // control, while allowing arbitrary string-valued options here.
        property var selectedValue: undefined
        signal renderTypeActivated(int value)
        signal optionActivated(var value)
        readonly property string popupObjectNamePrefix:
            themeRenderCombo.objectName === "themeFontRenderTypeCombo"
            ? "themeFontRenderType" : themeRenderCombo.objectName

        focusPolicy: Qt.NoFocus
        hoverEnabled: true
        padding: 0
        implicitHeight: root.snapPx(30)
        Layout.preferredHeight: root.snapPx(30)
        Layout.minimumHeight: root.snapPx(30)
        Layout.maximumHeight: root.snapPx(30)
        model: options
        textRole: "name"
        valueRole: "value"

        function syncCurrentIndex() {
            const values = themeRenderCombo.options || []
            const selected = themeRenderCombo.selectedValue !== undefined
                    ? themeRenderCombo.selectedValue
                    : themeRenderCombo.selectedRenderType
            let nextIndex = 0
            for (let i = 0; i < values.length; ++i) {
                const optionValue = values[i].value
                const equal = typeof selected === "number"
                        || typeof optionValue === "number"
                        ? Number(optionValue) === Number(selected)
                        : String(optionValue) === String(selected)
                if (equal) {
                    nextIndex = i
                    break
                }
            }
            if (themeRenderCombo.currentIndex !== nextIndex)
                themeRenderCombo.currentIndex = nextIndex
        }

        Component.onCompleted: syncCurrentIndex()
        onOptionsChanged: syncCurrentIndex()
        onSelectedRenderTypeChanged: syncCurrentIndex()
        onSelectedValueChanged: syncCurrentIndex()
        onActivated: function(index) {
            const values = themeRenderCombo.options || []
            if (index < 0 || index >= values.length)
                return
            const value = values[index].value
            themeRenderCombo.optionActivated(value)
            if (typeof value === "number")
                themeRenderCombo.renderTypeActivated(Number(value))
        }

        contentItem: Text {
            leftPadding: root.snapPx(10)
            rightPadding: root.snapPx(30)
            text: themeRenderCombo.currentText
            color: root.textColor
            font.family: root.guiMonospaceFontFamily
            font.pixelSize: 11
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }

        indicator: IconLabel {
            objectName: themeRenderCombo.objectName + "Indicator"
            readonly property url rasterizedIconSource:
                root.lucideIconSource(
                    "chevron-down", 14,
                    themeRenderCombo.enabled ? root.textColor : root.mutedText)
            x: root.snapPx(themeRenderCombo.width - width - 10)
            y: root.snapPx((themeRenderCombo.height - height) / 2)
            width: root.snapPx(14)
            height: root.snapPx(14)
            icon.source: rasterizedIconSource
            icon.width: root.snapPx(14)
            icon.height: root.snapPx(14)
            icon.color: themeRenderCombo.hovered ? root.textColor : root.mutedText
        }

        background: Rectangle {
            objectName: themeRenderCombo.objectName + "Background"
            readonly property color testBorderColor: border.color
            radius: root.snapPx(4)
            color: themeRenderCombo.down ? root.controlPressedBg
                   : themeRenderCombo.hovered ? root.controlHoverBg
                   : root.controlBg
            border.width: root.separatorWidth
            border.color: themeRenderCombo.activeFocus
                          ? root.dialogAccent : root.controlBorder
        }

        popup: T.Popup {
            y: root.snapPx(themeRenderCombo.height + 4)
            width: root.snapPx(themeRenderCombo.width)
            implicitHeight: root.snapPx(Math.min(contentItem.implicitHeight + 8, 240))
            padding: root.snapPx(4)

            background: Rectangle {
                objectName: themeRenderCombo.popupObjectNamePrefix
                           + "PopupBackground"
                radius: root.snapPx(5)
                color: root.dialogHeaderBg
                border.width: root.separatorWidth
                border.color: root.controlBorder
            }

            contentItem: ListView {
                objectName: themeRenderCombo.popupObjectNamePrefix
                           + "PopupList"
                clip: true
                implicitHeight: root.snapPx(contentHeight)
                model: themeRenderCombo.popup.visible
                       ? themeRenderCombo.delegateModel : null
                currentIndex: themeRenderCombo.highlightedIndex
                boundsBehavior: Flickable.StopAtBounds
            }
        }

        delegate: T.ItemDelegate {
            id: themeRenderComboItem
            objectName: themeRenderCombo.objectName + "Item"
            required property int index
            required property var model
            width: root.snapPx(themeRenderCombo.width - 8)
            height: root.snapPx(30)
            highlighted: themeRenderCombo.highlightedIndex === index

            contentItem: Text {
                leftPadding: root.snapPx(8)
                text: themeRenderComboItem.model.name
                color: root.textColor
                font.family: root.guiMonospaceFontFamily
                font.pixelSize: 11
                verticalAlignment: Text.AlignVCenter
                elide: Text.ElideRight
            }

            background: Rectangle {
                radius: root.snapPx(3)
                color: themeRenderComboItem.highlighted
                       ? root.selectedBg
                       : themeRenderComboItem.hovered
                         ? root.controlHoverBg : "transparent"
            }
        }

    }

    Window {
        id: themeColorConfigurator
        objectName: "themeColorConfigurator"
        title: "Theme Color Configurator"
        width: root.snapPx(720)
        height: root.snapPx(560)
        minimumWidth: root.snapPx(580)
        minimumHeight: root.snapPx(440)
        visible: false
        color: root.dialogBg
        flags: Qt.Window | Qt.WindowTitleHint | Qt.WindowSystemMenuHint | Qt.WindowMinMaxButtonsHint | Qt.WindowCloseButtonHint

        property int selectedIndex: 0
        readonly property var currentItem: (selectedIndex >= 0 && selectedIndex < root.themeColorDefinitions.length)
                                           ? root.themeColorDefinitions[selectedIndex] : null

        // The editor uses OKLCH rather than HSL/HSV. Chroma is kept in the
        // CSS/OKLCH-compatible 0..0.4 range; values outside the sRGB gamut
        // are reduced while preserving lightness and hue.
        readonly property real maxOklchChroma: 0.4
        readonly property real wheelOklchChroma: 0.32
        property real selectedHue: 0
        property real selectedChroma: 0
        property real selectedLightness: 0.5
        property real selectedAlpha: 1.0
        property string filterQuery: ""
        property string statusToast: ""
        property bool pressFlashActive: false
        property string pressFlashProperty: ""
        property bool preserveReleaseColorOnStop: false
        readonly property color activeFlashColor: "#00ff00"

        // A hover preview deliberately stops at green. The list row decides
        // when it is allowed to leave that state (pointer exit or mouse
        // release); there is no timer-driven return anymore.
        PropertyAnimation {
            id: flashElementAnimation
            target: root
            duration: 150
            easing.type: Easing.OutQuad
            property string targetProperty: ""
            property color originalColor: "transparent"
        }

        PropertyAnimation {
            id: flashPressToActive
            target: root
            duration: 150
            to: themeColorConfigurator.activeFlashColor
            easing.type: Easing.OutQuad
        }

        PropertyAnimation {
            id: flashReleaseBack
            target: root
            duration: 150
            easing.type: Easing.InQuad
            onFinished: {
                themeColorConfigurator.finishReleaseFlash()
            }
            onStopped: {
                if (themeColorConfigurator.preserveReleaseColorOnStop) {
                    themeColorConfigurator.preserveReleaseColorOnStop = false
                } else {
                    themeColorConfigurator.finishReleaseFlash()
                }
            }
        }

        function finishReleaseFlash() {
            if (flashElementAnimation.targetProperty !== "") {
                root[flashElementAnimation.targetProperty] =
                        flashElementAnimation.originalColor
                flashElementAnimation.targetProperty = ""
            }
            pressFlashActive = false
            pressFlashProperty = ""
        }

        function restoreFlashTarget() {
            if (flashElementAnimation.targetProperty !== "") {
                root[flashElementAnimation.targetProperty] =
                        flashElementAnimation.originalColor
                flashElementAnimation.targetProperty = ""
            }
        }

        function colorBeforeFlash(propId) {
            if (flashElementAnimation.targetProperty === propId)
                return flashElementAnimation.originalColor
            return root[propId]
        }

        function flash(propId) {
            if (!propId || root[propId] === undefined)
                return
            // Once a row is active, hover is quiet. Only a real press may
            // temporarily mark the active row green.
            if (currentItem && currentItem.id === propId
                    && !pressFlashActive)
                return
            if (pressFlashActive)
                return
            // Re-entering the same row must not restart a preview that is
            // already green (whether its animation has finished or not).
            if (flashElementAnimation.targetProperty === propId)
                return

            stopAllFlashing()
            flashElementAnimation.targetProperty = propId
            flashElementAnimation.originalColor = root[propId]
            flashElementAnimation.property = propId
            flashElementAnimation.from = root[propId]
            flashElementAnimation.to = activeFlashColor
            flashElementAnimation.restart()
        }

        function endHoverFlash(propId) {
            if (!propId || pressFlashActive
                    || flashElementAnimation.targetProperty !== propId
                    || flashReleaseBack.running)
                return
            if (flashElementAnimation.running)
                flashElementAnimation.stop()

            flashReleaseBack.property = propId
            flashReleaseBack.from = root[propId]
            flashReleaseBack.to = flashElementAnimation.originalColor
            flashReleaseBack.restart()
        }

        function startPressFlash(propId) {
            if (!propId || root[propId] === undefined)
                return

            if (pressFlashActive && pressFlashProperty === propId)
                return

            // If the row was already green from hover, preserve the current
            // green frame while switching ownership from hover to press. This
            // avoids a visible green -> real -> green flicker on mouse-down.
            const continuingPreview =
                    flashElementAnimation.targetProperty === propId
                    && !pressFlashActive
            const originalColor = continuingPreview
                    ? flashElementAnimation.originalColor : root[propId]
            if (continuingPreview) {
                if (flashReleaseBack.running) {
                    preserveReleaseColorOnStop = true
                    flashReleaseBack.stop()
                }
                if (flashElementAnimation.running)
                    flashElementAnimation.stop()
            } else {
                stopAllFlashing()
            }

            pressFlashActive = true
            pressFlashProperty = propId
            flashElementAnimation.targetProperty = propId
            flashElementAnimation.originalColor = originalColor

            flashPressToActive.property = propId
            flashPressToActive.from = root[propId]
            flashPressToActive.to = activeFlashColor
            flashPressToActive.restart()
        }

        function endPressFlash(propId) {
            if (!pressFlashActive || pressFlashProperty !== propId
                    || !flashElementAnimation.targetProperty
                    || flashReleaseBack.running)
                return
            if (flashPressToActive.running)
                flashPressToActive.stop()
            if (flashReleaseBack.running)
                flashReleaseBack.stop()

            const targetProp = flashElementAnimation.targetProperty
            const origClr = flashElementAnimation.originalColor

            flashReleaseBack.property = targetProp
            flashReleaseBack.from = root[targetProp]
            flashReleaseBack.to = origClr
            flashReleaseBack.restart()
        }

        function stopAllFlashing() {
            preserveReleaseColorOnStop = false
            if (flashReleaseBack.running)
                flashReleaseBack.stop()
            if (flashPressToActive.running)
                flashPressToActive.stop()
            if (flashElementAnimation.running)
                flashElementAnimation.stop()
            restoreFlashTarget()
            pressFlashActive = false
            pressFlashProperty = ""
        }

        function parseHex(text) {
            let s = String(text || "").trim()
            if (!s.startsWith("#"))
                s = "#" + s
            if (/^#[0-9A-Fa-f]{8}$/.test(s) || /^#[0-9A-Fa-f]{6}$/.test(s)
                    || /^#[0-9A-Fa-f]{4}$/.test(s) || /^#[0-9A-Fa-f]{3}$/.test(s)) {
                return Qt.color(s)
            }
            return null
        }

        function clampUnit(value) {
            const numeric = Number(value)
            if (!isFinite(numeric))
                return 0
            return Math.max(0, Math.min(1, numeric))
        }

        function clampOklchChroma(value) {
            const numeric = Number(value)
            if (!isFinite(numeric))
                return 0
            return Math.max(0, Math.min(maxOklchChroma, numeric))
        }

        function signedCubeRoot(value) {
            return value < 0
                    ? -Math.pow(-value, 1 / 3)
                    : Math.pow(value, 1 / 3)
        }

        function srgbToLinear(value) {
            const channel = clampUnit(value)
            return channel <= 0.04045
                    ? channel / 12.92
                    : Math.pow((channel + 0.055) / 1.055, 2.4)
        }

        function linearToSrgb(value) {
            const channel = Number(value)
            if (!isFinite(channel))
                return 0
            return channel <= 0.0031308
                    ? 12.92 * channel
                    : 1.055 * Math.pow(channel, 1 / 2.4) - 0.055
        }

        function colorToOklab(colorValue) {
            const red = srgbToLinear(colorValue.r)
            const green = srgbToLinear(colorValue.g)
            const blue = srgbToLinear(colorValue.b)

            const l = signedCubeRoot(
                0.4122214708 * red + 0.5363325363 * green
                + 0.0514459929 * blue)
            const m = signedCubeRoot(
                0.2119034982 * red + 0.6806995451 * green
                + 0.1073969566 * blue)
            const s = signedCubeRoot(
                0.0883024619 * red + 0.2817188376 * green
                + 0.6299787005 * blue)

            return {
                lightness: 0.2104542553 * l + 0.7936177850 * m
                            - 0.0040720468 * s,
                a: 1.9779984951 * l - 2.4285922050 * m
                   + 0.4505937099 * s,
                b: 0.0259040371 * l + 0.7827717662 * m
                   - 0.8086757660 * s,
            }
        }

        function colorToOklch(colorValue) {
            const lab = colorToOklab(colorValue)
            const chroma = Math.sqrt(lab.a * lab.a + lab.b * lab.b)
            let hue = Math.atan2(lab.b, lab.a) * 180 / Math.PI
            if (hue < 0)
                hue += 360
            return {
                lightness: clampUnit(lab.lightness),
                chroma: clampOklchChroma(chroma),
                hue: chroma > 0.000001 ? hue : 0,
            }
        }

        function oklchToLinearRgb(lightness, chroma, hueDegrees) {
            const hueRadians = Number(hueDegrees) * Math.PI / 180
            const labA = Number(chroma) * Math.cos(hueRadians)
            const labB = Number(chroma) * Math.sin(hueRadians)
            const l = Number(lightness) + 0.3963377774 * labA
                                      + 0.2158037573 * labB
            const m = Number(lightness) - 0.1055613458 * labA
                                      - 0.0638541728 * labB
            const s = Number(lightness) - 0.0894841775 * labA
                                      - 1.2914855480 * labB
            const l3 = l * l * l
            const m3 = m * m * m
            const s3 = s * s * s
            return {
                r: 4.0767416621 * l3 - 3.3077115913 * m3
                   + 0.2309699292 * s3,
                g: -1.2684380046 * l3 + 2.6097574011 * m3
                   - 0.3413193965 * s3,
                b: -0.0041960863 * l3 - 0.7034186147 * m3
                   + 1.7076147010 * s3,
            }
        }

        function isInSrgbGamut(linearRgb) {
            const epsilon = 0.000001
            return linearRgb.r >= -epsilon && linearRgb.r <= 1 + epsilon
                    && linearRgb.g >= -epsilon && linearRgb.g <= 1 + epsilon
                    && linearRgb.b >= -epsilon && linearRgb.b <= 1 + epsilon
        }

        function oklchColor(lightness, chroma, hueDegrees, alpha) {
            const safeLightness = clampUnit(lightness)
            let safeChroma = clampOklchChroma(chroma)
            let linearRgb = oklchToLinearRgb(safeLightness, safeChroma,
                                              hueDegrees)

            // A direct OKLCH value can fall outside sRGB. Gamut-map the
            // generated display color by reducing chroma, but keep the
            // editor's requested L/C/H coordinates untouched.
            if (!isInSrgbGamut(linearRgb)) {
                let low = 0
                let high = safeChroma
                for (let iteration = 0; iteration < 18; ++iteration) {
                    const middle = (low + high) / 2
                    const candidate = oklchToLinearRgb(
                        safeLightness, middle, hueDegrees)
                    if (isInSrgbGamut(candidate))
                        low = middle
                    else
                        high = middle
                }
                safeChroma = low
                linearRgb = oklchToLinearRgb(safeLightness, safeChroma,
                                              hueDegrees)
            }

            const opacity = alpha === undefined ? selectedAlpha
                                                 : clampUnit(alpha)
            return {
                color: Qt.rgba(
                    clampUnit(linearToSrgb(linearRgb.r)),
                    clampUnit(linearToSrgb(linearRgb.g)),
                    clampUnit(linearToSrgb(linearRgb.b)), opacity),
            }
        }

        function oklchColorValue(lightness, chroma, hueDegrees, alpha) {
            return oklchColor(lightness, chroma, hueDegrees, alpha).color
        }

        function oklchDisplayRgb(lightness, chroma, hueDegrees) {
            // The wheel is a preview, so avoid a binary-search gamut map for
            // every one of its thousands of canvas cells. Clipping here is
            // sufficient; committed colors still use oklchColor() below.
            const linearRgb = oklchToLinearRgb(lightness, chroma, hueDegrees)
            return [
                clampUnit(linearToSrgb(linearRgb.r)),
                clampUnit(linearToSrgb(linearRgb.g)),
                clampUnit(linearToSrgb(linearRgb.b)),
            ]
        }

        function setFromColor(colorValue) {
            if (!colorValue)
                return
            const oklch = colorToOklch(colorValue)
            selectedHue = oklch.hue / 360
            selectedChroma = oklch.chroma
            selectedLightness = oklch.lightness
            const alpha = Number(colorValue.a)
            selectedAlpha = isFinite(alpha) ? clampUnit(alpha) : 1.0
            colorWheel.requestPaint()
        }

        function setFromRgb(r, g, b, a) {
            const alpha = a !== undefined ? Math.max(0, Math.min(255, a)) / 255 : selectedAlpha
            const clr = Qt.rgba(Math.max(0, Math.min(255, r)) / 255,
                                Math.max(0, Math.min(255, g)) / 255,
                                Math.max(0, Math.min(255, b)) / 255, alpha)
            setFromColor(clr)
            applyCurrentColor()
        }

        function applyCurrentColor() {
            if (!visible || !currentItem)
                return
            if (flashElementAnimation.targetProperty === currentItem.id) {
                themeColorConfigurator.stopAllFlashing()
            }
            const converted = oklchColor(selectedLightness, selectedChroma,
                                          selectedHue * 360, selectedAlpha)
            root[currentItem.id] = converted.color
            colorWheel.requestPaint()
        }

        function selectItem(index, shouldFlash) {
            const item = (index >= 0
                          && index < root.themeColorDefinitions.length)
                    ? root.themeColorDefinitions[index] : null
            const itemColor = item ? colorBeforeFlash(item.id) : null
            selectedIndex = index
            if (currentItem) {
                setFromColor(itemColor)
                if (shouldFlash)
                    flash(currentItem.id)
            }
        }

        onClosing: themeColorConfigurator.stopAllFlashing()

        onVisibleChanged: {
            if (visible) {
                selectItem(selectedIndex, false)
                statusToast = ""
            } else {
                themeColorConfigurator.stopAllFlashing()
            }
        }

        Rectangle {
            anchors.fill: parent
            color: root.dialogBg

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: root.snapPx(14)
                spacing: root.snapPx(10)

                // Header
                RowLayout {
                    id: themeDialogHeader
                    objectName: "themeDialogHeader"
                    Layout.fillWidth: false
                    Layout.preferredHeight: root.snapPx(18)
                    Layout.minimumHeight: root.snapPx(18)
                    Layout.maximumHeight: root.snapPx(18)
                    Layout.preferredWidth: root.snapPx(parent.width)
                    spacing: root.snapPx(8)
                    transform: Translate {
                        x: root.dialogPixelOffsetX(
                            themeDialogHeader,
                            themeColorConfigurator.contentItem)
                        y: root.dialogPixelOffsetY(
                            themeDialogHeader,
                            themeColorConfigurator.contentItem)
                    }

                    IconLabel {
                        icon.source: root.lucideIconSource(
                                         "palette", 18, root.dialogAccent)
                        icon.width: root.snapPx(18)
                        icon.height: root.snapPx(18)
                        icon.color: root.dialogAccent
                    }

                    Text {
                        text: "Theme Color Configurator"
                        color: root.textColor
                        font.family: root.guiMonospaceFontFamily
                        font.pixelSize: 14
                        font.weight: Font.Bold
                        Layout.fillWidth: true
                    }

                    Text {
                        text: typeof qtTheme !== "undefined" && qtTheme ? qtTheme.themeFilePath : "gui_theme.ini"
                        color: root.mutedText
                        font.family: root.guiMonospaceFontFamily
                        font.pixelSize: 10
                        elide: Text.ElideMiddle
                        Layout.maximumWidth: 320
                    }
                }

                Rectangle {
                    id: themeHeaderDivider
                    objectName: "themeHeaderDivider"
                    Layout.fillWidth: false
                    Layout.preferredWidth: root.snapPx(parent.width)
                    Layout.preferredHeight: root.separatorWidth
                    Layout.minimumHeight: root.separatorWidth
                    Layout.maximumHeight: root.separatorWidth
                    implicitHeight: root.separatorWidth
                    color: root.separatorColor
                    transform: Translate {
                        x: root.dialogPixelOffsetX(
                            themeHeaderDivider,
                            themeColorConfigurator.contentItem)
                        y: root.dialogPixelOffsetY(
                            themeHeaderDivider,
                            themeColorConfigurator.contentItem)
                    }
                }

                Rectangle {
                    id: themeFontRenderTypePanel
                    objectName: "themeFontRenderTypePanel"
                    Layout.fillWidth: false
                    Layout.preferredWidth: root.snapPx(parent.width)
                    Layout.preferredHeight: root.snapPx(42)
                    implicitHeight: root.snapPx(42)
                    transform: Translate {
                        x: root.dialogPixelOffsetX(
                            themeFontRenderTypePanel,
                            themeColorConfigurator.contentItem)
                        y: root.dialogPixelOffsetY(
                            themeFontRenderTypePanel,
                            themeColorConfigurator.contentItem)
                    }
                    radius: root.snapPx(4)
                    color: root.dialogHeaderBg
                    border.width: root.separatorWidth
                    border.color: root.controlBorder

                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: root.snapPx(8)
                        anchors.rightMargin: root.snapPx(8)
                        spacing: root.snapPx(8)

                        ColumnLayout {
                            id: themeFontRenderTypeLabels
                            objectName: "themeFontRenderTypeLabels"
                            Layout.fillWidth: true
                            spacing: root.snapPx(1)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeFontRenderTypeLabels,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeFontRenderTypeLabels,
                                    themeColorConfigurator.contentItem)
                            }

                            Text {
                                id: themeFontRenderTypeTitle
                                objectName: "themeFontRenderTypeTitle"
                                text: "Font rendering"
                                color: root.textColor
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 11
                                font.weight: Font.Bold
                            }

                            Text {
                                id: themeFontRenderTypeDescription
                                objectName: "themeFontRenderTypeDescription"
                                text: root.fontRenderTypeDescription
                                color: root.mutedText
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 9
                                elide: Text.ElideRight
                                Layout.fillWidth: true
                            }
                        }

                        ThemeRenderTypeComboBox {
                            id: themeFontRenderTypeCombo
                            objectName: "themeFontRenderTypeCombo"
                            options: root.fontRenderTypeOptions
                            selectedRenderType: root.fontRenderType
                            Layout.preferredWidth: root.snapPx(174)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeFontRenderTypeCombo,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeFontRenderTypeCombo,
                                    themeColorConfigurator.contentItem)
                            }
                            onRenderTypeActivated: function(value) {
                                if (root.setFontRenderType(value))
                                    themeColorConfigurator.statusToast =
                                        "Font rendering: " + root.fontRenderTypeName
                            }
                        }
                    }
                }

                Rectangle {
                    id: themeMouseWheelPanel
                    objectName: "themeMouseWheelPanel"
                    Layout.fillWidth: false
                    Layout.preferredWidth: root.snapPx(parent.width)
                    Layout.preferredHeight: root.snapPx(42)
                    implicitHeight: root.snapPx(42)
                    transform: Translate {
                        x: root.dialogPixelOffsetX(
                            themeMouseWheelPanel,
                            themeColorConfigurator.contentItem)
                        y: root.dialogPixelOffsetY(
                            themeMouseWheelPanel,
                            themeColorConfigurator.contentItem)
                    }
                    radius: root.snapPx(4)
                    color: root.dialogHeaderBg
                    border.width: root.separatorWidth
                    border.color: root.controlBorder

                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: root.snapPx(8)
                        anchors.rightMargin: root.snapPx(8)
                        spacing: root.snapPx(8)

                        ColumnLayout {
                            id: themeMouseWheelLabels
                            objectName: "themeMouseWheelLabels"
                            Layout.fillWidth: true
                            spacing: root.snapPx(1)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeMouseWheelLabels,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeMouseWheelLabels,
                                    themeColorConfigurator.contentItem)
                            }

                            Text {
                                id: themeMouseWheelTitle
                                objectName: "themeMouseWheelTitle"
                                text: "Mouse wheel control"
                                color: root.textColor
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 11
                                font.weight: Font.Bold
                            }

                            Text {
                                id: themeMouseWheelDescription
                                objectName: "themeMouseWheelDescription"
                                text: root.mouseWheelModeDescription
                                color: root.mutedText
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 9
                                elide: Text.ElideRight
                                Layout.fillWidth: true
                            }
                        }

                        ThemeRenderTypeComboBox {
                            id: themeMouseWheelCombo
                            objectName: "themeMouseWheelCombo"
                            options: root.mouseWheelModeOptions
                            selectedValue: root.mouseWheelMode
                            Layout.preferredWidth: root.snapPx(174)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeMouseWheelCombo,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeMouseWheelCombo,
                                    themeColorConfigurator.contentItem)
                            }
                            onOptionActivated: function(value) {
                                if (root.setMouseWheelMode(value))
                                    themeColorConfigurator.statusToast =
                                        "Mouse wheel: " + root.mouseWheelModeName
                            }
                        }
                    }
                }

                Rectangle {
                    id: themeNeutralFileTextPanel
                    objectName: "themeNeutralFileTextPanel"
                    Layout.fillWidth: false
                    Layout.preferredWidth: root.snapPx(parent.width)
                    Layout.preferredHeight: root.snapPx(42)
                    implicitHeight: root.snapPx(42)
                    transform: Translate {
                        x: root.dialogPixelOffsetX(
                            themeNeutralFileTextPanel,
                            themeColorConfigurator.contentItem)
                        y: root.dialogPixelOffsetY(
                            themeNeutralFileTextPanel,
                            themeColorConfigurator.contentItem)
                    }
                    radius: root.snapPx(4)
                    color: root.dialogHeaderBg
                    border.width: root.separatorWidth
                    border.color: root.controlBorder

                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: root.snapPx(8)
                        anchors.rightMargin: root.snapPx(8)
                        spacing: root.snapPx(8)

                        ColumnLayout {
                            id: themeNeutralFileTextLabels
                            objectName: "themeNeutralFileTextLabels"
                            Layout.fillWidth: true
                            spacing: root.snapPx(1)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeNeutralFileTextLabels,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeNeutralFileTextLabels,
                                    themeColorConfigurator.contentItem)
                            }

                            Text {
                                id: themeNeutralFileTextTitle
                                objectName: "themeNeutralFileTextTitle"
                                text: "Panel file and folder text"
                                color: root.textColor
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 11
                                font.weight: Font.Bold
                            }

                            Text {
                                id: themeNeutralFileTextDescription
                                objectName: "themeNeutralFileTextDescription"
                                text: "Use neutral text colors; semantic colors still tint icons"
                                color: root.mutedText
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 9
                                elide: Text.ElideRight
                                Layout.fillWidth: true
                            }
                        }

                        DialogCheckBox {
                            id: themeNeutralFileTextCheckBox
                            objectName: "themeNeutralFileTextCheckBox"
                            text: "Enabled"
                            checked: root.galleryNeutralFileTextColors
                            Layout.preferredWidth: root.snapPx(100)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeNeutralFileTextCheckBox,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeNeutralFileTextCheckBox,
                                    themeColorConfigurator.contentItem)
                            }
                            onClicked: {
                                root.galleryNeutralFileTextColors = checked
                                themeColorConfigurator.statusToast = checked
                                        ? "Neutral panel text enabled"
                                        : "Semantic panel text enabled"
                            }
                        }
                    }
                }

                // Main Body: Left (List of items) + Right (Editor)
                RowLayout {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    spacing: root.snapPx(12)

                    // Left Column: Items List (Takes remaining width)
                    ColumnLayout {
                        Layout.fillWidth: true
                        Layout.minimumWidth: 200
                        Layout.fillHeight: true
                        spacing: root.snapPx(6)

                        // Filter box with search icon
                        Rectangle {
                            id: themeColorFilter
                            objectName: "themeColorFilter"
                            Layout.fillWidth: false
                            Layout.preferredWidth: root.snapPx(parent.width)
                            Layout.preferredHeight: root.snapPx(28)
                            Layout.minimumHeight: root.snapPx(28)
                            Layout.maximumHeight: root.snapPx(28)
                            implicitHeight: root.snapPx(28)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeColorFilter,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeColorFilter,
                                    themeColorConfigurator.contentItem)
                            }
                            radius: root.snapPx(4)
                            color: root.controlPressedBg
                            border.width: root.separatorWidth
                            border.color: filterInput.activeFocus ? root.dialogAccent : root.controlBorder

                            RowLayout {
                                anchors.fill: parent
                                anchors.leftMargin: root.snapPx(8)
                                anchors.rightMargin: root.snapPx(8)
                                spacing: root.snapPx(6)

                                IconLabel {
                                    icon.source: root.lucideIconSource(
                                                     "search", 14,
                                                     root.mutedText)
                                    icon.width: root.snapPx(14)
                                    icon.height: root.snapPx(14)
                                    icon.color: root.mutedText
                                }

                                TextInput {
                                    id: filterInput
                                    Layout.fillWidth: true
                                    verticalAlignment: TextInput.AlignVCenter
                                    font.family: root.guiMonospaceFontFamily
                                    font.pixelSize: 11
                                    color: root.textColor
                                    selectByMouse: true
                                    selectionColor: root.selectedBg
                                    selectedTextColor: root.textColor

                                    Text {
                                        text: "Filter elements..."
                                        color: root.mutedText
                                        font.family: root.guiMonospaceFontFamily
                                        font.pixelSize: 11
                                        anchors.fill: parent
                                        verticalAlignment: Text.AlignVCenter
                                        visible: !filterInput.text && !filterInput.activeFocus
                                    }

                                    onTextChanged: themeColorConfigurator.filterQuery = text.toLowerCase().trim()
                                }
                            }
                        }

                        // List View with ScrollBar
                        Item {
                            Layout.fillWidth: true
                            Layout.fillHeight: true
                            Layout.minimumHeight: root.snapPx(36)

                            ListView {
                                id: themeItemsList
                                objectName: "themeItemsList"
                                width: root.snapPx(parent.width)
                                height: root.snapPx(parent.height)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeItemsList,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeItemsList,
                                        themeColorConfigurator.contentItem)
                                }
                                clip: true
                                boundsBehavior: Flickable.StopAtBounds
                                model: root.themeColorDefinitions.length

                            ScrollBar.vertical: ScrollBar {
                                id: themeListScrollBar
                                objectName: "themeListScrollBar"
                                policy: ScrollBar.AsNeeded
                                width: root.snapPx(8)
                                contentItem: Rectangle {
                                    implicitWidth: root.snapPx(6)
                                    radius: root.snapPx(3)
                                    color: themeListScrollBar.pressed ? root.dialogAccent
                                           : themeListScrollBar.hovered ? root.controlHoverBg
                                           : root.controlBorder
                                }
                                background: Rectangle {
                                    implicitWidth: root.snapPx(8)
                                    color: "transparent"
                            }
                        }

                            delegate: Item {
                                id: itemDelegate
                                readonly property var def: root.themeColorDefinitions[index]
                                readonly property bool isSelected: themeColorConfigurator.selectedIndex === index
                                readonly property bool matchesFilter: {
                                    if (!themeColorConfigurator.filterQuery)
                                        return true
                                    return def.name.toLowerCase().includes(themeColorConfigurator.filterQuery)
                                        || def.group.toLowerCase().includes(themeColorConfigurator.filterQuery)
                                }

                                width: root.snapPx(themeItemsList.width - (themeListScrollBar.visible ? (themeListScrollBar.width + 6) : 0))
                                height: matchesFilter ? root.snapPx(36) : 0
                                visible: matchesFilter

                                Rectangle {
                                    anchors.fill: parent
                                    radius: root.snapPx(4)
                                    color: itemDelegate.isSelected ? root.panelSelectionBg
                                           : itemMouse.containsMouse ? root.controlHoverBg : "transparent"
                                    border.width: itemDelegate.isSelected ? root.separatorWidth : 0
                                    border.color: root.panelSelectionBorder

                                    RowLayout {
                                        anchors.fill: parent
                                        anchors.leftMargin: root.snapPx(6)
                                        anchors.rightMargin: root.snapPx(6)
                                        spacing: root.snapPx(8)

                                        // Color swatch
                                        Rectangle {
                                            implicitWidth: root.snapPx(20)
                                            implicitHeight: root.snapPx(20)
                                            radius: root.snapPx(3)
                                            color: root.controlBg
                                            border.width: root.separatorWidth
                                            border.color: root.controlBorder
                                            clip: true

                                            Canvas {
                                                anchors.fill: parent
                                                onPaint: {
                                                    const ctx = getContext("2d")
                                                    const sz = 3
                                                    for (let x = 0; x < width; x += sz) {
                                                        for (let y = 0; y < height; y += sz) {
                                                            ctx.fillStyle = ((Math.floor(x / sz) + Math.floor(y / sz)) % 2 === 0) ? "#404b5a" : "#222c38"
                                                            ctx.fillRect(x, y, sz, sz)
                                                        }
                                                    }
                                                }
                                            }

                                            Rectangle {
                                                anchors.fill: parent
                                                color: root[itemDelegate.def.id]
                                            }
                                        }

                                        // Name & group
                                        ColumnLayout {
                                            Layout.fillWidth: true
                                            spacing: root.snapPx(1)

                                            Text {
                                                text: itemDelegate.def.name
                                                color: root.textColor
                                                font.pixelSize: 11
                                                font.weight: itemDelegate.isSelected ? Font.Bold : Font.Normal
                                                elide: Text.ElideRight
                                                Layout.fillWidth: true
                                            }

                                            Text {
                                                text: itemDelegate.def.group
                                                color: root.mutedText
                                                font.pixelSize: 9
                                                elide: Text.ElideRight
                                                Layout.fillWidth: true
                                            }
                                        }

                                        // Hex preview
                                        Text {
                                            text: root.formatColorHex(root[itemDelegate.def.id])
                                            color: root.mutedText
                                            font.family: root.guiMonospaceFontFamily
                                            font.pixelSize: 10
                                        }
                                    }

                                    MouseArea {
                                        id: itemMouse
                                        anchors.fill: parent
                                        hoverEnabled: true
                                        cursorShape: Qt.PointingHandCursor
                                        onEntered: {
                                            if (!pressed)
                                                themeColorConfigurator.flash(itemDelegate.def.id)
                                        }
                                        onExited: {
                                            themeColorConfigurator.endHoverFlash(
                                                itemDelegate.def.id)
                                        }
                                        onPressed: function(mouse) {
                                            themeColorConfigurator.selectItem(index, false)
                                            themeColorConfigurator.startPressFlash(itemDelegate.def.id)
                                        }
                                        onReleased: function(mouse) {
                                            themeColorConfigurator.endPressFlash(itemDelegate.def.id)
                                        }
                                        onCanceled: {
                                            themeColorConfigurator.endPressFlash(itemDelegate.def.id)
                                        }
                                    }
                                }
                            }
                        }
                    }
                    }

                    // Vertical Divider
                    Rectangle {
                        id: themeColorDivider
                        objectName: "themeColorDivider"
                        Layout.fillHeight: false
                        Layout.preferredWidth: root.separatorWidth
                        Layout.minimumWidth: root.separatorWidth
                        Layout.maximumWidth: root.separatorWidth
                        Layout.preferredHeight: root.snapPx(parent.height)
                        implicitWidth: root.separatorWidth
                        color: root.separatorColor
                        transform: Translate {
                            x: root.dialogPixelOffsetX(
                                themeColorDivider,
                                themeColorConfigurator.contentItem)
                            y: root.dialogPixelOffsetY(
                                themeColorDivider,
                                themeColorConfigurator.contentItem)
                        }
                    }

                    // Right Column: Color Editor - STRICTLY FIXED WIDTH
                    ColumnLayout {
                        id: themeColorEditor
                        objectName: "themeColorEditor"
                        Layout.preferredWidth: root.snapPx(320)
                        Layout.minimumWidth: root.snapPx(320)
                        Layout.maximumWidth: root.snapPx(320)
                        Layout.fillHeight: true
                        spacing: root.snapPx(8)
                        transform: Translate {
                            x: root.dialogPixelOffsetX(
                                themeColorEditor,
                                themeColorConfigurator.contentItem)
                            y: root.dialogPixelOffsetY(
                                themeColorEditor,
                                themeColorConfigurator.contentItem)
                        }

                        // Active item title & badge
                        RowLayout {
                            id: themeActiveColorRow
                            objectName: "themeActiveColorRow"
                            Layout.fillWidth: true
                            Layout.preferredHeight: root.snapPx(26)
                            Layout.minimumHeight: root.snapPx(26)
                            Layout.maximumHeight: root.snapPx(26)
                            spacing: root.snapPx(8)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeActiveColorRow,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeActiveColorRow,
                                    themeColorConfigurator.contentItem)
                            }

                            Text {
                                text: themeColorConfigurator.currentItem ? themeColorConfigurator.currentItem.name : ""
                                color: root.textColor
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 12
                                font.weight: Font.Bold
                                elide: Text.ElideRight
                                Layout.fillWidth: true
                            }

                            Rectangle {
                                id: themeColorGroupBadge
                                objectName: "themeColorGroupBadge"
                                radius: root.snapPx(3)
                                color: root.controlBg
                                border.width: root.separatorWidth
                                border.color: root.controlBorder
                                implicitHeight: root.snapPx(18)
                                implicitWidth: root.snapPx(groupText.implicitWidth + 8)
                                Layout.preferredHeight: root.snapPx(18)
                                Layout.preferredWidth: root.snapPx(
                                    groupText.implicitWidth + 8)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeColorGroupBadge,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeColorGroupBadge,
                                        themeColorConfigurator.contentItem)
                                }

                                Text {
                                    id: groupText
                                    text: themeColorConfigurator.currentItem ? themeColorConfigurator.currentItem.group : ""
                                    color: root.mutedText
                                    font.pixelSize: 9
                                    x: root.snapPx((parent.width - width) / 2)
                                    y: root.snapPx((parent.height - height) / 2)
                                }
                            }

                            // Large preview swatch
                            Rectangle {
                                id: themeColorPreviewSwatch
                                objectName: "themeColorPreviewSwatch"
                                implicitWidth: root.snapPx(26)
                                implicitHeight: root.snapPx(26)
                                Layout.preferredWidth: root.snapPx(26)
                                Layout.preferredHeight: root.snapPx(26)
                                radius: root.snapPx(4)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeColorPreviewSwatch,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeColorPreviewSwatch,
                                        themeColorConfigurator.contentItem)
                                }
                                color: root.controlBg
                                border.width: root.separatorWidth
                                border.color: root.controlBorder
                                clip: true

                                Canvas {
                                    anchors.fill: parent
                                    onPaint: {
                                        const ctx = getContext("2d")
                                        const sz = 4
                                        for (let x = 0; x < width; x += sz) {
                                            for (let y = 0; y < height; y += sz) {
                                                ctx.fillStyle = ((Math.floor(x / sz) + Math.floor(y / sz)) % 2 === 0) ? "#404b5a" : "#222c38"
                                                ctx.fillRect(x, y, sz, sz)
                                            }
                                        }
                                    }
                                }

                                Rectangle {
                                    anchors.fill: parent
                                    color: themeColorConfigurator.currentItem ? root[themeColorConfigurator.currentItem.id] : "transparent"
                                }
                            }
                        }

                        // 2D OKLCH hue/chroma wheel at the current lightness.
                        Canvas {
                            id: colorWheel
                            objectName: "themeColorWheel"
                            Layout.alignment: Qt.AlignHCenter
                            Layout.preferredWidth: root.snapPx(170)
                            Layout.preferredHeight: root.snapPx(170)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    colorWheel,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    colorWheel,
                                    themeColorConfigurator.contentItem)
                            }

                            function selectPoint(pointX, pointY) {
                                const centerX = width / 2
                                const centerY = height / 2
                                const deltaX = pointX - centerX
                                const deltaY = pointY - centerY
                                const radius = Math.max(1, Math.min(width, height) / 2 - 2)
                                const distance = Math.sqrt(deltaX * deltaX + deltaY * deltaY)
                                let hue = Math.atan2(deltaY, deltaX) / (2 * Math.PI)
                                if (hue < 0)
                                    hue += 1
                                themeColorConfigurator.selectedHue = hue
                                themeColorConfigurator.selectedChroma =
                                    Math.min(
                                        themeColorConfigurator.wheelOklchChroma,
                                        distance / radius
                                        * themeColorConfigurator.wheelOklchChroma)
                                themeColorConfigurator.applyCurrentColor()
                            }

                            onPaint: {
                                const context = getContext("2d")
                                const canvasWidth = Math.max(1, width)
                                const canvasHeight = Math.max(1, height)
                                const centerX = canvasWidth / 2
                                const centerY = canvasHeight / 2
                                const radius = Math.max(1, Math.min(canvasWidth, canvasHeight) / 2 - 2)

                                context.clearRect(0, 0, canvasWidth, canvasHeight)
                                const chromaSteps = 24
                                const hueSteps = 180
                                for (let chromaIndex = 0;
                                     chromaIndex < chromaSteps;
                                     ++chromaIndex) {
                                    const innerRadius = radius * chromaIndex
                                                         / chromaSteps
                                    const outerRadius = radius
                                                         * (chromaIndex + 1)
                                                         / chromaSteps
                                    const chroma =
                                        (chromaIndex + 0.5) / chromaSteps
                                        * themeColorConfigurator.wheelOklchChroma
                                    for (let sectorIndex = 0;
                                         sectorIndex < hueSteps;
                                         ++sectorIndex) {
                                        const startAngle = sectorIndex
                                                           * 2 * Math.PI
                                                           / hueSteps
                                        const endAngle = (sectorIndex + 1)
                                                         * 2 * Math.PI
                                                         / hueSteps
                                        const rgb =
                                            themeColorConfigurator.oklchDisplayRgb(
                                                themeColorConfigurator.selectedLightness,
                                                chroma,
                                                sectorIndex * 360 / hueSteps)
                                        context.fillStyle = "rgb(" +
                                            Math.round(rgb[0] * 255) + "," +
                                            Math.round(rgb[1] * 255) + "," +
                                            Math.round(rgb[2] * 255) + ")"
                                        context.beginPath()
                                        if (innerRadius <= 0) {
                                            context.moveTo(centerX, centerY)
                                        } else {
                                            context.moveTo(
                                                centerX + Math.cos(startAngle)
                                                * innerRadius,
                                                centerY + Math.sin(startAngle)
                                                * innerRadius)
                                        }
                                        context.arc(centerX, centerY,
                                                    outerRadius, startAngle,
                                                    endAngle, false)
                                        if (innerRadius > 0) {
                                            context.arc(centerX, centerY,
                                                        innerRadius, endAngle,
                                                        startAngle, true)
                                        }
                                        context.closePath()
                                        context.fill()
                                    }
                                }

                                const markerDistance = Math.min(
                                    1,
                                    themeColorConfigurator.selectedChroma
                                    / themeColorConfigurator.wheelOklchChroma)
                                    * radius
                                const markerAngle = themeColorConfigurator.selectedHue * 2 * Math.PI
                                const markerX = centerX + Math.cos(markerAngle) * markerDistance
                                const markerY = centerY + Math.sin(markerAngle) * markerDistance
                                context.beginPath()
                                context.arc(markerX, markerY, 6, 0, 2 * Math.PI)
                                context.lineWidth = 3
                                context.strokeStyle = "#80000000"
                                context.stroke()
                                context.beginPath()
                                context.arc(markerX, markerY, 6, 0, 2 * Math.PI)
                                context.lineWidth = 2
                                context.strokeStyle = "#ffffff"
                                context.stroke()
                            }

                            MouseArea {
                                anchors.fill: parent
                                cursorShape: Qt.CrossCursor
                                onPressed: function(mouse) { colorWheel.selectPoint(mouse.x, mouse.y) }
                                onPositionChanged: function(mouse) { if (pressed) colorWheel.selectPoint(mouse.x, mouse.y) }
                            }
                        }

                        // HUE (OKLCH) Slider & Input
                        RowLayout {
                            id: themeHueRow
                            objectName: "themeHueRow"
                            Layout.fillWidth: true
                            Layout.preferredHeight: root.snapPx(20)
                            spacing: root.snapPx(6)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeHueRow,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeHueRow,
                                    themeColorConfigurator.contentItem)
                            }

                            Text { text: "H"; color: root.mutedText; font.family: root.guiMonospaceFontFamily; font.pixelSize: 11; font.weight: Font.DemiBold; Layout.preferredWidth: root.snapPx(12) }
                            Slider {
                                id: hueSlider
                                objectName: "themeHueSlider"
                                from: 0; to: 1; value: themeColorConfigurator.selectedHue
                                Layout.fillWidth: false
                                Layout.preferredWidth: root.snapPx(257)
                                Layout.preferredHeight: root.snapPx(18)
                                implicitHeight: root.snapPx(18)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        hueSlider,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        hueSlider,
                                        themeColorConfigurator.contentItem)
                                }
                                onMoved: { themeColorConfigurator.selectedHue = value; themeColorConfigurator.applyCurrentColor(); }
                                background: Rectangle {
                                    x: root.snapPx(hueSlider.leftPadding + hueSlider.handle.width / 2); y: root.snapPx(hueSlider.topPadding)
                                    width: root.snapPx(hueSlider.availableWidth - hueSlider.handle.width); height: root.snapPx(hueSlider.availableHeight)
                                    radius: root.snapPx(height / 2); border.width: root.separatorWidth; border.color: root.controlBorder
                                    gradient: Gradient {
                                        orientation: Gradient.Horizontal
                                        GradientStop {
                                            position: 0.000
                                            color: themeColorConfigurator.oklchColorValue(
                                                themeColorConfigurator.selectedLightness,
                                                Math.max(0.16,
                                                         themeColorConfigurator.selectedChroma),
                                                0, 1)
                                        }
                                        GradientStop {
                                            position: 0.167
                                            color: themeColorConfigurator.oklchColorValue(
                                                themeColorConfigurator.selectedLightness,
                                                Math.max(0.16,
                                                         themeColorConfigurator.selectedChroma),
                                                60, 1)
                                        }
                                        GradientStop {
                                            position: 0.333
                                            color: themeColorConfigurator.oklchColorValue(
                                                themeColorConfigurator.selectedLightness,
                                                Math.max(0.16,
                                                         themeColorConfigurator.selectedChroma),
                                                120, 1)
                                        }
                                        GradientStop {
                                            position: 0.500
                                            color: themeColorConfigurator.oklchColorValue(
                                                themeColorConfigurator.selectedLightness,
                                                Math.max(0.16,
                                                         themeColorConfigurator.selectedChroma),
                                                180, 1)
                                        }
                                        GradientStop {
                                            position: 0.667
                                            color: themeColorConfigurator.oklchColorValue(
                                                themeColorConfigurator.selectedLightness,
                                                Math.max(0.16,
                                                         themeColorConfigurator.selectedChroma),
                                                240, 1)
                                        }
                                        GradientStop {
                                            position: 0.833
                                            color: themeColorConfigurator.oklchColorValue(
                                                themeColorConfigurator.selectedLightness,
                                                Math.max(0.16,
                                                         themeColorConfigurator.selectedChroma),
                                                300, 1)
                                        }
                                        GradientStop {
                                            position: 1.000
                                            color: themeColorConfigurator.oklchColorValue(
                                                themeColorConfigurator.selectedLightness,
                                                Math.max(0.16,
                                                         themeColorConfigurator.selectedChroma),
                                                360, 1)
                                        }
                                    }
                                }
                                handle: Rectangle {
                                    x: root.snapPx(hueSlider.leftPadding + hueSlider.visualPosition * (hueSlider.availableWidth - width))
                                    y: root.snapPx(hueSlider.topPadding + (hueSlider.availableHeight - height) / 2)
                                    width: root.snapPx(14); height: root.snapPx(14); radius: root.snapPx(7); color: root.controlBg; border.width: root.snapPx(2); border.color: root.textColor
                                }
                            }
                            Rectangle {
                                id: themeHueInputBox
                                objectName: "themeHueInputBox"
                                implicitWidth: root.snapPx(38); implicitHeight: root.snapPx(20); radius: root.snapPx(3); color: root.controlPressedBg; border.width: root.separatorWidth; border.color: hInput.activeFocus ? root.dialogAccent : root.controlBorder
                                Layout.preferredWidth: root.snapPx(38)
                                Layout.preferredHeight: root.snapPx(20)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeHueInputBox,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeHueInputBox,
                                        themeColorConfigurator.contentItem)
                                }
                                TextInput {
                                    id: hInput
                                    anchors.fill: parent; anchors.leftMargin: root.snapPx(2); anchors.rightMargin: root.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                                    font.family: root.guiMonospaceFontFamily; font.pixelSize: 10; color: root.textColor; selectByMouse: true; selectionColor: root.selectedBg; selectedTextColor: root.textColor
                                    validator: IntValidator { bottom: 0; top: 360 }
                                    text: !activeFocus ? Math.round(themeColorConfigurator.selectedHue * 360).toString() : text
                                    onTextEdited: { const val = parseInt(text); if (!isNaN(val)) { themeColorConfigurator.selectedHue = Math.max(0, Math.min(360, val)) / 360; themeColorConfigurator.applyCurrentColor(); } }
                                    onEditingFinished: { text = Math.round(themeColorConfigurator.selectedHue * 360).toString() }
                                }
                            }
                        }

                        // CHROMA (OKLCH) Slider & Input
                        RowLayout {
                            id: themeChromaRow
                            objectName: "themeChromaRow"
                            Layout.fillWidth: true
                            Layout.preferredHeight: root.snapPx(20)
                            spacing: root.snapPx(6)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeChromaRow,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeChromaRow,
                                    themeColorConfigurator.contentItem)
                            }

                            Text { text: "C"; color: root.mutedText; font.family: root.guiMonospaceFontFamily; font.pixelSize: 11; font.weight: Font.DemiBold; Layout.preferredWidth: root.snapPx(12) }
                            Slider {
                                id: chromaSlider
                                objectName: "themeChromaSlider"
                                from: 0; to: themeColorConfigurator.maxOklchChroma
                                stepSize: 0.001
                                value: themeColorConfigurator.selectedChroma
                                Layout.fillWidth: false
                                Layout.preferredWidth: root.snapPx(257)
                                Layout.preferredHeight: root.snapPx(18)
                                implicitHeight: root.snapPx(18)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        chromaSlider,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        chromaSlider,
                                        themeColorConfigurator.contentItem)
                                }
                                onMoved: { themeColorConfigurator.selectedChroma = value; themeColorConfigurator.applyCurrentColor(); }
                                background: Rectangle {
                                    x: root.snapPx(chromaSlider.leftPadding + chromaSlider.handle.width / 2); y: root.snapPx(chromaSlider.topPadding)
                                    width: root.snapPx(chromaSlider.availableWidth - chromaSlider.handle.width); height: root.snapPx(chromaSlider.availableHeight)
                                    radius: root.snapPx(height / 2); border.width: root.separatorWidth; border.color: root.controlBorder
                                    gradient: Gradient {
                                        orientation: Gradient.Horizontal
                                        GradientStop {
                                            position: 0
                                            color: themeColorConfigurator.oklchColorValue(
                                                themeColorConfigurator.selectedLightness,
                                                0,
                                                themeColorConfigurator.selectedHue * 360,
                                                1)
                                        }
                                        GradientStop {
                                            position: 1
                                            color: themeColorConfigurator.oklchColorValue(
                                                themeColorConfigurator.selectedLightness,
                                                themeColorConfigurator.maxOklchChroma,
                                                themeColorConfigurator.selectedHue * 360,
                                                1)
                                        }
                                    }
                                }
                                handle: Rectangle {
                                    x: root.snapPx(chromaSlider.leftPadding + chromaSlider.visualPosition * (chromaSlider.availableWidth - width))
                                    y: root.snapPx(chromaSlider.topPadding + (chromaSlider.availableHeight - height) / 2)
                                    width: root.snapPx(14); height: root.snapPx(14); radius: root.snapPx(7); color: root.controlBg; border.width: root.snapPx(2); border.color: root.textColor
                                }
                            }
                            Rectangle {
                                id: themeChromaInputBox
                                objectName: "themeChromaInputBox"
                                implicitWidth: root.snapPx(38); implicitHeight: root.snapPx(20); radius: root.snapPx(3); color: root.controlPressedBg; border.width: root.separatorWidth; border.color: cInput.activeFocus ? root.dialogAccent : root.controlBorder
                                Layout.preferredWidth: root.snapPx(38)
                                Layout.preferredHeight: root.snapPx(20)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeChromaInputBox,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeChromaInputBox,
                                        themeColorConfigurator.contentItem)
                                }
                                TextInput {
                                    id: cInput
                                    anchors.fill: parent; anchors.leftMargin: root.snapPx(2); anchors.rightMargin: root.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                                    font.family: root.guiMonospaceFontFamily; font.pixelSize: 10; color: root.textColor; selectByMouse: true; selectionColor: root.selectedBg; selectedTextColor: root.textColor
                                    validator: DoubleValidator {
                                        bottom: 0
                                        top: themeColorConfigurator.maxOklchChroma
                                        decimals: 3
                                        notation: DoubleValidator.StandardNotation
                                    }
                                    text: !activeFocus ? themeColorConfigurator.selectedChroma.toFixed(3) : text
                                    onTextEdited: {
                                        const val = parseFloat(text)
                                        if (!isNaN(val)) {
                                            themeColorConfigurator.selectedChroma =
                                                themeColorConfigurator.clampOklchChroma(val)
                                            themeColorConfigurator.applyCurrentColor()
                                        }
                                    }
                                    onEditingFinished: {
                                        text = themeColorConfigurator.selectedChroma.toFixed(3)
                                    }
                                }
                            }
                        }

                        // LIGHTNESS Slider & Input
                        RowLayout {
                            id: themeLightnessRow
                            objectName: "themeLightnessRow"
                            Layout.fillWidth: true
                            Layout.preferredHeight: root.snapPx(20)
                            spacing: root.snapPx(6)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeLightnessRow,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeLightnessRow,
                                    themeColorConfigurator.contentItem)
                            }

                            Text { text: "L"; color: root.mutedText; font.family: root.guiMonospaceFontFamily; font.pixelSize: 11; font.weight: Font.DemiBold; Layout.preferredWidth: root.snapPx(12) }
                            Slider {
                                id: lightnessSlider
                                objectName: "themeLightnessSlider"
                                from: 0; to: 1; value: themeColorConfigurator.selectedLightness
                                Layout.fillWidth: false
                                Layout.preferredWidth: root.snapPx(257)
                                Layout.preferredHeight: root.snapPx(18)
                                implicitHeight: root.snapPx(18)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        lightnessSlider,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        lightnessSlider,
                                        themeColorConfigurator.contentItem)
                                }
                                onMoved: { themeColorConfigurator.selectedLightness = value; themeColorConfigurator.applyCurrentColor(); }
                                background: Rectangle {
                                    x: root.snapPx(lightnessSlider.leftPadding + lightnessSlider.handle.width / 2); y: root.snapPx(lightnessSlider.topPadding)
                                    width: root.snapPx(lightnessSlider.availableWidth - lightnessSlider.handle.width); height: root.snapPx(lightnessSlider.availableHeight)
                                    radius: root.snapPx(height / 2); border.width: root.separatorWidth; border.color: root.controlBorder
                                    gradient: Gradient {
                                        orientation: Gradient.Horizontal
                                        GradientStop { position: 0; color: "#000000" }
                                        GradientStop {
                                            position: 0.5
                                            color: themeColorConfigurator.oklchColorValue(
                                                0.5,
                                                themeColorConfigurator.selectedChroma,
                                                themeColorConfigurator.selectedHue * 360,
                                                1)
                                        }
                                        GradientStop { position: 1; color: "#ffffff" }
                                    }
                                }
                                handle: Rectangle {
                                    x: root.snapPx(lightnessSlider.leftPadding + lightnessSlider.visualPosition * (lightnessSlider.availableWidth - width))
                                    y: root.snapPx(lightnessSlider.topPadding + (lightnessSlider.availableHeight - height) / 2)
                                    width: root.snapPx(14); height: root.snapPx(14); radius: root.snapPx(7); color: root.controlBg; border.width: root.snapPx(2); border.color: root.textColor
                                }
                            }
                            Rectangle {
                                id: themeLightnessInputBox
                                objectName: "themeLightnessInputBox"
                                implicitWidth: root.snapPx(38); implicitHeight: root.snapPx(20); radius: root.snapPx(3); color: root.controlPressedBg; border.width: root.separatorWidth; border.color: lInput.activeFocus ? root.dialogAccent : root.controlBorder
                                Layout.preferredWidth: root.snapPx(38)
                                Layout.preferredHeight: root.snapPx(20)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeLightnessInputBox,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeLightnessInputBox,
                                        themeColorConfigurator.contentItem)
                                }
                                TextInput {
                                    id: lInput
                                    anchors.fill: parent; anchors.leftMargin: root.snapPx(2); anchors.rightMargin: root.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                                    font.family: root.guiMonospaceFontFamily; font.pixelSize: 10; color: root.textColor; selectByMouse: true; selectionColor: root.selectedBg; selectedTextColor: root.textColor
                                    validator: IntValidator { bottom: 0; top: 100 }
                                    text: !activeFocus ? Math.round(themeColorConfigurator.selectedLightness * 100).toString() : text
                                    onTextEdited: { const val = parseInt(text); if (!isNaN(val)) { themeColorConfigurator.selectedLightness = Math.max(0, Math.min(100, val)) / 100; themeColorConfigurator.applyCurrentColor(); } }
                                    onEditingFinished: { text = Math.round(themeColorConfigurator.selectedLightness * 100).toString() }
                                }
                            }
                        }

                        // ALPHA Slider & Input
                        RowLayout {
                            id: themeAlphaRow
                            objectName: "themeAlphaRow"
                            Layout.fillWidth: true
                            Layout.preferredHeight: root.snapPx(20)
                            spacing: root.snapPx(6)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeAlphaRow,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeAlphaRow,
                                    themeColorConfigurator.contentItem)
                            }

                            Text { text: "A"; color: root.mutedText; font.family: root.guiMonospaceFontFamily; font.pixelSize: 11; font.weight: Font.DemiBold; Layout.preferredWidth: root.snapPx(12) }
                            Slider {
                                id: alphaSlider
                                objectName: "themeAlphaSlider"
                                from: 0; to: 1; value: themeColorConfigurator.selectedAlpha
                                Layout.fillWidth: false
                                Layout.preferredWidth: root.snapPx(257)
                                Layout.preferredHeight: root.snapPx(18)
                                implicitHeight: root.snapPx(18)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        alphaSlider,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        alphaSlider,
                                        themeColorConfigurator.contentItem)
                                }
                                onMoved: { themeColorConfigurator.selectedAlpha = value; themeColorConfigurator.applyCurrentColor(); }
                                background: Rectangle {
                                    x: root.snapPx(alphaSlider.leftPadding + alphaSlider.handle.width / 2); y: root.snapPx(alphaSlider.topPadding)
                                    width: root.snapPx(alphaSlider.availableWidth - alphaSlider.handle.width); height: root.snapPx(alphaSlider.availableHeight)
                                    radius: root.snapPx(height / 2); border.width: root.separatorWidth; border.color: root.controlBorder
                                    clip: true

                                    Canvas {
                                        anchors.fill: parent
                                        onPaint: {
                                            const ctx = getContext("2d")
                                            const sz = 3
                                            for (let x = 0; x < width; x += sz) {
                                                for (let y = 0; y < height; y += sz) {
                                                    ctx.fillStyle = ((Math.floor(x / sz) + Math.floor(y / sz)) % 2 === 0) ? "#404b5a" : "#222c38"
                                                    ctx.fillRect(x, y, sz, sz)
                                                }
                                            }
                                        }
                                    }

                                    Rectangle {
                                        anchors.fill: parent
                                        radius: root.snapPx(height / 2)
                                        gradient: Gradient {
                                            orientation: Gradient.Horizontal
                                            GradientStop {
                                                position: 0
                                                color: themeColorConfigurator.oklchColorValue(
                                                    themeColorConfigurator.selectedLightness,
                                                    themeColorConfigurator.selectedChroma,
                                                    themeColorConfigurator.selectedHue * 360,
                                                    0)
                                            }
                                            GradientStop {
                                                position: 1
                                                color: themeColorConfigurator.oklchColorValue(
                                                    themeColorConfigurator.selectedLightness,
                                                    themeColorConfigurator.selectedChroma,
                                                    themeColorConfigurator.selectedHue * 360,
                                                    1)
                                            }
                                        }
                                    }
                                }
                                handle: Rectangle {
                                    x: root.snapPx(alphaSlider.leftPadding + alphaSlider.visualPosition * (alphaSlider.availableWidth - width))
                                    y: root.snapPx(alphaSlider.topPadding + (alphaSlider.availableHeight - height) / 2)
                                    width: root.snapPx(14); height: root.snapPx(14); radius: root.snapPx(7); color: root.controlBg; border.width: root.snapPx(2); border.color: root.textColor
                                }
                            }
                            Rectangle {
                                id: themeAlphaInputBox
                                objectName: "themeAlphaInputBox"
                                implicitWidth: root.snapPx(38); implicitHeight: root.snapPx(20); radius: root.snapPx(3); color: root.controlPressedBg; border.width: root.separatorWidth; border.color: aInput.activeFocus ? root.dialogAccent : root.controlBorder
                                Layout.preferredWidth: root.snapPx(38)
                                Layout.preferredHeight: root.snapPx(20)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeAlphaInputBox,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeAlphaInputBox,
                                        themeColorConfigurator.contentItem)
                                }
                                TextInput {
                                    id: aInput
                                    anchors.fill: parent; anchors.leftMargin: root.snapPx(2); anchors.rightMargin: root.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                                    font.family: root.guiMonospaceFontFamily; font.pixelSize: 10; color: root.textColor; selectByMouse: true; selectionColor: root.selectedBg; selectedTextColor: root.textColor
                                    validator: IntValidator { bottom: 0; top: 100 }
                                    text: !activeFocus ? Math.round(themeColorConfigurator.selectedAlpha * 100).toString() : text
                                    onTextEdited: { const val = parseInt(text); if (!isNaN(val)) { themeColorConfigurator.selectedAlpha = Math.max(0, Math.min(100, val)) / 100; themeColorConfigurator.applyCurrentColor(); } }
                                    onEditingFinished: { text = Math.round(themeColorConfigurator.selectedAlpha * 100).toString() }
                                }
                            }
                        }

                        // RGB & HEX Row
                        RowLayout {
                            id: themeRgbHexRow
                            objectName: "themeRgbHexRow"
                            Layout.fillWidth: true
                            Layout.preferredHeight: root.snapPx(20)
                            spacing: root.snapPx(6)
                            transform: Translate {
                                x: root.dialogPixelOffsetX(
                                    themeRgbHexRow,
                                    themeColorConfigurator.contentItem)
                                y: root.dialogPixelOffsetY(
                                    themeRgbHexRow,
                                    themeColorConfigurator.contentItem)
                            }

                            // R
                            Text { id: themeRedLabel; text: "R:"; color: root.mutedText; font.pixelSize: 10 }
                            Rectangle {
                                id: themeRedInputBox
                                objectName: "themeRedInputBox"
                                Layout.fillWidth: false; Layout.preferredWidth: root.snapPx(34); implicitHeight: root.snapPx(20); radius: root.snapPx(3); color: root.controlPressedBg; border.width: root.separatorWidth; border.color: rInput.activeFocus ? root.dialogAccent : root.controlBorder
                                Layout.preferredHeight: root.snapPx(20)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeRedInputBox,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeRedInputBox,
                                        themeColorConfigurator.contentItem)
                                }
                                TextInput {
                                    id: rInput
                                    anchors.fill: parent; anchors.leftMargin: root.snapPx(2); anchors.rightMargin: root.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                                    font.family: root.guiMonospaceFontFamily; font.pixelSize: 10; color: root.textColor; selectByMouse: true; selectionColor: root.selectedBg; selectedTextColor: root.textColor
                                    validator: IntValidator { bottom: 0; top: 255 }
                                    text: !activeFocus && themeColorConfigurator.currentItem ? Math.round(root[themeColorConfigurator.currentItem.id].r * 255).toString() : text
                                    onTextEdited: {
                                        const val = parseInt(text)
                                        if (!isNaN(val) && themeColorConfigurator.currentItem) {
                                            themeColorConfigurator.setFromRgb(val, Math.round(root[themeColorConfigurator.currentItem.id].g * 255), Math.round(root[themeColorConfigurator.currentItem.id].b * 255))
                                        }
                                    }
                                    onEditingFinished: { if (themeColorConfigurator.currentItem) text = Math.round(root[themeColorConfigurator.currentItem.id].r * 255).toString() }
                                }
                            }

                            // G
                            Text { id: themeGreenLabel; text: "G:"; color: root.mutedText; font.pixelSize: 10 }
                            Rectangle {
                                id: themeGreenInputBox
                                objectName: "themeGreenInputBox"
                                Layout.fillWidth: false; Layout.preferredWidth: root.snapPx(34); implicitHeight: root.snapPx(20); radius: root.snapPx(3); color: root.controlPressedBg; border.width: root.separatorWidth; border.color: gInput.activeFocus ? root.dialogAccent : root.controlBorder
                                Layout.preferredHeight: root.snapPx(20)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeGreenInputBox,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeGreenInputBox,
                                        themeColorConfigurator.contentItem)
                                }
                                TextInput {
                                    id: gInput
                                    anchors.fill: parent; anchors.leftMargin: root.snapPx(2); anchors.rightMargin: root.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                                    font.family: root.guiMonospaceFontFamily; font.pixelSize: 10; color: root.textColor; selectByMouse: true; selectionColor: root.selectedBg; selectedTextColor: root.textColor
                                    validator: IntValidator { bottom: 0; top: 255 }
                                    text: !activeFocus && themeColorConfigurator.currentItem ? Math.round(root[themeColorConfigurator.currentItem.id].g * 255).toString() : text
                                    onTextEdited: {
                                        const val = parseInt(text)
                                        if (!isNaN(val) && themeColorConfigurator.currentItem) {
                                            themeColorConfigurator.setFromRgb(Math.round(root[themeColorConfigurator.currentItem.id].r * 255), val, Math.round(root[themeColorConfigurator.currentItem.id].b * 255))
                                        }
                                    }
                                    onEditingFinished: { if (themeColorConfigurator.currentItem) text = Math.round(root[themeColorConfigurator.currentItem.id].g * 255).toString() }
                                }
                            }

                            // B
                            Text { id: themeBlueLabel; text: "B:"; color: root.mutedText; font.pixelSize: 10 }
                            Rectangle {
                                id: themeBlueInputBox
                                objectName: "themeBlueInputBox"
                                Layout.fillWidth: false; Layout.preferredWidth: root.snapPx(34); implicitHeight: root.snapPx(20); radius: root.snapPx(3); color: root.controlPressedBg; border.width: root.separatorWidth; border.color: bInput.activeFocus ? root.dialogAccent : root.controlBorder
                                Layout.preferredHeight: root.snapPx(20)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeBlueInputBox,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeBlueInputBox,
                                        themeColorConfigurator.contentItem)
                                }
                                TextInput {
                                    id: bInput
                                    anchors.fill: parent; anchors.leftMargin: root.snapPx(2); anchors.rightMargin: root.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                                    font.family: root.guiMonospaceFontFamily; font.pixelSize: 10; color: root.textColor; selectByMouse: true; selectionColor: root.selectedBg; selectedTextColor: root.textColor
                                    validator: IntValidator { bottom: 0; top: 255 }
                                    text: !activeFocus && themeColorConfigurator.currentItem ? Math.round(root[themeColorConfigurator.currentItem.id].b * 255).toString() : text
                                    onTextEdited: {
                                        const val = parseInt(text)
                                        if (!isNaN(val) && themeColorConfigurator.currentItem) {
                                            themeColorConfigurator.setFromRgb(Math.round(root[themeColorConfigurator.currentItem.id].r * 255), Math.round(root[themeColorConfigurator.currentItem.id].g * 255), val)
                                        }
                                    }
                                    onEditingFinished: { if (themeColorConfigurator.currentItem) text = Math.round(root[themeColorConfigurator.currentItem.id].b * 255).toString() }
                                }
                            }

                            // HEX
                            Text { id: themeHexLabel; text: "HEX:"; color: root.mutedText; font.pixelSize: 10 }
                            Rectangle {
                                id: themeHexInputBox
                                objectName: "themeHexInputBox"
                                Layout.preferredWidth: root.snapPx(74); implicitHeight: root.snapPx(20); radius: root.snapPx(3); color: root.controlPressedBg; border.width: root.separatorWidth; border.color: hexInput.activeFocus ? root.dialogAccent : root.controlBorder
                                Layout.preferredHeight: root.snapPx(20)
                                transform: Translate {
                                    x: root.dialogPixelOffsetX(
                                        themeHexInputBox,
                                        themeColorConfigurator.contentItem)
                                    y: root.dialogPixelOffsetY(
                                        themeHexInputBox,
                                        themeColorConfigurator.contentItem)
                                }
                                TextInput {
                                    id: hexInput
                                    anchors.fill: parent; anchors.leftMargin: root.snapPx(4); anchors.rightMargin: root.snapPx(4); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                                    font.family: root.guiMonospaceFontFamily; font.pixelSize: 10; color: root.textColor; selectByMouse: true; selectionColor: root.selectedBg; selectedTextColor: root.textColor
                                    maximumLength: 9
                                    text: !activeFocus && themeColorConfigurator.currentItem ? root.formatColorHex(root[themeColorConfigurator.currentItem.id]) : text
                                    onTextEdited: {
                                        const c = themeColorConfigurator.parseHex(text)
                                        if (c && themeColorConfigurator.currentItem) {
                                            themeColorConfigurator.setFromColor(c)
                                            themeColorConfigurator.applyCurrentColor()
                                        }
                                    }
                                    onEditingFinished: { if (themeColorConfigurator.currentItem) text = root.formatColorHex(root[themeColorConfigurator.currentItem.id]) }
                                }
                            }
                        }
                    }
                }

                Rectangle {
                    id: themeFooterDivider
                    objectName: "themeFooterDivider"
                    Layout.fillWidth: false
                    Layout.preferredWidth: root.snapPx(parent.width)
                    Layout.preferredHeight: root.separatorWidth
                    Layout.minimumHeight: root.separatorWidth
                    Layout.maximumHeight: root.separatorWidth
                    implicitHeight: root.separatorWidth
                    color: root.separatorColor
                    transform: Translate {
                        x: root.dialogPixelOffsetX(
                            themeFooterDivider,
                            themeColorConfigurator.contentItem)
                        y: root.dialogPixelOffsetY(
                            themeFooterDivider,
                            themeColorConfigurator.contentItem)
                    }
                }

                // Footer Actions
                RowLayout {
                    id: themeColorFooter
                    objectName: "themeColorFooter"
                    readonly property bool compactActions:
                        width < root.snapPx(675)
                        || themeColorConfigurator.statusToast !== ""
                    Layout.fillWidth: false
                    Layout.preferredWidth: root.snapPx(parent.width)
                    Layout.preferredHeight: root.snapPx(30)
                    Layout.minimumHeight: root.snapPx(30)
                    Layout.maximumHeight: root.snapPx(30)
                    spacing: root.snapPx(8)
                    transform: Translate {
                        x: root.dialogPixelOffsetX(
                            themeColorFooter,
                            themeColorConfigurator.contentItem)
                        y: root.dialogPixelOffsetY(
                            themeColorFooter,
                            themeColorConfigurator.contentItem)
                    }

                    ConfigDialogButton {
                        id: themeResetElementButton
                        objectName: "themeResetElementButton"
                        transform: Translate {
                            x: root.dialogPixelOffsetX(
                                themeResetElementButton,
                                themeColorConfigurator.contentItem)
                            y: root.dialogPixelOffsetY(
                                themeResetElementButton,
                                themeColorConfigurator.contentItem)
                        }
                        text: themeColorFooter.compactActions
                              ? "" : "Reset Element"
                        toolTipText: "Reset current element to default"
                        iconSource: root.lucideIconSource(
                                        "rotate-ccw", 14, root.textColor)
                        onClicked: {
                            if (themeColorConfigurator.currentItem) {
                                root[themeColorConfigurator.currentItem.id] = Qt.color(themeColorConfigurator.currentItem.defaultColor)
                                themeColorConfigurator.setFromColor(root[themeColorConfigurator.currentItem.id])
                                themeColorConfigurator.flash(themeColorConfigurator.currentItem.id)
                            }
                        }
                    }

                    ConfigDialogButton {
                        id: themeResetAllButton
                        objectName: "themeResetAllButton"
                        transform: Translate {
                            x: root.dialogPixelOffsetX(
                                themeResetAllButton,
                                themeColorConfigurator.contentItem)
                            y: root.dialogPixelOffsetY(
                                themeResetAllButton,
                                themeColorConfigurator.contentItem)
                        }
                        text: themeColorFooter.compactActions
                              ? "" : "Reset All"
                        toolTipText: "Reset all theme settings to defaults"
                        iconSource: root.lucideIconSource(
                                        "refresh-cw", 14, root.textColor)
                        onClicked: {
                            themeColorConfigurator.stopAllFlashing()
                            root.resetThemeToDefaults()
                            if (themeColorConfigurator.currentItem)
                                themeColorConfigurator.setFromColor(root[themeColorConfigurator.currentItem.id])
                            themeColorConfigurator.statusToast = "Reset all colors to default"
                        }
                    }

                    ConfigDialogButton {
                        id: themeRestoreSavedButton
                        objectName: "themeRestoreSavedButton"
                        transform: Translate {
                            x: root.dialogPixelOffsetX(
                                themeRestoreSavedButton,
                                themeColorConfigurator.contentItem)
                            y: root.dialogPixelOffsetY(
                                themeRestoreSavedButton,
                                themeColorConfigurator.contentItem)
                        }
                        text: "Restore"
                        toolTipText: "Restore the last saved theme"
                        iconSource: root.lucideIconSource(
                                        "clock-3", 14, root.textColor)
                        onClicked: {
                            themeColorConfigurator.stopAllFlashing()
                            if (root.loadThemeFromPersistence()) {
                                if (themeColorConfigurator.currentItem) {
                                    themeColorConfigurator.setFromColor(
                                        root[themeColorConfigurator.currentItem.id])
                                }
                                themeColorConfigurator.statusToast =
                                    "Restored saved theme"
                            } else {
                                themeColorConfigurator.statusToast =
                                    "No saved theme found"
                            }
                        }
                    }

                    Text {
                        text: themeColorConfigurator.statusToast
                        color: root.activeBorder
                        font.pixelSize: 11
                        Layout.fillWidth: true
                        Layout.minimumWidth: 0
                        horizontalAlignment: Text.AlignHCenter
                        elide: Text.ElideRight
                        visible: themeColorConfigurator.statusToast !== ""
                    }

                    Item { Layout.fillWidth: true; visible: themeColorConfigurator.statusToast === "" }

                    ConfigDialogButton {
                        id: themeSaveButton
                        objectName: "themeSaveButton"
                        transform: Translate {
                            x: root.dialogPixelOffsetX(
                                themeSaveButton,
                                themeColorConfigurator.contentItem)
                            y: root.dialogPixelOffsetY(
                                themeSaveButton,
                                themeColorConfigurator.contentItem)
                        }
                        text: "Save"
                        toolTipText: "Save the current theme"
                        highlighted: true
                        iconSource: root.lucideIconSource(
                                        "save", 14, "#ffffff")
                        onClicked: {
                            if (root.saveThemeToPersistence()) {
                                themeColorConfigurator.statusToast = "Theme saved to gui_theme.ini!"
                            } else {
                                themeColorConfigurator.statusToast = "Failed to save theme"
                            }
                        }
                    }

                    ConfigDialogButton {
                        id: themeCloseButton
                        objectName: "themeCloseButton"
                        transform: Translate {
                            x: root.dialogPixelOffsetX(
                                themeCloseButton,
                                themeColorConfigurator.contentItem)
                            y: root.dialogPixelOffsetY(
                                themeCloseButton,
                                themeColorConfigurator.contentItem)
                        }
                        text: themeColorFooter.compactActions ? "" : "Close"
                        toolTipText: "Close the theme editor"
                        iconSource: root.lucideIconSource(
                                        "x", 14, root.textColor)
                        onClicked: themeColorConfigurator.close()
                    }
                }
            }
        }
    }

    Item {
        id: semanticLayer
        anchors.fill: parent
        z: 10
        visible: !root.needsFallbackGrid()

        Rectangle {
            anchors.fill: parent
            color: root.useTransparentWindowBackground
                   ? root.windowBackgroundColor : "transparent"
        }

        Item {
            id: titleBar
            objectName: "titleBar"
            anchors.left: parent.left
            anchors.right: parent.right
            height: root.menuBarHeight
            z: 20

            Rectangle {
                id: titleBarBackground
                objectName: "titleBarBackground"
                anchors.fill: parent
                color: root.titleBarBg
                z: -1
            }

            Item {
                id: macSystemButtonArea
                visible: false
                x: root.macSystemButtonAreaLeftMargin
                y: root.titleBarContentVerticalOffset
                width: 70
                height: parent.height
            }

            ZG.Button {
                id: appIcon
                objectName: "appIconButton"
                anchors.left: parent.left
                anchors.verticalCenter: parent.verticalCenter
                implicitWidth: root.snapPx(46)
                implicitHeight: parent.height
                width: visible ? implicitWidth : 0
                height: parent.height
                visible: f4UsesQwk && Qt.platform.os !== "osx"

                leftPadding: 0
                topPadding: 0
                rightPadding: 0
                bottomPadding: 0
                colorfulIcon: true

                contentItem: Item {
                    PixelAlignedImage {
                        objectName: "appIconImage"
                        anchors.centerIn: parent
                        width: root.snapPx(18)
                        height: root.snapPx(18)
                        sourceSize: Qt.size(18, 18)
                        smooth: false
                        mipmap: false
                        source: "qrc:/F4QtHost/icons/app/f4.svg"
                    }
                }

                onClicked: {
                    if (themeColorConfigurator.visible) {
                        themeColorConfigurator.hide()
                    } else {
                        root.showApplicationSettings()
                    }
                }
            }

            SemanticMenuBar {
                id: semanticMenu
                menu: root.menuBarModel
                anchors.left: appIcon.right
                anchors.leftMargin: root.macTitleBarLeftPadding
                anchors.right: workspaceBar.visible
                               ? workspaceBar.left : windowButtons.left
                anchors.rightMargin: workspaceBar.visible
                                     ? 8
                                     : root.useMacNativeTitleBar
                                       ? -windowButtons.width : 0
                height: parent.height
                opacity: root.normalSurfaceOpacity
            }

            Item {
                id: workspaceBar
                objectName: "workspaceBar"
                x: root.snapPx(windowButtons.x - width - (root.useMacNativeTitleBar
                               ? -windowButtons.width + root.contentSpacing
                               : root.contentSpacing))
                anchors.bottom: parent.bottom
                width: visible
                       ? root.snapPx(Math.min(titleBar.width * 0.46,
                                              workspaceItemsRow.width))
                       : 0
                height: root.snapPx(36)
                visible: root.workspaceTabs.visible === true
                opacity: root.normalSurfaceOpacity
                z: 2
                property Item activeWorkspaceTab: null
                property int activeWorkspaceTabSeparatorRevision: 0
                property bool activeWorkspaceTabUpdatePending: false
                property string wheelNavigationModelSignature: ""
                property int wheelNavigationIndex: -1
                property int wheelNavigationAuthoritativeIndex: -1

                function workspaceTabModelSignature(tabs) {
                    var parts = []
                    for (var i = 0; i < tabs.length; ++i) {
                        var tab = tabs[i] || ({})
                        parts.push(root.cleanText(tab.id))
                    }
                    return parts.join("\u001f")
                }

                function authoritativeWorkspaceIndex(tabs) {
                    var activeIndex = Number(root.workspaceTabs.activeIndex)
                    if (Math.floor(activeIndex) === activeIndex
                            && activeIndex >= 0
                            && activeIndex < tabs.length)
                        return activeIndex
                    for (var i = 0; i < tabs.length; ++i) {
                        if (tabs[i] && tabs[i].active === true)
                            return i
                    }
                    return 0
                }

                function activateAdjacentWorkspaceTab(direction) {
                    var tabs = root.workspaces || []
                    if (tabs.length < 2)
                        return false

                    var authoritativeIndex = authoritativeWorkspaceIndex(tabs)
                    var signature = workspaceTabModelSignature(tabs)
                    if (signature !== wheelNavigationModelSignature
                            || wheelNavigationIndex < 0
                            || wheelNavigationIndex >= tabs.length
                            || authoritativeIndex
                               !== wheelNavigationAuthoritativeIndex) {
                        wheelNavigationModelSignature = signature
                        wheelNavigationIndex = authoritativeIndex
                        wheelNavigationAuthoritativeIndex = authoritativeIndex
                    }

                    var nextIndex = wheelNavigationIndex + direction
                    if (nextIndex < 0 || nextIndex >= tabs.length)
                        return true
                    wheelNavigationIndex = nextIndex

                    var tab = tabs[nextIndex] || ({})
                    root.action({
                        "target": root.cleanText(tab.id),
                        "action": root.cleanText(tab.action)
                                  || "workspace.activate",
                        "index": nextIndex
                    }, true)
                    return true
                }

                function refreshActiveWorkspaceTabGeometry() {
                    ++activeWorkspaceTabSeparatorRevision
                }

                function updateActiveWorkspaceTabNow() {
                    var nextTab = null
                    var activeIndex = Number(root.workspaceTabs.activeIndex)
                    if (Math.floor(activeIndex) === activeIndex
                            && activeIndex >= 0
                            && activeIndex < workspaceTabsRepeater.count) {
                        nextTab = workspaceTabsRepeater.itemAt(activeIndex)
                    }
                    if (!nextTab) {
                        for (var i = 0; i < workspaceTabsRepeater.count; ++i) {
                            var candidate = workspaceTabsRepeater.itemAt(i)
                            if (candidate && candidate.current) {
                                nextTab = candidate
                                break
                            }
                        }
                    }
                    activeWorkspaceTab = nextTab
                    refreshActiveWorkspaceTabGeometry()
                }

                function updateActiveWorkspaceTab() {
                    if (activeWorkspaceTabUpdatePending)
                        return
                    activeWorkspaceTabUpdatePending = true
                    Qt.callLater(function() {
                        activeWorkspaceTabUpdatePending = false
                        workspaceBar.updateActiveWorkspaceTabNow()
                    })
                }

                readonly property real activeWorkspaceTabLeft:
                    activeWorkspaceTabSeparatorRevision >= 0
                    && activeWorkspaceTab
                    ? Math.max(0, Math.min(workspaceBar.width,
                          activeWorkspaceTab.x + workspaceItemsRow.x
                          - workspaceFlick.contentX))
                    : 0
                readonly property real activeWorkspaceTabRight:
                    activeWorkspaceTabSeparatorRevision >= 0
                    && activeWorkspaceTab
                    ? Math.max(0, Math.min(workspaceBar.width,
                          activeWorkspaceTab.x + workspaceItemsRow.x
                          - workspaceFlick.contentX
                          + activeWorkspaceTab.width))
                    : 0

                onXChanged: refreshActiveWorkspaceTabGeometry()
                onWidthChanged: refreshActiveWorkspaceTabGeometry()

                MouseArea {
                    id: workspaceTabWheelArea
                    objectName: "workspaceTabWheelArea"
                    anchors.fill: parent
                    acceptedButtons: Qt.NoButton
                    hoverEnabled: false
                    preventStealing: false
                    enabled: workspaceBar.visible
                    onWheel: (wheel) => {
                        var delta = Number(wheel.angleDelta.y)
                        if (delta === 0)
                            delta = Number(wheel.pixelDelta.y)
                        if (!(delta > 0 || delta < 0))
                            return
                        // Wheel up selects the previous tab; wheel down selects
                        // the next one, with the same wraparound as Ctrl+Tab.
                        wheel.accepted = workspaceBar.activateAdjacentWorkspaceTab(
                            delta > 0 ? -1 : 1)
                    }
                }

                Flickable {
                    id: workspaceFlick
                    anchors.fill: parent
                    contentWidth: workspaceItemsRow.width
                    contentHeight: height
                    boundsBehavior: Flickable.StopAtBounds
                    flickableDirection: Flickable.HorizontalFlick
                    pixelAligned: true
                    clip: true

                    function revealCurrentWorkspace() {
                        contentX = Math.max(0, contentWidth - width)
                    }

                    onContentWidthChanged: {
                        revealCurrentWorkspace()
                        workspaceBar.refreshActiveWorkspaceTabGeometry()
                    }
                    onContentXChanged:
                        workspaceBar.refreshActiveWorkspaceTabGeometry()
                    onWidthChanged: {
                        revealCurrentWorkspace()
                        workspaceBar.refreshActiveWorkspaceTabGeometry()
                    }

                    Row {
                        id: workspaceItemsRow
                        width: childrenRect.width
                        height: parent.height
                        spacing: root.snapPx(4)

                        Repeater {
                            id: workspaceTabsRepeater
                            model: root.workspaces
                            onItemAdded: workspaceBar.updateActiveWorkspaceTab()
                            onItemRemoved: workspaceBar.updateActiveWorkspaceTab()

                            delegate: Rectangle {
                                id: workspaceTab
                                readonly property bool current: modelData.active === true
                                readonly property bool closeEnabled:
                                    root.workspaceTabCanClose(modelData)
                                readonly property string tabIconName:
                                    root.workspaceTabIconName(modelData)
                                readonly property string lucideName: tabIconName
                                readonly property color labelColor:
                                    root.workspaceTabTextColor(current)
                                readonly property int labelWeight:
                                    root.workspaceTabFontWeight()
                                readonly property bool hoverActive:
                                    workspaceHover.hovered
                                objectName: String(modelData.id || ("workspace-tab-" + index))
                                width: root.preferredWorkspaceTabWidth(
                                            workspaceLabel.implicitWidth,
                                            workspaceTab.closeEnabled)
                                height: parent.height
                                z: current ? 2 : 0
                                radius: 6
                                topLeftRadius: 6
                                topRightRadius: 6
                                bottomLeftRadius: 0
                                bottomRightRadius: 0
                                antialiasing: true
                                smooth: true
                                color: current
                                       ? root.panelPathBg
                                       : workspaceHover.hovered
                                         ? root.controlHoverBg : "transparent"
                                border.width: 0

                                // The active tab joins the panel below: its
                                // native background keeps the upper corners,
                                // while its outline is deliberately open at
                                // the bottom.
                                Shape {
                                    id: workspaceTabBorder
                                    anchors.fill: parent
                                    visible: workspaceTab.current
                                    z: 1
                                    antialiasing: true
                                    smooth: true
                                    preferredRendererType: Shape.CurveRenderer
                                    readonly property real borderHalf:
                                        root.separatorWidth / 2
                                    readonly property real borderRadius:
                                        6 - borderHalf
                                    readonly property real borderCurveFactor:
                                        borderRadius * 0.5522848

                                    ShapePath {
                                        strokeColor: root.separatorColor
                                        strokeWidth: root.separatorWidth
                                        fillColor: "transparent"
                                        capStyle: ShapePath.FlatCap
                                        joinStyle: ShapePath.RoundJoin
                                        startX: workspaceTabBorder.borderHalf
                                        startY: workspaceTab.height

                                        PathLine {
                                            x: workspaceTabBorder.borderHalf
                                            y: 6
                                        }
                                        PathCubic {
                                            control1X: workspaceTabBorder.borderHalf
                                            control1Y: 6
                                                         - workspaceTabBorder.borderCurveFactor
                                            control2X: 6
                                                         - workspaceTabBorder.borderCurveFactor
                                            control2Y: workspaceTabBorder.borderHalf
                                            x: 6
                                            y: workspaceTabBorder.borderHalf
                                        }
                                        PathLine {
                                            x: workspaceTab.width - 6
                                            y: workspaceTabBorder.borderHalf
                                        }
                                        PathCubic {
                                            control1X: workspaceTab.width - 6
                                                         + workspaceTabBorder.borderCurveFactor
                                            control1Y: workspaceTabBorder.borderHalf
                                            control2X: workspaceTab.width
                                                         - workspaceTabBorder.borderHalf
                                            control2Y: 6
                                                         - workspaceTabBorder.borderCurveFactor
                                            x: workspaceTab.width
                                               - workspaceTabBorder.borderHalf
                                            y: 6
                                        }
                                        PathLine {
                                            x: workspaceTab.width
                                               - workspaceTabBorder.borderHalf
                                            y: workspaceTab.height
                                        }
                                    }
                                }

                                Rectangle {
                                    anchors.left: parent.left
                                    anchors.right: parent.right
                                    anchors.bottom: parent.bottom
                                    height: root.separatorWidth
                                    color: root.separatorColor
                                    visible: !workspaceTab.current
                                    z: 1
                                    antialiasing: false
                                }

                                // Keep the divider in the existing spacing:
                                // this child is painted outside the tab but
                                // never participates in the Row geometry.
                                Rectangle {
                                    objectName: workspaceTab.objectName + "-divider"
                                    width: root.separatorWidth
                                    height: Math.max(0, parent.height - 12)
                                    anchors.left: parent.right
                                    anchors.leftMargin:
                                        (workspaceItemsRow.spacing - width) / 2
                                    anchors.verticalCenter: parent.verticalCenter
                                    color: root.separatorColor
                                    visible: !workspaceTab.current
                                             && !workspaceTab.hoverActive
                                             && index + 1
                                                < workspaceTabsRepeater.count
                                             && workspaceTabsRepeater.itemAt(index + 1)
                                             && !workspaceTabsRepeater.itemAt(
                                                    index + 1).hoverActive
                                             && !workspaceTabsRepeater.itemAt(
                                                    index + 1).current
                                    z: 2
                                }

                                onCurrentChanged:
                                    workspaceBar.updateActiveWorkspaceTab()
                                onWidthChanged:
                                    workspaceBar.refreshActiveWorkspaceTabGeometry()

                                Component.onCompleted: {
                                    workspaceBar.updateActiveWorkspaceTab()
                                    if (f4UsesQwk)
                                        windowAgent.setHitTestVisible(workspaceTab)
                                }

                                HoverHandler {
                                    id: workspaceHover
                                }

                                ZG.ToolTip {
                                    objectName: "workspace-tab-tooltip-"
                                                + workspaceTab.objectName
                                    visible: workspaceHover.hovered
                                    delay: 500
                                    timeout: 5000
                                    text: root.workspaceTabToolTip(
                                              modelData, Qt.platform.os)
                                }

                                PixelAlignedImage {
                                    id: workspaceIcon
                                    objectName: "workspace-tab-icon-"
                                                + root.cleanText(modelData.id)
                                    anchors.left: parent.left
                                    anchors.leftMargin: root.snapPx(10)
                                    y: root.snapPx((parent.height - height) / 2)
                                    width: root.snapPx(16)
                                    height: root.snapPx(16)
                                    alignmentRevision: workspaceTab.x
                                                       + workspaceTab.y
                                                       + workspaceTab.width
                                                       + workspaceTab.height
                                    smooth: false
                                    source: root.lucideIconSource(
                                                workspaceTab.tabIconName, 16,
                                                workspaceTab.labelColor)
                                }

                                Item {
                                    id: workspaceLabel
                                    objectName: "workspace-tab-label-"
                                                + workspaceTab.objectName
                                    anchors.left: workspaceIcon.right
                                    anchors.leftMargin: root.snapPx(7)
                                    anchors.right: workspaceAttention.visible
                                                   ? workspaceAttention.left
                                                   : workspaceClose.visible
                                                     ? workspaceClose.left
                                                     : parent.right
                                    anchors.rightMargin: root.snapPx(6)
                                    y: root.snapPx((parent.height - height) / 2)
                                    height: root.snapPx(
                                                Math.max(
                                                    workspaceNumber.implicitHeight,
                                                    workspaceTitle.implicitHeight))
                                    implicitWidth: workspaceNumber.implicitWidth
                                                   + (workspaceTitle.text === ""
                                                      ? 0 : 5)
                                                   + workspaceTitle.implicitWidth

                                    Text {
                                        id: workspaceTitle
                                        objectName: "workspace-tab-title-"
                                                    + workspaceTab.objectName
                                        anchors.left: parent.left
                                        anchors.right: workspaceNumber.left
                                        anchors.rightMargin: text === "" ||
                                                             workspaceNumber.text === ""
                                                             ? 0 : 5
                                        y: root.snapPx((parent.height - height) / 2)
                                        text: root.cleanText(modelData.text)
                                        color: workspaceTab.labelColor
                                        // Active state is conveyed only by text
                                        // brightness; changing weight makes tabs
                                        // shift visually and breaks typographic
                                        // consistency across the title bar.
                                        font.weight: workspaceTab.labelWeight
                                        elide: Text.ElideMiddle
                                    }

                                    Text {
                                        id: workspaceNumber
                                        objectName: "workspace-tab-number-"
                                                    + workspaceTab.objectName
                                        anchors.right: parent.right
                                        y: root.snapPx((parent.height - height) / 2)
                                        text: Number(modelData.number || 0) > 0
                                              ? String(modelData.number) : ""
                                        color: root.workspaceTabNumberColor()
                                        opacity: workspaceTab.current ? 0.9 : 0.76
                                        font.weight: workspaceTab.labelWeight
                                    }
                                }

                                Rectangle {
                                    id: workspaceAttention
                                    anchors.right: parent.right
                                    anchors.rightMargin: workspaceClose.visible ? root.snapPx(29) : root.snapPx(10)
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: root.snapPx(6)
                                    height: root.snapPx(6)
                                    radius: 3
                                    color: root.dialogAccent
                                    visible: modelData.attention === true
                                }

                                PixelAlignedImage {
                                    id: workspaceClose
                                    objectName: "workspace-close-"
                                                + root.cleanText(modelData.id)
                                    z: 2
                                    x: root.snapPx(parent.width - width - 8)
                                    y: root.snapPx((parent.height - height) / 2)
                                    width: root.snapPx(14)
                                    height: root.snapPx(14)
                                    alignmentRevision: workspaceTab.x
                                                       + workspaceTab.y
                                                       + workspaceTab.width
                                                       + workspaceTab.height
                                    smooth: false
                                    source: root.lucideIconSource(
                                                "x", 14,
                                                workspaceTab.labelColor)
                                    visible: workspaceTab.closeEnabled

                                    MouseArea {
                                        anchors.fill: parent
                                        anchors.margins: -6
                                        cursorShape: Qt.PointingHandCursor
                                        onClicked: function(mouse) {
                                            mouse.accepted = true
                                            root.action({
                                                "target": modelData.id,
                                                "action": modelData.closeAction,
                                                "index": modelData.index
                                            }, true)
                                        }
                                    }
                                }

                                Rectangle {
                                    anchors.left: parent.left
                                    anchors.leftMargin: 6
                                    anchors.bottom: parent.bottom
                                    anchors.bottomMargin: 2
                                    width: Math.max(0, parent.width - 12)
                                           * Math.max(0, Math.min(100,
                                               Number(modelData.progress))) / 100
                                    height: 2
                                    radius: 1
                                    color: root.dialogAccent
                                    visible: Number(modelData.progress) >= 0
                                }

                                MouseArea {
                                    anchors.fill: parent
                                    cursorShape: Qt.PointingHandCursor
                                    onClicked: root.action({
                                        "target": modelData.id,
                                        "action": modelData.action,
                                        "index": modelData.index
                                    }, true)
                                }
                            }
                        }

                        Rectangle {
                            id: workspaceNew
                            readonly property bool qwkHitTestRegistered:
                                !f4UsesQwk || workspaceNewHitTestRegistered
                            property bool workspaceNewHitTestRegistered: false
                            objectName: root.cleanText(root.workspaceTabs.newTab
                                                       ? root.workspaceTabs.newTab.id : "workspace-new")
                            width: visible ? root.snapPx(30) : 0
                            height: parent.height
                            radius: 0
                            topLeftRadius: 6
                            topRightRadius: 6
                            bottomLeftRadius: 0
                            bottomRightRadius: 0
                            antialiasing: true
                            smooth: true
                            color: newHover.hovered ? root.controlHoverBg : "transparent"
                            visible: !!root.workspaceTabs.newTab
                                     && root.workspaceTabs.newTab.visible === true
                            Component.onCompleted: {
                                if (f4UsesQwk) {
                                    windowAgent.setHitTestVisible(workspaceNew)
                                    workspaceNewHitTestRegistered = true
                                }
                            }

                            Rectangle {
                                anchors.left: parent.left
                                anchors.right: parent.right
                                anchors.bottom: parent.bottom
                                height: root.separatorWidth
                                color: root.separatorColor
                                z: 1
                                antialiasing: false
                            }

                            PixelAlignedImage {
                                x: root.snapPx((parent.width - width) / 2)
                                y: root.snapPx((parent.height - height) / 2)
                                width: root.snapPx(16)
                                height: root.snapPx(16)
                                alignmentRevision: workspaceNew.x
                                                   + workspaceNew.y
                                                   + workspaceNew.width
                                                   + workspaceNew.height
                                smooth: false
                                source: root.lucideIconSource(
                                            "plus", 16, root.chromeText)
                            }
                            HoverHandler { id: newHover }
                            MouseArea {
                                anchors.fill: parent
                                cursorShape: Qt.PointingHandCursor
                                onClicked: root.action({
                                    "target": root.workspaceTabs.newTab.id,
                                    "action": root.workspaceTabs.newTab.action
                                }, true)
                            }
                        }

                    }
                }

                Rectangle {
                    id: workspaceTabSeparatorLeft
                    objectName: "workspaceTabSeparatorLeft"
                    anchors.left: parent.left
                    anchors.bottom: parent.bottom
                    width: workspaceBar.activeWorkspaceTabLeft
                    height: root.separatorWidth
                    color: root.separatorColor
                    visible: workspaceBar.visible
                             && workspaceBar.activeWorkspaceTab !== null
                    z: -1
                    antialiasing: false
                }

                Rectangle {
                    id: workspaceTabSeparatorRight
                    objectName: "workspaceTabSeparatorRight"
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    width: Math.max(0, parent.width
                                       - workspaceBar.activeWorkspaceTabRight)
                    height: root.separatorWidth
                    color: root.separatorColor
                    visible: workspaceBar.visible
                             && workspaceBar.activeWorkspaceTab !== null
                    z: -1
                    antialiasing: false
                }
            }

            Rectangle {
                id: workspaceSeparatorLeft
                objectName: "workspaceSeparatorLeft"
                anchors.left: parent.left
                anchors.right: workspaceBar.left
                anchors.bottom: parent.bottom
                height: root.separatorWidth
                color: root.separatorColor
                visible: workspaceBar.visible
                z: 1
                antialiasing: false
            }

            Rectangle {
                id: workspaceSeparatorRight
                objectName: "workspaceSeparatorRight"
                anchors.left: workspaceBar.right
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                height: root.separatorWidth
                color: root.separatorColor
                visible: workspaceBar.visible
                z: 1
                antialiasing: false
            }

            Row {
                id: windowButtons
                objectName: "windowButtons"
                anchors.top: parent.top
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                spacing: 0
                height: parent.height
                visible: f4UsesQwk && !root.useMacNativeTitleBar

                ZG.TitleButton {
                    id: minimizeButton
                    objectName: "minimizeButton"

                    height: parent.height
                    opacity: root.active || minimizeButton.hoveredOverride ? 1 : 0.4

                    source: "qrc:/ZoinGallery/resources/WindowMinimize.svg"
                    onClicked: root.showMinimized()

                    Component.onCompleted: {
                        if (!root.useMacNativeTitleBar) {
                            windowAgent.setSystemButton(WindowAgent.Minimize,
                                                        minimizeButton)
                        }
                    }
                }

                ZG.TitleButton {
                    id: maximizeButton
                    objectName: "maximizeButton"

                    height: parent.height
                    opacity: root.active || maximizeButton.hoveredOverride ? 1 : 0.4

                    source: root.visibility === Window.Maximized
                            ? "qrc:/ZoinGallery/resources/WindowRestore.svg"
                            : root.visibility === Window.FullScreen
                              ? "qrc:/ZoinGallery/resources/WindowFullscreen.svg"
                              : "qrc:/ZoinGallery/resources/WindowMaximize.svg"
                    onClicked: {
                        if (root.visibility === Window.FullScreen) {
                            root.toggleFullscreen()
                        } else if (root.visibility === Window.Maximized) {
                            root.showNormal()
                        } else {
                            root.showMaximized()
                        }
                    }

                    Component.onCompleted: {
                        if (!root.useMacNativeTitleBar) {
                            windowAgent.setSystemButton(WindowAgent.Maximize,
                                                        maximizeButton)
                        }
                    }
                }

                ZG.TitleButton {
                    id: closeButton
                    objectName: "closeButton"

                    height: parent.height
                    opacity: root.active || closeButton.hoveredOverride ? 1 : 0.4

                    source: "qrc:/ZoinGallery/resources/WindowClose.svg"
                    icon.color: closeButton.hovered
                                ? ZG.Style.closeButtonHoveredIcon
                                : ZG.Style.text
                    backgroundColor: {
                        if (!closeButton.enabled) {
                            return "gray"
                        }
                        if (closeButton.pressed) {
                            return ZG.Style.closeButtonPressed
                        }
                        if (closeButton.hovered) {
                            return ZG.Style.closeButtonHovered
                        }
                        return "transparent"
                    }
                    onClicked: root.close()

                    Component.onCompleted: {
                        if (!root.useMacNativeTitleBar) {
                            windowAgent.setSystemButton(WindowAgent.Close,
                                                        closeButton)
                        }
                    }
                }
            }
        }

        Item {
            id: mainSurface
            anchors.fill: parent
            opacity: root.normalSurfaceOpacity

            Loader {
                id: persistentPanelsLayer
                objectName: "persistentPanelsLayer"
                anchors.fill: parent
                // The initial Qt object graph is built before the core can
                // publish a semantic scene.  Creating two complete Gallery
                // panels for that empty placeholder dominated engine.load().
                // Instantiate the persistent pair only when a real shell has
                // arrived; once created it remains alive across every cover.
                active: root.retainedShellSurfaceCreated
                // Keep the native panel tree logically visible underneath a
                // standalone document. Flipping visibility on a Gallery with
                // thousands of rows synchronously walks its complete object
                // tree. Opacity zero lets the scene graph prune it while the
                // document owns input, without charging that walk to F3/F4.
                visible: !root.hasOperationsQueueSurface()
                opacity: root.hasStandaloneDocumentSurface() ? 0 : 1
                sourceComponent: panelsSurface
            }

            Loader {
                id: persistentDocumentLayer
                objectName: "persistentDocumentLayer"
                anchors.fill: parent
                // DocumentSurface is a sizeable reusable object tree. Build
                // it once, just after the first Commander shell has settled,
                // instead of charging that construction to the first F3/F4
                // key press. It remains hidden and inert until a document is
                // actually current.
                active: root.retainedDocumentSurfaceCreated
                        || root.documentSurfacePrewarmed
                visible: root.hasStandaloneDocumentSurface()
                sourceComponent: documentSurface
                z: 10
            }

            Timer {
                interval: 0
                running: root.retainedShellSurfaceCreated
                         && !root.documentSurfacePrewarmed
                onTriggered: root.documentSurfacePrewarmed = true
            }

            Loader {
                id: operationsQueueLayer
                objectName: "operationsQueueLayer"
                anchors.fill: parent
                active: root.retainedOperationsQueueCreated
                visible: root.hasOperationsQueueSurface()
                sourceComponent: operationsQueueSurface
                z: 20
            }
        }

        Loader {
            id: galleryViewerLayer
            anchors.fill: parent
            active: qtGallery.viewerVisible && !root.hasDocumentSurface()
                    && !root.needsFallbackGrid()
            visible: active && !root.hasOperationsQueueSurface()
            sourceComponent: active ? galleryViewerSurface : undefined
            // The integrated viewer owns the complete content area, including
            // the normal menu/key chrome. Commander overlays remain above it.
            z: 60
        }

        Component {
            id: panelsSurface
            Item {
                id: panelsRoot
                anchors.fill: parent
                property var frame: root.shellFrame()
                property var panelList: frame.panels || []

                function panelForSide(side) {
                    const compactPanel = side === 0
                            ? root.leftPanelPresentationOverride
                            : root.rightPanelPresentationOverride
                    if (compactPanel !== null)
                        return compactPanel
                    for (var i = 0; i < panelList.length; ++i) {
                        if (Number(panelList[i].side) === side)
                            return panelList[i]
                    }
                    return ({ "side": side })
                }

                function hasPanelForSide(side) {
                    for (var i = 0; i < panelList.length; ++i) {
                        if (Number(panelList[i].side) === side)
                            return true
                    }
                    return false
                }

                function infoPanelForSide(side) {
                    return root.infoPanelForSide(side)
                }

                function quickViewForSide(side) {
                    return root.quickViewForSide(side)
                }

                function altPanelForSide(side) {
                    return infoPanelForSide(side)
                            || quickViewForSide(side)
                }

                // The terminal is the single persistent surface underneath the
                // commander UI. Panels merely cover it; hiding or shortening a
                // panel reveals the same history that is visible after Ctrl+O.
                TerminalBackdrop {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.top: parent.top
                    anchors.topMargin: semanticMenu.height
                    anchors.bottom: parent.bottom
                    anchors.bottomMargin: root.keyBarHeight()
                                          + root.commandLineHeight(panelsRoot.frame)
                    // Wide is a layout choice, not a hidden-panel state.  In
                    // that mode panelSideVisible() intentionally reports the
                    // passive side as hidden, but the active panel occupies
                    // the whole surface and must not reveal the terminal
                    // through its translucent panel background.
                    visible: panelsRoot.frame.terminalActive === true
                             || (root.widePanelSide() < 0
                                 && (panelsRoot.frame.showLeftPanel === false
                                     || panelsRoot.frame.showRightPanel === false))
                    terminal: panelsRoot.frame.terminal || ({})
                }

                Loader {
                    id: persistentPanelPair
                    objectName: "persistentPanelPair"
                    anchors.fill: parent
                    // Ctrl+O is a presentation-only cover change.  Destroying
                    // this Loader used to recreate both FilePanelView trees on
                    // return, which in turn rebuilt their Gallery viewports
                    // and revealed the cursor from the initial scroll position.
                    // Keep the pair (and both panels' local scroll state) alive
                    // underneath the terminal, exactly like the terminal is
                    // kept alive underneath the panels.
                    active: true
                    visible: frame.terminalActive !== true
                    sourceComponent: panelPairSurface
                }

                CommandLineView {
                    commandLine: root.commandLineFrame()
                }

                Component {
                    id: panelPairSurface
                    Item {
                        id: panelPairRoot
                        anchors.fill: parent

                        // Optional panel surfaces are expensive but normally
                        // absent.  Create each one on first use, then retain it
                        // so Ctrl+Q/Info toggles still preserve native state.
                        property bool leftInfoCreated: false
                        property bool rightInfoCreated: false
                        property bool leftQuickViewCreated: false
                        property bool rightQuickViewCreated: false

                        function retainOptionalPanels() {
                            if (panelsRoot.infoPanelForSide(0) !== null)
                                leftInfoCreated = true
                            if (panelsRoot.infoPanelForSide(1) !== null)
                                rightInfoCreated = true
                            if (panelsRoot.quickViewForSide(0) !== null)
                                leftQuickViewCreated = true
                            if (panelsRoot.quickViewForSide(1) !== null)
                                rightQuickViewCreated = true
                        }

                        Component.onCompleted: retainOptionalPanels()

                        Connections {
                            target: panelsRoot
                            function onFrameChanged() {
                                panelPairRoot.retainOptionalPanels()
                            }
                        }

                        FilePanelView {
                            panel: panelsRoot.panelForSide(0)
                            layoutState: root.leftPanelLayoutStateOverride
                            visible: root.panelSideVisible(0)
                                      && !panelsRoot.altPanelForSide(0)
                        }

                        FilePanelView {
                            panel: panelsRoot.panelForSide(1)
                            layoutState: root.rightPanelLayoutStateOverride
                            visible: root.panelSideVisible(1)
                                      && !panelsRoot.altPanelForSide(1)
                        }

                        Loader {
                            active: panelPairRoot.leftInfoCreated
                            sourceComponent: Component {
                                InfoPanelView {
                                    panel: panelsRoot.infoPanelForSide(0)
                                           || ({ "side": 0 })
                                    visible: root.panelSideVisible(0)
                                             && panelsRoot.infoPanelForSide(0) !== null
                                }
                            }
                        }

                        Loader {
                            active: panelPairRoot.rightInfoCreated
                            sourceComponent: Component {
                                InfoPanelView {
                                    panel: panelsRoot.infoPanelForSide(1)
                                           || ({ "side": 1 })
                                    visible: root.panelSideVisible(1)
                                             && panelsRoot.infoPanelForSide(1) !== null
                                }
                            }
                        }

                        // Keep two native Quick View hosts alive with the
                        // persistent panel pair. Ctrl+Q therefore only covers
                        // and uncovers the corresponding FilePanelView; its
                        // Gallery host, scroll position and thumbnail delegates
                        // retain object identity.
                        Loader {
                            active: panelPairRoot.leftQuickViewCreated
                            sourceComponent: Component {
                                QuickViewPanelView {
                                    quickView: panelsRoot.quickViewForSide(0)
                                               || ({ "side": 0 })
                                    visible: root.panelSideVisible(0)
                                             && panelsRoot.quickViewForSide(0) !== null
                                }
                            }
                        }

                        Loader {
                            active: panelPairRoot.rightQuickViewCreated
                            sourceComponent: Component {
                                QuickViewPanelView {
                                    quickView: panelsRoot.quickViewForSide(1)
                                               || ({ "side": 1 })
                                    visible: root.panelSideVisible(1)
                                             && panelsRoot.quickViewForSide(1) !== null
                                }
                            }
                        }

                        PanelSplitter {
                            id: panelSplitter
                            objectName: "mainPanelSplitter"
                            y: semanticMenu.height
                            height: Math.max(1, parent.height - y
                                             - root.commandLineHeight(panelsRoot.frame)
                                             - root.keyBarHeight())
                            availableWidth: parent.width
                            minimumPanelWidth: root.panelMinimumWidth
                            ratio: root.panelSplitRatio
                            // Double-clicking the divider is an explicit local
                            // reset to equal panels. The semantic ratio may be
                            // non-default because of a prior Ctrl+Left/Right
                            // adjustment and must not become the reset target.
                            defaultRatio: 0.5
                            keySink: grid
                            surfaceActive: root.nativeTwoPanelSurfaceActive
                                           && root.widePanelSide() < 0
                                           && panelsRoot.hasPanelForSide(0)
                                           && panelsRoot.hasPanelForSide(1)
                            surfaceVisible: root.nativeTwoPanelSurfaceVisible
                                            && root.widePanelSide() < 0
                                            && panelsRoot.hasPanelForSide(0)
                                            && panelsRoot.hasPanelForSide(1)
                            hoverLineColor: root.separatorHoverColor
                            activeLineColor: root.separatorActiveColor
                            // The two panels already paint the gutter with the
                            // same surface color. Keeping the splitter itself
                            // transparent lets every horizontal separator run
                            // uninterrupted beneath its center line.
                            trackColor: "transparent"
                            separatorColor: root.separatorColor
                            separatorWidth: root.separatorWidth
                            gutterWidth: root.panelContentSpacing * 2
                            // The left panel's Gallery scrollbar overlaps
                            // this gutter by panelContentSpacing (it anchors
                            // to its own panel's right edge with a matching
                            // negative margin so it sits flush with the
                            // divider). Leave that lane to the scrollbar so
                            // dragging it does not instead start a resize.
                            leadingHitInset: root.panelContentSpacing
                            z: 10
                            onRatioRequested: (nextRatio) => {
                                root.panelSplitRatio = nextRatio
                            }
                            onFocusReleaseRequested: {
                                Qt.callLater(root.restoreSurfaceFocus)
                            }
                        }
                    }
                }

            }
        }

        Component {
            id: documentSurface
            DocumentSurface {
                frame: root.currentDocumentFrame()
                       || root.retainedDocumentFrame || ({})
                interactionActive: root.hasStandaloneDocumentSurface()
            }
        }

        Component {
            id: operationsQueueSurface
            OperationsQueueSurface {
                queue: root.operationsQueueFrame()
                interactionActive: root.hasOperationsQueueSurface()
            }
        }

        Component {
            id: galleryViewerSurface
            Loader {
                anchors.fill: parent
                source: qtGallery.viewerComponentUrl
                onLoaded: {
                    if (!item)
                        return
                    item.session = Qt.binding(() => qtGallery.viewerSession)
                    item.sourcePanel = Qt.binding(
                        () => root.galleryPanelHost(qtGallery.viewerSide))
                    item.bridge = qtGallery
                    item.keySink = grid
                    item.theme = Qt.binding(function() {
                        return root.galleryTheme()
                    })
                    item.surfaceActive = Qt.binding(
                        () => qtGallery.viewerVisible
                              && !root.hasBlockingOverlay()
                              && !root.hasDocumentSurface()
                              && !root.hasOperationsQueueSurface()
                              && !root.needsFallbackGrid())
                    item.devicePixelRatio = Qt.binding(
                        () => root.screen ? root.screen.devicePixelRatio : 1.0)
                    item.forceActiveFocus()
                }
            }
        }

        Repeater {
            id: overlayRepeater
            model: root.overlayFrames()
            delegate: Loader {
                id: overlayLoader
                property var frame: modelData || ({})
                function bindFrame() {
                    if (item && item.frame !== undefined) {
                        item.frame = Qt.binding(function() {
                            return overlayLoader.frame
                        })
                    }
                }
                anchors.fill: parent
                // Repeater already contains only supported overlay frames.
                // Keeping Loader activation conditional on the freshly
                // rebound QVariantMap can leave it permanently inactive when
                // a document menu is created during the same scene update.
                active: true
                sourceComponent: frame.kind === "menu"
                                 ? (frame.role === "autocomplete" ? autocompletePopupComponent : menuPopupComponent)
                                 : dialogOverlayComponent
                onLoaded: bindFrame()
                z: 100 + index
            }
        }

        KeyBarView {
            keyBar: root.keyBarModel
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: root.keyBarHeight()
            opacity: root.normalSurfaceOpacity
            z: 40
        }

        ToastView {
            toast: root.toastModel
            anchors.horizontalCenter: parent.horizontalCenter
            y: semanticMenu.height + 8
            opacity: root.normalSurfaceOpacity
            z: 200
        }
    }

    component SemanticMenuBar: Rectangle {
        id: menuBarRoot
        property var menu: ({})
        property int pointerHoverIndex: -1
        readonly property int effectiveSelected:
            root.menuBarPreviewIndex >= 0
            ? root.menuBarPreviewIndex : Number(menu.selected || 0)
        color: "transparent"
        visible: menu.items !== undefined

        Timer {
            id: menuBarHoverSyncTimer
            interval: 12
            onTriggered: {
                var preview = root.menuBarPreviewIndex
                if (menu.active !== true || preview < 0
                        || Number(menu.selected || 0) === preview)
                    return
                // The preview has already been painted locally. Synchronize
                // Go on the next event-loop turn so IPC and scene generation
                // never delay the menu-bar hover frame.
                root.action({
                    "action": "menuBar.activate",
                    "index": preview
                }, true)
            }
        }

        onMenuChanged: {
            if (menu.active !== true) {
                menuBarHoverSyncTimer.stop()
                root.menuBarPreviewIndex = -1
                root.menuBarOpenedByPointer = false
                root.menuBarPointerHasSelectedItem = false
                root.clearMenuPointerSelection()
                return
            }

            var selected = Number(menu.selected || 0)
            if (root.menuBarPreviewIndex === selected) {
                // Go acknowledged the exact item already shown locally. Drop
                // the preview without changing a pixel so later Left/Right
                // scenes can become authoritative.
                menuBarHoverSyncTimer.stop()
                root.menuBarPreviewIndex = -1
            }
            if (root.menuPointerMenuIndex >= 0
                    && root.menuPointerMenuIndex !== selected
                    && root.menuPointerMenuIndex
                       !== root.menuBarPreviewIndex)
                root.clearMenuPointerSelection()
        }

        function activateItem(item, hoverOnly) {
            if (!item || item.disabled === true)
                return true
            if (hoverOnly) {
                if (menu.active === true
                        && item.index !== effectiveSelected) {
                    // Paint first; the short single-shot lets QML process the
                    // local state and coalesces rapid passes over adjacent
                    // top-level items before notifying Go.
                    root.clearMenuPointerSelection()
                    root.menuBarPointerHasSelectedItem = false
                    root.menuBarPreviewIndex = item.index
                    menuBarHoverSyncTimer.restart()
                }
            } else {
                menuBarHoverSyncTimer.stop()
                var closing = menu.active === true
                              && item.index === effectiveSelected
                root.clearMenuPointerSelection()
                root.menuBarOpenedByPointer = !closing
                root.menuBarPointerHasSelectedItem = false
                root.menuBarPreviewIndex = closing ? -1 : item.index
                root.action({
                    "action": "menuBar.toggle",
                    "index": item.index
                }, true)
            }
            return true
        }

        function activateAt(localX, hoverOnly) {
            for (var i = 0; i < menuItemRepeater.count; ++i) {
                var visualItem = menuItemRepeater.itemAt(i)
                if (!visualItem)
                    continue
                var x1 = visualItem.mapToItem(menuBarRoot, 0, 0).x
                var x2 = x1 + visualItem.width
                if (localX >= x1 && localX < x2) {
                    pointerHoverIndex = visualItem.menuIndex
                    return activateItem(visualItem.menuData, hoverOnly)
                }
            }
            pointerHoverIndex = -1
            return false
        }

        function itemWindowX(menuIndex) {
            for (var i = 0; i < menuItemRepeater.count; ++i) {
                var visualItem = menuItemRepeater.itemAt(i)
                if (visualItem
                        && Number(visualItem.menuIndex) === Number(menuIndex))
                    return visualItem.mapToItem(semanticLayer, 0, 0).x
            }
            var fallbackItem = root.menuBarItem(menuIndex)
            return mapToItem(semanticLayer,
                             fallbackItem ? root.pxX(fallbackItem.x) : 0,
                             0).x
        }

        function windowBottom() {
            return mapToItem(semanticLayer, 0, height).y
        }

        Row {
            id: menuItemsRow
            anchors.left: parent.left
            anchors.top: parent.top
            anchors.bottom: parent.bottom
            spacing: 2

            Repeater {
                id: menuItemRepeater
                model: menu.items || []
                delegate: Item {
                    id: menuItemHitTarget
                    readonly property var menuData: modelData
                    readonly property int menuIndex: Number(modelData.index)
                    height: parent.height
                    width: label.implicitWidth
                           + root.menuItemHorizontalPadding * 2

                    Component.onCompleted: {
                        if (f4UsesQwk)
                            windowAgent.setHitTestVisible(menuItemHitTarget)
                    }

                    Rectangle {
                        anchors.fill: parent
                        anchors.leftMargin: 2
                        anchors.rightMargin: 2
                        anchors.topMargin: 3
                                           + root.titleBarContentVerticalOffset
                        anchors.bottomMargin: 3
                                              - root.titleBarContentVerticalOffset
                        radius: 5
                        color: modelData.index
                               === menuBarRoot.effectiveSelected
                               && menu.active
                               ? root.selectedBg
                               : modelData.index
                                 === menuBarRoot.pointerHoverIndex
                                 ? root.panelSelectionBg : "transparent"
                    }

                    Text {
                        id: label
                        anchors.centerIn: parent
                        anchors.verticalCenterOffset:
                            root.titleBarContentVerticalOffset
                        text: root.mnemonicText(modelData.text,
                                                modelData.hotkey)
                        textFormat: Text.StyledText
                        color: modelData.disabled ? root.mutedText : root.chromeText
                    }
                }
            }
        }

        MouseArea {
            anchors.fill: parent
            acceptedButtons: Qt.LeftButton
            hoverEnabled: true
            onClicked: (mouse) => {
                if (!parent.activateAt(mouse.x, false) && menu.active === true) {
                    menuBarHoverSyncTimer.stop()
                    root.menuBarPreviewIndex = -1
                    root.clearMenuPointerSelection()
                    root.action({
                        "action": "menuBar.toggle",
                        "index": menu.selected
                    }, true)
                }
            }
            onPositionChanged: (mouse) => parent.activateAt(mouse.x, true)
            onExited: parent.pointerHoverIndex = -1
        }
    }

    component FilePanelView: Rectangle {
        id: panelRoot
        objectName: "filePanel-" + Number(panel.side || 0)
        property var panel: ({})
        property var layoutState: null
        readonly property bool layoutStateMatchesPanel:
            layoutState !== null
            && String(layoutState.id || "") === String(panel.id || "")
            && Number(layoutState.catalogRevision || 0)
               === Number(panel.catalogRevision || 0)
        readonly property string effectiveGalleryLayoutMode:
            root.cleanText(layoutStateMatchesPanel
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
        property bool nativeLayout: root.isAppScene()
        property real topChromeOffset: nativeLayout ? 0 : ((panel.y || 0) <= 0 ? semanticMenu.height : 0)
        readonly property bool panelIsActive:
            root.panelIsEffectivelyActive(panel)
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
            return qtGallery.available
        }

        function rendererChoiceActive(choice) {
            if (!choice || choice.heading === true)
                return false
            if (choice.wideToggle === true)
                return root.widePanelSide() === Number(panel.side || 0)
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
                    return root.cleanText(choice.icon)
            }
            return "layout-dashboard"
        }

        function sortModeName() {
            const mode = root.cleanText(panel.sortModeName).toLowerCase()
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
            root.action({
                "action": "panel.sort",
                "side": panel.side,
                "mode": choice.mode
            })
            sortMenu.close()
        }

        function chooseRenderer(choice) {
            if (!rendererChoiceEnabled(choice))
                return
            if (choice.wideToggle === true) {
                root.action({
                    "action": "panel.setWide",
                    "side": panel.side,
                    "enabled": root.widePanelSide()
                               !== Number(panel.side || 0)
                }, true)
            } else {
                qtGallery.requestGalleryLayout(
                            panel.side, choice.layoutMode,
                            Number(choice.columnCount || 0))
            }
            rendererMenu.close()
        }

        function galleryHost() {
            return galleryPanelContent.item
        }

        function updateRegisteredGalleryPanelHost() {
            var nextHost = galleryPanelContent.item
            if (registeredGalleryPanelHost
                    && registeredGalleryPanelHost !== nextHost) {
                root.clearGalleryPanelHost(panel.side,
                                           registeredGalleryPanelHost)
            }
            registeredGalleryPanelHost = nextHost
            if (nextHost)
                root.setGalleryPanelHost(panel.side, nextHost)
        }

        Component.onDestruction: {
            if (registeredGalleryPanelHost) {
                root.clearGalleryPanelHost(panel.side,
                                           registeredGalleryPanelHost)
            }
        }

        readonly property real nativeSplitPosition: root.nativePanelSplitPosition()

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
           ? root.nativePanelX(Number(panel.side || 0))
           : root.pxX(panel.x)
        y: nativeLayout ? semanticMenu.height : root.pxY(panel.y) + topChromeOffset
        width: nativeLayout
               ? root.nativePanelWidth(Number(panel.side || 0))
               : root.pxW(panel.w)
        height: nativeLayout ? Math.max(1, root.height - semanticMenu.height - root.commandLineHeight(root.shellFrame()) - root.keyBarHeight()) : Math.max(1, root.pxH(panel.h) - topChromeOffset)
        color: "transparent"
        border.width: 0
        clip: true

        Rectangle {
            id: panelHeader
            objectName: "panelHeader-" + Number(panel.side || 0)
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: Math.max(25, root.ch * 1.25)
                    + root.verticalContentSpacing
                    + root.pathRowExtraHeight
            // Keep the panel header color as a translucent foreground over
            // the same chrome surface used by the title bar.
            color: root.titleBarBg
            z: 2

            Rectangle {
                id: panelHeaderPanelBackground
                objectName: "panelHeaderPanelBackground-"
                            + Number(panel.side || 0)
                anchors.fill: parent
                color: root.panelPathBg
                z: 0
            }

            Rectangle {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                height: root.separatorWidth
                color: root.separatorColor
            }

            Item {
                id: panelPathArea
                anchors.left: parent.left
                anchors.right: sortButton.left
                anchors.verticalCenter: parent.verticalCenter
                anchors.leftMargin: root.panelTextInset
                anchors.rightMargin: root.panelTextInset
                height: Math.min(parent.height - 4, 32)
                clip: true

                ZG.Button {
                    id: panelDriveButton
                    objectName: "panelDriveButton-" + Number(panel.side || 0)
                    anchors.left: parent.left
                    anchors.top: parent.top
                    anchors.bottom: parent.bottom
                    width: root.snapPx(24)
                    leftPadding: 0
                    topPadding: 0
                    rightPadding: 0
                    bottomPadding: 0
                    focusPolicy: Qt.NoFocus
                    hoverEnabled: true

                    contentItem: Item {
                        PixelAlignedImage {
                            id: panelDriveButtonIcon
                            objectName: "panelDriveButtonIcon-"
                                        + Number(panel.side || 0)
                            anchors.centerIn: parent
                            width: panelPathControl.driveIconSize
                            height: panelPathControl.driveIconSize
                            smooth: false
                            mipmap: false
                            alignmentRevision: panelDriveButton.x
                                               + panelDriveButton.y
                                               + panelDriveButton.width
                                               + panelDriveButton.height
                                               + panelPathArea.width
                                               + panelPathArea.height
                            source: panelPathControl.currentDriveIconSource
                        }
                    }

                    background: Rectangle {
                        radius: 4
                        color: panelDriveButton.down
                               ? root.galleryPathItemPressedColor
                               : panelDriveButton.hovered
                                 ? root.galleryPathHoverColor : "transparent"
                    }

                    onClicked: root.action({
                        "action": "panel.driveMenu",
                        "side": Number(panel.side || 0)
                    })
                }

                ZG.PathControl {
                    id: panelPathControl
                    objectName: "panelPathTitle-" + Number(panel.side || 0)
                    anchors.left: panelDriveButton.right
                    anchors.leftMargin: root.snapPx(4)
                    anchors.top: parent.top
                    anchors.bottom: parent.bottom
                    anchors.right: parent.right
                    anchors.rightMargin: panelRoot.loadingIndicatorVisible ? 18 : 0
                    backgroundOnHoverOnly: true
                    // panelPathArea already begins on the shared 16 px
                    // content line; do not add the standalone control's inset.
                    leadingInset: 0
                    showDriveIcon: false
                    breadcrumbFontPixelSize: root.semanticTextFontPixelSize
                    pathBackgroundColor: root.galleryPathBackgroundColor
                    pathTextColor: root.galleryPathTextColor
                    pathHoveredColor: root.galleryPathHoverColor
                    pathItemHoveredColor: root.galleryPathItemHoverColor
                    pathItemPressedColor: root.galleryPathItemPressedColor
                    devicePixelRatio: root.iconDevicePixelRatio
                    alignmentRevision: panelRoot.x + panelRoot.y
                                       + panelRoot.width + panelRoot.height
                                       + root.panelSplitRatio
                    breadcrumbSeparatorIconSource:
                        root.lucideIconSource(
                            "chevron-right", 12, root.galleryPathTextColor)
                    localDriveIconSource:
                        root.lucideIconSource(
                            "hard-drive", 18, root.galleryPathTextColor)
                    networkDriveIconSource:
                        root.lucideIconSource(
                            "network", 18, root.galleryPathTextColor)
                    text: root.cleanText(panel.title || panel.path)
                    navigationPath: String(panel.path || "")
                    navigationHandler: function(path) {
                        root.action({
                            "action": "panel.navigatePath",
                            "side": Number(panel.side || 0),
                            "path": String(path)
                        })
                    }
                    onEditModeChanged: {
                        if (!editMode)
                            grid.forceActiveFocus()
                    }
                }

                Text {
                    id: panelLoadingIndicator
                    objectName: "panelLoadingIndicator-"
                                + Number(panel.side || 0)
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    visible: panelRoot.loadingIndicatorVisible
                    text: panelRoot.loadingIndicatorFrames[
                              panelRoot.loadingIndicatorFrame]
                    color: root.mutedText
                    font.pixelSize: 13
                }
            }

            ToolButton {
                id: sortButton
                objectName: "panelSortButton-" + Number(panel.side || 0)
                anchors.right: presentationButton.left
                anchors.rightMargin: root.snapPx(4)
                y: root.snapPx((parent.height - height) / 2)
                width: root.snapPx(sortButtonContent.implicitWidth
                                   + root.actionButtonHorizontalMargin * 2)
                height: root.snapPx(Math.min(parent.height - 4, 28))
                hoverEnabled: true
                focusPolicy: Qt.NoFocus

                ZG.ToolTip {
                    visible: sortButton.hovered && !sortMenu.opened
                    delay: 500
                    timeout: 5000
                    text: panelRoot.sortModeName() === "unsorted"
                          ? "Sorting: Unsorted"
                          : "Sort by " + panelRoot.sortModeLabel()
                            + (panelRoot.sortIsAscending()
                               ? " · Ascending" : " · Descending")
                }

                contentItem: Row {
                    id: sortButtonContent
                    objectName: "panelSortButtonContent-"
                                + Number(panel.side || 0)
                    x: root.snapPx((parent.width - width) / 2)
                    y: root.snapPx((parent.height - height) / 2)
                    width: root.snapPx(implicitWidth)
                    height: root.snapPx(implicitHeight)
                    spacing: root.snapPx(5)
                    property real alignmentRevision:
                        sortButton.x + sortButton.y
                        + sortButton.width + sortButton.height
                    transform: Translate {
                        id: sortButtonContentPixelTranslation
                        x: root.iconPixelOffsetX(sortButtonContent)
                        y: root.iconPixelOffsetY(sortButtonContent)
                    }

                    PixelAlignedImage {
                        objectName: "panelSortDirectionIcon-"
                                    + Number(panel.side || 0)
                        readonly property string lucideName:
                            panelRoot.sortDirectionIconName()
                        visible: panelRoot.sortModeName() !== "unsorted"
                        width: visible ? root.snapPx(14) : 0
                        height: root.snapPx(14)
                        alignmentRevision: sortButton.x + sortButton.y
                                           + sortButton.width + sortButton.height
                                           + sortButtonContent.x
                                           + sortButtonContent.y
                                           + sortButtonContentPixelTranslation.x
                                           + sortButtonContentPixelTranslation.y
                        y: root.snapPx((parent.height - height) / 2)
                        smooth: false
                        source: root.lucideIconSource(
                                    lucideName, 14,
                                    sortButton.enabled
                                    ? root.chromeText : root.mutedText)
                    }

                    Text {
                        id: sortButtonLabel
                        objectName: "panelSortLabel-"
                                    + Number(panel.side || 0)
                        y: root.snapPx((parent.height - height) / 2)
                        text: panelRoot.sortModeLabel()
                        color: sortButton.enabled
                               ? root.chromeText : root.mutedText
                        font.pixelSize: 12
                    }

                    PixelAlignedImage {
                        objectName: "panelSortChevron-"
                                    + Number(panel.side || 0)
                        width: root.snapPx(11)
                        height: root.snapPx(11)
                        alignmentRevision: sortButton.x + sortButton.y
                                           + sortButton.width + sortButton.height
                                           + sortButtonContent.x
                                           + sortButtonContent.y
                                           + sortButtonContentPixelTranslation.x
                                           + sortButtonContentPixelTranslation.y
                        y: root.snapPx((parent.height - height) / 2)
                        smooth: false
                        source: root.lucideIconSource(
                                    "chevron-down", 11,
                                    sortButton.enabled
                                    ? root.chromeText : root.mutedText)
                    }
                }

                background: Rectangle {
                    radius: 5
                    color: sortButton.down
                           ? root.controlPressedBg
                           : sortButton.hovered || sortMenu.opened
                             ? root.controlHoverBg : "transparent"
                    border.width: sortMenu.opened ? 1 : 0
                    border.color: root.panelSelectionBorder
                }

                onClicked: {
                    if (sortMenu.opened) {
                        sortMenu.close()
                        return
                    }
                    rendererMenu.close()
                    sortMenu.open()
                }
            }

            Popup {
                id: sortMenu
                objectName: "panelSortMenu-" + Number(panel.side || 0)
                parent: Overlay.overlay
                width: Math.max(160, root.repeaterMaxImplicitWidth(
                                          sortChoiceRepeater))
                       + leftPadding + rightPadding
                padding: 6
                modal: false
                dim: false
                z: 1001
                focus: false
                closePolicy: Popup.CloseOnEscape
                             | Popup.CloseOnPressOutside
                             | Popup.CloseOnPressOutsideParent

                onAboutToShow: {
                    const point = sortButton.mapToItem(
                                    root.contentItem, sortButton.width,
                                    sortButton.height + 3)
                    x = Math.max(6, Math.min(root.width - width - 6,
                                            point.x - width))
                    y = Math.max(6, Math.min(root.height - height - 6,
                                            point.y))
                }
                onClosed: Qt.callLater(function() {
                    if (!panelRoot.panelIsActive || qtGallery.viewerVisible
                            || root.hasBlockingOverlay()
                            || root.needsFallbackGrid()
                            || root.hasDocumentSurface()
                            || root.hasOperationsQueueSurface())
                        return
                    if (galleryPanelContent.item)
                        galleryPanelContent.item.forceActiveFocus()
                    else
                        grid.forceActiveFocus()
                })

                background: Rectangle {
                    color: root.controlBg
                    radius: 8
                    border.width: 1
                    border.color: root.controlBorder
                }

                contentItem: Column {
                    id: sortMenuColumn
                    spacing: 2

                    Repeater {
                        id: sortChoiceRepeater
                        model: panelRoot.sortChoices

                        delegate: Rectangle {
                            id: sortChoice
                            required property var modelData
                            objectName: "panelSortChoice-"
                                        + root.cleanText(modelData.mode)
                                        + "-" + Number(panel.side || 0)
                            width: sortMenu.availableWidth
                            // Content determines the menu's width (see
                            // sortMenu.contentWidth below); this row must
                            // never be narrower than what it needs.
                            implicitWidth: 10 + sortChoiceLeading.implicitWidth
                                           + 24 + sortChoiceShortcut.implicitWidth
                                           + 10
                            height: 31
                            radius: 5
                            readonly property bool choiceActive:
                                panelRoot.sortModeName()
                                === root.cleanText(modelData.mode)
                            color: sortChoicePointer.containsMouse
                                   ? root.controlHoverBg : "transparent"

                            Row {
                                id: sortChoiceLeading
                                anchors.left: parent.left
                                anchors.leftMargin: 10
                                anchors.verticalCenter: parent.verticalCenter
                                spacing: 8

                                // Always reserves its slot so the icon/label
                                // stay aligned across rows regardless of
                                // whether this particular row is the active
                                // choice.
                                Item {
                                    width: root.snapPx(14)
                                    height: root.snapPx(14)
                                    anchors.verticalCenter: parent.verticalCenter

                                    PixelAlignedImage {
                                        objectName: "panelSortChoiceCheck-"
                                                    + root.cleanText(
                                                          sortChoice.modelData.mode)
                                                    + "-" + Number(panel.side || 0)
                                        anchors.fill: parent
                                        alignmentRevision: sortMenu.x + sortMenu.y
                                                           + sortMenu.width
                                                           + sortMenu.height
                                        visible: sortChoice.choiceActive
                                        smooth: false
                                        source: root.lucideIconSource(
                                                    "check", 14,
                                                    root.dialogAccent)
                                    }
                                }

                                PixelAlignedImage {
                                    objectName: "panelSortChoiceIcon-"
                                                + root.cleanText(
                                                      sortChoice.modelData.mode)
                                                + "-" + Number(panel.side || 0)
                                    width: root.snapPx(16)
                                    height: root.snapPx(16)
                                    alignmentRevision: sortMenu.x + sortMenu.y
                                                       + sortMenu.width
                                                       + sortMenu.height
                                    anchors.verticalCenter: parent.verticalCenter
                                    smooth: false
                                    source: root.lucideIconSource(
                                                     root.cleanText(
                                                         sortChoice.modelData.icon),
                                                     16, root.textColor)
                                }

                                Text {
                                    objectName: "panelSortChoiceLabel-"
                                                + root.cleanText(
                                                      sortChoice.modelData.mode)
                                                + "-" + Number(panel.side || 0)
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: root.cleanText(sortChoice.modelData.label)
                                    color: root.textColor
                                    font.pixelSize: 12
                                }
                            }

                            Text {
                                id: sortChoiceShortcut
                                anchors.right: parent.right
                                anchors.rightMargin: 10
                                anchors.verticalCenter: parent.verticalCenter
                                text: root.cleanText(
                                          sortChoice.modelData.shortcut)
                                color: root.mutedText
                                font.pixelSize: 10
                            }

                            MouseArea {
                                id: sortChoicePointer
                                anchors.fill: parent
                                hoverEnabled: true
                                cursorShape: Qt.PointingHandCursor
                                onClicked: panelRoot.chooseSort(
                                               sortChoice.modelData)
                            }
                        }
                    }
                }
            }

            ToolButton {
                id: presentationButton
                objectName: "panelRendererButton-" + Number(panel.side || 0)
                anchors.right: parent.right
                anchors.rightMargin: root.panelTextInset
                anchors.verticalCenter: parent.verticalCenter
                width: presentationButtonContent.implicitWidth
                       + root.actionButtonHorizontalMargin * 2
                height: Math.min(parent.height - 4, 28)
                hoverEnabled: true
                focusPolicy: Qt.NoFocus
                // Once the renderer popup is open its contents are the
                // active explanation.  Keeping the delayed button tooltip
                // alive above the popup creates a detached black label that
                // overlaps the menu and obscures the first interaction.
                ZG.ToolTip {
                    objectName: "panelRendererToolTip-"
                                + Number(panel.side || 0)
                    visible: presentationButton.hovered
                             && !rendererMenu.opened
                    delay: 500
                    timeout: 5000
                    text: "Panel view mode"
                }

                contentItem: Row {
                    id: presentationButtonContent
                    objectName: "panelRendererButtonContent-"
                                + Number(panel.side || 0)
                    anchors.centerIn: parent
                    spacing: 5

                    PixelAlignedImage {
                        objectName: "panelRendererButtonIcon-"
                                    + Number(panel.side || 0)
                        readonly property string lucideName:
                            panelRoot.rendererButtonIconName()
                        width: root.snapPx(16)
                        height: root.snapPx(16)
                        alignmentRevision: presentationButton.x
                                           + presentationButton.y
                                           + presentationButton.width
                                           + presentationButton.height
                                           + presentationButtonContent.x
                                           + presentationButtonContent.y
                        y: root.snapPx((parent.height - height) / 2)
                        smooth: false
                        source: root.lucideIconSource(
                                    lucideName, 16, root.chromeText)
                    }

                    PixelAlignedImage {
                        objectName: "panelRendererButtonChevron-"
                                    + Number(panel.side || 0)
                        readonly property string lucideName: "chevron-down"
                        width: root.snapPx(11)
                        height: root.snapPx(11)
                        alignmentRevision: presentationButton.x
                                           + presentationButton.y
                                           + presentationButton.width
                                           + presentationButton.height
                                           + presentationButtonContent.x
                                           + presentationButtonContent.y
                        y: root.snapPx((parent.height - height) / 2)
                        smooth: false
                        source: root.lucideIconSource(
                                    lucideName, 11, root.chromeText)
                    }
                }

                background: Rectangle {
                    radius: 5
                    color: presentationButton.down
                           ? root.controlPressedBg
                           : presentationButton.hovered || rendererMenu.opened
                             ? root.controlHoverBg : "transparent"
                    border.width: rendererMenu.opened ? 1 : 0
                    border.color: root.panelSelectionBorder
                }

                onClicked: {
                    if (rendererMenu.opened) {
                        rendererMenu.close()
                        return
                    }
                    sortMenu.close()
                    rendererMenu.open()
                }
            }

            Popup {
                id: rendererMenu
                objectName: "panelRendererMenu-" + Number(panel.side || 0)
                parent: Overlay.overlay
                width: Math.max(160, root.repeaterMaxImplicitWidth(
                                          rendererChoiceRepeater))
                       + leftPadding + rightPadding
                padding: 6
                modal: false
                dim: false
                z: 1001
                focus: false
                closePolicy: Popup.CloseOnEscape
                             | Popup.CloseOnPressOutside
                             | Popup.CloseOnPressOutsideParent

                onAboutToShow: {
                    const point = presentationButton.mapToItem(
                                    root.contentItem, presentationButton.width,
                                    presentationButton.height + 3)
                    x = Math.max(6, Math.min(root.width - width - 6,
                                            point.x - width))
                    y = Math.max(6, Math.min(root.height - height - 6,
                                            point.y))
                }
                onClosed: Qt.callLater(function() {
                    if (!panelRoot.panelIsActive || qtGallery.viewerVisible
                            || root.hasBlockingOverlay()
                            || root.needsFallbackGrid()
                            || root.hasDocumentSurface()
                            || root.hasOperationsQueueSurface())
                        return
                    if (galleryPanelContent.item)
                        galleryPanelContent.item.forceActiveFocus()
                    else
                        grid.forceActiveFocus()
                })

                background: Rectangle {
                    color: root.controlBg
                    radius: 8
                    border.width: 1
                    border.color: root.controlBorder
                }

                contentItem: Column {
                    id: rendererMenuColumn
                    spacing: 2

                    Repeater {
                        id: rendererChoiceRepeater
                        model: panelRoot.rendererChoices

                        delegate: Rectangle {
                            id: rendererChoice
                            required property int index
                            required property var modelData
                            readonly property bool isHeading:
                                modelData.heading === true
                            width: rendererMenu.availableWidth
                            implicitWidth: isHeading
                                ? 16 + rendererHeadingLabel.implicitWidth
                                : 8 + rendererChoiceLeading.implicitWidth
                                  + 24 + rendererChoiceShortcut.implicitWidth
                                  + 8
                            height: isHeading ? 25 : 31
                            radius: 5
                            readonly property bool choiceEnabled:
                                panelRoot.rendererChoiceEnabled(modelData)
                            readonly property bool choiceActive:
                                panelRoot.rendererChoiceActive(modelData)
                            color: isHeading ? "transparent"
                                   : rendererChoicePointer.containsMouse
                                     && choiceEnabled
                                     ? root.controlHoverBg : "transparent"

                            Rectangle {
                                visible: rendererChoice.isHeading && index > 0
                                anchors.left: parent.left
                                anchors.right: parent.right
                                anchors.top: parent.top
                                height: 1
                                color: root.separatorColor
                            }

                            Text {
                                id: rendererHeadingLabel
                                visible: rendererChoice.isHeading
                                anchors.left: parent.left
                                anchors.right: parent.right
                                anchors.bottom: parent.bottom
                                anchors.leftMargin: 8
                                anchors.rightMargin: 8
                                height: 20
                                text: root.cleanText(rendererChoice.modelData.label)
                                color: root.mutedText
                                font.pixelSize: 10
                                font.weight: Font.DemiBold
                                verticalAlignment: Text.AlignVCenter
                            }

                            Row {
                                id: rendererChoiceLeading
                                visible: !rendererChoice.isHeading
                                anchors.left: parent.left
                                anchors.leftMargin: 8
                                anchors.verticalCenter: parent.verticalCenter
                                spacing: 8

                                // Always reserves its slot so the icon/label
                                // stay aligned across rows regardless of
                                // whether this particular row is active.
                                Item {
                                    width: root.snapPx(14)
                                    height: root.snapPx(14)
                                    anchors.verticalCenter: parent.verticalCenter

                                    PixelAlignedImage {
                                        objectName: "panelRendererChoiceCheck-"
                                                    + root.cleanText(
                                                          rendererChoice.modelData.mode)
                                                    + "-" + Number(panel.side || 0)
                                        anchors.fill: parent
                                        alignmentRevision: rendererMenu.x
                                                           + rendererMenu.y
                                                           + rendererMenu.width
                                                           + rendererMenu.height
                                        visible: rendererChoice.choiceActive
                                        smooth: false
                                        source: root.lucideIconSource(
                                                    "check", 14,
                                                    root.dialogAccent)
                                    }
                                }

                                PixelAlignedImage {
                                    objectName: "panelRendererChoiceIcon-"
                                                + root.cleanText(
                                                      rendererChoice.modelData.mode)
                                                + "-" + Number(panel.side || 0)
                                    width: root.snapPx(16)
                                    height: root.snapPx(16)
                                    alignmentRevision: rendererMenu.x
                                                       + rendererMenu.y
                                                       + rendererMenu.width
                                                       + rendererMenu.height
                                    anchors.verticalCenter: parent.verticalCenter
                                    smooth: false
                                    source: root.lucideIconSource(
                                                     root.cleanText(
                                                         rendererChoice.modelData.icon),
                                                     16,
                                                     rendererChoice.choiceEnabled
                                                     ? root.textColor
                                                     : root.mutedText)
                                }

                                Text {
                                    text: root.cleanText(rendererChoice.modelData.label)
                                    color: rendererChoice.choiceEnabled
                                           ? root.textColor : root.mutedText
                                    opacity: rendererChoice.choiceEnabled ? 1 : 0.5
                                    font.pixelSize: 12
                                }
                            }

                            Text {
                                id: rendererChoiceShortcut
                                visible: !rendererChoice.isHeading
                                anchors.right: parent.right
                                anchors.rightMargin: 8
                                anchors.verticalCenter: parent.verticalCenter
                                text: root.cleanText(
                                          rendererChoice.modelData.shortcut)
                                color: root.mutedText
                                opacity: rendererChoice.choiceEnabled ? 1 : 0.5
                                font.pixelSize: 10
                            }

                            MouseArea {
                                id: rendererChoicePointer
                                anchors.fill: parent
                                hoverEnabled: true
                                enabled: rendererChoice.choiceEnabled
                                cursorShape: enabled ? Qt.PointingHandCursor
                                                     : Qt.ArrowCursor
                                onClicked: panelRoot.chooseRenderer(
                                               rendererChoice.modelData)
                            }
                        }
                    }

                    Rectangle {
                        width: rendererMenu.availableWidth
                        height: 1
                        color: root.separatorColor
                        visible: rendererZoomRow.visible
                    }

                    Item {
                        id: rendererZoomRow
                        objectName: "panelRendererZoomRow-"
                                    + Number(panel.side || 0)
                        width: rendererMenu.availableWidth
                        height: visible ? 48 : 0
                        visible: qtGallery.available
                                 && panelRoot.galleryHost()
                                 && panelRoot.galleryHost().densityAdjustable

                        Text {
                            id: rendererZoomLabel
                            anchors.left: parent.left
                            anchors.leftMargin: 8
                            anchors.top: parent.top
                            anchors.topMargin: 5
                            text: "Zoom"
                            color: root.mutedText
                            font.pixelSize: 10
                            font.weight: Font.DemiBold
                        }

                        Text {
                            id: rendererZoomReset
                            objectName: "panelRendererZoomReset-"
                                        + Number(panel.side || 0)
                            anchors.right: rendererZoomValue.left
                            anchors.rightMargin: 8
                            anchors.baseline: rendererZoomLabel.baseline
                            text: "Reset"
                            color: rendererZoomResetPointer.containsMouse
                                   ? root.panelSelectionBorder : root.mutedText
                            font.pixelSize: 10
                            font.underline: rendererZoomResetPointer.containsMouse

                            MouseArea {
                                id: rendererZoomResetPointer
                                anchors.fill: parent
                                anchors.margins: -3
                                hoverEnabled: true
                                cursorShape: Qt.PointingHandCursor
                                onClicked: {
                                    root.action({
                                        "action": "panel.resetGalleryDensity",
                                        "side": panel.side,
                                        "layoutMode": effectiveGalleryLayoutMode
                                    }, true)
                                }
                            }
                        }

                        Text {
                            id: rendererZoomValue
                            anchors.right: parent.right
                            anchors.rightMargin: 8
                            anchors.baseline: rendererZoomLabel.baseline
                            text: Math.round(rendererZoomSlider.value) + " px"
                            color: root.mutedText
                            font.pixelSize: 10
                        }

                        T.Slider {
                            id: rendererZoomSlider
                            objectName: "panelRendererZoomSlider-"
                                        + Number(panel.side || 0)
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.bottom: parent.bottom
                            anchors.leftMargin: 8
                            anchors.rightMargin: 8
                            height: 27
                            focusPolicy: Qt.NoFocus
                            hoverEnabled: true
                            from: panelRoot.galleryHost()
                                  ? panelRoot.galleryHost().minimumDensity : 0
                            to: panelRoot.galleryHost()
                                ? panelRoot.galleryHost().maximumDensity : 1
                            stepSize: panelRoot.galleryHost()
                                      ? panelRoot.galleryHost().densityStep : 1
                            value: panelRoot.galleryHost()
                                   ? panelRoot.galleryHost().currentDensity : 0
                            snapMode: T.Slider.SnapAlways

                            onMoved: {
                                const host = panelRoot.galleryHost()
                                if (host)
                                    host.previewDensity(value)
                            }
                            onPressedChanged: {
                                if (pressed)
                                    return
                                const host = panelRoot.galleryHost()
                                if (host)
                                    host.commitDensity(value)
                            }

                            background: Rectangle {
                                x: rendererZoomSlider.leftPadding
                                y: Math.round((rendererZoomSlider.height
                                               - height) / 2)
                                width: rendererZoomSlider.availableWidth
                                height: 4
                                radius: 2
                                color: root.controlBorder

                                Rectangle {
                                    width: rendererZoomSlider.visualPosition
                                           * parent.width
                                    height: parent.height
                                    radius: parent.radius
                                    color: root.panelSelectionBorder
                                }
                            }

                            handle: Rectangle {
                                x: rendererZoomSlider.leftPadding
                                   + rendererZoomSlider.visualPosition
                                     * (rendererZoomSlider.availableWidth
                                        - width)
                                y: Math.round((rendererZoomSlider.height
                                               - height) / 2)
                                width: 14
                                height: 14
                                radius: 7
                                color: rendererZoomSlider.pressed
                                       ? root.panelSelectionBorder
                                       : root.chromeText
                                border.width: 2
                                border.color: root.controlBg
                            }
                        }
                    }
                }
            }

            MouseArea {
                // Popup.CloseOnPressOutside is not delivered reliably when a
                // non-windowed popup is parented to Overlay.overlay above the
                // persistent panel/grid pointer layers. Put a transparent
                // dismiss plane immediately below the popup instead: popup
                // contents still receive their input, every outside press is
                // consumed here and cannot also move the file-panel cursor.
                parent: Overlay.overlay
                anchors.fill: parent
                visible: rendererMenu.opened
                enabled: visible
                z: 1000
                acceptedButtons: Qt.LeftButton | Qt.RightButton
                                 | Qt.MiddleButton
                onPressed: rendererMenu.close()
            }

            MouseArea {
                parent: Overlay.overlay
                anchors.fill: parent
                visible: sortMenu.opened
                enabled: visible
                z: 1000
                acceptedButtons: Qt.LeftButton | Qt.RightButton
                                 | Qt.MiddleButton
                onPressed: sortMenu.close()
            }

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
                    ? Math.max(22, root.ch) + root.verticalContentSpacing : 0
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
                                            - root.panelContentSpacing * 2)
                return root.panelContentSpacing
                        + Math.round(contentWidth * before
                                     / totalColumnWidth)
            }

            function columnWidth(index) {
                var start = columnX(index)
                return index === columns.length - 1
                        ? width - root.panelContentSpacing - start
                        : columnX(index + 1) - start
            }

            Repeater {
                model: columnHeader.columns

                delegate: Rectangle {
                    id: columnHeaderCell
                    x: columnHeader.columnX(index)
                    width: columnHeader.columnWidth(index)
                    height: columnHeader.height
                    color: columnMouse.containsMouse && modelData.sortable
                           ? root.controlHoverBg : "transparent"

                    Behavior on color { ColorAnimation { duration: 70 } }

                    Text {
                        anchors.fill: parent
                        anchors.leftMargin: root.panelRowInnerSpacing
                        anchors.rightMargin: root.panelRowInnerSpacing
                        text: root.cleanText(modelData.title)
                        color: modelData.sortable
                               ? root.chromeText : root.mutedText
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
                                         - root.columnSeparatorVerticalMargin * 2)
                        color: root.separatorColor
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
                                root.action({
                                    "action": "panel.sortMenu",
                                    "side": panel.side
                                })
                            } else {
                                root.action({
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
                color: root.separatorColor
                opacity: 0.7
            }
        }

        Loader {
            id: galleryPanelContent
            objectName: "galleryPanelContent-" + Number(panel.side || 0)
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.leftMargin: root.panelContentSpacing
            anchors.rightMargin: root.panelContentSpacing
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
            source: qtGallery.available ? qtGallery.panelComponentUrl : ""

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
                item.theme = Qt.binding(function() {
                    return root.galleryTheme()
                })
                item.metrics = Qt.binding(() => root.galleryMetrics())
                item.devicePixelRatio = Qt.binding(
                    () => root.screen ? root.screen.devicePixelRatio : 1.0)
                item.defaultListDensity = Qt.binding(
                    () => root.snapPx(Math.max(22, root.ch * 1.1)))
                // Lightweight QML test embedders may supply an older panel
                // host without this optional input property.
                if (typeof item.mouseWheelMode !== "undefined")
                    item.mouseWheelMode = Qt.binding(
                        () => root.mouseWheelMode)
                item.panelActive = Qt.binding(
                    () => panelRoot.visible
                          && panelRoot.panelIsActive
                          && !qtGallery.viewerVisible
                          && !root.needsFallbackGrid()
                          && !root.hasDocumentSurface()
                          && !root.hasOperationsQueueSurface()
                          && !root.hasBlockingOverlay())
                item.commandLineHasText = Qt.binding(() => {
                    var commandLine = root.commandLineFrame()
                    return root.cleanText(commandLine.text).length > 0
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
                root.beginPointerPanelActivation(side)
            }
        }

        Rectangle {
            id: rendererFailure
            objectName: "panelRendererFailure-" + Number(panel.side || 0)
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.leftMargin: root.panelContentSpacing
            anchors.rightMargin: root.panelContentSpacing
            anchors.top: parent.top
            anchors.topMargin: panelHeader.height + columnHeader.height
            anchors.bottom: status.top
            z: 2
            visible: panelRoot.visible
                     && (!qtGallery.available
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
                    icon.source: root.lucideIconSource(
                                     "triangle-alert", 24, root.activeBorder)
                    icon.width: 24
                    icon.height: 24
                    icon.color: root.activeBorder
                }

                Text {
                    width: parent.width
                    text: qtGallery.available
                          ? "The unified panel renderer could not be loaded."
                          : "The unified panel renderer is unavailable in this build."
                    color: root.textColor
                    font.pixelSize: 13
                    horizontalAlignment: Text.AlignHCenter
                    wrapMode: Text.Wrap
                }
            }
        }

        onPanelIsActiveChanged: {
            if (!visible || !panelIsActive || qtGallery.viewerVisible
                    || root.hasBlockingOverlay() || root.needsFallbackGrid()
                    || root.hasDocumentSurface()
                    || root.hasOperationsQueueSurface())
                return
            if (galleryPanelContent.item)
                galleryPanelContent.item.forceActiveFocus()
            else
                grid.forceActiveFocus()
        }

        Connections {
            target: qtGallery
            function onViewerChanged() {
                if (qtGallery.viewerVisible || root.hasBlockingOverlay()
                        || root.needsFallbackGrid()
                        || root.hasDocumentSurface()
                        || root.hasOperationsQueueSurface())
                    return
                if (panelRoot.visible && panelRoot.panelIsActive
                        && galleryPanelContent.item) {
                    galleryPanelContent.item.forceActiveFocus()
                } else if (panelRoot.visible && panelRoot.panelIsActive) {
                    grid.forceActiveFocus()
                }
            }
        }

        Rectangle {
            id: fastFindOverlay
            objectName: "panelFastFindOverlay-"
                        + Number(panel.side || 0)
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.bottom: status.top
            anchors.bottomMargin: root.panelContentSpacing
            readonly property real desiredWidth:
                Math.max(root.snapPx(220),
                         fastFindQuery.implicitWidth + root.snapPx(64))
            width: Math.max(1,
                            Math.min(parent.width
                                     - root.panelContentSpacing * 2,
                                     desiredWidth))
            height: root.snapPx(36)
            visible: panel.fastFind === true
            z: 4
            clip: true
            radius: root.snapPx(8)
            color: root.dialogBg
            border.width: root.separatorWidth
            border.color: root.controlBorder

            FontMetrics {
                id: fastFindFontMetrics
                font.family: root.guiMonospaceFontFamily
                font.pixelSize: 13
            }

            PixelAlignedImage {
                id: fastFindIcon
                objectName: "panelFastFindIcon-"
                            + Number(panel.side || 0)
                anchors.left: parent.left
                anchors.leftMargin: root.snapPx(10)
                anchors.verticalCenter: parent.verticalCenter
                width: root.snapPx(15)
                height: root.snapPx(15)
                smooth: false
                source: root.lucideIconSource("search", 15,
                                             root.dialogAccent)
            }

            Text {
                id: fastFindQuery
                objectName: "panelFastFindText-"
                            + Number(panel.side || 0)
                anchors.left: fastFindIcon.right
                anchors.leftMargin: root.snapPx(8)
                anchors.right: parent.right
                anchors.rightMargin: root.snapPx(10)
                anchors.verticalCenter: parent.verticalCenter
                text: root.cleanText(panel.fastFindText)
                color: root.textColor
                font.family: root.guiMonospaceFontFamily
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
                y: fastFindQuery.y + root.snapPx(2)
                width: root.snapPx(2)
                height: Math.max(root.snapPx(1),
                                 fastFindQuery.height - root.snapPx(4))
                color: root.textColor
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
                    ? Math.max(24, root.ch * 1.15)
                      + root.verticalContentSpacing
                    : 0
            color: "transparent"
            // Keep the footer above dynamically loaded semantic content even
            // while Loader/anchor geometry is settling after scene changes.
            z: 3

            Rectangle {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.top: parent.top
                height: root.separatorWidth
                color: root.separatorColor
            }

            Text {
                objectName: "panelStatusSelection-"
                            + Number(panel.side || 0)
                anchors.left: parent.left
                anchors.verticalCenter: parent.verticalCenter
                anchors.leftMargin: root.panelTextInset
                text: root.cleanText(panel.selectedCount) + " selected"
                color: root.mutedText
                font.pixelSize: 12
            }

            Text {
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                anchors.rightMargin: root.panelTextInset
                text: root.cleanText(panel.totalCount) + " items"
                color: root.mutedText
                font.pixelSize: 12
            }
        }
    }

    component QuickViewPanelView: Rectangle {
        id: quickRoot
        property var quickView: ({})
        readonly property int side: Number(quickView.side || 0)
        readonly property var documentFrame:
            quickView.surface || ({})
        readonly property string previewKind:
            root.cleanText(quickView.previewKind).toLowerCase()
        readonly property bool showsDocument:
            previewKind === "text" || previewKind === "binary"
            || previewKind === "hex" || previewKind === "provider"
            || previewKind === "document"
            || (previewKind === "" && documentFrame.rows !== undefined)
        // Go exports the exact fixed header used by the text frontend.
        // Directory stats deliberately have no separate header because their
        // first body row already contains the selected folder name.
        readonly property var effectiveHeaderRows:
            quickView.headerRows || []
        readonly property var directoryRows: {
            if (documentFrame.windowRows !== undefined)
                return documentFrame.windowRows || []
            return documentFrame.rows || []
        }

        objectName: "quickViewPanel-" + side
        x: root.nativePanelX(side)
        y: semanticMenu.height
        width: root.nativePanelWidth(side)
        height: Math.max(1, root.height - semanticMenu.height
                         - root.commandLineHeight(root.shellFrame())
                         - root.keyBarHeight())
        color: "transparent"
        border.width: 0
        clip: true
        z: 3

        Rectangle {
            id: quickTitle
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: Math.max(25, root.ch * 1.25)
            color: "transparent"

            Text {
                anchors.fill: parent
                anchors.leftMargin: 8
                anchors.rightMargin: 8
                text: root.cleanText(quickRoot.quickView.title)
                color: quickRoot.quickView.active === true
                       ? root.activeBorder : root.textColor
                font.pixelSize: 13
                font.bold: true
                horizontalAlignment: Text.AlignHCenter
                verticalAlignment: Text.AlignVCenter
                elide: Text.ElideMiddle
            }

            Rectangle {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                height: root.separatorWidth
                color: root.separatorColor
            }
        }

        Item {
            id: quickHeader
            objectName: "quickViewHeader-" + quickRoot.side
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: quickTitle.bottom
            height: Math.min(4, quickRoot.effectiveHeaderRows.length)
                    * Math.max(20, root.ch)
            clip: true

            Repeater {
                model: quickRoot.effectiveHeaderRows
                delegate: ConsoleRunRow {
                    x: 0
                    y: index * Math.max(20, root.ch)
                    width: quickHeader.width
                    height: Math.max(20, root.ch)
                    runs: modelData.runs || []
                    fallbackText: root.rowText(modelData)
                }
            }
        }

        Item {
            id: quickContent
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: quickHeader.bottom
            anchors.bottom: quickFooter.top
            clip: true

            DocumentSurface {
                id: quickDocument
                frame: quickRoot.documentFrame
                embedded: true
                interactionActive: quickRoot.visible
                                   && quickRoot.showsDocument
                surfaceObjectName: "quickViewDocumentSurface-"
                                   + quickRoot.side
                anchors.fill: parent
                visible: quickRoot.showsDocument
            }

            ListView {
                id: quickDirectoryList
                objectName: "quickViewDirectoryList-" + quickRoot.side
                anchors.fill: parent
                anchors.leftMargin: 8
                anchors.rightMargin: 8
                visible: quickRoot.previewKind === "directory"
                model: quickRoot.directoryRows
                clip: true
                spacing: 1
                interactive: visible
                boundsBehavior: Flickable.StopAtBounds
                reuseItems: true

                delegate: ConsoleRunRow {
                    width: ListView.view.width
                    height: Math.max(20, root.ch)
                    runs: modelData.runs || []
                    fallbackText: root.rowText(modelData)
                }
            }

            Image {
                id: quickImage
                objectName: "quickViewImage-" + quickRoot.side
                anchors.fill: parent
                anchors.margins: 10
                visible: quickRoot.previewKind === "image"
                source: root.cleanText(quickRoot.quickView.imageSource)
                sourceSize.width: Number(quickRoot.quickView.imageWidth || 0)
                sourceSize.height: Number(quickRoot.quickView.imageHeight || 0)
                asynchronous: true
                cache: true
                fillMode: Image.PreserveAspectFit
                smooth: true
                mipmap: true
            }

            Text {
                objectName: "quickViewLoading-" + quickRoot.side
                anchors.centerIn: parent
                width: Math.max(0, parent.width - 24)
                visible: (quickRoot.previewKind === "loading"
                          && quickRoot.effectiveHeaderRows.length < 4)
                         || (quickRoot.previewKind === "image"
                             && quickRoot.quickView.loading === true)
                text: root.cleanText(quickRoot.quickView.label) !== ""
                      ? root.cleanText(quickRoot.quickView.label)
                      : "Loading…"
                color: root.mutedText
                font.pixelSize: 13
                horizontalAlignment: Text.AlignHCenter
                wrapMode: Text.WrapAtWordBoundaryOrAnywhere
            }

            Text {
                objectName: "quickViewError-" + quickRoot.side
                anchors.centerIn: parent
                width: Math.max(0, parent.width - 24)
                visible: quickRoot.previewKind === "error"
                         && quickRoot.effectiveHeaderRows.length < 4
                         && root.cleanText(quickRoot.quickView.error) !== ""
                text: root.cleanText(quickRoot.quickView.error)
                color: root.textColor
                font.pixelSize: 13
                horizontalAlignment: Text.AlignHCenter
                wrapMode: Text.WrapAtWordBoundaryOrAnywhere
            }
        }

        Rectangle {
            id: quickFooter
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: Math.max(24, root.ch * 1.15)
            color: "transparent"

            Rectangle {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.top: parent.top
                height: root.separatorWidth
                color: root.separatorColor
            }

            Text {
                anchors.fill: parent
                anchors.leftMargin: 8
                anchors.rightMargin: 8
                text: root.cleanText(quickRoot.quickView.bottomHint)
                color: root.mutedText
                font.pixelSize: 11
                horizontalAlignment: Text.AlignHCenter
                verticalAlignment: Text.AlignVCenter
                elide: Text.ElideMiddle
            }
        }

        TapHandler {
            acceptedButtons: Qt.LeftButton
            gesturePolicy: TapHandler.ReleaseWithinBounds
            onTapped: root.action({
                "action": "panel.activate",
                "side": quickRoot.side
            })
        }
    }

    component InfoPanelView: Rectangle {
        id: infoRoot
        property var panel: ({})
        readonly property int side: Number(panel.side || 0)

        x: root.nativePanelX(side)
        y: semanticMenu.height
        width: root.nativePanelWidth(side)
        height: Math.max(1, root.height - semanticMenu.height
                         - root.commandLineHeight(root.shellFrame())
                         - root.keyBarHeight())
        color: "transparent"
        border.width: 0
        clip: true

        Rectangle {
            id: infoHeader
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: Math.max(25, root.ch * 1.25)
            color: "transparent"

            Text {
                anchors.fill: parent
                anchors.leftMargin: 8
                anchors.rightMargin: 8
                text: root.cleanText(panel.title)
                color: root.textColor
                font.pixelSize: 13
                font.bold: true
                horizontalAlignment: Text.AlignHCenter
                verticalAlignment: Text.AlignVCenter
                elide: Text.ElideMiddle
            }
        }

        ListView {
            id: infoRows
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: infoHeader.bottom
            anchors.bottom: infoFooter.top
            anchors.margins: 8
            model: panel.rows || []
            clip: true
            spacing: 2
            boundsBehavior: Flickable.StopAtBounds

            delegate: Item {
                width: ListView.view.width
                height: modelData.kind === "blank" ? 10
                        : modelData.kind === "section" ? 30
                        : Math.max(25, infoValue.implicitHeight + 8)

                RowLayout {
                    anchors.fill: parent
                    spacing: 10
                    visible: modelData.kind === "row"

                    Text {
                        text: root.cleanText(modelData.label)
                        color: root.mutedText
                        font.pixelSize: 12
                        verticalAlignment: Text.AlignTop
                        Layout.alignment: Qt.AlignTop
                        Layout.preferredWidth: Math.min(150, Math.max(80, infoRows.width * 0.34))
                        elide: Text.ElideRight
                    }

                    Text {
                        id: infoValue
                        text: root.cleanText(modelData.value)
                        color: root.textColor
                        font.pixelSize: 12
                        wrapMode: Text.WrapAtWordBoundaryOrAnywhere
                        horizontalAlignment: Text.AlignRight
                        verticalAlignment: Text.AlignTop
                        Layout.alignment: Qt.AlignTop
                        Layout.fillWidth: true
                    }
                }

                RowLayout {
                    anchors.fill: parent
                    spacing: 8
                    visible: modelData.kind === "section"

                    Rectangle { height: 1; color: root.dialogAccent; Layout.fillWidth: true }
                    Text {
                        text: root.cleanText(modelData.label)
                        color: root.activeBorder
                        font.pixelSize: 12
                        font.bold: true
                    }
                    Rectangle { height: 1; color: root.dialogAccent; Layout.fillWidth: true }
                }
            }
        }

        Rectangle {
            id: infoFooter
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: Math.max(24, root.ch * 1.15)
            color: "transparent"

            Text {
                anchors.fill: parent
                anchors.leftMargin: 8
                anchors.rightMargin: 8
                text: root.cleanText(panel.bottomHint)
                color: root.mutedText
                font.pixelSize: 11
                horizontalAlignment: Text.AlignHCenter
                verticalAlignment: Text.AlignVCenter
                elide: Text.ElideMiddle
            }
        }

        MouseArea {
            anchors.fill: parent
            acceptedButtons: Qt.LeftButton
            propagateComposedEvents: true
            onClicked: (mouse) => {
                root.action({ "action": "panel.activate", "side": infoRoot.side })
                mouse.accepted = false
            }
            onWheel: (wheel) => { wheel.accepted = false }
        }
    }

    component ConsoleRunRow: Item {
        property var runs: []
        property string fallbackText: ""
        property bool transparentBlackBackground: false
        property bool ignoreRunBackground: false
        property real fallbackFontPixelSize: root.guiMonospaceFontPixelSize
        property real horizontalInset: 8

        function runBackground(value) {
            if (ignoreRunBackground)
                return "transparent"
            var background = root.cleanText(value).toLowerCase()
            if (transparentBlackBackground
                    && (background === "#000000"
                        || background === "#ff000000"
                        || background === "black"))
                return "transparent"
            return background !== "" ? value : "transparent"
        }

        // Use the data model, rather than effective visibility, to select the
        // measured content. Measurement-only rows are deliberately hidden;
        // inherited visibility must not collapse their reported run width.
        readonly property real contentWidth: horizontalInset
                                             + (runs && runs.length > 0
                                                ? runRow.implicitWidth
                                                : fallbackRunLabel.implicitWidth)

        Row {
            id: runRow
            anchors.left: parent.left
            anchors.leftMargin: horizontalInset
            height: parent.height
            visible: runs && runs.length > 0

            Repeater {
                model: runs || []

                delegate: Rectangle {
                    height: parent ? parent.height : root.ch
                    width: consoleRunLabel.implicitWidth
                    color: runBackground(modelData.background)

                    Text {
                        id: consoleRunLabel
                        anchors.verticalCenter: parent.verticalCenter
                        text: root.cleanText(modelData.text)
                        color: root.cleanText(modelData.foreground) !== ""
                               ? modelData.foreground : root.textColor
                        font.family: root.guiMonospaceFontFamily
                        font.pixelSize: root.semanticTextFontPixelSize
                        font.bold: modelData.bold === true
                        font.underline: modelData.underline === true
                        font.strikeout: modelData.strikeout === true
                    }
                }
            }
        }

        Text {
            id: fallbackRunLabel
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.leftMargin: horizontalInset
            anchors.rightMargin: horizontalInset
            anchors.verticalCenter: parent.verticalCenter
            visible: !runs || runs.length === 0
            text: fallbackText
            color: root.textColor
            font.family: root.guiMonospaceFontFamily
            font.pixelSize: fallbackFontPixelSize
            elide: Text.ElideRight
        }
    }

    component TerminalBackdrop: Rectangle {
        property var terminal: ({})
        objectName: "terminalBackdrop"
        color: "transparent"
        clip: true

        // Keep the terminal's newest row attached to the bottom of its own
        // viewport. The commander command line lives immediately below this
        // rectangle, so reducing the viewport must move rows upward instead
        // of clipping the latest output behind the prompt.
        Item {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: (terminal.rows || []).length * root.ch

            Repeater {
                model: terminal.rows || []

                delegate: ConsoleRunRow {
                    x: 0
                    y: Math.max(0, Number(modelData.index || 0)) * root.ch
                    width: parent.width
                    height: root.ch
                    transparentBlackBackground: true
                    runs: modelData.runs || []
                    fallbackText: root.rowText(modelData)
                }
            }
        }
    }

    component CommandLineView: Rectangle {
        property var commandLine: ({})
        property var shell: root.shellFrame()
        property bool nativeLayout: root.isAppScene()
        objectName: "commandLineView"

        // A command-line patch changes text and caret position together.  A
        // TextInput may apply its own cursor reset while accepting the new
        // text after a declarative cursor binding has already run.  Reapply
        // the semantic caret on the next event-loop turn, once both values
        // have settled.
        onCommandLineChanged: commandInput.scheduleSemanticCursorSync()

        x: nativeLayout ? 0 : root.pxX(commandLine.x)
        y: nativeLayout ? root.height - root.keyBarHeight() - root.commandLineHeight(shell) : root.pxY(commandLine.y)
        width: nativeLayout ? root.width : root.pxW(commandLine.w)
        height: nativeLayout ? root.commandLineHeight(shell)
                             : Math.max(root.ch, root.pxH(commandLine.h))
        visible: commandLine.visible !== false
        color: root.commandLineBg

        Item {
            id: commandPresentation
            objectName: "commandLinePresentation"
            anchors.fill: parent
            anchors.leftMargin: root.commandLineLeftMargin
            anchors.rightMargin: root.contentSpacing
            anchors.topMargin: root.commandLineVerticalMargin
                               + root.separatorWidth
            anchors.bottomMargin: root.commandLineVerticalMargin
                                  + root.separatorWidth
            clip: true

            ConsoleRunRow {
                id: commandPrompt
                objectName: "commandLinePrompt"
                anchors.left: parent.left
                anchors.verticalCenter: parent.verticalCenter
                width: Math.min(contentWidth, commandPresentation.width * 0.5)
                height: parent.height
                clip: true
                horizontalInset: 0
                transparentBlackBackground: true
                ignoreRunBackground: true
                runs: commandLine.promptRuns || []
                fallbackText: root.cleanText(commandLine.prompt)
            }

            TextInput {
                id: commandInput
                objectName: "commandLineInput"
                anchors.left: commandPrompt.right
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                height: parent.height
                text: root.cleanText(commandLine.text)
                color: root.textColor
                selectionColor: root.selectedBg
                selectedTextColor: root.textColor
                font.family: root.guiMonospaceFontFamily
                font.pixelSize: root.semanticTextFontPixelSize
                verticalAlignment: TextInput.AlignVCenter
                readOnly: true
                activeFocusOnPress: false
                cursorDelegate: Item { width: 0; height: 0 }

                function semanticCursorPosition() {
                    return Math.max(0, Math.min(text.length,
                                    Number(commandLine.cursorPosition || 0)))
                }

                function syncSemanticCursor() {
                    var position = semanticCursorPosition()
                    if (cursorPosition !== position)
                        cursorPosition = position
                }

                function scheduleSemanticCursorSync() {
                    commandCursorSyncTimer.restart()
                }

                onTextChanged: scheduleSemanticCursorSync()
                Component.onCompleted: scheduleSemanticCursorSync()

                Timer {
                    id: commandCursorSyncTimer
                    interval: 0
                    repeat: false
                    onTriggered: commandInput.syncSemanticCursor()
                }
            }
        }

        FontMetrics {
            id: commandLineFontMetrics
            font.family: root.guiMonospaceFontFamily
            font.pixelSize: root.guiMonospaceFontPixelSize
        }

        Rectangle {
            id: commandCursor
            objectName: "commandLineCursor"
            property bool blinkOn: true
            readonly property bool block: commandLine.cursorShape === "block"
            readonly property int textPosition: commandInput.cursorPosition
            readonly property rect caretRect: commandInput.cursorRectangle
            x: commandPresentation.x + commandInput.x + caretRect.x
            y: block ? commandPresentation.y
                     : commandPresentation.y + commandPresentation.height - 2
            width: Math.max(1, commandLineFontMetrics.advanceWidth("M"))
            height: block ? commandPresentation.height : 2
            color: "#ffffff"
            visible: commandLine.cursorVisible === true
            opacity: blinkOn ? 1 : 0
            z: 2

            function restartBlink() {
                blinkOn = true
                if (visible)
                    commandCursorBlinkTimer.restart()
            }

            onVisibleChanged: {
                if (visible)
                    restartBlink()
            }

            Connections {
                target: root
                function onKeyboardActivityRevisionChanged() {
                    commandCursor.restartBlink()
                }
            }

            Timer {
                id: commandCursorBlinkTimer
                interval: 520
                running: commandCursor.visible
                repeat: true
                onTriggered: commandCursor.blinkOn = !commandCursor.blinkOn
            }
        }

        Rectangle {
            objectName: "commandLineTopSeparator"
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: root.separatorWidth
            color: root.separatorColor
        }
    }

    component OperationsQueueSurface: Rectangle {
        id: queueRoot
        objectName: "operationsQueueSurface"
        property var queue: ({})
        property bool interactionActive: true
        property int localSelectedTaskId: -1
        property int pendingSelectedTaskId: -1
        property int lastSemanticSelectedTaskId: -1
        property bool syncingModel: false
        readonly property real topInset: semanticMenu.visible
                                          ? semanticMenu.height : 0
        readonly property real bottomInset:
                                             Object.keys(root.keyBarModel).length > 0
                                             ? root.keyBarHeight() : 0
        readonly property real rowHeight: Math.max(60, root.ch * 2.8)
        readonly property bool compactColumns: width < 900
        readonly property real idColumnWidth: compactColumns ? 0 : 54
        readonly property real stateColumnWidth: compactColumns ? 92 : 116
        readonly property real typeColumnWidth: compactColumns ? 92 : 116
        readonly property real progressColumnWidth: compactColumns ? 132 : 184
        readonly property real speedColumnWidth: compactColumns ? 0 : 112
        readonly property int selectedIndex:
            indexForTaskId(localSelectedTaskId)

        color: root.windowBackgroundColor
        Accessible.role: Accessible.Table
        Accessible.name: root.cleanText(queue.title || "Operations Queue")
        Accessible.description: root.cleanText(queue.accessibleDescription)

        function normalizedItem(item, fallbackIndex) {
            item = item || ({})
            return {
                "stableId": root.cleanText(item.id || ("queue-task-" + item.taskId)),
                "taskId": Number(item.taskId || 0),
                "itemIndex": Number(item.index !== undefined
                                      ? item.index : fallbackIndex),
                "taskType": root.cleanText(item.type),
                "description": root.cleanText(item.description),
                "state": root.cleanText(item.state),
                "stateClass": root.cleanText(item.stateClass),
                "currentFile": root.cleanText(item.currentFile),
                "displayText": root.cleanText(item.displayText),
                "currentProgress": item.currentProgress === undefined
                                   ? -1 : Math.max(-1, Math.min(100,
                                                Number(item.currentProgress))),
                "progress": item.progress === undefined
                            ? -1 : Math.max(-1, Math.min(100,
                                           Number(item.progress))),
                "totalText": root.cleanText(item.totalText),
                "speed": root.cleanText(item.speed),
                "error": root.cleanText(item.error),
                "cancellable": item.cancellable === true,
                "hasDetails": item.hasDetails === true,
                "terminal": item.terminal === true,
                "active": item.active === true
            }
        }

        function modelIndexForStableId(stableId, first) {
            for (var i = Math.max(0, Number(first || 0));
                    i < queueRowsModel.count; ++i) {
                if (queueRowsModel.get(i).stableId === stableId)
                    return i
            }
            return -1
        }

        function indexForTaskId(taskId) {
            for (var i = 0; i < queueRowsModel.count; ++i) {
                if (Number(queueRowsModel.get(i).taskId) === Number(taskId))
                    return i
            }
            return -1
        }

        function updateRole(index, role, value) {
            if (queueRowsModel.get(index)[role] !== value)
                queueRowsModel.setProperty(index, role, value)
        }

        function updateRow(index, row) {
            updateRole(index, "stableId", row.stableId)
            updateRole(index, "taskId", row.taskId)
            updateRole(index, "itemIndex", row.itemIndex)
            updateRole(index, "taskType", row.taskType)
            updateRole(index, "description", row.description)
            updateRole(index, "state", row.state)
            updateRole(index, "stateClass", row.stateClass)
            updateRole(index, "currentFile", row.currentFile)
            updateRole(index, "displayText", row.displayText)
            updateRole(index, "currentProgress", row.currentProgress)
            updateRole(index, "progress", row.progress)
            updateRole(index, "totalText", row.totalText)
            updateRole(index, "speed", row.speed)
            updateRole(index, "error", row.error)
            updateRole(index, "cancellable", row.cancellable)
            updateRole(index, "hasDetails", row.hasDetails)
            updateRole(index, "terminal", row.terminal)
            updateRole(index, "active", row.active)
        }

        function syncItems() {
            if (syncingModel)
                return
            syncingModel = true
            var incoming = queue.items || []
            var previousSelectedTaskId = localSelectedTaskId
            var wasEmpty = queueRowsModel.count === 0
            for (var index = 0; index < incoming.length; ++index) {
                var row = normalizedItem(incoming[index], index)
                var existing = modelIndexForStableId(row.stableId, index)
                if (existing < 0)
                    queueRowsModel.insert(index, row)
                else {
                    if (existing !== index)
                        queueRowsModel.move(existing, index, 1)
                    updateRow(index, row)
                }
            }
            if (queueRowsModel.count > incoming.length) {
                // Clamp the viewport before shrinking the model.  Qt can
                // otherwise keep incubating a delegate at the old tail and
                // emit DelegateModel::cancel out-of-range during the batch.
                var nextMaximumY = Math.max(0, incoming.length * rowHeight
                                            - operationsQueueList.height)
                if (operationsQueueList.contentY > nextMaximumY) {
                    operationsQueueList.cancelFlick()
                    operationsQueueList.contentY = nextMaximumY
                }
                if (incoming.length === 0)
                    queueRowsModel.clear()
                else
                    queueRowsModel.remove(incoming.length,
                                          queueRowsModel.count - incoming.length)
            }

            var semanticTaskId = Number(queue.selectedTaskId || 0)
            if (pendingSelectedTaskId >= 0
                    && indexForTaskId(pendingSelectedTaskId) >= 0) {
                if (semanticTaskId === pendingSelectedTaskId) {
                    pendingSelectedTaskId = -1
                    selectionAckTimer.stop()
                }
            } else {
                pendingSelectedTaskId = -1
                var semanticIndex = Math.max(0,
                                    Math.min(incoming.length - 1,
                                             Number(queue.selected || 0)))
                localSelectedTaskId = semanticTaskId > 0
                        ? semanticTaskId
                        : incoming.length > 0
                          ? Number(incoming[semanticIndex].taskId || 0) : -1
            }
            if (incoming.length === 0)
                localSelectedTaskId = -1
            else if (indexForTaskId(localSelectedTaskId) < 0)
                localSelectedTaskId = Number(incoming[0].taskId || 0)
            var semanticSelectionChanged = semanticTaskId > 0
                    && semanticTaskId !== lastSemanticSelectedTaskId
            lastSemanticSelectedTaskId = semanticTaskId
            syncingModel = false
            if (wasEmpty && incoming.length > 0)
                Qt.callLater(queueRoot.applySemanticTop)
            else if (semanticSelectionChanged
                     && previousSelectedTaskId
                        !== localSelectedTaskId)
                Qt.callLater(queueRoot.revealSelection)
        }

        function selectedItem() {
            return selectedIndex >= 0 && selectedIndex < queueRowsModel.count
                    ? queueRowsModel.get(selectedIndex) : null
        }

        function controlOwnsActivation() {
            return headerClearButton.activeFocus || cancelButton.activeFocus
        }

        function delegateForTaskId(taskId) {
            var rowIndex = indexForTaskId(taskId)
            return rowIndex >= 0
                    ? operationsQueueList.itemAtIndex(rowIndex) : null
        }

        function selectIndex(index, notifyBackend) {
            if (queueRowsModel.count < 1)
                return false
            index = Math.max(0, Math.min(queueRowsModel.count - 1, index))
            var row = queueRowsModel.get(index)
            localSelectedTaskId = Number(row.taskId)
            revealSelection()
            if (notifyBackend === true) {
                pendingSelectedTaskId = localSelectedTaskId
                selectionAckTimer.restart()
                root.action({
                    "target": root.cleanText(queue.id),
                    "action": "queue.select",
                    "taskId": Number(row.taskId)
                }, true)
            }
            return true
        }

        function revealSelection() {
            if (selectedIndex >= 0 && operationsQueueList.visible)
                operationsQueueList.positionViewAtIndex(selectedIndex,
                                                         ListView.Contain)
        }

        function applySemanticTop() {
            if (queueRowsModel.count < 1 || !operationsQueueList.visible)
                return
            var top = Math.max(0, Math.min(queueRowsModel.count - 1,
                                           Number(queue.top || 0)))
            operationsQueueList.positionViewAtIndex(top, ListView.Beginning)
        }

        function navigate(command) {
            if (queueRowsModel.count < 1)
                return false
            var index = selectedIndex >= 0 ? selectedIndex : 0
            var page = Math.max(1, Math.floor(operationsQueueList.height
                                              / rowHeight) - 1)
            if (command === "up")
                index -= 1
            else if (command === "down")
                index += 1
            else if (command === "pageUp")
                index -= page
            else if (command === "pageDown")
                index += page
            else if (command === "home")
                index = 0
            else if (command === "end")
                index = queueRowsModel.count - 1
            return selectIndex(index, true)
        }

        function activateItem(row) {
            if (!row)
                return false
            root.action({
                "target": root.cleanText(queue.id),
                "action": "queue.activate",
                "taskId": Number(row.taskId)
            }, true)
            return true
        }

        function activateSelection() {
            return activateItem(selectedItem())
        }

        function cancelSelection() {
            var row = selectedItem()
            if (!row || row.cancellable !== true)
                return false
            root.action({
                "target": root.cleanText(queue.id),
                "action": "queue.cancel",
                "taskId": Number(row.taskId)
            }, true)
            return true
        }

        function clearCompleted() {
            if (queue.canClear !== true)
                return false
            root.action({
                "target": root.cleanText(queue.id),
                "action": "queue.clearCompleted"
            }, true)
            return true
        }

        function stateColor(stateClass, state) {
            var value = root.cleanText(stateClass || state).toLowerCase()
            if (value === "error")
                return "#ee6a6a"
            if (value === "done" || value === "completed"
                    || value === "success")
                return "#75c991"
            if (value === "running" || value === "scanning"
                    || value === "active")
                return root.dialogAccent
            if (value === "queued" || value === "starting")
                return "#d9b866"
            return root.mutedText
        }

        function stateIconName(stateClass, state) {
            var value = root.cleanText(stateClass || state).toLowerCase()
            if (value === "error")
                return "triangle-alert"
            if (value === "done" || value === "completed"
                    || value === "success")
                return "circle-check"
            if (value === "running" || value === "scanning"
                    || value === "active")
                return "loader-circle"
            if (value === "queued" || value === "starting")
                return "clock-3"
            if (value === "cancelled" || value === "cancelling")
                return "circle-x"
            return "clock-3"
        }

        function columnTitle(id, fallback) {
            var columns = queue.columns || []
            for (var i = 0; i < columns.length; ++i) {
                if (root.cleanText(columns[i].id) === id)
                    return root.cleanText(columns[i].title || fallback)
            }
            return fallback
        }

        onQueueChanged: syncItems()
        onInteractionActiveChanged: {
            if (interactionActive)
                return
            operationsQueueList.cancelFlick()
            selectionAckTimer.stop()
            pendingSelectedTaskId = -1
        }
        Component.onCompleted: syncItems()

        Timer {
            id: selectionAckTimer
            interval: 700
            onTriggered: {
                queueRoot.pendingSelectedTaskId = -1
                queueRoot.syncItems()
            }
        }

        ListModel {
            id: queueRowsModel
            objectName: "operationsQueueRowsModel"
            dynamicRoles: true
        }

        Rectangle {
            id: queueChrome
            objectName: "operationsQueueHeader"
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.topMargin: queueRoot.topInset
            height: 62
            color: root.titleBarBg

            Text {
                anchors.left: parent.left
                anchors.leftMargin: root.contentSpacing
                anchors.top: parent.top
                anchors.topMargin: 9
                text: root.cleanText(queue.title || "Operations Queue")
                color: root.textColor
                font.pixelSize: 18
                font.weight: Font.DemiBold
                Accessible.role: Accessible.Heading
                Accessible.name: text
            }

            Row {
                anchors.left: parent.left
                anchors.leftMargin: root.contentSpacing
                anchors.bottom: parent.bottom
                anchors.bottomMargin: 8
                spacing: 15

                QueueSummaryItem {
                    objectName: "operationsQueueSummary-running"
                    statusName: "running"
                    iconName: "circle-play"
                    count: Number(queue.runningCount || 0)
                    accent: root.dialogAccent
                }

                QueueSummaryItem {
                    objectName: "operationsQueueSummary-queued"
                    statusName: "queued"
                    iconName: "clock-3"
                    count: Number(queue.queuedCount || 0)
                    accent: "#d9b866"
                }

                QueueSummaryItem {
                    objectName: "operationsQueueSummary-completed"
                    statusName: "completed"
                    iconName: "circle-check"
                    count: Number(queue.completedCount || 0)
                    accent: "#75c991"
                }

                QueueSummaryItem {
                    objectName: "operationsQueueSummary-errors"
                    statusName: "errors"
                    iconName: "triangle-alert"
                    count: Number(queue.errorCount || 0)
                    accent: "#ee6a6a"
                    visible: count > 0
                }
            }

            QueueActionButton {
                id: headerClearButton
                objectName: "operationsQueueClearButton"
                iconName: "trash-2"
                anchors.right: parent.right
                anchors.rightMargin: root.contentSpacing
                anchors.verticalCenter: parent.verticalCenter
                text: root.cleanText(queue.clearText || "Clear completed")
                enabled: queue.canClear === true
                Accessible.name: text
                Accessible.description: root.cleanText(queue.clearDescription)
                onClicked: queueRoot.clearCompleted()
            }
        }

        Rectangle {
            id: columnsHeader
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: queueChrome.bottom
            height: 34
            color: root.titleBarBg

            Text {
                x: root.contentSpacing
                width: queueRoot.idColumnWidth
                anchors.verticalCenter: parent.verticalCenter
                text: queueRoot.columnTitle("id", "ID")
                visible: width > 0
                color: root.chromeText
                font.pixelSize: 12
                font.weight: Font.DemiBold
            }
            Text {
                x: root.contentSpacing + queueRoot.idColumnWidth
                width: queueRoot.stateColumnWidth
                anchors.verticalCenter: parent.verticalCenter
                text: queueRoot.columnTitle("state", "State")
                color: root.chromeText
                font.pixelSize: 12
                font.weight: Font.DemiBold
            }
            Text {
                x: root.contentSpacing + queueRoot.idColumnWidth
                   + queueRoot.stateColumnWidth
                width: queueRoot.typeColumnWidth
                anchors.verticalCenter: parent.verticalCenter
                text: queueRoot.columnTitle("type", "Type")
                color: root.chromeText
                font.pixelSize: 12
                font.weight: Font.DemiBold
            }
            Text {
                anchors.left: parent.left
                anchors.leftMargin: root.contentSpacing
                                    + queueRoot.idColumnWidth
                                    + queueRoot.stateColumnWidth
                                    + queueRoot.typeColumnWidth
                anchors.right: progressHeader.left
                anchors.rightMargin: 8
                anchors.verticalCenter: parent.verticalCenter
                text: queueRoot.columnTitle("description",
                                            "Description / Current File")
                color: root.chromeText
                font.pixelSize: 12
                font.weight: Font.DemiBold
                elide: Text.ElideRight
            }
            Text {
                id: progressHeader
                anchors.right: speedHeader.left
                width: queueRoot.progressColumnWidth
                anchors.verticalCenter: parent.verticalCenter
                text: queueRoot.columnTitle("progress", "Progress")
                color: root.chromeText
                font.pixelSize: 12
                font.weight: Font.DemiBold
            }
            Text {
                id: speedHeader
                anchors.right: parent.right
                anchors.rightMargin: root.contentSpacing
                width: queueRoot.speedColumnWidth
                anchors.verticalCenter: parent.verticalCenter
                text: queueRoot.columnTitle("speed", "Speed")
                visible: width > 0
                color: root.chromeText
                font.pixelSize: 12
                font.weight: Font.DemiBold
            }
        }

        ListView {
            id: operationsQueueList
            objectName: "operationsQueueList"
            anchors.left: parent.left
            anchors.right: queueScrollBar.visible
                           ? queueScrollBar.left : parent.right
            anchors.top: columnsHeader.bottom
            anchors.bottom: queueFooter.top
            clip: true
            model: queueRowsModel
            reuseItems: true
            boundsBehavior: Flickable.StopAtBounds
            flickableDirection: Flickable.VerticalFlick
            interactive: queueRoot.interactionActive
            keyNavigationEnabled: false
            Accessible.role: Accessible.List
            Accessible.name: root.cleanText(queue.title || "Operations Queue")

            delegate: Rectangle {
                id: queueRow
                required property string stableId
                required property int taskId
                required property int itemIndex
                required property string taskType
                required property string description
                required property string state
                required property string stateClass
                required property string currentFile
                required property string displayText
                required property real currentProgress
                required property real progress
                required property string totalText
                required property string speed
                required property string error
                required property bool cancellable
                required property bool hasDetails
                required property bool terminal
                required property bool active
                required property int index
                readonly property bool current:
                    taskId === queueRoot.localSelectedTaskId
                readonly property bool totalProgressKnown:
                    progress >= 0 && stateClass !== "scanning"
                objectName: "operationsQueueRow-" + taskId
                width: operationsQueueList.width
                height: queueRoot.rowHeight
                color: current ? root.panelSelectionBg
                               : rowHover.hovered
                                 ? root.controlHoverBg
                                 : index % 2 === 0
                                   ? root.dialogBg : root.windowBackgroundColor
                border.width: current ? 1 : 0
                border.color: root.panelSelectionBorder
                Accessible.role: Accessible.ListItem
                Accessible.name: state + ", " + taskType + ", "
                                 + (displayText !== "" ? displayText
                                                       : description)
                Accessible.description: error !== "" ? error : totalText
                Accessible.selected: current
                Accessible.focusable: false
                Accessible.onPressAction: {
                    queueRoot.selectIndex(index, true)
                    queueRoot.activateItem(queueRowsModel.get(index))
                }

                HoverHandler { id: rowHover }

                Text {
                    x: root.contentSpacing
                    width: queueRoot.idColumnWidth - 8
                    anchors.verticalCenter: parent.verticalCenter
                    text: String(queueRow.taskId)
                    visible: queueRoot.idColumnWidth > 0
                    color: root.mutedText
                    font.family: root.guiMonospaceFontFamily
                    font.pixelSize: 12
                }

                Item {
                    x: root.contentSpacing + queueRoot.idColumnWidth
                    width: queueRoot.stateColumnWidth - 8
                    height: parent.height
                    IconLabel {
                        anchors.left: parent.left
                        anchors.verticalCenter: parent.verticalCenter
                        width: 14
                        height: 14
                        icon.source: root.lucideIconSource(
                                         queueRoot.stateIconName(
                                             queueRow.stateClass,
                                             queueRow.state),
                                         14,
                                         queueRoot.stateColor(
                                             queueRow.stateClass,
                                             queueRow.state))
                        icon.width: 14
                        icon.height: 14
                        icon.color: queueRoot.stateColor(queueRow.stateClass,
                                                         queueRow.state)
                    }
                    Text {
                        anchors.left: parent.left
                        anchors.leftMargin: 15
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        text: queueRow.state
                        color: queueRoot.stateColor(queueRow.stateClass,
                                                    queueRow.state)
                        elide: Text.ElideRight
                        font.pixelSize: 12
                        font.weight: queueRow.active
                                     ? Font.DemiBold : Font.Normal
                    }
                }

                Text {
                    x: root.contentSpacing + queueRoot.idColumnWidth
                       + queueRoot.stateColumnWidth
                    width: queueRoot.typeColumnWidth - 8
                    anchors.verticalCenter: parent.verticalCenter
                    text: queueRow.taskType
                    color: root.textColor
                    elide: Text.ElideRight
                    font.pixelSize: 12
                }

                Item {
                    anchors.left: parent.left
                    anchors.leftMargin: root.contentSpacing
                                        + queueRoot.idColumnWidth
                                        + queueRoot.stateColumnWidth
                                        + queueRoot.typeColumnWidth
                    anchors.right: progressCell.left
                    anchors.rightMargin: 8
                    height: parent.height

                    Text {
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        anchors.verticalCenterOffset:
                            (queueRow.error !== ""
                             || (queueRow.currentFile !== ""
                                 && queueRow.currentFile
                                    !== queueRow.displayText)) ? -9 : 0
                        text: queueRow.displayText !== ""
                              ? queueRow.displayText : queueRow.description
                        color: queueRow.error !== ""
                               ? "#ee8b8b" : root.textColor
                        elide: Text.ElideMiddle
                        font.pixelSize: 13
                    }
                    Text {
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.bottom: parent.bottom
                        anchors.bottomMargin: 7
                        text: queueRow.error !== ""
                              ? queueRow.error : queueRow.currentFile
                        visible: text !== ""
                                 && text !== queueRow.displayText
                        color: queueRow.error !== ""
                               ? "#ee8b8b" : root.mutedText
                        elide: Text.ElideMiddle
                        font.pixelSize: 11
                    }
                }

                Item {
                    id: progressCell
                    anchors.right: speedCell.left
                    width: queueRoot.progressColumnWidth
                    height: parent.height

                    Rectangle {
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.rightMargin: 18
                        anchors.verticalCenter: parent.verticalCenter
                        anchors.verticalCenterOffset: -6
                        height: 6
                        radius: 3
                        color: root.controlBg
                        visible: queueRow.totalProgressKnown
                        Rectangle {
                            width: parent.width * queueRow.progress / 100
                            height: parent.height
                            radius: parent.radius
                            color: queueRoot.stateColor(queueRow.stateClass,
                                                        queueRow.state)
                        }
                        Accessible.role: Accessible.ProgressBar
                        Accessible.name: queueRow.taskType
                    }
                    Text {
                        anchors.left: parent.left
                        anchors.leftMargin: rowBusy.visible ? 28 : 0
                        anchors.right: parent.right
                        anchors.rightMargin: 18
                        anchors.top: parent.verticalCenter
                        anchors.topMargin: 3
                        text: queueRow.totalProgressKnown
                              ? Math.round(queueRow.progress) + "%"
                                + (queueRow.totalText !== ""
                                   ? "  " + queueRow.totalText : "")
                              : queueRow.totalText
                        color: root.mutedText
                        elide: Text.ElideRight
                        font.pixelSize: 11
                    }
                    T.BusyIndicator {
                        id: rowBusy
                        anchors.left: parent.left
                        anchors.verticalCenter: parent.verticalCenter
                        width: 22
                        height: 22
                        visible: queueRow.active
                                 && !queueRow.totalProgressKnown
                        running: visible
                        Accessible.name: queueRow.state
                    }
                }

                Item {
                    id: speedCell
                    anchors.right: parent.right
                    anchors.rightMargin: root.contentSpacing
                    width: queueRoot.speedColumnWidth
                    height: parent.height
                    visible: width > 0
                    Text {
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        text: queueRow.speed
                        color: root.mutedText
                        elide: Text.ElideRight
                        font.pixelSize: 12
                    }
                }

                MouseArea {
                    anchors.fill: parent
                    acceptedButtons: Qt.LeftButton
                    hoverEnabled: false
                    preventStealing: false
                    scrollGestureEnabled: false
                    cursorShape: Qt.PointingHandCursor
                    onClicked: queueRoot.selectIndex(index, true)
                    onDoubleClicked: {
                        queueRoot.selectIndex(index, true)
                        queueRoot.activateItem(queueRowsModel.get(index))
                    }
                }
            }

            WheelHandler {
                enabled: queueRoot.interactionActive
                acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
                target: null
                onWheel: (event) => {
                    var delta = Number(event.pixelDelta.y)
                    if (delta === 0)
                        delta = Number(event.angleDelta.y) / 120
                                * queueRoot.rowHeight * 2
                    var maximum = Math.max(0,
                                           operationsQueueList.contentHeight
                                           - operationsQueueList.height)
                    operationsQueueList.contentY = Math.max(0,
                        Math.min(maximum,
                                 operationsQueueList.contentY - delta))
                    event.accepted = true
                }
            }
        }

        T.ScrollBar {
            id: queueScrollBar
            objectName: "operationsQueueScrollBar"
            anchors.top: operationsQueueList.top
            anchors.bottom: operationsQueueList.bottom
            anchors.right: parent.right
            enabled: queueRoot.interactionActive
            policy: operationsQueueList.contentHeight
                    > operationsQueueList.height
                    ? T.ScrollBar.AlwaysOn : T.ScrollBar.AlwaysOff
            orientation: Qt.Vertical
            size: operationsQueueList.contentHeight > 0
                  ? Math.min(1, operationsQueueList.height
                             / operationsQueueList.contentHeight) : 1
            position: operationsQueueList.contentHeight
                      > operationsQueueList.height
                      ? operationsQueueList.contentY
                        / operationsQueueList.contentHeight : 0
            onPositionChanged: {
                if (!pressed || operationsQueueList.contentHeight
                        <= operationsQueueList.height)
                    return
                operationsQueueList.contentY = position
                        * operationsQueueList.contentHeight
            }
            Accessible.name: root.cleanText(queue.scrollBarText
                                             || "Operations scroll bar")
        }

        Item {
            id: queueEmptyState
            objectName: "operationsQueueEmptyState"
            anchors.fill: operationsQueueList
            visible: queueRowsModel.count === 0
            Accessible.role: Accessible.StaticText
            Accessible.name: emptyLabel.text

            Column {
                anchors.centerIn: parent
                spacing: 8
                Text {
                    id: emptyLabel
                    anchors.horizontalCenter: parent.horizontalCenter
                    text: root.cleanText(queue.error) !== ""
                          ? root.cleanText(queue.error)
                          : root.cleanText(queue.emptyText || "No operations")
                    color: root.cleanText(queue.error) !== ""
                           ? "#ee8b8b" : root.mutedText
                    font.pixelSize: 16
                }
                Text {
                    anchors.horizontalCenter: parent.horizontalCenter
                    text: root.cleanText(queue.emptyDescription)
                    visible: text !== ""
                    color: root.mutedText
                    font.pixelSize: 12
                }
            }
        }

        Rectangle {
            id: queueFooter
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.bottomMargin: queueRoot.bottomInset
            height: 54
            color: root.titleBarBg
            border.width: 1
            border.color: root.separatorColor

            QueueActionButton {
                id: cancelButton
                objectName: "operationsQueueCancelButton"
                iconName: "circle-x"
                anchors.left: parent.left
                anchors.leftMargin: root.contentSpacing
                anchors.verticalCenter: parent.verticalCenter
                text: root.cleanText(queue.cancelText || "Cancel selected")
                enabled: {
                    var row = queueRoot.selectedItem()
                    return row !== null && row.cancellable === true
                }
                Accessible.name: text
                Accessible.description: root.cleanText(queue.cancelDescription)
                onClicked: queueRoot.cancelSelection()
            }

            Text {
                anchors.left: cancelButton.right
                anchors.leftMargin: 14
                anchors.right: parent.right
                anchors.rightMargin: root.contentSpacing
                anchors.verticalCenter: parent.verticalCenter
                text: root.cleanText(queue.detailsText)
                visible: text !== ""
                color: root.mutedText
                elide: Text.ElideRight
                font.pixelSize: 12
            }
        }
    }

    component DocumentSurface: Rectangle {
        id: documentRoot
        property string surfaceObjectName: "documentSurface"
        objectName: surfaceObjectName
        property var frame: ({})
        // Standalone F3/F4 owns the full application surface. Embedded
        // Quick View is already laid out between its own header and footer,
        // so applying the global menu/keybar insets there would double-pad it.
        property bool embedded: false
        property bool interactionActive: true
        property var displayedRows: []
        property bool windowInitialized: false
        property bool rebasingWindow: false
        property bool windowRequestPending: false
        property real requestedExtent: 0
        property real requestedFraction: 0
        property real requestedGeneration: 0
        property real resumeVelocity: 0
        property bool requestPreservesLiveAnchor: true
        property bool wheelGestureActive: false
        property real stableTopExtent: 0
        property real stableTopFraction: 0
        property real lastViewportStart: -1
        property real wheelTarget: 0
        property real queuedScrollBarPosition: -1
        property string appliedWindowSignature: ""
        property string appliedDocumentKey: ""
        property bool initialPlacementPending: false
        property real initialPlacementExtent: 0
        property real initialPlacementFraction: 0
        property int loadedSlotStart: 0
        property int loadedSlotEnd: 0
        property int liveRowDelegateCount: 0
        property int poolSlotWriteCount: 0
        property int lastEditorMouseColumn: 0
        property int lastEditorMouseRow: 0
        property var pendingEditorMouseMove: null
        property var latestWindowRows: []
        readonly property var cursorFrame:
            root.documentSurfaceStateOverride !== null
            && root.cleanText(root.documentSurfaceStateOverride.id)
               === root.cleanText(frame.id)
            ? root.documentSurfaceStateOverride : frame
        readonly property bool kineticActive:
            documentList.flicking || documentList.dragging
            || documentWheelAnimation.running || wheelGestureActive
        readonly property bool hasWindowProtocol:
            root.cleanText(frame.scrollUnit) !== ""
            && frame.windowRows !== undefined
        readonly property string documentKey:
            root.cleanText(frame.documentKey)
        readonly property real contentExtent:
            Math.max(0, Number(frame.contentExtent || 0))
        readonly property bool contentExtentKnown:
            frame.contentExtentKnown !== false
        readonly property real topInset: embedded ? 0
                                           : semanticMenu.visible
                                             ? semanticMenu.height : 0
        readonly property real bottomInset: embedded ? 0
            : root.keyBarHeight()
        readonly property real rowHeight: Math.max(20, root.ch)

        function runBackground(value) {
            var background = root.cleanText(value).toLowerCase()
            var defaultBackground = root.cleanText(
                        frame.defaultBackground).toLowerCase()
            if ((defaultBackground !== ""
                    && background === defaultBackground)
                    || background === "#000000"
                    || background === "#ff000000"
                    || background === "black")
                return "transparent"
            return background !== "" ? value : "transparent"
        }

        function editorMouseButton(buttons) {
            if ((buttons & Qt.LeftButton) !== 0)
                return "left"
            if ((buttons & Qt.RightButton) !== 0)
                return "right"
            if ((buttons & Qt.MiddleButton) !== 0)
                return "middle"
            return "none"
        }

        function editorMouseAction(mouse, phase, moved, doubleClick) {
            if (frame.kind !== "editor")
                return null
            var cellWidth = Math.max(1,
                                     documentFontMetrics.advanceWidth("M"))
            var column = Math.max(0, Math.floor((mouse.x - 10) / cellWidth))
            // Resolve the row in ListView content coordinates.  contentY can
            // sit between delegate boundaries while a native scroll/rebase is
            // settling; treating mouse.y as if the first row always started
            // at viewport Y=0 then targets the preceding line.
            var modelIndex = Math.floor(
                        (documentList.contentY + mouse.y) / rowHeight)
            var windowIndex = modelIndex - loadedSlotStart
            var absoluteRow = windowIndex >= 0
                    && windowIndex < displayedRows.length
                    ? rowExtent(windowIndex) : Number(frame.viewportStart || 0)
                      + Math.floor(mouse.y / rowHeight)
            var row = Math.max(0, Math.floor(
                        absoluteRow - Number(frame.viewportStart || 0)))
            lastEditorMouseColumn = column
            lastEditorMouseRow = row
            var buttons = phase === "release" ? Qt.NoButton
                                               : (mouse.buttons || mouse.button)
            return {
                "target": root.cleanText(frame.id),
                "action": "editor.mouse",
                "phase": phase,
                "button": editorMouseButton(buttons),
                "column": column,
                "row": row,
                "moved": moved === true,
                "doubleClick": doubleClick === true,
                "shift": (mouse.modifiers & Qt.ShiftModifier) !== 0,
                "ctrl": (mouse.modifiers & Qt.ControlModifier) !== 0,
                "alt": (mouse.modifiers & Qt.AltModifier) !== 0
            }
        }

        function flushEditorMouseMove() {
            if (pendingEditorMouseMove === null)
                return
            var actionMap = pendingEditorMouseMove
            pendingEditorMouseMove = null
            root.action(actionMap, true)
        }

        function sendEditorMouse(mouse, phase, moved, doubleClick) {
            var actionMap = editorMouseAction(mouse, phase, moved,
                                              doubleClick)
            if (actionMap === null)
                return
            if (moved === true) {
                // Native pointer devices can deliver several drag samples in
                // one presentation interval. Only the newest endpoint can be
                // visible; collapse the rest before crossing into Go.
                pendingEditorMouseMove = actionMap
                editorMouseMoveTimer.restart()
                return
            }
            editorMouseMoveTimer.stop()
            flushEditorMouseMove()
            root.action(actionMap, true)
        }

        function releaseEditorMouse() {
            if (frame.kind !== "editor")
                return
            editorMouseMoveTimer.stop()
            flushEditorMouseMove()
            root.action({
                "target": root.cleanText(frame.id),
                "action": "editor.mouse",
                "phase": "release",
                "button": "none",
                "column": lastEditorMouseColumn,
                "row": lastEditorMouseRow
            }, true)
        }

        function sourceRows() {
            if (hasWindowProtocol)
                return frame.windowRows || []
            return frame.rows || []
        }

        function windowSignature(rows) {
            // Scene updates that only move the editor cursor or change another
            // part of the application must not replace the live ListView
            // roles. Go supplies a complete content hash so this hot path is
            // O(1), including for token-dense 4K scenes. JSON is retained only
            // for legacy producers that predate windowContentKey.
            var contentKey = root.cleanText(frame.windowContentKey)
            if (contentKey !== "")
                return root.cleanText(frame.windowStart) + ":"
                        + root.cleanText(frame.windowEnd) + ":" + contentKey
            return JSON.stringify(rows || [])
        }

        function rowSignature(row) {
            var source = row || ({})
            var contentKey = root.cleanText(source.contentKey)
            return contentKey !== "" ? contentKey : JSON.stringify(source)
        }

        function clamp(value, minimum, maximum) {
            return Math.max(minimum, Math.min(maximum, value))
        }

        function rowExtent(index, rows) {
            var source = rows || displayedRows
            if (!source || index < 0 || index >= source.length)
                return 0
            var row = source[index] || ({})
            return frame.scrollUnit === "rows"
                    ? Number(row.visualRow || 0)
                    : Number(row.offset || 0)
        }

        function rowEndExtent(index, rows) {
            var source = rows || displayedRows
            if (!source || index < 0 || index >= source.length)
                return rowExtent(index, source)
            var row = source[index] || ({})
            if (frame.scrollUnit === "rows")
                return Number(row.visualRow || 0) + 1
            var end = Number(row.endOffset || 0)
            if (end > Number(row.offset || 0))
                return end
            if (index + 1 < source.length)
                return Number((source[index + 1] || ({})).offset || 0)
            return Number(frame.windowEnd || row.offset || 0)
        }

        function topState() {
            if (!displayedRows || displayedRows.length === 0)
                return { "index": 0, "fraction": 0, "extent": 0 }
            var raw = Math.max(0, documentList.contentY) / rowHeight
                    - loadedSlotStart
            var index = clamp(Math.floor(raw), 0, displayedRows.length - 1)
            var fraction = clamp(raw - index, 0, 0.999999)
            var start = rowExtent(index)
            var end = rowEndExtent(index)
            return {
                "index": index,
                "fraction": fraction,
                "extent": start + (end - start) * fraction
            }
        }

        function extentAtContentY(contentY) {
            if (!displayedRows || displayedRows.length === 0)
                return 0
            var raw = clamp(Number(contentY || 0) / rowHeight
                            - loadedSlotStart,
                            0, displayedRows.length)
            if (raw >= displayedRows.length)
                return rowEndExtent(displayedRows.length - 1)
            var index = Math.floor(raw)
            var fraction = raw - index
            var start = rowExtent(index)
            var end = rowEndExtent(index)
            return start + (end - start) * fraction
        }

        function captureTopState() {
            if (rebasingWindow || !windowInitialized)
                return
            var state = topState()
            stableTopExtent = state.extent
            stableTopFraction = state.fraction
        }

        function indexForExtent(extent, rows) {
            var source = rows || displayedRows
            if (!source || source.length === 0)
                return -1
            for (var i = 0; i < source.length; ++i) {
                var start = rowExtent(i, source)
                var end = rowEndExtent(i, source)
                if (Math.abs(start - extent) < 0.000001
                        || (extent >= start && extent < end))
                    return i
            }
            return -1
        }

        function emptyPoolSlot() {
            return { "loaded": false, "rowData": ({}) }
        }

        function setPoolSlot(slot, row) {
            if (slot < 0 || slot >= documentRowsModel.count)
                return false
            var current = documentRowsModel.get(slot)
            if (current.loaded === true
                    && rowSignature(current.rowData)
                       === rowSignature(row || ({})))
                return false
            documentRowsModel.set(slot, {
                "loaded": true,
                "rowData": row || ({})
            })
            ++poolSlotWriteCount
            return true
        }

        function clearPoolSlot(slot) {
            if (slot < 0 || slot >= documentRowsModel.count)
                return false
            if (documentRowsModel.get(slot).loaded !== true)
                return false
            documentRowsModel.set(slot, emptyPoolSlot())
            ++poolSlotWriteCount
            return true
        }

        function poolCapacityFor(rows) {
            var viewportRows = Math.max(
                        1, Math.ceil(documentList.height / rowHeight))
            // Twelve viewports leave 4.5 viewports of stable slots on each
            // side of the normal three-viewport semantic window.  That is
            // longer than Qt's maximum native ballistic travel at the
            // configured velocity/deceleration, while ListView still creates
            // delegates only around the visible viewport.
            return Math.max(120, viewportRows * 12,
                            (rows ? rows.length : 0) + viewportRows * 6)
        }

        function ensurePoolCapacity(capacity) {
            while (documentRowsModel.count < capacity)
                documentRowsModel.append(emptyPoolSlot())
        }

        function clearLoadedSlots() {
            for (var slot = loadedSlotStart;
                 slot < loadedSlotEnd && slot < documentRowsModel.count;
                 ++slot)
                clearPoolSlot(slot)
        }

        function recenterRows(rows, extent, fraction, deferPlacement) {
            var source = rows || []
            ensurePoolCapacity(poolCapacityFor(source))
            clearLoadedSlots()
            var start = Math.max(0, Math.floor(
                         (documentRowsModel.count - source.length) / 2))
            for (var i = 0; i < source.length; ++i)
                setPoolSlot(start + i, source[i])
            displayedRows = source
            loadedSlotStart = start
            loadedSlotEnd = start + source.length

            if (deferPlacement) {
                initialPlacementExtent = extent
                initialPlacementFraction = fraction
                initialPlacementPending = true
                initialPlacementTimer.restart()
            } else {
                placeAtExtent(extent, fraction)
            }
        }

        function mergeRowsWithoutRebase(nextRows, retainLiveUnion) {
            if (!displayedRows || displayedRows.length === 0) {
                recenterRows(nextRows, Number(frame.viewportStart || 0), 0,
                             false)
                return true
            }

            var oldIndex = -1
            var nextIndex = -1
            for (var n = 0; n < nextRows.length && nextIndex < 0; ++n) {
                var extent = rowExtent(n, nextRows)
                var found = indexForExtent(extent, displayedRows)
                if (found >= 0
                        && Math.abs(rowExtent(found, displayedRows) - extent)
                           < 0.000001) {
                    oldIndex = found
                    nextIndex = n
                }
            }
            if (oldIndex < 0)
                return false

            var nextStart = loadedSlotStart + oldIndex - nextIndex
            var nextEnd = nextStart + nextRows.length
            if (nextStart < 0 || nextEnd > documentRowsModel.count)
                return false

            var unionStart = Math.min(loadedSlotStart, nextStart)
            var unionEnd = Math.max(loadedSlotEnd, nextEnd)
            var unionRows = []
            // Model count and every physical row index remain unchanged. O(1)
            // row content keys make overlap updates cheap: selection changes
            // replace only their one or two affected delegates, while a
            // one-row edge scroll only fills its entering edge slot.
            for (var j = 0; j < nextRows.length; ++j) {
                var targetSlot = nextStart + j
                setPoolSlot(targetSlot, nextRows[j])
            }

            if (retainLiveUnion === true) {
                for (var slot = unionStart; slot < unionEnd; ++slot) {
                    var incoming = slot - nextStart
                    var existing = slot - loadedSlotStart
                    var hasIncoming = incoming >= 0
                            && incoming < nextRows.length
                    var hasExisting = existing >= 0
                            && existing < displayedRows.length
                    if (hasIncoming)
                        unionRows.push(nextRows[incoming])
                    else if (hasExisting)
                        unionRows.push(displayedRows[existing])
                    else
                        return false
                }
                displayedRows = unionRows
                loadedSlotStart = unionStart
                loadedSlotEnd = unionEnd
                return true
            }

            for (var oldSlot = loadedSlotStart;
                 oldSlot < loadedSlotEnd; ++oldSlot) {
                if (oldSlot < nextStart || oldSlot >= nextEnd)
                    clearPoolSlot(oldSlot)
            }
            displayedRows = nextRows
            loadedSlotStart = nextStart
            loadedSlotEnd = nextEnd
            return true
        }

        function compactWindowIfIdle() {
            if (kineticActive || wheelGestureActive || rebasingWindow
                    || !latestWindowRows || latestWindowRows.length === 0)
                return
            var state = topState()
            rebasingWindow = true
            recenterRows(latestWindowRows, state.extent, state.fraction, false)
            rebasingWindow = false
            captureTopState()
            syncScrollBar()
        }

        function minimumLoadedY() {
            return loadedSlotStart * rowHeight
        }

        function maximumLoadedY() {
            return Math.max(minimumLoadedY(),
                            loadedSlotEnd * rowHeight - documentList.height)
        }

        function frameReachesContentEnd() {
            if (!contentExtentKnown || contentExtent <= 0)
                return false
            var viewportEnd = Number(frame.viewportStart || 0)
                    + Number(frame.viewportSpan || 0)
            return viewportEnd >= contentExtent - 0.000001
        }

        function placeAtExtent(extent, fraction) {
            // The final semantic page may end between two physical row
            // boundaries. Placing its first row at y=0 would crop the last
            // row; at EOF anchor the loaded content's bottom instead.
            if (frameReachesContentEnd()) {
                documentList.contentY = maximumLoadedY()
                wheelTarget = documentList.contentY
                return
            }

            var index = indexForExtent(extent, displayedRows)
            if (index < 0)
                index = clamp(Number(frame.viewportRow || 0), 0,
                              Math.max(0, displayedRows.length - 1))
            documentList.contentY = (loadedSlotStart + index + fraction)
                    * rowHeight
            wheelTarget = documentList.contentY
        }

        function visibleExtentSpan() {
            if (!displayedRows || displayedRows.length === 0)
                return 0
            var top = Math.max(0, documentList.contentY)
            return Math.max(0, extentAtContentY(top + documentList.height)
                               - extentAtContentY(top))
        }

        function syncScrollBar() {
            if (!documentScrollBar.visible || documentScrollBar.pressed)
                return
            var extent = Math.max(1, contentExtent)
            var state = topState()
            var span = Math.max(0, visibleExtentSpan())
            documentScrollBar.size = clamp(span / extent, 0, 1)
            documentScrollBar.position = clamp(state.extent / extent,
                                                0, 1 - documentScrollBar.size)
        }

        function sendWindowRequest(extent, fraction, velocity,
                                   preserveLiveAnchor) {
            if (!interactionActive || !hasWindowProtocol
                    || windowRequestPending)
                return false
            var total = Math.max(0, contentExtent)
            var target = clamp(Number(extent || 0), 0, total)
            var current = Number(frame.viewportStart || 0)
            if (Math.abs(target - current) < 0.000001
                    && Math.abs(fraction) < 0.000001)
                return false

            windowRequestPending = true
            requestedExtent = target
            requestedFraction = clamp(Number(fraction || 0), 0, 0.999999)
            requestedGeneration = Number(frame.windowGeneration || 0) + 1
            resumeVelocity = Number(velocity || 0)
            requestPreservesLiveAnchor = preserveLiveAnchor !== false
            var actionMap = {
                "target": root.cleanText(frame.id),
                "action": root.cleanText(frame.scrollAction) !== ""
                          ? root.cleanText(frame.scrollAction)
                          : frame.kind === "editor"
                            ? "editor.scroll" : "viewer.scrollWindow"
            }
            if (documentKey !== "")
                actionMap.contentKey = documentKey
            actionMap.generation = requestedGeneration
            if (frame.scrollUnit === "rows")
                actionMap.visualRow = Math.floor(target)
            else
                actionMap.offset = Math.floor(target)
            root.action(actionMap, true)
            return true
        }

        function maybeRequestWindow() {
            if (!interactionActive || rebasingWindow || windowRequestPending
                    || !hasWindowProtocol
                    || !displayedRows || displayedRows.length === 0)
                return
            var state = topState()
            var visibleRows = Math.max(1, Math.ceil(documentList.height / rowHeight))
            var extraRows = Math.max(0, displayedRows.length - visibleRows)
            var threshold = Math.max(2, Math.floor(extraRows / 4))
            var rowsBefore = state.index
            var rowsAfter = displayedRows.length - state.index - visibleRows
            var atStart = state.extent <= 0.000001
            var atEnd = contentExtent > 0
                    && state.extent + visibleExtentSpan() >= contentExtent - 0.000001
            if ((rowsBefore <= threshold && !atStart)
                    || (rowsAfter <= threshold && !atEnd))
                sendWindowRequest(state.extent, state.fraction,
                                  documentList.verticalVelocity)
        }

        function applyFrameWindow() {
            var nextRows = sourceRows()
            var nextSignature = windowSignature(nextRows)
            var nextDocumentKey = documentKey
            var documentChanged = windowInitialized
                    && nextDocumentKey !== appliedDocumentKey
            if (documentChanged) {
                // A source-panel cursor move may replace a Quick View while a
                // wheel gesture/request from the old file is still live. Drop
                // every old-document transient atomically; overlapping row
                // numbers are unrelated and must never preserve the anchor.
                documentList.cancelFlick()
                documentWheelAnimation.stop()
                wheelCommitTimer.stop()
                initialPlacementTimer.stop()
                requestWindowTimer.stop()
                scrollBarRequestTimer.stop()
                editorMouseMoveTimer.stop()
                windowRequestPending = false
                requestedExtent = 0
                requestedFraction = 0
                requestedGeneration = 0
                resumeVelocity = 0
                requestPreservesLiveAnchor = true
                wheelGestureActive = false
                queuedScrollBarPosition = -1
                pendingEditorMouseMove = null
                appliedWindowSignature = ""
                lastViewportStart = -1
                windowInitialized = false
            }
            var wasInitialized = windowInitialized
            var oldState = windowInitialized ? topState()
                                             : { "extent": Number(frame.viewportStart || 0),
                                                 "fraction": 0 }
            var generation = Number(frame.windowGeneration || 0)
            var acknowledged = windowRequestPending
                    && generation >= requestedGeneration
            var viewportChanged = windowInitialized
                    && Number(frame.viewportStart || 0) !== lastViewportStart
            if (windowInitialized && !acknowledged && !viewportChanged
                    && nextSignature === appliedWindowSignature) {
                syncScrollBar()
                return
            }
            var targetExtent = oldState.extent
            var targetFraction = oldState.fraction
            if (!windowInitialized || viewportChanged) {
                targetExtent = Number(frame.viewportStart || 0)
                targetFraction = acknowledged ? requestedFraction : 0
            }
            if (acknowledged) {
                targetExtent = Number(frame.viewportStart || requestedExtent)
                targetFraction = requestedFraction
                if (requestPreservesLiveAnchor
                        && indexForExtent(oldState.extent, nextRows) >= 0) {
                    targetExtent = oldState.extent
                    targetFraction = oldState.fraction
                }
            }

            latestWindowRows = nextRows
            rebasingWindow = true
            var keepLiveCoordinates = wasInitialized && kineticActive
                    && acknowledged && requestPreservesLiveAnchor
            var mergedOverlap = wasInitialized
                    && mergeRowsWithoutRebase(nextRows,
                                              keepLiveCoordinates)
            if (!mergedOverlap) {
                recenterRows(nextRows, targetExtent, targetFraction,
                             !wasInitialized)
            } else if (!keepLiveCoordinates) {
                placeAtExtent(targetExtent, targetFraction)
            }
            appliedWindowSignature = nextSignature
            appliedDocumentKey = nextDocumentKey
            stableTopExtent = targetExtent
            stableTopFraction = targetFraction
            lastViewportStart = Number(frame.viewportStart || 0)
            rebasingWindow = false
            if (wasInitialized)
                windowInitialized = true
            if (acknowledged) {
                windowRequestPending = false
                resumeVelocity = 0
            }
            syncScrollBar()
            // ACKs received during native motion only fill preallocated model
            // slots.  Neither model count, item indices nor contentY changes,
            // so the original Qt kinetic timeline continues bit-for-bit.
            if (!windowRequestPending && queuedScrollBarPosition >= 0)
                scrollBarRequestTimer.restart()
            else
                requestWindowTimer.restart()
        }

        function handleWheel(wheel) {
            if (!interactionActive) {
                wheel.accepted = false
                return
            }
            wheelGestureActive = true
            wheelCommitTimer.restart()
            var pixelY = Number(wheel.pixelDelta.y || 0)
            var minY = minimumLoadedY()
            var maxY = maximumLoadedY()
            if (pixelY !== 0) {
                documentWheelAnimation.stop()
                documentList.contentY = clamp(documentList.contentY - pixelY,
                                              minY, maxY)
                wheelTarget = documentList.contentY
            } else {
                var steps = Number(wheel.angleDelta.y || 0) / 120
                var base = documentWheelAnimation.running
                         ? documentWheelAnimation.to : documentList.contentY
                wheelTarget = clamp(base - steps * rowHeight * 3, minY, maxY)
                documentWheelAnimation.stop()
                documentWheelAnimation.from = documentList.contentY
                documentWheelAnimation.to = wheelTarget
                documentWheelAnimation.restart()
            }
            wheel.accepted = true
        }

        function scheduleFrameWindowSync() {
            // The standalone document tree is prewarmed with an empty frame.
            // Leave its ListModel untouched until an actual viewer/editor is
            // attached; initializing a hidden, empty ListView can leave Qt's
            // content geometry stale when the surface is revealed later.
            if (root.cleanText(frame.kind) !== "")
                frameSyncTimer.restart()
        }

        onFrameChanged: scheduleFrameWindowSync()
        onInteractionActiveChanged: {
            if (interactionActive)
                return
            documentList.cancelFlick()
            documentWheelAnimation.stop()
            wheelCommitTimer.stop()
            requestWindowTimer.stop()
            scrollBarRequestTimer.stop()
            editorMouseMoveTimer.stop()
            initialPlacementTimer.stop()
            wheelGestureActive = false
            windowRequestPending = false
            requestedGeneration = 0
            requestPreservesLiveAnchor = true
            resumeVelocity = 0
            queuedScrollBarPosition = -1
            pendingEditorMouseMove = null
        }
        Component.onCompleted: scheduleFrameWindowSync()

        color: "transparent"

        FontMetrics {
            id: documentFontMetrics
            font.family: root.guiMonospaceFontFamily
            font.pixelSize: 13
        }

        ListModel {
            id: documentRowsModel
            objectName: "documentRowsModel"
            dynamicRoles: true
        }

        ListView {
            id: documentList
            objectName: "documentList"
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.topMargin: documentRoot.topInset
            anchors.bottom: parent.bottom
            anchors.bottomMargin: documentRoot.bottomInset
            clip: true
            model: documentRowsModel
            interactive: documentRoot.interactionActive
            boundsBehavior: Flickable.StopAtBounds
            reuseItems: true
            cacheBuffer: documentRoot.rowHeight * 2

            delegate: Rectangle {
                id: documentRow
                objectName: "documentRowDelegate"
                required property bool loaded
                required property var rowData
                property bool countedAsLive: true
                width: ListView.view.width
                height: documentRoot.rowHeight
                color: "transparent"
                Component.onCompleted: ++documentRoot.liveRowDelegateCount
                Component.onDestruction: {
                    if (countedAsLive)
                        --documentRoot.liveRowDelegateCount
                }
                ListView.onPooled: {
                    if (countedAsLive) {
                        countedAsLive = false
                        --documentRoot.liveRowDelegateCount
                    }
                }
                ListView.onReused: {
                    if (!countedAsLive) {
                        countedAsLive = true
                        ++documentRoot.liveRowDelegateCount
                    }
                }

                Row {
                    id: runRow
                    anchors.left: parent.left
                    anchors.leftMargin: 10
                    height: parent.height
                    visible: documentRow.loaded
                             && documentRow.rowData.runs !== undefined
                             && documentRow.rowData.runs.length > 0

                    Repeater {
                        model: documentRow.loaded
                               ? documentRow.rowData.runs || [] : []

                        delegate: Rectangle {
                            height: runRow.height
                            width: runLabel.implicitWidth
                            color: documentRoot.runBackground(
                                       modelData.background)

                            Text {
                                id: runLabel
                                anchors.verticalCenter: parent.verticalCenter
                                text: root.cleanText(modelData.text)
                                color: root.cleanText(modelData.foreground) !== ""
                                       ? modelData.foreground : root.textColor
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 13
                                font.bold: modelData.bold === true
                                font.underline: modelData.underline === true
                                font.strikeout: modelData.strikeout === true
                            }
                        }
                    }
                }

                Text {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    anchors.leftMargin: 10
                    anchors.rightMargin: 10
                    visible: documentRow.loaded
                             && (!documentRow.rowData.runs
                                 || documentRow.rowData.runs.length === 0)
                    text: root.rowText(documentRow.rowData)
                    color: root.textColor
                    font.family: root.guiMonospaceFontFamily
                    font.pixelSize: 13
                    elide: Text.ElideRight
                }
            }

            onContentYChanged: {
                if (!documentRoot.rebasingWindow
                        && documentRoot.windowInitialized) {
                    var bounded = documentRoot.clamp(
                                contentY, documentRoot.minimumLoadedY(),
                                documentRoot.maximumLoadedY())
                    if (Math.abs(bounded - contentY) > 0.001) {
                        contentY = bounded
                        return
                    }
                }
                documentRoot.captureTopState()
                documentRoot.syncScrollBar()
                requestWindowTimer.restart()
            }
            onMovementEnded: {
                if (documentRoot.rebasingWindow
                        || documentRoot.wheelGestureActive)
                    return
                documentRoot.compactWindowIfIdle()
                var state = documentRoot.topState()
                if (!documentRoot.sendWindowRequest(state.extent,
                                                     state.fraction, 0))
                    requestWindowTimer.restart()
            }
        }

        Rectangle {
            id: editorCursor
            objectName: "editorCursor"
            parent: documentList.contentItem
            property bool blinkOn: true
            readonly property bool block:
                documentRoot.cursorFrame.cursorShape === "block"
            readonly property int windowRow:
                frame.kind === "editor"
                ? documentRoot.indexForExtent(
                      Number(documentRoot.cursorFrame.cursorAbsoluteRow || 0),
                                              documentRoot.displayedRows)
                : -1
            x: 10 + Math.max(0, Number(
                                 documentRoot.cursorFrame.cursorVisualColumn || 0))
                    * documentFontMetrics.advanceWidth("M")
            y: (documentRoot.loadedSlotStart + Math.max(0, windowRow))
               * documentRoot.rowHeight
               + (block ? 1 : 2)
            width: block ? Math.max(1, documentFontMetrics.advanceWidth("M"))
                         : 2
            height: documentRoot.rowHeight - (block ? 2 : 4)
            color: "#ffffff"
            opacity: blinkOn ? 1 : 0
            visible: frame.kind === "editor"
                     && documentRoot.cursorFrame.cursorVisible === true
                     && windowRow >= 0
                     && Number(documentRoot.cursorFrame.cursorVisualColumn) >= 0
            z: 5

            onVisibleChanged: {
                if (visible)
                    restartBlink()
            }

            function restartBlink() {
                blinkOn = true
                if (visible)
                    editorCursorBlinkTimer.restart()
            }

            Connections {
                target: root
                function onKeyboardActivityRevisionChanged() {
                    editorCursor.restartBlink()
                }
            }

            Timer {
                id: editorCursorBlinkTimer
                interval: 520
                running: editorCursor.visible
                repeat: true
                onTriggered: editorCursor.blinkOn = !editorCursor.blinkOn
            }
        }

        MouseArea {
            anchors.left: documentList.left
            anchors.right: documentScrollBar.visible
                           ? documentScrollBar.left : documentList.right
            anchors.top: documentList.top
            anchors.bottom: documentList.bottom
            acceptedButtons: frame.kind === "editor"
                             ? Qt.LeftButton | Qt.RightButton | Qt.MiddleButton
                             : Qt.NoButton
            preventStealing: frame.kind === "editor"
            propagateComposedEvents: true
            enabled: documentRoot.interactionActive
            cursorShape: frame.kind === "editor" ? Qt.IBeamCursor
                                                  : Qt.ArrowCursor
            z: 8
            onPressed: mouse => {
                documentRoot.sendEditorMouse(mouse, "press", false, false)
                mouse.accepted = true
            }
            onPositionChanged: mouse => {
                if (frame.kind === "editor" && mouse.buttons !== Qt.NoButton)
                    documentRoot.sendEditorMouse(mouse, "move", true, false)
            }
            onReleased: mouse => {
                documentRoot.sendEditorMouse(mouse, "release", false, false)
                mouse.accepted = true
            }
            onCanceled: documentRoot.releaseEditorMouse()
            onDoubleClicked: mouse => {
                documentRoot.sendEditorMouse(mouse, "press", false, true)
                mouse.accepted = true
            }
            // Wheel gestures stay in the native QML scrolling pipeline for
            // both viewers and editors.  Only button/drag selection events
            // need the canonical Go editor mouse handler.
            onWheel: wheel => documentRoot.handleWheel(wheel)
        }

        T.ScrollBar {
            id: documentScrollBar
            objectName: "documentScrollBar"
            parent: documentRoot
            anchors.top: documentList.top
            anchors.bottom: documentList.bottom
            anchors.right: documentList.right
            width: 15
            orientation: Qt.Vertical
            policy: T.ScrollBar.AlwaysOn
            hoverEnabled: true
            visible: documentRoot.hasWindowProtocol
                     && documentRoot.interactionActive
                     && documentRoot.contentExtentKnown
                     && documentRoot.contentExtent
                        > Math.max(0, Number(frame.viewportSpan || 0))
            z: 10

            contentItem: Rectangle {
                implicitWidth: 15
                implicitHeight: 15
                anchors.margins: 4
                radius: 4
                color: documentScrollBar.pressed ? "#505050"
                     : documentScrollBar.hovered ? "#676767" : "#4a4a4a"
            }
            background: Rectangle {
                color: documentScrollBar.hovered || documentScrollBar.pressed
                       ? Qt.rgba(1, 1, 1, 0.06) : "transparent"
                radius: 15
            }

            onPositionChanged: {
                if (!pressed)
                    return
                documentRoot.queuedScrollBarPosition = position
                scrollBarRequestTimer.restart()
            }
            onPressedChanged: {
                if (!pressed && documentRoot.queuedScrollBarPosition >= 0)
                    scrollBarRequestTimer.restart()
            }
        }

        NumberAnimation {
            id: documentWheelAnimation
            target: documentList
            property: "contentY"
            duration: 130
            easing.type: Easing.OutCubic
            onFinished: {
                requestWindowTimer.restart()
            }
        }

        Timer {
            id: wheelCommitTimer
            interval: 180
            onTriggered: {
                if (documentWheelAnimation.running) {
                    restart()
                    return
                }
                documentRoot.wheelGestureActive = false
                documentRoot.compactWindowIfIdle()
                var state = documentRoot.topState()
                if (!documentRoot.sendWindowRequest(state.extent,
                                                     state.fraction, 0))
                    requestWindowTimer.restart()
            }
        }

        Timer {
            id: frameSyncTimer
            interval: 0
            onTriggered: documentRoot.applyFrameWindow()
        }

        Timer {
            id: editorMouseMoveTimer
            interval: 0
            onTriggered: documentRoot.flushEditorMouseMove()
        }

        Timer {
            id: initialPlacementTimer
            interval: 0
            onTriggered: {
                if (!documentRoot.initialPlacementPending)
                    return
                documentRoot.rebasingWindow = true
                documentRoot.placeAtExtent(
                            documentRoot.initialPlacementExtent,
                            documentRoot.initialPlacementFraction)
                documentRoot.rebasingWindow = false
                documentRoot.initialPlacementPending = false
                documentRoot.windowInitialized = true
                documentRoot.captureTopState()
                documentRoot.syncScrollBar()
            }
        }

        Timer {
            id: requestWindowTimer
            interval: 12
            onTriggered: documentRoot.maybeRequestWindow()
        }

        Timer {
            id: scrollBarRequestTimer
            // Coalesce all thumb changes posted in the current Qt event turn,
            // then start immediately. There is still at most one
            // authoritative request in flight; its ACK restarts this timer
            // with only the newest retained position.
            interval: 0
            onTriggered: {
                if (documentRoot.queuedScrollBarPosition < 0)
                    return
                // The ACK path restarts us with the newest retained thumb
                // position. Never poll or replay stale intermediate points.
                if (documentRoot.windowRequestPending)
                    return
                var position = documentRoot.queuedScrollBarPosition
                documentRoot.queuedScrollBarPosition = -1
                documentRoot.sendWindowRequest(position
                                               * documentRoot.contentExtent,
                                               0, 0, false)
            }
        }
    }

    component DialogButton: T.Button {
        id: dialogButton
        property bool semanticFocus: false
        property string mnemonicHotkey: ""

        focusPolicy: Qt.NoFocus
        hoverEnabled: true
        padding: 0
        implicitHeight: 34
        implicitWidth: 84

        contentItem: Text {
            text: root.mnemonicText(dialogButton.text,
                                    dialogButton.mnemonicHotkey)
            textFormat: Text.StyledText
            color: dialogButton.enabled
                   ? (dialogButton.semanticFocus ? "#f4f8fc" : root.textColor)
                   : root.mutedText
            opacity: dialogButton.enabled ? 1 : 0.55
            font.pixelSize: 13
            font.weight: Font.Medium
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }

        background: Rectangle {
            radius: 4
            color: !dialogButton.enabled ? root.dialogBg
                   : dialogButton.down ? root.controlPressedBg
                   : dialogButton.hovered ? root.controlHoverBg
                   : dialogButton.semanticFocus ? "#274b68"
                   : root.controlBg
            border.width: 1
            border.color: dialogButton.semanticFocus
                          ? root.dialogAccent : root.controlBorder

            Behavior on color { ColorAnimation { duration: 90 } }
            Behavior on border.color { ColorAnimation { duration: 90 } }
        }
    }

    component QueueActionButton: DialogButton {
        id: queueActionButton
        property string iconName: ""
        readonly property bool f4Themed: true

        focusPolicy: Qt.StrongFocus
        semanticFocus: activeFocus
        implicitWidth: Math.max(108, queueActionContent.implicitWidth + 24)

        contentItem: Item {
            id: queueActionContent
            implicitWidth: queueActionRow.implicitWidth
            implicitHeight: queueActionRow.implicitHeight

            Row {
                id: queueActionRow
                anchors.centerIn: parent
                spacing: 7

                IconLabel {
                    visible: queueActionButton.iconName !== ""
                    width: visible ? 15 : 0
                    height: 15
                    anchors.verticalCenter: parent.verticalCenter
                    icon.source: root.lucideIconSource(
                                     queueActionButton.iconName, 15,
                                     queueActionButton.enabled
                                     ? (queueActionButton.semanticFocus
                                        ? "#f4f8fc" : root.textColor)
                                     : root.mutedText)
                    icon.width: 15
                    icon.height: 15
                    icon.color: queueActionButton.enabled
                                ? (queueActionButton.semanticFocus
                                   ? "#f4f8fc" : root.textColor)
                                : root.mutedText
                    opacity: queueActionButton.enabled ? 1 : 0.52
                }

                Text {
                    anchors.verticalCenter: parent.verticalCenter
                    text: root.mnemonicText(queueActionButton.text,
                                            queueActionButton.mnemonicHotkey)
                    textFormat: Text.StyledText
                    color: queueActionButton.enabled
                           ? (queueActionButton.semanticFocus
                              ? "#f4f8fc" : root.textColor)
                           : root.mutedText
                    opacity: queueActionButton.enabled ? 1 : 0.52
                    font.pixelSize: 13
                    font.weight: Font.Medium
                    elide: Text.ElideRight
                }
            }
        }
    }

    component QueueSummaryItem: Row {
        id: queueSummaryItem
        property string statusName: ""
        property string iconName: ""
        property int count: 0
        property color accent: root.mutedText
        readonly property string lucideName: iconName

        spacing: 5
        Accessible.role: Accessible.StaticText
        Accessible.name: statusName + ": " + count

        IconLabel {
            width: 14
            height: 14
            anchors.verticalCenter: parent.verticalCenter
            icon.source: root.lucideIconSource(
                             queueSummaryItem.iconName, 14,
                             queueSummaryItem.accent)
            icon.width: 14
            icon.height: 14
            icon.color: queueSummaryItem.accent
        }

        Text {
            anchors.verticalCenter: parent.verticalCenter
            text: String(queueSummaryItem.count)
            color: root.mutedText
            font.pixelSize: 12
        }
    }

    component DialogCheckBox: T.CheckBox {
        id: dialogCheck
        property bool semanticFocus: false
        property string mnemonicHotkey: ""

        focusPolicy: Qt.NoFocus
        hoverEnabled: true
        spacing: 9
        leftPadding: 0

        indicator: Rectangle {
            x: 0
            anchors.verticalCenter: parent.verticalCenter
            width: 18
            height: 18
            radius: 3
            color: dialogCheck.checked
                   ? root.dialogAccent
                   : dialogCheck.down ? root.controlPressedBg
                   : dialogCheck.hovered ? root.controlHoverBg
                   : root.controlBg
            border.width: 1
            border.color: dialogCheck.checked || dialogCheck.semanticFocus
                          ? root.dialogAccent : root.controlBorder

            Rectangle {
                anchors.centerIn: parent
                width: dialogCheck.tristate
                       && dialogCheck.checkState === Qt.PartiallyChecked ? 9 : 6
                height: dialogCheck.tristate
                        && dialogCheck.checkState === Qt.PartiallyChecked ? 2 : 6
                radius: 1
                visible: dialogCheck.checkState !== Qt.Unchecked
                color: root.dialogBg
            }

            Behavior on color { ColorAnimation { duration: 90 } }
            Behavior on border.color { ColorAnimation { duration: 90 } }
        }

        contentItem: Text {
            leftPadding: dialogCheck.indicator.width + dialogCheck.spacing
            text: root.mnemonicText(dialogCheck.text,
                                    dialogCheck.mnemonicHotkey)
            textFormat: Text.StyledText
            color: dialogCheck.enabled ? root.textColor : root.mutedText
            opacity: dialogCheck.enabled ? 1 : 0.55
            font.pixelSize: 13
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }
    }

    component DialogRadioButton: T.RadioButton {
        id: dialogRadio
        property bool semanticFocus: false
        property string mnemonicHotkey: ""

        focusPolicy: Qt.NoFocus
        hoverEnabled: true
        spacing: 9
        leftPadding: 0

        indicator: Rectangle {
            x: 0
            anchors.verticalCenter: parent.verticalCenter
            width: 18
            height: 18
            radius: 9
            color: dialogRadio.down ? root.controlPressedBg
                   : dialogRadio.hovered ? root.controlHoverBg
                   : root.controlBg
            border.width: 1
            border.color: dialogRadio.checked || dialogRadio.semanticFocus
                          ? root.dialogAccent : root.controlBorder

            Rectangle {
                anchors.centerIn: parent
                width: 8
                height: 8
                radius: 4
                visible: dialogRadio.checked
                color: root.dialogAccent
            }

            Behavior on color { ColorAnimation { duration: 90 } }
            Behavior on border.color { ColorAnimation { duration: 90 } }
        }

        contentItem: Text {
            leftPadding: dialogRadio.indicator.width + dialogRadio.spacing
            text: root.mnemonicText(dialogRadio.text,
                                    dialogRadio.mnemonicHotkey)
            textFormat: Text.StyledText
            color: dialogRadio.enabled ? root.textColor : root.mutedText
            font.pixelSize: 13
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }
    }

    component DialogProgressBar: T.ProgressBar {
        id: dialogProgress

        background: Rectangle {
            implicitHeight: 8
            radius: 4
            color: root.controlPressedBg
            border.width: 1
            border.color: root.controlBorder
        }

        contentItem: Item {
            implicitHeight: 8
            clip: true

            Rectangle {
                width: dialogProgress.visualPosition * parent.width
                height: parent.height
                radius: 4
                color: root.dialogAccent
            }
        }
    }

    component DialogTextField: Rectangle {
        id: dialogEdit
        required property var widget

        color: root.controlPressedBg
        border.width: 1
        border.color: widget.focused ? root.dialogAccent : root.controlBorder
        radius: 4
        clip: true

        Behavior on border.color { ColorAnimation { duration: 90 } }

        TextInput {
            id: dialogTextInput
            anchors.fill: parent
            anchors.leftMargin: 10
            anchors.rightMargin: 10
            readOnly: true
            focus: false
            text: root.cleanText(dialogEdit.widget.text)
            color: root.textColor
            selectionColor: root.selectedBg
            selectedTextColor: root.textColor
            font.pixelSize: 13
            verticalAlignment: TextInput.AlignVCenter
            echoMode: dialogEdit.widget.password
                      ? TextInput.Password : TextInput.Normal
            cursorPosition: Math.max(0, Math.min(length,
                                      Number(dialogEdit.widget.cursor || 0)))
            cursorVisible: dialogEdit.widget.focused === true

            cursorDelegate: Rectangle {
                id: dialogCursor
                property bool blinkOn: true
                width: 1
                color: root.textColor
                opacity: blinkOn ? 1 : 0

                Timer {
                    interval: 480
                    running: dialogTextInput.cursorVisible
                    repeat: true
                    onTriggered: dialogCursor.blinkOn = !dialogCursor.blinkOn
                }
            }
        }

        MouseArea {
            anchors.fill: parent
            acceptedButtons: Qt.LeftButton
            onPressed: root.action({
                "target": dialogEdit.widget.id,
                "action": "control.focus"
            })
        }
    }

    component DialogComboBox: T.ComboBox {
        id: dialogCombo
        required property var widget

        focusPolicy: Qt.NoFocus
        hoverEnabled: true
        padding: 0
        model: widget.items || []
        textRole: "text"
        currentIndex: Math.max(0, Number(widget.selected || 0))
        displayText: root.cleanText(widget.text)

        contentItem: Text {
            leftPadding: 10
            rightPadding: 32
            text: dialogCombo.displayText
            color: root.textColor
            font.pixelSize: 13
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }

        indicator: Text {
            x: dialogCombo.width - width - 11
            y: Math.round((dialogCombo.height - height) / 2) - 1
            text: "⌄"
            color: comboMouse.containsMouse ? root.textColor : root.mutedText
            font.pixelSize: 14
        }

        background: Rectangle {
            radius: 4
            color: comboMouse.pressed ? root.controlPressedBg
                   : comboMouse.containsMouse ? root.controlHoverBg
                   : root.controlBg
            border.width: 1
            border.color: dialogCombo.widget.focused
                          ? root.dialogAccent : root.controlBorder
            Behavior on color { ColorAnimation { duration: 90 } }
            Behavior on border.color { ColorAnimation { duration: 90 } }
        }

        popup: T.Popup {
            y: dialogCombo.height + 4
            width: dialogCombo.width
            implicitHeight: Math.min(contentItem.implicitHeight + 8, 240)
            padding: 4

            background: Rectangle {
                radius: 5
                color: root.dialogHeaderBg
                border.width: 1
                border.color: root.controlBorder
            }

            contentItem: ListView {
                clip: true
                implicitHeight: contentHeight
                model: dialogCombo.popup.visible
                       ? dialogCombo.delegateModel : null
                currentIndex: dialogCombo.highlightedIndex
                boundsBehavior: Flickable.StopAtBounds
            }
        }

        delegate: T.ItemDelegate {
            id: dialogComboItem
            required property int index
            required property var model
            width: dialogCombo.width - 8
            height: 30
            highlighted: dialogCombo.highlightedIndex === index

            contentItem: Text {
                leftPadding: 8
                text: root.mnemonicText(dialogComboItem.model.text,
                                        dialogComboItem.model.hotkey)
                textFormat: Text.StyledText
                color: root.textColor
                font.pixelSize: 13
                verticalAlignment: Text.AlignVCenter
                elide: Text.ElideRight
            }

            background: Rectangle {
                radius: 3
                color: dialogComboItem.highlighted
                       ? root.selectedBg
                       : dialogComboItem.hovered ? root.controlHoverBg
                       : "transparent"
            }
        }

        MouseArea {
            id: comboMouse
            anchors.fill: parent
            z: 2
            acceptedButtons: Qt.LeftButton
            hoverEnabled: true
            cursorShape: Qt.PointingHandCursor
            onClicked: {
                if (dialogCombo.popup.visible)
                    dialogCombo.popup.close()
                else
                    dialogCombo.popup.open()
            }
        }

        onActivated: (index) => root.action({
            "target": widget.id,
            "action": "control.select",
            "index": index
        })
    }

    component DialogResizeHandle: MouseArea {
        id: resizeHandle
        required property Item targetDialog
        property int edges: 0
        property point pressPoint: Qt.point(0, 0)
        property rect startGeometry: Qt.rect(0, 0, 0, 0)

        acceptedButtons: Qt.LeftButton
        hoverEnabled: true
        preventStealing: true
        cursorShape: {
            if (edges === 1 || edges === 2)
                return Qt.SizeHorCursor
            if (edges === 4 || edges === 8)
                return Qt.SizeVerCursor
            if (edges === 5 || edges === 10)
                return Qt.SizeFDiagCursor
            return Qt.SizeBDiagCursor
        }

        onPressed: function(mouse) {
            pressPoint = mapToItem(targetDialog.parent, mouse.x, mouse.y)
            startGeometry = Qt.rect(targetDialog.x, targetDialog.y,
                                    targetDialog.width, targetDialog.height)
            mouse.accepted = true
        }
        onPositionChanged: function(mouse) {
            if (!pressed)
                return
            const point = mapToItem(targetDialog.parent, mouse.x, mouse.y)
            targetDialog.resizeFrom(edges,
                                    point.x - pressPoint.x,
                                    point.y - pressPoint.y,
                                    startGeometry)
            mouse.accepted = true
        }
        onReleased: function(mouse) {
            targetDialog.commitGeometry()
            mouse.accepted = true
        }
    }

    component GenericDialog: Rectangle {
        id: dialogRoot
        objectName: "semanticDialog-" + root.cleanText(frame.id)
        property var frame: ({})
        property bool nativeLayout: root.isAppScene()
        property bool userGeometrySet: false
        property bool maximized: false
        property real userX: 0
        property real userY: 0
        property real userWidth: 0
        property real userHeight: 0
        property rect restoredGeometry: Qt.rect(0, 0, 0, 0)
        readonly property real bodyContentHeight: calculateBodyContentHeight()
        readonly property real geometryLeft: 12
        readonly property real geometryTop: semanticMenu.height + 8
        readonly property real geometryRight: root.width - 12
        readonly property real geometryBottom: root.height - 12
        readonly property real availableWidth: Math.max(
                                                   1, geometryRight - geometryLeft)
        readonly property real availableHeight: Math.max(
                                                    1, geometryBottom - geometryTop)
        readonly property real minimumDialogWidth: Math.min(320, availableWidth)
        readonly property real minimumDialogHeight: Math.min(160, availableHeight)
        readonly property real preferredWidth: nativeLayout
            ? Math.min(availableWidth, Math.max(420, root.pxW(frame.w || 60)))
            : Math.min(availableWidth, root.pxW(frame.w))
        readonly property real preferredHeight: nativeLayout
            ? Math.min(availableHeight, Math.max(180, root.pxH(frame.h)))
            : Math.min(availableHeight, root.pxH(frame.h))

        function clamped(value, minimum, maximum) {
            return Math.max(minimum, Math.min(maximum, value))
        }

        function setUserGeometry(nextX, nextY, nextWidth, nextHeight) {
            maximized = false
            userGeometrySet = true
            userWidth = clamped(nextWidth, minimumDialogWidth, availableWidth)
            userHeight = clamped(nextHeight, minimumDialogHeight,
                                 availableHeight)
            userX = clamped(nextX, geometryLeft,
                            Math.max(geometryLeft, geometryRight - userWidth))
            userY = clamped(nextY, geometryTop,
                            Math.max(geometryTop, geometryBottom - userHeight))
        }

        function moveTo(nextX, nextY) {
            if (maximized) {
                const restore = restoredGeometry
                setUserGeometry(nextX, nextY,
                                restore.width > 0 ? restore.width : preferredWidth,
                                restore.height > 0 ? restore.height : preferredHeight)
                return
            }
            setUserGeometry(nextX, nextY, width, height)
        }

        function resizeFrom(edges, deltaX, deltaY, start) {
            if (maximized)
                return

            let left = start.x
            let top = start.y
            let right = start.x + start.width
            let bottom = start.y + start.height
            if ((edges & 1) !== 0)
                left = clamped(start.x + deltaX, geometryLeft,
                               right - minimumDialogWidth)
            if ((edges & 2) !== 0)
                right = clamped(start.x + start.width + deltaX,
                                left + minimumDialogWidth, geometryRight)
            if ((edges & 4) !== 0)
                top = clamped(start.y + deltaY, geometryTop,
                              bottom - minimumDialogHeight)
            if ((edges & 8) !== 0)
                bottom = clamped(start.y + start.height + deltaY,
                                 top + minimumDialogHeight, geometryBottom)
            setUserGeometry(left, top, right - left, bottom - top)
        }

        function commitGeometry() {
            if (!frame || root.cleanText(frame.id) === "")
                return
            root.action({
                "target": frame.id,
                "action": "dialog.geometry",
                "x": Math.round(x / root.cw),
                "y": Math.round(y / root.ch),
                "w": Math.max(1, Math.round(width / root.cw)),
                "h": Math.max(1, Math.round(height / root.ch))
            }, true)
        }

        function toggleMaximized() {
            if (maximized) {
                const restore = restoredGeometry
                setUserGeometry(restore.x, restore.y,
                                restore.width, restore.height)
            } else {
                restoredGeometry = Qt.rect(x, y, width, height)
                maximized = true
            }
            Qt.callLater(commitGeometry)
        }

        function widgetBottom(widget) {
            if (!widget || widget.visible === false)
                return 0

            var bottom = root.pxY(Number(widget.y || 0) - Number(frame.y || 0) - 1)
                         + Math.max(22, root.pxH(Number(widget.h || 1)))
            var children = widget.children || []
            for (var i = 0; i < children.length; ++i)
                bottom = Math.max(bottom, widgetBottom(children[i]))
            return bottom
        }

        function calculateBodyContentHeight() {
            var bottom = 0
            var children = frame.children || []
            for (var i = 0; i < children.length; ++i)
                bottom = Math.max(bottom, widgetBottom(children[i]))
            return bottom + 14
        }

        function focusedWidget(widgets) {
            for (var i = 0; i < widgets.length; ++i) {
                var widget = widgets[i]
                if (!widget || widget.visible === false)
                    continue
                if (widget.focused === true)
                    return widget
                var nested = focusedWidget(widget.children || [])
                if (nested)
                    return nested
            }
            return null
        }

        function ensureFocusedWidgetVisible() {
            if (!dialogBody || dialogBody.height <= 0)
                return
            var widget = focusedWidget(frame.children || [])
            if (!widget)
                return

            var top = root.pxY(Number(widget.y || 0) - Number(frame.y || 0) - 1)
            var bottom = top + Math.max(22, root.pxH(Number(widget.h || 1)))
            var maximum = Math.max(0, dialogBody.contentHeight - dialogBody.height)
            if (top < dialogBody.contentY)
                dialogBody.contentY = Math.max(0, top - 6)
            else if (bottom > dialogBody.contentY + dialogBody.height)
                dialogBody.contentY = Math.min(maximum, bottom - dialogBody.height + 6)
        }

        onFrameChanged: Qt.callLater(ensureFocusedWidgetVisible)

        width: maximized ? availableWidth
                         : userGeometrySet
                           ? clamped(userWidth, minimumDialogWidth,
                                     availableWidth)
                           : preferredWidth
        height: maximized ? availableHeight
                          : userGeometrySet
                            ? clamped(userHeight, minimumDialogHeight,
                                      availableHeight)
                            : preferredHeight
        x: maximized ? geometryLeft
                     : userGeometrySet
                       ? clamped(userX, geometryLeft,
                                 Math.max(geometryLeft, geometryRight - width))
                       : clamped(nativeLayout
                                 ? Math.round((root.width - width) / 2)
                                 : root.pxX(frame.x),
                                 geometryLeft,
                                 Math.max(geometryLeft, geometryRight - width))
        y: maximized ? geometryTop
                     : userGeometrySet
                       ? clamped(userY, geometryTop,
                                 Math.max(geometryTop, geometryBottom - height))
                       : clamped(nativeLayout
                                 ? Math.round((root.height - height) / 2)
                                 : root.pxY(frame.y),
                                 geometryTop,
                                 Math.max(geometryTop, geometryBottom - height))
        color: root.dialogBg
        border.width: 1
        border.color: "#46586b"
        radius: 9
        clip: true

        MouseArea {
            anchors.fill: parent
            acceptedButtons: Qt.AllButtons
            hoverEnabled: true
            preventStealing: true
            onPressed: (mouse) => { mouse.accepted = true }
            onReleased: (mouse) => { mouse.accepted = true }
            onPositionChanged: (mouse) => { mouse.accepted = true }
            onWheel: (wheel) => { wheel.accepted = true }
        }

        Rectangle {
            id: dialogHeader
            objectName: "dialogMoveHandle"
            x: 1
            y: 1
            width: parent.width - 2
            height: 43
            color: root.dialogHeaderBg
            radius: 8

            Rectangle {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                height: parent.radius
                color: parent.color
            }

            Rectangle {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                height: 1
                color: root.separatorColor
                opacity: 0.55
            }

            MouseArea {
                id: dialogMoveArea
                anchors.fill: parent
                anchors.rightMargin: dialogWindowButtons.width
                acceptedButtons: Qt.LeftButton
                hoverEnabled: true
                preventStealing: true
                cursorShape: Qt.SizeAllCursor
                property point pressPoint: Qt.point(0, 0)
                property point startPosition: Qt.point(0, 0)

                onPressed: function(mouse) {
                    pressPoint = mapToItem(dialogRoot.parent, mouse.x, mouse.y)
                    startPosition = Qt.point(dialogRoot.x, dialogRoot.y)
                    mouse.accepted = true
                }
                onPositionChanged: function(mouse) {
                    if (!pressed)
                        return
                    const point = mapToItem(dialogRoot.parent, mouse.x, mouse.y)
                    dialogRoot.moveTo(startPosition.x + point.x - pressPoint.x,
                                      startPosition.y + point.y - pressPoint.y)
                    mouse.accepted = true
                }
                onReleased: function(mouse) {
                    dialogRoot.commitGeometry()
                    mouse.accepted = true
                }
                onDoubleClicked: function(mouse) {
                    dialogRoot.toggleMaximized()
                    mouse.accepted = true
                }
            }
        }

        Text {
            anchors.left: parent.left
            anchors.right: dialogWindowButtons.left
            anchors.top: parent.top
            height: dialogHeader.height
            anchors.leftMargin: 18
            verticalAlignment: Text.AlignVCenter
            text: root.cleanText(frame.title)
            color: root.textColor
            font.pixelSize: 14
            font.weight: Font.DemiBold
            elide: Text.ElideMiddle
        }

        Row {
            id: dialogWindowButtons
            anchors.top: dialogHeader.top
            anchors.right: parent.right
            height: dialogHeader.height
            spacing: 0

            ZG.TitleButton {
                id: dialogMaximizeButton
                objectName: "dialogMaximizeButton"
                implicitWidth: 42
                implicitHeight: dialogHeader.height
                opacity: 1
                source: dialogRoot.maximized
                        ? "qrc:/ZoinGallery/resources/WindowRestore.svg"
                        : "qrc:/ZoinGallery/resources/WindowMaximize.svg"
                icon.color: ZG.Style.text
                onClicked: dialogRoot.toggleMaximized()
            }

            ZG.TitleButton {
                id: dialogCloseButton
                objectName: "dialogCloseButton"
                implicitWidth: 42
                implicitHeight: dialogHeader.height
                opacity: 1
                visible: frame.showClose === true
                source: "qrc:/ZoinGallery/resources/WindowClose.svg"
                icon.color: hovered
                            ? ZG.Style.closeButtonHoveredIcon : ZG.Style.text
                backgroundColor: {
                    if (!enabled)
                        return "gray"
                    if (pressed)
                        return ZG.Style.closeButtonPressed
                    if (hovered)
                        return ZG.Style.closeButtonHovered
                    return "transparent"
                }
                onClicked: root.action({
                    "target": frame.id,
                    "action": "dialog.close"
                })
            }
        }

        Flickable {
            id: dialogBody
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.topMargin: dialogHeader.height + 10
            anchors.bottom: parent.bottom
            clip: true
            contentWidth: width
            contentHeight: Math.max(height, dialogRoot.bodyContentHeight)
            flickableDirection: Flickable.VerticalFlick
            boundsBehavior: Flickable.StopAtBounds
            boundsMovement: Flickable.StopAtBounds
            interactive: contentHeight > height

            onHeightChanged: contentY = Math.min(contentY, Math.max(0, contentHeight - height))
            onContentHeightChanged: contentY = Math.min(contentY, Math.max(0, contentHeight - height))

            T.ScrollBar.vertical: T.ScrollBar {
                policy: T.ScrollBar.AsNeeded
                width: 10
                rightPadding: 2
                contentItem: Rectangle {
                    implicitWidth: 5
                    radius: width / 2
                    color: parent.pressed || parent.hovered ? root.dialogAccent : root.mutedText
                    opacity: parent.active ? 0.85 : 0.55
                    Behavior on color { ColorAnimation { duration: 90 } }
                    Behavior on opacity { NumberAnimation { duration: 90 } }
                }
                background: Item { }
            }

            Item {
                width: dialogBody.width
                height: dialogBody.contentHeight

                Repeater {
                    model: frame.children || []
                    delegate: WidgetDelegate {
                        widget: modelData
                        originX: frame.x || 0
                        originY: frame.y || 0
                        maximumWidth: Math.max(1, dialogBody.width
                                              - root.pxX((modelData.x || 0)
                                                         - (frame.x || 0))
                                              - 16)
                    }
                }
            }
        }

        DialogResizeHandle {
            targetDialog: dialogRoot
            edges: 1
            x: 0
            y: 9
            width: 6
            height: Math.max(1, dialogRoot.height - 18)
            z: 500
            visible: !dialogRoot.maximized
        }
        DialogResizeHandle {
            targetDialog: dialogRoot
            edges: 2
            x: dialogRoot.width - width
            y: 9
            width: 6
            height: Math.max(1, dialogRoot.height - 18)
            z: 500
            visible: !dialogRoot.maximized
        }
        DialogResizeHandle {
            targetDialog: dialogRoot
            edges: 4
            x: 9
            y: 0
            width: Math.max(1, dialogRoot.width - 18)
            height: 6
            z: 500
            visible: !dialogRoot.maximized
        }
        DialogResizeHandle {
            targetDialog: dialogRoot
            edges: 8
            x: 9
            y: dialogRoot.height - height
            width: Math.max(1, dialogRoot.width - 18)
            height: 6
            z: 500
            visible: !dialogRoot.maximized
        }
        DialogResizeHandle {
            objectName: "dialogResizeTopLeft"
            targetDialog: dialogRoot
            edges: 5
            x: 0
            y: 0
            width: 10
            height: 10
            z: 501
            visible: !dialogRoot.maximized
        }
        DialogResizeHandle {
            objectName: "dialogResizeTopRight"
            targetDialog: dialogRoot
            edges: 6
            x: dialogRoot.width - width
            y: 0
            width: 10
            height: 10
            z: 501
            visible: !dialogRoot.maximized
        }
        DialogResizeHandle {
            objectName: "dialogResizeBottomLeft"
            targetDialog: dialogRoot
            edges: 9
            x: 0
            y: dialogRoot.height - height
            width: 10
            height: 10
            z: 501
            visible: !dialogRoot.maximized
        }
        DialogResizeHandle {
            objectName: "dialogResizeBottomRight"
            targetDialog: dialogRoot
            edges: 10
            x: dialogRoot.width - width
            y: dialogRoot.height - height
            width: 10
            height: 10
            z: 501
            visible: !dialogRoot.maximized
        }
    }

    component WidgetDelegate: Item {
        id: widgetRoot
        property var widget: ({})
        property int originX: 0
        property int originY: 0
        property real maximumWidth: Number.POSITIVE_INFINITY

        x: root.pxX((widget.x || 0) - originX)
        y: root.pxY((widget.y || 0) - originY - 1)
        width: Math.min(root.pxW(widget.w || 1), maximumWidth)
        height: Math.max(22, root.pxH(widget.h || 1))
        visible: widget.visible !== false

        Loader {
            anchors.fill: parent
            sourceComponent: {
                switch (widget.kind) {
                case "button": return buttonDelegate
                case "checkbox": return checkboxDelegate
                case "edit": return editDelegate
                case "text": return textDelegate
                case "progressBar": return progressDelegate
                case "radioGroup": return choiceDelegate
                case "checkGroup": return choiceDelegate
                case "listBox": return listDelegate
                case "comboBox": return comboDelegate
                case "group": return groupDelegate
                default: return textDelegate
                }
            }
        }

        Component {
            id: textDelegate
            Text {
                text: root.mnemonicText(widget.text || widget.typeName,
                                        widget.hotkey)
                textFormat: Text.StyledText
                color: widget.disabled ? root.mutedText : root.textColor
                font.pixelSize: 13
                elide: Text.ElideRight
                verticalAlignment: Text.AlignVCenter
            }
        }

        Component {
            id: editDelegate
            DialogTextField {
                widget: widgetRoot.widget
            }
        }

        Component {
            id: buttonDelegate
            DialogButton {
                text: root.cleanText(widget.text)
                mnemonicHotkey: root.cleanText(widget.hotkey)
                enabled: widget.disabled !== true
                semanticFocus: widget.focused === true
                onClicked: root.action({ "target": widget.id, "action": "control.activate" })
            }
        }

        Component {
            id: checkboxDelegate
            DialogCheckBox {
                text: root.cleanText(widget.text)
                mnemonicHotkey: root.cleanText(widget.hotkey)
                checked: widget.state === 1
                tristate: widget.threeState === true
                enabled: widget.disabled !== true
                semanticFocus: widget.focused === true
                onClicked: root.action({ "target": widget.id, "action": "control.toggle" })
            }
        }

        Component {
            id: progressDelegate
            DialogProgressBar {
                from: 0
                to: 100
                value: widget.percent || 0
            }
        }

        Component {
            id: choiceDelegate
            Column {
                anchors.fill: parent
                spacing: 0

                Repeater {
                    model: widget.items || []
                    delegate: DialogRadioButton {
                        width: parent.width
                        height: widgetRoot.height
                                / Math.max(1, (widget.items || []).length)
                        text: root.cleanText(modelData)
                        checked: widget.kind === "radioGroup" ? index === widget.selected : !!(widget.states && widget.states[index])
                        semanticFocus: widget.focused === true
                                       && (index === widget.selected || widget.selected === undefined)
                        onClicked: root.action({ "target": widget.id, "action": "control.select", "index": index })
                    }
                }
            }
        }

        Component {
            id: listDelegate
            ListView {
                clip: true
                model: widget.items || []
                delegate: Rectangle {
                    id: listRow
                    width: ListView.view.width
                    height: Math.max(21, root.ch)
                    radius: 4
                    color: index === widget.cursor
                           ? root.selectedBg
                           : listMouse.containsMouse ? root.controlHoverBg
                           : "transparent"
                    Behavior on color { ColorAnimation { duration: 70 } }
                    Text {
                        anchors.fill: parent
                        anchors.leftMargin: 8
                        anchors.rightMargin: 8
                        text: root.mnemonicText(modelData, "")
                        textFormat: Text.StyledText
                        color: root.textColor
                        font.pixelSize: 13
                        verticalAlignment: Text.AlignVCenter
                        elide: Text.ElideRight
                    }
                    MouseArea {
                        id: listMouse
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: root.action({ "target": widget.id, "action": "control.select", "index": index })
                    }
                }
            }
        }

        Component {
            id: comboDelegate
            DialogComboBox {
                widget: widgetRoot.widget
            }
        }

        Component {
            id: groupDelegate
            Item {
                Rectangle {
                    anchors.fill: parent
                    color: "transparent"
                    border.width: widget.title ? 1 : 0
                    border.color: root.controlBorder
                    radius: 5
                }

                Rectangle {
                    anchors.left: parent.left
                    anchors.top: parent.top
                    anchors.leftMargin: 8
                    anchors.topMargin: -9
                    width: groupTitle.implicitWidth + 12
                    height: 18
                    radius: 3
                    color: root.dialogBg
                    visible: root.cleanText(widget.title) !== ""

                    Text {
                        id: groupTitle
                        anchors.centerIn: parent
                        text: root.mnemonicText(widget.title, widget.hotkey)
                        textFormat: Text.StyledText
                        color: root.mutedText
                        font.pixelSize: 12
                    }
                }

                Repeater {
                    model: widget.children || []
                    delegate: ShallowWidgetDelegate {
                        widget: modelData
                        originX: widgetRoot.originX
                        originY: widgetRoot.originY
                    }
                }
            }
        }
    }

    component ShallowWidgetDelegate: Item {
        id: shallowRoot
        property var widget: ({})
        property int originX: 0
        property int originY: 0

        x: root.pxX((widget.x || 0) - originX)
        y: root.pxY((widget.y || 0) - originY - 1)
        width: root.pxW(widget.w || 1)
        height: Math.max(22, root.pxH(widget.h || 1))
        visible: widget.visible !== false

        Loader {
            anchors.fill: parent
            sourceComponent: {
                switch (widget.kind) {
                case "button": return shallowButton
                case "checkbox": return shallowCheckbox
                case "edit": return shallowEdit
                case "progressBar": return shallowProgress
                case "radioGroup": return shallowChoice
                case "checkGroup": return shallowChoice
                case "listBox": return shallowList
                case "comboBox": return shallowCombo
                default: return shallowText
                }
            }
        }

        Component {
            id: shallowText
            Text {
                text: root.mnemonicText(widget.text || widget.title
                                        || widget.typeName, widget.hotkey)
                textFormat: Text.StyledText
                color: widget.disabled ? root.mutedText : root.textColor
                font.pixelSize: 13
                elide: Text.ElideRight
                verticalAlignment: Text.AlignVCenter
            }
        }

        Component {
            id: shallowEdit
            DialogTextField {
                widget: shallowRoot.widget
            }
        }

        Component {
            id: shallowButton
            DialogButton {
                text: root.cleanText(widget.text)
                mnemonicHotkey: root.cleanText(widget.hotkey)
                enabled: widget.disabled !== true
                semanticFocus: widget.focused === true
                onClicked: root.action({ "target": widget.id, "action": "control.activate" })
            }
        }

        Component {
            id: shallowCheckbox
            DialogCheckBox {
                text: root.cleanText(widget.text)
                mnemonicHotkey: root.cleanText(widget.hotkey)
                checked: widget.state === 1
                tristate: widget.threeState === true
                enabled: widget.disabled !== true
                semanticFocus: widget.focused === true
                onClicked: root.action({ "target": widget.id, "action": "control.toggle" })
            }
        }

        Component {
            id: shallowProgress
            DialogProgressBar {
                from: 0
                to: 100
                value: widget.percent || 0
            }
        }

        Component {
            id: shallowChoice
            Column {
                anchors.fill: parent
                spacing: 0

                Repeater {
                    model: widget.items || []
                    delegate: DialogRadioButton {
                        width: parent.width
                        height: shallowRoot.height
                                / Math.max(1, (widget.items || []).length)
                        text: root.cleanText(modelData)
                        checked: widget.kind === "radioGroup" ? index === widget.selected : !!(widget.states && widget.states[index])
                        semanticFocus: widget.focused === true
                                       && (index === widget.selected || widget.selected === undefined)
                        onClicked: root.action({ "target": widget.id, "action": "control.select", "index": index })
                    }
                }
            }
        }

        Component {
            id: shallowList
            ListView {
                clip: true
                model: widget.items || []
                delegate: Rectangle {
                    width: ListView.view.width
                    height: Math.max(21, root.ch)
                    radius: 4
                    color: index === widget.cursor
                           ? root.selectedBg
                           : shallowListMouse.containsMouse ? root.controlHoverBg
                           : "transparent"
                    Behavior on color { ColorAnimation { duration: 70 } }
                    Text {
                        anchors.fill: parent
                        anchors.leftMargin: 8
                        anchors.rightMargin: 8
                        text: root.mnemonicText(modelData, "")
                        textFormat: Text.StyledText
                        color: root.textColor
                        font.pixelSize: 13
                        verticalAlignment: Text.AlignVCenter
                        elide: Text.ElideRight
                    }
                    MouseArea {
                        id: shallowListMouse
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: root.action({ "target": widget.id, "action": "control.select", "index": index })
                    }
                }
            }
        }

        Component {
            id: shallowCombo
            DialogComboBox {
                widget: shallowRoot.widget
            }
        }
    }

    Component {
        id: dialogOverlayComponent
        Item {
            id: dialogOverlay
            property var frame: ({})

            Rectangle {
                anchors.fill: parent
                color: "#05080c"
                opacity: 0.58
            }

            MouseArea {
                anchors.fill: parent
                acceptedButtons: Qt.AllButtons
                hoverEnabled: true
                preventStealing: true
                onPressed: (mouse) => { mouse.accepted = true }
                onReleased: (mouse) => { mouse.accepted = true }
                onPositionChanged: (mouse) => { mouse.accepted = true }
                onWheel: (wheel) => { wheel.accepted = true }
            }

            GenericDialog {
                id: dialogSurface
                frame: parent.frame
            }
        }
    }

    Component {
        id: autocompletePopupComponent
        Item {
            id: autocompleteOverlay
            property var frame: ({})
            property var items: frame.items || []
            readonly property var commandLine: root.commandLineFrame()
            readonly property real commandLineX: root.isAppScene()
                                                     ? 0 : root.pxX(commandLine.x || 0)
            readonly property real commandLineY: root.isAppScene()
                                                     ? root.height - root.keyBarHeight()
                                                       - root.commandLineHeight(root.shellFrame())
                                                     : root.pxY(commandLine.y || 0)
            // CommandLine exports the authoritative Edit start column.  Its
            // prompt is monospaced, so translating that column with the same
            // Configured monospace metrics used by ConsoleRunRow land on the exact input x.
            readonly property real inputTextX: commandLineX
                                               + root.commandLineLeftMargin
                                               + Number(commandLine.inputX || 0)
                                                 * commandLineFontMetrics.advanceWidth("M")
            // ListView has a 4 px inset and each row has another 8 px text
            // inset. Offset the panel itself so the glyphs—not its border—
            // align with the command-line input.
            readonly property real preferredX: Math.max(0, inputTextX - 12)
            readonly property real rowHeight: Math.max(22, root.ch * 1.15)
            readonly property real maxHeight: Math.max(rowHeight + 8,
                                                       commandLineY - semanticMenu.height)
            readonly property real availableWidth: Math.max(1,
                                                             root.width - preferredX - 6)
            readonly property real contentWidth: {
                var widest = 0
                for (var i = 0; i < items.length; ++i)
                    widest = Math.max(widest, autocompleteFontMetrics.advanceWidth(
                                          root.cleanText(items[i].text)))
                return widest + 24
            }

            FontMetrics {
                id: autocompleteFontMetrics
                font.family: root.guiMonospaceFontFamily
                font.pixelSize: 13
            }

            FontMetrics {
                id: commandLineFontMetrics
                font.family: root.guiMonospaceFontFamily
                font.pixelSize: autocompleteOverlay.commandLine.runs
                                && autocompleteOverlay.commandLine.runs.length > 0
                                ? 13 : 18
            }

            MouseArea {
                anchors.fill: parent
                acceptedButtons: Qt.AllButtons
                hoverEnabled: true
                preventStealing: true
                onPressed: (mouse) => { mouse.accepted = true }
                onReleased: (mouse) => { mouse.accepted = true }
                onPositionChanged: (mouse) => { mouse.accepted = true }
                onWheel: (wheel) => { wheel.accepted = false }
            }

            Rectangle {
                x: autocompleteOverlay.preferredX
                y: Math.max(semanticMenu.height,
                            autocompleteOverlay.commandLineY - height)
                width: Math.min(autocompleteOverlay.availableWidth,
                                Math.max(80, autocompleteOverlay.contentWidth))
                height: Math.min(autocompleteOverlay.maxHeight, Math.max(1, Math.min(12, autocompleteList.count)) * autocompleteOverlay.rowHeight + 8)
                color: "#202833"
                radius: 4
                border.width: 1
                border.color: root.dialogAccent
                clip: true
                z: 170

                ListView {
                    id: autocompleteList
                    anchors.fill: parent
                    anchors.margins: 4
                    model: autocompleteOverlay.items
                    clip: true
                    currentIndex: root.autocompleteMenuId === root.cleanText(autocompleteOverlay.frame.id)
                                  ? root.autocompleteSelectedIndex : -1
                    boundsBehavior: Flickable.StopAtBounds
                    interactive: contentHeight > height
                    onCurrentIndexChanged: {
                        if (currentIndex >= 0)
                            positionViewAtIndex(currentIndex, ListView.Contain)
                    }

                    delegate: Rectangle {
                        width: ListView.view.width
                        height: autocompleteOverlay.rowHeight
                        radius: 3
                        color: index === autocompleteList.currentIndex
                               ? root.selectedBg : "transparent"

                        Row {
                            id: completionTextRow
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.verticalCenter: parent.verticalCenter
                            anchors.leftMargin: 8
                            anchors.rightMargin: 8
                            readonly property string fullText: root.cleanText(modelData.text)
                            readonly property string query: root.autocompleteQuery
                            readonly property int matchingLength:
                                fullText.toLocaleLowerCase().indexOf(query.toLocaleLowerCase()) === 0
                                ? Math.min(query.length, fullText.length) : 0

                            Text {
                                id: completionPrefixLabel
                                text: completionTextRow.fullText.substring(
                                          0, completionTextRow.matchingLength)
                                color: root.dialogAccent
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 13
                            }

                            Text {
                                width: Math.max(0, completionTextRow.width
                                                - completionPrefixLabel.implicitWidth)
                                text: completionTextRow.fullText.substring(
                                          completionTextRow.matchingLength)
                                color: root.textColor
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 13
                                elide: Text.ElideRight
                            }
                        }

                        MouseArea {
                            id: mouseArea
                            anchors.fill: parent
                            hoverEnabled: true
                            acceptedButtons: Qt.LeftButton
                            onPositionChanged: (mouse) => {
                                if (containsMouse)
                                    root.autocompleteSelectedIndex = index
                            }
                            onPressed: (mouse) => {
                                root.autocompleteSelectedIndex = index
                                mouse.accepted = true
                            }
                            onClicked: root.submitAutocomplete()
                        }
                    }
                }
            }
        }
    }

    Component {
        id: menuPopupComponent
        Item {
            id: menuOverlay
            anchors.fill: parent
            property var frame: ({})
            readonly property bool fromMenuBar: frame.menuBarSubmenu === true
            readonly property int effectiveMenuIndex:
                fromMenuBar && root.menuBarPreviewIndex >= 0
                ? root.menuBarPreviewIndex
                : Number(root.menuBarModel.selected || 0)
            readonly property var previewMenuItem:
                fromMenuBar ? root.menuBarItem(effectiveMenuIndex) : null
            readonly property var effectiveItems:
                previewMenuItem && previewMenuItem.items
                ? previewMenuItem.items : (frame.items || [])
            readonly property bool previewIsAhead:
                fromMenuBar && previewMenuItem
                && effectiveMenuIndex
                   !== Number(root.menuBarModel.selected || 0)
            readonly property bool hasLeadingIndicator: {
                for (var i = 0; i < effectiveItems.length; ++i) {
                    if (effectiveItems[i].checked === true
                            || root.cleanText(effectiveItems[i].icon) !== ""
                            || root.cleanText(effectiveItems[i].iconColor) !== "")
                        return true
                }
                return false
            }
            readonly property real menuRowHeight:
                root.snapPx(Math.max(27, root.ch * 1.02))
            // Section labels are intentionally separated from the preceding
            // item. They are menu chrome, not rows that need to align with a
            // leading icon or tag dot.
            readonly property real menuHeaderTopPadding: root.snapPx(6)
            readonly property real menuHeaderHeight:
                root.snapPx(Math.max(23, root.ch * 0.9))
                + menuHeaderTopPadding
            readonly property real menuSeparatorHeight: root.snapPx(11)
            property int pointerSelectedIndex: -1
            property int semanticSelectedIndex: 0
            property int semanticTopIndex: 0
            property bool pointerWindowPositionKnown: false
            property real pointerWindowX: 0
            property real pointerWindowY: 0
            readonly property int activePointerSelectedIndex:
                fromMenuBar
                && root.menuPointerMenuIndex === effectiveMenuIndex
                ? root.menuPointerItemIndex : pointerSelectedIndex
            readonly property int visualSelectedIndex:
                activePointerSelectedIndex >= 0
                ? activePointerSelectedIndex
                : fromMenuBar && root.menuBarOpenedByPointer
                  && !root.menuBarPointerHasSelectedItem ? -1
                : previewIsAhead ? 0
                : semanticSelectedIndex

            function syncFrameState() {
                semanticSelectedIndex = Math.max(0,
                    Number(frame.selected || 0))
                semanticTopIndex = Math.max(0, Number(frame.top || 0))
            }

            function applyCommandMenuStates(states) {
                const frameId = String(frame.id || "")
                for (var i = 0; i < states.length; ++i) {
                    if (String(states[i].id || "") !== frameId)
                        continue
                    semanticSelectedIndex = Math.max(0,
                        Number(states[i].selected || 0))
                    semanticTopIndex = Math.max(0,
                        Number(states[i].top || 0))
                    if (!fromMenuBar)
                        pointerSelectedIndex = -1
                    else
                        Qt.callLater(reconcilePointerState)
                    Qt.callLater(popupMenuList.syncTopPosition)
                    return
                }
            }

            function pointerActuallyMoved(area, mouse) {
                // MouseArea.positionChanged is expressed in delegate-local
                // coordinates. Qt also emits it when ListView moves that
                // delegate underneath a completely stationary cursor (for
                // example after keyboard selection scrolls the menu). Compare
                // in the stable window coordinate space so only a real mouse
                // move may take selection ownership away from the keyboard.
                const point = area.mapToItem(root.contentItem,
                                             mouse.x, mouse.y)
                const moved = pointerWindowPositionKnown
                        && (Math.abs(point.x - pointerWindowX) >= 0.5
                            || Math.abs(point.y - pointerWindowY) >= 0.5)
                pointerWindowX = point.x
                pointerWindowY = point.y
                pointerWindowPositionKnown = true
                return moved
            }

            function reconcilePointerSelection() {
                if (!fromMenuBar || previewIsAhead
                        || root.menuPointerMenuIndex !== effectiveMenuIndex
                        || root.menuPointerItemIndex < 0
                        || root.menuPointerSentItemIndex
                           !== root.menuPointerItemIndex
                        || semanticSelectedIndex
                           !== root.menuPointerItemIndex)
                    return
                // Go now owns exactly the row already painted by QML. Dropping
                // the local override is visually lossless and lets the next
                // keyboard Up/Down scene become authoritative immediately.
                root.clearMenuPointerSelection()
            }

            function retargetPointerSelection() {
                if (!fromMenuBar || previewIsAhead
                        || root.menuPointerMenuIndex !== effectiveMenuIndex
                        || root.menuPointerItemIndex < 0)
                    return
                var frameId = String(frame.id || "")
                if (frameId === "" || root.menuPointerFrameId === frameId)
                    return
                // A locally previewed top-level menu can receive pointer input
                // before Go has replaced the old submenu frame. Once the
                // matching frame arrives, retarget the pending row selection
                // instead of sending it with the stale popup id.
                root.menuPointerFrameId = frameId
                root.menuPointerSentItemIndex = -1
                menuPointerSyncTimer.restart()
            }

            function reconcilePointerState() {
                reconcilePointerSelection()
                // An exact acknowledgement clears the local state above. If
                // it was not an acknowledgement, keep the user's hovered row
                // and bind it to the newly materialized submenu frame.
                retargetPointerSelection()
            }

            onFrameChanged: {
                syncFrameState()
                if (!fromMenuBar)
                    pointerSelectedIndex = -1
                else
                    Qt.callLater(reconcilePointerState)
            }
            Component.onCompleted: {
                syncFrameState()
                Qt.callLater(reconcilePointerState)
            }

            Connections {
                target: qtShell
                ignoreUnknownSignals: true
                function onCommandMenuStatesChanged(states) {
                    menuOverlay.applyCommandMenuStates(states)
                }
            }

            FontMetrics {
                id: popupMenuMetrics
                font.pixelSize: 13
            }

            function preferredMenuWidth() {
                if (!fromMenuBar || !previewMenuItem) {
                    var semanticWidth = root.pxW(frame.w)
                    return Math.min(root.width - 8, Math.max(150, semanticWidth))
                }
                var preferred = 150
                for (var i = 0; i < effectiveItems.length; ++i) {
                    var item = effectiveItems[i]
                    var width = popupMenuMetrics.advanceWidth(
                                    root.cleanText(item.text)) + 32
                    if (root.cleanText(item.shortcut) !== "")
                        width += popupMenuMetrics.advanceWidth(
                                     root.cleanText(item.shortcut)) + 24
                    preferred = Math.max(preferred, width)
                }
                return Math.min(root.width - 12, preferred)
            }

            function preferredMenuHeight() {
                var height = 10
                for (var i = 0; i < effectiveItems.length; ++i) {
                    height += effectiveItems[i].separator
                              ? menuSeparatorHeight
                              : effectiveItems[i].header === true
                                ? menuHeaderHeight : menuRowHeight
                }
                if (root.cleanText(frame.bottomHint) !== "")
                    height += root.ch
                return height
            }

            function popupWindowX() { return popupSurface.x }
            function popupWindowY() { return popupSurface.y }
            function popupWindowWidth() { return popupSurface.width }

            function rowWindowY(index) {
                var row = popupMenuList.itemAtIndex(index)
                if (row) {
                    var mapped = row.mapToItem(root.contentItem, 0, 0)
                    return mapped.y
                }
                var y = popupSurface.y + popupMenuList.y
                        - popupMenuList.contentY
                for (var i = 0; i < effectiveItems.length && i < index; ++i) {
                    y += effectiveItems[i].separator
                         ? menuSeparatorHeight
                         : effectiveItems[i].header === true
                           ? menuHeaderHeight : menuRowHeight
                }
                return y
            }

            function preferredPopupX(popupWidth) {
                var parentMenu = root.menuOverlayForId(frame.parentId)
                if (parentMenu) {
                    var right = parentMenu.popupWindowX()
                                + parentMenu.popupWindowWidth() - 1
                    if (right + popupWidth > root.width - 4)
                        right = parentMenu.popupWindowX() - popupWidth + 1
                    return Math.max(4, Math.min(root.width - popupWidth - 4,
                                                right))
                }
                if (previewMenuItem)
                    return semanticMenu.itemWindowX(effectiveMenuIndex)
                return Math.max(4, Math.min(root.width - popupWidth - 4,
                                            root.pxX(frame.x)))
            }

            function preferredPopupY(popupHeight) {
                var parentMenu = root.menuOverlayForId(frame.parentId)
                var desired = parentMenu
                        ? parentMenu.rowWindowY(Number(frame.anchorIndex || 0))
                        : fromMenuBar ? semanticMenu.windowBottom()
                                      : root.pxY(frame.y)
                var minimum = fromMenuBar ? semanticMenu.windowBottom() : 4
                var maximum = root.height - root.keyBarHeight()
                              - popupHeight - 4
                return Math.max(minimum, Math.min(maximum, desired))
            }

            MouseArea {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.top: parent.top
                anchors.bottom: parent.bottom
                anchors.topMargin: menuOverlay.fromMenuBar ? semanticMenu.height : 0
                acceptedButtons: Qt.AllButtons
                hoverEnabled: true
                preventStealing: true
                onClicked: root.action({
                    "target": menuOverlay.frame.id,
                    "action": "menu.closeChain"
                })
                onPressed: {
                    root.menuBarPreviewIndex = -1
                    root.clearMenuPointerSelection()
                }
                onWheel: (wheel) => { wheel.accepted = true }
            }

            Rectangle {
                id: popupSurface
                width: root.snapPx(menuOverlay.preferredMenuWidth())
                height: root.snapPx(Math.min(root.height - root.keyBarHeight() - 8,
                                             Math.max(root.ch + 10,
                                                      menuOverlay.preferredMenuHeight())))
                x: root.snapPx(menuOverlay.preferredPopupX(width))
                y: root.snapPx(menuOverlay.preferredPopupY(height))
                objectName: "semanticMenuPopup-" + root.cleanText(menuOverlay.frame.id)
                color: root.dialogHeaderBg
                border.width: 1
                border.color: root.controlBorder
                radius: 7
                clip: true
                z: 160

                ListView {
                    id: popupMenuList
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.top: parent.top
                    anchors.bottom: parent.bottom
                    anchors.topMargin: 5
                    anchors.bottomMargin: root.cleanText(menuOverlay.frame.bottomHint) !== ""
                                          ? root.ch : 5
                    anchors.leftMargin: 5
                    anchors.rightMargin: 5
                    model: menuOverlay.effectiveItems
                    clip: true
                    currentIndex: menuOverlay.visualSelectedIndex
                    boundsBehavior: Flickable.StopAtBounds
                    interactive: contentHeight > height

                    function syncTopPosition() {
                        if (count > 0)
                            positionViewAtIndex(menuOverlay.semanticTopIndex,
                                                ListView.Beginning)
                    }

                    Component.onCompleted: Qt.callLater(syncTopPosition)
                    onModelChanged: Qt.callLater(syncTopPosition)

                    delegate: Rectangle {
                        objectName: "semanticMenuItem-"
                                    + root.cleanText(menuOverlay.frame.id)
                                    + "-" + Number(modelData.index)
                        width: ListView.view.width
                        height: modelData.separator
                                ? menuOverlay.menuSeparatorHeight
                                : modelData.header === true
                                  ? menuOverlay.menuHeaderHeight
                                  : menuOverlay.menuRowHeight
                        radius: 4
                        color: modelData.index === menuOverlay.visualSelectedIndex
                               && !modelData.separator
                               && modelData.header !== true
                               ? root.selectedBg : "transparent"

                        Rectangle {
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.leftMargin: 8
                            anchors.rightMargin: 8
                            anchors.verticalCenter: parent.verticalCenter
                            height: 1
                            color: root.separatorColor
                            opacity: 0.68
                            visible: modelData.separator
                        }

                        Text {
                            objectName: "semanticMenuItemText-"
                                        + root.cleanText(menuOverlay.frame.id)
                                        + "-" + Number(modelData.index)
                            anchors.left: parent.left
                            anchors.right: shortcut.left
                            anchors.verticalCenter: modelData.header === true
                                                   ? undefined
                                                   : parent.verticalCenter
                            anchors.top: modelData.header === true
                                         ? parent.top : undefined
                            anchors.bottom: modelData.header === true
                                            ? parent.bottom : undefined
                            anchors.topMargin: modelData.header === true
                                               ? menuOverlay.menuHeaderTopPadding
                                               : 0
                            anchors.leftMargin: modelData.header === true
                                                ? 10
                                                : menuOverlay.hasLeadingIndicator
                                                ? 32 : 10
                            verticalAlignment: Text.AlignVCenter
                            text: {
                                var label = root.cleanText(modelData.text)
                                if (menuOverlay.hasLeadingIndicator)
                                    label = label.replace(/^\s+/, "")
                                return root.mnemonicText(label,
                                                         modelData.hotkey)
                            }
                            textFormat: Text.StyledText
                            color: modelData.disabled || modelData.header === true
                                   ? root.mutedText : root.textColor
                            font.pixelSize: modelData.header === true ? 12 : 13
                            font.bold: modelData.header === true
                            visible: !modelData.separator
                            elide: Text.ElideRight
                        }

                        IconLabel {
                            id: leadingMenuIcon
                            objectName: "semanticMenuItemIcon-"
                                        + root.cleanText(menuOverlay.frame.id)
                                        + "-" + Number(modelData.index)
                            readonly property string semanticIconName:
                                modelData.checked === true ? "check"
                                : root.cleanText(modelData.icon)
                            readonly property url semanticIconSource:
                                semanticIconName === ""
                                || semanticIconName === "tag-dot" ? ""
                                : root.resolvedIconSource(semanticIconName, 15)
                            readonly property color semanticIconColor:
                                modelData.disabled ? root.mutedText
                                : root.cleanText(modelData.iconColor) !== ""
                                  ? root.cleanText(modelData.iconColor)
                                  : root.textColor
                            x: root.snapPx(10)
                            y: root.snapPx((parent.height - height) / 2)
                            width: root.snapPx(15)
                            height: root.snapPx(15)
                            property real alignmentRevision:
                                popupSurface.x + popupSurface.y
                                + popupMenuList.contentY + parent.y
                            transform: Translate {
                                x: root.iconPixelOffsetX(leadingMenuIcon)
                                y: root.iconPixelOffsetY(leadingMenuIcon)
                            }
                            visible: !modelData.separator
                                     && modelData.header !== true
                                     && semanticIconName !== "tag-dot"
                                     && semanticIconName !== ""
                            icon.source: semanticIconSource
                            icon.width: 15
                            icon.height: 15
                            icon.color: semanticIconColor
                        }

                        Rectangle {
                            id: menuItemColor
                            objectName: "semanticMenuItemColor-"
                                        + root.cleanText(menuOverlay.frame.id)
                                        + "-" + Number(modelData.index)
                            x: root.snapPx(13)
                            y: root.snapPx((parent.height - height) / 2)
                            width: root.snapPx(10)
                            height: width
                            radius: width / 2
                            color: root.cleanText(modelData.iconColor) !== ""
                                   ? root.cleanText(modelData.iconColor)
                                   : root.textColor
                            property real alignmentRevision:
                                popupSurface.x + popupSurface.y
                                + popupMenuList.contentY + parent.y
                            transform: Translate {
                                x: root.iconPixelOffsetX(menuItemColor)
                                y: root.iconPixelOffsetY(menuItemColor)
                            }
                            visible: !modelData.separator
                                     && modelData.header !== true
                                     && root.cleanText(modelData.icon) === "tag-dot"
                        }

                        Text {
                            id: menuItemChevron
                            objectName: "semanticMenuItemChevron-"
                                        + root.cleanText(menuOverlay.frame.id)
                                        + "-" + Number(modelData.index)
                            x: root.snapPx(parent.width - width - 9)
                            y: 0
                            width: root.snapPx(15)
                            height: root.snapPx(parent.height)
                            text: "›"
                            color: modelData.disabled ? root.mutedText : root.textColor
                            font.pixelSize: 17
                            horizontalAlignment: Text.AlignHCenter
                            verticalAlignment: Text.AlignVCenter
                            property real alignmentRevision:
                                popupSurface.x + popupSurface.y
                                + popupMenuList.contentY + parent.y
                            transform: Translate {
                                x: root.iconPixelOffsetX(menuItemChevron)
                                y: root.iconPixelOffsetY(menuItemChevron)
                            }
                            visible: !modelData.separator
                                     && modelData.header !== true
                                     && modelData.hasSubmenu === true
                        }

                        Text {
                            id: shortcut
                            anchors.right: parent.right
                            anchors.verticalCenter: parent.verticalCenter
                            anchors.rightMargin: modelData.hasSubmenu === true ? 28 : 10
                            text: root.cleanText(modelData.shortcut)
                            color: root.mutedText
                            font.pixelSize: 12
                            visible: !modelData.separator
                                     && modelData.header !== true
                        }

                        Timer {
                            id: submenuHoverTimer
                            interval: 180
                            repeat: false
                            onTriggered: root.action({
                                "target": menuOverlay.frame.id,
                                "action": "menu.openSubmenu",
                                "index": modelData.index
                            }, true)
                        }

                        MouseArea {
                            id: itemMouse
                            anchors.fill: parent
                            hoverEnabled: true
                            enabled: !modelData.separator
                                     && modelData.header !== true
                                     && !modelData.disabled
                            function selectFromPointer() {
                                if (menuOverlay.fromMenuBar) {
                                    root.menuBarPointerHasSelectedItem = true
                                    if (root.menuPointerItemIndex < 0
                                            && !menuOverlay.previewIsAhead
                                            && menuOverlay.semanticSelectedIndex
                                               === modelData.index)
                                        return
                                    if (root.menuPointerMenuIndex
                                            === menuOverlay.effectiveMenuIndex
                                            && root.menuPointerItemIndex
                                               === modelData.index)
                                        return
                                    root.menuPointerMenuIndex
                                            = menuOverlay.effectiveMenuIndex
                                    root.menuPointerItemIndex = modelData.index
                                    root.menuPointerFrameId
                                            = String(menuOverlay.frame.id || "")
                                    menuPointerSyncTimer.restart()
                                } else {
                                    if (menuOverlay.pointerSelectedIndex
                                            === modelData.index)
                                        return
                                    menuOverlay.pointerSelectedIndex = modelData.index
                                    root.action({
                                        "target": menuOverlay.frame.id,
                                        "action": "menu.select",
                                        "index": modelData.index
                                    }, true)
                                }
                            }
                            // Delegate creation and ListView scrolling both
                            // produce local position changes under a stationary
                            // cursor. Only movement in window coordinates is
                            // allowed to take selection ownership.
                            onPositionChanged: (mouse) => {
                                if (containsMouse
                                        && menuOverlay.pointerActuallyMoved(
                                            itemMouse, mouse))
                                    selectFromPointer()
                            }
                            onEntered: {
                                if (modelData.hasSubmenu === true)
                                    submenuHoverTimer.restart()
                            }
                            onExited: submenuHoverTimer.stop()
                            onClicked: {
                                submenuHoverTimer.stop()
                                if (menuOverlay.fromMenuBar) {
                                    root.action({
                                        "action": "menuBar.itemActivate",
                                        "menuIndex": menuOverlay.effectiveMenuIndex,
                                        "index": modelData.index
                                    }, true)
                                } else {
                                    root.action({
                                        "target": menuOverlay.frame.id,
                                        "action": "menu.activate",
                                        "index": modelData.index
                                    }, true)
                                }
                                root.menuBarPreviewIndex = -1
                                root.clearMenuPointerSelection()
                            }
                        }
                    }
                }

                MouseArea {
                    anchors.fill: popupMenuList
                    acceptedButtons: Qt.NoButton
                    onWheel: (wheel) => {
                        var delta = wheel.angleDelta.y > 0 ? -1 : 1
                        root.action({
                            "target": menuOverlay.frame.id,
                            "action": "menu.scroll",
                            "delta": delta
                        }, true)
                        wheel.accepted = true
                    }
                }

                Text {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    height: root.ch
                    anchors.leftMargin: 8
                    anchors.rightMargin: 8
                    text: root.cleanText(menuOverlay.frame.bottomHint)
                    color: root.mutedText
                    font.pixelSize: 11
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                    elide: Text.ElideMiddle
                    visible: text !== ""
                }

                Rectangle {
                    readonly property int itemCount: menuOverlay.effectiveItems.length
                    readonly property int pageSize: Math.max(1, Number(menuOverlay.frame.viewHeight || itemCount))
                    anchors.right: parent.right
                    anchors.rightMargin: 2
                    width: 3
                    height: Math.max(12, popupMenuList.height * Math.min(1, pageSize / Math.max(1, itemCount)))
                    y: popupMenuList.y + (popupMenuList.height - height)
                       * Math.max(0, Number(menuOverlay.frame.top || 0))
                       / Math.max(1, itemCount - pageSize)
                    radius: 2
                    color: root.mutedText
                    visible: itemCount > pageSize
                    opacity: 0.75
                }
            }
        }
    }

    component KeyBarView: Rectangle {
        id: keyBarRoot
        property var keyBar: ({})
        objectName: "keyBar"
        color: root.fBarBg
        visible: keyBar.visible !== false && keyBar.items !== undefined

        function openAlternativeMenu(anchorItem, item, functionKey,
                                     functionIndex) {
            const alternatives = item && item.alternatives
                    ? item.alternatives : []
            const activeModifier = root.cleanText(keyBar.modifier || "normal")
                    .trim().toLowerCase()
            var rows = []
            for (var i = 0; i < alternatives.length; ++i) {
                const alternative = alternatives[i] || ({})
                const modifier = root.cleanText(alternative.modifier)
                        .trim().toLowerCase()
                const text = root.cleanText(alternative.text)
                if (text === "" || modifier === activeModifier)
                    continue
                rows.push({
                    "text": text,
                    "icon": root.cleanText(alternative.icon),
                    "modifier": modifier,
                    "shortcut": root.keyBarModifierShortcut(
                                     functionKey, modifier)
                })
            }
            if (rows.length === 0)
                return false
            if (keyBarAlternativeMenu.opened)
                keyBarAlternativeMenu.close()
            keyBarAlternativeMenu.anchorItem = anchorItem
            keyBarAlternativeMenu.functionKey = functionKey
            keyBarAlternativeMenu.functionIndex = functionIndex
            keyBarAlternativeMenu.menuItems = rows
            keyBarAlternativeMenu.open()
            return true
        }

        // This is application chrome, not panel/document content. Keeping the
        // separator in the shared F-bar makes panels, viewer and editor end at
        // one identical boundary.
        Rectangle {
            objectName: "keyBarTopSeparator"
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: root.separatorWidth
            color: root.separatorColor
            antialiasing: false
            z: 2
        }

        Row {
            anchors.fill: parent
            anchors.leftMargin: 0
            anchors.rightMargin: 0
            anchors.topMargin: root.actionBarVerticalMargin
            anchors.bottomMargin: root.actionBarVerticalMargin
            Repeater {
                model: keyBar.items || []
                delegate: Rectangle {
                    id: actionButton
                    readonly property string functionKey:
                        root.cleanText(modelData.key) !== ""
                        ? root.cleanText(modelData.key)
                        : "F" + String(index + 1)
                    readonly property int functionIndex:
                        root.keyBarFunctionIndex(
                            { "key": actionButton.functionKey }, index)
                    readonly property string iconName:
                        root.cleanText(modelData.icon)
                    objectName: "key-bar-action-" + (functionIndex + 1)
                    width: parent.width / 12
                    height: parent.height
                    radius: root.snapPx(5)
                    color: actionButtonMouse.pressed
                           ? root.panelSelectionBorder
                           : actionButtonMouse.containsMouse
                             || (keyBarAlternativeMenu.opened
                                 && keyBarAlternativeMenu.functionIndex
                                    === actionButton.functionIndex)
                             ? root.panelSelectionBg : "transparent"

                    PixelAlignedImage {
                        id: actionIcon
                        objectName: "key-bar-icon-"
                                    + (actionButton.functionIndex + 1)
                        anchors.left: parent.left
                        anchors.verticalCenter: parent.verticalCenter
                        anchors.leftMargin: root.actionButtonHorizontalMargin
                        width: visible ? root.snapPx(14) : 0
                        height: visible ? root.snapPx(14) : 0
                        visible: actionButton.iconName !== ""
                        smooth: false
                        mipmap: false
                        alignmentRevision: actionButton.x + actionButton.y
                                           + actionButton.width
                                           + actionButton.height
                        source: root.lucideIconSource(
                                    actionButton.iconName, 14,
                                    actionButtonMouse.containsMouse
                                    ? root.textColor : root.chromeText)
                    }

                    Text {
                        id: actionTextLabel
                        objectName: "key-bar-label-"
                                    + (actionButton.functionIndex + 1)
                        anchors.left: actionIcon.visible
                                      ? actionIcon.right : parent.left
                        anchors.leftMargin: actionIcon.visible
                                            ? 7
                                            : root.actionButtonHorizontalMargin
                        anchors.right: functionKeyLabel.left
                        anchors.rightMargin: 7
                        anchors.verticalCenter: parent.verticalCenter
                        text: root.mnemonicText(modelData.text,
                                                modelData.hotkey)
                        textFormat: Text.StyledText
                        color: root.chromeText
                        font.pixelSize: 11
                        elide: Text.ElideRight
                    }

                    Text {
                        id: functionKeyLabel
                        objectName: "key-bar-shortcut-"
                                    + (actionButton.functionIndex + 1)
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        anchors.rightMargin: root.actionButtonHorizontalMargin
                        text: actionButton.functionKey
                        color: actionButtonMouse.containsMouse
                               ? root.textColor : root.mutedText
                        font.pixelSize: 11
                    }

                    Rectangle {
                        anchors.right: parent.right
                        anchors.top: parent.top
                        anchors.bottom: parent.bottom
                        anchors.topMargin: root.actionSeparatorVerticalMargin
                        anchors.bottomMargin: root.actionSeparatorVerticalMargin
                        width: root.separatorWidth
                        color: root.separatorColor
                        visible: index < (keyBar.items || []).length - 1
                    }

                    MouseArea {
                        id: actionButtonMouse
                        anchors.fill: parent
                        hoverEnabled: true
                        acceptedButtons: Qt.LeftButton | Qt.RightButton
                        cursorShape: Qt.PointingHandCursor
                        onClicked: function(mouse) {
                            if (mouse.button === Qt.RightButton) {
                                keyBarRoot.openAlternativeMenu(
                                    actionButton, modelData,
                                    actionButton.functionKey,
                                    actionButton.functionIndex)
                                return
                            }
                            // Dispatch the same semantic F-key that is
                            // visibly labelled. Repeater's injected `index`
                            // can transiently shadow a map field while a
                            // delegate is being created.
                            var keyNumber = Number(
                                        parent.functionKey
                                            .replace(/^F/i, ""))
                            var clickedIndex = !isNaN(keyNumber)
                                    && keyNumber >= 1 && keyNumber <= 24
                                    ? keyNumber - 1 : parent.functionIndex
                            var vk = 0x70 + clickedIndex
                            var mods = root.vtuiKeyModifiers(mouse.modifiers)
                            qtShell.sendKey(vk, 0, true, mods)
                            qtShell.sendKey(vk, 0, false, mods)
                            grid.forceActiveFocus()
                        }
                    }
                }
            }
        }

        Popup {
            id: keyBarAlternativeMenu
            objectName: "keyBarAlternativeMenu"
            parent: Overlay.overlay
            property var anchorItem: null
            property string functionKey: ""
            property int functionIndex: -1
            property var menuItems: []
            readonly property real menuRowHeight: root.snapPx(31)

            function preferredWidth() {
                var widestLabel = 0
                var widestShortcut = 0
                for (var i = 0; i < menuItems.length; ++i) {
                    const item = menuItems[i] || ({})
                    widestLabel = Math.max(widestLabel,
                        keyBarAlternativeFontMetrics.advanceWidth(
                            root.cleanText(item.text)))
                    widestShortcut = Math.max(widestShortcut,
                        keyBarAlternativeFontMetrics.advanceWidth(
                            root.cleanText(item.shortcut)))
                }
                return Math.max(root.snapPx(220),
                                root.snapPx(62) + widestLabel
                                + widestShortcut)
            }

            width: Math.min(root.width - root.snapPx(12), preferredWidth())
            implicitHeight: menuItems.length * menuRowHeight
                             + topPadding + bottomPadding
            height: Math.min(root.height - root.snapPx(12),
                             Math.max(menuRowHeight + topPadding + bottomPadding,
                                      implicitHeight))
            padding: root.snapPx(6)
            modal: false
            dim: false
            focus: false
            z: 1001
            closePolicy: Popup.CloseOnEscape
                         | Popup.CloseOnPressOutside
                         | Popup.CloseOnPressOutsideParent

            onAboutToShow: {
                if (!anchorItem) {
                    close()
                    return
                }
                const point = anchorItem.mapToItem(
                    root.contentItem, anchorItem.width / 2, 0)
                x = Math.max(root.snapPx(6), Math.min(
                    root.width - width - root.snapPx(6),
                    point.x - width / 2))
                var popupY = point.y - height - root.snapPx(4)
                if (popupY < root.snapPx(6))
                    popupY = point.y + anchorItem.height + root.snapPx(4)
                y = Math.max(root.snapPx(6), Math.min(
                    root.height - height - root.snapPx(6), popupY))
            }

            onClosed: {
                anchorItem = null
                functionKey = ""
                functionIndex = -1
                menuItems = []
            }

            background: Rectangle {
                color: root.dialogHeaderBg
                radius: root.snapPx(8)
                border.width: root.separatorWidth
                border.color: root.controlBorder
            }

            FontMetrics {
                id: keyBarAlternativeFontMetrics
                font.family: root.guiMonospaceFontFamily
                font.pixelSize: 12
            }

            contentItem: ListView {
                id: keyBarAlternativeList
                objectName: "keyBarAlternativeList"
                anchors.fill: parent
                clip: true
                model: keyBarAlternativeMenu.menuItems
                boundsBehavior: Flickable.StopAtBounds

                delegate: Rectangle {
                    id: keyBarAlternativeRow
                    required property var modelData
                    objectName: "keyBarAlternative-"
                                + root.cleanText(modelData.modifier)
                    width: keyBarAlternativeList.width
                    height: keyBarAlternativeMenu.menuRowHeight
                    radius: root.snapPx(5)
                    color: keyBarAlternativeMouse.containsMouse
                           ? root.controlHoverBg : "transparent"

                    Item {
                        id: keyBarAlternativeIconSlot
                        anchors.left: parent.left
                        anchors.leftMargin: root.snapPx(8)
                        anchors.verticalCenter: parent.verticalCenter
                        width: root.snapPx(18)
                        height: root.snapPx(18)

                        PixelAlignedImage {
                            objectName: "keyBarAlternativeIcon-"
                                        + root.cleanText(modelData.modifier)
                            anchors.centerIn: parent
                            width: root.snapPx(16)
                            height: root.snapPx(16)
                            visible: root.cleanText(modelData.icon) !== ""
                            smooth: false
                            mipmap: false
                            source: root.lucideIconSource(
                                        root.cleanText(modelData.icon), 16,
                                        keyBarAlternativeMouse.containsMouse
                                        ? root.textColor : root.chromeText)
                        }
                    }

                    Text {
                        id: keyBarAlternativeLabel
                        objectName: "keyBarAlternativeLabel-"
                                    + root.cleanText(modelData.modifier)
                        anchors.left: keyBarAlternativeIconSlot.right
                        anchors.right: keyBarAlternativeShortcut.left
                        anchors.leftMargin: root.snapPx(8)
                        anchors.rightMargin: root.snapPx(12)
                        anchors.verticalCenter: parent.verticalCenter
                        text: root.cleanText(modelData.text)
                        color: root.textColor
                        font.family: root.guiMonospaceFontFamily
                        font.pixelSize: 12
                        elide: Text.ElideRight
                    }

                    Text {
                        id: keyBarAlternativeShortcut
                        objectName: "keyBarAlternativeShortcut-"
                                    + root.cleanText(modelData.modifier)
                        anchors.right: parent.right
                        anchors.rightMargin: root.snapPx(10)
                        anchors.verticalCenter: parent.verticalCenter
                        text: root.cleanText(modelData.shortcut)
                        color: root.mutedText
                        font.family: root.guiMonospaceFontFamily
                        font.pixelSize: 11
                    }

                    MouseArea {
                        id: keyBarAlternativeMouse
                        anchors.fill: parent
                        acceptedButtons: Qt.LeftButton
                        hoverEnabled: true
                        onClicked: {
                            const keyNumber = Number(
                                keyBarAlternativeMenu.functionKey
                                    .replace(/^F/i, ""))
                            const clickedIndex = !isNaN(keyNumber)
                                    && keyNumber >= 1 && keyNumber <= 24
                                    ? keyNumber - 1
                                    : keyBarAlternativeMenu.functionIndex
                            const vk = 0x70 + clickedIndex
                            const modifiers = root.keyBarModifierFlags(
                                modelData.modifier)
                            qtShell.sendKey(vk, 0, true, modifiers)
                            qtShell.sendKey(vk, 0, false, modifiers)
                            keyBarAlternativeMenu.close()
                        }
                    }
                }
            }
        }

        MouseArea {
            // Popup.CloseOnPressOutside is not delivered reliably when a
            // non-windowed popup is parented to Overlay.overlay above the
            // persistent panel/grid pointer layers. Keep the dismiss plane
            // immediately below the popup so outside presses always close it
            // without moving the file-panel cursor underneath.
            parent: Overlay.overlay
            anchors.fill: parent
            visible: keyBarAlternativeMenu.opened
            enabled: visible
            z: 1000
            acceptedButtons: Qt.LeftButton | Qt.RightButton
                             | Qt.MiddleButton
            onPressed: keyBarAlternativeMenu.close()
        }
    }

    component ToastView: Rectangle {
        id: toastRoot
        property var toast: ({})
        readonly property string message: root.cleanText(toast.message)
        width: Math.min(root.width - 32,
                        toastText.implicitWidth + toastCloseButton.implicitWidth + 44)
        height: Math.max(32, toastText.implicitHeight + 14)
        radius: 6
        color: root.dialogBg
        border.width: 1
        border.color: root.controlBorder
        visible: toast.message !== undefined && message !== ""

        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: 12
            anchors.rightMargin: 6
            spacing: 8

            Text {
                id: toastText
                Layout.fillWidth: true
                Layout.maximumWidth: Math.max(0, root.width - 92)
                text: toastRoot.message
                color: root.textColor
                font.pixelSize: 13
                elide: Text.ElideRight
                verticalAlignment: Text.AlignVCenter
            }

            T.Button {
                id: toastCloseButton
                objectName: "toastCloseButton"
                implicitWidth: 24
                implicitHeight: 24
                padding: 0
                hoverEnabled: true
                Accessible.name: "Close notification"
                Accessible.role: Accessible.Button

                background: Rectangle {
                    radius: 4
                    color: toastCloseButton.down
                           ? root.controlPressedBg
                           : (toastCloseButton.hovered
                              ? root.controlHoverBg : "transparent")
                    border.width: toastCloseButton.activeFocus ? 1 : 0
                    border.color: root.dialogAccent
                }

                contentItem: Text {
                    text: "\u00d7"
                    color: root.textColor
                    font.pixelSize: 17
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
                }

                onClicked: root.action({
                    "action": "toast.dismiss",
                    "target": "toast"
                })
            }
        }
    }

    function toggleFullscreen() {
        if (visibility === Window.FullScreen)
            showNormal()
        else
            showFullScreen()
    }
}
