pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import F4QtHost 1.0
import ZoinGallery 1.0 as ZG
import QWindowKit 1.0

ApplicationWindow {
    id: host

    property Item titleBarItem: null
    property Item appIconItem: null
    property Item workspaceBarItem: null
    property Item macSystemButtonAreaItem: null
    property Item galleryViewerLayer: null
    property Item operationsQueueLayer: null
    property Item focusTarget: null
    property var sceneStoreApi: null
    property var interactionControllerApi: null
    property var shellControllerApi: null
    property var galleryControllerApi: null
    property var iconProvider: null
    property Window themeEditor: null

    readonly property alias nativeWindowAgent: windowAgent
    readonly property alias themePaletteObject: themePalette

    property bool isQWKLegacy: false
    property bool windowAgentReady: false
    property bool workspaceBarHitTestRegistered: false
    property int macWindowEffectApplyAttempts: 0
    property int keyboardActivityRevision: 0

    readonly property string guiMonospaceFontFamily:
        String(f4GuiFontFamily || "").length > 0
        ? String(f4GuiFontFamily) : "Monaco"
    readonly property int guiMonospaceFontPixelSize:
        Number(f4GuiFontPixelSize) > 0 ? Number(f4GuiFontPixelSize)
                                       : (Qt.platform.os === "osx" ? 17 : 16)

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

    property alias windowBackgroundColor: themePalette.windowBackgroundColor
    readonly property real dpr: screen ? screen.devicePixelRatio : 1.0
    readonly property int fontRenderType:
        typeof qtTextRendering !== "undefined" && qtTextRendering
        ? qtTextRendering.renderType : Text.NativeRendering
    readonly property var fontRenderTypeOptions:
        typeof qtTextRendering !== "undefined" && qtTextRendering
        ? qtTextRendering.options : [
            { value: Text.QtRendering, name: "QtRendering",
              description: "Qt distance-field rendering" },
            { value: Text.NativeRendering, name: "NativeRendering",
              description: "Native platform text rasterization" }
        ]
    readonly property string fontRenderTypeName:
        typeof qtTextRendering !== "undefined" && qtTextRendering
        ? qtTextRendering.renderTypeName
        : String(fontRenderTypeOption(fontRenderType).name || "NativeRendering")
    readonly property string fontRenderTypeDescription:
        String(fontRenderTypeOption(fontRenderType).description || "")

    property string mouseWheelMode: "gui"
    readonly property var mouseWheelModeOptions: [
        { value: "console", name: "F4 console",
          description: "Send wheel and middle-button events to the F4 panel" },
        { value: "gui", name: "GUI scrolling",
          description: "Smoothly scroll the panel and use middle-button auto-scroll" }
    ]
    readonly property string mouseWheelModeName:
        String(mouseWheelModeOption(mouseWheelMode).name || "GUI scrolling")
    readonly property string mouseWheelModeDescription:
        String(mouseWheelModeOption(mouseWheelMode).description || "")

    readonly property real physicalSeparatorPixels:
        Math.max(1, Math.round(dpr))
    readonly property real separatorWidth: physicalSeparatorPixels / dpr
    property real cw: focusTarget ? Math.max(8, focusTarget.cellWidth) : 8
    property real ch: focusTarget ? Math.max(17, focusTarget.cellHeight) : 17
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
    readonly property real commandLineLeftMargin: panelTextInset
    readonly property real commandLineVerticalMargin: 8
    readonly property real semanticTextFontPixelSize: 13
    readonly property real actionBarVerticalMargin: 3
    readonly property real actionSeparatorVerticalMargin: 5
    readonly property real actionButtonHorizontalMargin: 8
    readonly property real menuItemHorizontalPadding: 14

    property alias panelPathBg: themePalette.panelPathBg
    property alias commandLineBg: themePalette.commandLineBg
    property alias activeBorder: themePalette.activeBorder
    property alias textColor: themePalette.textColor
    property alias mutedText: themePalette.mutedText
    property alias selectedBg: themePalette.selectedBg
    property alias panelSelectionBg: themePalette.panelSelectionBg
    property alias panelSelectionBorder: themePalette.panelSelectionBorder
    property alias titleBarBg: themePalette.titleBarBg
    property alias fBarBg: themePalette.fBarBg
    property alias chromeText: themePalette.chromeText
    property alias dialogBg: themePalette.dialogBg
    property alias dialogHeaderBg: themePalette.dialogHeaderBg
    property alias controlBg: themePalette.controlBg
    property alias controlHoverBg: themePalette.controlHoverBg
    property alias controlPressedBg: themePalette.controlPressedBg
    property alias controlBorder: themePalette.controlBorder
    property alias separatorColor: themePalette.separatorColor
    property alias separatorHoverColor: themePalette.separatorHoverColor
    property alias separatorActiveColor: themePalette.separatorActiveColor
    property alias dialogAccent: themePalette.dialogAccent
    property alias galleryPanelBackgroundColor:
        themePalette.galleryPanelBackgroundColor
    property alias galleryViewerBackgroundColor:
        themePalette.galleryViewerBackgroundColor
    property alias galleryTextColor: themePalette.galleryTextColor
    property alias galleryMutedTextColor: themePalette.galleryMutedTextColor
    property alias galleryFileTextColor: themePalette.galleryFileTextColor
    property alias galleryFolderTextColor: themePalette.galleryFolderTextColor
    property alias galleryNeutralFileTextColors:
        themePalette.galleryNeutralFileTextColors
    property alias galleryQuickSearchMatchColor:
        themePalette.galleryQuickSearchMatchColor
    property alias galleryDirectoryTextColor:
        themePalette.galleryDirectoryTextColor
    property alias galleryFolderIconColor: themePalette.galleryFolderIconColor
    property alias galleryCursorColor: themePalette.galleryCursorColor
    property alias galleryCursorBackgroundColor:
        themePalette.galleryCursorBackgroundColor
    property alias galleryCursorBorderColor:
        themePalette.galleryCursorBorderColor
    property alias galleryCardCursorBorderColor:
        themePalette.galleryCardCursorBorderColor
    property alias gallerySelectionColor: themePalette.gallerySelectionColor
    property alias galleryMarkedBackgroundColor:
        themePalette.galleryMarkedBackgroundColor
    property alias galleryMarkedTextColor: themePalette.galleryMarkedTextColor
    property alias galleryItemBackgroundColor:
        themePalette.galleryItemBackgroundColor
    property alias galleryDirectoryBackgroundColor:
        themePalette.galleryDirectoryBackgroundColor
    property alias galleryItemHoverColor: themePalette.galleryItemHoverColor
    property alias galleryLabelBackgroundColor:
        themePalette.galleryLabelBackgroundColor
    property alias galleryPreviewBackdropColor:
        themePalette.galleryPreviewBackdropColor
    property alias gallerySeparatorColor: themePalette.gallerySeparatorColor
    property alias galleryHeaderTextColor: themePalette.galleryHeaderTextColor
    property alias galleryControlHoverColor:
        themePalette.galleryControlHoverColor
    property alias galleryScrollBarHandleColor:
        themePalette.galleryScrollBarHandleColor
    property alias galleryScrollBarBackgroundHoverColor:
        themePalette.galleryScrollBarBackgroundHoverColor
    property alias galleryScrollBarHoverColor:
        themePalette.galleryScrollBarHoverColor
    property alias galleryScrollBarPressedColor:
        themePalette.galleryScrollBarPressedColor
    property alias galleryScrollBarTrackHoverColor:
        themePalette.galleryScrollBarTrackHoverColor
    property alias galleryPathBackgroundColor:
        themePalette.galleryPathBackgroundColor
    property alias galleryPathTextColor: themePalette.galleryPathTextColor
    property alias galleryPathHoverColor: themePalette.galleryPathHoverColor
    property alias galleryPathItemHoverColor:
        themePalette.galleryPathItemHoverColor
    property alias galleryPathItemPressedColor:
        themePalette.galleryPathItemPressedColor

    readonly property real iconDevicePixelRatio:
        screen ? screen.devicePixelRatio : 1.0
    readonly property string fallbackExplanation:
        sceneStoreApi ? sceneStoreApi.semanticFallbackReason() : ""
    property real panelSplitRatio: semanticPanelSplitRatio()
    readonly property real panelMinimumWidth:
        Math.max(180, Math.min(280, cw * 22))
    readonly property bool nativeTwoPanelSurfaceActive:
        isAppScene() && !needsFallbackGrid() && !hasBlockingOverlay()
        && !hasDocumentSurface() && !hasOperationsQueueSurface()
        && galleryControllerApi && !galleryControllerApi.viewerVisible
        && !terminalActive()
    readonly property bool nativeTwoPanelSurfaceVisible:
        isAppScene() && !needsFallbackGrid() && !hasDocumentSurface()
        && !hasOperationsQueueSurface() && galleryControllerApi
        && !galleryControllerApi.viewerVisible
        && !terminalActive()
    readonly property real galleryViewerProgress: {
        const surfaceLoader = galleryViewerLayer ? galleryViewerLayer.item : null
        const viewerHost = surfaceLoader && surfaceLoader.item
                ? surfaceLoader.item : null
        return viewerHost
                ? Math.max(0, Math.min(1,
                           Number(viewerHost.surfaceProgress || 0))) : 0
    }
    readonly property real normalSurfaceOpacity:
        hasDocumentSurface() || hasOperationsQueueSurface()
        ? 1 : 1 - galleryViewerProgress

    readonly property ZG.GalleryThemePalette galleryThemePalette:
        themePalette.galleryTheme
    readonly property ZG.GalleryPresentationMetrics galleryPresentationMetrics:
        themePalette.galleryMetrics
    readonly property int themeSchemaVersion: themePalette.schemaVersion
    readonly property var themeColorDefinitions: themePalette.colorDefinitions

    color: useTransparentWindowBackground ? "transparent"
                                          : windowBackgroundColor

    function fontRenderTypeOption(value) {
        const options = fontRenderTypeOptions || []
        for (let index = 0; index < options.length; ++index) {
            if (Number(options[index].value) === Number(value))
                return options[index]
        }
        return options.length > 0 ? options[0] : ({})
    }

    function mouseWheelModeOption(value) {
        const options = mouseWheelModeOptions || []
        for (let index = 0; index < options.length; ++index) {
            if (String(options[index].value) === String(value))
                return options[index]
        }
        return options.length > 0 ? options[options.length - 1] : ({})
    }

    function setMouseWheelMode(value) {
        const normalized = String(value || "").toLowerCase()
        for (let index = 0; index < mouseWheelModeOptions.length; ++index) {
            if (String(mouseWheelModeOptions[index].value) === normalized) {
                mouseWheelMode = normalized
                return true
            }
        }
        return false
    }

    function showApplicationSettings() {
        if (!themeEditor)
            return
        themeEditor.show()
        themeEditor.requestActivate()
        themeEditor.raise()
    }

    function snapPx(value) {
        return presentationUtilities.snapPx(value)
    }
    function dialogPixelOffsetX(item, target) {
        return presentationUtilities.dialogPixelOffsetX(item, target)
    }
    function dialogPixelOffsetY(item, target) {
        return presentationUtilities.dialogPixelOffsetY(item, target)
    }
    function iconPixelOffsetX(item) {
        return presentationUtilities.iconPixelOffsetX(item)
    }
    function iconPixelOffsetY(item) {
        return presentationUtilities.iconPixelOffsetY(item)
    }

    function mergePanelLayoutState(currentState, delta) {
        return sceneStoreApi.mergePanelLayoutState(currentState, delta)
    }
    function commandLineFrame() { return sceneStoreApi.commandLineFrame() }
    function isAppScene() { return sceneStoreApi.isAppScene() }
    function currentOperationsQueue() {
        return sceneStoreApi.currentOperationsQueue()
    }
    function operationsQueueFrame() { return sceneStoreApi.operationsQueueFrame() }
    function hasOperationsQueueSurface() {
        return sceneStoreApi && sceneStoreApi.hasOperationsQueueSurface()
    }
    function currentShellFrame() { return sceneStoreApi.currentShellFrame() }
    function currentDocumentFrame() { return sceneStoreApi.currentDocumentFrame() }
    function captureRetainedSurfaces() {
        if (sceneStoreApi)
            sceneStoreApi.captureRetainedSurfaces()
    }
    function workspaceTabCanClose(tab) {
        return sceneStoreApi.workspaceTabCanClose(tab)
    }
    function frames() { return sceneStoreApi.frames() }
    function overlayFrames() { return sceneStoreApi.overlayFrames() }
    function hasBlockingOverlay() {
        return sceneStoreApi && sceneStoreApi.hasBlockingOverlay()
    }
    function shellFrame() { return sceneStoreApi ? sceneStoreApi.shellFrame() : ({}) }
    function quickViewForSide(side) { return sceneStoreApi.quickViewForSide(side) }
    function infoPanelForSide(side) { return sceneStoreApi.infoPanelForSide(side) }
    function panelSideCovered(side) { return sceneStoreApi.panelSideCovered(side) }
    function activeSurface() { return sceneStoreApi.activeSurface() }
    function isDocumentSurface(frame) { return sceneStoreApi.isDocumentSurface(frame) }
    function hasDocumentSurface() {
        return sceneStoreApi && sceneStoreApi.hasDocumentSurface()
    }
    function terminalActive() {
        return sceneStoreApi && sceneStoreApi.terminalActive()
    }
    function hasStandaloneDocumentSurface() {
        return sceneStoreApi && sceneStoreApi.hasStandaloneDocumentSurface()
    }
    function firstFrame(kind) { return sceneStoreApi.firstFrame(kind) }
    function topFrame() { return sceneStoreApi.topFrame() }
    function needsFallbackGrid() {
        return sceneStoreApi ? sceneStoreApi.needsFallbackGrid() : true
    }
    function fallbackReasonForNode(node) {
        return sceneStoreApi.fallbackReasonForNode(node)
    }
    function semanticFallbackReason() {
        return sceneStoreApi ? sceneStoreApi.semanticFallbackReason() : ""
    }
    function containsFallback(node) { return sceneStoreApi.containsFallback(node) }

    function activeAutocompleteFrame() {
        return interactionControllerApi.activeAutocompleteFrame()
    }
    function autocompleteSignature(frame) {
        return interactionControllerApi.autocompleteSignature(frame)
    }
    function syncAutocompleteSelection() {
        interactionControllerApi.syncAutocompleteSelection()
    }
    function navigateAutocomplete(delta) {
        interactionControllerApi.navigateAutocomplete(delta)
    }
    function autocompleteSelectedText() {
        return interactionControllerApi.autocompleteSelectedText()
    }
    function submitAutocomplete() { interactionControllerApi.submitAutocomplete() }
    function completeAutocomplete() { interactionControllerApi.completeAutocomplete() }
    function clearMenuPointerSelection() {
        interactionControllerApi.clearMenuPointerSelection()
    }
    function menuBarItem(index) { return interactionControllerApi.menuBarItem(index) }
    function setGalleryPanelHost(side, panelHost) {
        interactionControllerApi.setGalleryPanelHost(side, panelHost)
    }
    function clearGalleryPanelHost(side, panelHost) {
        interactionControllerApi.clearGalleryPanelHost(side, panelHost)
    }
    function galleryPanelHost(side) {
        return interactionControllerApi.galleryPanelHost(side)
    }
    function activeOperationsQueueView() {
        return interactionControllerApi.activeOperationsQueueView()
    }
    function navigateOperationsQueue(command) {
        return interactionControllerApi.navigateOperationsQueue(command)
    }
    function activateOperationsQueueSelection() {
        return interactionControllerApi.activateOperationsQueueSelection()
    }
    function operationsQueueShortcutCanActivate() {
        return interactionControllerApi.operationsQueueShortcutCanActivate()
    }
    function menuOverlayForId(menuId) {
        return interactionControllerApi.menuOverlayForId(menuId)
    }
    function createDialogOverlay(frame) {
        return interactionControllerApi.createDialogOverlay(frame)
    }
    function scheduleMenuPointerSync() {
        interactionControllerApi.scheduleMenuPointerSync()
    }
    function activePanelHasGalleryHost() {
        return interactionControllerApi.activePanelHasGalleryHost()
    }
    function effectiveActivePanelSide() {
        return interactionControllerApi.effectiveActivePanelSide()
    }
    function panelIsEffectivelyActive(panel) {
        return interactionControllerApi.panelIsEffectivelyActive(panel)
    }
    function beginPointerPanelActivation(side) {
        interactionControllerApi.beginPointerPanelActivation(side)
    }
    function finishPointerPanelActivation() {
        interactionControllerApi.finishPointerPanelActivation()
    }
    function galleryInputRoutingActive() {
        return interactionControllerApi.galleryInputRoutingActive()
    }
    function galleryViewerOwnsKeyboard() {
        return interactionControllerApi.galleryViewerOwnsKeyboard()
    }
    function restoreSurfaceFocus() {
        interactionControllerApi.restoreSurfaceFocus()
    }

    function keyBarHeight() {
        return sceneStoreApi && sceneStoreApi.keyBarModel.visible !== false
                && Object.keys(sceneStoreApi.keyBarModel).length > 0
                ? ch + commandLineVerticalMargin * 2 + separatorWidth * 2 : 0
    }
    function commandLineHeight(shell) {
        const commandLine = commandLineFrame()
        return commandLine && commandLine.visible !== false
                ? ch + commandLineVerticalMargin * 2 + separatorWidth * 2 : 0
    }

    function pxX(value) { return presentationUtilities.pxX(value) }
    function pxY(value) { return presentationUtilities.pxY(value) }
    function pxW(value) { return presentationUtilities.pxW(value) }
    function pxH(value) { return presentationUtilities.pxH(value) }
    function nativePanelSplitPosition() {
        return presentationUtilities.nativePanelSplitPosition()
    }
    function widePanelSide() { return presentationUtilities.widePanelSide() }
    function panelSideVisible(side) {
        return presentationUtilities.panelSideVisible(side)
    }
    function nativePanelX(side) { return presentationUtilities.nativePanelX(side) }
    function nativePanelWidth(side) {
        return presentationUtilities.nativePanelWidth(side)
    }
    function cleanText(value) { return presentationUtilities.cleanText(value) }
    function repeaterMaxImplicitWidth(repeater) {
        return presentationUtilities.repeaterMaxImplicitWidth(repeater)
    }
    function resolvedIconSource(name, logicalSize) {
        return presentationUtilities.resolvedIconSource(name, logicalSize)
    }
    function fileIconSource(localPath, fileName, directory, logicalSize,
                            version) {
        return presentationUtilities.fileIconSource(
                    localPath, fileName, directory, logicalSize, version)
    }
    function lucideIconSource(name, size, tint) {
        return presentationUtilities.lucideIconSource(name, size, tint)
    }
    function semanticMenuIconSource(name, size, tint) {
        return presentationUtilities.semanticMenuIconSource(name, size, tint)
    }
    function semanticPanelSplitRatio() {
        return presentationUtilities.semanticPanelSplitRatio()
    }
    function workspaceTabIconName(tab) {
        return presentationUtilities.workspaceTabIconName(tab)
    }
    function workspaceTabLabel(tab) {
        return presentationUtilities.workspaceTabLabel(tab)
    }
    function workspaceTabTextColor(active) {
        return presentationUtilities.workspaceTabTextColor(active)
    }
    function workspaceTabNumberColor() {
        return presentationUtilities.workspaceTabNumberColor()
    }
    function workspaceTabShortcut(tab, platformName) {
        return presentationUtilities.workspaceTabShortcut(tab, platformName)
    }
    function workspaceTabToolTip(tab, platformName) {
        return presentationUtilities.workspaceTabToolTip(tab, platformName)
    }
    function workspaceTabFontWeight() {
        return presentationUtilities.workspaceTabFontWeight()
    }
    function vtuiKeyModifiers(modifiers) {
        return presentationUtilities.vtuiKeyModifiers(modifiers)
    }
    function keyBarFunctionIndex(item, fallbackIndex) {
        return presentationUtilities.keyBarFunctionIndex(item, fallbackIndex)
    }
    function keyBarModifierShortcut(functionKey, modifier) {
        return presentationUtilities.keyBarModifierShortcut(functionKey,
                                                             modifier)
    }
    function keyBarModifierFlags(modifier) {
        return presentationUtilities.keyBarModifierFlags(modifier)
    }
    function preferredWorkspaceTabWidth(titleWidth, closeEnabled) {
        return presentationUtilities.preferredWorkspaceTabWidth(
                    titleWidth, closeEnabled)
    }
    function richTextEscape(value) {
        return presentationUtilities.richTextEscape(value)
    }
    function mnemonicText(value, hotkey) {
        return presentationUtilities.mnemonicText(value, hotkey)
    }
    function runsText(runs) { return presentationUtilities.runsText(runs) }
    function rowText(row) { return presentationUtilities.rowText(row) }

    function action(intent, preserveFocus) {
        interactionControllerApi.action(intent, preserveFocus)
    }

    function setFontRenderType(value) {
        return themePalette.setFontRenderType(value)
    }
    function loadThemeFromPersistence() {
        return themePalette.loadFromPersistence()
    }
    function saveThemeToPersistence() {
        return themePalette.saveToPersistence()
    }
    function resetThemeToDefaults() { themePalette.resetToDefaults() }
    function formatColorHex(colorValue) {
        return themePalette.formatColorHex(colorValue)
    }
    function toggleFullscreen() {
        if (visibility === Window.FullScreen)
            showNormal()
        else
            showFullScreen()
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
            ++macWindowEffectApplyAttempts
            macWindowEffectRetryTimer.restart()
        }
    }

    Component.onCompleted: {
        ZG.Style.isDarkTheme = true
        loadThemeFromPersistence()
        captureRetainedSurfaces()
        if (!f4UsesQwk)
            return
        windowAgent.setup(host)
        windowAgentReady = true
        if (Qt.platform.os === "windows") {
            isQWKLegacy = supportsTransparentWindowBackground
                    ? windowAgent.setWindowAttribute("mica-alt", true) !== true
                    : true
        } else if (useMacNativeTitleBar) {
            if (supportsTransparentWindowBackground)
                applyPlatformWindowEffects()
            else
                isQWKLegacy = true
        }
        if (titleBarItem)
            windowAgent.setTitleBar(titleBarItem)
        if (Qt.platform.os !== "osx" && appIconItem)
            windowAgent.setHitTestVisible(appIconItem)
        if (workspaceBarItem) {
            windowAgent.setHitTestVisible(workspaceBarItem)
            workspaceBarHitTestRegistered = true
        }
        if (useMacNativeTitleBar && macSystemButtonAreaItem)
            windowAgent.setSystemButtonArea(macSystemButtonAreaItem)
    }

    onClosing: {
        if (shellControllerApi)
            shellControllerApi.sendQuit()
    }

    WindowAgent {
        id: windowAgent
    }

    Timer {
        id: macWindowEffectRetryTimer
        interval: 100
        repeat: false
        onTriggered: host.applyPlatformWindowEffects()
    }

    HostPresentationUtilities {
        id: presentationUtilities
        hostWindow: host
        iconProvider: host.iconProvider
    }

    HostThemePalette {
        id: themePalette
        hostWindow: host
        persistence: typeof qtTheme !== "undefined" ? qtTheme : null
        textRendering:
            typeof qtTextRendering !== "undefined" ? qtTextRendering : null
    }
}
