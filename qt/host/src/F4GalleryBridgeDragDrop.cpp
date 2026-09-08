#include "F4GalleryBridge.h"

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
#ifdef Q_OS_WIN
#include <qt_windows.h>
#endif

namespace {
const char *sessionMime = "application/x-f4-drag-session";
}

void F4GalleryBridge::registerDragPanel(int side, QQuickItem *item)
{
    if (!validSide(side)) return;
    m_dragPanels[side] = item;
    qApp->installEventFilter(this);
}

QVariantMap F4GalleryBridge::dragEndpoint(int side, int sourceIndex) const
{
    if (!validSide(side)) return {};
    const auto &state = m_panelSessions.catalog(side);
    if (!state.initialized || state.loading) return {};
    QVariantMap result{{"side", side}, {"panelId", state.panelId},
        {"path", state.currentPath}, {"catalogRevision", state.catalogRevision}};
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
        if (!item->contains(point)) continue;
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

void F4GalleryBridge::clearDropHighlight()
{
    for (auto &item : m_dragPanels) if (item) item->setProperty("dropHoverIndex", -2);
}

bool F4GalleryBridge::finishInternalDrop(QObject *window, const QPointF &position, Qt::KeyboardModifiers modifiers)
{
    if (m_nativeDragCancelled || m_dragSource.isEmpty()) {
        if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish cancelled/empty" << m_nativeDragCancelled << m_dragSource.isEmpty();
        return false;
    }
    const auto source = dragEndpoint(m_dragSource.value("side", -1).toInt());
    for (const auto *key : {"panelId", "path", "catalogRevision"})
        if (source.value(key) != m_dragSource.value(key)) {
            if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish stale" << key << source.value(key) << m_dragSource.value(key);
            return false;
        }
    int targetSide;
    auto target = dragHit(window, position, &targetSide);
    if (target.isEmpty() || !m_panelSessions.catalog(targetSide).dropAllowed) {
        if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish invalid target" << position << targetSide << QGuiApplication::modalWindow();
        return false;
    }
    target.insert("action", "panel.dropFiles");
    target.insert("source", m_dragSource);
    target.insert("operation", (modifiers & Qt::ShiftModifier)
        && !(modifiers & Qt::ControlModifier) ? "move" : "copy");
    emit uiActionRequested(target);
    return true;
}

bool F4GalleryBridge::eventFilter(QObject *object, QEvent *event)
{
    if (!qobject_cast<QQuickWindow *>(object)) return QObject::eventFilter(object, event);
    bool ownsWindow = false;
    for (const auto &item : m_dragPanels)
        ownsWindow = ownsWindow || (item && item->window() == object);
    if (!ownsWindow) return QObject::eventFilter(object, event);
    if (m_nativeDragActive) {
        if (event->type() == QEvent::DragEnter) m_nativeDragEntered = true;
        if (event->type() == QEvent::MouseButtonRelease
            && static_cast<QMouseEvent *>(event)->button() == Qt::LeftButton)
            m_nativeDragReleased = true;
        if (event->type() == QEvent::KeyPress
            && static_cast<QKeyEvent *>(event)->key() == Qt::Key_Escape)
            m_nativeDragCancelled = true;
    }
    if (event->type() == QEvent::DragLeave) { clearDropHighlight(); return false; }
    if (event->type() == QEvent::DragEnter || event->type() == QEvent::DragMove
        || event->type() == QEvent::Drop) {
        auto *drop = static_cast<QDropEvent *>(event);
        int side;
        auto target = dragHit(object, drop->position(), &side);
        const auto action = acceptNativeDrop(drop->mimeData(), drop->possibleActions(), drop->modifiers());
        if (qEnvironmentVariableIsSet("VTUI_DEBUG") && event->type() != QEvent::DragMove)
            qInfo() << "QT_DND: native event" << event->type() << drop->position()
                    << "target" << side << "action" << action << "identity" << target.value("panelId");
        clearDropHighlight();
        if (target.isEmpty() || action == Qt::IgnoreAction
            || !m_panelSessions.catalog(side).dropAllowed) { drop->ignore(); return true; }
        if (event->type() != QEvent::Drop) {
            m_dragPanels[side]->setProperty("dropHoverIndex", target.value("isDir").toBool()
                && target.value("name").toString() != ".." ? target.value("index").toInt() : -1);
        } else {
            target.insert("action", "panel.dropFiles");
            target.insert("operation", "copy");
            if (!m_dragToken.isEmpty() && drop->mimeData()->data(sessionMime) == m_dragToken.toUtf8()) {
                target.insert("source", m_dragSource);
                if ((drop->modifiers() & Qt::ShiftModifier) && !(drop->modifiers() & Qt::ControlModifier))
                    target.insert("operation", "move");
            } else {
                QStringList paths;
                for (const auto &url : drop->mimeData()->urls()) paths.append(url.toLocalFile());
                target.insert("paths", paths);
            }
            emit uiActionRequested(target);
        }
        drop->setDropAction(action);
        drop->accept();
        return true;
    }
    if (m_nativeDragActive) return false;
    if (m_nativeDragStartupPending && event->type() == QEvent::KeyPress
        && static_cast<QKeyEvent *>(event)->key() == Qt::Key_Escape) {
        m_nativeDragStartupPending = false;
        m_dragArmedSide = -1;
        m_dragSource.clear();
        m_dragRequestId.clear();
        return true;
    }
    if (event->type() == QEvent::MouseButtonRelease || event->type() == QEvent::WindowDeactivate) {
        if (m_nativeDragStartupPending && event->type() == QEvent::MouseButtonRelease) {
            const auto *mouse = static_cast<QMouseEvent *>(event);
            if (mouse->button() == Qt::LeftButton
                && QGuiApplication::topLevelAt(mouse->globalPosition().toPoint()) == object)
                finishInternalDrop(object, mouse->position(), mouse->modifiers());
        }
        m_nativeDragStartupPending = false;
        m_dragArmedSide = -1;
        m_dragSource.clear();
        m_dragRequestId.clear();
    }
    if (event->type() == QEvent::MouseButtonPress) {
        auto *mouse = static_cast<QMouseEvent *>(event);
        m_dragArmedSide = -1;
        m_nativeDragStartupPending = false;
        m_dragSource.clear();
        m_dragPrepared = false;
        m_dragThresholdPassed = false;
        m_preparedDragUrls.clear();
        m_dragRequestId.clear();
        if (mouse->button() != Qt::LeftButton || mouse->modifiers() != Qt::NoModifier) return false;
        int side;
        auto source = dragHit(object, mouse->position(), &side);
        const auto id = source.value("entryId").toString();
        if (id.isEmpty() || source.value("name").toString() == "..") return false;
        const auto &state = m_panelSessions.catalog(side);
        QStringList ids;
        if (state.selectedEntryIds.contains(id)) ids = state.selectedEntryIdList;
        else if (state.selectedEntryIds.isEmpty()) ids.append(id);
        else return false;
        source.insert("entryIds", ids);
        m_dragSource = source;
        m_dragArmedSide = side;
        m_dragPress = mouse->position();
        // A complete local selection is already in the authoritative catalog.
        // Use it immediately for quick gestures; Go resolves off-page entries.
        if (state.sourceKind == "local") {
            for (const auto &id : ids) {
                for (const auto &v : state.entries) {
                    const auto row = v.toMap();
                    if (row.value("entryId").toString() != id) continue;
                    m_preparedDragUrls.append(QUrl::fromLocalFile(
                        QDir(state.currentPath).filePath(row.value("name").toString())));
                    break;
                }
            }
            m_dragPrepared = m_preparedDragUrls.size() == ids.size();
        }
        m_dragRequestId = QUuid::createUuid().toString(QUuid::WithoutBraces);
        auto request = source;
        request.insert("action", "panel.prepareDrag");
        request.insert("requestId", m_dragRequestId);
        emit uiActionRequested(request);
    }
    if (event->type() == QEvent::MouseMove && m_dragArmedSide >= 0) {
        auto *mouse = static_cast<QMouseEvent *>(event);
        if (!(mouse->buttons() & Qt::LeftButton)) { m_dragArmedSide = -1; return false; }
        if ((mouse->position() - m_dragPress).manhattanLength() < QGuiApplication::styleHints()->startDragDistance()) return false;
        m_dragThresholdPassed = true;
        startPreparedDrag();
        return true;
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
        const qreal dpr = window->devicePixelRatio();
        QPixmap preview(qCeil(220 * dpr), qCeil(36 * dpr));
        preview.fill(QColor(32, 36, 44, 235));
        QPainter painter(&preview);
        painter.setPen(Qt::white);
        QFont font = QGuiApplication::font();
        const qreal logicalSize = font.pixelSize() > 0 ? font.pixelSize() : font.pointSizeF() * 96.0 / 72.0;
        font.setPixelSize(qMax(1, qRound(logicalSize * dpr)));
        painter.setFont(font);
        const QString title = ids.size() == 1 ? m_dragSource.value("name").toString()
                                              : tr("%1 files").arg(ids.size());
        const QFontMetrics metrics(font);
        const int baseline = (preview.height() - metrics.height()) / 2 + metrics.ascent();
        painter.drawText(QPoint(qRound(10 * dpr), baseline),
                         metrics.elidedText(title, Qt::ElideMiddle, qRound(200 * dpr)));
        painter.end();
        preview.setDevicePixelRatio(dpr);
        drag.setPixmap(preview);
        drag.setHotSpot(QPoint(0, 0));
        // The desktop may only copy. Internal Shift-move is selected by our
        // receiver; a native move is never advertised to external receivers.
        Qt::DropAction finished = Qt::IgnoreAction;
#ifdef Q_OS_WIN
        bool missedRelease = false;
        int releasedPolls = 0;
        QTimer releaseWatchdog;
        releaseWatchdog.setInterval(75);
        connect(&releaseWatchdog, &QTimer::timeout, &drag, [&] {
            if (GetAsyncKeyState(VK_LBUTTON) & 0x8000) {
                releasedPolls = 0;
            } else if (++releasedPolls >= 2) {
                // Normal OLE completion returns before this grace period.
                missedRelease = true;
                releaseWatchdog.stop();
                QDrag::cancel();
            }
        });
        // Windows' OLE source learns its initial button state on its first
        // poll. If the physical release already happened while processing
        // queued input / preparing the preview, DoDragDrop never sees a
        // held button and waits for a second click. Complete only an internal
        // drop in that case; there is no desktop transaction to acknowledge.
        auto finishReleasedInternalDrop = [&] {
            if (m_nativeDragCancelled || (GetAsyncKeyState(VK_ESCAPE) & 0x8000)
                || QGuiApplication::topLevelAt(QCursor::pos()) != window) {
                if (qEnvironmentVariableIsSet("VTUI_DEBUG")) qInfo() << "QT_DND: internal finish wrong window/cancel" << m_nativeDragCancelled << QGuiApplication::topLevelAt(QCursor::pos()) << window;
                return;
            }
            if (finishInternalDrop(window, window->mapFromGlobal(QCursor::pos()), QGuiApplication::queryKeyboardModifiers())) {
                finished = Qt::CopyAction;
            }
        };
        if (!(GetAsyncKeyState(VK_LBUTTON) & 0x8000)) {
            finishReleasedInternalDrop();
        } else
#endif
        {
#ifdef Q_OS_WIN
            releaseWatchdog.start();
#endif
            finished = drag.exec(Qt::CopyAction, Qt::CopyAction);
#ifdef Q_OS_WIN
            releaseWatchdog.stop();
            if (!(GetAsyncKeyState(VK_LBUTTON) & 0x8000))
                m_nativeDragReleased = true;
            // Qt's Windows startup waits for a further WM_MOUSEMOVE. If the
            // async preparation consumed the last held-button move, it can
            // process the release and return E_FAIL before OLE ever starts.
            // Recover that observed release, never an Escape or rejected OLE drop.
            if ((missedRelease || nativeDragStartupWasReleased()) && finished == Qt::IgnoreAction)
                finishReleasedInternalDrop();
#endif
        }
        if (auto *grabber = window->mouseGrabberItem()) grabber->ungrabMouse();
        if (qEnvironmentVariableIsSet("VTUI_DEBUG"))
            qInfo() << "QT_DND: native drag finished" << finished << "cursor" << window->mapFromGlobal(QCursor::pos())
                    << "entered/released/cancelled" << m_nativeDragEntered << m_nativeDragReleased << m_nativeDragCancelled;
        m_nativeDragActive = false;
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
