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
                       ? workspaceBar.left : windowButtons.left
        anchors.rightMargin: workspaceBar.visible
                             ? 8
                             : hostWindow.useMacNativeTitleBar
                               ? -windowButtons.width : 0
        height: parent.height
        opacity: hostWindow.normalSurfaceOpacity
    }

    WorkspaceTabs {
        id: workspaceBar
        hostWindow: titleBar.hostWindow
        availableWidth: titleBar.width
        nativeWindowAgent: titleBar.nativeWindowAgent
        nativeWindowAgentReady: titleBar.nativeWindowAgentReady
        usesQwk: titleBar.usesQwk
        x: hostWindow.snapPx(windowButtons.x - width
                             - (hostWindow.useMacNativeTitleBar
                                ? -windowButtons.width
                                  + hostWindow.contentSpacing
                                : hostWindow.contentSpacing))
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
