#include "F4DirectoryPreviewProvider.h"
#include "F4GallerySourceDescriptor.h"
#include <ZoinGallery/MediaTimingTrace.h>

namespace {
class PreviewLease final : public ZoinGallery::DirectoryPreviewLease {
public:
    PreviewLease(QtMediaClient *client, QString resource, const QtMediaResult &result)
        : client(client), resource(std::move(resource)), id(result.leaseId), scope(result.releaseScope) {}
    ~PreviewLease() override { if (client) client->release(resource, id, scope); }
    QPointer<QtMediaClient> client;
    QString resource;
    QString id;
    quint64 scope;
};
}

ZoinGallery::DirectoryPreviewListing F4DirectoryPreviewProvider::enumerate(
    const ZoinGallery::DirectorySourceDescriptor &source,
    const QSharedPointer<ZoinGallery::ImageSourceCancellation> &cancel)
{
    ZoinGallery::DirectoryPreviewListing output;
    if (!m_client) { output.error = QStringLiteral("media client is unavailable"); return output; }
    const auto result = m_client->directoryPreviewBlocking(
        QStringLiteral("enumerateDirectoryPreview"), source.resourceId, {}, 30000,
        [cancel] { return cancel && cancel->isCanceled(); });
    if (!result.ok) { output.error = result.error; return output; }
    output.lease = QSharedPointer<PreviewLease>::create(m_client, source.resourceId, result);
    for (const auto &entry : result.entries) output.names.append(entry.toMap().value(QStringLiteral("name")).toString());
    return output;
}

ZoinGallery::DirectoryPreviewResult F4DirectoryPreviewProvider::resolve(
    const ZoinGallery::DirectorySourceDescriptor &source,
    const ZoinGallery::DirectoryPreviewListing &listing, const QStringList &names,
    const QSharedPointer<ZoinGallery::ImageSourceCancellation> &cancel)
{
    ZoinGallery::DirectoryPreviewResult output;
    const auto lease = qSharedPointerDynamicCast<PreviewLease>(listing.lease);
    if (!m_client || !lease) { output.error = QStringLiteral("directory listing lease is unavailable"); return output; }
    const auto result = m_client->directoryPreviewBlocking(
        QStringLiteral("resolveDirectoryPreview"), source.resourceId,
        {{QStringLiteral("listingLeaseId"), lease->id}, {QStringLiteral("names"), names}}, 30000,
        [cancel] { return cancel && cancel->isCanceled(); });
    if (!result.ok) { output.error = result.error; return output; }
    for (const auto &value : result.entries) {
        QVariantMap entry = value.toMap();
        F4GallerySourceDescriptor::insertSourceDescriptorFields(&entry, value.toMap());
        entry.remove(QStringLiteral("source"));
        output.entries.append(entry);
    }
    ZoinGallery::MediaTimingTrace::event(QStringLiteral("qt.directory.sources.normalized"),
        {{QStringLiteral("fix"), QStringLiteral("[FIX:directory-wire-contract]")},
         {QStringLiteral("images"), output.entries.size()}});
    output.lease = QSharedPointer<PreviewLease>::create(m_client, source.resourceId, result);
    return output;
}
