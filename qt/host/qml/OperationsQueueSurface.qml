pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.Basic as T
import QtQuick.Controls.impl

Rectangle {
    id: queueRoot
    required property ApplicationWindow hostWindow
    required property Item menuBar
    objectName: "operationsQueueSurface"
    OperationsQueueController {
        id: queueController
        hostWindow: queueRoot.hostWindow
        menuBar: queueRoot.menuBar
        listView: operationsQueueList
        surfaceWidth: queueRoot.width
        headerClearButton: headerClearButton
        cancelButton: cancelButton
    }

    property alias queue: queueController.queue
    property alias interactionActive: queueController.interactionActive
    property alias localSelectedTaskId: queueController.localSelectedTaskId
    property alias pendingSelectedTaskId: queueController.pendingSelectedTaskId
    readonly property real topInset: queueController.topInset
    readonly property real bottomInset: queueController.bottomInset
    readonly property real rowHeight: queueController.rowHeight
    readonly property int selectedIndex: queueController.selectedIndex

    function navigate(command) { return queueController.navigate(command) }
    function activateSelection() { return queueController.activateSelection() }
    function controlOwnsActivation() {
        return queueController.controlOwnsActivation()
    }
    function selectedItem() { return queueController.selectedItem() }
    function delegateForTaskId(taskId) {
        return queueController.delegateForTaskId(taskId)
    }
    function cancelSelection() { return queueController.cancelSelection() }
    function clearCompleted() { return queueController.clearCompleted() }

    color: hostWindow.windowBackgroundColor
    Accessible.role: Accessible.Table
    Accessible.name: hostWindow.cleanText(queue.title || "Operations Queue")
    Accessible.description: hostWindow.cleanText(queue.accessibleDescription)


    Rectangle {
        id: queueChrome
        objectName: "operationsQueueHeader"
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.topMargin: queueController.topInset
        height: 62
        color: hostWindow.titleBarBg

        Text {
            anchors.left: parent.left
            anchors.leftMargin: hostWindow.contentSpacing
            anchors.top: parent.top
            anchors.topMargin: 9
            text: hostWindow.cleanText(queue.title || "Operations Queue")
            color: hostWindow.textColor
            font.pixelSize: 18
            font.weight: Font.DemiBold
            Accessible.role: Accessible.Heading
            Accessible.name: text
        }

        Row {
            anchors.left: parent.left
            anchors.leftMargin: hostWindow.contentSpacing
            anchors.bottom: parent.bottom
            anchors.bottomMargin: 8
            spacing: 15

            QueueSummaryItem {
                hostWindow: queueController.hostWindow
                objectName: "operationsQueueSummary-running"
                statusName: "running"
                iconName: "circle-play"
                count: Number(queue.runningCount || 0)
                accent: hostWindow.dialogAccent
            }

            QueueSummaryItem {
                hostWindow: queueController.hostWindow
                objectName: "operationsQueueSummary-queued"
                statusName: "queued"
                iconName: "clock-3"
                count: Number(queue.queuedCount || 0)
                accent: "#d9b866"
            }

            QueueSummaryItem {
                hostWindow: queueController.hostWindow
                objectName: "operationsQueueSummary-completed"
                statusName: "completed"
                iconName: "circle-check"
                count: Number(queue.completedCount || 0)
                accent: "#75c991"
            }

            QueueSummaryItem {
                hostWindow: queueController.hostWindow
                objectName: "operationsQueueSummary-errors"
                statusName: "errors"
                iconName: "triangle-alert"
                count: Number(queue.errorCount || 0)
                accent: "#ee6a6a"
                visible: count > 0
            }
        }

        QueueActionButton {
            hostWindow: queueController.hostWindow
            id: headerClearButton
            objectName: "operationsQueueClearButton"
            iconName: "trash-2"
            anchors.right: parent.right
            anchors.rightMargin: hostWindow.contentSpacing
            anchors.verticalCenter: parent.verticalCenter
            text: hostWindow.cleanText(queue.clearText || "Clear completed")
            enabled: queue.canClear === true
            Accessible.name: text
            Accessible.description: hostWindow.cleanText(queue.clearDescription)
            onClicked: queueController.clearCompleted()
        }
    }

    Rectangle {
        id: columnsHeader
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: queueChrome.bottom
        height: 34
        color: hostWindow.titleBarBg

        Text {
            x: hostWindow.contentSpacing
            width: queueController.idColumnWidth
            anchors.verticalCenter: parent.verticalCenter
            text: queueController.columnTitle("id", "ID")
            visible: width > 0
            color: hostWindow.chromeText
            font.pixelSize: 12
            font.weight: Font.DemiBold
        }
        Text {
            x: hostWindow.contentSpacing + queueController.idColumnWidth
            width: queueController.stateColumnWidth
            anchors.verticalCenter: parent.verticalCenter
            text: queueController.columnTitle("state", "State")
            color: hostWindow.chromeText
            font.pixelSize: 12
            font.weight: Font.DemiBold
        }
        Text {
            x: hostWindow.contentSpacing + queueController.idColumnWidth
               + queueController.stateColumnWidth
            width: queueController.typeColumnWidth
            anchors.verticalCenter: parent.verticalCenter
            text: queueController.columnTitle("type", "Type")
            color: hostWindow.chromeText
            font.pixelSize: 12
            font.weight: Font.DemiBold
        }
        Text {
            anchors.left: parent.left
            anchors.leftMargin: hostWindow.contentSpacing
                                + queueController.idColumnWidth
                                + queueController.stateColumnWidth
                                + queueController.typeColumnWidth
            anchors.right: progressHeader.left
            anchors.rightMargin: 8
            anchors.verticalCenter: parent.verticalCenter
            text: queueController.columnTitle("description",
                                        "Description / Current File")
            color: hostWindow.chromeText
            font.pixelSize: 12
            font.weight: Font.DemiBold
            elide: Text.ElideRight
        }
        Text {
            id: progressHeader
            anchors.right: speedHeader.left
            width: queueController.progressColumnWidth
            anchors.verticalCenter: parent.verticalCenter
            text: queueController.columnTitle("progress", "Progress")
            color: hostWindow.chromeText
            font.pixelSize: 12
            font.weight: Font.DemiBold
        }
        Text {
            id: speedHeader
            anchors.right: parent.right
            anchors.rightMargin: hostWindow.contentSpacing
            width: queueController.speedColumnWidth
            anchors.verticalCenter: parent.verticalCenter
            text: queueController.columnTitle("speed", "Speed")
            visible: width > 0
            color: hostWindow.chromeText
            font.pixelSize: 12
            font.weight: Font.DemiBold
        }
    }

    ListView {
        id: operationsQueueList
        objectName: "operationsQueueList"
        anchors.left: parent.left
        anchors.right: queueScrollBar.visible
                       ? queueScrollBar.left : parent.right
        anchors.top: columnsHeader.bottom
        anchors.bottom: queueFooter.top
        clip: true
        model: queueController.rowsModel
        reuseItems: true
        boundsBehavior: Flickable.StopAtBounds
        flickableDirection: Flickable.VerticalFlick
        interactive: queueController.interactionActive
        keyNavigationEnabled: false
        Accessible.role: Accessible.List
        Accessible.name: hostWindow.cleanText(queue.title || "Operations Queue")

        delegate: Rectangle {
            id: queueRow
            required property string stableId
            required property int taskId
            required property int itemIndex
            required property string taskType
            required property string description
            required property string state
            required property string stateClass
            required property string currentFile
            required property string displayText
            required property real currentProgress
            required property real progress
            required property string totalText
            required property string speed
            required property string error
            required property bool cancellable
            required property bool hasDetails
            required property bool terminal
            required property bool active
            required property int index
            readonly property bool current:
                taskId === queueController.localSelectedTaskId
            readonly property bool totalProgressKnown:
                progress >= 0 && stateClass !== "scanning"
            objectName: "operationsQueueRow-" + taskId
            width: operationsQueueList.width
            height: queueController.rowHeight
            color: current ? hostWindow.panelSelectionBg
                           : rowHover.hovered
                             ? hostWindow.controlHoverBg
                             : index % 2 === 0
                               ? hostWindow.dialogBg : hostWindow.windowBackgroundColor
            border.width: current ? 1 : 0
            border.color: hostWindow.panelSelectionBorder
            Accessible.role: Accessible.ListItem
            Accessible.name: state + ", " + taskType + ", "
                             + (displayText !== "" ? displayText
                                                   : description)
            Accessible.description: error !== "" ? error : totalText
            Accessible.selected: current
            Accessible.focusable: false
            Accessible.onPressAction: {
                queueController.selectIndex(index, true)
                queueController.activateItem(
                    queueController.rowsModel.get(index))
            }

            HoverHandler { id: rowHover }

            Text {
                x: hostWindow.contentSpacing
                width: queueController.idColumnWidth - 8
                anchors.verticalCenter: parent.verticalCenter
                text: String(queueRow.taskId)
                visible: queueController.idColumnWidth > 0
                color: hostWindow.mutedText
                font.family: hostWindow.guiMonospaceFontFamily
                font.pixelSize: 12
            }

            Item {
                x: hostWindow.contentSpacing + queueController.idColumnWidth
                width: queueController.stateColumnWidth - 8
                height: parent.height
                IconLabel {
                    anchors.left: parent.left
                    anchors.verticalCenter: parent.verticalCenter
                    width: 14
                    height: 14
                    icon.source: hostWindow.lucideIconSource(
                                     queueController.stateIconName(
                                         queueRow.stateClass,
                                         queueRow.state),
                                     14,
                                     queueController.stateColor(
                                         queueRow.stateClass,
                                         queueRow.state))
                    icon.width: 14
                    icon.height: 14
                    icon.color: queueController.stateColor(queueRow.stateClass,
                                                     queueRow.state)
                }
                Text {
                    anchors.left: parent.left
                    anchors.leftMargin: 15
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    text: queueRow.state
                    color: queueController.stateColor(queueRow.stateClass,
                                                queueRow.state)
                    elide: Text.ElideRight
                    font.pixelSize: 12
                    font.weight: queueRow.active
                                 ? Font.DemiBold : Font.Normal
                }
            }

            Text {
                x: hostWindow.contentSpacing + queueController.idColumnWidth
                   + queueController.stateColumnWidth
                width: queueController.typeColumnWidth - 8
                anchors.verticalCenter: parent.verticalCenter
                text: queueRow.taskType
                color: hostWindow.textColor
                elide: Text.ElideRight
                font.pixelSize: 12
            }

            Item {
                anchors.left: parent.left
                anchors.leftMargin: hostWindow.contentSpacing
                                    + queueController.idColumnWidth
                                    + queueController.stateColumnWidth
                                    + queueController.typeColumnWidth
                anchors.right: progressCell.left
                anchors.rightMargin: 8
                height: parent.height

                Text {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    anchors.verticalCenterOffset:
                        (queueRow.error !== ""
                         || (queueRow.currentFile !== ""
                             && queueRow.currentFile
                                !== queueRow.displayText)) ? -9 : 0
                    text: queueRow.displayText !== ""
                          ? queueRow.displayText : queueRow.description
                    color: queueRow.error !== ""
                           ? "#ee8b8b" : hostWindow.textColor
                    elide: Text.ElideMiddle
                    font.pixelSize: 13
                }
                Text {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.bottomMargin: 7
                    text: queueRow.error !== ""
                          ? queueRow.error : queueRow.currentFile
                    visible: text !== ""
                             && text !== queueRow.displayText
                    color: queueRow.error !== ""
                           ? "#ee8b8b" : hostWindow.mutedText
                    elide: Text.ElideMiddle
                    font.pixelSize: 11
                }
            }

            Item {
                id: progressCell
                anchors.right: speedCell.left
                width: queueController.progressColumnWidth
                height: parent.height

                Rectangle {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.rightMargin: 18
                    anchors.verticalCenter: parent.verticalCenter
                    anchors.verticalCenterOffset: -6
                    height: 6
                    radius: 3
                    color: hostWindow.controlBg
                    visible: queueRow.totalProgressKnown
                    Rectangle {
                        width: parent.width * queueRow.progress / 100
                        height: parent.height
                        radius: parent.radius
                        color: queueController.stateColor(queueRow.stateClass,
                                                    queueRow.state)
                    }
                    Accessible.role: Accessible.ProgressBar
                    Accessible.name: queueRow.taskType
                }
                Text {
                    anchors.left: parent.left
                    anchors.leftMargin: rowBusy.visible ? 28 : 0
                    anchors.right: parent.right
                    anchors.rightMargin: 18
                    anchors.top: parent.verticalCenter
                    anchors.topMargin: 3
                    text: queueRow.totalProgressKnown
                          ? Math.round(queueRow.progress) + "%"
                            + (queueRow.totalText !== ""
                               ? "  " + queueRow.totalText : "")
                          : queueRow.totalText
                    color: hostWindow.mutedText
                    elide: Text.ElideRight
                    font.pixelSize: 11
                }
                T.BusyIndicator {
                    id: rowBusy
                    anchors.left: parent.left
                    anchors.verticalCenter: parent.verticalCenter
                    width: 22
                    height: 22
                    visible: queueRow.active
                             && !queueRow.totalProgressKnown
                    running: visible
                    Accessible.name: queueRow.state
                }
            }

            Item {
                id: speedCell
                anchors.right: parent.right
                anchors.rightMargin: hostWindow.contentSpacing
                width: queueController.speedColumnWidth
                height: parent.height
                visible: width > 0
                Text {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    text: queueRow.speed
                    color: hostWindow.mutedText
                    elide: Text.ElideRight
                    font.pixelSize: 12
                }
            }

            MouseArea {
                anchors.fill: parent
                acceptedButtons: Qt.LeftButton
                hoverEnabled: false
                preventStealing: false
                scrollGestureEnabled: false
                cursorShape: Qt.PointingHandCursor
                onClicked: queueController.selectIndex(index, true)
                onDoubleClicked: {
                    queueController.selectIndex(index, true)
                    queueController.activateItem(
                        queueController.rowsModel.get(index))
                }
            }
        }

        WheelHandler {
            enabled: queueController.interactionActive
            acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
            target: null
            onWheel: (event) => {
                var delta = Number(event.pixelDelta.y)
                if (delta === 0)
                    delta = Number(event.angleDelta.y) / 120
                            * queueController.rowHeight * 2
                var maximum = Math.max(0,
                                       operationsQueueList.contentHeight
                                       - operationsQueueList.height)
                operationsQueueList.contentY = Math.max(0,
                    Math.min(maximum,
                             operationsQueueList.contentY - delta))
                event.accepted = true
            }
        }
    }

    F4ScrollBar {
        id: queueScrollBar
        objectName: "operationsQueueScrollBar"
        hostWindow: queueRoot.hostWindow
        anchors.top: operationsQueueList.top
        anchors.bottom: operationsQueueList.bottom
        anchors.right: parent.right
        enabled: queueController.interactionActive
        policy: operationsQueueList.contentHeight
                > operationsQueueList.height
                ? T.ScrollBar.AlwaysOn : T.ScrollBar.AlwaysOff
        orientation: Qt.Vertical
        size: operationsQueueList.contentHeight > 0
              ? Math.min(1, operationsQueueList.height
                         / operationsQueueList.contentHeight) : 1
        position: operationsQueueList.contentHeight
                  > operationsQueueList.height
                  ? operationsQueueList.contentY
                    / operationsQueueList.contentHeight : 0
        onPositionChanged: {
            if (!pressed || operationsQueueList.contentHeight
                    <= operationsQueueList.height)
                return
            operationsQueueList.contentY = position
                    * operationsQueueList.contentHeight
        }
        Accessible.name: hostWindow.cleanText(queue.scrollBarText
                                         || "Operations scroll bar")
    }

    Item {
        id: queueEmptyState
        objectName: "operationsQueueEmptyState"
        anchors.fill: operationsQueueList
        visible: queueController.rowsModel.count === 0
        Accessible.role: Accessible.StaticText
        Accessible.name: emptyLabel.text

        Column {
            anchors.centerIn: parent
            spacing: 8
            Text {
                id: emptyLabel
                anchors.horizontalCenter: parent.horizontalCenter
                text: hostWindow.cleanText(queue.error) !== ""
                      ? hostWindow.cleanText(queue.error)
                      : hostWindow.cleanText(queue.emptyText || "No operations")
                color: hostWindow.cleanText(queue.error) !== ""
                       ? "#ee8b8b" : hostWindow.mutedText
                font.pixelSize: 16
            }
            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: hostWindow.cleanText(queue.emptyDescription)
                visible: text !== ""
                color: hostWindow.mutedText
                font.pixelSize: 12
            }
        }
    }

    Rectangle {
        id: queueFooter
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.bottomMargin: queueController.bottomInset
        height: 54
        color: hostWindow.titleBarBg
        border.width: 1
        border.color: hostWindow.separatorColor

        QueueActionButton {
            hostWindow: queueController.hostWindow
            id: cancelButton
            objectName: "operationsQueueCancelButton"
            iconName: "circle-x"
            anchors.left: parent.left
            anchors.leftMargin: hostWindow.contentSpacing
            anchors.verticalCenter: parent.verticalCenter
            text: hostWindow.cleanText(queue.cancelText || "Cancel selected")
            enabled: {
                var row = queueController.selectedItem()
                return row !== null && row.cancellable === true
            }
            Accessible.name: text
            Accessible.description: hostWindow.cleanText(queue.cancelDescription)
            onClicked: queueController.cancelSelection()
        }

        Text {
            anchors.left: cancelButton.right
            anchors.leftMargin: 14
            anchors.right: parent.right
            anchors.rightMargin: hostWindow.contentSpacing
            anchors.verticalCenter: parent.verticalCenter
            text: hostWindow.cleanText(queue.detailsText)
            visible: text !== ""
            color: hostWindow.mutedText
            elide: Text.ElideRight
            font.pixelSize: 12
        }
    }
}
