#include <QQmlComponent>
#include <QQmlEngine>
#include <QEventLoop>
#include <QQuickItem>
#include <QQuickView>
#include <QScopedPointer>
#include <QSignalSpy>
#include <QTimer>
#include <QtTest>

#include <algorithm>

class F4PanelSplitterTests final : public QObject
{
    Q_OBJECT

private slots:
    void offCenterPressPreservesDragOffset();
    void dragClampsAtBothLimitsAndInNarrowSurfaces();
    void doubleClickRestoresEqualPanels();
    void keyboardAdjustmentRequiresPointerFocus();
    void inactiveSurfaceHidesDisablesAndDropsFocus();
    void inactiveSurfaceCanKeepVisualDivider();
};

namespace
{
void waitUntilLoaded(QQmlComponent *component)
{
    if (!component || component->status() != QQmlComponent::Loading)
        return;

    QEventLoop loop;
    QObject::connect(component, &QQmlComponent::statusChanged, &loop,
                     [&loop](QQmlComponent::Status status) {
        if (status != QQmlComponent::Loading)
            loop.quit();
    });
    QTimer::singleShot(5000, &loop, &QEventLoop::quit);
    loop.exec();
}

class RatioBinder final : public QObject
{
    Q_OBJECT

public:
    QQuickItem *splitter = nullptr;

public slots:
    void apply(double ratio)
    {
        if (splitter)
            splitter->setProperty("ratio", ratio);
    }
};

class KeyRecorder final : public QObject
{
    Q_OBJECT

public:
    struct Event {
        int key = 0;
        bool down = false;
    };

    Q_INVOKABLE void sendQtKey(int key, const QString &, bool down, int)
    {
        events.push_back({key, down});
    }

    int count(int key, bool down) const
    {
        return std::count_if(events.cbegin(), events.cend(),
                             [key, down](const Event &event) {
            return event.key == key && event.down == down;
        });
    }

    QVector<Event> events;
};

struct SplitterFixture {
    QQuickView view;
    QScopedPointer<QQmlComponent> surfaceComponent;
    QScopedPointer<QQmlComponent> splitterComponent;
    QQuickItem *surface = nullptr;
    QQuickItem *splitter = nullptr;
    RatioBinder ratioBinder;
    KeyRecorder keyRecorder;

    SplitterFixture()
    {
        view.setResizeMode(QQuickView::SizeRootObjectToView);
        view.resize(1000, 600);

        surfaceComponent.reset(new QQmlComponent(view.engine()));
        surfaceComponent->setData(R"QML(
            import QtQuick
            Item { width: 1000; height: 600 }
        )QML", QUrl(QStringLiteral("inline:PanelSplitterSurface.qml")));
        waitUntilLoaded(surfaceComponent.data());
        if (!surfaceComponent->isReady())
            return;

        surface = qobject_cast<QQuickItem *>(surfaceComponent->create());
        if (!surface)
            return;
        view.setContent(QUrl(QStringLiteral("inline:PanelSplitterSurface.qml")),
                        surfaceComponent.data(), surface);

        splitterComponent.reset(new QQmlComponent(view.engine()));
        splitterComponent->loadUrl(
            QUrl(QStringLiteral("qrc:/F4QtHost/qml/PanelSplitter.qml")),
            QQmlComponent::PreferSynchronous);
        waitUntilLoaded(splitterComponent.data());
        if (!splitterComponent->isReady())
            return;

        splitter = qobject_cast<QQuickItem *>(splitterComponent->create());
        if (!splitter)
            return;
        splitter->setParent(surface);
        splitter->setParentItem(surface);
        splitter->setHeight(surface->height());
        splitter->setProperty("availableWidth", surface->width());
        splitter->setProperty("minimumPanelWidth", 220.0);
        splitter->setProperty("ratio", 0.5);
        splitter->setProperty("keySink",
                              QVariant::fromValue<QObject *>(&keyRecorder));

        ratioBinder.splitter = splitter;
        QObject::connect(splitter, SIGNAL(ratioRequested(double)),
                         &ratioBinder, SLOT(apply(double)));
    }

    bool ready() const
    {
        return surfaceComponent && surfaceComponent->isReady()
               && splitterComponent && splitterComponent->isReady()
               && surface && splitter;
    }

    QString errors() const
    {
        QString result = QStringLiteral("surface status=%1 object=%2; splitter status=%3 object=%4; ")
                             .arg(surfaceComponent ? int(surfaceComponent->status()) : -1)
                             .arg(surface != nullptr)
                             .arg(splitterComponent ? int(splitterComponent->status()) : -1)
                             .arg(splitter != nullptr);
        if (surfaceComponent)
            result += surfaceComponent->errorString();
        if (splitterComponent)
            result += splitterComponent->errorString();
        return result;
    }

    void show()
    {
        view.show();
        view.requestActivate();
        (void)QTest::qWaitForWindowExposed(&view);
    }

    QPoint center() const
    {
        return pointAtLocalX(splitter->width() / 2);
    }

    QPoint pointAtLocalX(qreal localX) const
    {
        const QPointF point = splitter->mapToItem(surface, localX,
                                                  splitter->height() / 2);
        return point.toPoint();
    }
};

}

void F4PanelSplitterTests::offCenterPressPreservesDragOffset()
{
    SplitterFixture fixture;
    QVERIFY2(fixture.ready(), qPrintable(fixture.errors()));
    fixture.show();

    QSignalSpy ratios(fixture.splitter, SIGNAL(ratioRequested(double)));
    QVERIFY(ratios.isValid());

    const QPoint pressPoint = fixture.pointAtLocalX(
        fixture.splitter->width() - 2);
    QTest::mousePress(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                      pressPoint);
    QCOMPARE(fixture.splitter->property("ratio").toDouble(), 0.5);
    QCOMPARE(ratios.count(), 0);
    QVERIFY(fixture.splitter->hasActiveFocus());

    QTest::mouseMove(&fixture.view,
                     QPoint(pressPoint.x() + 100, pressPoint.y()));
    QTest::mouseRelease(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                        QPoint(pressPoint.x() + 100, pressPoint.y()));
    QTRY_COMPARE_WITH_TIMEOUT(fixture.splitter->property("ratio").toDouble(),
                              0.6, 1000);
    QVERIFY(ratios.count() >= 1);
}

void F4PanelSplitterTests::dragClampsAtBothLimitsAndInNarrowSurfaces()
{
    SplitterFixture fixture;
    QVERIFY2(fixture.ready(), qPrintable(fixture.errors()));
    fixture.show();

    QTest::mousePress(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                      fixture.center());
    QTest::mouseMove(&fixture.view, QPoint(10, 300));
    QTest::mouseRelease(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                        QPoint(10, 300));
    QTRY_COMPARE_WITH_TIMEOUT(fixture.splitter->property("ratio").toDouble(),
                              0.22, 1000);

    QTest::mousePress(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                      fixture.center());
    QTest::mouseMove(&fixture.view, QPoint(990, 300));
    QTest::mouseRelease(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                        QPoint(990, 300));
    QTRY_COMPARE_WITH_TIMEOUT(fixture.splitter->property("ratio").toDouble(),
                              0.78, 1000);

    // If the surface is narrower than twice the requested minimum, both
    // panels receive half instead of producing a negative or inverted range.
    fixture.splitter->setProperty("availableWidth", 300.0);
    fixture.splitter->setProperty("ratio", 0.5);
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.splitter->property("splitPosition").toDouble(), 150.0, 1000);
    QTest::mousePress(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                      fixture.center());
    QTest::mouseMove(&fixture.view, QPoint(290, 300));
    QTest::mouseRelease(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                        QPoint(290, 300));
    QTRY_COMPARE_WITH_TIMEOUT(fixture.splitter->property("ratio").toDouble(),
                              0.5, 1000);
}

void F4PanelSplitterTests::doubleClickRestoresEqualPanels()
{
    SplitterFixture fixture;
    QVERIFY2(fixture.ready(), qPrintable(fixture.errors()));
    fixture.splitter->setProperty("ratio", 0.68);
    fixture.show();

    QTest::mouseDClick(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                       fixture.center());
    QTRY_COMPARE_WITH_TIMEOUT(fixture.splitter->property("ratio").toDouble(),
                              0.5, 1000);
}

void F4PanelSplitterTests::keyboardAdjustmentRequiresPointerFocus()
{
    SplitterFixture fixture;
    QVERIFY2(fixture.ready(), qPrintable(fixture.errors()));
    fixture.show();
    QVERIFY(!fixture.splitter->activeFocusOnTab());
    QVERIFY(!fixture.splitter->hasActiveFocus());

    QTest::keyClick(&fixture.view, Qt::Key_Right);
    QCOMPARE(fixture.splitter->property("ratio").toDouble(), 0.5);

    QTest::mouseClick(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                      fixture.center());
    QVERIFY(fixture.splitter->hasActiveFocus());

    QTest::keyClick(&fixture.view, Qt::Key_Tab);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.keyRecorder.count(Qt::Key_Tab, true),
                              1, 1000);
    QCOMPARE(fixture.keyRecorder.count(Qt::Key_Tab, false), 1);
    QVERIFY(fixture.splitter->hasActiveFocus());

    QTest::keyClick(&fixture.view, Qt::Key_Right, Qt::ControlModifier);
    QTRY_COMPARE_WITH_TIMEOUT(
        fixture.keyRecorder.count(Qt::Key_Right, true), 1, 1000);
    QCOMPARE(fixture.keyRecorder.count(Qt::Key_Right, false), 1);
    QCOMPARE(fixture.splitter->property("ratio").toDouble(), 0.5);

    QTest::keyClick(&fixture.view, Qt::Key_Right);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.splitter->property("ratio").toDouble(),
                              0.516, 1000);
    QTest::keyClick(&fixture.view, Qt::Key_Left, Qt::ShiftModifier);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.splitter->property("ratio").toDouble(),
                              0.452, 1000);
    QTest::keyClick(&fixture.view, Qt::Key_Home);
    QTRY_COMPARE_WITH_TIMEOUT(fixture.splitter->property("ratio").toDouble(),
                              0.5, 1000);

    QSignalSpy focusRelease(fixture.splitter,
                            SIGNAL(focusReleaseRequested()));
    QVERIFY(focusRelease.isValid());
    QTest::keyClick(&fixture.view, Qt::Key_Escape);
    QTRY_COMPARE_WITH_TIMEOUT(focusRelease.count(), 1, 1000);
    QVERIFY(!fixture.splitter->hasActiveFocus());
}

void F4PanelSplitterTests::inactiveSurfaceHidesDisablesAndDropsFocus()
{
    SplitterFixture fixture;
    QVERIFY2(fixture.ready(), qPrintable(fixture.errors()));
    fixture.show();

    QTest::mouseClick(&fixture.view, Qt::LeftButton, Qt::NoModifier,
                      fixture.center());
    QVERIFY(fixture.splitter->hasActiveFocus());
    QVERIFY(fixture.splitter->isVisible());
    QVERIFY(fixture.splitter->isEnabled());
    QVERIFY(!fixture.splitter->property("accessibilityHidden").toBool());

    fixture.splitter->setProperty("surfaceActive", false);
    QTRY_VERIFY_WITH_TIMEOUT(!fixture.splitter->hasActiveFocus(), 1000);
    QVERIFY(!fixture.splitter->isVisible());
    QVERIFY(!fixture.splitter->isEnabled());
    QVERIFY(fixture.splitter->property("accessibilityHidden").toBool());

    fixture.splitter->setProperty("surfaceActive", true);
    QTRY_VERIFY_WITH_TIMEOUT(fixture.splitter->isVisible(), 1000);
    QVERIFY(fixture.splitter->isEnabled());
    QVERIFY(!fixture.splitter->property("accessibilityHidden").toBool());
}

void F4PanelSplitterTests::inactiveSurfaceCanKeepVisualDivider()
{
    SplitterFixture fixture;
    QVERIFY2(fixture.ready(), qPrintable(fixture.errors()));
    fixture.show();

    fixture.splitter->setProperty("surfaceActive", false);
    fixture.splitter->setProperty("surfaceVisible", true);

    QTRY_VERIFY_WITH_TIMEOUT(fixture.splitter->isVisible(), 1000);
    QVERIFY(!fixture.splitter->isEnabled());
    QVERIFY(fixture.splitter->property("accessibilityHidden").toBool());
}

QTEST_MAIN(F4PanelSplitterTests)

#include "F4PanelSplitterTests.moc"
