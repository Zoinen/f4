#include "QtShellController.h"

#include <QCoreApplication>
#include <QDebug>

#include <msgpack.hpp>

#include <array>
#include <cstdint>
#include <exception>

namespace
{
constexpr quint32 MaxMessageSize = 64 * 1024 * 1024;

void packString(msgpack::packer<msgpack::sbuffer> &packer, const QString &value)
{
    const QByteArray bytes = value.toUtf8();
    packer.pack_str(static_cast<uint32_t>(bytes.size()));
    packer.pack_str_body(bytes.constData(), static_cast<uint32_t>(bytes.size()));
}

void packVariant(msgpack::packer<msgpack::sbuffer> &packer, const QVariant &value)
{
    if (!value.isValid() || value.isNull()) {
        packer.pack_nil();
        return;
    }

    switch (value.typeId()) {
    case QMetaType::Bool:
        packer.pack(value.toBool());
        return;
    case QMetaType::Int:
    case QMetaType::Short:
    case QMetaType::SChar:
        packer.pack_int64(value.toLongLong());
        return;
    case QMetaType::UInt:
    case QMetaType::UShort:
    case QMetaType::UChar:
        packer.pack_uint64(value.toULongLong());
        return;
    case QMetaType::LongLong:
        packer.pack_int64(value.toLongLong());
        return;
    case QMetaType::ULongLong:
        packer.pack_uint64(value.toULongLong());
        return;
    case QMetaType::Double:
    case QMetaType::Float:
        packer.pack_double(value.toDouble());
        return;
    case QMetaType::QString:
        packString(packer, value.toString());
        return;
    case QMetaType::QVariantList: {
        const QVariantList list = value.toList();
        packer.pack_array(static_cast<uint32_t>(list.size()));
        for (const QVariant &item : list) {
            packVariant(packer, item);
        }
        return;
    }
    case QMetaType::QVariantMap: {
        const QVariantMap map = value.toMap();
        packer.pack_map(static_cast<uint32_t>(map.size()));
        for (auto it = map.cbegin(); it != map.cend(); ++it) {
            packString(packer, it.key());
            packVariant(packer, it.value());
        }
        return;
    }
    default:
        if (value.canConvert<QString>()) {
            packString(packer, value.toString());
        } else {
            packer.pack_nil();
        }
        return;
    }
}

QVariant unpackObject(const msgpack::object &object)
{
    switch (object.type) {
    case msgpack::type::NIL:
        return QVariant();
    case msgpack::type::BOOLEAN:
        return QVariant(object.via.boolean);
    case msgpack::type::POSITIVE_INTEGER:
        return QVariant::fromValue<qulonglong>(object.via.u64);
    case msgpack::type::NEGATIVE_INTEGER:
        return QVariant::fromValue<qlonglong>(object.via.i64);
    case msgpack::type::FLOAT32:
    case msgpack::type::FLOAT64:
        return QVariant(object.via.f64);
    case msgpack::type::STR:
        return QString::fromUtf8(object.via.str.ptr, static_cast<qsizetype>(object.via.str.size));
    case msgpack::type::BIN:
        return QByteArray(object.via.bin.ptr, static_cast<qsizetype>(object.via.bin.size));
    case msgpack::type::ARRAY: {
        QVariantList list;
        list.reserve(static_cast<qsizetype>(object.via.array.size));
        for (uint32_t i = 0; i < object.via.array.size; ++i) {
            list.push_back(unpackObject(object.via.array.ptr[i]));
        }
        return list;
    }
    case msgpack::type::MAP: {
        QVariantMap map;
        for (uint32_t i = 0; i < object.via.map.size; ++i) {
            const QString key = unpackObject(object.via.map.ptr[i].key).toString();
            map.insert(key, unpackObject(object.via.map.ptr[i].val));
        }
        return map;
    }
    default:
        return QVariant();
    }
}

quint32 readBigEndianSize(const QByteArray &buffer)
{
    return (static_cast<quint32>(static_cast<unsigned char>(buffer[0])) << 24)
        | (static_cast<quint32>(static_cast<unsigned char>(buffer[1])) << 16)
        | (static_cast<quint32>(static_cast<unsigned char>(buffer[2])) << 8)
        | static_cast<quint32>(static_cast<unsigned char>(buffer[3]));
}

void writeBigEndianSize(QByteArray &buffer, quint32 size)
{
    buffer.resize(4);
    buffer[0] = static_cast<char>((size >> 24) & 0xff);
    buffer[1] = static_cast<char>((size >> 16) & 0xff);
    buffer[2] = static_cast<char>((size >> 8) & 0xff);
    buffer[3] = static_cast<char>(size & 0xff);
}
}

QtShellController::QtShellController(const QString &connectAddress,
                                     const QString &nonce,
                                     int cols,
                                     int rows,
                                     QObject *parent)
    : QObject(parent)
    , m_socket(new QTcpSocket(this))
    , m_nonce(nonce)
    , m_initialCols(cols)
    , m_initialRows(rows)
{
    if (!parseConnectAddress(connectAddress)) {
        emit fatalError(QStringLiteral("Invalid ExtUI connect address: %1").arg(connectAddress));
        return;
    }

    connect(m_socket, &QTcpSocket::connected, this, &QtShellController::onConnected);
    connect(m_socket, &QTcpSocket::readyRead, this, &QtShellController::onReadyRead);
    connect(m_socket, &QTcpSocket::disconnected, this, &QtShellController::onDisconnected);
    connect(m_socket, &QTcpSocket::errorOccurred, this, &QtShellController::onSocketError);

    m_socket->connectToHost(m_host, m_port);
}

void QtShellController::sendResize(int cols, int rows)
{
    if (cols <= 0 || rows <= 0) {
        return;
    }
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("resize")},
        {QStringLiteral("cols"), cols},
        {QStringLiteral("rows"), rows},
    });
}

void QtShellController::sendKey(int vk, int ch, bool down, int mods)
{
    sendKeyEvent(vk, ch, down, mods, false);
}

void QtShellController::sendKeyEvent(int vk, int ch, bool down, int mods, bool repeat)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("key")},
        {QStringLiteral("vk"), vk},
        {QStringLiteral("char"), ch},
        {QStringLiteral("down"), down},
        {QStringLiteral("mods"), mods},
        {QStringLiteral("repeat"), repeat},
    });
}

void QtShellController::sendText(const QString &text, int mods)
{
    if (text.isEmpty()) {
        return;
    }
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("text")},
        {QStringLiteral("text"), text},
        {QStringLiteral("mods"), mods},
    });
}

void QtShellController::sendMouse(int x, int y, int button, int flags, bool down, int mods)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("mouse")},
        {QStringLiteral("x"), x},
        {QStringLiteral("y"), y},
        {QStringLiteral("button"), button},
        {QStringLiteral("flags"), flags},
        {QStringLiteral("down"), down},
        {QStringLiteral("mods"), mods},
    });
}

void QtShellController::sendWheel(int x, int y, int dir, int mods)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("wheel")},
        {QStringLiteral("x"), x},
        {QStringLiteral("y"), y},
        {QStringLiteral("dir"), dir},
        {QStringLiteral("mods"), mods},
    });
}

void QtShellController::sendPaste(const QString &text)
{
    if (text.isEmpty()) {
        return;
    }
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("paste")},
        {QStringLiteral("text"), text},
    });
}

void QtShellController::sendClipboardGet()
{
    sendMessage({{QStringLiteral("type"), QStringLiteral("clipboard_get")}});
}

void QtShellController::sendClipboardSet(const QString &text)
{
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("clipboard_set")},
        {QStringLiteral("text"), text},
    });
}

void QtShellController::sendUiAction(const QVariantMap &action)
{
    QVariantMap message = action;
    message.insert(QStringLiteral("type"), QStringLiteral("ui_action"));
    sendMessage(message);
}

void QtShellController::sendQuit()
{
    sendMessage({{QStringLiteral("type"), QStringLiteral("quit")}});
}

void QtShellController::onConnected()
{
    m_connected = true;
    emit connectedChanged();

    const int cellWidth = 10;
    const int cellHeight = 20;
    sendMessage({
        {QStringLiteral("type"), QStringLiteral("hello")},
        {QStringLiteral("nonce"), m_nonce},
        {QStringLiteral("cols"), m_initialCols},
        {QStringLiteral("rows"), m_initialRows},
        {QStringLiteral("pixelWidth"), m_initialCols * cellWidth},
        {QStringLiteral("pixelHeight"), m_initialRows * cellHeight},
        {QStringLiteral("cellWidth"), cellWidth},
        {QStringLiteral("cellHeight"), cellHeight},
    });
}

void QtShellController::onReadyRead()
{
    m_readBuffer.append(m_socket->readAll());
    processBuffer();
}

void QtShellController::onDisconnected()
{
    if (m_connected) {
        m_connected = false;
        emit connectedChanged();
    }
    QCoreApplication::quit();
}

void QtShellController::onSocketError(QAbstractSocket::SocketError)
{
    emit fatalError(m_socket->errorString());
}

bool QtShellController::sendMessage(const QVariantMap &message)
{
    if (!m_socket || m_socket->state() != QAbstractSocket::ConnectedState) {
        return false;
    }

    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, message);

    if (payload.size() > MaxMessageSize) {
        emit fatalError(QStringLiteral("Message too large for f4 Qt protocol"));
        return false;
    }

    QByteArray frame;
    writeBigEndianSize(frame, static_cast<quint32>(payload.size()));
    frame.append(payload.data(), static_cast<qsizetype>(payload.size()));

    const qint64 written = m_socket->write(frame);
    if (written != frame.size()) {
        emit fatalError(QStringLiteral("Failed to write complete IPC frame"));
        return false;
    }
    m_socket->flush();
    return true;
}

bool QtShellController::parseConnectAddress(const QString &address)
{
    const int split = address.lastIndexOf(QLatin1Char(':'));
    if (split <= 0 || split == address.size() - 1) {
        return false;
    }

    bool ok = false;
    const int port = address.mid(split + 1).toInt(&ok);
    if (!ok || port <= 0 || port > 65535) {
        return false;
    }

    m_host = address.left(split);
    m_port = static_cast<quint16>(port);
    return true;
}

void QtShellController::processBuffer()
{
    while (m_readBuffer.size() >= 4) {
        const quint32 size = readBigEndianSize(m_readBuffer);
        if (size == 0 || size > MaxMessageSize) {
            emit fatalError(QStringLiteral("Invalid IPC frame size from f4"));
            m_socket->disconnectFromHost();
            return;
        }
        if (m_readBuffer.size() < static_cast<int>(size + 4)) {
            return;
        }

        const QByteArray payload = m_readBuffer.mid(4, static_cast<qsizetype>(size));
        m_readBuffer.remove(0, static_cast<qsizetype>(size + 4));

        try {
            msgpack::object_handle handle = msgpack::unpack(payload.constData(), static_cast<size_t>(payload.size()));
            QVariant decoded = unpackObject(handle.get());
            if (decoded.metaType().id() == QMetaType::QVariantMap) {
                const QVariantMap message = decoded.toMap();
                if (message.value(QStringLiteral("type")).toString() == QStringLiteral("scene")) {
                    m_scene = message;
                    emit sceneChanged();
                }
                emit messageReceived(message);
            }
        } catch (const std::exception &e) {
            emit fatalError(QStringLiteral("Failed to decode IPC frame: %1").arg(QString::fromUtf8(e.what())));
            m_socket->disconnectFromHost();
            return;
        }
    }
}
