pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQuick.Shapes
import ZoinGallery 1.0 as ZG

Item {
    id: workspaceBar
    required property ApplicationWindow hostWindow
    required property real availableWidth
    required property QtObject nativeWindowAgent
    required property bool nativeWindowAgentReady
    required property bool usesQwk
    objectName: "workspaceBar"
    anchors.bottom: parent.bottom
    width: visible
           ? hostWindow.snapPx(Math.min(availableWidth * 0.46,
                                  workspaceItemsRow.width))
           : 0
    height: hostWindow.snapPx(36)
    visible: hostWindow.workspaceTabs.visible === true
    opacity: hostWindow.normalSurfaceOpacity
    z: 2
    property Item activeWorkspaceTab: null
    property int activeWorkspaceTabSeparatorRevision: 0
    property bool activeWorkspaceTabUpdatePending: false
    property string wheelNavigationModelSignature: ""
    property int wheelNavigationIndex: -1
    property int wheelNavigationAuthoritativeIndex: -1

    function registerNativeHitTargets() {
        if (!usesQwk || !nativeWindowAgentReady)
            return
        for (var i = 0; i < workspaceTabsRepeater.count; ++i) {
            var item = workspaceTabsRepeater.itemAt(i)
            if (item)
                item.registerNativeHitTarget()
        }
        workspaceNew.registerNativeHitTarget()
    }

    onNativeWindowAgentReadyChanged: registerNativeHitTargets()

    function workspaceTabModelSignature(tabs) {
        var parts = []
        for (var i = 0; i < tabs.length; ++i) {
            var tab = tabs[i] || ({})
            parts.push(hostWindow.cleanText(tab.id))
        }
        return parts.join("\u001f")
    }

    function authoritativeWorkspaceIndex(tabs) {
        var activeIndex = Number(hostWindow.workspaceTabs.activeIndex)
        if (Math.floor(activeIndex) === activeIndex
                && activeIndex >= 0
                && activeIndex < tabs.length)
            return activeIndex
        for (var i = 0; i < tabs.length; ++i) {
            if (tabs[i] && tabs[i].active === true)
                return i
        }
        return 0
    }

    function activateAdjacentWorkspaceTab(direction) {
        var tabs = hostWindow.workspaces || []
        if (tabs.length < 2)
            return false

        var authoritativeIndex = authoritativeWorkspaceIndex(tabs)
        var signature = workspaceTabModelSignature(tabs)
        if (signature !== wheelNavigationModelSignature
                || wheelNavigationIndex < 0
                || wheelNavigationIndex >= tabs.length
                || authoritativeIndex
                   !== wheelNavigationAuthoritativeIndex) {
            wheelNavigationModelSignature = signature
            wheelNavigationIndex = authoritativeIndex
            wheelNavigationAuthoritativeIndex = authoritativeIndex
        }

        var nextIndex = wheelNavigationIndex + direction
        if (nextIndex < 0 || nextIndex >= tabs.length)
            return true
        wheelNavigationIndex = nextIndex

        var tab = tabs[nextIndex] || ({})
        hostWindow.action({
            "target": hostWindow.cleanText(tab.id),
            "action": hostWindow.cleanText(tab.action)
                      || "workspace.activate",
            "index": nextIndex
        }, true)
        return true
    }

    function refreshActiveWorkspaceTabGeometry() {
        ++activeWorkspaceTabSeparatorRevision
    }

    function updateActiveWorkspaceTabNow() {
        var nextTab = null
        var activeIndex = Number(hostWindow.workspaceTabs.activeIndex)
        if (Math.floor(activeIndex) === activeIndex
                && activeIndex >= 0
                && activeIndex < workspaceTabsRepeater.count) {
            nextTab = workspaceTabsRepeater.itemAt(activeIndex)
        }
        if (!nextTab) {
            for (var i = 0; i < workspaceTabsRepeater.count; ++i) {
                var candidate = workspaceTabsRepeater.itemAt(i)
                if (candidate && candidate.current) {
                    nextTab = candidate
                    break
                }
            }
        }
        activeWorkspaceTab = nextTab
        refreshActiveWorkspaceTabGeometry()
    }

    function updateActiveWorkspaceTab() {
        if (activeWorkspaceTabUpdatePending)
            return
        activeWorkspaceTabUpdatePending = true
        Qt.callLater(function() {
            activeWorkspaceTabUpdatePending = false
            workspaceBar.updateActiveWorkspaceTabNow()
        })
    }

    readonly property real activeWorkspaceTabLeft:
        activeWorkspaceTabSeparatorRevision >= 0
        && activeWorkspaceTab
        ? Math.max(0, Math.min(workspaceBar.width,
              activeWorkspaceTab.x + workspaceItemsRow.x
              - workspaceFlick.contentX))
        : 0
    readonly property real activeWorkspaceTabRight:
        activeWorkspaceTabSeparatorRevision >= 0
        && activeWorkspaceTab
        ? Math.max(0, Math.min(workspaceBar.width,
              activeWorkspaceTab.x + workspaceItemsRow.x
              - workspaceFlick.contentX
              + activeWorkspaceTab.width))
        : 0

    onXChanged: refreshActiveWorkspaceTabGeometry()
    onWidthChanged: refreshActiveWorkspaceTabGeometry()

    MouseArea {
        id: workspaceTabWheelArea
        objectName: "workspaceTabWheelArea"
        anchors.fill: parent
        acceptedButtons: Qt.NoButton
        hoverEnabled: false
        preventStealing: false
        enabled: workspaceBar.visible
        onWheel: (wheel) => {
            var delta = Number(wheel.angleDelta.y)
            if (delta === 0)
                delta = Number(wheel.pixelDelta.y)
            if (!(delta > 0 || delta < 0))
                return
            // Wheel up selects the previous tab; wheel down selects
            // the next one, with the same wraparound as Ctrl+Tab.
            wheel.accepted = workspaceBar.activateAdjacentWorkspaceTab(
                delta > 0 ? -1 : 1)
        }
    }

    Flickable {
        id: workspaceFlick
        anchors.fill: parent
        contentWidth: workspaceItemsRow.width
        contentHeight: height
        boundsBehavior: Flickable.StopAtBounds
        flickableDirection: Flickable.HorizontalFlick
        pixelAligned: true
        clip: true

        function revealCurrentWorkspace() {
            contentX = Math.max(0, contentWidth - width)
        }

        onContentWidthChanged: {
            revealCurrentWorkspace()
            workspaceBar.refreshActiveWorkspaceTabGeometry()
        }
        onContentXChanged:
            workspaceBar.refreshActiveWorkspaceTabGeometry()
        onWidthChanged: {
            revealCurrentWorkspace()
            workspaceBar.refreshActiveWorkspaceTabGeometry()
        }

        Row {
            id: workspaceItemsRow
            width: childrenRect.width
            height: parent.height
            spacing: hostWindow.snapPx(4)

            Repeater {
                id: workspaceTabsRepeater
                model: hostWindow.workspaces
                onItemAdded: workspaceBar.updateActiveWorkspaceTab()
                onItemRemoved: workspaceBar.updateActiveWorkspaceTab()

                delegate: Rectangle {
                    id: workspaceTab
                    required property var modelData
                    required property int index
                    readonly property bool current: modelData.active === true
                    readonly property bool closeEnabled:
                        hostWindow.workspaceTabCanClose(modelData)
                    readonly property string tabIconName:
                        hostWindow.workspaceTabIconName(modelData)
                    readonly property string lucideName: tabIconName
                    readonly property color labelColor:
                        hostWindow.workspaceTabTextColor(current)
                    readonly property int labelWeight:
                        hostWindow.workspaceTabFontWeight()
                    readonly property bool hoverActive:
                        workspaceHover.hovered
                    objectName: String(modelData.id || ("workspace-tab-" + index))
                    width: hostWindow.preferredWorkspaceTabWidth(
                                workspaceLabel.implicitWidth,
                                workspaceTab.closeEnabled)
                    height: parent.height
                    z: current ? 2 : 0
                    radius: 6
                    topLeftRadius: 6
                    topRightRadius: 6
                    bottomLeftRadius: 0
                    bottomRightRadius: 0
                    antialiasing: true
                    smooth: true
                    color: current
                           ? hostWindow.panelPathBg
                           : workspaceHover.hovered
                             ? hostWindow.controlHoverBg : "transparent"
                    border.width: 0

                    // The active tab joins the panel below: its
                    // native background keeps the upper corners,
                    // while its outline is deliberately open at
                    // the bottom.
                    Shape {
                        id: workspaceTabBorder
                        anchors.fill: parent
                        visible: workspaceTab.current
                        z: 1
                        antialiasing: true
                        smooth: true
                        preferredRendererType: Shape.CurveRenderer
                        readonly property real borderHalf:
                            hostWindow.separatorWidth / 2
                        readonly property real borderRadius:
                            6 - borderHalf
                        readonly property real borderCurveFactor:
                            borderRadius * 0.5522848

                        ShapePath {
                            strokeColor: hostWindow.separatorColor
                            strokeWidth: hostWindow.separatorWidth
                            fillColor: "transparent"
                            capStyle: ShapePath.FlatCap
                            joinStyle: ShapePath.RoundJoin
                            startX: workspaceTabBorder.borderHalf
                            startY: workspaceTab.height

                            PathLine {
                                x: workspaceTabBorder.borderHalf
                                y: 6
                            }
                            PathCubic {
                                control1X: workspaceTabBorder.borderHalf
                                control1Y: 6
                                             - workspaceTabBorder.borderCurveFactor
                                control2X: 6
                                             - workspaceTabBorder.borderCurveFactor
                                control2Y: workspaceTabBorder.borderHalf
                                x: 6
                                y: workspaceTabBorder.borderHalf
                            }
                            PathLine {
                                x: workspaceTab.width - 6
                                y: workspaceTabBorder.borderHalf
                            }
                            PathCubic {
                                control1X: workspaceTab.width - 6
                                             + workspaceTabBorder.borderCurveFactor
                                control1Y: workspaceTabBorder.borderHalf
                                control2X: workspaceTab.width
                                             - workspaceTabBorder.borderHalf
                                control2Y: 6
                                             - workspaceTabBorder.borderCurveFactor
                                x: workspaceTab.width
                                   - workspaceTabBorder.borderHalf
                                y: 6
                            }
                            PathLine {
                                x: workspaceTab.width
                                   - workspaceTabBorder.borderHalf
                                y: workspaceTab.height
                            }
                        }
                    }

                    Rectangle {
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.bottom: parent.bottom
                        height: hostWindow.separatorWidth
                        color: hostWindow.separatorColor
                        visible: !workspaceTab.current
                        z: 1
                        antialiasing: false
                    }

                    // Keep the divider in the existing spacing:
                    // this child is painted outside the tab but
                    // never participates in the Row geometry.
                    Rectangle {
                        objectName: workspaceTab.objectName + "-divider"
                        width: hostWindow.separatorWidth
                        height: Math.max(0, parent.height - 12)
                        anchors.left: parent.right
                        anchors.leftMargin:
                            (workspaceItemsRow.spacing - width) / 2
                        anchors.verticalCenter: parent.verticalCenter
                        color: hostWindow.separatorColor
                        visible: !workspaceTab.current
                                 && !workspaceTab.hoverActive
                                 && index + 1
                                    < workspaceTabsRepeater.count
                                 && workspaceTabsRepeater.itemAt(index + 1)
                                 && !workspaceTabsRepeater.itemAt(
                                        index + 1).hoverActive
                                 && !workspaceTabsRepeater.itemAt(
                                        index + 1).current
                        z: 2
                    }

                    onCurrentChanged:
                        workspaceBar.updateActiveWorkspaceTab()
                    onWidthChanged:
                        workspaceBar.refreshActiveWorkspaceTabGeometry()

                    function registerNativeHitTarget() {
                        if (workspaceTab.nativeHitTargetRegistered
                                || !workspaceBar.usesQwk
                                || !workspaceBar.nativeWindowAgentReady)
                            return
                        workspaceBar.nativeWindowAgent.setHitTestVisible(
                                    workspaceTab)
                        workspaceTab.nativeHitTargetRegistered = true
                    }

                    property bool nativeHitTargetRegistered: false

                    Component.onCompleted: {
                        workspaceBar.updateActiveWorkspaceTab()
                        registerNativeHitTarget()
                    }

                    HoverHandler {
                        id: workspaceHover
                    }

                    ZG.ToolTip {
                        objectName: "workspace-tab-tooltip-"
                                    + workspaceTab.objectName
                        visible: workspaceHover.hovered
                        delay: 500
                        timeout: 5000
                        text: hostWindow.workspaceTabToolTip(
                                  modelData, Qt.platform.os)
                    }

                    HostPixelAlignedImage {
                        hostWindow: workspaceBar.hostWindow
                        id: workspaceIcon
                        objectName: "workspace-tab-icon-"
                                    + hostWindow.cleanText(modelData.id)
                        anchors.left: parent.left
                        anchors.leftMargin: hostWindow.snapPx(10)
                        y: hostWindow.snapPx((parent.height - height) / 2)
                        width: hostWindow.snapPx(16)
                        height: hostWindow.snapPx(16)
                        alignmentRevision: workspaceTab.x
                                           + workspaceTab.y
                                           + workspaceTab.width
                                           + workspaceTab.height
                        smooth: false
                        source: hostWindow.lucideIconSource(
                                    workspaceTab.tabIconName, 16,
                                    workspaceTab.labelColor)
                    }

                    Item {
                        id: workspaceLabel
                        objectName: "workspace-tab-label-"
                                    + workspaceTab.objectName
                        anchors.left: workspaceIcon.right
                        anchors.leftMargin: hostWindow.snapPx(7)
                        anchors.right: workspaceAttention.visible
                                       ? workspaceAttention.left
                                       : workspaceClose.visible
                                         ? workspaceClose.left
                                         : parent.right
                        anchors.rightMargin: hostWindow.snapPx(6)
                        y: hostWindow.snapPx((parent.height - height) / 2)
                        height: hostWindow.snapPx(
                                    Math.max(
                                        workspaceNumber.implicitHeight,
                                        workspaceTitle.implicitHeight))
                        implicitWidth: workspaceNumber.implicitWidth
                                       + (workspaceTitle.text === ""
                                          ? 0 : 5)
                                       + workspaceTitle.implicitWidth

                        Text {
                            id: workspaceTitle
                            objectName: "workspace-tab-title-"
                                        + workspaceTab.objectName
                            anchors.left: parent.left
                            anchors.right: workspaceNumber.left
                            anchors.rightMargin: text === "" ||
                                                 workspaceNumber.text === ""
                                                 ? 0 : 5
                            y: hostWindow.snapPx((parent.height - height) / 2)
                            text: hostWindow.cleanText(modelData.text)
                            color: workspaceTab.labelColor
                            // Active state is conveyed only by text
                            // brightness; changing weight makes tabs
                            // shift visually and breaks typographic
                            // consistency across the title bar.
                            font.weight: workspaceTab.labelWeight
                            elide: Text.ElideMiddle
                        }

                        Text {
                            id: workspaceNumber
                            objectName: "workspace-tab-number-"
                                        + workspaceTab.objectName
                            anchors.right: parent.right
                            y: hostWindow.snapPx((parent.height - height) / 2)
                            text: Number(modelData.number || 0) > 0
                                  ? String(modelData.number) : ""
                            color: hostWindow.workspaceTabNumberColor()
                            opacity: workspaceTab.current ? 0.9 : 0.76
                            font.weight: workspaceTab.labelWeight
                        }
                    }

                    Rectangle {
                        id: workspaceAttention
                        anchors.right: parent.right
                        anchors.rightMargin: workspaceClose.visible ? hostWindow.snapPx(29) : hostWindow.snapPx(10)
                        anchors.verticalCenter: parent.verticalCenter
                        width: hostWindow.snapPx(6)
                        height: hostWindow.snapPx(6)
                        radius: 3
                        color: hostWindow.dialogAccent
                        visible: modelData.attention === true
                    }

                    HostPixelAlignedImage {
                        hostWindow: workspaceBar.hostWindow
                        id: workspaceClose
                        objectName: "workspace-close-"
                                    + hostWindow.cleanText(modelData.id)
                        z: 2
                        x: hostWindow.snapPx(parent.width - width - 8)
                        y: hostWindow.snapPx((parent.height - height) / 2)
                        width: hostWindow.snapPx(14)
                        height: hostWindow.snapPx(14)
                        alignmentRevision: workspaceTab.x
                                           + workspaceTab.y
                                           + workspaceTab.width
                                           + workspaceTab.height
                        smooth: false
                        source: hostWindow.lucideIconSource(
                                    "x", 14,
                                    workspaceTab.labelColor)
                        visible: workspaceTab.closeEnabled

                        MouseArea {
                            anchors.fill: parent
                            anchors.margins: -6
                            cursorShape: Qt.PointingHandCursor
                            onClicked: function(mouse) {
                                mouse.accepted = true
                                hostWindow.action({
                                    "target": modelData.id,
                                    "action": modelData.closeAction,
                                    "index": modelData.index
                                }, true)
                            }
                        }
                    }

                    Rectangle {
                        anchors.left: parent.left
                        anchors.leftMargin: 6
                        anchors.bottom: parent.bottom
                        anchors.bottomMargin: 2
                        width: Math.max(0, parent.width - 12)
                               * Math.max(0, Math.min(100,
                                   Number(modelData.progress))) / 100
                        height: 2
                        radius: 1
                        color: hostWindow.dialogAccent
                        visible: Number(modelData.progress) >= 0
                    }

                    MouseArea {
                        anchors.fill: parent
                        cursorShape: Qt.PointingHandCursor
                        onClicked: hostWindow.action({
                            "target": modelData.id,
                            "action": modelData.action,
                            "index": modelData.index
                        }, true)
                    }
                }
            }

            Rectangle {
                id: workspaceNew
                readonly property bool qwkHitTestRegistered:
                    !usesQwk || workspaceNewHitTestRegistered
                property bool workspaceNewHitTestRegistered: false
                objectName: hostWindow.cleanText(hostWindow.workspaceTabs.newTab
                                           ? hostWindow.workspaceTabs.newTab.id : "workspace-new")
                width: visible ? hostWindow.snapPx(30) : 0
                height: parent.height
                radius: 0
                topLeftRadius: 6
                topRightRadius: 6
                bottomLeftRadius: 0
                bottomRightRadius: 0
                antialiasing: true
                smooth: true
                color: newHover.hovered ? hostWindow.controlHoverBg : "transparent"
                visible: !!hostWindow.workspaceTabs.newTab
                         && hostWindow.workspaceTabs.newTab.visible === true

                function registerNativeHitTarget() {
                    if (workspaceNewHitTestRegistered
                            || !workspaceBar.usesQwk
                            || !workspaceBar.nativeWindowAgentReady)
                        return
                    workspaceBar.nativeWindowAgent.setHitTestVisible(
                                workspaceNew)
                    workspaceNewHitTestRegistered = true
                }

                Component.onCompleted: registerNativeHitTarget()

                Rectangle {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    height: hostWindow.separatorWidth
                    color: hostWindow.separatorColor
                    z: 1
                    antialiasing: false
                }

                HostPixelAlignedImage {
                    hostWindow: workspaceBar.hostWindow
                    x: hostWindow.snapPx((parent.width - width) / 2)
                    y: hostWindow.snapPx((parent.height - height) / 2)
                    width: hostWindow.snapPx(16)
                    height: hostWindow.snapPx(16)
                    alignmentRevision: workspaceNew.x
                                       + workspaceNew.y
                                       + workspaceNew.width
                                       + workspaceNew.height
                    smooth: false
                    source: hostWindow.lucideIconSource(
                                "plus", 16, hostWindow.chromeText)
                }
                HoverHandler { id: newHover }
                MouseArea {
                    anchors.fill: parent
                    cursorShape: Qt.PointingHandCursor
                    onClicked: hostWindow.action({
                        "target": hostWindow.workspaceTabs.newTab.id,
                        "action": hostWindow.workspaceTabs.newTab.action
                    }, true)
                }
            }

        }
    }

    Rectangle {
        id: workspaceTabSeparatorLeft
        objectName: "workspaceTabSeparatorLeft"
        anchors.left: parent.left
        anchors.bottom: parent.bottom
        width: workspaceBar.activeWorkspaceTabLeft
        height: hostWindow.separatorWidth
        color: hostWindow.separatorColor
        visible: workspaceBar.visible
                 && workspaceBar.activeWorkspaceTab !== null
        z: -1
        antialiasing: false
    }

    Rectangle {
        id: workspaceTabSeparatorRight
        objectName: "workspaceTabSeparatorRight"
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        width: Math.max(0, parent.width
                           - workspaceBar.activeWorkspaceTabRight)
        height: hostWindow.separatorWidth
        color: hostWindow.separatorColor
        visible: workspaceBar.visible
                 && workspaceBar.activeWorkspaceTab !== null
        z: -1
        antialiasing: false
    }
}
