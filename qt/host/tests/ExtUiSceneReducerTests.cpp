#include "ExtUiSceneReducer.h"

#include <QTest>

class ExtUiSceneReducerTests final : public QObject
{
    Q_OBJECT

private slots:
    void presentationProjectionDropsNativeCatalogRows();
    void snapshotMutatesOnlyItsOwnedStream();
    void snapshotPayloadTypeMustMatchStream();
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

QTEST_GUILESS_MAIN(ExtUiSceneReducerTests)

#include "ExtUiSceneReducerTests.moc"
