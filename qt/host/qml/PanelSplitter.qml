import QtQuick

Item {
    id: splitter

    objectName: "panelSplitter"

    // The containing panel surface owns the ratio. Keeping this component
    // stateless makes dragging presentation-only: it never emits an f4 action
    // or changes either panel's semantic model.
    property real ratio: 0.5
    property real defaultRatio: 0.5
    property real availableWidth: parent ? parent.width : 0
    property real minimumPanelWidth: 180
    property real keyboardStep: 16
    property bool surfaceActive: true
    // Modal chrome may disable dragging without removing the visual divider.
    // Keeping these concerns separate prevents menus from making the panel
    // boundary disappear underneath them.
    property bool surfaceVisible: surfaceActive
    property var keySink: null
    property var forwardedKeysDown: ({})
    property color hoverLineColor: "#4e9bd4"
    property color activeLineColor: "#f0c95a"
    property color trackColor: "#080b10"
    property color separatorColor: "#3a495b"
    property real gutterWidth: 16
    property real separatorWidth: 1
    // A panel can paint an interactive overlay (e.g. a scrollbar) into its
    // own reserved lane of this gutter. Keep the visual track/divider at the
    // full gutter width, but narrow the pointer hit area so a press there
    // reaches that overlay instead of starting a drag. "Leading" is the side
    // nearer the panel before the divider (smaller x); "trailing" is the
    // side nearer the panel after it (larger x).
    property real leadingHitInset: 0
    property real trailingHitInset: 0
    readonly property real effectiveMinimumWidth:
        Math.min(Math.max(0, minimumPanelWidth), Math.max(0, availableWidth / 2))
    readonly property real splitPosition: clampedPosition(availableWidth * ratio)
    readonly property bool dragging: pointer.dragging
    readonly property bool accessibilityHidden: !surfaceActive

    signal ratioRequested(real ratio)
    signal focusReleaseRequested()

    function clampedPosition(position) {
        if (availableWidth <= 0)
            return 0
        return Math.max(effectiveMinimumWidth,
                        Math.min(availableWidth - effectiveMinimumWidth,
                                 position))
    }

    function requestPosition(position) {
        if (availableWidth <= 0)
            return
        ratioRequested(clampedPosition(position) / availableWidth)
    }

    function reset() {
        requestPosition(availableWidth * defaultRatio)
    }

    function nudge(pixelDelta) {
        requestPosition(splitPosition + pixelDelta)
    }

    function rememberForwardedKey(key) {
        var keys = Object.assign({}, forwardedKeysDown)
        keys[String(key)] = true
        forwardedKeysDown = keys
    }

    function takeForwardedKey(key) {
        var name = String(key)
        if (!forwardedKeysDown[name])
            return false
        var keys = Object.assign({}, forwardedKeysDown)
        delete keys[name]
        forwardedKeysDown = keys
        return true
    }

    x: Math.round(splitPosition - width / 2)
    width: gutterWidth
    visible: surfaceVisible
    enabled: surfaceActive
    // Commander owns Tab. Keyboard resizing is intentionally opt-in by
    // clicking the divider, rather than joining the application's Tab chain.
    activeFocusOnTab: false

    Accessible.role: Accessible.Splitter
    Accessible.name: qsTr("Panel divider")
    Accessible.description: qsTr("Drag to resize the panels. Double-click to reset. Use the arrow keys for precise adjustment.")
    Accessible.focusable: surfaceActive
    Accessible.focused: activeFocus
    Accessible.ignored: accessibilityHidden

    onSurfaceActiveChanged: {
        if (!surfaceActive && activeFocus)
            focus = false
    }

    Keys.onPressed: (event) => {
        if (!surfaceActive || !activeFocus)
            return
        var modifiers = event.modifiers
                & (Qt.ShiftModifier | Qt.ControlModifier
                   | Qt.AltModifier | Qt.MetaModifier)
        var resizeModifiers = modifiers === Qt.NoModifier
                || modifiers === Qt.ShiftModifier
        if (event.key === Qt.Key_Left && resizeModifiers) {
            nudge(event.modifiers & Qt.ShiftModifier ? -keyboardStep * 4
                                                    : -keyboardStep)
            event.accepted = true
        } else if (event.key === Qt.Key_Right && resizeModifiers) {
            nudge(event.modifiers & Qt.ShiftModifier ? keyboardStep * 4
                                                    : keyboardStep)
            event.accepted = true
        } else if (event.key === Qt.Key_Home
                   && modifiers === Qt.NoModifier) {
            reset()
            event.accepted = true
        } else if (event.key === Qt.Key_Escape) {
            focus = false
            focusReleaseRequested()
            event.accepted = true
        } else if (keySink) {
            // Once pointer focus opts into resize mode, every non-resize key
            // remains a commander key. Accepting Tab here prevents Qt Quick
            // from entering its control focus chain.
            keySink.sendQtKey(event.key, event.text, true,
                              event.modifiers, event.nativeScanCode)
            rememberForwardedKey(event.key)
            event.accepted = true
        }
    }
    Keys.onReleased: (event) => {
        if (takeForwardedKey(event.key)) {
            if (keySink)
                keySink.sendQtKey(event.key, event.text, false,
                                  event.modifiers, event.nativeScanCode)
            event.accepted = true
            return
        }
        var modifiers = event.modifiers
                & (Qt.ShiftModifier | Qt.ControlModifier
                   | Qt.AltModifier | Qt.MetaModifier)
        var resizeModifiers = modifiers === Qt.NoModifier
                || modifiers === Qt.ShiftModifier
        if ((resizeModifiers
             && (event.key === Qt.Key_Left || event.key === Qt.Key_Right))
                || (modifiers === Qt.NoModifier
                    && event.key === Qt.Key_Home)) {
            event.accepted = true
        }
    }

    Rectangle {
        anchors.fill: parent
        color: splitter.trackColor
    }

    Rectangle {
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        anchors.horizontalCenter: parent.horizontalCenter
        width: splitter.separatorWidth
        color: splitter.dragging || splitter.activeFocus
               ? splitter.activeLineColor
               : (pointer.containsMouse ? splitter.hoverLineColor
                                        : splitter.separatorColor)
    }

    MouseArea {
        id: pointer
        x: splitter.leadingHitInset
        width: Math.max(0, splitter.width - splitter.leadingHitInset
                         - splitter.trailingHitInset)
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        acceptedButtons: Qt.LeftButton
        hoverEnabled: true
        preventStealing: true
        cursorShape: Qt.SplitHCursor
        property bool dragging: false
        property bool moved: false
        property real dragOffset: 0

        function parentPosition(mouse) {
            return mapToItem(splitter.parent, mouse.x, mouse.y)
        }

        function requestMousePosition(mouse) {
            var point = parentPosition(mouse)
            splitter.requestPosition(point.x - dragOffset)
        }

        onPressed: (mouse) => {
            var point = parentPosition(mouse)
            dragging = true
            moved = false
            // Preserve where within the gutter the user grabbed it,
            // so an off-center press never snaps the divider under the cursor.
            dragOffset = point.x - splitter.splitPosition
            splitter.forceActiveFocus()
            mouse.accepted = true
        }
        onPositionChanged: (mouse) => {
            if (dragging) {
                moved = true
                requestMousePosition(mouse)
            }
        }
        onReleased: (mouse) => {
            if (dragging && moved)
                requestMousePosition(mouse)
            dragging = false
            moved = false
            mouse.accepted = true
        }
        onCanceled: {
            dragging = false
            moved = false
        }
        onDoubleClicked: (mouse) => {
            dragging = false
            moved = false
            splitter.reset()
            mouse.accepted = true
        }
    }
}
