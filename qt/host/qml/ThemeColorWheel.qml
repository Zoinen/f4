pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Canvas {
    id: colorWheel
    required property ApplicationWindow hostWindow
    required property Window editorWindow
    required property ThemeDraftModel draft
    Connections {
        target: colorWheel.draft
        function onPaintRequested() { colorWheel.requestPaint() }
    }
    objectName: "themeColorWheel"
    Layout.alignment: Qt.AlignHCenter
    Layout.preferredWidth: hostWindow.snapPx(170)
    Layout.preferredHeight: hostWindow.snapPx(170)
    transform: Translate {
        x: hostWindow.dialogPixelOffsetX(
            colorWheel,
            editorWindow.contentItem)
        y: hostWindow.dialogPixelOffsetY(
            colorWheel,
            editorWindow.contentItem)
    }

    function selectPoint(pointX, pointY) {
        const centerX = width / 2
        const centerY = height / 2
        const deltaX = pointX - centerX
        const deltaY = pointY - centerY
        const radius = Math.max(1, Math.min(width, height) / 2 - 2)
        const distance = Math.sqrt(deltaX * deltaX + deltaY * deltaY)
        let hue = Math.atan2(deltaY, deltaX) / (2 * Math.PI)
        if (hue < 0)
            hue += 1
        draft.selectedHue = hue
        draft.selectedChroma =
            Math.min(
                draft.wheelOklchChroma,
                distance / radius
                * draft.wheelOklchChroma)
        draft.applyCurrentColor()
    }

    onPaint: {
        const context = getContext("2d")
        const canvasWidth = Math.max(1, width)
        const canvasHeight = Math.max(1, height)
        const centerX = canvasWidth / 2
        const centerY = canvasHeight / 2
        const radius = Math.max(1, Math.min(canvasWidth, canvasHeight) / 2 - 2)

        context.clearRect(0, 0, canvasWidth, canvasHeight)
        const chromaSteps = 24
        const hueSteps = 180
        for (let chromaIndex = 0;
             chromaIndex < chromaSteps;
             ++chromaIndex) {
            const innerRadius = radius * chromaIndex
                                 / chromaSteps
            const outerRadius = radius
                                 * (chromaIndex + 1)
                                 / chromaSteps
            const chroma =
                (chromaIndex + 0.5) / chromaSteps
                * draft.wheelOklchChroma
            for (let sectorIndex = 0;
                 sectorIndex < hueSteps;
                 ++sectorIndex) {
                const startAngle = sectorIndex
                                   * 2 * Math.PI
                                   / hueSteps
                const endAngle = (sectorIndex + 1)
                                 * 2 * Math.PI
                                 / hueSteps
                const rgb =
                    draft.oklchDisplayRgb(
                        draft.selectedLightness,
                        chroma,
                        sectorIndex * 360 / hueSteps)
                context.fillStyle = "rgb(" +
                    Math.round(rgb[0] * 255) + "," +
                    Math.round(rgb[1] * 255) + "," +
                    Math.round(rgb[2] * 255) + ")"
                context.beginPath()
                if (innerRadius <= 0) {
                    context.moveTo(centerX, centerY)
                } else {
                    context.moveTo(
                        centerX + Math.cos(startAngle)
                        * innerRadius,
                        centerY + Math.sin(startAngle)
                        * innerRadius)
                }
                context.arc(centerX, centerY,
                            outerRadius, startAngle,
                            endAngle, false)
                if (innerRadius > 0) {
                    context.arc(centerX, centerY,
                                innerRadius, endAngle,
                                startAngle, true)
                }
                context.closePath()
                context.fill()
            }
        }

        const markerDistance = Math.min(
            1,
            draft.selectedChroma
            / draft.wheelOklchChroma)
            * radius
        const markerAngle = draft.selectedHue * 2 * Math.PI
        const markerX = centerX + Math.cos(markerAngle) * markerDistance
        const markerY = centerY + Math.sin(markerAngle) * markerDistance
        context.beginPath()
        context.arc(markerX, markerY, 6, 0, 2 * Math.PI)
        context.lineWidth = 3
        context.strokeStyle = "#80000000"
        context.stroke()
        context.beginPath()
        context.arc(markerX, markerY, 6, 0, 2 * Math.PI)
        context.lineWidth = 2
        context.strokeStyle = "#ffffff"
        context.stroke()
    }

    MouseArea {
        anchors.fill: parent
        cursorShape: Qt.CrossCursor
        onPressed: function(mouse) { colorWheel.selectPoint(mouse.x, mouse.y) }
        onPositionChanged: function(mouse) { if (pressed) colorWheel.selectPoint(mouse.x, mouse.y) }
    }
}
