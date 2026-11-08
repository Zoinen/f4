pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import ZoinGallery.PixelGrid 1.0

WheelHandler {
    id: wheel
    required property Flickable viewport
    required property ApplicationWindow hostWindow
    property real rowHeight: hostWindow.ch
    property bool containAtBounds: false
    readonly property real minimum: viewport.originY - viewport.topMargin
    readonly property real maximum: Math.max(minimum, viewport.originY + viewport.contentHeight
                                              - viewport.height + viewport.bottomMargin)
    readonly property bool canScroll: maximum - minimum > 0.5 / hostWindow.dpr
    readonly property bool running: frameClock.running
    property real destinationY: 0
    property real segmentStartY: 0
    property real elapsed: 0
    readonly property real duration: 0.150
    signal scrolled(real rows)
    signal settled()

    enabled: viewport.visible && (canScroll || containAtBounds)
    target: null
    acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad

    function cancel() {
        frameClock.stop()
    }

    function updatePixelMotion() {
        viewport.contentItem.PixelGridScope.motionActive = running || viewport.moving
    }

    onRunningChanged: updatePixelMotion()

    property Binding smoothViewportMotion: Binding {
        target: wheel.viewport
        property: "pixelAligned"
        when: wheel.running
        value: false
    }

    function bounded(position) {
        return Math.max(minimum, Math.min(maximum, hostWindow.snapPx(position)))
    }

    onWheel: event => {
        const precise = event.pixelDelta.y !== 0
        const lines = Math.max(1, Qt.styleHints.wheelScrollLines)
        // Match GalleryViewportController's mouse-wheel distance. Keep the
        // terminal's row units for the optional Go scroll notification.
        const delta = precise ? -event.pixelDelta.y : -event.angleDelta.y
        const base = !precise && running ? destinationY : viewport.contentY
        const destination = precise ? Math.max(minimum, Math.min(maximum, base + delta))
                                    : bounded(base + delta)
        event.accepted = containAtBounds || (canScroll && delta !== 0
            && (destination !== (precise ? base : bounded(base)) || running))
        if (!event.accepted || !canScroll || delta === 0)
            return

        if (viewport.flicking)
            viewport.cancelFlick()
        if (precise) {
            cancel()
            viewport.contentY = destination
        } else if (!running || destinationY !== destination) {
            segmentStartY = viewport.contentY
            destinationY = destination
            elapsed = 0
            if (!running)
                frameClock.start()
        }
        scrolled(precise ? delta / Math.max(1, rowHeight) : -event.angleDelta.y / 120 * lines)
    }

    // The gallery's render-thread camera targets MasonryLayout. An ordinary
    // Flickable still needs its live contentY for delegates, input and bars.
    // Keep one frame clock across retargets so bursts never restart that clock.
    property FrameAnimation clock: FrameAnimation {
        id: frameClock
        onTriggered: {
            wheel.elapsed += frameTime
            const progress = Math.min(1, wheel.elapsed / wheel.duration)
            if (progress >= 1) {
                wheel.viewport.contentY = wheel.destinationY
                stop()
                wheel.settled()
            } else {
                const eased = Math.sin(progress * Math.PI / 2)
                wheel.viewport.contentY = wheel.segmentStartY
                    + (wheel.destinationY - wheel.segmentStartY) * eased
            }
        }
    }

    onEnabledChanged: if (!enabled) cancel()
    onMaximumChanged: if (running && destinationY > maximum) cancel()
    onMinimumChanged: if (running && destinationY < minimum) cancel()

    property Connections dragConnection: Connections {
        target: wheel.viewport
        function onDraggingChanged() { if (wheel.viewport.dragging) wheel.cancel() }
        function onMovingChanged() { wheel.updatePixelMotion() }
    }
    property Connections scrollbarConnection: Connections {
        target: wheel.viewport.ScrollBar.vertical
        function onPressedChanged() { if (target.pressed) wheel.cancel() }
    }
}
