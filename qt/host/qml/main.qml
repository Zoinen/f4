import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import F4QtHost 1.0

ApplicationWindow {
    id: root

    width: Math.max(720, Math.ceil(qtShell.initialCols * grid.cellWidth))
    height: Math.max(460, Math.ceil(qtShell.initialRows * grid.cellHeight))
    visible: true
    title: "f4"
    color: "#101318"

    property var scene: qtShell.scene || ({})
    property real cw: Math.max(8, grid.cellWidth)
    property real ch: Math.max(17, grid.cellHeight)
    property color panelBg: "#141922"
    property color panelBgAlt: "#10151d"
    property color panelBorder: "#4e9bd4"
    property color activeBorder: "#f0c95a"
    property color textColor: "#e8edf2"
    property color mutedText: "#9aa7b5"
    property color selectedBg: "#285d8f"
    property color markedBg: "#4f5037"
    property color chromeBg: "#202833"
    property color chromeText: "#d7e0ea"

    function frames() {
        return scene.frames || []
    }

    function firstFrame(kind) {
        var list = frames()
        for (var i = list.length - 1; i >= 0; --i) {
            if (list[i].kind === kind)
                return list[i]
        }
        return null
    }

    function topFrame() {
        var list = frames()
        return list.length > 0 ? list[list.length - 1] : null
    }

    function needsFallbackGrid() {
        var top = topFrame()
        if (!top)
            return true
        return top.fallback === true || top.kind === "fallback" || containsFallback(top)
    }

    function containsFallback(node) {
        if (!node)
            return false
        if (node.fallback === true || node.kind === "fallback" || node.kind === "fallbackWidget")
            return true
        var children = node.children || []
        for (var i = 0; i < children.length; ++i) {
            if (containsFallback(children[i]))
                return true
        }
        return false
    }

    function pxX(value) { return Math.round((value || 0) * cw) }
    function pxY(value) { return Math.round((value || 0) * ch) }
    function pxW(value) { return Math.max(1, Math.round((value || 0) * cw)) }
    function pxH(value) { return Math.max(1, Math.round((value || 0) * ch)) }

    function cleanText(value) {
        return value === undefined || value === null ? "" : String(value)
    }

    function runsText(runs) {
        if (!runs)
            return ""
        var out = ""
        for (var i = 0; i < runs.length; ++i)
            out += cleanText(runs[i].text)
        return out
    }

    function rowText(row) {
        if (!row)
            return ""
        if (row.text !== undefined)
            return cleanText(row.text)
        return runsText(row.runs)
    }

    function action(map) {
        qtShell.sendUiAction(map)
        grid.forceActiveFocus()
    }

    onClosing: qtShell.sendQuit()

    VtuiGridItem {
        id: grid
        anchors.fill: parent
        controller: qtShell
        focus: true
        opacity: root.needsFallbackGrid() ? 1.0 : 0.0
        visible: true
        Component.onCompleted: forceActiveFocus()
    }

    Item {
        id: semanticLayer
        anchors.fill: parent
        visible: !root.needsFallbackGrid()

        Rectangle {
            anchors.fill: parent
            color: root.panelBgAlt
        }

        SemanticMenuBar {
            id: semanticMenu
            menu: root.scene.menuBar || ({})
            anchors.left: parent.left
            anchors.right: parent.right
            height: Math.max(28, root.ch * 1.35)
            z: 20
        }

        Loader {
            id: mainSurface
            anchors.fill: parent
            sourceComponent: {
                var top = root.topFrame()
                if (top && top.kind === "viewer")
                    return documentSurface
                if (top && top.kind === "editor")
                    return documentSurface
                if (top && top.kind === "terminal")
                    return documentSurface
                return panelsSurface
            }
        }

        Component {
            id: panelsSurface
            Item {
                id: panelsRoot
                anchors.fill: parent
                property var frame: root.firstFrame("panels") || ({})
                property var panelList: frame.panels || []

                Loader {
                    anchors.fill: parent
                    sourceComponent: frame.terminalActive ? embeddedTerminalSurface : panelPairSurface
                }

                CommandLineView {
                    commandLine: frame.commandLine || ({})
                }

                Component {
                    id: panelPairSurface
                    Item {
                        anchors.fill: parent
                        Repeater {
                            model: panelList
                            delegate: FilePanelView {
                                panel: modelData
                            }
                        }
                    }
                }

                Component {
                    id: embeddedTerminalSurface
                    DocumentSurface {
                        frame: panelsRoot.frame.terminal || ({})
                    }
                }
            }
        }

        Component {
            id: documentSurface
            DocumentSurface {
                frame: root.topFrame() || ({})
            }
        }

        Repeater {
            model: root.frames()
            delegate: Loader {
                property var frame: modelData
                active: frame.kind === "dialog" || frame.kind === "window" || frame.kind === "menu"
                sourceComponent: frame.kind === "menu" ? menuPopupComponent : dialogComponent
                onLoaded: {
                    if (item && item.frame !== undefined)
                        item.frame = frame
                }
                z: 100 + index
            }
        }

        KeyBarView {
            keyBar: root.scene.keyBar || ({})
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: Math.max(26, root.ch * 1.35)
            z: 40
        }

        ToastView {
            toast: root.scene.toast || ({})
            anchors.horizontalCenter: parent.horizontalCenter
            y: semanticMenu.height + 8
            z: 200
        }
    }

    component SemanticMenuBar: Rectangle {
        property var menu: ({})
        color: root.chromeBg
        visible: menu.items !== undefined

        Row {
            anchors.fill: parent
            anchors.leftMargin: 8
            spacing: 2
            Repeater {
                model: menu.items || []
                delegate: Rectangle {
                    height: parent.height
                    width: Math.max(68, label.implicitWidth + 24)
                    color: modelData.index === menu.selected && menu.active ? root.selectedBg : "transparent"

                    Text {
                        id: label
                        anchors.centerIn: parent
                        text: root.cleanText(modelData.text)
                        color: modelData.disabled ? root.mutedText : root.chromeText
                        font.pixelSize: 13
                    }

                    MouseArea {
                        anchors.fill: parent
                        onClicked: {
                            if (modelData.command)
                                root.action({ "action": "emit_command", "command": modelData.command })
                        }
                    }
                }
            }
        }
    }

    component FilePanelView: Rectangle {
        id: panelRoot
        property var panel: ({})

        x: root.pxX(panel.x)
        y: root.pxY(panel.y)
        width: root.pxW(panel.w)
        height: root.pxH(panel.h)
        color: panel.active ? root.panelBg : root.panelBgAlt
        border.width: panel.active ? 2 : 1
        border.color: panel.active ? root.activeBorder : root.panelBorder
        clip: true

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: Math.max(25, root.ch * 1.25)
            color: panel.active ? "#26364a" : "#1c2531"

            Text {
                anchors.left: parent.left
                anchors.right: loading.left
                anchors.verticalCenter: parent.verticalCenter
                anchors.leftMargin: 8
                anchors.rightMargin: 8
                text: root.cleanText(panel.path)
                color: root.textColor
                elide: Text.ElideMiddle
                font.pixelSize: 13
            }

            Text {
                id: loading
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                anchors.rightMargin: 8
                text: panel.loading ? "Loading" : root.cleanText(panel.sortModeName)
                color: root.mutedText
                font.pixelSize: 12
            }
        }

        ListView {
            id: list
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.topMargin: Math.max(25, root.ch * 1.25)
            anchors.bottom: status.top
            clip: true
            model: panel.entries || []
            currentIndex: panel.cursor || 0
            boundsBehavior: Flickable.StopAtBounds

            delegate: Rectangle {
                width: ListView.view.width
                height: Math.max(22, root.ch * 1.1)
                color: {
                    if (index === panel.cursor)
                        return root.selectedBg
                    if (modelData.selected)
                        return root.markedBg
                    return "transparent"
                }

                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: 8
                    anchors.rightMargin: 8
                    spacing: 8

                    Text {
                        text: modelData.isDir ? (modelData.isUp ? "↰" : "▸") : " "
                        color: modelData.isDir ? "#7ed0ff" : root.mutedText
                        font.pixelSize: 13
                        Layout.preferredWidth: 18
                    }

                    Text {
                        text: root.cleanText(modelData.name)
                        color: modelData.isDir ? "#98d8ff" : root.textColor
                        elide: Text.ElideMiddle
                        font.pixelSize: 13
                        Layout.fillWidth: true
                    }

                    Text {
                        text: root.cleanText(modelData.sizeText)
                        color: root.mutedText
                        horizontalAlignment: Text.AlignRight
                        font.pixelSize: 12
                        Layout.preferredWidth: 96
                    }

                    Text {
                        text: root.cleanText(modelData.mtime)
                        color: root.mutedText
                        font.pixelSize: 12
                        visible: panel.viewModeName === "detailed"
                        Layout.preferredWidth: 120
                    }
                }

                MouseArea {
                    anchors.fill: parent
                    acceptedButtons: Qt.LeftButton | Qt.RightButton
                    onClicked: {
                        root.action({ "action": "activate_panel", "side": panel.side })
                        root.action({ "action": "panel_cursor", "side": panel.side, "index": modelData.index })
                    }
                    onDoubleClicked: root.action({ "action": "panel_open", "side": panel.side, "index": modelData.index })
                }
            }

            Component.onCompleted: positionViewAtIndex(currentIndex, ListView.Contain)
            onCurrentIndexChanged: positionViewAtIndex(currentIndex, ListView.Contain)
        }

        Rectangle {
            id: status
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            height: Math.max(24, root.ch * 1.15)
            color: "#1c2531"

            Text {
                anchors.left: parent.left
                anchors.verticalCenter: parent.verticalCenter
                anchors.leftMargin: 8
                text: panel.fastFind ? "/" + root.cleanText(panel.fastFindText) : root.cleanText(panel.selectedCount) + " selected"
                color: panel.fastFind ? root.activeBorder : root.mutedText
                font.pixelSize: 12
            }

            Text {
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                anchors.rightMargin: 8
                text: root.cleanText(panel.totalCount) + " items"
                color: root.mutedText
                font.pixelSize: 12
            }
        }
    }

    component CommandLineView: Rectangle {
        property var commandLine: ({})

        x: root.pxX(commandLine.x)
        y: root.pxY(commandLine.y)
        width: root.pxW(commandLine.w)
        height: Math.max(24, root.pxH(commandLine.h))
        visible: commandLine.visible !== false
        color: "#171d25"

        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: 8
            anchors.rightMargin: 8
            spacing: 0

            Text {
                text: root.runsText(commandLine.promptRuns) || root.cleanText(commandLine.prompt)
                color: "#8fe388"
                font.pixelSize: 13
                elide: Text.ElideMiddle
                Layout.maximumWidth: parent.width * 0.55
            }

            Text {
                text: root.cleanText(commandLine.text)
                color: root.textColor
                font.pixelSize: 13
                elide: Text.ElideRight
                Layout.fillWidth: true
            }
        }
    }

    component DocumentSurface: Rectangle {
        property var frame: ({})
        color: "#11161f"

        Rectangle {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            height: Math.max(30, root.ch * 1.45)
            color: "#202833"

            Text {
                anchors.left: parent.left
                anchors.right: mode.left
                anchors.verticalCenter: parent.verticalCenter
                anchors.leftMargin: 10
                anchors.rightMargin: 8
                text: root.cleanText(frame.title || frame.baseName)
                color: root.textColor
                font.pixelSize: 13
                elide: Text.ElideMiddle
            }

            Text {
                id: mode
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                anchors.rightMargin: 10
                text: frame.kind === "editor" ? (frame.dirty ? "modified" : "saved") : root.cleanText(frame.mode)
                color: frame.dirty ? root.activeBorder : root.mutedText
                font.pixelSize: 12
            }
        }

        ListView {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.topMargin: Math.max(30, root.ch * 1.45)
            anchors.bottom: parent.bottom
            anchors.bottomMargin: root.scene.keyBar ? Math.max(26, root.ch * 1.35) : 0
            clip: true
            model: frame.rows || []
            interactive: true

            delegate: Rectangle {
                width: ListView.view.width
                height: Math.max(20, root.ch)
                color: "transparent"

                Text {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    anchors.leftMargin: 10
                    anchors.rightMargin: 10
                    text: root.rowText(modelData)
                    color: root.textColor
                    font.family: "Menlo"
                    font.pixelSize: 13
                    elide: Text.ElideRight
                }
            }
        }
    }

    component GenericDialog: Rectangle {
        property var frame: ({})

        x: Math.max(12, root.pxX(frame.x))
        y: Math.max(semanticMenu.height + 8, root.pxY(frame.y))
        width: Math.min(root.width - 24, root.pxW(frame.w))
        height: Math.min(root.height - 36, root.pxH(frame.h))
        color: "#202833"
        border.width: 1
        border.color: root.activeBorder
        radius: 6
        clip: true

        Text {
            anchors.left: parent.left
            anchors.right: closeButton.left
            anchors.top: parent.top
            height: 30
            anchors.leftMargin: 10
            verticalAlignment: Text.AlignVCenter
            text: root.cleanText(frame.title)
            color: root.textColor
            font.pixelSize: 13
            elide: Text.ElideMiddle
        }

        Button {
            id: closeButton
            anchors.top: parent.top
            anchors.right: parent.right
            width: 34
            height: 30
            text: "×"
            visible: frame.showClose === true
            onClicked: root.action({ "target": frame.id, "action": "close" })
        }

        Item {
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.topMargin: 32
            anchors.bottom: parent.bottom

            Repeater {
                model: frame.children || []
                delegate: WidgetDelegate {
                    widget: modelData
                    originX: frame.x || 0
                    originY: frame.y || 0
                }
            }
        }
    }

    component WidgetDelegate: Item {
        id: widgetRoot
        property var widget: ({})
        property int originX: 0
        property int originY: 0

        x: root.pxX((widget.x || 0) - originX)
        y: root.pxY((widget.y || 0) - originY - 1)
        width: root.pxW(widget.w || 1)
        height: Math.max(22, root.pxH(widget.h || 1))
        visible: widget.visible !== false

        Loader {
            anchors.fill: parent
            sourceComponent: {
                switch (widget.kind) {
                case "button": return buttonDelegate
                case "checkbox": return checkboxDelegate
                case "edit": return editDelegate
                case "text": return textDelegate
                case "progressBar": return progressDelegate
                case "radioGroup": return choiceDelegate
                case "checkGroup": return choiceDelegate
                case "listBox": return listDelegate
                case "comboBox": return comboDelegate
                case "group": return groupDelegate
                default: return textDelegate
                }
            }
        }

        Component {
            id: textDelegate
            Text {
                text: root.cleanText(widget.text || widget.typeName)
                color: widget.disabled ? root.mutedText : root.textColor
                font.pixelSize: 13
                elide: Text.ElideRight
                verticalAlignment: Text.AlignVCenter
            }
        }

        Component {
            id: editDelegate
            Rectangle {
                color: "#11161f"
                border.width: 1
                border.color: widget.focused ? root.activeBorder : "#44505f"
                Text {
                    anchors.fill: parent
                    anchors.leftMargin: 6
                    anchors.rightMargin: 6
                    text: widget.password ? "••••••" : root.cleanText(widget.text)
                    color: root.textColor
                    font.pixelSize: 13
                    verticalAlignment: Text.AlignVCenter
                    elide: Text.ElideRight
                }
                MouseArea {
                    anchors.fill: parent
                    onClicked: root.action({ "target": widget.id, "action": "focus" })
                }
            }
        }

        Component {
            id: buttonDelegate
            Button {
                text: root.cleanText(widget.text)
                enabled: widget.disabled !== true
                onClicked: root.action({ "target": widget.id, "action": "activate" })
            }
        }

        Component {
            id: checkboxDelegate
            CheckBox {
                text: root.cleanText(widget.text)
                checked: widget.state === 1
                tristate: widget.threeState === true
                enabled: widget.disabled !== true
                onClicked: root.action({ "target": widget.id, "action": "toggle" })
            }
        }

        Component {
            id: progressDelegate
            ProgressBar {
                from: 0
                to: 100
                value: widget.percent || 0
            }
        }

        Component {
            id: choiceDelegate
            Column {
                Repeater {
                    model: widget.items || []
                    delegate: RadioButton {
                        text: root.cleanText(modelData)
                        checked: widget.kind === "radioGroup" ? index === widget.selected : !!(widget.states && widget.states[index])
                        onClicked: root.action({ "target": widget.id, "action": "select", "index": index })
                    }
                }
            }
        }

        Component {
            id: listDelegate
            ListView {
                clip: true
                model: widget.items || []
                delegate: Text {
                    width: ListView.view.width
                    height: Math.max(21, root.ch)
                    text: root.cleanText(modelData)
                    color: index === widget.cursor ? root.activeBorder : root.textColor
                    font.pixelSize: 13
                    MouseArea {
                        anchors.fill: parent
                        onClicked: root.action({ "target": widget.id, "action": "select", "index": index })
                    }
                }
            }
        }

        Component {
            id: comboDelegate
            Rectangle {
                color: "#11161f"
                border.width: 1
                border.color: "#44505f"
                Text {
                    anchors.fill: parent
                    anchors.leftMargin: 6
                    text: root.cleanText(widget.text)
                    color: root.textColor
                    verticalAlignment: Text.AlignVCenter
                    font.pixelSize: 13
                    elide: Text.ElideRight
                }
            }
        }

        Component {
            id: groupDelegate
            Item {
                Rectangle {
                    anchors.fill: parent
                    color: "transparent"
                    border.width: widget.title ? 1 : 0
                    border.color: "#44505f"
                    radius: 4
                }

                Text {
                    anchors.left: parent.left
                    anchors.top: parent.top
                    anchors.leftMargin: 8
                    text: root.cleanText(widget.title)
                    color: root.mutedText
                    font.pixelSize: 12
                    visible: root.cleanText(widget.title) !== ""
                }

                Repeater {
                    model: widget.children || []
                    delegate: ShallowWidgetDelegate {
                        widget: modelData
                        originX: widgetRoot.originX
                        originY: widgetRoot.originY
                    }
                }
            }
        }
    }

    component ShallowWidgetDelegate: Item {
        id: shallowRoot
        property var widget: ({})
        property int originX: 0
        property int originY: 0

        x: root.pxX((widget.x || 0) - originX)
        y: root.pxY((widget.y || 0) - originY - 1)
        width: root.pxW(widget.w || 1)
        height: Math.max(22, root.pxH(widget.h || 1))
        visible: widget.visible !== false

        Loader {
            anchors.fill: parent
            sourceComponent: {
                switch (widget.kind) {
                case "button": return shallowButton
                case "checkbox": return shallowCheckbox
                case "edit": return shallowEdit
                case "progressBar": return shallowProgress
                case "radioGroup": return shallowChoice
                case "checkGroup": return shallowChoice
                case "listBox": return shallowList
                case "comboBox": return shallowCombo
                default: return shallowText
                }
            }
        }

        Component {
            id: shallowText
            Text {
                text: root.cleanText(widget.text || widget.title || widget.typeName)
                color: widget.disabled ? root.mutedText : root.textColor
                font.pixelSize: 13
                elide: Text.ElideRight
                verticalAlignment: Text.AlignVCenter
            }
        }

        Component {
            id: shallowEdit
            Rectangle {
                color: "#11161f"
                border.width: 1
                border.color: widget.focused ? root.activeBorder : "#44505f"
                Text {
                    anchors.fill: parent
                    anchors.leftMargin: 6
                    anchors.rightMargin: 6
                    text: widget.password ? "••••••" : root.cleanText(widget.text)
                    color: root.textColor
                    font.pixelSize: 13
                    verticalAlignment: Text.AlignVCenter
                    elide: Text.ElideRight
                }
                MouseArea {
                    anchors.fill: parent
                    onClicked: root.action({ "target": widget.id, "action": "focus" })
                }
            }
        }

        Component {
            id: shallowButton
            Button {
                text: root.cleanText(widget.text)
                enabled: widget.disabled !== true
                onClicked: root.action({ "target": widget.id, "action": "activate" })
            }
        }

        Component {
            id: shallowCheckbox
            CheckBox {
                text: root.cleanText(widget.text)
                checked: widget.state === 1
                tristate: widget.threeState === true
                enabled: widget.disabled !== true
                onClicked: root.action({ "target": widget.id, "action": "toggle" })
            }
        }

        Component {
            id: shallowProgress
            ProgressBar {
                from: 0
                to: 100
                value: widget.percent || 0
            }
        }

        Component {
            id: shallowChoice
            Column {
                Repeater {
                    model: widget.items || []
                    delegate: RadioButton {
                        text: root.cleanText(modelData)
                        checked: widget.kind === "radioGroup" ? index === widget.selected : !!(widget.states && widget.states[index])
                        onClicked: root.action({ "target": widget.id, "action": "select", "index": index })
                    }
                }
            }
        }

        Component {
            id: shallowList
            ListView {
                clip: true
                model: widget.items || []
                delegate: Text {
                    width: ListView.view.width
                    height: Math.max(21, root.ch)
                    text: root.cleanText(modelData)
                    color: index === widget.cursor ? root.activeBorder : root.textColor
                    font.pixelSize: 13
                    MouseArea {
                        anchors.fill: parent
                        onClicked: root.action({ "target": widget.id, "action": "select", "index": index })
                    }
                }
            }
        }

        Component {
            id: shallowCombo
            Rectangle {
                color: "#11161f"
                border.width: 1
                border.color: "#44505f"
                Text {
                    anchors.fill: parent
                    anchors.leftMargin: 6
                    text: root.cleanText(widget.text)
                    color: root.textColor
                    verticalAlignment: Text.AlignVCenter
                    font.pixelSize: 13
                    elide: Text.ElideRight
                }
            }
        }
    }

    Component {
        id: dialogComponent
        GenericDialog {
            frame: ({})
        }
    }

    Component {
        id: menuPopupComponent
        Rectangle {
            property var frame: ({})
            x: root.pxX(frame.x)
            y: root.pxY(frame.y)
            width: root.pxW(frame.w)
            height: root.pxH(frame.h)
            color: "#202833"
            border.width: 1
            border.color: root.panelBorder
            z: 160

            ListView {
                anchors.fill: parent
                anchors.margins: 4
                model: frame.items || []
                clip: true
                delegate: Rectangle {
                    width: ListView.view.width
                    height: modelData.separator ? 8 : Math.max(22, root.ch * 1.1)
                    color: modelData.index === frame.selected ? root.selectedBg : "transparent"

                    Rectangle {
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        height: 1
                        color: "#44505f"
                        visible: modelData.separator
                    }

                    Text {
                        anchors.left: parent.left
                        anchors.right: shortcut.left
                        anchors.verticalCenter: parent.verticalCenter
                        anchors.leftMargin: 8
                        text: root.cleanText(modelData.text)
                        color: modelData.disabled ? root.mutedText : root.textColor
                        font.pixelSize: 13
                        visible: !modelData.separator
                        elide: Text.ElideRight
                    }

                    Text {
                        id: shortcut
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        anchors.rightMargin: 8
                        text: root.cleanText(modelData.shortcut)
                        color: root.mutedText
                        font.pixelSize: 12
                        visible: !modelData.separator
                    }

                    MouseArea {
                        anchors.fill: parent
                        enabled: !modelData.separator && !modelData.disabled
                        onClicked: {
                            if (modelData.command)
                                root.action({ "action": "emit_command", "command": modelData.command })
                            else
                                root.action({ "target": frame.id, "action": "menu_activate", "index": modelData.index })
                        }
                    }
                }
            }
        }
    }

    component KeyBarView: Rectangle {
        property var keyBar: ({})
        color: root.chromeBg
        visible: keyBar.visible !== false && keyBar.items !== undefined

        Row {
            anchors.fill: parent
            Repeater {
                model: keyBar.items || []
                delegate: Rectangle {
                    width: parent.width / 12
                    height: parent.height
                    color: index % 2 === 0 ? "#222b37" : "#1c2530"

                    Text {
                        anchors.left: parent.left
                        anchors.verticalCenter: parent.verticalCenter
                        anchors.leftMargin: 4
                        text: root.cleanText(index + 1)
                        color: root.activeBorder
                        font.pixelSize: 11
                    }

                    Text {
                        anchors.left: parent.left
                        anchors.leftMargin: 20
                        anchors.right: parent.right
                        anchors.rightMargin: 4
                        anchors.verticalCenter: parent.verticalCenter
                        text: root.cleanText(modelData.text)
                        color: root.chromeText
                        font.pixelSize: 11
                        elide: Text.ElideRight
                    }

                    MouseArea {
                        anchors.fill: parent
                        onClicked: grid.forceActiveFocus()
                    }
                }
            }
        }
    }

    component ToastView: Rectangle {
        property var toast: ({})
        width: Math.min(root.width - 32, toastText.implicitWidth + 28)
        height: toastText.implicitHeight + 14
        radius: 6
        color: "#2e343d"
        border.width: 1
        border.color: "#55616e"
        visible: toast.message !== undefined && root.cleanText(toast.message) !== ""

        Text {
            id: toastText
            anchors.centerIn: parent
            text: root.cleanText(toast.message)
            color: root.textColor
            font.pixelSize: 13
        }
    }
}
