pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

QtObject {
    required property ApplicationWindow hostWindow

    function fontRenderTypeOption(value) {
        const options = hostWindow.fontRenderTypeOptions || []
        for (let index = 0; index < options.length; ++index) {
            if (Number(options[index].value) === Number(value))
                return options[index]
        }
        return options.length > 0 ? options[0] : ({})
    }

    function mouseWheelModeOption(value) {
        const options = hostWindow.mouseWheelModeOptions || []
        for (let index = 0; index < options.length; ++index) {
            if (String(options[index].value) === String(value))
                return options[index]
        }
        return options.length > 0 ? options[options.length - 1] : ({})
    }

    function iconSetOption(value) {
        const options = hostWindow.iconSetOptions || []
        for (let index = 0; index < options.length; ++index) {
            if (String(options[index].value) === String(value))
                return options[index]
        }
        return options.length > 0 ? options[0] : ({})
    }

    function setMouseWheelMode(value) {
        const normalized = String(value || "").toLowerCase()
        for (let index = 0; index < hostWindow.mouseWheelModeOptions.length; ++index) {
            if (String(hostWindow.mouseWheelModeOptions[index].value) === normalized) {
                hostWindow.mouseWheelMode = normalized
                return true
            }
        }
        return false
    }

    function setIconSet(value) {
        const normalized = String(value || "").trim().toLowerCase()
        for (let index = 0; index < hostWindow.iconSetOptions.length; ++index) {
            if (String(hostWindow.iconSetOptions[index].value) !== normalized)
                continue
            if (!hostWindow.iconProvider || hostWindow.iconProvider.name === undefined)
                return false
            hostWindow.iconProvider.name = normalized
            return hostWindow.iconSetName === normalized
        }
        return false
    }
}
