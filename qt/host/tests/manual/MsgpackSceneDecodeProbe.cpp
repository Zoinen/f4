#include <QCoreApplication>
#include <QElapsedTimer>
#include <QString>
#include <QVariantList>
#include <QVariantMap>

#include <msgpack.hpp>

#include <algorithm>
#include <cmath>
#include <cstdint>
#include <iostream>
#include <vector>

namespace
{
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
    case QMetaType::LongLong:
        packer.pack_int64(value.toLongLong());
        return;
    case QMetaType::UInt:
    case QMetaType::UShort:
    case QMetaType::UChar:
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
        for (const QVariant &item : list)
            packVariant(packer, item);
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
        if (value.canConvert<QString>())
            packString(packer, value.toString());
        else
            packer.pack_nil();
    }
}

QVariant unpackObject(const msgpack::object &object)
{
    switch (object.type) {
    case msgpack::type::NIL:
        return {};
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
        return QString::fromUtf8(object.via.str.ptr,
                                 static_cast<qsizetype>(object.via.str.size));
    case msgpack::type::BIN:
        return QByteArray(object.via.bin.ptr,
                          static_cast<qsizetype>(object.via.bin.size));
    case msgpack::type::ARRAY: {
        QVariantList list;
        list.reserve(static_cast<qsizetype>(object.via.array.size));
        for (uint32_t index = 0; index < object.via.array.size; ++index)
            list.push_back(unpackObject(object.via.array.ptr[index]));
        return list;
    }
    case msgpack::type::MAP: {
        QVariantMap map;
        for (uint32_t index = 0; index < object.via.map.size; ++index) {
            const QString key = unpackObject(object.via.map.ptr[index].key)
                                    .toString();
            map.insert(key, unpackObject(object.via.map.ptr[index].val));
        }
        return map;
    }
    default:
        return {};
    }
}

QVariantList denseRows(int first, int count, int runsPerRow)
{
    QVariantList rows;
    rows.reserve(count);
    for (int row = 0; row < count; ++row) {
        QVariantList runs;
        runs.reserve(runsPerRow);
        for (int run = 0; run < runsPerRow; ++run) {
            runs.push_back(QVariantMap{
                {QStringLiteral("text"),
                 QString(QChar(QLatin1Char('!').unicode() + run % 80))},
                {QStringLiteral("attr"), static_cast<qulonglong>(run % 9)},
                {QStringLiteral("foreground"),
                 run & 1 ? QStringLiteral("#d8dee9")
                         : QStringLiteral("#88c0d0")},
                {QStringLiteral("background"), QStringLiteral("#101419")},
                {QStringLiteral("bold"), run % 7 == 0},
                {QStringLiteral("underline"), false},
                {QStringLiteral("strikeout"), false},
            });
        }
        const int visualRow = first + row;
        rows.push_back(QVariantMap{
            {QStringLiteral("index"), row},
            {QStringLiteral("visualRow"), visualRow},
            {QStringLiteral("logicalLine"), visualRow},
            {QStringLiteral("offset"), static_cast<qlonglong>(visualRow * 160)},
            {QStringLiteral("endOffset"),
             static_cast<qlonglong>(visualRow * 160 + 159)},
            {QStringLiteral("text"), QString()},
            {QStringLiteral("runs"), runs},
        });
    }
    return rows;
}

QVariantMap representativeScene()
{
    constexpr int viewportRows = 105;
    constexpr int windowRows = viewportRows * 3;
    constexpr int runsPerRow = 45;
    const QVariantList window = denseRows(100000, windowRows, runsPerRow);
    const QVariantList visible = window.mid(viewportRows, viewportRows);
    const QVariantMap surface{
        {QStringLiteral("id"), QStringLiteral("editor-4k-dense")},
        {QStringLiteral("kind"), QStringLiteral("editor")},
        {QStringLiteral("documentKey"), QStringLiteral("editor-4k-dense")},
        {QStringLiteral("scrollAction"), QStringLiteral("editor.scroll")},
        {QStringLiteral("scrollUnit"), QStringLiteral("rows")},
        {QStringLiteral("windowStart"), 100000},
        {QStringLiteral("windowEnd"), 100000 + windowRows},
        {QStringLiteral("viewportStart"), 100000 + viewportRows},
        {QStringLiteral("viewportSpan"), viewportRows},
        {QStringLiteral("viewportRow"), viewportRows},
        {QStringLiteral("viewportRows"), viewportRows},
        {QStringLiteral("contentExtent"), 10000000},
        {QStringLiteral("contentExtentKnown"), true},
        {QStringLiteral("windowGeneration"), 42},
        {QStringLiteral("cursorAbsoluteRow"), 100000 + viewportRows + 4},
        {QStringLiteral("cursorVisualColumn"), 17},
        {QStringLiteral("cursorVisible"), true},
        {QStringLiteral("rows"), visible},
        {QStringLiteral("windowRows"), window},
    };
    return {
        {QStringLiteral("type"), QStringLiteral("scene")},
        {QStringLiteral("schema"), QStringLiteral("app")},
        {QStringLiteral("surface"), surface},
        {QStringLiteral("shell"), QVariantMap{
             {QStringLiteral("terminalActive"), false},
         }},
    };
}

double percentile(std::vector<double> values, double quantile)
{
    std::sort(values.begin(), values.end());
    const auto index = static_cast<size_t>(
        std::ceil(quantile * static_cast<double>(values.size())) - 1.0);
    return values[std::min(index, values.size() - 1)];
}
}

int main(int argc, char **argv)
{
    QCoreApplication application(argc, argv);
    const QVariantMap scene = representativeScene();
    msgpack::sbuffer payload;
    msgpack::packer<msgpack::sbuffer> packer(payload);
    packVariant(packer, scene);

    constexpr int warmups = 20;
    constexpr int iterations = 250;
    std::vector<double> unpackMs;
    std::vector<double> convertMs;
    std::vector<double> totalMs;
    unpackMs.reserve(iterations);
    convertMs.reserve(iterations);
    totalMs.reserve(iterations);
    quint64 checksum = 0;

    for (int iteration = -warmups; iteration < iterations; ++iteration) {
        QElapsedTimer timer;
        timer.start();
        msgpack::object_handle handle = msgpack::unpack(payload.data(), payload.size());
        const qint64 unpackNs = timer.nsecsElapsed();
        timer.restart();
        const QVariant decoded = unpackObject(handle.get());
        const qint64 convertNs = timer.nsecsElapsed();
        checksum += static_cast<quint64>(decoded.toMap().size());
        if (iteration >= 0) {
            unpackMs.push_back(static_cast<double>(unpackNs) / 1.0e6);
            convertMs.push_back(static_cast<double>(convertNs) / 1.0e6);
            totalMs.push_back(static_cast<double>(unpackNs + convertNs) / 1.0e6);
        }
    }

    std::cout << "MSGPACK_SCENE_DECODE_PROBE"
              << " payload_bytes=" << payload.size()
              << " rows_serialized=" << (105 + 315)
              << " runs_per_row=45"
              << " run_maps=" << ((105 + 315) * 45)
              << " unpack_p50_ms=" << percentile(unpackMs, 0.50)
              << " unpack_p95_ms=" << percentile(unpackMs, 0.95)
              << " qvariant_p50_ms=" << percentile(convertMs, 0.50)
              << " qvariant_p95_ms=" << percentile(convertMs, 0.95)
              << " total_p50_ms=" << percentile(totalMs, 0.50)
              << " total_p95_ms=" << percentile(totalMs, 0.95)
              << " checksum=" << checksum << '\n';
    return 0;
}
