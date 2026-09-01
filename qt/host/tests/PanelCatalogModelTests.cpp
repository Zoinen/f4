#include "PanelCatalogModel.h"

#include <QtTest>

class PanelCatalogModelTests final : public QObject
{
    Q_OBJECT

private slots:
    void denseCatalogUsesDirectRows();
    void sparseCatalogRetainsOnlyMaterializedPages();
};

void PanelCatalogModelTests::denseCatalogUsesDirectRows()
{
    PanelCatalogModel model;
    model.entries = {
        QVariantMap{{QStringLiteral("entryId"), QStringLiteral("a")}},
        QVariantMap{{QStringLiteral("entryId"), QStringLiteral("b")}},
    };

    QCOMPARE(model.entryCount(), 2);
    QVERIFY(model.rowLoaded(0));
    QCOMPARE(model.entryAt(1).value(QStringLiteral("entryId")).toString(),
             QStringLiteral("b"));
    QVERIFY(model.setEntry(
        1, QVariantMap{{QStringLiteral("entryId"), QStringLiteral("c")}}));
    QCOMPARE(model.entryAt(1).value(QStringLiteral("entryId")).toString(),
             QStringLiteral("c"));
    QVERIFY(!model.setEntry(2, QVariantMap{}));
}

void PanelCatalogModelTests::sparseCatalogRetainsOnlyMaterializedPages()
{
    PanelCatalogModel model;
    model.catalogRowsDeferred = true;
    model.totalCount = 30000;

    for (int row = 14000; row < 14128; ++row) {
        QVERIFY(model.setEntry(
            row, QVariantMap{
                {QStringLiteral("entryId"),
                 QStringLiteral("entry-%1").arg(row)},
                {QStringLiteral("index"), row},
            }));
    }

    QCOMPARE(model.entryCount(), 30000);
    QCOMPARE(model.entries.size(), 128);
    QCOMPARE(model.entryOffsetByRow.size(), 128);
    QVERIFY(!model.rowLoaded(13999));
    QVERIFY(model.rowLoaded(14000));
    QVERIFY(model.rowLoaded(14127));
    QVERIFY(!model.rowLoaded(14128));

    QVERIFY(model.setEntry(
        14000, QVariantMap{
            {QStringLiteral("entryId"), QStringLiteral("replacement")},
            {QStringLiteral("index"), 14000},
        }));
    QCOMPARE(model.entries.size(), 128);
    QCOMPARE(model.entryAt(14000)
                 .value(QStringLiteral("entryId")).toString(),
             QStringLiteral("replacement"));
}

QTEST_GUILESS_MAIN(PanelCatalogModelTests)

#include "PanelCatalogModelTests.moc"
