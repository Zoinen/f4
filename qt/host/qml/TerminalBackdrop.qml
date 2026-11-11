pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQml.Models

Rectangle {
    id: terminalBackdrop
    required property ApplicationWindow hostWindow
    required property Item menuBar
    property var terminal: ({})
    property var shell: ({})
    property bool terminalSurfaceCreated: false
    readonly property real splitRatio: {
        var layout = shell && shell.panelLayout
                ? shell.panelLayout : ({})
        var columns = Number(layout.columns || 0)
        var splitColumn = Number(layout.splitColumn || 0)
        return columns > 0 && splitColumn > 0 && splitColumn < columns
                ? splitColumn / columns : 0.5
    }
    // If only the left side is exposed, the right edge of the terminal is
    // underneath the still-visible right panel. Keep the scrollbar on the
    // exposed side of the split instead of allowing that panel to cover it.
    readonly property real scrollBarRightInset:
        shell && shell.terminalActive !== true
        && shell.showLeftPanel === false
        && shell.showRightPanel !== false
        ? width * (1 - splitRatio) : 0
    objectName: "terminalBackdrop"
    color: "transparent"
    clip: true

    onVisibleChanged: {
        if (visible)
            terminalSurfaceCreated = true
    }
    Component.onCompleted: {
        if (visible)
            terminalSurfaceCreated = true
    }

    // Each exposed region is a real viewport. Only horizontal clipping is
    // needed to preserve terminal columns when panel heights differ.
    readonly property real leftCoveredHeight: coveredHeight(0)
    readonly property real rightCoveredHeight: coveredHeight(1)
    readonly property bool splitRegions: leftCoveredHeight !== rightCoveredHeight
    readonly property real splitX: hostWindow.nativePanelSplitPosition()

    function coveredHeight(side) {
        if (shell.terminalActive === true)
            return 0
        const wide = hostWindow.widePanelSide()
        const panelSide = wide >= 0 ? wide : side
        if (!hostWindow.panelSideVisible(panelSide))
            return 0
        return Math.min(height, hostWindow.nativePanelHeight(panelSide, hostWindow.menuBarHeight))
    }

    Instantiator {
        model: ["Ctrl+F", "Ctrl+G", "Ctrl+Shift+G"]
        delegate: Shortcut {
            required property int index
            required property string modelData
            sequence: Qt.platform.os === "osx" ? modelData.replace("Ctrl", "Meta") : modelData
            context: Qt.WindowShortcut
            autoRepeat: false
            enabled: terminalBackdrop.visible && terminalBackdrop.hostWindow.terminalActive()
                     && terminalBackdrop.terminal.altScreen !== true
                     && !terminalBackdrop.hostWindow.hasBlockingOverlay()
                     && !terminalBackdrop.hostWindow.hasStandaloneDocumentSurface()
                     && !terminalBackdrop.hostWindow.queueDropdownOpen
                     && terminalBackdrop.hostWindow.cleanText(terminalBackdrop.terminal.id) !== ""
                     && (index === 0 || !!terminalBackdrop.terminal.searchMatch)
            onActivated: terminalBackdrop.hostWindow.action({
                "target": terminalBackdrop.terminal.id,
                "action": index === 0 ? "terminal.search" : "terminal.searchAgain",
                "reverse": index === 2
            }, true)
        }
    }

    Item {
        id: leftRegion
        objectName: "terminalExposedLeft"
        y: terminalBackdrop.leftCoveredHeight
        width: terminalBackdrop.splitRegions ? terminalBackdrop.splitX : parent.width
        height: Math.max(0, parent.height - y)
        clip: true

        Loader {
            width: terminalBackdrop.width
            height: parent.height
            active: terminalBackdrop.terminalSurfaceCreated && leftRegion.height > 0
            sourceComponent: DocumentSurface {
                hostWindow: terminalBackdrop.hostWindow
                menuBar: terminalBackdrop.menuBar
                frame: terminalBackdrop.terminal
                embedded: true
                interactionActive: terminalBackdrop.visible
                scrollBarRightInset: terminalBackdrop.splitRegions
                    ? terminalBackdrop.width - leftRegion.width
                    : terminalBackdrop.scrollBarRightInset
                // One reporter reserves enough rows for both exposed regions.
                nativeViewportHeight: Math.max(leftRegion.height, rightRegion.height)
                surfaceObjectName: "terminalDocumentSurface"
            }
        }
    }

    Item {
        id: rightRegion
        objectName: "terminalExposedRight"
        x: terminalBackdrop.splitX
        y: terminalBackdrop.rightCoveredHeight
        width: Math.max(0, parent.width - x)
        height: Math.max(0, parent.height - y)
        visible: terminalBackdrop.splitRegions
        clip: true

        Loader {
            x: -rightRegion.x
            width: terminalBackdrop.width
            height: parent.height
            active: terminalBackdrop.terminalSurfaceCreated && terminalBackdrop.splitRegions && rightRegion.height > 0
            sourceComponent: DocumentSurface {
                hostWindow: terminalBackdrop.hostWindow
                menuBar: terminalBackdrop.menuBar
                frame: terminalBackdrop.terminal
                embedded: true
                // The first view owns terminal viewport geometry; publishing
                // it twice lets destruction of one view clear the other's size.
                reportsNativeViewport: leftRegion.height <= 0
                interactionActive: terminalBackdrop.visible
                scrollBarRightInset: terminalBackdrop.scrollBarRightInset
                surfaceObjectName: "terminalDocumentSurfaceRight"
            }
        }
    }
}
