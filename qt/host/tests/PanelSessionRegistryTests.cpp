#include "PanelSessionRegistry.h"

#include <QtTest>

class PanelSessionRegistryTests final : public QObject
{
    Q_OBJECT

private slots:
    void keepsSessionsAndCatalogsInTheSameTypedSlot();
    void resetOnlyClearsTheRequestedCatalog();
    void restoresIdentityAndEvictsLeastRecentlyUsed();
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

void PanelSessionRegistryTests::restoresIdentityAndEvictsLeastRecentlyUsed()
{
    PanelSessionRegistry registry;
    QObject initial;
    registry.setSession(0, &initial);
    registry.catalog(0).panelId = "one";
    registry.catalog(0).catalogRevision = 42;
    registry.catalog(0).entries.append(QVariantMap{{"entryId", "one:file"}});
    QList<QObject *> created;
    QList<QObject *> released;
    const auto create = [&]() { auto *object = new QObject(this); created.append(object); return object; };
    const auto release = [&](QObject *object) { released.append(object); };
    registry.activatePanel(0, "one", create, release);
    registry.catalog(0).metadataRequestInFlight = true;
    registry.catalog(0).catalogRowsRequestInFlight = true;
    registry.catalog(0).metadataAwaitingFrame = true;
    registry.activatePanel(0, "two", create, release);
    QVERIFY(registry.session(0) != &initial);
    registry.catalog(0).panelId = "two";
    registry.catalog(0).catalogRevision = 7;
    QObject *second = registry.session(0);
    QCOMPARE(registry.sessionForPanel(0, "one"), &initial);
    QCOMPARE(registry.sessionForPanel(0, "two"), second);
    QCOMPARE(registry.sessionForPanel(0, "missing"), nullptr);
    registry.activatePanel(0, "one", create, release);
    QCOMPARE(registry.session(0), &initial);
    QCOMPARE(registry.catalog(0).catalogRevision, 42ULL);
    QCOMPARE(registry.catalog(0).entries.size(), 1);
    QVERIFY(!registry.catalog(0).metadataRequestInFlight);
    QVERIFY(!registry.catalog(0).catalogRowsRequestInFlight);
    QVERIFY(!registry.catalog(0).metadataAwaitingFrame);
    registry.activatePanel(0, "two", create, release);
    QCOMPARE(registry.session(0), second);
    QCOMPARE(registry.catalog(0).catalogRevision, 7ULL);
    for (int index = 0; index < 8; ++index)
        registry.activatePanel(0, QString::number(index), create, release);
    QVERIFY(released.contains(&initial));
    QVERIFY(released.contains(second));
    QCOMPARE(registry.session(1), nullptr);
}

QTEST_GUILESS_MAIN(PanelSessionRegistryTests)

#include "PanelSessionRegistryTests.moc"
