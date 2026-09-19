pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls

Item {
    id: page
    required property ApplicationWindow hostWindow
    required property var settings
    property var quickViewPreferences: null
    property var quickViewDraft: quickViewPreferences ? Object.assign({}, quickViewPreferences.values) : ({})
    objectName: "gallerySettingsPage"
    signal closeRequested()
    property var draft: settings ? Object.assign({}, settings.values) : ({})
    property string status: ""
    readonly property real gap: hostWindow.snapPx(12)
    readonly property real innerWidth: width - gap * 2
    implicitWidth: hostWindow.snapPx(420)
    implicitHeight: decoderColumn.y + decoderColumn.height + gap

    function px(value) { return hostWindow.snapPx(value) }
    function bytes(value) {
        return value >= 1073741824 ? (value / 1073741824).toFixed(2) + " GiB"
             : (value / 1048576).toFixed(1) + " MiB"
    }
    function change(key, value) {
        let next = Object.assign({}, draft)
        next[key] = value
        draft = next
        status = ""
    }
    function apply() {
        if (settings.apply(draft) && (!quickViewPreferences || quickViewPreferences.apply(quickViewDraft)))
            status = qsTr("Saved. A changed cache location takes effect after restart.")
        else
            status = settings.error || (quickViewPreferences ? quickViewPreferences.error : "")
    }
    Component.onCompleted: {
        if (settings) {
            settings.updateDisplay(hostWindow)
            settings.refresh()
        }
    }
    Timer {
        interval: 3000
        repeat: true
        running: page.visible && page.settings && !page.settings.busy
        onTriggered: page.settings.refresh()
    }
    Connections {
        target: page.settings
        function onCacheCleared() {
            page.status = qsTr("Cache cleared. Visible images may refill it.")
        }
    }

    component Copy: Text {
        id: copy
        required property string identity
        objectName: identity
        color: page.hostWindow.textColor
        font.family: page.hostWindow.font.family
        font.pixelSize: 13
        renderType: page.hostWindow.fontRenderType
        width: parent.width
        height: page.px(implicitHeight)
        wrapMode: Text.Wrap
        transform: Translate {
            x: page.hostWindow.dialogPixelOffsetX(copy, page.hostWindow.contentItem)
            y: page.hostWindow.dialogPixelOffsetY(copy, page.hostWindow.contentItem)
        }
    }

    Copy {
        id: heading
        identity: "gallerySettingsTitle"; text: qsTr("Gallery & cache")
        x: page.gap; y: page.gap; width: page.innerWidth
        font.pixelSize: 23; font.bold: true
    }
    Copy {
        id: intro
        identity: "gallerySettingsIntro"
        x: page.gap; y: heading.y + heading.height + page.px(6); width: page.innerWidth
        text: qsTr("Folder collages, image rendering and the decoders available in this build.")
        color: page.hostWindow.mutedText
    }
    Rectangle {
        id: usage
        x: page.gap; y: intro.y + intro.height + page.gap; width: page.innerWidth; height: page.px(102)
        radius: page.px(8); color: page.hostWindow.dialogHeaderBg
        Copy { identity: "galleryCacheUsageTitle"; x: page.gap; y: page.px(10); text: qsTr("DISK CACHE"); font.pixelSize: 11; font.bold: true }
        Copy {
            identity: "galleryCacheUsageValue"; x: page.gap; y: page.px(31); width: parent.width - page.gap * 2
            text: page.settings ? page.bytes(page.settings.diskBytes) + " / " + page.bytes(page.settings.values.diskLimitMiB * 1048576) : ""
            font.pixelSize: 19
        }
        Rectangle {
            x: page.gap; y: page.px(66); width: parent.width - page.gap * 2; height: page.px(5); radius: height / 2
            color: page.hostWindow.controlBg
            Rectangle {
                width: page.px(parent.width * Math.min(1, page.settings ? page.settings.diskBytes / (page.settings.values.diskLimitMiB * 1048576) : 0))
                height: parent.height; radius: height / 2; color: page.hostWindow.dialogAccent
            }
        }
        Copy { identity: "galleryCacheRamInfo"; x: page.gap; y: page.px(79); font.pixelSize: 11; color: page.hostWindow.mutedText; text: qsTr("256 MiB shared memory cache · 128 recent folder collages per panel") }
    }

    Copy {
        id: imageModeTitle
        identity: "galleryImageModeTitle"; text: qsTr("Image cache"); font.bold: true
        x: page.gap; y: usage.y + usage.height + page.gap; width: page.innerWidth
    }
    F4ComboBox {
        id: imageMode
        objectName: "galleryImageMode"
        hostWindow: page.hostWindow
        focusPolicy: Qt.StrongFocus
        x: page.gap; y: imageModeTitle.y + imageModeTitle.height + page.px(6); width: page.innerWidth
        model: [{text: qsTr("Off")}, {text: qsTr("On")}, {text: qsTr("Cache only")}]
        textRole: "text"
        currentIndex: Number(page.draft.imageMode || 0)
        onActivated: index => page.change("imageMode", index)
    }
    Copy {
        id: folderModeTitle
        identity: "galleryFolderModeTitle"; text: qsTr("Folder preview cache"); font.bold: true
        x: page.gap; y: imageMode.y + imageMode.height + page.gap; width: page.innerWidth
    }
    F4ComboBox {
        id: folderMode
        objectName: "galleryFolderMode"
        hostWindow: page.hostWindow
        focusPolicy: Qt.StrongFocus
        x: page.gap; y: folderModeTitle.y + folderModeTitle.height + page.px(6); width: page.innerWidth
        model: [{text: qsTr("Off")}, {text: qsTr("On")}, {text: qsTr("Cache only")}]
        textRole: "text"
        currentIndex: Number(page.draft.folderMode || 0)
        onActivated: index => page.change("folderMode", index)
    }
    Copy {
        id: policy
        identity: "galleryCachePolicy"
        x: page.gap; y: folderMode.y + folderMode.height + page.px(6); width: page.innerWidth
        color: page.hostWindow.mutedText; font.pixelSize: 11
        text: qsTr("On reuses cached data and refreshes it in the background. Cache only prevents new source reads; missing images remain empty.")
    }
    Copy { id: limitTitle; identity: "galleryCacheLimitTitle"; x: page.gap; y: policy.y + policy.height + page.gap; width: page.innerWidth; text: qsTr("Disk limit (MiB) · default 512"); font.bold: true }
    F4TextField {
        id: limit
        objectName: "galleryCacheLimitInput"
        hostWindow: page.hostWindow
        x: page.gap; y: limitTitle.y + limitTitle.height + page.px(6); width: page.innerWidth
        text: String(page.draft.diskLimitMiB || 512)
        validator: IntValidator { bottom: 64; top: 65536 }
        inputMethodHints: Qt.ImhDigitsOnly
        onTextEdited: page.change("diskLimitMiB", Number(text))
    }
    Copy { id: locationTitle; identity: "galleryCacheLocationTitle"; x: page.gap; y: limit.y + limit.height + page.gap; width: page.innerWidth; text: qsTr("Cache location · empty uses the default"); font.bold: true }
    F4TextField {
        id: location
        objectName: "galleryCacheLocationInput"
        hostWindow: page.hostWindow
        x: page.gap; y: locationTitle.y + locationTitle.height + page.px(6); width: page.innerWidth
        text: page.draft.location || ""
        placeholderText: qsTr("Default cache location")
        onTextEdited: page.change("location", text)
    }
    Copy { id: activeLocation; identity: "galleryCacheActiveLocation"; x: page.gap; y: location.y + location.height + page.px(6); width: page.innerWidth; font.pixelSize: 11; color: page.hostWindow.mutedText; text: qsTr("Active: ") + (page.settings ? page.settings.values.activeLocation : "") }
    F4Button {
        id: clear
        objectName: "galleryCacheClear"
        hostWindow: page.hostWindow
        focusPolicy: Qt.StrongFocus
        x: page.gap; y: activeLocation.y + activeLocation.height + page.gap; width: page.innerWidth
        text: qsTr("Clear image and folder caches")
        enabled: page.settings && !page.settings.busy
        onClicked: {
            page.status = qsTr("Clearing cache…")
            page.settings.clearCache()
        }
    }
    Copy { id: colorTitle; identity: "galleryTargetColorSpace"; x: page.gap; y: clear.y + clear.height + page.gap * 2; width: page.innerWidth; font.bold: true; text: qsTr("Target color space: ") + (page.settings ? page.settings.targetColorSpace : "") }
    F4CheckBox {
        id: colors
        objectName: "galleryColorConversion"
        hostWindow: page.hostWindow
        focusPolicy: Qt.StrongFocus
        x: page.gap; y: colorTitle.y + colorTitle.height + page.px(6); width: page.innerWidth
        text: qsTr("Convert images to target color space")
        checked: page.draft.convertColors === true
        onToggled: page.change("convertColors", checked)
    }
    F4CheckBox {
        id: animation
        objectName: "galleryAnimateResizing"
        hostWindow: page.hostWindow
        focusPolicy: Qt.StrongFocus
        x: page.gap; y: colors.y + colors.height + page.gap; width: page.innerWidth
        text: qsTr("Animate resizing items in layout")
        checked: page.draft.animateResizing === true
        onToggled: page.change("animateResizing", checked)
    }
    Copy { id: quickViewTitle; identity: "galleryQuickViewTitle"; x: page.gap; y: animation.y + animation.height + page.gap; width: page.innerWidth; text: qsTr("Quick View"); font.bold: true }
    F4CheckBox {
        id: builtinQuickView
        objectName: "galleryBuiltinQuickView"
        hostWindow: page.hostWindow
        x: page.gap; y: quickViewTitle.y + quickViewTitle.height + page.px(6); width: page.innerWidth
        focusPolicy: Qt.StrongFocus
        text: qsTr("Use built-in F4 viewer for Quick View")
        checked: page.quickViewDraft.useBuiltinF4Viewer === true
        onToggled: page.quickViewDraft = Object.assign({}, page.quickViewDraft, {useBuiltinF4Viewer: checked})
    }
    F4CheckBox {
        id: hoverQuickView
        objectName: "galleryHoverQuickView"
        hostWindow: page.hostWindow
        x: page.gap; y: builtinQuickView.y + builtinQuickView.height + page.gap; width: page.innerWidth
        focusPolicy: Qt.StrongFocus
        text: qsTr("Preview hovered items in Quick View")
        checked: page.quickViewDraft.previewOnHover !== false
        onToggled: page.quickViewDraft = Object.assign({}, page.quickViewDraft, {previewOnHover: checked})
    }
    F4Button {
        id: save
        objectName: "gallerySettingsApply"
        hostWindow: page.hostWindow
        focusPolicy: Qt.StrongFocus
        x: page.gap; y: hoverQuickView.y + hoverQuickView.height + page.gap; width: page.px((page.innerWidth - page.gap) / 2)
        text: qsTr("Apply settings")
        enabled: page.settings && !page.settings.busy
        onClicked: page.apply()
    }
    F4Button {
        objectName: "gallerySettingsClose"
        hostWindow: page.hostWindow
        focusPolicy: Qt.StrongFocus
        x: save.x + save.width + page.gap; y: save.y; width: save.width
        text: qsTr("Close")
        onClicked: page.closeRequested()
    }
    Copy { id: status; identity: "gallerySettingsStatus"; x: page.gap; y: save.y + save.height + page.px(6); width: page.innerWidth; color: page.hostWindow.mutedText; text: page.status || qsTr("Changes are saved with Apply. Cache location changes require a restart."); font.pixelSize: 11 }
    Copy { id: decoderHeading; identity: "galleryDecodersTitle"; x: page.gap; y: status.y + status.height + page.gap * 2; width: page.innerWidth; text: qsTr("Image decoders"); font.bold: true; font.pixelSize: 20 }
    Copy { id: decoderHelp; identity: "galleryDecodersHelp"; x: page.gap; y: decoderHeading.y + decoderHeading.height + page.px(6); width: page.innerWidth; text: qsTr("Tried in this order. Higher priority wins; later decoders provide fallbacks."); color: page.hostWindow.mutedText; font.pixelSize: 11 }
    Column {
        id: decoderColumn
        x: page.gap; y: decoderHelp.y + decoderHelp.height + page.gap; width: page.innerWidth; spacing: page.px(8)
        Repeater {
            model: page.settings ? page.settings.decoders : []
            delegate: Rectangle {
                id: decoder
                required property var modelData
                required property int index
                width: decoderColumn.width; height: formats.y + formats.height + page.gap
                radius: page.px(7); color: page.hostWindow.dialogHeaderBg
                border.color: page.hostWindow.controlBorder; border.width: page.hostWindow.separatorWidth
                Copy { identity: "galleryDecoderName-" + decoder.index; x: page.gap; y: page.gap; width: parent.width - page.gap * 2; text: String(decoder.modelData.order).padStart(2, "0") + "   " + decoder.modelData.name + "   ·   " + qsTr("priority ") + decoder.modelData.priority; font.bold: true }
                Copy { id: library; identity: "galleryDecoderLibrary-" + decoder.index; x: page.gap; y: page.px(37); width: parent.width - page.gap * 2; text: decoder.modelData.library; color: page.hostWindow.dialogAccent }
                Copy { id: formats; identity: "galleryDecoderFormats-" + decoder.index; x: page.gap; y: library.y + library.height + page.px(7); width: parent.width - page.gap * 2; text: decoder.modelData.formats.join("  ·  ").toUpperCase(); font.pixelSize: 11; color: page.hostWindow.mutedText }
            }
        }
    }
}
