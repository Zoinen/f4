#pragma once

#include <QCursor>
#include <QQuickItem>
#include <QQuickWindow>

namespace F4PointerRowAnchor {
// Queued hover coordinates can predate a native cursor warp or a newer move.
inline bool eventIsCurrent(QQuickItem *item, qreal x, qreal y)
{
    if (!item || !item->window())
        return false;
    const QPoint eventGlobal = item->window()->mapToGlobal(item->mapToScene(QPointF(x, y)).toPoint());
    return (eventGlobal - QCursor::pos()).manhattanLength() <= 1;
}

// Reposition only a pointer still inside the hovered row's previous bounds.
inline bool preserve(QQuickWindow *window, QQuickItem *row, qreal previousSceneY)
{
    if (!row || !window || row->window() != window || !row->isVisible())
        return false;
    const auto origin = row->mapToScene(QPointF());
    const QPoint global = QCursor::pos();
    const QPoint local = window->mapFromGlobal(global);
    const QRectF previousRect(origin.x(), previousSceneY, row->width(), row->height());
    if (!previousRect.contains(local))
        return false;
    QCursor::setPos(global + QPoint(0, qRound(origin.y() - previousSceneY)));
    return true;
}
}
