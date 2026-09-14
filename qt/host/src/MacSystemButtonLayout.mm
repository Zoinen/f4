#include "MacSystemButtonLayout.h"

#include <QDebug>
#include <QPointer>
#include <QQuickItem>
#include <QQuickWindow>
#include <QWKQuick/quickwindowagent.h>
#import <AppKit/AppKit.h>

namespace {
class MacSystemButtonLayout final : public QObject
{
public:
    explicit MacSystemButtonLayout(QQuickWindow *window)
        : QObject(window), m_window(window)
    {
        setObjectName(QStringLiteral("macSystemButtonLayoutObserver"));
        // Keep waiting if QML has not supplied the native agent/area yet.
        m_startup = connect(window, &QQuickWindow::frameSwapped, this,
                            [this] { refresh(); }, Qt::QueuedConnection);
        connect(window, &QWindow::visibilityChanged, this, [this] { schedule(); });
        schedule();
    }

    ~MacSystemButtonLayout() override
    {
        for (id token in m_observers)
            [[NSNotificationCenter defaultCenter] removeObserver:token];
        [m_observers release];
    }

private:
    void schedule()
    {
        if (m_pending)
            return;
        m_pending = true;
        QMetaObject::invokeMethod(this, [this] {
            m_pending = false;
            refresh();
        }, Qt::QueuedConnection);
    }

    void observe(NSString *name, id object)
    {
        const QPointer<MacSystemButtonLayout> guard(this);
        id token = [[NSNotificationCenter defaultCenter]
            addObserverForName:name object:object queue:nil
            usingBlock:^(NSNotification *) {
                if (guard)
                    guard->schedule();
            }];
        [m_observers addObject:token];
    }

    void attach(NSWindow *native, QQuickItem *area)
    {
        m_observers = [[NSMutableArray alloc] init];
        NSMutableSet *views = [NSMutableSet set];
        for (NSWindowButton kind : {NSWindowCloseButton, NSWindowMiniaturizeButton, NSWindowZoomButton}) {
            for (NSView *view = [native standardWindowButton:kind]; view; view = view.superview) {
                if ([views containsObject:view])
                    break;
                [views addObject:view];
                view.postsFrameChangedNotifications = YES;
                observe(NSViewFrameDidChangeNotification, view);
            }
        }
        for (NSString *name in @[NSWindowDidBecomeKeyNotification,
                                  NSWindowDidExitFullScreenNotification,
                                  NSWindowDidChangeBackingPropertiesNotification])
            observe(name, native);
        // QWK watches the area's own geometry, but not ancestor translations.
        for (QQuickItem *item = area; item; item = item->parentItem()) {
            connect(item, &QQuickItem::xChanged, this, [this] { schedule(); });
            connect(item, &QQuickItem::yChanged, this, [this] { schedule(); });
            connect(item, &QQuickItem::widthChanged, this, [this] { schedule(); });
            connect(item, &QQuickItem::heightChanged, this, [this] { schedule(); });
        }
        disconnect(m_startup);
        if (qEnvironmentVariableIsSet("F4_QWK_LAYOUT_DEBUG"))
            qInfo() << "[FIX:mac-traffic-lights] observing native layout";
    }

    void refresh()
    {
        auto *agent = qobject_cast<QWK::QuickWindowAgent *>(
            m_window->property("nativeWindowAgent").value<QObject *>());
        auto *area = m_window->property("macSystemButtonAreaItem").value<QQuickItem *>();
        if (!agent || !area || !m_window->isVisible())
            return;
        NSView *view = reinterpret_cast<NSView *>(m_window->winId());
        NSWindow *native = view.window;
        NSButton *middle = [native standardWindowButton:NSWindowMiniaturizeButton];
        if (!middle || !middle.superview)
            return;
        if (!m_observers)
            attach(native, area);
        // AppKit owns fullscreen controls, including its transition animation.
        if (m_window->visibility() == QWindow::FullScreen
            || (native.styleMask & NSWindowStyleMaskFullScreen))
            return;
        const QPoint expected = QRectF(area->mapToScene(QPointF()), area->size()).toRect().center();
        const QPointF actual(NSMidX(middle.frame),
            middle.superview.frame.size.height - NSMidY(middle.frame));
        if (qAbs(actual.x() - expected.x()) < .01
            && qAbs(actual.y() - expected.y()) < .01)
            return;
        if (qEnvironmentVariableIsSet("F4_QWK_LAYOUT_DEBUG"))
            qInfo() << "[FIX:mac-traffic-lights] native layout" << actual << "->" << expected;
        agent->setSystemButtonArea(nullptr);
        agent->setSystemButtonArea(area);
        if (qEnvironmentVariableIsSet("F4_QWK_LAYOUT_DEBUG"))
            qInfo() << "[FIX:mac-traffic-lights] applied center"
                    << NSMidX(middle.frame)
                    << middle.superview.frame.size.height - NSMidY(middle.frame);
    }

    QPointer<QQuickWindow> m_window;
    QMetaObject::Connection m_startup;
    NSMutableArray *m_observers = nil;
    bool m_pending = false;
};
}

void scheduleMacSystemButtonLayout(QQuickWindow *window)
{
    if (window && !window->findChild<QObject *>(QStringLiteral("macSystemButtonLayoutObserver")))
        new MacSystemButtonLayout(window);
}
