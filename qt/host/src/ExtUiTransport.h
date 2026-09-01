#pragma once

#include <QByteArray>
#include <QElapsedTimer>
#include <QObject>
#include <QTcpSocket>
#include <QVariantMap>

#include <functional>

class ExtUiTransport final : public QObject
{
    Q_OBJECT

public:
    static constexpr quint32 MaxMessageSize = 64 * 1024 * 1024;

    struct Frame
    {
        QByteArray payload;
        qint64 receiveDurationNs = 0;
    };

    enum class ReadDisposition
    {
        FrameReady,
        NeedData,
        Backpressure,
        Fatal,
    };

    explicit ExtUiTransport(QObject *parent = nullptr);

    bool connectToAddress(const QString &address, QString *error);
    bool connected() const;
    bool connecting() const;
    bool waitForConnected(int timeoutMs);
    void abort();
    QString errorString() const;
    QString endpoint() const;

    bool sendMessage(const QVariantMap &message, QString *error = nullptr);
    ReadDisposition takeFrame(
        const std::function<bool(quint32)> &canAccept,
        Frame *frame, QString *error);

signals:
    void connectedToPeer();
    void readyRead();
    void disconnectedFromPeer();
    void transportError(const QString &message);

private:
    struct SendTrace;
    SendTrace beginSendTrace(const QVariantMap &message);
    QByteArray packWireFrame(const QVariantMap &message,
                             SendTrace *trace,
                             QString *error) const;
    bool writeWireFrame(const QByteArray &wireFrame,
                        SendTrace *trace) const;
    QVariantMap sendTraceFields(const QVariantMap &message,
                                const SendTrace &trace) const;
    void recordPackTrace(const QVariantMap &message,
                         const SendTrace &trace) const;
    void recordWriteTrace(const QVariantMap &message,
                          const SendTrace &trace) const;
    static bool parseAddress(const QString &address, QString *host,
                             quint16 *port);

    QTcpSocket *m_socket = nullptr;
    QString m_host;
    quint16 m_port = 0;
    QByteArray m_frameHeader;
    QByteArray m_framePayload;
    QElapsedTimer m_frameReceiveTimer;
    quint32 m_expectedFrameSize = 0;
    qsizetype m_frameBytesRead = 0;
    quint64 m_nextSendSequence = 1;
};
