#include "ExtUiProtocol.h"
#include "ExtUiMessageDecoder.h"

#include <QFile>
#include <QSignalSpy>
#include <QtTest>

#include <optional>

namespace
{
QVariantMap envelope(quint64 sequence, const QString &streamId,
                     quint64 revision, const QString &kind,
                     const QVariantMap &payload,
                     std::optional<quint64> baseRevision = std::nullopt)
{
    QVariantMap result{
        {QStringLiteral("type"), QStringLiteral("extui")},
        {QStringLiteral("version"), ExtUiProtocol::Version},
        {QStringLiteral("sequence"), sequence},
        {QStringLiteral("streamId"), streamId},
        {QStringLiteral("revision"), revision},
        {QStringLiteral("kind"), kind},
        {QStringLiteral("payload"), payload},
    };
    if (baseRevision.has_value()) {
        result.insert(QStringLiteral("baseRevision"),
                      baseRevision.value());
    }
    return result;
}
}

class ExtUiProtocolTests final : public QObject
{
    Q_OBJECT

private slots:
    void decodesGoGeneratedGoldenFixtures()
    {
        QFile fixture(QString::fromUtf8(F4_EXTUI_GOLDEN_FIXTURE));
        QVERIFY2(fixture.open(QIODevice::ReadOnly),
                 qPrintable(fixture.errorString()));

        ExtUiMessageDecoder decoder;
        QSignalSpy decoded(&decoder, &ExtUiMessageDecoder::decoded);
        QSignalSpy failed(&decoder, &ExtUiMessageDecoder::failed);
        decoder.decode(fixture.readAll(), 11, 17);
        QCOMPARE(failed.size(), 0);
        QCOMPARE(decoded.size(), 1);
        QCOMPARE(decoded.at(0).at(0).toULongLong(), quint64(11));
        QCOMPARE(decoded.at(0).at(1).toULongLong(), quint64(17));

        const QVariantList fixtures = decoded.at(0).at(2).toList();
        QCOMPARE(fixtures.size(), 4);
        ExtUiProtocol::StreamRegistry registry;
        QStringList streams;
        for (const QVariant &fixtureValue : fixtures) {
            const QVariantMap wire = fixtureValue.toMap();
            const auto inspection = registry.inspect(wire);
            QCOMPARE(inspection.disposition,
                     ExtUiProtocol::Disposition::Apply);
            streams.push_back(inspection.envelope.streamId);
            registry.commit(inspection.envelope);
        }
        QCOMPARE(streams, QStringList({QStringLiteral("command-line"),
                                      QStringLiteral("panel/0"),
                                      QStringLiteral("menus"),
                                      QStringLiteral("menus")}));

        const QVariantMap panelEnvelope = fixtures.at(1).toMap();
        const QVariantMap panelState = panelEnvelope
            .value(QStringLiteral("payload")).toMap()
            .value(QStringLiteral("state")).toMap();
        const QVariantMap panel = panelState.value(
            QStringLiteral("panel")).toMap();
        QCOMPARE(panel.value(QStringLiteral("totalCount")).toInt(), 30000);
        QCOMPARE(panel.value(QStringLiteral("entries")).toList().size(), 2);
        QCOMPARE(panel.value(QStringLiteral("catalogRevision")).toULongLong(),
                 quint64(19));
        QCOMPARE(registry.revision(QStringLiteral("panel/0")), quint64(7));
        QCOMPARE(registry.revision(QStringLiteral("menus")), quint64(5));
    }

    void independentStreamsAdvanceIndependently()
    {
        ExtUiProtocol::StreamRegistry registry;
        auto shell = registry.inspect(envelope(
            1, QStringLiteral("shell"), 1, QStringLiteral("snapshot"),
            {{QStringLiteral("type"), QStringLiteral("shell_snapshot")}}));
        QCOMPARE(shell.disposition, ExtUiProtocol::Disposition::Apply);
        registry.commit(shell.envelope);

        auto panel = registry.inspect(envelope(
            2, QStringLiteral("panel/0"), 1, QStringLiteral("reset"),
            {{QStringLiteral("type"), QStringLiteral("panel_catalog")}}, 0));
        QCOMPARE(panel.disposition, ExtUiProtocol::Disposition::Apply);
        registry.commit(panel.envelope);

        QCOMPARE(registry.revision(QStringLiteral("shell")), quint64(1));
        QCOMPARE(registry.revision(QStringLiteral("panel/0")), quint64(1));
    }

    void revisionGapRequestsOnlyAffectedStream()
    {
        ExtUiProtocol::StreamRegistry registry;
        auto shell = registry.inspect(envelope(
            1, QStringLiteral("shell"), 1, QStringLiteral("snapshot"),
            {{QStringLiteral("type"), QStringLiteral("shell_snapshot")}}));
        registry.commit(shell.envelope);

        auto gap = registry.inspect(envelope(
            2, QStringLiteral("panel/1"), 4, QStringLiteral("rows"),
            {{QStringLiteral("type"), QStringLiteral("panel_catalog_rows")}},
            3));
        QCOMPARE(gap.disposition,
                 ExtUiProtocol::Disposition::RequestSnapshot);
        QCOMPARE(gap.snapshotRequest.value(QStringLiteral("streamId"))
                     .toString(), QStringLiteral("panel/1"));
        QCOMPARE(gap.snapshotRequest.value(QStringLiteral("revision"))
                     .toULongLong(), quint64(0));
        QCOMPARE(registry.revision(QStringLiteral("shell")), quint64(1));
    }

    void staleResponseAfterSnapshotIsDiscarded()
    {
        ExtUiProtocol::StreamRegistry registry;
        auto snapshot = registry.inspect(envelope(
            1, QStringLiteral("document/editor"), 8,
            QStringLiteral("snapshot"),
            {{QStringLiteral("type"), QStringLiteral("document_snapshot")}}));
        registry.commit(snapshot.envelope);

        auto stale = registry.inspect(envelope(
            2, QStringLiteral("document/editor"), 7,
            QStringLiteral("patch"),
            {{QStringLiteral("type"), QStringLiteral("document_patch")}}, 6));
        QCOMPARE(stale.disposition,
                 ExtUiProtocol::Disposition::IgnoreStale);
        QCOMPARE(registry.revision(QStringLiteral("document/editor")),
                 quint64(8));
    }

    void unknownEnvelopeFieldIsFatal()
    {
        ExtUiProtocol::StreamRegistry registry;
        QVariantMap wire = envelope(
            1, QStringLiteral("menus"), 1, QStringLiteral("snapshot"),
            {{QStringLiteral("type"), QStringLiteral("menus_snapshot")}});
        wire.insert(QStringLiteral("catalog"), QVariantList{});
        const auto result = registry.inspect(wire);
        QCOMPARE(result.disposition, ExtUiProtocol::Disposition::Fatal);
        QVERIFY(!result.error.isEmpty());
    }

    void malformedEnvelopePayloadIsFatal()
    {
        ExtUiProtocol::StreamRegistry registry;
        QVariantMap wire = envelope(
            1, QStringLiteral("shell"), 1, QStringLiteral("snapshot"), {});
        wire.insert(QStringLiteral("payload"), QVariantList{});
        const auto result = registry.inspect(wire);
        QCOMPARE(result.disposition, ExtUiProtocol::Disposition::Fatal);
        QVERIFY(!result.error.isEmpty());
    }
};

QTEST_GUILESS_MAIN(ExtUiProtocolTests)

#include "ExtUiProtocolTests.moc"
