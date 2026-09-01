#pragma once

#include "PanelCatalogModel.h"
#include "PanelIntentController.h"
#include "PanelSessionRegistry.h"

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
class ViewerCoordinator;
class F4GalleryRowsResponseReducer;
class F4GalleryMetadataResponseReducer;

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
    Q_PROPERTY(bool benchmarkTraceEnabled READ benchmarkTraceEnabled CONSTANT)

public:
    explicit F4GalleryBridge(QQmlEngine *engine, QObject *parent = nullptr,
                             F4IconSet *iconSet = nullptr,
                             QtMediaClient *mediaClient = nullptr);
    ~F4GalleryBridge() override;

    bool available() const;
    QObject *viewerSession() const;
    bool viewerVisible() const;
    int viewerSide() const;
    QUrl panelComponentUrl() const;
    QUrl viewerComponentUrl() const;
    bool navigationBenchmarkEnabled() const;
    bool benchmarkTraceEnabled() const;

    Q_INVOKABLE QObject *sessionForSide(int side) const;
    Q_INVOKABLE QObject *sessionForPanel(const QString &panelId,
                                         int side) const;
    Q_INVOKABLE void requestActivate(int side);
    Q_INVOKABLE void requestCursor(int side,
                                   const QString &entryId,
                                   int index,
                                   qulonglong catalogRevision = 0,
                                   bool deferCommit = false,
                                   bool selectionGesture = false);
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
    Q_INVOKABLE void requestSelectionTransaction(
        int side, const QVariantList &changes,
        const QString &cursorEntryId = QString(), int cursorIndex = -1,
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
    void synchronizePanelCatalogAppend(const QVariantMap &append);
    void synchronizePanelState(const QVariantMap &patch);
    void synchronizePanelActivation(int activePanel, qulonglong revision);
    void beginCompactProtocolMessage(const QVariantMap &message);
    void handleCompactProtocolMessage(const QVariantMap &message);
    void handlePanelCatalogRowsMessage(const QVariantMap &message);
    void handlePanelCatalogMetadataMessage(const QVariantMap &message);
    // main.cpp connects QQuickWindow::frameSwapped to this slot. Keeping the
    // window dependency out of the bridge makes the runner testable with a
    // synthetic frame boundary.
    void notifyRenderSynchronized();
    void captureFrameSwapped();
    void notifyFrameSwapped(qulonglong synchronizedSerial);

signals:
    void uiActionRequested(const QVariantMap &action);
    void panelCatalogMetadataRequested(const QVariantMap &request);
    void panelCatalogRowsRequested(const QVariantMap &request);
    // Bracket catalog replacement with the bounded presentation descriptor
    // that belongs to the same semantic revision. QML applies this descriptor
    // before GallerySession emits path/model changes, so a path never reflows
    // through a stale presentation mode. No catalog rows are carried here.
    void panelPresentationTransactionStarted(
        int side, const QString &panelId, qulonglong catalogRevision,
        const QString &layoutMode, int columnCount, int density,
        const QVariantMap &presentationDensities,
        const QVariantList &columns, bool separateFileExtensions);
    void panelPresentationTransactionFinished(int side);
    void viewerChanged();
    void benchmarkFrameSwapped(qulonglong serial);

private:
    friend class F4GalleryBridgeTests;
    friend class F4GalleryRowsResponseReducer;
    friend class F4GalleryMetadataResponseReducer;
    using MetadataRange = PanelCatalogModel::MetadataRange;
    using SideState = PanelCatalogModel;

    struct PendingCursor {
        bool active = false;
        QString panelId;
        QString entryId;
        int index = -1;
        qulonglong catalogRevision = 0;
        // A selection gesture owns cursor and selection as one transaction.
        // If the catalog advances while it is in flight, keep that stable
        // cursor visible until the atomic retry is acknowledged.
        bool maskAcrossCatalog = false;
    };

    struct DeferredCatalogFinalization {
        bool active = false;
        bool scheduled = false;
        qulonglong requiredRenderSyncSerial = 0;
        QVariantMap panel;
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
        bool expectsPathChange = false;
        // If the source scan completes after a directory-open intent, retain
        // only its bounded wire page so a rejected navigation can recover.
        // A successful path acknowledgement discards it immediately.
        QVariantMap deferredSourcePanel;
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
        qulonglong selectionRevision = 0;
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

    struct PanelSyncContext;
    struct SceneSyncContext;
    struct CatalogAppendContext;
    struct PanelStatePatchContext;

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
    PanelSyncContext makePanelSyncContext(int side,
                                          const QVariantMap &panel);
    bool deferSupersededPanelCatalog(PanelSyncContext *context);
    bool deferPanelCatalogFinalization(PanelSyncContext *context);
    void acknowledgePanelOpen(PanelSyncContext *context);
    void tracePanelSyncBegin(const PanelSyncContext &context);
    void handlePanelIdentityChange(PanelSyncContext *context);
    bool applyProvisionalPanelUpdate(PanelSyncContext *context);
    void classifyPanelSyncChanges(PanelSyncContext *context);
    void preparePanelCatalogData(PanelSyncContext *context);
    void applyPanelSessionData(PanelSyncContext *context);
    void commitPanelSyncState(PanelSyncContext *context);
    void rebuildPanelCatalogIndex(PanelSyncContext *context);
    void updatePanelMetadataPlan(PanelSyncContext *context);
    void finalizePanelSync(PanelSyncContext *context);
    void replayPanelOpenAfterSync(const PanelSyncContext &context);
    SceneSyncContext makeSceneSyncContext(const QVariantMap &scene);
    void beginSceneSyncTrace(SceneSyncContext *context);
    void synchronizeScenePanels(SceneSyncContext *context);
    void synchronizeScenePanel(SceneSyncContext *context,
                               int side, const QVariantMap &panel);
    void traceSkippedScenePanel(const SceneSyncContext &context,
                                int side, const QVariantMap &panel);
    void reconcileSceneOwnedState(const SceneSyncContext &context);
    void finishBenchmarkSceneTrace(const SceneSyncContext &context);
    void finishGenericSceneTrace(SceneSyncContext *context);
    bool parseCatalogAppend(const QVariantMap &append,
                            CatalogAppendContext *context);
    bool prepareCatalogAppendIdentityIndex(CatalogAppendContext *context);
    bool validateCatalogAppendRows(CatalogAppendContext *context) const;
    bool applyCatalogAppendRows(CatalogAppendContext *context);
    void commitCatalogAppendRows(CatalogAppendContext *context);
    void finalizeCatalogAppend(CatalogAppendContext *context);
    void traceCatalogAppend(const CatalogAppendContext &context) const;
    bool parsePanelStatePatch(const QVariantMap &patch,
                              PanelStatePatchContext *context);
    bool resolvePanelStateRevision(PanelStatePatchContext *context);
    void derivePanelStateValues(PanelStatePatchContext *context);
    bool applyProvisionalPanelStatePatch(PanelStatePatchContext *context);
    bool applyPanelStateOperation(PanelStatePatchContext *context);
    bool applyPanelSelectionDelta(PanelStatePatchContext *context);
    bool applyPanelSelectionReplacement(PanelStatePatchContext *context);
    bool applyPanelStateCatalogOptions(PanelStatePatchContext *context);
    void commitPanelStatePatch(PanelStatePatchContext *context);
    void updatePanelStateMetadataPlan(PanelStatePatchContext *context);
    void finalizePanelStatePatch(PanelStatePatchContext *context);
    bool activatePanelSession(int side, const QString &panelId);
    bool requestPanelCatalogMetadata(int side);
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
    void requestPanelCatalogRows(int side);
    void schedulePanelCatalogRowsRequest(int side);
    int matchingCatalogRowsSide(const QVariantMap &message) const;
    bool catalogRowLoaded(const SideState &state, int row) const;
    static int catalogEntryCount(const SideState &state);
    static QVariantMap catalogEntryAt(const SideState &state, int row);
    static bool setCatalogEntry(SideState &state, int row,
                                const QVariant &entry);
    void addPanelCatalogMetadataRange(int side, int begin, int end);
    void refreshDeferredIconAppearance(int side);
    void sendPanelAction(int side,
                         const QString &action,
                         const QString &entryId = QString(),
                         int index = -1,
                         qulonglong catalogRevision = 0,
                         bool includeCatalogRevision = true,
                         bool activate = false);
    qulonglong effectiveCatalogRevision(int side, qulonglong supplied) const;
    void commitPendingCursor(int side);
    void reconcilePendingCursor(int side);
    void clearPendingCursor(int side);
    void reconcilePendingPanelOpen(int side);
    void clearPendingPanelOpen();
    void markPanelOpenInFlight(int side, const QString &entryId);
    void clearInFlightPanelOpen();
    void handlePanelOpenWatchdog();
    void replayDeferredPanelOpenRepeat(int side,
                                       const QString &panelId,
                                       const QString &sourcePath,
                                       qulonglong catalogRevision);
    void reconcilePendingSelection(int side);
    void clearPendingSelection(int side);
    bool normalizeSelectionChanges(
        const SideState &state, const QVariantList &changes,
        QVariantList &normalizedChanges) const;
    bool normalizeSelectionCursor(
        const SideState &state, const QString &cursorEntryId,
        QString &normalizedEntryId, int &normalizedIndex) const;
    void stageSelectionCursor(
        int side, const SideState &state, const QString &entryId,
        int index, qulonglong catalogRevision);
    void stageSelectionChanges(
        int side, const SideState &state,
        const QVariantList &normalizedChanges,
        qulonglong catalogRevision);
    void dispatchSelectionTransaction(
        int side, const SideState &state,
        const QVariantList &normalizedChanges,
        const QString &cursorEntryId, int cursorIndex,
        qulonglong catalogRevision);
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
    void finishPendingInputFrameTrace(qulonglong synchronizedSerial,
                                      qint64 frameBoundaryNs);
    void releaseFrameBoundMetadata(qulonglong synchronizedSerial);
    void scheduleDeferredCatalogFinalizations(
        qulonglong synchronizedSerial);
    void recordMediaFrameSwap();
    void advanceNavigationBenchmarkFrame(qint64 frameBoundaryNs);
    void advanceNavigationBenchmark();
    bool selectNavigationBenchmarkSide();
    bool recordPassiveBenchmarkStage(
        int side, const QString &stage, const QVariantMap &metadata);
    void queueActiveBenchmarkStage(
        int side, const QString &stage, const QVariantMap &metadata,
        const QVariant &traceId);
    void updateNavigationBenchmarkPlacement(
        int side, const QString &stage, const QVariantMap &metadata,
        const QVariant &traceId);
    void advanceBenchmarkWaitingForPanel(const SideState &state);
    void advanceBenchmarkSettingDetails(const SideState &state);
    void advanceBenchmarkTargetSetup(const SideState &state);
    void advanceBenchmarkParentSetup(const SideState &state);
    void advanceBenchmarkSetupReadiness(const SideState &state);
    bool materializeBenchmarkSetupTarget(const SideState &state,
                                         QVariantMap &targetEntry);
    bool selectBenchmarkSetupTarget(const SideState &state,
                                    const QVariantMap &targetEntry);
    void advanceBenchmarkReadyToDispatch(const SideState &state);
    void advanceBenchmarkTransitionReadiness(const SideState &state);
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
    PanelIntentController *m_panelIntentController = nullptr;
    ViewerCoordinator *m_viewerCoordinator = nullptr;
    QPointer<QObject> m_runtime;
    PanelSessionRegistry m_panelSessions;
    std::array<DeferredCatalogFinalization, 2>
        m_deferredCatalogFinalizations;
    std::array<QVariantMap, 2> m_panelSnapshots;
    std::array<bool, 2> m_stateReconciliationPending = {false, false};
    std::array<bool, 2> m_selectionActionPending = {false, false};
    std::array<PendingCursor, 2> m_pendingCursors;
    std::array<QTimer *, 2> m_cursorCommitTimers = {nullptr, nullptr};
    std::array<PendingSelection, 2> m_pendingSelections;
    PendingPanelOpen m_pendingPanelOpen;
    InFlightPanelOpen m_inFlightPanelOpen;
    DeferredPanelOpenRepeat m_deferredPanelOpenRepeat;
    QTimer *m_panelOpenWatchdog = nullptr;
    qulonglong m_panelActivationRevision = 0;
    qulonglong m_mediaFrameSerial = 0;
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
    std::array<bool, 2> m_catalogRowsRequestScheduled = {false, false};
    bool m_metadataInputBusy = false;
    QTimer *m_metadataIdleTimer = nullptr;
};
