pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import QtQuick.Controls
import QtQuick.Controls.Basic as T

Item {
    id: control

    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property alias text: innerInput.text
    property alias placeholderText: placeholderLabel.text
    property alias readOnly: innerInput.readOnly
    property alias echoMode: innerInput.echoMode
    property alias inputMethodHints: innerInput.inputMethodHints
    property alias validator: innerInput.validator
    property alias font: innerInput.font
    property alias horizontalAlignment: innerInput.horizontalAlignment
    property alias cursorPosition: innerInput.cursorPosition
    property alias selectedText: innerInput.selectedText
    property alias selectionStart: innerInput.selectionStart
    property alias selectionEnd: innerInput.selectionEnd
    property bool acceptableInput: innerInput.acceptableInput

    property bool hasBackground: true
    readonly property bool hovered: control.enabled && fieldHover.hovered
    property url leadingIconSource: ""
    property real leadingIconSize: 14
    property bool semanticFocus: false
    property bool remoteControlled: false
    property int remoteCursorPosition: 0
    property bool remoteCursorVisible: false
    property int cursorActivityRevision: 0

    signal accepted()
    signal textEdited()
    signal editingFinished()
    // Semantic dialog fields still use the native TextInput for pointer
    // selection.  The surrounding semantic layer may ask Go to focus the
    // control, but it must not take the press away from TextInput itself.
    signal pointerFocusRequested()

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    function selectAll() { innerInput.selectAll() }
    function select(start, end) { innerInput.select(start, end) }
    function deselect() { innerInput.deselect() }
    function copy() { innerInput.copy() }
    function cut() { innerInput.cut() }
    function paste() { innerInput.paste() }
    function forceActiveFocus() { innerInput.forceActiveFocus() }

    // The visual text input is inset by the field's RowLayout margins. Keep
    // those margins interactive as well: when a drag starts just inside the
    // border, Qt otherwise has no TextInput hit target and cannot establish a
    // selection anchor even though the cursor is already shown as an I-beam.
    // mapToItem() itself is not a reactive dependency. Include the complete
    // layout chain in a revision so the margin hit areas are recomputed after
    // the RowLayout has received its real width/height (rather than staying
    // at the zero-sized value observed during component construction).
    readonly property real textInputLayoutRevision: {
        var revision = control.width + control.height
        var item = innerInput
        for (var depth = 0; item && depth < 10; ++depth) {
            revision += item.x + item.y + item.width + item.height
            item = item.parent
        }
        return revision
    }
    readonly property real textInputLeftEdge: {
        const revision = textInputLayoutRevision
        return innerInput.mapToItem(control, 0, 0).x + revision * 0
    }
    readonly property real textInputRightEdge:
        textInputLeftEdge + innerInput.width

    function textPositionAtControlPoint(pointX, pointY) {
        const point = innerInput.mapFromItem(control, pointX, pointY)
        const x = Math.max(0, Math.min(innerInput.width, Number(point.x)))
        if (typeof innerInput.positionAt === "function") {
            const position = Number(innerInput.positionAt(x))
            if (isFinite(position))
                return Math.max(0, Math.min(innerInput.length, position))
        }
        return x <= 0 ? 0 : innerInput.length
    }

    // TextInput does not expose its native word/paragraph gesture handling
    // through the QML API.  The side hit areas below intentionally mirror
    // that behavior for clicks which land in the field's visual padding:
    // the second click selects the word under the pointer and the third
    // selects the complete value.  Keep the calculation in UTF-16 offsets,
    // since TextInput.selectionStart/End use those offsets even when the
    // displayed value contains non-BMP characters.
    function selectWordAtPosition(position) {
        const value = String(innerInput.text || "")
        if (value.length === 0) {
            innerInput.deselect()
            return
        }

        const codePoints = Array.from(value)
        const offsets = [0]
        for (var offsetIndex = 0; offsetIndex < codePoints.length;
             ++offsetIndex) {
            offsets.push(offsets[offsets.length - 1]
                         + codePoints[offsetIndex].length)
        }

        const bounded = Math.max(0, Math.min(value.length,
                                              Number(position || 0)))
        let codePointIndex = codePoints.length - 1
        for (var index = 0; index < codePoints.length; ++index) {
            if (bounded < offsets[index + 1]) {
                codePointIndex = index
                break
            }
        }

        function characterKind(character) {
            if (/\s/.test(character))
                return "space"
            // Treat letters, numbers and underscore as a word.  The case
            // comparison covers non-ASCII letters without splitting their
            // UTF-16 representation; punctuation remains its own token.
            if (character === "_"
                    || character.toUpperCase() !== character.toLowerCase()
                    || /^[0-9]$/.test(character))
                return "word"
            return "punctuation"
        }

        const kind = characterKind(codePoints[codePointIndex])
        let start = codePointIndex
        let end = codePointIndex + 1
        while (start > 0
               && characterKind(codePoints[start - 1]) === kind)
            --start
        while (end < codePoints.length
               && characterKind(codePoints[end]) === kind)
            ++end
        innerInput.select(offsets[start], offsets[end])
    }

    function marginClickInterval() {
        const hints = Qt.styleHints
        const interval = hints ? Number(hints.mouseDoubleClickInterval) : 0
        return isFinite(interval) && interval > 0 ? interval : 400
    }

    function beginMarginPress(area, mouse) {
        area.pressX = mouse.x
        area.pressY = mouse.y
        area.dragMoved = false
        control.beginMarginSelection(area, mouse)
    }

    function updateMarginPress(area, mouse) {
        if (!area.dragging)
            return
        const dx = mouse.x - area.pressX
        const dy = mouse.y - area.pressY
        if (dx * dx + dy * dy > 9)
            area.dragMoved = true
        control.updateMarginSelection(area, mouse)
    }

    function finishMarginPress(area, mouse) {
        if (!area.dragging)
            return
        control.updateMarginSelection(area, mouse)
        const moved = area.dragMoved
        area.dragging = false
        if (moved) {
            // A drag is a distinct gesture.  Do not let its release become
            // the first click of a later double-click sequence.
            area.clickCount = 0
            area.lastClickAt = 0
            return
        }

        const point = area.mapToItem(control, mouse.x, mouse.y)
        const now = Date.now()
        const sameSpot = area.clickCount > 0
                && now >= area.lastClickAt
                && now - area.lastClickAt <= control.marginClickInterval()
                && Math.abs(point.x - area.lastClickX) <= control.snap(6)
                && Math.abs(point.y - area.lastClickY) <= control.snap(6)
        area.clickCount = sameSpot
                ? Math.min(3, area.clickCount + 1) : 1
        area.lastClickAt = now
        area.lastClickX = point.x
        area.lastClickY = point.y

        const position = control.textPositionAtControlPoint(point.x, point.y)
        if (area.clickCount === 2)
            control.selectWordAtPosition(position)
        else if (area.clickCount >= 3) {
            innerInput.selectAll()
            area.clickCount = 0
        }
    }

    function cancelMarginPress(area) {
        area.dragging = false
        area.dragMoved = false
        area.clickCount = 0
        area.lastClickAt = 0
    }

    function beginMarginSelection(area, mouse) {
        area.dragging = true
        area.anchorPosition = 0
        const point = area.mapToItem(control, mouse.x, mouse.y)
        control.pointerFocusRequested()
        innerInput.forceActiveFocus()
        area.anchorPosition = textPositionAtControlPoint(point.x, point.y)
        innerInput.deselect()
        innerInput.cursorPosition = area.anchorPosition
        mouse.accepted = true
    }

    function updateMarginSelection(area, mouse) {
        if (!area.dragging)
            return
        const point = area.mapToItem(control, mouse.x, mouse.y)
        const position = textPositionAtControlPoint(point.x, point.y)
        if (position === area.anchorPosition) {
            innerInput.deselect()
            innerInput.cursorPosition = position
        } else {
            innerInput.select(Math.min(area.anchorPosition, position),
                              Math.max(area.anchorPosition, position))
        }
        mouse.accepted = true
    }

    implicitHeight: snap(32)
    implicitWidth: snap(180)

    HoverHandler {
        id: fieldHover
        enabled: control.enabled
    }

    Rectangle {
        id: bgRect
        objectName: control.objectName ? (control.objectName + "Background")
                                       : "textFieldBackground"
        readonly property color testBorderColor: border.color
        anchors.fill: parent
        visible: control.hasBackground
        radius: control.snap(4)
        color: control.hostWindow
               ? (control.hovered ? control.hostWindow.inputHoverBg
                                  : control.hostWindow.controlPressedBg)
               : "#18202a"
        border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
        border.color: {
            if (innerInput.activeFocus || control.semanticFocus)
                return control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
            return control.hostWindow ? control.hostWindow.controlBorder : "#25303d"
        }

        Behavior on border.color { ColorAnimation { duration: 90 } }
    }

    RowLayout {
        anchors.fill: parent
        anchors.leftMargin: control.snap(8)
        anchors.rightMargin: control.snap(8)
        spacing: control.snap(6)

        HostPixelAlignedImage {
            id: leadIcon
            hostWindow: control.hostWindow
            visible: control.leadingIconSource.toString() !== ""
            width: control.snap(control.leadingIconSize)
            height: control.snap(control.leadingIconSize)
            sourceSize: Qt.size(control.leadingIconSize, control.leadingIconSize)
            smooth: false
            mipmap: false
            source: control.leadingIconSource
            Layout.alignment: Qt.AlignVCenter
        }

        Item {
            Layout.fillWidth: true
            Layout.fillHeight: true

            Text {
                id: placeholderLabel
                objectName: control.objectName
                            ? (control.objectName + "Placeholder")
                            : "textFieldPlaceholder"
                anchors.fill: parent
                verticalAlignment: Text.AlignVCenter
                visible: innerInput.text === "" && !innerInput.inputMethodComposing
                color: control.hostWindow ? control.hostWindow.mutedText : "#666666"
                font: innerInput.font
                elide: Text.ElideRight
                transform: Translate {
                    x: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetX(
                             placeholderLabel,
                             control.hostWindow.contentItem) : 0
                    y: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetY(
                             placeholderLabel,
                             control.hostWindow.contentItem) : 0
                }
            }

            TextInput {
                id: innerInput
                objectName: control.objectName
                            ? (control.objectName + "TextInput")
                            : "textFieldTextInput"
                anchors.fill: parent
                verticalAlignment: TextInput.AlignVCenter
                color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
                selectionColor: control.hostWindow ? control.hostWindow.selectedBg : "#2c7be5"
                selectedTextColor: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
                font: control.hostWindow ? control.hostWindow.font : Qt.font({})
                // A remote-controlled field is normally read-only because Go
                // owns editing.  Read-only TextInput still provides native
                // selection, so keep that path enabled for semantic dialog
                // fields instead of making their text impossible to select.
                selectByMouse: !control.remoteControlled || innerInput.readOnly
                clip: true
                transform: Translate {
                    x: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetX(
                             innerInput, control.hostWindow.contentItem) : 0
                    y: control.hostWindow
                       ? control.hostWindow.dialogPixelOffsetY(
                             innerInput, control.hostWindow.contentItem) : 0
                }

                Binding {
                    target: innerInput
                    property: "cursorPosition"
                    value: Math.max(0, Math.min(innerInput.length, Number(control.remoteCursorPosition || 0)))
                    when: control.remoteControlled
                }
                Binding {
                    target: innerInput
                    property: "cursorVisible"
                    value: control.remoteCursorVisible
                    when: control.remoteControlled
                }

                onAccepted: control.accepted()
                onActiveFocusChanged: {
                    if (innerInput.activeFocus)
                        control.pointerFocusRequested()
                }
                onTextEdited: {
                    ++control.cursorActivityRevision
                    control.textEdited()
                }
                onEditingFinished: control.editingFinished()
                onCursorPositionChanged: ++control.cursorActivityRevision

                cursorDelegate: Rectangle {
                    id: customCursor
                    objectName: control.objectName
                                ? (control.objectName + "Cursor")
                                : "textFieldCursor"
                    property alias blinkOn: textCursorBlinkController.blinkOn
                    property alias blinkInterval:
                        textCursorBlinkController.interval
                    readonly property bool blinkTimerRunning:
                        textCursorBlinkController.running
                    width: control.hostWindow
                           ? control.hostWindow.separatorWidth : 1
                    color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
                    opacity: blinkOn ? 1.0 : 0.0
                    function restartBlink() {
                        textCursorBlinkController.restart()
                    }
                    transform: Translate {
                        x: control.hostWindow
                           ? control.hostWindow.dialogPixelOffsetX(
                                 customCursor,
                                 control.hostWindow.contentItem) : 0
                        y: control.hostWindow
                           ? control.hostWindow.dialogPixelOffsetY(
                                 customCursor,
                                 control.hostWindow.contentItem) : 0
                    }

                    ActivityBoundedCursorBlink {
                        id: textCursorBlinkController
                        objectName: control.objectName
                                    ? (control.objectName
                                       + "CursorBlinkController")
                                    : "textFieldCursorBlinkController"
                        interval: 480
                        active: customCursor.visible
                                && control.visible
                                && innerInput.cursorVisible
                                && (innerInput.activeFocus
                                    || control.semanticFocus)
                                && (!control.hostWindow
                                    || control.hostWindow.active)
                                && !editMenu.visible
                        activityRevision:
                            control.cursorActivityRevision
                            + (control.hostWindow
                               ? control.hostWindow.keyboardActivityRevision
                               : 0)
                    }
                }
            }
        }
    }

    MouseArea {
        objectName: control.objectName
                    ? (control.objectName + "TextCursorArea")
                    : "textFieldTextCursorArea"
        anchors.fill: parent
        acceptedButtons: Qt.RightButton
        hoverEnabled: true
        // The field remains remote-controlled for semantic dialogs, but it is
        // still a text-edit surface.  Keep the native I-beam cursor while
        // hovering it instead of exposing the default arrow.
        cursorShape: Qt.IBeamCursor
        onClicked: (mouse) => {
            if (mouse.button === Qt.RightButton && !control.readOnly) {
                editMenu.popup()
            }
        }
    }

    MouseArea {
        id: leftMarginSelectionArea
        objectName: control.objectName
                    ? (control.objectName + "LeftMarginSelectionArea")
                    : "textFieldLeftMarginSelectionArea"
        x: 0
        y: 0
        width: Math.max(0, Math.min(control.width, control.textInputLeftEdge))
        height: control.height
        z: 1
        acceptedButtons: Qt.LeftButton
        hoverEnabled: true
        preventStealing: true
        cursorShape: Qt.IBeamCursor
        property bool dragging: false
        property bool dragMoved: false
        property real pressX: 0
        property real pressY: 0
        property int clickCount: 0
        property double lastClickAt: 0
        property real lastClickX: 0
        property real lastClickY: 0
        property int anchorPosition: 0
        onPressed: function(mouse) {
            control.beginMarginPress(leftMarginSelectionArea, mouse)
        }
        onPositionChanged: function(mouse) {
            control.updateMarginPress(leftMarginSelectionArea, mouse)
        }
        onReleased: function(mouse) {
            control.finishMarginPress(leftMarginSelectionArea, mouse)
            mouse.accepted = true
        }
        onCanceled: control.cancelMarginPress(leftMarginSelectionArea)
    }

    MouseArea {
        id: rightMarginSelectionArea
        objectName: control.objectName
                    ? (control.objectName + "RightMarginSelectionArea")
                    : "textFieldRightMarginSelectionArea"
        x: Math.max(0, Math.min(control.width, control.textInputRightEdge))
        y: 0
        width: Math.max(0, control.width - x)
        height: control.height
        z: 1
        acceptedButtons: Qt.LeftButton
        hoverEnabled: true
        preventStealing: true
        cursorShape: Qt.IBeamCursor
        property bool dragging: false
        property bool dragMoved: false
        property real pressX: 0
        property real pressY: 0
        property int clickCount: 0
        property double lastClickAt: 0
        property real lastClickX: 0
        property real lastClickY: 0
        property int anchorPosition: 0
        onPressed: function(mouse) {
            control.beginMarginPress(rightMarginSelectionArea, mouse)
        }
        onPositionChanged: function(mouse) {
            control.updateMarginPress(rightMarginSelectionArea, mouse)
        }
        onReleased: function(mouse) {
            control.finishMarginPress(rightMarginSelectionArea, mouse)
            mouse.accepted = true
        }
        onCanceled: control.cancelMarginPress(rightMarginSelectionArea)
    }

    T.Menu {
        id: editMenu
        T.MenuItem {
            text: qsTr("Cut")
            enabled: innerInput.selectedText.length > 0 && !innerInput.readOnly
            onTriggered: {
                innerInput.cut()
                innerInput.forceActiveFocus()
            }
        }
        T.MenuItem {
            text: qsTr("Copy")
            enabled: innerInput.selectedText.length > 0
            onTriggered: {
                innerInput.copy()
                innerInput.forceActiveFocus()
            }
        }
        T.MenuItem {
            text: qsTr("Paste")
            enabled: innerInput.canPaste && !innerInput.readOnly
            onTriggered: {
                innerInput.paste()
                innerInput.forceActiveFocus()
            }
        }
        T.MenuSeparator {}
        T.MenuItem {
            text: qsTr("Select All")
            enabled: innerInput.text.length > 0
            onTriggered: {
                innerInput.selectAll()
                innerInput.forceActiveFocus()
            }
        }
    }
}
