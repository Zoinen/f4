pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Rectangle {
    id: documentRow

    required property int index
    required property bool loaded
    required property var rowData
    required property ApplicationWindow hostWindow
    required property var documentRoot
    required property ListView documentList
    required property DocumentViewportController viewportController

    objectName: "documentRowDelegate"
    property bool countedAsLive: true
    // During an atomic rebase, data notifications refer to slots at the old
    // viewport until placement finishes. Shape text only once the live
    // delegates occupy the committed viewport.
    readonly property bool contentActive: loaded
        && (!documentRoot.standaloneViewport
            || (countedAsLive && !viewportController.rowTextSuspended))
    width: ListView.view.width
    height: documentRoot.rowHeight
    color: "transparent"
    Component.onCompleted: ++documentRoot.liveRowDelegateCount
    Component.onDestruction: {
        if (countedAsLive)
            --documentRoot.liveRowDelegateCount
    }
    ListView.onPooled: {
        if (countedAsLive) {
            countedAsLive = false
            --documentRoot.liveRowDelegateCount
        }
    }
    ListView.onReused: {
        if (!countedAsLive) {
            countedAsLive = true
            ++documentRoot.liveRowDelegateCount
        }
    }

    Row {
        id: runRow
        anchors.left: parent.left
        anchors.leftMargin: documentRow.documentRoot.textHorizontalInset
        height: parent.height
        z: 1
        visible: documentRow.loaded
                 && documentRow.rowData.runs !== undefined
                 && documentRow.rowData.runs.length > 0

        Repeater {
            // A new array value is not a new set of visual objects. Keep the
            // same run leaves while their count is stable; data/style bindings
            // update them in place.
            model: documentRow.documentRoot.standaloneViewport
                   ? (documentRow.loaded
                      ? (documentRow.rowData.runs || []).length : 0)
                   : (documentRow.loaded
                      ? documentRow.rowData.runs || [] : [])

            delegate: Rectangle {
                id: runSegment
                required property int index
                readonly property var runData:
                    (documentRow.rowData.runs || [])[index] || ({})
                height: runRow.height
                width: runLabel.implicitWidth
                color: documentRow.documentRoot.runBackground(
                           runData.background)

                Text {
                    id: runLabel
                    objectName: "documentRunText"
                    anchors.verticalCenter: parent.verticalCenter
                    text: documentRow.contentActive
                        ? documentRow.hostWindow.cleanText(
                              runSegment.runData.text) : ""
                    textFormat: Text.PlainText
                    color: documentRow.hostWindow.cleanText(
                               runSegment.runData.foreground) !== ""
                           ? runSegment.runData.foreground
                           : documentRow.hostWindow.textColor
                    font.family:
                        documentRow.hostWindow.guiMonospaceFontFamily
                    font.pixelSize:
                        documentRow.hostWindow.semanticTextFontPixelSize
                    font.bold: runSegment.runData.bold === true
                    font.underline: runSegment.runData.underline === true
                    font.strikeout: runSegment.runData.strikeout === true
                    transform: Translate {
                        x: documentRow.documentRoot.bodyPixelOffsetX(
                               documentRow.documentList.x
                               + documentRow.documentList.contentItem.x
                               + documentRow.x + runRow.x + runSegment.x
                               + runLabel.x)
                        y: documentRow.documentRoot.bodyPixelOffsetY(
                               documentRow.documentList.y
                               + documentRow.documentList.contentItem.y
                               + documentRow.y + runRow.y + runSegment.y
                               + runLabel.y)
                    }
                }
            }
        }
    }

    Text {
        id: plainDocumentText
        objectName: "documentPlainText"
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.verticalCenter: parent.verticalCenter
        anchors.leftMargin: documentRow.documentRoot.textHorizontalInset
        anchors.rightMargin: documentRow.documentRoot.textHorizontalInset
                             + documentRow.documentRoot.documentGutterWidth
        visible: documentRow.loaded
                 && (!documentRow.rowData.runs
                     || documentRow.rowData.runs.length === 0)
        // Styled runs already own their text layout. An invisible fallback
        // must not concatenate and shape the same row again.
        text: documentRow.contentActive && (!documentRow.rowData.runs
                  || documentRow.rowData.runs.length === 0)
              ? documentRow.hostWindow.rowText(documentRow.rowData) : ""
        textFormat: Text.PlainText
        color: documentRow.hostWindow.textColor
        font.family: documentRow.hostWindow.guiMonospaceFontFamily
        font.pixelSize: documentRow.hostWindow.semanticTextFontPixelSize
        elide: Text.ElideRight
        z: 1
        transform: Translate {
            x: documentRow.documentRoot.bodyPixelOffsetX(
                   documentRow.documentList.x
                   + documentRow.documentList.contentItem.x + documentRow.x
                   + plainDocumentText.x)
            y: documentRow.documentRoot.bodyPixelOffsetY(
                   documentRow.documentList.y
                   + documentRow.documentList.contentItem.y + documentRow.y
                   + plainDocumentText.y)
        }
    }

    readonly property var terminalSelectionRange:
        documentRoot.terminalSelectionRangeForRow(
            loaded ? Number(rowData.visualRow || 0) : -1, width)
    readonly property var editorSelectionRange:
        documentRoot.editorSelectionRangeForRow(
            loaded ? Number(rowData.visualRow || 0) : -1,
            loaded ? Number(rowData.visualWidth || 0) : 0)

    Rectangle {
        id: editorSelectionClip
        objectName: "documentEditorSelectionClip"
        readonly property real rawStartX:
            documentRow.documentRoot.textHorizontalInset
            + documentRow.editorSelectionRange.start
              * documentRow.documentRoot.terminalCellWidth
        readonly property real rawEndX:
            documentRow.documentRoot.textHorizontalInset
            + documentRow.editorSelectionRange.end
              * documentRow.documentRoot.terminalCellWidth
        readonly property real snappedEndX:
            rawEndX + documentRow.documentRoot.bodyPixelOffsetX(
                documentRow.documentList.x
                + documentRow.documentList.contentItem.x + documentRow.x
                + rawEndX)
        x: rawStartX + documentRow.documentRoot.bodyPixelOffsetX(
            documentRow.documentList.x
            + documentRow.documentList.contentItem.x + documentRow.x
            + rawStartX)
        y: 0
        width: Math.max(0, snappedEndX - x)
        height: parent.height
        visible: documentRow.loaded && documentRow.editorSelectionRange.valid
        color: documentRow.hostWindow.cleanText(
                   documentRow.documentRoot.cursorFrame.selectionBackground)
               !== ""
               ? documentRow.documentRoot.cursorFrame.selectionBackground
               : documentRow.hostWindow.selectedBg
        clip: true
        z: 2

        Text {
            id: editorSelectedText
            objectName: "documentEditorSelectedText"
            x: documentRow.documentRoot.textHorizontalInset
               - editorSelectionClip.x
            anchors.verticalCenter: parent.verticalCenter
            text: documentRow.contentActive && editorSelectionClip.visible
                  ? documentRow.hostWindow.rowText(documentRow.rowData) : ""
            textFormat: Text.PlainText
            color: documentRow.hostWindow.cleanText(
                       documentRow.documentRoot.cursorFrame.selectionForeground)
                   !== ""
                   ? documentRow.documentRoot.cursorFrame.selectionForeground
                   : documentRow.hostWindow.textColor
            font.family: documentRow.hostWindow.guiMonospaceFontFamily
            font.pixelSize: documentRow.hostWindow.semanticTextFontPixelSize
            font.bold:
                documentRow.documentRoot.cursorFrame.selectionBold === true
            font.underline:
                documentRow.documentRoot.cursorFrame.selectionUnderline
                === true
            font.strikeout:
                documentRow.documentRoot.cursorFrame.selectionStrikeout
                === true
            transform: Translate {
                x: documentRow.documentRoot.bodyPixelOffsetX(
                       documentRow.documentList.x
                       + documentRow.documentList.contentItem.x + documentRow.x
                       + editorSelectionClip.x + editorSelectedText.x)
                y: documentRow.documentRoot.bodyPixelOffsetY(
                       documentRow.documentList.y
                       + documentRow.documentList.contentItem.y + documentRow.y
                       + editorSelectionClip.y + editorSelectedText.y)
            }
        }
    }

    Rectangle {
        x: documentRow.documentRoot.textHorizontalInset
           + documentRow.terminalSelectionRange.start
             * documentRow.documentRoot.terminalCellWidth
        y: 0
        width: Math.max(0,
            (documentRow.terminalSelectionRange.end
             - documentRow.terminalSelectionRange.start)
            * documentRow.documentRoot.terminalCellWidth)
        height: parent.height
        visible: documentRow.documentRoot.terminalSurface
                 && documentRow.loaded
                 && documentRow.terminalSelectionRange.valid
        color: documentRow.hostWindow.selectedBg
        opacity: 0.72
        z: 0
    }
}
