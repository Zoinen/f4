pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQuick.Shapes
import ZoinGallery 1.0 as ZG
import QWindowKit 1.0


Item {
    id: titleBar
    required property ApplicationWindow hostWindow
    required property Item semanticLayer
    required property QtObject nativeWindowAgent
    required property bool nativeWindowAgentReady
    required property bool usesQwk
    required property Window themeEditor
    readonly property alias menuBar: semanticMenu
    readonly property alias appIconButton: appIcon
    readonly property alias workspaceBarItem: workspaceBar
    readonly property alias macSystemButtonAreaItem: macSystemButtonArea
    objectName: "titleBar"

    function registerNativeSystemButtons() {
        if (!usesQwk || !nativeWindowAgentReady
                || hostWindow.useMacNativeTitleBar)
            return
        nativeWindowAgent.setSystemButton(WindowAgent.Minimize,
                                          minimizeButton)
        nativeWindowAgent.setSystemButton(WindowAgent.Maximize,
                                          maximizeButton)
        nativeWindowAgent.setSystemButton(WindowAgent.Close, closeButton)
    }

    onNativeWindowAgentReadyChanged: registerNativeSystemButtons()

    Rectangle {
        id: titleBarBackground
        objectName: "titleBarBackground"
        anchors.fill: parent
        color: hostWindow.titleBarBg
        z: -1
    }

    Text {
        id: worktreeBranchLabel
        objectName: "worktreeBranchLabel"
        property real alignmentRevision: titleBar.width + titleBar.height
        // Center in scene space before the physical-pixel correction below.
        anchors.alignWhenCentered: false
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.verticalCenter: parent.verticalCenter
        width: hostWindow.snapPx(Math.min(
            Math.max(0, parent.width - hostWindow.contentSpacing * 2),
            implicitWidth))
        height: hostWindow.snapPx(implicitHeight)
        text: hostWindow.worktreeBranchName
        visible: text !== "" && text !== "zoin"
        color: hostWindow.chromeText
        font.pixelSize: hostWindow.semanticTextFontPixelSize
        renderType: hostWindow.fontRenderType
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideMiddle
        transform: Translate {
            x: hostWindow.iconPixelOffsetX(worktreeBranchLabel)
            y: hostWindow.iconPixelOffsetY(worktreeBranchLabel)
        }
        z: 3
    }

    Item {
        id: macSystemButtonArea
        visible: false
        x: hostWindow.macSystemButtonAreaLeftMargin
        y: hostWindow.titleBarContentVerticalOffset
        width: 70
        height: parent.height
    }

    F4Button {
        id: appIcon
        objectName: "appIconButton"
        hostWindow: titleBar.hostWindow
        anchors.left: parent.left
        anchors.verticalCenter: parent.verticalCenter
        implicitWidth: hostWindow.snapPx(46)
        implicitHeight: hostWindow.snapPx(30)
        width: visible ? implicitWidth : 0
        height: parent.height
        visible: usesQwk && Qt.platform.os !== "osx"
        variant: "tool"
        flat: true

        leftPadding: 0
        topPadding: 0
        rightPadding: 0
        bottomPadding: 0

        contentItem: Item {
            HostPixelAlignedImage {
                hostWindow: titleBar.hostWindow
                objectName: "appIconImage"
                anchors.centerIn: parent
                width: hostWindow.snapPx(18)
                height: hostWindow.snapPx(18)
                sourceSize: Qt.size(18, 18)
                smooth: false
                mipmap: false
                source: "qrc:/F4QtHost/icons/app/f4.svg"
            }
        }

        onClicked: {
            if (themeEditor.visible) {
                themeEditor.hide()
            } else {
                hostWindow.showApplicationSettings()
            }
        }
    }

    SemanticMenuBar {
        id: semanticMenu
        hostWindow: titleBar.hostWindow
        semanticLayer: titleBar.semanticLayer
        nativeWindowAgent: titleBar.nativeWindowAgent
        nativeWindowAgentReady: titleBar.nativeWindowAgentReady
        usesQwk: titleBar.usesQwk
        menu: hostWindow.menuBarModel
        anchors.left: appIcon.right
        anchors.leftMargin: hostWindow.macTitleBarLeftPadding
        anchors.right: workspaceBar.visible
                       ? workspaceBar.left : queueButton.left
        anchors.rightMargin: workspaceBar.visible
                             ? 8
                             : hostWindow.useMacNativeTitleBar
                               ? -windowButtons.width : 0
        height: parent.height
    }



    WorkspaceTabs {
        id: workspaceBar
        hostWindow: titleBar.hostWindow
        availableWidth: titleBar.width
        nativeWindowAgent: titleBar.nativeWindowAgent
        nativeWindowAgentReady: titleBar.nativeWindowAgentReady
        usesQwk: titleBar.usesQwk
        x: hostWindow.snapPx(queueButton.x - width - hostWindow.snapPx(4))
    }

    ToolButton {
        id: queueButton
        objectName: "operationsQueueButton"
        x: hostWindow.snapPx((hostWindow.useMacNativeTitleBar ? titleBar.width : windowButtons.x) - width - hostWindow.contentSpacing)
        y: hostWindow.snapPx((titleBar.height - height) / 2)
        visible: hostWindow.nativeQueueDropdownEnabled
        width: visible ? hostWindow.snapPx(42) : 0
        height: hostWindow.snapPx(32)
        z: 3
        focusPolicy: Qt.StrongFocus
        Accessible.name: "Operations Queue"
        ToolTip.visible: hovered
        ToolTip.text: "Operations Queue"
        property bool closeQueueOnRelease: false
        onPressed: closeQueueOnRelease = hostWindow.queueDropdownOpen
        onClicked: {
            // An outside press may dismiss the popup before this release.
            // Preserve the intent of the original press instead of reopening it.
            if (closeQueueOnRelease) hostWindow.queueDropdownOpen = false
            else if (!hostWindow.queueDropdownOpen) hostWindow.toggleQueueDropdown()
        }
        function registerNativeHitTarget() {
            if (titleBar.usesQwk && titleBar.nativeWindowAgentReady)
                titleBar.nativeWindowAgent.setHitTestVisible(queueButton, true)
        }
        Component.onCompleted: registerNativeHitTarget()
        Connections {
            target: titleBar
            function onNativeWindowAgentReadyChanged() { queueButton.registerNativeHitTarget() }
        }
        background: Rectangle {
            radius: hostWindow.snapPx(5)
            color: queueButton.hovered || hostWindow.queueDropdownOpen ? hostWindow.selectedBg : "transparent"
        }
        contentItem: Item {
          Image {
            id: queueButtonIcon
            objectName: "operationsQueueButtonIcon"
            source: hostWindow.lucideIconSource("list-checks", 18, hostWindow.textColor)
            fillMode: Image.PreserveAspectFit
            sourceSize: Qt.size(18,18)
            width: hostWindow.snapPx(18)
            height: width
            x: hostWindow.snapPx((parent.width-width)/2)
            y: hostWindow.snapPx((parent.height-height)/2)
            transform: Translate {
                x: hostWindow.dialogPixelOffsetX(queueButtonIcon,hostWindow.contentItem)
                y: hostWindow.dialogPixelOffsetY(queueButtonIcon,hostWindow.contentItem)
            }
          }
        }
    }

    Rectangle {
        id: workspaceSeparatorLeft
        objectName: "workspaceSeparatorLeft"
        anchors.left: parent.left
        anchors.right: workspaceBar.left
        anchors.bottom: parent.bottom
        height: hostWindow.separatorWidth
        color: hostWindow.separatorColor
        visible: workspaceBar.visible
        z: 1
        antialiasing: false
    }

    Rectangle {
        id: workspaceSeparatorRight
        objectName: "workspaceSeparatorRight"
        anchors.left: workspaceBar.right
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: hostWindow.separatorWidth
        color: hostWindow.separatorColor
        visible: workspaceBar.visible
        z: 1
        antialiasing: false
    }

    Row {
        id: windowButtons
        objectName: "windowButtons"
        anchors.top: parent.top
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        spacing: 0
        height: parent.height
        visible: usesQwk && !hostWindow.useMacNativeTitleBar

        ZG.TitleButton {
            id: minimizeButton
            objectName: "minimizeButton"

            height: parent.height
            opacity: hostWindow.active || minimizeButton.hoveredOverride ? 1 : 0.4

            source: "qrc:/ZoinGallery/resources/WindowMinimize.svg"
            onClicked: hostWindow.showMinimized()

        }

        ZG.TitleButton {
            id: maximizeButton
            objectName: "maximizeButton"

            height: parent.height
            opacity: hostWindow.active || maximizeButton.hoveredOverride ? 1 : 0.4

            source: hostWindow.visibility === Window.Maximized
                    ? "qrc:/ZoinGallery/resources/WindowRestore.svg"
                    : hostWindow.visibility === Window.FullScreen
                      ? "qrc:/ZoinGallery/resources/WindowFullscreen.svg"
                      : "qrc:/ZoinGallery/resources/WindowMaximize.svg"
            onClicked: {
                if (hostWindow.visibility === Window.FullScreen) {
                    hostWindow.toggleFullscreen()
                } else if (hostWindow.visibility === Window.Maximized) {
                    hostWindow.showNormal()
                } else {
                    hostWindow.showMaximized()
                }
            }

        }

        ZG.TitleButton {
            id: closeButton
            objectName: "closeButton"

            height: parent.height
            opacity: hostWindow.active || closeButton.hoveredOverride ? 1 : 0.4

            source: "qrc:/ZoinGallery/resources/WindowClose.svg"
            icon.color: closeButton.hovered
                        ? ZG.Style.closeButtonHoveredIcon
                        : ZG.Style.text
            backgroundColor: {
                if (!closeButton.enabled) {
                    return "gray"
                }
                if (closeButton.pressed) {
                    return ZG.Style.closeButtonPressed
                }
                if (closeButton.hovered) {
                    return ZG.Style.closeButtonHovered
                }
                return "transparent"
            }
            onClicked: hostWindow.close()

        }
    }
}
