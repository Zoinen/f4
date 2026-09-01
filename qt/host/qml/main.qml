pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import F4QtHost 1.0
import ZoinGallery 1.0 as ZG

F4HostWindow {
    id: root

    width: Math.max(720, Math.ceil(qtShell.initialCols * grid.cellWidth))
    height: Math.max(460, Math.ceil(qtShell.initialRows * grid.cellHeight))
    minimumWidth: 320
    minimumHeight: 240
    topPadding: 0
    leftPadding: 0
    rightPadding: 0
    bottomPadding: 0
    visible: false
    title: fallbackExplanation !== ""
           ? "f4 [Using text presentation: " + fallbackExplanation + "]"
           : "f4"

    titleBarItem: shellSurfaces.titleBarItem
    appIconItem: shellSurfaces.appIconButton
    workspaceBarItem: shellSurfaces.workspaceBarItem
    macSystemButtonAreaItem: shellSurfaces.macSystemButtonAreaItem
    galleryViewerLayer: shellSurfaces.galleryViewerLoader
    operationsQueueLayer: shellSurfaces.operationsQueueLoader
    focusTarget: grid
    sceneStoreApi: sceneStore
    interactionControllerApi: interactionController
    shellControllerApi: qtShell
    galleryControllerApi: qtGallery
    iconProvider: qtIcons
    themeEditor: themeColorConfigurator

    readonly property Item titleBar: shellSurfaces.titleBarItem
    readonly property Item semanticMenu: shellSurfaces.menuBar
    readonly property Item appIcon: shellSurfaces.appIconButton
    readonly property Item workspaceBar: shellSurfaces.workspaceBarItem
    readonly property Item macSystemButtonArea:
        shellSurfaces.macSystemButtonAreaItem
    readonly property Item overlayHost: shellSurfaces.overlayController

    property alias workspaceTabsOverride: sceneStore.workspaceTabsOverride
    property alias menuBarOverride: sceneStore.menuBarOverride
    property alias keyBarOverride: sceneStore.keyBarOverride
    property alias toastOverride: sceneStore.toastOverride
    property alias leftPanelPresentationOverride:
        sceneStore.leftPanelPresentationOverride
    property alias rightPanelPresentationOverride:
        sceneStore.rightPanelPresentationOverride
    property alias leftPanelLayoutStateOverride:
        sceneStore.leftPanelLayoutStateOverride
    property alias rightPanelLayoutStateOverride:
        sceneStore.rightPanelLayoutStateOverride
    readonly property alias workspaceTabs: sceneStore.workspaceTabs
    readonly property alias menuBarModel: sceneStore.menuBarModel
    readonly property alias keyBarModel: sceneStore.keyBarModel
    readonly property alias toastModel: sceneStore.toastModel
    readonly property alias workspaces: sceneStore.workspaces
    property alias retainedShellFrame: sceneStore.retainedShellFrame
    property alias retainedDocumentFrame: sceneStore.retainedDocumentFrame
    property alias retainedOperationsQueue: sceneStore.retainedOperationsQueue
    property alias shellPresentationOverrideSet:
        sceneStore.shellPresentationOverrideSet
    property alias shellPresentationOverride:
        sceneStore.shellPresentationOverride
    property alias documentPresentationOverrideSet:
        sceneStore.documentPresentationOverrideSet
    property alias documentPresentationOverride:
        sceneStore.documentPresentationOverride
    property alias documentSurfaceStateOverride:
        sceneStore.documentSurfaceStateOverride
    property alias retainedShellSurfaceCreated:
        sceneStore.retainedShellSurfaceCreated
    property alias retainedDocumentSurfaceCreated:
        sceneStore.retainedDocumentSurfaceCreated
    property alias documentSurfacePrewarmed:
        sceneStore.documentSurfacePrewarmed
    property alias retainedOperationsQueueCreated:
        sceneStore.retainedOperationsQueueCreated
    property alias panelActivationOverride: sceneStore.panelActivationOverride

    property alias leftGalleryPanelHost:
        interactionController.leftGalleryPanelHost
    property alias rightGalleryPanelHost:
        interactionController.rightGalleryPanelHost
    property alias pointerPanelActivationOverride:
        interactionController.pointerPanelActivationOverride
    property alias pointerPanelActivationTimeoutMs:
        interactionController.pointerPanelActivationTimeoutMs
    property alias menuBarPreviewIndex:
        interactionController.menuBarPreviewIndex
    property alias menuPointerMenuIndex:
        interactionController.menuPointerMenuIndex
    property alias menuPointerItemIndex:
        interactionController.menuPointerItemIndex
    property alias menuPointerSentItemIndex:
        interactionController.menuPointerSentItemIndex
    property alias menuPointerFrameId:
        interactionController.menuPointerFrameId
    property alias menuBarOpenedByPointer:
        interactionController.menuBarOpenedByPointer
    property alias menuBarPointerHasSelectedItem:
        interactionController.menuBarPointerHasSelectedItem
    property alias autocompleteMenuId:
        interactionController.autocompleteMenuId
    property alias autocompleteQuery:
        interactionController.autocompleteQuery
    property alias autocompleteItemsSignature:
        interactionController.autocompleteItemsSignature
    property alias autocompleteSelectedIndex:
        interactionController.autocompleteSelectedIndex

    ShellShortcuts {
        hostWindow: root
        galleryController: qtGallery
    }

    ShellSceneStore {
        id: sceneStore
        hostWindow: root
        shellController: qtShell
    }

    ShellInteractionController {
        id: interactionController
        hostWindow: root
        focusTarget: grid
        sceneStore: sceneStore
        shellController: qtShell
        galleryController: qtGallery
        overlayHost: shellSurfaces.overlayController
        galleryViewerLayer: shellSurfaces.galleryViewerLoader
        operationsQueueLayer: shellSurfaces.operationsQueueLoader
    }

    VtuiGridItem {
        id: grid
        objectName: "vtuiGrid"
        anchors.fill: parent
        anchors.topMargin: root.needsFallbackGrid()
                           && root.useMacNativeTitleBar
                           ? root.menuBarHeight : 0
        controller: qtShell
        fontFamily: f4GuiFontFamily
        fontPixelSize: root.guiMonospaceFontPixelSize
        focus: true
        pointerInputEnabled: root.needsFallbackGrid()
        inputMethodForwardingEnabled: root.galleryInputRoutingActive()
        terminalInputEnabled: !root.galleryViewerOwnsKeyboard()
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

    ThemeEditor {
        id: themeColorConfigurator
        hostWindow: root
        themePersistence: typeof qtTheme !== "undefined" ? qtTheme : null
        transientParent: root
    }

    ShellSurfaceHost {
        id: shellSurfaces
        anchors.fill: parent
        z: 10
        visible: !root.needsFallbackGrid()
        hostWindow: root
        nativeWindowAgent: root.nativeWindowAgent
        nativeWindowAgentReady: root.windowAgentReady
        focusTarget: grid
        shellController: qtShell
        galleryController: qtGallery
        themeEditor: themeColorConfigurator
        galleryTheme: root.galleryThemePalette
        galleryMetrics: root.galleryPresentationMetrics
        usesQwk: f4UsesQwk
    }
}
