pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls

Item {
    id: page
    required property ApplicationWindow hostWindow
    objectName: "terminalColorsPage"
    readonly property var paletteState: hostWindow.terminalPalette
    property var draft: paletteState.colors.slice()
    property string draftPreset: paletteState.preset
    property bool draftEnabled: paletteState.enabled
    property string status: ""
    readonly property var names: [qsTr("Black"), qsTr("Blue"), qsTr("Green"), qsTr("Cyan"),
        qsTr("Red"), qsTr("Magenta"), qsTr("Brown / yellow"), qsTr("Light gray"),
        qsTr("Dark gray"), qsTr("Bright blue"), qsTr("Bright green"), qsTr("Bright cyan"),
        qsTr("Bright red"), qsTr("Bright magenta"), qsTr("Bright yellow"), qsTr("White")]
    implicitWidth: 0
    implicitHeight: hostWindow.snapPx(500)
    property int selectedIndex: 1
    readonly property color selectedColor: /^#[0-9a-fA-F]{6}$/.test(draft[selectedIndex]) ? draft[selectedIndex] : "#000000"
    readonly property real margin: hostWindow.snapPx(12)
    readonly property real editorWidth: hostWindow.snapPx(Math.min(240, width * 0.42))
    onSelectedColorChanged: wheelDraft.setFromColor(selectedColor)
    ThemeDraftModel {
        id: wheelDraft
        hostWindow: page.hostWindow
        colorSink: color => page.setColor(page.selectedIndex, color.toString())
        Component.onCompleted: setFromColor(page.selectedColor)
    }
    function resetDraft() {
        draft = paletteState.colors.slice(); draftPreset = paletteState.preset
        draftEnabled = paletteState.enabled; status = ""
    }
    function choosePreset(name) {
        draft = (name === "modern" ? paletteState.modern : paletteState.classic).slice()
        draftPreset = name; status = ""
    }
    function applyDraft() {
        status = paletteState.apply(draft, draftPreset, draftEnabled)
            ? qsTr("Terminal colors saved.") : paletteState.error
    }
    function setColor(index, value) {
        const next = draft.slice(); next[index] = value
        draft = next; draftPreset = "custom"; status = ""
    }
    // Correct the complete page origin, then keep every child on snapped
    // offsets. No automatic layout centering is used for text leaves.
    transform: Translate {
        x: {
            const dependency = page.x + page.y + page.width + page.height + page.hostWindow.width
            const p = page.parent ? page.parent.mapToItem(page.hostWindow.contentItem, page.x, page.y) : Qt.point(0,0)
            return page.hostWindow.snapPx(p.x) - p.x
        }
        y: {
            const dependency = page.x + page.y + page.width + page.height + page.hostWindow.height
            const p = page.parent ? page.parent.mapToItem(page.hostWindow.contentItem, page.x, page.y) : Qt.point(0,0)
            return page.hostWindow.snapPx(p.y) - p.y
        }
    }
    component Caption: Text {
        required property string identity
        objectName: identity
        color: page.hostWindow.textColor
        font.family: page.hostWindow.font.family
        font.pixelSize: page.hostWindow.semanticTextFontPixelSize
        renderType: page.hostWindow.fontRenderType
        height: page.hostWindow.snapPx(24)
    }
    component ActionButton: Rectangle {
        id: button
        required property string label
        required property string identity
        property bool selected: false
        signal clicked()
        objectName: identity
        width: page.hostWindow.snapPx(180)
        height: page.hostWindow.snapPx(36)
        color: mouse.containsMouse ? page.hostWindow.selectedBg : page.hostWindow.commandLineBg
        border.color: selected || activeFocus ? page.hostWindow.selectedBg : page.hostWindow.separatorColor
        border.width: page.hostWindow.separatorWidth
        radius: page.hostWindow.snapPx(4)
        activeFocusOnTab: true
        Accessible.role: Accessible.Button
        Accessible.name: label
        Accessible.onPressAction: clicked()
        Keys.onSpacePressed: clicked()
        Keys.onReturnPressed: clicked()
        Caption {
            identity: button.identity + "Text"
            text: button.label
            x: page.hostWindow.snapPx(10); y: page.hostWindow.snapPx(6)
        }
        MouseArea { id: mouse; anchors.fill: parent; hoverEnabled: true; onClicked: button.clicked() }
    }
    Caption {
        identity: "terminalColorsTitle"; text: qsTr("Terminal colors")
        x: page.hostWindow.snapPx(16); y: page.hostWindow.snapPx(12)
        font.bold: true
    }
    Caption {
        identity: "terminalColorsDescription"
        text: qsTr("Override the 16 console colors under the panels. RGB colors stay unchanged.")
        x: page.hostWindow.snapPx(16); y: page.hostWindow.snapPx(44)
        width: page.width - page.hostWindow.snapPx(32)
        wrapMode: Text.WordWrap; height: page.hostWindow.snapPx(48)
    }
    ActionButton {
        identity: "terminalColorsEnabled"; label: page.draftEnabled ? qsTr("Overrides: On") : qsTr("Overrides: Off")
        x: page.hostWindow.snapPx(16); y: page.hostWindow.snapPx(96)
        onClicked: page.draftEnabled = !page.draftEnabled
    }
    F4ComboBox {
        id: presets
        objectName: "terminalColorsPreset"
        hostWindow: page.hostWindow
        x: page.hostWindow.snapPx(208); y: page.hostWindow.snapPx(96)
        width: Math.max(1, page.width - x - page.margin)
        height: page.hostWindow.snapPx(36)
        model: [{text: qsTr("Modern")}, {text: qsTr("Classic DOS colors")}, {text: qsTr("Custom")}]
        textRole: "text"
        currentIndex: page.draftPreset === "modern" ? 0 : page.draftPreset === "classic" ? 1 : 2
        onActivated: index => { if (index < 2) page.choosePreset(index === 0 ? "modern" : "classic") }
    }
    Flickable {
        id: colorList
        objectName: "terminalColorsList"
        x: page.margin; y: page.hostWindow.snapPx(144)
        width: Math.max(1, page.width - page.editorWidth - 3 * page.margin)
        height: Math.max(1, footer.y - y - page.margin)
        contentHeight: page.hostWindow.snapPx(16 * 44)
        clip: true
        pixelAligned: true
        boundsBehavior: Flickable.StopAtBounds
        ScrollBar.vertical: F4ScrollBar { hostWindow: page.hostWindow; policy: ScrollBar.AsNeeded }
        Repeater {
            model: 16
            delegate: Rectangle {
                id: entry
                required property int index
                objectName: "terminalColorRow" + index
                y: page.hostWindow.snapPx(index * 44)
                width: colorList.width; height: page.hostWindow.snapPx(40)
                color: page.selectedIndex === index ? page.hostWindow.selectedBg : "transparent"
                MouseArea { anchors.fill: parent; onClicked: page.selectedIndex = entry.index }
                Rectangle {
                    x: page.hostWindow.snapPx(4); y: page.hostWindow.snapPx(8)
                    width: page.hostWindow.snapPx(24); height: width
                    color: /^#[0-9a-fA-F]{6}$/.test(page.draft[entry.index]) ? page.draft[entry.index] : "transparent"
                    border.width: page.hostWindow.separatorWidth; border.color: page.hostWindow.separatorColor
                }
                Caption {
                    identity: "terminalColorName" + entry.index
                    x: page.hostWindow.snapPx(36); y: page.hostWindow.snapPx(8)
                    width: Math.max(1, entry.width - x - page.margin)
                    elide: Text.ElideRight
                    text: page.names[entry.index]
                }
            }
        }
    }
    Item {
        id: editor
        x: page.hostWindow.snapPx(page.width - page.editorWidth - page.margin)
        y: colorList.y
        width: page.editorWidth
        Caption {
            identity: "terminalColorSelectedName"
            width: parent.width; elide: Text.ElideRight
            text: page.names[page.selectedIndex]
        }
        ThemeColorWheel {
            objectName: "terminalColorWheel"
            hostWindow: page.hostWindow
            editorWindow: page.hostWindow
            draft: wheelDraft
            y: page.hostWindow.snapPx(32)
            width: page.hostWindow.snapPx(Math.min(editor.width, Math.max(80, colorList.height - 150)))
            height: width
            onWidthChanged: requestPaint()
        }
        Caption {
            id: lightnessLabel
            identity: "terminalColorLightnessLabel"
            text: qsTr("Lightness")
            y: page.hostWindow.snapPx(40 + Math.min(editor.width, Math.max(80, colorList.height - 150)))
        }
        F4Slider {
            objectName: "terminalColorLightness"
            hostWindow: page.hostWindow
            y: lightnessLabel.y + page.hostWindow.snapPx(24)
            width: editor.width
            from: 0; to: 1; value: wheelDraft.selectedLightness
            onMoved: { wheelDraft.selectedLightness = value; wheelDraft.applyCurrentColor() }
        }
        F4TextField {
            objectName: "terminalColorHex"
            hostWindow: page.hostWindow
            y: lightnessLabel.y + page.hostWindow.snapPx(52)
            width: editor.width; height: page.hostWindow.snapPx(36)
            text: page.draft[page.selectedIndex]
            onTextEdited: page.setColor(page.selectedIndex, text)
        }
    }
    Item {
        id: footer
        x: page.margin
        y: page.hostWindow.snapPx(page.height - 112)
        width: page.width - 2 * page.margin
        Rectangle {
            width: parent.width; height: page.hostWindow.snapPx(32)
            color: page.paletteState.valid(page.draft) ? page.draft[0] : "#000000"
            Repeater {
                model: 16
                delegate: Caption {
                    required property int index
                    identity: "terminalColorPreview" + index
                    x: page.hostWindow.snapPx(index * footer.width / 16)
                    y: page.hostWindow.snapPx(4)
                    width: page.hostWindow.snapPx(footer.width / 16)
                    clip: true
                    text: "Aa"
                    color: page.paletteState.valid(page.draft) ? page.draft[index] : "white"
                }
            }
        }
        ActionButton {
            identity: "terminalColorsApply"; label: qsTr("Apply")
            y: page.hostWindow.snapPx(40)
            width: page.hostWindow.snapPx(Math.min(180, (footer.width - 12) / 2))
            onClicked: page.applyDraft()
        }
        ActionButton {
            identity: "terminalColorsCancel"; label: qsTr("Cancel changes")
            x: page.hostWindow.snapPx(Math.min(180, (footer.width - 12) / 2) + 12)
            y: page.hostWindow.snapPx(40)
            width: page.hostWindow.snapPx(Math.min(180, (footer.width - 12) / 2))
            onClicked: page.resetDraft()
        }
        Caption {
            identity: "terminalColorsStatus"; text: page.status
            y: page.hostWindow.snapPx(84)
            width: parent.width; elide: Text.ElideRight
        }
    }
}
