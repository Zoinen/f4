#include "ViewerCoordinator.h"

#include <QSignalSpy>
#include <QTest>

class ViewerCoordinatorTests final : public QObject
{
    Q_OBJECT

private slots:
    void ownsPendingAndVisibleViewerState();
    void dockTransitionsKeepOwnership();
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

void ViewerCoordinatorTests::dockTransitionsKeepOwnership()
{
    for (int side : {0, 1}) {
        ViewerCoordinator coordinator;
        coordinator.dock(side, 1 - side);
        QVERIFY(coordinator.mounted());
        QVERIFY(!coordinator.visible());
        QCOMPARE(coordinator.side(), side);
        QCOMPARE(coordinator.dockSide(), 1 - side);
        coordinator.show(side);
        QCOMPARE(coordinator.state(), ViewerCoordinator::Expanding);
        coordinator.collapse(); // An interrupted expansion reverses in place.
        QCOMPARE(coordinator.state(), ViewerCoordinator::Collapsing);
        coordinator.settle();
        QCOMPARE(coordinator.state(), ViewerCoordinator::Docked);
        coordinator.show(side);
        coordinator.settle();
        QCOMPARE(coordinator.state(), ViewerCoordinator::Full);
        coordinator.collapse();
        coordinator.removeDock(); // Disabling Quick View cancels the return.
        QCOMPARE(coordinator.state(), ViewerCoordinator::Full);
        QCOMPARE(coordinator.dockSide(), -1);
        coordinator.hide();
        QVERIFY(!coordinator.mounted());
        coordinator.dock(side, 1 - side);
        coordinator.removeDock();
        QVERIFY(!coordinator.mounted());
    }
}

QTEST_GUILESS_MAIN(ViewerCoordinatorTests)

#include "ViewerCoordinatorTests.moc"
