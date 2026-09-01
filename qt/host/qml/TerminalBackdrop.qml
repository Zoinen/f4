pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

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

    // Terminal history uses the same bounded, pooled row-window renderer
    // as Viewer/Editor. QML retains only a few viewports while Go's
    // PieceTable/GridHistory remain the sole owners of complete scrollback.
    Loader {
        anchors.fill: parent
        active: terminalBackdrop.terminalSurfaceCreated
        sourceComponent: Component {
            DocumentSurface {
                hostWindow: terminalBackdrop.hostWindow
                menuBar: terminalBackdrop.menuBar
                frame: terminalBackdrop.terminal
                embedded: true
                interactionActive: terminalBackdrop.visible
                scrollBarRightInset: terminalBackdrop.scrollBarRightInset
                surfaceObjectName: "terminalDocumentSurface"
            }
        }
    }
}
