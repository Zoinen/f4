pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

F4ComboBox {
    id: themeRenderCombo
    property var options: []
    property int selectedRenderType: Text.NativeRendering
    property var selectedValue: undefined
    signal renderTypeActivated(int value)
    signal optionActivated(var value)

    model: options
    textRole: "name"
    valueRole: "value"

    function syncCurrentIndex() {
        const values = themeRenderCombo.options || []
        const selected = themeRenderCombo.selectedValue !== undefined
                ? themeRenderCombo.selectedValue
                : themeRenderCombo.selectedRenderType
        let nextIndex = 0
        for (let i = 0; i < values.length; ++i) {
            const optionValue = values[i].value
            const equal = typeof selected === "number"
                    || typeof optionValue === "number"
                    ? Number(optionValue) === Number(selected)
                    : String(optionValue) === String(selected)
            if (equal) {
                nextIndex = i
                break
            }
        }
        if (themeRenderCombo.currentIndex !== nextIndex)
            themeRenderCombo.currentIndex = nextIndex
    }

    Component.onCompleted: syncCurrentIndex()
    onOptionsChanged: syncCurrentIndex()
    onSelectedRenderTypeChanged: syncCurrentIndex()
    onSelectedValueChanged: syncCurrentIndex()

    onActivated: function(index) {
        const values = themeRenderCombo.options || []
        if (index < 0 || index >= values.length)
            return
        const value = values[index].value
        themeRenderCombo.optionActivated(value)
        if (typeof value === "number")
            themeRenderCombo.renderTypeActivated(Number(value))
    }
}
