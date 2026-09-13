import QtQuick

QtObject {
    id: palette
    property var persistence: null
    readonly property var classic: ["#000000", "#0000aa", "#00aa00", "#00aaaa",
        "#aa0000", "#aa00aa", "#aa5500", "#aaaaaa", "#555555", "#5555ff",
        "#55ff55", "#55ffff", "#ff5555", "#ff55ff", "#ffff55", "#ffffff"]
    property var modern: ["#000000", "#212421", "#68a318", "#00508c", "#800000",
        "#aaf046", "#ffdc00", "#ece9d8", "#8c8c8c", "#0082ff", "#00ff00",
        "#aff7ff", "#ff8c00", "#c864ff", "#beb9ff", "#ffffff"]
    property var colors: modern.slice()
    property string preset: "modern"
    property bool enabled: true
    property string error: ""

    function valid(values) {
        return Array.isArray(values) && values.length === 16
            && values.every(value => /^#[0-9a-fA-F]{6}$/.test(String(value)))
    }
    function load() {
        if (!persistence) return
        if (typeof persistence.modernTerminalColors === "function")
            modern = persistence.modernTerminalColors()
        const saved = persistence.loadTheme()
        let values = []
        try { values = JSON.parse(saved.terminalColors || "[]") } catch (_) {}
        colors = valid(values) ? values : modern.slice()
        preset = valid(values) ? (saved.terminalPreset || "custom") : "modern"
        enabled = saved.terminalColorsEnabled !== "false"
    }
    Component.onCompleted: load()

    function apply(values, name, active) {
        if (!valid(values)) { error = qsTr("Use #RRGGBB for every color."); return false }
        if (persistence && !persistence.saveTheme({terminalColors: JSON.stringify(values),
                terminalPreset: name, terminalColorsEnabled: String(active)})) {
            error = qsTr("Could not save terminal colors."); return false
        }
        colors = values.slice(); preset = name; enabled = active; error = ""
        return true
    }
    function resolve(run, foreground, fallback) {
        if (!enabled) return fallback
        const key = foreground ? "foregroundPaletteIndex" : "backgroundPaletteIndex"
        const index = run[key] === undefined ? -1 : Number(run[key])
        if (index < 0 || index >= 16) return fallback
        const value = colors[index]
        if (!(foreground ? run.foregroundDim : run.backgroundDim)) return value
        return Qt.rgba(parseInt(value.slice(1,3),16)/510,
                       parseInt(value.slice(3,5),16)/510, parseInt(value.slice(5,7),16)/510, 1)
    }
}
