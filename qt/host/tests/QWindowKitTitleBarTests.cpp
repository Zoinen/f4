#include <QQuickItem>
#include <QQuickWindow>
#include <QTest>

#include <QWKQuick/quickwindowagent.h>

class QWindowKitTitleBarTests : public QObject
{
    Q_OBJECT

private slots:
    void defaultWindowFlagsAllowDoubleClickMaximize();
    void customizedWindowFlagsCanDisableDoubleClickMaximize();
};

void QWindowKitTitleBarTests::defaultWindowFlagsAllowDoubleClickMaximize()
{
    QQuickWindow window;
    window.resize(640, 480);

    QQuickItem titleBar(window.contentItem());
    titleBar.setWidth(window.width());
    titleBar.setHeight(48);

    QWK::QuickWindowAgent agent;
    QVERIFY(agent.setup(&window));
    agent.setTitleBar(&titleBar);

    // Qt's default window controls are enabled without setting each hint bit.
    // This is the flag combination that previously made QWindowKit ignore the
    // title-bar double-click on Wayland.
    QVERIFY(!(window.flags() & Qt::CustomizeWindowHint));
    QVERIFY(!(window.flags() & Qt::WindowMaximizeButtonHint));

    window.show();
    QVERIFY(QTest::qWaitForWindowExposed(&window));

    const QPoint titleBarPoint(window.width() / 2, titleBar.height() / 2);
    QTest::mouseDClick(&window, Qt::LeftButton, Qt::NoModifier, titleBarPoint);
    QTRY_VERIFY(window.windowStates() & Qt::WindowMaximized);

    QTest::mouseDClick(&window, Qt::LeftButton, Qt::NoModifier, titleBarPoint);
    QTRY_VERIFY(!(window.windowStates() & Qt::WindowMaximized));
}

void QWindowKitTitleBarTests::customizedWindowFlagsCanDisableDoubleClickMaximize()
{
    QQuickWindow window;
    window.setFlags(Qt::Window | Qt::CustomizeWindowHint
                    | Qt::WindowMinimizeButtonHint | Qt::WindowCloseButtonHint);
    window.resize(640, 480);

    QQuickItem titleBar(window.contentItem());
    titleBar.setWidth(window.width());
    titleBar.setHeight(48);

    QWK::QuickWindowAgent agent;
    QVERIFY(agent.setup(&window));
    agent.setTitleBar(&titleBar);

    QVERIFY(window.flags() & Qt::CustomizeWindowHint);
    QVERIFY(!(window.flags() & Qt::WindowMaximizeButtonHint));

    window.show();
    QVERIFY(QTest::qWaitForWindowExposed(&window));

    const QPoint titleBarPoint(window.width() / 2, titleBar.height() / 2);
    QTest::mouseDClick(&window, Qt::LeftButton, Qt::NoModifier, titleBarPoint);
    QTest::qWait(50);
    QVERIFY(!(window.windowStates() & Qt::WindowMaximized));
}

QTEST_MAIN(QWindowKitTitleBarTests)

#include "QWindowKitTitleBarTests.moc"
