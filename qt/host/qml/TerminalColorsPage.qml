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
    property int selectedIndex: 1
    readonly property color selectedColor: /^#[0-9a-fA-F]{6}$/.test(draft[selectedIndex]) ? draft[selectedIndex] : "#000000"
    readonly property real gap: px(12)
    readonly property bool wide: width >= px(520)
    readonly property real editorWidth: px(Math.min(wide ? 160 : 200, width))
    implicitWidth: 0
    implicitHeight: preview.y + preview.height
    onSelectedColorChanged: wheelDraft.setFromColor(selectedColor)

    function px(value) { return hostWindow.snapPx(value) }
    function resetDraft() {
        draft = paletteState.colors.slice()
        draftPreset = paletteState.preset
        draftEnabled = paletteState.enabled
        status = ""
    }
    function validateDraft() {
        const valid = paletteState.valid(draft)
        status = valid ? "" : qsTr("Use #RRGGBB for every color.")
        return valid
    }
    function choosePreset(name) {
        draft = (name === "modern" ? paletteState.modern : paletteState.classic).slice()
        draftPreset = name
        status = ""
    }
    function applyDraft() {
        if (!validateDraft())
            return false
        const saved = paletteState.apply(draft, draftPreset, draftEnabled)
        status = saved ? qsTr("Terminal colors saved.") : paletteState.error
        return saved
    }
    function setColor(index, value) {
        const next = draft.slice()
        next[index] = value
        draft = next
        draftPreset = "custom"
        status = ""
    }
    ThemeDraftModel {
        id: wheelDraft
        hostWindow: page.hostWindow
        colorSink: color => page.setColor(page.selectedIndex, color.toString())
        Component.onCompleted: setFromColor(page.selectedColor)
    }
    transform: Translate {
        x: page.hostWindow.dialogPixelOffsetX(page, page.hostWindow.contentItem)
        y: page.hostWindow.dialogPixelOffsetY(page, page.hostWindow.contentItem)
    }
    component Caption: Text {
        id: caption
        required property string identity
        objectName: identity
        color: page.hostWindow.textColor
        font.family: page.hostWindow.font.family
        font.pixelSize: page.hostWindow.semanticTextFontPixelSize
        renderType: page.hostWindow.fontRenderType
        height: page.px(implicitHeight)
        transform: Translate {
            x: page.hostWindow.dialogPixelOffsetX(caption, page.hostWindow.contentItem)
            y: page.hostWindow.dialogPixelOffsetY(caption, page.hostWindow.contentItem)
        }
    }
    Caption {
        id: description
        identity: "terminalColorsDescription"
        text: qsTr("Override the 16 console colors under the panels. RGB colors stay unchanged.")
        width: page.width
        wrapMode: Text.WordWrap
    }
    F4CheckBox {
        id: enabledControl
        objectName: "terminalColorsEnabled"
        hostWindow: page.hostWindow
        text: qsTr("Overrides")
        checked: page.draftEnabled
        focusPolicy: Qt.StrongFocus
        y: description.y + description.height + page.gap
        width: page.px(120)
        height: page.px(36)
        onToggled: { page.draftEnabled = checked; page.status = "" }
    }
    F4ComboBox {
        id: presets
        objectName: "terminalColorsPreset"
        hostWindow: page.hostWindow
        x: enabledControl.width + page.gap
        y: enabledControl.y
        width: Math.max(1, page.px(page.width - x))
        height: page.px(36)
        model: [{text: qsTr("Modern")}, {text: qsTr("Classic DOS colors")}, {text: qsTr("Custom")}]
        textRole: "text"
        currentIndex: page.draftPreset === "modern" ? 0 : page.draftPreset === "classic" ? 1 : 2
        onActivated: index => { if (index < 2) page.choosePreset(index === 0 ? "modern" : "classic") }
    }
    Item {
        id: colorList
        objectName: "terminalColorsList"
        y: enabledControl.y + enabledControl.height + page.gap
        width: page.px(page.wide ? page.width - page.editorWidth - page.gap : page.width)
        height: page.px(8 * 36 - 4)
        readonly property real columnWidth: page.px((width - page.gap) / 2)
        Repeater {
            model: 16
            delegate: Rectangle {
                id: entry
                required property int index
                objectName: "terminalColorRow" + index
                x: index < 8 ? 0 : colorList.columnWidth + page.gap
                y: page.px((index % 8) * 36)
                width: index < 8 ? colorList.columnWidth : colorList.width - x
                height: page.px(32)
                radius: page.px(4)
                color: page.selectedIndex === index ? page.hostWindow.selectedBg : "transparent"
                MouseArea { anchors.fill: parent; onClicked: page.selectedIndex = entry.index }
                Rectangle {
                    objectName: "terminalColorSwatch" + entry.index
                    x: page.px(4)
                    y: page.px(4)
                    width: page.px(24)
                    height: width
                    radius: page.px(3)
                    color: /^#[0-9a-fA-F]{6}$/.test(page.draft[entry.index]) ? page.draft[entry.index] : "transparent"
                }
                Caption {
                    identity: "terminalColorName" + entry.index
                    x: page.px(36)
                    y: page.px((entry.height - height) / 2)
                    width: Math.max(1, entry.width - x - page.px(4))
                    elide: Text.ElideRight
                    text: page.names[entry.index]
                }
            }
        }
    }
    Item {
        id: editor
        x: page.wide ? colorList.width + page.gap : 0
        y: page.wide ? colorList.y : colorList.y + colorList.height + page.gap
        width: page.editorWidth
        height: hexField.y + hexField.height
        Caption {
            id: selectedName
            identity: "terminalColorSelectedName"
            width: parent.width
            elide: Text.ElideRight
            text: page.names[page.selectedIndex]
        }
        ThemeColorWheel {
            id: wheel
            objectName: "terminalColorWheel"
            hostWindow: page.hostWindow
            editorWindow: page.hostWindow
            draft: wheelDraft
            y: selectedName.height + page.px(8)
            width: page.px(Math.min(editor.width, 170))
            height: width
            onWidthChanged: requestPaint()
        }
        Caption {
            id: lightnessLabel
            identity: "terminalColorLightnessLabel"
            text: qsTr("Lightness")
            y: wheel.y + wheel.height + page.px(8)
        }
        F4Slider {
            id: lightnessSlider
            objectName: "terminalColorLightness"
            hostWindow: page.hostWindow
            y: lightnessLabel.y + lightnessLabel.height
            width: editor.width
            height: page.px(28)
            from: 0
            to: 1
            value: wheelDraft.selectedLightness
            onMoved: { wheelDraft.selectedLightness = value; wheelDraft.applyCurrentColor() }
        }
        F4TextField {
            id: hexField
            objectName: "terminalColorHex"
            hostWindow: page.hostWindow
            y: lightnessSlider.y + lightnessSlider.height + page.px(8)
            width: editor.width
            height: page.px(36)
            text: page.draft[page.selectedIndex]
            onTextEdited: page.setColor(page.selectedIndex, text)
        }
    }
    Rectangle {
        id: preview
        objectName: "terminalColorsPreview"
        y: Math.max(colorList.y + colorList.height, editor.y + editor.height) + page.gap
        width: page.px(page.width)
        height: page.px(64)
        radius: page.px(4)
        color: page.paletteState.valid(page.draft) ? page.draft[0] : "#000000"
        Repeater {
            model: 16
            delegate: Caption {
                required property int index
                identity: "terminalColorPreview" + index
                x: page.px((index % 8 + 0.5) * preview.width / 8 - implicitWidth / 2)
                y: page.px(Math.floor(index / 8) * 32 + (32 - height) / 2)
                text: "Aa"
                color: page.paletteState.valid(page.draft) ? page.draft[index] : "white"
            }
        }
    }
}
