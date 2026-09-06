pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Controls.impl

Rectangle {
    id: documentHeader

    required property ApplicationWindow hostWindow
    required property var documentRoot

    objectName: documentRoot.surfaceObjectName === "documentSurface"
                ? "documentHeader"
                : documentRoot.surfaceObjectName + "Header"
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: parent.top
    anchors.topMargin: documentRoot.surfaceMenuInset
    height: documentRoot.documentHeaderHeight
    visible: documentRoot.showsConsoleTopBar
    color: hostWindow.titleBarBg
    z: 2

    Rectangle {
        id: documentHeaderBackground
        objectName: "documentHeaderBackground"
        anchors.fill: parent
        color: documentHeader.hostWindow.panelPathBg
        z: 0
    }

    Rectangle {
        id: documentHeaderSeparator
        objectName: "documentHeaderSeparator"
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: documentHeader.hostWindow.separatorWidth
        color: documentHeader.hostWindow.separatorColor
    }

    Item {
        id: documentHeaderIcon
        objectName: documentHeader.documentRoot.surfaceObjectName
                    === "documentSurface"
                    ? "documentHeaderIcon"
                    : documentHeader.documentRoot.surfaceObjectName
                      + "HeaderIcon"
        anchors.left: parent.left
        anchors.leftMargin: documentHeader.hostWindow.panelTextInset
        anchors.verticalCenter: parent.verticalCenter
        width: documentHeader.hostWindow.snapPx(18)
        height: documentHeader.hostWindow.snapPx(18)
        visible: documentHeader.documentRoot.documentFileIconAvailable
        z: 1

        IconLabel {
            id: documentHeaderLucideIcon
            objectName: documentHeader.documentRoot.surfaceObjectName
                        === "documentSurface"
                        ? "documentHeaderLucideIcon"
                        : documentHeader.documentRoot.surfaceObjectName
                          + "HeaderLucideIcon"
            anchors.centerIn: parent
            width: documentHeader.hostWindow.snapPx(16)
            height: documentHeader.hostWindow.snapPx(16)
            visible: !documentHeader.documentRoot.documentFileIconFullColor
            icon.source: documentHeader.documentRoot.documentFileIconSource
            icon.width: width
            icon.height: height
            icon.color: documentHeader.documentRoot.documentFileIconColor
            transform: Translate {
                x: documentHeader.documentRoot.pixelOffsetX(
                       documentHeaderLucideIcon)
                y: documentHeader.documentRoot.pixelOffsetY(
                       documentHeaderLucideIcon)
            }
        }

        Image {
            id: documentHeaderSystemIcon
            objectName: "documentHeaderSystemIcon"
            anchors.centerIn: parent
            width: documentHeader.hostWindow.snapPx(16)
            height: documentHeader.hostWindow.snapPx(16)
            source: documentHeader.documentRoot.documentFileIconSource
            fillMode: Image.PreserveAspectFit
            smooth: false
            mipmap: false
            asynchronous: true
            cache: true
            retainWhileLoading: true
            visible: documentHeader.documentRoot.documentFileIconFullColor
            transform: Translate {
                x: documentHeader.documentRoot.pixelOffsetX(
                       documentHeaderSystemIcon)
                y: documentHeader.documentRoot.pixelOffsetY(
                       documentHeaderSystemIcon)
            }
        }

        IconLabel {
            id: documentHeaderSystemFallbackIcon
            objectName: "documentHeaderSystemFallbackIcon"
            anchors.centerIn: parent
            width: documentHeader.hostWindow.snapPx(16)
            height: documentHeader.hostWindow.snapPx(16)
            visible: documentHeader.documentRoot.documentFileIconFullColor
                     && documentHeaderSystemIcon.status !== Image.Ready
            icon.source: documentHeader.hostWindow.lucideIconSource(
                             "file", 16,
                             documentHeader.documentRoot.documentFileIconColor)
            icon.width: width
            icon.height: height
            icon.color: documentHeader.documentRoot.documentFileIconColor
            transform: Translate {
                x: documentHeader.documentRoot.pixelOffsetX(
                       documentHeaderSystemFallbackIcon)
                y: documentHeader.documentRoot.pixelOffsetY(
                       documentHeaderSystemFallbackIcon)
            }
        }
    }

    Text {
        id: documentHeaderRight
        objectName: documentHeader.documentRoot.surfaceObjectName
                    === "documentSurface"
                    ? "documentHeaderRight"
                    : documentHeader.documentRoot.surfaceObjectName
                      + "HeaderRight"
        anchors.right: parent.right
        anchors.rightMargin: documentHeader.hostWindow.panelTextInset
        anchors.verticalCenter: parent.verticalCenter
        width: Math.min(implicitWidth,
                        Math.max(0, parent.width
                                - 2 * documentHeader.hostWindow.panelTextInset))
        text: documentHeader.documentRoot.topBarRightText
        textFormat: Text.PlainText
        color: documentHeader.hostWindow.galleryPathTextColor
        font.family: documentHeader.hostWindow.guiMonospaceFontFamily
        font.pixelSize: documentHeader.hostWindow.semanticTextFontPixelSize
        horizontalAlignment: Text.AlignRight
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideLeft
        transform: Translate {
            x: documentHeader.documentRoot.pixelOffsetX(documentHeaderRight)
            y: documentHeader.documentRoot.pixelOffsetY(documentHeaderRight)
        }
    }

    Text {
        id: documentHeaderLeft
        objectName: documentHeader.documentRoot.surfaceObjectName
                    === "documentSurface"
                    ? "documentHeaderLeft"
                    : documentHeader.documentRoot.surfaceObjectName
                      + "HeaderLeft"
        anchors.left: documentHeaderIcon.visible
                      ? documentHeaderIcon.right : parent.left
        anchors.right: documentHeaderRight.left
        anchors.leftMargin: documentHeaderIcon.visible
                            ? documentHeader.hostWindow.snapPx(7)
                            : documentHeader.hostWindow.panelTextInset
        anchors.rightMargin: documentHeader.hostWindow.panelTextInset
        anchors.verticalCenter: parent.verticalCenter
        text: documentHeader.documentRoot.topBarLeftText
        textFormat: Text.PlainText
        color: documentHeader.hostWindow.galleryPathTextColor
        font.family: documentHeader.hostWindow.guiMonospaceFontFamily
        font.pixelSize: documentHeader.hostWindow.semanticTextFontPixelSize
        verticalAlignment: Text.AlignVCenter
        // Preserve both the path root and the file name when the full path is
        // wider than the document header.
        elide: Text.ElideMiddle
        transform: Translate {
            x: documentHeader.documentRoot.pixelOffsetX(documentHeaderLeft)
            y: documentHeader.documentRoot.pixelOffsetY(documentHeaderLeft)
        }
    }
}
