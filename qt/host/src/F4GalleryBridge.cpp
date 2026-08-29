#include "F4GalleryBridge.h"
#include "F4IconProvider.h"
#include "NavigationBenchmarkTrace.h"

#include <QCoreApplication>
#include <QDir>
#include <QFileInfo>
#include <QGuiApplication>
#include <QQmlEngine>
#include <QSet>
#include <QTimer>
#include <QUrl>

#include <ZoinGallery/GalleryRuntime.h>
#include <ZoinGallery/GallerySession.h>

#include <utility>

namespace
{
constexpr int GalleryIconLogicalSize = 128;
constexpr int NavigationBenchmarkWatchdogMs = 15000;
// Metadata changes every visible delegate's native icon URL. Keep each GUI
// mutation comfortably below one keyboard-repeat frame and advance at most
// one chunk per rendered frame.
constexpr int CatalogMetadataChunkLimit = 8;
constexpr int CatalogMetadataWarmupLimit = 128;
constexpr int CatalogMetadataFrameFallbackMs = 17;
constexpr int CatalogMetadataCursorWindowChunks = 8;
constexpr int CatalogMetadataMaxFailures = 2;
constexpr int CatalogMetadataInputIdleMs = 100;
constexpr int PanelOpenWatchdogMs = 1500;

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

QVariantMap shellFromScene(const QVariantMap &scene)
{
    const QVariantMap shell = scene.value(QStringLiteral("shell")).toMap();
    if (!shell.isEmpty()) {
        return shell;
    }

    const QVariantList frames = scene.value(QStringLiteral("frames")).toList();
    for (auto it = frames.crbegin(); it != frames.crend(); ++it) {
        const QVariantMap frame = it->toMap();
        const QString kind = frame.value(QStringLiteral("kind")).toString();
        if (kind == QStringLiteral("shell") || kind == QStringLiteral("panels")) {
            return frame;
        }
    }
    return {};
}

bool isF4BundledLucideIcon(const QString &source)
{
    static const QString normalPrefix =
        QStringLiteral("qrc:/F4QtHost/icons/lucide/");
    static const QString galleryPrefix =
        QStringLiteral("qrc:/F4QtHost/icons/lucide-gallery/");
    return source.startsWith(normalPrefix)
        || source.startsWith(galleryPrefix);
}

bool isZoinGalleryDefaultIcon(const QString &source)
{
    static const QSet<QString> defaults{
        QStringLiteral("qrc:/ZoinGallery/resources/FileIcon.svg"),
        QStringLiteral("qrc:/ZoinGallery/resources/FolderIcon.svg"),
        QStringLiteral("qrc:/ZoinGallery/resources/ImageIcon.svg"),
    };
    return defaults.contains(source);
}

bool isF4SystemFileIcon(const QString &source, const QString &providerId)
{
    const QUrl url(source);
    return url.scheme() == QStringLiteral("image")
        && url.host().compare(providerId, Qt::CaseInsensitive) == 0
        && url.path().startsWith(QStringLiteral("/file/"));
}

qreal availableDevicePixelRatio()
{
    return qGuiApp ? qGuiApp->devicePixelRatio() : qreal(1);
}

QVariantMap rowFreePanelPresentation(QVariantMap panel)
{
    panel.remove(QStringLiteral("entries"));
    panel.remove(QStringLiteral("highlightStyles"));
    return panel;
}
}

F4GalleryBridge::F4GalleryBridge(QQmlEngine *engine, QObject *parent,
                                 F4IconSet *iconSet)
    : QObject(parent)
    , m_iconSet(iconSet)
{
    if (m_iconSet) {
        connect(m_iconSet, &F4IconSet::revisionChanged,
                this, &F4GalleryBridge::refreshIconAppearance);
    }
    for (int side = 0; side < 2; ++side) {
        auto *timer = new QTimer(this);
        timer->setSingleShot(true);
        // QML normally commits on key release. This persistent watchdog owns
        // the deferred intent if a transient panel Loader disappears or a
        // focus transition drops that release event.
        timer->setInterval(5000);
        connect(timer, &QTimer::timeout, this,
                [this, side]() { commitPendingCursor(side); });
        m_cursorCommitTimers[static_cast<size_t>(side)] = timer;
    }
    m_panelOpenWatchdog = new QTimer(this);
    m_panelOpenWatchdog->setSingleShot(true);
    m_panelOpenWatchdog->setInterval(PanelOpenWatchdogMs);
    connect(m_panelOpenWatchdog, &QTimer::timeout, this,
            &F4GalleryBridge::clearInFlightPanelOpen);
    m_metadataIdleTimer = new QTimer(this);
    m_metadataIdleTimer->setSingleShot(true);
    m_metadataIdleTimer->setInterval(CatalogMetadataInputIdleMs);
    connect(m_metadataIdleTimer, &QTimer::timeout, this, [this]() {
        m_metadataInputBusy = false;
        schedulePanelCatalogMetadataRequest();
    });
    if (!engine) {
        return;
    }

    ZoinGallery::RuntimeOptions options;
    options.providerPrefix = QStringLiteral("f4-zoingallery");
    options.storageNamespace = QStringLiteral("f4-qt-host");
    // Four workers leave one source-read lane and three decode/cache lanes.
    // DecodeManager starts each lane only on its first task, so constructing
    // the bridge no longer creates one OS thread per logical CPU.
    options.maxDecodeThreads = 4;
    options.persistentCache = true;
    auto *runtime = ZoinGallery::GalleryRuntime::install(engine, options);
    m_runtime = runtime;
    if (!runtime) {
        return;
    }

    m_sessions[0] = runtime->createExternalSession(QStringLiteral("f4-left"), this);
    m_sessions[1] = runtime->createExternalSession(QStringLiteral("f4-right"), this);
    if (m_sessions[0]) {
        m_allSessions.insert(m_sessions[0].data());
    }
    if (m_sessions[1]) {
        m_allSessions.insert(m_sessions[1].data());
    }
    configureNavigationBenchmark();
}

F4GalleryBridge::~F4GalleryBridge()
{
    for (QObject *sessionObject : std::as_const(m_allSessions)) {
        if (auto *session = qobject_cast<ZoinGallery::GallerySession *>(sessionObject)) {
            session->shutdown();
        }
    }
    if (auto *runtime = qobject_cast<ZoinGallery::GalleryRuntime *>(m_runtime.data())) {
        // The runtime owns the engine-level providers and shared decode pool.
        // Stop it while the QQmlEngine is still alive; its QObject destructor
        // remains an idempotent fallback for standalone/other embedders.
        runtime->shutdown();
    }
}

bool F4GalleryBridge::available() const
{
    return m_runtime && m_sessions[0] && m_sessions[1];
}

QObject *F4GalleryBridge::viewerSession() const
{
    return validSide(m_viewerSide) ? m_sessions[static_cast<size_t>(m_viewerSide)].data() : nullptr;
}

QUrl F4GalleryBridge::panelComponentUrl() const
{
    return available() ? QUrl(QStringLiteral("qrc:/F4QtHost/qml/GalleryPanelHost.qml")) : QUrl();
}

QUrl F4GalleryBridge::viewerComponentUrl() const
{
    return available() ? QUrl(QStringLiteral("qrc:/F4QtHost/qml/GalleryViewerHost.qml")) : QUrl();
}

bool F4GalleryBridge::navigationBenchmarkEnabled() const
{
    return m_navigationBenchmark.enabled;
}

QObject *F4GalleryBridge::sessionForSide(int side) const
{
    return validSide(side) ? m_sessions[static_cast<size_t>(side)].data() : nullptr;
}

QObject *F4GalleryBridge::sessionForPanel(const QString &panelId,
                                          int side) const
{
    if (panelId.isEmpty()) {
        return sessionForSide(side);
    }
    const auto cached = m_panelCache.constFind(panelId);
    if (cached != m_panelCache.cend() && cached->session
        && cached->state.initialized
        && cached->state.panelId == panelId) {
        return cached->session.data();
    }
    // A semantic identity must never capture whichever mutable side slot
    // happens to be current.  QML retains this pointer for the viewport's
    // lifetime; returning a fallback here made the startup workspace remain
    // empty even after its exact 5k-row cache was prepared later.
    return nullptr;
}

QVariantList F4GalleryBridge::cachedPanelPresentations(int side) const
{
    QVariantList result;
    if (!validSide(side)) {
        return result;
    }
    for (auto it = m_panelCache.cbegin(); it != m_panelCache.cend(); ++it) {
        const CachedPanel &cached = it.value();
        if (!cached.session || !cached.state.initialized
            || cached.snapshot.value(QStringLiteral("side")).toInt() != side
            || cached.snapshot.isEmpty()) {
            continue;
        }
        result.append(rowFreePanelPresentation(cached.snapshot));
    }
    return result;
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
    const SideState &state = m_states[static_cast<size_t>(side)];
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
    const SideState &state = m_states[static_cast<size_t>(side)];
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

void F4GalleryBridge::requestActivate(int side)
{
    if (!validSide(side)) {
        return;
    }
    noteMetadataInputActivity();
    emit uiActionRequested({
        {QStringLiteral("action"), QStringLiteral("panel.activate")},
        {QStringLiteral("side"), side},
    });
}

void F4GalleryBridge::requestCursor(int side,
                                    const QString &entryId,
                                    int index,
                                    qulonglong catalogRevision,
                                    bool deferCommit)
{
    if (!validSide(side)) {
        return;
    }
    noteMetadataInputActivity();
    prioritizePanelCatalogMetadataRow(side, index);
    if (m_pendingViewer.active && m_pendingViewer.side == side
        && m_pendingViewer.entryId != entryId) {
        clearPendingViewer();
    }
    if (m_pendingPanelOpen.active && m_pendingPanelOpen.side == side
        && m_pendingPanelOpen.entryId != entryId) {
        clearPendingPanelOpen();
    }
    if (!entryId.isEmpty()) {
        PendingCursor &pending = m_pendingCursors[static_cast<size_t>(side)];
        pending.active = true;
        pending.panelId = m_states[static_cast<size_t>(side)].panelId;
        pending.entryId = entryId;
        pending.index = index;
        pending.catalogRevision = effectiveCatalogRevision(side, catalogRevision);
    } else {
        clearPendingCursor(side);
    }
    m_stateReconciliationPending[static_cast<size_t>(side)] = true;
    if (deferCommit) {
        // Gallery already moved optimistically. Keep the latest stable ID in
        // pending state so older scenes cannot snap it backward, but avoid
        // serializing a full semantic catalog for every native key-repeat.
        m_cursorCommitTimers[static_cast<size_t>(side)]->start();
        return;
    }
    m_cursorCommitTimers[static_cast<size_t>(side)]->stop();
    const bool activateTarget = !entryId.isEmpty()
        && !m_states[static_cast<size_t>(side)].active;
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

    const SideState &sideState = m_states[static_cast<size_t>(side)];
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
        m_pendingViewer.active = true;
        m_pendingViewer.side = side;
        m_pendingViewer.panelId = m_states[static_cast<size_t>(side)].panelId;
        m_pendingViewer.entryId = entryId;
        m_pendingViewer.catalogRevision = effectiveCatalogRevision(side, catalogRevision);

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
            m_pendingViewer.catalogRevision = sideState.catalogRevision;
            reconcilePendingViewer(side);
            return;
        }
        requestCursor(side, entryId, index, m_pendingViewer.catalogRevision);
        return;
    }

    clearPendingViewer();
    clearPendingPanelOpen();
    m_pendingPanelOpen.active = true;
    m_pendingPanelOpen.side = side;
    m_pendingPanelOpen.panelId = m_states[static_cast<size_t>(side)].panelId;
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
    const SideState &state = m_states[sideIndex];
    PendingSelection &pending = m_pendingSelections[sideIndex];
    if (!pending.active || pending.panelId != state.panelId) {
        pending = PendingSelection{};
        pending.active = true;
        pending.panelId = state.panelId;
        pending.catalogRevision = effectiveCatalogRevision(side, catalogRevision);
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
    emitSelectionAction(side, normalizedMode, entryIds, revision);
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
    QVariantMap action = {
        {QStringLiteral("action"), QStringLiteral("panel.setGalleryLayout")},
        {QStringLiteral("side"), side},
        {QStringLiteral("layoutMode"), normalized},
    };
    if (columnCount > 0) {
        action.insert(QStringLiteral("columnCount"), columnCount);
    }
    emit uiActionRequested(action);
}

void F4GalleryBridge::requestGalleryDensity(int side,
                                            const QString &layoutMode,
                                            int density)
{
    if (!validSide(side)) {
        return;
    }
    static const QSet<QString> adjustable = {
        QStringLiteral("masonry"), QStringLiteral("grid"),
        QStringLiteral("icons"),
    };
    const QString normalized = layoutMode.trimmed().toLower();
    if (!adjustable.contains(normalized)) {
        return;
    }
    emit uiActionRequested({
        {QStringLiteral("action"), QStringLiteral("panel.setGalleryDensity")},
        {QStringLiteral("side"), side},
        {QStringLiteral("layoutMode"), normalized},
        {QStringLiteral("density"), density},
    });
}

void F4GalleryBridge::requestSort(int side, const QString &sortMode,
                                  bool contextMenu)
{
    if (!validSide(side)) {
        return;
    }
    const QString normalized = sortMode.trimmed().toLower();
    if (contextMenu) {
        emit uiActionRequested({
            {QStringLiteral("action"), QStringLiteral("panel.sortMenu")},
            {QStringLiteral("side"), side},
        });
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
    emit uiActionRequested({
        {QStringLiteral("action"), QStringLiteral("panel.sort")},
        {QStringLiteral("side"), side},
        {QStringLiteral("mode"), normalized},
    });
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
    if (F4NavigationBenchmarkTrace::enabled()
        && !m_navigationBenchmark.enabled && validSide(side)
        && m_lastInputSceneTraceId.isValid()) {
        QVariantMap fields = metadata;
        const SideState &state = m_states[static_cast<size_t>(side)];
        fields.insert(QStringLiteral("stage"), stage);
        fields.insert(QStringLiteral("side"), side);
        fields.insert(QStringLiteral("bridgePanelLoading"), state.loading);
        fields.insert(QStringLiteral("bridgePanelPath"), state.currentPath);
        fields.insert(QStringLiteral("bridgeCatalogRevision"),
                      QVariant::fromValue<qulonglong>(state.catalogRevision));
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.gallery.qml.%1").arg(stage),
            m_lastInputSceneTraceId, fields);
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
    QVariantMap fields = navigationBenchmarkFields();
    for (auto it = metadata.cbegin(); it != metadata.cend(); ++it) {
        fields.insert(it.key(), it.value());
    }
    fields.insert(QStringLiteral("stage"), stage);
    fields.insert(QStringLiteral("bridgePanelLoading"),
                  m_states[static_cast<size_t>(side)].loading);
    fields.insert(QStringLiteral("bridgePanelPath"),
                  m_states[static_cast<size_t>(side)].currentPath);
    fields.insert(QStringLiteral("sceneBenchmark"),
                  benchmark.lastSceneBenchmark);
    queueNavigationBenchmarkTrace(
        QStringLiteral("qt.gallery.qml.%1").arg(stage), traceId, fields);

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

    if (path == benchmark.expectedPath
        && catalogRevision != benchmark.placementCatalogRevision
        && (stage == QStringLiteral("session.catalog.changed")
            || stage == QStringLiteral("layout.reset")
            || stage == QStringLiteral("host.panel.changed"))) {
        benchmark.placementReady = false;
    }

    if (path == benchmark.expectedPath
        && presentationMode == QStringLiteral("details")
        && !placementPending && placementMatchesTarget
        && (count == 0 || geometryValid)) {
        benchmark.placementReady = true;
        benchmark.placementPath = path;
        benchmark.placementCatalogRevision = catalogRevision;
        benchmark.lastPlacement = metadata;
        restartNavigationBenchmarkWatchdog();
        scheduleNavigationBenchmarkAdvance();
    }
}

void F4GalleryBridge::notifyRenderSynchronized()
{
    // Connected directly to QQuickWindow::afterSynchronizing and therefore
    // potentially called on the render thread. The serial is the only state
    // touched here; notifyFrameSwapped consumes it on the GUI thread.
    m_renderSyncSerial.fetch_add(1, std::memory_order_release);
}

void F4GalleryBridge::captureFrameSwapped()
{
    // Pair this exact swapped frame with the synchronization that produced
    // it. Loading the atomic or sampling the clock later on the GUI thread
    // could observe a newer frame, while an occupied GUI queue would inflate
    // the measured presentation boundary by unrelated message/input work.
    const qulonglong synchronizedSerial =
        m_renderSyncSerial.load(std::memory_order_acquire);
    const qint64 frameBoundaryNs =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();
    QMetaObject::invokeMethod(this, [this, synchronizedSerial,
                                     frameBoundaryNs]() {
        notifyFrameSwappedAt(synchronizedSerial, frameBoundaryNs);
    }, Qt::QueuedConnection);
}

void F4GalleryBridge::notifyFrameSwapped(qulonglong synchronizedSerial)
{
    // Synthetic/test callers have no render-thread capture point. Preserve
    // the public serial-only API by treating the call itself as the boundary.
    notifyFrameSwappedAt(
        synchronizedSerial,
        F4NavigationBenchmarkTrace::monotonicNanoseconds());
}

void F4GalleryBridge::notifyFrameSwappedAt(
    qulonglong synchronizedSerial, qint64 frameBoundaryNs)
{
    if (F4NavigationBenchmarkTrace::enabled()
        && m_pendingInputFrameTraceId.isValid()
        && synchronizedSerial
            >= m_pendingInputFrameRequiredRenderSyncSerial) {
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.input.frame.swapped"), frameBoundaryNs,
            m_pendingInputFrameTraceId, {
                {QStringLiteral("sceneToFrameNs"),
                 frameBoundaryNs - m_pendingInputFrameSceneEndNs},
                {QStringLiteral("supersededScenes"),
                 QVariant::fromValue<qulonglong>(
                     m_inputScenesSupersededBeforeFrame)},
            });
        m_pendingInputFrameTraceId.clear();
        m_pendingInputFrameSceneEndNs = 0;
        m_pendingInputFrameRequiredRenderSyncSerial = 0;
        m_inputScenesSupersededBeforeFrame = 0;
    }

    bool metadataBecameEligible = false;
    for (int side = 0; side < 2; ++side) {
        SideState &state = m_states[static_cast<size_t>(side)];
        if (!state.metadataAwaitingFrame
            || synchronizedSerial
                < state.metadataRequiredRenderSyncSerial) {
            continue;
        }
        state.metadataAwaitingFrame = false;
        state.metadataRequiredRenderSyncSerial = 0;
        metadataBecameEligible = true;
    }
    if (metadataBecameEligible) {
        requestNextPanelCatalogMetadata();
    }

    NavigationBenchmarkState &benchmark = m_navigationBenchmark;
    if (!benchmark.enabled) {
        return;
    }
    ++benchmark.frameSerial;
    emit benchmarkFrameSwapped(benchmark.frameSerial);
    if (benchmark.phase == NavigationBenchmarkPhase::Finished
        || benchmark.phase == NavigationBenchmarkPhase::Failed) {
        return;
    }

    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("lastPlacement"),
                  benchmark.lastPlacement);
    m_pendingNavigationBenchmarkTrace.push_back({
        QStringLiteral("qt.gallery.frame-swapped"), frameBoundaryNs,
        benchmark.benchmarkTraceId, fields});

    const bool awaitingFrame =
        benchmark.phase == NavigationBenchmarkPhase::WaitingForSetupFrame
        || benchmark.phase
            == NavigationBenchmarkPhase::WaitingForTransitionFrame;
    if (awaitingFrame && benchmark.requiredFrameSerial != 0
        && benchmark.frameSerial >= benchmark.requiredFrameSerial) {
        completeNavigationBenchmarkFrame();
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

    if (!validSide(benchmark.side)) {
        for (int side = 0; side < 2; ++side) {
            const SideState &candidate =
                m_states[static_cast<size_t>(side)];
            if (candidate.initialized && candidate.active) {
                benchmark.side = side;
                break;
            }
        }
        if (!validSide(benchmark.side)) {
            return;
        }
        queueNavigationBenchmarkTrace(
            QStringLiteral("qt.gallery.runner.side-selected"),
            benchmark.runId, navigationBenchmarkFields());
    }

    const SideState &state =
        m_states[static_cast<size_t>(benchmark.side)];
    switch (benchmark.phase) {
    case NavigationBenchmarkPhase::WaitingForPanel:
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
        benchmark.phase =
            NavigationBenchmarkPhase::NavigatingToTargetForSetup;
        benchmark.actionSent = false;
        scheduleNavigationBenchmarkAdvance();
        return;

    case NavigationBenchmarkPhase::SettingDetails:
        if (state.galleryLayoutMode != QStringLiteral("details")) {
            return;
        }
        benchmark.phase =
            NavigationBenchmarkPhase::NavigatingToTargetForSetup;
        benchmark.actionSent = false;
        scheduleNavigationBenchmarkAdvance();
        return;

    case NavigationBenchmarkPhase::NavigatingToTargetForSetup:
        if (!benchmark.actionSent) {
            if (normalizedBenchmarkPath(state.currentPath)
                    == benchmark.targetPath
                && !state.loading) {
                benchmark.phase =
                    NavigationBenchmarkPhase::ReturningToParentForSetup;
                scheduleNavigationBenchmarkAdvance();
                return;
            }
            sendNavigationBenchmarkAction({
                {QStringLiteral("action"),
                 QStringLiteral("panel.navigatePath")},
                {QStringLiteral("side"), benchmark.side},
                {QStringLiteral("path"), benchmark.targetPath},
            }, QStringLiteral("setup"), QStringLiteral("setup-target"),
               state.currentPath, benchmark.targetPath);
            return;
        }
        if (normalizedBenchmarkPath(state.currentPath)
                == benchmark.targetPath
            && !state.loading) {
            queueNavigationBenchmarkTrace(
                QStringLiteral("qt.gallery.runner.setup-target-ready"),
                benchmark.benchmarkTraceId, navigationBenchmarkFields());
            benchmark.phase =
                NavigationBenchmarkPhase::ReturningToParentForSetup;
            benchmark.actionSent = false;
            scheduleNavigationBenchmarkAdvance();
        }
        return;

    case NavigationBenchmarkPhase::ReturningToParentForSetup:
        benchmark.phase =
            NavigationBenchmarkPhase::WaitingForSetupReadiness;
        sendNavigationBenchmarkAction({
            {QStringLiteral("action"),
             QStringLiteral("panel.navigatePath")},
            {QStringLiteral("side"), benchmark.side},
            {QStringLiteral("path"), benchmark.parentPath},
        }, QStringLiteral("setup"), QStringLiteral("setup-parent"),
           state.currentPath, benchmark.parentPath);
        return;

    case NavigationBenchmarkPhase::WaitingForSetupReadiness: {
        const QVariantMap targetEntry = navigationBenchmarkEntryForPath(
            benchmark.side, benchmark.targetPath);
        const bool targetCursor = !targetEntry.isEmpty()
            && state.cursorEntryId == targetEntry.value(
                QStringLiteral("entryId")).toString();
        const bool sceneReady = benchmark.sceneMatched
            && normalizedBenchmarkPath(state.currentPath)
                == benchmark.parentPath
            && !state.loading
            && state.galleryLayoutMode == QStringLiteral("details")
            && targetCursor;
        const bool placementReady = benchmark.placementReady
            && benchmark.placementPath == benchmark.parentPath
            && benchmark.placementCatalogRevision == state.catalogRevision;
        if (sceneReady && placementReady) {
            armNavigationBenchmarkFrame(true);
        }
        return;
    }

    case NavigationBenchmarkPhase::WaitingForSetupFrame:
        return;

    case NavigationBenchmarkPhase::ReadyToDispatch: {
        if (benchmark.completedCycles
            >= benchmark.warmup + benchmark.cycles) {
            finishNavigationBenchmark();
            return;
        }

        const bool entering = benchmark.nextTransitionEnters;
        const QString expectedSource = entering
            ? benchmark.parentPath : benchmark.targetPath;
        const QString expectedDestination = entering
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
                {QStringLiteral("direction"),
                 entering ? QStringLiteral("enter")
                          : QStringLiteral("leave")},
            });
            return;
        }

        benchmark.phase =
            NavigationBenchmarkPhase::WaitingForTransitionReadiness;
        sendNavigationBenchmarkAction({
            {QStringLiteral("action"), QStringLiteral("panel.open")},
            {QStringLiteral("side"), benchmark.side},
            {QStringLiteral("entryId"),
             entry.value(QStringLiteral("entryId"))},
            {QStringLiteral("index"),
             entry.value(QStringLiteral("index"))},
            {QStringLiteral("catalogRevision"),
             QVariant::fromValue<qulonglong>(state.catalogRevision)},
        }, benchmark.completedCycles < benchmark.warmup
               ? QStringLiteral("warmup") : QStringLiteral("measure"),
           entering ? QStringLiteral("enter") : QStringLiteral("leave"),
           expectedSource, expectedDestination);
        return;
    }

    case NavigationBenchmarkPhase::WaitingForTransitionReadiness: {
        const bool sceneReady = benchmark.sceneMatched
            && normalizedBenchmarkPath(state.currentPath)
                == benchmark.expectedPath
            && !state.loading
            && state.galleryLayoutMode == QStringLiteral("details");
        const bool placementReady = benchmark.placementReady
            && benchmark.placementPath == benchmark.expectedPath
            && benchmark.placementCatalogRevision == state.catalogRevision;
        if (!sceneReady || !placementReady) {
            return;
        }

        const QVariantMap expectedCursorEntry =
            benchmark.direction == QStringLiteral("enter")
            ? navigationBenchmarkUpEntry(benchmark.side)
            : navigationBenchmarkEntryForPath(benchmark.side,
                                               benchmark.targetPath);
        if (expectedCursorEntry.isEmpty()
            || state.cursorEntryId != expectedCursorEntry.value(
                QStringLiteral("entryId")).toString()) {
            return;
        }
        armNavigationBenchmarkFrame(false);
        return;
    }

    case NavigationBenchmarkPhase::WaitingForTransitionFrame:
    case NavigationBenchmarkPhase::Finished:
    case NavigationBenchmarkPhase::Failed:
    case NavigationBenchmarkPhase::Disabled:
        return;
    }
}

void F4GalleryBridge::closeViewer()
{
    clearPendingViewer();
    if (!m_viewerVisible) {
        return;
    }
    if (auto *session = qobject_cast<ZoinGallery::GallerySession *>(viewerSession())) {
        session->setViewerOpen(false);
    }
    setViewer(-1, false);
}

void F4GalleryBridge::synchronizeScene(const QVariantMap &scene)
{
    const bool inputTraceEnabled = F4NavigationBenchmarkTrace::enabled();
    const QVariant sceneTraceId = inputTraceEnabled
        ? F4NavigationBenchmarkTrace::benchmarkTraceId(scene) : QVariant();
    const bool traceInputScene = sceneTraceId.isValid();
    const bool benchmarkRunning = m_navigationBenchmark.enabled
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Finished
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Failed;
    const bool genericInputTrace = traceInputScene && !benchmarkRunning;
    const qint64 inputBridgeBeginNs = traceInputScene
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    if (genericInputTrace) {
        m_lastInputSceneTraceId = sceneTraceId;
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.gallery.bridge.scene.begin"),
            inputBridgeBeginNs, sceneTraceId, {
                {QStringLiteral("sceneType"),
                 scene.value(QStringLiteral("type"))},
            });
    }

    if (benchmarkRunning) {
        if (sceneTraceId.isValid()) {
            m_navigationBenchmark.lastSceneTraceId = sceneTraceId;
        }
        const QVariant benchmarkValue = scene.value(
            QStringLiteral("benchmark"));
        if (benchmarkValue.metaType().id() == QMetaType::QVariantMap) {
            m_navigationBenchmark.lastSceneBenchmark =
                benchmarkValue.toMap();
        } else {
            m_navigationBenchmark.lastSceneBenchmark.clear();
        }
        if (!m_navigationBenchmark.benchmarkTraceId.isEmpty()
            && sceneTraceId.toString()
                == m_navigationBenchmark.benchmarkTraceId) {
            m_navigationBenchmark.sceneMatched = true;
            restartNavigationBenchmarkWatchdog();
        }
        QVariantMap fields = navigationBenchmarkFields();
        fields.insert(QStringLiteral("sceneBenchmark"),
                      m_navigationBenchmark.lastSceneBenchmark);
        fields.insert(QStringLiteral("sceneType"),
                      scene.value(QStringLiteral("type")));
        queueNavigationBenchmarkTrace(
            QStringLiteral("qt.gallery.bridge.scene.begin"),
            sceneTraceId, fields);
    }

    const QVariantList panels = panelsFromScene(scene);
    std::array<bool, 2> found = {false, false};
    for (const QVariant &panelValue : panels) {
        const QVariantMap panel = panelValue.toMap();
        const int side = panel.value(QStringLiteral("side")).toInt();
        if (!validSide(side)) {
            continue;
        }
        found[static_cast<size_t>(side)] = true;
        if (canSkipUnchangedInactivePanel(side, panel)) {
            // Keep the newest implicitly-shared source snapshot for a later
            // icon-set refresh without walking an unchanged inactive catalog.
            m_panelSnapshots[static_cast<size_t>(side)] = panel;
            if (benchmarkRunning) {
                QVariantMap fields = navigationBenchmarkFields();
                fields.insert(QStringLiteral("syncSide"), side);
                fields.insert(QStringLiteral("syncPath"),
                              panel.value(QStringLiteral("path")));
                fields.insert(QStringLiteral("syncLoading"),
                              panel.value(QStringLiteral("loading")));
                fields.insert(QStringLiteral("syncCatalogRevision"),
                              panel.value(QStringLiteral(
                                  "catalogRevision")));
                queueNavigationBenchmarkTrace(
                    QStringLiteral("qt.gallery.bridge.panel.skipped"),
                    sceneTraceId, fields);
            }
            if (genericInputTrace) {
                F4NavigationBenchmarkTrace::event(
                    QStringLiteral("qt.gallery.bridge.panel.skipped"),
                    sceneTraceId, {
                        {QStringLiteral("side"), side},
                        {QStringLiteral("path"),
                         panel.value(QStringLiteral("path"))},
                    });
            }
            continue;
        }
        const qint64 panelSyncBeginNs = genericInputTrace
            ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
        if (genericInputTrace) {
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.gallery.bridge.panel.begin"),
                panelSyncBeginNs, sceneTraceId, {
                    {QStringLiteral("side"), side},
                    {QStringLiteral("path"),
                     panel.value(QStringLiteral("path"))},
                    {QStringLiteral("entries"),
                     panel.value(QStringLiteral("entries")).toList().size()},
                });
        }
        synchronizePanel(side, panel);
        if (genericInputTrace) {
            const qint64 panelSyncEndNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
            F4NavigationBenchmarkTrace::eventAt(
                QStringLiteral("qt.gallery.bridge.panel.end"), panelSyncEndNs,
                sceneTraceId, {
                    {QStringLiteral("side"), side},
                    {QStringLiteral("durationNs"),
                     panelSyncEndNs - panelSyncBeginNs},
                });
        }
    }

    if (m_viewerVisible
        && (!validSide(m_viewerSide) || !found[static_cast<size_t>(m_viewerSide)]
            || !m_states[static_cast<size_t>(m_viewerSide)].previewCapable
            || !m_states[static_cast<size_t>(m_viewerSide)].active)) {
        closeViewer();
    }
    if (m_pendingViewer.active
        && (!validSide(m_pendingViewer.side)
            || !found[static_cast<size_t>(m_pendingViewer.side)])) {
        clearPendingViewer();
    }
    if (m_pendingPanelOpen.active
        && (!validSide(m_pendingPanelOpen.side)
            || !found[static_cast<size_t>(m_pendingPanelOpen.side)])) {
        clearPendingPanelOpen();
    }
    if (m_inFlightPanelOpen.active
        && (!validSide(m_inFlightPanelOpen.side)
            || !found[static_cast<size_t>(m_inFlightPanelOpen.side)])) {
        clearInFlightPanelOpen();
    }
    for (int side = 0; side < 2; ++side) {
        if (!found[static_cast<size_t>(side)]) {
            m_panelSnapshots[static_cast<size_t>(side)].clear();
            clearPendingCursor(side);
            clearPendingSelection(side);
        }
    }

    if (benchmarkRunning) {
        QVariantMap fields = navigationBenchmarkFields();
        fields.insert(QStringLiteral("sceneBenchmark"),
                      m_navigationBenchmark.lastSceneBenchmark);
        fields.insert(QStringLiteral("panelCount"), panels.size());
        if (validSide(m_navigationBenchmark.side)) {
            const SideState &state = m_states[static_cast<size_t>(
                m_navigationBenchmark.side)];
            fields.insert(QStringLiteral("panelPath"), state.currentPath);
            fields.insert(QStringLiteral("panelLoading"), state.loading);
            fields.insert(QStringLiteral("panelCatalogRevision"),
                          QVariant::fromValue<qulonglong>(
                              state.catalogRevision));
            fields.insert(QStringLiteral("panelCursorEntryId"),
                          state.cursorEntryId);
            fields.insert(QStringLiteral("panelCursorIndex"),
                          state.cursorIndex);
            fields.insert(QStringLiteral("panelLayoutMode"),
                          state.galleryLayoutMode);
            fields.insert(QStringLiteral("panelEntryCount"),
                          state.entries.size());
        }
        queueNavigationBenchmarkTrace(
            QStringLiteral("qt.gallery.bridge.scene.end"),
            sceneTraceId, fields);
        scheduleNavigationBenchmarkAdvance();
    }

    if (genericInputTrace) {
        const qint64 inputBridgeEndNs =
            F4NavigationBenchmarkTrace::monotonicNanoseconds();
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.gallery.bridge.scene.end"), inputBridgeEndNs,
            sceneTraceId, {
                {QStringLiteral("durationNs"),
                 inputBridgeEndNs - inputBridgeBeginNs},
                {QStringLiteral("panelCount"), panels.size()},
            });

        if (m_pendingInputFrameTraceId.isValid()) {
            ++m_inputScenesSupersededBeforeFrame;
            F4NavigationBenchmarkTrace::event(
                QStringLiteral("qt.input.frame.superseded"),
                m_pendingInputFrameTraceId, {
                    {QStringLiteral("replacedByTraceId"),
                     sceneTraceId.toString()},
                    {QStringLiteral("sceneAgeNs"),
                     inputBridgeEndNs - m_pendingInputFrameSceneEndNs},
                });
        }
        m_pendingInputFrameTraceId = sceneTraceId;
        m_pendingInputFrameSceneEndNs = inputBridgeEndNs;
        m_pendingInputFrameRequiredRenderSyncSerial =
            m_renderSyncSerial.load(std::memory_order_acquire) + 1;
    }
    // A deferred base may have painted while the panel was still loading.
    // If loading=false later needs a full-scene fallback (rather than a
    // compact panel patch), re-evaluate the now-eligible metadata stream.
    schedulePanelCatalogMetadataRequest();
}

void F4GalleryBridge::synchronizePanelActivation(int activePanel,
                                                 qulonglong revision)
{
    if (!validSide(activePanel) || revision == 0
        || revision <= m_panelActivationRevision) {
        return;
    }
    noteMetadataInputActivity();
    m_panelActivationRevision = revision;

    for (int side = 0; side < 2; ++side) {
        const bool active = side == activePanel;
        SideState &state = m_states[static_cast<size_t>(side)];
        if (state.initialized) {
            state.active = active;
        }
        QVariantMap &snapshot = m_panelSnapshots[static_cast<size_t>(side)];
        if (!snapshot.isEmpty()) {
            snapshot.insert(QStringLiteral("active"), active);
        }
    }

    if (m_viewerVisible && m_viewerSide != activePanel) {
        closeViewer();
    }
    // An inactive Gallery click can queue a stable viewer intent before Go
    // acknowledges activation. Complete that intent without walking either
    // catalog once the revisioned authoritative patch arrives.
    reconcilePendingViewer(activePanel);
    prioritizePanelCatalogMetadataRow(
        activePanel,
        m_states[static_cast<size_t>(activePanel)].cursorIndex);
    schedulePanelCatalogMetadataRequest();
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

void F4GalleryBridge::synchronizePanelState(const QVariantMap &patch)
{
    bool sideOK = false;
    const int side = patch.value(QStringLiteral("side")).toInt(&sideOK);
    if (!sideOK || !validSide(side)) {
        return;
    }
    const size_t sideIndex = static_cast<size_t>(side);
    SideState &state = m_states[sideIndex];
    if (!state.initialized
        || patch.value(QStringLiteral("panelId")).toString()
            != state.panelId
        || revisionValue(patch, QStringLiteral("catalogRevision"))
            != state.catalogRevision) {
        return;
    }
    const QVariant panelValue = patch.value(QStringLiteral("panel"));
    if (panelValue.metaType().id() != QMetaType::QVariantMap) {
        return;
    }
    const QVariantMap panel = panelValue.toMap();
    if (panel.contains(QStringLiteral("entries"))
        || panel.contains(QStringLiteral("highlightStyles"))) {
        return;
    }

    const QString cursorEntryId = panel.value(
        QStringLiteral("cursorEntryId"), state.cursorEntryId).toString();
    const int cursorIndex = panel.value(
        QStringLiteral("cursor"), state.cursorIndex).toInt();
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_sessions[sideIndex].data());

    const QString nextCurrentPath = panel.value(
        QStringLiteral("path"), state.currentPath).toString();
    const QString nextSourceKind = panel.value(
        QStringLiteral("sourceKind"), state.sourceKind).toString();
    const bool nextPreviewCapable = panel.value(
        QStringLiteral("previewCapable"), state.previewCapable).toBool();
    const bool nextMetadataDeferred = panel.value(
        QStringLiteral("metadataDeferred"), state.metadataDeferred).toBool();
    const qulonglong nextMetadataRevision = revisionValue(
        panel, QStringLiteral("metadataRevision"));
    const bool nextCatalogProvisional = panel.value(
        QStringLiteral("catalogProvisional"), state.catalogProvisional)
                                            .toBool();
    const QString nextGalleryLayoutMode = panel.value(
        QStringLiteral("galleryLayoutMode"), state.galleryLayoutMode)
                                              .toString();
    const bool enteringDetails = state.galleryLayoutMode
            != QStringLiteral("details")
        && nextGalleryLayoutMode == QStringLiteral("details");
    const bool metadataStreamChanged =
        nextMetadataDeferred != state.metadataDeferred
        || (nextMetadataDeferred
            && nextMetadataRevision != state.metadataRevision)
        || nextCurrentPath != state.currentPath;
    const bool metadataRestartNeeded = nextMetadataDeferred
        && metadataStreamChanged;
    const QString op = patch.value(QStringLiteral("op")).toString();
    qulonglong nextSelectionRevision = state.selectionRevision;
    QStringList nextSelectedIds = state.selectedEntryIdList;
    QSet<QString> nextSelectedSet = state.selectedEntryIds;
    bool applied = false;

    if (op == QStringLiteral("state_update")) {
        applied = !session || session->applyExternalStateDelta(
            cursorEntryId, cursorIndex, {}, state.selectionRevision,
            state.selectionRevision);
    } else if (op == QStringLiteral("selection_delta")) {
        const qulonglong baseRevision = revisionValue(
            patch, QStringLiteral("baseSelectionRevision"));
        nextSelectionRevision = revisionValue(
            patch, QStringLiteral("selectionRevision"));
        const QVariant changesValue = patch.value(QStringLiteral("changes"));
        if (baseRevision != state.selectionRevision
            || nextSelectionRevision <= baseRevision
            || changesValue.metaType().id() != QMetaType::QVariantList) {
            return;
        }
        const QVariantList changes = changesValue.toList();
        applied = !session || session->applyExternalStateDelta(
            cursorEntryId, cursorIndex, changes, baseRevision,
            nextSelectionRevision);
        if (applied) {
            for (const QVariant &changeValue : changes) {
                const QVariantMap change = changeValue.toMap();
                const QString entryId = change.value(
                    QStringLiteral("entryId")).toString();
                if (change.value(QStringLiteral("selected")).toBool()) {
                    if (!nextSelectedSet.contains(entryId)) {
                        nextSelectedSet.insert(entryId);
                        nextSelectedIds.push_back(entryId);
                    }
                } else if (nextSelectedSet.remove(entryId)) {
                    nextSelectedIds.removeAll(entryId);
                }
            }
        }
    } else if (op == QStringLiteral("selection_replace")) {
        const qulonglong baseRevision = revisionValue(
            patch, QStringLiteral("baseSelectionRevision"));
        nextSelectionRevision = revisionValue(
            patch, QStringLiteral("selectionRevision"));
        const QVariant idsValue = patch.value(
            QStringLiteral("selectedEntryIds"));
        if (baseRevision != state.selectionRevision
            || nextSelectionRevision <= baseRevision
            || idsValue.metaType().id() != QMetaType::QVariantList) {
            return;
        }
        nextSelectedIds.clear();
        for (const QVariant &idValue : idsValue.toList()) {
            nextSelectedIds.push_back(idValue.toString());
        }
        applied = !session || session->applyExternalState(
            cursorEntryId, cursorIndex, nextSelectedIds,
            nextSelectionRevision);
        if (applied) {
            nextSelectedSet = QSet<QString>(nextSelectedIds.cbegin(),
                                            nextSelectedIds.cend());
        }
    } else {
        return;
    }
    if (!applied) {
        return;
    }

    if (session && (metadataStreamChanged
                    || nextCatalogProvisional
                        != state.catalogProvisional)) {
        if (!session->applyExternalCatalog(
                state.entries, state.catalogRevision, {
                    {QStringLiteral("currentPath"), nextCurrentPath},
                    {QStringLiteral("sourceKind"), nextSourceKind},
                    {QStringLiteral("previewCapable"), nextPreviewCapable},
                    {QStringLiteral("catalogProvisional"),
                     nextCatalogProvisional},
                    {QStringLiteral("metadataDeferred"),
                     nextMetadataDeferred},
                    {QStringLiteral("metadataRevision"),
                     QVariant::fromValue<qulonglong>(
                         nextMetadataRevision)},
                })) {
            return;
        }
    }

    QVariantMap &snapshot = m_panelSnapshots[sideIndex];
    for (auto it = panel.cbegin(); it != panel.cend(); ++it) {
        snapshot.insert(it.key(), it.value());
    }
    state.selectionRevision = nextSelectionRevision;
    state.selectedEntryIdList = std::move(nextSelectedIds);
    state.selectedEntryIds = std::move(nextSelectedSet);
    state.cursorEntryId = cursorEntryId;
    state.cursorIndex = cursorIndex;
    state.currentPath = nextCurrentPath;
    state.sourceKind = nextSourceKind;
    state.previewCapable = nextPreviewCapable;
    state.active = panel.value(QStringLiteral("active"), state.active)
                       .toBool();
    state.loading = panel.value(QStringLiteral("loading"), state.loading)
                        .toBool();
    state.catalogProvisional = nextCatalogProvisional;
    state.metadataDeferred = nextMetadataDeferred;
    state.metadataRevision = nextMetadataDeferred
        ? nextMetadataRevision : 0;
    state.galleryLayoutMode = nextGalleryLayoutMode;
    if (metadataRestartNeeded) {
        resetPanelCatalogMetadataPlan(side, false);
    } else if (!nextMetadataDeferred) {
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
    m_stateReconciliationPending[sideIndex] = false;
    reconcilePendingCursor(side);
    reconcilePendingPanelOpen(side);
    reconcilePendingSelection(side);
    reconcilePendingViewer(side);
    // Compact layouts can cover the same numeric row range while requiring
    // very different metadata. In particular, entering Details makes Size
    // paint-critical even when the Gallery viewport reports an unchanged
    // range, so re-arm one visible-first request instead of waiting for the
    // background stream to become idle.
    if (enteringDetails && state.metadataDeferred
        && !state.metadataComplete) {
        state.metadataUrgentBudget = 1;
    }
    prioritizePanelCatalogMetadataRow(side, cursorIndex);
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
    if (type == QStringLiteral("scene_patch")) {
        const QVariantMap rootPatch = message.value(
            QStringLiteral("root")).toMap();
        const QVariantMap rootSet = rootPatch.value(
            QStringLiteral("set")).toMap();
        const QVariant shellValue = rootSet.value(QStringLiteral("shell"));
        if (shellValue.metaType().id() == QMetaType::QVariantMap) {
            synchronizeWorkspaceShell(shellValue.toMap());
        }
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

void F4GalleryBridge::handleProtocolMessage(const QVariantMap &message)
{
    const QString type = message.value(QStringLiteral("type")).toString();
    if (type == QStringLiteral("panel_cache")) {
        if (message.value(QStringLiteral("schema")).toString()
                == QStringLiteral("app")
            && message.value(QStringLiteral("version")).toInt() == 4
            && message.value(QStringLiteral("panel")).metaType().id()
                == QMetaType::QVariantMap) {
            synchronizePanelCache(
                message.value(QStringLiteral("panel")).toMap(),
                message.value(QStringLiteral("metadata")).toMap());
        }
        return;
    }
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
    if (type != QStringLiteral("panel_catalog_metadata")
        && type != QStringLiteral("panel_catalog_metadata_rejected")) {
        return;
    }

    const int side = matchingMetadataSide(message);
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_states[static_cast<size_t>(side)];
    if (type == QStringLiteral("panel_catalog_metadata_rejected")) {
        failPanelCatalogMetadataRequest(side, false);
        return;
    }

    bool limitOK = false;
    bool totalOK = false;
    bool highlightRevisionOK = false;
    const int limit = message.value(QStringLiteral("limit")).toInt(&limitOK);
    const int total = message.value(QStringLiteral("total")).toInt(&totalOK);
    const qulonglong highlightRevision = message.value(
        QStringLiteral("highlightRevision")).toULongLong(
            &highlightRevisionOK);
    const QVariant entriesValue = message.value(QStringLiteral("entries"));
    const QVariant stylesValue = message.value(
        QStringLiteral("highlightStyles"));
    if (!limitOK || limit != state.metadataRequestLimit || limit <= 0
        || limit > CatalogMetadataChunkLimit || !totalOK
        || total < state.metadataRequestOffset || !highlightRevisionOK
        || entriesValue.metaType().id() != QMetaType::QVariantList
        || (message.contains(QStringLiteral("highlightStyles"))
            && stylesValue.metaType().id() != QMetaType::QVariantMap)
        || !message.contains(QStringLiteral("totalSize"))
        || !message.contains(QStringLiteral("final"))) {
        failPanelCatalogMetadataRequest(side, true);
        return;
    }

    const QVariantList sourceEntries = entriesValue.toList();
    const bool final = message.value(QStringLiteral("final")).toBool();
    const int requestOffset = state.metadataRequestOffset;
    const int endOffset = requestOffset + sourceEntries.size();
    if (total != state.entries.size()
        || sourceEntries.size() != limit || endOffset > total
        || endOffset > state.entries.size()
        || final != (endOffset == total)) {
        failPanelCatalogMetadataRequest(side, true);
        return;
    }
    for (qsizetype index = 0; index < sourceEntries.size(); ++index) {
        if (sourceEntries.at(index).metaType().id()
            != QMetaType::QVariantMap) {
            failPanelCatalogMetadataRequest(side, true);
            return;
        }
        const QVariantMap metadata = sourceEntries.at(index).toMap();
        const QVariantMap base = state.entries.at(
            requestOffset + index).toMap();
        bool metadataIndexOK = false;
        const int metadataIndex = metadata.value(QStringLiteral("index"))
                                      .toInt(&metadataIndexOK);
        if (metadata.value(QStringLiteral("entryId")).toString().isEmpty()
            || metadata.value(QStringLiteral("entryId"))
                    != base.value(QStringLiteral("entryId"))
            || !metadataIndexOK
            || metadataIndex != base.value(QStringLiteral("index")).toInt()) {
            failPanelCatalogMetadataRequest(side, true);
            return;
        }
    }

    const bool streamFinal = state.metadataPendingRanges.size() == 1
        && state.metadataPendingRanges.constFirst().begin == requestOffset
        && state.metadataPendingRanges.constFirst().end == endOffset;

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_sessions[static_cast<size_t>(side)].data());
    const bool traceStages = F4NavigationBenchmarkTrace::enabled();
    QVariant metadataTraceId = traceStages
        ? F4NavigationBenchmarkTrace::benchmarkTraceId(message) : QVariant();
    if (!metadataTraceId.isValid()) {
        metadataTraceId = m_lastInputSceneTraceId;
    }
    const qint64 normalizeStartedNs = traceStages
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    const QVariantList entries = normalizedMetadataEntries(
        side, requestOffset, sourceEntries, stylesValue.toMap());
    const qint64 normalizeCompletedNs = traceStages
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    const qint64 modelApplyStartedNs = normalizeCompletedNs;
    if (!session || !session->applyExternalMetadata(
            entries, state.catalogRevision, state.metadataRevision,
            streamFinal)) {
        failPanelCatalogMetadataRequest(side, false);
        return;
    }
    const qint64 modelApplyCompletedNs = traceStages
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    if (traceStages) {
        const QVariantMap fields{
            {QStringLiteral("side"), side},
            {QStringLiteral("offset"), requestOffset},
            {QStringLiteral("rows"), entries.size()},
            {QStringLiteral("serverFinal"), final},
            {QStringLiteral("streamFinal"), streamFinal},
            {QStringLiteral("normalizeDurationNs"),
             normalizeCompletedNs - normalizeStartedNs},
            {QStringLiteral("modelApplyDurationNs"),
             modelApplyCompletedNs - modelApplyStartedNs},
            {QStringLiteral("durationNs"),
             modelApplyCompletedNs - normalizeStartedNs},
        };
        F4NavigationBenchmarkTrace::eventAt(
            QStringLiteral("qt.gallery.bridge.metadata.applied"),
            modelApplyCompletedNs, metadataTraceId, fields);
    }

    if (!consumePanelCatalogMetadataRange(side, requestOffset, endOffset)) {
        failPanelCatalogMetadataRequest(side, false);
        return;
    }
    state.metadataRequestInFlight = false;
    state.metadataRequestOffset = -1;
    state.metadataRequestLimit = 0;
    state.metadataFailureCount = 0;
    state.highlightRevision = highlightRevision;
    // The applied chunk can invalidate many visible icon/image bindings. Do
    // not enqueue the next response until this exact mutation has reached a
    // synchronized rendered frame; otherwise QTimer(0) chains monopolize the
    // GUI queue and delay keyboard input behind metadata work.
    state.metadataAwaitingFrame = true;
    state.metadataRequiredRenderSyncSerial =
        m_renderSyncSerial.load(std::memory_order_acquire) + 1;
    const qulonglong pacedGeneration = ++state.metadataPacingGeneration;
    const QString pacedPanelId = state.panelId;
    const QString pacedPath = state.currentPath;
    const qulonglong pacedCatalogRevision = state.catalogRevision;
    const qulonglong pacedMetadataRevision = state.metadataRevision;
    QTimer::singleShot(CatalogMetadataFrameFallbackMs, this,
                      [this, side, pacedPanelId, pacedPath,
                       pacedCatalogRevision, pacedMetadataRevision,
                       pacedGeneration]() {
        if (!validSide(side)) {
            return;
        }
        SideState &paced = m_states[static_cast<size_t>(side)];
        // A hidden/offscreen panel may not produce frameSwapped. Yield one
        // nominal frame, then release only the exact stream/chunk that armed
        // this fallback; a navigation or newer response makes it a no-op.
        if (!paced.metadataAwaitingFrame
            || paced.metadataRequestInFlight
            || paced.panelId != pacedPanelId
            || paced.currentPath != pacedPath
            || paced.catalogRevision != pacedCatalogRevision
            || paced.metadataRevision != pacedMetadataRevision
            || paced.metadataPacingGeneration != pacedGeneration) {
            return;
        }
        paced.metadataAwaitingFrame = false;
        paced.metadataRequiredRenderSyncSerial = 0;
        schedulePanelCatalogMetadataRequest();
    });
    if (streamFinal) {
        state.metadataComplete = true;
        schedulePanelCatalogMetadataRequest();
        return;
    }
    schedulePanelCatalogMetadataRequest();
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
        const SideState &state = m_states[static_cast<size_t>(side)];
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

void F4GalleryBridge::reportMetadataVisibleRange(
    int side, int firstRow, int lastRow, qulonglong catalogRevision)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_states[static_cast<size_t>(side)];
    if (!state.initialized || !state.metadataDeferred
        || state.metadataComplete || state.entries.isEmpty()
        || (catalogRevision != 0
            && catalogRevision != state.catalogRevision)
        || firstRow < 0 || lastRow < firstRow) {
        return;
    }

    const int entryCount = static_cast<int>(state.entries.size());
    const int boundedFirst = qBound(0, firstRow, entryCount - 1);
    const int boundedLast = qBound(
        boundedFirst, lastRow, entryCount - 1);
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
    SideState &state = m_states[static_cast<size_t>(side)];
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
        const int entryCount = static_cast<int>(state.entries.size());
        state.metadataPendingRanges.push_back({0, entryCount});
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
    const SideState &state = m_states[static_cast<size_t>(side)];
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
    // Details row, or viewer intent. Then drain the currently reported
    // viewport window before returning to the earliest remaining gap.
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

    // Once the input-time urgent budget is spent, keep the same visible-first
    // order when idle; the caller classifies these remaining chunks as bulk.
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

    const MetadataRange &range = state.metadataPendingRanges.constFirst();
    *offset = range.begin;
    *limit = qMin(CatalogMetadataChunkLimit, range.end - range.begin);
    return *limit > 0;
}

bool F4GalleryBridge::consumePanelCatalogMetadataRange(
    int side, int offset, int end)
{
    if (!validSide(side) || offset < 0 || end <= offset) {
        return false;
    }
    SideState &state = m_states[static_cast<size_t>(side)];
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
    SideState &state = m_states[static_cast<size_t>(side)];
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
    SideState &state = m_states[static_cast<size_t>(side)];
    const int entryCount = static_cast<int>(state.entries.size());
    if (!state.initialized || !state.metadataDeferred
        || state.metadataComplete || row < 0 || row >= entryCount) {
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

void F4GalleryBridge::requestPanelCatalogMetadata(int side)
{
    if (!validSide(side)) {
        return;
    }
    SideState &state = m_states[static_cast<size_t>(side)];
    if (!state.initialized || !state.metadataDeferred
        || state.metadataComplete || state.metadataRequestInFlight
        || state.metadataAwaitingFrame || state.loading
        || state.panelId.isEmpty()) {
        return;
    }
    int offset = -1;
    int limit = 0;
    bool urgent = false;
    if (!choosePanelCatalogMetadataRange(
            side, &offset, &limit, &urgent)) {
        state.metadataComplete = true;
        state.metadataPendingRanges.clear();
        schedulePanelCatalogMetadataRequest();
        return;
    }
    if (m_metadataInputBusy && !urgent) {
        return;
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
}

void F4GalleryBridge::requestNextPanelCatalogMetadata()
{
    // Keep only one catalog metadata transaction in flight globally. The
    // active panel is always drained first; the inactive side starts only
    // once the active stream is complete (or itself becomes active).
    for (const SideState &state : std::as_const(m_states)) {
        if (state.metadataRequestInFlight || state.metadataAwaitingFrame) {
            return;
        }
    }
    for (int priority = 0; priority < 2; ++priority) {
        for (int side = 0; side < 2; ++side) {
            const SideState &state = m_states[static_cast<size_t>(side)];
            if ((priority == 0) != state.active
                || !state.initialized || !state.metadataDeferred
                || state.metadataComplete || state.metadataAwaitingFrame
                || state.loading) {
                continue;
            }
            requestPanelCatalogMetadata(side);
            return;
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

bool F4GalleryBridge::validSide(int side)
{
    return side == 0 || side == 1;
}

QVariantList F4GalleryBridge::panelsFromScene(const QVariantMap &scene)
{
    return shellFromScene(scene).value(QStringLiteral("panels")).toList();
}

QVariantList F4GalleryBridge::normalizedEntries(
    const QVariantMap &panel) const
{
    const QVariantList sourceEntries = panel.value(QStringLiteral("entries")).toList();
    const QVariantMap styles = panel.value(QStringLiteral("highlightStyles")).toMap();
    const bool metadataDeferred = panel.value(
        QStringLiteral("metadataDeferred")).toBool();
    const qreal devicePixelRatio = availableDevicePixelRatio();
    QVariantList entries;
    entries.reserve(sourceEntries.size());
    for (qsizetype row = 0; row < sourceEntries.size(); ++row) {
        const QVariantMap source = sourceEntries[row].toMap();
        QVariantMap entry;
        entry.insert(QStringLiteral("entryId"), source.value(QStringLiteral("entryId")));
        entry.insert(QStringLiteral("index"), source.value(QStringLiteral("index"), row));
        entry.insert(QStringLiteral("name"), source.value(QStringLiteral("name")));
        // Older/legacy semantic producers do not have the preformatted name
        // roles.  Omitting an absent value lets ImageFile derive it from
        // `name`; inserting an invalid QVariant would instead suppress that
        // fallback and leave Columns/Details labels empty.
        if (source.contains(QStringLiteral("displayBaseName"))) {
            entry.insert(QStringLiteral("displayBaseName"),
                         source.value(QStringLiteral("displayBaseName")));
        }
        if (source.contains(QStringLiteral("displayExtension"))) {
            entry.insert(QStringLiteral("displayExtension"),
                         source.value(QStringLiteral("displayExtension")));
        }
        if (source.contains(QStringLiteral("localPath"))) {
            entry.insert(QStringLiteral("localPath"),
                         source.value(QStringLiteral("localPath")));
        } else if (!metadataDeferred
                   && source.contains(QStringLiteral("path"))) {
            // Legacy complete catalogs may still use `path`. Deferred base
            // rows intentionally have no filesystem identity until their
            // authoritative metadata chunk arrives.
            entry.insert(QStringLiteral("localPath"),
                         source.value(QStringLiteral("path")));
        }
        entry.insert(QStringLiteral("isDir"), source.value(QStringLiteral("isDir")));
        entry.insert(QStringLiteral("isUp"), source.value(QStringLiteral("isUp")));
        if (source.contains(QStringLiteral("isHidden"))) {
            entry.insert(QStringLiteral("isHidden"),
                         source.value(QStringLiteral("isHidden")));
        }
        if (source.contains(QStringLiteral("isImage"))) {
            entry.insert(QStringLiteral("isImage"), source.value(QStringLiteral("isImage")));
        }
        entry.insert(QStringLiteral("selected"), source.value(QStringLiteral("selected")));
        if (source.contains(QStringLiteral("mtimeNanos"))) {
            entry.insert(QStringLiteral("mtimeNs"),
                         source.value(QStringLiteral("mtimeNanos")));
        }
        for (const QString &key : {QStringLiteral("size"),
                                   QStringLiteral("sizeText"),
                                   QStringLiteral("sizeCalculated")}) {
            if (source.contains(key)) {
                entry.insert(key, source.value(key));
            }
        }
        if (source.contains(QStringLiteral("mtime"))) {
            entry.insert(QStringLiteral("mtimeText"),
                         source.value(QStringLiteral("mtime")));
        }
        if (source.contains(QStringLiteral("mode"))) {
            entry.insert(QStringLiteral("modeText"),
                         source.value(QStringLiteral("mode")));
        }
        if (source.contains(QStringLiteral("highlightStyleId"))) {
            entry.insert(QStringLiteral("highlightStyleId"),
                         source.value(QStringLiteral("highlightStyleId")));
        }
        const QString styleId = source.value(QStringLiteral("highlightStyleId")).toString();
        QVariantMap style;
        if (!styleId.isEmpty()) {
            style = styles.value(styleId).toMap();
        }

        if (m_iconSet) {
            const QString configuredIcon =
                style.value(QStringLiteral("icon")).toString();
            const bool bundledLucideIcon =
                isF4BundledLucideIcon(configuredIcon);
            const bool replaceableIcon = configuredIcon.isEmpty()
                || isZoinGalleryDefaultIcon(configuredIcon)
                || isF4SystemFileIcon(configuredIcon,
                                      m_iconSet->providerId())
                || (m_iconSet->system() && bundledLucideIcon);
            const bool hasMarker = !style.value(
                QStringLiteral("marker")).toString().isEmpty();

            // User-supplied URLs are appearance overrides and stay intact.
            // An explicitly configured bundled Lucide icon is also an
            // override while Lucide is active. Under System it becomes a
            // replaceable default, so a marker can suppress it (a colored
            // native icon and a colored marker glyph read as redundant);
            // otherwise it is translated to the equivalent native file icon.
            // Under Lucide the icon is always a thin monochrome glyph that
            // combines fine with a marker, so a marker must not suppress it
            // there.
            if (replaceableIcon) {
                if (hasMarker && m_iconSet->system()) {
                    style.remove(QStringLiteral("icon"));
                } else {
                    const bool isUp = source.value(
                        QStringLiteral("isUp")).toBool();
                    const bool directory = isUp || source.value(
                        QStringLiteral("isDir")).toBool();
                    QString fileName = source.value(
                        QStringLiteral("name")).toString();
                    if (isUp) {
                        fileName = QStringLiteral("..");
                    }
                    // The deferred base catalog deliberately has no stable
                    // filesystem identity yet.  In the System set, make all
                    // such rows share just the native generic file/folder
                    // source rather than enqueueing one shell/MIME lookup per
                    // filename.  QML retains that already-cached generic
                    // image while the metadata stream replaces it with the
                    // precise path-aware shell icon.
                    const bool genericSystemIcon = metadataDeferred
                        && m_iconSet->system();
                    const QUrl iconSource = m_iconSet->fileIconSource(
                        genericSystemIcon ? QString()
                                          : entry.value(
                                                QStringLiteral("localPath"))
                                                .toString(),
                        genericSystemIcon ? QString() : fileName,
                        directory,
                        GalleryIconLogicalSize,
                        devicePixelRatio,
                        genericSystemIcon ? 0 : source.value(
                                                QStringLiteral("mtimeNanos"))
                                                .toULongLong());
                    style.insert(QStringLiteral("icon"),
                                 iconSource.toString());
                }
            }
        }
        entry.insert(QStringLiteral("highlightStyle"), style);
        entries.push_back(entry);
    }
    return entries;
}

QVariantList F4GalleryBridge::normalizedMetadataEntries(
    int side, int offset, const QVariantList &sourceEntries,
    const QVariantMap &highlightStyles) const
{
    QVariantList entries;
    if (!validSide(side)) {
        return entries;
    }
    const SideState &state = m_states[static_cast<size_t>(side)];
    entries.reserve(sourceEntries.size());
    const qreal devicePixelRatio = availableDevicePixelRatio();
    for (qsizetype index = 0; index < sourceEntries.size(); ++index) {
        const QVariantMap source = sourceEntries.at(index).toMap();
        const QVariantMap base = state.entries.at(
            offset + index).toMap();
        QVariantMap entry = source;
        if (source.contains(QStringLiteral("mtimeNanos"))) {
            entry.insert(QStringLiteral("mtimeNs"),
                         source.value(QStringLiteral("mtimeNanos")));
        }
        if (source.contains(QStringLiteral("mtime"))) {
            entry.insert(QStringLiteral("mtimeText"),
                         source.value(QStringLiteral("mtime")));
        }
        if (source.contains(QStringLiteral("mode"))) {
            entry.insert(QStringLiteral("modeText"),
                         source.value(QStringLiteral("mode")));
        }

        const QString styleId = source.value(
            QStringLiteral("highlightStyleId")).toString();
        QVariantMap style;
        if (!styleId.isEmpty()) {
            style = highlightStyles.value(styleId).toMap();
        }
        if (m_iconSet) {
            const QString configuredIcon = style.value(
                QStringLiteral("icon")).toString();
            const bool bundledLucideIcon =
                isF4BundledLucideIcon(configuredIcon);
            const bool replaceableIcon = configuredIcon.isEmpty()
                || isZoinGalleryDefaultIcon(configuredIcon)
                || isF4SystemFileIcon(configuredIcon,
                                      m_iconSet->providerId())
                || (m_iconSet->system() && bundledLucideIcon);
            const bool hasMarker = !style.value(
                QStringLiteral("marker")).toString().isEmpty();
            if (replaceableIcon) {
                if (hasMarker && m_iconSet->system()) {
                    style.remove(QStringLiteral("icon"));
                } else {
                    const bool isUp = base.value(
                        QStringLiteral("isUp")).toBool();
                    const bool directory = isUp || base.value(
                        QStringLiteral("isDir")).toBool();
                    QString fileName = base.value(
                        QStringLiteral("name")).toString();
                    if (isUp) {
                        fileName = QStringLiteral("..");
                    }
                    const QUrl iconSource = m_iconSet->fileIconSource(
                        entry.value(QStringLiteral("localPath")).toString(),
                        fileName, directory, GalleryIconLogicalSize,
                        devicePixelRatio,
                        source.value(QStringLiteral("mtimeNanos"))
                            .toULongLong());
                    style.insert(QStringLiteral("icon"),
                                 iconSource.toString());
                }
            }
        }
        entry.insert(QStringLiteral("highlightStyle"), style);
        entries.push_back(entry);
    }
    return entries;
}

void F4GalleryBridge::refreshDeferredIconAppearance(int side)
{
    if (!validSide(side) || !m_iconSet) {
        return;
    }
    const SideState &state = m_states[static_cast<size_t>(side)];
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_sessions[static_cast<size_t>(side)].data());
    if (!session || !state.initialized || !state.metadataDeferred
        || !state.metadataComplete) {
        return;
    }

    const qreal devicePixelRatio = availableDevicePixelRatio();
    QVariantList appearance;
    appearance.reserve(state.entries.size());
    for (qsizetype row = 0; row < state.entries.size(); ++row) {
        const QVariantMap base = state.entries.at(row).toMap();
        QVariantMap style = session->highlightStyleAt(row);
        const QString configuredIcon = style.value(
            QStringLiteral("icon")).toString();
        const bool bundledLucideIcon =
            isF4BundledLucideIcon(configuredIcon);
        const bool replaceableIcon = configuredIcon.isEmpty()
            || isZoinGalleryDefaultIcon(configuredIcon)
            || isF4SystemFileIcon(configuredIcon,
                                  m_iconSet->providerId())
            || (m_iconSet->system() && bundledLucideIcon);
        const bool hasMarker = !style.value(
            QStringLiteral("marker")).toString().isEmpty();
        if (replaceableIcon) {
            if (hasMarker && m_iconSet->system()) {
                style.remove(QStringLiteral("icon"));
            } else {
                const bool isUp = base.value(
                    QStringLiteral("isUp")).toBool();
                const bool directory = isUp
                    || session->isDirectoryAt(row);
                QString fileName = session->entryNameAt(row);
                if (isUp) {
                    fileName = QStringLiteral("..");
                }
                style.insert(QStringLiteral("icon"),
                             m_iconSet->fileIconSource(
                                 session->localPathAt(row), fileName,
                                 directory, GalleryIconLogicalSize,
                                 devicePixelRatio).toString());
            }
        }
        appearance.push_back(QVariantMap{
            {QStringLiteral("entryId"), session->entryIdAt(row)},
            {QStringLiteral("highlightStyle"), style},
        });
    }
    session->applyExternalAppearance(appearance, state.highlightRevision);
}

QStringList F4GalleryBridge::selectedEntryIds(const QVariantList &entries)
{
    QStringList ids;
    for (const QVariant &entryValue : entries) {
        const QVariantMap entry = entryValue.toMap();
        if (entry.value(QStringLiteral("selected")).toBool()) {
            const QString id = entry.value(QStringLiteral("entryId")).toString();
            if (!id.isEmpty()) {
                ids.push_back(id);
            }
        }
    }
    return ids;
}

int F4GalleryBridge::sourceIndexForEntryId(const QVariantList &entries,
                                           const QString &entryId)
{
    for (qsizetype row = 0; row < entries.size(); ++row) {
        const QVariantMap entry = entries.at(row).toMap();
        if (entry.value(QStringLiteral("entryId")).toString() == entryId) {
            return entry.value(QStringLiteral("index"), row).toInt();
        }
    }
    return -1;
}

qulonglong F4GalleryBridge::revisionValue(const QVariantMap &map, const QString &key)
{
    bool ok = false;
    const qulonglong value = map.value(key).toULongLong(&ok);
    return ok ? value : 0;
}

QObject *F4GalleryBridge::createPanelSession()
{
    auto *runtime = qobject_cast<ZoinGallery::GalleryRuntime *>(m_runtime.data());
    if (!runtime) {
        return nullptr;
    }
    QObject *session = runtime->createExternalSession(
        QStringLiteral("f4-panel-cache-%1").arg(++m_nextPanelSessionId), this);
    if (session) {
        m_allSessions.insert(session);
    }
    return session;
}

void F4GalleryBridge::cacheCurrentPanel(int side)
{
    if (!validSide(side)) {
        return;
    }
    const size_t index = static_cast<size_t>(side);
    const SideState &state = m_states[index];
    if (!state.initialized || state.panelId.isEmpty()) {
        return;
    }
    CachedPanel cached;
    cached.session = m_sessions[index];
    cached.state = state;
    cached.snapshot = m_panelSnapshots[index];
    m_panelCache.insert(state.panelId, std::move(cached));
}

bool F4GalleryBridge::activatePanelSession(int side,
                                           const QString &panelId)
{
    if (!validSide(side) || panelId.isEmpty()) {
        return false;
    }
    const size_t index = static_cast<size_t>(side);
    if (m_states[index].initialized
        && m_states[index].panelId == panelId) {
        cacheCurrentPanel(side);
        return true;
    }

    cacheCurrentPanel(side);
    const auto cached = m_panelCache.constFind(panelId);
    if (cached != m_panelCache.cend()) {
        m_sessions[index] = cached->session;
        m_states[index] = cached->state;
        m_panelSnapshots[index] = cached->snapshot;
        return true;
    }

    // The two constructor sessions are the cold slots for the first visible
    // pair. Every later identity receives its own retained model so preparing
    // it cannot reset a session which is still painted by QML.
    QPointer<QObject> session = !m_cacheWarmup
            && !m_states[index].initialized
            && m_panelSnapshots[index].isEmpty()
        ? m_sessions[index] : QPointer<QObject>(createPanelSession());
    m_sessions[index] = session;
    m_states[index] = SideState{};
    m_panelSnapshots[index].clear();
    CachedPanel empty;
    empty.session = session;
    m_panelCache.insert(panelId, std::move(empty));
    return true;
}

bool F4GalleryBridge::applyCachedPanelMetadata(
    int side, const QVariantMap &metadata)
{
    if (!validSide(side) || metadata.isEmpty()
        || metadata.value(QStringLiteral("type")).toString()
            != QStringLiteral("panel_catalog_metadata")) {
        return false;
    }

    SideState &state = m_states[static_cast<size_t>(side)];
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_sessions[static_cast<size_t>(side)].data());
    bool catalogRevisionOK = false;
    bool metadataRevisionOK = false;
    bool highlightRevisionOK = false;
    bool offsetOK = false;
    bool limitOK = false;
    bool totalOK = false;
    const qulonglong catalogRevision = metadata.value(
        QStringLiteral("catalogRevision")).toULongLong(
            &catalogRevisionOK);
    const qulonglong metadataRevision = metadata.value(
        QStringLiteral("metadataRevision")).toULongLong(
            &metadataRevisionOK);
    const qulonglong highlightRevision = metadata.value(
        QStringLiteral("highlightRevision")).toULongLong(
            &highlightRevisionOK);
    const int offset = metadata.value(
        QStringLiteral("offset")).toInt(&offsetOK);
    const int limit = metadata.value(
        QStringLiteral("limit")).toInt(&limitOK);
    const int total = metadata.value(
        QStringLiteral("total")).toInt(&totalOK);
    const QVariant entriesValue = metadata.value(QStringLiteral("entries"));
    const QVariant stylesValue = metadata.value(
        QStringLiteral("highlightStyles"));
    if (!session || !state.initialized || !state.metadataDeferred
        || state.metadataComplete || !catalogRevisionOK
        || !metadataRevisionOK || !highlightRevisionOK || !offsetOK
        || !limitOK || !totalOK || offset < 0 || limit <= 0
        || limit > CatalogMetadataWarmupLimit
        || metadata.value(QStringLiteral("panelId")).toString()
            != state.panelId
        || metadata.value(QStringLiteral("path")).toString()
            != state.currentPath
        || catalogRevision != state.catalogRevision
        || metadataRevision != state.metadataRevision
        || total != state.entries.size()
        || entriesValue.metaType().id() != QMetaType::QVariantList
        || (metadata.contains(QStringLiteral("highlightStyles"))
            && stylesValue.metaType().id() != QMetaType::QVariantMap)
        || !metadata.contains(QStringLiteral("totalSize"))
        || !metadata.contains(QStringLiteral("final"))) {
        return false;
    }

    const QVariantList sourceEntries = entriesValue.toList();
    const int endOffset = offset + sourceEntries.size();
    const bool serverFinal = metadata.value(
        QStringLiteral("final")).toBool();
    if (sourceEntries.size() != limit || endOffset > total
        || serverFinal != (endOffset == total)) {
        return false;
    }
    for (qsizetype index = 0; index < sourceEntries.size(); ++index) {
        if (sourceEntries.at(index).metaType().id()
            != QMetaType::QVariantMap) {
            return false;
        }
        const QVariantMap row = sourceEntries.at(index).toMap();
        const QVariantMap base = state.entries.at(offset + index).toMap();
        bool rowIndexOK = false;
        const int rowIndex = row.value(
            QStringLiteral("index")).toInt(&rowIndexOK);
        if (!rowIndexOK
            || row.value(QStringLiteral("entryId")).toString().isEmpty()
            || row.value(QStringLiteral("entryId"))
                != base.value(QStringLiteral("entryId"))
            || rowIndex
                != base.value(QStringLiteral("index")).toInt()) {
            return false;
        }
    }

    bool rangePending = false;
    for (const MetadataRange &range : std::as_const(
             state.metadataPendingRanges)) {
        if (offset >= range.begin && endOffset <= range.end) {
            rangePending = true;
            break;
        }
    }
    if (!rangePending) {
        return false;
    }

    const bool streamFinal = state.metadataPendingRanges.size() == 1
        && state.metadataPendingRanges.constFirst().begin == offset
        && state.metadataPendingRanges.constFirst().end == endOffset;
    const QVariantList entries = normalizedMetadataEntries(
        side, offset, sourceEntries, stylesValue.toMap());
    if (!session->applyExternalMetadata(
            entries, state.catalogRevision, state.metadataRevision,
            streamFinal)
        || !consumePanelCatalogMetadataRange(side, offset, endOffset)) {
        return false;
    }

    state.highlightRevision = highlightRevision;
    state.metadataRequestInFlight = false;
    state.metadataRequestOffset = -1;
    state.metadataRequestLimit = 0;
    state.metadataFailureCount = 0;
    state.metadataComplete = state.metadataPendingRanges.isEmpty();
    if (state.metadataComplete) {
        state.metadataAwaitingFrame = false;
        state.metadataRequiredRenderSyncSerial = 0;
    } else {
        // Do not let the remaining off-screen stream race the first reveal.
        // Once this retained session becomes current, its first synchronized
        // frame releases the normal viewport-prioritized request planner.
        state.metadataAwaitingFrame = true;
        state.metadataRequiredRenderSyncSerial =
            m_renderSyncSerial.load(std::memory_order_acquire) + 1;
    }
    F4NavigationBenchmarkTrace::event(
        QStringLiteral("qt.gallery.workspace.cache_metadata_applied"), {}, {
            {QStringLiteral("side"), side},
            {QStringLiteral("panelId"), state.panelId},
            {QStringLiteral("offset"), offset},
            {QStringLiteral("rows"), entries.size()},
            {QStringLiteral("streamFinal"), streamFinal},
        });
    return true;
}

void F4GalleryBridge::synchronizePanelCache(
    const QVariantMap &panel, const QVariantMap &metadata)
{
    bool sideOK = false;
    const int side = panel.value(QStringLiteral("side")).toInt(&sideOK);
    const QString panelId = panel.value(QStringLiteral("id")).toString();
    const qulonglong catalogRevision = revisionValue(
        panel, QStringLiteral("catalogRevision"));
    if (!sideOK || !validSide(side) || panelId.isEmpty()
        || catalogRevision == 0
        || !panel.contains(QStringLiteral("entries"))) {
        return;
    }
    const size_t index = static_cast<size_t>(side);
    if (m_states[index].initialized
        && m_states[index].panelId == panelId) {
        // A cache message is never authoritative for the visible scene. The
        // normal scene/catalog protocol owns any revision advance there.
        applyCachedPanelMetadata(side, metadata);
        cacheCurrentPanel(side);
        emit panelCachePrepared(side, rowFreePanelPresentation(panel));
        return;
    }

    cacheCurrentPanel(side);
    const QPointer<QObject> activeSession = m_sessions[index];
    const SideState activeState = m_states[index];
    const QVariantMap activeSnapshot = m_panelSnapshots[index];
    const bool reconciliationPending =
        m_stateReconciliationPending[index];
    const bool selectionPending = m_selectionActionPending[index];

    // An off-screen warmup must never borrow the constructor's cold side slot.
    // QML may already have observed that slot while building the startup shell;
    // cache preparation always receives an identity-owned session instead.
    m_cacheWarmup = true;
    activatePanelSession(side, panelId);
    m_stateReconciliationPending[index] = false;
    m_selectionActionPending[index] = false;
    synchronizePanel(side, panel);
    applyCachedPanelMetadata(side, metadata);
    m_cacheWarmup = false;
    cacheCurrentPanel(side);

    m_sessions[index] = activeSession;
    m_states[index] = activeState;
    m_panelSnapshots[index] = activeSnapshot;
    m_stateReconciliationPending[index] = reconciliationPending;
    m_selectionActionPending[index] = selectionPending;
    emit panelCachePrepared(side, rowFreePanelPresentation(panel));
}

bool F4GalleryBridge::synchronizeWorkspaceShell(const QVariantMap &shell)
{
    const QVariant panelsValue = shell.value(QStringLiteral("panels"));
    if (panelsValue.metaType().id() != QMetaType::QVariantList) {
        return false;
    }
    std::array<QVariantMap, 2> snapshots;
    std::array<bool, 2> found = {false, false};
    for (const QVariant &value : panelsValue.toList()) {
        if (value.metaType().id() != QMetaType::QVariantMap) {
            return false;
        }
        const QVariantMap header = value.toMap();
        bool sideOK = false;
        const int side = header.value(QStringLiteral("side")).toInt(&sideOK);
        const QString panelId = header.value(QStringLiteral("id")).toString();
        if (!sideOK || !validSide(side) || panelId.isEmpty()) {
            return false;
        }
        const size_t index = static_cast<size_t>(side);
        cacheCurrentPanel(side);
        const auto cached = m_panelCache.constFind(panelId);
        if (cached == m_panelCache.cend()
            || !cached->state.initialized
            || cached->state.catalogRevision != revisionValue(
                header, QStringLiteral("catalogRevision"))
            || cached->state.selectionRevision != revisionValue(
                header, QStringLiteral("selectionRevision"))
            || cached->state.currentPath
                != header.value(QStringLiteral("path")).toString()
            || !cached->snapshot.contains(QStringLiteral("entries"))) {
            F4NavigationBenchmarkTrace::event(
                QStringLiteral("qt.gallery.workspace.cache_miss"),
                m_lastInputSceneTraceId, {
                    {QStringLiteral("side"), side},
                    {QStringLiteral("panelId"), panelId},
                    {QStringLiteral("catalogRevision"),
                     header.value(QStringLiteral("catalogRevision"))},
                });
            return false;
        }
        QVariantMap snapshot = cached->snapshot;
        for (auto it = header.cbegin(); it != header.cend(); ++it) {
            if (it.key() != QStringLiteral("entries")
                && it.key() != QStringLiteral("highlightStyles")) {
                snapshot.insert(it.key(), it.value());
            }
        }
        snapshots[index] = std::move(snapshot);
        found[index] = true;
    }
    if (!found[0] || !found[1]) {
        return false;
    }

    // Validation above is all-or-nothing. Only after both exact catalog
    // tuples are known do we change either active side, so QML can swap the
    // complete pair on one compact presentation notification.
    synchronizePanel(0, snapshots[0]);
    synchronizePanel(1, snapshots[1]);
    schedulePanelCatalogMetadataRequest();
    return true;
}

bool F4GalleryBridge::canSkipUnchangedInactivePanel(
    int side, const QVariantMap &panel) const
{
    if (!validSide(side)) {
        return false;
    }

    const size_t sideIndex = static_cast<size_t>(side);
    const SideState &state = m_states[sideIndex];
    if (!state.initialized || state.active
        || panel.value(QStringLiteral("active")).toBool()
        || m_stateReconciliationPending[sideIndex]
        || m_selectionActionPending[sideIndex]
        || m_pendingCursors[sideIndex].active
        || m_pendingSelections[sideIndex].active
        || (m_pendingPanelOpen.active && m_pendingPanelOpen.side == side)
        || (m_pendingViewer.active && m_pendingViewer.side == side)) {
        return false;
    }

    const QString sourceKind = panel.value(
        QStringLiteral("sourceKind"), QStringLiteral("vfs")).toString();
    const bool previewCapable = panel.value(
        QStringLiteral("previewCapable")).toBool()
        && sourceKind == QStringLiteral("local");
    const bool metadataDeferred = panel.value(
        QStringLiteral("metadataDeferred")).toBool();
    const qulonglong iconRevision = m_iconSet ? m_iconSet->revision() : 0;
    return panel.value(QStringLiteral("id")).toString() == state.panelId
        && revisionValue(panel, QStringLiteral("catalogRevision"))
            == state.catalogRevision
        && revisionValue(panel, QStringLiteral("selectionRevision"))
            == state.selectionRevision
        && metadataDeferred == state.metadataDeferred
        && (!metadataDeferred
            ? revisionValue(panel, QStringLiteral("highlightRevision"))
                == state.highlightRevision
            : revisionValue(panel, QStringLiteral("metadataRevision"))
                == state.metadataRevision)
        && iconRevision == state.iconRevision
        && panel.value(QStringLiteral("path")).toString()
            == state.currentPath
        && sourceKind == state.sourceKind
        && previewCapable == state.previewCapable
        && panel.value(QStringLiteral("cursorEntryId")).toString()
            == state.cursorEntryId
        && panel.value(QStringLiteral("cursor"), -1).toInt()
            == state.cursorIndex
        && panel.value(QStringLiteral("loading")).toBool()
            == state.loading
        && panel.value(QStringLiteral("catalogProvisional")).toBool()
            == state.catalogProvisional
        && panel.value(QStringLiteral("galleryLayoutMode")).toString()
            == state.galleryLayoutMode;
}

void F4GalleryBridge::synchronizePanel(int side, const QVariantMap &panel)
{
    const size_t sideIndex = static_cast<size_t>(side);
    SideState &state = m_states[sideIndex];
    const QString panelId = panel.value(QStringLiteral("id")).toString();
    const qulonglong catalogRevision = revisionValue(panel, QStringLiteral("catalogRevision"));
    const qulonglong selectionRevision = revisionValue(panel, QStringLiteral("selectionRevision"));
    const qulonglong panelHighlightRevision = revisionValue(
        panel, QStringLiteral("highlightRevision"));
    const bool metadataDeferred = panel.value(
        QStringLiteral("metadataDeferred")).toBool();
    const qulonglong metadataRevision = revisionValue(
        panel, QStringLiteral("metadataRevision"));
    const qulonglong iconRevision = m_iconSet ? m_iconSet->revision() : 0;
    const QString currentPath = panel.value(QStringLiteral("path")).toString();
    const QString cursorEntryId = panel.value(QStringLiteral("cursorEntryId")).toString();
    const int cursorIndex = panel.value(QStringLiteral("cursor"), -1).toInt();
    const QString sourceKind = panel.value(QStringLiteral("sourceKind"), QStringLiteral("vfs")).toString();
    const bool previewCapable = panel.value(QStringLiteral("previewCapable")).toBool()
        && sourceKind == QStringLiteral("local");
    const bool active = panel.value(QStringLiteral("active")).toBool();
    const bool loading = panel.value(QStringLiteral("loading")).toBool();
    const bool catalogProvisional = panel.value(
        QStringLiteral("catalogProvisional")).toBool();
    const QString galleryLayoutMode = panel.value(
        QStringLiteral("galleryLayoutMode")).toString();
    const bool identityChanged = state.initialized && panelId != state.panelId;
    const bool provisionalReplacementDeferred = catalogProvisional
        && state.initialized && panelId == state.panelId
        && currentPath != state.currentPath;

    DeferredPanelOpenRepeat repeatToReplay;

    if (!m_cacheWarmup && m_inFlightPanelOpen.active
        && m_inFlightPanelOpen.side == side
        && (m_inFlightPanelOpen.panelId != panelId
            || (!provisionalReplacementDeferred
                && m_inFlightPanelOpen.sourcePath != currentPath))) {
        const bool authoritativePathAcknowledgement =
            m_inFlightPanelOpen.panelId == panelId
            && !provisionalReplacementDeferred
            && m_inFlightPanelOpen.sourcePath != currentPath;
        if (authoritativePathAcknowledgement
            && m_deferredPanelOpenRepeat.active
            && m_deferredPanelOpenRepeat.side == side
            && m_deferredPanelOpenRepeat.panelId
                == m_inFlightPanelOpen.panelId
            && m_deferredPanelOpenRepeat.sourcePath
                == m_inFlightPanelOpen.sourcePath
            && m_deferredPanelOpenRepeat.catalogRevision
                == m_inFlightPanelOpen.catalogRevision) {
            repeatToReplay = m_deferredPanelOpenRepeat;
        }
        clearInFlightPanelOpen();
    }

    if (!m_cacheWarmup && m_navigationBenchmark.enabled
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Finished
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Failed) {
        QVariantMap fields = navigationBenchmarkFields();
        fields.insert(QStringLiteral("syncSide"), side);
        fields.insert(QStringLiteral("syncPath"), currentPath);
        fields.insert(QStringLiteral("syncLoading"), loading);
        fields.insert(QStringLiteral("syncLayoutMode"), galleryLayoutMode);
        fields.insert(QStringLiteral("syncCatalogRevision"),
                      QVariant::fromValue<qulonglong>(catalogRevision));
        fields.insert(QStringLiteral("syncEntryCount"),
                      panel.value(QStringLiteral("entries")).toList().size());
        queueNavigationBenchmarkTrace(
            QStringLiteral("qt.gallery.bridge.panel.begin"),
            m_navigationBenchmark.lastSceneTraceId, fields);
    }

    if (identityChanged) {
        if (!m_cacheWarmup && m_viewerVisible && m_viewerSide == side) {
            closeViewer();
        }
        if (!m_cacheWarmup && m_pendingPanelOpen.active
            && m_pendingPanelOpen.side == side) {
            clearPendingPanelOpen();
        }
        if (!m_cacheWarmup) {
            clearPendingCursor(side);
            clearPendingSelection(side);
            m_selectionActionPending[sideIndex] = false;
        }
        activatePanelSession(side, panelId);
    }
    m_panelSnapshots[sideIndex] = panel;
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_sessions[sideIndex].data());

    // A cold VFS read exposes a temporary catalog while the authoritative
    // scene is being assembled. Keep the populated persistent session until
    // that scene arrives instead of replacing it with an incomplete catalog.
    if (provisionalReplacementDeferred) {
        state.active = active;
        state.loading = loading;
        state.galleryLayoutMode = galleryLayoutMode;
        if (!m_cacheWarmup && m_navigationBenchmark.enabled
            && m_navigationBenchmark.phase
                != NavigationBenchmarkPhase::Finished
            && m_navigationBenchmark.phase
                != NavigationBenchmarkPhase::Failed) {
            QVariantMap fields = navigationBenchmarkFields();
            fields.insert(QStringLiteral("syncSide"), side);
            fields.insert(QStringLiteral("syncPath"), state.currentPath);
            fields.insert(QStringLiteral("syncLoading"), state.loading);
            fields.insert(QStringLiteral("syncLayoutMode"),
                          state.galleryLayoutMode);
            fields.insert(QStringLiteral("syncCatalogRevision"),
                          QVariant::fromValue<qulonglong>(
                              state.catalogRevision));
            fields.insert(QStringLiteral("syncEntryCount"),
                          state.entries.size());
            fields.insert(QStringLiteral("provisionalReplacementDeferred"),
                          true);
            queueNavigationBenchmarkTrace(
                QStringLiteral("qt.gallery.bridge.panel.end"),
                m_navigationBenchmark.lastSceneTraceId, fields);
        }
        cacheCurrentPanel(side);
        return;
    }
    const bool catalogPayloadChanged = !state.initialized
        || catalogRevision != state.catalogRevision
        || currentPath != state.currentPath
        || sourceKind != state.sourceKind
        || previewCapable != state.previewCapable;
    const bool catalogChanged = catalogPayloadChanged
        || catalogProvisional != state.catalogProvisional;
    const bool metadataStreamChanged = !state.initialized
        || metadataDeferred != state.metadataDeferred
        || catalogRevision != state.catalogRevision
        || currentPath != state.currentPath
        || (metadataDeferred
            && metadataRevision != state.metadataRevision);
    const bool selectionChanged = !state.initialized
        || selectionRevision != state.selectionRevision;
    const bool highlightChanged = !metadataDeferred
        && (!state.initialized
            || panelHighlightRevision != state.highlightRevision);
    const bool iconChanged = !state.initialized
        || iconRevision != state.iconRevision;
    // A metadata stream is keyed by the exact panel/path/catalog/metadata
    // tuple. Re-applying the same base after final=true would re-advertise
    // metadataDeferred from Go, but must not restart the already completed
    // pull. An icon refresh may restart only an unfinished stream; Gallery's
    // session intentionally preserves completion for an identical tuple.
    const bool metadataRestartNeeded = metadataDeferred
        && (metadataStreamChanged
            || (iconChanged && !state.metadataComplete));
    if (metadataDeferred && iconChanged && state.initialized
        && state.metadataComplete && !metadataStreamChanged) {
        refreshDeferredIconAppearance(side);
    }
    // Semantic scenes retain the complete catalog for compatibility, but a
    // cursor acknowledgement does not need to normalize/copy it again. Keep
    // the revision-owned snapshot in the persistent bridge session.
    const bool appearanceChanged = !metadataDeferred
        && (highlightChanged || iconChanged);
    const bool traceCatalogStages = F4NavigationBenchmarkTrace::enabled();
    const QVariant catalogTraceId = m_lastInputSceneTraceId;
    const bool normalizeCatalog = catalogPayloadChanged || appearanceChanged;
    const qint64 normalizeStartedNs = traceCatalogStages && normalizeCatalog
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    // Normalize structural and appearance fields in one traversal. On a
    // catalog reset ExternalCatalogModel consumes highlightStyle before
    // endResetModel(), avoiding a second full pass and a post-reset update
    // storm.
    const QVariantList entries = normalizeCatalog
        ? normalizedEntries(panel) : state.entries;
    const qint64 normalizeCompletedNs = traceCatalogStages && normalizeCatalog
        ? F4NavigationBenchmarkTrace::monotonicNanoseconds() : 0;
    const QStringList selectedIds = catalogPayloadChanged || selectionChanged
        ? selectedEntryIds(catalogPayloadChanged
              ? entries
              : panel.value(QStringLiteral("entries")).toList())
        : state.selectedEntryIdList;

    bool catalogApplied = true;
    qint64 catalogApplyStartedNs = 0;
    qint64 catalogApplyCompletedNs = 0;
    if (session && (catalogChanged || metadataRestartNeeded)) {
        if (traceCatalogStages) {
            catalogApplyStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
        catalogApplied = session->applyExternalCatalog(entries, catalogRevision, {
            {QStringLiteral("currentPath"), currentPath},
            {QStringLiteral("sourceKind"), sourceKind},
            {QStringLiteral("previewCapable"), previewCapable},
            {QStringLiteral("catalogProvisional"), catalogProvisional},
            {QStringLiteral("metadataDeferred"), metadataDeferred},
            {QStringLiteral("metadataRevision"),
             QVariant::fromValue<qulonglong>(metadataRevision)},
            // The authoritative cursor is the second half of this update.
            // Keep Details hidden until both halves have been applied.
            {QStringLiteral("deferCatalogReady"),
             catalogPayloadChanged && !catalogProvisional},
        });
        if (traceCatalogStages) {
            catalogApplyCompletedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
    }

    if (session && !catalogChanged && appearanceChanged) {
        session->applyExternalAppearance(entries, panelHighlightRevision);
    }

    qint64 stateApplyStartedNs = 0;
    qint64 stateApplyCompletedNs = 0;
    if (session
        && (catalogChanged || selectionChanged
            || cursorEntryId != state.cursorEntryId || cursorIndex != state.cursorIndex
            || m_stateReconciliationPending[static_cast<size_t>(side)])) {
        QString appliedCursorEntryId = cursorEntryId;
        int appliedCursorIndex = cursorIndex;
        const PendingCursor &pendingCursor =
            m_pendingCursors[static_cast<size_t>(side)];
        const bool pendingCursorExists = catalogChanged
            ? sourceIndexForEntryId(entries, pendingCursor.entryId) >= 0
            : state.sourceIndexByEntryId.contains(pendingCursor.entryId);
        if (!m_cacheWarmup && pendingCursor.active
            && pendingCursor.panelId == panelId
            && pendingCursor.catalogRevision == catalogRevision
            && pendingCursor.entryId != cursorEntryId
            && pendingCursorExists) {
            // Clicking an inactive panel emits activate before cursor. The
            // activation-only scene still carries the old authoritative
            // cursor at the same catalog revision; keep the stable pending
            // cursor visible until its immediately-following action is
            // acknowledged instead of visibly snapping backward.
            appliedCursorEntryId = pendingCursor.entryId;
            appliedCursorIndex = pendingCursor.index;
        }
        if (traceCatalogStages) {
            stateApplyStartedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
        session->applyExternalState(appliedCursorEntryId,
                                    appliedCursorIndex,
                                    selectedIds,
                                    selectionRevision);
        if (traceCatalogStages) {
            stateApplyCompletedNs =
                F4NavigationBenchmarkTrace::monotonicNanoseconds();
        }
    }
    if (session && catalogChanged && catalogApplied) {
        session->setExternalCatalogReady(!catalogProvisional);
    }

    state.initialized = true;
    state.panelId = panelId;
    state.catalogRevision = catalogRevision;
    state.selectionRevision = selectionRevision;
    if (!metadataDeferred) {
        state.highlightRevision = panelHighlightRevision;
    }
    state.iconRevision = iconRevision;
    state.currentPath = currentPath;
    state.sourceKind = sourceKind;
    state.cursorEntryId = cursorEntryId;
    state.cursorIndex = cursorIndex;
    state.previewCapable = previewCapable;
    state.active = active;
    state.loading = loading;
    state.catalogProvisional = catalogProvisional;
    state.metadataDeferred = metadataDeferred;
    state.metadataRevision = metadataDeferred ? metadataRevision : 0;
    state.galleryLayoutMode = galleryLayoutMode;
    if (catalogPayloadChanged) {
        state.entries = entries;
        state.entryIds.clear();
        state.sourceIndexByEntryId.clear();
        for (qsizetype row = 0; row < entries.size(); ++row) {
            const QVariantMap entry = entries.at(row).toMap();
            const QString entryId =
                entry.value(QStringLiteral("entryId")).toString();
            if (entryId.isEmpty()) {
                continue;
            }
            state.entryIds.insert(entryId);
            state.sourceIndexByEntryId.insert(
                entryId,
                entry.value(QStringLiteral("index"), row).toInt());
        }
        if (m_inFlightPanelOpen.active
            && m_inFlightPanelOpen.side == side
            && !state.entryIds.contains(m_inFlightPanelOpen.entryId)) {
            clearInFlightPanelOpen();
        }
    }
    else if (appearanceChanged) {
        state.entries = entries;
    }
    if (metadataRestartNeeded) {
        if (session && catalogApplied) {
            // A new base catalog must reach one synchronized frame before
            // auxiliary mutations begin. A metadata-only revision update
            // keeps the current model and can request its cursor range now.
            resetPanelCatalogMetadataPlan(side, catalogPayloadChanged);
        }
        else {
            state.metadataRequestInFlight = false;
            state.metadataAwaitingFrame = false;
            state.metadataRequiredRenderSyncSerial = 0;
            state.metadataComplete = true;
            state.metadataRequestOffset = -1;
            state.metadataRequestLimit = 0;
            state.metadataPendingRanges.clear();
        }
    }
    else if (!metadataDeferred) {
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
    if (catalogChanged || selectionChanged) {
        state.selectedEntryIdList = selectedIds;
        state.selectedEntryIds = QSet<QString>(selectedIds.cbegin(),
                                               selectedIds.cend());
    }
    if (!m_cacheWarmup) {
        m_stateReconciliationPending[sideIndex] = false;
        reconcilePendingCursor(side);
        reconcilePendingPanelOpen(side);
        reconcilePendingSelection(side);
        reconcilePendingViewer(side);
    }

    cacheCurrentPanel(side);

    if (traceCatalogStages && (normalizeCatalog
            || catalogApplyStartedNs != 0 || stateApplyStartedNs != 0)) {
        F4NavigationBenchmarkTrace::event(
            QStringLiteral("qt.gallery.bridge.catalog.applied"),
            catalogTraceId, {
                {QStringLiteral("side"), side},
                {QStringLiteral("rows"), entries.size()},
                {QStringLiteral("normalizeDurationNs"),
                 normalizeCatalog
                    ? normalizeCompletedNs - normalizeStartedNs : 0},
                {QStringLiteral("catalogApplyDurationNs"),
                 catalogApplyStartedNs != 0
                    ? catalogApplyCompletedNs - catalogApplyStartedNs : 0},
                {QStringLiteral("stateApplyDurationNs"),
                 stateApplyStartedNs != 0
                    ? stateApplyCompletedNs - stateApplyStartedNs : 0},
                {QStringLiteral("catalogApplied"), catalogApplied},
            });
    }

    if (!m_cacheWarmup && m_navigationBenchmark.enabled
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Finished
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Failed) {
        QVariantMap fields = navigationBenchmarkFields();
        fields.insert(QStringLiteral("syncSide"), side);
        fields.insert(QStringLiteral("syncPath"), state.currentPath);
        fields.insert(QStringLiteral("syncLoading"), state.loading);
        fields.insert(QStringLiteral("syncLayoutMode"),
                      state.galleryLayoutMode);
        fields.insert(QStringLiteral("syncCatalogRevision"),
                      QVariant::fromValue<qulonglong>(
                          state.catalogRevision));
        fields.insert(QStringLiteral("syncEntryCount"),
                      state.entries.size());
        queueNavigationBenchmarkTrace(
            QStringLiteral("qt.gallery.bridge.panel.end"),
            m_navigationBenchmark.lastSceneTraceId, fields);
    }

    if (!m_cacheWarmup && repeatToReplay.active
        && catalogApplied && state.initialized && state.active
        && !state.catalogProvisional && state.panelId == panelId
        && state.currentPath == currentPath) {
        // Queue after the complete bridge/session transaction. Re-entering
        // requestOpen synchronously from synchronizePanel would expose a half-
        // applied cursor/catalog to observers and could send the stale source
        // row. A newer explicit input or scene invalidates this exact tuple.
        m_deferredPanelOpenRepeat.active = true;
        m_deferredPanelOpenRepeat.side = side;
        m_deferredPanelOpenRepeat.panelId = state.panelId;
        m_deferredPanelOpenRepeat.sourcePath = state.currentPath;
        m_deferredPanelOpenRepeat.catalogRevision = state.catalogRevision;
        const QString replayPanelId = state.panelId;
        const QString replayPath = state.currentPath;
        const qulonglong replayRevision = state.catalogRevision;
        QTimer::singleShot(0, this,
                           [this, side, replayPanelId, replayPath,
                            replayRevision]() {
            replayDeferredPanelOpenRepeat(side, replayPanelId, replayPath,
                                          replayRevision);
        });
    }
}

void F4GalleryBridge::refreshIconAppearance()
{
    for (int side = 0; side < 2; ++side) {
        const QVariantMap panel =
            m_panelSnapshots[static_cast<size_t>(side)];
        if (!panel.isEmpty()) {
            synchronizePanel(side, panel);
        }
    }
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
    QVariantMap action = {
        {QStringLiteral("action"), actionName},
        {QStringLiteral("side"), side},
    };
    if (!entryId.isEmpty()) {
        action.insert(QStringLiteral("entryId"), entryId);
    }
    if (index >= 0) {
        action.insert(QStringLiteral("index"), index);
    }
    if (includeCatalogRevision) {
        const qulonglong revision = effectiveCatalogRevision(side, catalogRevision);
        if (revision != 0) {
            action.insert(QStringLiteral("catalogRevision"), revision);
        }
    }
    if (activate) {
        action.insert(QStringLiteral("activate"), true);
    }
    emit uiActionRequested(action);
}

qulonglong F4GalleryBridge::effectiveCatalogRevision(int side, qulonglong supplied) const
{
    if (!validSide(side)) {
        return supplied;
    }

    const SideState &state = m_states[static_cast<size_t>(side)];
    if (state.initialized && state.catalogRevision != 0) {
        // The bridge is connected to QtShellController::sceneChanged before
        // QML observes that same signal, so its stable-ID catalog snapshot is
        // authoritative. A pointer gesture can nevertheless finish on a
        // delegate created from the preceding Loader binding. Forwarding that
        // stale non-zero revision makes Go reject panel.cursor; because the
        // scene itself did not change, there is then no acknowledgement with
        // which to reconcile a pending double-click/open. Resolve the stable
        // identity against the bridge-owned snapshot and always use its
        // revision. The supplied value remains the bootstrap fallback before
        // the first semantic scene has initialized this side.
        return state.catalogRevision;
    }
    return supplied;
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
                    !m_states[static_cast<size_t>(side)].active);
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

    const SideState &state = m_states[static_cast<size_t>(side)];
    if (state.panelId != pending.panelId) {
        clearPendingCursor(side);
        if (m_pendingViewer.active && m_pendingViewer.side == side) {
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
        if (m_pendingViewer.active && m_pendingViewer.side == side) {
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
        if (m_pendingViewer.active && m_pendingViewer.side == side
            && m_pendingViewer.entryId == pending.entryId) {
            m_pendingViewer.catalogRevision = state.catalogRevision;
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

    const SideState &state = m_states[static_cast<size_t>(side)];
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
    m_inFlightPanelOpen.active = true;
    m_inFlightPanelOpen.side = side;
    m_inFlightPanelOpen.panelId = state.panelId;
    m_inFlightPanelOpen.entryId = entryId;
    m_inFlightPanelOpen.sourcePath = state.currentPath;
    m_inFlightPanelOpen.catalogRevision = state.catalogRevision;
    if (m_panelOpenWatchdog) {
        m_panelOpenWatchdog->start();
    }
    sendPanelAction(side, QStringLiteral("panel.open"), entryId,
                    currentSourceIndex, 0, false);
}

void F4GalleryBridge::clearPendingPanelOpen()
{
    m_pendingPanelOpen = PendingPanelOpen{};
}

void F4GalleryBridge::clearInFlightPanelOpen()
{
    if (m_panelOpenWatchdog) {
        m_panelOpenWatchdog->stop();
    }
    m_inFlightPanelOpen = InFlightPanelOpen{};
    m_deferredPanelOpenRepeat = DeferredPanelOpenRepeat{};
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

    const SideState &state = m_states[static_cast<size_t>(side)];
    if (!state.initialized || !state.active || state.catalogProvisional
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

    const SideState &state = m_states[static_cast<size_t>(side)];
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

    if (state.catalogRevision == pending.catalogRevision) {
        return;
    }

    // The original action raced a newer catalog. Retry only the still-missing
    // target states and convert toggles to idempotent add/remove operations,
    // so an already accepted mutation is never applied twice.
    pending.catalogRevision = state.catalogRevision;
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
    QVariantMap action = {
        {QStringLiteral("action"), QStringLiteral("panel.setSelection")},
        {QStringLiteral("side"), side},
        {QStringLiteral("mode"), mode},
        {QStringLiteral("entryIds"), entryIds},
    };
    if (catalogRevision != 0) {
        action.insert(QStringLiteral("catalogRevision"), catalogRevision);
    }

    const size_t sideIndex = static_cast<size_t>(side);
    const qulonglong selectionRevision = m_states[sideIndex].selectionRevision;
    // Multiple Gallery selection gestures can reach Go before the semantic
    // scene acknowledging the first one returns. Only the first action is
    // guarded by the cached selection revision; later actions remain ordered
    // on the same IPC stream and omit that optional guard so they do not all
    // conflict with the first accepted mutation.
    if (selectionRevision != 0 && !m_selectionActionPending[sideIndex]) {
        action.insert(QStringLiteral("selectionRevision"), selectionRevision);
    }
    m_selectionActionPending[sideIndex] = true;
    emit uiActionRequested(action);
}

void F4GalleryBridge::reconcilePendingViewer(int side)
{
    if (!m_pendingViewer.active || m_pendingViewer.side != side || !validSide(side)) {
        return;
    }

    const SideState &state = m_states[static_cast<size_t>(side)];
    if (!state.previewCapable || state.panelId != m_pendingViewer.panelId
        || (m_pendingViewer.catalogRevision != 0
            && state.catalogRevision != m_pendingViewer.catalogRevision)) {
        clearPendingViewer();
        return;
    }

    // Opening from an inactive panel first activates that panel. Cursor and
    // activation acknowledgements can arrive in either order; retain the
    // stable viewer intent until both are authoritative.
    if (!state.active) {
        return;
    }

    if (state.cursorEntryId != m_pendingViewer.entryId) {
        // A cursor request can still be in flight or can be retried after a
        // catalog revision advance. Never open a different image, but keep
        // the viewer intent until the stable cursor is confirmed or removed.
        const PendingCursor &pending =
            m_pendingCursors[static_cast<size_t>(side)];
        if (!pending.active || pending.entryId != m_pendingViewer.entryId) {
            clearPendingViewer();
        }
        return;
    }

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(
        m_sessions[static_cast<size_t>(side)].data());
    if (!session || !session->isImageAt(session->currentIndex())) {
        clearPendingViewer();
        return;
    }
    const int confirmedSide = m_pendingViewer.side;
    clearPendingViewer();
    session->setViewerOpen(true);
    setViewer(confirmedSide, true);
}

void F4GalleryBridge::clearPendingViewer()
{
    m_pendingViewer = PendingViewer{};
}

void F4GalleryBridge::setViewer(int side, bool visible)
{
    const int normalizedSide = visible && validSide(side) ? side : -1;
    if (m_viewerVisible == visible && m_viewerSide == normalizedSide) {
        return;
    }
    m_viewerVisible = visible;
    m_viewerSide = normalizedSide;
    emit viewerChanged();
}
