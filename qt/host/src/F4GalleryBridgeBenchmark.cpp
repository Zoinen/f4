#include "F4GalleryBridge.h"
#include "NavigationBenchmarkTrace.h"

#include <QCoreApplication>
#include <QDir>
#include <QFileInfo>
#include <QTimer>

#include <ZoinGallery/GallerySession.h>

#include <algorithm>

namespace
{
constexpr int NavigationBenchmarkWatchdogMs = 15000;
constexpr int CatalogRowsPageSize = 64;

int benchmarkEnvironmentInteger(const char *name, int fallback, int minimum,
                                int maximum)
{
    bool ok = false;
    const int value = qEnvironmentVariableIntValue(name, &ok);
    return ok ? qBound(minimum, value, maximum) : fallback;
}

bool benchmarkEnvironmentFlag(const char *name)
{
    if (!qEnvironmentVariableIsSet(name)) {
        return false;
    }
    const QString value = qEnvironmentVariable(name).trimmed().toLower();
    return value.isEmpty() || value == QStringLiteral("1")
        || value == QStringLiteral("true") || value == QStringLiteral("yes")
        || value == QStringLiteral("on");
}

QString normalizedBenchmarkPath(const QString &path)
{
    if (path.trimmed().isEmpty()) {
        return {};
    }
    return QDir::cleanPath(QFileInfo(path.trimmed()).absoluteFilePath());
}
}
void F4GalleryBridge::configureNavigationBenchmark()
{
    const QString requestedTarget =
        qEnvironmentVariable("F4_NAV_BENCHMARK_TARGET").trimmed();
    if (requestedTarget.isEmpty()) {
        return;
    }

    const QFileInfo requestedInfo(requestedTarget);
    const QString targetPath = normalizedBenchmarkPath(requestedTarget);
    const QFileInfo targetInfo(targetPath);
    const QString parentPath = normalizedBenchmarkPath(targetInfo.absolutePath());
    if (!requestedInfo.isAbsolute() || targetPath.isEmpty()
        || parentPath.isEmpty() || targetInfo.fileName().isEmpty()
        || parentPath == targetPath) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.gallery.runner.invalid-config"), {}, {
                {QStringLiteral("requestedTarget"), requestedTarget},
                {QStringLiteral("normalizedTarget"), targetPath},
                {QStringLiteral("normalizedParent"), parentPath},
            });
        return;
    }

    m_navigationBenchmark.enabled = true;
    m_navigationBenchmark.exitWhenFinished =
        benchmarkEnvironmentFlag("F4_NAV_BENCHMARK_EXIT");
    m_navigationBenchmark.phase = NavigationBenchmarkPhase::WaitingForPanel;
    m_navigationBenchmark.targetPath = targetPath;
    m_navigationBenchmark.parentPath = parentPath;
    m_navigationBenchmark.targetName = targetInfo.fileName();
    m_navigationBenchmark.cycles = benchmarkEnvironmentInteger(
        "F4_NAV_BENCHMARK_CYCLES", 50, 1, 10000);
    m_navigationBenchmark.warmup = benchmarkEnvironmentInteger(
        "F4_NAV_BENCHMARK_WARMUP", 10, 0, 10000);
    m_navigationBenchmark.runId = QStringLiteral("qt-%1-nav")
        .arg(QCoreApplication::applicationPid());

    auto *watchdog = new QTimer(this);
    watchdog->setSingleShot(true);
    watchdog->setInterval(NavigationBenchmarkWatchdogMs);
    connect(watchdog, &QTimer::timeout, this, [this]() {
        failNavigationBenchmark(QStringLiteral("stage-timeout"));
    });
    m_navigationBenchmarkWatchdog = watchdog;
    restartNavigationBenchmarkWatchdog();

    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("traceEnabled"),
                  F4NavigationBenchmarkTrace::enabled());
    F4NavigationBenchmarkTrace::event(
        QStringLiteral("qt.gallery.runner.configured"),
        m_navigationBenchmark.runId, fields);
}

QVariantMap F4GalleryBridge::navigationBenchmarkFields() const
{
    const NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    QString phase;
    switch (benchmark.phase) {
    case NavigationBenchmarkPhase::Disabled:
        phase = QStringLiteral("disabled");
        break;
    case NavigationBenchmarkPhase::WaitingForPanel:
        phase = QStringLiteral("waiting-for-panel");
        break;
    case NavigationBenchmarkPhase::SettingDetails:
        phase = QStringLiteral("setting-details");
        break;
    case NavigationBenchmarkPhase::NavigatingToTargetForSetup:
        phase = QStringLiteral("setup-target");
        break;
    case NavigationBenchmarkPhase::ReturningToParentForSetup:
        phase = QStringLiteral("setup-parent");
        break;
    case NavigationBenchmarkPhase::WaitingForSetupReadiness:
        phase = QStringLiteral("setup-readiness");
        break;
    case NavigationBenchmarkPhase::WaitingForSetupFrame:
        phase = QStringLiteral("setup-frame");
        break;
    case NavigationBenchmarkPhase::ReadyToDispatch:
        phase = QStringLiteral("ready-to-dispatch");
        break;
    case NavigationBenchmarkPhase::WaitingForTransitionReadiness:
        phase = QStringLiteral("transition-readiness");
        break;
    case NavigationBenchmarkPhase::WaitingForTransitionFrame:
        phase = QStringLiteral("transition-frame");
        break;
    case NavigationBenchmarkPhase::Finished:
        phase = QStringLiteral("finished");
        break;
    case NavigationBenchmarkPhase::Failed:
        phase = QStringLiteral("failed");
        break;
    }

    return {
        {QStringLiteral("runId"), benchmark.runId},
        {QStringLiteral("runnerPhase"), phase},
        {QStringLiteral("actionPhase"), benchmark.actionPhase},
        {QStringLiteral("side"), benchmark.side},
        {QStringLiteral("targetPath"), benchmark.targetPath},
        {QStringLiteral("parentPath"), benchmark.parentPath},
        {QStringLiteral("fromPath"), benchmark.fromPath},
        {QStringLiteral("toPath"), benchmark.expectedPath},
        {QStringLiteral("direction"), benchmark.direction},
        {QStringLiteral("cycle"), benchmark.completedCycles},
        {QStringLiteral("measuredCycle"),
         qMax(0, benchmark.completedCycles - benchmark.warmup)},
        {QStringLiteral("transition"), benchmark.completedTransitions},
        {QStringLiteral("warmup"),
         benchmark.completedCycles < benchmark.warmup},
        {QStringLiteral("warmupCycles"), benchmark.warmup},
        {QStringLiteral("measuredCycles"), benchmark.cycles},
        {QStringLiteral("phaseSequence"),
         QVariant::fromValue<qulonglong>(benchmark.phaseSequence)},
        {QStringLiteral("actionSequence"),
         QVariant::fromValue<qulonglong>(benchmark.actionSequence)},
        {QStringLiteral("frameSerial"),
         QVariant::fromValue<qulonglong>(benchmark.frameSerial)},
        {QStringLiteral("requiredFrameSerial"),
         QVariant::fromValue<qulonglong>(benchmark.requiredFrameSerial)},
        {QStringLiteral("sceneMatched"), benchmark.sceneMatched},
        {QStringLiteral("placementReady"), benchmark.placementReady},
        {QStringLiteral("placementPath"), benchmark.placementPath},
        {QStringLiteral("placementCatalogRevision"),
         QVariant::fromValue<qulonglong>(
             benchmark.placementCatalogRevision)},
    };
}

void F4GalleryBridge::sendNavigationBenchmarkAction(
    QVariantMap action, const QString &phase, const QString &direction,
    const QString &fromPath, const QString &toPath)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    ++benchmark.actionSequence;
    ++benchmark.phaseSequence;
    benchmark.benchmarkTraceId = QStringLiteral("%1-%2")
        .arg(benchmark.runId)
        .arg(benchmark.actionSequence);
    benchmark.actionPhase = phase;
    benchmark.direction = direction;
    benchmark.fromPath = fromPath;
    benchmark.expectedPath = toPath;
    benchmark.actionSent = true;
    benchmark.sceneMatched = false;
    benchmark.placementReady = false;
    benchmark.placementPath.clear();
    benchmark.placementCatalogRevision = 0;
    benchmark.lastPlacement.clear();
    benchmark.requiredFrameSerial = 0;

    QVariantMap metadata = {
        {QStringLiteral("schema"), 1},
        {QStringLiteral("benchmarkTraceId"),
         benchmark.benchmarkTraceId},
        {QStringLiteral("phase"), phase},
        {QStringLiteral("phaseSequence"),
         QVariant::fromValue<qulonglong>(benchmark.phaseSequence)},
        {QStringLiteral("side"), benchmark.side},
        {QStringLiteral("fromPath"), fromPath},
        {QStringLiteral("toPath"), toPath},
        {QStringLiteral("direction"), direction},
        {QStringLiteral("cycle"), benchmark.completedCycles},
        {QStringLiteral("transition"), benchmark.completedTransitions},
        {QStringLiteral("warmup"),
         benchmark.completedCycles < benchmark.warmup},
    };
    action.insert(QStringLiteral("benchmarkTraceId"),
                  benchmark.benchmarkTraceId);
    action.insert(QStringLiteral("benchmark"), metadata);

    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("action"),
                  action.value(QStringLiteral("action")));
    fields.insert(QStringLiteral("entryId"),
                  action.value(QStringLiteral("entryId")));
    fields.insert(QStringLiteral("index"),
                  action.value(QStringLiteral("index")));
    const qint64 actionBoundary =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();
    restartNavigationBenchmarkWatchdog();
    if (action.value(QStringLiteral("action")).toString()
        == QStringLiteral("panel.open")) {
        const int actionSide = action.value(
            QStringLiteral("side"), benchmark.side).toInt();
        markPanelOpenInFlight(
            actionSide,
            action.value(QStringLiteral("entryId")).toString());
    }
    emit uiActionRequested(action);
    m_pendingNavigationBenchmarkTrace.push_back({
        QStringLiteral("qt.gallery.runner.action"), actionBoundary,
        benchmark.benchmarkTraceId, fields});
}

QVariantMap F4GalleryBridge::navigationBenchmarkEntryForPath(
    int side, const QString &path) const
{
    if (!validSide(side)) {
        return {};
    }
    const QString normalizedPath = normalizedBenchmarkPath(path);
    const SideState &state = m_panelSessions.catalog(side);
    QVariantMap nameFallback;
    for (const QVariant &value : state.entries) {
        const QVariantMap entry = value.toMap();
        if (entry.value(QStringLiteral("isUp")).toBool()) {
            continue;
        }
        const QString localPath = normalizedBenchmarkPath(
            entry.value(QStringLiteral("localPath")).toString());
        if (!localPath.isEmpty() && localPath == normalizedPath) {
            return entry;
        }
        if (entry.value(QStringLiteral("name")).toString()
            == QFileInfo(normalizedPath).fileName()) {
            nameFallback = entry;
        }
    }
    return nameFallback;
}

QVariantMap F4GalleryBridge::navigationBenchmarkUpEntry(int side) const
{
    if (!validSide(side)) {
        return {};
    }
    const SideState &state = m_panelSessions.catalog(side);
    for (const QVariant &value : state.entries) {
        const QVariantMap entry = value.toMap();
        if (entry.value(QStringLiteral("isUp")).toBool()
            || entry.value(QStringLiteral("name")).toString()
                == QStringLiteral("..")) {
            return entry;
        }
    }
    return {};
}

void F4GalleryBridge::queueNavigationBenchmarkTrace(
    const QString &name, const QVariant &benchmarkTraceId,
    const QVariantMap &fields)
{
    if (!F4NavigationBenchmarkTrace::enabled()) {
        return;
    }
    m_pendingNavigationBenchmarkTrace.push_back({
        name, F4NavigationBenchmarkTrace::monotonicNanoseconds(),
        benchmarkTraceId, fields});
}

void F4GalleryBridge::flushNavigationBenchmarkTrace()
{
    const QList<PendingNavigationBenchmarkTrace> pending =
        std::exchange(m_pendingNavigationBenchmarkTrace,
                      QList<PendingNavigationBenchmarkTrace>{});
    for (const PendingNavigationBenchmarkTrace &event : pending) {
        F4NavigationBenchmarkTrace::eventAt(
            event.name, event.monotonicNs, event.benchmarkTraceId,
            event.fields);
    }
}

void F4GalleryBridge::recordBenchmarkStage(
    int side, const QString &stage, const QVariantMap &metadata)
{
    if (recordPassiveBenchmarkStage(side, stage, metadata)) {
        return;
    }
    if (!m_navigationBenchmark.enabled || !validSide(side)
        || m_navigationBenchmark.phase
            == NavigationBenchmarkPhase::Finished
        || m_navigationBenchmark.phase
            == NavigationBenchmarkPhase::Failed) {
        return;
    }

    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    const QVariant traceId = benchmark.lastSceneTraceId.isValid()
        ? benchmark.lastSceneTraceId
        : QVariant(benchmark.benchmarkTraceId);
    queueActiveBenchmarkStage(side, stage, metadata, traceId);
    updateNavigationBenchmarkPlacement(side, stage, metadata, traceId);
}

bool F4GalleryBridge::recordPassiveBenchmarkStage(
    int side, const QString &stage, const QVariantMap &metadata)
{
    if (!F4NavigationBenchmarkTrace::enabled()
        || m_navigationBenchmark.enabled || !validSide(side)) {
        return false;
    }
    QVariantMap fields = metadata;
    const SideState &state = m_panelSessions.catalog(side);
    fields.insert(QStringLiteral("stage"), stage);
    fields.insert(QStringLiteral("side"), side);
    fields.insert(QStringLiteral("bridgePanelLoading"), state.loading);
    fields.insert(QStringLiteral("bridgePanelPath"), state.currentPath);
    fields.insert(QStringLiteral("bridgeCatalogRevision"),
                  QVariant::fromValue<qulonglong>(state.catalogRevision));
    F4NavigationBenchmarkTrace::event(
        QStringLiteral("qt.gallery.qml.%1").arg(stage),
        m_lastInputSceneTraceId, fields);
    return true;
}

void F4GalleryBridge::queueActiveBenchmarkStage(
    int side, const QString &stage, const QVariantMap &metadata,
    const QVariant &traceId)
{
    const NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    QVariantMap fields = navigationBenchmarkFields();
    for (auto it = metadata.cbegin(); it != metadata.cend(); ++it) {
        fields.insert(it.key(), it.value());
    }
    fields.insert(QStringLiteral("stage"), stage);
    fields.insert(QStringLiteral("bridgePanelLoading"),
                  m_panelSessions.catalog(side).loading);
    fields.insert(QStringLiteral("bridgePanelPath"),
                  m_panelSessions.catalog(side).currentPath);
    fields.insert(QStringLiteral("sceneBenchmark"),
                  benchmark.lastSceneBenchmark);
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.qml.%1").arg(stage), traceId, fields);
}

void F4GalleryBridge::updateNavigationBenchmarkPlacement(
    int side, const QString &stage, const QVariantMap &metadata,
    const QVariant &traceId)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    const bool waitingForPlacement =
        benchmark.phase == NavigationBenchmarkPhase::WaitingForSetupReadiness
        || benchmark.phase
            == NavigationBenchmarkPhase::WaitingForTransitionReadiness;
    const bool traceMatches = !benchmark.benchmarkTraceId.isEmpty()
        && traceId.toString() == benchmark.benchmarkTraceId;
    if (!waitingForPlacement || side != benchmark.side || !traceMatches) {
        return;
    }
    QString path = metadata.value(QStringLiteral("currentPath")).toString();
    if (path.isEmpty()) {
        path = metadata.value(QStringLiteral("hostPanelPath")).toString();
    }
    path = normalizedBenchmarkPath(path);
    const qulonglong catalogRevision = metadata.value(
        QStringLiteral("catalogRevision")).toULongLong();
    const QString presentationMode = metadata.value(
        QStringLiteral("presentationMode"),
        metadata.value(QStringLiteral("hostPresentationMode"))).toString();
    const bool placementPending = metadata.value(
        QStringLiteral("pathViewportPlacementPending"), true).toBool();
    const int count = metadata.value(QStringLiteral("count"), -1).toInt();
    const bool geometryValid = metadata.value(
        QStringLiteral("geometryValid")).toBool();
    const bool placementMatchesTarget = metadata.value(
        QStringLiteral("placementMatchesTarget")).toBool();
    const qreal viewportExtent = metadata.value(
        QStringLiteral("viewportExtent")).toReal();
    const qreal contentOffset = metadata.value(
        QStringLiteral("contentY")).toReal();
    const qreal cursorOffset = presentationMode == QStringLiteral("columns")
        ? metadata.value(QStringLiteral("cursorX")).toReal()
        : metadata.value(QStringLiteral("cursorY")).toReal();
    const qreal cursorExtent = presentationMode == QStringLiteral("columns")
        ? metadata.value(QStringLiteral("cursorWidth")).toReal()
        : metadata.value(QStringLiteral("cursorHeight")).toReal();
    const bool cursorFullyVisible = geometryValid && viewportExtent > 0
        && cursorOffset >= contentOffset - 0.51
        && cursorOffset + cursorExtent
            <= contentOffset + viewportExtent + 0.51;
    if (path == benchmark.expectedPath
        && catalogRevision != benchmark.placementCatalogRevision
        && (stage == QStringLiteral("session.catalog.changed")
            || stage == QStringLiteral("layout.reset")
            || stage == QStringLiteral("host.panel.changed"))) {
        benchmark.placementReady = false;
    }
    if (path == benchmark.expectedPath
        && presentationMode == QStringLiteral("details")
        && !placementPending
        && (placementMatchesTarget || cursorFullyVisible)
        && (count == 0 || geometryValid)) {
        benchmark.placementReady = true;
        benchmark.placementPath = path;
        benchmark.placementCatalogRevision = catalogRevision;
        benchmark.lastPlacement = metadata;
        restartNavigationBenchmarkWatchdog();
        scheduleNavigationBenchmarkAdvance();
    }
}

void F4GalleryBridge::scheduleNavigationBenchmarkAdvance()
{
    if (!m_navigationBenchmark.enabled) {
        return;
    }
    QTimer::singleShot(0, this,
                       [this]() { advanceNavigationBenchmark(); });
}

void F4GalleryBridge::restartNavigationBenchmarkWatchdog()
{
    if (m_navigationBenchmarkWatchdog
        && m_navigationBenchmark.enabled
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Finished
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Failed) {
        m_navigationBenchmarkWatchdog->start();
    }
}

void F4GalleryBridge::armNavigationBenchmarkFrame(bool setup)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    benchmark.requiredFrameSerial = benchmark.frameSerial + 1;
    benchmark.phase = setup
        ? NavigationBenchmarkPhase::WaitingForSetupFrame
        : NavigationBenchmarkPhase::WaitingForTransitionFrame;
    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("lastPlacement"),
                  benchmark.lastPlacement);
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.runner.frame-armed"),
        benchmark.benchmarkTraceId, fields);
    restartNavigationBenchmarkWatchdog();
}

void F4GalleryBridge::completeNavigationBenchmarkFrame()
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    const bool setup = benchmark.phase
        == NavigationBenchmarkPhase::WaitingForSetupFrame;
    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("lastPlacement"),
                  benchmark.lastPlacement);
    queueNavigationBenchmarkTrace(
        setup ? QStringLiteral("qt.gallery.runner.setup-ready")
              : QStringLiteral("qt.gallery.runner.transition-complete"),
        benchmark.benchmarkTraceId, fields);

    benchmark.requiredFrameSerial = 0;
    benchmark.actionSent = false;
    if (setup) {
        benchmark.phase = NavigationBenchmarkPhase::ReadyToDispatch;
    } else {
        ++benchmark.completedTransitions;
        if (benchmark.nextTransitionEnters) {
            benchmark.nextTransitionEnters = false;
        } else {
            benchmark.nextTransitionEnters = true;
            ++benchmark.completedCycles;
        }
        benchmark.phase = NavigationBenchmarkPhase::ReadyToDispatch;
    }
    if (m_navigationBenchmarkWatchdog) {
        m_navigationBenchmarkWatchdog->stop();
    }
    // Trace output is intentionally outside action-to-frame measurement. The
    // next transition is dispatched on a later event-loop turn.
    flushNavigationBenchmarkTrace();
    scheduleNavigationBenchmarkAdvance();
}

void F4GalleryBridge::finishNavigationBenchmark()
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    benchmark.phase = NavigationBenchmarkPhase::Finished;
    if (m_navigationBenchmarkWatchdog) {
        m_navigationBenchmarkWatchdog->stop();
    }
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.runner.finished"),
        benchmark.benchmarkTraceId, navigationBenchmarkFields());
    flushNavigationBenchmarkTrace();
    if (benchmark.exitWhenFinished) {
        QTimer::singleShot(0, QCoreApplication::instance(), []() {
            QCoreApplication::exit(0);
        });
    }
}

void F4GalleryBridge::failNavigationBenchmark(
    const QString &reason, const QVariantMap &extraFields)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    if (!benchmark.enabled
        || benchmark.phase == NavigationBenchmarkPhase::Finished
        || benchmark.phase == NavigationBenchmarkPhase::Failed) {
        return;
    }
    benchmark.phase = NavigationBenchmarkPhase::Failed;
    if (m_navigationBenchmarkWatchdog) {
        m_navigationBenchmarkWatchdog->stop();
    }
    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("reason"), reason);
    for (auto it = extraFields.cbegin(); it != extraFields.cend(); ++it) {
        fields.insert(it.key(), it.value());
    }
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.runner.failed"),
        benchmark.benchmarkTraceId, fields);
    flushNavigationBenchmarkTrace();
    if (benchmark.exitWhenFinished) {
        QTimer::singleShot(0, QCoreApplication::instance(), []() {
            QCoreApplication::exit(4);
        });
    }
}

void F4GalleryBridge::advanceNavigationBenchmark()
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    if (!benchmark.enabled
        || benchmark.phase == NavigationBenchmarkPhase::Disabled
        || benchmark.phase == NavigationBenchmarkPhase::Finished
        || benchmark.phase == NavigationBenchmarkPhase::Failed) {
        return;
    }

    // Panel state can arrive while the root window is still hidden and QML is
    // compiling its first scene. Dispatching setup navigation there folds
    // unrelated startup work into the cold-folder result and races the first
    // retained-surface frame. Start only after one real swapped frame.
    if (benchmark.frameSerial == 0) {
        return;
    }

    if (!selectNavigationBenchmarkSide()) {
        return;
    }

    const SideState &state =
        m_panelSessions.catalog(benchmark.side);
    switch (benchmark.phase) {
    case NavigationBenchmarkPhase::WaitingForPanel:
        advanceBenchmarkWaitingForPanel(state);
        return;

    case NavigationBenchmarkPhase::SettingDetails:
        advanceBenchmarkSettingDetails(state);
        return;

    case NavigationBenchmarkPhase::NavigatingToTargetForSetup:
        advanceBenchmarkTargetSetup(state);
        return;

    case NavigationBenchmarkPhase::ReturningToParentForSetup:
        advanceBenchmarkParentSetup(state);
        return;

    case NavigationBenchmarkPhase::WaitingForSetupReadiness: {
        advanceBenchmarkSetupReadiness(state);
        return;
    }

    case NavigationBenchmarkPhase::WaitingForSetupFrame:
        return;

    case NavigationBenchmarkPhase::ReadyToDispatch: {
        advanceBenchmarkReadyToDispatch(state);
        return;
    }

    case NavigationBenchmarkPhase::WaitingForTransitionReadiness: {
        advanceBenchmarkTransitionReadiness(state);
        return;
    }

    case NavigationBenchmarkPhase::WaitingForTransitionFrame:
    case NavigationBenchmarkPhase::Finished:
    case NavigationBenchmarkPhase::Failed:
    case NavigationBenchmarkPhase::Disabled:
        return;
    }
}

bool F4GalleryBridge::selectNavigationBenchmarkSide()
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    if (validSide(benchmark.side)) {
        return true;
    }
    for (int side = 0; side < 2; ++side) {
        const SideState &candidate = m_panelSessions.catalog(side);
        if (candidate.initialized && candidate.active) {
            benchmark.side = side;
            break;
        }
    }
    if (!validSide(benchmark.side)) {
        return false;
    }
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.runner.side-selected"),
        benchmark.runId, navigationBenchmarkFields());
    return true;
}

void F4GalleryBridge::advanceBenchmarkWaitingForPanel(
    const SideState &state)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    if (state.galleryLayoutMode != QStringLiteral("details")) {
        benchmark.phase = NavigationBenchmarkPhase::SettingDetails;
        sendNavigationBenchmarkAction({
            {QStringLiteral("action"),
             QStringLiteral("panel.setGalleryLayout")},
            {QStringLiteral("side"), benchmark.side},
            {QStringLiteral("layoutMode"), QStringLiteral("details")},
        }, QStringLiteral("setup"), QStringLiteral("details"),
           state.currentPath, state.currentPath);
        return;
    }
    benchmark.phase = benchmark.warmup == 0
        ? NavigationBenchmarkPhase::ReturningToParentForSetup
        : NavigationBenchmarkPhase::NavigatingToTargetForSetup;
    benchmark.actionSent = false;
    scheduleNavigationBenchmarkAdvance();
}

void F4GalleryBridge::advanceBenchmarkSettingDetails(
    const SideState &state)
{
    if (state.galleryLayoutMode != QStringLiteral("details")) {
        return;
    }
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    benchmark.phase = benchmark.warmup == 0
        ? NavigationBenchmarkPhase::ReturningToParentForSetup
        : NavigationBenchmarkPhase::NavigatingToTargetForSetup;
    benchmark.actionSent = false;
    scheduleNavigationBenchmarkAdvance();
}

void F4GalleryBridge::advanceBenchmarkTargetSetup(const SideState &state)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    const bool targetReady = normalizedBenchmarkPath(state.currentPath)
            == benchmark.targetPath
        && !state.loading && !state.catalogProvisional;
    if (!benchmark.actionSent) {
        if (targetReady) {
            benchmark.phase =
                NavigationBenchmarkPhase::ReturningToParentForSetup;
            scheduleNavigationBenchmarkAdvance();
            return;
        }
        sendNavigationBenchmarkAction({
            {QStringLiteral("action"), QStringLiteral("panel.navigatePath")},
            {QStringLiteral("side"), benchmark.side},
            {QStringLiteral("path"), benchmark.targetPath},
        }, QStringLiteral("setup"), QStringLiteral("setup-target"),
           state.currentPath, benchmark.targetPath);
        return;
    }
    if (!targetReady) {
        return;
    }
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.runner.setup-target-ready"),
        benchmark.benchmarkTraceId, navigationBenchmarkFields());
    benchmark.phase = NavigationBenchmarkPhase::ReturningToParentForSetup;
    benchmark.actionSent = false;
    scheduleNavigationBenchmarkAdvance();
}

void F4GalleryBridge::advanceBenchmarkParentSetup(const SideState &state)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    benchmark.phase = NavigationBenchmarkPhase::WaitingForSetupReadiness;
    sendNavigationBenchmarkAction({
        {QStringLiteral("action"), QStringLiteral("panel.navigatePath")},
        {QStringLiteral("side"), benchmark.side},
        {QStringLiteral("path"), benchmark.parentPath},
    }, QStringLiteral("setup"), QStringLiteral("setup-parent"),
       state.currentPath, benchmark.parentPath);
}

bool F4GalleryBridge::materializeBenchmarkSetupTarget(
    const SideState &state, QVariantMap &targetEntry)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    targetEntry = navigationBenchmarkEntryForPath(
        benchmark.side, benchmark.targetPath);
    if (!targetEntry.isEmpty() || !state.catalogRowsDeferred
        || state.catalogRowsRequestInFlight) {
        return !targetEntry.isEmpty();
    }
    SideState &mutableState = m_panelSessions.catalog(benchmark.side);
    int missing = 0;
    while (missing < mutableState.totalCount
           && catalogRowLoaded(mutableState, missing)) {
        ++missing;
    }
    if (missing < mutableState.totalCount) {
        mutableState.catalogRowsVisibleFirst = missing;
        mutableState.catalogRowsVisibleLast = qMin(
            mutableState.totalCount - 1,
            missing + CatalogRowsPageSize - 1);
        schedulePanelCatalogRowsRequest(benchmark.side);
        return false;
    }
    failNavigationBenchmark(
        QStringLiteral("navigation-entry-missing"), {
            {QStringLiteral("actualPath"), state.currentPath},
            {QStringLiteral("direction"), QStringLiteral("setup-cursor")},
        });
    return false;
}

bool F4GalleryBridge::selectBenchmarkSetupTarget(
    const SideState &state, const QVariantMap &targetEntry)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    const QString entryId = targetEntry.value(
        QStringLiteral("entryId")).toString();
    if (state.cursorEntryId == entryId) {
        return true;
    }
    if (benchmark.actionSent
        && benchmark.direction == QStringLiteral("setup-cursor")) {
        return false;
    }
    const QVariant index = targetEntry.value(QStringLiteral("index"));
    benchmark.actionSent = true;
    benchmark.direction = QStringLiteral("setup-cursor");
    restartNavigationBenchmarkWatchdog();
    emit uiActionRequested({
        {QStringLiteral("action"), QStringLiteral("panel.cursor")},
        {QStringLiteral("side"), benchmark.side},
        {QStringLiteral("entryId"), entryId},
        {QStringLiteral("index"), index},
        {QStringLiteral("catalogRevision"),
         QVariant::fromValue<qulonglong>(state.catalogRevision)},
        {QStringLiteral("benchmarkTraceId"), benchmark.benchmarkTraceId},
    });
    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("action"), QStringLiteral("panel.cursor"));
    fields.insert(QStringLiteral("entryId"), entryId);
    fields.insert(QStringLiteral("index"), index);
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.runner.setup-cursor"),
        benchmark.benchmarkTraceId, fields);
    return false;
}

void F4GalleryBridge::advanceBenchmarkSetupReadiness(
    const SideState &state)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    if (normalizedBenchmarkPath(state.currentPath) != benchmark.parentPath
        || state.loading || state.catalogProvisional) {
        return;
    }
    QVariantMap targetEntry;
    if (!materializeBenchmarkSetupTarget(state, targetEntry)
        || !selectBenchmarkSetupTarget(state, targetEntry)) {
        return;
    }
    const bool sceneReady = benchmark.sceneMatched
        && state.galleryLayoutMode == QStringLiteral("details");
    const bool placementReady = benchmark.placementReady
        && benchmark.placementPath == benchmark.parentPath
        && benchmark.placementCatalogRevision == state.catalogRevision;
    if (sceneReady && placementReady) {
        armNavigationBenchmarkFrame(true);
    }
}

void F4GalleryBridge::advanceBenchmarkReadyToDispatch(
    const SideState &state)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    if (benchmark.completedCycles >= benchmark.warmup + benchmark.cycles) {
        finishNavigationBenchmark();
        return;
    }
    const bool entering = benchmark.nextTransitionEnters;
    const QString expectedSource = entering
        ? benchmark.parentPath : benchmark.targetPath;
    const QString destination = entering
        ? benchmark.targetPath : benchmark.parentPath;
    if (normalizedBenchmarkPath(state.currentPath) != expectedSource
        || state.loading) {
        failNavigationBenchmark(QStringLiteral("unexpected-source-state"), {
            {QStringLiteral("actualPath"), state.currentPath},
            {QStringLiteral("actualLoading"), state.loading},
            {QStringLiteral("expectedSource"), expectedSource},
        });
        return;
    }
    const QVariantMap entry = entering
        ? navigationBenchmarkEntryForPath(benchmark.side,
                                          benchmark.targetPath)
        : navigationBenchmarkUpEntry(benchmark.side);
    if (entry.isEmpty()) {
        failNavigationBenchmark(QStringLiteral("navigation-entry-missing"), {
            {QStringLiteral("actualPath"), state.currentPath},
            {QStringLiteral("direction"), entering
                ? QStringLiteral("enter") : QStringLiteral("leave")},
        });
        return;
    }
    benchmark.phase = NavigationBenchmarkPhase::WaitingForTransitionReadiness;
    sendNavigationBenchmarkAction({
        {QStringLiteral("action"), QStringLiteral("panel.open")},
        {QStringLiteral("side"), benchmark.side},
        {QStringLiteral("entryId"), entry.value(QStringLiteral("entryId"))},
        {QStringLiteral("index"), entry.value(QStringLiteral("index"))},
        {QStringLiteral("catalogRevision"),
         QVariant::fromValue<qulonglong>(state.catalogRevision)},
    }, benchmark.completedCycles < benchmark.warmup
           ? QStringLiteral("warmup") : QStringLiteral("measure"),
       entering ? QStringLiteral("enter") : QStringLiteral("leave"),
       expectedSource, destination);
}

void F4GalleryBridge::advanceBenchmarkTransitionReadiness(
    const SideState &state)
{
    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    const bool sceneReady = benchmark.sceneMatched
        && normalizedBenchmarkPath(state.currentPath) == benchmark.expectedPath
        && !state.loading
        && !state.catalogProvisional
        && state.galleryLayoutMode == QStringLiteral("details");
    const bool placementReady = benchmark.placementReady
        && benchmark.placementPath == benchmark.expectedPath
        && benchmark.placementCatalogRevision == state.catalogRevision;
    if (!sceneReady || !placementReady) {
        return;
    }
    const QVariantMap expectedCursor =
        benchmark.direction == QStringLiteral("enter")
        ? navigationBenchmarkUpEntry(benchmark.side)
        : navigationBenchmarkEntryForPath(
            benchmark.side, benchmark.targetPath);
    if (expectedCursor.isEmpty()
        || state.cursorEntryId != expectedCursor.value(
            QStringLiteral("entryId")).toString()) {
        return;
    }
    armNavigationBenchmarkFrame(false);
}
