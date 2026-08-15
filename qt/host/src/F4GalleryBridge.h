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

class QQmlEngine;
class QTimer;
class F4IconSet;

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
                             F4IconSet *iconSet = nullptr);
    ~F4GalleryBridge() override;

    bool available() const;
    QObject *viewerSession() const;
    bool viewerVisible() const { return m_viewerVisible; }
    int viewerSide() const { return m_viewerSide; }
    QUrl panelComponentUrl() const;
    QUrl viewerComponentUrl() const;
    bool navigationBenchmarkEnabled() const;

    Q_INVOKABLE QObject *sessionForSide(int side) const;
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
                                 qulonglong catalogRevision = 0);
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
    Q_INVOKABLE void closeViewer();

public slots:
    void synchronizeScene(const QVariantMap &scene);
    // main.cpp connects QQuickWindow::frameSwapped to this slot. Keeping the
    // window dependency out of the bridge makes the runner testable with a
    // synthetic frame boundary.
    void notifyFrameSwapped();

signals:
    void uiActionRequested(const QVariantMap &action);
    void viewerChanged();
    void benchmarkFrameSwapped(qulonglong serial);

private:
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
        QString galleryLayoutMode;
        QVariantList entries;
        QStringList selectedEntryIdList;
        QHash<QString, int> sourceIndexByEntryId;
        QSet<QString> entryIds;
        QSet<QString> selectedEntryIds;
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
    static QVariantList normalizedEntries(const QVariantMap &panel);
    QVariantList normalizedAppearance(const QVariantMap &panel) const;
    static QStringList selectedEntryIds(const QVariantList &entries);
    static int sourceIndexForEntryId(const QVariantList &entries,
                                     const QString &entryId);
    static qulonglong revisionValue(const QVariantMap &map, const QString &key);

    bool canSkipUnchangedInactivePanel(int side,
                                       const QVariantMap &panel) const;
    void synchronizePanel(int side, const QVariantMap &panel);
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
    std::array<bool, 2> m_stateReconciliationPending = {false, false};
    std::array<bool, 2> m_selectionActionPending = {false, false};
    std::array<PendingCursor, 2> m_pendingCursors;
    std::array<QTimer *, 2> m_cursorCommitTimers = {nullptr, nullptr};
    std::array<PendingSelection, 2> m_pendingSelections;
    PendingPanelOpen m_pendingPanelOpen;
    PendingViewer m_pendingViewer;
    bool m_viewerVisible = false;
    int m_viewerSide = -1;
    NavigationBenchmarkState m_navigationBenchmark;
    QTimer *m_navigationBenchmarkWatchdog = nullptr;
    QList<PendingNavigationBenchmarkTrace> m_pendingNavigationBenchmarkTrace;
};
