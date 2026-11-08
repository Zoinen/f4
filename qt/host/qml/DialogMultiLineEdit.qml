pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import F4QtHost 1.0

// Go owns the document and keyboard editing. Pointer positions are translated
// from Qt's UTF-16 offsets into logical rune offsets before reaching the owner.
Item {
    id: root

    readonly property var dialogColors: dialogColorContext.colors
    DialogColorContext {
        id: dialogColorContext
        control: root
    }
    required property ApplicationWindow hostWindow
    required property var widget
    enabled: widget.disabled !== true
    readonly property bool maskField: widget.maskChips != null
    readonly property bool showingChips: maskField && !(enabled && widget.focused === true)
    readonly property real documentHeight: showingChips ? chips.implicitHeight : editor.contentHeight
    readonly property real padding: hostWindow.snapPx(8)
    readonly property real innerWidth: Math.max(1, width - 2 * padding)
    readonly property real innerHeight: Math.max(1, height - 2 * padding)
    readonly property bool verticalOverflow: documentHeight > innerHeight
        - (!maskField && editor.contentWidth > innerWidth ? horizontal.implicitHeight : 0)
    readonly property bool horizontalOverflow: !maskField && editor.contentWidth > innerWidth
        - (verticalOverflow ? vertical.implicitWidth : 0)
    onShowingChipsChanged: {
        viewport.contentX = 0
        viewport.contentY = 0
        if (!showingChips)
            Qt.callLater(syncSelection)
    }
    function nextRune(offset) {
        const high = editor.text.charCodeAt(offset)
        const low = editor.text.charCodeAt(offset + 1)
        return offset + (high >= 0xd800 && high <= 0xdbff && low >= 0xdc00 && low <= 0xdfff ? 2 : 1)
    }
    function utf16(offset) {
        let i = 0
        for (let count = 0; count < offset && i < editor.text.length; ++count) i = nextRune(i)
        return i
    }
    function runes(offset) {
        let count = 0
        for (let i = 0; i < offset; i = nextRune(i)) ++count
        return count
    }
    function syncSelection() {
        const cursor = utf16(Number(widget.cursor || 0))
        if (widget.focused && widget.selectionActive)
            editor.select(utf16(Number(widget.selectionStart || 0)), cursor)
        else {
            editor.deselect()
            editor.cursorPosition = cursor
        }
        if (!showingChips && widget.focused && !pointer.pressed) {
            const rect = editor.positionToRectangle(cursor)
            if (rect.y < viewport.contentY) viewport.contentY = hostWindow.snapPx(rect.y)
            else if (rect.y + rect.height > viewport.contentY + viewport.height)
                viewport.contentY = hostWindow.snapPx(rect.y + rect.height - viewport.height)
            if (rect.x < viewport.contentX) viewport.contentX = hostWindow.snapPx(rect.x)
            else if (rect.x + rect.width > viewport.contentX + viewport.width)
                viewport.contentX = hostWindow.snapPx(rect.x + rect.width - viewport.width)
        }
    }
    onWidgetChanged: Qt.callLater(syncSelection)
    Component.onCompleted: Qt.callLater(syncSelection)
    Rectangle {
        anchors.fill: parent
        radius: root.hostWindow.snapPx(4)
        color: !root.enabled ? root.dialogColors.dialogBg : hover.hovered
            ? root.dialogColors.inputHoverBg : root.dialogColors.controlPressedBg
        border.width: root.hostWindow.separatorWidth
        border.color: root.enabled && root.widget.focused
            ? root.dialogColors.dialogAccent : root.dialogColors.controlBorder
    }
    HoverHandler { id: hover }
    MouseArea {
        anchors.fill: parent
        z: 1
        visible: root.showingChips
        enabled: root.enabled
        onClicked: root.hostWindow.action({target: root.widget.id, action: "control.focus"})
    }
    Flickable {
        id: viewport
        objectName: root.objectName + "Viewport"
        x: root.padding
        y: root.padding
        width: Math.max(1, root.width - 2 * root.padding - (root.maskField || vertical.visible ? vertical.width : 0))
        height: Math.max(1, root.height - 2 * root.padding - (horizontal.visible ? horizontal.height : 0))
        clip: true
        pixelAligned: true
        boundsBehavior: Flickable.StopAtBounds
        contentWidth: root.maskField ? width : Math.max(width, root.hostWindow.snapPx(editor.contentWidth + 2))
        contentHeight: Math.max(height, root.hostWindow.capacityPx(root.documentHeight))
        interactive: contentHeight > height || contentWidth > width
        DialogWheelScroll { viewport: viewport; hostWindow: root.hostWindow; rowHeight: editor.font.pixelSize }
        ScrollBar.vertical: F4ScrollBar {
            id: vertical
            objectName: root.objectName + "VerticalScroll"
            hostWindow: root.hostWindow
            policy: ScrollBar.AlwaysOn
            visible: root.verticalOverflow
            z: root.showingChips ? 2 : 0
            parent: root
            x: root.width - root.padding - width
            y: root.padding
            height: viewport.height
        }
        ScrollBar.horizontal: F4ScrollBar {
            id: horizontal
            hostWindow: root.hostWindow
            policy: ScrollBar.AlwaysOn
            visible: root.horizontalOverflow
            parent: root
            x: root.padding
            y: root.height - root.padding - height
            width: viewport.width
        }
        Flow {
            id: chips
            objectName: root.objectName + "Chips"
            visible: root.showingChips
            width: viewport.width
            spacing: root.hostWindow.snapPx(4)
            Repeater {
                model: root.maskField ? root.widget.maskChips : []
                delegate: Rectangle {
                    id: chip
                    required property string modelData
                    required property int index
                    readonly property real inset: root.hostWindow.snapPx(6)
                    width: Math.min(chips.width, root.hostWindow.capacityPx(chipLabel.implicitWidth + 2 * inset))
                    height: root.hostWindow.capacityPx(chipLabel.implicitHeight + root.hostWindow.snapPx(6))
                    radius: root.hostWindow.snapPx(4)
                    color: root.enabled ? root.dialogColors.controlBg : root.dialogColors.dialogBg
                    Text {
                        id: chipLabel
                        objectName: root.objectName + "ChipText-" + chip.index
                        x: chip.inset
                        y: root.hostWindow.centerPx(chip.height, height)
                        width: Math.max(0, chip.width - 2 * chip.inset)
                        height: root.hostWindow.capacityPx(implicitHeight)
                        text: chip.modelData.replace(/\s*\n\s*/g, " ")
                        font: root.hostWindow.font
                        textFormat: Text.PlainText
                        elide: Text.ElideRight
                        color: root.enabled ? root.hostWindow.textColor : root.hostWindow.mutedText
                        transform: Translate {
                            x: chipLabel.ScenePixelAlignment.offset.x
                            y: chipLabel.ScenePixelAlignment.offset.y
                        }
                    }
                }
            }
        }
        TextEdit {
            id: editor
            visible: !root.showingChips
            objectName: root.objectName + "TextEdit"
            width: viewport.width
            height: implicitHeight
            text: String(root.widget.text || "")
            textFormat: TextEdit.PlainText
            wrapMode: root.maskField ? TextEdit.Wrap : TextEdit.NoWrap
            readOnly: true
            persistentSelection: true
            font: root.hostWindow.font
            color: root.enabled ? root.hostWindow.textColor : root.hostWindow.mutedText
            selectionColor: root.dialogColors.selectedBg
            selectedTextColor: root.hostWindow.textColor
            cursorVisible: root.enabled && root.widget.focused === true
            transform: Translate {
                x: editor.ScenePixelAlignment.offset.x
                y: editor.ScenePixelAlignment.offset.y
            }
            cursorDelegate: Rectangle {
                id: caret
                objectName: root.objectName + "Cursor"
                width: root.hostWindow.separatorWidth
                color: root.hostWindow.textColor
                opacity: root.enabled && root.widget.focused === true
                         && editor.cursorVisible && blink.blinkOn ? 1 : 0
                ActivityBoundedCursorBlink {
                    id: blink
                    active: caret.visible && root.enabled && root.widget.focused === true
                        && root.hostWindow.active
                    activityRevision: root.hostWindow.keyboardActivityRevision + Number(root.widget.cursor || 0)
                    interval: 480
                }
                transform: Translate {
                    x: caret.ScenePixelAlignment.offset.x
                    y: caret.ScenePixelAlignment.offset.y
                }
            }
        }
        MouseArea {
            id: pointer
            enabled: !root.showingChips
            anchors.fill: parent
            preventStealing: true
            cursorShape: Qt.IBeamCursor
            property int anchor: 0
            function position(mouse) {
                const p = editor.mapFromItem(pointer, mouse.x, mouse.y)
                return editor.positionAt(p.x, p.y)
            }
            function publish(position) {
                root.hostWindow.action({ target: root.widget.id, action: "control.select",
                    anchor: root.runes(anchor), cursor: root.runes(position) })
            }
            onPressed: function(mouse) {
                anchor = mouse.modifiers & Qt.ShiftModifier
                    ? root.utf16(Number(root.widget.selectionActive ? root.widget.selectionStart : root.widget.cursor) || 0)
                    : position(mouse)
                publish(position(mouse))
            }
            onPositionChanged: function(mouse) { if (pressed) publish(position(mouse)) }
            onReleased: function(mouse) { publish(position(mouse)) }
        }
    }
}
