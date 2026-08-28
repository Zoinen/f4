#pragma once

#include <QObject>
#include <QPointer>
#include <QHash>
#include <QList>
#include <QSet>
#include <QStringList>
#include <QUrl>
#include <QVariantList>
#include <QVariantMap>

#include <array>
#include <atomic>

class QQmlEngine;
class QTimer;
class F4IconSet;
class QtMediaClient;

class F4GalleryBridge final : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool available READ available CONSTANT)
    Q_PROPERTY(QObject *viewerSession READ viewerSession NOTIFY viewerChanged)
    Q_PROPERTY(bool viewerVisible READ viewerVisible NOTIFY viewerChanged)
    Q_PROPERTY(int viewerSide READ viewerSide NOTIFY viewerChanged)
    Q_PROPERTY(QUrl panelComponentUrl READ panelComponentUrl CONSTANT)
    Q_PROPERTY(QUrl viewerComponentUrl READ viewerComponentUrl CONSTANT)
    Q_PROPERTY(bool navigationBenchmarkEnabled READ navigationBenchmarkEnabled CONSTANT)

public:
    explicit F4GalleryBridge(QQmlEngine *engine, QObject *parent = nullptr,
                             F4IconSet *iconSet = nullptr,
                             QtMediaClient *mediaClient = nullptr);
    ~F4GalleryBridge() override;

    bool available() const;
    QObject *viewerSession() const;
    bool viewerVisible() const { return m_viewerVisible; }
    int viewerSide() const { return m_viewerSide; }
    QUrl panelComponentUrl() const;
    QUrl viewerComponentUrl() const;
    bool navigationBenchmarkEnabled() const;

    Q_INVOKABLE QObject *sessionForSide(int side) const;
    Q_INVOKABLE QObject *sessionForPanel(const QString &panelId,
                                         int side) const;
    // Row-free snapshots for QML viewport warmup. Catalog entries remain in
    // the identity-keyed C++ GallerySession and never cross this API.
    Q_INVOKABLE QVariantList cachedPanelPresentations(int side) const;
    Q_INVOKABLE void requestActivate(int side);
    Q_INVOKABLE void requestCursor(int side,
                                   const QString &entryId,
                                   int index,
                                   qulonglong catalogRevision = 0,
                                   bool deferCommit = false);
    Q_INVOKABLE void requestOpen(int side,
                                 const QString &entryId,
                                 int index,
                                 bool isImage,
                                 qulonglong catalogRevision = 0,
                                 bool autoRepeat = false);
    Q_INVOKABLE void requestSelection(int side,
                                      const QString &mode,
                                      const QVariantList &entryIds,
                                      qulonglong catalogRevision = 0);
    Q_INVOKABLE void requestGalleryLayout(int side, const QString &layoutMode,
                                          int columnCount = 0);
    Q_INVOKABLE void requestGalleryDensity(int side, const QString &layoutMode,
                                           int density);
    Q_INVOKABLE void requestSort(int side, const QString &sortMode,
                                 bool contextMenu = false);
    Q_INVOKABLE void recordBenchmarkStage(int side, const QString &stage,
                                          const QVariantMap &metadata = {});
    Q_INVOKABLE void reportMetadataVisibleRange(
        int side, int firstRow, int lastRow,
        qulonglong catalogRevision = 0);
    Q_INVOKABLE void closeViewer();

public slots:
    void synchronizeScene(const QVariantMap &scene);
    void synchronizePanelCatalog(const QVariantMap &panel);
    void synchronizePanelState(const QVariantMap &patch);
    void synchronizePanelActivation(int activePanel, qulonglong revision);
    void beginCompactProtocolMessage(const QVariantMap &message);
    void handleProtocolMessage(const QVariantMap &message);
    // main.cpp connects QQuickWindow::frameSwapped to this slot. Keeping the
    // window dependency out of the bridge makes the runner testable with a
    // synthetic frame boundary.
    void notifyRenderSynchronized();
    void captureFrameSwapped();
    void notifyFrameSwapped(qulonglong synchronizedSerial);

signals:
    void uiActionRequested(const QVariantMap &action);
    void panelCatalogMetadataRequested(const QVariantMap &request);
    void panelCachePrepared(int side, const QVariantMap &panel);
    void viewerChanged();
    void benchmarkFrameSwapped(qulonglong serial);

private:
    friend class F4GalleryBridgeTests;

    struct MetadataRange {
        int begin = 0;
        int end = 0;
    };

    struct SideState {
        bool initialized = false;
        QString panelId;
        qulonglong catalogRevision = 0;
        qulonglong selectionRevision = 0;
        qulonglong highlightRevision = 0;
        qulonglong iconRevision = 0;
        QString currentPath;
        QString sourceKind;
        QString cursorEntryId;
        int cursorIndex = -1;
        bool previewCapable = false;
        bool active = false;
        bool loading = false;
        bool catalogProvisional = false;
        bool metadataDeferred = false;
        bool metadataComplete = true;
        bool metadataRequestInFlight = false;
        bool metadataAwaitingFrame = false;
        qulonglong metadataRequiredRenderSyncSerial = 0;
        qulonglong metadataPacingGeneration = 0;
        qulonglong metadataRevision = 0;
        int metadataRequestOffset = -1;
        int metadataRequestLimit = 0;
        int metadataVisibleFirst = -1;
        int metadataVisibleLast = -1;
        int metadataUrgentBudget = 0;
        int metadataFailureCount = 0;
        QList<MetadataRange> metadataPendingRanges;
        QString galleryLayoutMode;
        QVariantList entries;
        QStringList selectedEntryIdList;
        QHash<QString, int> sourceIndexByEntryId;
        QSet<QString> entryIds;
        QSet<QString> selectedEntryIds;
    };

    struct CachedPanel {
        QPointer<QObject> session;
        SideState state;
        QVariantMap snapshot;
    };

    struct PendingViewer {
        bool active = false;
        int side = -1;
        QString panelId;
        QString entryId;
        qulonglong catalogRevision = 0;
    };

    struct PendingCursor {
        bool active = false;
        QString panelId;
        QString entryId;
        int index = -1;
        qulonglong catalogRevision = 0;
    };

    struct PendingPanelOpen {
        bool active = false;
        int side = -1;
        QString panelId;
        QString entryId;
    };

    struct InFlightPanelOpen {
        bool active = false;
        int side = -1;
        QString panelId;
        QString entryId;
        QString sourcePath;
        qulonglong catalogRevision = 0;
    };

    // One held-key repeat which arrived while panel.open was still in flight.
    // It is intentionally an epoch marker rather than a stale entry request:
    // once the authoritative destination catalog arrives, replay resolves the
    // destination's current cursor stable ID from SideState.
    struct DeferredPanelOpenRepeat {
        bool active = false;
        int side = -1;
        QString panelId;
        QString sourcePath;
        qulonglong catalogRevision = 0;
    };

    struct PendingSelection {
        bool active = false;
        QString panelId;
        qulonglong catalogRevision = 0;
        QHash<QString, bool> desiredByEntryId;
    };

    enum class NavigationBenchmarkPhase {
        Disabled,
        WaitingForPanel,
        SettingDetails,
        NavigatingToTargetForSetup,
        ReturningToParentForSetup,
        WaitingForSetupReadiness,
        WaitingForSetupFrame,
        ReadyToDispatch,
        WaitingForTransitionReadiness,
        WaitingForTransitionFrame,
        Finished,
        Failed,
    };

    struct NavigationBenchmarkState {
        bool enabled = false;
        bool exitWhenFinished = false;
        NavigationBenchmarkPhase phase = NavigationBenchmarkPhase::Disabled;
        QString runId;
        QString targetPath;
        QString parentPath;
        QString targetName;
        int side = -1;
        int cycles = 50;
        int warmup = 10;
        int completedCycles = 0;
        int completedTransitions = 0;
        bool nextTransitionEnters = true;
        quint64 phaseSequence = 0;
        quint64 actionSequence = 0;
        quint64 frameSerial = 0;
        quint64 requiredFrameSerial = 0;
        QString benchmarkTraceId;
        QString actionPhase;
        QString direction;
        QString fromPath;
        QString expectedPath;
        bool actionSent = false;
        bool sceneMatched = false;
        bool placementReady = false;
        QString placementPath;
        qulonglong placementCatalogRevision = 0;
        QVariantMap lastPlacement;
        QVariant lastSceneTraceId;
        QVariantMap lastSceneBenchmark;
    };

    struct PendingNavigationBenchmarkTrace {
        QString name;
        qint64 monotonicNs = 0;
        QVariant benchmarkTraceId;
        QVariantMap fields;
    };

    static bool validSide(int side);
    static QVariantList panelsFromScene(const QVariantMap &scene);
    QVariantList normalizedEntries(const QVariantMap &panel) const;
    QVariantList normalizedMetadataEntries(
        int side, int offset, const QVariantList &entries,
        const QVariantMap &highlightStyles) const;
    static QStringList selectedEntryIds(const QVariantList &entries);
    static int sourceIndexForEntryId(const QVariantList &entries,
                                     const QString &entryId);
    static qulonglong revisionValue(const QVariantMap &map, const QString &key);

    bool canSkipUnchangedInactivePanel(int side,
                                       const QVariantMap &panel) const;
    void synchronizePanel(int side, const QVariantMap &panel);
    QObject *createPanelSession();
    void cacheCurrentPanel(int side);
    bool activatePanelSession(int side, const QString &panelId);
    void synchronizePanelCache(const QVariantMap &panel,
                               const QVariantMap &metadata = {});
    bool applyCachedPanelMetadata(int side, const QVariantMap &metadata);
    bool synchronizeWorkspaceShell(const QVariantMap &shell);
    void requestPanelCatalogMetadata(int side);
    void requestNextPanelCatalogMetadata();
    void schedulePanelCatalogMetadataRequest();
    void resetPanelCatalogMetadataPlan(int side, bool ready);
    bool choosePanelCatalogMetadataRange(int side, int *offset,
                                         int *limit, bool *urgent) const;
    bool consumePanelCatalogMetadataRange(int side, int offset, int end);
    void failPanelCatalogMetadataRequest(int side, bool retry);
    void noteMetadataInputActivity();
    void prioritizePanelCatalogMetadataRow(int side, int row);
    int matchingMetadataSide(const QVariantMap &message) const;
    void refreshDeferredIconAppearance(int side);
    void sendPanelAction(int side,
                         const QString &action,
                         const QString &entryId = QString(),
                         int index = -1,
                         qulonglong catalogRevision = 0,
                         bool includeCatalogRevision = true);
    qulonglong effectiveCatalogRevision(int side, qulonglong supplied) const;
    void commitPendingCursor(int side);
    void reconcilePendingCursor(int side);
    void clearPendingCursor(int side);
    void reconcilePendingPanelOpen(int side);
    void clearPendingPanelOpen();
    void clearInFlightPanelOpen();
    void replayDeferredPanelOpenRepeat(int side,
                                       const QString &panelId,
                                       const QString &sourcePath,
                                       qulonglong catalogRevision);
    void reconcilePendingSelection(int side);
    void clearPendingSelection(int side);
    void emitSelectionAction(int side, const QString &mode,
                             const QVariantList &entryIds,
                             qulonglong catalogRevision);
    void reconcilePendingViewer(int side);
    void clearPendingViewer();
    void setViewer(int side, bool visible);
    void refreshIconAppearance();
    void configureNavigationBenchmark();
    void scheduleNavigationBenchmarkAdvance();
    void notifyFrameSwappedAt(qulonglong synchronizedSerial,
                              qint64 frameBoundaryNs);
    void advanceNavigationBenchmark();
    void sendNavigationBenchmarkAction(QVariantMap action,
                                       const QString &phase,
                                       const QString &direction,
                                       const QString &fromPath,
                                       const QString &toPath);
    void armNavigationBenchmarkFrame(bool setup);
    void completeNavigationBenchmarkFrame();
    void finishNavigationBenchmark();
    void failNavigationBenchmark(const QString &reason,
                                 const QVariantMap &fields = {});
    void restartNavigationBenchmarkWatchdog();
    void queueNavigationBenchmarkTrace(const QString &name,
                                       const QVariant &benchmarkTraceId,
                                       const QVariantMap &fields = {});
    void flushNavigationBenchmarkTrace();
    QVariantMap navigationBenchmarkFields() const;
    QVariantMap navigationBenchmarkEntryForPath(int side,
                                                const QString &path) const;
    QVariantMap navigationBenchmarkUpEntry(int side) const;

    QPointer<F4IconSet> m_iconSet;
    QPointer<QObject> m_runtime;
    std::array<QPointer<QObject>, 2> m_sessions;
    std::array<SideState, 2> m_states;
    std::array<QVariantMap, 2> m_panelSnapshots;
    QHash<QString, CachedPanel> m_panelCache;
    QSet<QObject *> m_allSessions;
    qulonglong m_nextPanelSessionId = 0;
    bool m_cacheWarmup = false;
    std::array<bool, 2> m_stateReconciliationPending = {false, false};
    std::array<bool, 2> m_selectionActionPending = {false, false};
    std::array<PendingCursor, 2> m_pendingCursors;
    std::array<QTimer *, 2> m_cursorCommitTimers = {nullptr, nullptr};
    std::array<PendingSelection, 2> m_pendingSelections;
    PendingPanelOpen m_pendingPanelOpen;
    InFlightPanelOpen m_inFlightPanelOpen;
    DeferredPanelOpenRepeat m_deferredPanelOpenRepeat;
    QTimer *m_panelOpenWatchdog = nullptr;
    PendingViewer m_pendingViewer;
    bool m_viewerVisible = false;
    int m_viewerSide = -1;
    qulonglong m_panelActivationRevision = 0;
    NavigationBenchmarkState m_navigationBenchmark;
    QTimer *m_navigationBenchmarkWatchdog = nullptr;
    QList<PendingNavigationBenchmarkTrace> m_pendingNavigationBenchmarkTrace;
    QVariant m_lastInputSceneTraceId;
    QVariant m_pendingInputFrameTraceId;
    qint64 m_pendingInputFrameSceneEndNs = 0;
    qulonglong m_pendingInputFrameRequiredRenderSyncSerial = 0;
    qulonglong m_inputScenesSupersededBeforeFrame = 0;
    std::atomic<qulonglong> m_renderSyncSerial{0};
    bool m_metadataRequestScheduled = false;
    bool m_metadataInputBusy = false;
    QTimer *m_metadataIdleTimer = nullptr;
};
