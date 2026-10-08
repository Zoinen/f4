pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

F4TextField {
    id: dialogEdit

    required property var widget
    readonly property alias indicator: historyIcon
    trailingInset: widget.history === true ? hostWindow.snapPx(30) : 0
    Item {
        id: historyButton
        objectName: dialogEdit.objectName + "HistoryButton"
        visible: dialogEdit.widget.history === true
        z: 10
        width: dialogEdit.trailingInset
        height: dialogEdit.height
        x: hostWindow.snapPx(dialogEdit.width - width)
        HostPixelAlignedImage {
            id: historyIcon
            objectName: dialogEdit.objectName + "HistoryIcon"
            hostWindow: dialogEdit.hostWindow
            width: hostWindow.snapPx(14)
            height: width
            x: hostWindow.snapPx((historyButton.width - width) / 2)
            y: hostWindow.snapPx((historyButton.height - height) / 2)
            sourceSize: Qt.size(width * hostWindow.dpr, height * hostWindow.dpr)
            source: hostWindow.lucideIconSource("chevron-down", 14, hostWindow.mutedText)
        }
        MouseArea {
            anchors.fill: parent
            onClicked: dialogEdit.hostWindow.action({target: dialogEdit.widget.id, action: "control.history"}, true)
        }
    }
    property string registeredOwnerId: ""
    function syncDropdownRegistration() {
        const nextId = hostWindow.cleanText(widget.id)
        if (registeredOwnerId !== "" && registeredOwnerId !== nextId)
            hostWindow.unregisterDropdownAnchor(registeredOwnerId, dialogEdit)
        registeredOwnerId = nextId
        hostWindow.registerDropdownAnchor(registeredOwnerId, dialogEdit)
    }
    Component.onDestruction: {
        if (hostWindow && typeof hostWindow.unregisterDropdownAnchor === "function")
            hostWindow.unregisterDropdownAnchor(registeredOwnerId, dialogEdit)
    }
    enabled: widget.disabled !== true
    remoteControlled: true
    readOnly: true
    text: hostWindow.cleanText(widget.text)
    echoMode: widget.password ? TextInput.Password : TextInput.Normal
    // Go initializes an edit with a selection, but keeps it in a dormant
    // state until the semantic control is focused.  Do not expose that
    // selection while the dialog is merely being shown; the focused update
    // below is the point at which the native TextInput should paint it.
    readonly property bool remoteSelectionActivated:
        widget && widget.focused === true
            && (widget.selectionActive === undefined
                || widget.selectionActive === true)

    function utf16IndexForRuneIndex(runeIndex) {
        const value = hostWindow.cleanText(text)
        const codePoints = Array.from(value)
        const count = Math.max(0, Math.min(codePoints.length,
                                            Number(runeIndex)))
        var utf16Index = 0
        for (var i = 0; i < count; ++i)
            utf16Index += codePoints[i].length
        return utf16Index
    }

    function syncRemoteSelection() {
        if (!remoteSelectionActivated
                || !widget || widget.selectionStart === undefined
                || widget.selectionEnd === undefined)
        {
            dialogEdit.deselect()
            return
        }
        const start = Number(widget.selectionStart)
        const end = Number(widget.selectionEnd)
        if (!isFinite(start) || !isFinite(end)
                || start < 0 || end < 0 || start === end) {
            dialogEdit.deselect()
            return
        }
        const cursorAtStart = Number(widget.cursor) === start
        dialogEdit.select(utf16IndexForRuneIndex(cursorAtStart ? end : start),
                          utf16IndexForRuneIndex(cursorAtStart ? start : end))
    }

    remoteCursorPosition: utf16IndexForRuneIndex(Number(widget.cursor || 0))
    readonly property bool completionOwnsEdit: {
        const completion = hostWindow.activeAutocompleteFrame()
        return completion !== null && completion.ownerId === widget.id
    }
    remoteCursorVisible: widget.focused === true || completionOwnsEdit
    semanticFocus: widget.focused === true || completionOwnsEdit
    continuousCursorBlink: completionOwnsEdit

    function publishPointerSelection() {
        const cursor = Array.from(text.slice(0, cursorPosition)).length
        const anchorOffset = cursorPosition === selectionStart
                           ? selectionEnd : selectionStart
        hostWindow.action({target: widget.id, action: "control.select",
                           anchor: Array.from(text.slice(0, anchorOffset)).length,
                           cursor: cursor}, true)
    }
    onPointerSelectionFinished: publishPointerSelection()
    onRemoteKeyEvent: (event, down) => {
        const sink = hostWindow.focusTarget
        if (!sink || typeof sink.sendQtKeyEvent !== "function") return
        sink.sendQtKeyEvent(event.key, event.text, down, event.modifiers,
                            event.nativeScanCode || 0, event.isAutoRepeat === true)
        event.accepted = true
    }

    onPointerFocusRequested: dialogEdit.hostWindow.action({
            "target": dialogEdit.widget.id,
            "action": "control.focus"
        }, true)
    onRemoteSelectionActivatedChanged: Qt.callLater(syncRemoteSelection)
    onWidgetChanged: { syncDropdownRegistration(); Qt.callLater(syncRemoteSelection) }
    onTextChanged: Qt.callLater(syncRemoteSelection)
    Component.onCompleted: { syncDropdownRegistration(); Qt.callLater(syncRemoteSelection) }
}
