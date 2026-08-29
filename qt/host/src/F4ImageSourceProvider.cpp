#include "F4ImageSourceProvider.h"

#include "QtMediaClient.h"

#include <ZoinGallery/MediaTimingTrace.h>

#include <QCoreApplication>
#include <QFileInfo>

#include <algorithm>
#include <atomic>
#include <utility>

namespace
{
constexpr qint64 ConservativeChunkSize = 256 * 1024;
constexpr qint64 MaxLogicalRangeSize = 64 * 1024 * 1024;
// A probe is deliberately bounded tightly: it is only a speculative first
// pass and must not hold both AFC/native-range workers for half a minute when
// a stale remote transport stops answering.  Full materialization gets a
// larger, but still finite, budget because it is the authoritative viewer
// path and can legitimately transfer a larger image.
constexpr int RangeTimeoutMs = 8000;
constexpr int MaterializeTimeoutMs = 45000;

std::atomic<quint64> NextSourceTrace{1};

QString nextSourceTraceId(const QString &operation)
{
    return QStringLiteral("qt-provider-%1-%2-%3")
        .arg(QCoreApplication::applicationPid())
        .arg(operation)
        .arg(NextSourceTrace.fetch_add(1, std::memory_order_relaxed));
}

class F4ImageSourceLease final : public ZoinGallery::ImageSourceLease
{
public:
    F4ImageSourceLease(QtMediaClient *client, QString resourceId,
                       QString leaseId, quint64 releaseScope, QString path)
        : m_client(client)
        , m_resourceId(std::move(resourceId))
        , m_leaseId(std::move(leaseId))
        , m_releaseScope(releaseScope)
        , m_path(std::move(path))
    {
    }

    ~F4ImageSourceLease() override
    {
        if (m_client) {
            m_client->release(m_resourceId, m_leaseId, m_releaseScope);
        }
    }

    QString localPath() const override
    {
        return m_path;
    }

    qint64 retainedBytes() const override
    {
        const QFileInfo info(m_path);
        return info.exists() && info.isFile() ? info.size() : -1;
    }

private:
    QPointer<QtMediaClient> m_client;
    QString m_resourceId;
    QString m_leaseId;
    quint64 m_releaseScope = 0;
    QString m_path;
};

bool canceled(const QSharedPointer<ZoinGallery::ImageSourceCancellation>
                  &cancellation)
{
    return cancellation && cancellation->isCanceled();
}

QString mediaError(const QtMediaResult &result)
{
    if (result.errorCode.isEmpty()) {
        return result.error;
    }
    if (result.error.isEmpty()) {
        return result.errorCode;
    }
    return QStringLiteral("%1: %2").arg(result.errorCode, result.error);
}
}

F4ImageSourceProvider::F4ImageSourceProvider(QtMediaClient *client)
    : m_client(client)
{
}

ZoinGallery::ImageSourceReadResult F4ImageSourceProvider::readRange(
    const ZoinGallery::ImageSourceDescriptor &source,
    qint64 offset, qint64 length,
    const QSharedPointer<ZoinGallery::ImageSourceCancellation> &cancellation)
{
    ZoinGallery::ImageSourceReadResult output;
    const QString traceId = nextSourceTraceId(QStringLiteral("range"));
    QVariantMap traceFields =
        ZoinGallery::MediaTimingTrace::sourceFields(source);
    traceFields.insert(QStringLiteral("traceId"), traceId);
    traceFields.insert(QStringLiteral("offset"), offset);
    traceFields.insert(QStringLiteral("requestedBytes"), length);
    ZoinGallery::MediaTimingTrace::Span span(
        QStringLiteral("qt.provider.range"), traceFields);
    if (!m_client) {
        output.errorString = QStringLiteral("f4 media client is unavailable");
        span.set(QStringLiteral("outcome"), QStringLiteral("unavailable"));
        return output;
    }
    if (!source.isValid() || offset < 0 || length <= 0
        || length > MaxLogicalRangeSize) {
        output.errorString = QStringLiteral("invalid f4 media range request");
        span.set(QStringLiteral("outcome"), QStringLiteral("invalid"));
        return output;
    }
    if (canceled(cancellation)) {
        output.errorString = QStringLiteral("cancelled");
        span.set(QStringLiteral("outcome"), QStringLiteral("cancelled"));
        return output;
    }

    const qint64 advertised = m_client->maxChunkSize();
    const qint64 chunkLimit = advertised > 0
        ? advertised : ConservativeChunkSize;
    output.data.reserve(static_cast<qsizetype>(length));

    qint64 consumed = 0;
    while (consumed < length) {
        if (canceled(cancellation)) {
            output.data.clear();
            output.errorString = QStringLiteral("cancelled");
            span.set(QStringLiteral("outcome"), QStringLiteral("cancelled"));
            return output;
        }
        const qint64 requested = std::min(chunkLimit, length - consumed);
        const QtMediaResult result = m_client->readRangeBlocking(
            source.resourceId, offset + consumed, requested, RangeTimeoutMs,
            [cancellation]() { return canceled(cancellation); }, traceId);
        if (!result.ok) {
            output.data.clear();
            output.errorString = mediaError(result);
            span.set(QStringLiteral("outcome"), QStringLiteral("failed"));
            span.set(QStringLiteral("errorCode"), result.errorCode);
            span.set(QStringLiteral("error"), result.error);
            return output;
        }
        output.data.append(result.data);
        consumed += result.data.size();
        if (result.endOfFile || result.data.size() < requested) {
            output.endOfFile = true;
            break;
        }
        if (result.data.isEmpty()) {
            output.endOfFile = true;
            break;
        }
    }
    span.set(QStringLiteral("outcome"), QStringLiteral("ok"));
    span.set(QStringLiteral("bytes"), output.data.size());
    span.set(QStringLiteral("endOfFile"), output.endOfFile);
    return output;
}

QSharedPointer<ZoinGallery::ImageSourceLease>
F4ImageSourceProvider::materialize(
    const ZoinGallery::ImageSourceDescriptor &source,
    const QSharedPointer<ZoinGallery::ImageSourceCancellation> &cancellation)
{
    const QString traceId = nextSourceTraceId(QStringLiteral("materialize"));
    QVariantMap traceFields =
        ZoinGallery::MediaTimingTrace::sourceFields(source);
    traceFields.insert(QStringLiteral("traceId"), traceId);
    ZoinGallery::MediaTimingTrace::Span span(
        QStringLiteral("qt.provider.materialize"), traceFields);
    if (!m_client || !source.isValid() || canceled(cancellation)) {
        span.set(QStringLiteral("outcome"), QStringLiteral("rejected"));
        return {};
    }
    const QtMediaResult result = m_client->materializeBlocking(
        source.resourceId, MaterializeTimeoutMs,
        [cancellation]() { return canceled(cancellation); }, traceId);
    if (!result.ok || result.path.isEmpty() || canceled(cancellation)) {
        if (result.ok && !result.path.isEmpty()) {
            m_client->release(source.resourceId, result.leaseId,
                              result.releaseScope);
        }
        span.set(QStringLiteral("outcome"),
                 canceled(cancellation) ? QStringLiteral("cancelled")
                                        : QStringLiteral("failed"));
        span.set(QStringLiteral("errorCode"), result.errorCode);
        span.set(QStringLiteral("error"), result.error);
        return {};
    }
    span.set(QStringLiteral("outcome"), QStringLiteral("ok"));
    span.set(QStringLiteral("bytes"), result.size);
    span.set(QStringLiteral("retainedPathBytes"), QFileInfo(result.path).size());
    return QSharedPointer<F4ImageSourceLease>::create(
        m_client.data(), source.resourceId, result.leaseId,
        result.releaseScope, result.path);
}
