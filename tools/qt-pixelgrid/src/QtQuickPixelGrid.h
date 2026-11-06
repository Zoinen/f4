#pragma once

#include <QtCore/QStringList>
#include <QtCore/QRectF>

class QQuickItem;
class QQuickWindow;

namespace QtQuickPixelGrid {

// Opt a single QQuickWindow into the Qt Quick presentation pixel grid.
// Logical item geometry and QML bindings remain unchanged.
void setEnabled(QQuickWindow *window, bool enabled);
bool isEnabled(const QQuickWindow *window);

// Physical-grid-aligned bounds in QQuickWindow::contentItem() coordinates.
QRectF presentationBounds(const QQuickItem *item);

// Visual items/subtrees outside the proof policy (for example transformed or
// layered content) are returned as object names or class names.
QStringList unsupportedItems(const QQuickWindow *window);

} // namespace QtQuickPixelGrid
