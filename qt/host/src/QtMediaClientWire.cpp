#include "QtMediaClientWire.h"

#include <cstdint>

namespace QtMediaClientWire
{
namespace
{
void packString(msgpack::packer<msgpack::sbuffer> &packer,
                const QString &value)
{
    const QByteArray bytes = value.toUtf8();
    packer.pack_str(static_cast<uint32_t>(bytes.size()));
    packer.pack_str_body(bytes.constData(),
                         static_cast<uint32_t>(bytes.size()));
}

void packVariant(msgpack::packer<msgpack::sbuffer> &packer,
                 const QVariant &value)
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
    case QMetaType::LongLong:
        packer.pack_int64(value.toLongLong());
        return;
    case QMetaType::UInt:
    case QMetaType::UShort:
    case QMetaType::UChar:
    case QMetaType::ULongLong:
        packer.pack_uint64(value.toULongLong());
        return;
    case QMetaType::QString:
        packString(packer, value.toString());
        return;
    case QMetaType::QByteArray: {
        const QByteArray bytes = value.toByteArray();
        packer.pack_bin(static_cast<uint32_t>(bytes.size()));
        packer.pack_bin_body(bytes.constData(),
                             static_cast<uint32_t>(bytes.size()));
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
        packString(packer, value.toString());
        return;
    }
}
}

QVariant unpackObject(const msgpack::object &object)
{
    switch (object.type) {
    case msgpack::type::NIL:
        return {};
    case msgpack::type::BOOLEAN:
        return object.via.boolean;
    case msgpack::type::POSITIVE_INTEGER:
        return QVariant::fromValue<qulonglong>(object.via.u64);
    case msgpack::type::NEGATIVE_INTEGER:
        return QVariant::fromValue<qlonglong>(object.via.i64);
    case msgpack::type::STR:
        return QString::fromUtf8(object.via.str.ptr,
                                 static_cast<qsizetype>(object.via.str.size));
    case msgpack::type::BIN:
        return QByteArray(object.via.bin.ptr,
                          static_cast<qsizetype>(object.via.bin.size));
    case msgpack::type::MAP: {
        QVariantMap map;
        for (uint32_t index = 0; index < object.via.map.size; ++index) {
            const auto &item = object.via.map.ptr[index];
            map.insert(unpackObject(item.key).toString(),
                       unpackObject(item.val));
        }
        return map;
    }
    case msgpack::type::ARRAY: {
        QVariantList list;
        list.reserve(static_cast<qsizetype>(object.via.array.size));
        for (uint32_t index = 0; index < object.via.array.size; ++index) {
            list.push_back(unpackObject(object.via.array.ptr[index]));
        }
        return list;
    }
    default:
        return {};
    }
}

quint32 readBigEndianSize(const QByteArray &header)
{
    const auto byte = [&header](int index) {
        return static_cast<quint32>(
            static_cast<unsigned char>(header.at(index)));
    };
    return (byte(0) << 24) | (byte(1) << 16) | (byte(2) << 8)
        | byte(3);
}

QByteArray frameFor(const QVariantMap &message, quint32 maximumFrameSize)
{
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, message);
    if (payload.size() == 0
        || payload.size() > static_cast<qsizetype>(maximumFrameSize)) {
        return {};
    }
    const quint32 size = static_cast<quint32>(payload.size());
    QByteArray frame(4, Qt::Uninitialized);
    frame[0] = static_cast<char>((size >> 24) & 0xff);
    frame[1] = static_cast<char>((size >> 16) & 0xff);
    frame[2] = static_cast<char>((size >> 8) & 0xff);
    frame[3] = static_cast<char>(size & 0xff);
    frame.append(payload.data(), static_cast<qsizetype>(payload.size()));
    return frame;
}
}
