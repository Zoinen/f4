#pragma once

#include <QAbstractSocket>
#include <QByteArray>
#include <QObject>
#include <QTcpSocket>
#include <QVariant>

class QtShellController : public QObject
{
    Q_OBJECT
    Q_PROPERTY(int initialCols READ initialCols CONSTANT)
    Q_PROPERTY(int initialRows READ initialRows CONSTANT)
    Q_PROPERTY(bool connected READ connected NOTIFY connectedChanged)
    Q_PROPERTY(QVariantMap scene READ scene NOTIFY sceneChanged)

public:
    explicit QtShellController(const QString &connectAddress,
                               const QString &nonce,
                               int cols,
                               int rows,
                               QObject *parent = nullptr);

    int initialCols() const { return m_initialCols; }
    int initialRows() const { return m_initialRows; }
    bool connected() const { return m_connected; }
    QVariantMap scene() const { return m_scene; }

    Q_INVOKABLE void sendResize(int cols, int rows);
    Q_INVOKABLE void sendKey(int vk, int ch, bool down, int mods);
    Q_INVOKABLE void sendText(const QString &text, int mods = 0);
    Q_INVOKABLE void sendMouse(int x, int y, int button, int flags, bool down, int mods);
    Q_INVOKABLE void sendWheel(int x, int y, int dir, int mods);
    Q_INVOKABLE void sendPaste(const QString &text);
    Q_INVOKABLE void sendClipboardGet();
    Q_INVOKABLE void sendClipboardSet(const QString &text);
    Q_INVOKABLE void sendUiAction(const QVariantMap &action);
    Q_INVOKABLE void sendQuit();

signals:
    void connectedChanged();
    void sceneChanged();
    void fatalError(const QString &message);
    void messageReceived(const QVariantMap &message);

private slots:
    void onConnected();
    void onReadyRead();
    void onDisconnected();
    void onSocketError(QAbstractSocket::SocketError error);

private:
    bool sendMessage(const QVariantMap &message);
    bool parseConnectAddress(const QString &address);
    void processBuffer();

    QTcpSocket *m_socket = nullptr;
    QByteArray m_readBuffer;
    QString m_host;
    quint16 m_port = 0;
    QString m_nonce;
    int m_initialCols = 100;
    int m_initialRows = 30;
    bool m_connected = false;
    QVariantMap m_scene;
};
