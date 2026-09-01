#include "F4GalleryBridge.h"

#include <ZoinGallery/GallerySession.h>

struct F4GalleryBridge::PanelStatePatchContext
{
    int side = -1;
    size_t sideIndex = 0;
    SideState *state = nullptr;
    ZoinGallery::GallerySession *session = nullptr;
    QVariantMap patch;
    QVariantMap panel;
    QString panelId;
    QString op;
    qulonglong catalogRevision = 0;

    QString cursorEntryId;
    int cursorIndex = -1;
    QString nextCurrentPath;
    QString nextSourceKind;
    bool nextPreviewCapable = false;
    bool nextMetadataDeferred = false;
    qulonglong nextMetadataRevision = 0;
    bool nextCatalogProvisional = false;
    QString nextGalleryLayoutMode;
    bool enteringDetails = false;
    bool metadataStreamChanged = false;
    bool metadataRestartNeeded = false;

    qulonglong nextSelectionRevision = 0;
    QStringList nextSelectedIds;
    QSet<QString> nextSelectedSet;
};

bool F4GalleryBridge::parsePanelStatePatch(
    const QVariantMap &patch, PanelStatePatchContext *context)
{
    if (!context) {
        return false;
    }
    bool sideOK = false;
    context->side = patch.value(QStringLiteral("side")).toInt(&sideOK);
    if (!sideOK || !validSide(context->side)) {
        return false;
    }
    const QVariant panelValue = patch.value(QStringLiteral("panel"));
    if (panelValue.metaType().id() != QMetaType::QVariantMap) {
        return false;
    }
    context->patch = patch;
    context->panel = panelValue.toMap();
    if (context->panel.contains(QStringLiteral("entries"))
        || context->panel.contains(QStringLiteral("highlightStyles"))) {
        return false;
    }
    context->sideIndex = static_cast<size_t>(context->side);
    context->state = &m_panelSessions.catalog(context->side);
    context->panelId = patch.value(QStringLiteral("panelId")).toString();
    context->catalogRevision = revisionValue(
        patch, QStringLiteral("catalogRevision"));
    context->op = patch.value(QStringLiteral("op")).toString();
    if (!context->state->initialized
        || context->panelId != context->state->panelId) {
        return false;
    }
    context->session = qobject_cast<ZoinGallery::GallerySession *>(
        m_panelSessions.session(context->side));
    return true;
}

bool F4GalleryBridge::resolvePanelStateRevision(
    PanelStatePatchContext *context)
{
    if (context->catalogRevision == context->state->catalogRevision) {
        return true;
    }
    const QVariantMap deferred = m_panelSnapshots[context->sideIndex];
    const QVariant provisionalValue = deferred.value(
        QStringLiteral("catalogProvisional"));
    const QVariant finalValue = context->panel.value(
        QStringLiteral("catalogProvisional"));
    const bool finalizesDeferredCatalog =
        context->op == QStringLiteral("state_update")
        && context->catalogRevision != 0
        && deferred.value(QStringLiteral("id")).toString()
            == context->panelId
        && revisionValue(deferred, QStringLiteral("catalogRevision"))
            == context->catalogRevision
        && deferred.contains(QStringLiteral("entries"))
        && provisionalValue.metaType().id() == QMetaType::Bool
        && provisionalValue.toBool()
        && finalValue.metaType().id() == QMetaType::Bool
        && !finalValue.toBool();
    if (!finalizesDeferredCatalog) {
        return false;
    }

    QVariantMap authoritative = deferred;
    for (auto it = context->panel.cbegin(); it != context->panel.cend(); ++it) {
        authoritative.insert(it.key(), it.value());
    }
    synchronizePanel(context->side, authoritative);
    schedulePanelCatalogMetadataRequest();
    return false;
}

void F4GalleryBridge::derivePanelStateValues(
    PanelStatePatchContext *context)
{
    const SideState &state = *context->state;
    context->cursorEntryId = context->panel.value(
        QStringLiteral("cursorEntryId"), state.cursorEntryId).toString();
    context->cursorIndex = context->panel.value(
        QStringLiteral("cursor"), state.cursorIndex).toInt();
    context->nextCurrentPath = context->panel.value(
        QStringLiteral("path"), state.currentPath).toString();
    context->nextSourceKind = context->panel.value(
        QStringLiteral("sourceKind"), state.sourceKind).toString();
    context->nextPreviewCapable = context->panel.value(
        QStringLiteral("previewCapable"), state.previewCapable).toBool();
    context->nextMetadataDeferred = context->panel.value(
        QStringLiteral("metadataDeferred"), state.metadataDeferred).toBool();
    context->nextMetadataRevision = revisionValue(
        context->panel, QStringLiteral("metadataRevision"));
    context->nextCatalogProvisional = context->panel.value(
        QStringLiteral("catalogProvisional"), state.catalogProvisional)
                                              .toBool();
    context->nextGalleryLayoutMode = context->panel.value(
        QStringLiteral("galleryLayoutMode"), state.galleryLayoutMode)
                                                .toString();
    context->enteringDetails = state.galleryLayoutMode
            != QStringLiteral("details")
        && context->nextGalleryLayoutMode == QStringLiteral("details");
    context->metadataStreamChanged =
        context->nextMetadataDeferred != state.metadataDeferred
        || (context->nextMetadataDeferred
            && context->nextMetadataRevision != state.metadataRevision)
        || context->nextCurrentPath != state.currentPath;
    context->metadataRestartNeeded = context->nextMetadataDeferred
        && context->metadataStreamChanged;
    context->nextSelectionRevision = state.selectionRevision;
    context->nextSelectedIds = state.selectedEntryIdList;
    context->nextSelectedSet = state.selectedEntryIds;
}

bool F4GalleryBridge::applyProvisionalPanelStatePatch(
    PanelStatePatchContext *context)
{
    SideState &state = *context->state;
    const bool provisionalPathUpdate =
        context->op == QStringLiteral("state_update")
        && state.initialized && context->nextCatalogProvisional
        && context->catalogRevision == state.catalogRevision
        && context->nextCurrentPath != state.currentPath;
    if (!provisionalPathUpdate) {
        return false;
    }
    state.active = context->panel.value(
        QStringLiteral("active"), state.active).toBool();
    state.loading = context->panel.value(
        QStringLiteral("loading"), state.loading).toBool();
    state.catalogProvisional = true;
    state.galleryLayoutMode = context->nextGalleryLayoutMode;
    m_stateReconciliationPending[context->sideIndex] = false;
    prioritizePanelCatalogMetadataRow(context->side, state.cursorIndex);
    return true;
}

bool F4GalleryBridge::applyPanelSelectionDelta(
    PanelStatePatchContext *context)
{
    const qulonglong baseRevision = revisionValue(
        context->patch, QStringLiteral("baseSelectionRevision"));
    context->nextSelectionRevision = revisionValue(
        context->patch, QStringLiteral("selectionRevision"));
    const QVariant changesValue = context->patch.value(
        QStringLiteral("changes"));
    if (baseRevision != context->state->selectionRevision
        || context->nextSelectionRevision <= baseRevision
        || changesValue.metaType().id() != QMetaType::QVariantList) {
        return false;
    }
    const QVariantList changes = changesValue.toList();
    const bool applied = !context->session
        || context->session->applyExternalStateDelta(
            context->cursorEntryId, context->cursorIndex, changes,
            baseRevision, context->nextSelectionRevision);
    if (!applied) {
        return false;
    }
    for (const QVariant &changeValue : changes) {
        const QVariantMap change = changeValue.toMap();
        const QString entryId = change.value(
            QStringLiteral("entryId")).toString();
        if (change.value(QStringLiteral("selected")).toBool()) {
            if (!context->nextSelectedSet.contains(entryId)) {
                context->nextSelectedSet.insert(entryId);
                context->nextSelectedIds.push_back(entryId);
            }
        } else if (context->nextSelectedSet.remove(entryId)) {
            context->nextSelectedIds.removeAll(entryId);
        }
    }
    return true;
}

bool F4GalleryBridge::applyPanelSelectionReplacement(
    PanelStatePatchContext *context)
{
    const qulonglong baseRevision = revisionValue(
        context->patch, QStringLiteral("baseSelectionRevision"));
    context->nextSelectionRevision = revisionValue(
        context->patch, QStringLiteral("selectionRevision"));
    const QVariant idsValue = context->patch.value(
        QStringLiteral("selectedEntryIds"));
    if (baseRevision != context->state->selectionRevision
        || context->nextSelectionRevision <= baseRevision
        || idsValue.metaType().id() != QMetaType::QVariantList) {
        return false;
    }
    context->nextSelectedIds.clear();
    for (const QVariant &idValue : idsValue.toList()) {
        context->nextSelectedIds.push_back(idValue.toString());
    }
    const bool applied = !context->session
        || context->session->applyExternalState(
            context->cursorEntryId, context->cursorIndex,
            context->nextSelectedIds, context->nextSelectionRevision);
    if (applied) {
        context->nextSelectedSet = QSet<QString>(
            context->nextSelectedIds.cbegin(),
            context->nextSelectedIds.cend());
    }
    return applied;
}

bool F4GalleryBridge::applyPanelStateOperation(
    PanelStatePatchContext *context)
{
    if (context->op == QStringLiteral("state_update")) {
        return !context->session
            || context->session->applyExternalStateDelta(
                context->cursorEntryId, context->cursorIndex, {},
                context->state->selectionRevision,
                context->state->selectionRevision);
    }
    if (context->op == QStringLiteral("selection_delta")) {
        return applyPanelSelectionDelta(context);
    }
    if (context->op == QStringLiteral("selection_replace")) {
        return applyPanelSelectionReplacement(context);
    }
    return false;
}

bool F4GalleryBridge::applyPanelStateCatalogOptions(
    PanelStatePatchContext *context)
{
    const SideState &state = *context->state;
    if (!context->session
        || (!context->metadataStreamChanged
            && context->nextCatalogProvisional
                == state.catalogProvisional)) {
        return true;
    }
    return context->session->applyExternalCatalog(
        state.entries, state.catalogRevision, {
            {QStringLiteral("currentPath"), context->nextCurrentPath},
            {QStringLiteral("sourceKind"), context->nextSourceKind},
            {QStringLiteral("previewCapable"),
             context->nextPreviewCapable},
            {QStringLiteral("catalogProvisional"),
             context->nextCatalogProvisional},
            {QStringLiteral("metadataDeferred"),
             context->nextMetadataDeferred},
            {QStringLiteral("metadataRevision"),
             QVariant::fromValue<qulonglong>(
                 context->nextMetadataRevision)},
            {QStringLiteral("catalogRowsDeferred"),
             state.catalogRowsDeferred},
            {QStringLiteral("totalCount"), state.totalCount},
        });
}

void F4GalleryBridge::commitPanelStatePatch(
    PanelStatePatchContext *context)
{
    QVariantMap &snapshot = m_panelSnapshots[context->sideIndex];
    for (auto it = context->panel.cbegin(); it != context->panel.cend(); ++it) {
        snapshot.insert(it.key(), it.value());
    }
    SideState &state = *context->state;
    state.selectionRevision = context->nextSelectionRevision;
    state.selectedEntryIdList = std::move(context->nextSelectedIds);
    state.selectedEntryIds = std::move(context->nextSelectedSet);
    state.cursorEntryId = context->cursorEntryId;
    state.cursorIndex = context->cursorIndex;
    state.currentPath = context->nextCurrentPath;
    state.sourceKind = context->nextSourceKind;
    state.previewCapable = context->nextPreviewCapable;
    state.active = context->panel.value(
        QStringLiteral("active"), state.active).toBool();
    state.loading = context->panel.value(
        QStringLiteral("loading"), state.loading).toBool();
    state.catalogProvisional = context->nextCatalogProvisional;
    state.metadataDeferred = context->nextMetadataDeferred;
    state.metadataRevision = context->nextMetadataDeferred
        ? context->nextMetadataRevision : 0;
    state.galleryLayoutMode = context->nextGalleryLayoutMode;
}

void F4GalleryBridge::updatePanelStateMetadataPlan(
    PanelStatePatchContext *context)
{
    SideState &state = *context->state;
    if (context->metadataRestartNeeded) {
        resetPanelCatalogMetadataPlan(context->side, false);
        return;
    }
    if (context->nextMetadataDeferred) {
        return;
    }
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
}

void F4GalleryBridge::finalizePanelStatePatch(
    PanelStatePatchContext *context)
{
    SideState &state = *context->state;
    m_stateReconciliationPending[context->sideIndex] = false;
    reconcilePendingSelection(context->side);
    reconcilePendingCursor(context->side);
    reconcilePendingPanelOpen(context->side);
    reconcilePendingViewer(context->side);
    if (context->enteringDetails && state.metadataDeferred
        && !state.metadataComplete) {
        state.metadataUrgentBudget = 1;
    }
    prioritizePanelCatalogMetadataRow(
        context->side, context->cursorIndex);
    schedulePanelCatalogMetadataRequest();
}

void F4GalleryBridge::synchronizePanelState(const QVariantMap &patch)
{
    PanelStatePatchContext context;
    if (!parsePanelStatePatch(patch, &context)
        || !resolvePanelStateRevision(&context)) {
        return;
    }
    derivePanelStateValues(&context);
    if (applyProvisionalPanelStatePatch(&context)) {
        return;
    }
    if (!applyPanelStateOperation(&context)
        || !applyPanelStateCatalogOptions(&context)) {
        return;
    }
    commitPanelStatePatch(&context);
    updatePanelStateMetadataPlan(&context);
    finalizePanelStatePatch(&context);
}
