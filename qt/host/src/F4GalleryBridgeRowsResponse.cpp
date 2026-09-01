#include "F4GalleryBridge.h"

#include <ZoinGallery/GallerySession.h>

class F4GalleryRowsResponseReducer
{
public:
    F4GalleryRowsResponseReducer(
        F4GalleryBridge &bridge, const QVariantMap &message)
        : m_bridge(bridge), m_message(message)
    {
    }

    void run();

private:
    bool resolveTarget();
    bool parseEnvelope();
    bool validateRows() const;
    bool normalizeRows();
    bool applyRowsToSession();
    bool commitRows();
    void clearRequest();
    void finish();

    F4GalleryBridge &m_bridge;
    const QVariantMap &m_message;
    QString m_type;
    int m_side = -1;
    F4GalleryBridge::SideState *m_state = nullptr;
    int m_offset = -1;
    int m_limit = 0;
    int m_total = 0;
    QVariantList m_sourceEntries;
    QVariantList m_entries;
    QVariantMap m_highlightStyles;
};

bool F4GalleryRowsResponseReducer::resolveTarget()
{
    m_type = m_message.value(QStringLiteral("type")).toString();
    if (m_type != QStringLiteral("panel_catalog_rows")
        && m_type != QStringLiteral("panel_catalog_rows_rejected")) {
        return false;
    }
    m_side = m_bridge.matchingCatalogRowsSide(m_message);
    if (!F4GalleryBridge::validSide(m_side)) {
        return false;
    }
    m_state = &m_bridge.m_panelSessions.catalog(m_side);
    return true;
}

void F4GalleryRowsResponseReducer::clearRequest()
{
    m_state->catalogRowsRequestInFlight = false;
    m_state->catalogRowsRequestOffset = -1;
    m_state->catalogRowsRequestLimit = 0;
}

bool F4GalleryRowsResponseReducer::parseEnvelope()
{
    bool limitOK = false;
    bool totalOK = false;
    m_limit = m_message.value(QStringLiteral("limit")).toInt(&limitOK);
    m_total = m_message.value(QStringLiteral("total")).toInt(&totalOK);
    const QVariant entriesValue = m_message.value(
        QStringLiteral("entries"));
    const QVariant stylesValue = m_message.value(
        QStringLiteral("highlightStyles"));
    m_offset = m_state->catalogRowsRequestOffset;
    if (!limitOK || !totalOK || m_limit <= 0
        || m_limit != m_state->catalogRowsRequestLimit
        || m_total != m_state->totalCount
        || entriesValue.metaType().id() != QMetaType::QVariantList
        || (m_message.contains(QStringLiteral("highlightStyles"))
            && stylesValue.metaType().id() != QMetaType::QVariantMap)) {
        return false;
    }
    m_sourceEntries = entriesValue.toList();
    m_highlightStyles = stylesValue.toMap();
    return m_sourceEntries.size() == m_limit && m_offset >= 0
        && m_offset + m_limit <= m_total;
}

bool F4GalleryRowsResponseReducer::validateRows() const
{
    for (qsizetype index = 0; index < m_sourceEntries.size(); ++index) {
        if (m_sourceEntries.at(index).metaType().id()
            != QMetaType::QVariantMap) {
            return false;
        }
        const QVariantMap entry = m_sourceEntries.at(index).toMap();
        bool indexOK = false;
        const int sourceIndex = entry.value(QStringLiteral("index"))
                                    .toInt(&indexOK);
        if (!indexOK || sourceIndex != m_offset + index
            || entry.value(QStringLiteral("entryId")).toString().isEmpty()) {
            return false;
        }
    }
    return true;
}

bool F4GalleryRowsResponseReducer::normalizeRows()
{
    const QVariantMap page{
        {QStringLiteral("entries"), m_sourceEntries},
        {QStringLiteral("metadataDeferred"), m_state->metadataDeferred},
        {QStringLiteral("highlightStyles"), m_highlightStyles},
    };
    m_entries = m_bridge.normalizedEntries(page);
    return m_entries.size() == m_sourceEntries.size();
}

bool F4GalleryRowsResponseReducer::applyRowsToSession()
{
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_bridge.m_panelSessions.session(m_side));
    return session && session->applyExternalCatalogRows(
        m_entries, m_state->catalogRevision);
}

bool F4GalleryRowsResponseReducer::commitRows()
{
    for (qsizetype index = 0; index < m_entries.size(); ++index) {
        const int row = m_offset + static_cast<int>(index);
        const QVariantMap previous =
            F4GalleryBridge::catalogEntryAt(*m_state, row);
        const QString previousId = previous.value(
            QStringLiteral("entryId")).toString();
        if (!previousId.isEmpty()) {
            m_state->entryIds.remove(previousId);
            m_state->sourceIndexByEntryId.remove(previousId);
            m_state->selectedEntryIds.remove(previousId);
            m_state->selectedEntryIdList.removeAll(previousId);
        }
        const QVariantMap entry = m_entries.at(index).toMap();
        const QString entryId = entry.value(
            QStringLiteral("entryId")).toString();
        if (!F4GalleryBridge::setCatalogEntry(
                *m_state, row, m_entries.at(index))) {
            return false;
        }
        m_state->entryIds.insert(entryId);
        m_state->sourceIndexByEntryId.insert(entryId, row);
        if (entry.value(QStringLiteral("selected")).toBool()) {
            m_state->selectedEntryIds.insert(entryId);
            m_state->selectedEntryIdList.push_back(entryId);
        }
    }
    return true;
}

void F4GalleryRowsResponseReducer::finish()
{
    if (m_state->metadataDeferred) {
        m_bridge.addPanelCatalogMetadataRange(
            m_side, m_offset, m_offset + m_limit);
    }
    clearRequest();
    m_bridge.schedulePanelCatalogRowsRequest(m_side);
    m_bridge.schedulePanelCatalogMetadataRequest();
    const auto phase = m_bridge.m_navigationBenchmark.phase;
    if (m_bridge.m_navigationBenchmark.enabled
        && phase != F4GalleryBridge::NavigationBenchmarkPhase::Finished
        && phase != F4GalleryBridge::NavigationBenchmarkPhase::Failed) {
        m_bridge.scheduleNavigationBenchmarkAdvance();
    }
}

void F4GalleryRowsResponseReducer::run()
{
    if (!resolveTarget()) {
        return;
    }
    if (m_type == QStringLiteral("panel_catalog_rows_rejected")) {
        clearRequest();
        return;
    }
    if (!parseEnvelope() || !validateRows() || !normalizeRows()
        || !applyRowsToSession() || !commitRows()) {
        clearRequest();
        return;
    }
    finish();
}

void F4GalleryBridge::handlePanelCatalogRowsMessage(
    const QVariantMap &message)
{
    F4GalleryRowsResponseReducer reducer(*this, message);
    reducer.run();
}
