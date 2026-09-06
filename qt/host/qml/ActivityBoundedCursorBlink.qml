pragma ComponentBehavior: Bound

import QtQuick

// Preserve the useful caret activity cue without keeping the scene graph
// alive forever. Each real input/focus change gets one ordinary blink cycle;
// the steady state is a solid caret and a stopped timer.
Item {
    id: controller

    width: 0
    height: 0
    visible: false

    property bool active: false
    property int activityRevision: 0
    property int interval: 520
    property bool blinkOn: true
    property int remainingTransitions: 0
    readonly property bool running: blinkTimer.running

    function settle() {
        blinkTimer.stop()
        remainingTransitions = 0
        blinkOn = true
    }

    function restart() {
        blinkTimer.stop()
        blinkOn = true
        if (!active) {
            remainingTransitions = 0
            return
        }
        // Off and back on is one complete, bounded blink cycle.
        remainingTransitions = 2
        blinkTimer.start()
    }

    onActiveChanged: {
        if (active)
            restart()
        else
            settle()
    }
    onActivityRevisionChanged: {
        if (active)
            restart()
    }
    Component.onDestruction: blinkTimer.stop()

    Timer {
        id: blinkTimer
        interval: controller.interval
        repeat: false
        onTriggered: {
            if (!controller.active
                    || controller.remainingTransitions <= 0) {
                controller.settle()
                return
            }
            controller.blinkOn = !controller.blinkOn
            --controller.remainingTransitions
            if (controller.remainingTransitions > 0)
                blinkTimer.start()
            else
                controller.blinkOn = true
        }
    }
}
