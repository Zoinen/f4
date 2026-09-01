#include "PanelSessionRegistry.h"

#include <QtTest>

class PanelSessionRegistryTests final : public QObject
{
    Q_OBJECT

private slots:
    void keepsSessionsAndCatalogsInTheSameTypedSlot();
    void resetOnlyClearsTheRequestedCatalog();
};

void PanelSessionRegistryTests::keepsSessionsAndCatalogsInTheSameTypedSlot()
{
    PanelSessionRegistry registry;
    QObject left;
    QObject right;
    registry.setSession(0, &left);
    registry.setSession(1, &right);
    registry.catalog(0).panelId = QStringLiteral("left");
    registry.catalog(1).panelId = QStringLiteral("right");

    QCOMPARE(registry.session(0), &left);
    QCOMPARE(registry.session(1), &right);
    QCOMPARE(registry.catalog(0).panelId, QStringLiteral("left"));
    QCOMPARE(registry.catalog(1).panelId, QStringLiteral("right"));
    QVERIFY(!PanelSessionRegistry::validSide(-1));
    QVERIFY(!PanelSessionRegistry::validSide(2));
    QCOMPARE(registry.session(2), nullptr);
}

void PanelSessionRegistryTests::resetOnlyClearsTheRequestedCatalog()
{
    PanelSessionRegistry registry;
    QObject left;
    registry.setSession(0, &left);
    registry.catalog(0).panelId = QStringLiteral("old-left");
    registry.catalog(0).entries.push_back(QVariantMap{
        {QStringLiteral("entryId"), QStringLiteral("entry")},
    });
    registry.catalog(1).panelId = QStringLiteral("right");

    registry.resetCatalog(0);

    QCOMPARE(registry.session(0), &left);
    QVERIFY(registry.catalog(0).panelId.isEmpty());
    QVERIFY(registry.catalog(0).entries.isEmpty());
    QCOMPARE(registry.catalog(1).panelId, QStringLiteral("right"));
}

QTEST_GUILESS_MAIN(PanelSessionRegistryTests)

#include "PanelSessionRegistryTests.moc"
