#include "PanelIntentController.h"

#include <QSignalSpy>
#include <QtTest>

class PanelIntentControllerTests final : public QObject
{
    Q_OBJECT

private slots:
    void cursorIntentHasFixedWireShape();
    void selectionTransactionCarriesTypedOptionalFields();
    void dispatchRemainsTypedUntilBoundary();
};

void PanelIntentControllerTests::cursorIntentHasFixedWireShape()
{
    PanelIntent intent;
    intent.kind = PanelIntent::Kind::Cursor;
    intent.side = 1;
    intent.entryId = QStringLiteral("stable-entry");
    intent.index = 42;
    intent.catalogRevision = 19;
    intent.includeCatalogRevision = true;
    intent.activate = true;

    QCOMPARE(PanelIntentController::toWireMap(intent), QVariantMap({
        {QStringLiteral("action"), QStringLiteral("panel.cursor")},
        {QStringLiteral("side"), 1},
        {QStringLiteral("entryId"), QStringLiteral("stable-entry")},
        {QStringLiteral("index"), 42},
        {QStringLiteral("catalogRevision"), qulonglong(19)},
        {QStringLiteral("activate"), true},
    }));
}

void PanelIntentControllerTests::selectionTransactionCarriesTypedOptionalFields()
{
    PanelIntent intent;
    intent.kind = PanelIntent::Kind::SetSelection;
    intent.side = 0;
    intent.mode = QStringLiteral("set");
    intent.changes = {
        QVariantMap{{QStringLiteral("entryId"), QStringLiteral("a")},
                    {QStringLiteral("selected"), true}},
    };
    intent.cursorEntryId = QStringLiteral("a");
    intent.cursorIndex = 7;
    intent.catalogRevision = 11;
    intent.includeCatalogRevision = true;
    intent.selectionRevision = 4;

    const QVariantMap wire = PanelIntentController::toWireMap(intent);
    QCOMPARE(wire.value(QStringLiteral("action")).toString(),
             QStringLiteral("panel.setSelection"));
    QCOMPARE(wire.value(QStringLiteral("changes")).toList(),
             intent.changes);
    QVERIFY(!wire.contains(QStringLiteral("entryIds")));
    QCOMPARE(wire.value(QStringLiteral("cursorEntryId")).toString(),
             QStringLiteral("a"));
    QCOMPARE(wire.value(QStringLiteral("selectionRevision")).toULongLong(),
             qulonglong(4));
}

void PanelIntentControllerTests::dispatchRemainsTypedUntilBoundary()
{
    PanelIntentController controller;
    QSignalSpy requested(&controller,
                         &PanelIntentController::intentRequested);
    QVERIFY(requested.isValid());

    PanelIntent intent;
    intent.kind = PanelIntent::Kind::SetGalleryDensity;
    intent.side = 1;
    intent.layoutMode = QStringLiteral("icons");
    intent.density = 96;
    controller.dispatch(intent);

    QCOMPARE(requested.size(), 1);
    const PanelIntent received = qvariant_cast<PanelIntent>(
        requested.constFirst().constFirst());
    QCOMPARE(received.kind, PanelIntent::Kind::SetGalleryDensity);
    QCOMPARE(received.layoutMode, QStringLiteral("icons"));
    QCOMPARE(received.density, 96);
}

QTEST_GUILESS_MAIN(PanelIntentControllerTests)

#include "PanelIntentControllerTests.moc"
