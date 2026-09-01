#include "F4GalleryBridge.h"
#include "NavigationBenchmarkTrace.h"

#include <QTimer>

#include <ZoinGallery/GallerySession.h>

namespace
{
constexpr int CatalogMetadataChunkLimit = 8;
constexpr int CatalogMetadataFrameFallbackMs = 17;
}

class F4GalleryMetadataResponseReducer
{
public:
    F4GalleryMetadataResponseReducer(
        F4GalleryBridge &bridge, const QVariantMap &message)
        : m_bridge(bridge), m_message(message)
    {
    }

    void run();

private:
    bool resolveTarget();
    bool parseEnvelope();
    bool validateRows() const;
    bool applyToSession();
    bool consumeRange();
    void traceApplied() const;
    void commitResponse();
    void armFramePacing();
    void fail(bool retry);

    F4GalleryBridge &m_bridge;
    const QVariantMap &m_message;
    QString m_type;
    int m_side = -1;
    F4GalleryBridge::SideState *m_state = nullptr;
    int m_limit = 0;
    int m_total = 0;
    qulonglong m_highlightRevision = 0;
    QVariantList m_sourceEntries;
    QVariantMap m_highlightStyles;
    bool m_final = false;
    int m_requestOffset = -1;
    int m_endOffset = -1;
    bool m_streamFinal = false;
    QVariantList m_entries;
    bool m_traceStages = false;
    QVariant m_traceId;
    qint64 m_normalizeStartedNs = 0;
    qint64 m_normalizeCompletedNs = 0;
    qint64 m_modelApplyCompletedNs = 0;
};

bool F4GalleryMetadataResponseReducer::resolveTarget()
{
    m_type = m_message.value(QStringLiteral("type")).toString();
    if (m_type != QStringLiteral("panel_catalog_metadata")
        && m_type != QStringLiteral("panel_catalog_metadata_rejected")) {
        return false;
    }
    m_side = m_bridge.matchingMetadataSide(m_message);
    if (!F4GalleryBridge::validSide(m_side)) {
        return false;
    }
    m_state = &m_bridge.m_panelSessions.catalog(m_side);
    return true;
}

void F4GalleryMetadataResponseReducer::fail(bool retry)
{
    m_bridge.failPanelCatalogMetadataRequest(m_side, retry);
}

bool F4GalleryMetadataResponseReducer::parseEnvelope()
{
    bool limitOK = false;
    bool totalOK = false;
    bool highlightRevisionOK = false;
    m_limit = m_message.value(QStringLiteral("limit")).toInt(&limitOK);
    m_total = m_message.value(QStringLiteral("total")).toInt(&totalOK);
    m_highlightRevision = m_message.value(
        QStringLiteral("highlightRevision")).toULongLong(
            &highlightRevisionOK);
    const QVariant entriesValue = m_message.value(
        QStringLiteral("entries"));
    const QVariant stylesValue = m_message.value(
        QStringLiteral("highlightStyles"));
    if (!limitOK || m_limit != m_state->metadataRequestLimit
        || m_limit <= 0 || m_limit > CatalogMetadataChunkLimit
        || !totalOK || m_total < m_state->metadataRequestOffset
        || !highlightRevisionOK
        || entriesValue.metaType().id() != QMetaType::QVariantList
        || (m_message.contains(QStringLiteral("highlightStyles"))
            && stylesValue.metaType().id() != QMetaType::QVariantMap)
        || !m_message.contains(QStringLiteral("totalSize"))
        || !m_message.contains(QStringLiteral("final"))) {
        return false;
    }
    m_sourceEntries = entriesValue.toList();
    m_highlightStyles = stylesValue.toMap();
    m_final = m_message.value(QStringLiteral("final")).toBool();
    m_requestOffset = m_state->metadataRequestOffset;
    m_endOffset = m_requestOffset + m_sourceEntries.size();
    return m_total == F4GalleryBridge::catalogEntryCount(*m_state)
        && m_sourceEntries.size() == m_limit
        && m_endOffset <= m_total
        && m_endOffset <= F4GalleryBridge::catalogEntryCount(*m_state)
        && m_final == (m_endOffset == m_total);
}

bool F4GalleryMetadataResponseReducer::validateRows() const
{
    for (qsizetype index = 0; index < m_sourceEntries.size(); ++index) {
        if (m_sourceEntries.at(index).metaType().id()
            != QMetaType::QVariantMap) {
            return false;
        }
        const QVariantMap metadata = m_sourceEntries.at(index).toMap();
        const QVariantMap base = F4GalleryBridge::catalogEntryAt(
            *m_state, m_requestOffset + static_cast<int>(index));
        bool metadataIndexOK = false;
        const int metadataIndex = metadata.value(QStringLiteral("index"))
                                      .toInt(&metadataIndexOK);
        if (metadata.value(QStringLiteral("entryId")).toString().isEmpty()
            || metadata.value(QStringLiteral("entryId"))
                != base.value(QStringLiteral("entryId"))
            || !metadataIndexOK
            || metadataIndex
                != base.value(QStringLiteral("index")).toInt()) {
            return false;
        }
    }
    return true;
}

bool F4GalleryMetadataResponseReducer::applyToSession()
{
    m_streamFinal = !m_state->catalogRowsDeferred
        && m_state->metadataPendingRanges.size() == 1
        && m_state->metadataPendingRanges.constFirst().begin
            == m_requestOffset
        && m_state->metadataPendingRanges.constFirst().end == m_endOffset;
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_bridge.m_panelSessions.session(m_side));
    m_traceStages = F4NavigationBenchmarkTrace::enabled();
    m_traceId = m_traceStages
        ? F4NavigationBenchmarkTrace::benchmarkTraceId(m_message)
        : QVariant();
    if (!m_traceId.isValid()) {
        m_traceId = m_bridge.m_lastInputSceneTraceId;
    }
    m_normalizeStartedNs = m_traceStages
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    m_entries = m_bridge.normalizedMetadataEntries(
        m_side, m_requestOffset, m_sourceEntries, m_highlightStyles);
    m_normalizeCompletedNs = m_traceStages
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    if (!session || !session->applyExternalMetadata(
            m_entries, m_state->catalogRevision,
            m_state->metadataRevision, m_streamFinal)) {
        return false;
    }
    m_modelApplyCompletedNs = m_traceStages
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    traceApplied();
    return true;
}

void F4GalleryMetadataResponseReducer::traceApplied() const
{
    if (!m_traceStages) {
        return;
    }
    const QVariantMap fields{
        {QStringLiteral("side"), m_side},
        {QStringLiteral("offset"), m_requestOffset},
        {QStringLiteral("rows"), m_entries.size()},
        {QStringLiteral("serverFinal"), m_final},
        {QStringLiteral("streamFinal"), m_streamFinal},
        {QStringLiteral("normalizeDurationNs"),
         m_normalizeCompletedNs - m_normalizeStartedNs},
        {QStringLiteral("modelApplyDurationNs"),
         m_modelApplyCompletedNs - m_normalizeCompletedNs},
        {QStringLiteral("durationNs"),
         m_modelApplyCompletedNs - m_normalizeStartedNs},
    };
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.gallery.bridge.metadata.applied"),
        m_modelApplyCompletedNs, m_traceId, fields);
}

bool F4GalleryMetadataResponseReducer::consumeRange()
{
    return m_bridge.consumePanelCatalogMetadataRange(
        m_side, m_requestOffset, m_endOffset);
}

void F4GalleryMetadataResponseReducer::armFramePacing()
{
    m_state->metadataAwaitingFrame = true;
    m_state->metadataRequiredRenderSyncSerial =
        m_bridge.m_renderSyncSerial.load(std::memory_order_acquire) + 1;
    const qulonglong generation = ++m_state->metadataPacingGeneration;
    const QString panelId = m_state->panelId;
    const QString path = m_state->currentPath;
    const qulonglong catalogRevision = m_state->catalogRevision;
    const qulonglong metadataRevision = m_state->metadataRevision;
    QTimer::singleShot(
        CatalogMetadataFrameFallbackMs, &m_bridge,
        [thisBridge = &m_bridge, side = m_side, panelId, path,
         catalogRevision, metadataRevision, generation]() {
            if (!F4GalleryBridge::validSide(side)) {
                return;
            }
            F4GalleryBridge::SideState &paced =
                thisBridge->m_panelSessions.catalog(side);
            if (!paced.metadataAwaitingFrame
                || paced.metadataRequestInFlight
                || paced.panelId != panelId
                || paced.currentPath != path
                || paced.catalogRevision != catalogRevision
                || paced.metadataRevision != metadataRevision
                || paced.metadataPacingGeneration != generation) {
                return;
            }
            paced.metadataAwaitingFrame = false;
            paced.metadataRequiredRenderSyncSerial = 0;
            thisBridge->schedulePanelCatalogMetadataRequest();
        });
}

void F4GalleryMetadataResponseReducer::commitResponse()
{
    m_state->metadataRequestInFlight = false;
    m_state->metadataRequestOffset = -1;
    m_state->metadataRequestLimit = 0;
    m_state->metadataFailureCount = 0;
    m_state->highlightRevision = m_highlightRevision;
    armFramePacing();
    if (m_streamFinal) {
        m_state->metadataComplete = true;
    }
    m_bridge.schedulePanelCatalogMetadataRequest();
}

void F4GalleryMetadataResponseReducer::run()
{
    if (!resolveTarget()) {
        return;
    }
    if (m_type == QStringLiteral("panel_catalog_metadata_rejected")) {
        fail(false);
        return;
    }
    if (!parseEnvelope() || !validateRows()) {
        fail(true);
        return;
    }
    if (!applyToSession() || !consumeRange()) {
        fail(false);
        return;
    }
    commitResponse();
}

void F4GalleryBridge::handlePanelCatalogMetadataMessage(
    const QVariantMap &message)
{
    F4GalleryMetadataResponseReducer reducer(*this, message);
    reducer.run();
}
