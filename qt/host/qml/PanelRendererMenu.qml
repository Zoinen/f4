pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Layouts
import ZoinGallery 1.0 as ZG

Popup {
    id: rendererMenu
    required property ApplicationWindow hostWindow
    required property Item panelView
    required property QtObject galleryController
    required property Item focusTarget
    required property Item anchorItem
    required property var panel
    property bool siblingMenuVisible: false
    objectName: "panelRendererMenu-" + Number(panel.side || 0)
    parent: Overlay.overlay
    popupType: Popup.Item
    width: hostWindow.snapPx(Math.max(160, hostWindow.repeaterMaxImplicitWidth(
                              rendererChoiceRepeater))
           + leftPadding + rightPadding)
    padding: hostWindow.snapPx(6)
    modal: false
    dim: false
    z: 1001
    focus: true
    property int keyboardIndex: -1
    readonly property real menuItemHeight: hostWindow.snapPx(31)

    function pixelOffset(item, horizontal) {
        let revision = item.x + item.y
        for (let ancestor = item.parent; ancestor; ancestor = ancestor.parent)
            revision += ancestor.x + ancestor.y
        const point = item.parent.mapToItem(hostWindow.contentItem, item.x, item.y)
        const value = horizontal ? point.x : point.y
        return hostWindow.snapPx(value) - value + revision * 0
    }

    function firstKeyboardIndex() {
        for (let i = 0; i < panelView.rendererChoices.length; ++i) {
            const item = rendererChoiceRepeater.itemAt(i)
            if (item && item.choiceEnabled)
                return i
        }
        return panelView.rendererChoices.length
    }

    function moveKeyboardIndex(delta) {
        const groupByIndex = panelView.rendererChoices.length
        const thumbnailsIndex = groupByIndex + 1
        const count = groupByIndex + 2 // Group by and Thumbnails
        let next = keyboardIndex < 0 ? firstKeyboardIndex() : keyboardIndex
        for (let i = 0; i < count; ++i) {
            next = (next + delta + count) % count
            if (next === groupByIndex || next === thumbnailsIndex) {
                keyboardIndex = next
                return
            }
            const item = rendererChoiceRepeater.itemAt(next)
            if (item && item.choiceEnabled) {
                keyboardIndex = next
                return
            }
        }
    }

    function openGroupByPopup() {
        keyboardIndex = panelView.rendererChoices.length
        if (!groupByPopup.opened)
            groupByPopup.open()
    }

    readonly property string panelContextKey: JSON.stringify([
        panel.side, panel.id, panel.path])
    onPanelContextKeyChanged: {
        if (opened) {
            groupByPopup.close()
            close()
        }
    }

    function handleKey(event) {
        if (event.key === Qt.Key_Escape) {
            close()
        } else if (event.key === Qt.Key_Up || event.key === Qt.Key_Down) {
            moveKeyboardIndex(event.key === Qt.Key_Up ? -1 : 1)
        } else if (event.key === Qt.Key_Right
                   && keyboardIndex === panelView.rendererChoices.length) {
            openGroupByPopup()
        } else if ((event.key === Qt.Key_Return
                    || event.key === Qt.Key_Enter
                    || event.key === Qt.Key_Space)
                   && keyboardIndex >= 0) {
            if (keyboardIndex === panelView.rendererChoices.length) {
                openGroupByPopup()
            } else if (keyboardIndex === panelView.rendererChoices.length + 1) {
                if (panelView.toggleThumbnails())
                    close()
            } else {
                const item = rendererChoiceRepeater.itemAt(keyboardIndex)
                if (item) {
                    panelView.chooseRenderer(item.modelData)
                    close()
                }
            }
        } else {
            return
        }
        event.accepted = true
    }
    closePolicy: Popup.CloseOnEscape
                 | Popup.CloseOnPressOutside
                 | Popup.CloseOnPressOutsideParent

    onAboutToShow: {
        const point = anchorItem.mapToItem(
                        hostWindow.contentItem, anchorItem.width,
                        anchorItem.height + 3)
        x = hostWindow.snapPx(Math.max(6, Math.min(hostWindow.width - width - 6,
                                point.x - width)))
        y = hostWindow.snapPx(Math.max(6, Math.min(hostWindow.height - height - 6,
                                point.y)))
        keyboardIndex = firstKeyboardIndex()
        Qt.callLater(forceActiveFocus)
    }
    onClosed: {
        groupByPopup.close()
        Qt.callLater(function() {
            if (siblingMenuVisible || !panelView.panelIsActive || galleryController.viewerVisible
                    || hostWindow.hasBlockingOverlay()
                    || hostWindow.needsFallbackGrid()
                    || hostWindow.hasDocumentSurface()
                    || hostWindow.hasOperationsQueueSurface())
                return
            const galleryHost = panelView.galleryHost()
            if (galleryHost)
                galleryHost.forceActiveFocus()
            else
                focusTarget.forceActiveFocus()
        })
    }

    background: Rectangle {
        color: hostWindow.controlBg
        radius: 8
        border.width: 1
        border.color: hostWindow.controlBorder
    }

    contentItem: Column {
        id: rendererMenuColumn
        focus: true
        Keys.onPressed: event => rendererMenu.handleKey(event)
        spacing: hostWindow.snapPx(2)

        Repeater {
            id: rendererChoiceRepeater
            model: panelView.rendererChoices

            delegate: Rectangle {
                id: rendererChoice
                required property int index
                required property var modelData
                readonly property bool isHeading:
                    modelData.heading === true
                width: rendererMenu.availableWidth
                implicitWidth: isHeading
                    ? 16 + rendererHeadingLabel.implicitWidth
                    : 8 + rendererChoiceLeading.implicitWidth
                      + 24 + rendererChoiceShortcut.implicitWidth
                      + 8
                height: hostWindow.snapPx(isHeading ? 25 : rendererMenu.menuItemHeight)
                radius: 5
                readonly property bool choiceEnabled:
                    panelView.rendererChoiceEnabled(modelData)
                readonly property bool choiceActive:
                    panelView.rendererChoiceActive(modelData)
                color: isHeading ? "transparent"
                       : (rendererMenu.keyboardIndex === index && choiceEnabled)
                         ? hostWindow.controlHoverBg : "transparent"

                Rectangle {
                    visible: rendererChoice.isHeading && index > 0
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.top: parent.top
                    height: 1
                    color: hostWindow.separatorColor
                }

                Text {
                    id: rendererHeadingLabel
                    objectName: "panelRendererHeading-" + index + "-" + Number(panel.side || 0)
                    visible: rendererChoice.isHeading
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.leftMargin: hostWindow.snapPx(8)
                    anchors.rightMargin: hostWindow.snapPx(8)
                    height: hostWindow.snapPx(20)
                    text: hostWindow.cleanText(rendererChoice.modelData.label)
                    color: hostWindow.mutedText
                    font.pixelSize: 10
                    font.weight: Font.DemiBold
                    verticalAlignment: Text.AlignVCenter
                    transform: Translate {
                        x: rendererMenu.pixelOffset(rendererHeadingLabel, true)
                        y: rendererMenu.pixelOffset(rendererHeadingLabel, false)
                    }
                }

                Row {
                    id: rendererChoiceLeading
                    visible: !rendererChoice.isHeading
                    anchors.left: parent.left
                    anchors.leftMargin: hostWindow.snapPx(8)
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: 8

                    // Always reserves its slot so the icon/label
                    // stay aligned across rows regardless of
                    // whether this particular row is active.
                    Item {
                        width: hostWindow.snapPx(14)
                        height: hostWindow.snapPx(14)
                        anchors.verticalCenter: parent.verticalCenter

                        Image {
                            id: rendererCheckIcon
                            objectName: "panelRendererChoiceCheck-"
                                        + hostWindow.cleanText(
                                              rendererChoice.modelData.mode)
                                        + "-" + Number(panel.side || 0)
                            anchors.fill: parent
                            transform: Translate {
                                x: rendererMenu.pixelOffset(rendererCheckIcon, true)
                                y: rendererMenu.pixelOffset(rendererCheckIcon, false)
                            }
                            visible: rendererChoice.choiceActive
                            smooth: false
                            source: hostWindow.lucideIconSource(
                                        "check", 14,
                                        hostWindow.dialogAccent)
                        }
                    }

                    Image {
                        id: rendererModeIcon
                        objectName: "panelRendererChoiceIcon-"
                                    + hostWindow.cleanText(
                                          rendererChoice.modelData.mode)
                                    + "-" + Number(panel.side || 0)
                        width: hostWindow.snapPx(16)
                        height: hostWindow.snapPx(16)
                        transform: Translate {
                            x: rendererMenu.pixelOffset(rendererModeIcon, true)
                            y: rendererMenu.pixelOffset(rendererModeIcon, false)
                        }
                        anchors.verticalCenter: parent.verticalCenter
                        smooth: false
                        source: hostWindow.lucideIconSource(
                                         hostWindow.cleanText(
                                             rendererChoice.modelData.icon),
                                         16,
                                         rendererChoice.choiceEnabled
                                         ? hostWindow.textColor
                                         : hostWindow.mutedText)
                    }

                    Text {
                        id: rendererModeLabel
                        objectName: "panelRendererChoiceLabel-"
                                    + (rendererChoice.modelData.mode === "file-field-columns"
                                       ? "file-field-columns" : rendererChoice.index)
                                    + "-" + Number(panel.side || 0)
                        text: hostWindow.cleanText(rendererChoice.modelData.label)
                        color: rendererChoice.choiceEnabled
                               ? hostWindow.textColor : hostWindow.mutedText
                        opacity: rendererChoice.choiceEnabled ? 1 : 0.5
                        width: hostWindow.snapPx(implicitWidth)
                        height: hostWindow.snapPx(implicitHeight)
                        font.pixelSize: 12
                        renderType: Text.NativeRendering
                        transform: Translate {
                            x: rendererMenu.pixelOffset(rendererModeLabel, true)
                            y: rendererMenu.pixelOffset(rendererModeLabel, false)
                        }
                    }
                }

                Text {
                    id: rendererChoiceShortcut
                    objectName: "panelRendererChoiceShortcut-" + index + "-" + Number(panel.side || 0)
                    visible: !rendererChoice.isHeading
                    anchors.right: parent.right
                    anchors.rightMargin: hostWindow.snapPx(8)
                    anchors.verticalCenter: parent.verticalCenter
                    text: hostWindow.cleanText(
                              rendererChoice.modelData.shortcut)
                    color: hostWindow.mutedText
                    opacity: rendererChoice.choiceEnabled ? 1 : 0.5
                    width: hostWindow.snapPx(implicitWidth)
                    height: hostWindow.snapPx(implicitHeight)
                    font.pixelSize: 10
                    renderType: Text.NativeRendering
                    transform: Translate {
                        x: rendererMenu.pixelOffset(rendererChoiceShortcut, true)
                        y: rendererMenu.pixelOffset(rendererChoiceShortcut, false)
                    }
                }

                MouseArea {
                        id: rendererChoicePointer
                        anchors.fill: parent
                        hoverEnabled: true
                        enabled: rendererChoice.choiceEnabled
                        cursorShape: enabled ? Qt.PointingHandCursor
                                             : Qt.ArrowCursor
                        onEntered: {
                            groupByPopup.close()
                            rendererMenu.keyboardIndex = rendererChoice.index
                        }
                        onClicked: {
                            panelView.chooseRenderer(rendererChoice.modelData)
                            rendererMenu.close()
                        }
                    }
                }
            }

            Item {
                id: groupByRow
                objectName: "panelRendererGroupByRow-"
                            + Number(panel.side || 0)
                width: rendererMenu.availableWidth
                height: rendererMenu.menuItemHeight
                Accessible.role: Accessible.MenuItem
                Accessible.name: "Group by"
                Accessible.description: "Grouping options "
                                      + (groupByPopup.opened
                                         ? "expanded" : "collapsed")

                Rectangle {
                    objectName: "panelRendererGroupByHighlight-" + Number(panel.side || 0)
                    anchors.fill: parent
                    radius: 5
                    color: rendererMenu.keyboardIndex
                              === panelView.rendererChoices.length
                           ? hostWindow.controlHoverBg : "transparent"
                }

                Image {
                    id: groupByIcon
                    objectName: "panelRendererGroupByIcon-"
                                + Number(panel.side || 0)
                    anchors.left: parent.left
                    anchors.leftMargin: 8 + hostWindow.snapPx(14) + 8
                    anchors.verticalCenter: parent.verticalCenter
                    width: hostWindow.snapPx(16)
                    height: hostWindow.snapPx(16)
                    smooth: false
                    transform: Translate {
                        x: rendererMenu.pixelOffset(groupByIcon, true)
                        y: rendererMenu.pixelOffset(groupByIcon, false)
                    }
                    source: hostWindow.lucideIconSource(
                                "list-tree", 16, hostWindow.textColor)
                }

                Text {
                    id: groupByLabel
                    objectName: "panelRendererGroupByLabel-"
                                + Number(panel.side || 0)
                    anchors.left: groupByIcon.right
                    anchors.leftMargin: 8
                    height: implicitHeight
                    anchors.verticalCenter: parent.verticalCenter
                    renderType: Text.NativeRendering
                    transform: Translate {
                        x: rendererMenu.pixelOffset(groupByLabel, true)
                        y: rendererMenu.pixelOffset(groupByLabel, false)
                    }
                    text: qsTr("Group by")
                    color: hostWindow.textColor
                    font.pixelSize: 12
                }

                Text {
                    id: groupByValue
                    objectName: "panelRendererGroupByValue-"
                                + Number(panel.side || 0)
                    anchors.right: groupByChevron.left
                    anchors.rightMargin: hostWindow.snapPx(8)
                    width: Math.min(implicitWidth, Math.max(0,
                               groupByChevron.x - groupByLabel.x - groupByLabel.width
                               - hostWindow.snapPx(16)))
                    height: implicitHeight
                    anchors.verticalCenter: parent.verticalCenter
                    renderType: Text.NativeRendering
                    transform: Translate {
                        x: rendererMenu.pixelOffset(groupByValue, true)
                        y: rendererMenu.pixelOffset(groupByValue, false)
                    }
                    text: panelView.groupModeLabel()
                    color: hostWindow.mutedText
                    font.pixelSize: 11
                    elide: Text.ElideRight
                }

                Image {
                    id: groupByChevron
                    objectName: "panelRendererGroupByChevron-"
                                + Number(panel.side || 0)
                    anchors.right: parent.right
                    anchors.rightMargin: hostWindow.snapPx(8)
                    anchors.verticalCenter: parent.verticalCenter
                    width: hostWindow.snapPx(12)
                    height: hostWindow.snapPx(12)
                    smooth: false
                    source: hostWindow.lucideIconSource(
                                "chevron-right", 12, hostWindow.mutedText)
                    transform: Translate {
                        x: rendererMenu.pixelOffset(groupByChevron, true)
                        y: rendererMenu.pixelOffset(groupByChevron, false)
                    }
                }

                MouseArea {
                    id: groupByPointer
                    anchors.fill: parent
                    hoverEnabled: true
                    cursorShape: Qt.PointingHandCursor
                    onEntered: rendererMenu.openGroupByPopup()
                    onPressed: rendererMenu.keyboardIndex
                              = panelView.rendererChoices.length
                    onClicked: {
                        if (groupByPopup.opened)
                            groupByPopup.close()
                        else
                            rendererMenu.openGroupByPopup()
                    }
                }
            }

            Item {
                id: thumbnailsRow
                objectName: "panelThumbnailsRow-" + Number(panel.side || 0)
                property bool checked: panelView.thumbnailsEnabled()
                width: rendererMenu.availableWidth
                height: rendererMenu.menuItemHeight
                Accessible.role: Accessible.MenuItem
                Accessible.name: qsTr("Thumbnails")
                Accessible.checkable: true
                Accessible.checked: checked
                Accessible.description: galleryController.panelPreferences
                                       ? galleryController.panelPreferences.error
                                       : ""

                Rectangle {
                    anchors.fill: parent
                    radius: 5
                    color: rendererMenu.keyboardIndex
                              === panelView.rendererChoices.length + 1
                           ? hostWindow.controlHoverBg : "transparent"
                }

                Row {
                    anchors.left: parent.left
                    anchors.leftMargin: 8
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: 8

                    Item {
                        width: hostWindow.snapPx(14)
                        height: hostWindow.snapPx(14)
                        anchors.verticalCenter: parent.verticalCenter

                        Image {
                            id: thumbnailsCheck
                            objectName: "panelThumbnailsCheck-"
                                        + Number(panel.side || 0)
                            anchors.fill: parent
                            visible: thumbnailsRow.checked
                            smooth: false
                            source: hostWindow.lucideIconSource(
                                        "check", 14,
                                        hostWindow.dialogAccent)
                            transform: Translate {
                                x: rendererMenu.pixelOffset(thumbnailsCheck, true)
                                y: rendererMenu.pixelOffset(thumbnailsCheck, false)
                            }
                        }
                    }

                    Image {
                        id: thumbnailsIcon
                        objectName: "panelThumbnailsIcon-"
                                    + Number(panel.side || 0)
                        width: hostWindow.snapPx(16)
                        height: hostWindow.snapPx(16)
                        anchors.verticalCenter: parent.verticalCenter
                        smooth: false
                        source: hostWindow.lucideIconSource(
                                    "image", 16, hostWindow.textColor)
                        transform: Translate {
                            x: rendererMenu.pixelOffset(thumbnailsIcon, true)
                            y: rendererMenu.pixelOffset(thumbnailsIcon, false)
                        }
                    }

                    Text {
                        id: thumbnailsLabel
                        objectName: "panelThumbnailsLabel-"
                                    + Number(panel.side || 0)
                        anchors.verticalCenter: parent.verticalCenter
                        text: qsTr("Thumbnails")
                        color: hostWindow.textColor
                        font.pixelSize: 12
                        renderType: Text.NativeRendering
                        transform: Translate {
                            x: rendererMenu.pixelOffset(thumbnailsLabel, true)
                            y: rendererMenu.pixelOffset(thumbnailsLabel, false)
                        }
                    }
                }

                MouseArea {
                    anchors.fill: parent
                    hoverEnabled: true
                    cursorShape: Qt.PointingHandCursor
                    onEntered: {
                        rendererMenu.keyboardIndex =
                                panelView.rendererChoices.length + 1
                        groupByPopup.close()
                    }
                    onClicked: {
                        if (panelView.toggleThumbnails())
                            rendererMenu.close()
                    }
                }
            }

            Text {
                objectName: "panelThumbnailsError-" + Number(panel.side || 0)
                visible: galleryController.panelPreferences
                         && galleryController.panelPreferences.error !== ""
                width: rendererMenu.availableWidth - hostWindow.snapPx(16)
                anchors.horizontalCenter: parent.horizontalCenter
                wrapMode: Text.Wrap
                text: galleryController.panelPreferences
                      ? galleryController.panelPreferences.error : ""
                color: hostWindow.dialogAccent
                font.pixelSize: 10
            }
            Rectangle {
                width: rendererMenu.availableWidth
                height: 1
                color: hostWindow.separatorColor
                visible: rendererZoomRow.visible
            }

            Item {
                id: rendererZoomRow
                objectName: "panelRendererZoomRow-"
                            + Number(panel.side || 0)
                width: rendererMenu.availableWidth
                height: visible ? hostWindow.snapPx(48) : 0
                visible: galleryController.available
                         && panelView.galleryHost()
                         && panelView.galleryHost().densityAdjustable

                HoverHandler {
                    onHoveredChanged: {
                        if (hovered) {
                            rendererMenu.keyboardIndex = -1
                            groupByPopup.close()
                        }
                    }
                }

                Text {
                    id: rendererZoomLabel
                    objectName: "panelRendererZoomLabel-"
                                + Number(panel.side || 0)
                    anchors.left: parent.left
                    anchors.leftMargin: hostWindow.snapPx(8)
                    anchors.top: parent.top
                    anchors.topMargin: hostWindow.snapPx(5)
                    text: "Zoom"
                    color: hostWindow.mutedText
                    width: hostWindow.snapPx(implicitWidth)
                    height: hostWindow.snapPx(implicitHeight)
                    font.pixelSize: 10
                    font.weight: Font.DemiBold
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               rendererZoomLabel, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               rendererZoomLabel, hostWindow.contentItem)
                    }
                }

                Text {
                    id: rendererZoomReset
                    objectName: "panelRendererZoomReset-"
                                + Number(panel.side || 0)
                    anchors.right: rendererZoomValue.left
                    anchors.rightMargin: hostWindow.snapPx(8)
                    anchors.baseline: rendererZoomLabel.baseline
                    text: "Reset"
                    color: rendererZoomResetPointer.containsMouse
                           ? hostWindow.panelSelectionBorder : hostWindow.mutedText
                    width: hostWindow.snapPx(implicitWidth)
                    height: hostWindow.snapPx(implicitHeight)
                    font.pixelSize: 10
                    font.underline: rendererZoomResetPointer.containsMouse
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               rendererZoomReset, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               rendererZoomReset, hostWindow.contentItem)
                    }

                    MouseArea {
                        id: rendererZoomResetPointer
                        anchors.fill: parent
                        anchors.margins: -3
                        hoverEnabled: true
                        cursorShape: Qt.PointingHandCursor
                        onClicked: {
                            hostWindow.action({
                                "action": "panel.resetGalleryDensity",
                                "side": panel.side,
                                "layoutMode": panelView.effectiveGalleryLayoutMode
                            }, true)
                        }
                    }
                }

                Text {
                    id: rendererZoomValue
                    objectName: "panelRendererZoomValue-"
                                + Number(panel.side || 0)
                    anchors.right: parent.right
                    anchors.rightMargin: hostWindow.snapPx(8)
                    anchors.baseline: rendererZoomLabel.baseline
                    text: Math.round(rendererZoomSlider.value) + " px"
                    color: hostWindow.mutedText
                    width: hostWindow.snapPx(implicitWidth)
                    height: hostWindow.snapPx(implicitHeight)
                    font.pixelSize: 10
                    transform: Translate {
                        x: hostWindow.dialogPixelOffsetX(
                               rendererZoomValue, hostWindow.contentItem)
                        y: hostWindow.dialogPixelOffsetY(
                               rendererZoomValue, hostWindow.contentItem)
                    }
                }

                T.Slider {
                    id: rendererZoomSlider
                    objectName: "panelRendererZoomSlider-"
                                + Number(panel.side || 0)
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.leftMargin: hostWindow.snapPx(8)
                    anchors.rightMargin: hostWindow.snapPx(8)
                    height: hostWindow.snapPx(27)
                    focusPolicy: Qt.NoFocus
                    hoverEnabled: true
                    from: panelView.galleryHost()
                          ? panelView.galleryHost().minimumDensity : 0
                    to: panelView.galleryHost()
                        ? panelView.galleryHost().maximumDensity : 1
                    stepSize: panelView.galleryHost()
                              ? panelView.galleryHost().densityStep : 1
                    value: panelView.galleryHost()
                           ? panelView.galleryHost().currentDensity : 0
                    snapMode: T.Slider.SnapAlways

                    onMoved: {
                        const host = panelView.galleryHost()
                        if (host)
                            host.previewDensity(value)
                    }
                    onPressedChanged: {
                        if (pressed)
                            return
                        const host = panelView.galleryHost()
                        if (host)
                            host.commitDensity(value)
                    }

                    background: Rectangle {
                        x: rendererZoomSlider.leftPadding
                        y: Math.round((rendererZoomSlider.height
                                       - height) / 2)
                        width: rendererZoomSlider.availableWidth
                        height: hostWindow.snapPx(4)
                        radius: 2
                        color: hostWindow.controlBorder

                        Rectangle {
                            width: rendererZoomSlider.visualPosition
                                   * parent.width
                            height: parent.height
                            radius: parent.radius
                            color: hostWindow.panelSelectionBorder
                        }
                    }

                    handle: Rectangle {
                        x: rendererZoomSlider.leftPadding
                           + rendererZoomSlider.visualPosition
                             * (rendererZoomSlider.availableWidth
                                - width)
                        y: Math.round((rendererZoomSlider.height
                                       - height) / 2)
                        width: hostWindow.snapPx(14)
                        height: hostWindow.snapPx(14)
                        radius: 7
                        color: rendererZoomSlider.pressed
                               ? hostWindow.panelSelectionBorder
                               : hostWindow.chromeText
                        border.width: 2
                        border.color: hostWindow.controlBg
                    }
                }
            }
        }

    Popup {
        id: groupByPopup
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
            color: rendererMenu.background.color
            radius: rendererMenu.background.radius
            border.width: rendererMenu.background.border.width
            border.color: rendererMenu.background.border.color
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
            const point = groupByRow.mapToItem(parent, 0, 0)
            const menuLeft = rendererMenu.contentItem.mapToItem(
                        parent, -rendererMenu.leftPadding, 0).x
            const preferredRight = menuLeft + rendererMenu.width
            const preferredX = preferredRight + width <= hostWindow.width - 6
                    ? preferredRight : menuLeft - width
            x = hostWindow.snapPx(Math.max(6, Math.min(
                hostWindow.width - width - 6, preferredX)))
            y = hostWindow.snapPx(Math.max(6, Math.min(
                hostWindow.height - height - 6, point.y - topPadding)))
            Qt.callLater(forceActiveFocus)
        }
        onClosed: {
            if (rendererMenu.opened)
                Qt.callLater(rendererMenu.forceActiveFocus)
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
                    rendererMenu.close()
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
                        height: separator ? hostWindow.snapPx(13) : rendererMenu.menuItemHeight
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
                                x: rendererMenu.pixelOffset(groupChoiceSeparator, true)
                                y: rendererMenu.pixelOffset(groupChoiceSeparator, false)
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
                                x: rendererMenu.pixelOffset(groupChoiceIcon, true)
                                y: rendererMenu.pixelOffset(groupChoiceIcon, false)
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
                                x: rendererMenu.pixelOffset(groupChoiceLabel, true)
                                y: rendererMenu.pixelOffset(groupChoiceLabel, false)
                            }
                            text: hostWindow.cleanText(modelData.label)
                            color: hostWindow.textColor
                            font.pixelSize: 12
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
                                x: rendererMenu.pixelOffset(groupChoiceCheck, true)
                                y: rendererMenu.pixelOffset(groupChoiceCheck, false)
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
                                rendererMenu.close()
                            }
                        }
                    }
                }
            }
        }
    }
    }
