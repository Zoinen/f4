#include "WindowGeometryPersistence.h"

#include <QGuiApplication>
#include <QScreen>
#include <QSettings>
#include <QTemporaryDir>
#include <QTest>
#include <QWindow>

class WindowGeometryPersistenceTests final : public QObject
{
    Q_OBJECT

private slots:
    void settingsRoundTripPreservesEveryField_data();
    void settingsRoundTripPreservesEveryField();
    void exactGeometrySurvivesOnUnchangedScreen();
    void geometryTracksMovedScreenOrigin();
    void missingScreenFallsBackAndRemainsVisible();
    void oversizedGeometryFitsAvailableArea();
    void realWindowSaveAndRestoreRoundTrip();
    void hiddenOnCloseRetainsMaximizedStateAndNormalFrame();
};

void WindowGeometryPersistenceTests::settingsRoundTripPreservesEveryField_data()
{
    QTest::addColumn<int>("state");
    QTest::newRow("windowed")
        << int(PersistedWindowState::Windowed);
    QTest::newRow("maximized")
        << int(PersistedWindowState::Maximized);
    QTest::newRow("fullscreen")
        << int(PersistedWindowState::FullScreen);
}

void WindowGeometryPersistenceTests::settingsRoundTripPreservesEveryField()
{
    QFETCH(int, state);
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    QSettings settings(directory.filePath(QStringLiteral("geometry.ini")),
                       QSettings::IniFormat);
    PersistedWindowGeometry source;
    source.valid = true;
    source.normalGeometry = QRect(117, 83, 1234, 777);
    source.screenName = QStringLiteral("Studio Display");
    source.screenAvailableGeometry = QRect(-1728, 25, 1728, 1080);
    source.state = PersistedWindowState(state);

    WindowGeometryPersistence::write(settings, source);
    const PersistedWindowGeometry restored =
        WindowGeometryPersistence::read(settings);
    QVERIFY(restored.valid);
    QCOMPARE(restored.normalGeometry, source.normalGeometry);
    QCOMPARE(restored.screenName, source.screenName);
    QCOMPARE(restored.screenAvailableGeometry,
             source.screenAvailableGeometry);
    QCOMPARE(int(restored.state), state);
}

void WindowGeometryPersistenceTests::exactGeometrySurvivesOnUnchangedScreen()
{
    PersistedWindowGeometry stored;
    stored.valid = true;
    stored.normalGeometry = QRect(140, 90, 1100, 720);
    stored.screenName = QStringLiteral("main");
    stored.screenAvailableGeometry = QRect(0, 0, 1920, 1080);
    QCOMPARE(WindowGeometryPersistence::resolvedNormalGeometry(
                 stored, {{QStringLiteral("main"), QRect(0, 0, 1920, 1080)}},
                 QStringLiteral("main")),
             stored.normalGeometry);
}

void WindowGeometryPersistenceTests::geometryTracksMovedScreenOrigin()
{
    PersistedWindowGeometry stored;
    stored.valid = true;
    stored.normalGeometry = QRect(-1600, 120, 900, 650);
    stored.screenName = QStringLiteral("secondary");
    stored.screenAvailableGeometry = QRect(-1728, 25, 1728, 1080);
    const QRect resolved = WindowGeometryPersistence::resolvedNormalGeometry(
        stored,
        {{QStringLiteral("primary"), QRect(0, 0, 1920, 1080)},
         {QStringLiteral("secondary"), QRect(1920, 40, 1728, 1080)}},
        QStringLiteral("primary"));
    QCOMPARE(resolved, QRect(2048, 135, 900, 650));
}

void WindowGeometryPersistenceTests::missingScreenFallsBackAndRemainsVisible()
{
    PersistedWindowGeometry stored;
    stored.valid = true;
    stored.normalGeometry = QRect(-2400, -800, 1000, 700);
    stored.screenName = QStringLiteral("disconnected");
    stored.screenAvailableGeometry = QRect(-2560, -900, 2560, 1440);
    const QRect available(0, 25, 1440, 875);
    const QRect resolved = WindowGeometryPersistence::resolvedNormalGeometry(
        stored, {{QStringLiteral("laptop"), available}},
        QStringLiteral("laptop"));
    QVERIFY(available.contains(resolved));
    QCOMPARE(resolved.size(), stored.normalGeometry.size());
}

void WindowGeometryPersistenceTests::oversizedGeometryFitsAvailableArea()
{
    PersistedWindowGeometry stored;
    stored.valid = true;
    stored.normalGeometry = QRect(-100, -100, 4000, 3000);
    stored.screenName = QStringLiteral("main");
    stored.screenAvailableGeometry = QRect(0, 0, 1920, 1080);
    const QRect available(0, 24, 1280, 776);
    const QRect resolved = WindowGeometryPersistence::resolvedNormalGeometry(
        stored, {{QStringLiteral("main"), available}},
        QStringLiteral("main"));
    QCOMPARE(resolved, available);
}

void WindowGeometryPersistenceTests::realWindowSaveAndRestoreRoundTrip()
{
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    const QString settingsPath =
        directory.filePath(QStringLiteral("geometry.ini"));
    QScreen *screen = QGuiApplication::primaryScreen();
    QVERIFY(screen);
    const QRect available = screen->availableGeometry();
    const QSize size(qMin(640, available.width()),
                     qMin(480, available.height()));
    const QRect wanted(available.topLeft() + QPoint(20, 20), size);

    {
        QWindow first;
        first.setGeometry(wanted);
        first.show();
        QTRY_VERIFY(first.isVisible());
        WindowGeometryPersistence persistence(&first, settingsPath);
        first.close();
        QTRY_VERIFY(!first.isVisible());
    }

    QSettings settings(settingsPath, QSettings::IniFormat);
    const PersistedWindowGeometry stored =
        WindowGeometryPersistence::read(settings);
    QVERIFY(stored.valid);
    QCOMPARE(stored.normalGeometry, wanted);

    QWindow second;
    second.setGeometry(QRect(0, 0, 100, 100));
    second.show();
    WindowGeometryPersistence persistence(&second, settingsPath);
    QVERIFY(persistence.restore());
    QCOMPARE(second.geometry(), wanted);
}

void WindowGeometryPersistenceTests::hiddenOnCloseRetainsMaximizedStateAndNormalFrame()
{
    QTemporaryDir directory;
    QVERIFY(directory.isValid());
    const QString settingsPath =
        directory.filePath(QStringLiteral("geometry.ini"));
    QScreen *screen = QGuiApplication::primaryScreen();
    QVERIFY(screen);
    const QRect available = screen->availableGeometry();
    const QRect normal(available.topLeft() + QPoint(30, 30),
                       QSize(qMin(620, available.width()),
                             qMin(440, available.height())));

    QWindow window;
    window.setGeometry(normal);
    window.show();
    QTRY_COMPARE(window.visibility(), QWindow::Windowed);
    WindowGeometryPersistence persistence(&window, settingsPath);
    window.showMaximized();
    QTRY_COMPARE(window.visibility(), QWindow::Maximized);
    window.hide();
    QTRY_COMPARE(window.visibility(), QWindow::Hidden);
    persistence.save();

    QSettings settings(settingsPath, QSettings::IniFormat);
    const PersistedWindowGeometry stored =
        WindowGeometryPersistence::read(settings);
    QVERIFY(stored.valid);
    QCOMPARE(stored.normalGeometry, normal);
    QCOMPARE(int(stored.state), int(PersistedWindowState::Maximized));
}

QTEST_MAIN(WindowGeometryPersistenceTests)
#include "WindowGeometryPersistenceTests.moc"
