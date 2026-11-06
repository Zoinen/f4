#include <QQuickItem>
#include <QQuickWindow>
#include <QtQuick/private/qquickwindow_p.h>
#include <QtQuick/private/qquickwindowcontainer_p.h>
#include <QtTest>
#ifdef Q_OS_WIN
#include <QtCore/qt_windows.h>
#endif

namespace {
class CountedItem : public QQuickItem {
public:
    using QQuickItem::QQuickItem;
    mutable int metaQueries = 0;
    const QMetaObject *metaObject() const override {
        ++metaQueries;
        return QQuickItem::metaObject();
    }
};
}

class F4QtChildWindowStackingTests : public QObject {
    Q_OBJECT
private slots:
    void skipsScenesWithoutNativeChildren_data() {
        QTest::addColumn<bool>("nativeChild");
        QTest::newRow("no-native-child") << false;
        QTest::newRow("native-child") << true;
    }
    void skipsScenesWithoutNativeChildren() {
        QFETCH(bool, nativeChild);
        QQuickWindow window;
        QWindow child;
        if (nativeChild) child.setParent(&window);
        QList<CountedItem *> items;
        auto *parent = window.contentItem();
        for (int i = 0; i < 500; ++i) {
            auto *item = new CountedItem(parent);
            items.append(item);
            if (i % 10 == 0) parent = item;
        }
        for (auto *item : items) item->metaQueries = 0;
        QQuickWindowPrivate::get(&window)->updateChildWindowStackingOrder();
        int queries = 0;
        for (auto *item : items) queries += item->metaQueries;
        if (nativeChild) QVERIFY(queries >= items.size());
        else QCOMPARE(queries, 0);
    }

    void nativeContainersKeepTheirStackingOrder() {
#ifndef Q_OS_WIN
        QSKIP("The native stacking-order observation uses Windows HWNDs");
#else
        if (QGuiApplication::platformName() != QStringLiteral("windows"))
            QSKIP("Requires the Windows native platform");
        QQuickWindow window;
        window.setGeometry(80, 80, 160, 96);
        QQuickWindow red(&window), blue(&window);
        red.setObjectName(QStringLiteral("stackingRedWindow"));
        blue.setObjectName(QStringLiteral("stackingBlueWindow"));
        red.setColor(Qt::red);
        blue.setColor(Qt::blue);
        auto *redContainer = new QQuickWindowContainer(window.contentItem());
        auto *blueContainer = new QQuickWindowContainer(window.contentItem());
        redContainer->setObjectName(QStringLiteral("stackingRedContainer"));
        blueContainer->setObjectName(QStringLiteral("stackingBlueContainer"));
        for (auto *container : {redContainer, blueContainer}) {
            container->setPosition(QPointF(8, 8));
            container->setSize(QSizeF(64, 64));
        }
        redContainer->setContainedWindow(&red);
        blueContainer->setContainedWindow(&blue);
        window.show();
        QVERIFY(QTest::qWaitForWindowExposed(&window));
        QTRY_COMPARE(red.parent(), &window);
        QTRY_COMPARE(blue.parent(), &window);
        QVERIFY(QTest::qWaitForWindowExposed(&red));
        QVERIFY(QTest::qWaitForWindowExposed(&blue));
        auto *parentHwnd = reinterpret_cast<HWND>(window.winId());
        auto *redHwnd = reinterpret_cast<HWND>(red.winId());
        auto *blueHwnd = reinterpret_cast<HWND>(blue.winId());
        red.raise();
        QTRY_COMPARE(GetTopWindow(parentHwnd), redHwnd);
        QQuickWindowPrivate::get(&window)->updateChildWindowStackingOrder();
        QTRY_COMPARE(GetTopWindow(parentHwnd), blueHwnd);
        redContainer->setZ(1);
        QQuickWindowPrivate::get(&window)->updateChildWindowStackingOrder();
        QTRY_COMPARE(GetTopWindow(parentHwnd), redHwnd);
        const qreal dpr = window.devicePixelRatio();
        if (qEnvironmentVariableIsSet("ZOIN_EXPECT_TEST_DPR"))
            QCOMPARE(dpr, qEnvironmentVariable("ZOIN_EXPECT_TEST_DPR").toDouble());
        for (auto *container : {redContainer, blueContainer}) {
            const QPointF origin = container->mapToScene(QPointF());
            QVERIFY(qAbs(origin.x() * dpr - qRound(origin.x() * dpr)) < .001);
            QVERIFY(qAbs(origin.y() * dpr - qRound(origin.y() * dpr)) < .001);
            QCOMPARE(container->mapToScene(QPointF(1, 0)) - origin, QPointF(1, 0));
            QCOMPARE(container->mapToScene(QPointF(0, 1)) - origin, QPointF(0, 1));
        }
#endif
    }
};

QTEST_MAIN(F4QtChildWindowStackingTests)
#include "F4QtChildWindowStackingTests.moc"
