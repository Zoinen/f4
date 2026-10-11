#include "F4GalleryBridge.h"
#include "F4NativeDragVisuals.h"
#include <ZoinGallery/GallerySession.h>
#include <QAbstractItemModel>
#include <QQmlEngine>
#include <QQuickImageProvider>
#include <QIcon>

#include <QCoreApplication>
#include <QDir>
#include <QDrag>
#include <QDropEvent>
#include <QGuiApplication>
#include <QMimeData>
#include <QMouseEvent>
#include <QKeyEvent>
#include <QQuickItem>
#include <QQuickWindow>
#include <QStyleHints>
#include <QTimer>
#include <QPainter>
#include <QUuid>
#include <QCursor>
#include <functional>
#ifdef Q_OS_WIN
#include <qt_windows.h>
#endif

namespace {
const char *sessionMime = "application/x-f4-drag-session";

void populateDropRequest(QVariantMap &request, const QDropEvent &drop,
                         const QVariantMap &source, bool internal, bool allowMove)
{
    if (internal) {
        request.insert("source", source);
        if (allowMove && (drop.modifiers() & Qt::ShiftModifier)
            && !(drop.modifiers() & Qt::ControlModifier))
            request.insert("operation", "move");
    } else {
        QStringList paths;
        for (const auto &url : drop.mimeData()->urls()) paths.append(url.toLocalFile());
        request.insert("paths", paths);
    }
}

void acceptCommandLineDrop(F4GalleryBridge &bridge, QQuickItem &commandLine,
                           QDropEvent &drop, const QVariantMap &source, bool internal)
{
    commandLine.setProperty("dropHovered", drop.type() != QEvent::Drop);
    if (drop.type() == QEvent::Drop) {
        QVariantMap request{{"action", "commandLine.dropPaths"}};
        populateDropRequest(request, drop, source, internal, false);
        emit bridge.uiActionRequested(request);
        commandLine.forceActiveFocus();
    }
    drop.setDropAction(Qt::CopyAction);
    drop.accept();
}

void acceptPanelDrop(F4GalleryBridge &bridge, QQuickItem &panel, QDropEvent &drop,
                     QVariantMap target, const QVariantMap &source,
                     bool internal, Qt::DropAction action)
{
    if (drop.type() != QEvent::Drop) {
        panel.setProperty("dropHoverIndex", target.value("isDir").toBool()
            ? target.value("index").toInt() : -1);
    } else {
        target.insert("action", "panel.dropFiles");
        target.insert("operation", "copy");
        populateDropRequest(target, drop, source, internal, true);
        emit bridge.uiActionRequested(target);
    }
    drop.setDropAction(action);
    drop.accept();
}

struct PreparedDragGesture {
    bool &startupPending;
    int &armedSide;
    QVariantMap &source;
    QString &requestId;
    bool &prepared;
    bool &thresholdPassed;
    QList<QUrl> &urls;

    void cancel()
    {
        startupPending = false;
        armedSide = -1;
        source.clear();
        requestId.clear();
    }

    void reset()
    {
        cancel();
        prepared = false;
        thresholdPassed = false;
        urls.clear();
    }

    void release(QObject *window, QEvent &event,
                 const std::function<void(const QPointF &, Qt::KeyboardModifiers)> &finish)
    {
        if (startupPending && event.type() == QEvent::MouseButtonRelease) {
            const auto &mouse = static_cast<QMouseEvent &>(event);
            if (mouse.button() == Qt::LeftButton
                && QGuiApplication::topLevelAt(mouse.globalPosition().toPoint()) == window)
                finish(mouse.position(), mouse.modifiers());
        }
        cancel();
    }

    bool advance(const QMouseEvent &mouse, const QPointF &press,
                 const std::function<void()> &start)
    {
        if (!(mouse.buttons() & Qt::LeftButton)) { armedSide = -1; return false; }
        if ((mouse.position() - press).manhattanLength()
            < QGuiApplication::styleHints()->startDragDistance()) return false;
        thresholdPassed = true;
        start();
        return true;
    }
};

void observeNativeDragEvent(QEvent &event, bool &entered, bool &released, bool &cancelled)
{
    if (event.type() == QEvent::DragEnter) entered = true;
    if (event.type() == QEvent::MouseButtonRelease
        && static_cast<QMouseEvent &>(event).button() == Qt::LeftButton) released = true;
    if (event.type() == QEvent::KeyPress
        && static_cast<QKeyEvent &>(event).key() == Qt::Key_Escape) cancelled = true;
}

void updateWorkspaceDrop(F4GalleryBridge &bridge, QDropEvent &drop,
                         const QVariantMap &workspace, const QVariantMap &source,
                         bool internal, QString &hoveredWorkspace)
{
    const QString id = workspace.value("target").toString();
    if (drop.type() == QEvent::Drop) {
        auto request = workspace;
        request.insert("action", "workspace.dropFiles");
        request.insert("operation", "copy");
        populateDropRequest(request, drop, source, internal, true);
        emit bridge.uiActionRequested(request);
        hoveredWorkspace.clear();
    } else if (id != hoveredWorkspace) {
        hoveredWorkspace = id;
        if (!workspace.value("active").toBool())
            emit bridge.uiActionRequested({{"action", "workspace.dragActivate"}, {"target", id}});
    }
}

struct WorkspaceDropState {
    QString &hoveredWorkspace;
    bool &internal;
    QTimer *timer;
};

void acceptWorkspaceDrop(F4GalleryBridge &bridge, QDropEvent &drop,
                         const QVariantMap &workspace, const QVariantMap &source,
                         bool internal, Qt::DropAction action, WorkspaceDropState state,
                         const std::function<void()> &refreshHighlight)
{
    updateWorkspaceDrop(bridge, drop, workspace, source, internal, state.hoveredWorkspace);
    if (drop.type() != QEvent::Drop) {
        state.internal = internal;
        if (state.timer) state.timer->start();
        refreshHighlight();
    }
    drop.setDropAction(action);
    drop.accept();
}

bool ownsDragWindow(QObject *window, QQuickItem *workspaceBar, QQuickItem *commandLine,
                    const std::array<QPointer<QQuickItem>, 2> &panels)
{
    bool ownsWindow = workspaceBar && workspaceBar->window() == window;
    ownsWindow = ownsWindow || (commandLine && commandLine->window() == window);
    for (const auto &item : panels)
        ownsWindow = ownsWindow || (item && item->window() == window);
    return ownsWindow;
}

struct PreparedPanelDrag {
    QVariantMap source;
    QList<QUrl> urls;
    bool complete = false;
};

template <typename Catalog>
PreparedPanelDrag preparePanelDragSource(QVariantMap source, const Catalog &state,
                                         Qt::KeyboardModifiers modifiers)
{
    const auto id = source.value("entryId").toString();
    QStringList ids;
#ifdef Q_OS_MACOS
    const bool singleItem = modifiers.testFlag(Qt::MetaModifier);
#else
    const bool singleItem = modifiers.testFlag(Qt::AltModifier);
#endif
    if (!singleItem && state.selectedEntryIds.contains(id)) ids = state.selectedEntryIdList;
    else ids.append(id);
    source.insert("entryIds", ids);
    PreparedPanelDrag result{source, {}, false};
    // Complete local selections need no round trip; Go resolves off-page entries.
    if (state.sourceKind == "local") {
        for (const auto &entryId : ids) {
            for (const auto &value : state.entries) {
                const auto row = value.toMap();
                if (row.value("entryId").toString() != entryId) continue;
                result.urls.append(QUrl::fromLocalFile(
                    QDir(state.currentPath).filePath(row.value("name").toString())));
                break;
            }
        }
        result.complete = result.urls.size() == ids.size();
    }
    return result;
}

#ifdef Q_OS_WIN
class WindowsDragFeedback {
public:
    WindowsDragFeedback(QDrag &drag, QQuickWindow &window,
                        const std::function<bool()> &internalTarget)
    {
        const auto dpr = window.devicePixelRatio();
        const auto previewSize = drag.pixmap().deviceIndependentSize();
        const auto copyCursor = F4NativeDragVisuals::windowsDragCursorPixmap(
            dpr, previewSize, drag.hotSpot(), Qt::CopyAction);
        const auto moveCursor = F4NativeDragVisuals::windowsDragCursorPixmap(
            dpr, previewSize, drag.hotSpot(), Qt::MoveAction);
        drag.setDragCursor(copyCursor, Qt::CopyAction);
        drag.setDragCursor(moveCursor, Qt::MoveAction);
        drag.setDragCursor(F4NativeDragVisuals::windowsDragCursorPixmap(
            dpr, previewSize, drag.hotSpot(), Qt::IgnoreAction), Qt::IgnoreAction);
        // Internal moves remain Go-owned; OLE only advertises Copy.
        auto updateFeedback = [&drag, internalTarget, copyCursor, moveCursor] {
            const auto mods = QGuiApplication::queryKeyboardModifiers();
            const bool move = internalTarget() && (mods & Qt::ShiftModifier)
                && !(mods & Qt::ControlModifier);
            drag.setDragCursor(move ? moveCursor : copyCursor, Qt::CopyAction);
        };
        m_timer.setInterval(16);
        QObject::connect(&m_timer, &QTimer::timeout, &drag, updateFeedback);
        updateFeedback();
        m_timer.start();
    }
private:
    QTimer m_timer;
};

struct NativeDragObservation {
    bool &entered;
    bool &released;
    bool &cancelled;
};

Qt::DropAction executeWindowsDrag(QDrag &drag, QQuickWindow &window,
                                  NativeDragObservation observation,
                                  const std::function<bool()> &finishInternalDrop)
{
    Qt::DropAction finished = Qt::IgnoreAction;
    bool missedRelease = false;
    int releasedPolls = 0;
    QTimer releaseWatchdog;
    releaseWatchdog.setInterval(75);
    QObject::connect(&releaseWatchdog, &QTimer::timeout, &drag, [&] {
        if (GetAsyncKeyState(VK_LBUTTON) & 0x8000) {
            releasedPolls = 0;
        } else if (++releasedPolls >= 2) {
            // Normal OLE completion returns before this grace period.
            missedRelease = true;
            releaseWatchdog.stop();
            QDrag::cancel();
        }
    });
    auto finishReleasedInternalDrop = [&] {
        if (observation.cancelled || (GetAsyncKeyState(VK_ESCAPE) & 0x8000)
            || QGuiApplication::topLevelAt(QCursor::pos()) != &window) {
            if (qEnvironmentVariableIsSet("VTUI_DEBUG"))
                qInfo() << "QT_DND: internal finish wrong window/cancel" << observation.cancelled
                        << QGuiApplication::topLevelAt(QCursor::pos()) << &window;
            return;
        }
        if (finishInternalDrop()) finished = Qt::CopyAction;
    };
    // A release before OLE's first poll cannot start a desktop transaction.
    if (!(GetAsyncKeyState(VK_LBUTTON) & 0x8000)) {
        finishReleasedInternalDrop();
    } else {
        releaseWatchdog.start();
        finished = drag.exec(Qt::CopyAction, Qt::CopyAction);
        releaseWatchdog.stop();
        if (!(GetAsyncKeyState(VK_LBUTTON) & 0x8000)) observation.released = true;
        // Recover an observed startup release, never Escape or rejected OLE entry.
        const bool startupReleased = observation.released
            && !observation.entered && !observation.cancelled;
        if ((missedRelease || startupReleased) && finished == Qt::IgnoreAction)
            finishReleasedInternalDrop();
    }
    return finished;
}
#endif
}

void F4GalleryBridge::registerDragPanel(int side, QQuickItem *item)
{
    if (!validSide(side)) return;
    m_dragPanels[side] = item;
    qApp->installEventFilter(this);
}

void F4GalleryBridge::registerDragWorkspaceBar(QQuickItem *item)
{
    m_dragWorkspaceBar = item;
    if (!m_workspaceDropTimer) {
        m_workspaceDropTimer = new QTimer(this);
        m_workspaceDropTimer->setInterval(16);
        connect(m_workspaceDropTimer, &QTimer::timeout, this, &F4GalleryBridge::refreshWorkspaceDropHighlight);
    }
    qApp->installEventFilter(this);
}

void F4GalleryBridge::registerDragCommandLine(QQuickItem *item)
{
    m_dragCommandLine = item;
    qApp->installEventFilter(this);
}

bool F4GalleryBridge::dragCommandLineHit(QObject *window, const QPointF &position) const
{
    const auto *item = m_dragCommandLine.data();
    return item && item->window() == window && item->isVisible() && item->isEnabled()
        && item->property("dropInputEnabled").toBool() && !QGuiApplication::modalWindow()
        && item->contains(item->mapFromScene(position));
}

QVariantMap F4GalleryBridge::dragWorkspaceHit(QObject *window, const QPointF &position) const
{
    auto *bar = m_dragWorkspaceBar.data();
    if (!bar || bar->window() != window || !bar->isVisible() || !bar->isEnabled()
        || QGuiApplication::modalWindow()) return {};
    const auto point = bar->mapFromScene(position);
    if (!bar->contains(point)) return {};
    QVariant hit;
    if (!QMetaObject::invokeMethod(bar, "dragWorkspaceHit", Q_RETURN_ARG(QVariant, hit),
            Q_ARG(QVariant, point.x()), Q_ARG(QVariant, point.y()))) return {};
    return hit.toMap();
}

QVariantMap F4GalleryBridge::dragEndpoint(int side, int sourceIndex) const
{
    if (!validSide(side)) return {};
    const auto &state = m_panelSessions.catalog(side);
    if (!state.initialized || state.loading) return {};
    QVariantMap result{{"side", side}, {"panelId", state.panelId},
        {"path", state.currentPath}, {"sourceKind", state.sourceKind}, {"catalogRevision", state.catalogRevision}};
    if (sourceIndex >= 0) {
        // sourceIndex is the Go index returned by GalleryPanelController.
        for (const auto &value : state.entries) {
            const auto row = value.toMap();
            if (row.value("index").toInt() != sourceIndex) continue;
            result.insert("entryId", row.value("entryId"));
            result.insert("index", sourceIndex);
            result.insert("name", row.value("name"));
            result.insert("isDir", row.value("isDir"));
            return result;
        }
        return {}; // A not-yet-loaded page is not an empty-area drop.
    }
    return result;
}

QVariantMap F4GalleryBridge::dragHit(QObject *window, const QPointF &pos, int *side) const
{
    *side = -1;
    if (QGuiApplication::modalWindow()) return {};
    for (int i = 0; i < 2; ++i) {
        auto *item = m_dragPanels[i].data();
        if (!item || item->window() != window || !item->isVisible() || !item->isEnabled()
            || !item->property("dropInputEnabled").toBool()) continue;
        const auto point = item->mapFromScene(pos);
        // QML tests the same full-panel bounds used by the outline. The
        // content host itself excludes the gutters, so contains() is too narrow.
        QVariant hit;
        if (!QMetaObject::invokeMethod(item, "dragHit", Q_RETURN_ARG(QVariant, hit),
                Q_ARG(QVariant, point.x()), Q_ARG(QVariant, point.y()))) continue;
        const auto data = hit.toMap();
        if (!data.value("valid").toBool()) continue;
        auto endpoint = dragEndpoint(i, data.value("index", -1).toInt());
        if (endpoint.isEmpty()) continue;
        *side = i;
        return endpoint;
    }
    return {};
}

Qt::DropAction F4GalleryBridge::acceptNativeDrop(const QMimeData *mime,
        Qt::DropActions allowed, Qt::KeyboardModifiers mods) const
{
    if (!mime) return Qt::IgnoreAction;
    const bool internal = !m_dragToken.isEmpty()
        && mime->data(sessionMime) == m_dragToken.toUtf8();
    if (internal && (mods & Qt::ShiftModifier) && !(mods & Qt::ControlModifier)
        && allowed.testFlag(Qt::MoveAction)) return Qt::MoveAction;
    if (!allowed.testFlag(Qt::CopyAction)) return Qt::IgnoreAction;
    if (internal) return Qt::CopyAction;
    if (!mime->hasUrls() || mime->urls().isEmpty()) return Qt::IgnoreAction;
    for (const auto &url : mime->urls()) {
        if (!url.isLocalFile() || !QDir::isAbsolutePath(url.toLocalFile()))
            return Qt::IgnoreAction;
    }
    return Qt::CopyAction;
}

bool F4GalleryBridge::dropTargetAllowed(const QVariantMap &target, bool internal) const
{
    const int side=target.value("side",-1).toInt();
    if (target.isEmpty() || !validSide(side) || !m_panelSessions.catalog(side).dropAllowed) return false;
    if (!internal) return true;
    const bool samePanel=target.value("panelId")==m_dragSource.value("panelId");
    const bool folder=target.value("isDir").toBool();
    if (samePanel && folder && m_dragSource.value("entryIds").toStringList().contains(target.value("entryId").toString())) return false;
    QString destination=target.value("path").toString();
    if (folder) destination=QDir(destination).filePath(target.value("name").toString());
    const bool local=m_dragSource.value("sourceKind").toString()=="local"
        && target.value("sourceKind").toString()=="local";
    if (samePanel || local) {
#ifdef Q_OS_WIN
        constexpr auto sensitivity=Qt::CaseInsensitive;
#else
        constexpr auto sensitivity=Qt::CaseSensitive;
#endif
        if (QDir::cleanPath(destination).compare(QDir::cleanPath(m_dragSource.value("path").toString()),sensitivity)==0) return false;
    }
    return true;
}

void F4GalleryBridge::refreshWorkspaceDropHighlight()
{
    if (m_dragHoveredWorkspace.isEmpty() || !m_dragWorkspaceBar || !m_dragWorkspaceBar->window()) {
        if (m_workspaceDropTimer) m_workspaceDropTimer->stop();
        return;
    }
    clearDropHighlight();
    auto *window=m_dragWorkspaceBar->window();
    const auto hit=dragWorkspaceHit(window,window->mapFromGlobal(QCursor::pos()));
    if (hit.value("target").toString()!=m_dragHoveredWorkspace) return;
    // Wait for the activated tab's scene; never outline the old workspace.
    if (!hit.value("active").toBool()) return;
    for (int side=0;side<2;++side) {
        auto *panel=m_dragPanels[side].data();
        if (!panel || !panel->isVisible() || !panel->property("dropInputEnabled").toBool()
            || !m_panelSessions.catalog(side).active) continue;
        if (!dropTargetAllowed(dragEndpoint(side),m_workspaceDropInternal)) continue;
        panel->setProperty("dropTabHover",true);
        panel->setProperty("dropHoverIndex",-1);
        break;
    }
}

void F4GalleryBridge::clearDropHighlight()
{
    if (m_dragCommandLine) m_dragCommandLine->setProperty("dropHovered", false);
    for (auto &item : m_dragPanels) if (item) { item->setProperty("dropHoverIndex", -2); item->setProperty("dropTabHover",false); }
}

bool F4GalleryBridge::finishInternalDrop(QObject *window, const QPointF &position, Qt::KeyboardModifiers modifiers)
{
    if (m_nativeDragCancelled || m_dragSource.isEmpty()) {
        if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish cancelled/empty" << m_nativeDragCancelled << m_dragSource.isEmpty();
        return false;
    }
    const auto source = dragEndpoint(m_dragSource.value("side", -1).toInt());
    // After switching workspaces the visible side belongs to a different panel.
    // Go revalidates the captured source against its owning workspace.
    if (source.value("panelId") == m_dragSource.value("panelId"))
    for (const auto *key : {"panelId", "path", "catalogRevision"})
        if (source.value(key) != m_dragSource.value(key)) {
            if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish stale" << key << source.value(key) << m_dragSource.value(key);
            return false;
        }
    int targetSide = -1;
    if (dragCommandLineHit(window, position)) {
        emit uiActionRequested({{"action", "commandLine.dropPaths"}, {"source", m_dragSource}});
        return true;
    }
    auto target = dragWorkspaceHit(window, position);
    const bool workspace = !target.isEmpty();
    if (!workspace) target = dragHit(window, position, &targetSide);
    if (target.isEmpty() || (!workspace && !dropTargetAllowed(target,true))) {
        if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish invalid target" << position << targetSide << QGuiApplication::modalWindow();
        return false;
    }
    target.insert("action", workspace ? "workspace.dropFiles" : "panel.dropFiles");
    target.insert("source", m_dragSource);
    target.insert("operation", (modifiers & Qt::ShiftModifier)
        && !(modifiers & Qt::ControlModifier) ? "move" : "copy");
    emit uiActionRequested(target);
    return true;
}

bool F4GalleryBridge::eventFilter(QObject *object, QEvent *event)
{
    if (!qobject_cast<QQuickWindow *>(object)) return QObject::eventFilter(object, event);
    if (!ownsDragWindow(object, m_dragWorkspaceBar, m_dragCommandLine, m_dragPanels))
        return QObject::eventFilter(object, event);
    if (m_nativeDragActive)
        observeNativeDragEvent(*event, m_nativeDragEntered, m_nativeDragReleased, m_nativeDragCancelled);
    if (event->type() == QEvent::DragLeave) { clearDropHighlight(); m_dragHoveredWorkspace.clear(); return false; }
    if (event->type() == QEvent::DragEnter || event->type() == QEvent::DragMove
        || event->type() == QEvent::Drop) {
        auto *drop = static_cast<QDropEvent *>(event);
        const bool internal=!m_dragToken.isEmpty() && drop->mimeData()->data(sessionMime)==m_dragToken.toUtf8();
        if (dragCommandLineHit(object, drop->position())) {
            clearDropHighlight();
            m_dragHoveredWorkspace.clear();
            const auto accepted = acceptNativeDrop(drop->mimeData(), drop->possibleActions(), Qt::NoModifier);
            if (accepted == Qt::IgnoreAction) { drop->ignore(); return true; }
            acceptCommandLineDrop(*this, *m_dragCommandLine, *drop, m_dragSource, internal);
            return true;
        }
        const auto workspace = dragWorkspaceHit(object, drop->position());
        const auto action = acceptNativeDrop(drop->mimeData(), drop->possibleActions(), drop->modifiers());
        if (!workspace.isEmpty() && action != Qt::IgnoreAction) {
            clearDropHighlight();
            acceptWorkspaceDrop(*this, *drop, workspace, m_dragSource, internal, action,
                {m_dragHoveredWorkspace, m_workspaceDropInternal, m_workspaceDropTimer},
                [this] { refreshWorkspaceDropHighlight(); });
            return true;
        }
        m_dragHoveredWorkspace.clear();
        int side;
        auto target = dragHit(object, drop->position(), &side);
        if (qEnvironmentVariableIsSet("VTUI_DEBUG") && event->type() != QEvent::DragMove)
            qInfo() << "QT_DND: native event" << event->type() << drop->position()
                    << "target" << side << "action" << action << "identity" << target.value("panelId");
        clearDropHighlight();
        if (target.isEmpty() || action == Qt::IgnoreAction
            || !dropTargetAllowed(target,internal)) { drop->ignore(); return true; }
        acceptPanelDrop(*this, *m_dragPanels[side], *drop, target, m_dragSource, internal, action);
        return true;
    }
    if (m_nativeDragActive) return false;
    PreparedDragGesture gesture{m_nativeDragStartupPending, m_dragArmedSide,
        m_dragSource, m_dragRequestId, m_dragPrepared, m_dragThresholdPassed, m_preparedDragUrls};
    if (m_nativeDragStartupPending && event->type() == QEvent::KeyPress
        && static_cast<QKeyEvent *>(event)->key() == Qt::Key_Escape) {
        gesture.cancel();
        return true;
    }
    if (event->type() == QEvent::MouseButtonRelease || event->type() == QEvent::WindowDeactivate) {
        gesture.release(object, *event, [this, object](const QPointF &position, Qt::KeyboardModifiers mods) {
            finishInternalDrop(object, position, mods);
        });
    }
    if (event->type() == QEvent::MouseButtonPress) {
        auto *mouse = static_cast<QMouseEvent *>(event);
        gesture.reset();
        if (mouse->button() != Qt::LeftButton) return false;
        int side;
        auto source = dragHit(object, mouse->position(), &side);
        const auto id = source.value("entryId").toString();
        if (id.isEmpty() || source.value("name").toString() == "..") return false;
        const auto prepared = preparePanelDragSource(
            source, m_panelSessions.catalog(side), mouse->modifiers());
        source = prepared.source;
        m_dragSource = source;
        m_dragArmedSide = side;
        m_dragPress = mouse->position();
        m_preparedDragUrls = prepared.urls;
        m_dragPrepared = prepared.complete;
        m_dragRequestId = QUuid::createUuid().toString(QUuid::WithoutBraces);
        prepareDragPreview(m_dragPanels[side]);
        auto request = source;
        request.insert("action", "panel.prepareDrag");
        request.insert("requestId", m_dragRequestId);
        emit uiActionRequested(request);
    }
    if (event->type() == QEvent::MouseMove && m_dragArmedSide >= 0) {
        return gesture.advance(*static_cast<QMouseEvent *>(event), m_dragPress,
                               [this] { startPreparedDrag(); });
    }
    return QObject::eventFilter(object, event);
}

void F4GalleryBridge::handleDragPrepared(const QVariantMap &message)
{
    if (message.value("type") != "drag_prepared" || m_dragRequestId.isEmpty()
        || message.value("requestId") != m_dragRequestId || m_dragArmedSide < 0) return;
    if (!message.value("ok").toBool()) { m_dragArmedSide = -1; return; }
    m_preparedDragUrls.clear();
    for (const auto &value : message.value("paths").toList())
        m_preparedDragUrls.append(QUrl::fromLocalFile(value.toString()));
    m_dragPrepared = true;
    QTimer::singleShot(0, this, &F4GalleryBridge::startPreparedDrag);
}

void F4GalleryBridge::prepareDragPreview(QQuickItem *host)
{
    m_dragPreviewPixmap = {};
    m_dragPreviewHotSpot = QPoint(18,18);
    if (!host || !host->window()) return;
    const int side = m_dragSource.value("side").toInt();
    const auto ids = m_dragSource.value("entryIds").toStringList();
    const qreal dpr = host->window()->devicePixelRatio();
    QList<QImage> images;
    auto *session = qobject_cast<ZoinGallery::GallerySession *>(sessionForSide(side));
    auto *model = session ? session->model() : nullptr;
    auto *engine = qmlEngine(host);
    int visualRole = -1;
    if (model) {
        const auto roles = model->roleNames();
        for (auto it=roles.begin(); it!=roles.end(); ++it)
            if (it.value()=="visualSnapshot") visualRole=it.key();
    }

    for (const auto &id : ids.mid(0,5)) {
        QVariantMap visual;
        if (model && visualRole>=0) {
            const int row=session->indexForEntryId(id);
            if (row>=0) visual=model->data(model->index(row,0),visualRole).toMap();
        }
        QImage image;
        const QUrl url(visual.value("imageIdUrl").toString());
        if (engine && url.scheme()=="image") {
            auto *provider=dynamic_cast<QQuickImageProvider *>(engine->imageProvider(url.host()));
            if (provider && provider->imageType()==QQmlImageProviderBase::Image) {
                QSize size;
                image=provider->requestImage(url.path().mid(1),&size,QSize(qCeil(40*dpr),qCeil(40*dpr)));
                // Keep the complete provider image; compactPreview fits it
                // inside the tile while preserving its source aspect ratio.
            }
        }
        if (image.isNull()) {
            image=F4NativeDragVisuals::fileIcon(engine,visual,dpr,
                QGuiApplication::styleHints()->colorScheme()==Qt::ColorScheme::Dark);
        }
        if (image.isNull()) {
            QIcon icon;
            if (icon.isNull()) icon=QIcon(visual.value("isFolder").toBool()
                ? ":/F4QtHost/icons/lucide-gallery/folder.svg" : ":/F4QtHost/icons/lucide-gallery/file.svg");
            image=icon.pixmap(QSize(qCeil(40*dpr),qCeil(40*dpr))).toImage();
        }
        images.append(image);
    }
    m_dragPreviewPixmap=F4NativeDragVisuals::compactPreview(images,ids.size(),dpr,
        QGuiApplication::styleHints()->colorScheme()==Qt::ColorScheme::Dark);

}

void F4GalleryBridge::startPreparedDrag()
{
        if (!m_dragPrepared || !m_dragThresholdPassed || m_dragArmedSide < 0
            || m_nativeDragActive || !(QGuiApplication::mouseButtons() & Qt::LeftButton)) return;
        const int side = m_dragArmedSide;
        m_dragArmedSide = -1;
        auto *host = m_dragPanels[side].data();
        if (!host || !host->window() || !host->property("dropInputEnabled").toBool()) return;
        const auto current = dragEndpoint(side);
        if (current.value("catalogRevision") != m_dragSource.value("catalogRevision")
            || current.value("panelId") != m_dragSource.value("panelId")
            || current.value("path") != m_dragSource.value("path")) return;
        auto *mime = new QMimeData;
        m_dragToken = QUuid::createUuid().toString(QUuid::WithoutBraces);
        mime->setData(sessionMime, m_dragToken.toUtf8());
        const auto ids = m_dragSource.value("entryIds").toStringList();
        if (m_preparedDragUrls.size() == ids.size()) mime->setUrls(m_preparedDragUrls);
        if (m_dragWorkspaceBar)
            QMetaObject::invokeMethod(m_dragWorkspaceBar, "beginWorkspaceDrag");
        m_nativeDragActive = true;
        m_nativeDragEntered = false;
        m_nativeDragReleased = false;
        m_nativeDragCancelled = false;
        m_nativeDragStartupPending = false;
        if (qEnvironmentVariableIsSet("VTUI_DEBUG"))
            qInfo() << "QT_DND: native drag starting, files" << ids.size();
        auto *window = host->window();
        QMetaObject::invokeMethod(m_dragPanels[side], "endNativeDragPointer");
        QDrag drag(window);
        drag.setMimeData(mime);
        drag.setPixmap(F4NativeDragVisuals::withFileName(m_dragPreviewPixmap,
            m_dragSource.value("name").toString(), ids.size(),
            QGuiApplication::styleHints()->colorScheme()==Qt::ColorScheme::Dark));
        drag.setHotSpot(m_dragPreviewHotSpot);
#ifdef Q_OS_WIN
        WindowsDragFeedback actionFeedback(drag, *window, [&] {
            if (QGuiApplication::topLevelAt(QCursor::pos()) != window) return false;
            const QPointF point = window->mapFromGlobal(QCursor::pos());
            if (!dragWorkspaceHit(window, point).isEmpty()) return true;
            int targetSide = -1;
            return dropTargetAllowed(dragHit(window, point, &targetSide), true);
        });
        const auto finished = executeWindowsDrag(drag, *window,
            {m_nativeDragEntered, m_nativeDragReleased, m_nativeDragCancelled}, [&] {
                return finishInternalDrop(window, window->mapFromGlobal(QCursor::pos()),
                                          QGuiApplication::queryKeyboardModifiers());
            });
#else
        // Native receivers may only copy; Go owns internal Shift-move.
        const auto finished = drag.exec(Qt::CopyAction, Qt::CopyAction);
#endif
        if (auto *grabber = window->mouseGrabberItem()) grabber->ungrabMouse();
        if (qEnvironmentVariableIsSet("VTUI_DEBUG"))
            qInfo() << "QT_DND: native drag finished" << finished << "cursor" << window->mapFromGlobal(QCursor::pos())
                    << "entered/released/cancelled" << m_nativeDragEntered << m_nativeDragReleased << m_nativeDragCancelled;
        if (m_dragWorkspaceBar)
            m_dragWorkspaceBar->setProperty("dragSourceWorkspace", QString());
        m_nativeDragActive = false;
        m_dragHoveredWorkspace.clear();
        m_dragToken.clear();
        clearDropHighlight();
#ifdef Q_OS_WIN
        // A stale no-button WM_MOUSEMOVE can also fail Qt startup while the
        // actual button is still held. Keep this gesture for the next move or
        // its release, instead of discarding an otherwise valid internal drop.
        if (finished == Qt::IgnoreAction && !m_nativeDragEntered && !m_nativeDragCancelled
            && (GetAsyncKeyState(VK_LBUTTON) & 0x8000) && !(GetAsyncKeyState(VK_ESCAPE) & 0x8000)) {
            m_nativeDragStartupPending = true;
            m_dragArmedSide = side;
            return;
        }
#endif
        m_dragSource.clear();
}
