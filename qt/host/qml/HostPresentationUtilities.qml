pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

QtObject {
    id: utilities

    required property ApplicationWindow hostWindow
    required property var iconProvider

    function cleanText(value) {
        return value === undefined || value === null ? "" : String(value)
    }

    function snapPx(value) {
        return Math.round(Number(value || 0) * hostWindow.dpr) / hostWindow.dpr
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
        return snapPx(localPoint.x) - localPoint.x + layoutRevision * 0
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
        return snapPx(localPoint.y) - localPoint.y + layoutRevision * 0
    }

    function iconPixelOffsetX(item) {
        if (!item || !item.parent || !hostWindow.contentItem)
            return 0
        const layoutRevision = hostWindow.width + hostWindow.height
                + hostWindow.panelSplitRatio + item.alignmentRevision
        const scenePoint = item.parent.mapToItem(hostWindow.contentItem,
                                                 item.x, item.y)
        return snapPx(scenePoint.x) - scenePoint.x + layoutRevision * 0
    }

    function iconPixelOffsetY(item) {
        if (!item || !item.parent || !hostWindow.contentItem)
            return 0
        const layoutRevision = hostWindow.width + hostWindow.height
                + hostWindow.panelSplitRatio + item.alignmentRevision
        const scenePoint = item.parent.mapToItem(hostWindow.contentItem,
                                                 item.x, item.y)
        return snapPx(scenePoint.y) - scenePoint.y + layoutRevision * 0
    }

    function pxX(value) {
        return Math.round((value || 0) * hostWindow.cw)
    }

    function pxY(value) {
        return Math.round((value || 0) * hostWindow.ch)
    }

    function pxW(value) {
        return Math.max(1, Math.round((value || 0) * hostWindow.cw))
    }

    function pxH(value) {
        return Math.max(1, Math.round((value || 0) * hostWindow.ch))
    }

    function nativePanelSplitPosition() {
        const minimum = Math.min(hostWindow.panelMinimumWidth,
                                 Math.max(0, hostWindow.width / 2))
        return Math.round(Math.max(minimum,
                    Math.min(hostWindow.width - minimum,
                             hostWindow.width * hostWindow.panelSplitRatio)))
    }

    function widePanelSide() {
        const shell = hostWindow.shellFrame()
        return shell.wide === true ? Number(shell.widePanel || 0) : -1
    }

    function panelSideVisible(side) {
        const wideSide = widePanelSide()
        if (wideSide >= 0)
            return Number(side) === wideSide
        const shell = hostWindow.shellFrame()
        return Number(side) === 0 ? shell.showLeftPanel !== false
                                  : shell.showRightPanel !== false
    }

    function nativePanelX(side) {
        if (widePanelSide() >= 0)
            return 0
        return Number(side) === 0 ? 0 : nativePanelSplitPosition()
    }

    function nativePanelWidth(side) {
        if (widePanelSide() >= 0)
            return hostWindow.width
        const split = nativePanelSplitPosition()
        return Number(side) === 0 ? split : hostWindow.width - split
    }

    function repeaterMaxImplicitWidth(repeater) {
        let maxWidth = 0
        for (let index = 0; index < repeater.count; ++index) {
            const item = repeater.itemAt(index)
            if (item)
                maxWidth = Math.max(maxWidth, item.implicitWidth)
        }
        return maxWidth
    }

    function resolvedIconSource(name, logicalSize) {
        const generation = iconProvider.revision
        return iconProvider.iconSource(name, logicalSize,
                                       hostWindow.iconDevicePixelRatio)
    }

    function fileIconSource(localPath, fileName, directory, logicalSize,
                            version) {
        const generation = iconProvider.revision
        const size = Number(logicalSize) > 0 ? Number(logicalSize) : 16
        const iconVersion = Number(version) > 0 ? Number(version) : 0
        return iconProvider.fileIconSource(
                    cleanText(localPath), cleanText(fileName),
                    directory === true, size,
                    hostWindow.iconDevicePixelRatio, iconVersion)
    }

    function lucideIconSource(name, size, tint) {
        const value = cleanText(name)
        if (value === "")
            return ""
        const logicalSize = Number(size) > 0 ? Number(size) : 16
        const color = tint === undefined || tint === null
                ? hostWindow.chromeText : tint
        if (typeof iconProvider.rasterizedLucideSource === "function") {
            return iconProvider.rasterizedLucideSource(
                        value, logicalSize,
                        hostWindow.iconDevicePixelRatio, color)
        }
        return typeof iconProvider.iconSource === "function"
                ? iconProvider.iconSource(value, logicalSize,
                                          hostWindow.iconDevicePixelRatio) : ""
    }

    function semanticMenuIconSource(name, size, tint) {
        const value = cleanText(name)
        if (value === "")
            return ""
        return iconProvider.system === true
                ? resolvedIconSource(value, size)
                : lucideIconSource(value, size, tint)
    }

    function semanticPanelSplitRatio() {
        const shell = hostWindow.shellFrame()
        const layout = shell && shell.panelLayout ? shell.panelLayout : ({})
        const columns = Number(layout.columns || 0)
        const splitColumn = Number(layout.splitColumn || 0)
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
        return active === true ? hostWindow.textColor : hostWindow.mutedText
    }

    function workspaceTabNumberColor() {
        return hostWindow.mutedText
    }

    function workspaceTabShortcut(tab, platformName) {
        tab = tab || ({})
        const number = Number(tab.number || 0)
        if (number < 1 || number > 9 || tab.shortcutAvailable === false)
            return ""
        return cleanText(platformName) === "osx"
                ? "\u2325" + String(number) : "Alt+" + String(number)
    }

    function workspaceTabToolTip(tab, platformName) {
        tab = tab || ({})
        let primary = cleanText(tab.tooltipPrimary).trim()
        const secondary = cleanText(tab.tooltipSecondary).trim()
        if (primary === "")
            primary = cleanText(tab.text).trim()
        if (secondary !== "")
            primary = primary === "" ? secondary : primary + "\n" + secondary
        const shortcut = workspaceTabShortcut(tab, platformName)
        if (primary === "")
            return shortcut
        return shortcut === "" ? primary : primary + "\t" + shortcut
    }

    function workspaceTabFontWeight() {
        return Font.Normal
    }

    function vtuiKeyModifiers(modifiers) {
        let result = 0
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
        let result = 0
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
        return snapPx(Math.min(hostWindow.workspaceTabMaxWidth,
                 Math.max(hostWindow.workspaceTabMinWidth,
                          Number(titleWidth || 0) + chromeWidth)))
    }

    function richTextEscape(value) {
        return cleanText(value).replace(/&/g, "&amp;")
                               .replace(/</g, "&lt;")
                               .replace(/>/g, "&gt;")
                               .replace(/\"/g, "&quot;")
    }

    function mnemonicText(value, hotkey) {
        const source = cleanText(value)
        const characters = []
        let mnemonicIndex = -1
        for (let index = 0; index < source.length; ++index) {
            if (source[index] !== "&") {
                characters.push(source[index])
                continue
            }
            if (index + 1 >= source.length) {
                characters.push("&")
                continue
            }
            if (source[index + 1] === "&") {
                characters.push("&")
                ++index
                continue
            }
            mnemonicIndex = characters.length
            characters.push(source[++index])
        }

        if (mnemonicIndex < 0 && cleanText(hotkey) !== "") {
            const wanted = cleanText(hotkey).toLocaleLowerCase()
            for (let index = 0; index < characters.length; ++index) {
                if (characters[index].toLocaleLowerCase() === wanted) {
                    mnemonicIndex = index
                    break
                }
            }
        }

        let result = ""
        for (let index = 0; index < characters.length; ++index) {
            const escaped = richTextEscape(characters[index])
            result += index === mnemonicIndex
                    ? "<font color=\"" + hostWindow.dialogAccent + "\">"
                      + escaped + "</font>" : escaped
        }
        return result
    }

    function runsText(runs) {
        if (!runs)
            return ""
        let result = ""
        for (let index = 0; index < runs.length; ++index)
            result += cleanText(runs[index].text)
        return result
    }

    function rowText(row) {
        if (!row)
            return ""
        return row.text !== undefined ? cleanText(row.text)
                                      : runsText(row.runs)
    }
}
