#pragma once

#include <QCoreApplication>
#include <QDebug>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QMutex>
#include <QMutexLocker>
#include <QThread>
#include <QVariant>

#include <chrono>
#include <ctime>
#include <utility>

#if defined(Q_OS_WIN)
#include <qt_windows.h>
#endif

// Lightweight, opt-in events for measuring one native navigation across the
// Qt transport, controller, Gallery bridge and rendered-frame boundaries.
// Keep this header-only so each layer can use the same schema without adding
// another target/library dependency. No QVariant or JSON work happens unless
// F4_NAV_BENCHMARK_TRACE is present in the environment.
namespace F4NavigationBenchmarkTrace
{
inline bool enabled()
{
    // Benchmarking is a process-launch mode. Cache the gate so the normal UI
    // path pays one environment lookup total, not one lookup per IPC frame.
    static const bool traceEnabled =
        qEnvironmentVariableIsSet("F4_NAV_BENCHMARK_TRACE");
    return traceEnabled;
}

inline qint64 monotonicNanoseconds()
{
#if defined(Q_OS_WIN)
    LARGE_INTEGER counter{};
    static const qint64 frequency = []() {
        LARGE_INTEGER value{};
        return QueryPerformanceFrequency(&value) && value.QuadPart > 0
            ? static_cast<qint64>(value.QuadPart) : qint64(0);
    }();
    if (QueryPerformanceCounter(&counter)
        && frequency > 0) {
        // Converting the quotient and remainder separately avoids overflowing
        // a 64-bit counter when scaling long system uptimes to nanoseconds.
        const qint64 wholeSeconds = counter.QuadPart / frequency;
        const qint64 remainder = counter.QuadPart % frequency;
        return wholeSeconds * 1000000000LL
            + remainder * 1000000000LL / frequency;
    }
#endif
#if defined(Q_OS_DARWIN) || defined(Q_OS_LINUX)
    // The Go side uses clock_gettime(CLOCK_MONOTONIC_RAW), so using that same
    // kernel clock here makes timestamps directly comparable across the two
    // local processes without a wall-clock calibration handshake.
    timespec timestamp{};
    if (clock_gettime(CLOCK_MONOTONIC_RAW, &timestamp) == 0) {
        return static_cast<qint64>(timestamp.tv_sec) * 1000000000LL
            + static_cast<qint64>(timestamp.tv_nsec);
    }
#endif
    return std::chrono::duration_cast<std::chrono::nanoseconds>(
               std::chrono::steady_clock::now().time_since_epoch())
        .count();
}

inline QVariant normalizedTraceId(const QVariant &value)
{
    if (!value.isValid() || value.isNull()
        || value.toString().trimmed().isEmpty()) {
        return {};
    }
    return value;
}

inline QVariant traceIdFromPanels(const QVariant &value)
{
    if (value.metaType().id() != QMetaType::QVariantList) {
        return {};
    }

    QVariant fallback;
    const QVariantList panels = value.toList();
    for (const QVariant &panelValue : panels) {
        if (panelValue.metaType().id() != QMetaType::QVariantMap) {
            continue;
        }
        const QVariantMap panel = panelValue.toMap();
        const QVariant traceId = normalizedTraceId(
            panel.value(QStringLiteral("benchmarkTraceId")));
        if (!traceId.isValid()) {
            continue;
        }
        if (panel.value(QStringLiteral("active")).toBool()) {
            return traceId;
        }
        if (!fallback.isValid()) {
            fallback = traceId;
        }
    }
    return fallback;
}

inline QVariant traceIdFromContainer(const QVariantMap &container)
{
    if (const QVariant direct = normalizedTraceId(
            container.value(QStringLiteral("benchmarkTraceId")));
        direct.isValid()) {
        return direct;
    }

    for (const QString &metadataKey : {
             QStringLiteral("benchmark"), QStringLiteral("metadata")}) {
        const QVariant metadataValue = container.value(metadataKey);
        if (metadataValue.metaType().id() == QMetaType::QVariantMap) {
            if (const QVariant metadata = normalizedTraceId(
                    metadataValue.toMap().value(
                        QStringLiteral("benchmarkTraceId")));
                metadata.isValid()) {
                return metadata;
            }
        }
    }

    return traceIdFromPanels(container.value(QStringLiteral("panels")));
}

// Scenes are expected to carry benchmarkTraceId at their top level. The
// targeted fallbacks make the logger tolerant of panel-scoped metadata while
// deliberately avoiding a recursive walk through potentially huge entries.
inline QVariant benchmarkTraceId(const QVariantMap &message)
{
    if (const QVariant direct = traceIdFromContainer(message);
        direct.isValid()) {
        return direct;
    }

    const QVariant shellValue = message.value(QStringLiteral("shell"));
    if (shellValue.metaType().id() == QMetaType::QVariantMap) {
        if (const QVariant shell = traceIdFromContainer(shellValue.toMap());
            shell.isValid()) {
            return shell;
        }
    }

    const QVariant framesValue = message.value(QStringLiteral("frames"));
    if (framesValue.metaType().id() == QMetaType::QVariantList) {
        const QVariantList frames = framesValue.toList();
        for (const QVariant &frameValue : frames) {
            if (frameValue.metaType().id() != QMetaType::QVariantMap) {
                continue;
            }
            if (const QVariant frame = traceIdFromContainer(frameValue.toMap());
                frame.isValid()) {
                return frame;
            }
        }
    }
    return {};
}

inline void eventAt(const QString &name,
                    qint64 monotonicNs,
                    const QVariant &traceId = {},
                    QVariantMap fields = {})
{
    if (!enabled()) {
        return;
    }

    QJsonObject object = QJsonObject::fromVariantMap(fields);
    object.insert(QStringLiteral("event"), name);
    object.insert(QStringLiteral("monotonicNs"), monotonicNs);
    object.insert(QStringLiteral("pid"),
                  QCoreApplication::applicationPid());
    QString threadName = QThread::currentThread()->objectName();
    if (threadName.isEmpty()) {
        const QCoreApplication *application = QCoreApplication::instance();
        threadName = application && QThread::currentThread() ==
                application->thread()
            ? QStringLiteral("qt-main") : QStringLiteral("unnamed");
    }
    object.insert(QStringLiteral("thread"), threadName);
    if (const QVariant normalized = normalizedTraceId(traceId);
        normalized.isValid()) {
        // MessagePack represents this as uint64, but JSON numbers cannot
        // preserve every uint64 exactly. A string keeps correlation lossless.
        object.insert(QStringLiteral("benchmarkTraceId"),
                      normalized.toString());
    }

    const QByteArray json = QJsonDocument(object).toJson(
        QJsonDocument::Compact);
    // GUI-subsystem builds do not necessarily have a console sink for qInfo.
    // Keep file output opt-in and local to benchmark mode so live cross-
    // process traces can still include the Qt decode/apply/render boundaries.
    static const QString outputPath = qEnvironmentVariable(
        "F4_NAV_BENCHMARK_QT_OUTPUT");
    if (outputPath.isEmpty()) {
        qInfo().noquote() << "F4_NAV_BENCHMARK_TRACE" << json;
    }
    else {
        static QMutex outputMutex;
        // Opening and closing the trace file for every event added several
        // milliseconds of disk I/O to the very GUI-thread spans being
        // measured. Keep one append handle for the benchmark process. QFile
        // writes reach the OS immediately, and the process close releases the
        // handle after the final runner event.
        static QFile *output = []() -> QFile * {
            auto *file = new QFile(qEnvironmentVariable(
                "F4_NAV_BENCHMARK_QT_OUTPUT"));
            if (!file->open(QIODevice::WriteOnly | QIODevice::Append)) {
                delete file;
                return nullptr;
            }
            if (QCoreApplication *application =
                    QCoreApplication::instance()) {
                QObject::connect(
                    application, &QCoreApplication::aboutToQuit,
                    application, [file]() { file->flush(); },
                    Qt::DirectConnection);
            }
            return file;
        }();
        const QMutexLocker locker(&outputMutex);
        if (output) {
            output->write("F4_NAV_BENCHMARK_TRACE ");
            output->write(json);
            output->write("\n");
            if (name == QStringLiteral("qt.gallery.runner.finished")
                || name == QStringLiteral("qt.gallery.runner.failed")) {
                output->flush();
            }
        }
        else {
            qInfo().noquote() << "F4_NAV_BENCHMARK_TRACE" << json;
        }
    }
}

inline void event(const QString &name,
                  const QVariant &traceId = {},
                  QVariantMap fields = {})
{
    eventAt(name, monotonicNanoseconds(), traceId, std::move(fields));
}
}
