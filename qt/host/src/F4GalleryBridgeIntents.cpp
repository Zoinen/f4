#include "F4GalleryBridge.h"
#include "NavigationBenchmarkTrace.h"
#include "ViewerCoordinator.h"

#include <QSet>
#include <QTimer>

#include <ZoinGallery/GallerySession.h>

#include <algorithm>
#include <utility>

namespace
{
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
void F4GalleryBridge::requestActivate(int side)
{
    if (!validSide(side)) {
        return;
    }
    noteMetadataInputActivity();
    PanelIntent intent;
    intent.kind = PanelIntent::Kind::Activate;
    intent.side = side;
    m_panelIntentController->dispatch(intent);
}

void F4GalleryBridge::requestCursor(int side,
                                    const QString &entryId,
                                    int index,
                                    qulonglong catalogRevision,
                                    bool deferCommit,
                                    bool selectionGesture)
{
    if (!validSide(side)) {
        return;
    }
    noteMetadataInputActivity();
    prioritizePanelCatalogMetadataRow(side, index);
    if (m_viewerCoordinator->pendingIntent().active && m_viewerCoordinator->pendingIntent().side == side
        && m_viewerCoordinator->pendingIntent().entryId != entryId) {
        clearPendingViewer();
    }
    if (m_pendingPanelOpen.active && m_pendingPanelOpen.side == side
        && m_pendingPanelOpen.entryId != entryId) {
        clearPendingPanelOpen();
    }
    if (!entryId.isEmpty()) {
        PendingCursor &pending = m_pendingCursors[static_cast<size_t>(side)];
        pending.active = true;
        pending.panelId = m_panelSessions.catalog(side).panelId;
        pending.entryId = entryId;
        pending.index = index;
        pending.catalogRevision = effectiveCatalogRevision(side, catalogRevision);
        pending.maskAcrossCatalog = selectionGesture;
    } else {
        clearPendingCursor(side);
    }
    m_stateReconciliationPending[static_cast<size_t>(side)] = true;
    if (deferCommit) {
        // Gallery already moved optimistically. Keep the latest stable ID in
        // pending state so older scenes cannot snap it backward, but avoid
        // serializing a full semantic catalog for every native key-repeat.
        QTimer *timer = m_cursorCommitTimers[static_cast<size_t>(side)];
        if (selectionGesture) {
            // Shift/Insert/Space have an explicit physical release boundary
            // which atomically carries cursor plus selection. A watchdog
            // cursor action in the middle of that hold would split the user
            // operation and reintroduce the very stale-scene race this local
            // preview is meant to avoid.
            timer->stop();
        } else {
            timer->start();
        }
        return;
    }
    m_cursorCommitTimers[static_cast<size_t>(side)]->stop();
    const bool activateTarget = !entryId.isEmpty()
        && !m_panelSessions.catalog(side).active;
    sendPanelAction(side, QStringLiteral("panel.cursor"), entryId, index,
                    catalogRevision, true, activateTarget);
}

void F4GalleryBridge::requestOpen(int side,
                                  const QString &entryId,
                                  int index,
                                  bool isImage,
                                  qulonglong catalogRevision,
                                  bool autoRepeat)
{
    if (!validSide(side)) {
        return;
    }
    if (!autoRepeat && m_deferredPanelOpenRepeat.active) {
        // A fresh press/pointer gesture is newer than a queued synthetic
        // repeat and must not be followed by that older intent.
        m_deferredPanelOpenRepeat = DeferredPanelOpenRepeat{};
    }
    noteMetadataInputActivity();
    prioritizePanelCatalogMetadataRow(side, index);

    const SideState &sideState = m_panelSessions.catalog(side);
    if (autoRepeat && m_inFlightPanelOpen.active) {
        // A delivered open intent is authoritative for the current panel
        // snapshot. Key repeat can otherwise enqueue the same stale row many
        // times before Go publishes the destination catalog. A path/panel
        // transition clears this guard immediately; the watchdog permits a
        // retry if the operation is rejected without a semantic update.
        if (m_inFlightPanelOpen.side == side
            && m_inFlightPanelOpen.panelId == sideState.panelId
            && m_inFlightPanelOpen.sourcePath == sideState.currentPath) {
            // Keep one repeat intent, but never keep the stale row identity.
            // The destination catalog owns the next row under the cursor; it
            // is resolved only after that authoritative path is synchronized.
            m_deferredPanelOpenRepeat.active = true;
            m_deferredPanelOpenRepeat.side = side;
            m_deferredPanelOpenRepeat.panelId = m_inFlightPanelOpen.panelId;
            m_deferredPanelOpenRepeat.sourcePath = m_inFlightPanelOpen.sourcePath;
            m_deferredPanelOpenRepeat.catalogRevision =
                m_inFlightPanelOpen.catalogRevision;
            F4NavigationBenchmarkTrace::event(
                QStringLiteral("qt.gallery.open.repeat.deferred"),
                m_lastInputSceneTraceId, {
                    {QStringLiteral("side"), side},
                    {QStringLiteral("sourcePath"), sideState.currentPath},
                    {QStringLiteral("catalogRevision"),
                     QVariant::fromValue<qulonglong>(
                         sideState.catalogRevision)},
                });
            return;
        }
        clearInFlightPanelOpen();
    }
    else if (!autoRepeat && m_inFlightPanelOpen.active) {
        // A fresh key press or pointer gesture is a new explicit user intent.
        // Only synthetic keyboard repeat is coalesced.
        clearInFlightPanelOpen();
    }
    if (isImage && available() && sideState.previewCapable) {
        clearPendingPanelOpen();
        closeViewer();
        m_viewerCoordinator->beginPending(
            side, m_panelSessions.catalog(side).panelId, entryId,
            effectiveCatalogRevision(side, catalogRevision));

        // Enter and the second press of a double-click often target the
        // cursor that Go has already confirmed. Opening that image must not
        // wait for another identical panel.cursor scene: unchanged semantic
        // scenes are deliberately suppressed by the renderer, so no such
        // acknowledgement is guaranteed to arrive.
        if (sideState.active && sideState.cursorEntryId == entryId) {
            // The QML delegate can still carry the preceding scene revision
            // while its stable entry identity already matches the bridge's
            // authoritative state. Use that authoritative revision so a
            // harmless stale binding cannot turn the immediate path back
            // into a pending open with no future scene to reconcile it.
            m_viewerCoordinator->pendingIntent().catalogRevision = sideState.catalogRevision;
            reconcilePendingViewer(side);
            return;
        }
        requestCursor(side, entryId, index, m_viewerCoordinator->pendingIntent().catalogRevision);
        return;
    }

    clearPendingViewer();
    clearPendingPanelOpen();
    m_pendingPanelOpen.active = true;
    m_pendingPanelOpen.side = side;
    m_pendingPanelOpen.panelId = m_panelSessions.catalog(side).panelId;
    m_pendingPanelOpen.entryId = entryId;

    // panel.open carries the row's stable entry identity, and Go resolves the
    // row from that identity and moves its own cursor onto it before acting.
    // The open therefore needs no prior cursor round trip, and must not wait
    // for one: once the two sides disagree about the cursor, Go has nothing
    // new to say (it suppresses unchanged semantic scenes), so a confirmation
    // that never arrives would strand the open forever. panel.open activates
    // the owning panel and moves its cursor on the Go side in that same
    // semantic operation.
    clearPendingCursor(side);
    reconcilePendingPanelOpen(side);
}

void F4GalleryBridge::requestSelection(int side,
                                       const QString &mode,
                                       const QVariantList &entryIds,
                                       qulonglong catalogRevision)
{
    if (!validSide(side)) {
        return;
    }

    static const QSet<QString> validModes = {
        QStringLiteral("replace"), QStringLiteral("add"),
        QStringLiteral("remove"), QStringLiteral("toggle"),
    };
    const QString normalizedMode = validModes.contains(mode) ? mode : QStringLiteral("toggle");
    if (entryIds.isEmpty() && normalizedMode != QStringLiteral("replace")) {
        return;
    }

    const size_t sideIndex = static_cast<size_t>(side);
    const SideState &state = m_panelSessions.catalog(static_cast<int>(sideIndex));
    PendingSelection &pending = m_pendingSelections[sideIndex];
    if (!pending.active || pending.panelId != state.panelId) {
        pending = PendingSelection{};
        pending.active = true;
        pending.panelId = state.panelId;
        pending.catalogRevision = effectiveCatalogRevision(side, catalogRevision);
        pending.selectionRevision = state.selectionRevision;
    }

    QSet<QString> requested;
    for (const QVariant &value : entryIds) {
        const QString id = value.toString();
        if (!id.isEmpty()) {
            requested.insert(id);
        }
    }
    if (normalizedMode == QStringLiteral("replace")) {
        for (const QString &id : state.entryIds) {
            pending.desiredByEntryId.insert(id, requested.contains(id));
        }
    } else {
        for (const QString &id : requested) {
            if (normalizedMode == QStringLiteral("add")) {
                pending.desiredByEntryId.insert(id, true);
            } else if (normalizedMode == QStringLiteral("remove")) {
                pending.desiredByEntryId.insert(id, false);
            } else {
                const bool current = pending.desiredByEntryId.contains(id)
                    ? pending.desiredByEntryId.value(id)
                    : state.selectedEntryIds.contains(id);
                pending.desiredByEntryId.insert(id, !current);
            }
        }
    }

    const qulonglong revision = effectiveCatalogRevision(side, catalogRevision);
    pending.catalogRevision = revision;
    pending.selectionRevision = state.selectionRevision;
    emitSelectionAction(side, normalizedMode, entryIds, revision);
}

void F4GalleryBridge::requestSelectionTransaction(
    int side, const QVariantList &changes, const QString &cursorEntryId,
    int cursorIndex, qulonglong catalogRevision)
{
    Q_UNUSED(cursorIndex)
    if (!validSide(side)) {
        return;
    }
    const size_t sideIndex = static_cast<size_t>(side);
    const SideState &state = m_panelSessions.catalog(static_cast<int>(sideIndex));
    QVariantList normalizedChanges;
    if (!normalizeSelectionChanges(state, changes, normalizedChanges)) {
        return;
    }
    QString normalizedCursorEntryId;
    int normalizedCursorIndex = -1;
    if (!normalizeSelectionCursor(
            state, cursorEntryId, normalizedCursorEntryId,
            normalizedCursorIndex)) {
        return;
    }
    if (normalizedChanges.isEmpty()) {
        if (!normalizedCursorEntryId.isEmpty()) {
            requestCursor(side, normalizedCursorEntryId,
                          normalizedCursorIndex, catalogRevision, false);
        }
        return;
    }
    noteMetadataInputActivity();
    if (normalizedCursorIndex >= 0) {
        stageSelectionCursor(side, state, normalizedCursorEntryId,
                             normalizedCursorIndex, catalogRevision);
    }
    stageSelectionChanges(
        side, state, normalizedChanges, catalogRevision);
    dispatchSelectionTransaction(
        side, state, normalizedChanges, normalizedCursorEntryId,
        normalizedCursorIndex, catalogRevision);
}

bool F4GalleryBridge::normalizeSelectionChanges(
    const SideState &state, const QVariantList &changes,
    QVariantList &normalizedChanges) const
{
    QHash<QString, int> normalizedPosition;
    for (const QVariant &value : changes) {
        const QVariantMap change = value.toMap();
        const QString entryId = change.value(
            QStringLiteral("entryId")).toString();
        if (entryId.isEmpty() || !change.contains(QStringLiteral("selected"))
            || !state.entryIds.contains(entryId)) {
            return false;
        }
        const QVariantMap normalized = {
            {QStringLiteral("entryId"), entryId},
            {QStringLiteral("selected"),
             change.value(QStringLiteral("selected")).toBool()},
        };
        const auto existing = normalizedPosition.constFind(entryId);
        if (existing == normalizedPosition.cend()) {
            normalizedPosition.insert(entryId, normalizedChanges.size());
            normalizedChanges.push_back(normalized);
        } else {
            normalizedChanges[existing.value()] = normalized;
        }
    }
    return true;
}

bool F4GalleryBridge::normalizeSelectionCursor(
    const SideState &state, const QString &cursorEntryId,
    QString &normalizedEntryId, int &normalizedIndex) const
{
    normalizedEntryId = cursorEntryId;
    normalizedIndex = -1;
    if (normalizedEntryId.isEmpty()) {
        return true;
    }
    normalizedIndex = state.sourceIndexByEntryId.value(
        normalizedEntryId, -1);
    return normalizedIndex >= 0;
}

void F4GalleryBridge::stageSelectionCursor(
    int side, const SideState &state, const QString &entryId,
    int index, qulonglong catalogRevision)
{
    const size_t sideIndex = static_cast<size_t>(side);
    prioritizePanelCatalogMetadataRow(side, index);
    PendingCursor &pending = m_pendingCursors[sideIndex];
    pending.active = true;
    pending.panelId = state.panelId;
    pending.entryId = entryId;
    pending.index = index;
    pending.catalogRevision = effectiveCatalogRevision(
        side, catalogRevision);
    pending.maskAcrossCatalog = true;
    if (QTimer *timer = m_cursorCommitTimers[sideIndex]) {
        timer->stop();
    }
}

void F4GalleryBridge::stageSelectionChanges(
    int side, const SideState &state,
    const QVariantList &normalizedChanges,
    qulonglong catalogRevision)
{
    const size_t sideIndex = static_cast<size_t>(side);
    PendingSelection &pendingSelection = m_pendingSelections[sideIndex];
    if (!pendingSelection.active
        || pendingSelection.panelId != state.panelId) {
        pendingSelection = PendingSelection{};
        pendingSelection.active = true;
        pendingSelection.panelId = state.panelId;
    }
    for (const QVariant &value : std::as_const(normalizedChanges)) {
        const QVariantMap change = value.toMap();
        pendingSelection.desiredByEntryId.insert(
            change.value(QStringLiteral("entryId")).toString(),
            change.value(QStringLiteral("selected")).toBool());
    }

    const qulonglong revision = effectiveCatalogRevision(
        side, catalogRevision);
    pendingSelection.catalogRevision = revision;
    pendingSelection.selectionRevision = state.selectionRevision;
    m_stateReconciliationPending[sideIndex] = true;
}

void F4GalleryBridge::dispatchSelectionTransaction(
    int side, const SideState &state,
    const QVariantList &normalizedChanges,
    const QString &cursorEntryId, int cursorIndex,
    qulonglong catalogRevision)
{
    const size_t sideIndex = static_cast<size_t>(side);
    const qulonglong revision = effectiveCatalogRevision(
        side, catalogRevision);
    PanelIntent intent;
    intent.kind = PanelIntent::Kind::SetSelection;
    intent.side = side;
    intent.mode = QStringLiteral("set");
    intent.changes = normalizedChanges;
    intent.catalogRevision = revision;
    intent.includeCatalogRevision = true;
    if (!cursorEntryId.isEmpty()) {
        intent.cursorEntryId = cursorEntryId;
        intent.cursorIndex = cursorIndex;
        intent.activate = !state.active;
    }
    if (state.selectionRevision != 0
        && !m_selectionActionPending[sideIndex]) {
        intent.selectionRevision = state.selectionRevision;
    }
    m_selectionActionPending[sideIndex] = true;
    m_panelIntentController->dispatch(intent);
}

void F4GalleryBridge::requestGalleryLayout(int side,
                                           const QString &layoutMode,
                                           int columnCount)
{
    if (!validSide(side)) {
        return;
    }
    static const QSet<QString> supported = {
        QStringLiteral("masonry"), QStringLiteral("columns"),
        QStringLiteral("details"), QStringLiteral("grid"),
        QStringLiteral("icons"),
    };
    const QString normalized = layoutMode.trimmed().toLower();
    if (!supported.contains(normalized)) {
        return;
    }
    PanelIntent intent;
    intent.kind = PanelIntent::Kind::SetGalleryLayout;
    intent.side = side;
    intent.layoutMode = normalized;
    intent.columnCount = columnCount;
    m_panelIntentController->dispatch(intent);
}

void F4GalleryBridge::requestGalleryDensity(int side,
                                            const QString &layoutMode,
                                            int density)
{
    if (!validSide(side)) {
        return;
    }
    static const QSet<QString> adjustable = {
        QStringLiteral("masonry"), QStringLiteral("columns"),
        QStringLiteral("details"), QStringLiteral("grid"),
        QStringLiteral("icons"),
    };
    const QString normalized = layoutMode.trimmed().toLower();
    if (!adjustable.contains(normalized)) {
        return;
    }
    PanelIntent intent;
    intent.kind = PanelIntent::Kind::SetGalleryDensity;
    intent.side = side;
    intent.layoutMode = normalized;
    intent.density = density;
    m_panelIntentController->dispatch(intent);
}

void F4GalleryBridge::requestSort(int side, const QString &sortMode,
                                  bool contextMenu)
{
    if (!validSide(side)) {
        return;
    }
    const QString normalized = sortMode.trimmed().toLower();
    if (contextMenu) {
        PanelIntent intent;
        intent.kind = PanelIntent::Kind::SortMenu;
        intent.side = side;
        m_panelIntentController->dispatch(intent);
        return;
    }
    static const QSet<QString> supported = {
        QStringLiteral("name"), QStringLiteral("extension"),
        QStringLiteral("time"), QStringLiteral("size"),
        QStringLiteral("unsorted"),
    };
    if (!supported.contains(normalized)) {
        return;
    }
    PanelIntent intent;
    intent.kind = PanelIntent::Kind::Sort;
    intent.side = side;
    intent.mode = normalized;
    m_panelIntentController->dispatch(intent);
}

void F4GalleryBridge::closeViewer()
{
    clearPendingViewer();
    if (!viewerVisible()) {
        return;
    }
    if (auto *session = qobject_cast<ZoinGallery::GallerySession *>(viewerSession())) {
        session->setViewerOpen(false);
    }
    setViewer(-1, false);
}

void F4GalleryBridge::sendPanelAction(int side,
                                      const QString &actionName,
                                      const QString &entryId,
                                      int index,
                                      qulonglong catalogRevision,
                                      bool includeCatalogRevision,
                                      bool activate)
{
    if (!validSide(side)) {
        return;
    }
    PanelIntent intent;
    intent.kind = actionName == QStringLiteral("panel.open")
        ? PanelIntent::Kind::Open : PanelIntent::Kind::Cursor;
    intent.side = side;
    intent.entryId = entryId;
    intent.index = index;
    intent.includeCatalogRevision = includeCatalogRevision;
    if (includeCatalogRevision) {
        intent.catalogRevision = effectiveCatalogRevision(
            side, catalogRevision);
    }
    intent.activate = activate;
    m_panelIntentController->dispatch(intent);
}

qulonglong F4GalleryBridge::effectiveCatalogRevision(int side, qulonglong supplied) const
{
    if (!validSide(side)) {
        return supplied;
    }

    const size_t sideIndex = static_cast<size_t>(side);
    const SideState &state = m_panelSessions.catalog(static_cast<int>(sideIndex));
    qulonglong effective = supplied;
    if (m_inFlightPanelOpen.active
        && m_inFlightPanelOpen.side == side
        && !m_inFlightPanelOpen.deferredSourcePanel.isEmpty()) {
        const QVariantMap &deferredSource =
            m_inFlightPanelOpen.deferredSourcePanel;
        const QString deferredPanelId = deferredSource.value(
            QStringLiteral("id")).toString();
        const QString deferredPath = deferredSource.value(
            QStringLiteral("path")).toString();
        if ((!state.initialized || deferredPanelId == state.panelId)
            && (!state.initialized || deferredPath == state.currentPath)) {
            const qulonglong revision = revisionValue(
                deferredSource, QStringLiteral("catalogRevision"));
            if (revision != 0) {
                // Rendering a source-catalog completion is deliberately held
                // while a directory open is awaiting its path acknowledgement.
                // The held descriptor is still the newest semantic catalog,
                // however, so a newer pointer gesture must validate against
                // its revision rather than the older rows still on screen.
                effective = qMax(effective, revision);
            }
        }
    }
    const DeferredCatalogFinalization &deferred =
        m_deferredCatalogFinalizations[sideIndex];
    if (deferred.active) {
        const qulonglong revision = revisionValue(
            deferred.panel, QStringLiteral("catalogRevision"));
        if (revision != 0) {
            effective = qMax(effective, revision);
        }
    }
    if (state.initialized && state.catalogRevision != 0) {
        // The bridge receives the native panel stream before QML observes its
        // compact presentation update, so its stable-ID catalog snapshot is
        // authoritative. A pointer gesture can nevertheless finish on a
        // delegate created from the preceding Loader binding. Forwarding that
        // stale non-zero revision makes Go reject panel.cursor; because the
        // scene itself did not change, there is then no acknowledgement with
        // which to reconcile a pending double-click/open. Resolve the stable
        // identity against the bridge-owned snapshot and always use its
        // revision. The supplied value remains the bootstrap fallback before
        // the first semantic scene has initialized this side.
        effective = qMax(effective, state.catalogRevision);
        effective = qMax(effective, state.latestSemanticCatalogRevision);
    }
    return effective;
}

void F4GalleryBridge::commitPendingCursor(int side)
{
    if (!validSide(side)) {
        return;
    }
    if (QTimer *timer = m_cursorCommitTimers[static_cast<size_t>(side)]) {
        timer->stop();
    }
    PendingCursor &pending = m_pendingCursors[static_cast<size_t>(side)];
    if (!pending.active || pending.entryId.isEmpty()) {
        return;
    }
    m_stateReconciliationPending[static_cast<size_t>(side)] = true;
    sendPanelAction(side, QStringLiteral("panel.cursor"), pending.entryId,
                    pending.index, pending.catalogRevision, true,
                    !m_panelSessions.catalog(side).active);
}

void F4GalleryBridge::reconcilePendingCursor(int side)
{
    if (!validSide(side)) {
        return;
    }

    PendingCursor &pending = m_pendingCursors[static_cast<size_t>(side)];
    if (!pending.active) {
        return;
    }

    const SideState &state = m_panelSessions.catalog(side);
    if (state.panelId != pending.panelId) {
        clearPendingCursor(side);
        if (m_viewerCoordinator->pendingIntent().active && m_viewerCoordinator->pendingIntent().side == side) {
            clearPendingViewer();
        }
        return;
    }

    if (state.cursorEntryId == pending.entryId) {
        clearPendingCursor(side);
        return;
    }

    const int currentSourceIndex =
        state.sourceIndexByEntryId.value(pending.entryId, -1);
    if (currentSourceIndex < 0) {
        clearPendingCursor(side);
        if (m_viewerCoordinator->pendingIntent().active && m_viewerCoordinator->pendingIntent().side == side) {
            clearPendingViewer();
        }
        return;
    }

    // A local catalog can advance between the scene used to create an intent
    // and Go processing that intent (for example when cached Desktop entries
    // are replaced by the completed async scan). Go correctly rejects the
    // stale revision. Retry the same stable identity against the newer
    // authoritative revision; never retry by the stale row alone.
    if (state.catalogRevision != pending.catalogRevision) {
        pending.catalogRevision = state.catalogRevision;
        pending.index = currentSourceIndex;
        if (m_viewerCoordinator->pendingIntent().active && m_viewerCoordinator->pendingIntent().side == side
            && m_viewerCoordinator->pendingIntent().entryId == pending.entryId) {
            m_viewerCoordinator->pendingIntent().catalogRevision = state.catalogRevision;
        }
        m_stateReconciliationPending[static_cast<size_t>(side)] = true;
        sendPanelAction(side, QStringLiteral("panel.cursor"), pending.entryId,
                        pending.index, pending.catalogRevision, true,
                        !state.active);
    }
}

void F4GalleryBridge::clearPendingCursor(int side)
{
    if (validSide(side)) {
        if (QTimer *timer = m_cursorCommitTimers[static_cast<size_t>(side)]) {
            timer->stop();
        }
        m_pendingCursors[static_cast<size_t>(side)] = PendingCursor{};
    }
}

void F4GalleryBridge::reconcilePendingPanelOpen(int side)
{
    if (!m_pendingPanelOpen.active || m_pendingPanelOpen.side != side
        || !validSide(side)) {
        return;
    }

    const SideState &state = m_panelSessions.catalog(side);
    if (state.panelId != m_pendingPanelOpen.panelId) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.gallery.open.reconcile.panelIdChanged"), {},
            {{QStringLiteral("side"), side},
             {QStringLiteral("entryId"), m_pendingPanelOpen.entryId}});
        clearPendingPanelOpen();
        return;
    }

    const int currentSourceIndex = state.sourceIndexByEntryId.value(
        m_pendingPanelOpen.entryId, -1);
    if (currentSourceIndex < 0) {
        // A successful directory open replaces the catalog and removes the
        // source entry. Removal by a concurrent file operation has the same
        // safe terminal behavior: never open a different row.
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.gallery.open.reconcile.entryMissing"), {},
            {{QStringLiteral("side"), side},
             {QStringLiteral("entryId"), m_pendingPanelOpen.entryId},
             {QStringLiteral("knownEntries"),
              static_cast<int>(state.sourceIndexByEntryId.size())},
             {QStringLiteral("currentPath"), state.currentPath},
             {QStringLiteral("catalogRevision"),
              QVariant::fromValue<qulonglong>(state.catalogRevision)},
             {QStringLiteral("cursorEntryId"), state.cursorEntryId}});
        clearPendingPanelOpen();
        return;
    }

    // The row was found in this panel's current catalog by its stable entry
    // identity, which is all panel.open needs: Go resolves the same identity
    // and moves its own cursor onto it. Deliberately do not also require the
    // authoritative cursor to already sit on that row — the two sides can
    // legitimately disagree about the cursor (Go suppresses unchanged
    // semantic scenes, so a cursor it already applied is never re-announced),
    // and waiting for a confirmation that can never arrive silently dropped
    // the open.
    //
    // From this point the operation is an exactly-once stable-ID intent:
    // including a catalog revision would only make an unrelated later
    // revision look retryable and could relaunch an external application.
    // Clear before emitting so even a reentrant scene update cannot dispatch
    // it twice.
    const QString entryId = m_pendingPanelOpen.entryId;
    clearPendingPanelOpen();
    markPanelOpenInFlight(side, entryId);
    sendPanelAction(side, QStringLiteral("panel.open"), entryId,
                    currentSourceIndex, 0, false,
                    !m_panelSessions.catalog(side).active);
}

void F4GalleryBridge::clearPendingPanelOpen()
{
    m_pendingPanelOpen = PendingPanelOpen{};
}

void F4GalleryBridge::markPanelOpenInFlight(int side,
                                            const QString &entryId)
{
    if (!validSide(side) || entryId.isEmpty()) {
        return;
    }
    const SideState &state = m_panelSessions.catalog(side);
    bool expectsPathChange = false;
    for (const QVariant &value : state.entries) {
        const QVariantMap entry = value.toMap();
        if (entry.value(QStringLiteral("entryId")).toString() == entryId) {
            expectsPathChange = entry.value(QStringLiteral("isDir")).toBool()
                || entry.value(QStringLiteral("isUp")).toBool();
            break;
        }
    }
    clearInFlightPanelOpen();
    m_inFlightPanelOpen.active = true;
    m_inFlightPanelOpen.side = side;
    m_inFlightPanelOpen.panelId = state.panelId;
    m_inFlightPanelOpen.entryId = entryId;
    m_inFlightPanelOpen.sourcePath = state.currentPath;
    m_inFlightPanelOpen.catalogRevision = state.catalogRevision;
    m_inFlightPanelOpen.expectsPathChange = expectsPathChange;
    if (m_panelOpenWatchdog) {
        m_panelOpenWatchdog->start();
    }
}

void F4GalleryBridge::clearInFlightPanelOpen()
{
    if (m_panelOpenWatchdog) {
        m_panelOpenWatchdog->stop();
    }
    m_inFlightPanelOpen = InFlightPanelOpen{};
    m_deferredPanelOpenRepeat = DeferredPanelOpenRepeat{};
}

void F4GalleryBridge::handlePanelOpenWatchdog()
{
    if (!m_inFlightPanelOpen.active) {
        return;
    }
    const int side = m_inFlightPanelOpen.side;
    const QVariantMap deferredPanel =
        std::move(m_inFlightPanelOpen.deferredSourcePanel);
    clearInFlightPanelOpen();
    if (validSide(side) && !deferredPanel.isEmpty()) {
        // No destination acknowledgement arrived. Restore the authoritative
        // source update that was held out of the interaction-critical path.
        synchronizePanel(side, deferredPanel);
    }
}

void F4GalleryBridge::replayDeferredPanelOpenRepeat(
    int side, const QString &panelId, const QString &sourcePath,
    qulonglong catalogRevision)
{
    if (!m_deferredPanelOpenRepeat.active
        || m_deferredPanelOpenRepeat.side != side
        || m_deferredPanelOpenRepeat.panelId != panelId
        || m_deferredPanelOpenRepeat.sourcePath != sourcePath
        || m_deferredPanelOpenRepeat.catalogRevision != catalogRevision) {
        return;
    }
    m_deferredPanelOpenRepeat = DeferredPanelOpenRepeat{};
    if (!validSide(side)) {
        return;
    }

    const size_t sideIndex = static_cast<size_t>(side);
    const SideState &state = m_panelSessions.catalog(static_cast<int>(sideIndex));
    const bool usefulLocalPreview = state.catalogProvisional
        && usefulLocalCatalogPreview(state.sourceKind, state.previewCapable,
                                     state.entries);
    if (!state.initialized || !state.active
        || (state.catalogProvisional && !usefulLocalPreview)
        || state.panelId != panelId || state.currentPath != sourcePath
        || state.catalogRevision != catalogRevision
        || state.cursorEntryId.isEmpty()) {
        return;
    }
    const int sourceIndex = state.sourceIndexByEntryId.value(
        state.cursorEntryId, -1);
    if (sourceIndex < 0) {
        return;
    }

    bool isImage = false;
    for (const QVariant &value : state.entries) {
        const QVariantMap entry = value.toMap();
        if (entry.value(QStringLiteral("entryId")).toString()
            == state.cursorEntryId) {
            isImage = entry.value(QStringLiteral("isImage")).toBool();
            break;
        }
    }
    F4NavigationBenchmarkTrace::event(
        QStringLiteral("qt.gallery.open.repeat.replayed"),
        m_lastInputSceneTraceId, {
            {QStringLiteral("side"), side},
            {QStringLiteral("path"), state.currentPath},
            {QStringLiteral("entryId"), state.cursorEntryId},
            {QStringLiteral("index"), sourceIndex},
            {QStringLiteral("catalogRevision"),
             QVariant::fromValue<qulonglong>(state.catalogRevision)},
        });
    requestOpen(side, state.cursorEntryId, sourceIndex, isImage,
                state.catalogRevision, true);
}

void F4GalleryBridge::reconcilePendingSelection(int side)
{
    if (!validSide(side)) {
        return;
    }
    PendingSelection &pending = m_pendingSelections[static_cast<size_t>(side)];
    if (!pending.active) {
        m_selectionActionPending[static_cast<size_t>(side)] = false;
        return;
    }

    const SideState &state = m_panelSessions.catalog(side);
    if (state.panelId != pending.panelId) {
        clearPendingSelection(side);
        return;
    }

    for (auto it = pending.desiredByEntryId.begin();
         it != pending.desiredByEntryId.end();) {
        if (!state.entryIds.contains(it.key())
            || state.selectedEntryIds.contains(it.key()) == it.value()) {
            it = pending.desiredByEntryId.erase(it);
        } else {
            ++it;
        }
    }
    if (pending.desiredByEntryId.isEmpty()) {
        clearPendingSelection(side);
        return;
    }

    if (state.catalogRevision == pending.catalogRevision
        && state.selectionRevision == pending.selectionRevision) {
        return;
    }

    // The original action raced a newer catalog or selection revision. Retry
    // only still-missing target states, always idempotently, so an accepted
    // mutation is never applied twice.
    const PendingCursor &pendingCursor =
        m_pendingCursors[static_cast<size_t>(side)];
    if (pendingCursor.active && pendingCursor.panelId == state.panelId
        && state.entryIds.contains(pendingCursor.entryId)) {
        QVariantList changes;
        changes.reserve(pending.desiredByEntryId.size());
        for (auto it = pending.desiredByEntryId.cbegin();
             it != pending.desiredByEntryId.cend(); ++it) {
            changes.push_back(QVariantMap{
                {QStringLiteral("entryId"), it.key()},
                {QStringLiteral("selected"), it.value()},
            });
        }
        m_selectionActionPending[static_cast<size_t>(side)] = false;
        requestSelectionTransaction(side, changes, pendingCursor.entryId,
                                    pendingCursor.index,
                                    state.catalogRevision);
        return;
    }

    pending.catalogRevision = state.catalogRevision;
    pending.selectionRevision = state.selectionRevision;
    m_selectionActionPending[static_cast<size_t>(side)] = false;
    QVariantList add;
    QVariantList remove;
    for (auto it = pending.desiredByEntryId.cbegin();
         it != pending.desiredByEntryId.cend(); ++it) {
        (it.value() ? add : remove).push_back(it.key());
    }
    if (!add.isEmpty()) {
        emitSelectionAction(side, QStringLiteral("add"), add,
                            pending.catalogRevision);
    }
    if (!remove.isEmpty()) {
        emitSelectionAction(side, QStringLiteral("remove"), remove,
                            pending.catalogRevision);
    }
}

void F4GalleryBridge::clearPendingSelection(int side)
{
    if (validSide(side)) {
        const size_t sideIndex = static_cast<size_t>(side);
        m_pendingSelections[sideIndex] = PendingSelection{};
        m_selectionActionPending[sideIndex] = false;
    }
}

void F4GalleryBridge::emitSelectionAction(int side, const QString &mode,
                                          const QVariantList &entryIds,
                                          qulonglong catalogRevision)
{
    if (!validSide(side)) {
        return;
    }
    PanelIntent intent;
    intent.kind = PanelIntent::Kind::SetSelection;
    intent.side = side;
    intent.mode = mode;
    intent.entryIds = entryIds;
    intent.catalogRevision = catalogRevision;
    intent.includeCatalogRevision = true;

    const size_t sideIndex = static_cast<size_t>(side);
    const PendingCursor &pendingCursor = m_pendingCursors[sideIndex];
    if (pendingCursor.active && !pendingCursor.entryId.isEmpty()
        && pendingCursor.panelId == m_panelSessions.catalog(side).panelId) {
        intent.cursorEntryId = pendingCursor.entryId;
        intent.cursorIndex = pendingCursor.index;
        intent.activate = !m_panelSessions.catalog(side).active;
    }

    const qulonglong selectionRevision = m_panelSessions.catalog(static_cast<int>(sideIndex)).selectionRevision;
    // Multiple Gallery selection gestures can reach Go before the semantic
    // scene acknowledging the first one returns. Only the first action is
    // guarded by the cached selection revision; later actions remain ordered
    // on the same IPC stream and omit that optional guard so they do not all
    // conflict with the first accepted mutation.
    if (selectionRevision != 0 && !m_selectionActionPending[sideIndex]) {
        intent.selectionRevision = selectionRevision;
    }
    m_selectionActionPending[sideIndex] = true;
    m_panelIntentController->dispatch(intent);
}

void F4GalleryBridge::reconcilePendingViewer(int side)
{
    if (!m_viewerCoordinator->pendingIntent().active || m_viewerCoordinator->pendingIntent().side != side || !validSide(side)) {
        return;
    }

    const SideState &state = m_panelSessions.catalog(side);
    if (!state.previewCapable || state.panelId != m_viewerCoordinator->pendingIntent().panelId
        || (m_viewerCoordinator->pendingIntent().catalogRevision != 0
            && state.catalogRevision != m_viewerCoordinator->pendingIntent().catalogRevision)) {
        clearPendingViewer();
        return;
    }

    // Opening from an inactive panel first activates that panel. Cursor and
    // activation acknowledgements can arrive in either order; retain the
    // stable viewer intent until both are authoritative.
    if (!state.active) {
        return;
    }

    if (state.cursorEntryId != m_viewerCoordinator->pendingIntent().entryId) {
        // A cursor request can still be in flight or can be retried after a
        // catalog revision advance. Never open a different image, but keep
        // the viewer intent until the stable cursor is confirmed or removed.
        const PendingCursor &pending =
            m_pendingCursors[static_cast<size_t>(side)];
        if (!pending.active || pending.entryId != m_viewerCoordinator->pendingIntent().entryId) {
            clearPendingViewer();
        }
        return;
    }

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_panelSessions.session(side));
    if (!session || !session->isImageAt(session->currentIndex())) {
        clearPendingViewer();
        return;
    }
    const int confirmedSide = m_viewerCoordinator->pendingIntent().side;
    clearPendingViewer();
    session->setViewerOpen(true);
    setViewer(confirmedSide, true);
}

void F4GalleryBridge::clearPendingViewer()
{
    m_viewerCoordinator->clearPending();
}

void F4GalleryBridge::setViewer(int side, bool visible)
{
    if (visible && validSide(side)) {
        m_viewerCoordinator->show(side);
    } else {
        m_viewerCoordinator->hide();
    }
}
