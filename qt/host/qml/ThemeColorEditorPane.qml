pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.impl
import QtQuick.Layouts
import QtQuick.Shapes

ColumnLayout {
    id: themeColorEditor
    required property ApplicationWindow hostWindow
    required property Window editorWindow
    required property ThemeDraftModel draft
    objectName: "themeColorEditor"
    Layout.preferredWidth: hostWindow.snapPx(320)
    Layout.minimumWidth: hostWindow.snapPx(320)
    Layout.maximumWidth: hostWindow.snapPx(320)
    Layout.fillHeight: true
    spacing: hostWindow.snapPx(8)
    transform: Translate {
        x: hostWindow.dialogPixelOffsetX(
            themeColorEditor,
            editorWindow.contentItem)
        y: hostWindow.dialogPixelOffsetY(
            themeColorEditor,
            editorWindow.contentItem)
    }

    // Active item title & badge
    RowLayout {
        id: themeActiveColorRow
        objectName: "themeActiveColorRow"
        Layout.fillWidth: false
        Layout.preferredWidth: hostWindow.snapPx(320)
        Layout.minimumWidth: hostWindow.snapPx(320)
        Layout.maximumWidth: hostWindow.snapPx(320)
        width: hostWindow.snapPx(320)
        Layout.preferredHeight: hostWindow.snapPx(26)
        Layout.minimumHeight: hostWindow.snapPx(26)
        Layout.maximumHeight: hostWindow.snapPx(26)
        spacing: hostWindow.snapPx(8)
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeActiveColorRow,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeActiveColorRow,
                editorWindow.contentItem)
        }

        Text {
            text: draft.currentItem ? draft.currentItem.name : ""
            color: hostWindow.textColor
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: 12
            font.weight: Font.Bold
            elide: Text.ElideRight
            Layout.fillWidth: true
        }

        Rectangle {
            id: themeColorGroupBadge
            objectName: "themeColorGroupBadge"
            radius: hostWindow.snapPx(3)
            color: hostWindow.controlBg
            border.width: hostWindow.separatorWidth
            border.color: hostWindow.controlBorder
            implicitHeight: hostWindow.snapPx(18)
            implicitWidth: hostWindow.snapPx(groupText.implicitWidth + 8)
            Layout.preferredHeight: hostWindow.snapPx(18)
            Layout.preferredWidth: hostWindow.snapPx(
                groupText.implicitWidth + 8)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeColorGroupBadge,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeColorGroupBadge,
                    editorWindow.contentItem)
            }

            Text {
                id: groupText
                text: draft.currentItem ? draft.currentItem.group : ""
                color: hostWindow.mutedText
                font.pixelSize: 9
                x: hostWindow.snapPx((parent.width - width) / 2)
                y: hostWindow.snapPx((parent.height - height) / 2)
            }
        }

        // Large preview swatch
        Rectangle {
            id: themeColorPreviewSwatch
            objectName: "themeColorPreviewSwatch"
            implicitWidth: hostWindow.snapPx(26)
            implicitHeight: hostWindow.snapPx(26)
            Layout.preferredWidth: hostWindow.snapPx(26)
            Layout.preferredHeight: hostWindow.snapPx(26)
            radius: hostWindow.snapPx(4)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeColorPreviewSwatch,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeColorPreviewSwatch,
                    editorWindow.contentItem)
            }
            color: hostWindow.controlBg
            border.width: hostWindow.separatorWidth
            border.color: hostWindow.controlBorder
            clip: true

            Canvas {
                anchors.fill: parent
                onPaint: {
                    const ctx = getContext("2d")
                    const sz = 4
                    for (let x = 0; x < width; x += sz) {
                        for (let y = 0; y < height; y += sz) {
                            ctx.fillStyle = ((Math.floor(x / sz) + Math.floor(y / sz)) % 2 === 0) ? "#404b5a" : "#222c38"
                            ctx.fillRect(x, y, sz, sz)
                        }
                    }
                }
            }

            Rectangle {
                anchors.fill: parent
                color: draft.currentItem ? hostWindow[draft.currentItem.id] : "transparent"
            }
        }
    }

    // 2D OKLCH hue/chroma wheel at the current lightness.
    ThemeColorWheel {
        hostWindow: themeColorEditor.hostWindow
        editorWindow: themeColorEditor.editorWindow
        draft: themeColorEditor.draft
    }

    // HUE (OKLCH) Slider & Input
    RowLayout {
        id: themeHueRow
        objectName: "themeHueRow"
        Layout.fillWidth: true
        Layout.preferredHeight: hostWindow.snapPx(20)
        spacing: hostWindow.snapPx(6)
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeHueRow,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeHueRow,
                editorWindow.contentItem)
        }

        Text { text: "H"; color: hostWindow.mutedText; font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 11; font.weight: Font.DemiBold; Layout.preferredWidth: hostWindow.snapPx(12) }
        Slider {
            id: hueSlider
            objectName: "themeHueSlider"
            from: 0; to: 1; value: draft.selectedHue
            Layout.fillWidth: false
            Layout.preferredWidth: hostWindow.snapPx(257)
            Layout.preferredHeight: hostWindow.snapPx(18)
            implicitHeight: hostWindow.snapPx(18)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    hueSlider,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    hueSlider,
                    editorWindow.contentItem)
            }
            onMoved: { draft.selectedHue = value; draft.applyCurrentColor(); }
            background: Rectangle {
                x: hostWindow.snapPx(hueSlider.leftPadding + hueSlider.handle.width / 2); y: hostWindow.snapPx(hueSlider.topPadding)
                width: hostWindow.snapPx(hueSlider.availableWidth - hueSlider.handle.width); height: hostWindow.snapPx(hueSlider.availableHeight)
                radius: hostWindow.snapPx(height / 2); border.width: hostWindow.separatorWidth; border.color: hostWindow.controlBorder
                gradient: Gradient {
                    orientation: Gradient.Horizontal
                    GradientStop {
                        position: 0.000
                        color: draft.oklchColorValue(
                            draft.selectedLightness,
                            Math.max(0.16,
                                     draft.selectedChroma),
                            0, 1)
                    }
                    GradientStop {
                        position: 0.167
                        color: draft.oklchColorValue(
                            draft.selectedLightness,
                            Math.max(0.16,
                                     draft.selectedChroma),
                            60, 1)
                    }
                    GradientStop {
                        position: 0.333
                        color: draft.oklchColorValue(
                            draft.selectedLightness,
                            Math.max(0.16,
                                     draft.selectedChroma),
                            120, 1)
                    }
                    GradientStop {
                        position: 0.500
                        color: draft.oklchColorValue(
                            draft.selectedLightness,
                            Math.max(0.16,
                                     draft.selectedChroma),
                            180, 1)
                    }
                    GradientStop {
                        position: 0.667
                        color: draft.oklchColorValue(
                            draft.selectedLightness,
                            Math.max(0.16,
                                     draft.selectedChroma),
                            240, 1)
                    }
                    GradientStop {
                        position: 0.833
                        color: draft.oklchColorValue(
                            draft.selectedLightness,
                            Math.max(0.16,
                                     draft.selectedChroma),
                            300, 1)
                    }
                    GradientStop {
                        position: 1.000
                        color: draft.oklchColorValue(
                            draft.selectedLightness,
                            Math.max(0.16,
                                     draft.selectedChroma),
                            360, 1)
                    }
                }
            }
            handle: Rectangle {
                x: hostWindow.snapPx(hueSlider.leftPadding + hueSlider.visualPosition * (hueSlider.availableWidth - width))
                y: hostWindow.snapPx(hueSlider.topPadding + (hueSlider.availableHeight - height) / 2)
                width: hostWindow.snapPx(14); height: hostWindow.snapPx(14); radius: hostWindow.snapPx(7); color: hostWindow.controlBg; border.width: hostWindow.snapPx(2); border.color: hostWindow.textColor
            }
        }
        Rectangle {
            id: themeHueInputBox
            objectName: "themeHueInputBox"
            implicitWidth: hostWindow.snapPx(38); implicitHeight: hostWindow.snapPx(20); radius: hostWindow.snapPx(3); color: hostWindow.controlPressedBg; border.width: hostWindow.separatorWidth; border.color: hInput.activeFocus ? hostWindow.dialogAccent : hostWindow.controlBorder
            Layout.preferredWidth: hostWindow.snapPx(38)
            Layout.preferredHeight: hostWindow.snapPx(20)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeHueInputBox,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeHueInputBox,
                    editorWindow.contentItem)
            }
            TextInput {
                id: hInput
                anchors.fill: parent; anchors.leftMargin: hostWindow.snapPx(2); anchors.rightMargin: hostWindow.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 10; color: hostWindow.textColor; selectByMouse: true; selectionColor: hostWindow.selectedBg; selectedTextColor: hostWindow.textColor
                validator: IntValidator { bottom: 0; top: 360 }
                text: !activeFocus ? Math.round(draft.selectedHue * 360).toString() : text
                onTextEdited: { const val = parseInt(text); if (!isNaN(val)) { draft.selectedHue = Math.max(0, Math.min(360, val)) / 360; draft.applyCurrentColor(); } }
                onEditingFinished: { text = Math.round(draft.selectedHue * 360).toString() }
            }
        }
    }

    // CHROMA (OKLCH) Slider & Input
    RowLayout {
        id: themeChromaRow
        objectName: "themeChromaRow"
        Layout.fillWidth: true
        Layout.preferredHeight: hostWindow.snapPx(20)
        spacing: hostWindow.snapPx(6)
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeChromaRow,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeChromaRow,
                editorWindow.contentItem)
        }

        Text { text: "C"; color: hostWindow.mutedText; font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 11; font.weight: Font.DemiBold; Layout.preferredWidth: hostWindow.snapPx(12) }
        Slider {
            id: chromaSlider
            objectName: "themeChromaSlider"
            from: 0; to: draft.maxOklchChroma
            stepSize: 0.001
            value: draft.selectedChroma
            Layout.fillWidth: false
            Layout.preferredWidth: hostWindow.snapPx(257)
            Layout.preferredHeight: hostWindow.snapPx(18)
            implicitHeight: hostWindow.snapPx(18)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    chromaSlider,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    chromaSlider,
                    editorWindow.contentItem)
            }
            onMoved: { draft.selectedChroma = value; draft.applyCurrentColor(); }
            background: Rectangle {
                x: hostWindow.snapPx(chromaSlider.leftPadding + chromaSlider.handle.width / 2); y: hostWindow.snapPx(chromaSlider.topPadding)
                width: hostWindow.snapPx(chromaSlider.availableWidth - chromaSlider.handle.width); height: hostWindow.snapPx(chromaSlider.availableHeight)
                radius: hostWindow.snapPx(height / 2); border.width: hostWindow.separatorWidth; border.color: hostWindow.controlBorder
                gradient: Gradient {
                    orientation: Gradient.Horizontal
                    GradientStop {
                        position: 0
                        color: draft.oklchColorValue(
                            draft.selectedLightness,
                            0,
                            draft.selectedHue * 360,
                            1)
                    }
                    GradientStop {
                        position: 1
                        color: draft.oklchColorValue(
                            draft.selectedLightness,
                            draft.maxOklchChroma,
                            draft.selectedHue * 360,
                            1)
                    }
                }
            }
            handle: Rectangle {
                x: hostWindow.snapPx(chromaSlider.leftPadding + chromaSlider.visualPosition * (chromaSlider.availableWidth - width))
                y: hostWindow.snapPx(chromaSlider.topPadding + (chromaSlider.availableHeight - height) / 2)
                width: hostWindow.snapPx(14); height: hostWindow.snapPx(14); radius: hostWindow.snapPx(7); color: hostWindow.controlBg; border.width: hostWindow.snapPx(2); border.color: hostWindow.textColor
            }
        }
        Rectangle {
            id: themeChromaInputBox
            objectName: "themeChromaInputBox"
            implicitWidth: hostWindow.snapPx(38); implicitHeight: hostWindow.snapPx(20); radius: hostWindow.snapPx(3); color: hostWindow.controlPressedBg; border.width: hostWindow.separatorWidth; border.color: cInput.activeFocus ? hostWindow.dialogAccent : hostWindow.controlBorder
            Layout.preferredWidth: hostWindow.snapPx(38)
            Layout.preferredHeight: hostWindow.snapPx(20)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeChromaInputBox,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeChromaInputBox,
                    editorWindow.contentItem)
            }
            TextInput {
                id: cInput
                anchors.fill: parent; anchors.leftMargin: hostWindow.snapPx(2); anchors.rightMargin: hostWindow.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 10; color: hostWindow.textColor; selectByMouse: true; selectionColor: hostWindow.selectedBg; selectedTextColor: hostWindow.textColor
                validator: DoubleValidator {
                    bottom: 0
                    top: draft.maxOklchChroma
                    decimals: 3
                    notation: DoubleValidator.StandardNotation
                }
                text: !activeFocus ? draft.selectedChroma.toFixed(3) : text
                onTextEdited: {
                    const val = parseFloat(text)
                    if (!isNaN(val)) {
                        draft.selectedChroma =
                            draft.clampOklchChroma(val)
                        draft.applyCurrentColor()
                    }
                }
                onEditingFinished: {
                    text = draft.selectedChroma.toFixed(3)
                }
            }
        }
    }

    // LIGHTNESS Slider & Input
    RowLayout {
        id: themeLightnessRow
        objectName: "themeLightnessRow"
        Layout.fillWidth: true
        Layout.preferredHeight: hostWindow.snapPx(20)
        spacing: hostWindow.snapPx(6)
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeLightnessRow,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeLightnessRow,
                editorWindow.contentItem)
        }

        Text { text: "L"; color: hostWindow.mutedText; font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 11; font.weight: Font.DemiBold; Layout.preferredWidth: hostWindow.snapPx(12) }
        Slider {
            id: lightnessSlider
            objectName: "themeLightnessSlider"
            from: 0; to: 1; value: draft.selectedLightness
            Layout.fillWidth: false
            Layout.preferredWidth: hostWindow.snapPx(257)
            Layout.preferredHeight: hostWindow.snapPx(18)
            implicitHeight: hostWindow.snapPx(18)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    lightnessSlider,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    lightnessSlider,
                    editorWindow.contentItem)
            }
            onMoved: { draft.selectedLightness = value; draft.applyCurrentColor(); }
            background: Rectangle {
                x: hostWindow.snapPx(lightnessSlider.leftPadding + lightnessSlider.handle.width / 2); y: hostWindow.snapPx(lightnessSlider.topPadding)
                width: hostWindow.snapPx(lightnessSlider.availableWidth - lightnessSlider.handle.width); height: hostWindow.snapPx(lightnessSlider.availableHeight)
                radius: hostWindow.snapPx(height / 2); border.width: hostWindow.separatorWidth; border.color: hostWindow.controlBorder
                gradient: Gradient {
                    orientation: Gradient.Horizontal
                    GradientStop { position: 0; color: "#000000" }
                    GradientStop {
                        position: 0.5
                        color: draft.oklchColorValue(
                            0.5,
                            draft.selectedChroma,
                            draft.selectedHue * 360,
                            1)
                    }
                    GradientStop { position: 1; color: "#ffffff" }
                }
            }
            handle: Rectangle {
                x: hostWindow.snapPx(lightnessSlider.leftPadding + lightnessSlider.visualPosition * (lightnessSlider.availableWidth - width))
                y: hostWindow.snapPx(lightnessSlider.topPadding + (lightnessSlider.availableHeight - height) / 2)
                width: hostWindow.snapPx(14); height: hostWindow.snapPx(14); radius: hostWindow.snapPx(7); color: hostWindow.controlBg; border.width: hostWindow.snapPx(2); border.color: hostWindow.textColor
            }
        }
        Rectangle {
            id: themeLightnessInputBox
            objectName: "themeLightnessInputBox"
            implicitWidth: hostWindow.snapPx(38); implicitHeight: hostWindow.snapPx(20); radius: hostWindow.snapPx(3); color: hostWindow.controlPressedBg; border.width: hostWindow.separatorWidth; border.color: lInput.activeFocus ? hostWindow.dialogAccent : hostWindow.controlBorder
            Layout.preferredWidth: hostWindow.snapPx(38)
            Layout.preferredHeight: hostWindow.snapPx(20)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeLightnessInputBox,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeLightnessInputBox,
                    editorWindow.contentItem)
            }
            TextInput {
                id: lInput
                anchors.fill: parent; anchors.leftMargin: hostWindow.snapPx(2); anchors.rightMargin: hostWindow.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 10; color: hostWindow.textColor; selectByMouse: true; selectionColor: hostWindow.selectedBg; selectedTextColor: hostWindow.textColor
                validator: IntValidator { bottom: 0; top: 100 }
                text: !activeFocus ? Math.round(draft.selectedLightness * 100).toString() : text
                onTextEdited: { const val = parseInt(text); if (!isNaN(val)) { draft.selectedLightness = Math.max(0, Math.min(100, val)) / 100; draft.applyCurrentColor(); } }
                onEditingFinished: { text = Math.round(draft.selectedLightness * 100).toString() }
            }
        }
    }

    // ALPHA Slider & Input
    RowLayout {
        id: themeAlphaRow
        objectName: "themeAlphaRow"
        Layout.fillWidth: true
        Layout.preferredHeight: hostWindow.snapPx(20)
        spacing: hostWindow.snapPx(6)
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeAlphaRow,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeAlphaRow,
                editorWindow.contentItem)
        }

        Text { text: "A"; color: hostWindow.mutedText; font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 11; font.weight: Font.DemiBold; Layout.preferredWidth: hostWindow.snapPx(12) }
        Slider {
            id: alphaSlider
            objectName: "themeAlphaSlider"
            from: 0; to: 1; value: draft.selectedAlpha
            Layout.fillWidth: false
            Layout.preferredWidth: hostWindow.snapPx(257)
            Layout.preferredHeight: hostWindow.snapPx(18)
            implicitHeight: hostWindow.snapPx(18)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    alphaSlider,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    alphaSlider,
                    editorWindow.contentItem)
            }
            onMoved: { draft.selectedAlpha = value; draft.applyCurrentColor(); }
            background: Rectangle {
                x: hostWindow.snapPx(alphaSlider.leftPadding + alphaSlider.handle.width / 2); y: hostWindow.snapPx(alphaSlider.topPadding)
                width: hostWindow.snapPx(alphaSlider.availableWidth - alphaSlider.handle.width); height: hostWindow.snapPx(alphaSlider.availableHeight)
                radius: hostWindow.snapPx(height / 2); border.width: hostWindow.separatorWidth; border.color: hostWindow.controlBorder
                clip: true

                Canvas {
                    anchors.fill: parent
                    onPaint: {
                        const ctx = getContext("2d")
                        const sz = 3
                        for (let x = 0; x < width; x += sz) {
                            for (let y = 0; y < height; y += sz) {
                                ctx.fillStyle = ((Math.floor(x / sz) + Math.floor(y / sz)) % 2 === 0) ? "#404b5a" : "#222c38"
                                ctx.fillRect(x, y, sz, sz)
                            }
                        }
                    }
                }

                Rectangle {
                    anchors.fill: parent
                    radius: hostWindow.snapPx(height / 2)
                    gradient: Gradient {
                        orientation: Gradient.Horizontal
                        GradientStop {
                            position: 0
                            color: draft.oklchColorValue(
                                draft.selectedLightness,
                                draft.selectedChroma,
                                draft.selectedHue * 360,
                                0)
                        }
                        GradientStop {
                            position: 1
                            color: draft.oklchColorValue(
                                draft.selectedLightness,
                                draft.selectedChroma,
                                draft.selectedHue * 360,
                                1)
                        }
                    }
                }
            }
            handle: Rectangle {
                x: hostWindow.snapPx(alphaSlider.leftPadding + alphaSlider.visualPosition * (alphaSlider.availableWidth - width))
                y: hostWindow.snapPx(alphaSlider.topPadding + (alphaSlider.availableHeight - height) / 2)
                width: hostWindow.snapPx(14); height: hostWindow.snapPx(14); radius: hostWindow.snapPx(7); color: hostWindow.controlBg; border.width: hostWindow.snapPx(2); border.color: hostWindow.textColor
            }
        }
        Rectangle {
            id: themeAlphaInputBox
            objectName: "themeAlphaInputBox"
            implicitWidth: hostWindow.snapPx(38); implicitHeight: hostWindow.snapPx(20); radius: hostWindow.snapPx(3); color: hostWindow.controlPressedBg; border.width: hostWindow.separatorWidth; border.color: aInput.activeFocus ? hostWindow.dialogAccent : hostWindow.controlBorder
            Layout.preferredWidth: hostWindow.snapPx(38)
            Layout.preferredHeight: hostWindow.snapPx(20)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeAlphaInputBox,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeAlphaInputBox,
                    editorWindow.contentItem)
            }
            TextInput {
                id: aInput
                anchors.fill: parent; anchors.leftMargin: hostWindow.snapPx(2); anchors.rightMargin: hostWindow.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 10; color: hostWindow.textColor; selectByMouse: true; selectionColor: hostWindow.selectedBg; selectedTextColor: hostWindow.textColor
                validator: IntValidator { bottom: 0; top: 100 }
                text: !activeFocus ? Math.round(draft.selectedAlpha * 100).toString() : text
                onTextEdited: { const val = parseInt(text); if (!isNaN(val)) { draft.selectedAlpha = Math.max(0, Math.min(100, val)) / 100; draft.applyCurrentColor(); } }
                onEditingFinished: { text = Math.round(draft.selectedAlpha * 100).toString() }
            }
        }
    }

    // RGB & HEX Row
    RowLayout {
        id: themeRgbHexRow
        objectName: "themeRgbHexRow"
        Layout.fillWidth: true
        Layout.preferredHeight: hostWindow.snapPx(20)
        spacing: hostWindow.snapPx(6)
        transform: Translate {
            x: hostWindow.dialogPixelOffsetX(
                themeRgbHexRow,
                editorWindow.contentItem)
            y: hostWindow.dialogPixelOffsetY(
                themeRgbHexRow,
                editorWindow.contentItem)
        }

        // R
        Text { id: themeRedLabel; text: "R:"; color: hostWindow.mutedText; font.pixelSize: 10 }
        Rectangle {
            id: themeRedInputBox
            objectName: "themeRedInputBox"
            Layout.fillWidth: false; Layout.preferredWidth: hostWindow.snapPx(34); implicitHeight: hostWindow.snapPx(20); radius: hostWindow.snapPx(3); color: hostWindow.controlPressedBg; border.width: hostWindow.separatorWidth; border.color: rInput.activeFocus ? hostWindow.dialogAccent : hostWindow.controlBorder
            Layout.preferredHeight: hostWindow.snapPx(20)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeRedInputBox,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeRedInputBox,
                    editorWindow.contentItem)
            }
            TextInput {
                id: rInput
                anchors.fill: parent; anchors.leftMargin: hostWindow.snapPx(2); anchors.rightMargin: hostWindow.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 10; color: hostWindow.textColor; selectByMouse: true; selectionColor: hostWindow.selectedBg; selectedTextColor: hostWindow.textColor
                validator: IntValidator { bottom: 0; top: 255 }
                text: !activeFocus && draft.currentItem ? Math.round(hostWindow[draft.currentItem.id].r * 255).toString() : text
                onTextEdited: {
                    const val = parseInt(text)
                    if (!isNaN(val) && draft.currentItem) {
                        draft.setFromRgb(val, Math.round(hostWindow[draft.currentItem.id].g * 255), Math.round(hostWindow[draft.currentItem.id].b * 255))
                    }
                }
                onEditingFinished: { if (draft.currentItem) text = Math.round(hostWindow[draft.currentItem.id].r * 255).toString() }
            }
        }

        // G
        Text { id: themeGreenLabel; text: "G:"; color: hostWindow.mutedText; font.pixelSize: 10 }
        Rectangle {
            id: themeGreenInputBox
            objectName: "themeGreenInputBox"
            Layout.fillWidth: false; Layout.preferredWidth: hostWindow.snapPx(34); implicitHeight: hostWindow.snapPx(20); radius: hostWindow.snapPx(3); color: hostWindow.controlPressedBg; border.width: hostWindow.separatorWidth; border.color: gInput.activeFocus ? hostWindow.dialogAccent : hostWindow.controlBorder
            Layout.preferredHeight: hostWindow.snapPx(20)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeGreenInputBox,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeGreenInputBox,
                    editorWindow.contentItem)
            }
            TextInput {
                id: gInput
                anchors.fill: parent; anchors.leftMargin: hostWindow.snapPx(2); anchors.rightMargin: hostWindow.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 10; color: hostWindow.textColor; selectByMouse: true; selectionColor: hostWindow.selectedBg; selectedTextColor: hostWindow.textColor
                validator: IntValidator { bottom: 0; top: 255 }
                text: !activeFocus && draft.currentItem ? Math.round(hostWindow[draft.currentItem.id].g * 255).toString() : text
                onTextEdited: {
                    const val = parseInt(text)
                    if (!isNaN(val) && draft.currentItem) {
                        draft.setFromRgb(Math.round(hostWindow[draft.currentItem.id].r * 255), val, Math.round(hostWindow[draft.currentItem.id].b * 255))
                    }
                }
                onEditingFinished: { if (draft.currentItem) text = Math.round(hostWindow[draft.currentItem.id].g * 255).toString() }
            }
        }

        // B
        Text { id: themeBlueLabel; text: "B:"; color: hostWindow.mutedText; font.pixelSize: 10 }
        Rectangle {
            id: themeBlueInputBox
            objectName: "themeBlueInputBox"
            Layout.fillWidth: false; Layout.preferredWidth: hostWindow.snapPx(34); implicitHeight: hostWindow.snapPx(20); radius: hostWindow.snapPx(3); color: hostWindow.controlPressedBg; border.width: hostWindow.separatorWidth; border.color: bInput.activeFocus ? hostWindow.dialogAccent : hostWindow.controlBorder
            Layout.preferredHeight: hostWindow.snapPx(20)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeBlueInputBox,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeBlueInputBox,
                    editorWindow.contentItem)
            }
            TextInput {
                id: bInput
                anchors.fill: parent; anchors.leftMargin: hostWindow.snapPx(2); anchors.rightMargin: hostWindow.snapPx(2); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 10; color: hostWindow.textColor; selectByMouse: true; selectionColor: hostWindow.selectedBg; selectedTextColor: hostWindow.textColor
                validator: IntValidator { bottom: 0; top: 255 }
                text: !activeFocus && draft.currentItem ? Math.round(hostWindow[draft.currentItem.id].b * 255).toString() : text
                onTextEdited: {
                    const val = parseInt(text)
                    if (!isNaN(val) && draft.currentItem) {
                        draft.setFromRgb(Math.round(hostWindow[draft.currentItem.id].r * 255), Math.round(hostWindow[draft.currentItem.id].g * 255), val)
                    }
                }
                onEditingFinished: { if (draft.currentItem) text = Math.round(hostWindow[draft.currentItem.id].b * 255).toString() }
            }
        }

        // HEX
        Text { id: themeHexLabel; text: "HEX:"; color: hostWindow.mutedText; font.pixelSize: 10 }
        Rectangle {
            id: themeHexInputBox
            objectName: "themeHexInputBox"
            Layout.preferredWidth: hostWindow.snapPx(74); implicitHeight: hostWindow.snapPx(20); radius: hostWindow.snapPx(3); color: hostWindow.controlPressedBg; border.width: hostWindow.separatorWidth; border.color: hexInput.activeFocus ? hostWindow.dialogAccent : hostWindow.controlBorder
            Layout.preferredHeight: hostWindow.snapPx(20)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(
                    themeHexInputBox,
                    editorWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(
                    themeHexInputBox,
                    editorWindow.contentItem)
            }
            TextInput {
                id: hexInput
                anchors.fill: parent; anchors.leftMargin: hostWindow.snapPx(4); anchors.rightMargin: hostWindow.snapPx(4); verticalAlignment: TextInput.AlignVCenter; horizontalAlignment: TextInput.AlignHCenter
                font.family: hostWindow.guiMonospaceFontFamily; font.pixelSize: 10; color: hostWindow.textColor; selectByMouse: true; selectionColor: hostWindow.selectedBg; selectedTextColor: hostWindow.textColor
                maximumLength: 9
                text: !activeFocus && draft.currentItem ? hostWindow.formatColorHex(hostWindow[draft.currentItem.id]) : text
                onTextEdited: {
                    const c = draft.parseHex(text)
                    if (c && draft.currentItem) {
                        draft.setFromColor(c)
                        draft.applyCurrentColor()
                    }
                }
                onEditingFinished: { if (draft.currentItem) text = hostWindow.formatColorHex(hostWindow[draft.currentItem.id]) }
            }
        }
    }

    Item {
        Layout.fillHeight: true
    }
}
