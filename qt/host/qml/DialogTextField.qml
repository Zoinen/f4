pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

F4TextField {
    id: dialogEdit

    required property var widget
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
        dialogEdit.select(utf16IndexForRuneIndex(start),
                          utf16IndexForRuneIndex(end))
    }

    remoteCursorPosition: utf16IndexForRuneIndex(Number(widget.cursor || 0))
    remoteCursorVisible: widget.focused === true
    semanticFocus: widget.focused === true

    onPointerFocusRequested: dialogEdit.hostWindow.action({
            "target": dialogEdit.widget.id,
            "action": "control.focus"
        })
    onRemoteSelectionActivatedChanged: Qt.callLater(syncRemoteSelection)
    onWidgetChanged: Qt.callLater(syncRemoteSelection)
    onTextChanged: Qt.callLater(syncRemoteSelection)
    Component.onCompleted: Qt.callLater(syncRemoteSelection)
}
