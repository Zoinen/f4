#include "ExtUiSceneReducer.h"

#include <QTest>

class ExtUiSceneReducerTests final : public QObject
{
    Q_OBJECT

private slots:
    void presentationProjectionDropsNativeCatalogRows();
    void snapshotMutatesOnlyItsOwnedStream();
    void snapshotPayloadTypeMustMatchStream();
    void selectionPatchUsesLogicalCatalogRows_data();
    void selectionPatchUsesLogicalCatalogRows();
};

void ExtUiSceneReducerTests::presentationProjectionDropsNativeCatalogRows()
{
    const QVariantMap panel{
        {QStringLiteral("id"), QStringLiteral("left")},
        {QStringLiteral("totalCount"), 30000},
        {QStringLiteral("entries"), QVariantList{
             QVariantMap{{QStringLiteral("name"), QStringLiteral("a")}}}},
        {QStringLiteral("highlightStyles"), QVariantMap{
             {QStringLiteral("hidden"), QVariantMap{}}}},
    };
    const QVariantMap scene{
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("panels"), QVariantList{panel}}}},
    };

    const QVariantMap presentation =
        ExtUiSceneReducer::makePresentationScene(scene);
    const QVariantMap projectedPanel = presentation
        .value(QStringLiteral("shell")).toMap()
        .value(QStringLiteral("panels")).toList().at(0).toMap();
    QCOMPARE(projectedPanel.value(QStringLiteral("id")).toString(),
             QStringLiteral("left"));
    QCOMPARE(projectedPanel.value(QStringLiteral("totalCount")).toInt(),
             30000);
    QVERIFY(!projectedPanel.contains(QStringLiteral("entries")));
    QVERIFY(!projectedPanel.contains(QStringLiteral("highlightStyles")));
}

void ExtUiSceneReducerTests::snapshotMutatesOnlyItsOwnedStream()
{
    QVariantMap scene{
        {QStringLiteral("menus"), QVariantList{
             QVariantMap{{QStringLiteral("id"), QStringLiteral("keep")}}}},
        {QStringLiteral("dialogs"), QVariantList{
             QVariantMap{{QStringLiteral("id"), QStringLiteral("dialog")}}}},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("activePanel"), 1},
             {QStringLiteral("panels"), QVariantList{
                  QVariantMap{{QStringLiteral("side"), 0},
                              {QStringLiteral("id"), QStringLiteral("old")}}}}}},
    };
    const QVariantMap panel{
        {QStringLiteral("side"), 0},
        {QStringLiteral("id"), QStringLiteral("new")},
        {QStringLiteral("entries"), QVariantList{}},
    };
    const QVariantMap snapshot{
        {QStringLiteral("type"), QStringLiteral("panel_catalog_snapshot")},
        {QStringLiteral("state"), QVariantMap{
             {QStringLiteral("side"), 0},
             {QStringLiteral("panel"), panel}}},
    };
    QVariantMap emittedPanel;
    QString error;
    QVERIFY(ExtUiSceneReducer::applyStreamSnapshotPayload(
        QStringLiteral("panel/0"), snapshot, &scene, &emittedPanel, &error));
    QVERIFY2(error.isEmpty(), qPrintable(error));
    QCOMPARE(emittedPanel, panel);
    QCOMPARE(scene.value(QStringLiteral("menus")).toList().at(0).toMap()
                 .value(QStringLiteral("id")).toString(),
             QStringLiteral("keep"));
    QCOMPARE(scene.value(QStringLiteral("dialogs")).toList().at(0).toMap()
                 .value(QStringLiteral("id")).toString(),
             QStringLiteral("dialog"));
}

void ExtUiSceneReducerTests::snapshotPayloadTypeMustMatchStream()
{
    QVariantMap scene;
    QVariantMap panel;
    QString error;
    const QVariantMap wrongPayload{
        {QStringLiteral("type"), QStringLiteral("menus_snapshot")},
        {QStringLiteral("state"), QVariantMap{}},
    };
    QVERIFY(!ExtUiSceneReducer::applyStreamSnapshotPayload(
        QStringLiteral("panel/0"), wrongPayload, &scene, &panel, &error));
    QVERIFY(!error.isEmpty());
    QVERIFY(scene.isEmpty());
}

void ExtUiSceneReducerTests::selectionPatchUsesLogicalCatalogRows_data()
{
    QTest::addColumn<bool>("paged");
    QTest::addColumn<int>("row");
    QTest::addColumn<QString>("entryId");
    QTest::addColumn<bool>("replace");
    QTest::addColumn<bool>("accepted");

    QTest::newRow("dense-known") << false << 0 << "entry-0" << false << true;
    QTest::newRow("dense-mismatch") << false << 0 << "wrong" << false << false;
    QTest::newRow("dense-out-of-range") << false << 2 << "entry-2" << false << false;
    QTest::newRow("paged-known-logical-row") << true << 500 << "entry-500" << false << true;
    QTest::newRow("paged-before-window") << true << 0 << "entry-0" << false << true;
    QTest::newRow("paged-after-window") << true << 29999 << "entry-29999" << false << true;
    QTest::newRow("paged-known-mismatch") << true << 500 << "wrong" << false << false;
    QTest::newRow("paged-wrong-row-for-known-id") << true << 0 << "entry-500" << false << false;
    QTest::newRow("paged-out-of-range") << true << 30000 << "entry-30000" << false << false;
    QTest::newRow("paged-negative") << true << -1 << "entry-0" << false << false;
    QTest::newRow("paged-empty-id") << true << 500 << "" << false << false;
    QTest::newRow("dense-replace-known") << false << 0 << "entry-0" << true << true;
    QTest::newRow("dense-replace-unknown") << false << 2 << "entry-2" << true << false;
    QTest::newRow("paged-replace-unloaded") << true << 29999 << "entry-29999" << true << true;
}

void ExtUiSceneReducerTests::selectionPatchUsesLogicalCatalogRows()
{
    QFETCH(bool, paged);
    QFETCH(int, row);
    QFETCH(QString, entryId);
    QFETCH(bool, replace);
    QFETCH(bool, accepted);

    QVariantList entries;
    for (int index = paged ? 500 : 0; entries.size() < 2; ++index) {
        entries.push_back(QVariantMap{{"index", index},
            {"entryId", QStringLiteral("entry-%1").arg(index)}, {"selected", false}});
    }
    const QVariantMap panel{{"id", "left"}, {"side", 0},
        {"catalogRevision", 10}, {"selectionRevision", 4},
        {"catalogRowsDeferred", paged}, {"totalCount", paged ? 30000 : 2},
        {"entries", entries}};
    const QVariantMap scene{{"schema", "app"}, {"version", 4},
        {"shell", QVariantMap{{"panels", QVariantList{panel}}}}};
    QVariantMap operation{{"op", replace ? "selection_replace" : "selection_delta"},
        {"side", 0}, {"panelId", "left"}, {"catalogRevision", 10},
        {"baseSelectionRevision", 4}, {"selectionRevision", 5}};
    if (replace) {
        operation.insert("selectedEntryIds", QVariantList{entryId});
    } else {
        operation.insert("changes", QVariantList{QVariantMap{
            {"index", row}, {"entryId", entryId}, {"selected", true}}});
    }
    const QVariantMap patch{{"type", "scene_patch"}, {"schema", "app"},
        {"version", 4}, {"baseRevision", 1}, {"revision", 2},
        {"shell", QVariantMap{{"panels", QVariantList{operation}}}}};
    ExtUiSceneReducer::AppliedScenePatch result;
    QString error;
    const bool applied = ExtUiSceneReducer::applyScenePatch(patch, scene,
        ExtUiSceneReducer::makePresentationScene(scene), 1, &result, &error);
    QVERIFY2(applied == accepted, qPrintable(error));
    if (!accepted) {
        QVERIFY(!error.isEmpty());
        QVERIFY(result.panelPatches.isEmpty());
        return;
    }
    QCOMPARE(result.panelPatches.size(), 1);
    const QVariantMap updated = result.scene.value("shell").toMap()
        .value("panels").toList().first().toMap();
    QCOMPARE(updated.value("selectionRevision").toInt(), 5);
    QCOMPARE(updated.value("entries").toList(), entries);
    QVERIFY(!result.panelPatches.first().value("panel").toMap().contains("entries"));
}

QTEST_GUILESS_MAIN(ExtUiSceneReducerTests)

#include "ExtUiSceneReducerTests.moc"
