#include "MacScrollDiagnostics.h"

#import <AppKit/AppKit.h>

#include <QCoreApplication>
#include <QDateTime>
#include <QElapsedTimer>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QMutex>
#include <QMutexLocker>
#include <QQuickItem>
#include <QQuickWindow>
#include <QScreen>
#include <QTimer>
#include <QWheelEvent>
#include <atomic>

struct MacScrollDiagnostics::State {
    QFile file;
    QQuickWindow *window = nullptr;
    id monitor = nil;
    QElapsedTimer clock;
    QTimer heartbeat;
    QMutex mutex;
    QList<QJsonObject> pending;
    std::atomic<qint64> activeUntil{0};
    QMetaObject::Connection frames;
    int total = 0;
    int dropped = 0;

    void record(QJsonObject row) {
        row.insert("tMs", clock.nsecsElapsed() / 1000000.0);
        QMutexLocker lock(&mutex);
        if (pending.size() >= 20000 || total >= 200000) {
            ++dropped;
            return;
        }
        ++total;
        pending.append(std::move(row));
    }
};

MacScrollDiagnostics *MacScrollDiagnostics::start(QQuickWindow *window, const QString &path)
{
    if (!window || path.isEmpty())
        return nullptr;
    auto *recorder = new MacScrollDiagnostics(window, path);
    if (!recorder->state->file.isOpen()) {
        delete recorder;
        return nullptr;
    }
    return recorder;
}

MacScrollDiagnostics::MacScrollDiagnostics(QQuickWindow *window, const QString &path)
    : QObject(window), state(std::make_unique<State>())
{
    auto &s = *state;
    s.window = window;
    s.clock.start();
    s.file.setFileName(path);
    if (!s.file.open(QIODevice::WriteOnly | QIODevice::Append)) {
        qWarning() << "Cannot open scroll trace:" << s.file.errorString();
        return;
    }
    s.record({{"kind", "session"}, {"utc", QDateTime::currentDateTimeUtc().toString(Qt::ISODateWithMs)},
              {"qt", qVersion()}, {"refreshRate", window->screen()->refreshRate()},
              {"dpr", window->devicePixelRatio()}});
    flush();
    s.monitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskScrollWheel
        handler:^NSEvent *(NSEvent *event) {
            beginGesture();
            bool lowPower = false;
            if (@available(macOS 12.0, *))
                lowPower = NSProcessInfo.processInfo.lowPowerModeEnabled;
            state->record({{"kind", "appkit"}, {"nativeSeconds", event.timestamp},
                {"dx", event.scrollingDeltaX}, {"dy", event.scrollingDeltaY},
                {"precise", bool(event.hasPreciseScrollingDeltas)},
                {"phase", int(event.phase)}, {"momentumPhase", int(event.momentumPhase)},
                {"inverted", bool(event.isDirectionInvertedFromDevice)}, {"lowPower", lowPower}});
            return event;
        }];
    qApp->installEventFilter(this);
    s.frames = connect(window, &QQuickWindow::frameSwapped, this, [this] {
        if (state->clock.elapsed() < state->activeUntil.load())
            state->record({{"kind", "frameSwapped"}});
    }, Qt::DirectConnection);
    s.heartbeat.setInterval(16);
    s.heartbeat.setTimerType(Qt::PreciseTimer);
    connect(&s.heartbeat, &QTimer::timeout, this, [this] {
        state->record({{"kind", "uiHeartbeat"}});
        if (state->clock.elapsed() >= state->activeUntil.load()) {
            state->heartbeat.stop();
            flush();
        }
    });
    connect(qApp, &QCoreApplication::aboutToQuit, this, &MacScrollDiagnostics::flush);
}

MacScrollDiagnostics::~MacScrollDiagnostics()
{
    if (state->monitor)
        [NSEvent removeMonitor:state->monitor];
    qApp->removeEventFilter(this);
    disconnect(state->frames);
    flush();
}

void MacScrollDiagnostics::beginGesture()
{
    const bool starting = !state->heartbeat.isActive();
    state->activeUntil.store(state->clock.elapsed() + 2000);
    if (!starting)
        return;
    // Inspect the visual tree only once per gesture, not on every wheel/frame.
    const auto visit = [this](auto &&self, QQuickItem *item) -> void {
        if (item->objectName() == "galleryViewportItem") {
            connect(item, SIGNAL(contentYChanged()), this, SLOT(recordPosition()), Qt::UniqueConnection);
            state->record({{"kind", "viewportStart"}, {"viewport", double(quintptr(item))},
                           {"y", item->property("contentY").toDouble()},
                           {"height", item->height()}, {"contentHeight", item->property("contentHeight").toDouble()}});
        }
        for (auto *child : item->childItems())
            self(self, child);
    };
    visit(visit, state->window->contentItem());
    state->heartbeat.start();
}

bool MacScrollDiagnostics::eventFilter(QObject *receiver, QEvent *event)
{
    if (event->type() != QEvent::Wheel)
        return false;
    const auto *wheel = static_cast<QWheelEvent *>(event);
    beginGesture();
    state->record({{"kind", "qtWheel"}, {"receiverType", receiver->metaObject()->className()},
        {"windowReceiver", receiver == state->window},
        {"wheelArea", receiver->objectName() == "galleryWheelArea"},
        {"timestamp", double(wheel->timestamp())},
        {"px", wheel->pixelDelta().x()}, {"py", wheel->pixelDelta().y()},
        {"ax", wheel->angleDelta().x()}, {"ay", wheel->angleDelta().y()},
        {"phase", int(wheel->phase())}, {"source", int(wheel->source())},
        {"inverted", wheel->inverted()}});
    return false;
}

void MacScrollDiagnostics::recordPosition()
{
    if (state->clock.elapsed() < state->activeUntil.load())
        state->record({{"kind", "position"}, {"viewport", double(quintptr(sender()))},
                       {"y", sender()->property("contentY").toDouble()}});
}

void MacScrollDiagnostics::flush()
{
    if (!state->file.isOpen())
        return;
    QList<QJsonObject> rows;
    {
        QMutexLocker lock(&state->mutex);
        rows.swap(state->pending);
        if (state->dropped) {
            rows.append({{"kind", "dropped"}, {"count", state->dropped}});
            state->dropped = 0;
        }
    }
    QByteArray batch;
    for (const auto &row : rows)
        batch += QJsonDocument(row).toJson(QJsonDocument::Compact) + '\n';
    if (state->file.write(batch) != batch.size() || !state->file.flush())
        qWarning() << "Cannot write scroll trace:" << state->file.errorString();
}
