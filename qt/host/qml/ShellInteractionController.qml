pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: controller

    required property ApplicationWindow hostWindow
    required property Item focusTarget
    required property var sceneStore
    required property var shellController
    required property var galleryController

    property var overlayHost: null
    property var galleryViewerLayer: null
    property var operationsQueueLayer: null

    property var leftGalleryPanelHost: null
    property var rightGalleryPanelHost: null

    property int pointerPanelActivationOverride: -1
    property int pointerPanelActivationTimeoutMs: 6000

    property int menuBarPreviewIndex: -1
    property int menuPointerMenuIndex: -1
    property int menuPointerItemIndex: -1
    property int menuPointerSentItemIndex: -1
    property string menuPointerFrameId: ""
    property bool menuBarOpenedByPointer: false
    property bool menuBarPointerHasSelectedItem: false

    property string autocompleteMenuId: ""
    property string autocompleteQuery: ""
    property string autocompleteItemsSignature: ""
    property int autocompleteSelectedIndex: -1

    visible: false
    width: 0
    height: 0

    function cleanText(value) {
        return value === undefined || value === null ? "" : String(value)
    }

    function action(intent, preserveFocus) {
        shellController.sendUiAction(intent)
        if (preserveFocus !== true)
            focusTarget.forceActiveFocus()
    }

    function activeAutocompleteFrame() {
        const menus = sceneStore.overlayState.commandMenus || []
        for (let index = menus.length - 1; index >= 0; --index) {
            if (menus[index].role === "autocomplete")
                return menus[index]
        }
        return null
    }

    function autocompleteSignature(frame) {
        const items = frame && frame.items ? frame.items : []
        const parts = []
        for (let index = 0; index < items.length; ++index)
            parts.push(cleanText(items[index].text))
        return parts.join("\u001f")
    }

    function syncAutocompleteSelection() {
        const frame = activeAutocompleteFrame()
        if (!frame) {
            autocompleteMenuId = ""
            autocompleteQuery = ""
            autocompleteItemsSignature = ""
            autocompleteSelectedIndex = -1
            return
        }
        const id = cleanText(frame.id)
        const query = cleanText(frame.query)
        const signature = autocompleteSignature(frame)
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
        const frame = activeAutocompleteFrame()
        const count = frame && frame.items ? frame.items.length : 0
        if (count < 1)
            return
        if (autocompleteSelectedIndex < 0) {
            autocompleteSelectedIndex = delta < 0 ? count - 1 : 0
            return
        }
        autocompleteSelectedIndex = (autocompleteSelectedIndex + delta
                                     + count) % count
    }

    function autocompleteSelectedText() {
        const frame = activeAutocompleteFrame()
        const items = frame && frame.items ? frame.items : []
        const index = autocompleteSelectedIndex
        return index >= 0 && index < items.length
                ? cleanText(items[index].text) : ""
    }

    function submitAutocomplete() {
        submitAutocompleteAction("command.submit")
    }

    function completeAutocomplete() {
        submitAutocompleteAction("command.complete")
    }

    function submitAutocompleteAction(actionName) {
        const frame = activeAutocompleteFrame()
        if (!frame)
            return
        const shell = sceneStore.shellFrame()
        const intent = {
            "target": cleanText(shell.id) !== "" ? shell.id : frame.id,
            "action": actionName
        }
        if (autocompleteSelectedIndex >= 0)
            intent.text = autocompleteSelectedText()
        action(intent, true)
    }

    function clearMenuPointerSelection() {
        menuPointerSyncTimer.stop()
        menuPointerMenuIndex = -1
        menuPointerItemIndex = -1
        menuPointerSentItemIndex = -1
        menuPointerFrameId = ""
    }

    function menuBarItem(index) {
        const items = sceneStore.menuBarModel.items || []
        for (let itemIndex = 0; itemIndex < items.length; ++itemIndex) {
            if (Number(items[itemIndex].index) === Number(index))
                return items[itemIndex]
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

    function activeOperationsQueueView() {
        if (!sceneStore.hasOperationsQueueSurface()
                || sceneStore.hasBlockingOverlay())
            return null
        return operationsQueueLayer ? operationsQueueLayer.item : null
    }

    function navigateOperationsQueue(command) {
        const view = activeOperationsQueueView()
        return view ? view.navigate(command) : false
    }

    function activateOperationsQueueSelection() {
        const view = activeOperationsQueueView()
        return view ? view.activateSelection() : false
    }

    function operationsQueueShortcutCanActivate() {
        const view = activeOperationsQueueView()
        return view !== null && !view.controlOwnsActivation()
    }

    function menuOverlayForId(menuId) {
        return overlayHost ? overlayHost.menuOverlayForId(menuId) : null
    }

    function createDialogOverlay(frame) {
        return overlayHost ? overlayHost.createDialogOverlay(frame) : null
    }

    function scheduleMenuPointerSync() {
        menuPointerSyncTimer.restart()
    }

    function activePanelHasGalleryHost() {
        if (!galleryController.available)
            return false
        const shell = sceneStore.shellFrame()
        const panels = shell && shell.panels ? shell.panels : []
        const activeSide = effectiveActivePanelSide()
        for (let index = 0; index < panels.length; ++index) {
            const side = Number(panels[index].side)
            if (side === activeSide && hostWindow.panelSideVisible(side)
                    && !sceneStore.panelSideCovered(side)) {
                return galleryPanelHost(side) !== null
            }
        }
        return false
    }

    function effectiveActivePanelSide() {
        if (pointerPanelActivationOverride === 0
                || pointerPanelActivationOverride === 1)
            return pointerPanelActivationOverride
        if (sceneStore.panelActivationOverride === 0
                || sceneStore.panelActivationOverride === 1)
            return sceneStore.panelActivationOverride
        const typedShell = sceneStore.typedShellState
        if (typedShell) {
            const typedSide = Number(typedShell.activePanel)
            if (typedSide === 0 || typedSide === 1)
                return typedSide
        }
        const shell = sceneStore.shellFrame()
        const shellSide = Number(shell && shell.activePanel)
        if (shellSide === 0 || shellSide === 1)
            return shellSide
        const panels = shell && shell.panels ? shell.panels : []
        for (let index = 0; index < panels.length; ++index) {
            if (panels[index].active === true)
                return Number(panels[index].side)
        }
        return -1
    }

    function panelIsEffectivelyActive(panel) {
        const activeSide = effectiveActivePanelSide()
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

    function galleryInputRoutingActive() {
        if (sceneStore.hasBlockingOverlay() || sceneStore.needsFallbackGrid()
                || sceneStore.hasDocumentSurface()
                || sceneStore.hasOperationsQueueSurface())
            return false
        if (galleryController.viewerVisible)
            return false
        return activePanelHasGalleryHost()
    }

    function galleryViewerOwnsKeyboard() {
        return galleryController.viewerVisible
                && !sceneStore.hasBlockingOverlay()
                && !sceneStore.needsFallbackGrid()
                && !sceneStore.hasDocumentSurface()
                && !sceneStore.hasOperationsQueueSurface()
    }

    function restoreSurfaceFocus() {
        if (sceneStore.hasBlockingOverlay() || sceneStore.needsFallbackGrid()
                || sceneStore.hasDocumentSurface()
                || sceneStore.hasOperationsQueueSurface()) {
            focusTarget.forceActiveFocus()
            return
        }
        if (galleryController.viewerVisible) {
            const viewerLoader = galleryViewerLayer
                    ? galleryViewerLayer.item : null
            if (viewerLoader && viewerLoader.item)
                viewerLoader.item.forceActiveFocus()
            return
        }

        const shell = sceneStore.shellFrame()
        const panels = shell && shell.panels ? shell.panels : []
        const activeSide = effectiveActivePanelSide()
        for (let index = 0; index < panels.length; ++index) {
            const side = Number(panels[index].side)
            if (side !== activeSide)
                continue
            if (!hostWindow.panelSideVisible(side)
                    || sceneStore.panelSideCovered(side)) {
                focusTarget.forceActiveFocus()
                return
            }
            const panelHost = galleryPanelHost(side)
            if (panelHost) {
                panelHost.forceActiveFocus()
                return
            }
        }
        focusTarget.forceActiveFocus()
    }

    Timer {
        id: menuPointerSyncTimer
        interval: 90
        onTriggered: {
            if (controller.menuPointerMenuIndex < 0
                    || controller.menuPointerItemIndex < 0
                    || controller.menuPointerSentItemIndex
                       === controller.menuPointerItemIndex)
                return
            controller.menuPointerSentItemIndex = controller.menuPointerItemIndex
            controller.action({
                "action": "menuBar.itemSelect",
                "target": controller.menuPointerFrameId,
                "menuIndex": controller.menuPointerMenuIndex,
                "index": controller.menuPointerItemIndex
            }, true)
        }
    }

    Timer {
        id: pointerPanelActivationTimer
        interval: Math.max(1, controller.pointerPanelActivationTimeoutMs)
        repeat: false
        onTriggered: controller.finishPointerPanelActivation()
    }

    Connections {
        target: controller.sceneStore

        function onSceneReset() {
            controller.finishPointerPanelActivation()
            controller.syncAutocompleteSelection()
            if (controller.galleryController.viewerVisible
                    && (controller.sceneStore.hasDocumentSurface()
                        || controller.sceneStore.needsFallbackGrid())) {
                controller.galleryController.closeViewer()
            }
            Qt.callLater(controller.restoreSurfaceFocus)
        }

        function onActivePanelAcknowledged(side) {
            controller.finishPointerPanelActivation()
            if (!controller.activePanelHasGalleryHost())
                Qt.callLater(controller.restoreSurfaceFocus)
        }

        function onStructuralSurfaceUpdated() {
            if (controller.galleryController.viewerVisible
                    && (controller.sceneStore.hasDocumentSurface()
                        || controller.sceneStore.needsFallbackGrid())) {
                controller.galleryController.closeViewer()
            }
            Qt.callLater(controller.restoreSurfaceFocus)
        }

        function onCommandMenusUpdated() {
            controller.syncAutocompleteSelection()
            controller.restoreSurfaceFocus()
        }
    }
}
