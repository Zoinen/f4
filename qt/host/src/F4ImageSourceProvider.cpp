#include "F4ImageSourceProvider.h"

#include "QtMediaClient.h"

#include <QFileInfo>

#include <algorithm>
#include <utility>

namespace
{
constexpr qint64 ConservativeChunkSize = 256 * 1024;
constexpr qint64 MaxLogicalRangeSize = 64 * 1024 * 1024;
constexpr int RangeTimeoutMs = 30000;
constexpr int MaterializeTimeoutMs = 120000;

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
    if (!m_client) {
        output.errorString = QStringLiteral("f4 media client is unavailable");
        return output;
    }
    if (!source.isValid() || offset < 0 || length <= 0
        || length > MaxLogicalRangeSize) {
        output.errorString = QStringLiteral("invalid f4 media range request");
        return output;
    }
    if (canceled(cancellation)) {
        output.errorString = QStringLiteral("cancelled");
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
            return output;
        }
        const qint64 requested = std::min(chunkLimit, length - consumed);
        const QtMediaResult result = m_client->readRangeBlocking(
            source.resourceId, offset + consumed, requested, RangeTimeoutMs,
            [cancellation]() { return canceled(cancellation); });
        if (!result.ok) {
            output.data.clear();
            output.errorString = mediaError(result);
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
    return output;
}

QSharedPointer<ZoinGallery::ImageSourceLease>
F4ImageSourceProvider::materialize(
    const ZoinGallery::ImageSourceDescriptor &source,
    const QSharedPointer<ZoinGallery::ImageSourceCancellation> &cancellation)
{
    if (!m_client || !source.isValid() || canceled(cancellation)) {
        return {};
    }
    const QtMediaResult result = m_client->materializeBlocking(
        source.resourceId, MaterializeTimeoutMs,
        [cancellation]() { return canceled(cancellation); });
    if (!result.ok || result.path.isEmpty() || canceled(cancellation)) {
        if (result.ok && !result.path.isEmpty()) {
            m_client->release(source.resourceId, result.leaseId,
                              result.releaseScope);
        }
        return {};
    }
    return QSharedPointer<F4ImageSourceLease>::create(
        m_client.data(), source.resourceId, result.leaseId,
        result.releaseScope, result.path);
}
