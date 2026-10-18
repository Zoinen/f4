#pragma once

#include <QQmlEngine>
#include <QQuickItem>
#include <QQuickWindow>

// Diagnostic prototype only. No production call site or lifetime guarantees.
inline int ownQmlRoots(QQuickWindow *window, QQmlEngine *engine)
{
    int owned = 0;
    if (!window->QObject::parent()) {
        window->QObject::setParent(engine);
        ++owned;
    }
    QList<QQuickItem *> items{window->contentItem()};
    for (int i = 0; i < items.size(); ++i)
        items.append(items[i]->childItems());
    for (auto *item : items) {
        if (item->objectName().startsWith("retainedGalleryPanel-")
            && !item->QObject::parent() && item->parentItem()) {
            item->QObject::setParent(item->parentItem());
            ++owned;
        }
    }
    return owned;
}
