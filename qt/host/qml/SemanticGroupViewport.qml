pragma ComponentBehavior: Bound

import QtQuick
import F4QtHost 1.0
import QtQuick.Controls

Item {
    id: root
    required property ApplicationWindow hostWindow
    required property var widget
    objectName: "dialogWidget-" + hostWindow.cleanText(widget.id) + "ViewportController"
    readonly property bool description: widget.layoutRole === "description"
    readonly property real padding: description ? 0 : hostWindow.snapPx(10)
    readonly property real scrollbarContentGap: hostWindow.snapPx(Number(widget.scrollbarContentGap || 0))
    readonly property real contentAreaWidth: Math.max(1, viewport.width
        - (scrollBar.visible ? scrollBar.width + scrollbarContentGap
                            : hostWindow.snapPx(Number(widget.unscrolledRightInset || 0))))
    readonly property real maximumScroll: Math.max(0, viewport.contentHeight - viewport.height)
    readonly property int semanticMaximum: Math.max(0, Number(widget.contentHeight || 0) - Number(widget.h || 1))
    property bool synchronizing: false
    property var submittedScrolls: []
    property int lastNativeScroll: -1
    readonly property bool nativeScrollOwner: widget.nativeScrollOwner === true
    readonly property int scrollRequestRevision: Number((widget.scrollRequest || {}).revision || 0)
    readonly property int semanticScroll: Number(widget.scrollTop || 0)
    readonly property var focused: widget.focused === true ? metrics.focusedWidget(widget.children || []) : null
    readonly property string focusedID: focused ? String(focused.id) : ""
    readonly property var wireGroups: widget.children || []
    readonly property bool groupAppearance: !!widget.appearancePreview
    readonly property real columnGap: hostWindow.snapPx(16)
    readonly property bool twoColumns: groupAppearance && contentAreaWidth >= 680
    readonly property real leftWidth: twoColumns
        ? hostWindow.snapPx((contentAreaWidth - columnGap) / 2) : contentAreaWidth
    readonly property real rightX: twoColumns ? leftWidth + columnGap : 0
    readonly property bool pairedAssociations: groupAppearance && leftWidth >= 328
    readonly property real associationGap: hostWindow.snapPx(10)
    readonly property real associationWidth: hostWindow.snapPx((leftWidth - associationGap) / 2)
    function isAssociation(group) {
        return group.layoutRole === "association-regular" || group.layoutRole === "association-alternative"
    }
    readonly property var groups: {
        if (!groupAppearance)
            return wireGroups
        const left = [], right = [], icons = []
        let colors = []
        for (const group of wireGroups) {
            if (group.layoutRole === "filegroup-colors")
                colors = (group.children || []).filter(control => control.colorMatrixCell === true)
            else if (group.sectionID === "filegroup-Icons" || group.layoutRole === "filegroup-icons")
                icons.push(Object.assign({}, group, {collapsible: false}))
            else if (group.sectionID === "filegroup-Highlighting" || group.layoutRole === "filegroup-highlight")
                right.push(Object.assign({}, group, {appearanceColumn: true, conditionColumn: true}))
            else
                left.push(Object.assign({}, group, {conditionColumn: true}))
        }
        right.splice(1, 0, Object.assign({}, widget.appearancePreview, {
            appearanceColumn: true, previewRows: colors, iconGroups: icons
        }))
        return left.concat(right)
    }
    readonly property var appearancePlacement: {
        if (!groupAppearance)
            return {tops: [], height: 0}
        const tops = []
        let left = 0, right = 0
        for (let i = 0; i < groups.length; ++i) {
            const inRight = groups[i].appearanceColumn === true
            if (inRight && !twoColumns && right === 0)
                right = left
            tops.push(inRight ? right : left)
            const height = naturalHeightAt(i)
            if (pairedAssociations && groups[i].layoutRole === "association-regular"
                    && groups[i + 1] && groups[i + 1].layoutRole === "association-alternative") {
                tops.push(left)
                left += Math.max(height, naturalHeightAt(++i)) + associationGap
            } else if (inRight)
                right += height + hostWindow.snapPx(10)
            else
                left += height + hostWindow.snapPx(10)
        }
        return {tops: tops, height: Math.max(left, right)}
    }
    function naturalHeightAt(index) {
        const row = rows.revision > 0 ? rows.itemAt(index) : null
        return row && row.item ? row.item.naturalGroupHeight : 0
    }
    readonly property var conditionPlacement: {
        const left = groups.findIndex(group => group.conditionColumn === true && group.x === widget.x)
        if (groupAppearance || left < 0)
            return {tops: [], growth: 0, end: -1, shift: 0}
        let end = left + 1
        const tops = []
        const top = metrics.widgetTop(groups[left])
        const gap = hostWindow.snapPx(8)
        let rightBottom = top
        const leftItem = rows.revision > 0 ? rows.itemAt(left) : null
        const leftBottom = top + (leftItem && leftItem.item ? leftItem.item.naturalGroupHeight : 0)
        while (end < groups.length && groups[end].conditionColumn === true) {
            const row = rows.itemAt(end)
            tops[end] = rightBottom
            rightBottom += (row && row.item ? row.item.naturalGroupHeight : 0) + gap
            ++end
        }
        const bottom = Math.max(leftBottom, rightBottom - gap)
        const nextTop = end < groups.length ? metrics.widgetTop(groups[end]) : metrics.contentHeight
        return {tops: tops, growth: Math.max(0, bottom - leftBottom), end: end,
                shift: bottom + hostWindow.snapPx(12) - nextTop}
    }
    readonly property real contentHeight: groupAppearance ? appearancePlacement.height
        : Math.max(0, metrics.contentHeight + conditionPlacement.shift)

    SemanticDialogLayout {
        id: metrics
        hostWindow: root.hostWindow
        widgets: root.widget.children || []
        naturalText: root.description
        originX: Number(root.widget.x || 0)
        originY: Number(root.widget.y || 0)
        maximumWidth: root.contentAreaWidth
    }

    function synchronizeScroll(top = semanticScroll) {
        wheelScroll.cancel()
        synchronizing = true
        viewport.contentY = hostWindow.snapPx(semanticMaximum > 0
                            ? maximumScroll * Math.min(semanticMaximum, top) / semanticMaximum : 0)
        synchronizing = false
    }
    function commitScroll() {
        if (widget.localScrollOnly === true || synchronizing || maximumScroll <= 0 || semanticMaximum <= 0)
            return
        const value = Math.round(viewport.contentY / maximumScroll * semanticMaximum)
        if (nativeScrollOwner) {
            if (value === lastNativeScroll)
                return
            lastNativeScroll = value
        } else {
            const lastSubmitted = submittedScrolls.length > 0
                ? submittedScrolls[submittedScrolls.length - 1] : semanticScroll
            if (value === lastSubmitted)
                return
            submittedScrolls.push(value)
        }
        hostWindow.action({"target": widget.id, "action": "control.scroll", "value": value}, true)
    }
    function clampScroll() {
        // Layout can settle after native scrolling starts, before its debounced
        // position reaches Go. New geometry must not restore that stale offset.
        const bounded = Math.max(0, Math.min(maximumScroll, viewport.contentY))
        if (Math.abs(bounded - viewport.contentY) < .001)
            return
        synchronizing = true
        viewport.contentY = hostWindow.snapPx(bounded)
        synchronizing = false
    }
    function ensureFocusVisible() {
        if (!focused)
            return
        // Use the displayed geometry when condition groups have independent
        // row metrics and the semantic coordinates still describe the terminal.
        const item = content.findDialogControl(focusedID)
        if (groupAppearance && !item)
            return
        const top = item ? item.mapToItem(content, 0, 0).y + padding : padding + metrics.widgetTop(focused)
        const bottom = top + (item ? item.height : metrics.widgetHeight(focused))
        if (top < viewport.contentY || bottom > viewport.contentY + viewport.height) {
            wheelScroll.cancel()
            viewport.contentY = hostWindow.snapPx(top < viewport.contentY
                ? Math.max(0, top - padding) : Math.min(maximumScroll, bottom - viewport.height + padding))
        }
    }
    onScrollRequestRevisionChanged: {
        if (nativeScrollOwner && scrollRequestRevision > 0) {
            lastNativeScroll = -1
            Qt.callLater(() => {
                synchronizeScroll(Number(widget.scrollRequest.top))
                commitScroll()
            })
        }
    }
    onSemanticScrollChanged: {
        // In the single-page editor scrollTop is a mirror of the native
        // viewport. A delayed frame must not drive the viewport backwards.
        if (nativeScrollOwner)
            return
        // The core can coalesce updates and acknowledge a position after the
        // user has already scrolled farther. Keep the native pixel position.
        const acknowledged = submittedScrolls.indexOf(semanticScroll)
        if (acknowledged >= 0) {
            submittedScrolls = submittedScrolls.slice(acknowledged + 1)
            return
        }
        submittedScrolls = []
        Qt.callLater(synchronizeScroll)
    }
    onMaximumScrollChanged: Qt.callLater(clampScroll)
    onFocusedIDChanged: Qt.callLater(ensureFocusVisible)
    Component.onCompleted: Qt.callLater(synchronizeScroll)

    Flickable {
        id: viewport
        objectName: "dialogWidget-" + root.hostWindow.cleanText(root.widget.id) + "Viewport"
        anchors.fill: parent
        clip: true
        contentWidth: width
        contentHeight: Math.max(height, root.hostWindow.snapPx(root.contentHeight + 2 * root.padding))
        boundsBehavior: Flickable.StopAtBounds
        pixelAligned: true
        flickableDirection: Flickable.VerticalFlick
        onMovementEnded: root.commitScroll()
        interactive: root.maximumScroll > 0
        DialogWheelScroll {
            id: wheelScroll
            viewport: viewport
            hostWindow: root.hostWindow
            onScrolled: {
                if (running) scrollCommit.stop()
                else scrollCommit.restart()
            }
            onSettled: root.commitScroll()
        }
        onContentYChanged: {
            if (!root.synchronizing && (moving || scrollBar.pressed))
                scrollCommit.restart()
        }
        Timer {
            id: scrollCommit
            interval: 100
            onTriggered: root.commitScroll()
        }
        ScrollBar.vertical: F4ScrollBar {
            id: scrollBar
            objectName: "dialogWidget-" + root.hostWindow.cleanText(root.widget.id) + "ViewportScrollBar"
            policy: ScrollBar.AlwaysOn
            hostWindow: root.hostWindow
            visible: root.maximumScroll > 0
            onPressedChanged: if (!pressed) root.commitScroll()
        }
        Item {
            id: content
            y: root.padding
            width: root.contentAreaWidth
            height: root.contentHeight
            function findDialogControl(id) {
                const name = "dialogWidget-" + root.hostWindow.cleanText(id) + "Root"
                function find(item) {
                    if (item.objectName === name)
                        return item
                    for (const child of item.children) {
                        const control = find(child)
                        if (control)
                            return control
                    }
                    return null
                }
                return find(content)
            }
            // Snap the scroll translation as a whole before native text, icons
            // and control backgrounds inherit it, including wheel rest states.
            readonly property point dialogPixelTranslation: Qt.point(pixelTranslation.x, pixelTranslation.y)
            transform: Translate {
                id: pixelTranslation
                x: root.hostWindow.dialogPixelOffsetX(content, root.hostWindow.contentItem)
                y: wheelScroll.running ? 0 : root.hostWindow.dialogPixelOffsetY(content, root.hostWindow.contentItem)
            }
            Repeater {
                id: rows
                property int revision: 0
                onItemAdded: revision++
                onItemRemoved: revision++
                model: SemanticChildrenModel { widgets: root.groups }
                delegate: Loader {
                    id: childLoader
                    required property var widgetData
                    required property var labelData
                    required property int index
                    readonly property var placement: root.conditionPlacement
                    readonly property bool pairedAssociation: root.pairedAssociations && root.isAssociation(widgetData)
                    x: root.groupAppearance && widgetData.appearanceColumn === true ? root.rightX
                        : pairedAssociation && widgetData.layoutRole === "association-alternative"
                            ? root.associationWidth + root.associationGap : 0
                    y: root.groupAppearance ? (root.appearancePlacement.tops[index] || 0)
                        : placement.tops[index] !== undefined ? placement.tops[index] - metrics.widgetTop(widgetData)
                        : placement.end >= 0 && index >= placement.end ? placement.shift : 0
                    readonly property real availableWidth: root.groupAppearance
                        ? (widgetData.appearanceColumn === true ? content.width - root.rightX
                            : pairedAssociation ? root.associationWidth : root.leftWidth)
                        : Math.max(1, content.width - root.hostWindow.pxX(Number(widgetData.x || 0) - Number(root.widget.x || 0)))
                    width: root.groupAppearance ? availableWidth : root.description || (widgetData.kind === "group" && widgetData.conditionColumn !== true)
                        ? availableWidth : Math.min(root.hostWindow.pxW(widgetData.w || 1), availableWidth)
                    Component.onCompleted: setSource(Qt.resolvedUrl("SemanticWidgetDelegate.qml"), {
                            "hostWindow": root.hostWindow,
                            "widget": Qt.binding(() => childLoader.widgetData),
                            "labelData": Qt.binding(() => childLoader.labelData),
                            "dialogLayout": metrics,
                            "groupBodyGrowth": Qt.binding(() => childLoader.widgetData.conditionColumn === true
                            && childLoader.widgetData.x === root.widget.x ? childLoader.placement.growth : 0),
                            "originX": Qt.binding(() => Number((root.groupAppearance ? childLoader.widgetData : root.widget).x || 0)),
                            "originY": Qt.binding(() => Number(root.widget.y || 0) - 1),
                            "maximumWidth": Qt.binding(() => childLoader.availableWidth),
                            "positionedByParent": Qt.binding(() => root.groupAppearance)
                    })
                }
            }
        }
    }
}
