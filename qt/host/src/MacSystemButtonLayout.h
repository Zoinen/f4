#pragma once

#include <QQuickWindow>
#include <QPointer>
#include <QQuickItem>
#include <QDebug>
#include <QWKQuick/quickwindowagent.h>

// Kept separate from startup so the native geometry contract can be tested.
inline void scheduleMacSystemButtonLayout(QQuickWindow *window)
{
    if (!window)
        return;
    const QPointer<QWK::QuickWindowAgent> agent(qobject_cast<QWK::QuickWindowAgent *>(
        window->property("nativeWindowAgent").value<QObject *>()));
    const QPointer<QQuickItem> area(window->property("macSystemButtonAreaItem").value<QQuickItem *>());
    const bool debug = qEnvironmentVariableIsSet("F4_QWK_LAYOUT_DEBUG");
    if (debug)
        qWarning() << "[FIX:mac-traffic-lights] schedule" << agent << area << window->visibility();
    if (!agent || !area)
        return;
    // AppKit may reset standard button frames while showing the native window.
    // Refresh after that first frame, on the GUI thread. Reassigning the same
    // area is a QWK no-op, so detach/rebind to invoke its public layout hook.
    QObject::connect(window, &QQuickWindow::frameSwapped, window, [agent, area, debug] {
        if (!agent || !area)
            return;
        agent->setSystemButtonArea(nullptr);
        agent->setSystemButtonArea(area);
        if (debug)
            qWarning() << "[FIX:mac-traffic-lights] applied" << area->mapToScene(QPointF()) << area->size();
    }, Qt::ConnectionType(Qt::QueuedConnection | Qt::SingleShotConnection));
    window->requestUpdate();
}
