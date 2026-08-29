#pragma once

#include <QByteArray>
#include <QObject>
#include <QThread>
#include <QVariantMap>

#include <atomic>
#include <functional>
#include <memory>

class QtMediaWorker;

struct QtMediaResult {
    bool ok = false;
    bool endOfFile = false;
    QByteArray data;
    QString path;
    QString leaseId;
    quint64 releaseScope = 0;
    qint64 size = -1;
    QString errorCode;
    QString error;
};

// A bounded, independently backpressured client for f4's media byte broker.
// Public methods are safe to call from any thread. Asynchronous result signals
// are always delivered on this object's thread; the blocking methods are for
// decoder workers and must not be called from the owning (normally GUI) thread.
class QtMediaClient final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool ready READ ready NOTIFY readyChanged)

public:
    explicit QtMediaClient(QObject *parent = nullptr);
    ~QtMediaClient() override;

    bool ready() const;
    qint64 maxChunkSize() const;

    void configure(const QVariantMap &advertisement);
    void clearConfiguration();

    QString readRange(const QString &resourceId, qint64 offset,
                      qint64 length, int timeoutMs = 15000);
    QString materialize(const QString &resourceId,
                        int timeoutMs = 120000);
    void cancel(const QString &requestId);
    void release(const QString &resourceId,
                 const QString &leaseId = QString(),
                 quint64 releaseScope = 0);

    QtMediaResult readRangeBlocking(
        const QString &resourceId, qint64 offset, qint64 length,
        int timeoutMs = 15000,
        const std::function<bool()> &isCanceled = {},
        const QString &traceId = {});
    QtMediaResult materializeBlocking(
        const QString &resourceId, int timeoutMs = 120000,
        const std::function<bool()> &isCanceled = {},
        const QString &traceId = {});

signals:
    void readyChanged();
    void rangeReady(const QString &requestId, const QByteArray &data,
                    bool endOfFile);
    void materialized(const QString &requestId, const QString &path,
                      const QString &leaseId, qint64 size);
    void requestFailed(const QString &requestId, const QString &errorCode,
                       const QString &message);
    void transportError(const QString &message);

private:
    friend class QtMediaWorker;

    QString nextRequestId();
    void setReadyFromWorker(bool ready, qint64 maxChunkSize);
    void completeFromWorker(const QString &requestId, const QString &operation,
                            const QtMediaResult &result);
    void transportErrorFromWorker(const QString &message);

    QtMediaResult requestBlocking(
        const QString &operation, const QString &resourceId,
        qint64 offset, qint64 length, int timeoutMs,
        const std::function<bool()> &isCanceled,
        const QString &traceId);

    QThread m_thread;
    QtMediaWorker *m_worker = nullptr;
    std::atomic<quint64> m_nextRequest{1};
    std::atomic_bool m_ready{false};
    std::atomic<qint64> m_maxChunkSize{0};
};
