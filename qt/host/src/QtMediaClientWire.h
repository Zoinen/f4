#pragma once

#include <QByteArray>
#include <QVariant>
#include <QVariantMap>

#include <msgpack.hpp>

#include <QtGlobal>

namespace QtMediaClientWire
{
QVariant unpackObject(const msgpack::object &object);
quint32 readBigEndianSize(const QByteArray &header);
QByteArray frameFor(const QVariantMap &message, quint32 maximumFrameSize);
}
