#pragma once

#include <QAbstractSocket>
#include <QByteArray>
#include <QElapsedTimer>
#include <QObject>
#include <QQueue>
#include <QTcpSocket>
#include <QThread>
#include <QVariant>

class QtShellMessageDecoder;

class QtShellController : public QObject
{
    Q_OBJECT
    Q_PROPERTY(int initialCols READ initialCols CONSTANT)
    Q_PROPERTY(int initialRows READ initialRows CONSTANT)
    Q_PROPERTY(bool connected READ connected NOTIFY connectedChanged)
    Q_PROPERTY(QVariantMap scene READ scene NOTIFY sceneChanged)
    Q_PROPERTY(QVariantMap presentationScene READ presentationScene NOTIFY presentationSceneChanged)
    Q_PROPERTY(QVariantMap commandLine READ commandLine NOTIFY commandLineChanged)
    Q_PROPERTY(QVariantList commandMenus READ commandMenus NOTIFY commandMenusChanged)

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
    QVariantMap scene() const { return m_scene; }
    QVariantMap presentationScene() const { return m_presentationScene; }
    QVariantMap commandLine() const { return m_commandLine; }
    QVariantList commandMenus() const { return m_commandMenus; }
    QString startupError() const { return m_startupError; }
    static bool initialSceneReadyForDisplay(const QVariantMap &scene);

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
    Q_INVOKABLE void sendQuit();

signals:
    void connectedChanged();
    void sceneChanged();
    void presentationSceneChanged();
    void panelCatalogChanged(const QVariantMap &panel);
    // Row-free projection for QML-only panel/chrome bindings. Full catalog
    // rows stay on the direct C++ bridge signal above.
    void compactPresentationChanged(const QVariantMap &patch);
    void panelActivationChanged(int activePanel, qulonglong revision);
    // Emitted after the controller cache is patched but before bridge/QML
    // observers run, so compact messages retain the same benchmark identity
    // as a full scene throughout their synchronous apply path.
    void compactMessageApplying(const QVariantMap &message);
    void commandLineChanged();
    void commandMenusChanged();
    void fatalError(const QString &message);
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
    void onSocketError(QAbstractSocket::SocketError error);
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

    bool sendMessage(const QVariantMap &message);
    bool parseConnectAddress(const QString &address);
    bool canQueueFrame(quint32 payloadSize) const;
    void processBuffer();
    void enqueueFrame(QByteArray payload, qint64 receiveDurationNs);
    void applyFrameDecoded(quint64 epoch, quint64 sequence,
                           const QVariant &decoded);
    void applyFrameDecodeFailed(quint64 epoch, quint64 sequence,
                                const QString &message);
    void applyDecodeResult(DeferredDecodeResult result);
    void scheduleDeferredDecodeResult();
    void invalidateDecodeSession();
    void failProtocol(const QString &message);

    QTcpSocket *m_socket = nullptr;
    QByteArray m_frameHeader;
    QByteArray m_framePayload;
    QElapsedTimer m_frameReceiveTimer;
    quint32 m_expectedFrameSize = 0;
    qsizetype m_frameBytesRead = 0;
    QQueue<QueuedFrameMetadata> m_queuedFrames;
    qsizetype m_queuedPayloadBytes = 0;
    qsizetype m_applyingPayloadBytes = 0;
    QQueue<DeferredDecodeResult> m_deferredDecodeResults;
    QThread m_decodeThread;
    QtShellMessageDecoder *m_decoder = nullptr;
    quint64 m_decodeEpoch = 1;
    quint64 m_nextDecodeSequence = 1;
    quint64 m_nextApplySequence = 1;
    quint64 m_nextSendSequence = 1;
    quint64 m_nextKeySequence = 1;
    quint64 m_nextActionSequence = 1;
    bool m_applyInProgress = false;
    bool m_deferredDecodeScheduled = false;
    bool m_acceptDecodedFrames = true;
    bool m_protocolFailed = false;
    QString m_host;
    quint16 m_port = 0;
    QString m_nonce;
    int m_initialCols = 100;
    int m_initialRows = 30;
    bool m_connected = false;
    bool m_helloSent = false;
    bool m_initialHandshakeComplete = false;
    QString m_startupError;
    QVariantMap m_scene;
    QVariantMap m_presentationScene;
    qulonglong m_panelActivationRevision = 0;
    QVariantMap m_commandLine;
    QVariantList m_commandMenus;
};
