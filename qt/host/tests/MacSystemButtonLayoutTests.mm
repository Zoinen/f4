#include "MacSystemButtonLayout.h"

#include <QQuickItem>
#include <QSignalSpy>
#include <QtTest>
#include <QWKQuick/quickwindowagent.h>
#import <AppKit/AppKit.h>

class MacSystemButtonLayoutTests : public QObject
{
    Q_OBJECT
private slots:
    void startupPositionsButtonsWithoutResizing_data();
    void startupPositionsButtonsWithoutResizing();
};

void MacSystemButtonLayoutTests::startupPositionsButtonsWithoutResizing_data()
{
    QTest::addColumn<bool>("maximized");
    QTest::newRow("windowed") << false;
    QTest::newRow("maximized") << true;
}

void MacSystemButtonLayoutTests::startupPositionsButtonsWithoutResizing()
{
    QFETCH(bool, maximized);
    QQuickWindow window;
    window.setGeometry(100, 100, 800, 600);
    QQuickItem title(window.contentItem());
    title.setSize(QSizeF(800, 36));
    QQuickItem area(&title);
    area.setPosition(QPointF(14, 1));
    area.setSize(QSizeF(70, 36));
    QWK::QuickWindowAgent agent;
    QVERIFY(agent.setup(&window));
    agent.setTitleBar(&title);
    agent.setSystemButtonArea(&area);
    window.setProperty("nativeWindowAgent", QVariant::fromValue(static_cast<QObject *>(&agent)));
    window.setProperty("macSystemButtonAreaItem", QVariant::fromValue(&area));
    if (maximized)
        window.showMaximized();
    else
        window.show();
    QSignalSpy widths(&window, &QWindow::widthChanged);
    QSignalSpy heights(&window, &QWindow::heightChanged);
    const QRect geometry = window.geometry();
    scheduleMacSystemButtonLayout(&window);
    QVERIFY(QTest::qWaitForWindowExposed(&window));
    QTest::qWait(150);
    if (!maximized) {
        QCOMPARE(window.geometry(), geometry);
        QCOMPARE(widths.count(), 0);
        QCOMPARE(heights.count(), 0);
    }
    NSView *view = reinterpret_cast<NSView *>(window.winId());
    NSWindow *native = view.window;
    QVERIFY(native);
    NSButton *middle = [native standardWindowButton:NSWindowMiniaturizeButton];
    QVERIFY(middle);
    const NSRect frame = middle.frame;
    const QPoint expected = QRectF(area.mapToScene(QPointF()), area.size()).toRect().center();
    const double actualX = NSMidX(frame);
    const double actualY = middle.superview.frame.size.height - NSMidY(frame);
    qInfo() << "[FIX:mac-traffic-lights]" << actualX << actualY << "expected" << expected;
    QVERIFY(qAbs(actualX - expected.x()) < .51);
    QVERIFY(qAbs(actualY - expected.y()) < .51);
    double previousX = -1;
    for (NSWindowButton kind : {NSWindowCloseButton, NSWindowMiniaturizeButton, NSWindowZoomButton}) {
        NSButton *button = [native standardWindowButton:kind];
        QVERIFY(button && !button.hidden);
        const NSRect rect = button.frame;
        QVERIFY(rect.origin.x > previousX);
        previousX = rect.origin.x;
        QVERIFY(qAbs(button.superview.frame.size.height - NSMidY(rect) - expected.y()) < .51);
        const double scale = native.backingScaleFactor;
        const NSRect scene = [button.superview convertRect:rect toView:nil];
        for (double edge : {scene.origin.x, scene.origin.y, scene.size.width, scene.size.height})
            QVERIFY(qAbs(edge * scale - qRound64(edge * scale)) < .01);
    }
}

QTEST_MAIN(MacSystemButtonLayoutTests)
#include "MacSystemButtonLayoutTests.moc"
