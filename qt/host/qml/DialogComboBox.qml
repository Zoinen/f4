pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

F4ComboBox {
    id: dialogCombo
    required property var widget
    enabled: widget.disabled !== true
    property string registeredOwnerId: ""
    leadingPreviewWidth: widget.iconTheme !== undefined ? hostWindow.snapPx(28) : 0
    HostPixelAlignedImage {
        hostWindow: dialogCombo.hostWindow
        objectName: dialogCombo.objectName + "IconPreview"
        visible: dialogCombo.leadingPreviewWidth > 0
        x: dialogCombo.hostWindow.snapPx(6)
        y: dialogCombo.hostWindow.snapPx((dialogCombo.height-height)/2)
        width: dialogCombo.hostWindow.snapPx(22)
        height: width
        sourceSize: Qt.size(width*dialogCombo.hostWindow.dpr,height*dialogCombo.hostWindow.dpr)
        fillMode: Image.PreserveAspectFit
        source: visible && dialogCombo.hostWindow.iconProvider
                ? dialogCombo.hostWindow.iconProvider.catalogIconSource(
                      String(dialogCombo.widget.iconTheme),String(dialogCombo.widget.iconReference || ""),
                      22,dialogCombo.hostWindow.dpr) : ""
    }

    function syncDropdownRegistration() {
        if (!hostWindow
                || typeof hostWindow.registerDropdownAnchor !== "function"
                || typeof hostWindow.unregisterDropdownAnchor !== "function")
            return
        const nextId = widget.dropdownOnly === true
                ? hostWindow.cleanText(widget.id) : ""
        if (registeredOwnerId !== "" && registeredOwnerId !== nextId)
            hostWindow.unregisterDropdownAnchor(registeredOwnerId, dialogCombo)
        registeredOwnerId = nextId
        if (registeredOwnerId !== "")
            hostWindow.registerDropdownAnchor(registeredOwnerId, dialogCombo)
    }

    // Dropdown-only choices belong to the separate semantic menu.
    model: externallyOwnedPopup ? [] : (widget.items || [])
    textRole: "text"
    currentIndex: Math.max(0, Number(widget.selected || 0))
    displayText: hostWindow.cleanText(widget.text)
    // Go's ComboBox keeps the edit field live even when the Qt surface is
    // only a projection.  Reflect that distinction to the native control's
    // cursor presentation without enabling the Qt template's own editable
    // popup/content lifecycle.
    textEditable: widget.dropdownOnly !== true
    semanticFocus: widget.focused === true
    externallyOwnedPopup: widget.dropdownOnly === true

    Component.onCompleted: syncDropdownRegistration()
    onWidgetChanged: syncDropdownRegistration()

    onExternalPopupRequested: hostWindow.action({
        "target": widget.id,
        "action": "control.open"
    }, true)

    onActivated: (index) => hostWindow.action({
        "target": widget.id,
        "action": "control.select",
        "index": index
    })
}
