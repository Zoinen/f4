pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl

T.ComboBox {
    id: control

    property ApplicationWindow hostWindow: (control.Window.window as ApplicationWindow) || null
    property bool semanticFocus: false

    function snap(val) {
        return hostWindow ? hostWindow.snapPx(val) : Math.round(val)
    }

    focusPolicy: Qt.NoFocus
    hoverEnabled: true
    padding: 0
    implicitHeight: snap(30)
    implicitWidth: snap(160)
    Layout.preferredHeight: implicitHeight
    Layout.minimumHeight: implicitHeight
    Layout.maximumHeight: implicitHeight

    readonly property string popupObjectNamePrefix:
        control.objectName === "themeFontRenderTypeCombo"
        ? "themeFontRenderType" : control.objectName

    contentItem: Text {
        leftPadding: control.snap(10)
        rightPadding: control.snap(30)
        text: control.displayText
        color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
        font.family: control.hostWindow ? control.hostWindow.guiMonospaceFontFamily : "monospace"
        font.pixelSize: 11
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }

    indicator: IconLabel {
        objectName: control.objectName ? (control.objectName + "Indicator") : ""
        readonly property url rasterizedIconSource:
            control.hostWindow
            ? control.hostWindow.lucideIconSource(
                "chevron-down", 14,
                control.enabled ? control.hostWindow.textColor : control.hostWindow.mutedText)
            : ""
        x: control.snap(control.width - width - 10)
        y: control.snap((control.height - height) / 2)
        width: control.snap(14)
        height: control.snap(14)
        icon.source: rasterizedIconSource
        icon.width: control.snap(14)
        icon.height: control.snap(14)
        icon.color: control.hovered
                    ? (control.hostWindow ? control.hostWindow.textColor : "#ffffff")
                    : (control.hostWindow ? control.hostWindow.mutedText : "#888888")
    }

    background: Rectangle {
        objectName: control.objectName ? (control.objectName + "Background") : ""
        readonly property color testBorderColor: border.color
        radius: control.snap(4)
        color: {
            if (!control.enabled)
                return control.hostWindow ? control.hostWindow.dialogBg : "#18202a"
            if (control.down)
                return control.hostWindow ? control.hostWindow.controlPressedBg : "#334455"
            if (control.hovered)
                return control.hostWindow ? control.hostWindow.controlHoverBg : "#223344"
            return control.hostWindow ? control.hostWindow.controlBg : "#18202a"
        }
        border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
        border.color: {
            if (control.semanticFocus || control.visualFocus || control.activeFocus)
                return control.hostWindow ? control.hostWindow.dialogAccent : "#2c7be5"
            return control.hostWindow ? control.hostWindow.controlBorder : "#25303d"
        }
    }

    popup: T.Popup {
        y: control.snap(control.height + 4)
        width: control.snap(control.width)
        implicitHeight: control.snap(Math.min(contentItem.implicitHeight + 8, 240))
        padding: control.snap(4)

        background: Rectangle {
            objectName: control.popupObjectNamePrefix
                       ? (control.popupObjectNamePrefix + "PopupBackground") : ""
            radius: control.snap(5)
            color: control.hostWindow ? control.hostWindow.dialogHeaderBg : "#1c2430"
            border.width: control.hostWindow ? control.hostWindow.separatorWidth : 1
            border.color: control.hostWindow ? control.hostWindow.controlBorder : "#25303d"
        }

        contentItem: ListView {
            objectName: control.popupObjectNamePrefix
                       ? (control.popupObjectNamePrefix + "PopupList") : ""
            clip: true
            implicitHeight: control.snap(contentHeight)
            model: control.popup.visible ? control.delegateModel : null
            currentIndex: control.highlightedIndex
            boundsBehavior: Flickable.StopAtBounds

            ScrollBar.vertical: F4ScrollBar {
                policy: ScrollBar.AsNeeded
                thickness: 6
            }
        }
    }

    delegate: T.ItemDelegate {
        id: itemDelegate
        objectName: control.objectName ? (control.objectName + "Item") : ""
        required property int index
        required property var model
        width: control.snap(control.width - 8)
        height: control.snap(30)
        highlighted: control.highlightedIndex === index

        contentItem: Text {
            leftPadding: control.snap(8)
            text: {
                if (itemDelegate.model && itemDelegate.model.name !== undefined)
                    return itemDelegate.model.name
                if (itemDelegate.model && itemDelegate.model.text !== undefined)
                    return itemDelegate.model.text
                return String(itemDelegate.model || "")
            }
            color: control.hostWindow ? control.hostWindow.textColor : "#ffffff"
            font.family: control.hostWindow ? control.hostWindow.guiMonospaceFontFamily : "monospace"
            font.pixelSize: 11
            verticalAlignment: Text.AlignVCenter
            elide: Text.ElideRight
        }

        background: Rectangle {
            radius: control.snap(3)
            color: itemDelegate.highlighted
                   ? (control.hostWindow ? control.hostWindow.selectedBg : "#2c7be5")
                   : itemDelegate.hovered
                     ? (control.hostWindow ? control.hostWindow.controlHoverBg : "#223344") : "transparent"
        }
    }
}
