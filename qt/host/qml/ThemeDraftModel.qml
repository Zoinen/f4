pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: themeDraft
    required property ApplicationWindow hostWindow
    property bool editorVisible: false
    signal paintRequested()
    visible: false
    width: 0
    height: 0

    property int selectedIndex: 0
    readonly property var currentItem: (selectedIndex >= 0 && selectedIndex < hostWindow.themeColorDefinitions.length)
                                       ? hostWindow.themeColorDefinitions[selectedIndex] : null

    // The editor uses OKLCH rather than HSL/HSV. Chroma is kept in the
    // CSS/OKLCH-compatible 0..0.4 range; values outside the sRGB gamut
    // are reduced while preserving lightness and hue.
    readonly property real maxOklchChroma: 0.4
    readonly property real wheelOklchChroma: 0.32
    property real selectedHue: 0
    property real selectedChroma: 0
    property real selectedLightness: 0.5
    property real selectedAlpha: 1.0
    property string filterQuery: ""
    property string statusToast: ""
    property bool pressFlashActive: false
    property string pressFlashProperty: ""
    property bool preserveReleaseColorOnStop: false
    readonly property color activeFlashColor: "#00ff00"

    // A hover preview deliberately stops at green. The list row decides
    // when it is allowed to leave that state (pointer exit or mouse
    // release); there is no timer-driven return anymore.
    PropertyAnimation {
        id: flashElementAnimation
        target: hostWindow
        duration: 150
        easing.type: Easing.OutQuad
        property string targetProperty: ""
        property color originalColor: "transparent"
    }

    PropertyAnimation {
        id: flashPressToActive
        target: hostWindow
        duration: 150
        to: themeDraft.activeFlashColor
        easing.type: Easing.OutQuad
    }

    PropertyAnimation {
        id: flashReleaseBack
        target: hostWindow
        duration: 150
        easing.type: Easing.InQuad
        onFinished: {
            themeDraft.finishReleaseFlash()
        }
        onStopped: {
            if (themeDraft.preserveReleaseColorOnStop) {
                themeDraft.preserveReleaseColorOnStop = false
            } else {
                themeDraft.finishReleaseFlash()
            }
        }
    }

    function finishReleaseFlash() {
        if (flashElementAnimation.targetProperty !== "") {
            hostWindow[flashElementAnimation.targetProperty] =
                    flashElementAnimation.originalColor
            flashElementAnimation.targetProperty = ""
        }
        pressFlashActive = false
        pressFlashProperty = ""
    }

    function restoreFlashTarget() {
        if (flashElementAnimation.targetProperty !== "") {
            hostWindow[flashElementAnimation.targetProperty] =
                    flashElementAnimation.originalColor
            flashElementAnimation.targetProperty = ""
        }
    }

    function colorBeforeFlash(propId) {
        if (flashElementAnimation.targetProperty === propId)
            return flashElementAnimation.originalColor
        return hostWindow[propId]
    }

    function flash(propId) {
        if (!propId || hostWindow[propId] === undefined)
            return
        // Once a row is active, hover is quiet. Only a real press may
        // temporarily mark the active row green.
        if (currentItem && currentItem.id === propId
                && !pressFlashActive)
            return
        if (pressFlashActive)
            return
        // Re-entering the same row must not restart a preview that is
        // already green (whether its animation has finished or not).
        if (flashElementAnimation.targetProperty === propId)
            return

        stopAllFlashing()
        flashElementAnimation.targetProperty = propId
        flashElementAnimation.originalColor = hostWindow[propId]
        flashElementAnimation.property = propId
        flashElementAnimation.from = hostWindow[propId]
        flashElementAnimation.to = activeFlashColor
        flashElementAnimation.restart()
    }

    function endHoverFlash(propId) {
        if (!propId || pressFlashActive
                || flashElementAnimation.targetProperty !== propId
                || flashReleaseBack.running)
            return
        if (flashElementAnimation.running)
            flashElementAnimation.stop()

        flashReleaseBack.property = propId
        flashReleaseBack.from = hostWindow[propId]
        flashReleaseBack.to = flashElementAnimation.originalColor
        flashReleaseBack.restart()
    }

    function startPressFlash(propId) {
        if (!propId || hostWindow[propId] === undefined)
            return

        if (pressFlashActive && pressFlashProperty === propId)
            return

        // If the row was already green from hover, preserve the current
        // green frame while switching ownership from hover to press. This
        // avoids a visible green -> real -> green flicker on mouse-down.
        const continuingPreview =
                flashElementAnimation.targetProperty === propId
                && !pressFlashActive
        const originalColor = continuingPreview
                ? flashElementAnimation.originalColor : hostWindow[propId]
        if (continuingPreview) {
            if (flashReleaseBack.running) {
                preserveReleaseColorOnStop = true
                flashReleaseBack.stop()
            }
            if (flashElementAnimation.running)
                flashElementAnimation.stop()
        } else {
            stopAllFlashing()
        }

        pressFlashActive = true
        pressFlashProperty = propId
        flashElementAnimation.targetProperty = propId
        flashElementAnimation.originalColor = originalColor

        flashPressToActive.property = propId
        flashPressToActive.from = hostWindow[propId]
        flashPressToActive.to = activeFlashColor
        flashPressToActive.restart()
    }

    function endPressFlash(propId) {
        if (!pressFlashActive || pressFlashProperty !== propId
                || !flashElementAnimation.targetProperty
                || flashReleaseBack.running)
            return
        if (flashPressToActive.running)
            flashPressToActive.stop()
        if (flashReleaseBack.running)
            flashReleaseBack.stop()

        const targetProp = flashElementAnimation.targetProperty
        const origClr = flashElementAnimation.originalColor

        flashReleaseBack.property = targetProp
        flashReleaseBack.from = hostWindow[targetProp]
        flashReleaseBack.to = origClr
        flashReleaseBack.restart()
    }

    function stopAllFlashing() {
        preserveReleaseColorOnStop = false
        if (flashReleaseBack.running)
            flashReleaseBack.stop()
        if (flashPressToActive.running)
            flashPressToActive.stop()
        if (flashElementAnimation.running)
            flashElementAnimation.stop()
        restoreFlashTarget()
        pressFlashActive = false
        pressFlashProperty = ""
    }

    function parseHex(text) {
        let s = String(text || "").trim()
        if (!s.startsWith("#"))
            s = "#" + s
        if (/^#[0-9A-Fa-f]{8}$/.test(s) || /^#[0-9A-Fa-f]{6}$/.test(s)
                || /^#[0-9A-Fa-f]{4}$/.test(s) || /^#[0-9A-Fa-f]{3}$/.test(s)) {
            return Qt.color(s)
        }
        return null
    }

    function clampUnit(value) {
        const numeric = Number(value)
        if (!isFinite(numeric))
            return 0
        return Math.max(0, Math.min(1, numeric))
    }

    function clampOklchChroma(value) {
        const numeric = Number(value)
        if (!isFinite(numeric))
            return 0
        return Math.max(0, Math.min(maxOklchChroma, numeric))
    }

    function signedCubeRoot(value) {
        return value < 0
                ? -Math.pow(-value, 1 / 3)
                : Math.pow(value, 1 / 3)
    }

    function srgbToLinear(value) {
        const channel = clampUnit(value)
        return channel <= 0.04045
                ? channel / 12.92
                : Math.pow((channel + 0.055) / 1.055, 2.4)
    }

    function linearToSrgb(value) {
        const channel = Number(value)
        if (!isFinite(channel))
            return 0
        return channel <= 0.0031308
                ? 12.92 * channel
                : 1.055 * Math.pow(channel, 1 / 2.4) - 0.055
    }

    function colorToOklab(colorValue) {
        const red = srgbToLinear(colorValue.r)
        const green = srgbToLinear(colorValue.g)
        const blue = srgbToLinear(colorValue.b)

        const l = signedCubeRoot(
            0.4122214708 * red + 0.5363325363 * green
            + 0.0514459929 * blue)
        const m = signedCubeRoot(
            0.2119034982 * red + 0.6806995451 * green
            + 0.1073969566 * blue)
        const s = signedCubeRoot(
            0.0883024619 * red + 0.2817188376 * green
            + 0.6299787005 * blue)

        return {
            lightness: 0.2104542553 * l + 0.7936177850 * m
                        - 0.0040720468 * s,
            a: 1.9779984951 * l - 2.4285922050 * m
               + 0.4505937099 * s,
            b: 0.0259040371 * l + 0.7827717662 * m
               - 0.8086757660 * s,
        }
    }

    function colorToOklch(colorValue) {
        const lab = colorToOklab(colorValue)
        const chroma = Math.sqrt(lab.a * lab.a + lab.b * lab.b)
        let hue = Math.atan2(lab.b, lab.a) * 180 / Math.PI
        if (hue < 0)
            hue += 360
        return {
            lightness: clampUnit(lab.lightness),
            chroma: clampOklchChroma(chroma),
            hue: chroma > 0.000001 ? hue : 0,
        }
    }

    function oklchToLinearRgb(lightness, chroma, hueDegrees) {
        const hueRadians = Number(hueDegrees) * Math.PI / 180
        const labA = Number(chroma) * Math.cos(hueRadians)
        const labB = Number(chroma) * Math.sin(hueRadians)
        const l = Number(lightness) + 0.3963377774 * labA
                                  + 0.2158037573 * labB
        const m = Number(lightness) - 0.1055613458 * labA
                                  - 0.0638541728 * labB
        const s = Number(lightness) - 0.0894841775 * labA
                                  - 1.2914855480 * labB
        const l3 = l * l * l
        const m3 = m * m * m
        const s3 = s * s * s
        return {
            r: 4.0767416621 * l3 - 3.3077115913 * m3
               + 0.2309699292 * s3,
            g: -1.2684380046 * l3 + 2.6097574011 * m3
               - 0.3413193965 * s3,
            b: -0.0041960863 * l3 - 0.7034186147 * m3
               + 1.7076147010 * s3,
        }
    }

    function isInSrgbGamut(linearRgb) {
        const epsilon = 0.000001
        return linearRgb.r >= -epsilon && linearRgb.r <= 1 + epsilon
                && linearRgb.g >= -epsilon && linearRgb.g <= 1 + epsilon
                && linearRgb.b >= -epsilon && linearRgb.b <= 1 + epsilon
    }

    function oklchColor(lightness, chroma, hueDegrees, alpha) {
        const safeLightness = clampUnit(lightness)
        let safeChroma = clampOklchChroma(chroma)
        let linearRgb = oklchToLinearRgb(safeLightness, safeChroma,
                                          hueDegrees)

        // A direct OKLCH value can fall outside sRGB. Gamut-map the
        // generated display color by reducing chroma, but keep the
        // editor's requested L/C/H coordinates untouched.
        if (!isInSrgbGamut(linearRgb)) {
            let low = 0
            let high = safeChroma
            for (let iteration = 0; iteration < 18; ++iteration) {
                const middle = (low + high) / 2
                const candidate = oklchToLinearRgb(
                    safeLightness, middle, hueDegrees)
                if (isInSrgbGamut(candidate))
                    low = middle
                else
                    high = middle
            }
            safeChroma = low
            linearRgb = oklchToLinearRgb(safeLightness, safeChroma,
                                          hueDegrees)
        }

        const opacity = alpha === undefined ? selectedAlpha
                                             : clampUnit(alpha)
        return {
            color: Qt.rgba(
                clampUnit(linearToSrgb(linearRgb.r)),
                clampUnit(linearToSrgb(linearRgb.g)),
                clampUnit(linearToSrgb(linearRgb.b)), opacity),
        }
    }

    function oklchColorValue(lightness, chroma, hueDegrees, alpha) {
        return oklchColor(lightness, chroma, hueDegrees, alpha).color
    }

    function oklchDisplayRgb(lightness, chroma, hueDegrees) {
        // The wheel is a preview, so avoid a binary-search gamut map for
        // every one of its thousands of canvas cells. Clipping here is
        // sufficient; committed colors still use oklchColor() below.
        const linearRgb = oklchToLinearRgb(lightness, chroma, hueDegrees)
        return [
            clampUnit(linearToSrgb(linearRgb.r)),
            clampUnit(linearToSrgb(linearRgb.g)),
            clampUnit(linearToSrgb(linearRgb.b)),
        ]
    }

    function setFromColor(colorValue) {
        if (!colorValue)
            return
        const oklch = colorToOklch(colorValue)
        selectedHue = oklch.hue / 360
        selectedChroma = oklch.chroma
        selectedLightness = oklch.lightness
        const alpha = Number(colorValue.a)
        selectedAlpha = isFinite(alpha) ? clampUnit(alpha) : 1.0
        paintRequested()
    }

    function setFromRgb(r, g, b, a) {
        const alpha = a !== undefined ? Math.max(0, Math.min(255, a)) / 255 : selectedAlpha
        const clr = Qt.rgba(Math.max(0, Math.min(255, r)) / 255,
                            Math.max(0, Math.min(255, g)) / 255,
                            Math.max(0, Math.min(255, b)) / 255, alpha)
        setFromColor(clr)
        applyCurrentColor()
    }

    function applyCurrentColor() {
        if (!editorVisible || !currentItem)
            return
        if (flashElementAnimation.targetProperty === currentItem.id) {
            themeDraft.stopAllFlashing()
        }
        const converted = oklchColor(selectedLightness, selectedChroma,
                                      selectedHue * 360, selectedAlpha)
        hostWindow[currentItem.id] = converted.color
        paintRequested()
    }

    function selectItem(index, shouldFlash) {
        const item = (index >= 0
                      && index < hostWindow.themeColorDefinitions.length)
                ? hostWindow.themeColorDefinitions[index] : null
        const itemColor = item ? colorBeforeFlash(item.id) : null
        selectedIndex = index
        if (currentItem) {
            setFromColor(itemColor)
            if (shouldFlash)
                flash(currentItem.id)
        }
    }
}
