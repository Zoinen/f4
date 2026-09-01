pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl
import QtQuick.Layouts

T.ComboBox {
    id: themeRenderCombo
    required property ApplicationWindow hostWindow
    property var options: []
    property int selectedRenderType: Text.NativeRendering
    // The component is also used by non-rendering theme preferences. Keep
    // selectedRenderType/renderTypeActivated for the existing font
    // control, while allowing arbitrary string-valued options here.
    property var selectedValue: undefined
    signal renderTypeActivated(int value)
    signal optionActivated(var value)
    readonly property string popupObjectNamePrefix:
        themeRenderCombo.objectName === "themeFontRenderTypeCombo"
        ? "themeFontRenderType" : themeRenderCombo.objectName

    focusPolicy: Qt.NoFocus
    hoverEnabled: true
    padding: 0
    implicitHeight: hostWindow.snapPx(30)
    Layout.preferredHeight: hostWindow.snapPx(30)
    Layout.minimumHeight: hostWindow.snapPx(30)
    Layout.maximumHeight: hostWindow.snapPx(30)
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

    contentItem: Text {
        leftPadding: hostWindow.snapPx(10)
        rightPadding: hostWindow.snapPx(30)
        text: themeRenderCombo.currentText
        color: hostWindow.textColor
        font.family: hostWindow.guiMonospaceFontFamily
        font.pixelSize: 11
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }

    indicator: IconLabel {
        objectName: themeRenderCombo.objectName + "Indicator"
        readonly property url rasterizedIconSource:
            hostWindow.lucideIconSource(
                "chevron-down", 14,
                themeRenderCombo.enabled ? hostWindow.textColor : hostWindow.mutedText)
        x: hostWindow.snapPx(themeRenderCombo.width - width - 10)
        y: hostWindow.snapPx((themeRenderCombo.height - height) / 2)
        width: hostWindow.snapPx(14)
        height: hostWindow.snapPx(14)
        icon.source: rasterizedIconSource
        icon.width: hostWindow.snapPx(14)
        icon.height: hostWindow.snapPx(14)
        icon.color: themeRenderCombo.hovered ? hostWindow.textColor : hostWindow.mutedText
    }

    background: Rectangle {
        objectName: themeRenderCombo.objectName + "Background"
        readonly property color testBorderColor: border.color
        radius: hostWindow.snapPx(4)
        color: themeRenderCombo.down ? hostWindow.controlPressedBg
               : themeRenderCombo.hovered ? hostWindow.controlHoverBg
               : hostWindow.controlBg
        border.width: hostWindow.separatorWidth
        border.color: themeRenderCombo.activeFocus
                      ? hostWindow.dialogAccent : hostWindow.controlBorder
    }

    popup: T.Popup {
        y: hostWindow.snapPx(themeRenderCombo.height + 4)
        width: hostWindow.snapPx(themeRenderCombo.width)
        implicitHeight: hostWindow.snapPx(Math.min(contentItem.implicitHeight + 8, 240))
        padding: hostWindow.snapPx(4)

        background: Rectangle {
            objectName: themeRenderCombo.popupObjectNamePrefix
                       + "PopupBackground"
            radius: hostWindow.snapPx(5)
            color: hostWindow.dialogHeaderBg
            border.width: hostWindow.separatorWidth
            border.color: hostWindow.controlBorder
        }

        contentItem: ListView {
            objectName: themeRenderCombo.popupObjectNamePrefix
                       + "PopupList"
            clip: true
            implicitHeight: hostWindow.snapPx(contentHeight)
            model: themeRenderCombo.popup.visible
                   ? themeRenderCombo.delegateModel : null
            currentIndex: themeRenderCombo.highlightedIndex
            boundsBehavior: Flickable.StopAtBounds
        }
    }

    delegate: T.ItemDelegate {
        id: themeRenderComboItem
        objectName: themeRenderCombo.objectName + "Item"
        required property int index
        required property var model
        width: hostWindow.snapPx(themeRenderCombo.width - 8)
        height: hostWindow.snapPx(30)
        highlighted: themeRenderCombo.highlightedIndex === index

        contentItem: Text {
            leftPadding: hostWindow.snapPx(8)
            text: themeRenderComboItem.model.name
            color: hostWindow.textColor
            font.family: hostWindow.guiMonospaceFontFamily
            font.pixelSize: 11
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }

        background: Rectangle {
            radius: hostWindow.snapPx(3)
            color: themeRenderComboItem.highlighted
                   ? hostWindow.selectedBg
                   : themeRenderComboItem.hovered
                     ? hostWindow.controlHoverBg : "transparent"
        }
    }

}
