pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

QtObject {
    id: context
    required property Item control
    property ApplicationWindow hostWindow: control.hostWindow || null
    // Detached popups inherit from their registered owning field.
    property Item anchor: null
    readonly property var inherited: {
        for (let item = anchor || control.parent; item; item = item.parent) {
            if (item.dialogColors !== undefined && item.dialogColors !== null)
                return item.dialogColors
        }
        return null
    }
    // The unscoped host already supplies the original palette. A dialog owns
    // one derived palette shared by all descendants, not one per control.
    readonly property var colors: inherited || hostWindow
    function tone(value) {
        return colors && typeof colors.tone === "function" ? colors.tone(value) : value
    }
}
