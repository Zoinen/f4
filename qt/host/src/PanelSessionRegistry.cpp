#include "PanelSessionRegistry.h"

#include <QtGlobal>
#include <utility>

bool PanelSessionRegistry::validSide(int side)
{
    return side >= 0 && side < PanelCount;
}

void PanelSessionRegistry::setSession(int side, QObject *session)
{
    Q_ASSERT(validSide(side));
    m_sessions[static_cast<size_t>(side)] = session;
}

QObject *PanelSessionRegistry::session(int side) const
{
    return validSide(side)
        ? m_sessions[static_cast<size_t>(side)].data() : nullptr;
}

PanelCatalogModel &PanelSessionRegistry::catalog(int side)
{
    Q_ASSERT(validSide(side));
    return m_catalogs[static_cast<size_t>(side)];
}

const PanelCatalogModel &PanelSessionRegistry::catalog(int side) const
{
    Q_ASSERT(validSide(side));
    return m_catalogs[static_cast<size_t>(side)];
}

const std::array<PanelCatalogModel, PanelSessionRegistry::PanelCount> &
PanelSessionRegistry::catalogs() const
{
    return m_catalogs;
}

void PanelSessionRegistry::resetCatalog(int side)
{
    catalog(side) = PanelCatalogModel{};
}

void PanelSessionRegistry::activatePanel(
    int side, const QString &panelId,
    const std::function<QObject *()> &create,
    const std::function<void(QObject *)> &release)
{
    Q_ASSERT(validSide(side));
    const size_t index = static_cast<size_t>(side);
    if (panelId.isEmpty() || m_panelIds[index] == panelId)
        return;
    if (m_panelIds[index].isEmpty()) {
        m_panelIds[index] = panelId;
        return;
    }
    auto &retained = m_retained[index];
    auto &recency = m_recency[index];
    const QString previous = m_panelIds[index];
    // Requests target the currently visible side. Replies for a hidden panel
    // are rejected by identity, so retain data, not its in-flight leases.
    auto &catalog = m_catalogs[index];
    catalog.catalogRowsRequestInFlight = false;
    catalog.catalogRowsRequestOffset = -1;
    catalog.catalogRowsRequestLimit = 0;
    catalog.groupPageRequestInFlight = false;
    catalog.groupPageRequestOffset = -1;
    catalog.groupPageRequestLimit = 0;
    catalog.metadataRequestInFlight = false;
    catalog.metadataRequestOffset = -1;
    catalog.metadataRequestLimit = 0;
    catalog.metadataAwaitingFrame = false;
    catalog.metadataRequiredRenderSyncSerial = 0;
    ++catalog.metadataPacingGeneration;
    retained.insert(previous, {m_sessions[index], std::move(m_catalogs[index])});
    recency.removeAll(previous);
    recency.append(previous);
    if (retained.contains(panelId)) {
        RetainedPanel restored = retained.take(panelId);
        recency.removeAll(panelId);
        m_sessions[index] = restored.session;
        m_catalogs[index] = std::move(restored.catalog);
    } else {
        m_sessions[index] = create();
        m_catalogs[index] = PanelCatalogModel{};
    }
    m_panelIds[index] = panelId;
    while (retained.size() > RetainedPanelsPerSide) {
        const RetainedPanel evicted = retained.take(recency.takeFirst());
        if (evicted.session)
            release(evicted.session);
    }
}

QObject *PanelSessionRegistry::sessionForPanel(int side, const QString &panelId) const
{
    if (!validSide(side))
        return nullptr;
    const size_t index = static_cast<size_t>(side);
    if (m_panelIds[index].isEmpty() || panelId.isEmpty()
        || panelId == m_panelIds[index])
        return session(side);
    const auto found = m_retained[index].constFind(panelId);
    return found == m_retained[index].cend() ? nullptr : found->session.data();
}
