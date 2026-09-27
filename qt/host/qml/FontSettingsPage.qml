pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Item {
    id: page
    required property ApplicationWindow hostWindow
    objectName: "fontSettingsPage"
    readonly property var typography: hostWindow.typography
    property var draftBaseline: ({})
    Component.onCompleted: draftBaseline = typography.snapshot()
    property string status: ""
    readonly property var families: Qt.fontFamilies()
    readonly property var fixedFamilies: typeof f4MonospaceFontFamilies !== "undefined"
            ? f4MonospaceFontFamilies : [typography.defaultMonoFamily]
    implicitHeight: rows.implicitHeight
    function resetDraft() { typography.restore(draftBaseline); status = "" }
    function applyDraft() {
        if (!hostWindow.saveThemeToPersistence()) {
            status = qsTr("Could not save fonts")
            return false
        }
        draftBaseline = typography.snapshot()
        status = qsTr("Fonts saved")
        return true
    }
    Column {
        id: rows
        width: parent.width
        spacing: page.hostWindow.snapPx(16)
        Repeater {
            model: [qsTr("Interface"), qsTr("Panels"), qsTr("Monospace: console, viewer, editor and command line")]
            delegate: Column {
                id: group
                required property int index
                required property string modelData
                readonly property bool linked: index === 1 && page.typography.panelsLinked
                readonly property string family: index === 0 ? page.typography.effectiveUiFamily
                    : index === 1 ? page.typography.effectivePanelFamily : page.typography.effectiveMonoFamily
                readonly property int size: index === 0 ? page.typography.uiSize
                    : index === 1 ? page.typography.effectivePanelSize : page.typography.effectiveMonoSize
                width: rows.width
                spacing: page.hostWindow.snapPx(8)
                Label {
                    id: titleLabel
                    objectName: "fontSettingsTitle-" + group.index
                    text: group.modelData
                    width: parent.width
                    wrapMode: Text.Wrap
                    color: page.hostWindow.textColor
                    font: page.hostWindow.font
                    transform: Translate {
                        x: page.hostWindow.dialogPixelOffsetX(titleLabel, page.hostWindow.contentItem)
                        y: page.hostWindow.dialogPixelOffsetY(titleLabel, page.hostWindow.contentItem)
                    }
                }
                RowLayout {
                    width: parent.width
                    spacing: page.hostWindow.snapPx(8)
                    F4EditableComboBox {
                        hostWindow: page.hostWindow
                        focusPolicy: Qt.StrongFocus
                        objectName: "fontSettingsFamily-" + group.index
                        Layout.fillWidth: true
                        Layout.minimumWidth: 0
                        enabled: !group.linked
                        model: group.index === 2 ? page.fixedFamilies : page.families
                        currentIndex: model.indexOf(group.family)
                        displayText: group.family
                        font: page.hostWindow.font
                        function applyFamily() {
                            if (currentIndex < 0) {
                                currentIndex = model.indexOf(group.family)
                                editText = group.family
                                return
                            }
                            if (group.index === 0) page.typography.uiFamily = currentText
                            else if (group.index === 1) page.typography.panelFamily = currentText
                            else page.typography.monoFamily = currentText
                        }
                        onActivated: applyFamily()
                        onAccepted: applyFamily()
                    }
                    F4ComboBox {
                        hostWindow: page.hostWindow
                        focusPolicy: Qt.StrongFocus
                        objectName: "fontSettingsSize-" + group.index
                        Layout.preferredWidth: page.hostWindow.snapPx(Math.max(110, page.hostWindow.typography.uiSize * 6))
                        model: Array.from({length: 29}, (_, i) => String(i + 8))
                        currentIndex: group.size - 8
                        displayText: group.size + " px"
                        enabled: !group.linked
                        font: page.hostWindow.font
                        onActivated: {
                            const value = Number(currentText)
                            if (group.index === 0) page.typography.uiSize = value
                            else if (group.index === 1) page.typography.panelSize = value
                            else page.typography.monoSize = value
                        }
                    }
                    F4Button {
                        hostWindow: page.hostWindow
                        variant: "tool"
                        focusPolicy: Qt.StrongFocus
                        objectName: "fontSettingsReset-" + group.index
                        enabled: !group.linked
                        iconSource: page.hostWindow.lucideIconSource(
                            "rotate-ccw", 16, page.hostWindow.textColor)
                        toolTipText: qsTr("Reset font and size to platform default: %1, %2 px")
                            .arg(group.index === 2 ? page.typography.platformMonoFamily
                                                   : page.typography.defaultUiFamily)
                            .arg(group.index === 2 ? page.typography.platformMonoSize : 13)
                        onClicked: page.typography.resetFont(group.index)
                    }
                    F4Button {
                        hostWindow: page.hostWindow
                        variant: "tool"
                        focusPolicy: Qt.StrongFocus
                        objectName: "fontSettingsLink-" + group.index
                        visible: group.index === 1
                        checkable: true
                        checked: page.typography.panelsLinked
                        iconSource: page.hostWindow.lucideIconSource(
                            checked ? "lock-keyhole" : "lock-keyhole-open", 16,
                            page.hostWindow.textColor)
                        Accessible.name: qsTr("Use the interface font for panels")
                        toolTipText: Accessible.name
                        onClicked: page.typography.setLinked(!page.typography.panelsLinked)
                    }
                }
                Text {
                    id: sample
                    objectName: "fontSettingsPreview-" + group.index
                    width: parent.width
                    text: "Aa Бб 0123456789 — example.txt"
                    font.family: group.family
                    font.pixelSize: group.size
                    color: page.hostWindow.textColor
                    elide: Text.ElideRight
                    transform: Translate {
                        x: page.hostWindow.dialogPixelOffsetX(sample, page.hostWindow.contentItem)
                        y: page.hostWindow.dialogPixelOffsetY(sample, page.hostWindow.contentItem)
                    }
                }
            }
        }
    }
}
