#include "ExtUiProtocol.h"

#include <QMetaType>

namespace
{
bool nonNegativeInteger(const QVariant &value, quint64 *result)
{
    bool ok = false;
    const quint64 number = value.toULongLong(&ok);
    if (!ok) {
        return false;
    }
    if (result) {
        *result = number;
    }
    return true;
}
}

namespace ExtUiProtocol
{
bool isEnvelope(const QVariantMap &message)
{
    return message.value(QStringLiteral("type")).toString()
        == QStringLiteral("extui");
}

bool isSemanticPayloadType(const QString &type)
{
    static const QSet<QString> semanticTypes = {
        QStringLiteral("scene"),
        QStringLiteral("scene_patch"),
        QStringLiteral("panel_catalog"),
        QStringLiteral("panel_chrome"),
        QStringLiteral("panel_activation"),
        QStringLiteral("command_line"),
        QStringLiteral("panel_catalog_metadata"),
        QStringLiteral("panel_catalog_metadata_rejected"),
        QStringLiteral("panel_catalog_rows"),
        QStringLiteral("panel_catalog_rows_rejected"),
        QStringLiteral("chrome_snapshot"),
        QStringLiteral("workspaces_snapshot"),
        QStringLiteral("menus_snapshot"),
        QStringLiteral("dialogs_snapshot"),
        QStringLiteral("operations_snapshot"),
        QStringLiteral("command_line_snapshot"),
        QStringLiteral("shell_snapshot"),
        QStringLiteral("panel_catalog_snapshot"),
        QStringLiteral("document_snapshot"),
    };
    return semanticTypes.contains(type);
}

Inspection StreamRegistry::inspect(const QVariantMap &wireMessage)
{
    Inspection result;
    static const QSet<QString> envelopeKeys = {
        QStringLiteral("type"), QStringLiteral("version"),
        QStringLiteral("sequence"), QStringLiteral("streamId"),
        QStringLiteral("revision"), QStringLiteral("baseRevision"),
        QStringLiteral("kind"), QStringLiteral("payload"),
    };
    for (auto it = wireMessage.cbegin(); it != wireMessage.cend(); ++it) {
        if (!envelopeKeys.contains(it.key())) {
            result.error = QStringLiteral("Unknown ExtUI v4 envelope field");
            return result;
        }
    }

    quint64 version = 0;
    const QVariant payloadValue = wireMessage.value(QStringLiteral("payload"));
    Envelope &envelope = result.envelope;
    envelope.streamId = wireMessage.value(QStringLiteral("streamId")).toString();
    envelope.kind = wireMessage.value(QStringLiteral("kind")).toString();
    envelope.hasBaseRevision = wireMessage.contains(
        QStringLiteral("baseRevision"));
    if (!nonNegativeInteger(wireMessage.value(QStringLiteral("version")),
                            &version)
        || version != Version
        || !nonNegativeInteger(wireMessage.value(QStringLiteral("sequence")),
                               &envelope.sequence)
        || envelope.sequence != m_nextSequence
        || envelope.streamId.isEmpty() || envelope.kind.isEmpty()
        || !nonNegativeInteger(wireMessage.value(QStringLiteral("revision")),
                               &envelope.revision)
        || envelope.revision == 0
        || payloadValue.metaType().id() != QMetaType::QVariantMap) {
        result.error = QStringLiteral("Invalid ExtUI v4 envelope");
        return result;
    }
    envelope.payload = payloadValue.toMap();
    ++m_nextSequence;

    const quint64 currentRevision = m_revisions.value(envelope.streamId, 0);
    if (envelope.kind == QStringLiteral("snapshot")) {
        if (envelope.hasBaseRevision) {
            result.error = QStringLiteral(
                "ExtUI stream snapshot contains baseRevision");
            return result;
        }
        if (envelope.revision <= currentRevision) {
            result.disposition = Disposition::IgnoreStale;
            return result;
        }
        result.disposition = Disposition::Apply;
        return result;
    }

    if (!envelope.hasBaseRevision
        || !nonNegativeInteger(
            wireMessage.value(QStringLiteral("baseRevision")),
            &envelope.baseRevision)
        || envelope.revision != envelope.baseRevision + 1) {
        result.error = QStringLiteral("Invalid ExtUI stream revision pair");
        return result;
    }
    if (envelope.revision <= currentRevision
        || envelope.baseRevision < currentRevision) {
        result.disposition = Disposition::IgnoreStale;
        return result;
    }
    if (envelope.baseRevision > currentRevision) {
        result.disposition = Disposition::RequestSnapshot;
        if (!m_resyncPending.contains(envelope.streamId)) {
            m_resyncPending.insert(envelope.streamId);
            result.snapshotRequest = {
                {QStringLiteral("type"),
                 QStringLiteral("stream_snapshot_request")},
                {QStringLiteral("streamId"), envelope.streamId},
                {QStringLiteral("revision"),
                 QVariant::fromValue<qulonglong>(currentRevision)},
            };
        }
        return result;
    }

    result.disposition = Disposition::Apply;
    return result;
}

void StreamRegistry::commit(const Envelope &envelope)
{
    m_revisions.insert(envelope.streamId, envelope.revision);
    m_resyncPending.remove(envelope.streamId);
}

void StreamRegistry::reset()
{
    m_nextSequence = 1;
    m_revisions.clear();
    m_resyncPending.clear();
}

quint64 StreamRegistry::revision(const QString &streamId) const
{
    return m_revisions.value(streamId, 0);
}
}
