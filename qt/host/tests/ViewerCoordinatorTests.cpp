#include "ViewerCoordinator.h"

#include <QSignalSpy>
#include <QTest>

class ViewerCoordinatorTests final : public QObject
{
    Q_OBJECT

private slots:
    void ownsPendingAndVisibleViewerState();
};

void ViewerCoordinatorTests::ownsPendingAndVisibleViewerState()
{
    ViewerCoordinator coordinator;
    QSignalSpy changed(&coordinator, &ViewerCoordinator::changed);

    coordinator.beginPending(1, QStringLiteral("right"),
                             QStringLiteral("image"), 7);
    QCOMPARE(coordinator.pendingIntent().active, true);
    QCOMPARE(coordinator.pendingIntent().side, 1);
    QCOMPARE(coordinator.pendingIntent().panelId, QStringLiteral("right"));
    QCOMPARE(coordinator.pendingIntent().entryId, QStringLiteral("image"));
    QCOMPARE(coordinator.pendingIntent().catalogRevision, quint64(7));
    QCOMPARE(changed.size(), 0);

    coordinator.show(1);
    QCOMPARE(coordinator.visible(), true);
    QCOMPARE(coordinator.side(), 1);
    QCOMPARE(changed.size(), 1);
    coordinator.show(1);
    QCOMPARE(changed.size(), 1);

    coordinator.clearPending();
    QCOMPARE(coordinator.pendingIntent().active, false);
    coordinator.hide();
    QCOMPARE(coordinator.visible(), false);
    QCOMPARE(coordinator.side(), -1);
    QCOMPARE(changed.size(), 2);
}

QTEST_GUILESS_MAIN(ViewerCoordinatorTests)

#include "ViewerCoordinatorTests.moc"
