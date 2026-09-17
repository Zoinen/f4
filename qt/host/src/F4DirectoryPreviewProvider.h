#pragma once

#include "QtMediaClient.h"
#include <ZoinGallery/DirectoryPreviewProvider.h>
#include <QPointer>

class F4DirectoryPreviewProvider final : public ZoinGallery::DirectoryPreviewProvider {
public:
    explicit F4DirectoryPreviewProvider(QtMediaClient *client) : m_client(client) {}
    ZoinGallery::DirectoryPreviewListing enumerate(
        const ZoinGallery::DirectorySourceDescriptor &source,
        const QSharedPointer<ZoinGallery::ImageSourceCancellation> &cancel) override;
    ZoinGallery::DirectoryPreviewResult resolve(
        const ZoinGallery::DirectorySourceDescriptor &source,
        const ZoinGallery::DirectoryPreviewListing &listing, const QStringList &names,
        const QSharedPointer<ZoinGallery::ImageSourceCancellation> &cancel) override;
private:
    QPointer<QtMediaClient> m_client;
};
