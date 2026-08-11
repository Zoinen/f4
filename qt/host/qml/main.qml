import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl
import QtQuick.Layouts
import F4QtHost 1.0
import QWindowKit 1.0

ApplicationWindow {
    id: root

    width: Math.max(720, Math.ceil(qtShell.initialCols * grid.cellWidth))
    height: Math.max(460, Math.ceil(qtShell.initialRows * grid.cellHeight))
    topPadding: 0
    leftPadding: 0
    rightPadding: 0
    bottomPadding: 0
    visible: true
    title: fallbackExplanation !== ""
           ? "f4 [Using text presentation: " + fallbackExplanation + "]"
           : "f4"
    property bool isQWKLegacy: false
    property bool windowAgentReady: false
    property int macWindowEffectApplyAttempts: 0
    readonly property string guiMonospaceFontFamily:
        String(f4GuiFontFamily || "").length > 0 ? String(f4GuiFontFamily) : "Monaco"
    readonly property int guiMonospaceFontPixelSize:
        Number(f4GuiFontPixelSize) > 0 ? Number(f4GuiFontPixelSize)
                                       : (Qt.platform.os === "osx" ? 17 : 16)
    readonly property bool supportsTransparentWindowBackground:
        f4UsesQwk && (Qt.platform.os === "windows" || Qt.platform.os === "osx")
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
    color: useTransparentWindowBackground ? "transparent" : "#101318"

    property var scene: qtShell.scene || ({})
    readonly property var workspaceTabs: scene.workspaceTabs || ({})
    readonly property var workspaces: workspaceTabs.tabs || []
    property real cw: Math.max(8, grid.cellWidth)
    property real ch: Math.max(17, grid.cellHeight)
    readonly property real menuBarHeight: 42
    readonly property real contentSpacing: 16
    readonly property real panelContentSpacing: 8
    readonly property real panelTextInset: 16
    readonly property real panelRowInnerSpacing:
        panelTextInset - panelContentSpacing
    readonly property real verticalContentSpacing: 8
    readonly property real pathRowExtraHeight: 4
    readonly property real columnSeparatorVerticalMargin: 6
    readonly property real separatorWidth: 1
    readonly property real commandLineLeftMargin: 8
    readonly property real commandLineVerticalMargin: 8
    readonly property real actionBarVerticalMargin: 3
    readonly property real actionSeparatorVerticalMargin: 5
    readonly property real actionButtonHorizontalMargin: 8
    readonly property real menuItemHorizontalPadding: 14
    property color panelBg: "#99121822"
    property color panelPathBg: "#26314152"
    property color panelBgAlt: "#d910151d"
    property color terminalBg: "#99000000"
    property color commandLineBg: "#33000000"
    property color panelBorder: "#4e9bd4"
    property color activeBorder: "#f0c95a"
    property color textColor: "#e8edf2"
    property color mutedText: "#9aa7b5"
    property color selectedBg: "#285d8f"
    property color panelSelectionBg: "#18456e"
    property color panelSelectionBorder: "#1d5888"
    property color folderIconColor: "#5ab2f1"
    property color markedBg: "#4f5037"
    property color panelHeaderBg: "#1c2531"
    property color chromeBg: "#202833"
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
    readonly property real iconDevicePixelRatio:
        root.screen ? root.screen.devicePixelRatio : 1.0
    readonly property string fallbackExplanation: semanticFallbackReason()
    // Panel sizing belongs exclusively to this Qt presentation. The Go scene
    // continues to supply two complete panels and is never notified when the
    // divider moves.
    property real panelSplitRatio: 0.5
    readonly property real panelMinimumWidth:
        Math.max(180, Math.min(280, cw * 22))
    readonly property bool nativeTwoPanelSurfaceActive:
        isAppScene()
        && !needsFallbackGrid()
        && !hasBlockingOverlay()
        && !hasDocumentSurface()
        && !qtGallery.viewerVisible
        && shellFrame().terminalActive !== true
    readonly property bool nativeTwoPanelSurfaceVisible:
        isAppScene()
        && !needsFallbackGrid()
        && !hasDocumentSurface()
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
        hasDocumentSurface() ? 1 : 1 - galleryViewerProgress
    // FilePanelView instances live inside transient semantic Loaders, while a
    // Gallery viewer needs the active panel host throughout its reverse
    // animation. Retain non-owning QML object references by side and clear them when
    // the corresponding panel Loader is destroyed/replaced.
    property var leftGalleryPanelHost: null
    property var rightGalleryPanelHost: null
    // Native file-panel views keep the hidden terminal grid as the general
    // commander key sink, but plain cursor movement uses QML's actual viewport
    // and is committed to Go only after autorepeat pauses.
    property var leftLocalPanelHost: null
    property var rightLocalPanelHost: null
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
        if (!f4UsesQwk)
            return

        windowAgent.setup(root)
        windowAgentReady = true

        if (Qt.platform.os === "windows") {
            isQWKLegacy = windowAgent.setWindowAttribute("mica-alt", true) !== true
        } else if (useMacNativeTitleBar) {
            applyPlatformWindowEffects()
        }

        windowAgent.setTitleBar(titleBar)
        if (useMacNativeTitleBar) {
            windowAgent.setSystemButtonArea(macSystemButtonArea)
        } else {
            windowAgent.setSystemButton(WindowAgent.Minimize, minimizeButton)
            windowAgent.setSystemButton(WindowAgent.Maximize, maximizeButton)
            windowAgent.setSystemButton(WindowAgent.Close, closeButton)
        }
    }

    Timer {
        id: macWindowEffectRetryTimer
        interval: 100
        repeat: false
        onTriggered: root.applyPlatformWindowEffects()
    }

    function applyPlatformWindowEffects() {
        if (!windowAgentReady || !useMacNativeTitleBar)
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
        var menus = scene.menus || []
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
        var actionMap = { "target": frame.id, "action": "command.submit" }
        if (autocompleteSelectedIndex >= 0)
            actionMap.text = autocompleteSelectedText()
        action(actionMap, true)
    }

    function completeAutocomplete() {
        var frame = activeAutocompleteFrame()
        if (!frame)
            return
        var actionMap = { "target": frame.id, "action": "command.complete" }
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
        var items = (scene.menuBar && scene.menuBar.items)
                    ? scene.menuBar.items : []
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

    function setLocalPanelHost(side, panelHost) {
        if (Number(side) === 0)
            leftLocalPanelHost = panelHost
        else if (Number(side) === 1)
            rightLocalPanelHost = panelHost
    }

    function clearLocalPanelHost(side, panelHost) {
        if (Number(side) === 0 && leftLocalPanelHost === panelHost)
            leftLocalPanelHost = null
        else if (Number(side) === 1 && rightLocalPanelHost === panelHost)
            rightLocalPanelHost = null
    }

    function activeLocalPanelHost() {
        if (needsFallbackGrid() || hasBlockingOverlay()
                || hasDocumentSurface() || qtGallery.viewerVisible)
            return null
        var shell = shellFrame()
        var panels = shell && shell.panels ? shell.panels : []
        for (var i = 0; i < panels.length; ++i) {
            var panel = panels[i]
            if (panel.active !== true || !panelSideVisible(panel.side)
                    || qtGallery.shouldUseGallery(panel))
                continue
            var mode = cleanText(panel.viewModeName)
            if (mode !== "brief" && mode !== "medium"
                    && mode !== "detailed" && mode !== "wide")
                return null
            return Number(panel.side || 0) === 0
                    ? leftLocalPanelHost : rightLocalPanelHost
        }
        return null
    }

    function activeLocalPanelCanNavigate(key) {
        var host = activeLocalPanelHost()
        if (!host)
            return false
        return typeof host.canNavigate !== "function"
                || host.canNavigate(key)
    }

    function navigateActiveLocalPanel(key) {
        var host = activeLocalPanelHost()
        return host && activeLocalPanelCanNavigate(key)
                ? host.navigate(key) : false
    }

    function commandLineHasText() {
        var shell = shellFrame()
        var commandLine = shell && shell.commandLine
                          ? shell.commandLine : ({})
        return cleanText(commandLine.text).length > 0
    }

    function isAppScene() {
        return scene.schema === "app" && scene.shell !== undefined
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
        var menus = scene.menus || []
        var dialogs = scene.dialogs || []
        for (var i = 0; i < menus.length; ++i)
            out.push(menus[i])
        for (var j = 0; j < dialogs.length; ++j)
            out.push(dialogs[j])
        return out
    }

    function hasBlockingOverlay() {
        return overlayFrames().length > 0
    }

    function activePanelUsesGallery() {
        var shell = shellFrame()
        var panels = shell && shell.panels ? shell.panels : []
        for (var i = 0; i < panels.length; ++i) {
            if (panels[i].active === true)
                return qtGallery.shouldUseGallery(panels[i])
        }
        return false
    }

    function galleryInputRoutingActive() {
        if (hasBlockingOverlay() || needsFallbackGrid()
                || hasDocumentSurface())
            return false
        // The full-area GalleryViewer is a modal keyboard/text surface.  Its
        // key handlers already swallow every press/release; disabling the
        // hidden grid's window-level IME filter closes the equivalent commit-
        // string path into Go as well.
        if (qtGallery.viewerVisible)
            return false
        return activePanelUsesGallery()
    }

    function galleryViewerOwnsKeyboard() {
        return qtGallery.viewerVisible
                && !hasBlockingOverlay()
                && !needsFallbackGrid()
                && !hasDocumentSurface()
    }

    function restoreSurfaceFocus() {
        // Commander overlays and non-panel surfaces always own keys, even
        // when the integrated viewer remains loaded underneath them.
        if (hasBlockingOverlay() || needsFallbackGrid()
                || hasDocumentSurface()) {
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
        for (var i = 0; i < panels.length; ++i) {
            if (panels[i].active !== true
                    || !panelSideVisible(panels[i].side)
                    || !qtGallery.shouldUseGallery(panels[i]))
                continue
            var panelHost = galleryPanelHost(panels[i].side)
            if (panelHost) {
                panelHost.forceActiveFocus()
                return
            }
        }
        grid.forceActiveFocus()
    }

    function shellFrame() {
        return scene.shell || firstFrame("shell") || firstFrame("panels") || ({})
    }

    function activeSurface() {
        if (scene.surface)
            return scene.surface
        return topFrame()
    }

    function isDocumentSurface(frame) {
        return frame && (frame.kind === "viewer" || frame.kind === "editor"
                         || frame.kind === "terminal")
    }

    function hasDocumentSurface() {
        var shell = shellFrame()
        return isDocumentSurface(activeSurface())
                || (shell && shell.terminalActive === true)
    }

    function keyBarHeight() {
        return root.scene.keyBar
                ? root.ch + root.commandLineVerticalMargin * 2
                  + root.separatorWidth * 2
                : 0
    }

    function commandLineHeight(shell) {
        var cmd = shell && shell.commandLine ? shell.commandLine : null
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
        var panels = shell.panels || []
        for (var i = 0; i < panels.length; ++i) {
            if (cleanText(panels[i].viewModeName) === "wide")
                return Number(panels[i].side || 0)
        }
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

    function resolvedIconSource(name, logicalSize) {
        // Reading revision makes every binding below refresh after an icon-set
        // or desktop-theme change; it is also encoded into system image URLs
        // so Qt Quick cannot serve an obsolete cached texture.
        const generation = qtIcons.revision
        return qtIcons.iconSource(name, logicalSize,
                                  root.iconDevicePixelRatio)
    }

    function bundledLucideIconName(source) {
        const value = cleanText(source)
        const normalPrefix = "qrc:/F4QtHost/icons/lucide/"
        const galleryPrefix = "qrc:/F4QtHost/icons/lucide-gallery/"
        var name = ""
        if (value.indexOf(normalPrefix) === 0)
            name = value.substring(normalPrefix.length)
        else if (value.indexOf(galleryPrefix) === 0)
            name = value.substring(galleryPrefix.length)
        if (!name.endsWith(".svg") || name.indexOf("/") >= 0)
            return ""
        return name.substring(0, name.length - 4)
    }

    function resolvedFileIconSource(entry, configuredSource, marker,
                                    logicalSize) {
        const configured = cleanText(configuredSource)
        if (configured === "" && cleanText(entry.name) === ""
                && cleanText(entry.localPath) === "")
            return ""
        if (configured !== "") {
            // An arbitrary file: icon is a user override, not part of the
            // selectable bundled pack. Only bundled Lucide URLs are replaced
            // when System is selected.
            if (!qtIcons.system
                    || bundledLucideIconName(configured) === "")
                return configured
        }
        if (cleanText(marker) !== "") {
            // Highlight markers remain above the built-in pack in the same
            // precedence order as the text frontend.
            return ""
        }

        const generation = qtIcons.revision
        return qtIcons.fileIconSource(cleanText(entry.localPath),
                                      cleanText(entry.name),
                                      entry.isDir === true,
                                      logicalSize,
                                      root.iconDevicePixelRatio,
                                      Number(entry.mtimeNanos || 0))
    }

    function isFullColorFileIcon(source) {
        return qtIcons.fileIconsAreFullColor
                && cleanText(source).indexOf("image://f4icons/file/") === 0
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
                    ? "<font color=\"#9fc8dc\">" + escaped + "</font>"
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
            "background": "transparent",
            "panelBackground": "transparent",
            "backgroundAlternate": panelBgAlt,
            "viewerBackground": panelBgAlt,
            "border": panelBorder,
            "activeBorder": activeBorder,
            "cursor": panelSelectionBorder,
            "cursorBackground": panelSelectionBg,
            "cursorBorder": panelSelectionBorder,
            "text": textColor,
            "mutedText": mutedText,
            "selection": markedBg,
            "marked": markedBg,
            "markedBackground": markedBg,
            "directoryText": "#98d8ff",
            "folderIcon": folderIconColor,
            "separator": separatorColor,
            "headerText": chromeText,
            "controlHover": controlHoverBg,
            "chrome": chromeBg,
            "chromeText": chromeText
        }
    }

    function galleryMetrics() {
        return {
            "detailsRowInset": panelRowInnerSpacing,
            "detailsRowSpacing": 8,
            "detailsIconSlotSize": 18,
            "detailsIconSize": 16,
            "detailsNameFontPixelSize": 13,
            "detailsSecondaryFontPixelSize": 12,
            "detailsExtensionMinimumWidth": 40,
            "detailsExtensionMaximumWidth": 80,
            "detailsSizeColumnWidth": 96,
            "detailsHeaderHeight": Math.max(22, ch)
                                   + verticalContentSpacing,
            "detailsHeaderCellInset": panelRowInnerSpacing,
            "detailsHeaderFontPixelSize": 12,
            "detailsSeparatorVerticalMargin":
                columnSeparatorVerticalMargin,
            "detailsSeparatorWidth": separatorWidth,
            "detailsScrollBarWidth": 16
        }
    }

    function action(map, preserveFocus) {
        qtShell.sendUiAction(map)
        if (preserveFocus !== true)
            grid.forceActiveFocus()
    }

    onSceneChanged: {
        syncAutocompleteSelection()
        if (qtGallery.viewerVisible
                && (hasDocumentSurface() || needsFallbackGrid()))
            qtGallery.closeViewer()
        Qt.callLater(root.restoreSurfaceFocus)
    }

    onClosing: qtShell.sendQuit()

    Shortcut {
        sequence: "Shift+F12"
		context: Qt.ApplicationShortcut
		enabled: !qtGallery.viewerVisible
		onActivated: root.action({"target": "app", "action": "presentation.toggle"}, true)
	}

    // Application shortcuts run before the focused hidden terminal grid and
    // repeat while the key is held. Modified arrows do not match these
    // sequences, so commander shortcuts and Shift-selection still reach Go.
    Shortcut {
        sequence: "Left"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeLocalPanelCanNavigate(Qt.Key_Left)
        onActivated: root.navigateActiveLocalPanel(Qt.Key_Left)
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

    Shortcut {
        sequence: "Right"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeLocalPanelCanNavigate(Qt.Key_Right)
        onActivated: root.navigateActiveLocalPanel(Qt.Key_Right)
    }

    Shortcut {
        sequence: "Up"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeLocalPanelCanNavigate(Qt.Key_Up)
        onActivated: root.navigateActiveLocalPanel(Qt.Key_Up)
    }

    Shortcut {
        sequence: "Down"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: root.activeLocalPanelCanNavigate(Qt.Key_Down)
        onActivated: root.navigateActiveLocalPanel(Qt.Key_Down)
    }

    VtuiGridItem {
        id: grid
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
        z: 0
        opacity: root.needsFallbackGrid() ? 1.0 : 0.0
        visible: true
        Component.onCompleted: forceActiveFocus()
    }

    Item {
        id: semanticLayer
        anchors.fill: parent
        z: 10
        visible: !root.needsFallbackGrid()

        Rectangle {
            anchors.fill: parent
            color: root.panelBg
        }

        Item {
            id: titleBar
            anchors.left: parent.left
            anchors.right: parent.right
            height: root.menuBarHeight
            z: 20

            Item {
                id: macSystemButtonArea
                visible: false
                x: root.macSystemButtonAreaLeftMargin
                y: root.titleBarContentVerticalOffset
                width: 70
                height: parent.height
            }

            SemanticMenuBar {
                id: semanticMenu
                menu: root.scene.menuBar || ({})
                anchors.left: parent.left
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
                anchors.right: windowButtons.left
                anchors.rightMargin: root.useMacNativeTitleBar
                                     ? -windowButtons.width
                                       + root.contentSpacing
                                     : root.contentSpacing
                anchors.verticalCenter: parent.verticalCenter
                anchors.verticalCenterOffset:
                    root.titleBarContentVerticalOffset
                width: visible
                       ? Math.min(titleBar.width * 0.46,
                                  workspaceItemsRow.width)
                       : 0
                height: 30
                visible: root.workspaceTabs.visible === true
                opacity: root.normalSurfaceOpacity

                Flickable {
                    id: workspaceFlick
                    anchors.fill: parent
                    contentWidth: workspaceItemsRow.width
                    contentHeight: height
                    boundsBehavior: Flickable.StopAtBounds
                    flickableDirection: Flickable.HorizontalFlick
                    clip: true

                    function revealCurrentWorkspace() {
                        contentX = Math.max(0, contentWidth - width)
                    }

                    onContentWidthChanged: revealCurrentWorkspace()
                    onWidthChanged: revealCurrentWorkspace()

                    Row {
                        id: workspaceItemsRow
                        width: childrenRect.width
                        height: parent.height
                        spacing: 4

                        Repeater {
                            model: root.workspaces

                            delegate: Rectangle {
                                id: workspaceTab
                                readonly property bool current: modelData.active === true
                                objectName: String(modelData.id || ("workspace-tab-" + index))
                                width: Math.min(148,
                                                Math.max(72,
                                                         workspaceLabel.implicitWidth
                                                         + (modelData.closable === true ? 42 : 24)))
                                height: parent.height
                                radius: 6
                                color: current
                                       ? root.panelSelectionBg
                                       : workspaceHover.hovered
                                         ? root.controlHoverBg : "transparent"
                                border.width: current ? 1 : 0
                                border.color: root.panelSelectionBorder

                                Component.onCompleted: {
                                    if (f4UsesQwk)
                                        windowAgent.setHitTestVisible(workspaceTab)
                                }

                                HoverHandler {
                                    id: workspaceHover
                                }

                                Text {
                                    id: workspaceLabel
                                    anchors.left: parent.left
                                    anchors.leftMargin: 10
                                    anchors.right: workspaceClose.visible
                                                   ? workspaceClose.left
                                                   : workspaceAttention.left
                                    anchors.rightMargin: 6
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: root.cleanText(modelData.text)
                                    color: workspaceTab.current
                                           ? root.textColor : root.chromeText
                                    font.weight: workspaceTab.current
                                                 ? Font.DemiBold : Font.Normal
                                    elide: Text.ElideMiddle
                                }

                                Rectangle {
                                    id: workspaceAttention
                                    anchors.right: parent.right
                                    anchors.rightMargin: workspaceClose.visible ? 29 : 10
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: 6
                                    height: 6
                                    radius: 3
                                    color: root.dialogAccent
                                    visible: modelData.attention === true
                                }

                                Text {
                                    id: workspaceClose
									z: 2
                                    anchors.right: parent.right
                                    anchors.rightMargin: 8
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: "×"
                                    color: root.chromeText
                                    font.pixelSize: 15
                                    visible: modelData.closable === true

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
                            objectName: root.cleanText(root.workspaceTabs.newTab
                                                       ? root.workspaceTabs.newTab.id : "workspace-new")
                            width: visible ? 30 : 0
                            height: parent.height
                            radius: 6
                            color: newHover.hovered ? root.controlHoverBg : "transparent"
                            visible: root.workspaceTabs.newTab
                                     && root.workspaceTabs.newTab.visible === true
                            Text {
                                anchors.centerIn: parent
                                text: root.cleanText(root.workspaceTabs.newTab.text || "+")
                                color: root.chromeText
                                font.pixelSize: 18
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

                        Rectangle {
                            id: workspaceCounter
                            objectName: root.cleanText(root.workspaceTabs.counter
                                                       ? root.workspaceTabs.counter.id : "workspace-counter")
                            width: visible ? Math.max(42, counterText.implicitWidth + 16) : 0
                            height: parent.height
                            radius: 6
                            color: counterHover.hovered ? root.controlHoverBg : "transparent"
                            visible: root.workspaceTabs.counter
                                     && root.workspaceTabs.counter.visible === true
                            Text {
                                id: counterText
                                anchors.centerIn: parent
                                text: root.cleanText(root.workspaceTabs.counter.text)
                                color: root.chromeText
                            }
                            HoverHandler { id: counterHover }
                            MouseArea {
                                anchors.fill: parent
                                cursorShape: Qt.PointingHandCursor
                                onClicked: root.action({
                                    "target": root.workspaceTabs.counter.id,
                                    "action": root.workspaceTabs.counter.action
                                }, true)
                            }
                        }
                    }
                }
            }

            Row {
                id: windowButtons
                anchors.top: parent.top
                anchors.right: parent.right
                height: parent.height
                visible: f4UsesQwk && !root.useMacNativeTitleBar

                component WindowButton: Rectangle {
                    required property string label
                    property bool closeControl: false
                    width: 46
                    height: windowButtons.height
                    color: "transparent"

                    Text {
                        anchors.centerIn: parent
                        text: parent.label
                        color: root.chromeText
                        font.pixelSize: 14
                    }

                    MouseArea {
                        id: buttonMouse
                        anchors.fill: parent
                        hoverEnabled: true
                        onClicked: parent.clicked()
                    }

                    signal clicked()
                }

                WindowButton {
                    id: minimizeButton
                    label: "−"
                    onClicked: root.showMinimized()
                }

                WindowButton {
                    id: maximizeButton
                    label: root.visibility === Window.Maximized ? "❐" : "□"
                    onClicked: root.visibility === Window.Maximized
                               ? root.showNormal() : root.showMaximized()
                }

                WindowButton {
                    id: closeButton
                    label: "×"
                    closeControl: true
                    onClicked: root.close()
                }
            }
        }

        Loader {
            id: mainSurface
            anchors.fill: parent
            opacity: root.normalSurfaceOpacity
            sourceComponent: {
                // A shell with hidden panels is still the same commander
                // surface: its terminal buffer and command line must remain
                // composed together by panelsSurface. Reserve
                // documentSurface for standalone viewer/editor frames.
                if (root.shellFrame().terminalActive === true)
                    return panelsSurface
                var top = root.activeSurface()
                if (root.isDocumentSurface(top))
                    return documentSurface
                return panelsSurface
            }
        }

        Loader {
            id: galleryViewerLayer
            anchors.fill: parent
            active: qtGallery.viewerVisible && !root.hasDocumentSurface()
                    && !root.needsFallbackGrid()
            visible: active
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
                    var infoPanels = frame.infoPanels || []
                    for (var i = 0; i < infoPanels.length; ++i) {
                        if (Number(infoPanels[i].side) === side)
                            return infoPanels[i]
                    }
                    return null
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
                    visible: panelsRoot.frame.terminalActive === true
                             || !root.panelSideVisible(0)
                             || !root.panelSideVisible(1)
                    terminal: panelsRoot.frame.terminal || ({})
                }

                Loader {
                    id: persistentPanelPair
                    objectName: "persistentPanelPair"
                    anchors.fill: parent
                    // Ctrl+O is a presentation-only cover change.  Destroying
                    // this Loader used to recreate both FilePanelView trees on
                    // return, which in turn rebuilt their ListView/Gallery
                    // viewports and revealed the cursor from contentY == 0.
                    // Keep the pair (and both panels' local scroll state) alive
                    // underneath the terminal, exactly like the terminal is
                    // kept alive underneath the panels.
                    active: true
                    visible: frame.terminalActive !== true
                    sourceComponent: panelPairSurface
                }

                CommandLineView {
                    commandLine: frame.commandLine || ({})
                }

                Component {
                    id: panelPairSurface
                    Item {
                        id: panelPairRoot
                        anchors.fill: parent

                        FilePanelView {
                            panel: panelsRoot.panelForSide(0)
                            visible: root.panelSideVisible(0)
                                      && !panelsRoot.infoPanelForSide(0)
                        }

                        FilePanelView {
                            panel: panelsRoot.panelForSide(1)
                            visible: root.panelSideVisible(1)
                                      && !panelsRoot.infoPanelForSide(1)
                        }

                        InfoPanelView {
                            panel: panelsRoot.infoPanelForSide(0) || ({ "side": 0 })
                            visible: root.panelSideVisible(0)
                                     && panelsRoot.infoPanelForSide(0) !== null
                        }

                        InfoPanelView {
                            panel: panelsRoot.infoPanelForSide(1) || ({ "side": 1 })
                            visible: root.panelSideVisible(1)
                                     && panelsRoot.infoPanelForSide(1) !== null
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
                frame: root.activeSurface() || ({})
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
                    item.theme = root.galleryTheme()
                    item.surfaceActive = Qt.binding(
                        () => qtGallery.viewerVisible
                              && !root.hasBlockingOverlay()
                              && !root.hasDocumentSurface()
                              && !root.needsFallbackGrid())
                    item.devicePixelRatio = Qt.binding(
                        () => root.screen ? root.screen.devicePixelRatio : 1.0)
                    item.forceActiveFocus()
                }
            }
        }

        Repeater {
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
            keyBar: root.scene.keyBar || ({})
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: root.keyBarHeight()
            opacity: root.normalSurfaceOpacity
            z: 40
        }

        ToastView {
            toast: root.scene.toast || ({})
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
        property bool nativeLayout: root.isAppScene()
        property real topChromeOffset: nativeLayout ? 0 : ((panel.y || 0) <= 0 ? semanticMenu.height : 0)
        readonly property bool usesGallery: qtGallery.shouldUseGallery(panel)
        readonly property bool panelIsActive: panel.active === true
        property var registeredGalleryPanelHost: null
        readonly property var rendererChoices: [
            { "heading": true, "label": "Existing" },
            { "label": "Brief", "viewMode": "brief" },
            { "label": "Medium", "viewMode": "medium" },
            { "label": "Detailed", "viewMode": "detailed" },
            { "label": "Wide", "viewMode": "wide" },
            { "heading": true, "label": "Unified" },
            { "label": "Masonry", "layoutMode": "masonry" },
            { "label": "Columns · 2", "layoutMode": "columns", "columnCount": 2 },
            { "label": "Columns · 3", "layoutMode": "columns", "columnCount": 3 },
            { "label": "Details", "layoutMode": "details" },
            { "label": "Grid", "layoutMode": "grid" },
            { "label": "Icons", "layoutMode": "icons" }
        ]

        function rendererChoiceEnabled(choice) {
            if (!choice || choice.heading === true)
                return false
            if (choice.viewMode !== undefined)
                return true
            return qtGallery.available && panel.previewCapable === true
                    && panel.sourceKind === "local"
        }

        function rendererChoiceActive(choice) {
            if (!choice || choice.heading === true)
                return false
            if (choice.viewMode !== undefined) {
                return panel.presentation !== "gallery"
                        && root.cleanText(panel.viewModeName) === choice.viewMode
            }
            if (panel.presentation !== "gallery"
                    || root.cleanText(panel.galleryLayoutMode) !== choice.layoutMode)
                return false
            return choice.layoutMode !== "columns"
                    || Number(panel.galleryColumnCount || 2)
                       === Number(choice.columnCount || 2)
        }

        function rendererButtonText() {
            if (panel.presentation === "gallery") {
                const mode = root.cleanText(panel.galleryLayoutMode)
                if (mode === "columns")
                    return "C" + Number(panel.galleryColumnCount || 2)
                if (mode === "details") return "D"
                if (mode === "grid") return "G"
                if (mode === "icons") return "I"
                return "M"
            }
            const mode = root.cleanText(panel.viewModeName)
            return mode === "brief" ? "B"
                 : mode === "detailed" ? "D"
                 : mode === "wide" ? "W" : "M"
        }

        function chooseRenderer(choice) {
            if (!rendererChoiceEnabled(choice))
                return
            if (choice.viewMode !== undefined) {
                root.action({
                    "action": "panel.setViewMode",
                    "side": panel.side,
                    "viewMode": choice.viewMode
                }, true)
            } else {
                qtGallery.requestGalleryLayout(
                            panel.side, choice.layoutMode,
                            Number(choice.columnCount || 0))
            }
            rendererMenu.close()
        }

        function pointerButtonName(button) {
            if ((button & Qt.RightButton) !== 0)
                return "right"
            if ((button & Qt.MiddleButton) !== 0)
                return "middle"
            return "left"
        }

        function sendPointerEvent(entry, phase, button) {
            if (!entry || entry.index === undefined)
                return
            root.action({
                "action": "panel.pointer",
                "side": panel.side,
                "phase": phase,
                "button": typeof button === "string"
                          ? button : pointerButtonName(button),
                "index": entry.index,
                "entryId": entry.entryId || "",
                "catalogRevision": panel.catalogRevision || 0
            })
        }

        function fileNameParts(entry) {
            var name = root.cleanText(entry && entry.name !== undefined
                                      ? entry.name : "")
            var displayBaseName = root.cleanText(
                        entry ? entry.displayBaseName : "")
            if (displayBaseName !== "" || name === "") {
                return {
                    "base": displayBaseName,
                    "extension": root.cleanText(entry.displayExtension)
                }
            }
            if (panel.separateFileExtensions !== true || !entry
                    || entry.isDir === true || entry.isUp === true)
                return { "base": name, "extension": "" }
            var dot = name.lastIndexOf(".")
            if (dot <= 0 || dot >= name.length - 1)
                return { "base": name, "extension": "" }
            return {
                "base": name.substring(0, dot),
                "extension": name.substring(dot + 1)
            }
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
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: Math.max(25, root.ch * 1.25)
                    + root.verticalContentSpacing
                    + root.pathRowExtraHeight
            color: root.panelPathBg
            z: 2

            Rectangle {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.top: parent.top
                height: root.separatorWidth
                color: root.separatorColor
            }

            Rectangle {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                height: root.separatorWidth
                color: root.separatorColor
            }

            Text {
                anchors.left: parent.left
                anchors.right: presentationButton.left
                anchors.verticalCenter: parent.verticalCenter
                anchors.leftMargin: root.panelTextInset
                anchors.rightMargin: root.panelTextInset
                text: root.cleanText(panel.title || panel.path)
                color: root.textColor
                elide: Text.ElideMiddle
                font.pixelSize: 13
            }

            ToolButton {
                id: presentationButton
                objectName: "panelRendererButton-" + Number(panel.side || 0)
                anchors.right: loading.left
                anchors.verticalCenter: parent.verticalCenter
                width: 32
                height: Math.min(parent.height - 4, 28)
                hoverEnabled: true
                text: panelRoot.rendererButtonText()
                focusPolicy: Qt.NoFocus
                // Once the renderer popup is open its contents are the
                // active explanation.  Keeping the delayed button tooltip
                // alive above the popup creates a detached black label that
                // overlaps the menu and obscures the first interaction.
                ToolTip.visible: hovered && !rendererMenu.opened
                ToolTip.text: "Panel renderer"

                contentItem: Text {
                    text: presentationButton.text + " ⌄"
                    color: presentationButton.enabled
                           ? root.chromeText : root.mutedText
                    font.pixelSize: 11
                    font.weight: Font.DemiBold
                    horizontalAlignment: Text.AlignHCenter
                    verticalAlignment: Text.AlignVCenter
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
                    if (rendererMenu.opened)
                        rendererMenu.close()
                    else
                        rendererMenu.open()
                }
            }

            Popup {
                id: rendererMenu
                objectName: "panelRendererMenu-" + Number(panel.side || 0)
                parent: Overlay.overlay
                width: 224
                padding: 6
                modal: false
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
                            || root.hasDocumentSurface())
                        return
                    if (panelRoot.usesGallery && galleryPanelContent.item)
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
                        model: panelRoot.rendererChoices

                        delegate: Rectangle {
                            id: rendererChoice
                            required property int index
                            required property var modelData
                            width: rendererMenu.availableWidth
                            height: modelData.heading === true ? 25 : 31
                            radius: 5
                            readonly property bool choiceEnabled:
                                panelRoot.rendererChoiceEnabled(modelData)
                            readonly property bool choiceActive:
                                panelRoot.rendererChoiceActive(modelData)
                            color: modelData.heading === true ? "transparent"
                                   : rendererChoicePointer.containsMouse
                                     && choiceEnabled
                                     ? root.controlHoverBg : "transparent"

                            Rectangle {
                                visible: rendererChoice.modelData.heading === true
                                         && index > 0
                                anchors.left: parent.left
                                anchors.right: parent.right
                                anchors.top: parent.top
                                height: 1
                                color: root.separatorColor
                            }

                            Text {
                                visible: rendererChoice.modelData.heading === true
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

                            IconLabel {
                                visible: rendererChoice.modelData.heading !== true
                                         && rendererChoice.choiceActive
                                anchors.left: parent.left
                                anchors.leftMargin: 7
                                anchors.verticalCenter: parent.verticalCenter
                                width: 15
                                height: 15
                                icon.source: root.resolvedIconSource("check", 15)
                                icon.width: 14
                                icon.height: 14
                                icon.color: root.panelSelectionBorder
                            }

                            Text {
                                visible: rendererChoice.modelData.heading !== true
                                anchors.left: parent.left
                                anchors.leftMargin: 29
                                anchors.right: parent.right
                                anchors.rightMargin: 8
                                anchors.verticalCenter: parent.verticalCenter
                                text: root.cleanText(rendererChoice.modelData.label)
                                color: rendererChoice.choiceEnabled
                                       ? root.textColor : root.mutedText
                                opacity: rendererChoice.choiceEnabled ? 1 : 0.5
                                font.pixelSize: 12
                                elide: Text.ElideRight
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
                }
            }

            Text {
                id: loading
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                anchors.rightMargin: root.panelTextInset
                text: panel.loading ? "Loading"
                                    : (panelRoot.usesGallery
                                       && String(panel.galleryLayoutMode
                                                 || "masonry") !== "details"
                                       ? "Gallery · "
                                         + root.cleanText(panel.sortModeName)
                                       : "")
                color: root.mutedText
                font.pixelSize: 12
            }
        }

        Rectangle {
            id: columnHeader
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: panelHeader.bottom
            readonly property bool showsGalleryDetails:
                panelRoot.usesGallery
                && String(panel.galleryLayoutMode || "masonry") === "details"
            height: panelRoot.usesGallery && !showsGalleryDetails
                    ? 0 : Math.max(22, root.ch) + root.verticalContentSpacing
            visible: !panelRoot.usesGallery || showsGalleryDetails
            color: "transparent"
            z: 2

            readonly property var columns: showsGalleryDetails
                ? (panel.galleryColumns || []) : (panel.columns || [])
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
            id: listPanelContent
            objectName: "listPanelContent-" + Number(panel.side || 0)
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.leftMargin: root.panelContentSpacing
            anchors.rightMargin: root.panelContentSpacing
            anchors.top: parent.top
            anchors.topMargin: panelHeader.height + columnHeader.height
            anchors.bottom: status.top
            z: 1
            // A hidden side or a temporary Info panel is only a cover change;
            // retaining the active presentation avoids reconstructing its
            // ListView and losing the exact fractional contentY/local cursor.
            active: !panelRoot.usesGallery
            visible: panelRoot.visible && active
            sourceComponent: panel.viewModeName === "brief"
                             || panel.viewModeName === "medium"
                             ? compactPanelComponent : listPanelComponent
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
            // GallerySession survives Loader recreation, but renderer-local
            // layout state and its live delegates do not.  Keep this host alive
            // while the panel is hidden so showing it is a pure visibility
            // flip with no restore/reveal pass and no thumbnail blink.
            active: panelRoot.usesGallery
            visible: panelRoot.visible && active
            source: active ? qtGallery.panelComponentUrl : ""

            onItemChanged: panelRoot.updateRegisteredGalleryPanelHost()

            onLoaded: {
                if (!item)
                    return
                item.side = panel.side
                item.panel = Qt.binding(() => panelRoot.panel)
                item.bridge = qtGallery
                item.keySink = grid
                item.theme = root.galleryTheme()
                item.metrics = Qt.binding(() => root.galleryMetrics())
                item.devicePixelRatio = Qt.binding(
                    () => root.screen ? root.screen.devicePixelRatio : 1.0)
                item.defaultListDensity = Qt.binding(
                    () => Math.max(22, root.ch * 1.1))
                item.panelActive = Qt.binding(
                    () => panelRoot.visible
                          && panelRoot.panelIsActive
                          && !qtGallery.viewerVisible
                          && !root.needsFallbackGrid()
                          && !root.hasDocumentSurface()
                          && !root.hasBlockingOverlay())
                item.commandLineHasText = Qt.binding(() => {
                    var shell = root.shellFrame()
                    var commandLine = shell && shell.commandLine
                                      ? shell.commandLine : ({})
                    return root.cleanText(commandLine.text).length > 0
                })
                item.fastFindActive = Qt.binding(
                    () => panelRoot.panel.fastFind === true)
                if (item.panelActive)
                    item.forceActiveFocus()
                panelRoot.updateRegisteredGalleryPanelHost()
            }
        }

        onUsesGalleryChanged: {
            if (!visible || !panelIsActive || qtGallery.viewerVisible
                    || root.hasBlockingOverlay() || root.needsFallbackGrid()
                    || root.hasDocumentSurface())
                return
            if (usesGallery && galleryPanelContent.item)
                galleryPanelContent.item.forceActiveFocus()
            else if (!usesGallery)
                grid.forceActiveFocus()
        }

        onPanelIsActiveChanged: {
            if (!visible || !panelIsActive || qtGallery.viewerVisible
                    || root.hasBlockingOverlay() || root.needsFallbackGrid()
                    || root.hasDocumentSurface())
                return
            if (usesGallery && galleryPanelContent.item)
                galleryPanelContent.item.forceActiveFocus()
            else
                grid.forceActiveFocus()
        }

        Connections {
            target: qtGallery
            function onViewerChanged() {
                if (qtGallery.viewerVisible || root.hasBlockingOverlay()
                        || root.needsFallbackGrid()
                        || root.hasDocumentSurface())
                    return
                if (panelRoot.visible && panelRoot.usesGallery
                        && panelRoot.panelIsActive
                        && galleryPanelContent.item) {
                    galleryPanelContent.item.forceActiveFocus()
                } else if (panelRoot.visible && panelRoot.panelIsActive
                           && !panelRoot.usesGallery) {
                    grid.forceActiveFocus()
                }
            }
        }

        Component {
            id: compactPanelComponent

            Item {
                id: compactPanel
                clip: true
                readonly property int columnCount:
                    panel.viewModeName === "brief" ? 3 : 2
                readonly property real rowHeight: Math.max(22, root.ch * 1.1)
                readonly property int rowsPerColumn:
                    Math.max(1, Math.floor(height / rowHeight))
                readonly property real columnWidth: width / columnCount
                readonly property int backendCursor: Number(panel.cursor || 0)
                readonly property int backendCatalogRevision:
                    Number(panel.catalogRevision || 0)
                readonly property int backendSelectionRevision:
                    Number(panel.selectionRevision || 0)
                readonly property int backendHighlightRevision:
                    Number(panel.highlightRevision || 0)
                readonly property bool backendSeparateFileExtensions:
                    panel.separateFileExtensions === true
                readonly property bool backendActive: panel.active === true
                property int localCursor: backendCursor
                property int localTop: Math.max(0, Number(panel.top || 0))
                property real scrollPosition: localTop
                property real wheelTarget: localTop
                property bool localCursorOverride: false
                property var cachedEntries: []
                property var cachedHighlightStyles: ({})

                Timer {
                    id: cursorCommitTimer
                    interval: 70
                    repeat: false
                    onTriggered: compactPanel.commitLocalCursor()
                }

                NumberAnimation {
                    id: compactWheelAnimation
                    target: compactPanel
                    property: "scrollPosition"
                    duration: 130
                    easing.type: Easing.OutCubic
                }

                Component.onCompleted: {
                    refreshCachedEntries()
                    root.setLocalPanelHost(panel.side, compactPanel)
                    ensureCursorVisible()
                }
                Component.onDestruction: {
                    if (cursorCommitTimer.running)
                        commitLocalCursor()
                    root.clearLocalPanelHost(panel.side, compactPanel)
                }

                onBackendCatalogRevisionChanged: {
                    compactWheelAnimation.stop()
                    cursorCommitTimer.stop()
                    refreshCachedEntries()
                    localCursor = backendCursor
                    localTop = Math.max(0, Number(panel.top || 0))
                    scrollPosition = localTop
                    wheelTarget = localTop
                    localCursorOverride = false
                    ensureCursorVisible()
                }
                onBackendSelectionRevisionChanged: refreshCachedEntries()
                onBackendHighlightRevisionChanged: refreshCachedEntries()
                onBackendSeparateFileExtensionsChanged: refreshCachedEntries()
                onBackendCursorChanged: {
                    // Repeated-key acknowledgements can arrive behind the
                    // locally rendered cursor. Ignore those intermediate
                    // scenes and converge only on the latest request.
                    if (!localCursorOverride) {
                        localCursor = backendCursor
                        ensureCursorVisible()
                    } else if (!cursorCommitTimer.running
                               && backendCursor === localCursor) {
                        localCursorOverride = false
                    }
                }
                onRowsPerColumnChanged: ensureCursorVisible()
                onScrollPositionChanged: {
                    var nextTop = clampTop(Math.round(scrollPosition))
                    if (nextTop !== localTop)
                        localTop = nextTop
                    syncGalleryScrollBar()
                }

                function refreshCachedEntries() {
                    cachedEntries = panel.entries || []
                    cachedHighlightStyles = panel.highlightStyles || ({})
                }

                function maximumTop() {
                    var count = cachedEntries.length
                    return Math.max(0, count - rowsPerColumn * columnCount)
                }

                function clampTop(value) {
                    return Math.max(0, Math.min(maximumTop(), value))
                }

                function setTop(value) {
                    compactWheelAnimation.stop()
                    localTop = Math.round(clampTop(value))
                    scrollPosition = localTop
                    wheelTarget = localTop
                    syncGalleryScrollBar()
                }

                function scrollByWheel(wheel) {
                    var pixelY = Number(wheel.pixelDelta.y || 0)
                    var angleY = Number(wheel.angleDelta.y || 0)
                    var rows = pixelY !== 0 ? pixelY / rowHeight
                                           : (angleY / 120) * 3
                    if (rows === 0)
                        return false
                    wheelTarget = clampTop(wheelTarget - rows)
                    compactWheelAnimation.stop()
                    compactWheelAnimation.from = scrollPosition
                    compactWheelAnimation.to = wheelTarget
                    compactWheelAnimation.start()
                    return true
                }

                function syncGalleryScrollBar() {
                    var scrollBar = compactScrollBar.item
                    if (!scrollBar || scrollBar.pressed)
                        return
                    var count = cachedEntries.length
                    var capacity = rowsPerColumn * columnCount
                    scrollBar.size = count > 0
                            ? Math.min(1, capacity / count) : 1
                    scrollBar.position = count > 0
                            ? scrollPosition / count : 0
                }

                function ensureCursorVisible() {
                    var capacity = rowsPerColumn * columnCount
                    if (localCursor < localTop)
                        setTop(localCursor)
                    else if (localCursor >= localTop + capacity)
                        setTop(localCursor - capacity + 1)
                    else if (localTop !== clampTop(localTop))
                        setTop(localTop)
                }

                function commitLocalCursor() {
                    cursorCommitTimer.stop()
                    var entries = cachedEntries
                    if (localCursor < 0 || localCursor >= entries.length)
                        return
                    var targetEntry = entries[localCursor]
                    root.action({
                        "action": "panel.cursor",
                        "side": panel.side,
                        "index": targetEntry.index !== undefined
                                 ? targetEntry.index : localCursor,
                        "entryId": targetEntry.entryId || "",
                        "catalogRevision": panel.catalogRevision || 0,
                        "activate": false
                    }, true)
                }

                function selectLocally(target, nextTop) {
                    if (target === localCursor)
                        return false
                    localCursor = target
                    setTop(nextTop)
                    ensureCursorVisible()
                    localCursorOverride = true
                    // Holding an arrow changes only QML state. Once input
                    // pauses, one stable-ID cursor update synchronizes Go.
                    cursorCommitTimer.restart()
                    return true
                }

                function navigate(key) {
                    var entries = cachedEntries
                    var current = localCursor
                    var top = localTop
                    var relative = current - top
                    var capacity = rowsPerColumn * columnCount
                    if (current < 0 || current >= entries.length
                            || relative < 0 || relative >= capacity)
                        return false

                    var column = Math.floor(relative / rowsPerColumn)
                    var target = current
                    var nextTop = top
                    if (key === Qt.Key_Left) {
                        if (column > 0) {
                            target -= rowsPerColumn
                        } else {
                            nextTop = clampTop(top - capacity)
                            target = nextTop === top
                                     ? 0 : nextTop + relative
                        }
                    } else if (key === Qt.Key_Right) {
                        if (column < columnCount - 1) {
                            target += rowsPerColumn
                        } else {
                            nextTop = clampTop(top + capacity)
                            target = nextTop === top
                                     ? entries.length - 1
                                     : Math.min(entries.length - 1,
                                                nextTop + relative)
                        }
                    } else if (key === Qt.Key_Up) {
                        target--
                        if (target < top)
                            nextTop = target
                    } else if (key === Qt.Key_Down) {
                        target++
                        if (target >= top + capacity)
                            nextTop = target - capacity + 1
                    } else {
                        return false
                    }

                    if (target < 0 || target >= entries.length
                            || target === current)
                        return false
                    return selectLocally(target, nextTop)
                }

                Repeater {
                    // Only visible cells exist as QML objects. Cursor-only
                    // semantic scenes therefore update a few dozen bindings,
                    // not every entry in a large directory.
                    model: compactPanel.rowsPerColumn * compactPanel.columnCount

                    delegate: Rectangle {
                        id: compactFileCell
                        readonly property int entryIndex:
                            compactPanel.localTop + index
                        readonly property var entry:
                            compactPanel.cachedEntries[entryIndex] || ({})
                        readonly property int relativeIndex: index
                        readonly property bool cursorState:
                            compactPanel.backendActive
                            && entryIndex === compactPanel.localCursor
                        readonly property var highlightStyle: {
                            var styles = compactPanel.cachedHighlightStyles
                            var styleId = entry.highlightStyleId || ""
                            return styleId !== "" && styles[styleId]
                                    ? styles[styleId] : ({})
                        }
                        readonly property var highlightPatch: {
                            if (cursorState && entry.selected)
                                return highlightStyle.selectedCursor || ({})
                            if (cursorState)
                                return highlightStyle.cursor || ({})
                            if (entry.selected)
                                return highlightStyle.selected || ({})
                            return highlightStyle.normal || ({})
                        }
                        readonly property string highlightForeground:
                            highlightPatch.foreground || ""
                        readonly property string highlightBackground:
                            highlightPatch.background || ""
                        readonly property string highlightMarker:
                            highlightStyle.marker || ""
                        readonly property url highlightIcon:
                            root.resolvedFileIconSource(
                                entry, highlightStyle.icon || "",
                                highlightMarker, 16)
                        readonly property var fileNameParts:
                            panelRoot.fileNameParts(entry)

                        visible: entryIndex >= 0
                                 && entryIndex < compactPanel.cachedEntries.length
                        x: Math.floor(relativeIndex / compactPanel.rowsPerColumn)
                           * compactPanel.columnWidth
                        y: (relativeIndex % compactPanel.rowsPerColumn)
                           * compactPanel.rowHeight
                        width: compactPanel.columnWidth
                        height: compactPanel.rowHeight
                        color: highlightBackground !== ""
                               ? highlightBackground
                               : cursorState ? root.panelSelectionBg
                               : entry.selected ? root.markedBg
                               : "transparent"
                        radius: 4
                        border.width: cursorState ? 1 : 0
                        border.color: root.panelSelectionBorder

                        RowLayout {
                            anchors.fill: parent
                            anchors.leftMargin: root.panelRowInnerSpacing
                            anchors.rightMargin: root.panelRowInnerSpacing
                            spacing: 6

                            Item {
                                Layout.preferredWidth: 18
                                Layout.preferredHeight: 18

                                IconLabel {
                                    anchors.centerIn: parent
                                    visible:
                                        compactFileCell.highlightIcon.toString() !== ""
                                    icon.source: compactFileCell.highlightIcon
                                    icon.width: 16
                                    icon.height: 16
                                    icon.color: root.isFullColorFileIcon(
                                                    compactFileCell.highlightIcon)
                                                ? "transparent"
                                                : compactFileCell.entry.isDir
                                                ? (compactFileCell.cursorState
                                                   ? root.textColor
                                                   : root.folderIconColor)
                                                : compactFileCell.highlightForeground !== ""
                                                  ? compactFileCell.highlightForeground
                                                  : root.mutedText
                                }

                                Text {
                                    anchors.centerIn: parent
                                    visible:
                                        compactFileCell.highlightIcon.toString() === ""
                                    text: compactFileCell.highlightMarker !== ""
                                          ? compactFileCell.highlightMarker
                                          : compactFileCell.entry.isDir
                                            ? (compactFileCell.entry.isUp ? "↰" : "▸") : " "
                                    color: compactFileCell.entry.isDir
                                           ? (compactFileCell.cursorState
                                              ? root.textColor
                                              : root.folderIconColor)
                                           : compactFileCell.highlightForeground !== ""
                                             ? compactFileCell.highlightForeground
                                             : root.mutedText
                                    font.pixelSize: 13
                                }
                            }

                            Text {
                                text: compactFileCell.fileNameParts.base
                                color: compactFileCell.highlightForeground !== ""
                                       ? compactFileCell.highlightForeground
                                       : (compactFileCell.entry.isDir ? "#98d8ff"
                                                          : root.textColor)
                                elide: Text.ElideMiddle
                                font.pixelSize: 13
                                Layout.fillWidth: true
                            }

                            Text {
                                text: compactFileCell.fileNameParts.extension
                                visible: text.length > 0
                                color: compactFileCell.highlightForeground !== ""
                                       ? compactFileCell.highlightForeground
                                       : root.mutedText
                                elide: Text.ElideRight
                                horizontalAlignment: Text.AlignLeft
                                font.pixelSize: 12
                                Layout.preferredWidth: Math.min(64,
                                    Math.max(32, implicitWidth))
                            }
                        }

                        MouseArea {
                            id: compactPointer
                            anchors.fill: parent
                            acceptedButtons: Qt.AllButtons
                            property string pressedButton: ""
                            property int lastDragIndex: -1
                            onPressed: mouse => {
                                cursorCommitTimer.stop()
                                compactPanel.localCursor = compactFileCell.entryIndex
                                compactPanel.localCursorOverride = true
                                pressedButton = panelRoot.pointerButtonName(mouse.button)
                                lastDragIndex = compactFileCell.entryIndex
                                panelRoot.sendPointerEvent(compactFileCell.entry,
                                                           "down", pressedButton)
                                mouse.accepted = true
                            }
                            onPositionChanged: mouse => {
                                if (pressedButton === "" || mouse.buttons === Qt.NoButton)
                                    return
                                var point = mapToItem(compactPanel, mouse.x, mouse.y)
                                var column = Math.floor(point.x / compactPanel.columnWidth)
                                var row = Math.floor(point.y / compactPanel.rowHeight)
                                if (column < 0 || column >= compactPanel.columnCount
                                        || row < 0 || row >= compactPanel.rowsPerColumn)
                                    return
                                var target = compactPanel.localTop + row
                                             + column * compactPanel.rowsPerColumn
                                if (target === lastDragIndex || target < 0
                                        || target >= compactPanel.cachedEntries.length)
                                    return
                                lastDragIndex = target
                                compactPanel.localCursor = target
                                compactPanel.localCursorOverride = true
                                panelRoot.sendPointerEvent(
                                    compactPanel.cachedEntries[target], "move",
                                    pressedButton)
                            }
                            onReleased: mouse => {
                                panelRoot.sendPointerEvent(compactFileCell.entry,
                                                           "up", pressedButton)
                                pressedButton = ""
                                lastDragIndex = -1
                            }
                            onCanceled: {
                                panelRoot.sendPointerEvent(compactFileCell.entry,
                                                           "cancel", pressedButton)
                                pressedButton = ""
                                lastDragIndex = -1
                            }
                            onClicked: mouse => panelRoot.sendPointerEvent(
                                compactFileCell.entry, "click", mouse.button)
                            onDoubleClicked: mouse => panelRoot.sendPointerEvent(
                                compactFileCell.entry, "doubleClick", mouse.button)
                            onWheel: wheel => {
                                wheel.accepted = compactPanel.scrollByWheel(wheel)
                            }
                        }
                    }
                }

                WheelHandler {
                    id: compactWheelHandler
                    target: null
                    acceptedDevices: PointerDevice.Mouse
                                     | PointerDevice.TouchPad
                    onWheel: event => {
                        event.accepted = compactPanel.scrollByWheel(event)
                    }
                }

                Loader {
                    id: compactScrollBar
                    objectName: "filePanelScrollBar-" + Number(panel.side || 0)
                    anchors.top: parent.top
                    anchors.bottom: parent.bottom
                    anchors.right: parent.right
                    width: active ? 16 : 0
                    z: 10
                    active: qtGallery.available && compactPanel.maximumTop() > 0
                    visible: active
                    source: active ? qtGallery.scrollBarComponentUrl : ""

                    onLoaded: {
                        if (!item)
                            return
                        item.theme = root.galleryTheme()
                        item.orientation = Qt.Vertical
                        compactPanel.syncGalleryScrollBar()
                    }

                    Connections {
                        target: compactScrollBar.item
                        function onPositionChanged() {
                            var scrollBar = compactScrollBar.item
                            if (!scrollBar || !scrollBar.pressed)
                                return
                            compactWheelAnimation.stop()
                            var count = compactPanel.cachedEntries.length
                            compactPanel.scrollPosition = compactPanel.clampTop(
                                        scrollBar.position * count)
                            compactPanel.wheelTarget = compactPanel.scrollPosition
                        }
                    }
                }
            }
        }

        Component {
            id: listPanelComponent

            ListView {
                id: list
                clip: true
                readonly property real rowHeight: Math.max(22, root.ch * 1.1)
                readonly property int rowsPerPage:
                    Math.max(1, Math.floor(height / rowHeight))
                readonly property int backendCursor: Number(panel.cursor || 0)
                readonly property int backendCatalogRevision:
                    Number(panel.catalogRevision || 0)
                readonly property int backendSelectionRevision:
                    Number(panel.selectionRevision || 0)
                readonly property int backendHighlightRevision:
                    Number(panel.highlightRevision || 0)
                readonly property bool backendSeparateFileExtensions:
                    panel.separateFileExtensions === true
                property int localCursor: backendCursor
                property bool localCursorOverride: false
                property var cachedEntries: []
                property var cachedHighlightStyles: ({})
                // Keep delegates alive across cursor/selection-only semantic
                // snapshots. Replacing a JS-array model after right mouse-down
                // destroys the MouseArea before Qt can deliver the second
                // press of a double-click.
                model: cachedEntries.length
                currentIndex: localCursor
                boundsBehavior: Flickable.StopAtBounds

                Timer {
                    id: listCursorCommitTimer
                    interval: 70
                    repeat: false
                    onTriggered: list.commitLocalCursor()
                }

                function refreshCachedEntries() {
                    cachedEntries = panel.entries || []
                    cachedHighlightStyles = panel.highlightStyles || ({})
                }

                function ensureCursorVisible() {
                    if (localCursor >= 0 && localCursor < cachedEntries.length)
                        positionViewAtIndex(localCursor, ListView.Contain)
                }

                function commitLocalCursor() {
                    listCursorCommitTimer.stop()
                    if (localCursor < 0 || localCursor >= cachedEntries.length)
                        return
                    var targetEntry = cachedEntries[localCursor]
                    root.action({
                        "action": "panel.cursor",
                        "side": panel.side,
                        "index": targetEntry.index !== undefined
                                 ? targetEntry.index : localCursor,
                        "entryId": targetEntry.entryId || "",
                        "catalogRevision": panel.catalogRevision || 0,
                        "activate": false
                    }, true)
                }

                function selectLocally(target) {
                    target = Math.max(0, Math.min(cachedEntries.length - 1,
                                                  target))
                    if (target === localCursor || cachedEntries.length === 0)
                        return false
                    localCursor = target
                    localCursorOverride = true
                    ensureCursorVisible()
                    listCursorCommitTimer.restart()
                    return true
                }

                function navigate(key) {
                    var delta = 0
                    if (key === Qt.Key_Up)
                        delta = -1
                    else if (key === Qt.Key_Down)
                        delta = 1
                    else if (key === Qt.Key_Left)
                        delta = -rowsPerPage
                    else if (key === Qt.Key_Right)
                        delta = rowsPerPage
                    else
                        return false
                    return selectLocally(localCursor + delta)
                }

                function canNavigate(key) {
                    if (key === Qt.Key_Up || key === Qt.Key_Down)
                        return true
                    return panel.viewModeName === "detailed"
                            && !root.commandLineHasText()
                }

                Component.onCompleted: {
                    refreshCachedEntries()
                    root.setLocalPanelHost(panel.side, list)
                    ensureCursorVisible()
                }
                Component.onDestruction: {
                    if (listCursorCommitTimer.running)
                        commitLocalCursor()
                    root.clearLocalPanelHost(panel.side, list)
                }
                onBackendCatalogRevisionChanged: {
                    listCursorCommitTimer.stop()
                    refreshCachedEntries()
                    localCursor = backendCursor
                    localCursorOverride = false
                    ensureCursorVisible()
                }
                onBackendSelectionRevisionChanged: refreshCachedEntries()
                onBackendHighlightRevisionChanged: refreshCachedEntries()
                onBackendSeparateFileExtensionsChanged: refreshCachedEntries()
                onBackendCursorChanged: {
                    if (!localCursorOverride) {
                        localCursor = backendCursor
                        ensureCursorVisible()
                    } else if (!listCursorCommitTimer.running
                               && backendCursor === localCursor) {
                        localCursorOverride = false
                    }
                }
                onRowsPerPageChanged: ensureCursorVisible()

                function syncGalleryScrollBar() {
                    var scrollBar = listScrollBar.item
                    if (!scrollBar || scrollBar.pressed)
                        return
                    scrollBar.size = contentHeight > 0
                            ? Math.min(1, height / contentHeight) : 1
                    scrollBar.position = contentHeight > 0
                            ? contentY / contentHeight : 0
                }

                delegate: Rectangle {
                    id: fileRow
                    width: ListView.view.width
                    height: list.rowHeight
                    readonly property var entry:
                        list.cachedEntries[index] || ({})
                    readonly property bool cursorState:
                        panel.active && index === list.localCursor
                    readonly property var highlightStyle: {
                        var styles = list.cachedHighlightStyles
                        var styleId = entry.highlightStyleId || ""
                        return styleId !== "" && styles[styleId]
                                ? styles[styleId] : ({})
                    }
                    readonly property var highlightPatch: {
                        if (cursorState && entry.selected)
                            return highlightStyle.selectedCursor || ({})
                        if (cursorState)
                            return highlightStyle.cursor || ({})
                        if (entry.selected)
                            return highlightStyle.selected || ({})
                        return highlightStyle.normal || ({})
                    }
                    readonly property string highlightForeground:
                        highlightPatch.foreground || ""
                    readonly property string highlightBackground:
                        highlightPatch.background || ""
                    readonly property string highlightMarker:
                        highlightStyle.marker || ""
                    readonly property url highlightIcon:
                        root.resolvedFileIconSource(
                            entry, highlightStyle.icon || "",
                            highlightMarker, 16)
                    readonly property var fileNameParts:
                        panelRoot.fileNameParts(entry)
                    color: {
                        var base
                        if (fileRow.cursorState)
                            base = root.panelSelectionBg
                        else if (entry.selected)
                            base = root.markedBg
                        else
                            base = "transparent"
                        return highlightBackground !== ""
                                ? highlightBackground : base
                    }
                    radius: 4
                    border.width: fileRow.cursorState ? 1 : 0
                    border.color: root.panelSelectionBorder

                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: root.panelRowInnerSpacing
                        anchors.rightMargin: root.panelRowInnerSpacing
                        spacing: 8

                        Item {
                            Layout.preferredWidth: 18
                            Layout.preferredHeight: 18

                            IconLabel {
                                anchors.centerIn: parent
                                visible: fileRow.highlightIcon.toString() !== ""
                                icon.source: fileRow.highlightIcon
                                icon.width: 16
                                icon.height: 16
                                icon.color: root.isFullColorFileIcon(
                                                fileRow.highlightIcon)
                                            ? "transparent"
                                            : fileRow.entry.isDir
                                            ? (fileRow.cursorState
                                               ? root.textColor
                                               : root.folderIconColor)
                                            : fileRow.highlightForeground !== ""
                                              ? fileRow.highlightForeground
                                              : root.mutedText
                            }

                            Text {
                                anchors.centerIn: parent
                                visible: fileRow.highlightIcon.toString() === ""
                                text: fileRow.highlightMarker !== ""
                                      ? fileRow.highlightMarker
                                      : fileRow.entry.isDir
                                        ? (fileRow.entry.isUp ? "↰" : "▸") : " "
                                color: fileRow.entry.isDir
                                       ? (fileRow.cursorState
                                          ? root.textColor
                                          : root.folderIconColor)
                                       : fileRow.highlightForeground !== ""
                                         ? fileRow.highlightForeground
                                         : root.mutedText
                                font.pixelSize: 13
                            }
                        }

                        Text {
                            text: fileRow.fileNameParts.base
                            color: fileRow.highlightForeground !== ""
                                   ? fileRow.highlightForeground
                                   : (fileRow.entry.isDir ? "#98d8ff"
                                                      : root.textColor)
                            elide: Text.ElideMiddle
                            font.pixelSize: 13
                            Layout.fillWidth: true
                        }

                        Text {
                            text: fileRow.fileNameParts.extension
                            visible: text.length > 0
                            color: fileRow.highlightForeground !== ""
                                   ? fileRow.highlightForeground
                                   : root.mutedText
                            elide: Text.ElideRight
                            horizontalAlignment: Text.AlignLeft
                            font.pixelSize: 12
                            Layout.preferredWidth: Math.min(80,
                                Math.max(40, implicitWidth))
                        }

                        Text {
                            text: root.cleanText(fileRow.entry.sizeText)
                            color: fileRow.highlightForeground !== ""
                                   ? fileRow.highlightForeground
                                   : root.mutedText
                            horizontalAlignment: Text.AlignRight
                            font.pixelSize: 12
                            Layout.preferredWidth: 96
                            visible: panel.viewModeName === "detailed"
                                     || panel.viewModeName === "wide"
                        }

                        Text {
                            text: root.cleanText(fileRow.entry.mtime)
                            color: fileRow.highlightForeground !== ""
                                   ? fileRow.highlightForeground
                                   : root.mutedText
                            font.pixelSize: 12
                            visible: panel.viewModeName === "wide"
                            Layout.preferredWidth: 120
                        }
                    }

                    MouseArea {
                        id: filePointer
                        anchors.fill: parent
                        acceptedButtons: Qt.AllButtons
                        property string pressedButton: ""
                        property int lastDragIndex: -1
                        onPressed: mouse => {
                            listCursorCommitTimer.stop()
                            list.localCursor = index
                            list.localCursorOverride = true
                            list.ensureCursorVisible()
                            pressedButton = panelRoot.pointerButtonName(mouse.button)
                            lastDragIndex = index
                            panelRoot.sendPointerEvent(fileRow.entry, "down",
                                                       pressedButton)
                            mouse.accepted = true
                        }
                        onPositionChanged: mouse => {
                            if (pressedButton === "" || mouse.buttons === Qt.NoButton)
                                return
                            var point = mapToItem(list.contentItem,
                                                  mouse.x, mouse.y)
                            var target = list.indexAt(point.x, point.y)
                            if (target === lastDragIndex || target < 0
                                    || target >= list.cachedEntries.length)
                                return
                            lastDragIndex = target
                            list.localCursor = target
                            list.localCursorOverride = true
                            list.ensureCursorVisible()
                            panelRoot.sendPointerEvent(list.cachedEntries[target],
                                                       "move", pressedButton)
                        }
                        onReleased: mouse => {
                            panelRoot.sendPointerEvent(fileRow.entry, "up",
                                                       pressedButton)
                            pressedButton = ""
                            lastDragIndex = -1
                        }
                        onCanceled: {
                            panelRoot.sendPointerEvent(fileRow.entry, "cancel",
                                                       pressedButton)
                            pressedButton = ""
                            lastDragIndex = -1
                        }
                        onClicked: mouse => panelRoot.sendPointerEvent(
                            fileRow.entry, "click", mouse.button)
                        onDoubleClicked: mouse => panelRoot.sendPointerEvent(
                            fileRow.entry, "doubleClick", mouse.button)
                    }
                }

                onCurrentIndexChanged: ensureCursorVisible()
                onContentYChanged: syncGalleryScrollBar()
                onContentHeightChanged: syncGalleryScrollBar()
                onHeightChanged: syncGalleryScrollBar()

                Loader {
                    id: listScrollBar
                    objectName: "filePanelScrollBar-" + Number(panel.side || 0)
                    // Flickable normally reparents declarative children into
                    // its scrolling contentItem. Keep this viewport chrome
                    // attached to the ListView itself.
                    parent: list
                    anchors.top: parent.top
                    anchors.bottom: parent.bottom
                    anchors.right: parent.right
                    width: active ? 16 : 0
                    z: 10
                    active: qtGallery.available
                            && list.contentHeight > list.height
                    visible: active
                    source: active ? qtGallery.scrollBarComponentUrl : ""

                    onLoaded: {
                        if (!item)
                            return
                        item.theme = root.galleryTheme()
                        item.orientation = Qt.Vertical
                        list.syncGalleryScrollBar()
                    }

                    Connections {
                        target: listScrollBar.item
                        function onPositionChanged() {
                            var scrollBar = listScrollBar.item
                            if (scrollBar && scrollBar.pressed)
                                list.contentY = scrollBar.position
                                                * list.contentHeight
                        }
                    }
                }
            }
        }

        Rectangle {
            id: status
            objectName: "panelStatus-" + Number(panel.side || 0)
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: Math.max(24, root.ch * 1.15)
                    + root.verticalContentSpacing
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
                anchors.left: parent.left
                anchors.verticalCenter: parent.verticalCenter
                anchors.leftMargin: root.panelTextInset
                text: panel.fastFind ? "/" + root.cleanText(panel.fastFindText) : root.cleanText(panel.selectedCount) + " selected"
                color: panel.fastFind ? root.activeBorder : root.mutedText
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

                    Rectangle { height: 1; color: root.panelBorder; Layout.fillWidth: true }
                    Text {
                        text: root.cleanText(modelData.label)
                        color: root.activeBorder
                        font.pixelSize: 12
                        font.bold: true
                    }
                    Rectangle { height: 1; color: root.panelBorder; Layout.fillWidth: true }
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

        function runBackground(value) {
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
        readonly property real contentWidth: 8 + (runs && runs.length > 0
                                                   ? runRow.implicitWidth
                                                   : fallbackRunLabel.implicitWidth)

        Row {
            id: runRow
            anchors.left: parent.left
            anchors.leftMargin: 8
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
                        font.pixelSize: 13
                        font.bold: modelData.bold === true
                        font.underline: modelData.underline === true
                        font.strikeout: modelData.strikeout === true
                        renderType: Text.NativeRendering
                    }
                }
            }
        }

        Text {
            id: fallbackRunLabel
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.leftMargin: 8
            anchors.rightMargin: 8
            anchors.verticalCenter: parent.verticalCenter
            visible: !runs || runs.length === 0
            text: fallbackText
            color: root.textColor
            font.family: root.guiMonospaceFontFamily
            font.pixelSize: root.guiMonospaceFontPixelSize
            renderType: Text.NativeRendering
            elide: Text.ElideRight
        }
    }

    component TerminalBackdrop: Rectangle {
        property var terminal: ({})
        color: root.terminalBg
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

        x: nativeLayout ? 0 : root.pxX(commandLine.x)
        y: nativeLayout ? root.height - root.keyBarHeight() - root.commandLineHeight(shell) : root.pxY(commandLine.y)
        width: nativeLayout ? root.width : root.pxW(commandLine.w)
        height: nativeLayout ? root.commandLineHeight(shell)
                             : Math.max(root.ch, root.pxH(commandLine.h))
        visible: commandLine.visible !== false
        color: root.commandLineBg

        ConsoleRunRow {
            anchors.fill: parent
            anchors.leftMargin: root.commandLineLeftMargin
            anchors.rightMargin: root.contentSpacing
            anchors.topMargin: root.commandLineVerticalMargin
                               + root.separatorWidth
            anchors.bottomMargin: root.commandLineVerticalMargin
                                  + root.separatorWidth
            transparentBlackBackground: true
            runs: commandLine.runs || []
            fallbackText: (root.runsText(commandLine.promptRuns)
                           || root.cleanText(commandLine.prompt))
                          + root.cleanText(commandLine.text)
        }

        FontMetrics {
            id: commandLineFontMetrics
            font.family: root.guiMonospaceFontFamily
            font.pixelSize: root.guiMonospaceFontPixelSize
        }

        // Measure the exact styled glyph sequence before the backend cursor.
        // Prompt runs may differ in weight and therefore cannot be mapped
        // reliably through a single average character width.
        ConsoleRunRow {
            id: commandCursorPrefix
            x: root.commandLineLeftMargin
            y: root.commandLineVerticalMargin + root.separatorWidth
            width: parent.width - root.commandLineLeftMargin
                    - root.contentSpacing
            height: parent.height - root.commandLineVerticalMargin * 2
                    - root.separatorWidth * 2
            transparentBlackBackground: true
            runs: commandLine.cursorPrefixRuns || []
            visible: false
        }

        Rectangle {
            id: commandCursor
            property bool blinkOn: true
            readonly property bool block: commandLine.cursorShape === "block"
            x: commandCursorPrefix.x + commandCursorPrefix.contentWidth
            y: block ? commandCursorPrefix.y
                     : commandCursorPrefix.y + commandCursorPrefix.height - 2
            width: Math.max(1, commandLineFontMetrics.advanceWidth("M"))
            height: block ? commandCursorPrefix.height : 2
            color: root.textColor
            visible: commandLine.cursorVisible === true
            opacity: blinkOn ? 0.8 : 0.2
            z: 2

            onVisibleChanged: {
                if (visible)
                    blinkOn = true
            }

            Timer {
                interval: 520
                running: commandCursor.visible
                repeat: true
                onTriggered: commandCursor.blinkOn = !commandCursor.blinkOn
            }
        }

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: root.separatorWidth
            color: root.separatorColor
        }

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: root.separatorWidth
            color: root.separatorColor
        }
    }

    component DocumentSurface: Rectangle {
        id: documentRoot
        property var frame: ({})
        readonly property real topInset: semanticMenu.visible
                                          ? semanticMenu.height : 0
        readonly property real rowHeight: Math.max(20, root.ch)

        function rowBackground(row) {
            var runs = row && row.runs ? row.runs : []
            for (var i = runs.length - 1; i >= 0; --i) {
                var background = root.cleanText(runs[i].background)
                if (background !== "")
                    return background
            }
            return root.terminalBg
        }

        color: frame.rows && frame.rows.length > 0
               ? rowBackground(frame.rows[0]) : root.terminalBg

        FontMetrics {
            id: documentFontMetrics
            font.family: root.guiMonospaceFontFamily
            font.pixelSize: 13
        }

        ListView {
            id: documentList
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.topMargin: documentRoot.topInset
            anchors.bottom: parent.bottom
            anchors.bottomMargin: root.scene.keyBar ? Math.max(26, root.ch * 1.35) : 0
            clip: true
            model: frame.rows || []
            interactive: true

            delegate: Rectangle {
                id: documentRow
                property var rowData: modelData
                width: ListView.view.width
                height: documentRoot.rowHeight
                color: documentRoot.rowBackground(rowData)

                Row {
                    id: runRow
                    anchors.left: parent.left
                    anchors.leftMargin: 10
                    height: parent.height
                    visible: documentRow.rowData.runs
                             && documentRow.rowData.runs.length > 0

                    Repeater {
                        model: documentRow.rowData.runs || []

                        delegate: Rectangle {
                            height: runRow.height
                            width: runLabel.implicitWidth
                            color: root.cleanText(modelData.background) !== ""
                                   ? modelData.background : "transparent"

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
                                renderType: Text.NativeRendering
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
                    visible: !documentRow.rowData.runs
                             || documentRow.rowData.runs.length === 0
                    text: root.rowText(documentRow.rowData)
                    color: root.textColor
                    font.family: root.guiMonospaceFontFamily
                    font.pixelSize: 13
                    elide: Text.ElideRight
                }
            }
        }

        Rectangle {
            id: editorCursor
            property bool blinkOn: true
            readonly property bool block: frame.cursorShape === "block"
            x: 10 + Math.max(0, Number(frame.cursorVisualColumn || 0))
                    * documentFontMetrics.advanceWidth("M")
            y: documentRoot.topInset
               + Math.max(0, Number(frame.cursorVisualRow || 0))
                 * documentRoot.rowHeight
               + (block ? 1 : documentRoot.rowHeight - 3)
            width: Math.max(1, documentFontMetrics.advanceWidth("M"))
            height: block ? documentRoot.rowHeight - 2 : 2
            color: root.dialogAccent
            opacity: blinkOn ? 0.72 : 0.18
            visible: frame.kind === "editor"
                     && frame.cursorVisible === true
                     && Number(frame.cursorVisualRow) >= 0
                     && Number(frame.cursorVisualColumn) >= 0
            z: 5

            onVisibleChanged: {
                if (visible)
                    blinkOn = true
            }

            Timer {
                interval: 520
                running: editorCursor.visible
                repeat: true
                onTriggered: editorCursor.blinkOn = !editorCursor.blinkOn
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

    component GenericDialog: Rectangle {
        id: dialogRoot
        property var frame: ({})
        property bool nativeLayout: root.isAppScene()
        readonly property real bodyContentHeight: calculateBodyContentHeight()

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

        width: nativeLayout
               ? Math.min(root.width - 48,
                          Math.max(420, root.pxW(frame.w || 60)))
               : Math.min(root.width - 24, root.pxW(frame.w))
        height: nativeLayout ? Math.min(root.height - 96, Math.max(180, root.pxH(frame.h))) : Math.min(root.height - 36, root.pxH(frame.h))
        x: nativeLayout ? Math.round((root.width - width) / 2) : Math.max(12, root.pxX(frame.x))
        y: nativeLayout ? Math.round((root.height - height) / 2) : Math.max(semanticMenu.height + 8, root.pxY(frame.y))
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
        }

        Text {
            anchors.left: parent.left
            anchors.right: closeButton.left
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

        DialogButton {
            id: closeButton
            anchors.verticalCenter: dialogHeader.verticalCenter
            anchors.right: parent.right
            anchors.rightMargin: 10
            width: 30
            height: 28
            text: "×"
            visible: frame.showClose === true
            background: Rectangle {
                radius: 4
                color: closeButton.down ? root.controlPressedBg
                       : closeButton.hovered ? root.controlHoverBg
                       : "transparent"
                border.width: 0
                Behavior on color { ColorAnimation { duration: 90 } }
            }
            onClicked: root.action({ "target": frame.id, "action": "dialog.close" })
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
            readonly property var commandLine: (root.shellFrame().commandLine || ({}))
            readonly property real commandLineX: root.isAppScene()
                                                     ? 0 : root.pxX(commandLine.x || 0)
            readonly property real commandLineY: root.isAppScene()
                                                     ? root.height - root.keyBarHeight()
                                                       - root.commandLineHeight(root.shellFrame())
                                                     : root.pxY(commandLine.y || 0)
            // CommandLine exports the authoritative Edit start column.  Its
            // prompt is monospaced, so translating that column with the same
            // Configured monospace metrics used by ConsoleRunRow land on the exact input x.
            readonly property real inputTextX: commandLineX + 8
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
                border.color: root.panelBorder
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
                                renderType: Text.NativeRendering
                            }

                            Text {
                                width: Math.max(0, completionTextRow.width
                                                - completionPrefixLabel.implicitWidth)
                                text: completionTextRow.fullText.substring(
                                          completionTextRow.matchingLength)
                                color: root.textColor
                                font.family: root.guiMonospaceFontFamily
                                font.pixelSize: 13
                                renderType: Text.NativeRendering
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
                : Number((root.scene.menuBar || {}).selected || 0)
            readonly property var previewMenuItem:
                fromMenuBar ? root.menuBarItem(effectiveMenuIndex) : null
            readonly property var effectiveItems:
                previewMenuItem && previewMenuItem.items
                ? previewMenuItem.items : (frame.items || [])
            readonly property bool previewIsAhead:
                fromMenuBar && previewMenuItem
                && effectiveMenuIndex
                   !== Number((root.scene.menuBar || {}).selected || 0)
            readonly property bool hasCheckIndicator: {
                for (var i = 0; i < effectiveItems.length; ++i) {
                    if (effectiveItems[i].checked === true)
                        return true
                }
                return false
            }
            readonly property real menuRowHeight: Math.max(27, root.ch * 1.02)
            readonly property real menuSeparatorHeight: 11
            property int pointerSelectedIndex: -1
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
                : Math.max(0, Number(frame.selected || 0))

            function reconcilePointerSelection() {
                if (!fromMenuBar || previewIsAhead
                        || root.menuPointerMenuIndex !== effectiveMenuIndex
                        || root.menuPointerItemIndex < 0
                        || root.menuPointerSentItemIndex
                           !== root.menuPointerItemIndex
                        || Number(frame.selected || 0)
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
                if (!fromMenuBar)
                    pointerSelectedIndex = -1
                else
                    Qt.callLater(reconcilePointerState)
            }
            Component.onCompleted: Qt.callLater(reconcilePointerState)

            FontMetrics {
                id: popupMenuMetrics
                font.pixelSize: 13
            }

            function preferredMenuWidth() {
                if (!fromMenuBar || !previewMenuItem)
                    return root.pxW(frame.w)
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
                              ? menuSeparatorHeight : menuRowHeight
                }
                if (root.cleanText(frame.bottomHint) !== "")
                    height += root.ch
                return height
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
                    "action": "menu.close"
                })
                onPressed: {
                    root.menuBarPreviewIndex = -1
                    root.clearMenuPointerSelection()
                }
                onWheel: (wheel) => { wheel.accepted = true }
            }

            Rectangle {
                id: popupSurface
                x: menuOverlay.previewMenuItem
                   ? semanticMenu.itemWindowX(menuOverlay.effectiveMenuIndex)
                   : root.pxX(menuOverlay.frame.x)
                y: menuOverlay.fromMenuBar
                   ? semanticMenu.windowBottom()
                   : root.pxY(menuOverlay.frame.y)
                width: menuOverlay.preferredMenuWidth()
                height: Math.min(root.height - y - root.keyBarHeight() - 4,
                                 Math.max(root.ch + 10,
                                          menuOverlay.preferredMenuHeight()))
                color: root.dialogHeaderBg
                border.width: 1
                border.color: "#46586b"
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
                    interactive: false

                    function syncTopPosition() {
                        if (count > 0)
                            positionViewAtIndex(Math.max(0, Number(menuOverlay.frame.top || 0)),
                                                ListView.Beginning)
                    }

                    Component.onCompleted: Qt.callLater(syncTopPosition)
                    onModelChanged: Qt.callLater(syncTopPosition)

                    delegate: Rectangle {
                        width: ListView.view.width
                        height: modelData.separator
                                ? menuOverlay.menuSeparatorHeight
                                : menuOverlay.menuRowHeight
                        radius: 4
                        color: modelData.index === menuOverlay.visualSelectedIndex
                               && !modelData.separator
                               ? "#2a5777" : "transparent"

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
                            anchors.left: parent.left
                            anchors.right: shortcut.left
                            anchors.verticalCenter: parent.verticalCenter
                            anchors.leftMargin: menuOverlay.hasCheckIndicator
                                                ? 32 : 10
                            text: {
                                var label = root.cleanText(modelData.text)
                                if (menuOverlay.hasCheckIndicator)
                                    label = label.replace(/^\s+/, "")
                                return root.mnemonicText(label,
                                                         modelData.hotkey)
                            }
                            textFormat: Text.StyledText
                            color: modelData.disabled ? root.mutedText : root.textColor
                            font.pixelSize: 13
                            visible: !modelData.separator
                            elide: Text.ElideRight
                        }

                        IconLabel {
                            anchors.left: parent.left
                            anchors.leftMargin: 10
                            anchors.verticalCenter: parent.verticalCenter
                            visible: !modelData.separator
                                     && modelData.checked === true
                            icon.source: root.resolvedIconSource("check", 15)
                            icon.width: 15
                            icon.height: 15
                            icon.color: modelData.disabled
                                        ? root.mutedText : root.textColor
                        }

                        Text {
                            id: shortcut
                            anchors.right: parent.right
                            anchors.verticalCenter: parent.verticalCenter
                            anchors.rightMargin: 10
                            text: root.cleanText(modelData.shortcut)
                            color: root.mutedText
                            font.pixelSize: 12
                            visible: !modelData.separator
                        }

                        MouseArea {
                            id: itemMouse
                            anchors.fill: parent
                            hoverEnabled: true
                            enabled: !modelData.separator && !modelData.disabled
                            onEntered: {
                                if (menuOverlay.fromMenuBar) {
                                    root.menuBarPointerHasSelectedItem = true
                                    if (root.menuPointerItemIndex < 0
                                            && !menuOverlay.previewIsAhead
                                            && Number(menuOverlay.frame.selected || 0)
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
                            onClicked: {
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
        property var keyBar: ({})
        color: "transparent"
        visible: keyBar.visible !== false && keyBar.items !== undefined

        Row {
            anchors.fill: parent
            anchors.leftMargin: root.contentSpacing
            anchors.rightMargin: root.contentSpacing
            anchors.topMargin: root.actionBarVerticalMargin
            anchors.bottomMargin: root.actionBarVerticalMargin
            Repeater {
                model: keyBar.items || []
                delegate: Rectangle {
                    id: actionButton
                    width: parent.width / 12
                    height: parent.height
                    color: actionButtonMouse.pressed
                           ? root.panelSelectionBorder
                           : actionButtonMouse.containsMouse
                             ? root.panelSelectionBg : "transparent"

                    Text {
                        anchors.left: parent.left
                        anchors.verticalCenter: parent.verticalCenter
                        anchors.leftMargin: root.actionButtonHorizontalMargin
                        text: root.cleanText(index + 1)
                        color: actionButtonMouse.containsMouse
                               ? root.textColor : root.dialogAccent
                        font.pixelSize: 11
                    }

                    Text {
                        anchors.left: parent.left
                        anchors.leftMargin: root.actionButtonHorizontalMargin
                                            + 20
                        anchors.right: parent.right
                        anchors.rightMargin: root.actionButtonHorizontalMargin
                        anchors.verticalCenter: parent.verticalCenter
                        text: root.mnemonicText(modelData.text,
                                                modelData.hotkey)
                        textFormat: Text.StyledText
                        color: root.chromeText
                        font.pixelSize: 11
                        elide: Text.ElideRight
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
                        cursorShape: Qt.PointingHandCursor
                        onClicked: {
                            var vk = 0x70 + index
                            qtShell.sendKey(vk, 0, true, 0)
                            qtShell.sendKey(vk, 0, false, 0)
                            grid.forceActiveFocus()
                        }
                    }
                }
            }
        }
    }

    component ToastView: Rectangle {
        property var toast: ({})
        width: Math.min(root.width - 32, toastText.implicitWidth + 28)
        height: toastText.implicitHeight + 14
        radius: 6
        color: "#2e343d"
        border.width: 1
        border.color: "#55616e"
        visible: toast.message !== undefined && root.cleanText(toast.message) !== ""

        Text {
            id: toastText
            anchors.centerIn: parent
            text: root.cleanText(toast.message)
            color: root.textColor
            font.pixelSize: 13
        }
    }
}
