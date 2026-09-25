pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: fileFieldTools
    required property ApplicationWindow hostWindow
    required property Item panelItem
    required property Item panelHeader
    required property var panel
    width: 0
    height: 0

    function fileFieldDescriptor(fieldId) {
        const fields = panel.fileFieldDescriptors || []
        for (let index = 0; index < fields.length; ++index) {
            if (String(fields[index].id || "") === String(fieldId || ""))
                return fields[index]
        }
        return null
    }

    function buildSortChoices() {
        const choices = [
            { "label": "Name", "mode": "name", "icon": "arrow-down-a-z", "shortcut": "Ctrl+F3" },
            { "label": "Extension", "mode": "extension", "icon": "file-type", "shortcut": "Ctrl+F4" },
            { "label": "Time", "mode": "time", "icon": "clock-3", "shortcut": "Ctrl+F5" },
            { "label": "Size", "mode": "size", "icon": "arrow-down-wide-narrow", "shortcut": "Ctrl+F6" },
            { "label": "Unsorted", "mode": "unsorted", "icon": "list", "shortcut": "Ctrl+F7" },
            { "label": "Use sort groups", "mode": "groups", "icon": "list", "shortcut": "" },
            { "heading": true, "label": "Sort by file field" }
        ]
        const fields = panel.fileFieldDescriptors || []
        for (let index = 0; index < fields.length; ++index) {
            choices.push({
                "label": String(fields[index].title || fields[index].id),
                "mode": String(fields[index].id),
                "icon": "image", "shortcut": ""
            })
        }
        choices.push({ "heading": true, "label": "Group by file field" })
        choices.push({ "label": "No field grouping", "mode": "group-none",
                        "icon": "list", "shortcut": "" })
        for (let index = 0; index < fields.length; ++index) {
            choices.push({
                "label": String(fields[index].title || fields[index].id),
                "mode": "group:" + String(fields[index].id),
                "icon": "list-tree", "shortcut": ""
            })
        }
        if (panel.groupBy === "FileField") {
            choices.push({ "label": panel.groupReverse === true
                           ? "Ascending group order" : "Descending group order",
                           "mode": "group-reverse", "icon": "arrow-down-up",
                           "shortcut": "" })
        }
        choices.push({ "heading": true, "label": "File field tools" })
        choices.push({ "label": "Filter…", "mode": "file-field-filter",
                        "icon": "list-checks", "shortcut": "" })
        return choices
    }

    readonly property var sortChoices: buildSortChoices()

    function sortModeLabel(mode) {
        const custom = fileFieldDescriptor(mode)
        if (custom)
            return String(custom.title || custom.id)
        switch (mode) {
        case "extension": return "Extension"
        case "time": return "Time"
        case "size": return "Size"
        case "unsorted": return "Unsorted"
        default: return "Name"
        }
    }

    function openFileFieldColumns() {
        fileFieldColumnsPopup.openFileFieldColumns()
    }

    function openFileFieldFilter() {
        fileFieldFilterPopup.openFileFieldFilter()
    }

    FileFieldColumnsPopup {
        id: fileFieldColumnsPopup
        hostWindow: fileFieldTools.hostWindow
        panelItem: fileFieldTools.panelItem
        panelHeader: fileFieldTools.panelHeader
        panel: fileFieldTools.panel
    }

    FileFieldFilterPopup {
        id: fileFieldFilterPopup
        hostWindow: fileFieldTools.hostWindow
        panelItem: fileFieldTools.panelItem
        panelHeader: fileFieldTools.panelHeader
        panel: fileFieldTools.panel
    }
}
