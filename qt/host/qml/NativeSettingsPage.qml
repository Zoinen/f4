import QtQuick

// Frontend-owned content. No descriptors, values or controls cross ExtUI.
QtObject {
    required property string pageId
    required property string title
    property string iconName: "settings"
    required property Component content
}
