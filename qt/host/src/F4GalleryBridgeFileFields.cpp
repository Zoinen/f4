#include "F4GalleryBridge.h"

#include <QMetaObject>

#include <ZoinGallery/GallerySession.h>

#include <utility>

void F4GalleryBridge::connectSessionFileFieldUpdates()
{
    for (int side = 0; side < PanelSessionRegistry::PanelCount; ++side) {
        auto *session = qobject_cast<ZoinGallery::GallerySession *>(
            m_panelSessions.session(side));
        if (!session) {
            continue;
        }
        connect(session, &ZoinGallery::GallerySession::fileFieldsRead,
                this, [this, side](const QVariantMap &update) {
            // Cached metadata can be restored while a catalog transaction is
            // being assembled. Deliver after it commits; Go rechecks identity.
            queueSessionFileFieldUpdates(side, {update});
        });
        connect(session, &ZoinGallery::GallerySession::fileFieldsReadBatch,
                this, [this, side](const QVariantList &updates) {
            queueSessionFileFieldUpdates(side, updates);
        });
    }
}

void F4GalleryBridge::queueSessionFileFieldUpdates(
    int side, const QVariantList &updates)
{
    if (!PanelSessionRegistry::validSide(side)) {
        return;
    }
    const auto &catalog = m_panelSessions.catalog(side);
    if (!catalog.initialized || catalog.panelId.isEmpty()) {
        return;
    }

    const size_t index = static_cast<size_t>(side);
    if (!m_pendingFileFieldPanelIds[index].isEmpty()
        && m_pendingFileFieldPanelIds[index] != catalog.panelId) {
        m_pendingFileFieldUpdates[index].clear();
        m_pendingFileFieldUpdateKeys[index].clear();
        m_fileFieldUpdateFlushScheduled[index] = false;
    }
    m_pendingFileFieldPanelIds[index] = catalog.panelId;

    for (const QVariant &value : updates) {
        if (!value.canConvert<QVariantMap>()) {
            continue;
        }
        const QVariantMap update = value.toMap();
        const QString sourceKey = update.value(
            QStringLiteral("sourceKey")).toString();
        const QString sourceVersion = update.value(
            QStringLiteral("sourceVersion")).toString();
        if (sourceKey.isEmpty() || sourceVersion.isEmpty()) {
            continue;
        }
        const QString updateKey = sourceKey + QChar(0x1f) + sourceVersion
            + QChar(0x1f)
            + update.value(QStringLiteral("generation")).toString();
        if (m_pendingFileFieldUpdateKeys[index].contains(updateKey)) {
            continue;
        }
        m_pendingFileFieldUpdateKeys[index].insert(updateKey);
        m_pendingFileFieldUpdates[index].append(update);
    }

    if (m_pendingFileFieldUpdates[index].isEmpty()
        || m_fileFieldUpdateFlushScheduled[index]) {
        return;
    }
    m_fileFieldUpdateFlushScheduled[index] = true;
    QMetaObject::invokeMethod(this, [this, side] {
        flushSessionFileFieldUpdates(side);
    }, Qt::QueuedConnection);
}

void F4GalleryBridge::flushSessionFileFieldUpdates(int side)
{
    if (!PanelSessionRegistry::validSide(side)) {
        return;
    }
    const size_t index = static_cast<size_t>(side);
    m_fileFieldUpdateFlushScheduled[index] = false;
    const QString panelId = std::exchange(
        m_pendingFileFieldPanelIds[index], QString());
    QList<QVariantMap> pending = std::exchange(
        m_pendingFileFieldUpdates[index], {});
    m_pendingFileFieldUpdateKeys[index].clear();
    const auto &catalog = m_panelSessions.catalog(side);
    if (pending.isEmpty() || !catalog.initialized
        || panelId.isEmpty() || catalog.panelId != panelId) {
        return;
    }

    QVariantList updates;
    updates.reserve(pending.size());
    for (const QVariantMap &update : std::as_const(pending)) {
        updates.append(update);
    }
    emit uiActionRequested({
        {QStringLiteral("action"),
         QStringLiteral("panel.fileFields.updateBatch")},
        {QStringLiteral("side"), side},
        {QStringLiteral("panelId"), panelId},
        {QStringLiteral("updates"), updates},
    });
}
