pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: shortcuts

    required property ApplicationWindow hostWindow
    required property var galleryController
    visible: false
    width: 0
    height: 0

    Shortcut {
        sequence: "Shift+F12"
        context: Qt.ApplicationShortcut
        enabled: !shortcuts.galleryController.viewerVisible
        onActivated: shortcuts.hostWindow.action({
            "target": "app",
            "action": "presentation.toggle"
        }, true)
    }

    Shortcut {
        sequence: "Up"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: shortcuts.hostWindow.activeAutocompleteFrame() !== null
        onActivated: shortcuts.hostWindow.navigateAutocomplete(-1)
    }

    Shortcut {
        sequence: "Down"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: shortcuts.hostWindow.activeAutocompleteFrame() !== null
        onActivated: shortcuts.hostWindow.navigateAutocomplete(1)
    }

    Shortcut {
        sequence: "Return"
        context: Qt.ApplicationShortcut
        enabled: shortcuts.hostWindow.activeAutocompleteFrame() !== null
        onActivated: shortcuts.hostWindow.submitAutocomplete()
    }

    Shortcut {
        sequence: "Enter"
        context: Qt.ApplicationShortcut
        enabled: shortcuts.hostWindow.activeAutocompleteFrame() !== null
        onActivated: shortcuts.hostWindow.submitAutocomplete()
    }

    Shortcut {
        sequence: "Tab"
        context: Qt.ApplicationShortcut
        enabled: shortcuts.hostWindow.activeAutocompleteFrame() !== null
        onActivated: shortcuts.hostWindow.completeAutocomplete()
    }

    // Modified queue keys remain authoritative vtui commands. Plain table
    // navigation is mirrored locally for native autorepeat and synchronized by
    // stable task identity.
    Shortcut {
        sequence: "Up"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: shortcuts.hostWindow.activeOperationsQueueView() !== null
        onActivated: shortcuts.hostWindow.navigateOperationsQueue("up")
    }

    Shortcut {
        sequence: "Down"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: shortcuts.hostWindow.activeOperationsQueueView() !== null
        onActivated: shortcuts.hostWindow.navigateOperationsQueue("down")
    }

    Shortcut {
        sequence: "PgUp"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: shortcuts.hostWindow.activeOperationsQueueView() !== null
        onActivated: shortcuts.hostWindow.navigateOperationsQueue("pageUp")
    }

    Shortcut {
        sequence: "PgDown"
        context: Qt.ApplicationShortcut
        autoRepeat: true
        enabled: shortcuts.hostWindow.activeOperationsQueueView() !== null
        onActivated: shortcuts.hostWindow.navigateOperationsQueue("pageDown")
    }

    Shortcut {
        sequence: "Home"
        context: Qt.ApplicationShortcut
        enabled: shortcuts.hostWindow.activeOperationsQueueView() !== null
        onActivated: shortcuts.hostWindow.navigateOperationsQueue("home")
    }

    Shortcut {
        sequence: "End"
        context: Qt.ApplicationShortcut
        enabled: shortcuts.hostWindow.activeOperationsQueueView() !== null
        onActivated: shortcuts.hostWindow.navigateOperationsQueue("end")
    }

    Shortcut {
        sequence: "Return"
        context: Qt.ApplicationShortcut
        enabled: shortcuts.hostWindow.operationsQueueShortcutCanActivate()
        onActivated: shortcuts.hostWindow.activateOperationsQueueSelection()
    }

    Shortcut {
        sequence: "Enter"
        context: Qt.ApplicationShortcut
        enabled: shortcuts.hostWindow.operationsQueueShortcutCanActivate()
        onActivated: shortcuts.hostWindow.activateOperationsQueueSelection()
    }
}
