pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T

Popup {
    id: groupByPopup
    required property ApplicationWindow hostWindow
    required property Item panelView
    required property var panel
    required property Popup parentMenu
    required property Item anchorItem
    objectName: "panelGroupBySubmenu-" + Number(panel.side || 0)
    parent: Overlay.overlay
    popupType: Popup.Item
    width: hostWindow.snapPx(300)
    height: Math.max(1, Math.min(hostWindow.height - hostWindow.snapPx(12),
                     groupByColumn.implicitHeight + topPadding + bottomPadding))
    padding: hostWindow.snapPx(6)
    modal: false
    dim: false
    z: 1002
    focus: true
    closePolicy: Popup.CloseOnEscape
                 | Popup.CloseOnPressOutside
                 | Popup.CloseOnPressOutsideParent
    property int currentIndex: -1

    background: Rectangle {
        color: parentMenu.background.color
        radius: parentMenu.background.radius
        border.width: parentMenu.background.border.width
        border.color: parentMenu.background.border.color
    }

    onCurrentIndexChanged: Qt.callLater(function() {
        const row = groupChoiceRepeater.itemAt(currentIndex)
        const flickable = groupByScroll.contentItem
        if (!row || !flickable)
            return
        if (row.y < flickable.contentY)
            flickable.contentY = row.y
        else if (row.y + row.height > flickable.contentY + flickable.height)
            flickable.contentY = row.y + row.height - flickable.height
    })

    function firstSelectable() {
        for (let i = 0; i < panelView.groupChoices.length; ++i) {
            if (panelView.groupChoices[i].separator !== true)
                return i
        }
        return -1
    }
    function moveSelection(delta) {
        const choices = panelView.groupChoices
        if (choices.length === 0)
            return
        let next = currentIndex < 0 ? firstSelectable() : currentIndex
        for (let i = 0; i < choices.length; ++i) {
            next = (next + delta + choices.length) % choices.length
            if (choices[next].separator !== true) {
                currentIndex = next
                return
            }
        }
    }

    onAboutToShow: {
        currentIndex = panelView.groupChoices.length > 0
                ? panelView.groupChoices.findIndex(
                      choice => choice.mode === panelView.groupModeName())
                : -1
        if (currentIndex < 0)
            currentIndex = firstSelectable()
        const point = anchorItem.mapToItem(parent, 0, 0)
        const menuLeft = parentMenu.contentItem.mapToItem(
                    parent, -parentMenu.leftPadding, 0).x
        const preferredRight = menuLeft + parentMenu.width
        const preferredX = preferredRight + width <= hostWindow.width - 6
                ? preferredRight : menuLeft - width
        x = hostWindow.snapPx(Math.max(6, Math.min(
            hostWindow.width - width - 6, preferredX)))
        y = hostWindow.snapPx(Math.max(6, Math.min(
            hostWindow.height - height - 6, point.y - topPadding)))
        Qt.callLater(forceActiveFocus)
    }
    onClosed: {
        if (parentMenu.opened)
            Qt.callLater(parentMenu.forceActiveFocus)
    }

    function handleKey(event) {
        if (event.key === Qt.Key_Escape || event.key === Qt.Key_Left) {
            close()
            event.accepted = true
        } else if (event.key === Qt.Key_Up) {
            moveSelection(-1)
            event.accepted = true
        } else if (event.key === Qt.Key_Down) {
            moveSelection(1)
            event.accepted = true
        } else if (event.key === Qt.Key_Return
                   || event.key === Qt.Key_Enter
                   || event.key === Qt.Key_Space) {
            if (currentIndex >= 0) {
                panelView.chooseGrouping(
                            panelView.groupChoices[currentIndex])
                close()
                parentMenu.close()
            }
            event.accepted = true
        }
    }

    contentItem: T.ScrollView {
        id: groupByScroll
        focus: true
        Keys.priority: Keys.BeforeItem
        Keys.onPressed: event => groupByPopup.handleKey(event)
        clip: true
        contentWidth: availableWidth
        contentHeight: groupByColumn.implicitHeight
        ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
        ScrollBar.vertical: T.ScrollBar {
            parent: groupByScroll
            x: groupByScroll.width - width
            y: groupByScroll.topPadding
            height: groupByScroll.availableHeight
            visible: size < 1
            policy: ScrollBar.AsNeeded
            contentItem: Rectangle {
                implicitWidth: hostWindow.snapPx(6)
                implicitHeight: hostWindow.snapPx(24)
                radius: width / 2
                color: parent.pressed ? hostWindow.textColor : hostWindow.mutedText
            }
            background: Item {}
        }

        Column {
            id: groupByColumn
            width: groupByScroll.availableWidth
            spacing: 2

            Repeater {
                id: groupChoiceRepeater
                model: panelView.groupChoices

                delegate: Rectangle {
                    id: groupChoice
                    objectName: "panelGroupChoiceRow-" + Number(panel.side || 0) + "-" + index
                    required property int index
                    required property var modelData
                    readonly property bool separator:
                        modelData.separator === true
                    readonly property bool active:
                        panelView.groupChoiceActive(modelData)
                    readonly property bool directional:
                        active && !modelData.special && modelData.mode !== "None"
                    width: groupByColumn.width
                    height: separator ? hostWindow.snapPx(13) : parentMenu.menuItemHeight
                    radius: 5
                    Accessible.role: separator
                                     ? Accessible.Separator
                                     : Accessible.MenuItem
                    Accessible.name: separator
                                      ? ""
                                      : hostWindow.cleanText(modelData.label)
                    Accessible.description: separator
                                            ? ""
                                            : "Group by "
                                              + hostWindow.cleanText(
                                                    modelData.label)
                                              + (directional
                                                 ? (panelView.groupDirectionIconName() === "arrow-up"
                                                    ? qsTr(". Ascending. Activate again to reverse.")
                                                    : qsTr(". Descending. Activate again to reverse."))
                                                 : "")
                    color: separator ? "transparent"
                           : (groupByPopup.currentIndex === index)
                             ? hostWindow.controlHoverBg : "transparent"

                    Rectangle {
                        id: groupChoiceSeparator
                        objectName: "panelGroupChoiceSeparator-"
                                    + Number(panel.side || 0) + "-" + index
                        visible: groupChoice.separator
                        anchors.left: parent.left
                        anchors.right: parent.right
                        y: hostWindow.snapPx((parent.height - height) / 2)
                        height: hostWindow.snapPx(1)
                        color: hostWindow.separatorColor
                        transform: Translate {
                            x: parentMenu.pixelOffset(groupChoiceSeparator, true)
                            y: parentMenu.pixelOffset(groupChoiceSeparator, false)
                        }
                    }

                    Image {
                        id: groupChoiceIcon
                        visible: !groupChoice.separator
                        objectName: "panelGroupChoiceIcon-"
                                    + Number(panel.side || 0) + "-" + index
                        anchors.left: parent.left
                        anchors.leftMargin: hostWindow.snapPx(30)
                        anchors.verticalCenter: parent.verticalCenter
                        width: hostWindow.snapPx(16)
                        height: hostWindow.snapPx(16)
                        smooth: false
                        transform: Translate {
                            x: parentMenu.pixelOffset(groupChoiceIcon, true)
                            y: parentMenu.pixelOffset(groupChoiceIcon, false)
                        }
                        source: hostWindow.lucideIconSource(
                                    hostWindow.cleanText(modelData.icon || "list"),
                                    16,
                                    hostWindow.textColor)
                    }

                    Text {
                        id: groupChoiceLabel
                        visible: !groupChoice.separator
                        objectName: "panelGroupChoiceLabel-"
                                    + Number(panel.side || 0) + "-" + index
                        anchors.left: parent.left
                        anchors.leftMargin: hostWindow.snapPx(54)
                        anchors.right: parent.right
                        anchors.rightMargin: hostWindow.snapPx(8)
                        height: implicitHeight
                        anchors.verticalCenter: parent.verticalCenter
                        renderType: Text.NativeRendering
                        transform: Translate {
                            x: parentMenu.pixelOffset(groupChoiceLabel, true)
                            y: parentMenu.pixelOffset(groupChoiceLabel, false)
                        }
                        text: hostWindow.cleanText(modelData.label)
                        color: hostWindow.textColor
                        font.pixelSize: (hostWindow ? hostWindow.uiTextSize(12) : 12)
                        elide: Text.ElideRight
                    }

                    Image {
                        id: groupChoiceCheck
                        visible: !groupChoice.separator && groupChoice.active
                        objectName: "panelGroupChoiceCheck-"
                                    + Number(panel.side || 0) + "-" + index
                        anchors.left: parent.left
                        anchors.leftMargin: hostWindow.snapPx(8)
                        anchors.verticalCenter: parent.verticalCenter
                        width: hostWindow.snapPx(14)
                        height: hostWindow.snapPx(14)
                        smooth: false
                        transform: Translate {
                            x: parentMenu.pixelOffset(groupChoiceCheck, true)
                            y: parentMenu.pixelOffset(groupChoiceCheck, false)
                        }
                        source: hostWindow.lucideIconSource(
                                    groupChoice.directional ? panelView.groupDirectionIconName() : "check",
                                    14, hostWindow.dialogAccent)
                    }

                    MouseArea {
                        id: groupChoicePointer
                        anchors.fill: parent
                        enabled: !groupChoice.separator
                        hoverEnabled: true
                        cursorShape: Qt.PointingHandCursor
                        onEntered: groupByPopup.currentIndex = index
                        onClicked: {
                            panelView.chooseGrouping(modelData)
                            groupByPopup.close()
                            parentMenu.close()
                        }
                    }
                }
            }
        }
    }
}
