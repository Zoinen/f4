#include "F4GalleryBridge.h"
#include "NavigationBenchmarkTrace.h"

#include <QTimer>

#include <ZoinGallery/GallerySession.h>

#include <algorithm>
#include <utility>

namespace
{
constexpr int CatalogMetadataChunkLimit = 8;
constexpr int CatalogMetadataFrameFallbackMs = 17;
constexpr int CatalogMetadataCursorWindowChunks = 8;
constexpr int CatalogMetadataMaxFailures = 2;
constexpr int CatalogRowsPageSize = 64;
constexpr int CatalogRowsViewportOverscan = 64;

}
void F4GalleryBridge::synchronizePanelCatalog(const QVariantMap &panel)
{
    bool sideOK = false;
    const int side = panel.value(QStringLiteral("side")).toInt(&sideOK);
    if (!sideOK || !validSide(side) || panel.isEmpty()) {
        return;
    }
    synchronizePanel(side, panel);
    schedulePanelCatalogMetadataRequest();
}

void F4GalleryBridge::beginCompactProtocolMessage(
    const QVariantMap &message)
{
    const QString type = message.value(QStringLiteral("type")).toString();
    if (type != QStringLiteral("panel_catalog")
        && type != QStringLiteral("panel_activation")
        && type != QStringLiteral("scene_patch")) {
        return;
    }
    const QVariant traceId = F4NavigationBenchmarkTrace::enabled()
        ? F4NavigationBenchmarkTrace::benchmarkTraceId(message)
        : QVariant();
    const bool benchmarkRunning = m_navigationBenchmark.enabled
        && m_navigationBenchmark.phase != NavigationBenchmarkPhase::Finished
        && m_navigationBenchmark.phase != NavigationBenchmarkPhase::Failed;
    if (!benchmarkRunning && traceId.isValid()) {
        m_lastInputSceneTraceId = traceId;
    }
    if (!benchmarkRunning) {
        return;
    }

    if (traceId.isValid()) {
        m_navigationBenchmark.lastSceneTraceId = traceId;
    }
    const QVariant benchmarkValue = message.value(
        QStringLiteral("benchmark"));
    if (benchmarkValue.metaType().id() == QMetaType::QVariantMap) {
        m_navigationBenchmark.lastSceneBenchmark = benchmarkValue.toMap();
    } else {
        m_navigationBenchmark.lastSceneBenchmark.clear();
    }
    if (!m_navigationBenchmark.benchmarkTraceId.isEmpty()
        && traceId.toString() == m_navigationBenchmark.benchmarkTraceId) {
        m_navigationBenchmark.sceneMatched = true;
        restartNavigationBenchmarkWatchdog();
    }
    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("sceneBenchmark"),
                  m_navigationBenchmark.lastSceneBenchmark);
    fields.insert(QStringLiteral("sceneType"), type);
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.bridge.patch.begin"), traceId, fields);
}

void F4GalleryBridge::handleCompactProtocolMessage(const QVariantMap &message)
{
    const QString type = message.value(QStringLiteral("type")).toString();
    if (type == QStringLiteral("panel_catalog")
        || type == QStringLiteral("panel_activation")
        || type == QStringLiteral("scene_patch")) {
        // Compact patches bypass synchronizeScene(), but they still produce
        // visible QML work. Arm the same first-frame trace after the
        // controller and bridge have synchronously applied the patch so live
        // held-key measurements include the actual painted result.
        const QVariant traceId = F4NavigationBenchmarkTrace::enabled()
            ? F4NavigationBenchmarkTrace::benchmarkTraceId(message)
            : QVariant();
        if (traceId.isValid()) {
            const qint64 appliedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
            if (m_pendingInputFrameTraceId.isValid()) {
                ++m_inputScenesSupersededBeforeFrame;
                F4NavigationBenchmarkTrace::event(
                    QStringLiteral("qt.input.frame.superseded"),
                    m_pendingInputFrameTraceId, {
                        {QStringLiteral("replacedByTraceId"),
                         traceId.toString()},
                        {QStringLiteral("sceneAgeNs"),
                         appliedNs - m_pendingInputFrameSceneEndNs},
                    });
            }
            m_pendingInputFrameTraceId = traceId;
            m_pendingInputFrameSceneEndNs = appliedNs;
            m_pendingInputFrameRequiredRenderSyncSerial =
                m_renderSyncSerial.load(std::memory_order_acquire) + 1;
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.input.patch.applied"), appliedNs,
                traceId, {{QStringLiteral("messageType"), type}});
        }
        const bool benchmarkRunning = m_navigationBenchmark.enabled
            && m_navigationBenchmark.phase
                != NavigationBenchmarkPhase::Finished
            && m_navigationBenchmark.phase
                != NavigationBenchmarkPhase::Failed;
        if (benchmarkRunning) {
            QVariantMap fields = navigationBenchmarkFields();
            fields.insert(QStringLiteral("sceneBenchmark"),
                          m_navigationBenchmark.lastSceneBenchmark);
            fields.insert(QStringLiteral("sceneType"), type);
            queueNavigationBenchmarkTrace(
                QStringLiteral("qt.gallery.bridge.patch.end"),
                traceId, fields);
            scheduleNavigationBenchmarkAdvance();
        }
        return;
    }
}

int F4GalleryBridge::matchingMetadataSide(const QVariantMap &message) const
{
    bool catalogRevisionOK = false;
    bool metadataRevisionOK = false;
    bool offsetOK = false;
    const qulonglong catalogRevision = message.value(
        QStringLiteral("catalogRevision")).toULongLong(&catalogRevisionOK);
    const qulonglong metadataRevision = message.value(
        QStringLiteral("metadataRevision")).toULongLong(&metadataRevisionOK);
    const int offset = message.value(QStringLiteral("offset")).toInt(&offsetOK);
    if (!catalogRevisionOK || !metadataRevisionOK || !offsetOK
        || !message.contains(QStringLiteral("panelId"))
        || !message.contains(QStringLiteral("path"))) {
        return -1;
    }

    int match = -1;
    for (int side = 0; side < 2; ++side) {
        const SideState &state = m_panelSessions.catalog(side);
        if (!state.initialized || !state.metadataDeferred
            || state.metadataComplete || !state.metadataRequestInFlight
            || state.panelId != message.value(QStringLiteral("panelId")).toString()
            || state.currentPath != message.value(QStringLiteral("path")).toString()
            || state.catalogRevision != catalogRevision
            || state.metadataRevision != metadataRevision
            || state.metadataRequestOffset != offset) {
            continue;
        }
        if (match != -1) {
            return -1;
        }
        match = side;
    }
    return match;
}

bool F4GalleryBridge::catalogRowLoaded(const SideState &state, int row) const
{
    return state.rowLoaded(row);
}

int F4GalleryBridge::catalogEntryCount(const SideState &state)
{
    return state.entryCount();
}

QVariantMap F4GalleryBridge::catalogEntryAt(
    const SideState &state, int row)
{
    return state.entryAt(row);
}

bool F4GalleryBridge::setCatalogEntry(
    SideState &state, int row, const QVariant &entry)
{
    return state.setEntry(row, entry);
}

int F4GalleryBridge::matchingCatalogRowsSide(
    const QVariantMap &message) const
{
    bool catalogRevisionOK = false;
    bool offsetOK = false;
    const qulonglong catalogRevision = message.value(
        QStringLiteral("catalogRevision")).toULongLong(
            &catalogRevisionOK);
    const int offset = message.value(QStringLiteral("offset"))
                           .toInt(&offsetOK);
    if (!catalogRevisionOK || !offsetOK
        || !message.contains(QStringLiteral("panelId"))
        || !message.contains(QStringLiteral("path"))) {
        return -1;
    }

    int match = -1;
    for (int side = 0; side < 2; ++side) {
        const SideState &state = m_panelSessions.catalog(side);
        if (!state.initialized || !state.catalogRowsDeferred
            || !state.catalogRowsRequestInFlight
            || state.panelId
                != message.value(QStringLiteral("panelId")).toString()
            || state.currentPath
                != message.value(QStringLiteral("path")).toString()
            || state.catalogRevision != catalogRevision
            || state.catalogRowsRequestOffset != offset) {
            continue;
        }
        if (match != -1) {
            return -1;
        }
        match = side;
    }
    return match;
}

void F4GalleryBridge::requestPanelCatalogRows(int side)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_panelSessions.catalog(side);
    if (!state.initialized || !state.catalogRowsDeferred
        || state.catalogRowsRequestInFlight || state.panelId.isEmpty()
        || state.totalCount <= 0) {
        return;
    }

    int first = state.catalogRowsVisibleFirst;
    int last = state.catalogRowsVisibleLast;
    if (first < 0 || last < first) {
        first = state.cursorIndex;
        last = state.cursorIndex;
    }
    if (first < 0 || first >= state.totalCount) {
        return;
    }
    last = qBound(first, last, state.totalCount - 1);
    const int wantedFirst = qMax(0, first - CatalogRowsViewportOverscan);
    const int wantedLast = qMin(
        state.totalCount - 1, last + CatalogRowsViewportOverscan);
    int missing = -1;
    for (int row = wantedFirst; row <= wantedLast; ++row) {
        if (!catalogRowLoaded(state, row)) {
            missing = row;
            break;
        }
    }
    if (missing < 0) {
        return;
    }

    // The initial semantic page already owns rows before `missing`. Aligning
    // the request down to a page boundary retransmitted and replaced those
    // rows, including every visible delegate. Start at the first actual hole
    // so page responses contain only new viewport data.
    const int offset = missing;
    const int limit = qMin(CatalogRowsPageSize,
                           state.totalCount - offset);
    if (limit <= 0) {
        return;
    }
    state.catalogRowsRequestInFlight = true;
    state.catalogRowsRequestOffset = offset;
    state.catalogRowsRequestLimit = limit;
    emit panelCatalogRowsRequested({
        {QStringLiteral("panelId"), state.panelId},
        {QStringLiteral("path"), state.currentPath},
        {QStringLiteral("catalogRevision"),
         QVariant::fromValue<qulonglong>(state.catalogRevision)},
        {QStringLiteral("offset"), offset},
        {QStringLiteral("limit"), limit},
    });
}

void F4GalleryBridge::schedulePanelCatalogRowsRequest(int side)
{
    if (!validSide(side)
        || m_catalogRowsRequestScheduled[static_cast<size_t>(side)]) {
        return;
    }
    m_catalogRowsRequestScheduled[static_cast<size_t>(side)] = true;
    QTimer::singleShot(0, this, [this, side]() {
        m_catalogRowsRequestScheduled[static_cast<size_t>(side)] = false;
        requestPanelCatalogRows(side);
    });
}

void F4GalleryBridge::addPanelCatalogMetadataRange(
    int side, int begin, int end)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_panelSessions.catalog(side);
    const int entryCount = catalogEntryCount(state);
    begin = qBound(0, begin, entryCount);
    end = qBound(begin, end, entryCount);
    if (begin >= end) {
        return;
    }

    MetadataRange merged{begin, end};
    qsizetype index = 0;
    while (index < state.metadataPendingRanges.size()
           && state.metadataPendingRanges.at(index).end < merged.begin) {
        ++index;
    }
    while (index < state.metadataPendingRanges.size()
           && state.metadataPendingRanges.at(index).begin <= merged.end) {
        const MetadataRange current = state.metadataPendingRanges.at(index);
        merged.begin = qMin(merged.begin, current.begin);
        merged.end = qMax(merged.end, current.end);
        state.metadataPendingRanges.removeAt(index);
    }
    state.metadataPendingRanges.insert(index, merged);
    state.metadataComplete = false;
}

void F4GalleryBridge::reportMetadataVisibleRange(
    int side, int firstRow, int lastRow, qulonglong catalogRevision)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_panelSessions.catalog(side);
    if (!state.initialized || state.entries.isEmpty()
        || (catalogRevision != 0
            && catalogRevision != state.catalogRevision)
        || firstRow < 0 || lastRow < firstRow) {
        return;
    }

    const int entryCount = catalogEntryCount(state);
    const int boundedFirst = qBound(0, firstRow, entryCount - 1);
    const int boundedLast = qBound(
        boundedFirst, lastRow, entryCount - 1);
    if (state.catalogRowsDeferred
        && (state.catalogRowsVisibleFirst != boundedFirst
            || state.catalogRowsVisibleLast != boundedLast)) {
        state.catalogRowsVisibleFirst = boundedFirst;
        state.catalogRowsVisibleLast = boundedLast;
        schedulePanelCatalogRowsRequest(side);
    }
    if (!state.metadataDeferred || state.metadataComplete) {
        return;
    }
    if (state.metadataVisibleFirst == boundedFirst
        && state.metadataVisibleLast == boundedLast) {
        return;
    }
    state.metadataVisibleFirst = boundedFirst;
    state.metadataVisibleLast = boundedLast;
    state.metadataUrgentBudget = 1;
    schedulePanelCatalogMetadataRequest();
}

void F4GalleryBridge::resetPanelCatalogMetadataPlan(
    int side, bool awaitFirstFrame)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_panelSessions.catalog(side);
    ++state.metadataPacingGeneration;
    state.metadataRequestInFlight = false;
    state.metadataRequestOffset = -1;
    state.metadataRequestLimit = 0;
    state.metadataFailureCount = 0;
    state.metadataUrgentBudget = state.entries.isEmpty() ? 0 : 1;
    state.metadataPendingRanges.clear();
    state.metadataVisibleFirst = -1;
    state.metadataVisibleLast = -1;
    if (!state.entries.isEmpty()) {
        const int entryCount = catalogEntryCount(state);
        if (state.catalogRowsDeferred) {
            QList<int> loadedRows = state.entryOffsetByRow.keys();
            std::sort(loadedRows.begin(), loadedRows.end());
            for (qsizetype offset = 0; offset < loadedRows.size();) {
                const int begin = loadedRows.at(offset);
                int end = begin + 1;
                ++offset;
                while (offset < loadedRows.size()
                       && loadedRows.at(offset) == end) {
                    ++end;
                    ++offset;
                }
                state.metadataPendingRanges.push_back({begin, end});
            }
        } else {
            state.metadataPendingRanges.push_back({0, entryCount});
        }
        const int cursor = qBound(
            0, state.cursorIndex >= 0 ? state.cursorIndex : 0,
            entryCount - 1);
        const int radius = CatalogMetadataChunkLimit
            * CatalogMetadataCursorWindowChunks / 2;
        state.metadataVisibleFirst = qMax(0, cursor - radius);
        state.metadataVisibleLast = qMin(
            entryCount - 1, cursor + radius - 1);
    }
    state.metadataComplete = state.metadataPendingRanges.isEmpty();
    state.metadataAwaitingFrame = awaitFirstFrame
        && !state.metadataComplete;
    state.metadataRequiredRenderSyncSerial = state.metadataAwaitingFrame
        ? m_renderSyncSerial.load(std::memory_order_acquire) + 1
        : 0;
}

bool F4GalleryBridge::choosePanelCatalogMetadataRange(
    int side, int *offset, int *limit, bool *urgent) const
{
    if (!validSide(side) || !offset || !limit || !urgent) {
        return false;
    }
    const SideState &state = m_panelSessions.catalog(side);
    if (state.metadataPendingRanges.isEmpty()) {
        return false;
    }

    const auto chooseAt = [&](int target, bool center) {
        for (const MetadataRange &range : state.metadataPendingRanges) {
            if (target < range.begin || target >= range.end) {
                continue;
            }
            const int start = center
                ? qMax(range.begin, target - CatalogMetadataChunkLimit / 2)
                : target;
            *offset = start;
            *limit = qMin(CatalogMetadataChunkLimit, range.end - start);
            return *limit > 0;
        }
        return false;
    };

    // The cursor row is the first metadata needed for a restored viewport,
    // Details row, or viewer intent. Then drain only the currently reported
    // viewport window. Rows outside that window remain sparse until scrolling
    // makes them visible; crawling the complete catalog would turn a 30K-row
    // directory into thousands of needless IPC/model mutations.
    *urgent = false;
    if (state.metadataUrgentBudget > 0 && state.cursorIndex >= 0
        && chooseAt(state.cursorIndex, true)) {
        *urgent = true;
        return true;
    }
    if (state.metadataUrgentBudget > 0
        && state.metadataVisibleFirst >= 0
        && state.metadataVisibleLast >= state.metadataVisibleFirst) {
        for (const MetadataRange &range : state.metadataPendingRanges) {
            const int start = qMax(range.begin, state.metadataVisibleFirst);
            const int end = qMin(range.end, state.metadataVisibleLast + 1);
            if (start >= end) {
                continue;
            }
            *offset = start;
            *limit = qMin(CatalogMetadataChunkLimit, end - start);
            *urgent = true;
            return true;
        }
    }

    // Once the input-time urgent budget is spent, finish the same visible
    // window when idle.
    if (state.metadataVisibleFirst >= 0
        && state.metadataVisibleLast >= state.metadataVisibleFirst) {
        for (const MetadataRange &range : state.metadataPendingRanges) {
            const int start = qMax(range.begin, state.metadataVisibleFirst);
            const int end = qMin(range.end, state.metadataVisibleLast + 1);
            if (start >= end) {
                continue;
            }
            *offset = start;
            *limit = qMin(CatalogMetadataChunkLimit, end - start);
            return true;
        }
    }

    return false;
}

bool F4GalleryBridge::consumePanelCatalogMetadataRange(
    int side, int offset, int end)
{
    if (!validSide(side) || offset < 0 || end <= offset) {
        return false;
    }
    SideState &state = m_panelSessions.catalog(side);
    for (qsizetype index = 0;
         index < state.metadataPendingRanges.size(); ++index) {
        const MetadataRange range = state.metadataPendingRanges.at(index);
        if (offset < range.begin || end > range.end) {
            continue;
        }
        state.metadataPendingRanges.removeAt(index);
        if (range.begin < offset) {
            state.metadataPendingRanges.insert(
                index++, {range.begin, offset});
        }
        if (end < range.end) {
            state.metadataPendingRanges.insert(index, {end, range.end});
        }
        return true;
    }
    return false;
}

void F4GalleryBridge::failPanelCatalogMetadataRequest(
    int side, bool retry)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_panelSessions.catalog(side);
    state.metadataRequestInFlight = false;
    state.metadataRequestOffset = -1;
    state.metadataRequestLimit = 0;
    state.metadataAwaitingFrame = false;
    state.metadataRequiredRenderSyncSerial = 0;
    if (retry && ++state.metadataFailureCount
        < CatalogMetadataMaxFailures) {
        schedulePanelCatalogMetadataRequest();
        return;
    }
    state.metadataComplete = true;
    state.metadataPendingRanges.clear();
    schedulePanelCatalogMetadataRequest();
}

void F4GalleryBridge::noteMetadataInputActivity()
{
    m_metadataInputBusy = true;
    if (m_metadataIdleTimer) {
        m_metadataIdleTimer->start();
    }
}

void F4GalleryBridge::prioritizePanelCatalogMetadataRow(int side, int row)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_panelSessions.catalog(side);
    const int entryCount = catalogEntryCount(state);
    if (!state.initialized || row < 0 || row >= entryCount) {
        return;
    }
    if (state.catalogRowsDeferred && !catalogRowLoaded(state, row)) {
        state.catalogRowsVisibleFirst = row;
        state.catalogRowsVisibleLast = row;
        schedulePanelCatalogRowsRequest(side);
        return;
    }
    if (!state.metadataDeferred || state.metadataComplete) {
        return;
    }
    bool rowPending = false;
    for (const MetadataRange &range : state.metadataPendingRanges) {
        if (row >= range.begin && row < range.end) {
            rowPending = true;
            break;
        }
    }
    if (!rowPending) {
        return;
    }
    const int radius = CatalogMetadataChunkLimit
        * CatalogMetadataCursorWindowChunks / 2;
    state.metadataVisibleFirst = qMax(0, row - radius);
    state.metadataVisibleLast = qMin(entryCount - 1, row + radius - 1);
    state.metadataUrgentBudget = 1;
    schedulePanelCatalogMetadataRequest();
}

bool F4GalleryBridge::requestPanelCatalogMetadata(int side)
{
    if (!validSide(side)) {
        return false;
    }
    SideState &state = m_panelSessions.catalog(side);
    if (!state.initialized || !state.metadataDeferred
        || state.metadataComplete || state.metadataRequestInFlight
        || state.metadataAwaitingFrame || state.loading
        || state.panelId.isEmpty()) {
        return false;
    }
    int offset = -1;
    int limit = 0;
    bool urgent = false;
    if (!choosePanelCatalogMetadataRange(
            side, &offset, &limit, &urgent)) {
        return false;
    }
    if (m_metadataInputBusy && !urgent) {
        return false;
    }
    if (urgent && state.metadataUrgentBudget > 0) {
        --state.metadataUrgentBudget;
    }
    state.metadataRequestInFlight = true;
    state.metadataRequestOffset = offset;
    state.metadataRequestLimit = limit;
    emit panelCatalogMetadataRequested({
        {QStringLiteral("panelId"), state.panelId},
        {QStringLiteral("path"), state.currentPath},
        {QStringLiteral("catalogRevision"),
         QVariant::fromValue<qulonglong>(state.catalogRevision)},
        {QStringLiteral("metadataRevision"),
         QVariant::fromValue<qulonglong>(state.metadataRevision)},
        {QStringLiteral("offset"), offset},
        {QStringLiteral("limit"), limit},
    });
    return true;
}

void F4GalleryBridge::requestNextPanelCatalogMetadata()
{
    // Keep only one catalog metadata transaction in flight globally. The
    // active panel is always drained first; the inactive side starts only
    // once the active stream is complete (or itself becomes active).
    for (const SideState &state : m_panelSessions.catalogs()) {
        if (state.metadataRequestInFlight || state.metadataAwaitingFrame) {
            return;
        }
    }
    for (int priority = 0; priority < 2; ++priority) {
        for (int side = 0; side < 2; ++side) {
            const SideState &state = m_panelSessions.catalog(side);
            if ((priority == 0) != state.active
                || !state.initialized || !state.metadataDeferred
                || state.metadataComplete || state.metadataAwaitingFrame
                || state.loading) {
                continue;
            }
            if (requestPanelCatalogMetadata(side)) {
                return;
            }
        }
    }
}

void F4GalleryBridge::schedulePanelCatalogMetadataRequest()
{
    if (m_metadataRequestScheduled) {
        return;
    }
    m_metadataRequestScheduled = true;
    QTimer::singleShot(0, this, [this]() {
        m_metadataRequestScheduled = false;
        requestNextPanelCatalogMetadata();
    });
}
