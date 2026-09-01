#pragma once

#include "ExtUiProtocol.h"
#include "ExtUiStateStores.h"
#include "ShellStateStore.h"

#include <QByteArray>
#include <QList>
#include <QObject>
#include <QQueue>
#include <QThread>
#include <QVariant>

#include <functional>
#include <array>

#if defined(F4_EXTUI_REDUCER_TEST_HARNESS) \
    || defined(F4_MEDIA_CLIENT_TESTING)
#define F4_QT_SCENE_TEST_API 1
#endif

class ExtUiMessageDecoder;
class ExtUiTransport;
namespace ExtUiSceneReducer
{
struct AppliedScenePatch;
}

class QtShellController : public QObject
{
    Q_OBJECT
    Q_PROPERTY(int initialCols READ initialCols CONSTANT)
    Q_PROPERTY(int initialRows READ initialRows CONSTANT)
    Q_PROPERTY(bool connected READ connected NOTIFY connectedChanged)
    Q_PROPERTY(ShellStateStore* shellState READ shellState CONSTANT)
    Q_PROPERTY(ChromeStateStore* chromeState READ chromeState CONSTANT)
    Q_PROPERTY(WorkspaceStateStore* workspaceState READ workspaceState CONSTANT)
    Q_PROPERTY(OverlayStateStore* overlayState READ overlayState CONSTANT)
    Q_PROPERTY(CommandLineStateStore* commandLineState READ commandLineState CONSTANT)
    Q_PROPERTY(SurfaceRegistry* surfaceRegistry READ surfaceRegistry CONSTANT)

public:
    explicit QtShellController(const QString &connectAddress,
                               const QString &nonce,
                               int cols,
                               int rows,
                               QObject *parent = nullptr);
    ~QtShellController() override;

    int initialCols() const { return m_initialCols; }
    int initialRows() const { return m_initialRows; }
    bool connected() const { return m_connected; }
#if defined(F4_QT_SCENE_TEST_API)
    // Complete maps are a reducer oracle for protocol tests only. They are
    // absent from production builds as well as the QML property surface.
    QVariantMap scene() const { return m_scene; }
    QVariantMap presentationScene() const { return m_presentationScene; }
#endif
    ShellStateStore *shellState() const { return m_shellState; }
    ChromeStateStore *chromeState() const { return m_chromeState; }
    WorkspaceStateStore *workspaceState() const { return m_workspaceState; }
    OverlayStateStore *overlayState() const { return m_overlayState; }
    CommandLineStateStore *commandLineState() const
    {
        return m_commandLineState;
    }
    SurfaceRegistry *surfaceRegistry() const { return m_surfaceRegistry; }
    QVariantMap commandLine() const { return m_commandLineState->frame(); }
    QVariantList commandMenus() const
    {
        return m_overlayState->commandMenus();
    }
    QVariantList commandMenuStates() const
    {
        return m_overlayState->commandMenuStates();
    }
    QString startupError() const { return m_startupError; }
    static bool initialSceneReadyForDisplay(const QVariantMap &scene);
    bool initialStateReadyForDisplay() const;
    QVariantMap panelCatalogSnapshot(int side) const;
#if defined(F4_EXTUI_PRODUCTION_TESTING)
    bool retainsMasterSceneForTesting() const
    {
        return !m_scene.isEmpty() || !m_presentationScene.isEmpty();
    }
#endif

    // Deliberately not a Qt signal, slot, property or invokable: the media
    // endpoint and nonce are bearer capabilities for native transport code
    // and must not enter the QML meta-object surface exposed as qtShell.
    void setMediaAdvertisementHandler(
        std::function<void(const QVariantMap &)> handler);
    void setPlatformRequestHandler(
        std::function<void(const QVariantMap &)> handler);
    bool sendPlatformMessage(const QVariantMap &message);

    // Opportunistically completes the initial loopback connection without
    // running a nested event loop. A timeout leaves the asynchronous socket
    // connection active so normal event-loop startup remains the fallback.
    bool waitForInitialHandshake(int timeoutMs);

    // Completes the same connection/client-hello phase using the controller's
    // full startup deadline. Failure is latched synchronously so callers can
    // return before constructing heavyweight UI/runtime objects.
    bool completeInitialHandshake();

    Q_INVOKABLE void sendResize(int cols, int rows);
    Q_INVOKABLE void sendKey(int vk, int ch, bool down, int mods);
    void sendKeyEvent(int vk, int ch, bool down, int mods, bool repeat);
    Q_INVOKABLE void sendText(const QString &text, int mods = 0);
    Q_INVOKABLE void sendMouse(int x, int y, int button, int flags, bool down, int mods);
    Q_INVOKABLE void sendWheel(int x, int y, int dir, int mods);
    Q_INVOKABLE void sendPaste(const QString &text);
    Q_INVOKABLE void sendClipboardGet();
    Q_INVOKABLE void sendClipboardSet(const QString &text);
    Q_INVOKABLE void sendUiAction(const QVariantMap &action);
    Q_INVOKABLE void sendPanelCatalogMetadataRequest(
        const QVariantMap &request);
    Q_INVOKABLE void sendPanelCatalogRowsRequest(
        const QVariantMap &request);
    Q_INVOKABLE void sendQuit();

signals:
    void connectedChanged();
#if defined(F4_QT_SCENE_TEST_API)
    void sceneChanged();
    void presentationSceneChanged();
#endif
    void panelCatalogChanged(const QVariantMap &panel);
    void panelCatalogAppendChanged(const QVariantMap &append);
    // Row-free panel state and sparse selection updates. Catalog rows remain
    // immutable between catalog notifications; large catalogs extend through
    // panelCatalogAppendChanged without another model reset.
    void panelStateChanged(const QVariantMap &patch);
    // Row-free projection for QML-only panel/chrome bindings. Full catalog
    // rows stay on the direct C++ bridge signal above.
    void compactPresentationChanged(const QVariantMap &patch);
    void panelActivationChanged(int activePanel, qulonglong revision);
    // Emitted after the controller cache is patched but before bridge/QML
    // observers run, so compact messages retain the same benchmark identity
    // as a full scene throughout their synchronous apply path.
    void compactMessageApplying(const QVariantMap &message);
    // Native-only completion lanes. These messages may contain catalog rows
    // and must never travel through the public presentation signal below.
    void compactMessageApplied(const QVariantMap &message);
    void panelCatalogRowsReceived(const QVariantMap &message);
    void panelCatalogMetadataReceived(const QVariantMap &message);
    void commandLineChanged();
    void commandMenusChanged();
    // Selection/viewport-only menu updates stay off commandMenusChanged so a
    // QML Repeater does not destroy and recreate every popup row on Up/Down.
    void commandMenuStatesChanged(const QVariantList &states);
    void qmlIconSetChanged(const QString &name);
    void fatalError(const QString &message);
    // Presentation-only non-semantic messages used by the retained terminal
    // grid (palette/frame/cursor/clipboard/quit) and sanitized handshake data.
    // ExtUI semantic messages use the typed signals above and state stores.
    void messageReceived(const QVariantMap &message);

    // Emitted after a complete frame has been removed from the socket but
    // before it is handed to the decoder thread. Several bounded frames may
    // be queued before an earlier result is applied. This is intentionally a
    // lightweight diagnostic boundary; decoding never runs on this thread.
    void frameDecodeQueued(quint64 sequence);

private slots:
    void onConnected();
    void onReadyRead();
    void onDisconnected();
    void onTransportError(const QString &message);
    void onFrameDecoded(quint64 epoch, quint64 sequence,
                        const QVariant &decoded);
    void onFrameDecodeFailed(quint64 epoch, quint64 sequence,
                             const QString &message);

private:
    struct QueuedFrameMetadata {
        quint64 sequence = 0;
        qsizetype payloadBytes = 0;
        qint64 receivedNs = 0;
        qint64 receiveDurationNs = 0;
    };

    struct DeferredDecodeResult {
        quint64 epoch = 0;
        quint64 sequence = 0;
        QVariant decoded;
        QString error;
        bool failed = false;
    };

    struct FrameApplyTrace;

    bool sendMessage(const QVariantMap &message);
    bool canQueueFrame(quint32 payloadSize) const;
    void processBuffer();
    void enqueueFrame(QByteArray payload, qint64 receiveDurationNs);
    void applyFrameDecoded(quint64 epoch, quint64 sequence,
                           const QVariant &decoded);
    bool applyHelloFrame(const QVariantMap &message);
    bool applyStreamSnapshotFrame(const ExtUiProtocol::Envelope &envelope,
                                  const QVariantMap &message);
    bool applyFullSceneFrame(const QVariantMap &message,
                             bool hasSemanticEnvelope,
                             FrameApplyTrace *trace);
    bool applyScenePatchFrame(const QVariantMap &message,
                              const ExtUiProtocol::Envelope &envelope,
                              bool hasSemanticEnvelope,
                              FrameApplyTrace *trace);
    void emitScenePatchSignals(
        const QVariantMap &message,
        const ExtUiSceneReducer::AppliedScenePatch &applied,
        FrameApplyTrace *trace);
    void emitScenePatchRootSignals(
        const ExtUiSceneReducer::AppliedScenePatch &applied,
        FrameApplyTrace *trace);
    bool applyPanelCatalogFrame(const QVariantMap &message,
                                const ExtUiProtocol::Envelope &envelope,
                                bool hasSemanticEnvelope,
                                FrameApplyTrace *trace);
    void traceRejectedPanelCatalog(const QVariantMap &message,
                                   const QVariantMap &panel,
                                   const FrameApplyTrace &trace) const;
    bool commitPanelCatalogFrame(const QVariantMap &message, int side,
                                 const QVariantMap &panel,
                                 const ExtUiProtocol::Envelope &envelope,
                                 bool hasSemanticEnvelope,
                                 FrameApplyTrace *trace);
    bool applyPanelChromeFrame(const QVariantMap &message,
                               const ExtUiProtocol::Envelope &envelope,
                               bool hasSemanticEnvelope);
    bool applyPanelActivationFrame(const QVariantMap &message,
                                   const ExtUiProtocol::Envelope &envelope,
                                   bool hasSemanticEnvelope,
                                   FrameApplyTrace *trace);
    bool applyCommandLineFrame(const QVariantMap &message,
                               const ExtUiProtocol::Envelope &envelope,
                               bool hasSemanticEnvelope);
    bool dispatchDecodedFrame(const QString &messageType,
                              const QVariantMap &message,
                              const ExtUiProtocol::Envelope &envelope,
                              bool hasSemanticEnvelope,
                              FrameApplyTrace *trace);
    void finalizeDecodedFrame(const QString &messageType,
                              const QVariantMap &message,
                              const ExtUiProtocol::Envelope &envelope,
                              bool hasSemanticEnvelope,
                              quint64 sequence,
                              const QueuedFrameMetadata &frame,
                              FrameApplyTrace *trace);
    void traceDecodedFrame(const QString &messageType, quint64 sequence,
                           const QueuedFrameMetadata &frame,
                           const FrameApplyTrace &trace) const;
    void traceFullSceneFrame(const QString &messageType, quint64 sequence,
                             const QueuedFrameMetadata &frame,
                             const FrameApplyTrace &trace) const;
    void traceFrameCompletion(const QString &messageType, quint64 sequence,
                              const QueuedFrameMetadata &frame,
                              const FrameApplyTrace &trace) const;
    void applyFrameDecodeFailed(quint64 epoch, quint64 sequence,
                                const QString &message);
    void applyDecodeResult(DeferredDecodeResult result);
    void scheduleDeferredDecodeResult();
    void invalidateDecodeSession();
    void failProtocol(const QString &message);
    void updateCommandMenus(const QVariantList &menus,
                            bool allowStateOnlyUpdate);
    void synchronizeTypedState(const QString &messageType,
                               const QVariantMap &message,
                               const ExtUiProtocol::Envelope &envelope,
                               bool hasSemanticEnvelope);
    void synchronizeLegacyTypedState(const QString &messageType,
                                     const QVariantMap &message,
                                     qulonglong revision);
    void synchronizeTypedStreamOwnedState(
        const QString &messageType, const QVariantMap &message,
        const ExtUiProtocol::Envelope &envelope);
    void synchronizeTypedCrossStreamPatch(
        const QVariantMap &message,
        const ExtUiProtocol::Envelope &envelope);
    void synchronizeAllTypedState(qulonglong revision,
                                  bool allowMenuStateOnlyUpdate = false);
    QVariantMap streamReducerScene(const QString &streamId) const;
    void commitTypedScenePatch(
        const QString &streamId,
        const ExtUiSceneReducer::AppliedScenePatch &applied);
    void applyCompactFieldsToTypedState(const QVariantMap &message,
                                        int activePanel,
                                        qulonglong revision,
                                        bool replacePanelDescriptor,
                                        const QVariantMap &panel = {});

    ShellStateStore *m_shellState = nullptr;
    ChromeStateStore *m_chromeState = nullptr;
    WorkspaceStateStore *m_workspaceState = nullptr;
    OverlayStateStore *m_overlayState = nullptr;
    CommandLineStateStore *m_commandLineState = nullptr;
    SurfaceRegistry *m_surfaceRegistry = nullptr;
    ExtUiTransport *m_transport = nullptr;
    QQueue<QueuedFrameMetadata> m_queuedFrames;
    qsizetype m_queuedPayloadBytes = 0;
    qsizetype m_applyingPayloadBytes = 0;
    QQueue<DeferredDecodeResult> m_deferredDecodeResults;
    QThread m_decodeThread;
    ExtUiMessageDecoder *m_decoder = nullptr;
    quint64 m_decodeEpoch = 1;
    quint64 m_nextDecodeSequence = 1;
    quint64 m_nextApplySequence = 1;
    quint64 m_nextKeySequence = 1;
    quint64 m_nextActionSequence = 1;
    bool m_applyInProgress = false;
    bool m_deferredDecodeScheduled = false;
    bool m_acceptDecodedFrames = true;
    bool m_protocolFailed = false;
    QString m_nonce;
    int m_initialCols = 100;
    int m_initialRows = 30;
    bool m_connected = false;
    bool m_helloSent = false;
    bool m_serverHandshakeComplete = false;
    bool m_initialHandshakeComplete = false;
    QString m_startupError;
    QVariantMap m_scene;
    QVariantMap m_presentationScene;
    qulonglong m_sceneRevision = 0;
    qulonglong m_panelActivationRevision = 0;
    ExtUiProtocol::StreamRegistry m_streamRegistry;
    QVariantMap m_mediaAdvertisement;
    std::array<QVariantMap, 2> m_panelCatalogSnapshots;
    std::function<void(const QVariantMap &)> m_mediaAdvertisementHandler;
    std::function<void(const QVariantMap &)> m_platformRequestHandler;
};
