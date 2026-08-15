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
    if (!engine) {
        return;
    }

    ZoinGallery::RuntimeOptions options;
    options.providerPrefix = QStringLiteral("f4-zoingallery");
    options.storageNamespace = QStringLiteral("f4-qt-host");
    // f4 owns one shared runtime for both panels. Preserve ZoinGallery's
    // historical platform-sized decode pool here: the runtime still bounds
    // compressed payloads and viewer frames independently, while a fixed
    // four-worker host pool cannot keep up with held navigation on machines
    // with substantially more decode capacity.
    options.maxDecodeThreads = 0;
    options.persistentCache = true;
    auto *runtime = ZoinGallery::GalleryRuntime::install(engine, options);
    m_runtime = runtime;
    if (!runtime) {
        return;
    }

    m_sessions[0] = runtime->createExternalSession(QStringLiteral("f4-left"), this);
    m_sessions[1] = runtime->createExternalSession(QStringLiteral("f4-right"), this);
    configureNavigationBenchmark();
}

F4GalleryBridge::~F4GalleryBridge()
{
    for (const QPointer<QObject> &sessionObject : m_sessions) {
        if (auto *session = qobject_cast<ZoinGallery::GallerySession *>(sessionObject.data())) {
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
    sendPanelAction(side, QStringLiteral("panel.cursor"), entryId, index, catalogRevision);
}

void F4GalleryBridge::requestOpen(int side,
                                  const QString &entryId,
                                  int index,
                                  bool isImage,
                                  qulonglong catalogRevision)
{
    if (!validSide(side)) {
        return;
    }

    const SideState &sideState = m_states[static_cast<size_t>(side)];
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
        if (!sideState.active) {
            requestActivate(side);
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

    // A folder double-click is two independent pointer presses. The first
    // press can be acknowledged before QML delivers doubleClicked, leaving
    // the bridge already authoritative at the requested stable identity.
    // Do not wait for the identical cursor action from the second press: f4
    // suppresses unchanged semantic scenes, so that no-op has no guaranteed
    // acknowledgement. panel.open itself activates the owning panel.
    if (sideState.cursorEntryId == entryId) {
        clearPendingCursor(side);
        reconcilePendingPanelOpen(side);
        return;
    }
    requestActivate(side);
    requestCursor(side, entryId, index, catalogRevision);
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
    static const QSet<QString> supported = {
        QStringLiteral("masonry"), QStringLiteral("columns"),
        QStringLiteral("details"), QStringLiteral("grid"),
        QStringLiteral("icons"),
    };
    const QString normalized = layoutMode.trimmed().toLower();
    if (!supported.contains(normalized)) {
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

void F4GalleryBridge::notifyFrameSwapped()
{
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

    const qint64 frameBoundary =
        F4NavigationBenchmarkTrace::monotonicNanoseconds();
    QVariantMap fields = navigationBenchmarkFields();
    fields.insert(QStringLiteral("lastPlacement"),
                  benchmark.lastPlacement);
    m_pendingNavigationBenchmarkTrace.push_back({
        QStringLiteral("qt.gallery.frame-swapped"), frameBoundary,
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
    QVariant sceneTraceId;
    const bool benchmarkRunning = m_navigationBenchmark.enabled
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Finished
        && m_navigationBenchmark.phase
            != NavigationBenchmarkPhase::Failed;
    if (benchmarkRunning) {
        sceneTraceId = F4NavigationBenchmarkTrace::benchmarkTraceId(scene);
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
            continue;
        }
        synchronizePanel(side, panel);
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
}

bool F4GalleryBridge::validSide(int side)
{
    return side == 0 || side == 1;
}

QVariantList F4GalleryBridge::panelsFromScene(const QVariantMap &scene)
{
    return shellFromScene(scene).value(QStringLiteral("panels")).toList();
}

QVariantList F4GalleryBridge::normalizedEntries(const QVariantMap &panel)
{
    const QVariantList sourceEntries = panel.value(QStringLiteral("entries")).toList();
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
        entry.insert(QStringLiteral("localPath"), source.value(QStringLiteral("localPath")));
        entry.insert(QStringLiteral("isDir"), source.value(QStringLiteral("isDir")));
        entry.insert(QStringLiteral("isUp"), source.value(QStringLiteral("isUp")));
        entry.insert(QStringLiteral("isHidden"),
                     source.value(QStringLiteral("isHidden")));
        if (source.contains(QStringLiteral("isImage"))) {
            entry.insert(QStringLiteral("isImage"), source.value(QStringLiteral("isImage")));
        }
        entry.insert(QStringLiteral("selected"), source.value(QStringLiteral("selected")));
        entry.insert(QStringLiteral("mtimeNs"), source.value(QStringLiteral("mtimeNanos")));
        entry.insert(QStringLiteral("size"), source.value(QStringLiteral("size")));
        entry.insert(QStringLiteral("sizeText"), source.value(QStringLiteral("sizeText")));
        entry.insert(QStringLiteral("sizeCalculated"),
                     source.value(QStringLiteral("sizeCalculated")));
        entry.insert(QStringLiteral("mtimeText"), source.value(QStringLiteral("mtime")));
        entry.insert(QStringLiteral("modeText"), source.value(QStringLiteral("mode")));
        entry.insert(QStringLiteral("highlightStyleId"),
                     source.value(QStringLiteral("highlightStyleId")));
        entries.push_back(entry);
    }
    return entries;
}

QVariantList F4GalleryBridge::normalizedAppearance(
    const QVariantMap &panel) const
{
    const QVariantList sourceEntries = panel.value(QStringLiteral("entries")).toList();
    const QVariantMap styles = panel.value(QStringLiteral("highlightStyles")).toMap();
    const qreal devicePixelRatio = availableDevicePixelRatio();
    QVariantList appearance;
    appearance.reserve(sourceEntries.size());
    for (const QVariant &value : sourceEntries) {
        const QVariantMap source = value.toMap();
        const QString styleId = source.value(QStringLiteral("highlightStyleId")).toString();
        QVariantMap entry;
        entry.insert(QStringLiteral("entryId"), source.value(QStringLiteral("entryId")));
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
            // replaceable default, so a marker can suppress it; otherwise it
            // is translated to the equivalent native file icon.
            if (replaceableIcon) {
                if (hasMarker) {
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
                    const QUrl iconSource = m_iconSet->fileIconSource(
                        source.value(QStringLiteral("localPath")).toString(),
                        fileName,
                        directory,
                        GalleryIconLogicalSize,
                        devicePixelRatio,
                        source.value(QStringLiteral("mtimeNanos"))
                            .toULongLong());
                    style.insert(QStringLiteral("icon"),
                                 iconSource.toString());
                }
            }
        }
        entry.insert(QStringLiteral("highlightStyle"), style);
        appearance.push_back(entry);
    }
    return appearance;
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
    const qulonglong iconRevision = m_iconSet ? m_iconSet->revision() : 0;
    return panel.value(QStringLiteral("id")).toString() == state.panelId
        && revisionValue(panel, QStringLiteral("catalogRevision"))
            == state.catalogRevision
        && revisionValue(panel, QStringLiteral("selectionRevision"))
            == state.selectionRevision
        && revisionValue(panel, QStringLiteral("highlightRevision"))
            == state.highlightRevision
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
        && panel.value(QStringLiteral("galleryLayoutMode")).toString()
            == state.galleryLayoutMode;
}

void F4GalleryBridge::synchronizePanel(int side, const QVariantMap &panel)
{
    m_panelSnapshots[static_cast<size_t>(side)] = panel;
    SideState &state = m_states[static_cast<size_t>(side)];
    const QString panelId = panel.value(QStringLiteral("id")).toString();
    const qulonglong catalogRevision = revisionValue(panel, QStringLiteral("catalogRevision"));
    const qulonglong selectionRevision = revisionValue(panel, QStringLiteral("selectionRevision"));
    const qulonglong highlightRevision = revisionValue(panel, QStringLiteral("highlightRevision"));
    const qulonglong iconRevision = m_iconSet ? m_iconSet->revision() : 0;
    const QString currentPath = panel.value(QStringLiteral("path")).toString();
    const QString cursorEntryId = panel.value(QStringLiteral("cursorEntryId")).toString();
    const int cursorIndex = panel.value(QStringLiteral("cursor"), -1).toInt();
    const QString sourceKind = panel.value(QStringLiteral("sourceKind"), QStringLiteral("vfs")).toString();
    const bool previewCapable = panel.value(QStringLiteral("previewCapable")).toBool()
        && sourceKind == QStringLiteral("local");
    const bool active = panel.value(QStringLiteral("active")).toBool();
    const bool loading = panel.value(QStringLiteral("loading")).toBool();
    const QString galleryLayoutMode = panel.value(
        QStringLiteral("galleryLayoutMode")).toString();
    const bool identityChanged = state.initialized && panelId != state.panelId;

    if (m_navigationBenchmark.enabled
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

    auto *session = qobject_cast<ZoinGallery::GallerySession *>(m_sessions[static_cast<size_t>(side)].data());
    if (session && identityChanged) {
        session->resetExternalSource();
    }
    if (identityChanged) {
        if (m_viewerVisible && m_viewerSide == side) {
            closeViewer();
        }
        if (m_pendingPanelOpen.active && m_pendingPanelOpen.side == side) {
            clearPendingPanelOpen();
        }
        clearPendingCursor(side);
        clearPendingSelection(side);
        m_selectionActionPending[static_cast<size_t>(side)] = false;
        state = SideState{};
    }
    const bool catalogChanged = !state.initialized
        || catalogRevision != state.catalogRevision
        || currentPath != state.currentPath
        || sourceKind != state.sourceKind
        || previewCapable != state.previewCapable;
    const bool selectionChanged = !state.initialized
        || selectionRevision != state.selectionRevision;
    const bool highlightChanged = !state.initialized
        || highlightRevision != state.highlightRevision;
    const bool iconChanged = !state.initialized
        || iconRevision != state.iconRevision;
    // Semantic scenes retain the complete catalog for compatibility, but a
    // cursor acknowledgement does not need to normalize/copy it again. Keep
    // the revision-owned snapshot in the persistent bridge session.
    const QVariantList entries = catalogChanged
        ? normalizedEntries(panel) : state.entries;
    const QStringList selectedIds = catalogChanged || selectionChanged
        ? selectedEntryIds(catalogChanged
              ? entries
              : panel.value(QStringLiteral("entries")).toList())
        : state.selectedEntryIdList;

    if (session && catalogChanged) {
        session->applyExternalCatalog(entries, catalogRevision, {
            {QStringLiteral("currentPath"), currentPath},
            {QStringLiteral("sourceKind"), sourceKind},
            {QStringLiteral("previewCapable"), previewCapable},
        });
    }

    if (session
        && (catalogChanged || highlightChanged || iconChanged)) {
        session->applyExternalAppearance(normalizedAppearance(panel),
                                         highlightRevision);
    }

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
        if (pendingCursor.active && pendingCursor.panelId == panelId
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
        session->applyExternalState(appliedCursorEntryId,
                                    appliedCursorIndex,
                                    selectedIds,
                                    selectionRevision);
    }

    state.initialized = true;
    state.panelId = panelId;
    state.catalogRevision = catalogRevision;
    state.selectionRevision = selectionRevision;
    state.highlightRevision = highlightRevision;
    state.iconRevision = iconRevision;
    state.currentPath = currentPath;
    state.sourceKind = sourceKind;
    state.cursorEntryId = cursorEntryId;
    state.cursorIndex = cursorIndex;
    state.previewCapable = previewCapable;
    state.active = active;
    state.loading = loading;
    state.galleryLayoutMode = galleryLayoutMode;
    if (catalogChanged) {
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
    }
    if (catalogChanged || selectionChanged) {
        state.selectedEntryIdList = selectedIds;
        state.selectedEntryIds = QSet<QString>(selectedIds.cbegin(),
                                               selectedIds.cend());
    }
    m_stateReconciliationPending[static_cast<size_t>(side)] = false;
    reconcilePendingCursor(side);
    reconcilePendingPanelOpen(side);
    reconcilePendingSelection(side);
    reconcilePendingViewer(side);

    if (m_navigationBenchmark.enabled
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
                                      bool includeCatalogRevision)
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
                    pending.index, pending.catalogRevision);
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
                        pending.index, pending.catalogRevision);
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
        clearPendingPanelOpen();
        return;
    }

    const int currentSourceIndex = state.sourceIndexByEntryId.value(
        m_pendingPanelOpen.entryId, -1);
    if (currentSourceIndex < 0) {
        // A successful directory open replaces the catalog and removes the
        // source entry. Removal by a concurrent file operation has the same
        // safe terminal behavior: never open a different row.
        clearPendingPanelOpen();
        return;
    }

    if (state.cursorEntryId != m_pendingPanelOpen.entryId) {
        const PendingCursor &cursor =
            m_pendingCursors[static_cast<size_t>(side)];
        if (!cursor.active || cursor.entryId != m_pendingPanelOpen.entryId) {
            clearPendingPanelOpen();
        }
        return;
    }

    // Cursor confirmation resolves the catalog race. From this point the
    // operation is an exactly-once stable-ID intent: including a catalog
    // revision would only make an unrelated later revision look retryable and
    // could relaunch an external application. Clear before emitting so even a
    // reentrant scene update cannot dispatch it twice.
    const QString entryId = m_pendingPanelOpen.entryId;
    clearPendingPanelOpen();
    sendPanelAction(side, QStringLiteral("panel.open"), entryId,
                    currentSourceIndex, 0, false);
}

void F4GalleryBridge::clearPendingPanelOpen()
{
    m_pendingPanelOpen = PendingPanelOpen{};
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
