#include "F4GalleryBridge.h"

#include "F4IconProvider.h"
#include "NavigationBenchmarkTrace.h"

#include <ZoinGallery/GallerySession.h>
#include <ZoinGallery/MediaTimingTrace.h>

#include <algorithm>
#include <utility>

namespace
{
constexpr int MaximumDeferredCatalogPageRows = 256;

bool usefulLocalCatalogPreview(const QString &sourceKind,
                               bool previewCapable,
                               const QVariantList &entries)
{
    if (sourceKind != QStringLiteral("local") || !previewCapable) {
        return false;
    }
    return std::any_of(entries.cbegin(), entries.cend(),
                       [](const QVariant &value) {
        const QString name = value.toMap().value(
            QStringLiteral("name")).toString();
        return !name.isEmpty() && name != QStringLiteral("..");
    });
}
}

struct F4GalleryBridge::PanelSyncContext
{
    int side = -1;
    size_t sideIndex = 0;
    QVariantMap panel;
    SideState *state = nullptr;
    ZoinGallery::GallerySession *session = nullptr;
    QString panelId;
    QString currentPath;
    QString cursorEntryId;
    QString sourceKind;
    QString galleryLayoutMode;
    qulonglong catalogRevision = 0;
    qulonglong selectionRevision = 0;
    qulonglong highlightRevision = 0;
    qulonglong metadataRevision = 0;
    qulonglong iconRevision = 0;
    int cursorIndex = -1;
    int incomingTotalCount = 0;
    bool metadataDeferred = false;
    bool previewCapable = false;
    bool active = false;
    bool loading = false;
    bool catalogProvisional = false;
    bool catalogRowsDeferred = false;
    bool usefulLocalPreview = false;
    bool catalogStreamStart = false;
    bool identityChanged = false;
    bool provisionalReplacementDeferred = false;
    bool catalogPayloadChanged = false;
    bool catalogChanged = false;
    bool metadataStreamChanged = false;
    bool selectionChanged = false;
    bool highlightChanged = false;
    bool iconChanged = false;
    bool metadataRestartNeeded = false;
    bool appearanceChanged = false;
    bool normalizeCatalog = false;
    bool catalogApplied = true;
    bool traceCatalogStages = false;
    QVariant catalogTraceId;
    QVariantList incomingEntries;
    QVariantList entries;
    QStringList selectedIds;
    QString appliedCursorEntryId;
    int appliedCursorIndex = -1;
    DeferredPanelOpenRepeat repeatToReplay;
    qint64 normalizeStartedNs = 0;
    qint64 normalizeCompletedNs = 0;
    qint64 catalogApplyStartedNs = 0;
    qint64 catalogApplyCompletedNs = 0;
    qint64 stateApplyStartedNs = 0;
    qint64 stateApplyCompletedNs = 0;
};

F4GalleryBridge::PanelSyncContext F4GalleryBridge::makePanelSyncContext(
    int side, const QVariantMap &panel)
{
    PanelSyncContext context;
    context.side = side;
    context.sideIndex = static_cast<size_t>(side);
    context.panel = panel;
    context.state = &m_panelSessions.catalog(side);
    context.panelId = panel.value(QStringLiteral("id")).toString();
    context.catalogRevision = revisionValue(
        panel, QStringLiteral("catalogRevision"));
    context.selectionRevision = revisionValue(
        panel, QStringLiteral("selectionRevision"));
    context.highlightRevision = revisionValue(
        panel, QStringLiteral("highlightRevision"));
    context.metadataDeferred = panel.value(
        QStringLiteral("metadataDeferred")).toBool();
    context.metadataRevision = revisionValue(
        panel, QStringLiteral("metadataRevision"));
    context.iconRevision = m_iconSet ? m_iconSet->revision() : 0;
    context.currentPath = panel.value(QStringLiteral("path")).toString();
    context.cursorEntryId = panel.value(
        QStringLiteral("cursorEntryId")).toString();
    context.cursorIndex = panel.value(QStringLiteral("cursor"), -1).toInt();
    context.sourceKind = panel.value(
        QStringLiteral("sourceKind"), QStringLiteral("vfs")).toString();
    context.previewCapable = panel.value(
        QStringLiteral("previewCapable")).toBool();
    context.active = panel.value(QStringLiteral("active")).toBool();
    context.loading = panel.value(QStringLiteral("loading")).toBool();
    context.catalogProvisional = panel.value(
        QStringLiteral("catalogProvisional")).toBool();
    context.catalogRowsDeferred = panel.value(
        QStringLiteral("catalogRowsDeferred")).toBool();
    context.galleryLayoutMode = panel.value(
        QStringLiteral("galleryLayoutMode")).toString();
    context.incomingEntries = panel.value(
        QStringLiteral("entries")).toList();
    context.incomingTotalCount = panel.value(
        QStringLiteral("totalCount"), context.incomingEntries.size()).toInt();

    SideState &state = *context.state;
    if (state.initialized && context.panelId == state.panelId
        && context.currentPath == state.currentPath
        && context.catalogRevision != 0) {
        state.latestSemanticCatalogRevision = qMax(
            state.latestSemanticCatalogRevision, context.catalogRevision);
    }
    context.usefulLocalPreview = context.catalogProvisional
        && usefulLocalCatalogPreview(context.sourceKind,
                                     context.previewCapable,
                                     context.incomingEntries);
    context.catalogStreamStart = !context.catalogRowsDeferred
        && context.metadataDeferred && context.catalogProvisional
        && context.incomingTotalCount > context.incomingEntries.size();
    context.identityChanged = state.initialized
        && context.panelId != state.panelId;
    context.provisionalReplacementDeferred = context.catalogProvisional
        && !context.catalogStreamStart && !context.usefulLocalPreview
        && state.initialized && context.panelId == state.panelId
        && context.currentPath != state.currentPath;
    return context;
}

bool F4GalleryBridge::deferSupersededPanelCatalog(
    PanelSyncContext *context)
{
    SideState &state = *context->state;
    const bool superseded = m_inFlightPanelOpen.active
        && m_inFlightPanelOpen.expectsPathChange
        && m_inFlightPanelOpen.side == context->side
        && m_inFlightPanelOpen.panelId == context->panelId
        && m_inFlightPanelOpen.sourcePath == context->currentPath
        && state.initialized && state.panelId == context->panelId
        && state.currentPath == context->currentPath
        && !context->catalogProvisional
        && (state.catalogProvisional
            || context->catalogRevision != state.catalogRevision
            || context->catalogRowsDeferred != state.catalogRowsDeferred
            || context->incomingTotalCount != state.totalCount);
    if (!superseded
        || context->incomingEntries.size() > MaximumDeferredCatalogPageRows) {
        return false;
    }
    m_inFlightPanelOpen.deferredSourcePanel = context->panel;
    m_deferredCatalogFinalizations[context->sideIndex] = {};
    F4NavigationBenchmarkTrace::event(
        QStringLiteral("qt.gallery.bridge.catalog.superseded-by-open"),
        m_lastInputSceneTraceId, {
            {QStringLiteral("side"), context->side},
            {QStringLiteral("path"), context->currentPath},
            {QStringLiteral("entries"), context->incomingEntries.size()},
            {QStringLiteral("totalCount"), context->incomingTotalCount},
            {QStringLiteral("catalogRevision"),
             QVariant::fromValue<qulonglong>(context->catalogRevision)},
        });
    return true;
}

bool F4GalleryBridge::deferPanelCatalogFinalization(
    PanelSyncContext *context)
{
    SideState &state = *context->state;
    DeferredCatalogFinalization &deferred =
        m_deferredCatalogFinalizations[context->sideIndex];
    if (deferred.active
        && (context->panelId != state.panelId
            || context->currentPath != state.currentPath
            || context->catalogProvisional)) {
        deferred = {};
    }
    const bool canWait = state.initialized && state.catalogProvisional
        && !context->catalogProvisional
        && state.provisionalFrameRequiredRenderSyncSerial != 0
        && state.catalogRowsDeferred && context->catalogRowsDeferred
        && context->panelId == state.panelId
        && context->currentPath == state.currentPath
        && context->sourceKind == state.sourceKind
        && context->previewCapable == state.previewCapable
        && context->incomingEntries.size() <= MaximumDeferredCatalogPageRows
        && context->incomingTotalCount >= state.totalCount;
    if (canWait) {
        const QVariantList entries = normalizedEntries(context->panel);
        const bool prefixUnchanged = entries.size() >= state.entries.size()
            && std::equal(state.entries.cbegin(), state.entries.cend(),
                          entries.cbegin());
        if (prefixUnchanged) {
            deferred.active = true;
            deferred.scheduled = false;
            deferred.requiredRenderSyncSerial =
                state.provisionalFrameRequiredRenderSyncSerial;
            deferred.panel = context->panel;
            if (m_navigationBenchmark.enabled) {
                QVariantMap fields = navigationBenchmarkFields();
                fields.insert(QStringLiteral("syncSide"), context->side);
                fields.insert(QStringLiteral("syncPath"), context->currentPath);
                fields.insert(QStringLiteral("syncCatalogRevision"),
                              QVariant::fromValue<qulonglong>(
                                  context->catalogRevision));
                fields.insert(QStringLiteral("syncEntryCount"),
                              context->incomingEntries.size());
                queueNavigationBenchmarkTrace(
                    QStringLiteral(
                        "qt.gallery.bridge.catalog.finalization-deferred"),
                    m_navigationBenchmark.lastSceneTraceId, fields);
            }
            return true;
        }
    }
    if (deferred.active) {
        deferred = {};
    }
    return false;
}

void F4GalleryBridge::acknowledgePanelOpen(PanelSyncContext *context)
{
    if (!m_inFlightPanelOpen.active
        || m_inFlightPanelOpen.side != context->side
        || (m_inFlightPanelOpen.panelId == context->panelId
            && (context->provisionalReplacementDeferred
                || m_inFlightPanelOpen.sourcePath == context->currentPath))) {
        return;
    }
    const bool pathAcknowledged =
        m_inFlightPanelOpen.panelId == context->panelId
        && !context->provisionalReplacementDeferred
        && m_inFlightPanelOpen.sourcePath != context->currentPath;
    if (pathAcknowledged && m_deferredPanelOpenRepeat.active
        && m_deferredPanelOpenRepeat.side == context->side
        && m_deferredPanelOpenRepeat.panelId == m_inFlightPanelOpen.panelId
        && m_deferredPanelOpenRepeat.sourcePath
            == m_inFlightPanelOpen.sourcePath
        && m_deferredPanelOpenRepeat.catalogRevision
            == m_inFlightPanelOpen.catalogRevision) {
        context->repeatToReplay = m_deferredPanelOpenRepeat;
    }
    clearInFlightPanelOpen();
}

void F4GalleryBridge::tracePanelSyncBegin(
    const PanelSyncContext &context)
{
    if (!m_navigationBenchmark.enabled
        || m_navigationBenchmark.phase == NavigationBenchmarkPhase::Finished
        || m_navigationBenchmark.phase == NavigationBenchmarkPhase::Failed) {
        return;
    }
    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("syncSide"), context.side);
    fields.insert(QStringLiteral("syncPath"), context.currentPath);
    fields.insert(QStringLiteral("syncLoading"), context.loading);
    fields.insert(QStringLiteral("syncLayoutMode"),
                  context.galleryLayoutMode);
    fields.insert(QStringLiteral("syncCatalogRevision"),
                  QVariant::fromValue<qulonglong>(context.catalogRevision));
    fields.insert(QStringLiteral("syncEntryCount"),
                  context.incomingEntries.size());
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.bridge.panel.begin"),
        m_navigationBenchmark.lastSceneTraceId, fields);
}

void F4GalleryBridge::handlePanelIdentityChange(PanelSyncContext *context)
{
    if (!context->identityChanged) {
        return;
    }
    if (viewerVisible() && viewerSide() == context->side) {
        closeViewer();
    }
    if (m_pendingPanelOpen.active
        && m_pendingPanelOpen.side == context->side) {
        clearPendingPanelOpen();
    }
    clearPendingCursor(context->side);
    clearPendingSelection(context->side);
    m_selectionActionPending[context->sideIndex] = false;
    activatePanelSession(context->side, context->panelId);
}

bool F4GalleryBridge::applyProvisionalPanelUpdate(
    PanelSyncContext *context)
{
    if (!context->provisionalReplacementDeferred) {
        return false;
    }
    SideState &state = *context->state;
    state.active = context->active;
    state.loading = context->loading;
    state.galleryLayoutMode = context->galleryLayoutMode;
    if (m_navigationBenchmark.enabled
        && m_navigationBenchmark.phase != NavigationBenchmarkPhase::Finished
        && m_navigationBenchmark.phase != NavigationBenchmarkPhase::Failed) {
        QVariantMap fields = navigationBenchmarkFields();
        fields.insert(QStringLiteral("syncSide"), context->side);
        fields.insert(QStringLiteral("syncPath"), state.currentPath);
        fields.insert(QStringLiteral("syncLoading"), state.loading);
        fields.insert(QStringLiteral("syncLayoutMode"),
                      state.galleryLayoutMode);
        fields.insert(QStringLiteral("syncCatalogRevision"),
                      QVariant::fromValue<qulonglong>(
                          state.catalogRevision));
        fields.insert(QStringLiteral("syncEntryCount"), state.entries.size());
        fields.insert(QStringLiteral("provisionalReplacementDeferred"), true);
        queueNavigationBenchmarkTrace(
            QStringLiteral("qt.gallery.bridge.panel.end"),
            m_navigationBenchmark.lastSceneTraceId, fields);
    }
    return true;
}

void F4GalleryBridge::classifyPanelSyncChanges(PanelSyncContext *context)
{
    const SideState &state = *context->state;
    context->catalogPayloadChanged = !state.initialized
        || context->catalogRevision != state.catalogRevision
        || context->currentPath != state.currentPath
        || context->sourceKind != state.sourceKind
        || context->previewCapable != state.previewCapable
        || context->catalogRowsDeferred != state.catalogRowsDeferred
        || context->incomingTotalCount != state.totalCount;
    context->catalogChanged = context->catalogPayloadChanged
        || context->catalogProvisional != state.catalogProvisional;
    context->metadataStreamChanged = !state.initialized
        || context->metadataDeferred != state.metadataDeferred
        || context->catalogRevision != state.catalogRevision
        || context->currentPath != state.currentPath
        || (context->metadataDeferred
            && context->metadataRevision != state.metadataRevision);
    context->selectionChanged = !state.initialized
        || context->selectionRevision != state.selectionRevision;
    context->highlightChanged = !context->metadataDeferred
        && (!state.initialized
            || context->highlightRevision != state.highlightRevision);
    context->iconChanged = !state.initialized
        || context->iconRevision != state.iconRevision;
    context->metadataRestartNeeded = !context->catalogStreamStart
        && context->metadataDeferred
        && (context->metadataStreamChanged
            || (context->iconChanged && !state.metadataComplete));
    if (context->metadataDeferred && context->iconChanged
        && state.initialized && state.metadataComplete
        && !context->metadataStreamChanged) {
        refreshDeferredIconAppearance(context->side);
    }
    context->appearanceChanged = !context->metadataDeferred
        && (context->highlightChanged || context->iconChanged);
    context->normalizeCatalog = context->catalogPayloadChanged
        || context->appearanceChanged;
    context->traceCatalogStages = F4NavigationBenchmarkTrace::enabled();
    context->catalogTraceId = m_lastInputSceneTraceId;
}

void F4GalleryBridge::preparePanelCatalogData(PanelSyncContext *context)
{
    if (context->traceCatalogStages && context->normalizeCatalog) {
        context->normalizeStartedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
    }
    if (context->normalizeCatalog) {
        ZoinGallery::MediaTimingTrace::Span span(
            QStringLiteral("qt.gallery.bridge.catalog_normalize"), {
                {QStringLiteral("side"), context->side},
                {QStringLiteral("path"), context->currentPath},
                {QStringLiteral("inputEntries"),
                 context->incomingEntries.size()},
            });
        context->entries = normalizedEntries(context->panel);
        span.set(QStringLiteral("outputEntries"), context->entries.size());
    } else {
        context->entries = context->state->entries;
    }
    if (context->traceCatalogStages && context->normalizeCatalog) {
        context->normalizeCompletedNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
    }
    if (context->catalogPayloadChanged || context->selectionChanged) {
        context->selectedIds = selectedEntryIds(
            context->catalogPayloadChanged ? context->entries
                                           : context->incomingEntries);
    } else {
        context->selectedIds = context->state->selectedEntryIdList;
    }

    context->appliedCursorEntryId = context->cursorEntryId;
    context->appliedCursorIndex = context->cursorIndex;
    const PendingCursor &pending = m_pendingCursors[context->sideIndex];
    const int pendingSourceIndex = context->catalogChanged
        ? sourceIndexForEntryId(context->entries, pending.entryId)
        : context->state->sourceIndexByEntryId.value(pending.entryId, -1);
    if (pending.active && pending.panelId == context->panelId
        && (pending.catalogRevision == context->catalogRevision
            || pending.maskAcrossCatalog)
        && pending.entryId != context->cursorEntryId
        && pendingSourceIndex >= 0) {
        context->appliedCursorEntryId = pending.entryId;
        context->appliedCursorIndex = pendingSourceIndex;
    }
}

void F4GalleryBridge::applyPanelSessionData(PanelSyncContext *context)
{
    if (context->session
        && (context->catalogChanged || context->metadataRestartNeeded)) {
        if (context->traceCatalogStages) {
            context->catalogApplyStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
        ZoinGallery::MediaTimingTrace::Span span(
            QStringLiteral("qt.gallery.bridge.catalog_apply"), {
                {QStringLiteral("side"), context->side},
                {QStringLiteral("path"), context->currentPath},
                {QStringLiteral("entries"), context->entries.size()},
                {QStringLiteral("catalogRevision"),
                 QVariant::fromValue<qulonglong>(context->catalogRevision)},
                {QStringLiteral("catalogProvisional"),
                 context->catalogProvisional},
            });
        context->catalogApplied = context->session->applyExternalCatalog(
            context->entries, context->catalogRevision, {
                {QStringLiteral("currentPath"), context->currentPath},
                {QStringLiteral("sourceKind"), context->sourceKind},
                {QStringLiteral("previewCapable"), context->previewCapable},
                {QStringLiteral("sourceIdentityChanged"),
                 context->identityChanged},
                {QStringLiteral("catalogProvisional"),
                 context->catalogProvisional},
                {QStringLiteral("metadataDeferred"),
                 context->metadataDeferred},
                {QStringLiteral("metadataRevision"),
                 QVariant::fromValue<qulonglong>(context->metadataRevision)},
                {QStringLiteral("catalogRowsDeferred"),
                 context->catalogRowsDeferred},
                {QStringLiteral("totalCount"), context->incomingTotalCount},
                {QStringLiteral("catalogStreaming"),
                 context->catalogStreamStart},
                {QStringLiteral("cursorEntryId"),
                 context->appliedCursorEntryId},
                {QStringLiteral("cursorIndex"),
                 context->appliedCursorIndex},
                {QStringLiteral("deferCatalogReady"),
                 context->catalogPayloadChanged
                     && !context->catalogProvisional},
            });
        if (context->traceCatalogStages) {
            context->catalogApplyCompletedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
        span.set(QStringLiteral("applied"), context->catalogApplied);
    }
    if (context->session && !context->catalogChanged
        && context->appearanceChanged) {
        context->session->applyExternalAppearance(
            context->entries, context->highlightRevision);
    }
    const bool stateChanged = context->catalogChanged
        || context->selectionChanged
        || context->cursorEntryId != context->state->cursorEntryId
        || context->cursorIndex != context->state->cursorIndex
        || m_stateReconciliationPending[context->sideIndex];
    if (context->session && stateChanged) {
        if (context->traceCatalogStages) {
            context->stateApplyStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
        context->session->applyExternalState(
            context->appliedCursorEntryId, context->appliedCursorIndex,
            context->selectedIds, context->selectionRevision);
        if (context->traceCatalogStages) {
            context->stateApplyCompletedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
    }
    if (context->session && context->catalogChanged
        && context->catalogApplied) {
        context->session->setExternalCatalogReady(
            !context->catalogProvisional || context->usefulLocalPreview);
    }
}

void F4GalleryBridge::rebuildPanelCatalogIndex(PanelSyncContext *context)
{
    SideState &state = *context->state;
    state.entryOffsetByRow.clear();
    if (context->catalogRowsDeferred) {
        state.entries.clear();
        state.entries.reserve(context->entries.size());
        for (const QVariant &entryValue : context->entries) {
            const QVariantMap entry = entryValue.toMap();
            bool indexOK = false;
            const int row = entry.value(QStringLiteral("index"))
                                .toInt(&indexOK);
            if (indexOK && row >= 0 && row < state.totalCount) {
                state.entryOffsetByRow.insert(row, state.entries.size());
                state.entries.push_back(entryValue);
            }
        }
    } else {
        state.entries = context->entries;
    }
    state.entryIds.clear();
    state.sourceIndexByEntryId.clear();
    for (const QVariant &entryValue : state.entries) {
        const QVariantMap entry = entryValue.toMap();
        const QString entryId = entry.value(
            QStringLiteral("entryId")).toString();
        if (entryId.isEmpty()) {
            continue;
        }
        state.entryIds.insert(entryId);
        state.sourceIndexByEntryId.insert(
            entryId, entry.value(QStringLiteral("index")).toInt());
    }
    state.catalogRowsRequestInFlight = false;
    state.catalogRowsRequestOffset = -1;
    state.catalogRowsRequestLimit = 0;
    state.catalogRowsVisibleFirst = context->cursorIndex >= 0
        ? context->cursorIndex : 0;
    state.catalogRowsVisibleLast = state.catalogRowsVisibleFirst;
    if (m_inFlightPanelOpen.active
        && m_inFlightPanelOpen.side == context->side
        && !state.entryIds.contains(m_inFlightPanelOpen.entryId)) {
        clearInFlightPanelOpen();
    }
}

void F4GalleryBridge::commitPanelSyncState(PanelSyncContext *context)
{
    SideState &state = *context->state;
    state.initialized = true;
    state.panelId = context->panelId;
    state.catalogRevision = context->catalogRevision;
    state.latestSemanticCatalogRevision = context->catalogRevision;
    state.selectionRevision = context->selectionRevision;
    if (!context->metadataDeferred) {
        state.highlightRevision = context->highlightRevision;
    }
    state.iconRevision = context->iconRevision;
    state.currentPath = context->currentPath;
    state.sourceKind = context->sourceKind;
    state.cursorEntryId = context->cursorEntryId;
    state.cursorIndex = context->cursorIndex;
    state.previewCapable = context->previewCapable;
    state.active = context->active;
    state.loading = context->loading;
    state.catalogProvisional = context->catalogProvisional;
    if (context->catalogApplied && context->catalogPayloadChanged
        && context->usefulLocalPreview && context->catalogRowsDeferred) {
        state.provisionalFrameRequiredRenderSyncSerial =
            m_renderSyncSerial.load(std::memory_order_acquire) + 1;
    } else if (!context->catalogProvisional) {
        state.provisionalFrameRequiredRenderSyncSerial = 0;
    }
    state.catalogRowsDeferred = context->catalogRowsDeferred;
    state.totalCount = context->catalogRowsDeferred
        ? qMax(0, context->incomingTotalCount) : context->entries.size();
    state.metadataDeferred = context->metadataDeferred;
    state.metadataRevision = context->metadataDeferred
        ? context->metadataRevision : 0;
    state.galleryLayoutMode = context->galleryLayoutMode;
    if (context->catalogPayloadChanged) {
        rebuildPanelCatalogIndex(context);
    } else if (context->appearanceChanged) {
        state.entries = context->entries;
    }
}

void F4GalleryBridge::updatePanelMetadataPlan(PanelSyncContext *context)
{
    SideState &state = *context->state;
    if (context->catalogStreamStart) {
        ++state.metadataPacingGeneration;
        state.metadataRequestInFlight = false;
        state.metadataAwaitingFrame = false;
        state.metadataRequiredRenderSyncSerial = 0;
        state.metadataComplete = true;
        state.metadataRequestOffset = -1;
        state.metadataRequestLimit = 0;
        state.metadataFailureCount = 0;
        state.metadataPendingRanges.clear();
        state.metadataVisibleFirst = -1;
        state.metadataVisibleLast = -1;
    } else if (context->metadataRestartNeeded) {
        if (context->session && context->catalogApplied) {
            resetPanelCatalogMetadataPlan(
                context->side, context->catalogPayloadChanged);
        } else {
            state.metadataRequestInFlight = false;
            state.metadataAwaitingFrame = false;
            state.metadataRequiredRenderSyncSerial = 0;
            state.metadataComplete = true;
            state.metadataRequestOffset = -1;
            state.metadataRequestLimit = 0;
            state.metadataPendingRanges.clear();
        }
    } else if (!context->metadataDeferred) {
        state.metadataRequestInFlight = false;
        state.metadataAwaitingFrame = false;
        state.metadataRequiredRenderSyncSerial = 0;
        state.metadataComplete = true;
        state.metadataRequestOffset = -1;
        state.metadataRequestLimit = 0;
        state.metadataFailureCount = 0;
        state.metadataPendingRanges.clear();
        state.metadataVisibleFirst = -1;
        state.metadataVisibleLast = -1;
    }
}

void F4GalleryBridge::finalizePanelSync(PanelSyncContext *context)
{
    SideState &state = *context->state;
    if (context->catalogRowsDeferred) {
        schedulePanelCatalogRowsRequest(context->side);
    }
    if (context->catalogChanged || context->selectionChanged) {
        state.selectedEntryIdList = context->selectedIds;
        state.selectedEntryIds = QSet<QString>(
            context->selectedIds.cbegin(), context->selectedIds.cend());
    }
    m_stateReconciliationPending[context->sideIndex] = false;
    reconcilePendingSelection(context->side);
    reconcilePendingCursor(context->side);
    reconcilePendingPanelOpen(context->side);
    reconcilePendingViewer(context->side);

    if (context->traceCatalogStages
        && (context->normalizeCatalog
            || context->catalogApplyStartedNs != 0
            || context->stateApplyStartedNs != 0)) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.gallery.bridge.catalog.applied"),
            context->catalogTraceId, {
                {QStringLiteral("side"), context->side},
                {QStringLiteral("rows"), context->entries.size()},
                {QStringLiteral("normalizeDurationNs"),
                 context->normalizeCatalog
                    ? context->normalizeCompletedNs
                        - context->normalizeStartedNs : 0},
                {QStringLiteral("catalogApplyDurationNs"),
                 context->catalogApplyStartedNs != 0
                    ? context->catalogApplyCompletedNs
                        - context->catalogApplyStartedNs : 0},
                {QStringLiteral("stateApplyDurationNs"),
                 context->stateApplyStartedNs != 0
                    ? context->stateApplyCompletedNs
                        - context->stateApplyStartedNs : 0},
                {QStringLiteral("catalogApplied"), context->catalogApplied},
            });
    }
    if (m_navigationBenchmark.enabled
        && m_navigationBenchmark.phase != NavigationBenchmarkPhase::Finished
        && m_navigationBenchmark.phase != NavigationBenchmarkPhase::Failed) {
        QVariantMap fields = navigationBenchmarkFields();
        fields.insert(QStringLiteral("syncSide"), context->side);
        fields.insert(QStringLiteral("syncPath"), state.currentPath);
        fields.insert(QStringLiteral("syncLoading"), state.loading);
        fields.insert(QStringLiteral("syncLayoutMode"),
                      state.galleryLayoutMode);
        fields.insert(QStringLiteral("syncCatalogRevision"),
                      QVariant::fromValue<qulonglong>(state.catalogRevision));
        fields.insert(QStringLiteral("syncEntryCount"),
                      catalogEntryCount(state));
        queueNavigationBenchmarkTrace(
            QStringLiteral("qt.gallery.bridge.panel.end"),
            m_navigationBenchmark.lastSceneTraceId, fields);
    }
}

void F4GalleryBridge::replayPanelOpenAfterSync(
    const PanelSyncContext &context)
{
    const SideState &state = *context.state;
    if (!context.repeatToReplay.active || !context.catalogApplied
        || !state.initialized || !state.active
        || (state.catalogProvisional && !context.usefulLocalPreview)
        || state.panelId != context.panelId
        || state.currentPath != context.currentPath) {
        return;
    }
    m_deferredPanelOpenRepeat.active = true;
    m_deferredPanelOpenRepeat.side = context.side;
    m_deferredPanelOpenRepeat.panelId = state.panelId;
    m_deferredPanelOpenRepeat.sourcePath = state.currentPath;
    m_deferredPanelOpenRepeat.catalogRevision = state.catalogRevision;
    replayDeferredPanelOpenRepeat(context.side, state.panelId,
                                  state.currentPath,
                                  state.catalogRevision);
}

void F4GalleryBridge::synchronizePanel(int side, const QVariantMap &panel)
{
    PanelSyncContext context = makePanelSyncContext(side, panel);
    if (deferSupersededPanelCatalog(&context)
        || deferPanelCatalogFinalization(&context)) {
        return;
    }
    QVariantMap mediaFields;
    if (ZoinGallery::MediaTimingTrace::enabled()) {
        mediaFields = {
            {QStringLiteral("side"), side},
            {QStringLiteral("panelId"), context.panelId},
            {QStringLiteral("path"), context.currentPath},
            {QStringLiteral("catalogRevision"),
             QVariant::fromValue<qulonglong>(context.catalogRevision)},
            {QStringLiteral("metadataDeferred"), context.metadataDeferred},
            {QStringLiteral("metadataRevision"),
             QVariant::fromValue<qulonglong>(context.metadataRevision)},
            {QStringLiteral("sourceKind"), context.sourceKind},
            {QStringLiteral("previewCapable"), context.previewCapable},
            {QStringLiteral("entries"), context.incomingEntries.size()},
            {QStringLiteral("loading"), context.loading},
            {QStringLiteral("catalogProvisional"),
             context.catalogProvisional},
            {QStringLiteral("layoutMode"), context.galleryLayoutMode},
        };
    }
    ZoinGallery::MediaTimingTrace::Span mediaSpan(
        QStringLiteral("qt.gallery.bridge.panel"), mediaFields);
    acknowledgePanelOpen(&context);
    tracePanelSyncBegin(context);
    handlePanelIdentityChange(&context);
    m_panelSnapshots[context.sideIndex] = panel;
    context.session = qobject_cast<ZoinGallery::GallerySession *>(
        m_panelSessions.session(side));
    if (applyProvisionalPanelUpdate(&context)) {
        mediaSpan.set(QStringLiteral("outcome"),
                      QStringLiteral("provisional-deferred"));
        return;
    }

    classifyPanelSyncChanges(&context);
    preparePanelCatalogData(&context);
    const bool presentationTransaction = context.session
        && (context.catalogChanged || context.metadataRestartNeeded);
    if (presentationTransaction) {
        emit panelPresentationTransactionStarted(
            side, context.panelId, context.catalogRevision,
            context.galleryLayoutMode,
            panel.value(QStringLiteral("galleryColumnCount"), 2).toInt(),
            panel.value(QStringLiteral("galleryDensity"), 0).toInt(),
            panel.value(QStringLiteral("galleryDensities")).toMap(),
            panel.value(QStringLiteral("galleryColumns")).toList(),
            panel.value(QStringLiteral("separateFileExtensions")).toBool());
    }
    applyPanelSessionData(&context);
    commitPanelSyncState(&context);
    updatePanelMetadataPlan(&context);
    finalizePanelSync(&context);
    if (presentationTransaction) {
        emit panelPresentationTransactionFinished(side);
    }
    mediaSpan.set(QStringLiteral("outcome"), QStringLiteral("applied"));
    mediaSpan.set(QStringLiteral("catalogChanged"), context.catalogChanged);
    mediaSpan.set(QStringLiteral("catalogPayloadChanged"),
                  context.catalogPayloadChanged);
    mediaSpan.set(QStringLiteral("catalogApplied"), context.catalogApplied);
    mediaSpan.set(QStringLiteral("normalizedEntries"),
                  context.entries.size());
    replayPanelOpenAfterSync(context);
}
