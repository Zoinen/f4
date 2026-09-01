#include "F4GalleryBridge.h"

#include "NavigationBenchmarkTrace.h"
#include "ViewerCoordinator.h"

struct F4GalleryBridge::SceneSyncContext
{
    QVariantMap scene;
    QVariant traceId;
    QVariantList panels;
    std::array<bool, 2> found = {false, false};
    bool traceInputScene = false;
    bool benchmarkRunning = false;
    bool genericInputTrace = false;
    qint64 inputBridgeBeginNs = 0;
};

F4GalleryBridge::SceneSyncContext F4GalleryBridge::makeSceneSyncContext(
    const QVariantMap &scene)
{
    SceneSyncContext context;
    context.scene = scene;
    context.traceId = F4NavigationBenchmarkTrace::enabled()
        ? F4NavigationBenchmarkTrace::benchmarkTraceId(scene) : QVariant();
    context.traceInputScene = context.traceId.isValid();
    context.benchmarkRunning = m_navigationBenchmark.enabled
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Finished
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Failed;
    context.genericInputTrace = context.traceInputScene
        && !context.benchmarkRunning;
    context.inputBridgeBeginNs = context.traceInputScene
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    context.panels = panelsFromScene(scene);
    return context;
}

void F4GalleryBridge::beginSceneSyncTrace(SceneSyncContext *context)
{
    if (!context) {
        return;
    }
    if (context->genericInputTrace) {
        m_lastInputSceneTraceId = context->traceId;
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.gallery.bridge.scene.begin"),
            context->inputBridgeBeginNs, context->traceId, {
                {QStringLiteral("sceneType"),
                 context->scene.value(QStringLiteral("type"))},
            });
    }
    if (!context->benchmarkRunning) {
        return;
    }

    if (context->traceId.isValid()) {
        m_navigationBenchmark.lastSceneTraceId = context->traceId;
    }
    const QVariant benchmarkValue = context->scene.value(
        QStringLiteral("benchmark"));
    m_navigationBenchmark.lastSceneBenchmark =
        benchmarkValue.metaType().id() == QMetaType::QVariantMap
        ? benchmarkValue.toMap() : QVariantMap{};
    if (!m_navigationBenchmark.benchmarkTraceId.isEmpty()
        && context->traceId.toString()
            == m_navigationBenchmark.benchmarkTraceId) {
        m_navigationBenchmark.sceneMatched = true;
        restartNavigationBenchmarkWatchdog();
    }
    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("sceneBenchmark"),
                  m_navigationBenchmark.lastSceneBenchmark);
    fields.insert(QStringLiteral("sceneType"),
                  context->scene.value(QStringLiteral("type")));
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.bridge.scene.begin"),
        context->traceId, fields);
}

void F4GalleryBridge::synchronizeScenePanels(SceneSyncContext *context)
{
    if (!context) {
        return;
    }
    for (const QVariant &panelValue : context->panels) {
        const QVariantMap panel = panelValue.toMap();
        const int side = panel.value(QStringLiteral("side")).toInt();
        if (!validSide(side)) {
            continue;
        }
        context->found[static_cast<size_t>(side)] = true;
        synchronizeScenePanel(context, side, panel);
    }
}

void F4GalleryBridge::synchronizeScenePanel(
    SceneSyncContext *context, int side, const QVariantMap &panel)
{
    if (canSkipUnchangedInactivePanel(side, panel)) {
        m_panelSnapshots[static_cast<size_t>(side)] = panel;
        traceSkippedScenePanel(*context, side, panel);
        return;
    }

    const qint64 panelSyncBeginNs = context->genericInputTrace
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    if (context->genericInputTrace) {
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.gallery.bridge.panel.begin"),
            panelSyncBeginNs, context->traceId, {
                {QStringLiteral("side"), side},
                {QStringLiteral("path"),
                 panel.value(QStringLiteral("path"))},
                {QStringLiteral("entries"),
                 panel.value(QStringLiteral("entries")).toList().size()},
            });
    }
    synchronizePanel(side, panel);
    if (!context->genericInputTrace) {
        return;
    }
    const qint64 panelSyncEndNs =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.gallery.bridge.panel.end"), panelSyncEndNs,
        context->traceId, {
            {QStringLiteral("side"), side},
            {QStringLiteral("durationNs"),
             panelSyncEndNs - panelSyncBeginNs},
        });
}

void F4GalleryBridge::traceSkippedScenePanel(
    const SceneSyncContext &context, int side, const QVariantMap &panel)
{
    if (context.benchmarkRunning) {
        QVariantMap fields = navigationBenchmarkFields();
        fields.insert(QStringLiteral("syncSide"), side);
        fields.insert(QStringLiteral("syncPath"),
                      panel.value(QStringLiteral("path")));
        fields.insert(QStringLiteral("syncLoading"),
                      panel.value(QStringLiteral("loading")));
        fields.insert(QStringLiteral("syncCatalogRevision"),
                      panel.value(QStringLiteral("catalogRevision")));
        queueNavigationBenchmarkTrace(
            QStringLiteral("qt.gallery.bridge.panel.skipped"),
            context.traceId, fields);
    }
    if (context.genericInputTrace) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.gallery.bridge.panel.skipped"),
            context.traceId, {
                {QStringLiteral("side"), side},
                {QStringLiteral("path"),
                 panel.value(QStringLiteral("path"))},
            });
    }
}

void F4GalleryBridge::reconcileSceneOwnedState(
    const SceneSyncContext &context)
{
    if (viewerVisible()
        && (!validSide(viewerSide())
            || !context.found[static_cast<size_t>(viewerSide())]
            || !m_panelSessions.catalog(viewerSide()).previewCapable
            || !m_panelSessions.catalog(viewerSide()).active)) {
        closeViewer();
    }
    const auto &viewerIntent = m_viewerCoordinator->pendingIntent();
    if (viewerIntent.active
        && (!validSide(viewerIntent.side)
            || !context.found[static_cast<size_t>(viewerIntent.side)])) {
        clearPendingViewer();
    }
    if (m_pendingPanelOpen.active
        && (!validSide(m_pendingPanelOpen.side)
            || !context.found[static_cast<size_t>(m_pendingPanelOpen.side)])) {
        clearPendingPanelOpen();
    }
    if (m_inFlightPanelOpen.active
        && (!validSide(m_inFlightPanelOpen.side)
            || !context.found[static_cast<size_t>(m_inFlightPanelOpen.side)])) {
        clearInFlightPanelOpen();
    }
    for (int side = 0; side < 2; ++side) {
        if (context.found[static_cast<size_t>(side)]) {
            continue;
        }
        m_panelSnapshots[static_cast<size_t>(side)].clear();
        clearPendingCursor(side);
        clearPendingSelection(side);
    }
}

void F4GalleryBridge::finishBenchmarkSceneTrace(
    const SceneSyncContext &context)
{
    if (!context.benchmarkRunning) {
        return;
    }
    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("sceneBenchmark"),
                  m_navigationBenchmark.lastSceneBenchmark);
    fields.insert(QStringLiteral("panelCount"), context.panels.size());
    if (validSide(m_navigationBenchmark.side)) {
        const SideState &state = m_panelSessions.catalog(
            m_navigationBenchmark.side);
        fields.insert(QStringLiteral("panelPath"), state.currentPath);
        fields.insert(QStringLiteral("panelLoading"), state.loading);
        fields.insert(QStringLiteral("panelCatalogRevision"),
                      QVariant::fromValue<qulonglong>(state.catalogRevision));
        fields.insert(QStringLiteral("panelCursorEntryId"),
                      state.cursorEntryId);
        fields.insert(QStringLiteral("panelCursorIndex"), state.cursorIndex);
        fields.insert(QStringLiteral("panelLayoutMode"),
                      state.galleryLayoutMode);
        fields.insert(QStringLiteral("panelEntryCount"),
                      catalogEntryCount(state));
    }
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.bridge.scene.end"),
        context.traceId, fields);
    scheduleNavigationBenchmarkAdvance();
}

void F4GalleryBridge::finishGenericSceneTrace(SceneSyncContext *context)
{
    if (!context || !context->genericInputTrace) {
        return;
    }
    const qint64 inputBridgeEndNs =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();
    F4NavigationBenchmarkTrace::eventAt(
        QStringLiteral("qt.gallery.bridge.scene.end"), inputBridgeEndNs,
        context->traceId, {
            {QStringLiteral("durationNs"),
             inputBridgeEndNs - context->inputBridgeBeginNs},
            {QStringLiteral("panelCount"), context->panels.size()},
        });

    if (m_pendingInputFrameTraceId.isValid()) {
        ++m_inputScenesSupersededBeforeFrame;
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.input.frame.superseded"),
            m_pendingInputFrameTraceId, {
                {QStringLiteral("replacedByTraceId"),
                 context->traceId.toString()},
                {QStringLiteral("sceneAgeNs"),
                 inputBridgeEndNs - m_pendingInputFrameSceneEndNs},
            });
    }
    m_pendingInputFrameTraceId = context->traceId;
    m_pendingInputFrameSceneEndNs = inputBridgeEndNs;
    m_pendingInputFrameRequiredRenderSyncSerial =
        m_renderSyncSerial.load(std::memory_order_acquire) + 1;
}

void F4GalleryBridge::synchronizeScene(const QVariantMap &scene)
{
    SceneSyncContext context = makeSceneSyncContext(scene);
    beginSceneSyncTrace(&context);
    synchronizeScenePanels(&context);
    reconcileSceneOwnedState(context);
    finishBenchmarkSceneTrace(context);
    finishGenericSceneTrace(&context);
    schedulePanelCatalogMetadataRequest();
}
