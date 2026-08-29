#pragma once

#include <QPointer>

#include <ZoinGallery/ImageSourceProvider.h>

class QtMediaClient;

// Native ZoinGallery adapter for f4's resource broker. ZoinGallery invokes
// this interface synchronously on decode workers; QtMediaClient keeps the
// socket asynchronous on its own thread and these calls wait only on the
// calling decode worker.
class F4ImageSourceProvider final
    : public ZoinGallery::ImageSourceProvider
{
public:
    explicit F4ImageSourceProvider(QtMediaClient *client);

    ZoinGallery::ImageSourceReadResult readRange(
        const ZoinGallery::ImageSourceDescriptor &source,
        qint64 offset, qint64 length,
        const QSharedPointer<ZoinGallery::ImageSourceCancellation>
            &cancellation) override;

    QSharedPointer<ZoinGallery::ImageSourceLease> materialize(
        const ZoinGallery::ImageSourceDescriptor &source,
        const QSharedPointer<ZoinGallery::ImageSourceCancellation>
            &cancellation) override;

private:
    QPointer<QtMediaClient> m_client;
};
