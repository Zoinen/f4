#pragma once

#include <QHash>
#include <QSet>
#include <QString>
#include <QVariantMap>

namespace ExtUiProtocol
{
inline constexpr int Version = 4;

struct Envelope
{
    quint64 sequence = 0;
    QString streamId;
    quint64 revision = 0;
    quint64 baseRevision = 0;
    QString kind;
    QVariantMap payload;
    bool hasBaseRevision = false;
};

enum class Disposition
{
    Apply,
    IgnoreStale,
    RequestSnapshot,
    Fatal,
};

struct Inspection
{
    Disposition disposition = Disposition::Fatal;
    Envelope envelope;
    QString error;
    QVariantMap snapshotRequest;
};

// Validates the typed v4 envelope and owns only transport ordering metadata.
// Payload reducers commit a revision after they have applied successfully, so
// malformed stream data can never advance the authoritative stream cursor.
class StreamRegistry
{
public:
    Inspection inspect(const QVariantMap &wireMessage);
    void commit(const Envelope &envelope);
    void reset();

    quint64 revision(const QString &streamId) const;
    quint64 nextSequence() const { return m_nextSequence; }

private:
    quint64 m_nextSequence = 1;
    QHash<QString, quint64> m_revisions;
    QSet<QString> m_resyncPending;
};

bool isEnvelope(const QVariantMap &message);
bool isSemanticPayloadType(const QString &type);
}
