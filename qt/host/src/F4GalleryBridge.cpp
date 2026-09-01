#include "F4GalleryBridge.h"
#include "F4ImageSourceProvider.h"
#include "F4IconProvider.h"
#include "NavigationBenchmarkTrace.h"
#include "ViewerCoordinator.h"

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
#include <ZoinGallery/MediaTimingTrace.h>

#include <algorithm>
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
constexpr int CatalogRowsPageSize = 64;
constexpr int CatalogRowsViewportOverscan = 64;
constexpr int MaximumDeferredCatalogPageRows = 256;

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

bool catalogIntegerValue(const QVariant &value, qlonglong *result = nullptr)
{
    const int type = value.metaType().id();
    const bool integer = type == QMetaType::Char
        || type == QMetaType::SChar || type == QMetaType::UChar
        || type == QMetaType::Short || type == QMetaType::UShort
        || type == QMetaType::Int || type == QMetaType::UInt
        || type == QMetaType::LongLong || type == QMetaType::ULongLong;
    if (!integer) {
        return false;
    }
    bool ok = false;
    const qlonglong converted = value.toLongLong(&ok);
    if (!ok) {
        return false;
    }
    if (result) {
        *result = converted;
    }
    return true;
}

qreal availableDevicePixelRatio()
{
    return qGuiApp ? qGuiApp->devicePixelRatio() : qreal(1);
}

}

F4GalleryBridge::F4GalleryBridge(QQmlEngine *engine, QObject *parent,
                                 F4IconSet *iconSet,
                                 QtMediaClient *mediaClient)
    : QObject(parent)
    , m_iconSet(iconSet)
    , m_panelIntentController(new PanelIntentController(this))
    , m_viewerCoordinator(new ViewerCoordinator(this))
{
    connect(m_panelIntentController, &PanelIntentController::intentRequested,
            this, [this](const PanelIntent &intent) {
        emit uiActionRequested(PanelIntentController::toWireMap(intent));
    });
    connect(m_viewerCoordinator, &ViewerCoordinator::changed,
            this, &F4GalleryBridge::viewerChanged);
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
            &F4GalleryBridge::handlePanelOpenWatchdog);
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
    if (mediaClient) {
        options.imageSourceProvider =
            QSharedPointer<F4ImageSourceProvider>::create(mediaClient);
    }
    auto *runtime = ZoinGallery::GalleryRuntime::install(engine, options);
    m_runtime = runtime;
    if (!runtime) {
        return;
    }

    m_panelSessions.setSession(
        0, runtime->createExternalSession(QStringLiteral("f4-left"), this));
    m_panelSessions.setSession(
        1, runtime->createExternalSession(QStringLiteral("f4-right"), this));
    configureNavigationBenchmark();
}

F4GalleryBridge::~F4GalleryBridge()
{
    QSet<QObject *> sessions;
    for (int side = 0; side < PanelSessionRegistry::PanelCount; ++side) {
        if (QObject *session = m_panelSessions.session(side)) {
            sessions.insert(session);
        }
    }
    for (QObject *sessionObject : std::as_const(sessions)) {
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
    return m_runtime && m_panelSessions.session(0)
        && m_panelSessions.session(1);
}

bool F4GalleryBridge::viewerVisible() const
{
    return m_viewerCoordinator->visible();
}

int F4GalleryBridge::viewerSide() const
{
    return m_viewerCoordinator->side();
}

QObject *F4GalleryBridge::viewerSession() const
{
    const int side = viewerSide();
    return m_panelSessions.session(side);
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

bool F4GalleryBridge::benchmarkTraceEnabled() const
{
    return F4NavigationBenchmarkTrace::enabled();
}

QObject *F4GalleryBridge::sessionForSide(int side) const
{
    return m_panelSessions.session(side);
}

QObject *F4GalleryBridge::sessionForPanel(const QString &panelId,
                                          int side) const
{
    if (!validSide(side)) {
        return nullptr;
    }
    const SideState &state = m_panelSessions.catalog(side);
    if (!panelId.isEmpty() && state.initialized
        && state.panelId != panelId) {
        return nullptr;
    }
    return m_panelSessions.session(side);
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
    finishPendingInputFrameTrace(synchronizedSerial, frameBoundaryNs);
    releaseFrameBoundMetadata(synchronizedSerial);
    scheduleDeferredCatalogFinalizations(synchronizedSerial);
    recordMediaFrameSwap();
    advanceNavigationBenchmarkFrame(frameBoundaryNs);
}
void F4GalleryBridge::finishPendingInputFrameTrace(
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
}

void F4GalleryBridge::releaseFrameBoundMetadata(
    qulonglong synchronizedSerial)
{
    bool metadataBecameEligible = false;
    for (int side = 0; side < 2; ++side) {
        SideState &state = m_panelSessions.catalog(side);
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
}

void F4GalleryBridge::scheduleDeferredCatalogFinalizations(
    qulonglong synchronizedSerial)
{
    for (int side = 0; side < 2; ++side) {
        const size_t sideIndex = static_cast<size_t>(side);
        SideState &state = m_panelSessions.catalog(static_cast<int>(sideIndex));
        DeferredCatalogFinalization &deferred =
            m_deferredCatalogFinalizations[sideIndex];
        if (state.provisionalFrameRequiredRenderSyncSerial == 0
            || synchronizedSerial
                < state.provisionalFrameRequiredRenderSyncSerial) {
            continue;
        }
        state.provisionalFrameRequiredRenderSyncSerial = 0;
        if (!deferred.active || deferred.scheduled
            || synchronizedSerial < deferred.requiredRenderSyncSerial) {
            continue;
        }
        deferred.scheduled = true;
        // Let every observer finish consuming the preview frame before the
        // authoritative logical row count resets the sparse model. The held
        // map contains only the negotiated viewport page (at most 256 rows),
        // never a complete directory catalog.
        QTimer::singleShot(0, this, [this, side]() {
            DeferredCatalogFinalization &pending =
                m_deferredCatalogFinalizations[static_cast<size_t>(side)];
            if (!pending.active) {
                return;
            }
            const QVariantMap panel = std::move(pending.panel);
            pending = {};
            synchronizePanel(side, panel);
        });
    }
}

void F4GalleryBridge::recordMediaFrameSwap()
{
    if (ZoinGallery::MediaTimingTrace::enabled()) {
        ZoinGallery::MediaTimingTrace::event(
            QStringLiteral("qt.gallery.frame_swapped"), {
                {QStringLiteral("serial"),
                 QVariant::fromValue<qulonglong>(++m_mediaFrameSerial)},
            });
    }
}

void F4GalleryBridge::advanceNavigationBenchmarkFrame(
    qint64 frameBoundaryNs)
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

    if (benchmark.frameSerial == 1
        && benchmark.phase == NavigationBenchmarkPhase::WaitingForPanel) {
        scheduleNavigationBenchmarkAdvance();
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
        SideState &state = m_panelSessions.catalog(side);
        if (state.initialized) {
            state.active = active;
        }
        QVariantMap &snapshot = m_panelSnapshots[static_cast<size_t>(side)];
        if (!snapshot.isEmpty()) {
            snapshot.insert(QStringLiteral("active"), active);
        }
    }

    if (viewerVisible() && viewerSide() != activePanel) {
        closeViewer();
    }
    // An inactive Gallery click can queue a stable viewer intent before Go
    // acknowledges activation. Complete that intent without walking either
    // catalog once the revisioned authoritative patch arrives.
    reconcilePendingViewer(activePanel);
    prioritizePanelCatalogMetadataRow(
        activePanel,
        m_panelSessions.catalog(activePanel).cursorIndex);
    schedulePanelCatalogMetadataRequest();
}
